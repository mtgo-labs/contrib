// Package uploader provides high-level Telegram file upload helpers
// for the mtgo-labs/raw MTProto client, matching the mtcute TypeScript
// upload API patterns.
//
// Usage:
//
//	import (
//	    raw "github.com/mtgo-labs/raw"
//	    "github.com/mtgo-labs/contrib/uploader"
//	)
//
//	client, _ := raw.NewClient(raw.Config{…})
//	client.Connect(ctx)
//
//	u := uploader.NewUploader(client)
//	uploaded, err := u.Upload(ctx, uploader.FromPath("/path/to/file.jpg"))
//	if err != nil {
//	    log.Fatal(err)
//	}
//	// uploaded.InputFile can be used in sendMedia, etc.
//
// Features:
//   - Chunked upload via upload.saveFilePart / upload.saveBigFilePart
//   - Auto part-size selection (64KB default, max 512KB)
//   - Auto big-file detection (>10MB uses BigFile path)
//   - Small-file fast path (<128KB uses main connection)
//   - MD5 checksum for small files
//   - Progress callback
//   - MIME type detection from file name/magic bytes
//   - Input: file path, io.Reader, or []byte
package uploader
