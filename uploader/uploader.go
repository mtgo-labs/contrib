// Package uploader provides high-level Telegram file upload helpers
// for the mtgo-labs/raw MTProto client, matching the mtcute TypeScript
// uploader patterns.
//
// Usage:
//
//	u := uploader.NewUploader(client)
//	uploaded, err := u.Upload(ctx, uploader.FromPath("/path/to/file.jpg"))
//	// uploaded.InputFile can be used in sendMedia, etc.
package uploader

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	raw "github.com/mtgo-labs/raw"
	"github.com/mtgo-labs/raw/tl"
)

// smallFileMaxSize is the threshold (128KB) below which files are uploaded
// using the main connection instead of the upload pool.
const smallFileMaxSize = 131072

// bigFileMinSize is the threshold (10MB) above which files use
// saveBigFilePart instead of saveFilePart.
const bigFileMinSize = 10485760

// defaultPartSize is the default upload chunk size in bytes.
const defaultPartSize = 64 * 1024 // 64KB

// maxPartSize is the maximum upload chunk size (512KB).
const maxPartSize = 512 * 1024

// maxPartCount is the maximum number of parts for a non-premium upload.
const maxPartCount = 4000

// maxPartCountPremium allows double the parts for premium users.
const maxPartCountPremium = 8000

// defaultRequestsPerConnection is the number of concurrent part sends per
// connection (matches mtcute's REQUESTS_PER_CONNECTION).
const defaultRequestsPerConnection = 3

// defaultFileName is used when no file name can be inferred.
const defaultFileName = "unnamed"

// ProgressFunc reports upload progress.
type ProgressFunc func(uploaded int64, total int64)

// Input is the source for an upload.
type Input struct {
	// File is the data source. Exactly one of these must be set.
	File     io.Reader // Read from any io.Reader
	FilePath string    // Read from a file path

	// FileName is the file name sent to Telegram. Defaults to file path
	// basename, or "unnamed" if uploading from a Reader.
	FileName string

	// FileSize is the total size in bytes. Set for io.Reader sources where
	// the size is known; if zero, the file is treated as streamed (big file
	// path, unknown total parts).
	FileSize int64

	// EstimatedSize is used when FileSize is unknown, to determine the
	// initial part size.
	EstimatedSize int64

	// MimeType is the file's MIME type. If empty, it is auto-detected from
	// the first chunk's magic bytes.
	MimeType string

	// RequireFileSize buffers the entire stream to determine its size before
	// uploading. Required for inputMediaUploadedPhoto.
	RequireFileSize bool

	// RequireExtension guesses the file extension from the MIME type. Used
	// when sending as inputMediaUploadedPhoto.
	RequireExtension bool
}

// UploadedFile is the result of an upload, ready to be used in
// sendMedia, sendDocument, etc.
type UploadedFile struct {
	// InputFile is the TL InputFile or InputFileBig.
	InputFile tl.InputFileClass

	// FileName is the resolved file name.
	FileName string

	// FileSize is the total byte count that was uploaded.
	FileSize int64

	// MimeType is the resolved MIME type.
	MimeType string
}

// Uploader uploads files to Telegram servers, matching the mtcute
// uploadFile patterns.
type Uploader struct {
	client *raw.Client

	partSize              int
	autoPartSize          bool
	requestsPerConnection int
	progress              ProgressFunc
	premium               bool
	poolSize              int // 0 = auto (use requestsPerConnection)
}

// NewUploader creates an Uploader backed by the given raw Client.
func NewUploader(client *raw.Client) *Uploader {
	return &Uploader{
		client:                client,
		partSize:              defaultPartSize,
		autoPartSize:          true,
		requestsPerConnection: defaultRequestsPerConnection,
		poolSize:              0,
	}
}

// WithPartSize sets the chunk size in bytes. Must be divisible by 1024,
// max 512KB. Setting this disables automatic part size computation.
func (u *Uploader) WithPartSize(size int) *Uploader {
	if size < 1024 {
		size = 1024
	}
	if size > maxPartSize {
		size = maxPartSize
	}
	u.partSize = size
	u.autoPartSize = false
	return u
}

// WithRequestsPerConnection sets the number of concurrent part sends per
// connection (default 3, matches mtcute).
func (u *Uploader) WithRequestsPerConnection(n int) *Uploader {
	if n < 1 {
		n = 1
	}
	u.requestsPerConnection = n
	return u
}

