package frontmatter

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func noticeRecords() []rec.Map {
	return []rec.Map{
		{"kind": "candidate", "candidate": int64(1), "text": "ab", "glyphs": int64(2)},
		{"code": "excluded_page_decoration", "char": "a", "text": "a", "candidate": int64(1), "position": int64(1),
			"page": int64(1), "source": "acmart-copyright"},
		{"code": "excluded_page_decoration", "char": "b", "text": "b", "candidate": int64(1), "position": int64(2),
			"page": int64(1), "source": "acmart-copyright"},
	}
}

func noticeDocument() rec.Map {
	return rec.Map{"identity": rec.Map{"input_sha256": "abc"}, "nodes": rec.Map{}}
}

func TestPageDecorationsStandOnTheirOwn(t *testing.T) {
	document := noticeDocument()
	rows := PageDecorations(nil, noticeRecords(), rec.FetchMap(document, "identity"), document)
	if len(rows) != 1 || rec.AsMap(rows[0])["text"] != "ab" {
		t.Fatalf("rows %v", rows)
	}
	document["page_decorations"] = rows
	Validate(document)
	if got := Decorations(document); len(got) != 1 {
		t.Errorf("Decorations without front matter: %v", got)
	}
}

func TestDecorationsPreferTheFrontMatter(t *testing.T) {
	document := rec.Map{"frontmatter": rec.Map{"decorations": []any{rec.Map{"id": "front"}}},
		"page_decorations": []any{rec.Map{"id": "own"}}}
	got := Decorations(document)
	if len(got) != 1 || rec.AsMap(got[0])["id"] != "front" {
		t.Errorf("got %v", got)
	}
	if got := Decorations(rec.Map{}); len(got) != 0 {
		t.Errorf("empty document: %v", got)
	}
}

func TestPageDecorationsFailOnAnUnboundRow(t *testing.T) {
	document := noticeDocument()
	records := noticeRecords()[:2]
	if message := failure(func() { PageDecorations(nil, records, rec.FetchMap(document, "identity"), document) }); message == "" {
		t.Error("an incomplete decoration validated")
	}
}
