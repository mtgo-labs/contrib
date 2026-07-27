package dispatcher

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"time"

	raw "github.com/mtgo-labs/raw"
	"github.com/mtgo-labs/raw/tl"
)

// baseContext is embedded by every typed update context. It carries the
// client, peers index, dispatch context, and propagation state.
type baseContext struct {
	client      *raw.Client
	peers       *PeersIndex
	ctx         context.Context
	propagation propagationAction
}

// Client returns the underlying raw client.
func (c *baseContext) Client() *raw.Client { return c.client }

// Peers returns the peers index built from the current update batch.
func (c *baseContext) Peers() *PeersIndex { return c.peers }

// Context returns the dispatch context (typically from Start).
func (c *baseContext) Context() context.Context { return c.ctx }

// Continue tries the next handler in the same group.
func (c *baseContext) Continue() { c.propagation = propagationContinue }

// Stop stops all groups in the current dispatcher. Child dispatchers still
// receive the update.
func (c *baseContext) Stop() { c.propagation = propagationStop }

// StopChildren stops all groups and skips child dispatchers entirely.
func (c *baseContext) StopChildren() { c.propagation = propagationStopChildren }
// setPropagation sets the propagation action (internal, used by dispatch loop).
func (c *baseContext) setPropagation(a propagationAction) { c.propagation = a }

// getPropagation returns the current propagation action (internal).
func (c *baseContext) getPropagation() propagationAction { return c.propagation }

// ---------------------------------------------------------------------------
// MessageContext
// ---------------------------------------------------------------------------

// ChatType classifies the chat where a message was sent.
type ChatType uint8

const (
	ChatTypePrivate ChatType = iota
	ChatTypeBasicGroup
	ChatTypeSupergroup
	ChatTypeChannel
	ChatTypeUnknown
)

// MessageContext wraps a new or edited message update. It is the context
// type for OnNewMessage and OnEditMessage handlers.
type MessageContext struct {
	baseContext
	Message *tl.Message
	Edited  bool
}

// Text returns the message text (the Message field of the underlying TL
// message). For media-only messages this is empty.
func (c *MessageContext) Text() string {
	if c.Message == nil {
		return ""
	}
	return c.Message.Message
}

// MessageID returns the TL message ID.
func (c *MessageContext) MessageID() int32 {
	if c.Message == nil {
		return 0
	}
	return c.Message.ID
}

// Date returns the message timestamp as a [time.Time].
func (c *MessageContext) Date() time.Time {
	if c.Message == nil {
		return time.Time{}
	}
	return time.Unix(int64(c.Message.Date), 0)
}

// PeerID returns the PeerID of the underlying message as a [tl.PeerClass].
func (c *MessageContext) PeerID() tl.PeerClass {
	if c.Message == nil {
		return nil
	}
	return c.Message.PeerID
}

// SenderID extracts the sender's user/bot ID from the message's FromID. For
// messages without an explicit FromID (e.g., channel posts), the channel ID
// is returned.
func (c *MessageContext) SenderID() int64 {
	if c.Message == nil {
		return 0
	}
	if c.Message.FromID != nil {
		if u, ok := c.Message.FromID.(*tl.PeerUser); ok {
			return u.UserID
		}
	}
	// Fall back to the chat/channel itself (channel posts).
	if c.Message.PeerID != nil {
		if ch, ok := c.Message.PeerID.(*tl.PeerChannel); ok {
			return ch.ChannelID
		}
	}
	return 0
}

// ChatID returns the peer ID (user/chat/channel) and its [ChatType] for the
// conversation this message belongs to.
func (c *MessageContext) ChatID() (int64, ChatType) {
	if c.Message == nil || c.Message.PeerID == nil {
		return 0, ChatTypeUnknown
	}
	switch peer := c.Message.PeerID.(type) {
	case *tl.PeerUser:
		return peer.UserID, ChatTypePrivate
	case *tl.PeerChat:
		return peer.ChatID, ChatTypeBasicGroup
	case *tl.PeerChannel:
		info := c.peers.Channel(peer.ChannelID)
		if info != nil && info.Type == PeerTypeChannel {
			// Distinguish channel from supergroup via the Channel struct.
			return peer.ChannelID, ChatTypeSupergroup
		}
		return peer.ChannelID, ChatTypeSupergroup
	}
	return 0, ChatTypeUnknown
}

