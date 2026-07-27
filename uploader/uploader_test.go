package uploader

import (
	"bytes"
	"context"
	"crypto/md5"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mtgo-labs/raw/tl"
)

func TestComputePartSize(t *testing.T) {
	tests := []struct {
		name     string
		fileSize int64
		current  int
		want     int
	}{
		{"zero size", 0, 65536, 65536},
		{"small file", 1024, 65536, 65536},
		{"normal file", 10 * 1024 * 1024, 65536, 65536},
		{"large file grows", 3 * 1024 * 1024 * 1024, 65536, 524288},
		{"capped at max", 100 * 1024 * 1024 * 1024, 65536, 524288},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computePartSize(tt.fileSize, tt.current)
			if got != tt.want {
				t.Errorf("computePartSize(%d, %d) = %d, want %d",
					tt.fileSize, tt.current, got, tt.want)
			}
		})
	}
}

func TestFromPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}
	input := FromPath(path)
	if input.FilePath != path {
		t.Errorf("FilePath = %q, want %q", input.FilePath, path)
	}
}

func TestFromReader(t *testing.T) {
	r := strings.NewReader("hello")
	input := FromReader(r, "test.bin", 5)
	if input.FileName != "test.bin" {
		t.Errorf("FileName = %q, want %q", input.FileName, "test.bin")
	}
	if input.FileSize != 5 {
		t.Errorf("FileSize = %d, want 5", input.FileSize)
	}
}

func TestResolveInput_FilePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.bin")
	data := []byte("hello world")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	u := NewUploader(nil)
	reader, name, size, err := u.resolveInput(FromPath(path))
	if err != nil {
		t.Fatalf("resolveInput: %v", err)
	}
	defer reader.(io.Closer).Close()

	if name != "test.bin" {
		t.Errorf("name = %q, want %q", name, "test.bin")
	}
	if size != int64(len(data)) {
		t.Errorf("size = %d, want %d", size, len(data))
	}

	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, reader); err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(buf.Bytes(), data) {
		t.Error("read data mismatch")
	}
}

func TestResolveInput_Reader(t *testing.T) {
	data := []byte("hello")
	u := NewUploader(nil)
	reader, name, size, err := u.resolveInput(FromReader(bytes.NewReader(data), "custom.txt", int64(len(data))))
	if err != nil {
		t.Fatalf("resolveInput: %v", err)
	}
	if name != "custom.txt" {
		t.Errorf("name = %q, want %q", name, "custom.txt")
	}
	if size != int64(len(data)) {
		t.Errorf("size = %d, want %d", size, len(data))
	}

	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, reader); err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(buf.Bytes(), data) {
		t.Error("read data mismatch")
	}
}

func TestResolveInput_ReaderDefaultName(t *testing.T) {
	u := NewUploader(nil)
	reader, name, _, err := u.resolveInput(Input{File: strings.NewReader("x")})
	if err != nil {
		t.Fatalf("resolveInput: %v", err)
	}
	if name != defaultFileName {
		t.Errorf("default name = %q, want %q", name, defaultFileName)
	}
	_ = reader
}

func TestResolveInput_RequireFileSize(t *testing.T) {
	data := []byte("hello world")
	u := NewUploader(nil)
	reader, name, size, err := u.resolveInput(Input{
		File:            bytes.NewReader(data),
		RequireFileSize: true,
		FileName:        "test.bin",
	})
	if err != nil {
		t.Fatalf("resolveInput with requireFileSize: %v", err)
	}
	if name != "test.bin" {
		t.Errorf("name = %q", name)
	}
	if size != int64(len(data)) {
		t.Errorf("size = %d, want %d", size, len(data))
	}
	if _, ok := reader.(*byteReader); !ok {
		t.Errorf("expected *byteReader, got %T", reader)
	}
}

