package paperdoc

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/sarielhp/relatex/internal/rec"
)

// Hand-written proofs (decision 177, plans/proof-marks.md phase 5). Some authors write a proof without an
// environment: a paragraph opened by a styled "Proof." heading, ended some paragraphs later by a QED-like mark
// typed at its end. Such a run of paragraphs becomes a proof when both ends are found among the children of
// one section (or the document), with only paragraphs, displayed formulas and lists between them. The heading must be set apart
// from the text after it (bold, italic, slanted or small capitals); the mark keeps headings such as
// "Proof of concept." out.

var (
	proofHeading = regexp.MustCompile(`(?i)\A\s*(proof|sketch of (the )?proof|proof sketch|proof idea|outline of (the )?proof|` +
		`beweis|preuve|démonstration|demonstration|dimostrazione|demostración|demostracion)` +
		`(\s+(of|of the|for|des|de la|du|von|di|del)\s+[^.:]{1,80}|\s*\([^)]{1,80}\))?\s*[.:]\s*\z`)
	qedMarks      = "∎□■◻◼▢☐"
	proofSiblings = map[string]bool{"paragraph": true, "display_math": true, "unordered_list": true, "ordered_list": true,
		"figure_float": true}
	// maxProofParagraphs bounds how far a heading looks for its end mark.
	maxProofParagraphs = 40
)

// inferProofs wraps each hand-written proof in a proof node and returns how many it found.
func (b *builder) inferProofs(root string) int {
	found := 0
	var visit func(id string)
	visit = func(id string) {
		node := rec.AsMap(b.nodes[id])
		kind := rec.FetchString(node, "type")
		if kind != "document" && kind != "section" {
			return
		}
		rebuilt, count := b.proofRuns(rec.FetchList(node, "children"), visit)
		node["children"] = rebuilt
		found += count
	}
	visit(root)
	return found
}

// proofRuns rebuilds a run of siblings with each hand-written proof wrapped, a proof nested in another (a
// claim proved inside a lemma's proof) inside its own, and returns how many it wrapped.
func (b *builder) proofRuns(children []any, visit func(string)) ([]any, int) {
	rebuilt, found := []any{}, 0
	for index := 0; index < len(children); index++ {
		child := toS(children[index])
		if end := b.proofEnd(children, index); end >= 0 {
			inner, count := b.proofRuns(children[index+1:end+1], visit)
			rebuilt = append(rebuilt, b.storeProof(append([]any{child}, inner...), child))
			found += 1 + count
			index = end
			continue
		}
		visit(child)
		rebuilt = append(rebuilt, child)
	}
	return rebuilt, found
}

// proofEnd is the index of the sibling whose end mark closes the proof a heading at start opens, or -1.
func (b *builder) proofEnd(children []any, start int) int {
	first := rec.AsMap(b.nodes[toS(children[start])])
	if rec.FetchString(first, "type") != "paragraph" || !proofOpening(rec.FetchList(first, "content")) {
		return -1
	}
	for index := start; index < len(children) && index-start < maxProofParagraphs; index++ {
		node := rec.AsMap(b.nodes[toS(children[index])])
		kind := rec.FetchString(node, "type")
		if !proofSiblings[kind] || (index > start && runInHeading(node)) {
			return -1
		}
		// A proof headed inside this one ends first; the search goes on after its mark.
		if index > start && kind == "paragraph" && proofOpening(rec.FetchList(node, "content")) {
			inner := b.proofEnd(children, index)
			if inner < 0 {
				return -1
			}
			index = inner
			continue
		}
		if kind == "paragraph" && b.endsWithMark(rec.FetchList(node, "content")) {
			return index
		}
	}
	return -1
}

// runInHeading: the paragraph opens with a sectioning heading set run-in. A heading that declares a proof
// (\paragraph{\bf Proof.}) opens one (papers4/1041; the author, 2026-11-14), but no proof runs across a later one:
// that heading belongs to its section. The heading's own paragraph carries its title's span count (withTitle); the
// paragraphs after it share its semantics without one and are ordinary (papers2/0232, whose subsections are run-in).
func runInHeading(node rec.Map) bool {
	semantics, _ := node["semantics"].(rec.Map)
	_, titled := semantics["title_spans"]
	return semantics != nil && rec.FetchString(semantics, "kind") == "heading" && titled
}

