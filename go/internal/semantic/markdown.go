package semantic

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/sarielhp/relatex/internal/authortex"
	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/htmlkit"
	"github.com/sarielhp/relatex/internal/inline"
	"github.com/sarielhp/relatex/internal/mathexport"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
)

// Root is the repository root, where profiles/katex-support.json is read; set it before use.
var Root string

var groupTags = map[string]string{"document": "div", "section": "section", "theorem": "section", "proof": "section",
	"ordered_list": "ol", "unordered_list": "ul", "list_item": "li", "code": "div",
	"figure_float": "figure", "table_float": "figure", "float_caption": "figcaption"}

// mathEnd marks where inline math closes; Pandoc does not close it at a `$` a digit follows
// ($\leq$500), so an empty comment goes between. Only with the author's math (decision 064).
const mathEnd = "\U0000E020"

// closeInlineMath is MarkdownExport#render's gsub(/MATH_END(?=\d)/, '<!-- -->').delete(MATH_END).
func closeInlineMath(text string) string {
	if !strings.Contains(text, mathEnd) {
		return text
	}
	var builder strings.Builder
	for {
		at := strings.Index(text, mathEnd)
		if at < 0 {
			builder.WriteString(text)
			break
		}
		builder.WriteString(text[:at])
		text = text[at+len(mathEnd):]
		if next, _ := utf8.DecodeRuneInString(text); text != "" && next >= '0' && next <= '9' {
			builder.WriteString("<!-- -->")
		}
	}
	return builder.String()
}

// markdown is MarkdownExport with MarkdownAuthorMath.
type markdown struct {
	*export
	translator *mathexport.TeX
	groupRows  map[any][]rec.Map
}

func (m *markdown) header() string {
	notice := "<!-- Math uses normalized export TeX or native MathML derived from retained presentation evidence. -->\n\n"
	if _, ok := m.document["math_macros"]; ok {
		notice = "<!-- Math is the author's TeX where retypesetting verified it, else normalized export TeX or " +
			"native MathML derived from retained presentation evidence. -->\n\n" + m.macroBlock()
	}
	fonts := htmlkit.SourceFontsCSS(m.document) + mathexport.AccentFontCSS(m.document)
	if fonts == "" {
		return notice
	}
	return notice + "<style>" + fonts + htmlkit.LinkCSS + "</style>\n\n"
}

func (m *markdown) renderGroup(node rec.Map, children []string) string {
	body := strings.Join(children, "\n\n")
	id := toS(rec.Fetch(node, "id"))
	if node["type"] == "note" {
		return "<aside id=\"" + id + "\">\n\n" + body + "\n\n</aside>"
	}
	tag, ok := groupTags[rec.FetchString(node, "type")]
	if !ok {
		rec.Fail("Unsupported Markdown structure: %s", toS(node["type"]))
	}
	style := ""
	if tag == "ol" || tag == "ul" {
		style = ` style="list-style-type:none"`
	}
	// An item whose marker a beamer theme draws (decision 127) shows it as its list style.
	if marker, marked := node["marker"].(rec.Map); marked && tag == "li" {
		style = ` style="list-style-type:` + rec.FetchString(marker, "shape") + `"`
	}
	return "<" + tag + " id=\"" + id + "\"" + style + ">\n\n" + body + "\n\n</" + tag + ">"
}

// sectionNumber is /\A(\d+(?:\.\d+)*\.?)(?=[\p{L}<*_])/ with the lookahead consumed: Go's
// regexp picks the submatch a backtracking engine would.
var sectionNumber = regexp.MustCompile(`\A(\d+(?:\.\d+)*\.?)([\p{L}<*_])`)

func (m *markdown) block(node rec.Map, body string) string {
	anchor := "<a id=\"" + toS(rec.Fetch(node, "id")) + "\"></a>\n\n"
	switch node["type"] {
	case "heading":
		if match := sectionNumber.FindStringSubmatchIndex(body); match != nil {
			body = body[:match[3]] + " " + body[match[3]:]
		}
		return anchor + "## " + body
	case "paragraph", "display_math":
		return anchor + body
	}
	rec.Fail("Unsupported Markdown block: %s", toS(node["type"]))
	return ""
}