func TestResolveInput_NoSource(t *testing.T) {
	u := NewUploader(nil)
	_, _, _, err := u.resolveInput(Input{})
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestRandomInt64(t *testing.T) {
	a := randomInt64()
	b := randomInt64()
	if a == b {
		t.Error("two randomInt64 calls returned same value")
	}
}

func TestWithPartSize(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want int
	}{
		{"below min", 512, 1024},
		{"default", defaultPartSize, defaultPartSize},
		{"max", maxPartSize, maxPartSize},
		{"above max", 1024 * 1024, maxPartSize},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := NewUploader(nil).WithPartSize(tt.in)
			if u.partSize != tt.want {
				t.Errorf("WithPartSize(%d) = %d, want %d", tt.in, u.partSize, tt.want)
			}
			if u.autoPartSize {
				t.Error("autoPartSize should be false after explicit WithPartSize")
			}
		})
	}
}

func TestWithRequestsPerConnection(t *testing.T) {
	u := NewUploader(nil).WithRequestsPerConnection(5)
	if u.requestsPerConnection != 5 {
		t.Errorf("requestsPerConnection = %d, want 5", u.requestsPerConnection)
	}
	u.WithRequestsPerConnection(0)
	if u.requestsPerConnection != 1 {
		t.Errorf("after 0: requestsPerConnection = %d, want 1", u.requestsPerConnection)
	}
}

func TestWithPoolSize(t *testing.T) {
	u := NewUploader(nil).WithPoolSize(3)
	if u.poolSize != 3 {
		t.Errorf("poolSize = %d, want 3", u.poolSize)
	}
}

func TestWithPremium(t *testing.T) {
	u := NewUploader(nil).WithPremium(true)
	if !u.premium {
		t.Error("expected premium=true")
	}
}

func TestWithProgress(t *testing.T) {
	called := false
	u := NewUploader(nil).WithProgress(func(uploaded, total int64) {
		called = true
	})
	if u.progress == nil {
		t.Error("progress callback was not set")
	}
	u.progress(0, 100)
	if !called {
		t.Error("progress callback did not fire")
	}
}

func TestDetectMimeType(t *testing.T) {
	// JPEG magic bytes.
	jpeg := []byte{0xFF, 0xD8, 0xFF}
	mime, r := detectMimeType(bytes.NewReader(jpeg))
	if mime != "image/jpeg" {
		t.Errorf("jpeg mime = %q, want image/jpeg", mime)
	}
	// Verify reader still has the bytes.
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, jpeg) {
		t.Error("detectMimeType reader lost peeked bytes")
	}

	// PNG magic bytes.
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n', 0, 0, 0}
	mime, r = detectMimeType(bytes.NewReader(png))
	if mime != "image/png" {
		t.Errorf("png mime = %q, want image/png", mime)
	}
	out, err = io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, png) {
		t.Error("detectMimeType reader lost peeked bytes")
	}

	// Short input (< 512 bytes).
	short := []byte("hello")
	_, r = detectMimeType(bytes.NewReader(short))
	out, err = io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, short) {
		t.Error("short detectMimeType reader lost peeked bytes")
	}
}

func TestUploadedFileTypes(t *testing.T) {
	f := &UploadedFile{
		InputFile: &tl.InputFile{
			ID:          12345,
			Parts:       1,
			Name:        "test.jpg",
			Md5Checksum: "abc123",
		},
		FileName: "test.jpg",
		FileSize: 1024,
		MimeType: "image/jpeg",
	}
	if f.InputFile == nil {
		t.Error("InputFile should not be nil")
	}

	big := &UploadedFile{
		InputFile: &tl.InputFileBig{
			ID:    67890,
			Parts: 10,
			Name:  "large.bin",
		},
		FileName: "large.bin",
		FileSize: 10 * 1024 * 1024,
	}
	if big.InputFile == nil {
		t.Error("InputFileBig should not be nil")
	}
}

func TestNewUploaderDefaults(t *testing.T) {
	u := NewUploader(nil)
	if u.client != nil {
		t.Error("expected nil client")
	}
	if u.partSize != defaultPartSize {
		t.Errorf("partSize = %d, want %d", u.partSize, defaultPartSize)
	}
	if u.requestsPerConnection != defaultRequestsPerConnection {
		t.Errorf("requestsPerConnection = %d, want %d", u.requestsPerConnection, defaultRequestsPerConnection)
	}
	if u.progress != nil {
		t.Error("expected nil progress")
	}
	if !u.autoPartSize {
		t.Error("expected autoPartSize=true")
	}
}

