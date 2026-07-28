package compression

import (
	"bytes"
	stdgzip "compress/gzip"
	"errors"
	"io"
	"testing"

	klauspostgzip "github.com/klauspost/compress/gzip"
)

func TestGzipRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		level int
		data  []byte
	}{
		{name: "empty", level: LevelDefault},
		{name: "small", level: LevelDefault, data: []byte("hello world")},
		{name: "best", level: LevelBest, data: []byte("hello world")},
		{name: "fastest", level: LevelBestSpeed, data: []byte("hello world")},
		{name: "huffman", level: LevelHuffman, data: []byte("hello world")},
		{name: "repetitive", level: LevelDefault, data: bytes.Repeat([]byte("gzip"), 4096)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			compressed, err := Gzip(test.data, test.level)
			if err != nil {
				t.Fatalf("Gzip: %v", err)
			}
			decompressed, err := Gunzip(compressed, 0)
			if err != nil {
				t.Fatalf("Gunzip: %v", err)
			}
			if !bytes.Equal(decompressed, test.data) {
				t.Fatal("round trip changed the payload")
			}
		})
	}
}

func TestGzipStandardLibraryCompatibility(t *testing.T) {
	source := bytes.Repeat([]byte("mtproto gzip test "), 500)
	compressed, err := Gzip(source, LevelDefault)
	if err != nil {
		t.Fatalf("Gzip: %v", err)
	}

	reader, err := stdgzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("stdlib NewReader: %v", err)
	}
	decompressed, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("stdlib ReadAll: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("stdlib Close: %v", err)
	}
	if !bytes.Equal(decompressed, source) {
		t.Fatal("stdlib decompression changed the payload")
	}

	var output bytes.Buffer
	writer := stdgzip.NewWriter(&output)
	if _, err := writer.Write(source); err != nil {
		t.Fatalf("stdlib Write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("stdlib Close: %v", err)
	}

	decompressed, err = Gunzip(output.Bytes(), 0)
	if err != nil {
		t.Fatalf("Gunzip: %v", err)
	}
	if !bytes.Equal(decompressed, source) {
		t.Fatal("Gunzip changed the stdlib payload")
	}
}

func TestGunzipLimit(t *testing.T) {
	source := bytes.Repeat([]byte("overflow"), 1024)
	compressed, err := Gzip(source, LevelDefault)
	if err != nil {
		t.Fatalf("Gzip: %v", err)
	}

	if _, err := Gunzip(compressed, len(source)-1); !errors.Is(err, io.ErrShortBuffer) {
		t.Fatalf("Gunzip below limit error = %v, want io.ErrShortBuffer", err)
	}
	decompressed, err := Gunzip(compressed, len(source))
	if err != nil {
		t.Fatalf("Gunzip at exact limit: %v", err)
	}
	if !bytes.Equal(decompressed, source) {
		t.Fatal("Gunzip at exact limit changed the payload")
	}
}

func TestGunzipExactLimitValidatesChecksum(t *testing.T) {
	source := bytes.Repeat([]byte("checksum"), 128)
	compressed, err := Gzip(source, LevelDefault)
	if err != nil {
		t.Fatalf("Gzip: %v", err)
	}
	compressed[len(compressed)-8] ^= 0xff

	if _, err := Gunzip(compressed, len(source)); !errors.Is(err, klauspostgzip.ErrChecksum) {
		t.Fatalf("Gunzip checksum error = %v, want ErrChecksum", err)
	}
}

func TestGzipInvalidLevelFallsBackToDefault(t *testing.T) {
	compressed, err := Gzip([]byte("fallback"), 9999)
	if err != nil {
		t.Fatalf("Gzip: %v", err)
	}
	decompressed, err := Gunzip(compressed, 0)
	if err != nil {
		t.Fatalf("Gunzip: %v", err)
	}
	if string(decompressed) != "fallback" {
		t.Fatalf("Gunzip = %q, want fallback", decompressed)
	}
}

func TestGzipPoolReuse(t *testing.T) {
	for range 100 {
		source := []byte("pool reuse")
		compressed, err := Gzip(source, LevelDefault)
		if err != nil {
			t.Fatalf("Gzip: %v", err)
		}
		decompressed, err := Gunzip(compressed, 0)
		if err != nil {
			t.Fatalf("Gunzip: %v", err)
		}
		if !bytes.Equal(decompressed, source) {
			t.Fatal("round trip changed the payload")
		}
	}
}
