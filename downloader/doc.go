// Package downloader provides high-level Telegram file download helpers
// for the mtgo-labs/raw MTProto client, matching the mtcute TypeScript
// download API patterns.
//
// Usage:
//
//	import (
//	    raw "github.com/mtgo-labs/raw"
//	    "github.com/mtgo-labs/contrib/downloader"
//	)
//
//	client, _ := raw.NewClient(raw.Config{…})
//	client.Connect(ctx)
//
//	d := downloader.NewDownloader(client)
//	err := d.Download(ctx, location, outputFile)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// Features:
//   - Chunked download via upload.getFile
//   - Auto part-size selection (64KB default, max 512KB)
//   - Small-file fast path (<128KB uses main connection)
//   - FILE_MIGRATE handling
//   - Parallel download via io.WriterAt
//   - Progress callback
//
// CDN redirects are not yet supported.
package downloader
