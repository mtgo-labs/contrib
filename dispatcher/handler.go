package dispatcher

import "context"

// handlerKind identifies the category of update a handler processes.
type handlerKind string

const (
	kindNewMessage    handlerKind = "new_message"
	kindEditMessage   handlerKind = "edit_message"
	kindDeleteMessage handlerKind = "delete_message"
	kindCallbackQuery handlerKind = "callback_query"
	kindInlineQuery   handlerKind = "inline_query"
	kindChosenInline  handlerKind = "chosen_inline_result"
	kindChatMember    handlerKind = "chat_member"
	kindUserStatus    handlerKind = "user_status"
	kindUserTyping    handlerKind = "user_typing"
	kindPreCheckout   handlerKind = "pre_checkout_query"
	kindRaw           handlerKind = "raw"
)

// handler is a type-erased registered handler. The check function returns
// false to skip (filter did not match); callback runs the handler logic.
// Both operate on an opaque any value that is actually the typed context.
type handler struct {
	check    func(any) bool  // nil = always match
	callback func(any) error
}

// rawHandler is a simplified handler for raw updates (before parsing).
type rawHandler struct {
	callback func(ctx context.Context, update any) error
}

// RegisterOption configures handler registration.
type RegisterOption func(*registerConfig)

type registerConfig struct {
	group int
}

// WithGroup places the handler in the given priority group instead of the
// default group 0. Groups are iterated in ascending order.
func WithGroup(group int) RegisterOption {
	return func(c *registerConfig) { c.group = group }
}

// ErrorHandler is called when a handler's callback returns a non-nil error.
// If it returns false (or is nil), the error is logged.
type ErrorHandler func(err error, update any) bool

// RawHandlerFunc handles raw updates before they are parsed into typed
// contexts. The update parameter is a [tl.UpdatesClass].
type RawHandlerFunc func(ctx context.Context, update any) error
