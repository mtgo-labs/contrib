package redis

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/mtgo-labs/raw/session"
	"github.com/redis/go-redis/v9"
)

// fakeRedis is a minimal in-memory Redis mock for testing.
// It implements just enough of redis.Cmdable for our Store.
type fakeRedis struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func newFakeRedis() *fakeRedis {
	return &fakeRedis{data: make(map[string][]byte)}
}

func (f *fakeRedis) Get(_ context.Context, key string) *redis.StringCmd {
	f.mu.RLock()
	defer f.mu.RUnlock()
	val, ok := f.data[key]
	if !ok {
		cmd := redis.NewStringCmd(context.Background())
		cmd.SetErr(redis.Nil)
		return cmd
	}
	cmd := redis.NewStringCmd(context.Background())
	cmd.SetVal(string(val))
	return cmd
}

func (f *fakeRedis) Set(_ context.Context, key string, value interface{}, _ time.Duration) *redis.StatusCmd {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := value.([]byte)
	if !ok {
		s, ok := value.(string)
		if !ok {
			cmd := redis.NewStatusCmd(context.Background())
			cmd.SetErr(errors.New("fakeRedis: value must be []byte or string"))
			return cmd
		}
		b = []byte(s)
	}
	f.data[key] = b
	cmd := redis.NewStatusCmd(context.Background())
	cmd.SetVal("OK")
	return cmd
}

func (f *fakeRedis) Del(_ context.Context, keys ...string) *redis.IntCmd {
	f.mu.Lock()
	defer f.mu.Unlock()
	var n int64
	for _, key := range keys {
		if _, ok := f.data[key]; ok {
			delete(f.data, key)
			n++
		}
	}
	cmd := redis.NewIntCmd(context.Background())
	cmd.SetVal(n)
	return cmd
}

func (f *fakeRedis) Close() error { return nil }

// Ensure Store implements session.Store.
var _ session.Store = (*Store)(nil)

func TestRedisStoreImplementsSessionStore(t *testing.T) {
	store := NewStore(newFakeRedis(), "test:session")
	var _ session.Store = store
}

func TestRedisStoreSaveAndLoad(t *testing.T) {
	store := NewStore(newFakeRedis(), "test:session")

	ctx := context.Background()
	input := []byte("redis-session-state")
	if err := store.Save(ctx, input); err != nil {
		t.Fatal(err)
	}

	output, err := store.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != string(input) {
		t.Fatalf("expected %q, got %q", input, output)
	}
}

func TestRedisStoreLoadNotFound(t *testing.T) {
	store := NewStore(newFakeRedis(), "test:missing")

	ctx := context.Background()
	_, err := store.Load(ctx)
	if err == nil {
		t.Fatal("expected error for missing key")
	}
	if !errors.Is(err, session.ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestRedisStoreOverwrite(t *testing.T) {
	fr := newFakeRedis()
	store := NewStore(fr, "test:session")
	ctx := context.Background()

	if err := store.Save(ctx, []byte("v1")); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(ctx, []byte("v2")); err != nil {
		t.Fatal(err)
	}

	data, err := store.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "v2" {
		t.Fatalf("expected v2, got %q", data)
	}
}

func TestRedisStoreDelete(t *testing.T) {
	store := NewStore(newFakeRedis(), "test:session")
	ctx := context.Background()

	if err := store.Save(ctx, []byte("data")); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx); err != nil {
		t.Fatal(err)
	}

	_, err := store.Load(ctx)
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestRedisStoreRejectEmptySave(t *testing.T) {
	store := NewStore(newFakeRedis(), "test:session")
	ctx := context.Background()

	if err := store.Save(ctx, nil); err == nil {
		t.Fatal("expected error for empty save")
	}
	if err := store.Save(ctx, []byte{}); err == nil {
		t.Fatal("expected error for empty save")
	}
}

func TestRedisStoreHonorsCancellation(t *testing.T) {
	store := NewStore(newFakeRedis(), "test:session")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := store.Save(ctx, []byte("x")); err == nil {
		t.Fatal("expected error for cancelled save")
	}
	if _, err := store.Load(ctx); err == nil {
		t.Fatal("expected error for cancelled load")
	}
}

func TestRedisStoreMultipleKeys(t *testing.T) {
	fr := newFakeRedis()

	store1 := NewStore(fr, "mtgo:bot1")
	store2 := NewStore(fr, "mtgo:bot2")
	ctx := context.Background()

	if err := store1.Save(ctx, []byte("bot1-data")); err != nil {
		t.Fatal(err)
	}
	if err := store2.Save(ctx, []byte("bot2-data")); err != nil {
		t.Fatal(err)
	}

	data1, _ := store1.Load(ctx)
	data2, _ := store2.Load(ctx)

	if string(data1) != "bot1-data" {
		t.Fatalf("bot1: expected 'bot1-data', got %q", data1)
	}
	if string(data2) != "bot2-data" {
		t.Fatalf("bot2: expected 'bot2-data', got %q", data2)
	}

	// Delete only bot1
	if err := store1.Delete(ctx); err != nil {
		t.Fatal(err)
	}
	_, err := store1.Load(ctx)
	if err == nil {
		t.Fatal("bot1: expected error after delete")
	}
	data2, _ = store2.Load(ctx)
	if string(data2) != "bot2-data" {
		t.Fatal("bot2: should still exist after bot1 delete")
	}
}

func TestRedisStoreWithTTL(t *testing.T) {
	fr := newFakeRedis()
	store := NewStore(fr, "test:ttl", WithTTL(10*time.Minute))
	ctx := context.Background()

	if err := store.Save(ctx, []byte("data")); err != nil {
		t.Fatal(err)
	}

	data, err := store.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "data" {
		t.Fatalf("expected 'data', got %q", data)
	}
}
