# supervisor

Goroutine coordination primitives modeled after gotd/td's `tdsync`.

```go
import "github.com/mtgo-labs/contrib/supervisor"
```

## Overview

- **`Ready`** — one-shot signal; all waiters proceed when `Signal()` is called
- **`ResetReady`** — reusable readiness signal that can cycle between ready/not-ready
- **`Supervisor`** — manages a group of goroutines with coordinated startup and shutdown via `errgroup`

Zero dependencies beyond `golang.org/x/sync`.
