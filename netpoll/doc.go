// Package netpoll provides a CloudWeGo/netpoll-based dialer for
// mtgo-labs/raw. On Linux it uses epoll; on macOS and BSDs it
// uses kqueue; on Windows it falls back to the standard library.
//
// Usage:
//
//	import (
//	    raw "github.com/mtgo-labs/raw"
//	    "github.com/mtgo-labs/contrib/netpoll"
//	)
//
//	client, err := raw.NewClient(raw.Config{
//	    // ...
//	    DialFunc: netpoll.Dial,
//	})
package netpoll
