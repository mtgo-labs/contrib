package downloader

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	raw "github.com/mtgo-labs/raw"
	"github.com/mtgo-labs/raw/tgerr"
	"github.com/mtgo-labs/raw/tl"
)

const smallFileMaxSize = 131072
const defaultPartSize = 64 * 1024
const maxPartSize = 512 * 1024
const requestsPerConnection = 3

type ProgressFunc func(downloaded int64, total int64)
type ThrottleFunc func(chunkSize int) <-chan struct{}

type Downloader struct {
	client       *raw.Client
	partSize     int
	threads      int
	progress     ProgressFunc
	throttle     ThrottleFunc
	dcID         int
	premium      bool
	stallTimeout time.Duration
}

func NewDownloader(client *raw.Client) *Downloader {
	return &Downloader{client: client, partSize: defaultPartSize, threads: 1}
}

func (d *Downloader) WithPartSize(size int) *Downloader {
	if size < 4096 {
		size = 4096
	}
	if size > maxPartSize {
		size = maxPartSize
	}
	d.partSize = (size / 4096) * 4096
	if d.partSize == 0 {
		d.partSize = 4096
	}
	return d
}

func (d *Downloader) WithThreads(threads int) *Downloader {
	if threads < 1 {
		threads = 1
	}
	d.threads = threads
	return d
}

func (d *Downloader) WithProgress(fn ProgressFunc) *Downloader {
	d.progress = fn
	return d
}

func (d *Downloader) WithThrottle(fn ThrottleFunc) *Downloader {
	d.throttle = fn
	return d
}

func (d *Downloader) WithDCID(dcID int) *Downloader {
	d.dcID = dcID
	return d
}

func (d *Downloader) WithPremium(premium bool) *Downloader {
	d.premium = premium
	return d
}

func (d *Downloader) WithStallTimeout(timeout time.Duration) *Downloader {
	d.stallTimeout = timeout
	return d
}

func (d *Downloader) Download(ctx context.Context, location tl.InputFileLocationClass, w io.Writer) (tl.StorageFileTypeClass, error) {
	return d.downloadSequential(ctx, location, 0, w)
}

func (d *Downloader) DownloadParallel(ctx context.Context, location tl.InputFileLocationClass, w io.WriterAt) (tl.StorageFileTypeClass, error) {
	if d.threads <= 1 {
		return d.downloadSequential(ctx, location, 0, &writerAtAdapter{w: w})
	}
	return d.downloadParallel(ctx, location, 0, w)
}

func (d *Downloader) DownloadToFile(ctx context.Context, location tl.InputFileLocationClass, path string) (tl.StorageFileTypeClass, error) {
	f, err := os.Create(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("downloader: create file: %w", err)
	}
	typ, runErr := d.DownloadParallel(ctx, location, f)
	closeErr := f.Close()
	if runErr != nil {
		return nil, runErr
	}
	if closeErr != nil {
		return nil, fmt.Errorf("downloader: close file: %w", closeErr)
	}
	return typ, nil
}

