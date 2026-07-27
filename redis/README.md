# redis

Redis-backed session store for `mtgo-labs/raw`.

```go
import "github.com/mtgo-labs/contrib/redis"
```

## Overview

- Sessions stored as binary blobs under a configurable key
- Multiple sessions per Redis instance via different keys
- Follows the gotd/contrib/redis pattern

## Usage

```go
rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
store := redis.NewStore(rdb, "mtgo:session")

client, err := raw.NewClient(raw.Config{
    Store: store,
})
```

Dependencies: `redis/go-redis/v9`.
