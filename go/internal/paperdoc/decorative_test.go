package paperdoc

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func image(id, asset, source string, width, height int64) rec.Map {
	return rec.Map{"type": "image", "id": id, "asset": asset, "source": source, "width_sp": width, "height_sp": height}
}

// Decision 111: an invisible image, a tiny drawn mark repeated three times and a drawn rule are
// decorative (inferred); a mark seen twice, an included small file, a larger drawing, a short or
// thick line and a described image are not.
func TestInferDecorative(t *testing.T) {
	point := int64(65_536)
	pt := func(value float64) int64 { return int64(value * 65_536) }
	bullet := func(id string) rec.Map { return image(id, "b", "rendered/b.pdf", 4*point, 4*point) }
	described := image("d", "e", "", point, point)
	described["alt"], described["alt_provenance"] = "A dot", "source"
	describedRule := image("r6", "r6", "rendered/r6.pdf", pt(220.8), pt(1))
	describedRule["alt"], describedRule["alt_provenance"] = "A rule", "source"
	images := []rec.Map{bullet("b1"), bullet("b2"), bullet("b3"),
		image("i", "i", "rendered/i.pdf", point, 2*point),
		image("t1", "t", "rendered/t.pdf", 4*point, 4*point), image("t2", "t", "rendered/t.pdf", 4*point, 4*point),
		image("f1", "f", "icon.png", 4*point, 4*point), image("f2", "f", "icon.png", 4*point, 4*point), image("f3", "f", "icon.png", 4*point, 4*point),
		image("l1", "l", "rendered/l.pdf", 20*point, 4*point), image("l2", "l", "rendered/l.pdf", 20*point, 4*point),
		image("l3", "l", "rendered/l.pdf", 20*point, 4*point), described,
		image("r1", "r", "rendered/r.pdf", pt(220.8), pt(1)),
		image("r2", "r2", "rendered/r2.pdf", pt(220.8), pt(3)),
		image("r3", "r3", "rendered/r3.pdf", pt(15), pt(1)),
		image("r4", "r4", "rendered/r4.pdf", pt(1), pt(300)),
		image("r5", "r5", "icon.png", pt(220), pt(1)), describedRule}
	content := []any{}
	for _, span := range images {
		content = append(content, span)
	}
	nodes := rec.Map{"p": rec.Map{"content": content}}
	if marked := InferDecorative(nodes); marked != 6 {
		t.Fatalf("marked %d, want 6", marked)
	}
	for _, span := range images {
		want := map[string]bool{"b1": true, "b2": true, "b3": true, "i": true, "r1": true, "r4": true}[span["id"].(string)]
		if got := span["decorative"] == true && span["alt_provenance"] == Inferred; got != want {
			t.Errorf("%s: decorative %v", span["id"], got)
		}
	}
	for _, span := range []rec.Map{described, describedRule} {
		if span["alt_provenance"] != "source" {
			t.Errorf("%s: a described image keeps its description", span["id"])
		}
	}
}

// A row of tiny drawn marks of one size is decorative even where one mark is drawn once (a theme's
// navigation dots, the current frame's dot unique); a pair, or a row broken by words, is not.
func TestInferDecorativeRowOfMarks(t *testing.T) {
	pt := func(value float64) int64 { return int64(value * 65_536) }
	dot := func(id string) rec.Map { return image(id, id, "rendered/"+id+".pdf", pt(4), pt(4.3)) }
	space := rec.Map{"type": "text", "text": " "}
	word := rec.Map{"type": "text", "text": "and"}
	row := rec.Map{"content": []any{dot("a"), space, dot("b"), dot("c")}}
	pair := rec.Map{"content": []any{dot("d"), dot("e")}}
	broken := rec.Map{"content": []any{dot("f"), word, dot("g"), dot("h")}}
	mixed := rec.Map{"content": []any{dot("i"), dot("j"), image("k", "k", "rendered/k.pdf", pt(5.9), pt(4.3))}}
	nodes := rec.Map{"row": row, "pair": pair, "broken": broken, "mixed": mixed}
	if marked := InferDecorative(nodes); marked != 3 {
		t.Fatalf("marked %d, want 3 (the row a, b, c)", marked)
	}
	for _, node := range nodes {
		for _, item := range rec.AsMap(node)["content"].([]any) {
			span := item.(rec.Map)
			if span["type"] != "image" {
				continue
			}
			want := map[string]bool{"a": true, "b": true, "c": true}[span["id"].(string)]
			if got := span["decorative"] == true; got != want {
				t.Errorf("%s: decorative %v", span["id"], got)
			}
		}
	}
}