func (d *Downloader) downloadSequential(ctx context.Context, location tl.InputFileLocationClass, offset int64, w io.Writer) (tl.StorageFileTypeClass, error) {
	dcID := d.dcID
	kind := d.connectionKind(0)
	partSize := d.partSize
	var fileType tl.StorageFileTypeClass
	var downloaded int64

	if d.stallTimeout > 0 {
		stallCtx, stallCancel := context.WithCancel(ctx)
		defer stallCancel()
		go d.stallWatcher(stallCtx, stallCancel, &downloaded, d.stallTimeout)
		ctx = stallCtx
	}

	delayGate := newDownloadDelayGate()
	delayGate.enabled = kind != raw.ConnectionMain

	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := delayGate.wait(ctx); err != nil {
			return nil, err
		}

		req := &tl.UploadGetFileRequest{
			Precise:      offset != 0,
			CDNSupported: false,
			Location:     location,
			Offset:       offset + downloaded,
			Limit:        int32(partSize),
		}

		result, err := raw.InvokeWithOptions(ctx, d.client, req, raw.InvokeOptions{
			DCID: dcID, Kind: kind,
		})
		if err != nil {
			if rpcErr, ok := tgerr.As(err); ok {
				switch {
				case rpcErr.IsType(tgerr.ErrFileMigrate):
					if newDC, mok := rpcErr.MigrationDC(); mok && newDC != dcID {
						dcID = newDC
						kind = d.connectionKind(0)
						if connectErr := d.client.ConnectDCWithKind(ctx, dcID, kind); connectErr != nil {
							return nil, fmt.Errorf("downloader: connect migrated DC %d: %w", dcID, connectErr)
						}
						continue
					}
				case tgerr.IsFileRefUpgradeNeeded(err):
					return nil, fmt.Errorf("downloader: file reference expired (FILEREF_UPGRADE_NEEDED)")
				}
			}
			return nil, fmt.Errorf("downloader: getFile at offset %d: %w", offset+downloaded, err)
		}

		switch r := result.(type) {
		case *tl.UploadFile:
			if fileType == nil {
				fileType = r.Type
			}
			chunk := r.Bytes
			if len(chunk) == 0 {
				return fileType, nil
			}
			if _, err := w.Write(chunk); err != nil {
				return nil, fmt.Errorf("downloader: write: %w", err)
			}
			downloaded += int64(len(chunk))
			if d.progress != nil {
				d.progress(downloaded, -1)
			}
			if len(chunk) < partSize {
				return fileType, nil
			}
		case *tl.UploadFileCDNRedirect:
			return nil, fmt.Errorf("downloader: CDN redirect not supported (file_token=%x dc=%d)", r.FileToken, r.DCID)
		default:
			return nil, fmt.Errorf("downloader: unexpected response type: %T", result)
		}
	}
}

