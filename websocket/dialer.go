package websocket

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	ws "github.com/coder/websocket"
)

// dcDomains maps Telegram WebSocket domains by DC ID.
var dcDomains = map[int]string{
	1: "pluto.web.telegram.org",
	2: "venus.web.telegram.org",
	3: "aurora.web.telegram.org",
	4: "vesta.web.telegram.org",
	5: "flora.web.telegram.org",
}

// WebSocketDCAddresses returns a map of DC IDs to WebSocket domain:port
// addresses for use with raw.Config.DCAddresses. Set this so that DC
// migrations use WebSocket endpoints instead of raw DC IPs.
func WebSocketDCAddresses() map[int]string {
	out := make(map[int]string, len(dcDomains))
	for dc, domain := range dcDomains {
		out[dc] = domain + ":443"
	}
	return out
}

// defaultDialContext prefers IPv4 to avoid IPv6 routing issues with
// Telegram's WebSocket endpoints.
func defaultDialContext(ctx context.Context, _, addr string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, "tcp4", addr)
}

var ipv4Client = &http.Client{Transport: &http.Transport{
	DialContext:           defaultDialContext,
	ForceAttemptHTTP2:     true,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: time.Second,
}}

// Dial connects to a Telegram DC over WebSocket and returns a net.Conn
// for use with raw.Config.DialFunc. It constructs the URL
// wss://<address>/apiws from the host:port address.
//
// Telegram's WebSocket endpoints require obfuscated2 transport and use
// domain names, not raw DC IPs. Set Config.Obfuscate to true and use
// WebSocketDCAddresses() for Config.DCAddresses:
//
//	client, _ := raw.NewClient(raw.Config{
//	    APIID:       12345,
//	    APIHash:     "...",
//	    BotToken:    "...",
//	    Obfuscate:   true,
//	    DCAddresses: websocket.WebSocketDCAddresses(),
//	    DialFunc:    websocket.Dial,
//	})
func Dial(ctx context.Context, address string) (net.Conn, error) {
	return DialURL(ctx, "wss://"+address+"/apiws")
}

// DialURL connects to a WebSocket server at the given URL and returns a
// net.Conn. Use this when you need a custom URL scheme, path, or port.
//
// The returned connection uses coder/websocket's built-in NetConn
// adapter, which sends each Write as one binary WebSocket frame.
func DialURL(ctx context.Context, url string) (net.Conn, error) {
	if ctx == nil {
		return nil, errors.New("websocket: nil dial context")
	}
	if url == "" {
		return nil, errors.New("websocket: empty dial URL")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	conn, _, err := ws.Dial(ctx, url, &ws.DialOptions{
		Subprotocols: []string{"binary"},
		HTTPClient:   ipv4Client,
	})
	if err != nil {
		return nil, fmt.Errorf("websocket: dial: %w", err)
	}
	return ws.NetConn(context.Background(), conn, ws.MessageBinary), nil
}
