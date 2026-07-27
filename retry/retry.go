// Package retry provides backoff strategies and retry helpers for use with
// raw's RPC path or as standalone utilities. All implementations are safe
// for concurrent use when wrapped with SyncBackoff.
package retry

import (
	"context"
	"math"
	"math/rand/v2"
	"sync"
	"time"
)

// Backoff computes the next wait duration after a failed attempt.
// Implementations are not required to be safe for concurrent use;
// wrap with SyncBackoff when shared across goroutines.
type Backoff interface {
	// NextBackOff returns the duration to wait before the next retry.
	NextBackOff() time.Duration
	// Reset restores the backoff to its initial state.
	Reset()
}

// ExponentialOptions configures exponential backoff.
type ExponentialOptions struct {
	// InitialInterval is the delay before the first retry. Default: 1s.
	InitialInterval time.Duration
	// MaxInterval is the cap on delay growth. Default: 30s.
	MaxInterval time.Duration
	// Multiplier grows the delay each retry. Default: 2.0.
	Multiplier float64
	// RandomizationFactor adds jitter: [1-factor, 1+factor).
	// A value of 0.2 means ±20%. Default: 0.2.
	RandomizationFactor float64
}

func (o *ExponentialOptions) defaults() {
	if o.InitialInterval <= 0 {
		o.InitialInterval = time.Second
	}
	if o.MaxInterval <= 0 {
		o.MaxInterval = 30 * time.Second
	}
	if o.Multiplier <= 0 {
		o.Multiplier = 2.0
	}
	if o.RandomizationFactor < 0 {
		o.RandomizationFactor = 0.2
	}
}

// ExponentialBackoff implements exponential backoff with optional jitter.
type ExponentialBackoff struct {
	opts          ExponentialOptions
	current       time.Duration
	attempt       int
}

// NewExponentialBackoff returns a new exponential backoff with the given
// options. Unset fields are filled from defaults.
func NewExponentialBackoff(opts ExponentialOptions) *ExponentialBackoff {
	opts.defaults()
	return &ExponentialBackoff{
		opts:    opts,
		current: opts.InitialInterval,
	}
}

// NextBackOff returns the next backoff duration with jitter applied.
func (b *ExponentialBackoff) NextBackOff() time.Duration {
	d := b.current

	// Apply jitter: randomize around the computed delay.
	if b.opts.RandomizationFactor > 0 {
		delta := b.opts.RandomizationFactor * float64(d)
		min := float64(d) - delta
		max := float64(d) + delta
		d = time.Duration(min + rand.Float64()*(max-min+1))
	}

	// Grow for next call.
	b.attempt++
	next := time.Duration(float64(b.opts.InitialInterval) * math.Pow(b.opts.Multiplier, float64(b.attempt)))
	if next > b.opts.MaxInterval {
		next = b.opts.MaxInterval
	}
	b.current = next

	return d
}

// Reset restores the backoff to its initial state.
func (b *ExponentialBackoff) Reset() {
	b.current = b.opts.InitialInterval
	b.attempt = 0
}

// ConstantBackoff returns a fixed interval on every call.
type ConstantBackoff struct {
	interval time.Duration
}

// NewConstantBackoff returns a backoff that always returns interval.
func NewConstantBackoff(interval time.Duration) *ConstantBackoff {
	return &ConstantBackoff{interval: interval}
}

// NextBackOff returns the configured interval.
func (b *ConstantBackoff) NextBackOff() time.Duration {
	return b.interval
}

// Reset is a no-op for constant backoff.
func (b *ConstantBackoff) Reset() {}

// SyncBackoff wraps b with a mutex, making it safe for concurrent use.
func SyncBackoff(b Backoff) Backoff {
	return &syncBackoff{base: b}
}

type syncBackoff struct {
	mu   sync.Mutex
	base Backoff
}

func (s *syncBackoff) NextBackOff() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.base.NextBackOff()
}

func (s *syncBackoff) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.base.Reset()
}

// Do retries fn with backoff between attempts. It returns immediately if fn
// succeeds (returns nil) or ctx is cancelled. The backoff is Reset before
// the first attempt.
func Do(ctx context.Context, b Backoff, fn func() error) error {
	b.Reset()

	timer := time.NewTimer(0)
	defer timer.Stop()

	for i := 0; ; i++ {
		err := fn()
		if err == nil {
			return nil
		}

		// Wait before retrying.
		d := b.NextBackOff()

		// First iteration: already ran once — drain the zero timer, then
		// arm with the computed delay.
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(d)

		select {
		case <-ctx.Done():
			// Report the last fn error, not the context error, so callers
			// can see what failed before cancellation.
			return err
		case <-timer.C:
		}
	}
}

// FloodWaitSleep sleeps for d, respecting context cancellation. Use this
// to handle FLOOD_WAIT errors from Telegram's MTProto API.
func FloodWaitSleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