func (d *Downloader) downloadParallel(ctx context.Context, location tl.InputFileLocationClass, offset int64, w io.WriterAt) (tl.StorageFileTypeClass, error) {
	dcID := d.dcID
	kind := d.connectionKind(0)
	partSize := d.partSize

	if d.stallTimeout > 0 {
		stallCtx, stallCancel := context.WithCancel(ctx)
		defer stallCancel()
		ctx = stallCtx
	}

	delayGate := newDownloadDelayGate()
	delayGate.enabled = kind != raw.ConnectionMain

	workers := d.threads * requestsPerConnection

	var (
		mu             sync.Mutex
		cond           = sync.NewCond(&mu)
		buffer         = make(map[int][]byte)
		nextChunkIdx   int
		workerChunkIdx int
		ended          bool
		fileType       tl.StorageFileTypeClass
		firstErr       error
		downloaded     int64
		wg             sync.WaitGroup
	)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	callPart := func(chunkIdx int) ([]byte, error) {
		for {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if err := delayGate.wait(ctx); err != nil {
				return nil, err
			}

			req := &tl.UploadGetFileRequest{
				Precise:      offset != 0,
				CDNSupported: false,
				Location:     location,
				Offset:       offset + int64(chunkIdx)*int64(partSize),
				Limit:        int32(partSize),
			}

			result, err := raw.InvokeWithOptions(ctx, d.client, req, raw.InvokeOptions{
				DCID: dcID, Kind: kind,
			})
			if err != nil {
				if rpcErr, ok := tgerr.As(err); ok {
					switch {
					case rpcErr.IsType(tgerr.ErrFileMigrate):
						if newDC, mok := rpcErr.MigrationDC(); mok && newDC != dcID {
							dcID = newDC
							kind = d.connectionKind(0)
							if connectErr := d.client.ConnectDCWithKind(ctx, dcID, kind); connectErr != nil {
								return nil, connectErr
							}
							continue
						}
					case tgerr.IsFileRefUpgradeNeeded(err):
						return nil, fmt.Errorf("downloader: file reference expired")
					}
				}
				return nil, err
			}

			switch r := result.(type) {
			case *tl.UploadFile:
				mu.Lock()
				if fileType == nil {
					fileType = r.Type
				}
				mu.Unlock()
				return r.Bytes, nil
			case *tl.UploadFileCDNRedirect:
				return nil, fmt.Errorf("downloader: CDN redirect not supported")
			default:
				return nil, fmt.Errorf("downloader: unexpected response type: %T", result)
			}
		}
	}

	var stallDownloaded int64
	if d.stallTimeout > 0 {
		go d.stallWatcher(ctx, cancel, &stallDownloaded, d.stallTimeout)
	}

	downloadWorker := func() {
		defer wg.Done()
		for {
			mu.Lock()
			if firstErr != nil || ended {
				mu.Unlock()
				return
			}
			chunkIdx := workerChunkIdx
			workerChunkIdx++
			mu.Unlock()

			if d.throttle != nil {
				if ch := d.throttle(partSize); ch != nil {
					select {
					case <-ch:
					case <-ctx.Done():
						return
					}
				}
			}

			data, err := callPart(chunkIdx)
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
					cancel()
				}
				mu.Unlock()
				return
			}

			mu.Lock()
			if ended {
				mu.Unlock()
				return
			}
			buffer[chunkIdx] = data
			atomic.StoreInt64(&stallDownloaded, atomic.LoadInt64(&stallDownloaded)+int64(len(data)))

			for {
				chunk, ok := buffer[nextChunkIdx]
				if !ok {
					break
				}
				delete(buffer, nextChunkIdx)
				mu.Unlock()

				if len(chunk) > 0 {
					if _, err := w.WriteAt(chunk, offset+int64(nextChunkIdx)*int64(partSize)); err != nil {
						mu.Lock()
						if firstErr == nil {
							firstErr = fmt.Errorf("downloader: writeAt chunk %d: %w", nextChunkIdx, err)
							cancel()
						}
						mu.Unlock()
						return
					}
				}

				mu.Lock()
				downloaded += int64(len(chunk))
				if d.progress != nil {
					d.progress(downloaded, -1)
				}
				nextChunkIdx++
				if len(chunk) < partSize {
					ended = true
					cond.Broadcast()
					mu.Unlock()
					return
				}
			}
			cond.Broadcast()
			mu.Unlock()
		}
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go downloadWorker()
	}
	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}
	return fileType, nil
}

func (d *Downloader) stallWatcher(ctx context.Context, cancel context.CancelFunc, downloaded *int64, timeout time.Duration) {
	ticker := time.NewTicker(timeout / 2)
	defer ticker.Stop()
	var last int64
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			current := atomic.LoadInt64(downloaded)
			if current == last {
				cancel()
				return
			}
			last = current
		}
	}
}

func (d *Downloader) connectionKind(fileSize int64) raw.ConnectionKind {
	if fileSize > 0 && fileSize <= smallFileMaxSize {
		return raw.ConnectionMain
	}
	return raw.ConnectionDownload
}

type downloadDelayGate struct {
	enabled     bool
	mu          sync.Mutex
	last        time.Time
	minInterval time.Duration
}

func newDownloadDelayGate() *downloadDelayGate {
	return &downloadDelayGate{minInterval: 50 * time.Millisecond}
}

func (g *downloadDelayGate) wait(ctx context.Context) error {
	if !g.enabled {
		return nil
	}
	g.mu.Lock()
	elapsed := time.Since(g.last)
	wait := g.minInterval - elapsed
	if wait > 0 {
		g.mu.Unlock()
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		g.mu.Lock()
	}
	g.last = time.Now()
	g.mu.Unlock()
	return nil
}

type writerAtAdapter struct {
	w io.WriterAt
	n atomic.Int64
}

func (a *writerAtAdapter) Write(p []byte) (int, error) {
	off := a.n.Load()
	n, err := a.w.WriteAt(p, off)
	if n > 0 {
		a.n.Add(int64(n))
	}
	return n, err
}
