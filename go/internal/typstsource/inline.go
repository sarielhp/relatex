package typstsource

import (
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/figures"
	"github.com/sarielhp/relatex/internal/geometry"
	"github.com/sarielhp/relatex/internal/inline"
	"github.com/sarielhp/relatex/internal/mediamodel"
	"github.com/sarielhp/relatex/internal/paperdoc"
	"github.com/sarielhp/relatex/internal/rec"
)

// Inline content of the source-faithful writer (TypstSourceInline): styled text,
// references, citations, footnotes, links, inline math and images.

const (
	refStart = "\x01"
	refEnd   = "\x02"
	marker   = "\x00"
)

type textStyle struct {
	bold, italic, mono, caps, underline, strike bool
	lang                                        string // a BCP 47 tag other than the document's (decision 080)
}

// run is one inline run: text with its style and link, or raw markup.
type run struct {
	raw   bool
	text  string
	style textStyle
	link  any
}

var (
	runSpaces  = regexp.MustCompile(`[ \t\n]+`)
	callPiece  = regexp.MustCompile(`\A[ \t\n\v\f\r]*#`)
	callEnd    = regexp.MustCompile(`[)\]][ \t\n\v\f\r]*\z`)
	callFollow = regexp.MustCompile(`\A[(\[.;]`)
	urlText    = regexp.MustCompile(`\A[a-z]+://[^ \t\n\v\f\r]+\z`)
	dashes     = regexp.MustCompile(`[–-]`)
)

// inline is TypstSourceInline#inline: spans as markup. skip drops the first characters (see
// marked); upright drops the class's italics (a theorem body the helper sets in italics
// itself).
func (e *exporter) inline(spans []rec.Map, owner rec.Map, skip int, upright bool) string {
	spans = dropText(spans, skip)
	runs := []run{}
	for index := 0; index < len(spans); {
		span := spans[index]
		if truthy(span["citation"]) {
			end := index
			for end < len(spans) && rec.Equal(spans[end]["citation"], span["citation"]) {
				end++
			}
			runs = append(runs, run{raw: true, text: e.citation(spans[index:end])})
			index = end
			continue
		}
		runs = append(runs, e.inlineRun(span, owner, upright))
		index++
	}
	return terminateReferences(e.joinRuns(runs))
}

// marked: text with each non-text span as one marker character: offsets into it address spans.
func marked(node rec.Map) string {
	var builder strings.Builder
	for _, span := range content(node) {
		if span["type"] == "text" {
			builder.WriteString(rec.FetchString(span, "text"))
		} else {
			builder.WriteString(marker)
		}
	}
	return builder.String()
}

func plain(node rec.Map) string {
	var builder strings.Builder
	for _, span := range content(node) {
		if span["type"] == "text" {
			builder.WriteString(rec.FetchString(span, "text"))
		}
	}
	return builder.String()
}

func withText(span rec.Map, text string) rec.Map {
	copied := rec.Map{}
	for key, value := range span {
		copied[key] = value
	}
	copied["text"] = text
	return copied
}

// dropText: the spans after the first count characters of marked.
func dropText(spans []rec.Map, count int) []rec.Map {
	if count <= 0 {
		return spans
	}
	result := []rec.Map{}
	for _, span := range spans {
		if count > 0 {
			length := 1
			if span["type"] == "text" {
				length = size(rec.FetchString(span, "text"))
			}
			if length <= count {
				count -= length
				continue
			}
			span = withText(span, from(rec.FetchString(span, "text"), count))
			count = 0
		}
		result = append(result, span)
	}
	return result
}

// takeText: the spans of the first count characters of marked.
func takeText(spans []rec.Map, count int) []rec.Map {
	result := []rec.Map{}
	for _, span := range spans {
		if count <= 0 {
			break
		}
		if span["type"] == "text" {
			text := rec.FetchString(span, "text")
			result = append(result, withText(span, prefix(text, count)))
			count -= size(text)
			continue
		}
		result = append(result, span)
		count--
	}
	return result
}