func (b *builder) storeProof(members []any, first string) string {
	id := b.uid("inferred_proof", first)
	b.store(id, "proof", append([]any{}, members...), "main/inferred_proof:"+first, nil)
	return id
}

// proofOpening: the paragraph's leading run of one style (a cross-reference inside it counts as part of it)
// reads as a proof heading, and that style is bold, italic, slanted or small capitals. The heading may be the
// whole paragraph (a display follows), and its colon may sit just after the styled run ({\em Proof of
// Lemma~\ref{x}}:).
func proofOpening(content []any) bool {
	heading, style := "", rec.Map(nil)
	for _, value := range content {
		span := rec.AsMap(value)
		kind, _ := span["type"].(string)
		if kind == "reference" && style != nil {
			heading += "X"
			continue
		}
		current, _ := span["style"].(rec.Map)
		if kind != "text" || (style != nil && !sameFace(current, style)) {
			heading += trailingStop(heading, span)
			break
		}
		style = current
		heading += rec.FetchString(span, "text")
	}
	return style != nil && emphasized(style) && proofHeading.MatchString(heading)
}

// trailingStop is the "." or ":" opening the text after a heading run that has none of its own.
func trailingStop(heading string, span rec.Map) string {
	const blank = " \u00a0"
	trimmed := strings.TrimRight(heading, blank)
	if span["type"] != "text" || strings.HasSuffix(trimmed, ".") || strings.HasSuffix(trimmed, ":") {
		return ""
	}
	text := strings.TrimLeft(rec.FetchString(span, "text"), blank)
	if strings.HasPrefix(text, ".") || strings.HasPrefix(text, ":") {
		return text[:1]
	}
	return ""
}

func sameFace(a, b rec.Map) bool {
	return a != nil && b != nil && rec.Equal(a["series"], b["series"]) && rec.Equal(a["shape"], b["shape"])
}

func emphasized(style rec.Map) bool {
	series, _ := style["series"].(string)
	shape, _ := style["shape"].(string)
	return strings.Contains(series, "b") || shape == "it" || shape == "sl" || shape == "sc"
}

// endsWithMark: the paragraph's last item is a QED-like mark: a QED symbol, a text ending in one of the
// tombstone signs, a formula that is only one of them, or a small square rule.
func (b *builder) endsWithMark(content []any) bool {
	for index := len(content) - 1; index >= 0; index-- {
		span := rec.AsMap(content[index])
		switch span["type"] {
		case "symbol":
			return true
		case "text":
			text := strings.TrimRight(rec.FetchString(span, "text"), "  \t")
			if text == "" {
				continue
			}
			last, _ := utf8.DecodeLastRuneInString(text)
			return strings.ContainsRune(qedMarks, last)
		case "math":
			return b.markFormula(span)
		case "rule":
			return squareRule(span)
		default:
			return false
		}
	}
	return false
}

var mathTokenText = regexp.MustCompile(`<(mi|mo|mtext)[^>]*>([^<]*)</(mi|mo|mtext)>`)
var mathTokens = regexp.MustCompile(`<(mi|mo|mn|mtext|ms|mspace|mglyph)\b`)

// markFormula: a formula whose only token is one tombstone sign ($\Box$, $\blacksquare$).
func (b *builder) markFormula(span rec.Map) bool {
	node, ok := b.nodes[toS(span["node"])].(rec.Map)
	if !ok {
		return false
	}
	mathml, _ := node["mathml"].(string)
	tokens := mathTokens.FindAllString(mathml, -1)
	match := mathTokenText.FindStringSubmatch(mathml)
	if len(tokens) != 1 || match == nil {
		return false
	}
	text := strings.TrimSpace(match[2])
	return utf8.RuneCountInString(text) == 1 && strings.Contains(qedMarks, text)
}

// squareRule: a painted rule about as wide as high and no larger than a capital letter (\rule{1ex}{1ex}).
func squareRule(span rec.Map) bool {
	width, _ := rec.Number(span["width_sp"])
	height, _ := rec.Number(span["height_sp"])
	depth, _ := rec.Number(span["depth_sp"])
	tall := height + depth
	return width > 0 && tall > 0 && width <= 12*65536 && tall <= 12*65536 && width <= 1.5*tall && tall <= 1.5*width
}
