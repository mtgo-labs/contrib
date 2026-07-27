package dispatcher

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"

	raw "github.com/mtgo-labs/raw"
	"github.com/mtgo-labs/raw/tl"
)

// Dispatcher consumes raw updates from a [raw.Client] and routes them to
// registered handlers through a priority-group system with composable
// filters and propagation control.
//
// A zero-value Dispatcher is not usable; create one with [New] or [NewChild].
type Dispatcher struct {
	mu sync.Mutex

	client *raw.Client
	parent *Dispatcher

	// groups maps group number → handler-kind → handlers.
	groups     map[int]map[handlerKind][]handler
	groupOrder []int

	// rawHandlers receive every update before parsing.
	rawHandlers []RawHandlerFunc

	children []*Dispatcher

	// errorHandler is called when a handler returns a non-nil error.
	errorHandler ErrorHandler

	// log receives errors that have no error handler or that the error
	// handler declines to handle.
	log *slog.Logger

	// Lifecycle.
	cancel  context.CancelFunc
	done    chan struct{}
	startMu sync.Mutex
	running bool
}

// New creates a Dispatcher bound to client. The dispatcher does not start
// consuming updates until [Dispatcher.Start] is called.
func New(client *raw.Client) *Dispatcher {
	return &Dispatcher{
		client: client,
		groups: make(map[int]map[handlerKind][]handler),
		log:    slog.Default(),
	}
}

// NewChild creates a child dispatcher. Children are not bound to a client
// directly; they inherit the client from their parent when added via
// [Dispatcher.AddChild].
func NewChild() *Dispatcher {
	return &Dispatcher{
		groups: make(map[int]map[handlerKind][]handler),
		log:    slog.Default(),
	}
}

// SetLogger overrides the default slog logger.
func (d *Dispatcher) SetLogger(l *slog.Logger) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.log = l
}

// SetErrorHandler sets a custom error handler invoked when any handler's
// callback returns a non-nil error. If the error handler returns false (or
// is unset), the error is logged.
func (d *Dispatcher) SetErrorHandler(h ErrorHandler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.errorHandler = h
}

// ---------------------------------------------------------------------------
// Start / Stop
// ---------------------------------------------------------------------------

// Start begins consuming updates from the bound client's Updates channel.
// It blocks until ctx is cancelled or the client's update channel is closed.
// Call Start in a goroutine:
//
//	go dp.Start(ctx)
//
// Calling Start on a child dispatcher (created with [NewChild]) is an error;
// children are dispatched automatically by their parent.
func (d *Dispatcher) Start(ctx context.Context) error {
	d.startMu.Lock()
	if d.client == nil {
		d.startMu.Unlock()
		return ErrNoClient
	}
	if d.running {
		d.startMu.Unlock()
		return ErrAlreadyRunning
	}
	d.running = true
	d.startMu.Unlock()

	runCtx, cancel := context.WithCancel(ctx)
	d.mu.Lock()
	d.cancel = cancel
	d.done = make(chan struct{})
	updates := d.client.Updates()
	log := d.log
	d.mu.Unlock()

	defer func() {
		d.mu.Lock()
		d.cancel = nil
		close(d.done)
		d.running = false
		d.mu.Unlock()
	}()

	for {
		select {
		case <-runCtx.Done():
			return runCtx.Err()
		case update, ok := <-updates:
			if !ok {
				// Client closed its update channel.
				return nil
			}
			d.processUpdate(runCtx, update, log)
		}
	}
}

