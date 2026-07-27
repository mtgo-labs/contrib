# netpoll

High-performance TCP dialer for `mtgo-labs/raw` using CloudWeGo/netpoll.

```go
import "github.com/mtgo-labs/contrib/netpoll"
```

## Platform support

| Platform | Backend | Mechanism |
|----------|---------|-----------|
| Linux    | CloudWeGo/netpoll | epoll |
| macOS    | CloudWeGo/netpoll | kqueue |
| FreeBSD / OpenBSD / NetBSD / Dragonfly | CloudWeGo/netpoll | kqueue |
| Windows  | standard `net.Dialer` | IOCP |

## Usage

```go
client, err := raw.NewClient(raw.Config{
    DialFunc: netpoll.Dial,
})
```

On Unix, `Dial` returns a packet-aware connection. `raw` detects its method
set and uses CloudWeGo's native `Reader` and `Writer` for intermediate,
abridged, and padded-intermediate framing. The package does not import or
require `github.com/mtgo-labs/raw`; Windows keeps the standard `net.Dialer`
fallback.

For latency-sensitive request/response traffic, a dialer can cooperatively spin
for a short interval before blocking in netpoll:

```go
dialer := netpoll.NewDialer(netpoll.Config{
    ReadSpin: 50 * time.Microsecond,
})

client, err := raw.NewClient(raw.Config{
    DialFunc: dialer.Dial,
})
```

`ReadSpin` is disabled by default. On Unix it can reduce scheduler wake-up
latency for native packet reads, at the cost of up to the configured amount of
extra CPU time after each packet. It has no effect on the Windows fallback.

The only direct dependency is `github.com/cloudwego/netpoll` (Unix only).
