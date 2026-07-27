# downloader

High-level Telegram file download helpers for `mtgo-labs/raw`.

```go
import "github.com/mtgo-labs/contrib/downloader"
```

## Overview

- Chunked download via `upload.getFile` with auto part-size selection (64 KB–512 KB)
- Small-file fast path (<128 KB uses main connection)
- `FILE_MIGRATE` handling
- Parallel download via `io.WriterAt`
- Progress callback
- CDN redirects not yet supported

## Usage

```go
d := downloader.NewDownloader(client)
err := d.Download(ctx, location, outputFile)
```
