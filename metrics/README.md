# metrics

Prometheus instrumentation middleware for `mtgo-labs/raw`.

```go
import "github.com/mtgo-labs/contrib/metrics"
```

## Overview

- **`Metrics`** — collectors for RPC requests, duration, errors, and flood waits
- **`Middleware()`** — returns a `raw.Middleware` that records every RPC call
- **`Register(registry)`** — attaches collectors to a Prometheus registry
- Counters: `raw_rpc_requests_total`, `raw_rpc_request_errors_total`, `raw_flood_waits_total`
- Histogram: `raw_rpc_request_duration_seconds`

Dependencies: `prometheus/client_golang`.
