// Package filters provides built-in [dispatcher.Filter] predicates for
// common update-matching patterns.
//
// Filters are composable using [dispatcher.And], [dispatcher.Or], and
// [dispatcher.Not] from the parent package.
//
//	dp.OnNewMessage(
//	    dispatcher.And(
//	        filters.Command("start"),
//	        filters.Private,
//	    ),
//	    handler,
//	)
package filters

import (
	"regexp"
	"strings"

	"github.com/mtgo-labs/contrib/dispatcher"
)

// --- Message filters -------------------------------------------------------

// Command matches a bot command (e.g., "/start"). The prefix defaults to "/"
// if none is given. Multiple prefixes can be provided for multi-language
// bots (e.g., "/", "!"). The match is case-insensitive on the command name.
//
//	dp.OnNewMessage(filters.Command("start"), handler)
//	dp.OnNewMessage(filters.Command("help", "!"), handler)
func Command(cmd string, prefixes ...string) dispatcher.Filter[*dispatcher.MessageContext] {
	if len(prefixes) == 0 {
		prefixes = []string{"/"}
	}
	cmdLower := strings.ToLower(cmd)
	return func(ctx *dispatcher.MessageContext) bool {
		text := ctx.Text()
		for _, prefix := range prefixes {
			if !strings.HasPrefix(text, prefix) {
				continue
			}
			rest := text[len(prefix):]
			// Strip @botname suffix.
			if at := strings.IndexByte(rest, '@'); at >= 0 {
				rest = rest[:at]
			}
			if strings.ToLower(rest) == cmdLower {
				return true
			}
		}
		return false
	}
}

// CommandPrefix matches any command with the given prefix(es), defaulting to
// "/". Useful for grouping commands under a handler.
func CommandPrefix(prefixes ...string) dispatcher.Filter[*dispatcher.MessageContext] {
	if len(prefixes) == 0 {
		prefixes = []string{"/"}
	}
	return func(ctx *dispatcher.MessageContext) bool {
		text := ctx.Text()
		for _, prefix := range prefixes {
			if strings.HasPrefix(text, prefix) {
				return true
			}
		}
		return false
	}
}

// Regexp matches a message whose text matches the given compiled regexp.
func Regexp(re *regexp.Regexp) dispatcher.Filter[*dispatcher.MessageContext] {
	return func(ctx *dispatcher.MessageContext) bool {
		return re.MatchString(ctx.Text())
	}
}

// Text matches a message whose text equals s exactly (case-sensitive).
func Text(s string) dispatcher.Filter[*dispatcher.MessageContext] {
	return func(ctx *dispatcher.MessageContext) bool {
		return ctx.Text() == s
	}
}

// TextIgnoreCase matches a message whose text equals s, ignoring case.
func TextIgnoreCase(s string) dispatcher.Filter[*dispatcher.MessageContext] {
	want := strings.ToLower(s)
	return func(ctx *dispatcher.MessageContext) bool {
		return strings.ToLower(ctx.Text()) == want
	}
}

// HasPrefix matches a message whose text starts with prefix.
func HasPrefix(prefix string) dispatcher.Filter[*dispatcher.MessageContext] {
	return func(ctx *dispatcher.MessageContext) bool {
		return strings.HasPrefix(ctx.Text(), prefix)
	}
}

// HasSuffix matches a message whose text ends with suffix.
func HasSuffix(suffix string) dispatcher.Filter[*dispatcher.MessageContext] {
	return func(ctx *dispatcher.MessageContext) bool {
		return strings.HasSuffix(ctx.Text(), suffix)
	}
}

// Contains matches a message whose text contains substr.
func Contains(substr string) dispatcher.Filter[*dispatcher.MessageContext] {
	return func(ctx *dispatcher.MessageContext) bool {
		return strings.Contains(ctx.Text(), substr)
	}
}

// Private matches messages from private (1:1) chats.
func Private(ctx *dispatcher.MessageContext) bool {
	return ctx.IsPrivate()
}

// Group matches messages from basic groups or supergroups.
func Group(ctx *dispatcher.MessageContext) bool {
	return ctx.IsGroup()
}

// Channel matches messages from channels (broadcasts).
func Channel(ctx *dispatcher.MessageContext) bool {
	return ctx.IsChannel()
}

// ChatID matches messages from a specific chat ID (user, chat, or channel).
func ChatID(id int64) dispatcher.Filter[*dispatcher.MessageContext] {
	return func(ctx *dispatcher.MessageContext) bool {
		chatID, _ := ctx.ChatID()
		return chatID == id
	}
}

// SenderID matches messages sent by a specific user ID.
func SenderID(id int64) dispatcher.Filter[*dispatcher.MessageContext] {
	return func(ctx *dispatcher.MessageContext) bool {
		return ctx.SenderID() == id
	}
}

// Incoming matches messages that were not sent by the current account.
func Incoming(ctx *dispatcher.MessageContext) bool {
	return !ctx.IsOutgoing()
}

// Outgoing matches messages sent by the current account.
func Outgoing(ctx *dispatcher.MessageContext) bool {
	return ctx.IsOutgoing()
}

// --- Callback query filters ------------------------------------------------

// CallbackData matches a callback query whose data equals data (as a string).
func CallbackData(data string) dispatcher.Filter[*dispatcher.CallbackQueryContext] {
	return func(ctx *dispatcher.CallbackQueryContext) bool {
		return ctx.DataString() == data
	}
}

// CallbackDataPrefix matches a callback query whose data starts with prefix.
func CallbackDataPrefix(prefix string) dispatcher.Filter[*dispatcher.CallbackQueryContext] {
	return func(ctx *dispatcher.CallbackQueryContext) bool {
		return strings.HasPrefix(ctx.DataString(), prefix)
	}
}

// CallbackUser matches a callback query from a specific user ID.
func CallbackUser(id int64) dispatcher.Filter[*dispatcher.CallbackQueryContext] {
	return func(ctx *dispatcher.CallbackQueryContext) bool {
		return ctx.UserID() == id
	}
}

// --- Inline query filters --------------------------------------------------

// QueryText matches an inline query whose text equals s.
func QueryText(s string) dispatcher.Filter[*dispatcher.InlineQueryContext] {
	return func(ctx *dispatcher.InlineQueryContext) bool {
		return ctx.QueryText() == s
	}
}

// QueryPrefix matches an inline query whose text starts with prefix.
func QueryPrefix(prefix string) dispatcher.Filter[*dispatcher.InlineQueryContext] {
	return func(ctx *dispatcher.InlineQueryContext) bool {
		return strings.HasPrefix(ctx.QueryText(), prefix)
	}
}