// Stop signals the dispatcher to stop consuming updates. It blocks until
// the update loop has exited.
func (d *Dispatcher) Stop() {
	d.mu.Lock()
	cancel := d.cancel
	done := d.done
	d.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

// Done returns a channel that is closed when the dispatcher has stopped.
func (d *Dispatcher) Done() <-chan struct{} {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.done == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return d.done
}

// ---------------------------------------------------------------------------
// Update processing
// ---------------------------------------------------------------------------

func (d *Dispatcher) processUpdate(ctx context.Context, update tl.UpdatesClass, log *slog.Logger) {
	d.mu.Lock()
	rawHandlers := d.rawHandlers
	d.mu.Unlock()

	// Phase 1: raw handlers (before parsing).
	for _, rh := range rawHandlers {
		if err := rh(ctx, update); err != nil {
			d.handleError(err, update, log)
		}
	}

	// Phase 2: parse into typed events.
	events, _ := parseUpdates(ctx, d.client, update)
	for _, event := range events {
		d.dispatch(ctx, event, log)
	}
}

// dispatch routes a single parsed update through this dispatcher's handler
// groups, then through children.
func (d *Dispatcher) dispatch(ctx context.Context, event parsedUpdate, log *slog.Logger) {
	d.dispatchSelf(ctx, event, log)

	// If the handler flagged StopChildren, skip children entirely.
	if getPropagation(event.context) == propagationStopChildren {
		return
	}

	d.mu.Lock()
	children := make([]*Dispatcher, len(d.children))
	copy(children, d.children)
	d.mu.Unlock()

	for _, child := range children {
		// Re-check between children in case a sibling set StopChildren.
		if getPropagation(event.context) == propagationStopChildren {
			return
		}
		child.dispatch(ctx, event, log)
	}
}

// dispatchSelf runs the update through this dispatcher's handler groups only
// (no children). Returns true if any handler matched.
func (d *Dispatcher) dispatchSelf(ctx context.Context, event parsedUpdate, log *slog.Logger) bool {
	d.mu.Lock()
	groupOrder := d.groupOrder
	// Copy group data to avoid holding the lock during callbacks.
	groupsCopy := make(map[int][]handler, len(groupOrder))
	for _, g := range groupOrder {
		if handlers, ok := d.groups[g][event.kind]; ok {
			groupsCopy[g] = handlers
		}
	}
	d.mu.Unlock()

	handled := false

	for _, groupNum := range groupOrder {
		handlers, ok := groupsCopy[groupNum]
		if !ok {
			continue
		}
		for _, h := range handlers {
			if h.check != nil && !h.check(event.context) {
				continue // filter did not match
			}
			handled = true

			// Reset propagation before running.
			resetPropagation(event.context)

			if err := h.callback(event.context); err != nil {
				d.handleError(err, event.context, log)
			}

			switch getPropagation(event.context) {
			case propagationContinue:
				continue // next handler in same group
			case propagationStop:
				return true // stop all groups in self
			case propagationStopChildren:
				return true // stop all groups + signal children skip
			}
			break // default: stop in this group, move to next group
		}
	}

	return handled
}

func (d *Dispatcher) handleError(err error, update any, log *slog.Logger) {
	d.mu.Lock()
	eh := d.errorHandler
	d.mu.Unlock()

	if eh != nil && eh(err, update) {
		return
	}
	log.Error("dispatcher handler error", "error", err, "update_type", typeName(update))
}

// ---------------------------------------------------------------------------
// Handler registration
// ---------------------------------------------------------------------------

func (d *Dispatcher) addHandler(kind handlerKind, h handler, opts ...RegisterOption) {
	cfg := registerConfig{group: 0}
	for _, opt := range opts {
		opt(&cfg)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.groups[cfg.group] == nil {
		d.groups[cfg.group] = make(map[handlerKind][]handler)
		d.groupOrder = append(d.groupOrder, cfg.group)
		sort.Ints(d.groupOrder)
	}
	d.groups[cfg.group][kind] = append(d.groups[cfg.group][kind], h)
}

// OnNewMessage registers a handler for new messages.
func (d *Dispatcher) OnNewMessage(filter Filter[*MessageContext], cb func(*MessageContext) error, opts ...RegisterOption) {
	h := handler{
		check:    wrapCheck(filter),
		callback: wrapCallback(cb),
	}
	d.addHandler(kindNewMessage, h, opts...)
}

// OnEditMessage registers a handler for edited messages.
func (d *Dispatcher) OnEditMessage(filter Filter[*MessageContext], cb func(*MessageContext) error, opts ...RegisterOption) {
	h := handler{
		check:    wrapCheck(filter),
		callback: wrapCallback(cb),
	}
	d.addHandler(kindEditMessage, h, opts...)
}

// OnDeleteMessage registers a handler for message deletions.
func (d *Dispatcher) OnDeleteMessage(filter Filter[*DeleteMessageContext], cb func(*DeleteMessageContext) error, opts ...RegisterOption) {
	h := handler{
		check:    wrapCheck(filter),
		callback: wrapCallback(cb),
	}
	d.addHandler(kindDeleteMessage, h, opts...)
}

// OnCallbackQuery registers a handler for callback queries (inline button presses).
func (d *Dispatcher) OnCallbackQuery(filter Filter[*CallbackQueryContext], cb func(*CallbackQueryContext) error, opts ...RegisterOption) {
	h := handler{
		check:    wrapCheck(filter),
		callback: wrapCallback(cb),
	}
	d.addHandler(kindCallbackQuery, h, opts...)
}

// OnInlineQuery registers a handler for inline queries.
func (d *Dispatcher) OnInlineQuery(filter Filter[*InlineQueryContext], cb func(*InlineQueryContext) error, opts ...RegisterOption) {
	h := handler{
		check:    wrapCheck(filter),
		callback: wrapCallback(cb),
	}
	d.addHandler(kindInlineQuery, h, opts...)
}

// OnChatMember registers a handler for chat member updates.
func (d *Dispatcher) OnChatMember(filter Filter[*ChatMemberContext], cb func(*ChatMemberContext) error, opts ...RegisterOption) {
	h := handler{
		check:    wrapCheck(filter),
		callback: wrapCallback(cb),
	}
	d.addHandler(kindChatMember, h, opts...)
}

// OnUserTyping registers a handler for typing updates.
func (d *Dispatcher) OnUserTyping(filter Filter[*UserTypingContext], cb func(*UserTypingContext) error, opts ...RegisterOption) {
	h := handler{
		check:    wrapCheck(filter),
		callback: wrapCallback(cb),
	}
	d.addHandler(kindUserTyping, h, opts...)
}

// OnPreCheckout registers a handler for payment pre-checkout queries.
func (d *Dispatcher) OnPreCheckout(filter Filter[*PreCheckoutContext], cb func(*PreCheckoutContext) error, opts ...RegisterOption) {
	h := handler{
		check:    wrapCheck(filter),
		callback: wrapCallback(cb),
	}
	d.addHandler(kindPreCheckout, h, opts...)
}

// OnRawUpdate registers a raw handler that receives every [tl.UpdatesClass]
// before it is parsed into typed events. Multiple raw handlers are called in
// registration order.
func (d *Dispatcher) OnRawUpdate(cb RawHandlerFunc) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.rawHandlers = append(d.rawHandlers, cb)
}

// ---------------------------------------------------------------------------
// Child dispatchers
// ---------------------------------------------------------------------------

// AddChild attaches a child dispatcher. Updates flow from parent to child
// after the parent's handler groups are exhausted (unless [StopChildren] was
// called).
func (d *Dispatcher) AddChild(child *Dispatcher) {
	d.mu.Lock()
	defer d.mu.Unlock()
	child.mu.Lock()
	child.client = d.client
	child.parent = d
	child.mu.Unlock()
	d.children = append(d.children, child)
}

// RemoveChild detaches a child dispatcher.
func (d *Dispatcher) RemoveChild(child *Dispatcher) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i, c := range d.children {
		if c == child {
			d.children = append(d.children[:i], d.children[i+1:]...)
			child.mu.Lock()
			child.parent = nil
			child.mu.Unlock()
			return
		}
	}
}

