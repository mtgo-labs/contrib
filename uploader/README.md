# uploader

High-level Telegram file upload helpers for `mtgo-labs/raw`.

```go
import "github.com/mtgo-labs/contrib/uploader"
```

## Overview

- Chunked upload via `upload.saveFilePart` / `upload.saveBigFilePart`
- Auto part-size selection (64 KB–512 KB)
- Auto big-file detection (>10 MB uses BigFile path)
- Small-file fast path (<128 KB uses main connection)
- MD5 checksum for small files, progress callback, MIME type detection
- Input: file path, `io.Reader`, or `[]byte`

## Usage

```go
u := uploader.NewUploader(client)
uploaded, err := u.Upload(ctx, uploader.FromPath("/path/to/file.jpg"))
// uploaded.InputFile can be passed to sendMedia, etc.
```
