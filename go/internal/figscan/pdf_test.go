package figscan

import (
	"strings"
	"testing"
)

// TestDamagedIpeSource scans books/0003's food.pdf (Ipe 7.0.2): its Ipe stream declares /Length
// 1243 but the file holds 342 bytes of it (truncated, and not inflatable), so the page is read
// from the trace instead and says why.
func TestDamagedIpeSource(t *testing.T) {
	figure, err := Scan(mustRead(t, "ipe-damaged-source"), 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if figure.Source != SourceTrace || figure.Total == 0 {
		t.Fatalf("source %q, %d shapes", figure.Source, figure.Total)
	}
	if !strings.Contains(figure.Detail, "embedded Ipe source unreadable") {
		t.Fatalf("detail %q", figure.Detail)
	}
}
