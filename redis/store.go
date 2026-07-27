package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mtgo-labs/raw/session"
	"github.com/redis/go-redis/v9"
)

// Client is the subset of redis.Cmdable used by Store.
// Both *redis.Client and *redis.ClusterClient satisfy this interface.
type Client interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
}

// Store persists session state in Redis. It implements session.Store
// for use with raw.Client.
//
// Multiple sessions can share a single Redis instance by using
// different keys (e.g. "mtgo:session:bot1", "mtgo:session:user2").
type Store struct {
	client Client
	key    string
	ttl    time.Duration
}

// StoreOption configures a Store.
type StoreOption func(*Store)

// WithTTL sets a time-to-live for session data. When zero (the default),
// session data has no expiry. Use this to auto-cleanup abandoned sessions.
func WithTTL(ttl time.Duration) StoreOption {
	return func(s *Store) {
		s.ttl = ttl
	}
}

// NewStore creates a Redis-backed session store. The client can be a
// *redis.Client, *redis.ClusterClient, or any type implementing Client.
// key is the Redis key under which the session blob is stored.
func NewStore(client Client, key string, opts ...StoreOption) *Store {
	s := &Store{client: client, key: key}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Load returns the stored session data. It implements session.Store.
func (s *Store) Load(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := s.client.Get(ctx, s.key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, fmt.Errorf("%w: %s", session.ErrSessionNotFound, s.key)
		}
		return nil, fmt.Errorf("redis: get %q: %w", s.key, err)
	}
	return data, nil
}

// Save persists session data. It implements session.Store.
func (s *Store) Save(ctx context.Context, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(data) == 0 {
		return fmt.Errorf("redis: empty session data for key %q", s.key)
	}
	if err := s.client.Set(ctx, s.key, data, s.ttl).Err(); err != nil {
		return fmt.Errorf("redis: set %q: %w", s.key, err)
	}
	return nil
}

// Delete removes the session data from Redis.
func (s *Store) Delete(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.client.Del(ctx, s.key).Err(); err != nil {
		return fmt.Errorf("redis: del %q: %w", s.key, err)
	}
	return nil
}
