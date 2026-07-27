# utls

uTLS-based transport dialer for `mtgo-labs/raw`.

```go
import "github.com/mtgo-labs/contrib/utls"
```

## Overview

- Spoofable TLS ClientHello fingerprints via `refraction-networking/utls`
- Falls back to standard `crypto/tls` when `UTLS` is disabled
- Configurable fingerprint (default: modern Chrome)
- Completely opt-in; does not modify default transport behavior

## Usage

```go
client, err := raw.NewClient(raw.Config{
    DialFunc: utls.NewDialer(utls.Config{UTLS: true}).Dial,
})
```

Dependencies: `refraction-networking/utls`.
