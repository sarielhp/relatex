package assets

import (
	"bytes"
	"testing"

	"github.com/sarielhp/relatex/internal/links"
	"github.com/sarielhp/relatex/internal/rec"
)

// An ORCID logo span becomes the shared logo, linked and square at the picture's size; any
// other rendered picture stays to be cropped.
func TestApplyOrcid(t *testing.T) {
	logo := rec.Map{"type": "rendered_graphic", "id": "r1", "graphic": int64(3), "page": int64(1),
		"region_sp": []any{int64(0), int64(0), int64(400_000), int64(500_000)}, "depth_sp": int64(50_000),
		"external_link": int64(7)}
	other := rec.Map{"type": "rendered_graphic", "id": "r2", "graphic": int64(4), "page": int64(1),
		"region_sp": []any{int64(0), int64(0), int64(4_000_000), int64(4_000_000)}, "depth_sp": int64(0)}
	files := map[string][]byte{}
	crops := applyOrcid([]rec.Map{logo, other}, files)
	if len(crops) != 1 || crops[0]["id"] != "r2" {
		t.Fatalf("crops = %v, want only r2", crops)
	}
	if !bytes.Equal(files[links.OrcidSource], orcidLogo) || !bytes.HasPrefix(orcidLogo, []byte("%PDF-")) {
		t.Fatal("the shared logo PDF is the file of the logo span")
	}
	if logo["type"] != "image" || logo["filename"] != links.OrcidSource || logo["external_link"] != int64(7) || logo["id"] != "r1" {
		t.Fatalf("logo span = %v", logo)
	}
	if logo["width_sp"] != int64(500_000) || logo["height_sp"] != int64(500_000) || logo["depth_sp"] != int64(50_000) {
		t.Fatalf("logo size = %v x %v, depth %v", logo["width_sp"], logo["height_sp"], logo["depth_sp"])
	}
	if _, rendered := logo["rendered_from"]; rendered {
		t.Fatal("the logo is not rendered from the PDF")
	}
}
