// Package opentelemetry provides OpenTelemetry tracing middleware for the raw
// MTProto client. Wrap the client's RPC path with NewTracingMiddleware to
// create a span for each outgoing request, recording method name, payload
// sizes, and error status.
package opentelemetry

import (
	"context"
	"reflect"

	"github.com/mtgo-labs/raw"
	"github.com/mtgo-labs/raw/tl"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// TracingOption configures the tracing middleware.
type TracingOption func(*tracingConfig)

type tracingConfig struct {
	payloadLog bool
	extraAttrs []attribute.KeyValue
}

// WithPayloadLogging controls whether request and response payloads are
// recorded as span attributes. Defaults to false for security — MTProto
// payloads may contain sensitive data.
func WithPayloadLogging(enabled bool) TracingOption {
	return func(c *tracingConfig) {
		c.payloadLog = enabled
	}
}

// WithAttributes adds the given key-value pairs as extra span attributes on
// every traced RPC.
func WithAttributes(attrs ...attribute.KeyValue) TracingOption {
	return func(c *tracingConfig) {
		c.extraAttrs = append(c.extraAttrs, attrs...)
	}
}

// NewTracingMiddleware returns a raw.Middleware that creates an OpenTelemetry
// span for each RPC invocation. The span is named after the TL method
// (e.g., "HelpGetConfig", "MessagesSendMessage"). Span attributes include:
//
//   - rpc.method: the TL constructor name
//   - rpc.system: "mtproto"
//   - rpc.response.size: encoded response payload size in bytes
//   - Any extra attributes supplied via WithAttributes
//
// On error the span status is set to Error and the error message is recorded.
func NewTracingMiddleware(tracer trace.Tracer, opts ...TracingOption) raw.Middleware {
	cfg := &tracingConfig{}
	for _, o := range opts {
		o(cfg)
	}
	return raw.MiddlewareFunc(func(next raw.InvokeFunc) raw.InvokeFunc {
		return func(ctx context.Context, request tl.Object) ([]byte, error) {
			method := reflect.TypeOf(request).Elem().Name()
			spanName := "RPC " + method

			ctx, span := tracer.Start(ctx, spanName,
				trace.WithSpanKind(trace.SpanKindClient),
			)
			defer span.End()

			span.SetAttributes(
				attribute.String("rpc.method", method),
				attribute.String("rpc.system", "mtproto"),
			)

			for _, attr := range cfg.extraAttrs {
				span.SetAttributes(attr)
			}

			// Execute the RPC.
			resp, err := next(ctx, request)

			if err != nil {
				span.SetStatus(codes.Error, err.Error())
				span.RecordError(err)
				return nil, err
			}

			// Record response payload size.
			span.SetAttributes(attribute.Int("rpc.response.size", len(resp)))

			if cfg.payloadLog {
				span.SetAttributes(
					attribute.String("rpc.request.type", method),
					attribute.Int("rpc.response.payload_bytes", len(resp)),
				)
			}

			return resp, nil
		}
	})
}