// WithPoolSize sets the explicit connection pool size. When 0 (default),
// the uploader uses WithRequestsPerConnection as the effective concurrency.
func (u *Uploader) WithPoolSize(n int) *Uploader {
	if n < 0 {
		n = 0
	}
	u.poolSize = n
	return u
}

// WithProgress sets the progress callback.
func (u *Uploader) WithProgress(fn ProgressFunc) *Uploader {
	u.progress = fn
	return u
}

// WithPremium enables premium part-count limits (8000 vs 4000).
func (u *Uploader) WithPremium(premium bool) *Uploader {
	u.premium = premium
	return u
}

// FromPath creates an Input from a file path. FileSize and FileName are
// inferred from the file.
func FromPath(path string) Input {
	return Input{FilePath: path}
}

// FromReader creates an Input from an io.Reader. fileName and fileSize
// should be provided; fileSize can be 0 for streamed uploads.
func FromReader(r io.Reader, fileName string, fileSize int64) Input {
	return Input{File: r, FileName: fileName, FileSize: fileSize}
}

// Upload uploads the file described by input and returns an UploadedFile
// whose InputFile field can be used in sendMedia and similar methods.
func (u *Uploader) Upload(ctx context.Context, input Input) (*UploadedFile, error) {
	reader, fileName, fileSize, err := u.resolveInput(input)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closer, ok := reader.(io.Closer); ok {
			closer.Close()
		}
	}()

	// Auto-detect MIME type from first chunk if not provided.
	mimeType := input.MimeType

	// Determine part size.
	partSize := u.partSize
	if u.autoPartSize {
		targetSize := fileSize
		if targetSize <= 0 && input.EstimatedSize > 0 {
			targetSize = input.EstimatedSize
		}
		partSize = computePartSize(targetSize, partSize)
	}

	// Validate part count.
	if fileSize > 0 {
		maxPC := maxPartCount
		if u.premium {
			maxPC = maxPartCountPremium
		}
		partCount := (fileSize + int64(partSize) - 1) / int64(partSize)
		if partCount > int64(maxPC) {
			return nil, fmt.Errorf("uploader: file too large (%d parts > %d max)", partCount, maxPC)
		}
	}

	isBig := fileSize <= 0 || fileSize > bigFileMinSize
	isSmall := fileSize > 0 && fileSize < smallFileMaxSize
	kind := raw.ConnectionUpload
	if isSmall {
		kind = raw.ConnectionMain
	}

	// Determine effective concurrency.
	workers := u.requestsPerConnection
	if u.poolSize > 0 {
		workers = u.poolSize * u.requestsPerConnection
	}
	// Streamed uploads must be serialized; otherwise we get FILE_PART_SIZE_INVALID.
	if fileSize <= 0 {
		workers = 1
	}

	fileID := randomInt64()

	if mimeType == "" {
		// Peek at the first bytes for MIME detection.
		mimeType, reader = detectMimeType(reader)
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	// Override: Telegram needs audio/ogg for voice messages.
	if mimeType == "audio/opus" {
		mimeType = "audio/ogg"
	}

	if isBig {
		return u.uploadBig(ctx, reader, fileID, fileName, fileSize, mimeType, partSize, kind, workers)
	}
	return u.uploadSmall(ctx, reader, fileID, fileName, fileSize, mimeType, partSize, kind, workers)
}

// resolveInput normalizes the input into an io.Reader with metadata.
func (u *Uploader) resolveInput(input Input) (io.Reader, string, int64, error) {
	if input.FilePath != "" {
		f, err := os.Open(filepath.Clean(input.FilePath))
		if err != nil {
			return nil, "", 0, fmt.Errorf("uploader: open file: %w", err)
		}
		fi, err := f.Stat()
		if err != nil {
			f.Close()
			return nil, "", 0, fmt.Errorf("uploader: stat file: %w", err)
		}
		fileName := input.FileName
		if fileName == "" {
			fileName = filepath.Base(input.FilePath)
		}
		return f, fileName, fi.Size(), nil
	}

	if input.File != nil {
		reader := input.File
		fileName := input.FileName
		if fileName == "" {
			fileName = defaultFileName
		}

		// If requireFileSize and size is unknown, buffer the stream.
		if input.RequireFileSize && input.FileSize <= 0 {
			data, err := io.ReadAll(reader)
			if err != nil {
				return nil, "", 0, fmt.Errorf("uploader: buffer stream: %w", err)
			}
			return &byteReader{data: data}, fileName, int64(len(data)), nil
		}

		return reader, fileName, input.FileSize, nil
	}

	return nil, "", 0, fmt.Errorf("uploader: no input source (set FilePath or File)")
}

