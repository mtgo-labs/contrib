package supervisor

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestReady_Signal(t *testing.T) {
	r := NewReady()

	select {
	case <-r.Ready():
		t.Fatal("Ready signaled before Signal call")
	default:
	}

	r.Signal()

	select {
	case <-r.Ready():
	default:
		t.Fatal("Ready not signaled after Signal call")
	}

	// Multiple Signal calls are safe.
	r.Signal()
	r.Signal()
}

func TestReady_ReadyAfterSignal(t *testing.T) {
	r := NewReady()
	r.Signal()

	// Should be immediately ready.
	select {
	case <-r.Ready():
	default:
		t.Fatal("Ready not signaled after Signal")
	}
}

func TestResetReady_SignalReset(t *testing.T) {
	rr := NewResetReady()

	// Not ready initially.
	select {
	case <-rr.Ready():
		t.Fatal("ResetReady signaled before Signal")
	default:
	}

	rr.Signal()

	// Ready after Signal.
	select {
	case <-rr.Ready():
	default:
		t.Fatal("ResetReady not signaled after Signal")
	}

	rr.Reset()

	// Not ready after Reset.
	select {
	case <-rr.Ready():
		t.Fatal("ResetReady signaled after Reset")
	default:
	}

	rr.Signal()

	// Ready again after second Signal.
	select {
	case <-rr.Ready():
	default:
		t.Fatal("ResetReady not signaled after second Signal")
	}
}

func TestResetReady_ResetUnblocksWaiters(t *testing.T) {
	rr := NewResetReady()

	var wg sync.WaitGroup
	blocked := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		close(blocked)
		<-rr.Ready()
	}()

	// Wait until the goroutine is blocked.
	<-blocked
	time.Sleep(10 * time.Millisecond)

	rr.Reset()
	wg.Wait() // goroutine should be unblocked by Reset signaling the old channel.
}

func TestResetReady_ConcurrentSignalAndReset(t *testing.T) {
	rr := NewResetReady()
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				rr.Signal()
				rr.Reset()
			}
		}()
	}
	wg.Wait()
}

func TestSupervisor_GoWait(t *testing.T) {
	ctx := context.Background()
	s := NewSupervisor(ctx)

	var ran bool
	s.Go(func(ctx context.Context) error {
		ran = true
		return nil
	})

	if err := s.Wait(); err != nil {
		t.Fatalf("Wait returned error: %v", err)
	}
	if !ran {
		t.Fatal("task did not run")
	}
}

func TestSupervisor_ErrorHandler(t *testing.T) {
	ctx := context.Background()
	var caught error
	s := NewSupervisor(ctx)
	s.WithErrorHandler(func(err error) {
		caught = err
	})

	s.Go(func(ctx context.Context) error {
		return errors.New("task failed")
	})

	s.Wait()
	if caught == nil || caught.Error() != "task failed" {
		t.Fatalf("error handler not called, caught: %v", caught)
	}
}

func TestSupervisor_NoCancelOnError(t *testing.T) {
	ctx := context.Background()
	s := NewSupervisor(ctx)

	s.Go(func(ctx context.Context) error {
		return errors.New("first fails")
	})

	task2Started := make(chan struct{})
	task2Done := make(chan struct{})
	s.Go(func(ctx context.Context) error {
		close(task2Started)
		<-ctx.Done()
		close(task2Done)
		return nil
	})

	<-task2Started
	s.Cancel()
	<-task2Done

	s.Wait()
}

func TestSupervisor_Cancel(t *testing.T) {
	ctx := context.Background()
	s := NewSupervisor(ctx)

	done := make(chan struct{})
	s.Go(func(ctx context.Context) error {
		<-ctx.Done()
		close(done)
		return nil
	})

	s.Cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Cancel did not cancel the task context")
	}

	s.Wait()
}

func TestCancellableGroup_WaitError(t *testing.T) {
	ctx := context.Background()
	g := NewCancellableGroup(ctx)

	expectedErr := errors.New("boom")
	g.Go(func(ctx context.Context) error {
		return expectedErr
	})

	err := g.Wait()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, expectedErr) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCancellableGroup_AutoCancel(t *testing.T) {
	ctx := context.Background()
	g := NewCancellableGroup(ctx)

	task2Canceled := make(chan struct{})
	g.Go(func(ctx context.Context) error {
		return errors.New("first fails")
	})
	g.Go(func(ctx context.Context) error {
		<-ctx.Done()
		close(task2Canceled)
		return nil
	})

	g.Wait()
	select {
	case <-task2Canceled:
	case <-time.After(time.Second):
		t.Fatal("sibling goroutine was not canceled on first error")
	}
}

func TestCancellableGroup_Cancel(t *testing.T) {
	ctx := context.Background()
	g := NewCancellableGroup(ctx)

	done := make(chan struct{})
	g.Go(func(ctx context.Context) error {
		<-ctx.Done()
		close(done)
		return nil
	})

	g.Cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("explicit Cancel did not cancel task context")
	}

	err := g.Wait()
	if err != nil {
		t.Fatalf("Wait returned unexpected error: %v", err)
	}
}