func (e *exporter) inlineRun(span, owner rec.Map, upright bool) run {
	switch rec.Fetch(span, "type") {
	case "text":
		if truthy(span["line_break"]) {
			// A heading set in several lines (decision 126): its space, then a visible break.
			return run{raw: true, text: ` \ `}
		}
		return run{text: spokenText(span), style: styleOf(span, upright || e.upright), link: span["external_link"]}
	case "math":
		return run{raw: true, text: e.linkedFormula(span, e.mathRun(span))}
	case "reference":
		return run{raw: true, text: e.reference(span)}
	case "note_reference":
		return run{raw: true, text: e.footnote(e.node(rec.Fetch(span, "target")))}
	case "image":
		return run{raw: true, text: e.image(span)}
	case "table_grid":
		return run{raw: true, text: e.table(rec.FetchMap(span, "grid"))}
	case "symbol":
		// An author's QED mark shows its image (decision 176).
		if image, ok := span["image"].(rec.Map); ok {
			return run{raw: true, text: e.image(image)}
		}
		return run{raw: true, text: escape(toS(span["text"]))}
	case "rule":
		return run{raw: true}
	}
	rec.Fail("Unsupported inline node for Typst: %s in %s", toS(span["type"]), toS(owner["id"]))
	return run{}
}

// spokenText is a text span's text, or for a glyph with no character but a name (marvosym's
// Tumbler) the name in brackets in place of U+FFFD.
func spokenText(span rec.Map) string {
	if name, named := span["glyph_name"].(string); named {
		return "[" + inline.GlyphLabel(name) + "]"
	}
	return rec.FetchString(span, "text")
}

func styleOf(span rec.Map, upright bool) textStyle {
	style, _ := span["style"].(rec.Map)
	shape := toS(style["shape"])
	return textStyle{bold: strings.HasPrefix(toS(style["series"]), "b"), italic: !upright && (shape == "it" || shape == "sl"),
		mono: style["generic"] == "monospace", caps: shape == "sc", lang: toS(span["lang"]), underline: span["decoration"] == "underline",
		strike: span["decoration"] == "strike"}
}

// joinRuns: adjacent text runs of one style become one styled run; whitespace is collapsed. A run
// that writes nothing (a rule) separates nothing: text escaped in two parts around it would meet
// unescaped ("-" and "93" a minus sign, "/" and "/" a comment; decision 154).
func (e *exporter) joinRuns(runs []run) string {
	merged := []run{}
	for _, current := range runs {
		if current.raw && current.text == "" {
			continue
		}
		if count := len(merged); count > 0 && !current.raw && !merged[count-1].raw && merged[count-1].style == current.style &&
			merged[count-1].link == current.link {
			merged[count-1].text += current.text
			continue
		}
		merged = append(merged, current)
	}
	pieces := make([]string, len(merged))
	for index, current := range merged {
		if current.raw {
			pieces[index] = current.text
		} else {
			pieces[index] = e.styled(current.text, current.style, current.link)
		}
	}
	return runSpaces.ReplaceAllString(strings.Join(ended(pieces), ""), " ")
}

// ended: text right after an embedded call that starts with ( [ . or ; would continue it (or lose
// its semicolon): end the call with ;.
// The text that follows is the next piece that writes anything: a rule between them writes
// nothing, so `#raw("…")` and "(1)" still meet (books/0001: a listing, a rule, its tag).
func ended(pieces []string) []string {
	result := make([]string, len(pieces))
	for index, piece := range pieces {
		following := ""
		for _, next := range pieces[index+1:] {
			if next != "" {
				following = next
				break
			}
		}
		if callPiece.MatchString(piece) && callEnd.MatchString(piece) && callFollow.MatchString(following) {
			result[index] = rec.RStrip(piece) + ";"
		} else {
			result[index] = piece
		}
	}
	return result
}

func (e *exporter) styled(text string, style textStyle, link any) string {
	text = runSpaces.ReplaceAllString(text, " ")
	core := rec.Strip(text)
	if core == "" {
		return text
	}
	body := escapeNBSP(core)
	if style.mono {
		body = "#raw(" + typstString(core) + ")"
	} else if wrapped(style, link) {
		// A content block starts a line: "- ", "/ ", "1. " there would open a list or a term
		// (decision 154, found by the escaping fuzz).
		body = protectStart(body)
	}
	if style.caps {
		body = "#smallcaps[" + body + "]"
	}
	if style.italic {
		body = "#emph[" + body + "]"
	}
	if style.bold {
		body = "#strong[" + body + "]"
	}
	// Text-mode \underline and soul's \ul: underlined text; soul's \st: struck-out text.
	if style.underline {
		body = "#underline[" + body + "]"
	}
	if style.strike {
		body = "#strike[" + body + "]"
	}
	if style.lang != "" {
		body = "#text(" + typstLanguage(style.lang) + ")[" + body + "]"
	}
	if truthy(link) {
		if href := e.externalHref(link); truthy(href) {
			body = "#link(" + typstString(toS(href)) + ")[" + body + "]"
		}
	}
	return lead(text) + body + trail(text)
}

