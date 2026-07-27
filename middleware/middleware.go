// Package middleware provides composable middleware implementations for raw's
// InvokeFunc chain. The middleware in this package can be combined with
// Chain and applied via raw.Config.Middlewares.
package middleware

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/mtgo-labs/raw"
	"github.com/mtgo-labs/contrib/retry"
	"github.com/mtgo-labs/raw/tl"
)

// Chain composes middlewares into a single Middleware. The first element in
// the slice is outermost (called first); the last element is innermost
// (closest to the actual RPC invocation).
func Chain(middlewares ...raw.Middleware) raw.Middleware {
	return raw.MiddlewareFunc(func(next raw.InvokeFunc) raw.InvokeFunc {
		for i := len(middlewares) - 1; i >= 0; i-- {
			next = middlewares[i].Handle(next)
		}
		return next
	})
}

// NewLoggingMiddleware returns a middleware that logs every RPC invocation at
// debug level on success and error level on failure. Each log entry includes
// the request type name and call duration.
func NewLoggingMiddleware(logger *slog.Logger) raw.Middleware {
	return raw.MiddlewareFunc(func(next raw.InvokeFunc) raw.InvokeFunc {
		return func(ctx context.Context, req tl.Object) ([]byte, error) {
			reqType := typeName(req)
			start := time.Now()
			resp, err := next(ctx, req)
			dur := time.Since(start)

			if err != nil {
				logger.Error("rpc failed",
					"type", reqType,
					"duration", dur.String(),
					"error", err,
				)
			} else {
				logger.Debug("rpc ok",
					"type", reqType,
					"duration", dur.String(),
				)
			}
			return resp, err
		}
	})
}

// RetryOptions configures the retry middleware.
type RetryOptions struct {
	// MaxAttempts is the maximum number of RPC attempts. Values <= 0
	// default to 3.
	MaxAttempts int
	// Backoff computes the wait duration between attempts.
	Backoff retry.Backoff
}

// NewRetryMiddleware returns a middleware that retries failed RPCs with
// exponential (or custom) backoff. Only transient errors (network timeouts,
// temporary failures, context deadlines from outer timeout middleware) are
// retried; non-retryable errors are returned immediately.
func NewRetryMiddleware(opts RetryOptions) raw.Middleware {
	return raw.MiddlewareFunc(func(next raw.InvokeFunc) raw.InvokeFunc {
		return func(ctx context.Context, req tl.Object) ([]byte, error) {
			maxAttempts := opts.MaxAttempts
			if maxAttempts <= 0 {
				maxAttempts = 3
			}

			b := opts.Backoff
			if b != nil {
				b.Reset()
			}

			var resp []byte
			var err error

			for attempt := 0; attempt < maxAttempts; attempt++ {
				resp, err = next(ctx, req)
				if err == nil {
					return resp, nil
				}

				if !isRetryableError(err) {
					return nil, err
				}

				if attempt == maxAttempts-1 {
					break
				}

				d := nextBackoff(b)
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(d):
				}
			}
			return resp, err
		}
	})
}

// NewTimeoutMiddleware returns a middleware that applies a per-RPC deadline
// via context.WithTimeout. If the RPC exceeds the deadline, the context is
// cancelled and the call returns ctx.Err().
func NewTimeoutMiddleware(timeout time.Duration) raw.Middleware {
	return raw.MiddlewareFunc(func(next raw.InvokeFunc) raw.InvokeFunc {
		return func(ctx context.Context, req tl.Object) ([]byte, error) {
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			return next(ctx, req)
		}
	})
}

// NewDebugMiddleware returns a middleware that logs the encoded request bytes
// and raw response bytes as hex dumps (truncated at 1024 bytes per side).
// Only log if logger's level permits debug output; encoding cost is paid
// only when the log would be emitted.
func NewDebugMiddleware(logger *slog.Logger) raw.Middleware {
	return raw.MiddlewareFunc(func(next raw.InvokeFunc) raw.InvokeFunc {
		return func(ctx context.Context, req tl.Object) ([]byte, error) {
			if !logger.Enabled(ctx, slog.LevelDebug) {
				return next(ctx, req)
			}

			reqBytes, encodeErr := tl.Encode(req)
			resp, err := next(ctx, req)

			reqType := typeName(req)
			reqHex := hexify(reqBytes, encodeErr)
			respHex := hexify(resp, nil)

			if err != nil {
				logger.Debug("rpc debug (failed)",
					"type", reqType,
					"request_hex", reqHex,
					"response_hex", respHex,
					"error", err,
				)
			} else {
				logger.Debug("rpc debug",
					"type", reqType,
					"request_hex", reqHex,
					"response_hex", respHex,
				)
			}
			return resp, err
		}
	})
}

// typeName returns a short type name for the request, stripping the package
// path prefix (e.g. "*tl.HelpGetNearestDCRequest").
func typeName(req tl.Object) string {
	t := fmt.Sprintf("%T", req)
	if idx := strings.LastIndexByte(t, '.'); idx >= 0 {
		t = t[idx+1:]
	}
	return t
}

// isRetryableError returns true for transient network errors that are safe to
// retry: context deadline exceeded (from middleware), net.Error with timeout
// or temporary flag, and unknown-network errors.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}
	return false
}

// nextBackoff returns the next backoff duration, or 0 if b is nil.
func nextBackoff(b retry.Backoff) time.Duration {
	if b == nil {
		return 0
	}
	return b.NextBackOff()
}

// hexify returns a trimmed hex dump of data. If data is nil and err is
// non-nil, returns the error string. Output is truncated at 1024 hex
// characters (512 bytes).
func hexify(data []byte, err error) string {
	if err != nil {
		return fmt.Sprintf("<encode error: %v>", err)
	}
	if len(data) == 0 {
		return "<empty>"
	}
	const limit = 1024
	hex := hex.EncodeToString(data)
	if len(hex) > limit {
		hex = hex[:limit] + "..."
	}
	return hex
}