// uploadSmall uploads a small file (<=10MB) with MD5 checksum.
func (u *Uploader) uploadSmall(ctx context.Context, reader io.Reader, fileID int64, fileName string, fileSize int64, mimeType string, partSize int, kind raw.ConnectionKind, workers int) (*UploadedFile, error) {
	h := md5.New()
	lockedReader := &lockedReader{r: reader}

	var (
		partNum   atomic.Int32
		uploaded  atomic.Int64
		readErr   error
		readErrMu sync.Mutex
		readEnded atomic.Bool
		wg        sync.WaitGroup
		firstErr  error
		firstErrMu sync.Mutex
	)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	setReadErr := func(err error) {
		readErrMu.Lock()
		if readErr == nil {
			readErr = err
		}
		readErrMu.Unlock()
	}

	setFirstErr := func(err error) {
		firstErrMu.Lock()
		if firstErr == nil {
			firstErr = err
			cancel()
		}
		firstErrMu.Unlock()
	}

	uploadWorker := func() {
		defer wg.Done()
		for {
			if ctx.Err() != nil {
				return
			}

			idx := partNum.Add(1) - 1

			// Read the next part under the lock.
			lockedReader.mu.Lock()
			if readEnded.Load() {
				lockedReader.mu.Unlock()
				return
			}
			buf := make([]byte, partSize)
			n, err := io.ReadFull(lockedReader.r, buf)
			lockedReader.mu.Unlock()

			if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
				setReadErr(err)
				setFirstErr(fmt.Errorf("uploader: read part %d: %w", idx, err))
				return
			}

			part := buf[:n]
			if n == 0 {
				readEnded.Store(true)
				return
			}

			h.Write(part)

			req := &tl.UploadSaveFilePartRequest{
				FileID:   fileID,
				FilePart: idx,
				Bytes:    part,
			}

			if _, rpcErr := raw.InvokeWithOptions(ctx, u.client, req, raw.InvokeOptions{
				Kind: kind,
			}); rpcErr != nil {
				setFirstErr(fmt.Errorf("uploader: saveFilePart %d: %w", idx, rpcErr))
				return
			}

			uploaded.Add(int64(n))
			if u.progress != nil {
				u.progress(uploaded.Load(), fileSize)
			}

			if err == io.ErrUnexpectedEOF || err == io.EOF {
				readEnded.Store(true)
				return
			}
		}
	}

	// Start workers.
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go uploadWorker()
	}
	wg.Wait()

	readErrMu.Lock()
	re := readErr
	readErrMu.Unlock()
	if re != nil && re != io.EOF && re != io.ErrUnexpectedEOF {
		return nil, re
	}

	firstErrMu.Lock()
	fe := firstErr
	firstErrMu.Unlock()
	if fe != nil {
		return nil, fe
	}

	md5Hex := hex.EncodeToString(h.Sum(nil))

	return &UploadedFile{
		InputFile: &tl.InputFile{
			ID:          fileID,
			Parts:       partNum.Load(),
			Name:        fileName,
			Md5Checksum: md5Hex,
		},
		FileName: fileName,
		FileSize: fileSize,
		MimeType: mimeType,
	}, nil
}

