package input

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"os"
	"time"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/zgzip"
)

// zipEntry is one file of a directory snapshot as rubyzip writes it.
type zipEntry struct {
	name    string
	crc     uint32
	size    int
	packed  int
	offset  int
	extraUT []byte
}

// writeZip is Zip::File.open(path, create: true) adding each file with time 1980-01-01 00:00
// (local) and permissions 0644, as SourceSnapshot.directory_zip does: deflated at zlib's
// default level, sizes and CRC in the local header, a UT extra field with the modification
// time, internal attributes "text" and a Unix creator.
func writeZip(path string, names []string, read func(name string) []byte) {
	var output bytes.Buffer
	stamp := time.Date(1980, 1, 1, 0, 0, 0, 0, time.Local).Unix()
	extra := []byte{0x55, 0x54, 5, 0, 1, 0, 0, 0, 0}
	binary.LittleEndian.PutUint32(extra[5:], uint32(int32(stamp)))
	entries := []zipEntry{}
	for _, name := range names {
		plain := read(name)
		packed, err := zgzip.Deflate(plain, zgzip.Default)
		if err != nil {
			rec.Fail("%v", err)
		}
		entry := zipEntry{name: name, crc: crc32.ChecksumIEEE(plain), size: len(plain), packed: len(packed), offset: output.Len(),
			extraUT: extra}
		writeLocalHeader(&output, entry)
		output.Write(packed)
		entries = append(entries, entry)
	}
	start := output.Len()
	for _, entry := range entries {
		writeCentralHeader(&output, entry)
	}
	size := output.Len() - start
	end := make([]byte, 22)
	binary.LittleEndian.PutUint32(end, 0x06054b50)
	binary.LittleEndian.PutUint16(end[8:], uint16(len(entries)))
	binary.LittleEndian.PutUint16(end[10:], uint16(len(entries)))
	binary.LittleEndian.PutUint32(end[12:], uint32(size))
	binary.LittleEndian.PutUint32(end[16:], uint32(start))
	output.Write(end)
	if err := os.WriteFile(path, output.Bytes(), 0o644); err != nil {
		rec.Fail("%v", err)
	}
}

// dosDate is 1980-01-01 in MS-DOS date form; its time of day is 0.
const dosDate = 0x21

func writeLocalHeader(output *bytes.Buffer, entry zipEntry) {
	header := make([]byte, 30)
	binary.LittleEndian.PutUint32(header, 0x04034b50)
	binary.LittleEndian.PutUint16(header[4:], 20)
	binary.LittleEndian.PutUint16(header[8:], 8)
	binary.LittleEndian.PutUint16(header[12:], dosDate)
	binary.LittleEndian.PutUint32(header[14:], entry.crc)
	binary.LittleEndian.PutUint32(header[18:], uint32(entry.packed))
	binary.LittleEndian.PutUint32(header[22:], uint32(entry.size))
	binary.LittleEndian.PutUint16(header[26:], uint16(len(entry.name)))
	binary.LittleEndian.PutUint16(header[28:], uint16(len(entry.extraUT)))
	output.Write(header)
	output.WriteString(entry.name)
	output.Write(entry.extraUT)
}

func writeCentralHeader(output *bytes.Buffer, entry zipEntry) {
	header := make([]byte, 46)
	binary.LittleEndian.PutUint32(header, 0x02014b50)
	binary.LittleEndian.PutUint16(header[4:], 3<<8|52)
	binary.LittleEndian.PutUint16(header[6:], 20)
	binary.LittleEndian.PutUint16(header[10:], 8)
	binary.LittleEndian.PutUint16(header[14:], dosDate)
	binary.LittleEndian.PutUint32(header[16:], entry.crc)
	binary.LittleEndian.PutUint32(header[20:], uint32(entry.packed))
	binary.LittleEndian.PutUint32(header[24:], uint32(entry.size))
	binary.LittleEndian.PutUint16(header[28:], uint16(len(entry.name)))
	binary.LittleEndian.PutUint16(header[30:], uint16(len(entry.extraUT)))
	binary.LittleEndian.PutUint16(header[36:], 1)
	binary.LittleEndian.PutUint32(header[38:], 0o100644<<16)
	binary.LittleEndian.PutUint32(header[42:], uint32(entry.offset))
	output.Write(header)
	output.WriteString(entry.name)
	output.Write(entry.extraUT)
}
