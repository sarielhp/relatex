package tables

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/textstyle"
)

// A cell printed in a direct TeX font (\font\dos=cmti10, paper 139) carries that font's style.
func TestGridCellTextKeepsADirectTeXFontStyle(t *testing.T) {
	records := []rec.Map{
		{"kind": "context", "entity": int64(0), "role": "paragraph"},
		{"kind": "diagnostic", "code": "unobserved_text_font", "font": int64(20), "font_name": "cmti10", "size_sp": int64(786_432),
			"first_char": int64(16), "first_text": "ı"},
		{"kind": "flow_candidate", "owner": int64(17), "entity": int64(0), "role": "paragraph",
			"items": []any{rec.Map{"type": "text", "text": "Fısica", "space": ""}}},
	}
	g := &tableGrid{styles: textstyle.Catalog{}, owners: map[string]rec.Map{key(int64(17)): records[2]}, owner: int64(17)}
	g.adoptUnobservedFonts(records)
	runs := g.flatContent()
	if len(runs) != 1 || rec.FetchString(rec.AsMap(runs[0]["style"]), "shape") != "it" {
		t.Fatalf("cell runs = %v", runs)
	}
	if _, styled := rec.AsMap(records[2]["items"].([]any)[0])["style"]; styled {
		t.Fatal("the flow record itself was changed")
	}
}
