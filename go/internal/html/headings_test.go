package html

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/htmlkit"
	"github.com/sarielhp/relatex/internal/rec"
)

// headingPaper is a document with a title, a section holding a subsection, and a subsection
// directly under the document (a level the source skips).
func headingPaper() *Paper {
	text := func(value string) []any { return []any{rec.Map{"type": "text", "text": value}} }
	nodes := rec.Map{
		"root":  rec.Map{"id": "root", "type": "document", "children": []any{"t", "s1", "s2"}},
		"t":     rec.Map{"id": "t", "type": "paragraph", "content": text("A  Title∗")},
		"s1":    rec.Map{"id": "s1", "type": "section", "children": []any{"h1", "s1a"}},
		"h1":    rec.Map{"id": "h1", "type": "heading", "content": text("1 Intro"), "semantics": rec.Map{"level": int64(1)}},
		"s1a":   rec.Map{"id": "s1a", "type": "section", "children": []any{"h1a"}},
		"h1a":   rec.Map{"id": "h1a", "type": "heading", "content": text("1.1 More"), "semantics": rec.Map{"level": int64(2)}},
		"s2":    rec.Map{"id": "s2", "type": "section", "children": []any{"h2"}},
		"h2":    rec.Map{"id": "h2", "type": "heading", "content": text("Orphan"), "semantics": rec.Map{"level": int64(2)}},
		"after": rec.Map{"id": "after", "type": "paragraph", "content": text("A Title")},
	}
	document := rec.Map{"root": "root", "nodes": nodes, "frontmatter": rec.Map{"title": "A Title"},
		"main_block_order": []any{"t", "h1", "h1a", "h2", "after"}}
	return &Paper{document: document, nodes: nodes}
}

// Heading levels follow the sections' nesting from h2, so they never skip; the title is the h1.
func TestHeadingLevelsFollowTheNesting(t *testing.T) {
	paper := headingPaper()
	for id, want := range map[string]string{"h1": "h2", "h1a": "h3", "h2": "h2"} {
		if got := paper.headingTag(id); got != want {
			t.Errorf("%s: %s, want %s", id, got, want)
		}
	}
	page := paper.RenderNode("root")
	if !strings.Contains(page, `<h1 id="t" data-block="paragraph" class="paragraph title">`) ||
		!strings.HasPrefix(paper.RenderNode("after"), `<p id="after" data-block="paragraph" class="paragraph">`) {
		t.Errorf("title block: %s", page)
	}
	if !strings.Contains(page, `<h3 id="h1a" data-block="heading" class="heading">`) {
		t.Errorf("subsection: %s", page)
	}
}

func TestHeadingDepthIsCappedAtSix(t *testing.T) {
	nodes := rec.Map{"root": rec.Map{"id": "root", "type": "document", "children": []any{"s0"}}}
	parent := "s0"
	for depth := range 7 {
		child := "s" + string(rune('1'+depth))
		nodes[parent] = rec.Map{"id": parent, "type": "section", "children": []any{child}}
		parent = child
	}
	nodes[parent] = rec.Map{"id": parent, "type": "heading"}
	paper := &Paper{document: rec.Map{"root": "root"}, nodes: nodes}
	if got := paper.headingTag(parent); got != "h6" {
		t.Errorf("deep heading %s", got)
	}
}

func TestNoTitleWithoutAMatchingParagraph(t *testing.T) {
	paper := headingPaper()
	rec.AsMap(paper.document["frontmatter"])["title"] = "Another"
	if paper.titleBlock() != "" {
		t.Errorf("title block %q", paper.titleBlock())
	}
	if TitleText(" A  Title*† ") != "A Title" {
		t.Errorf("title text %q", TitleText(" A  Title*† "))
	}
}

// The skip link is the body's first element and leads to main.
func TestSkipLinkLeadsToMain(t *testing.T) {
	if !strings.HasPrefix(SkipLink, `<a class="skip-link" href="#main">`) || htmlkit.MainID != "main" {
		t.Fatalf("skip link %s", SkipLink)
	}
	if !strings.Contains(pageStyle, ".skip-link:focus { top: .5rem; }") {
		t.Fatal("the skip link shows when focused")
	}
}
