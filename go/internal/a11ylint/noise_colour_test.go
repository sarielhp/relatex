package a11ylint

import (
	"slices"
	"testing"
)

// Prose colour phrases are one item per sentence (decision 167). Colours that only name things (the
// sets of a red-blue problem) in a paragraph that refers to no figure are advice; a paragraph that
// points at a figure, or a phrase that sends the reader to something drawn ("shown in red"), is an
// item.
func TestColourItemsOncePerSentenceAndNamesAreAdvice(t *testing.T) {
	source := "Let $R$ and $B$ be sets of red and blue points, so the red points and the blue points differ.\n" +
		"\n" +
		"The annulus is seen as the gray area in the figure on the right.\n" +
		"\n" +
		"The optimum is marked in red.\n"
	items := colourItems(newFile("main.tex", source), nil)
	type result struct {
		line     int
		advisory bool
		message  string
	}
	got := []result{}
	for _, item := range items {
		got = append(got, result{item.Line, item.Advisory, item.Message})
	}
	want := []result{
		{1, true, `the text points to colours ("blue points", "red points")`},
		{3, false, `the text points to a colour ("gray area")`},
		{5, false, `the text points to a colour ("marked in red")`},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("colour items %+v, want %+v", got, want)
	}
}

// An argument the command never typesets (a description kept for another build) is no prose.
func TestColourInADiscardedArgumentIsNoItem(t *testing.T) {
	source := "\\newcommand{\\fig}[4]{\\includegraphics[#1]{figures/#3}}\n" +
		"\\fig{width=3in}{width=4in}{slope}{Overlaid are three red lines.}\n" +
		"Overlaid are three red lines.\n"
	file := newFile("main.tex", source)
	items := colourItems(file, macros([]*File{file}))
	if len(items) != 1 || items[0].Line != 3 {
		t.Fatalf("colour items %+v, want only the prose on line 3", items)
	}
}
