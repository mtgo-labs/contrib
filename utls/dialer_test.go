package utls

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"testing"
	"time"
)

// selfSignedCert generates a self-signed TLS certificate for testing.
func selfSignedCert() tls.Certificate {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	return tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
	}
}

// echoServer starts a TLS server on a random port that echoes back the
// first byte it receives.
func echoServer(t *testing.T) (addr string, stop func()) {
	t.Helper()
	cert := selfSignedCert()
	cfg := &tls.Config{Certificates: []tls.Certificate{cert}}
	listener, err := tls.Listen("tcp", "127.0.0.1:0", cfg)
	if err != nil {
		t.Fatalf("failed to start TLS echo server: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, aerr := listener.Accept()
		if aerr != nil {
			return
		}
		buf := make([]byte, 1)
		conn.Read(buf)
		conn.Write(buf)
		conn.Close()
	}()
	return listener.Addr().String(), func() {
		listener.Close()
		<-done
	}
}

func TestNewDialerDefaults(t *testing.T) {
	// Zero-value Config should default to FingerprintChrome.
	d := NewDialer(Config{})
	if d.cfg.Fingerprint != FingerprintChrome {
		t.Errorf("zero Fingerprint: got %v, want FingerprintChrome", d.cfg.Fingerprint.Str())
	}
	if d.cfg.UTLS {
		t.Error("UTLS should default to false")
	}
}

func TestNewDialerCustomFingerprint(t *testing.T) {
	d := NewDialer(Config{
		UTLS:        true,
		Fingerprint: FingerprintFirefox,
	})
	if d.cfg.Fingerprint != FingerprintFirefox {
		t.Errorf("Fingerprint: got %v, want FingerprintFirefox", d.cfg.Fingerprint.Str())
	}
	if !d.cfg.UTLS {
		t.Error("UTLS should be true")
	}
}

func TestDialNilContext(t *testing.T) {
	d := NewDialer(Config{})
	var nilCtx context.Context
	_, err := d.Dial(nilCtx, "127.0.0.1:443")
	if err == nil {
		t.Fatal("Dial(nil) should return an error")
	}
}

func TestDialEmptyAddress(t *testing.T) {
	d := NewDialer(Config{})
	_, err := d.Dial(context.Background(), "")
	if err == nil {
		t.Fatal("Dial with empty address should return an error")
	}
}

func TestDialCanceledContext(t *testing.T) {
	d := NewDialer(Config{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := d.Dial(ctx, "127.0.0.1:443")
	if err == nil {
		t.Fatal("Dial with canceled context should return an error")
	}
}

func TestDialStandardTLS(t *testing.T) {
	addr, stop := echoServer(t)
	defer stop()

	d := NewDialer(Config{UTLS: false})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := d.Dial(ctx, addr)
	if err != nil {
		t.Fatalf("Dial standard TLS: %v", err)
	}
	defer conn.Close()

	// Verify the connection is TLS-wrapped by checking it implements
	// the TLS connection state interface.
	type connectionStater interface {
		ConnectionState() tls.ConnectionState
	}
	if _, ok := conn.(connectionStater); !ok {
		t.Error("returned connection does not expose TLS ConnectionState")
	}

	// Write a byte to verify the connection works.
	if _, err := conn.Write([]byte{0x42}); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, 1)
	if _, err := conn.Read(buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if buf[0] != 0x42 {
		t.Errorf("echo: got 0x%02x, want 0x42", buf[0])
	}
}

func TestDialUTLS(t *testing.T) {
	addr, stop := echoServer(t)
	defer stop()

	d := NewDialer(Config{UTLS: true})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := d.Dial(ctx, addr)
	if err != nil {
		t.Fatalf("Dial uTLS: %v", err)
	}
	defer conn.Close()

	// Verify basic read/write.
	if _, err := conn.Write([]byte{0x7f}); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, 1)
	if _, err := conn.Read(buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if buf[0] != 0x7f {
		t.Errorf("echo: got 0x%02x, want 0x7f", buf[0])
	}
}

func TestConfigSelection(t *testing.T) {
	t.Run("UTLS disabled uses standard TLS", func(t *testing.T) {
		addr, stop := echoServer(t)
		defer stop()

		d := NewDialer(Config{UTLS: false})
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		conn, err := d.Dial(ctx, addr)
		if err != nil {
			t.Fatalf("Dial: %v", err)
		}
		conn.Close()
	})

	t.Run("UTLS enabled uses uTLS", func(t *testing.T) {
		addr, stop := echoServer(t)
		defer stop()

		d := NewDialer(Config{UTLS: true})
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		conn, err := d.Dial(ctx, addr)
		if err != nil {
			t.Fatalf("Dial: %v", err)
		}
		conn.Close()
	})
}

func TestDialCustomServerName(t *testing.T) {
	addr, stop := echoServer(t)
	defer stop()

	// Extract host part for ServerName override.
	host, _, _ := net.SplitHostPort(addr)
	d := NewDialer(Config{UTLS: false, ServerName: host})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := d.Dial(ctx, addr)
	if err != nil {
		t.Fatalf("Dial with custom ServerName: %v", err)
	}
	conn.Close()
}

func TestDialTimeout(t *testing.T) {
	// Dialing an unroutable address should timeout.
	d := NewDialer(Config{UTLS: false})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := d.Dial(ctx, "192.0.2.1:443") // TEST-NET-1, should timeout
	if err == nil {
		t.Error("expected timeout error, got nil")
	}
}

func TestDialInvalidAddress(t *testing.T) {
	d := NewDialer(Config{})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := d.Dial(ctx, "not-an-address")
	if err == nil {
		t.Error("expected error for invalid address, got nil")
	}
}
