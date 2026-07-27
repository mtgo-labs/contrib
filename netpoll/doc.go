// Package netpoll provides a CloudWeGo/netpoll-based dialer for mtgo-labs/raw.
// On Unix, raw automatically detects its packet-aware connection and uses the
// native Reader and Writer for MTProto framing without a module dependency from
// this package back to raw. Linux uses epoll, macOS and BSDs use kqueue, and
// Windows falls back to the standard library.
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
//
// For latency-sensitive request/response traffic, use a configured Dialer:
//
//	dialer := netpoll.NewDialer(netpoll.Config{
//	    ReadSpin: 50 * time.Microsecond,
//	})
//	client, err := raw.NewClient(raw.Config{
//	    // ...
//	    DialFunc: dialer.Dial,
//	})
//
// ReadSpin is disabled by default and trades bounded CPU time for lower
// scheduler wake-up latency on Unix packet reads.
package netpoll