// IsPrivate returns true if the message is in a 1:1 chat.
func (c *MessageContext) IsPrivate() bool {
	_, typ := c.ChatID()
	return typ == ChatTypePrivate
}

// IsGroup returns true if the message is in a basic group or supergroup.
func (c *MessageContext) IsGroup() bool {
	_, typ := c.ChatID()
	return typ == ChatTypeBasicGroup || typ == ChatTypeSupergroup
}

// IsChannel returns true if the message is in a channel (broadcast).
func (c *MessageContext) IsChannel() bool {
	_, typ := c.ChatID()
	return typ == ChatTypeChannel
}

// IsOutgoing returns true if the message was sent by the current account.
func (c *MessageContext) IsOutgoing() bool {
	return c.Message != nil && c.Message.Out
}

// inputPeerForChat constructs an InputPeer for the message's chat using
// cached access hashes. Returns nil if access hash is unavailable.
func (c *MessageContext) inputPeerForChat() tl.InputPeerClass {
	if c.Message == nil || c.Message.PeerID == nil {
		return nil
	}
	return c.peers.InputPeer(c.Message.PeerID)
}

// Reply sends a text message to the same chat as the received message. It
// uses the cached peers index to resolve access hashes. For private chats
// where no access hash is cached (e.g., from a short-message update), the
// call will fail.
func (c *MessageContext) Reply(text string) error {
	return c.ReplyWith(text, false)
}

// ReplyWith is like Reply but allows sending silently.
func (c *MessageContext) ReplyWith(text string, silent bool) error {
	peer := c.inputPeerForChat()
	if peer == nil {
		return ErrPeerNotResolved
	}
	req := &tl.MessagesSendMessageRequest{
		Peer:     peer,
		Message:  text,
		RandomID: randomID(),
		Silent:   silent,
	}
	_, err := raw.Invoke(c.ctx, c.client, req)
	return err
}

// AnswerCallback answers the callback query associated with this message
// (if this context originated from a callback query update). For message
// updates, use [MessageContext.Reply] instead.
func (c *MessageContext) AnswerCallback(text string) error {
	return c.Reply(text)
}

// ---------------------------------------------------------------------------
// CallbackQueryContext
// ---------------------------------------------------------------------------

// CallbackQueryContext wraps a bot callback query update. The Data field
// contains the payload set by the inline keyboard button.
type CallbackQueryContext struct {
	baseContext
	Query    *tl.UpdateBotCallbackQuery
	Inline   bool
	InlineQ  *tl.UpdateInlineBotCallbackQuery
}

// QueryID returns the callback query ID.
func (c *CallbackQueryContext) QueryID() int64 {
	if c.Inline && c.InlineQ != nil {
		return c.InlineQ.QueryID
	}
	if c.Query != nil {
		return c.Query.QueryID
	}
	return 0
}

// UserID returns the user who pressed the button.
func (c *CallbackQueryContext) UserID() int64 {
	if c.Inline && c.InlineQ != nil {
		return c.InlineQ.UserID
	}
	if c.Query != nil {
		return c.Query.UserID
	}
	return 0
}

// DataString returns the callback data as a string.
func (c *CallbackQueryContext) DataString() string {
	return string(c.DataBytes())
}

// DataBytes returns the raw callback data bytes.
func (c *CallbackQueryContext) DataBytes() []byte {
	if c.Inline && c.InlineQ != nil {
		return c.InlineQ.Data
	}
	if c.Query != nil {
		return c.Query.Data
	}
	return nil
}

// MessageID returns the message ID the callback is attached to.
func (c *CallbackQueryContext) MessageID() int32 {
	if c.Query != nil {
		return c.Query.MessageID
	}
	return 0
}

// Answer sends a callback answer (the popup/alert shown to the user).
func (c *CallbackQueryContext) Answer(text string) error {
	return c.AnswerWith(text, false)
}

// AnswerWith sends a callback answer with an optional alert.
func (c *CallbackQueryContext) AnswerWith(text string, alert bool) error {
	qid := c.QueryID()
	msg := text
	req := &tl.MessagesSetBotCallbackAnswerRequest{
		Alert:     alert,
		QueryID:   qid,
		Message:   &msg,
		CacheTime: 0,
	}
	_, err := raw.Invoke(c.ctx, c.client, req)
	return err
}

// ---------------------------------------------------------------------------
// InlineQueryContext
// ---------------------------------------------------------------------------

