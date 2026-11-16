// Package legacyinput reads the 8-bit input encodings inputenc declares (decision 143): the engine
// re-encodes a build's working copy with it, and the reconcile layer applies the probe's recorded
// re-encoding to the archive's bytes to know the input the build read.
package legacyinput

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode/utf8"

	"github.com/sarielhp/relatex/internal/rec"
)

// tables are inputenc's option names with the table of the bytes 0x80-0xFF they read; a zero rune is
// a byte the encoding leaves undefined.
var tables = map[string]*[128]rune{
	"latin1":  latin1Table(),
	"latin9":  latin9Table(),
	"cp1252":  cp1252Table(),
	"ansinew": cp1252Table(),
}

// Known reports whether encoding is an inputenc option Decode reads.
func Known(encoding string) bool {
	return tables[encoding] != nil
}

// Decode is data read in encoding as UTF-8; false when the encoding is not one Known reads, data is
// already valid UTF-8 (nothing to re-encode), or a byte is one the encoding leaves undefined.
func Decode(data []byte, encoding string) ([]byte, bool) {
	table := tables[encoding]
	if table == nil || utf8.Valid(data) {
		return nil, false
	}
	var out strings.Builder
	for _, value := range data {
		if value < 0x80 {
			out.WriteByte(value)
			continue
		}
		decoded := table[value-0x80]
		if decoded == 0 {
			return nil, false
		}
		out.WriteRune(decoded)
	}
	return []byte(out.String()), true
}

// Records are a probe report's reencoded_inputs (path, encoding, sha256, reencoded_sha256) by path.
func Records(report rec.Map) map[string]rec.Map {
	records := map[string]rec.Map{}
	reencoded, _ := report["reencoded_inputs"].([]any)
	for _, value := range reencoded {
		entry := rec.AsMap(value)
		records[rec.ToS(entry["path"])] = entry
	}
	return records
}

// Apply is original, a file's bytes in the archive, with the probe's recorded re-encoding applied: the
// file the builds read. The record is believed only when the report names the inputenc shim, the record
// names original's digest and the re-encoding here gives the recorded result.
func Apply(original []byte, entry, report rec.Map) []byte {
	text, ok := Decode(original, rec.ToS(entry["encoding"]))
	if !ok || !inputencShim(report) || rec.ToS(entry["sha256"]) != Digest(original) ||
		rec.ToS(entry["reencoded_sha256"]) != Digest(text) {
		rec.Fail("Re-encoded input record mismatch")
	}
	return text
}

// AsBuilt is a source file as the builds read it: Apply when the report records its re-encoding,
// else original itself.
func AsBuilt(original []byte, path string, report rec.Map) []byte {
	if entry, ok := Records(report)[path]; ok {
		return Apply(original, entry, report)
	}
	return original
}

// inputencShim reports whether the probe built with the inputenc compatibility shim.
func inputencShim(report rec.Map) bool {
	compatibility, _ := report["compatibility"].([]any)
	for _, value := range compatibility {
		if rec.AsMap(value)["shim"] == "inputenc" {
			return true
		}
	}
	return false
}

// Digest is the SHA-256 of data in hex.
func Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// latin1Table is ISO 8859-1 as inputenc's latin1.def reads it: 0x80-0x9F undefined.
func latin1Table() *[128]rune {
	table := [128]rune{}
	for index := 0x20; index < 0x80; index++ {
		table[index] = rune(0x80 + index)
	}
	return &table
}

// latin9Table is ISO 8859-15: latin1 with eight letters and the euro sign changed.
func latin9Table() *[128]rune {
	table := latin1Table()
	changes := map[int]rune{0xA4: 0x20AC, 0xA6: 0x160, 0xA8: 0x161, 0xB4: 0x17D, 0xB8: 0x17E, 0xBC: 0x152, 0xBD: 0x153, 0xBE: 0x178}
	for code, value := range changes {
		table[code-0x80] = value
	}
	return table
}

// cp1252Table is Windows-1252 (inputenc's cp1252 and ansinew): latin1 with typographic characters in
// 0x80-0x9F, five of which stay undefined.
func cp1252Table() *[128]rune {
	table := latin1Table()
	upper := []rune{0x20AC, 0, 0x201A, 0x192, 0x201E, 0x2026, 0x2020, 0x2021, 0x2C6, 0x2030, 0x160, 0x2039, 0x152, 0, 0x17D, 0,
		0, 0x2018, 0x2019, 0x201C, 0x201D, 0x2022, 0x2013, 0x2014, 0x2DC, 0x2122, 0x161, 0x203A, 0x153, 0, 0x17E, 0x178}
	copy(table[:], upper)
	return table
}
