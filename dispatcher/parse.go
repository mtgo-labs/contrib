package dispatcher

import (
	"context"

	raw "github.com/mtgo-labs/raw"
	"github.com/mtgo-labs/raw/tl"
)

// parsedUpdate is a classified update ready for dispatch. The context field
// holds the typed context (*MessageContext, *CallbackQueryContext, etc.)
// and kind identifies which handler list it targets.
type parsedUpdate struct {
	kind    handlerKind
	context any // typed context embedding baseContext
}

// parseUpdates unpacks a [tl.UpdatesClass] into a batch of parsed updates
// and a peers index. Container types (Updates, UpdatesCombined) carry users
// and chats that populate the index. Short-message variants are synthesized
// into full [tl.Message] objects.
//
// The client and ctx are threaded into each context for RPC convenience
// methods (Reply, Answer, etc.).
func parseUpdates(ctx context.Context, client *raw.Client, update tl.UpdatesClass) ([]parsedUpdate, *PeersIndex) {
	peers := NewPeersIndex()

	switch u := update.(type) {
	case *tl.UpdateShort:
		return classifyUpdates(ctx, client, peers, []tl.UpdateClass{u.Update}), peers

	case *tl.Updates:
		peers.AddUsers(u.Users)
		peers.AddChats(u.Chats)
		return classifyUpdates(ctx, client, peers, u.Updates), peers

	case *tl.UpdatesCombined:
		peers.AddUsers(u.Users)
		peers.AddChats(u.Chats)
		return classifyUpdates(ctx, client, peers, u.Updates), peers

	case *tl.UpdateShortMessage:
		msg := shortMessageToMessage(u)
		return []parsedUpdate{{
			kind: kindNewMessage,
			context: &MessageContext{
				baseContext: baseContext{client: client, peers: peers, ctx: ctx},
				Message:     msg,
			},
		}}, peers

	case *tl.UpdateShortChatMessage:
		msg := shortChatMessageToMessage(u)
		return []parsedUpdate{{
			kind: kindNewMessage,
			context: &MessageContext{
				baseContext: baseContext{client: client, peers: peers, ctx: ctx},
				Message:     msg,
			},
		}}, peers

	case *tl.UpdateShortSentMessage:
		// Sent-message confirmations are rarely useful for dispatch; skip.
		return nil, peers

	case *tl.UpdatesTooLong:
		// Gap in the update sequence; nothing to dispatch.
		return nil, peers

	default:
		return nil, peers
	}
}

// classifyUpdates maps each individual [tl.UpdateClass] to a typed context
// and handler kind. Unknown update types are silently skipped (the raw
// handler still receives them before parsing).
func classifyUpdates(ctx context.Context, client *raw.Client, peers *PeersIndex, updates []tl.UpdateClass) []parsedUpdate {
	var result []parsedUpdate
	for _, upd := range updates {
		if pu, ok := classifyOne(ctx, client, peers, upd); ok {
			result = append(result, pu)
		}
	}
	return result
}

