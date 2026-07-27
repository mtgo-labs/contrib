# websocket

WebSocket transport dialer for `mtgo-labs/raw`.

```go
import "github.com/mtgo-labs/contrib/websocket"
```

## Overview

- WebSocket transport via `coder/websocket`
- Telegram's WebSocket endpoints require the obfuscated2 transport layer (`Config.Obfuscate: true`)
- Provides `WebSocketDCAddresses()` mapping DC IDs to `*.web.telegram.org` domains

## Usage

```go
client, err := raw.NewClient(raw.Config{
    Obfuscate:   true,
    DCAddresses: websocket.WebSocketDCAddresses(),
    DialFunc:    websocket.Dial,
})
```

Dependencies: `coder/websocket`.