func TestLockedReaderConcurrentReadPart(t *testing.T) {
	const partSize = 8

	tests := []struct {
		name string
		size int
	}{
		{name: "exact multiple", size: 4 * partSize},
		{name: "short final part", size: 3*partSize + 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := make([]byte, tt.size)
			for i := range data {
				data[i] = byte(i)
			}

			checksum := md5.New()
			reader := &lockedReader{
				r:        bytes.NewReader(data),
				checksum: checksum,
			}
			wantParts := (len(data) + partSize - 1) / partSize
			workers := wantParts + 4
			start := make(chan struct{})
			results := make(chan struct {
				idx  int32
				part []byte
				err  error
			}, workers)

			var wg sync.WaitGroup
			wg.Add(workers)
			for range workers {
				go func() {
					defer wg.Done()
					<-start
					idx, part, err := reader.readPart(partSize)
					results <- struct {
						idx  int32
						part []byte
						err  error
					}{idx: idx, part: part, err: err}
				}()
			}
			close(start)
			wg.Wait()
			close(results)

			ordered := make([][]byte, wantParts)
			seen := make([]bool, wantParts)
			nonEmptyParts := 0
			for result := range results {
				if result.err != nil && result.err != io.EOF && result.err != io.ErrUnexpectedEOF {
					t.Errorf("readPart() error = %v", result.err)
					continue
				}
				if len(result.part) == 0 {
					if result.err != io.EOF {
						t.Errorf("empty read error = %v, want EOF", result.err)
					}
					continue
				}

				nonEmptyParts++
				slot := int(result.idx)
				if slot < 0 || slot >= len(ordered) {
					t.Errorf("part index = %d, want [0, %d)", result.idx, len(ordered))
					continue
				}
				if seen[slot] {
					t.Errorf("duplicate part index %d", result.idx)
					continue
				}
				seen[slot] = true
				ordered[slot] = result.part
			}

			for idx, ok := range seen {
				if !ok {
					t.Errorf("missing part index %d", idx)
				}
			}
			if got, want := reader.parts, int32(nonEmptyParts); got != want {
				t.Errorf("Parts = %d, non-empty parts = %d", got, want)
			}
			if nonEmptyParts != wantParts {
				t.Errorf("non-empty parts = %d, want %d", nonEmptyParts, wantParts)
			}

			joined := make([]byte, 0, len(data))
			for _, part := range ordered {
				joined = append(joined, part...)
			}
			if !bytes.Equal(joined, data) {
				t.Errorf("ordered parts = %x, want %x", joined, data)
			}

			wantChecksum := md5.Sum(data)
			if got := checksum.Sum(nil); !bytes.Equal(got, wantChecksum[:]) {
				t.Errorf("MD5 = %x, want %x", got, wantChecksum)
			}
		})
	}
}

func TestLockedReaderReadPartReturnsUnlocked(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "full part", data: []byte("ab")},
		{name: "short final part", data: []byte("a")},
		{name: "empty EOF"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &lockedReader{r: bytes.NewReader(tt.data)}
			_, _, _ = reader.readPart(2)
			if !reader.mu.TryLock() {
				t.Fatal("readPart returned with reader mutex locked")
			}
			reader.mu.Unlock()
		})
	}
}

func TestInputFileLocationTypes(t *testing.T) {
	var _ tl.InputFileLocationClass = &tl.InputFileLocation{
		VolumeID:      1,
		LocalID:       2,
		Secret:        3,
		FileReference: []byte{1},
	}
	var _ tl.InputFileLocationClass = &tl.InputPeerPhotoFileLocation{
		Big:     false,
		Peer:    &tl.InputPeerUser{UserID: 1, AccessHash: 2},
		PhotoID: 456,
	}
	var _ tl.StorageFileTypeClass = &tl.StorageFileJPEG{}
	var _ tl.StorageFileTypeClass = &tl.StorageFilePNG{}
	var _ context.Context = context.Background()
}
