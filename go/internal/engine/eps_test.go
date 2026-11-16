package engine

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// A profile of 176 bytes with one text tag (offset 156, 16 bytes: "ab", its terminator and
// leftover memory) and one curve tag whose zero-free bytes stay.
func TestClearProfilePadding(t *testing.T) {
	header := make([]byte, 132)
	binary.BigEndian.PutUint32(header, 176)
	copy(header[36:], "acsp")
	binary.BigEndian.PutUint32(header[128:], 2)
	tags := []byte{}
	for _, entry := range []struct {
		signature      string
		offset, length uint32
	}{{"cprt", 156, 16}, {"rTRC", 172, 4}} {
		tags = append(tags, entry.signature...)
		tags = binary.BigEndian.AppendUint32(tags, entry.offset)
		tags = binary.BigEndian.AppendUint32(tags, entry.length)
	}
	prefix := append([]byte("1 0 obj\nstream\n"), append(header, tags...)...)
	written := append(bytes.Clone(prefix), "text\x00\x00\x00\x00ab\x00XYZW\x00XYZW\nendstream\n"...)
	path := filepath.Join(t.TempDir(), "expanded.pdf")
	if err := os.WriteFile(path, written, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := clearProfilePadding(path); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := append(bytes.Clone(prefix), "text\x00\x00\x00\x00ab\x00\x00\x00\x00\x00\x00XYZW\nendstream\n"...)
	if !bytes.Equal(got, want) {
		t.Errorf("clearProfilePadding:\n got %q\nwant %q", got, want)
	}
}
