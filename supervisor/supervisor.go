// Package supervisor provides synchronization primitives for coordinating
// goroutines. It is modeled after gotd/td's internal tdsync package.
package supervisor

import (
	"context"
	"sync"
	"sync/atomic"

	"golang.org/x/sync/errgroup"
)

// Ready is a one-shot signal primitive.
// Once signaled, all current and future waiters proceed immediately.
// The zero value is invalid; use NewReady.
type Ready struct {
	wait chan struct{}
	done int32
}

// NewReady creates a new Ready.
func NewReady() *Ready {
	return &Ready{
		wait: make(chan struct{}),
	}
}

// reset replaces the underlying channel and resets the done flag.
// Not safe for concurrent use — only called by ResetReady under lock.
func (r *Ready) reset() {
	r.wait = make(chan struct{})
	atomic.StoreInt32(&r.done, 0)
}

// Signal marks the Ready as signaled.
// Safe for concurrent use. Only the first call closes the channel.
func (r *Ready) Signal() {
	if atomic.CompareAndSwapInt32(&r.done, 0, 1) {
		close(r.wait)
	}
}

// Ready returns a channel that is closed when Signal is called.
func (r *Ready) Ready() <-chan struct{} {
	return r.wait
}

// ResetReady is a reusable readiness signal that can transition
// between ready and not-ready states repeatedly.
// The zero value is invalid; use NewResetReady.
type ResetReady struct {
	ready *Ready
	lock  sync.Mutex
}

// NewResetReady creates a new ResetReady in the not-ready state.
func NewResetReady() *ResetReady {
	return &ResetReady{
		ready: NewReady(),
	}
}

// Signal marks the ResetReady as ready.
// Safe for concurrent use.
func (r *ResetReady) Signal() {
	r.lock.Lock()
	r.ready.Signal()
	r.lock.Unlock()
}

// Ready returns a channel that is closed when the current cycle is signaled.
// Safe for concurrent use.
func (r *ResetReady) Ready() <-chan struct{} {
	r.lock.Lock()
	defer r.lock.Unlock()
	return r.ready.Ready()
}

// Reset unblocks all current waiters (by signaling the current Ready)
// and creates a fresh un-signaled channel for the next cycle.
// Safe for concurrent use.
func (r *ResetReady) Reset() {
	r.lock.Lock()
	r.ready.Signal()
	r.ready.reset()
	r.lock.Unlock()
}

// Supervisor manages a group of long-lived goroutines.
// Unlike errgroup.Group, it does NOT cancel siblings when one goroutine fails.
// Errors are reported via an optional handler instead of surfacing through Wait.
// The zero value is invalid; use NewSupervisor.
type Supervisor struct {
	wg sync.WaitGroup

	ctx    context.Context
	cancel context.CancelFunc

	onError func(err error)
}

// NewSupervisor creates a new Supervisor derived from parent.
func NewSupervisor(parent context.Context) *Supervisor {
	ctx, cancel := context.WithCancel(parent)
	return &Supervisor{
		ctx:    ctx,
		cancel: cancel,
	}
}

// WithErrorHandler sets the error handler for tasks.
// Must be called before any Go calls.
func (s *Supervisor) WithErrorHandler(h func(err error)) *Supervisor {
	s.onError = h
	return s
}

// Go starts task in a new goroutine.
// If the task returns an error, it is passed to the error handler (if set).
// Returning an error does NOT cancel sibling goroutines.
func (s *Supervisor) Go(task func(ctx context.Context) error) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := task(s.ctx); err != nil {
			if s.onError != nil {
				s.onError(err)
			}
		}
	}()
}

// Cancel cancels the Supervisor's context, signaling all tasks to stop.
func (s *Supervisor) Cancel() {
	s.cancel()
}

// Wait blocks until all tasks have returned, then returns nil.
func (s *Supervisor) Wait() error {
	s.wg.Wait()
	return nil
}

// CancellableGroup is a goroutine group with explicit cancel and
// first-error propagation. It wraps golang.org/x/sync/errgroup.
// When any goroutine returns an error, errgroup cancels the derived
// context (causing sibling goroutines to stop) and Wait returns that error.
// The zero value is invalid; use NewCancellableGroup.
type CancellableGroup struct {
	group  *errgroup.Group
	ctx    context.Context
	cancel context.CancelFunc
}

// NewCancellableGroup creates a new CancellableGroup derived from parent.
func NewCancellableGroup(parent context.Context) *CancellableGroup {
	ctx, cancel := context.WithCancel(parent)
	g, gctx := errgroup.WithContext(ctx)
	return &CancellableGroup{
		group:  g,
		ctx:    gctx,
		cancel: cancel,
	}
}

// Go starts f in a new goroutine with the group's context.
// If any goroutine returns an error, the context is canceled and
// sibling goroutines see context.Canceled on their next check.
func (g *CancellableGroup) Go(f func(ctx context.Context) error) {
	g.group.Go(func() error {
		return f(g.ctx)
	})
}

// Cancel explicitly cancels the group's context, causing all
// goroutines to stop.
func (g *CancellableGroup) Cancel() {
	g.cancel()
}

// Wait blocks until all goroutines have returned, then returns the
// first non-nil error (if any).
func (g *CancellableGroup) Wait() error {
	return g.group.Wait()
}