func classifyOne(ctx context.Context, client *raw.Client, peers *PeersIndex, upd tl.UpdateClass) (parsedUpdate, bool) {
	base := baseContext{client: client, peers: peers, ctx: ctx}

	switch u := upd.(type) {
	// --- New messages ---
	case *tl.UpdateNewMessage:
		if msg, ok := unwrapMessage(u.Message); ok {
			return parsedUpdate{
				kind:    kindNewMessage,
				context: &MessageContext{baseContext: base, Message: msg},
			}, true
		}
	case *tl.UpdateNewChannelMessage:
		if msg, ok := unwrapMessage(u.Message); ok {
			return parsedUpdate{
				kind:    kindNewMessage,
				context: &MessageContext{baseContext: base, Message: msg},
			}, true
		}

	// --- Edited messages ---
	case *tl.UpdateEditMessage:
		if msg, ok := unwrapMessage(u.Message); ok {
			return parsedUpdate{
				kind:    kindEditMessage,
				context: &MessageContext{baseContext: base, Message: msg, Edited: true},
			}, true
		}
	case *tl.UpdateEditChannelMessage:
		if msg, ok := unwrapMessage(u.Message); ok {
			return parsedUpdate{
				kind:    kindEditMessage,
				context: &MessageContext{baseContext: base, Message: msg, Edited: true},
			}, true
		}

	// --- Delete messages ---
	case *tl.UpdateDeleteMessages:
		return parsedUpdate{
			kind: kindDeleteMessage,
			context: &DeleteMessageContext{
				baseContext: base,
				Update:      u,
				Messages:    u.Messages,
			},
		}, true
	case *tl.UpdateDeleteChannelMessages:
		return parsedUpdate{
			kind: kindDeleteMessage,
			context: &DeleteMessageContext{
				baseContext: base,
				Messages:    u.Messages,
			},
		}, true

	// --- Callback queries ---
	case *tl.UpdateBotCallbackQuery:
		return parsedUpdate{
			kind:    kindCallbackQuery,
			context: &CallbackQueryContext{baseContext: base, Query: u},
		}, true
	case *tl.UpdateInlineBotCallbackQuery:
		return parsedUpdate{
			kind:    kindCallbackQuery,
			context: &CallbackQueryContext{baseContext: base, Inline: true, InlineQ: u},
		}, true

	// --- Inline queries ---
	case *tl.UpdateBotInlineQuery:
		return parsedUpdate{
			kind:    kindInlineQuery,
			context: &InlineQueryContext{baseContext: base, Query: u},
		}, true
	case *tl.UpdateBotInlineSend:
		// Chosen inline result — could add a dedicated context if needed.
		return parsedUpdate{}, false

	// --- Chat member ---
	case *tl.UpdateChannelParticipant:
		return parsedUpdate{
			kind:    kindChatMember,
			context: &ChatMemberContext{baseContext: base, Update: u},
		}, true

	// --- Typing ---
	case *tl.UpdateUserTyping:
		return parsedUpdate{
			kind: kindUserTyping,
			context: &UserTypingContext{
				baseContext: base,
				UserID:      u.UserID,
				Action:      u.Action,
			},
		}, true
	case *tl.UpdateChatUserTyping:
		return parsedUpdate{
			kind: kindUserTyping,
			context: &UserTypingContext{
				baseContext: base,
				ChatID:      u.ChatID,
				Action:      u.Action,
			},
		}, true
	case *tl.UpdateChannelUserTyping:
		return parsedUpdate{
			kind: kindUserTyping,
			context: &UserTypingContext{
				baseContext: base,
				ChatID:      u.ChannelID,
				Action:      u.Action,
			},
		}, true

	// --- Pre-checkout ---
	case *tl.UpdateBotPrecheckoutQuery:
		return parsedUpdate{
			kind:    kindPreCheckout,
			context: &PreCheckoutContext{baseContext: base, Query: u},
		}, true

	// --- Other update types that could be added: poll, poll_vote,
	//     user_status, bot_stopped, story, etc. ---
	}

	return parsedUpdate{}, false
}

// unwrapMessage extracts a *tl.Message from a [tl.MessageClass], returning
// false for service messages or empty messages.
func unwrapMessage(msg tl.MessageClass) (*tl.Message, bool) {
	if msg == nil {
		return nil, false
	}
	m, ok := msg.(*tl.Message)
	if !ok || m == nil {
		return nil, false
	}
	return m, true
}

// shortMessageToMessage synthesizes a full [tl.Message] from an
// [tl.UpdateShortMessage], which represents a private-chat message without
// a container. The peer is set to the sender's user ID.
func shortMessageToMessage(u *tl.UpdateShortMessage) *tl.Message {
	return &tl.Message{
		Out:         u.Out,
		MediaUnread: u.MediaUnread,
		Silent:      u.Silent,
		ID:          u.ID,
		PeerID:      &tl.PeerUser{UserID: u.UserID},
		FwdFrom:     u.FwdFrom,
		ViaBotID:    u.ViaBotID,
		ReplyTo:     u.ReplyTo,
		Date:        u.Date,
		Message:     u.Message,
		Entities:    u.Entities,
		TTLPeriod:   u.TTLPeriod,
	}
}

// shortChatMessageToMessage synthesizes a full [tl.Message] from an
// [tl.UpdateShortChatMessage], which represents a basic-group message
// without a container.
func shortChatMessageToMessage(u *tl.UpdateShortChatMessage) *tl.Message {
	return &tl.Message{
		Out:         u.Out,
		MediaUnread: u.MediaUnread,
		Silent:      u.Silent,
		ID:          u.ID,
		FromID:      &tl.PeerUser{UserID: u.FromID},
		PeerID:      &tl.PeerChat{ChatID: u.ChatID},
		FwdFrom:     u.FwdFrom,
		ViaBotID:    u.ViaBotID,
		ReplyTo:     u.ReplyTo,
		Date:        u.Date,
		Message:     u.Message,
		Entities:    u.Entities,
		TTLPeriod:   u.TTLPeriod,
	}
}
