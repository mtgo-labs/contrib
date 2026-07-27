//go:build windows

package netpoll

import (
	"context"
	"net"
)

// Dial opens a TCP connection using the standard library. On Unix
// (Linux, macOS, BSDs) the CloudWeGo/netpoll implementation is used;
// on Windows the standard net.Dialer is used instead.
func Dial(ctx context.Context, address string) (net.Conn, error) {
	var dialer net.Dialer
	return dialer.DialContext(ctx, "tcp", address)
}
