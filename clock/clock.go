// Package clock provides a Clock interface for testing time-dependent code.
//
// The System variable provides a real-time implementation backed by the
// standard library's time package. Tests can supply their own Clock
// implementation to control time deterministically.
package clock

import "time"

// Clock represents a time source. Use System for real time; inject a custom
// implementation for deterministic tests.
type Clock interface {
	Now() time.Time
	Timer(d time.Duration) Timer
	Ticker(d time.Duration) Ticker
}

// Timer is a one-shot time event.
type Timer interface {
	C() <-chan time.Time
	Stop() bool
	Reset(d time.Duration) bool
}

// Ticker is a periodic time event.
type Ticker interface {
	C() <-chan time.Time
	Stop()
	Reset(d time.Duration)
}

// System is the real-time Clock implementation.
var System Clock = systemClock{}

type systemClock struct{}

func (systemClock) Now() time.Time                    { return time.Now() }
func (systemClock) Timer(d time.Duration) Timer       { return systemTimer{time.NewTimer(d)} }
func (systemClock) Ticker(d time.Duration) Ticker     { return systemTicker{time.NewTicker(d)} }

type systemTimer struct{ t *time.Timer }

func (s systemTimer) C() <-chan time.Time          { return s.t.C }
func (s systemTimer) Stop() bool                    { return s.t.Stop() }
func (s systemTimer) Reset(d time.Duration) bool    { return s.t.Reset(d) }

type systemTicker struct{ t *time.Ticker }

func (s systemTicker) C() <-chan time.Time       { return s.t.C }
func (s systemTicker) Stop()                      { s.t.Stop() }
func (s systemTicker) Reset(d time.Duration)      { s.t.Reset(d) }
