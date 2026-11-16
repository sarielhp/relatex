// Package zgzip writes gzip files the way Ruby's Zlib::GzipWriter writes them at a quick level
// with mtime 0: a header with no name, mtime 0, extra flags 0 and the Unix OS byte, the deflate
// stream, and the CRC-32 and length trailer. The deflate stream is Go's compress/flate on every
// platform (decision 129, D4): it needs no C toolchain, so a Windows build is pure Go, and the
// bytes are the same wherever relatex runs. They differ from the system zlib's, which the
// package used before, so the hashes of compressed files changed once. Reading uses
// compress/gzip, which accepts any valid stream.
package zgzip

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
)

// header is a gzip header: magic, deflate, no flags, mtime 0, no stated compression level, Unix.
var header = []byte{0x1f, 0x8b, 8, 0, 0, 0, 0, 0, 0, 3}

// Default and Best are zlib's Z_DEFAULT_COMPRESSION and Z_BEST_COMPRESSION (compress/flate uses
// the same numbers); Quick is level 3, a third of the default level's time for a sixth more bytes
// on a large document.
const (
	Default = flate.DefaultCompression
	Quick   = 3
	Best    = flate.BestCompression
)

// Deflate is the raw deflate stream (no zlib or gzip framing) of plain at a level.
func Deflate(plain []byte, level int) ([]byte, error) {
	var output bytes.Buffer
	writer, err := flate.NewWriter(&output, level)
	if err != nil {
		return nil, fmt.Errorf("deflate at level %d: %w", level, err)
	}
	if _, err := writer.Write(plain); err != nil {
		return nil, fmt.Errorf("deflate: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("deflate: %w", err)
	}
	return output.Bytes(), nil
}

// Compress is a gzip stream at the quick level (a bundle of hundreds of megabytes is a matter of seconds at the best level) with mtime 0, writing plain once.
func Compress(plain []byte) ([]byte, error) {
	deflated, err := Deflate(plain, Quick)
	if err != nil {
		return nil, err
	}
	result := make([]byte, 0, len(header)+len(deflated)+8)
	result = append(result, header...)
	result = append(result, deflated...)
	trailer := make([]byte, 8)
	binary.LittleEndian.PutUint32(trailer, crc32.ChecksumIEEE(plain))
	binary.LittleEndian.PutUint32(trailer[4:], uint32(len(plain)))
	return append(result, trailer...), nil
}

// Decompress reads a gzip stream, at most limit+1 bytes of it (Ruby's read(MAX_BYTES + 1)).
func Decompress(compressed []byte, limit int64) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(io.LimitReader(reader, limit+1))
}