// A navigation row whose current dot is a point narrower (4x4, 3x4, 4x4) is one row (amendment 4).
func TestInferDecorativeRowWithNarrowDot(t *testing.T) {
	pt := func(value float64) int64 { return int64(value * 65_536) }
	dot := func(id string, width float64) rec.Map { return image(id, id, "rendered/"+id+".pdf", pt(width), pt(4)) }
	spans := []any{dot("a", 4), dot("b", 3), dot("c", 4)}
	if marked := InferDecorative(rec.Map{"p": rec.Map{"content": spans}}); marked != 3 {
		t.Fatalf("marked %d, want 3", marked)
	}
}

// Decision 111 amendment 4: a drawn bar of at most 4 bp and at least 100 pt that recurs at one size (a theme's
// frame rule or progress bar) is decorative; a lone one, a thicker one, a short one and an included file are
// not, and a 2 pt rule boxed at 2 bp is a rule.
func TestInferDecorativeBars(t *testing.T) {
	pt := func(value float64) int64 { return int64(value * 65_536) }
	bar := func(id string, height float64) rec.Map {
		return image(id, id, "rendered/"+id+".pdf", pt(364.4), pt(height))
	}
	spans := []rec.Map{bar("b1", 4.015), bar("b2", 4.015), bar("b3", 4.015),
		bar("t1", 5.02), bar("t2", 5.02), bar("t3", 5.02), bar("lone", 3.01),
		image("s1", "s1", "rendered/s1.pdf", pt(50), pt(3)), image("s2", "s2", "rendered/s2.pdf", pt(50), pt(3)),
		image("s3", "s3", "rendered/s3.pdf", pt(50), pt(3)),
		image("f1", "f1", "bar.png", pt(364.4), pt(4)), image("f2", "f2", "bar.png", pt(364.4), pt(4)),
		image("f3", "f3", "bar.png", pt(364.4), pt(4)), bar("two", 2.007)}
	content := []any{}
	for _, span := range spans {
		content = append(content, span)
	}
	if marked := InferDecorative(rec.Map{"p": rec.Map{"content": content}}); marked != 4 {
		t.Fatalf("marked %d, want 4", marked)
	}
	for _, span := range spans {
		want := map[string]bool{"b1": true, "b2": true, "b3": true, "two": true}[span["id"].(string)]
		if got := span["decorative"] == true; got != want {
			t.Errorf("%s: decorative %v", span["id"], got)
		}
	}
}

// A small included ORCID or cas-template icon is named by its file name; a drawn mark, a large logo, an
// unknown file and a described icon are not.
func TestInferIconAlt(t *testing.T) {
	pt := func(value float64) int64 { return int64(value * 65_536) }
	described := image("described", "x", "orcid.pdf", pt(9), pt(9))
	described["alt"], described["alt_provenance"] = "ORCID profile", "source"
	spans := []rec.Map{
		image("aas", "a", "Orcid-ID.png", pt(9), pt(9)), image("lipics", "b", "orcid", pt(9), pt(9)),
		image("beamer", "c", "images/orcid.pdf", pt(10), pt(10)),
		image("mail", "d", "relatex-texmf/tex/latex/els-cas-templates/thumbnails/cas-email.jpeg", pt(8), pt(8)),
		image("url", "e", "relatex-texmf/tex/latex/els-cas-templates/thumbnails/cas-url.jpeg", pt(8), pt(8)),
		image("drawn", "f", "rendered/orcid.pdf", pt(9), pt(9)), image("logo", "g", "orcid.png", pt(80), pt(80)),
		image("other", "h", "icon.png", pt(9), pt(9)), described}
	content := []any{}
	for _, span := range spans {
		content = append(content, span)
	}
	if named := InferIconAlt(rec.Map{"p": rec.Map{"content": content}}); named != 5 {
		t.Fatalf("named %d, want 5", named)
	}
	want := map[string]string{"aas": "ORCID iD", "lipics": "ORCID iD", "beamer": "ORCID iD", "mail": "Email", "url": "URL"}
	for _, span := range spans {
		id := span["id"].(string)
		if id == "described" {
			if span["alt"] != "ORCID profile" || span["alt_provenance"] != "source" {
				t.Errorf("described: keeps its description, got %v", span["alt"])
			}
			continue
		}
		got, _ := span["alt"].(string)
		if got != want[id] || (got != "" && span["alt_provenance"] != Inferred) {
			t.Errorf("%s: alt %q (%v), want %q", id, got, span["alt_provenance"], want[id])
		}
	}
}
