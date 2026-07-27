// Package utls provides an optional TLS transport for mtgo-labs/raw
// using github.com/refraction-networking/utls for spoofable ClientHello
// fingerprints.
//
// Usage:
//
//	import (
//	    raw "github.com/mtgo-labs/raw"
//	    "github.com/mtgo-labs/contrib/utls"
//	)
//
//	client, err := raw.NewClient(raw.Config{
//	    // ...
//	    DialFunc: utls.NewDialer(utls.Config{UTLS: true}).Dial,
//	})
//
// When UTLS is false (the default), the dialer wraps the connection in
// standard crypto/tls. When UTLS is true, it uses uTLS with the configured
// fingerprint (default: modern Chrome).
//
// This package is completely opt-in and does not modify the default
// transport behavior of raw in any way.
package utls
