// Package websocket provides a WebSocket transport for mtgo-labs/raw,
// using github.com/coder/websocket. Telegram's WebSocket endpoints
// require the obfuscated2 transport layer.
//
// Usage:
//
//	import (
//	    raw "github.com/mtgo-labs/raw"
//	    "github.com/mtgo-labs/contrib/websocket"
//	)
//
//	client, err := raw.NewClient(raw.Config{
//	    APIID:       12345,
//	    APIHash:     "...",
//	    BotToken:    "...",
//	    Obfuscate:   true,
//	    DCAddresses: websocket.WebSocketDCAddresses(),
//	    DialFunc:    websocket.Dial,
//	})
package websocket
