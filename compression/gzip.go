package compression

import (
	"bytes"
	"io"
	"sync"

	"github.com/klauspost/compress/gzip"
)

// Compression levels for Gzip. These match the standard flate level integers.
const (
	LevelDefault   = gzip.DefaultCompression
	LevelBestSpeed = gzip.BestSpeed
	LevelBest      = gzip.BestCompression
	LevelHuffman   = gzip.HuffmanOnly
)

var gzipWriterPool sync.Pool
var gzipReaderPool sync.Pool

// Gzip compresses src at the given flate level and returns the gzip stream.
// Use LevelDefault for balanced compression.
func Gzip(src []byte, level int) ([]byte, error) {
	writer := acquireGzipWriter(level)
	defer putGzipWriter(writer)

	var output bytes.Buffer
	writer.writer.Reset(&output)
	if _, err := writer.writer.Write(src); err != nil {
		return nil, err
	}
	if err := writer.writer.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

// Gunzip decompresses a gzip stream. If maxBytes is positive, decompression
// returns io.ErrShortBuffer when the output would exceed it. A zero or negative
// limit disables the size check.
func Gunzip(src []byte, maxBytes int) ([]byte, error) {
	reader := acquireGzipReader()
	defer putGzipReader(reader)

	if err := reader.Reset(bytes.NewReader(src)); err != nil {
		return nil, err
	}
	reader.Multistream(false)

	output := make([]byte, 0, gunzipInitialCapacity(src, maxBytes))
	for {
		if len(output) == cap(output) {
			if maxBytes > 0 && len(output) >= maxBytes {
				var probe [1]byte
				n, err := reader.Read(probe[:])
				switch {
				case n > 0:
					return nil, io.ErrShortBuffer
				case err == io.EOF:
					return output, nil
				case err != nil:
					return nil, err
				default:
					return nil, io.ErrNoProgress
				}
			}

			next := cap(output) * 2
			if next == 0 {
				next = 512
			}
			if maxBytes > 0 && next > maxBytes {
				next = maxBytes
			}
			grown := make([]byte, len(output), next)
			copy(grown, output)
			output = grown
		}

		start := len(output)
		n, err := reader.Read(output[start:cap(output)])
		output = output[:start+n]
		if err != nil {
			if err == io.EOF {
				return output, nil
			}
			return nil, err
		}
		if n == 0 {
			return nil, io.ErrNoProgress
		}
	}
}

// gunzipInitialCapacity reads the ISIZE field from the gzip trailer to
// pre-size the output buffer, avoiding repeated growth and copies.
func gunzipInitialCapacity(src []byte, maxBytes int) int {
	const trailerSize = 8
	if len(src) < trailerSize {
		return initialGunzipCapacity(maxBytes)
	}

	size := int(src[len(src)-4]) | int(src[len(src)-3])<<8 |
		int(src[len(src)-2])<<16 | int(src[len(src)-1])<<24
	if size <= 0 {
		return initialGunzipCapacity(maxBytes)
	}
	if maxBytes > 0 && size > maxBytes {
		return initialGunzipCapacity(maxBytes)
	}
	return size
}

func initialGunzipCapacity(maxBytes int) int {
	const defaultCapacity = 512
	if maxBytes > 0 && maxBytes < defaultCapacity {
		return maxBytes
	}
	return defaultCapacity
}

type pooledGzipWriter struct {
	writer *gzip.Writer
	level  int
}

func acquireGzipWriter(level int) *pooledGzipWriter {
	if value := gzipWriterPool.Get(); value != nil {
		writer := value.(*pooledGzipWriter)
		if writer.level == level {
			return writer
		}
	}

	writer, err := gzip.NewWriterLevel(io.Discard, level)
	if err != nil {
		writer, _ = gzip.NewWriterLevel(io.Discard, gzip.DefaultCompression)
		level = gzip.DefaultCompression
	}
	return &pooledGzipWriter{writer: writer, level: level}
}

func putGzipWriter(writer *pooledGzipWriter) {
	writer.writer.Reset(io.Discard)
	gzipWriterPool.Put(writer)
}

func acquireGzipReader() *gzip.Reader {
	if value := gzipReaderPool.Get(); value != nil {
		return value.(*gzip.Reader)
	}
	return new(gzip.Reader)
}

func putGzipReader(reader *gzip.Reader) {
	_ = reader.Reset(bytes.NewReader(nil))
	gzipReaderPool.Put(reader)
}
