package websocket

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	ws "github.com/coder/websocket"
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
	_, err := Dial(ctx, "127.0.0.1:1")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func echoServer(t *testing.T) (*http.Server, net.Listener) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/apiws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := ws.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close(ws.StatusNormalClosure, "")
		conn.SetReadLimit(16 << 20)
		for {
			_, data, err := conn.Read(r.Context())
			if err != nil {
				return
			}
			if err := conn.Write(r.Context(), ws.MessageBinary, data); err != nil {
				return
			}
		}
	})
	server := &http.Server{Handler: mux}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go server.Serve(listener)
	return server, listener
}

func TestDialRoundTrip(t *testing.T) {
	server, listener := echoServer(t)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := DialURL(ctx, "ws://"+listener.Addr().String()+"/apiws")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	message := []byte("hello websocket")
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
}

func TestDialRoundTripLargeMessage(t *testing.T) {
	// Verify that messages larger than the caller's read buffer are
	// correctly buffered across multiple Read calls.
	server, listener := echoServer(t)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := DialURL(ctx, "ws://"+listener.Addr().String()+"/apiws")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// 64 KB — larger than bufio.Reader's 32 KB default.
	message := strings.Repeat("A", 64*1024)
	if _, err := conn.Write([]byte(message)); err != nil {
		t.Fatal(err)
	}

	reply := make([]byte, len(message))
	if _, err := io.ReadFull(conn, reply); err != nil {
		t.Fatal(err)
	}

	if string(reply) != message {
		t.Fatal("large message round-trip mismatch")
	}
}