// wrapped: styled puts the text in a content block of its own.
func wrapped(style textStyle, link any) bool {
	return style.caps || style.italic || style.bold || style.underline || style.strike || style.lang != "" || truthy(link)
}

// escapeNBSP: a no-break space is Typst's ~; everything else is escaped text.
func escapeNBSP(text string) string {
	parts := strings.Split(text, " ")
	for index, part := range parts {
		parts[index] = escape(part)
	}
	return strings.Join(parts, "~")
}

func (e *exporter) externalHref(link any) any {
	if e.externalHrefs == nil {
		e.externalHrefs = map[string]any{}
		for _, row := range maps(dig(e.document, "external_links", "links")) {
			e.externalHrefs[toS(row["id"])] = linkTarget(row)
		}
		// The model's external links are a map from the link's id to its record (paperdoc's "items").
		if items, ok := dig(e.document, "external_links", "items").(rec.Map); ok {
			for id, row := range items {
				if record, isMap := row.(rec.Map); isMap {
					e.externalHrefs[id] = linkTarget(record)
				}
			}
		}
	}
	return e.externalHrefs[toS(link)]
}

// linkTarget is a link record's address: href, else uri, else url.
func linkTarget(row rec.Map) any {
	for _, field := range []string{"href", "uri", "url"} {
		if truthy(row[field]) {
			return row[field]
		}
	}
	return nil
}

// mathRun: a URL the reading took for a formula (\url set in math mode) is the link it
// prints: its target, or the characters it shows.
func (e *exporter) mathRun(span rec.Map) string {
	shown := strings.ReplaceAll(strings.ReplaceAll(e.mathText(span), " ", ""), "−", "-")
	if !urlText.MatchString(shown) {
		return "$" + e.mathBody(e.node(rec.Fetch(span, "node"))) + "$"
	}
	href := any(shown)
	if link := span["external_link"]; truthy(link) {
		if found := e.externalHref(link); truthy(found) {
			href = found
		}
	}
	return "#link(" + typstString(toS(href)) + ")"
}

func (e *exporter) mathBody(node rec.Map) (body string) {
	defer e.lost(rec.Fetch(node, "id"), &body)
	return e.math.convert(rec.FetchString(node, "mathml"))
}

// lost is the rescue of a formula Typst math cannot express: a loss, and a placeholder.
func (e *exporter) lost(node any, body *string) {
	recovered := recover()
	if recovered == nil {
		return
	}
	failure, ok := recovered.(rec.Failure)
	if !ok || failure.Lookup {
		panic(recovered)
	}
	e.losses = append(e.losses, rec.Map{"node": node, "reason": failure.Message})
	*body = `"[formula]"`
}

// refMark: @label, marked where the reference ends.
func refMark(label string) string { return refStart + "@" + label + refEnd }

// terminateReferences: a reference followed by a character Typst would read as part of the
// label ("@fig:2We", "@key998") is written #ref(<label>) instead.
func terminateReferences(text string) string {
	var builder strings.Builder
	for {
		start := strings.Index(text, refStart+"@")
		if start < 0 {
			break
		}
		end := strings.Index(text[start+2:], refEnd)
		if end <= 0 {
			builder.WriteString(text[:start+2])
			text = text[start+2:]
			continue
		}
		label := text[start+2 : start+2+end]
		rest := text[start+2+end+1:]
		builder.WriteString(text[:start])
		if continues(rest) {
			builder.WriteString("#ref(<" + label + ">);")
		} else {
			builder.WriteString("@" + label)
		}
		text = rest
	}
	builder.WriteString(text)
	return builder.String()
}

// continues is (?=([\p{L}\p{N}_-]|[.:][\p{L}\p{N}_])).
func continues(rest string) bool {
	runes := []rune(rest)
	if len(runes) == 0 {
		return false
	}
	if letterOrNumber(runes[0]) || runes[0] == '_' || runes[0] == '-' {
		return true
	}
	return (runes[0] == '.' || runes[0] == ':') && len(runes) > 1 && (letterOrNumber(runes[1]) || runes[1] == '_')
}

