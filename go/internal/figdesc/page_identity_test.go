package figdesc

import (
	"fmt"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/geometry"
	"github.com/sarielhp/relatex/internal/media"
	"github.com/sarielhp/relatex/internal/mediamodel"
	"github.com/sarielhp/relatex/internal/rec"
)

// threePagePDF is a small valid PDF whose three pages carry a filled square in a different place,
// so a render of one page differs from a render of another.
func threePagePDF(t *testing.T) []byte {
	t.Helper()
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R 5 0 R 7 0 R] /Count 3 >>",
	}
	for page := 0; page < 3; page++ {
		content := fmt.Sprintf("0 0 1 rg %d 20 40 40 re f", 10+page*50)
		pageObject := len(objects) + 1
		objects = append(objects, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents %d 0 R >>", pageObject+1),
			fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content))
	}
	var out strings.Builder
	out.WriteString("%PDF-1.7\n")
	offsets := []int{}
	for index, object := range objects {
		offsets = append(offsets, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", index+1, object)
	}
	start := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets {
		fmt.Fprintf(&out, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, start)
	return []byte(out.String())
}

// pageDocument is a document whose one asset is a three-page PDF, its three pages included in
// reading order as views of the same file (as Ipe views are).
func pageDocument(t *testing.T) rec.Map {
	t.Helper()
	pdf := threePagePDF(t)
	original := mediamodel.Encoded(pdf, "application/pdf")
	sha := original["sha256"].(string)
	asset := rec.Map{"schema_version": mediamodel.MediaVersion, "sha256": sha, "metadata": rec.Map{},
		"placements": rec.Map{}, "representations": rec.Map{"original": original}}
	nodes, order := rec.Map{}, []any{}
	for page := int64(1); page <= 3; page++ {
		size := geometry.BPToSP(200)
		placement := media.Placement(rec.Map{"page": page, "pagebox": "cropbox", "natural_width": size, "natural_height": size},
			rec.Map{"width": size, "height": size, "viewport": []any{0.0, 0.0, 200.0, 200.0}, "clip": false}, []any{})
		id := rec.FetchString(placement, "id")
		rec.FetchMap(asset, "placements")[id] = placement
		display := mediamodel.Encoded([]byte(fmt.Sprintf(
			`<svg xmlns="http://www.w3.org/2000/svg"><rect x="%d" y="0" width="10" height="10"/></svg>`, page)), "image/svg+xml")
		display["placement"] = id
		rec.FetchMap(asset, "representations")["display:"+id] = display
		block := fmt.Sprintf("p%d", page)
		nodes[block] = rec.Map{"id": block, "type": "paragraph", "content": []any{rec.Map{"type": "image",
			"id": fmt.Sprintf("i%d", page), "asset": sha, "source": "figures/views.pdf", "placement": placement}}}
		order = append(order, block)
	}
	return rec.Map{"language": rec.Map{"tag": "en"}, "assets": rec.Map{"items": rec.Map{sha: asset}},
		"nodes": nodes, "block_order": order}
}

func TestFigureIdentityIncludesThePage(t *testing.T) {
	figures := Figures(pageDocument(t))
	if len(figures) != 3 {
		t.Fatalf("figures %+v", figures)
	}
	identities := map[string]bool{}
	for index, figure := range figures {
		if figure.Page != index+1 {
			t.Errorf("%s on page %d, want %d", figure.Key, figure.Page, index+1)
		}
		identities[figure.Identity()] = true
	}
	if len(identities) != 3 {
		t.Errorf("views share an identity: %v", identities)
	}
	if figures[0].Identity() != figures[0].SHA256 {
		t.Errorf("a whole first page is not the file's hash: %q vs %q", figures[0].Identity(), figures[0].SHA256)
	}
	if figures[1].Identity() != figures[1].SHA256+"#p2" {
		t.Errorf("second page identity %q", figures[1].Identity())
	}
}

func TestBySHAKeepsTheDraftOfAnotherPageOut(t *testing.T) {
	figures := Figures(pageDocument(t))
	first := &Entry{Key: figures[0].Key, Alt: "page one", Provenance: Model}
	first.SHA256 = figures[0].Identity()
	sidecar := &Sidecar{Entries: map[string]*Entry{first.Key: first}}
	if other := sidecar.BySHA(figures[0].Identity(), figures[1].Key); other != first {
		t.Errorf("the same page's draft was not reused: %+v", other)
	}
	if other := sidecar.BySHA(figures[1].Identity(), figures[1].Key); other != nil {
		t.Errorf("page 1's draft was reused for page 2: %+v", other)
	}
}

func TestFigureIdentityIncludesTheCropBox(t *testing.T) {
	size := geometry.BPToSP(200)
	placement := media.Placement(rec.Map{"page": int64(1), "pagebox": "cropbox", "natural_width": size, "natural_height": size},
		rec.Map{"width": size, "height": size, "viewport": []any{0.0, 0.0, 100.0, 200.0}, "clip": false}, []any{})
	page, box, pageBox := figurePage(rec.Map{"placement": placement})
	if page != 1 || len(box) != 4 || pageBox != "cropbox" {
		t.Fatalf("page %d box %v in %q", page, box, pageBox)
	}
	figure := &Figure{SHA256: "abc", Page: page, Box: box, PageBox: pageBox}
	if figure.Identity() != "abc" {
		t.Errorf("a page-one view changed its identity: %q", figure.Identity())
	}
	figure.Page = 3
	if figure.Identity() != "abc#p3#bcropbox:0:0:100:200" {
		t.Errorf("identity %q", figure.Identity())
	}
}
