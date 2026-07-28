# mtgo-labs/contrib

Contrib transport and utility packages for [mtgo-labs/raw](https://github.com/mtgo-labs/raw).

## Packages

### compression

Gzip compression backed by
[`github.com/klauspost/compress/gzip`](https://github.com/klauspost/compress/tree/v1.19.1/gzip).

```go
compressed, err := compression.Gzip(data, compression.LevelDefault)
if err != nil {
    log.Fatal(err)
}

data, err = compression.Gunzip(compressed, 16<<20)
if err != nil {
    log.Fatal(err)
}
```

`Gunzip` accepts a decompressed-size limit to prevent memory exhaustion when
processing untrusted input. This module provides gzip only.

### netpoll

CloudWeGo/netpoll epoll-based transport for Linux.

```go
import (
    raw "github.com/mtgo-labs/raw"
    "github.com/mtgo-labs/contrib/netpoll"
)

client, err := raw.NewClient(raw.Config{
    APIID:    12345,
    APIHash:  "...",
    BotToken: "...",
    DialFunc: netpoll.Dial,
})
```

`netpoll.Dial` returns a `net.Conn`-compatible packet connection, so it plugs
directly into `raw.Config.DialFunc`. On Unix, raw uses CloudWeGo's native
`Reader` and `Writer` for MTProto framing; Windows retains the standard
`net.Dialer` fallback. TCP_NODELAY is enabled by default inside netpoll.

For latency-sensitive request/response traffic, `netpoll.NewDialer` accepts an
optional `ReadSpin` duration that trades bounded CPU time for lower scheduler
wake-up latency on Unix.

### utls

Optional TLS transport with spoofable ClientHello fingerprints via
[refraction-networking/utls](https://github.com/refraction-networking/utls).

```go
import (
    raw "github.com/mtgo-labs/raw"
    "github.com/mtgo-labs/contrib/utls"
)

client, err := raw.NewClient(raw.Config{
    APIID:    12345,
    APIHash:  "...",
    BotToken: "...",
    DialFunc: utls.NewDialer(utls.Config{UTLS: true}).Dial,
})
```

When `UTLS` is `false` (the default), the dialer uses standard `crypto/tls`.
When `true`, it uses uTLS with the configured fingerprint (default: modern Chrome).
`FingerprintFirefox`, `FingerprintSafari`, `FingerprintiOS`, and
`FingerprintRandomized` are also available.


### websocket

WebSocket transport via [coder/websocket](https://github.com/coder/websocket).
Telegram's WebSocket endpoints require obfuscated2 transport.

```go
import (
    raw "github.com/mtgo-labs/raw"
    "github.com/mtgo-labs/contrib/websocket"
)

client, err := raw.NewClient(raw.Config{
    APIID:       12345,
    APIHash:     "...",
    BotToken:    "...",
    Obfuscate:   true,
    DCAddresses: websocket.WebSocketDCAddresses(),
    DialFunc:    websocket.Dial,
})
```

### sqlite

SQLite-backed session storage for mtgo-labs/raw. Stores session state,
auth keys, peer cache, and message references in a local database using
pure-Go SQLite (zero CGO). Matches the @mtcute/sqlite storage API.

```go
import (
    raw "github.com/mtgo-labs/raw"
    "github.com/mtgo-labs/contrib/sqlite"
)

store, err := sqlite.NewStore("session.db")
if err != nil {
    log.Fatal(err)
}
defer store.Close()

client, err := raw.NewClient(raw.Config{
    APIID:    12345,
    APIHash:  "...",
    BotToken: "...",
    Store:    store,
})
```

The `Store` also exposes repository types for direct use:

```go
store.AuthKeys.Set(2, authKey)
if err := store.KV.Set("my_key", value); err != nil {
    log.Fatal(err)
}
store.Peers.Store(peer)
store.RefMessages.Store(peerID, chatID, msgID)
```

### redis

Redis-backed session storage for mtgo-labs/raw. Multiple sessions can
share a single Redis instance using different keys.

```go
import (
    "github.com/redis/go-redis/v9"
    raw "github.com/mtgo-labs/raw"
    "github.com/mtgo-labs/contrib/redis"
)

rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
store := redis.NewStore(rdb, "mtgo:session")

client, err := raw.NewClient(raw.Config{
    APIID:    12345,
    APIHash:  "...",
    BotToken: "...",
    Store:    store,
})
```

Optional TTL for auto-expiry:

```go
store := redis.NewStore(rdb, "mtgo:session", redis.WithTTL(24*time.Hour))
```
