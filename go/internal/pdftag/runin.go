package pdftag

import "strings"

// A run-in heading (\paragraph and a class's run-in levels) is set at the start of its paragraph;
// the model's paragraph names its title spans (semantics.title_spans, decision 088 amendment 1).
// The title is an H of its own before the paragraph's P, as LaTeX's tagging makes it: a P may
// hold no H.

// titleRun is a block's run-in title as its first run, and the content after it.
func titleRun(n node) ([]run, []contentItem) {
	count := n.Semantics.TitleSpans
	if !n.Semantics.Runin || n.Type != "paragraph" || count < 1 || count > len(n.Content) {
		return nil, n.Content
	}
	return []run{{items: n.Content[:count], title: true}}, n.Content[count:]
}

// titleSuffix marks the owner of a run-in title's glyph (reading.TitleSuffix): the title's
// paragraph's id followed by it.
const titleSuffix = "#title"

// titleLeaf is the leaf of an owned glyph of a paragraph with a run-in title: the title's run for a
// title glyph, the body's run for another when the body is one run (else the paragraph, cut at its
// tables, which the alignment shares out); any other owner is itself.
func (b *builder) titleLeaf(owner string) string {
	base, title := strings.CutSuffix(owner, titleSuffix)
	n, ok := b.doc.Nodes[base]
	if !ok {
		return base
	}
	runs := b.split(n)
	switch {
	case len(runs) == 0 || !runs[0].title:
		return base
	case title:
		return runID(n, len(runs), 0)
	case len(runs) == 2:
		return runID(n, len(runs), 1)
	}
	return base
}

// runRole is a run's structure role: a run-in title is a heading one level below the last heading
// built (so the levels never skip), else the block's role.
func (b *builder) runRole(role string, r run) string {
	if !r.title {
		return role
	}
	return headingRole(b.heading + 1)
}
