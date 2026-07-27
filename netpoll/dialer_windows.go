//go:build windows

package netpoll

import (
	"context"
	"net"
)

func dial(ctx context.Context, address string, _ Config) (net.Conn, error) {
	var dialer net.Dialer
	return dialer.DialContext(ctx, "tcp", address)
}
