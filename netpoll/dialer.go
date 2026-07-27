package netpoll

import (
	"context"
	"net"
	"time"
)

// Config controls optional netpoll transport behavior.
type Config struct {
	// ReadSpin cooperatively waits for packet data before blocking in netpoll.
	// A short duration can reduce scheduler latency for request/response traffic
	// at the cost of extra CPU. It applies only to native packet reads on Unix;
	// zero disables it, and Windows ignores it.
	ReadSpin time.Duration
}

// Dialer establishes netpoll-backed TCP connections using Config.
type Dialer struct {
	cfg Config
}

// NewDialer returns a Dialer configured with cfg.
func NewDialer(cfg Config) *Dialer {
	if cfg.ReadSpin < 0 {
		cfg.ReadSpin = 0
	}
	return &Dialer{cfg: cfg}
}

// Dial opens a connection suitable for raw.Config.DialFunc.
func (dialer *Dialer) Dial(ctx context.Context, address string) (net.Conn, error) {
	return dial(ctx, address, dialer.cfg)
}

// Dial opens a connection with the default configuration.
func Dial(ctx context.Context, address string) (net.Conn, error) {
	return dial(ctx, address, Config{})
}
