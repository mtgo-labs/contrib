# retry

Backoff strategies and retry helpers for `mtgo-labs/raw`.

```go
import "github.com/mtgo-labs/contrib/retry"
```

## Overview

- **`Backoff`** — interface for computing wait durations between retries
- **`ExponentialBackoff`** — exponential backoff with configurable jitter
- **`ConstantBackoff`** — fixed-interval backoff
- **`SyncBackoff(b)`** — wraps any `Backoff` with a mutex for concurrent use
- **`Do(ctx, backoff, fn)`** — retries `fn` with backoff until success or cancellation
- **`FloodWaitSleep(ctx, d)`** — context-aware sleep for `FLOOD_WAIT` handling

Zero external dependencies.
