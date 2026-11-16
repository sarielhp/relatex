package epub

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"hash/crc32"
	"os"

	"github.com/sarielhp/relatex/internal/rec"
)

// entry is one file of the ZIP container.
type entry struct {
	name string
	data []byte
}

// ZIP methods and the fixed timestamp of every entry: 1980-01-01 00:00, the earliest MS-DOS date.
const (
	methodStore   = 0
	methodDeflate = 8
	dosDate       = 1<<5 | 1
	dosTime       = 0
)

// writeArchive writes the container: mimetype first and stored, then the entries in the order
// given, deflated. Every entry has the same fixed timestamp and no extra field (EPUB forbids one on
// mimetype; the publication's date is in the package document), so the same entries give the same
// bytes. The archive is written by hand because archive/zip's raw entries take no timestamp
// without deprecated fields; it is plain ZIP: local headers, data, central directory, end record.
func writeArchive(path string, entries []entry) {
	var archive, directory bytes.Buffer
	all := append([]entry{{"mimetype", []byte("application/epub+zip")}}, entries...)
	for index, file := range all {
		method, data := uint16(methodDeflate), deflate(file.data)
		if index == 0 {
			method, data = methodStore, file.data
		}
		offset := archive.Len()
		header := entryHeader(file, method, data)
		archive.Write(le(uint32(0x04034b50), uint16(20)))
		archive.Write(header)
		archive.WriteString(file.name)
		archive.Write(data)
		directory.Write(le(uint32(0x02014b50), uint16(20), uint16(20)))
		directory.Write(header)
		directory.Write(le(uint16(0), uint16(0), uint16(0), uint32(0), uint32(offset)))
		directory.WriteString(file.name)
	}
	start := archive.Len()
	archive.Write(directory.Bytes())
	count := uint16(len(all))
	archive.Write(le(uint32(0x06054b50), uint16(0), uint16(0), count, count, uint32(directory.Len()), uint32(start), uint16(0)))
	if err := os.WriteFile(path, archive.Bytes(), 0o644); err != nil {
		rec.Fail("%v", err)
	}
}

// entryHeader is what the local and central headers share after their version fields: flags
// (UTF-8 names), method, time, date, CRC-32, sizes, name length and an empty extra field.
func entryHeader(file entry, method uint16, data []byte) []byte {
	if len(file.data) > 0xFFFFFFFE || len(data) > 0xFFFFFFFE {
		rec.Fail("EPUB entry too large for ZIP: %s", file.name)
	}
	return le(uint16(0x800), method, uint16(dosTime), uint16(dosDate), crc32.ChecksumIEEE(file.data),
		uint32(len(data)), uint32(len(file.data)), uint16(len(file.name)), uint16(0))
}

// le is the values in little-endian order, as ZIP stores them.
func le(values ...any) []byte {
	var buffer bytes.Buffer
	for _, value := range values {
		if err := binary.Write(&buffer, binary.LittleEndian, value); err != nil {
			rec.Fail("%v", err)
		}
	}
	return buffer.Bytes()
}

func deflate(data []byte) []byte {
	var buffer bytes.Buffer
	writer, err := flate.NewWriter(&buffer, flate.BestCompression)
	if err != nil {
		rec.Fail("%v", err)
	}
	if _, err := writer.Write(data); err != nil {
		rec.Fail("%v", err)
	}
	if err := writer.Close(); err != nil {
		rec.Fail("%v", err)
	}
	return buffer.Bytes()
}
