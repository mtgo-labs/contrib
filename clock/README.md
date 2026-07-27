# clock

Injectable `Clock` interface for testing time-dependent code.

```go
import "github.com/mtgo-labs/contrib/clock"
```

## Overview

- **`clock.Clock`** — interface with `Now()`, `Timer(d)`, `Ticker(d)`
- **`clock.System`** — real-time implementation backed by `time` stdlib
- **`clock.Timer`** / **`clock.Ticker`** — interfaces matching `time.Timer`/`time.Ticker`

Tests supply a custom `Clock` to control time deterministically. Zero external dependencies.
