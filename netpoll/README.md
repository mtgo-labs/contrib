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

Dependency: `cloudwego/netpoll` (Unix only).