var markdownSpecial = strings.NewReplacer(`\`, `\\`, "`", "\\`", "*", `\*`, "_", `\_`, "[", `\[`, "]", `\]`, "<", `\<`, ">", `\>`, "$", `\$`)

func (m *markdown) styledText(span rec.Map) string {
	value := markdownSpecial.Replace(spokenText(span))
	style, ok := span["style"].(rec.Map)
	if rec.Strip(value) == "" {
		return value
	}
	if !ok {
		return underlined(span, value)
	}
	trimmed := strings.TrimLeft(value, spaceCharacters)
	before := value[:len(value)-len(trimmed)]
	after := trimmed[len(strings.TrimRight(trimmed, spaceCharacters)):]
	if strings.TrimRight(value, spaceCharacters) == "" {
		after = ""
	}
	value = rec.Strip(value)
	if strings.HasPrefix(rec.FetchString(style, "series"), "b") {
		value = "**" + value + "**"
	}
	if shape := rec.FetchString(style, "shape"); shape == "it" || shape == "sl" {
		value = "*" + value + "*"
	}
	if rec.FetchString(style, "shape") == "sc" {
		value = `<span style="font-variant:small-caps">` + value + "</span>"
	}
	return before + underlined(span, value) + after
}

// markdownDecorations is the CSS line of each text decoration: text-mode \underline and soul's \ul
// underline, soul's \st strikes out (decision 134).
var markdownDecorations = map[string]string{"underline": "underline", "strike": "line-through"}

// underlined: decorated text (an underline, a strike-out) is written like small caps, as the page's
// own CSS (Markdown has no underline of its own, and CommonMark no strike-out). Leading white space
// stays outside the line.
func underlined(span rec.Map, value string) string {
	line, decorated := markdownDecorations[rec.ToS(span["decoration"])]
	if !decorated {
		return value
	}
	trimmed := strings.TrimLeft(value, spaceCharacters)
	return value[:len(value)-len(trimmed)] + `<span style="text-decoration:` + line + `">` + trimmed + "</span>"
}

func (m *markdown) reference(span rec.Map) string {
	return m.internalLink(htmlkit.Escape(rec.FetchString(span, "text")), rec.Fetch(span, "target"))
}

func (m *markdown) internalLink(content string, id any) string {
	return "[" + content + "](#" + toS(id) + ")"
}

func (m *markdown) externalLink(content string, href any) string {
	return "[" + content + "](" + strings.NewReplacer("(", "%28", ")", "%29").Replace(rec.AsString(href)) + ")"
}

func (m *markdown) noteReference(span rec.Map) string {
	return "<a id=\"" + toS(rec.Fetch(span, "callsite")) + "\" href=\"#" + toS(rec.Fetch(span, "target")) + "\"><sup>" +
		htmlkit.Escape(rec.FetchString(span, "text")) + "</sup></a>"
}

func (m *markdown) convert(xml string) string {
	if m.translator == nil {
		m.translator = mathexport.NewTeX("pandoc")
	}
	return m.translator.Convert(xml)
}

func (m *markdown) mathFormat() string { return "tex" }

// nativeMath is MarkdownExport#native_math_projection: the formula as native MathML, or ok
// false when even that fails.
func (m *markdown) nativeMath(node rec.Map, id, failure string) (result string, ok bool) {
	defer withoutProjection(&result, &ok)
	xml := mathexport.ParsePaper(rec.FetchString(node, "mathml"))
	xml.SetAttribute("id", id)
	for _, child := range rec.FetchList(node, "children") {
		row := m.node(child)
		element := xml.Path(rec.FetchString(row, "mathml_path"))
		if element == nil {
			panic(errNilElement)
		}
		element.SetAttribute("id", rec.AsString(child))
	}
	m.representations = m.representations.Set(id, ojson.Object{{"kind", "native_mathml"}, {"format", "mathml"},
		{"source_mathml_sha256", digest(rec.FetchString(node, "mathml"))}, {"reason", failure}, {"rendering", "unverified"}})
	return mathexport.Project(xml), true
}

// withoutProjection is native_math_projection's rescue: a Failure means no native MathML.
func withoutProjection(result *string, ok *bool) {
	recovered := recover()
	if recovered == nil {
		return
	}
	if caught, isFailure := recovered.(rec.Failure); isFailure && !caught.Lookup {
		*result, *ok = "", false
		return
	}
	panic(recovered)
}

var errNilElement = errorString("undefined method 'add_attribute' for nil")

type errorString string

func (e errorString) Error() string { return string(e) }

func (m *markdown) math(value string, display bool, id string) string {
	anchors := m.rowAnchors(id)
	if display {
		return anchors + "\n\n$$\n" + value + "\n$$\n\n"
	}
	end := ""
	if _, ok := m.document["math_macros"]; ok {
		end = mathEnd
	}
	return anchors + "$" + value + "$" + end
}

func (m *markdown) rowAnchors(id string) string {
	var builder strings.Builder
	for _, child := range rec.FetchList(m.node(id), "children") {
		builder.WriteString(m.rowAnchor(m.node(child)))
	}
	return builder.String()
}

// image is a figure with its description from relatex/alt.toml, else "Source figure".
func (m *markdown) image(span rec.Map) string {
	alt := "Source figure"
	if text, ok := span["alt"].(string); ok && strings.TrimSpace(text) != "" {
		alt = markdownAlt.Replace(text)
	}
	// A decorative image (decision 096) has an empty alternative.
	if span["decorative"] == true {
		alt = ""
	}
	return "![" + alt + "](" + m.assetFile(span) + ")"
}

// longDescription is a figure's long description (decision 110): a paragraph after the figure's.
func (m *markdown) longDescription(text string) string {
	return "*Description of the figure:* " + markdownSpecial.Replace(text)
}

// markdownAlt escapes what would end or break an image's alternative text.
var markdownAlt = strings.NewReplacer(`\`, `\\`, "[", `\[`, "]", `\]`, "\n", " ")

func (m *markdown) fallbackImage(path, _ string) string {
	return "![Formula rendered from retained MathML](" + path + ")"
}

func (m *markdown) table(grid, span rec.Map) string {
	for _, item := range inline.Flat([]any{span}, false) {
		if item["type"] != "math" {
			continue
		}
		id := rec.FetchString(item, "node")
		m.representations = m.representations.Set(id, ojson.Object{{"kind", "native_mathml"}, {"format", "mathml"},
			{"source_mathml_sha256", digest(rec.FetchString(m.node(id), "mathml"))},
			{"reason", "Native MathML in a structured HTML table cell"}, {"rendering", "unverified"}})
	}
	return "\n\n" + m.tableGridHTML(rec.Merge(span, rec.Map{"grid": grid})) + "\n\n"
}

// symbol: the open square, or an author's QED mark as drawn (decision 176): its glyphs or its image without alt text.
func (m *markdown) symbol(span rec.Map) string {
	switch {
	case span["name"] == "qed_open_square":
		return "□"
	case span["name"] != "qed":
		rec.Fail("Unsupported Markdown symbol")
	}
	if image, ok := span["image"].(rec.Map); ok {
		return m.image(image)
	}
	if text, ok := span["text"].(string); ok {
		return markdownSpecial.Replace(text)
	}
	return "□"
}

// rule: Markdown has no rule inline; raw HTML keeps the bar at its size.
func (m *markdown) rule(span rec.Map) string {
	return "<span style=\"" + htmlkit.RuleCSS(span) + "\"></span>"
}

func (m *markdown) pageTotal(node rec.Map) string {
	value := toS(rec.Fetch(node, "count"))
	if rendered, ok := renderedContent(node, m.renderSpan); ok {
		value = rendered
	}
	return "<a id=\"" + toS(rec.Fetch(node, "id")) + "\"></a>Source PDF: " + value + " pages."
}

func (m *markdown) rowAnchor(node rec.Map) string {
	return "<a id=\"" + toS(rec.Fetch(node, "id")) + "\"></a>"
}

// mathSpan is MarkdownAuthorMath#math_span: a verified formula as its author wrote it.
func (m *markdown) mathSpan(span, owner rec.Map) string {
	id := rec.FetchString(span, "node")
	node := m.node(id)
	text, ok := m.authorMath(node)
	if !ok {
		if formula, proven := m.formulaMath(node); proven {
			m.representations = m.representations.Set(id, ojson.Object{{"kind", "formula_tex"}, {"format", "tex"},
				{"source_mathml_sha256", digest(rec.FetchString(node, "mathml"))}})
			return m.math(formula, owner["type"] == "display_math", id)
		}
		return m.semanticMath(span, owner)
	}
	representation := ojson.Object{{"kind", "author_tex"}, {"format", "tex"}, {"source_mathml_sha256", digest(rec.FetchString(node, "mathml"))}}
	if text == "" {
		m.representations = m.representations.Set(id, append(representation, ojson.Pair{"written_in", "first_row"}))
		return m.rowAnchors(id)
	}
	m.representations = m.representations.Set(id, representation)
	return m.math(text, owner["type"] == "display_math", id)
}

// formulaMath is the formula TeX written from the node's MathML (decision 141), when the round trip
// proved it and Markdown's renderers take it: an alignment as its inner environment, without row
// numbers (the row anchors serve references), and no "$" that would end Markdown's math.
func (m *markdown) formulaMath(node rec.Map) (string, bool) {
	field, ok := node["formula_tex"].(rec.Map)
	if !ok {
		return "", false
	}
	text := rec.FetchString(field, "text")
	if environment, has := field["environment"].(string); has && environment != "" {
		name, known := inner[environment]
		if !known || hasTag(text) {
			return "", false
		}
		text = "\\begin{" + name + "}\n" + text + "\n\\end{" + name + "}"
	}
	if strings.Contains(text, "$") || !m.katex(text) {
		return "", false
	}
	return text, true
}

// inner is MarkdownAuthorMath::INNER: a display environment of rows becomes the inner
// environment Markdown math takes.
var inner = map[string]string{"align": "aligned", "align*": "aligned", "flalign": "aligned", "flalign*": "aligned",
	"eqnarray": "aligned", "eqnarray*": "aligned", "gather": "gathered", "gather*": "gathered",
	"alignat": "alignedat", "alignat*": "alignedat"}

// pandocUnsupported: commands KaTeX takes but Pandoc's TeX reader rejects.
var pandocUnsupported = []string{"rm", "bf", "it", "sl", "sf", "tt", "cal", "frak", "Bbb", "em", "sc", "textup", "textnormal", "smash",
	"hspace", "over", "atop", "copyright", "varprojlim", "varliminf", "thicksim", "bigm", "tiny", "hbox", "hskip"}

// pandocUnparsed: constructs Pandoc's TeX reader cannot parse.
var pandocUnparsed = regexp.MustCompile(`\\begin[ \t\n\v\f\r]*\{array\}[ \t\n\v\f\r]*\[|\\(?:tilde|bar|hat|check|vec|dot|ddot|breve|acute|grave|` +
	`widetilde|widehat|overline)[ \t\n\v\f\r]*\\[A-Za-z]+[ \t\n\v\f\r]*\{|\\~`)

var (
	singleEnvironments = []string{"equation", "equation*", "displaymath", "math"}
	commandName        = regexp.MustCompile(`\\([A-Za-z]+)`)
	beginEnvironment   = regexp.MustCompile(`\\begin[ \t\n\v\f\r]*\{([^{}]+)\}`)
	numbering          = regexp.MustCompile(`\\(?:nonumber|notag)`)
	inlineBreak        = regexp.MustCompile(`[ \t\n\v\f\r]*\n[ \t\n\v\f\r]*`)
)

// authorMath is the author's TeX for a math node: "" for a row another row already wrote;
// ok false to fall back.
func (m *markdown) authorMath(node rec.Map) (string, bool) {
	tex, ok := node["tex"].(rec.Map)
	if !ok || tex["status"] != "verified" {
		return "", false
	}
	if truthy(tex["group"]) {
		return m.groupMath(node)
	}
	macros, _ := m.document["math_macros"].(rec.Map)
	text := authortex.Shipped(rec.FetchString(tex, "text"))
	usable := true
	if tex["form"] == "environment" {
		text, usable = singleEnvironment(tex, text)
	}
	if usable {
		text, usable = authortex.ExpandPairedVariants(text, macros)
	}
	if !usable || !m.katex(text) {
		return "", false
	}
	if tex["form"] == "inline" {
		text = inlineBreak.ReplaceAllString(text, " ")
	}
	return text, true
}

func singleEnvironment(tex rec.Map, text string) (string, bool) {
	environment := rec.FetchString(tex, "environment")
	if contains(singleEnvironments, environment) {
		return text, true
	}
	name, ok := inner[environment]
	if !ok || hasTag(text) {
		return "", false
	}
	return "\\begin{" + name + "}\n" + unnumbered(text) + "\n\\end{" + name + "}", true
}

// hasTag is /\\tag\b/: "\tag" not followed by a word character (Onigmo's boundary is
// Unicode-aware).
func hasTag(text string) bool {
	for offset := 0; ; {
		at := strings.Index(text[offset:], `\tag`)
		if at < 0 {
			return false
		}
		end := offset + at + len(`\tag`)
		next, _ := utf8.DecodeRuneInString(text[end:])
		if end >= len(text) || !isWord(next) {
			return true
		}
		offset = end
	}
}

func isWord(character rune) bool {
	return unicode.IsLetter(character) || unicode.IsDigit(character) || character == '_' || unicode.Is(unicode.Mn, character)
}

// unnumbered: without numbering commands, and without the blank lines they leave (a blank
// line ends Markdown math).
func unnumbered(text string) string {
	return rec.Strip(rec.WithoutBlankLines(removeWordMatches(text, numbering)))
}

// removeWordMatches is gsub(/\\(?:nonumber|notag)\b/, ”): a match counts only at a word
// boundary.
func removeWordMatches(text string, pattern *regexp.Regexp) string {
	var builder strings.Builder
	cursor := 0
	for _, match := range pattern.FindAllStringIndex(text, -1) {
		next, _ := utf8.DecodeRuneInString(text[match[1]:])
		if match[1] < len(text) && isWord(next) {
			continue
		}
		builder.WriteString(text[cursor:match[0]])
		cursor = match[1]
	}
	return builder.String() + text[cursor:]
}

func (m *markdown) groupMath(node rec.Map) (string, bool) {
	tex := rec.FetchMap(node, "tex")
	rows := m.rowsOf(rec.Fetch(tex, "group"))
	name, known := inner[rec.FetchString(tex, "environment")]
	macros, _ := m.document["math_macros"].(rec.Map)
	texts := []string{}
	usable := known && rec.Equal(int64(len(rows)), rec.Fetch(tex, "rows"))
	for _, row := range rows {
		rowText := unnumbered(authortex.Shipped(rec.FetchString(rec.FetchMap(row, "tex"), "text")))
		expanded, ok := authortex.ExpandPairedVariants(rowText, macros)
		if !ok {
			usable = false
		}
		texts = append(texts, expanded)
	}
	for index, row := range rows {
		usable = usable && rec.FetchMap(row, "tex")["status"] == "verified" && !hasTag(texts[index]) && m.katex(texts[index])
	}
	if !usable {
		return "", false
	}
	if !rec.Equal(rec.Fetch(tex, "row"), int64(1)) {
		return "", true
	}
	return "\\begin{" + name + "}\n" + strings.Join(texts, "\\\\\n") + "\n\\end{" + name + "}", true
}

func (m *markdown) rowsOf(group any) []rec.Map {
	if m.groupRows == nil {
		m.groupRows = map[any][]rec.Map{}
		for _, id := range sortedIDs(m.nodes) {
			node := rec.AsMap(m.nodes[id])
			tex, _ := node["tex"].(rec.Map)
			if node["type"] == "math" && tex != nil && truthy(tex["group"]) {
				m.groupRows[tex["group"]] = append(m.groupRows[tex["group"]], node)
			}
		}
		for key, rows := range m.groupRows {
			sort.SliceStable(rows, func(a, b int) bool {
				return rec.FetchInt(rec.FetchMap(rows[a], "tex"), "row") < rec.FetchInt(rec.FetchMap(rows[b], "tex"), "row")
			})
			m.groupRows[key] = rows
		}
	}
	rows, ok := m.groupRows[group]
	if !ok {
		panic(rec.Failure{Message: "key not found: " + toS(group), Lookup: true})
	}
	return rows
}

func sortedIDs(nodes rec.Map) []string {
	ids := make([]string, 0, len(nodes))
	for id := range nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

var (
	katexOnce     sync.Once
	katexCommands map[string]bool
	katexEnvs     map[string]bool
)

func loadKaTeX() {
	katexOnce.Do(func() {
		file, err := os.Open(filepath.Join(Root, "profiles", "katex-support.json"))
		if err != nil {
			rec.Fail("%v", err)
		}
		defer file.Close()
		value, err := canonjson.Decode(file)
		if err != nil {
			rec.Fail("%v", err)
		}
		profile := rec.AsMap(value)
		katexCommands, katexEnvs = map[string]bool{}, map[string]bool{}
		for _, name := range rec.FetchList(profile, "commands") {
			katexCommands[rec.AsString(name)] = true
		}
		for _, name := range rec.FetchList(profile, "environments") {
			katexEnvs[rec.AsString(name)] = true
		}
	})
}

// used is MathMacros.used: the control-sequence names a TeX text uses.
func used(text string) []string {
	result := []string{}
	for _, match := range commandName.FindAllStringSubmatch(text, -1) {
		if !contains(result, match[1]) {
			result = append(result, match[1])
		}
	}
	return result
}

// usedBy is MathMacros.used_by: the kept macros a set of texts uses, closed under use.
func usedBy(kept rec.Map, texts []string) rec.Map {
	wanted := []string{}
	for _, text := range texts {
		for _, name := range used(text) {
			if _, ok := kept[name]; ok && !contains(wanted, name) {
				wanted = append(wanted, name)
			}
		}
	}
	for {
		more := []string{}
		for _, name := range wanted {
			for _, inner := range used(rec.FetchString(rec.FetchMap(kept, name), "body")) {
				if _, ok := kept[inner]; ok && !contains(wanted, inner) && !contains(more, inner) {
					more = append(more, inner)
				}
			}
		}
		if len(more) == 0 {
			break
		}
		wanted = append(wanted, more...)
	}
	result := rec.Map{}
	for _, name := range wanted {
		result[name] = kept[name]
	}
	return result
}

// katex: KaTeX knows every command and environment the formula uses, and those of its
// macros, and Pandoc's TeX reader does too.
func (m *markdown) katex(text string) bool {
	loadKaTeX()
	macros, _ := m.document["math_macros"].(rec.Map)
	texts := []string{text}
	for _, name := range sortedKeys(usedBy(macros, []string{text})) {
		texts = append(texts, rec.FetchString(rec.FetchMap(macros, name), "body"))
	}
	for _, part := range texts {
		if authortex.PairedVariant(part, macros) {
			return false
		}
		for _, name := range used(part) {
			if _, macro := macros[name]; macro {
				continue
			}
			if !katexCommands[name] || contains(pandocUnsupported, name) {
				return false
			}
		}
		for _, match := range beginEnvironment.FindAllStringSubmatch(part, -1) {
			if !katexEnvs[match[1]] {
				return false
			}
		}
		if pandocUnparsed.MatchString(part) {
			return false
		}
	}
	return true
}

func sortedKeys(m rec.Map) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// macroBlock: the kept macros the written formulas use, as definitions Pandoc, MathJax and
// KaTeX read.
func (m *markdown) macroBlock() string {
	texts := []string{}
	for _, id := range sortedIDs(m.nodes) {
		node := rec.AsMap(m.nodes[id])
		tex, _ := node["tex"].(rec.Map)
		if node["type"] == "math" && tex != nil && tex["status"] == "verified" {
			if text, ok := tex["text"].(string); ok {
				texts = append(texts, text)
			}
		}
	}
	macros, _ := m.document["math_macros"].(rec.Map)
	kept := usedBy(macros, texts)
	if len(kept) == 0 {
		return ""
	}
	return authortex.Block(kept) + "\n\n"
}
