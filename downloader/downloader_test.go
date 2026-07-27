package downloader

import (
	"context"
	"testing"
	"time"

	"github.com/mtgo-labs/raw/tl"
)

func TestNewDownloaderDefaults(t *testing.T) {
	d := NewDownloader(nil)
	if d.client != nil {
		t.Error("expected nil client")
	}
	if d.partSize != defaultPartSize {
		t.Errorf("partSize = %d, want %d", d.partSize, defaultPartSize)
	}
	if d.threads != 1 {
		t.Errorf("threads = %d, want 1", d.threads)
	}
	if d.progress != nil {
		t.Error("expected nil progress")
	}
}

func TestWithPartSize(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want int
	}{
		{"default", defaultPartSize, defaultPartSize},
		{"too small", 1024, 4096},
		{"already aligned", 100 * 1024, 100 * 1024},
		{"rounds down", 7000, 4096},
		{"exact 4K", 4096, 4096},
		{"max", maxPartSize, maxPartSize},
		{"above max", 1024 * 1024, maxPartSize},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDownloader(nil).WithPartSize(tt.in)
			if d.partSize != tt.want {
				t.Errorf("WithPartSize(%d) = %d, want %d", tt.in, d.partSize, tt.want)
			}
		})
	}
}

func TestWithThreads(t *testing.T) {
	d := NewDownloader(nil).WithThreads(4)
	if d.threads != 4 {
		t.Errorf("threads = %d, want 4", d.threads)
	}
	d.WithThreads(0)
	if d.threads != 1 {
		t.Errorf("threads after 0 = %d, want 1", d.threads)
	}
}

func TestWithStallTimeout(t *testing.T) {
	d := NewDownloader(nil).WithStallTimeout(30 * time.Second)
	if d.stallTimeout != 30*time.Second {
		t.Errorf("stallTimeout = %v, want 30s", d.stallTimeout)
	}
}

func TestWithDCID(t *testing.T) {
	d := NewDownloader(nil).WithDCID(4)
	if d.dcID != 4 {
		t.Errorf("dcID = %d, want 4", d.dcID)
	}
}

func TestWithPremium(t *testing.T) {
	d := NewDownloader(nil).WithPremium(true)
	if !d.premium {
		t.Error("expected premium=true")
	}
}

func TestConnectionKind(t *testing.T) {
	tests := []struct {
		name     string
		fileSize int64
		wantKind int // raw.ConnectionMain=0, raw.ConnectionDownload=2
	}{
		{"unknown size", 0, 2},
		{"small file", 100 * 1024, 0},
		{"exactly 128KB", smallFileMaxSize, 0},
		{"above 128KB", smallFileMaxSize + 1, 2},
		{"large file", 10 * 1024 * 1024, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDownloader(nil)
			got := d.connectionKind(tt.fileSize)
			if int(got) != tt.wantKind {
				t.Errorf("connectionKind(%d) = %d, want %d", tt.fileSize, int(got), tt.wantKind)
			}
		})
	}
}

func TestDownloadDelayGate(t *testing.T) {
	t.Run("disabled gate passes through", func(t *testing.T) {
		g := &downloadDelayGate{enabled: false}
		if err := g.wait(context.Background()); err != nil {
			t.Errorf("disabled gate should not error: %v", err)
		}
	})

	t.Run("enabled gate blocks briefly", func(t *testing.T) {
		g := newDownloadDelayGate()
		g.enabled = true
		start := time.Now()
		if err := g.wait(context.Background()); err != nil {
			t.Errorf("first wait: %v", err)
		}
		elapsed := time.Since(start)
		// First call should be immediate.
		if elapsed > 10*time.Millisecond {
			t.Errorf("first wait took %v, expected <10ms", elapsed)
		}
		// Second call should block.
		start = time.Now()
		if err := g.wait(context.Background()); err != nil {
			t.Errorf("second wait: %v", err)
		}
		elapsed = time.Since(start)
		if elapsed < 20*time.Millisecond {
			t.Errorf("second wait took %v, expected >=20ms gating", elapsed)
		}
	})
}

func TestProgressCallback(t *testing.T) {
	called := false
	var lastDownloaded, lastTotal int64
	d := NewDownloader(nil).WithProgress(func(downloaded, total int64) {
		called = true
		lastDownloaded = downloaded
		lastTotal = total
	})
	d.progress(1024, -1)
	if !called {
		t.Error("progress callback was not called")
	}
	if lastDownloaded != 1024 {
		t.Errorf("downloaded = %d, want 1024", lastDownloaded)
	}
	if lastTotal != -1 {
		t.Errorf("total = %d, want -1", lastTotal)
	}
}

func TestUploadGetFileRequestFields(t *testing.T) {
	loc := &tl.InputFileLocation{VolumeID: 1, LocalID: 2, Secret: 3, FileReference: []byte{1, 2, 3}}
	req := &tl.UploadGetFileRequest{
		Precise:      true,
		CDNSupported: false,
		Location:     loc,
		Offset:       0,
		Limit:        65536,
	}
	if req.Limit != 65536 {
		t.Errorf("Limit = %d, want 65536", req.Limit)
	}
	if !req.Precise {
		t.Error("expected Precise=true")
	}
}

func TestCDNRedirectType(t *testing.T) {
	var _ tl.UploadFileClass = &tl.UploadFileCDNRedirect{
		FileToken:     []byte{1},
		DCID:          4,
		EncryptionKey: []byte{2},
		EncryptionIv:  []byte{3},
	}
}