// InlineQueryContext wraps an inline bot query update.
type InlineQueryContext struct {
	baseContext
	Query *tl.UpdateBotInlineQuery
}

// QueryID returns the inline query ID.
func (c *InlineQueryContext) QueryID() int64 {
	if c.Query == nil {
		return 0
	}
	return c.Query.QueryID
}

// QueryText returns the user's search text.
func (c *InlineQueryContext) QueryText() string {
	if c.Query == nil {
		return ""
	}
	return c.Query.Query
}

// UserID returns the user who issued the query.
func (c *InlineQueryContext) UserID() int64 {
	if c.Query == nil {
		return 0
	}
	return c.Query.UserID
}

// Answer sends inline query results to the user.
func (c *InlineQueryContext) Answer(results []tl.InputBotInlineResultClass) error {
	req := &tl.MessagesSetInlineBotResultsRequest{
		QueryID: c.QueryID(),
		Results: results,
	}
	_, err := raw.Invoke(c.ctx, c.client, req)
	return err
}

// ---------------------------------------------------------------------------
// DeleteMessageContext
// ---------------------------------------------------------------------------

// DeleteMessageContext wraps a message deletion update.
type DeleteMessageContext struct {
	baseContext
	Update    *tl.UpdateDeleteMessages
	ChannelID *int64 // set when the update is from a channel
	Messages  []int32
}

// ---------------------------------------------------------------------------
// ChatMemberContext
// ---------------------------------------------------------------------------

// ChatMemberContext wraps a chat member update (join, leave, ban, etc.).
type ChatMemberContext struct {
	baseContext
	Update *tl.UpdateChannelParticipant
}

// ChannelID returns the chat/channel ID.
func (c *ChatMemberContext) ChannelID() int64 {
	if c.Update == nil {
		return 0
	}
	return c.Update.ChannelID
}

// UserID returns the affected user's ID.
func (c *ChatMemberContext) UserID() int64 {
	if c.Update == nil {
		return 0
	}
	return c.Update.UserID
}

// ---------------------------------------------------------------------------
// UserStatusContext
// ---------------------------------------------------------------------------

// UserStatusContext wraps a user status update (online/offline).
type UserStatusContext struct {
	baseContext
	UserID int64
	Status tl.UserStatusClass
}

// ---------------------------------------------------------------------------
// UserTypingContext
// ---------------------------------------------------------------------------

// UserTypingContext wraps a user typing update.
type UserTypingContext struct {
	baseContext
	UserID  int64
	ChatID  int64
	Action  tl.SendMessageActionClass
}

// ---------------------------------------------------------------------------
// PreCheckoutContext
// ---------------------------------------------------------------------------

// PreCheckoutContext wraps a pre-checkout query for payments.
type PreCheckoutContext struct {
	baseContext
	Query *tl.UpdateBotPrecheckoutQuery
}

// QueryID returns the pre-checkout query ID.
func (c *PreCheckoutContext) QueryID() int64 {
	if c.Query == nil {
		return 0
	}
	return c.Query.QueryID
}

// UserID returns the buyer's user ID.
func (c *PreCheckoutContext) UserID() int64 {
	if c.Query == nil {
		return 0
	}
	return c.Query.UserID
}

// AnswerOk signals that the pre-checkout is accepted.
func (c *PreCheckoutContext) AnswerOk() error {
	req := &tl.MessagesSetBotPrecheckoutResultsRequest{
		Success: true,
		QueryID: c.QueryID(),
	}
	_, err := raw.Invoke(c.ctx, c.client, req)
	return err
}

// AnswerFail signals a pre-checkout failure with an error message.
func (c *PreCheckoutContext) AnswerFail(errMsg string) error {
	req := &tl.MessagesSetBotPrecheckoutResultsRequest{
		QueryID: c.QueryID(),
		Error:   &errMsg,
	}
	_, err := raw.Invoke(c.ctx, c.client, req)
	return err
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// randomID generates a random int64 for message RandomID fields.
func randomID() int64 {
	var buf [8]byte
	_, _ = rand.Read(buf[:])
	return int64(binary.LittleEndian.Uint64(buf[:]))
}

// ErrPeerNotResolved is returned when an access hash is needed to send an
// RPC call but is not available in the peers index.
var ErrPeerNotResolved = peerNotResolvedError{}

type peerNotResolvedError struct{}

func (peerNotResolvedError) Error() string {
	return "dispatcher: peer access hash not resolved (not in peers index)"
}

