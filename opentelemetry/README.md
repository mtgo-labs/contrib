# opentelemetry

OpenTelemetry tracing middleware for `mtgo-labs/raw`.

```go
import "github.com/mtgo-labs/contrib/opentelemetry"
```

## Overview

- **`NewTracingMiddleware(opts...)`** — creates a span for each RPC invocation
- Span name is the TL method (e.g., `HelpGetConfig`, `MessagesSendMessage`)
- Attributes: `rpc.method`, `rpc.system`, `rpc.response.size`
- Errors set span status to `Error` with the error message
- **`WithPayloadLogging(true)`** — record request/response payload sizes (off by default)
- **`WithAttributes(attrs...)`** — add extra span attributes

Dependencies: `go.opentelemetry.io/otel`.