// reference: \ref prints the number; @label prints the same number when the target's
// numbering matches it, else the printed text links to the target.
func (e *exporter) reference(span rec.Map) string {
	text := rec.FetchString(span, "text")
	leading := lead(text)
	printed := rec.Strip(text)
	name := toS(span["label"])
	label, labelled := e.labels[name]
	placed := e.placedBefore[name]
	if !truthy(span["label"]) || !labelled || placed == "" {
		return leading + escape(printed)
	}
	if placed == anchor {
		return leading + "#link(<" + label + ">)[" + escape(printed) + "]"
	}
	shown, known := e.labelNumbers[name]
	return leading + e.numberedReference(label, printed, shown, known)
}

// linkedFormula: a formula set wholly inside a \hyperref text is part of that link, where the
// link's label is placed; elsewhere it stays as set.
func (e *exporter) linkedFormula(span rec.Map, formula string) string {
	name := toS(span["link_label"])
	label, labelled := e.labels[name]
	if !truthy(span["link_label"]) || !labelled || e.placedBefore[name] == "" {
		return formula
	}
	return "#link(<" + label + ">)[" + formula + "]"
}

func (e *exporter) numberedReference(label, printed, shown string, known bool) string {
	// A thin space in the number (REVTeX's III\,B) may be lost from the reading.
	if known && strings.ReplaceAll(printed, " ", "") == strings.ReplaceAll(shown, " ", "") {
		return refMark(label)
	}
	if known && printed == "("+shown+")" {
		return "(" + refMark(label) + ")"
	}
	if known {
		supplement := regexp.MustCompile(`\A(.+?)[ ~]` + regexp.QuoteMeta(shown) + `\z`)
		if match := supplement.FindStringSubmatch(printed); match != nil {
			return "#ref(<" + label + ">, supplement: [" + escape(match[1]) + "])"
		}
	}
	e.frozen++
	return "#link(<" + label + ">)[" + escape(printed) + "]"
}

// citation: a citation call: its printed labels become @key references ([@a, @b] prints
// [1, 2]); a compressed range [2–8] keeps its form; an author-year call keeps its printed
// text. A formula in it (mnras's $et~al$) reads as its characters.
func (e *exporter) citation(group []rec.Map) string {
	var text strings.Builder
	for _, span := range group {
		if span["type"] == "math" {
			text.WriteString(e.mathText(span))
		} else if value, ok := span["text"]; ok && value != nil {
			text.WriteString(toS(value))
		}
	}
	call, ok := e.citationCalls[toS(group[0]["citation"])]
	if !ok {
		return escape(text.String())
	}
	keys := stringList(rec.Fetch(call, "keys"))
	authorYear := slices.ContainsFunc(keys, func(key string) bool {
		return e.placedBefore["cite:"+key] == anchor && !e.labelsMerged[key]
	})
	if !authorYear {
		return e.numberedCitation(text.String(), keys)
	}
	var markup strings.Builder
	for _, span := range group {
		if span["type"] == "math" {
			markup.WriteString("_" + escape(e.mathText(span)) + "_")
		} else if value, ok := span["text"]; ok && truthy(value) {
			markup.WriteString(escape(toS(value)))
		}
	}
	return e.authorYearCitation(text.String(), keys, markup.String())
}

type citationPosition struct {
	position int
	key      string
	printed  string
}

func (e *exporter) numberedCitation(text string, keys []string) string {
	brackets := !balanced(text)
	found := e.citationPositions(text, keys)
	slices.SortStableFunc(found, func(a, b citationPosition) int { return a.position - b.position })
	runes := []rune(text)
	cursor := 0
	var parts strings.Builder
	for _, entry := range found {
		parts.WriteString(escapeBrackets(string(runes[cursor:entry.position]), brackets))
		parts.WriteString(e.citationReference(entry.key, entry.printed))
		cursor = entry.position + size(entry.printed)
	}
	// Keys merged into one entry (REVTeX's \bibitem lists) share its printed label.
	missing := 0
	for _, key := range keys {
		label, ok := e.citationLabels[key]
		if !ok || !slices.ContainsFunc(found, func(entry citationPosition) bool { return entry.printed == label }) {
			missing++
		}
	}
	if !(dashes.MatchString(text) && missing < len(keys)) {
		e.unexpressed += missing
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}
	parts.WriteString(escapeBrackets(string(runes[cursor:]), brackets))
	return parts.String()
}

