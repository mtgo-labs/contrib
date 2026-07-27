//go:build !windows

package netpoll

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/cloudwego/netpoll"
)

// Dial opens a CloudWeGo/netpoll TCP connection and returns it as a
// net.Conn for use with mtgo-labs/raw's Config.DialFunc. On Linux it
// uses epoll; on macOS and BSDs it uses kqueue.
//
// netpoll.Connection embeds net.Conn: its Read blocks until at least one
// byte is available then returns all buffered data (standard io.Reader
// semantics), and Write buffers and immediately flushes (standard
// io.Writer semantics). TCP_NODELAY is enabled by default inside netpoll.
//
// Upstream note: CloudWeGo/netpoll's BSD kqueue poll (v0.7.4) does not
// check EV_ERROR on returned kevents. This is not reachable from this
// wrapper because EV_ERROR only appears on asynchronously failed filter
// registrations, which cannot occur during a standard client dial-and-use
// lifecycle. No local workaround is required.
func Dial(ctx context.Context, address string) (net.Conn, error) {
	if ctx == nil {
		return nil, errors.New("netpoll: nil dial context")
	}
	if address == "" {
		return nil, errors.New("netpoll: empty dial address")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	timeout := time.Duration(0)
	if deadline, ok := ctx.Deadline(); ok {
		timeout = time.Until(deadline)
		if timeout <= 0 {
			return nil, context.DeadlineExceeded
		}
	}
	connection, err := netpoll.DialConnection("tcp", address, timeout)
	if err != nil {
		return nil, fmt.Errorf("netpoll: dial: %w", err)
	}
	return connection, nil
}