// uploadBig uploads a large file (>10MB) without MD5 checksum.
func (u *Uploader) uploadBig(ctx context.Context, reader io.Reader, fileID int64, fileName string, fileSize int64, mimeType string, partSize int, kind raw.ConnectionKind, workers int) (*UploadedFile, error) {
	lockedReader := &lockedReader{r: reader}

	var totalParts int32
	if fileSize > 0 {
		totalParts = int32((fileSize + int64(partSize) - 1) / int64(partSize))
	} else {
		totalParts = -1 // streamed upload
	}

	var (
		partNum   atomic.Int32
		uploaded  atomic.Int64
		readErr   error
		readErrMu sync.Mutex
		readEnded atomic.Bool
		wg        sync.WaitGroup
		firstErr  error
		firstErrMu sync.Mutex
	)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	setReadErr := func(err error) {
		readErrMu.Lock()
		if readErr == nil {
			readErr = err
		}
		readErrMu.Unlock()
	}

	setFirstErr := func(err error) {
		firstErrMu.Lock()
		if firstErr == nil {
			firstErr = err
			cancel()
		}
		firstErrMu.Unlock()
	}

	uploadWorker := func() {
		defer wg.Done()
		for {
			if ctx.Err() != nil {
				return
			}

			idx := partNum.Add(1) - 1

			lockedReader.mu.Lock()
			if readEnded.Load() {
				lockedReader.mu.Unlock()
				return
			}
			buf := make([]byte, partSize)
			n, err := io.ReadFull(lockedReader.r, buf)
			lockedReader.mu.Unlock()

			if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
				setReadErr(err)
				setFirstErr(fmt.Errorf("uploader: read part %d: %w", idx, err))
				return
			}
			if n == 0 {
				readEnded.Store(true)
				return
			}

			req := &tl.UploadSaveBigFilePartRequest{
				FileID:         fileID,
				FilePart:       idx,
				FileTotalParts: totalParts,
				Bytes:          buf[:n],
			}

			if _, rpcErr := raw.InvokeWithOptions(ctx, u.client, req, raw.InvokeOptions{
				Kind: kind,
			}); rpcErr != nil {
				setFirstErr(fmt.Errorf("uploader: saveBigFilePart %d: %w", idx, rpcErr))
				return
			}

			uploaded.Add(int64(n))
			if u.progress != nil {
				u.progress(uploaded.Load(), fileSize)
			}

			if err == io.ErrUnexpectedEOF || err == io.EOF {
				readEnded.Store(true)
				return
			}
		}
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go uploadWorker()
	}
	wg.Wait()

	readErrMu.Lock()
	re := readErr
	readErrMu.Unlock()
	if re != nil && re != io.EOF && re != io.ErrUnexpectedEOF {
		return nil, re
	}

	firstErrMu.Lock()
	fe := firstErr
	firstErrMu.Unlock()
	if fe != nil {
		return nil, fe
	}

	return &UploadedFile{
		InputFile: &tl.InputFileBig{
			ID:    fileID,
			Parts: partNum.Load(),
			Name:  fileName,
		},
		FileName: fileName,
		FileSize: fileSize,
		MimeType: mimeType,
	}, nil
}

// lockedReader wraps an io.Reader with a mutex, matching mtcute's AsyncLock
// pattern: reads are serialized while RPC sends happen in parallel.
type lockedReader struct {
	mu sync.Mutex
	r  io.Reader
}

// byteReader is an io.Reader over an in-memory byte slice (for buffered streams).
type byteReader struct {
	data   []byte
	offset int
}

func (b *byteReader) Read(p []byte) (int, error) {
	if b.offset >= len(b.data) {
		return 0, io.EOF
	}
	n := copy(p, b.data[b.offset:])
	b.offset += n
	return n, nil
}

// detectMimeType reads up to 512 bytes from r to detect the MIME type,
// returning the MIME type and a new reader that prepends the peeked bytes.
func detectMimeType(r io.Reader) (string, io.Reader) {
	buf := make([]byte, 512)
	n, err := io.ReadFull(r, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return "", r
	}
	if n == 0 {
		return "", r
	}
	mime := http.DetectContentType(buf[:n])
	return mime, io.MultiReader(bytes.NewReader(buf[:n]), r)
}
// computePartSize computes an appropriate part size for a file, growing
// it to stay under the maximum part count.
func computePartSize(fileSize int64, currentPartSize int) int {
	if fileSize <= 0 {
		return currentPartSize
	}
	partSize := currentPartSize
	maxPC := maxPartCount
	for fileSize/int64(partSize) > int64(maxPC) && partSize < maxPartSize {
		partSize *= 2
	}
	if partSize > maxPartSize {
		partSize = maxPartSize
	}
	return partSize
}

// randomInt64 returns a cryptographically random int64 for use as a file ID.
func randomInt64() int64 {
	var buf [8]byte
	if _, err := io.ReadFull(rand.Reader, buf[:]); err != nil {
		panic("uploader: crypto/rand.Read failed: " + err.Error())
	}
	return int64(binary.LittleEndian.Uint64(buf[:]))
}
