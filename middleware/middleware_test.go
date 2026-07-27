package middleware

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mtgo-labs/contrib/retry"
	"github.com/mtgo-labs/raw"
	"github.com/mtgo-labs/raw/tl"
)

type countingBackoff struct {
	resetCalls int
	nextCalls  int
}

func (b *countingBackoff) NextBackOff() time.Duration {
	b.nextCalls++
	return 0
}

func (b *countingBackoff) Reset() {
	b.resetCalls++
}

type retryInvocationKey struct{}

func TestRetryBackoffFactoryPerConcurrentRPC(t *testing.T) {
	const rpcCount = 32

	var factoryCalls atomic.Int64
	var instancesMu sync.Mutex
	instances := make([]*countingBackoff, 0, rpcCount)
	mw := NewRetryMiddleware(RetryOptions{
		MaxAttempts: 2,
		Backoff: func() retry.Backoff {
			factoryCalls.Add(1)
			backoff := &countingBackoff{}
			instancesMu.Lock()
			instances = append(instances, backoff)
			instancesMu.Unlock()
			return backoff
		},
	})

	var attempts [rpcCount]atomic.Int64
	next := raw.InvokeFunc(func(ctx context.Context, _ tl.Object) ([]byte, error) {
		index := ctx.Value(retryInvocationKey{}).(int)
		if attempts[index].Add(1) == 1 {
			return nil, context.DeadlineExceeded
		}
		return []byte{byte(index)}, nil
	})
	invoke := mw.Handle(next)

	start := make(chan struct{})
	var wg sync.WaitGroup
	for index := range rpcCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			ctx := context.WithValue(context.Background(), retryInvocationKey{}, index)
			response, err := invoke(ctx, nil)
			if err != nil {
				t.Errorf("RPC %d failed: %v", index, err)
				return
			}
			if len(response) != 1 || response[0] != byte(index) {
				t.Errorf("RPC %d response = %v", index, response)
			}
		}()
	}
	close(start)
	wg.Wait()

	if got := factoryCalls.Load(); got != rpcCount {
		t.Fatalf("backoff factory calls = %d, want %d", got, rpcCount)
	}
	if len(instances) != rpcCount {
		t.Fatalf("backoff instances = %d, want %d", len(instances), rpcCount)
	}

	seen := make(map[*countingBackoff]struct{}, rpcCount)
	for index, backoff := range instances {
		if _, duplicate := seen[backoff]; duplicate {
			t.Fatalf("backoff instance %d was reused", index)
		}
		seen[backoff] = struct{}{}
		if backoff.resetCalls != 1 {
			t.Errorf("backoff %d Reset calls = %d, want 1", index, backoff.resetCalls)
		}
		if backoff.nextCalls != 1 {
			t.Errorf("backoff %d NextBackOff calls = %d, want 1", index, backoff.nextCalls)
		}
	}
	for index := range rpcCount {
		if got := attempts[index].Load(); got != 2 {
			t.Errorf("RPC %d attempts = %d, want 2", index, got)
		}
	}
}
