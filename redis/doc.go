// Package redis provides a Redis-backed session store for mtgo-labs/raw.
//
// Session state is stored as a binary blob under a configurable key,
// matching the gotd/contrib/redis session storage pattern.
//
// Usage:
//
//	import (
//	    "github.com/redis/go-redis/v9"
//	    raw "github.com/mtgo-labs/raw"
//	    "github.com/mtgo-labs/contrib/redis"
//	)
//
//	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
//	store := redis.NewStore(rdb, "mtgo:session")
//
//	client, err := raw.NewClient(raw.Config{
//	    APIID:    12345,
//	    APIHash:  "...",
//	    BotToken: "...",
//	    Store:    store,
//	})
//
// Multiple sessions can share a Redis instance by using different keys.
package redis