// citationPositions: where each key's printed label stands, as a whole token: a sorting
// citation package prints the labels in its own order, and [12] does not contain [1].
func (e *exporter) citationPositions(text string, keys []string) []citationPosition {
	runes := []rune(text)
	taken := map[int]bool{}
	result := []citationPosition{}
	for _, key := range keys {
		printed, ok := e.citationLabels[key]
		if !ok {
			continue
		}
		for _, start := range tokenPositions(runes, []rune(printed)) {
			if !taken[start] {
				taken[start] = true
				result = append(result, citationPosition{start, key, printed})
				break
			}
		}
	}
	return result
}

// tokenPositions is text.scan(/(?<![\p{L}\p{N}])printed(?![\p{L}\p{N}])/) as start offsets.
func tokenPositions(text, token []rune) []int {
	positions := []int{}
	for at := 0; at <= len(text)-len(token); {
		matches := slices.Equal(text[at:at+len(token)], token) && (at == 0 || !letterOrNumber(text[at-1])) &&
			(at+len(token) == len(text) || !letterOrNumber(text[at+len(token)]))
		if !matches {
			at++
			continue
		}
		positions = append(positions, at)
		at += max(len(token), 1)
	}
	return positions
}

// balanced: [1, 2] with its brackets paired: they can stay literal.
func balanced(text string) bool {
	depth := 0
	for _, char := range text {
		if char == '[' {
			depth++
		}
		if char == ']' {
			depth--
		}
		if depth < 0 {
			return false
		}
	}
	return depth == 0
}

func (e *exporter) authorYearCitation(text string, keys []string, markup string) string {
	labels := []string{}
	for _, key := range keys {
		label, ok := e.labels["cite:"+key]
		if ok && e.placedBefore["cite:"+key] != "" {
			labels = append(labels, "<"+label+">")
		}
	}
	if len(labels) == 0 {
		return markup
	}
	return lead(text) + "#citeas(" + strings.Join(labels, ", ") + ")[" + rec.Strip(markup) + "]"
}

// citationReference: @key for an item's own key; a key merged into another item links to it
// with its label.
func (e *exporter) citationReference(key, printed string) string {
	if cite, ok := e.styledCitation(key); ok {
		return cite
	}
	label, ok := e.labels["cite:"+key]
	placed := e.placedBefore["cite:"+key]
	if !ok || placed == "" {
		return escape(printed)
	}
	if placed == anchor {
		return "#link(<" + label + ">)[" + escape(printed) + "]"
	}
	return refMark(label)
}

