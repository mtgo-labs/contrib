//go:build !windows

package netpoll

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"
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

func TestDialHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Dial(ctx, "192.0.2.1:443")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
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
