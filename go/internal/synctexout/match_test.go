package synctexout

import (
	"bytes"
	"testing"
)

const trailer = "<< /Type /XRef /Size 9 /ID [<5dca7da23b3c0740ff6c3bf954000e5b><390066e5b8c04492992fff5ea87b4267>] >>\n" +
	"startxref\n42\n%%EOF\n"

func TestOnlyTheTrailerIDMayDiffer(t *testing.T) {
	first := []byte("%PDF-1.5\n1 0 obj (page) endobj\n" + trailer)
	other := bytes.Replace(first, []byte("5dca7da23b3c"), []byte("6806ca7baf48"), 1)
	if !Same(first, other) {
		t.Fatal("two builds differing only in /ID must be the same build")
	}
	if len(Normalized(first)) != len(first) {
		t.Fatal("normalizing must keep every offset")
	}
	changed := bytes.Replace(other, []byte("(page)"), []byte("(pagE)"), 1)
	if Same(first, changed) {
		t.Fatal("a changed page must not pass")
	}
	dated := bytes.Replace(first, []byte("(page)"), []byte("/CreationDate (D:20251031084000Z)"), 1)
	if Same(dated, bytes.Replace(dated, []byte("084000Z"), []byte("034000-05'00'"), 1)) {
		t.Fatal("dates belong to the document and must match")
	}
}

func TestADifferentIDLengthIsADifference(t *testing.T) {
	if Same([]byte("x /ID [<00><11>] y"), []byte("x /ID [<0000><11>] y")) {
		t.Fatal("an /ID of another length moves offsets and must not pass")
	}
}
