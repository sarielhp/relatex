package html

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/contrast"
	"github.com/sarielhp/relatex/internal/rec"
)

func headingNode(text, counter string) rec.Map {
	return rec.Map{"type": "heading", "content": []any{rec.Map{"type": "text", "text": text}}, "semantics": rec.Map{"counter": counter}}
}

// The table of contents nests subsections under their sections, never skipping a depth, and
// the page links to it from the top.
func TestContentsNestsHeadingsByTheirCounter(t *testing.T) {
	nodes := rec.Map{"h1": headingNode("1 Intro", "section"), "h2": headingNode("1.1 Setup", "subsection"),
		"h3": headingNode("1.1.1 Deep", "subsubsection"), "h4": headingNode("2 Next", "section"), "p": rec.Map{"type": "paragraph"}}
	paper := &Paper{document: rec.Map{"main_block_order": []any{"h1", "p", "h2", "h3", "h4"}}, nodes: nodes}
	want := `<nav id="contents" aria-label="Contents"><h2>Contents</h2><ol><li><a href="#h1">1 Intro</a><ol><li><a href="#h2">1.1 Setup</a>` +
		`<ol><li><a href="#h3">1.1.1 Deep</a></li></ol></li></ol></li><li><a href="#h4">2 Next</a></li></ol></nav>`
	if got := paper.contents(); got != want {
		t.Errorf("contents\n got %s\nwant %s", got, want)
	}
	if link := paper.contentsLink(); !strings.Contains(link, `href="#contents"`) {
		t.Errorf("contents link %s", link)
	}
	// A subsection before any section opens at the first depth.
	paper = &Paper{document: rec.Map{"main_block_order": []any{"h2"}}, nodes: nodes}
	if got := paper.contents(); !strings.Contains(got, `<ol><li><a href="#h2">1.1 Setup</a></li></ol>`) {
		t.Errorf("orphan subsection %s", got)
	}
	paper = &Paper{document: rec.Map{"main_block_order": []any{"p"}}, nodes: nodes}
	if paper.contents() != "" || paper.contentsLink() != "" {
		t.Error("contents without headings")
	}
}

// A document that keeps page decorations (decision 016, amendment 2026-11-17) still shows no
// source page notices after its text.
func TestAfterMainContentShowsNoPageNotices(t *testing.T) {
	row := rec.Map{"id": "n-1", "page": int64(1), "source": "latex-outputpage", "candidate": int64(1), "glyphs": int64(1), "text": "1"}
	paper := &Paper{document: rec.Map{"note_order": []any{}, "page_decorations": []any{row}}}
	if got := paper.afterMainContent(); got != "" {
		t.Errorf("after main: %s", got)
	}
}

func paragraphNode(text string) rec.Map {
	return rec.Map{"type": "paragraph", "content": []any{rec.Map{"type": "text", "text": text}}}
}

// A paper with its own printed table of contents gets the generated contents at the top of the
// page, without the skip link; any other paper keeps it after the text, with the link (the
// author, 2026-11-17, decision 095 amendment 3).
func TestContentsOpensThePageOfAPaperWithPrintedContents(t *testing.T) {
	nodes := rec.Map{"h1": headingNode("1 Intro", "section"), "t": paragraphNode("Contents"),
		"e1": paragraphNode("1 Intro 1"), "e2": paragraphNode("2 Next 4"), "p": paragraphNode("text")}
	printed := &Paper{document: rec.Map{"main_block_order": []any{"t", "e1", "e2", "h1"}}, nodes: nodes}
	if printed.contentsAtTop() == "" || printed.contentsAtEnd() != "" || printed.contentsLink() != "" {
		t.Errorf("printed contents: top %q end %q link %q", printed.contentsAtTop(), printed.contentsAtEnd(), printed.contentsLink())
	}
	plain := &Paper{document: rec.Map{"main_block_order": []any{"h1", "p"}}, nodes: nodes}
	if plain.contentsAtTop() != "" || plain.contentsAtEnd() == "" || plain.contentsLink() == "" {
		t.Errorf("plain: top %q end %q link %q", plain.contentsAtTop(), plain.contentsAtEnd(), plain.contentsLink())
	}
}

// The text paint maps to the theme's ink, a declared ink to its custom property, and any other
// color stays itself; the dark theme raises a faint ink to 4.5:1 and leaves readable ones alone.
func TestInkColorsAreThemedAndDarkInksLightened(t *testing.T) {
	paper := &Paper{document: rec.Map{"text_paint": rec.Map{"inks": []any{"#000080", "#ff0000", "#ffffaa"}}}}
	for color, want := range map[string]string{textPaintColor: "var(--ink)", "#000080": "var(--ink-000080)", "rgb(200, 0, 0)": "rgb(200, 0, 0)", "#123456": "#123456"} {
		if got := paper.inkColor(color); got != want {
			t.Errorf("%s: %s, want %s", color, got, want)
		}
	}
	css := inksCSS(paper.document)
	if !strings.Contains(css, "--ink-000080: #4a7ef6;") || !strings.Contains(css, `[data-theme="light"] { --ink-000080: #000080;`) {
		t.Errorf("inks css %s", css)
	}
	if !strings.Contains(css, "--ink-ffffaa: #ffffaa;") {
		t.Errorf("a readable ink changed in the dark theme: %s", css)
	}
}

// A text paint without colored text declares no inks: the page gets no ink properties.
func TestInksCSSIsEmptyWithoutColoredText(t *testing.T) {
	if css := inksCSS(rec.Map{"text_paint": rec.Map{"color": "rgb(0, 0, 0)"}}); css != "" {
		t.Errorf("css %q", css)
	}
	if (&Paper{document: rec.Map{"text_paint": rec.Map{}}}).inkColor("#000080") != "#000080" {
		t.Error("undeclared ink changed")
	}
}

// The browser checks force the light theme and read its ink as the default text color
// (browser.defaultTextColor, rgb(0, 0, 0)): the theme must keep that ink.
func TestLightThemeInkIsTheCheckedDefault(t *testing.T) {
	if !strings.Contains(lightTheme, "--ink: rgb(0, 0, 0);") {
		t.Fatalf("light theme ink changed: %s", lightTheme)
	}
}

// The contrast palettes are the stylesheet's backgrounds behind text, in both themes.
func TestContrastPalettesAreTheStylesheets(t *testing.T) {
	dark := pageStyle[:strings.Index(pageStyle, "\n")]
	for _, row := range []struct {
		css   string
		theme contrast.Theme
	}{{lightTheme, contrast.Light}, {dark, contrast.Dark}} {
		for kind, property := range map[string]string{"page": "--bg", "theorem": "--theorem-bg", "proof": "--proof-bg"} {
			if declaration := property + ": " + row.theme.Backgrounds[kind] + ";"; !strings.Contains(row.css, declaration) {
				t.Errorf("%s theme lacks %s", row.theme.Name, declaration)
			}
		}
	}
}

// A footnote call mark is a raised superscript, not a baseline digit (voronoi_prophet's "prophet1").
func TestNoteReferenceIsRaisedAboveTheBaseline(t *testing.T) {
	if !strings.Contains(pageStyle, `a[data-kind="note_reference"] { vertical-align: .4rem; line-height: 0; }`) {
		t.Errorf("the page style does not raise the footnote call mark")
	}
}
