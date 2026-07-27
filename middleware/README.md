# middleware

Composable RPC middleware for the `mtgo-labs/raw` client.

```go
import "github.com/mtgo-labs/contrib/middleware"
```

## Overview

- **`Chain(middlewares...)`** — compose multiple middlewares into one
- **`NewLoggingMiddleware(logger)`** — logs every RPC with duration and status
- **`NewRetryMiddleware(opts)`** — retries transient errors with configurable backoff
- **`NewRateLimitMiddleware(rate)`** — rate-limits RPC invocations per second
- **`NewTimeoutMiddleware(timeout)`** — enforces a deadline on each RPC

Apply via `raw.Config.Middlewares`. Dependencies: `contrib/retry`.
