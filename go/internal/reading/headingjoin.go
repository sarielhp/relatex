package reading

import "github.com/sarielhp/relatex/internal/rec"

// A heading's title box may set its title in more than one paragraph: a beamer frame title broken
// with \\ under a theme that sets it line by line, the frame's subtitle under it, or IEEEtran's
// class mark "Appendix A" above an appendix title. Decision 126: the title box is one heading in
// every class, a heading of several lines. Each paragraph would open a block of its own, two
// headings of one entity; the paragraph continues the heading's block instead, after a line break:
// a span of one space marked line_break, which the HTML writes as the space and a <br>, and every
// text reader (accessible name, outline, contents, PDF tags, Markdown, Typst, chapter names)
// reads as the space between the lines.

// headingContinues: the owner is a paragraph of the heading entity whose block was the last one read
// in this partition.
func headingContinues(last, owner rec.Map) bool {
	return last != nil && rec.Fetch(last, "type") == "section" && rec.FetchString(owner, "role") == "section" &&
		rec.Equal(rec.Fetch(last, "entity"), rec.Fetch(owner, "entity")) && len(rec.FetchList(last, "content")) > 0
}

// lineBreak is the span between two lines of a heading; no later text merges into it.
func lineBreak() rec.Map {
	return rec.Map{"type": "text", "text": " ", "line_break": true}
}