// RemoveAllHandlers removes all registered handlers and raw handlers from
// this dispatcher (children are not affected).
func (d *Dispatcher) RemoveAllHandlers() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.groups = make(map[int]map[handlerKind][]handler)
	d.groupOrder = nil
	d.rawHandlers = nil
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

var (
	ErrNoClient       = dispatcherError("dispatcher: no client bound (use New or AddChild)")
	ErrAlreadyRunning = dispatcherError("dispatcher: already running")
)

type dispatcherError string

func (e dispatcherError) Error() string { return string(e) }

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// wrapCheck converts a typed Filter into a type-erased check function.
func wrapCheck[T any](filter Filter[T]) func(any) bool {
	if filter == nil {
		return nil
	}
	return func(a any) bool {
		ctx, ok := a.(T)
		if !ok {
			return false
		}
		return filter(ctx)
	}
}

// wrapCallback converts a typed callback into a type-erased callback.
func wrapCallback[T any](cb func(T) error) func(any) error {
	return func(a any) error {
		ctx, ok := a.(T)
		if !ok {
			return nil
		}
		return cb(ctx)
	}
}

// resetPropagation zeros the propagation field on a context that embeds
// baseContext.
func resetPropagation(ctx any) {
	if bc, ok := ctx.(interface{ setPropagation(propagationAction) }); ok {
		bc.setPropagation(propagationDefault)
	}
}

// getPropagation reads the propagation field from a context.
func getPropagation(ctx any) propagationAction {
	if bc, ok := ctx.(interface{ getPropagation() propagationAction }); ok {
		return bc.getPropagation()
	}
	return propagationDefault
}

func typeName(v any) string {
	if v == nil {
		return "nil"
	}
	return fmtType(v)
}

// fmtType is split out for testability.
var fmtType = func(v any) string {
	return sprintType(v)
}
