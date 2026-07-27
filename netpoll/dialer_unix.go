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

func dial(ctx context.Context, address string, cfg Config) (net.Conn, error) {
	return dialWith(ctx, address, cfg, netpoll.ResolveTCPAddr, netpoll.DialTCP)
}

func dialWith(
	ctx context.Context,
	address string,
	cfg Config,
	resolveTCPAddr func(network, address string) (*netpoll.TCPAddr, error),
	dialTCP func(ctx context.Context, network string, laddr, raddr *netpoll.TCPAddr) (*netpoll.TCPConnection, error),
) (net.Conn, error) {
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
	remoteAddr, err := resolveTCPAddr("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("netpoll: dial: %w", err)
	}
	connection, err := dialTCP(ctx, "tcp", nil, remoteAddr)
	if err != nil {
		return nil, fmt.Errorf("netpoll: dial: %w", err)
	}
	if connection == nil {
		return nil, errors.New("netpoll: dial returned nil connection")
	}
	return newConn(connection, cfg.ReadSpin), nil
}
