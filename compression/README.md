# compression

Gzip compression for Go backed by
[`github.com/klauspost/compress/gzip`](https://github.com/klauspost/compress/tree/v1.19.1/gzip).

This module provides gzip only. It does not include S2, Zstandard, LZ4, or
transport-compression negotiation.

## Install

```bash
go get github.com/mtgo-labs/contrib/compression@latest
```

## Compress

```go
package main

import (
	"log"

	"github.com/mtgo-labs/contrib/compression"
)

func main() {
	data := []byte("hello from mtgo")

	compressed, err := compression.Gzip(data, compression.LevelDefault)
	if err != nil {
		log.Fatal(err)
	}

	_ = compressed
}
```

Available levels:

| Constant | Description |
|---|---|
| `LevelDefault` | Balanced compression |
| `LevelBestSpeed` | Fastest compression |
| `LevelBest` | Best compression ratio |
| `LevelHuffman` | Huffman-only compression |

## Decompress

```go
const maxDecompressedSize = 16 << 20 // 16 MiB

data, err := compression.Gunzip(compressed, maxDecompressedSize)
if err != nil {
	log.Fatal(err)
}
```

Always set a positive decompressed-size limit for untrusted input. `Gunzip`
returns `io.ErrShortBuffer` when the output exceeds the limit. A zero or
negative limit disables this protection.

## Compatibility

The generated streams use the standard gzip format and interoperate with Go's
`compress/gzip` package.

`mtgo-labs/raw` already decodes Telegram `gzip_packed` responses internally
using standard gzip. This package can be used directly by applications, but it
does not replace `raw`'s internal decoder unless `raw` exposes a decompressor
hook.
