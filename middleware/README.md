# middleware

Composable RPC middleware for the `mtgo-labs/raw` client.

```go
import (
    "time"

    "github.com/mtgo-labs/contrib/middleware"
    "github.com/mtgo-labs/contrib/retry"
)

retryMiddleware := middleware.NewRetryMiddleware(middleware.RetryOptions{
    MaxAttempts: 3,
    Backoff: func() retry.Backoff {
        return retry.NewExponentialBackoff(retry.ExponentialOptions{
            InitialInterval: 250 * time.Millisecond,
        })
    },
})
```

## Overview

- **`Chain(middlewares...)`** — compose multiple middlewares into one
- **`NewLoggingMiddleware(logger)`** — logs every RPC with duration and status
- **`NewRetryMiddleware(opts)`** — retries transient errors with configurable backoff
- **`NewRateLimitMiddleware(rate)`** — rate-limits RPC invocations per second
- **`NewTimeoutMiddleware(timeout)`** — enforces a deadline on each RPC

Apply via `raw.Config.Middlewares`. Dependencies: `contrib/retry`.