// footnote: a footnote is set where it is called; a second call refers to the first.
func (e *exporter) footnote(note rec.Map) string {
	id := toS(rec.Fetch(note, "id"))
	if label, ok := e.footnoteLabels[id]; ok {
		return "#footnote(<" + label + ">)"
	}
	e.footnotesWritten[id] = true
	number := rec.Fetch(note, "number")
	// A mark LaTeX left as code (amsart's title notes print \@thefnmark) prints no mark.
	printed := toS(number)
	// A margin note (\marginpar, the number "marginpar") has no mark either.
	if strings.Contains(printed, `\`) || printed == "marginpar" {
		printed = ""
	}
	directive := e.footnotes.next(printed)
	update := ""
	if directive.update != nil {
		update = "#counter(footnote).update(" + directive.update.String() + ")"
	}
	head := "#footnote"
	if directive.override != nil {
		head = "#footnote(numbering: _ => " + typstString(*directive.override) + ")"
	}
	label := e.footnoteLabel(note)
	e.noteDepth++
	defer func() { e.noteDepth-- }()
	body := e.nested(func() string {
		parts := []string{}
		for _, child := range children(note) {
			if output := e.renderNode(child); output != "" {
				parts = append(parts, output)
			}
		}
		return strings.Join(parts, " #parbreak() ")
	})
	return update + head + "[" + rec.Strip(body) + "]" + label
}

// footnoteLabel: a \label in a footnote names the footnote (\ref prints its number): it
// labels the element. A footnote called again needs a label too.
func (e *exporter) footnoteLabel(note rec.Map) string {
	id := toS(rec.Fetch(note, "id"))
	name := ""
	e.walk(id, func(inner rec.Map) {
		if name == "" && toS(inner["id"]) != id {
			name = e.ownLabel(inner)
		}
	})
	if name != "" {
		number := toS(rec.Fetch(note, "number"))
		e.claimedLabels[name] = true
		e.labelNumbers[name] = number
		e.footnoteLabels[id] = e.labels[name]
		return e.labelSuffix(name, true, &number)
	}
	if len(rec.FetchList(note, "callsites")) <= 1 {
		return ""
	}
	label := "fn-" + itoa(len(e.footnoteLabels)+1)
	e.footnoteLabels[id] = label
	return " <" + label + ">"
}

func itoa(number int) string { return toS(int64(number)) }

func (e *exporter) image(span rec.Map) string {
	width := rubyRound(geometry.SPToBP(float64(rec.FetchInt(span, "width_sp"))), 2)
	formatted, err := canonjson.Marshal(width)
	if err != nil {
		rec.Fail("%v", err)
	}
	alt := ""
	if text, ok := span["alt"].(string); ok && strings.TrimSpace(text) != "" {
		alt = ", alt: " + typstString(text)
	}
	image := "#image(" + typstString(e.imagePrefix+e.imageFile(span)) + ", width: " + string(formatted) + "pt" + alt + ")"
	// A long description (decision 110) follows the figure's block as text of its own.
	if long, ok := span["long_description"].(string); ok && strings.TrimSpace(long) != "" && alt != "" {
		if text := "#block[#emph[Description of the figure:] #text(" + typstString(long) + ")]"; !slices.Contains(e.longDescriptions, text) {
			e.longDescriptions = append(e.longDescriptions, text)
		}
	}
	// The ORCID logo is its link.
	if _, linked := span["external_link"]; linked {
		return "#link(" + typstString(rec.AsString(paperdoc.ExternalHref(e.document, span))) + ")[" + image + "]"
	}
	return image
}

func (e *exporter) imageFile(span rec.Map) string {
	keyBytes, err := canonjson.Marshal([]any{rec.Fetch(span, "asset"), span["placement"]})
	if err != nil {
		rec.Fail("%v", err)
	}
	key := string(keyBytes)
	if name, ok := e.writtenImages[key]; ok {
		return name
	}
	items := rec.FetchMap(rec.FetchMap(e.document, "assets"), "items")
	asset, ok := items[rec.FetchString(span, "asset")].(rec.Map)
	if !ok {
		panic(rec.Failure{Message: "key not found: " + toS(span["asset"]), Lookup: true})
	}
	placement, _ := span["placement"].(rec.Map)
	figure := mediamodel.NewMediaAsset(asset)
	item := figure.Representation("typst", placement)
	extension, known := mediamodel.Extensions[rec.FetchString(item, "mime_type")]
	if !known {
		rec.Fail("key not found: %s", typstString(rec.FetchString(item, "mime_type")))
	}
	names := e.figureNames()
	if err := os.MkdirAll(filepath.Join(e.directory, figuresDirectory), 0o755); err != nil {
		rec.Fail("%v", err)
	}
	name := figuresDirectory + "/" + figure.Materialize(filepath.Join(e.directory, figuresDirectory), "typst", placement,
		names.Claim(rec.FetchString(item, "sha256"), names.Stem(rec.FetchString(span, "asset"), span)+"."+extension))
	e.writtenImages[key] = name
	return name
}

// figureNames is the figures of this output, in the document's reading order (decision 068).
func (e *exporter) figureNames() *figures.Names {
	if e.names == nil {
		e.names = figures.Stems(e.document)
	}
	return e.names
}

var hundred = 100.0

// rubyRound is Float#round(ndigits) for small ndigits (round half up, as Ruby's
// round_half_up corrects the scaled value).
func rubyRound(number float64, digits int) float64 {
	if number == 0 {
		return number
	}
	scale := math.Pow(10, float64(digits))
	if digits == 2 {
		scale = hundred
	}
	scaled := number * scale
	rounded := math.Round(scaled)
	if number > 0 {
		if (rounded+0.5)/scale <= number {
			rounded++
		}
	} else if (rounded-0.5)/scale >= number {
		rounded--
	}
	return rounded / scale
}

// typstLanguage is a BCP 47 tag as Typst's text arguments: the language, and the region when the
// tag names one (de-AT is lang: "de", region: "AT").
func typstLanguage(tag string) string {
	parts := strings.Split(tag, "-")
	arguments := "lang: " + typstString(parts[0])
	for _, part := range parts[1:] {
		if len(part) == 2 {
			return arguments + ", region: " + typstString(part)
		}
	}
	return arguments
}
