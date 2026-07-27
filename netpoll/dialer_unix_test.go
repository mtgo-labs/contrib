//go:build !windows

package netpoll

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	cloudnetpoll "github.com/cloudwego/netpoll"
)

func TestDialRejectsInvalidArguments(t *testing.T) {
	var nilCtx context.Context
	if _, err := Dial(nilCtx, "127.0.0.1:1"); err == nil {
		t.Fatal("nil context unexpectedly succeeded")
	}
	if _, err := Dial(context.Background(), ""); err == nil {
		t.Fatal("empty address unexpectedly succeeded")
	}
}

func TestDialerAppliesReadSpin(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	accepted := make(chan net.Conn, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr == nil {
			accepted <- connection
		}
	}()

	const readSpin = 50 * time.Microsecond
	dialer := NewDialer(Config{ReadSpin: readSpin})
	connection, err := dialer.Dial(context.Background(), listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()

	packetConnection, ok := connection.(*Conn)
	if !ok {
		t.Fatalf("connection type = %T, want *Conn", connection)
	}
	if packetConnection.readSpin != readSpin {
		t.Fatalf("read spin = %v, want %v", packetConnection.readSpin, readSpin)
	}

	select {
	case server := <-accepted:
		_ = server.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("server did not accept connection")
	}
}

func TestDialHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Dial(ctx, "192.0.2.1:443")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestDialHonorsInFlightCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	remoteAddr := &cloudnetpoll.TCPAddr{}
	type dialCall struct {
		ctx           context.Context
		network       string
		local, remote *cloudnetpoll.TCPAddr
	}
	calls := make(chan dialCall, 1)
	errs := make(chan error, 1)

	go func() {
		_, err := dialWith(
			ctx,
			"192.0.2.1:443",
			Config{},
			func(_, _ string) (*cloudnetpoll.TCPAddr, error) {
				return remoteAddr, nil
			},
			func(dialCtx context.Context, network string, local, remote *cloudnetpoll.TCPAddr) (*cloudnetpoll.TCPConnection, error) {
				calls <- dialCall{ctx: dialCtx, network: network, local: local, remote: remote}
				<-dialCtx.Done()
				return nil, dialCtx.Err()
			},
		)
		errs <- err
	}()

	select {
	case call := <-calls:
		if call.ctx != ctx {
			t.Fatal("dial did not receive the caller context")
		}
		if call.network != "tcp" {
			t.Fatalf("network = %q, want tcp", call.network)
		}
		if call.local != nil {
			t.Fatalf("local address = %v, want nil", call.local)
		}
		if call.remote != remoteAddr {
			t.Fatal("dial did not receive the resolved remote address")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("dial did not start")
	}

	cancel()
	select {
	case err := <-errs:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("dial did not stop after cancellation")
	}
}

func TestDialRoundTrip(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	serverErr := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()
		buf := make([]byte, 32)
		n, err := conn.Read(buf)
		if err != nil {
			serverErr <- err
			return
		}
		if _, err := conn.Write(buf[:n]); err != nil {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Dial(ctx, listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	message := []byte("hello netpoll")
	if _, err := conn.Write(message); err != nil {
		t.Fatal(err)
	}

	reply := make([]byte, len(message))
	if _, err := io.ReadFull(conn, reply); err != nil {
		t.Fatal(err)
	}

	if string(reply) != string(message) {
		t.Fatalf("got %q, want %q", reply, message)
	}

	select {
	case err := <-serverErr:
		if err != nil {
			t.Fatalf("server error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not complete in time")
	}
}
