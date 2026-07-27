// Package metrics provides Prometheus instrumentation for raw RPC
// invocations. Use NewMetrics to create a collector, Middleware() to get
// the raw.Middleware that records metrics, and Register to add the
// collectors to a Prometheus registry.
package metrics

import (
	"context"
	"fmt"
	"time"

	"github.com/mtgo-labs/raw"
	"github.com/mtgo-labs/raw/tgerr"
	"github.com/mtgo-labs/raw/tl"
	"github.com/prometheus/client_golang/prometheus"
)
// All counters are safe for concurrent use.
type Metrics struct {
	rpcRequestsTotal          *prometheus.CounterVec
	rpcRequestDurationSeconds *prometheus.HistogramVec
	rpcRequestErrorsTotal     *prometheus.CounterVec
	floodWaitsTotal           *prometheus.CounterVec
	reconnectTotal            *prometheus.CounterVec
}

// NewMetrics returns a new Metrics with all collectors initialised but
// not yet registered. Call Register to attach them to a registry.
func NewMetrics() *Metrics {
	return &Metrics{
		rpcRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "raw_rpc_requests_total",
				Help: "Total RPC requests, partitioned by method and status.",
			},
			[]string{"method", "status"},
		),
		rpcRequestDurationSeconds: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "raw_rpc_request_duration_seconds",
				Help:    "RPC request latency in seconds, partitioned by method and status.",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"method", "status"},
		),
		rpcRequestErrorsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "raw_rpc_request_errors_total",
				Help: "Total RPC errors, partitioned by method and error type.",
			},
			[]string{"method", "error_type"},
		),
		floodWaitsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "raw_flood_waits_total",
				Help: "Total flood-wait responses, partitioned by method.",
			},
			[]string{"method"},
		),
		reconnectTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "raw_reconnects_total",
				Help: "Total reconnection attempts, partitioned by DC.",
			},
			[]string{"dc"},
		),
	}
}

// Register registers every collector with the given registry. The
// registry must be non-nil; passing nil is a programming error.
func (m *Metrics) Register(registry prometheus.Registerer) {
	registry.MustRegister(
		m.rpcRequestsTotal,
		m.rpcRequestDurationSeconds,
		m.rpcRequestErrorsTotal,
		m.floodWaitsTotal,
		m.reconnectTotal,
	)
}

// ReconnectTotal returns the reconnect counter. Callers that detect
// reconnections should increment it with the DC as the label value.
func (m *Metrics) ReconnectTotal() *prometheus.CounterVec {
	return m.reconnectTotal
}

// Middleware returns a raw.Middleware that records RPC metrics for every
// invocation that passes through it.
//
// Metrics recorded:
//   - raw_rpc_requests_total{method, status}       – every call
//   - raw_rpc_request_duration_seconds{method, status} – request latency
//   - raw_rpc_request_errors_total{method, error_type}  – on error
//   - raw_flood_waits_total{method}                – on flood-wait responses
//
// The method label is the TL constructor ID formatted as a hex string
// (e.g. "0xda9b0d0d"). The status label is "success" or "error".
func (m *Metrics) Middleware() raw.Middleware {
	return raw.MiddlewareFunc(func(next raw.InvokeFunc) raw.InvokeFunc {
		return func(ctx context.Context, request tl.Object) ([]byte, error) {
			method := fmt.Sprintf("0x%08x", request.ConstructorID())

			start := time.Now()
			result, err := next(ctx, request)
			duration := time.Since(start).Seconds()

			if err != nil {
				m.rpcRequestsTotal.WithLabelValues(method, "error").Inc()
				m.rpcRequestDurationSeconds.WithLabelValues(method, "error").Observe(duration)

				errorType := classifyError(err)
				m.rpcRequestErrorsTotal.WithLabelValues(method, errorType).Inc()

				if isFloodWait(err) {
					m.floodWaitsTotal.WithLabelValues(method).Inc()
				}
				return result, err
			}

			m.rpcRequestsTotal.WithLabelValues(method, "success").Inc()
			m.rpcRequestDurationSeconds.WithLabelValues(method, "success").Observe(duration)
			return result, nil
		}
	})
}

// classifyError returns the error type label for an RPC error.
// For Telegram RPC errors it uses the normalised error type (e.g.
// "FLOOD_WAIT", "PHONE_NUMBER_INVALID"). For non-Telegram errors
// it returns "unknown".
func classifyError(err error) string {
	if rpcErr, ok := tgerr.As(err); ok {
		return rpcErr.Type
	}
	return "unknown"
}

// isFloodWait reports whether err is any flood-wait variant
// (FLOOD_WAIT, FLOOD_PREMIUM_WAIT, FLOOD_TEST_PHONE_WAIT, SLOWMODE_WAIT).
func isFloodWait(err error) bool {
	if rpcErr, ok := tgerr.As(err); ok {
		return rpcErr.IsFloodWaitFamily()
	}
	return false
}
