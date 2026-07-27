package utls

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"

	utls "github.com/refraction-networking/utls"
)

// Config controls the TLS transport behavior.
type Config struct {
	// UTLS selects the TLS implementation. When false, the dialer uses
	// standard crypto/tls. When true, it uses uTLS with the configured
	// Fingerprint.
	UTLS bool

	// Fingerprint selects the ClientHello fingerprint. Defaults to
	// FingerprintChrome (HelloChrome_Auto) when zero-valued.
	Fingerprint Fingerprint

	// NoDelay enables TCP_NODELAY on the underlying connection before
	// the TLS handshake. This is typically desired for MTProto.
	NoDelay bool

	// ServerName overrides the TLS ServerName (SNI). When empty, the
	// host portion of the dial address is used.
	ServerName string
}

// Dialer establishes TLS-wrapped TCP connections using either standard
// crypto/tls or uTLS depending on Config.UTLS.
//
// The zero-value Dialer is usable and defaults to standard crypto/tls
// with the latest Chrome fingerprint when UTLS is enabled.
type Dialer struct {
	cfg Config
}

// NewDialer returns a Dialer configured with cfg. Unset fields are
// filled with defaults: Fingerprint defaults to FingerprintChrome.
func NewDialer(cfg Config) *Dialer {
	if cfg.Fingerprint.Client == "" {
		cfg.Fingerprint = FingerprintChrome
	}
	return &Dialer{cfg: cfg}
}

// Dial satisfies raw.Config.DialFunc. It opens a TCP connection to
// address, optionally enables TCP_NODELAY, then completes a TLS
// handshake using either standard crypto/tls or uTLS.
//
// The context is used for both the TCP dial and the TLS handshake.
// A deadline set on the context applies to the combined operation.
func (d *Dialer) Dial(ctx context.Context, address string) (net.Conn, error) {
	if ctx == nil {
		return nil, errors.New("utls: nil dial context")
	}
	if address == "" {
		return nil, errors.New("utls: empty dial address")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("utls: dial: %w", err)
	}
	// Ensure the raw connection is closed on any error after this point.
	deferOnErr := true
	defer func() {
		if deferOnErr {
			conn.Close()
		}
	}()

	if d.cfg.NoDelay {
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			_ = tcpConn.SetNoDelay(true)
		}
	}

	serverName := d.cfg.ServerName
	if serverName == "" {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("utls: parse address: %w", err)
		}
		serverName = host
	}

	tlsConn, err := d.tlsHandshake(ctx, conn, serverName)
	if err != nil {
		return nil, err
	}

	deferOnErr = false
	return tlsConn, nil
}

func (d *Dialer) tlsHandshake(ctx context.Context, conn net.Conn, serverName string) (net.Conn, error) {
	if d.cfg.UTLS {
		return d.handshakeUTLS(ctx, conn, serverName)
	}
	return d.handshakeStandard(ctx, conn, serverName)
}

func (d *Dialer) handshakeStandard(ctx context.Context, conn net.Conn, serverName string) (net.Conn, error) {
	tlsCfg := &tls.Config{
		ServerName:         serverName,
		InsecureSkipVerify: true,
	}
	tlsConn := tls.Client(conn, tlsCfg)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return nil, fmt.Errorf("utls: tls handshake: %w", err)
	}
	return tlsConn, nil
}

func (d *Dialer) handshakeUTLS(ctx context.Context, conn net.Conn, serverName string) (net.Conn, error) {
	utlsCfg := &utls.Config{
		ServerName:         serverName,
		InsecureSkipVerify: true,
	}
	uconn := utls.UClient(conn, utlsCfg, d.cfg.Fingerprint)
	if err := uconn.HandshakeContext(ctx); err != nil {
		return nil, fmt.Errorf("utls: utls handshake: %w", err)
	}
	return uconn, nil
}
