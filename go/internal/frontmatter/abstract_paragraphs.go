package frontmatter

// abstractParagraph is a structured abstract's \paragraph{Heading}: the heading prints before
// its prose, as the PDF sets it.
var abstractParagraph = rx(`\\paragraph\*?\s*\{((?:[^{}]|\{(?:[^{}]|\{[^{}]*\})*\})*)\}`)

// abstractParagraphs reads a structured abstract's \paragraph{Heading} as it prints: the
// heading, then ". " before the prose. Elsewhere \paragraph stays a heading command.
func abstractParagraphs(value string) string {
	return abstractParagraph.ReplaceAllStringFunc(value, func(match string) string {
		return abstractParagraph.FindStringSubmatch(match)[1] + ". "
	})
}
