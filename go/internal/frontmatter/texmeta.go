package frontmatter

import (
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/sarielhp/relatex/internal/rec"
)

// The private-use characters PaperTexMetadata keeps a printed citation, reference or URL
// in, for matching against the typeset prose.
const (
	citationSlot  = ""
	referenceSlot = ""
	linkSlot      = ""
)

var (
	wrappers = []string{"emph", "textit", "textbf", "textnormal", "textrm", "institution", "department", "streetaddress",
		"city", "state", "postcode", "country", "texttt", "textsf", "textsc", "textmd", "textup", "textsl", "mbox", "fnm",
		"sur", "surname", "Name", "IEEEauthorblockN", "authorblockN", "context", "aims", "methods", "results", "conclusions", "url", "nolinkurl",
		"textsubscript", "textsuperscript", "affaddr"}
	// dropped: notes, marks, spacing and affiliation fields beside a name, dropped with their
	// argument.
	dropped = []string{"thanks", "sthanks", "footnote", "corref", "fnref", "thanksref", "IEEEmembership", "IEEEauthorrefmark",
		"IEEEcompsocitemizethanks", "textsuperscript", "vspace", "hspace", "inst", "email", "Email", "addr", "IEEEauthorblockA",
		"authorblockA", "orcid", "orcidlink", "footnotemark", "cite", "citep", "citet", "citealp", "color", "altaffilmark", "note", "keywords"}
	marks   = []string{"dag", "ddag", "dagger", "ddagger", "S", "P", "ast", "star"}
	spacing = []string{"noindent", "indent", "centering", "quad", "qquad", "par", "newline", "linebreak", "endgraf", "break",
		"smallskip", "medskip", "bigskip", "hfill", "vfill", "IEEEcompsocthanksitem"}
	affiliationFields = []string{"institution", "department", "streetaddress", "city", "state", "postcode", "country"}
	fontSwitches      = []string{"em", "it", "bf", "rm", "sl", "sc", "tt", "sf", "normalfont", "tiny", "scriptsize", "footnotesize",
		"small", "normalsize", "large", "Large", "LARGE", "huge", "Huge", "boldmath", "unboldmath", "rmfamily", "sffamily", "ttfamily",
		"mdseries", "bfseries", "upshape", "itshape", "slshape", "scshape", "protect"}
	aaAbstract  = []string{"Context", "Aims", "Methods", "Results", "Conclusions"}
	letterNames = []string{"L", "l", "o", "O", "ss", "ae", "AE", "oe", "OE", "aa", "AA"}
	letters     = map[string]string{"L": "Ł", "l": "ł", "o": "ø", "O": "Ø", "ss": "ß", "ae": "æ", "AE": "Æ", "oe": "œ", "OE": "Œ", "aa": "å", "AA": "Å"}
	// The TeX logos print their names; like any control word they take the spaces after them,
	// so \LaTeX\ Project needs its control space (read before control spaces are).
	logoNames     = []string{"TeX", "LaTeX", "LaTeXe"}
	logos         = map[string]string{"TeX": "TeX", "LaTeX": "LaTeX", "LaTeXe": "LaTeX2ε"}
	accents       = map[string]string{"~": "̃", "'": "́", "`": "̀", `"`: "̈", "^": "̂"}
	letterAccents = map[string]string{"v": "̌", "H": "̋", "c": "̧", "u": "̆", "k": "̨", "r": "̊",
		"=": "̄", ".": "̇"}
	escaped     = map[string]string{"{": "", "}": "", "$": ""}
	mathSymbols = buildMathSymbols()

	command           = regexp.MustCompile(`\\([A-Za-z]+)\*?`)
	boundCommand      = regexp.MustCompile(`\\(?:ref|eqref|autoref|[cC]ref|cite[a-zA-Z]*|ensuremath)`)
	ensureMath        = rx(`\\ensuremath\s*\{((?:[^{}]|\{(?:[^{}]|\{[^{}]*\})*\})*)\}`)
	affiliationMark   = regexp.MustCompile(`\$(?:\{\})?\^(?:\{(?:[^{}$]|\{[^{}$]*\})*\}|\\?[0-9A-Za-z*]+)[^$]*\$`)
	definition        = rx(`(?:\\(?:(?:re)?newcommand|providecommand|DeclareRobustCommand)\*?\s*(?:\{\\[A-Za-z@]+\}|\\[A-Za-z@]+)\s*(?:\[\d\]\s*)?(?:\[[^\]]*\]\s*)?|\\[gex]?def\s*\\[A-Za-z@]+[^{]*)\{`)
	controlWordEnd    = regexp.MustCompile(`\\[A-Za-z]+$`)
	documentClass     = rx(`\\documentclass(?:\[[^\]]*\])?\s*\{\s*([^{}\s]+)\s*\}`)
	abstractEnv       = regexp.MustCompile(`(?s)\\begin\{abstract\}(.*?)\\end\{abstract\}`)
	documentEnv       = regexp.MustCompile(`\\begin\{document\}`)
	abstractLists     = regexp.MustCompile(`(?s)\\begin\{description\}.*?\\end\{description\}|\\begin\{itemize\}.*?\\end\{itemize\}|\\begin\{enumerate\}.*?\\end\{enumerate\}`)
	citeCall          = rx(`\\cite[pt]?\*?\s*(?:\[[^\]]*\]\s*)?\{[^{}]*\}`)
	referenceCall     = rx(`\\(?:eq|auto|c|C)?ref\*?\s*\{[^{}]*\}`)
	urlCall           = rx(`\\url\s*\{[^{}]*\}`)
	linkEscape        = regexp.MustCompile(`\\([%_&#])`)
	lineBreak         = regexp.MustCompile(`\\\\(?:\[[^\]]*\])?`)
	lineBreakAt       = regexp.MustCompile(`\A\\\\(?:\[[^\]]*\])?`)
	hrefColor         = rx(`\\(?:href|textcolor)\s*\{[^{}]*\}`)
	controlSymbol     = regexp.MustCompile(`\\[ ,;:!/@\n-]`)
	minipage          = regexp.MustCompile(`(?s)\\begin\{minipage\}(?:\[[^\]]*\])?\{[^{}]*\}(.*?)\\end\{minipage\}`)
	emailLine         = regexp.MustCompile(`\A[^ \t\n\v\f\r@]+@[^ \t\n\v\f\r@]+\.[^ \t\n\v\f\r@]+\z`)
	ordinal           = rx(`\d+\\textsuperscript\s*\{(?:st|nd|rd|th)\}\s*`)
	tabular           = regexp.MustCompile(`\\(?:begin|end)\{tabular\}(?:\[[^\]]*\])?(?:\{(?:[^{}]|\{[^{}]*\})*\})?`)
	membership        = rx(`,?\s*(?:(?:Student|Senior|Life|Graduate Student)\s+)?(?:Member|Fellow),?\s*IEEE`)
	authorMark        = regexp.MustCompile(`[\s*†‡∗]+$`) // a note mark beside a name is no name text
	nameSeparator     = rx(`\s*,\s*|\s+and\s+`)
	leadingAnd        = rx(`\Aand\s+`)
	contact           = rx(`\A[\w.+-]+(?:\s*,\s*[\w.+-]+)*\s*@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\z`)
	contactCommand    = regexp.MustCompile(`\\[A-Za-z]+\*?`)
	contactSymbol     = regexp.MustCompile(`\\.`)
	affiliationStart  = rx(`\A\s*\{?(?:\\[A-Za-z]+\s*)*\$(?:\{\})?\^`)
	typesetMarkStart  = rx(`\A\s*\\textsuperscript\s*\{[^{}]*\}`)
	footnoteMarkStart = rx(`\A\s*\\footnotemark\b`)
	typesetNameMark   = rx(`\\textsuperscript\s*\{|\\footnotemark\b`)
	andWord           = rx(`\\(?:and|And|AND)\b`)
	authorMarkLine    = rx(`[^}\s]\$?\^\{?(?:\((?:[\d,*]+|[A-Za-z])\)|[\d,*]+)`)
	commaOnlyArgument = rx(`\A[^{}]*,[^{}]*\z`)
	thanksOneLevel    = regexp.MustCompile(`\\thanks\s*\{[^{}]*\}`)
	textttOneLevel    = regexp.MustCompile(`\\texttt\s*\{[^{}]*\}`)
	parenEmail        = rx(`\([^()]*@[^()]*\)`)
	bareEmail         = rx(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`)
	emailPattern      = rx(`(?:\(([^()\s]+@[^()\s]+)\)|\b([A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,})\b)`)

	textMacro         = rx(`\\(?:re)?newcommand\*?\s*\{?\\([A-Za-z]+)\}?\s*\{((?:[^{}#]|\{(?:[^{}#]|\{[^{}#]*\})*\})*)\}`)
	defMacro          = rx(`\\def\s*\\([A-Za-z]+)\s*\{((?:[^{}#]|\{(?:[^{}#]|\{[^{}#]*\})*\})*)\}`)
	argumentHead      = rx(`(?:\\(?:re)?newcommand\*?\s*\{?\\([A-Za-z]+)\}?\s*\[1\]\s*|\\def\s*\\([A-Za-z]+)\s*#1\s*)\{`)
	parameter         = regexp.MustCompile(`#\d`)
	inlineMath        = regexp.MustCompile(`\$([^$]*)\$`)
	mathCommand       = rx(`\\([A-Za-z]+)\s*`)
	subscript         = regexp.MustCompile(`_\{(\d+)\}|_(\d+)`)
	superscript       = regexp.MustCompile(`\^\{(\d+)\}|\^(\d+)`)
	scriptGroup       = regexp.MustCompile(`([_^])\{([^{}\\]*)\}`)
	mathGroup         = regexp.MustCompile(`\{([^{}\\]*)\}`)
	plainMath         = regexp.MustCompile(`\A[\p{L}\p{N}\p{S}/\-., ()\[\]_]*\z`)
	mathAlphabet      = rx(`\\(?:math(?:bb|cal|rm|it|bf|sf|tt|frak)|text(?:rm|it|bf|up)?|boldsymbol)(?:\s*\{([^{}\\]*)\}|\s+([A-Za-z0-9]))`)
	glueSkip          = rx(`\\[vh]skip\s*-?[\d.]+\s*(?:pt|em|ex|mm|cm|in|bp|sp)`)
	mathSpacing       = regexp.MustCompile(`\\[,;:!]|\{\}`)
	hspaceCall        = rx(`\\hspace\*?\s*\{[^{}]*\}`)
	counterChange     = rx(`\\(?:addtocounter|setcounter)\s*\{[^{}]*\}\s*\{[^{}]*\}|\\(?:ref)?stepcounter\s*\{[^{}]*\}`)
	accent            = regexp.MustCompile("\\\\([~'`\"^])[ \\t\\n\\v\\f\\r]*(?:\\{([^{}])\\}|([^\\\\{} \\t\\n\\v\\f\\r]))")
	letterAccent      = rx(`\\([vHcukr=.])(?:\{(\p{L})\}|\s+(\p{L}))`)
	escapedCharacter  = regexp.MustCompile(`\\([{}$])`)
	fontDefinition    = rx(`\\font\s*\\([A-Za-z]+)\s*=`)
	register          = rx(`\\[A-Za-z]+\s*=\s*-?[\d.]+(?:pt|em|ex|mm|cm|in|bp|sp)?`)
	unsupportedMacro  = regexp.MustCompile(`\\[A-Za-z@]+|\\.|\$[^$]{0,20}`)
	affiliationCommas = []string{"address", "curraddr"}
)

func buildMathSymbols() map[string]string {
	names := []string{"alpha", "beta", "gamma", "delta", "epsilon", "zeta", "eta", "theta", "iota", "kappa", "lambda", "mu", "nu",
		"xi", "omicron", "pi", "rho", "sigma", "tau", "upsilon", "phi", "chi", "psi", "omega"}
	greek := []rune("αβγδεζηθικλμνξοπρστυφχψω")
	result := map[string]string{}
	for index, name := range names {
		result[name] = string(greek[index])
	}
	for index, name := range names {
		result[strings.ToUpper(name[:1])+name[1:]] = upcase(string(greek[index]))
	}
	for name, symbol := range map[string]string{"times": "×", "otimes": "⊗", "oplus": "⊕", "pm": "±", "cdot": "·", "to": "→",
		"rightarrow": "→", "infty": "∞", "sim": "∼", "approx": "≈", "le": "≤", "ge": "≥", "leq": "≤", "geq": "≥", "ell": "ℓ",
		"varepsilon": "ε", "varphi": "φ"} {
		result[name] = symbol
	}
	return result
}

type macro struct{ name, body string }

// TexMetadata is PaperTexMetadata: a bounded reader for literal front matter; it never
// executes source macros.
type TexMetadata struct {
	source      string
	definitions string
	macros      map[string]string
	clean       string
	arguments   []macro
	argumentsOK bool
	// environments: the paper's own environments without arguments, read on first use.
	environments map[string][2]string
	// styles: the paper's own class and package files, without comments (WithStyles).
	styles string
}

// NewTexMetadata is PaperTexMetadata.new(source).
func NewTexMetadata(source string) *TexMetadata {
	// A source that is not UTF-8 is read as Latin-1, inputenc's usual alternative.
	if !utf8.ValidString(source) {
		runes := make([]rune, len(source))
		for index := 0; index < len(source); index++ {
			runes[index] = rune(source[index])
		}
		source = string(runes)
	}
	t := &TexMetadata{source: source}
	// Macro definitions are read from the whole text; front matter from the text without
	// their bodies (a macro file's \newcommand{\hw}{\title{...}} sets no title here).
	t.definitions = withoutComments(source)
	t.macros = t.textMacros()
	t.clean = t.withoutSwitches(t.withoutDefinitions(t.definitions))
	return t
}

// withoutComments is gsub(/(?<!\\)%[^\n]*/, ”).
func withoutComments(text string) string {
	var builder strings.Builder
	position := 0
	for index := 0; index < len(text); index++ {
		if text[index] != '%' || (index > 0 && text[index-1] == '\\') {
			continue
		}
		builder.WriteString(text[position:index])
		end := strings.IndexByte(text[index:], '\n')
		if end < 0 {
			position = len(text)
			break
		}
		index += end
		position = index
	}
	if position < len(text) {
		builder.WriteString(text[position:])
	}
	return builder.String()
}

// front is text.split('\maketitle', 2).first.
func front(text string) string {
	before, _, _ := strings.Cut(text, `\maketitle`)
	return before
}

// Project is PaperTexMetadata#project: the literal front matter, nil when the paper has none.
func (t *TexMetadata) Project() rec.Map {
	if t.AMS() {
		return t.amsMetadata()
	}
	if t.class() != "acmart" {
		return t.genericMetadata()
	}
	head := front(t.clean)
	titles := []string{}
	for _, row := range t.commands(head, []string{"title"}) {
		titles = append(titles, t.plainTitle(row.argument))
	}
	if len(titles) != 1 {
		rec.Fail("Literal ACM metadata requires one title")
	}
	abstracts := scanFirst(abstractEnv, head)
	if len(abstracts) != 1 {
		rec.Fail("Literal ACM metadata requires one abstract")
	}
	return rec.Map{"title": titles[0], "authors": t.authors(head, []string{"affiliation"}), "abstract": t.plain(abstracts[0], false)}
}

func scanFirst(pattern *regexp.Regexp, text string) []string {
	result := []string{}
	for _, match := range pattern.FindAllStringSubmatch(text, -1) {
		result = append(result, match[1])
	}
	return result
}

// AMS is PaperTexMetadata#ams?.
func (t *TexMetadata) AMS() bool { return t.class() == "amsart" }

// class is the paper's own document class: the first \documentclass. A manual quotes others in
// its verbatim examples (amsldoc shows \documentclass{amsart}).
func (t *TexMetadata) class() string {
	if match := documentClass.FindStringSubmatch(t.clean); match != nil {
		return match[1]
	}
	return ""
}

// AbstractProse is PaperTexMetadata#abstract_prose: an abstract's source prose without its
// math, citations, references and URLs kept as slots.
func (t *TexMetadata) AbstractProse(value string) string {
	value = t.abstractDecorations(value)
	// Displays first: a formula \ensuremath makes may sit beside a $...$ ($X$$(v)$ is two formulas).
	value = ensureMath.ReplaceAllStringFunc(inlineEnvironments(inlineDisplays(value)), func(match string) string {
		return "$" + ensureMath.FindStringSubmatch(match)[1] + "$"
	})
	pieces := splitUnescapedDollar(value)
	valid := len(pieces)%2 == 1
	for index := 1; index < len(pieces); index += 2 {
		valid = valid && pieces[index] != ""
	}
	if !valid {
		rec.Fail("Unsupported or unbalanced AMS abstract math delimiters")
	}
	var builder strings.Builder
	for index := 0; index < len(pieces); index += 2 {
		builder.WriteString(pieces[index])
	}
	// A&A's structured abstract prints each part's heading (\aims{...} reads "Aims. ...").
	prose := replaceCommands(builder.String(), []string{"context", "aims", "methods", "results", "conclusions"},
		func(text string, end int) (int, bool) {
			if !boundary(text, end) {
				return 0, false
			}
			return beforeBrace(text, end)
		}, func(name string) string { return strings.ToUpper(name[:1]) + name[1:] + ". " })
	prose = abstractParagraphs(prose)
	// A citation prints its label ([DJ17]); the prose keeps its place for matching.
	prose = citeCall.ReplaceAllLiteralString(prose, citationSlot)
	prose = referenceCall.ReplaceAllLiteralString(prose, referenceSlot)
	prose = urlCall.ReplaceAllLiteralString(prose, linkSlot)
	return t.plain(prose, false)
}

// splitUnescapedDollar is split(/(?<!\\)\$/, -1).
func splitUnescapedDollar(value string) []string {
	result := []string{}
	start := 0
	for _, index := range unescaped(value, '$') {
		result = append(result, value[start:index])
		start = index + 1
	}
	return append(result, value[start:])
}

// LiteralText is PaperTexMetadata#literal_text.
func (t *TexMetadata) LiteralText(value string) string { return t.plain(value, false) }

// Links is PaperTexMetadata#links: the source's \url and \href declarations.
func (t *TexMetadata) Links() []rec.Map {
	result := []rec.Map{}
	// A \url with no argument is a test of the command (amsldoc's \ifx\UndEfiNed\url), no link.
	for _, row := range t.calledCommands(t.clean, []string{"url", "href"}) {
		href := strip(linkEscape.ReplaceAllString(row.argument, "$1"))
		text := href
		if row.name == "href" {
			inner, _ := t.argument(t.clean, row.end)
			text = t.plain(inner, false)
		}
		result = append(result, rec.Map{"href": href, "text": text})
	}
	return result
}

func (t *TexMetadata) genericMetadata() rec.Map {
	head := front(t.clean)
	body := head
	if match := documentEnv.FindStringSubmatchIndex(head); match != nil {
		body = head[match[1]:]
	}
	// A template's leftover empty \title{} is no title.
	titles := nonEmptyArguments(t, head, "title")
	bodyTitles := nonEmptyArguments(t, body, "title")
	if len(bodyTitles) > 0 {
		titles = bodyTitles
	}
	if len(titles) == 0 {
		return nil
	}
	if len(titles) != 1 {
		rec.Fail("Generic literal metadata requires one title")
	}
	// A preamble \author is overridden by the body's declaration; only the body sets authors.
	authorHead := body
	if len(t.commands(body, []string{"author"})) == 0 && len(t.commands(body, []string{"name"})) == 0 {
		authorHead = head
	}
	names := t.genericNames(authorHead)
	abstracts := t.genericAbstracts()
	authors := t.genericAuthors(authorHead, names)
	if len(authors) == 0 {
		// No \author or \name in effect: a class's own author form (decision 091).
		if declared := t.classAuthors(authorHead); declared != nil {
			authors = declared
		}
	}
	metadata := rec.Map{"title": t.plainTitle(titles[0].argument), "authors": authors}
	// An abstract with inline math or a reference is bound to its observed block, as for AMS
	// papers, with the paper's own text macros expanded: validation reads it without them. A text
	// macro may print math (\newcommand{\transition}{${}^5D_0$}): bound as written math is.
	if len(abstracts) > 0 {
		if bound := t.expandMacros(strip(abstracts[0])); boundAbstract(bound) {
			metadata["abstract_source"] = bound
			return metadata
		}
	}
	if len(abstracts) == 0 {
		return metadata
	}
	if abstract := t.plain(abstractParagraphs(abstracts[0]), false); abstract != "" {
		metadata["abstract"] = abstract
		return metadata
	}
	// Declared but empty (A&A's \abstract{} with the text elsewhere): typed without it, reported.
	metadata["abstract_missing"] = true
	return metadata
}

// nonEmptyArguments are a command's arguments on text, a template's leftover empty one left out.
func nonEmptyArguments(t *TexMetadata, text string, name string) []commandRow {
	titles := []commandRow{}
	for _, row := range t.commands(text, []string{name}) {
		if strip(row.argument) != "" {
			titles = append(titles, row)
		}
	}
	return titles
}

// genericAbstracts: the abstract environment, or the abstract command some classes take
// (A&A, Springer Nature: \abstract{...}); at most one.
func (t *TexMetadata) genericAbstracts() []string {
	abstracts := scanFirst(abstractEnv, t.clean)
	if len(abstracts) == 0 {
		abstracts = plainAbstracts(t.clean)
	}
	if len(abstracts) == 0 {
		for _, row := range t.commands(t.clean, []string{"abstract"}) {
			abstracts = append(abstracts, t.structuredAbstract(row))
		}
	}
	if len(abstracts) > 1 {
		rec.Fail("Generic literal metadata has multiple abstracts")
	}
	// A list in the abstract (REVTeX's PACS numbers) reads as its own blocks, after the prose.
	for index, text := range abstracts {
		abstracts[index] = abstractLists.ReplaceAllLiteralString(text, " ")
	}
	return abstracts
}

// boundAbstract is BOUND_ABSTRACT: an unescaped $, \[ or \(, or a reference, citation or
// \ensuremath command.
func boundAbstract(text string) bool {
	if len(unescaped(text, '$')) > 0 {
		return true
	}
	// A display or parenthesized formula: \[ or \( no backslash precedes.
	for _, index := range unescaped(text, '\\') {
		if index+1 < len(text) && (text[index+1] == '[' || text[index+1] == '(') {
			return true
		}
	}
	for _, location := range boundCommand.FindAllStringIndex(text, -1) {
		if boundary(text, location[1]) {
			return true
		}
	}
	return false
}

func (t *TexMetadata) amsMetadata() rec.Map {
	head := front(t.clean)
	titles := []string{}
	for _, row := range t.commands(head, []string{"title"}) {
		titles = append(titles, t.plainTitle(row.argument))
	}
	abstracts := scanFirst(abstractEnv, t.clean)
	if len(titles) != 1 || len(abstracts) != 1 {
		rec.Fail("Literal AMS metadata requires one title and abstract")
	}
	// amsart's \address and \curraddr follow their \author as ACM's \affiliation does.
	return rec.Map{"title": titles[0], "authors": t.authors(t.clean, affiliationCommas),
		"abstract_source": t.expandMacros(strip(abstracts[0]))}
}

type commandRow struct {
	name, argument string
	// options is the optional [...] argument before the braced one (\affiliation[obeypunctuation=true]).
	options string
	end     int
}

func (t *TexMetadata) commands(text string, names []string) []commandRow {
	rows := []commandRow{}
	for _, match := range command.FindAllStringSubmatchIndex(text, -1) {
		name := text[match[2]:match[3]]
		if !slices.Contains(names, name) {
			continue
		}
		value, ending := t.argument(text, match[1])
		rows = append(rows, commandRow{name: name, argument: value, end: ending, options: optionalArgument(text, match[1])})
	}
	return rows
}

// optionalArgument is the text of the [...] argument at position, after spaces, if one opens there.
func optionalArgument(text string, position int) string {
	position = skipSpace(text, position)
	if position >= len(text) || text[position] != '[' {
		return ""
	}
	options, _, _ := strings.Cut(text[position+1:], "]")
	return options
}

// calledCommands are commands' uses followed by an argument.
func (t *TexMetadata) calledCommands(text string, names []string) []commandRow {
	rows := []commandRow{}
	for _, match := range command.FindAllStringSubmatchIndex(text, -1) {
		name := text[match[2]:match[3]]
		rest := strings.TrimLeft(text[match[1]:], " \t\n")
		if !slices.Contains(names, name) || !(strings.HasPrefix(rest, "{") || strings.HasPrefix(rest, "[")) {
			continue
		}
		value, ending := t.argument(text, match[1])
		rows = append(rows, commandRow{name: name, argument: value, end: ending})
	}
	return rows
}

// structuredAbstract: A&A's \abstract{context}{aims}{methods}{results}{conclusions} prints
// each nonempty part after its heading; any other class's \abstract{...} is its one argument.
func (t *TexMetadata) structuredAbstract(row commandRow) string {
	if t.class() != "aa" {
		return row.argument
	}
	parts := []string{row.argument}
	ending := row.end
	for len(parts) < len(aaAbstract) && opensGroup(t.clean, ending) {
		var value string
		value, ending = t.argument(t.clean, ending)
		parts = append(parts, value)
	}
	if len(parts) != len(aaAbstract) {
		return parts[0]
	}
	kept := []string{}
	for index, text := range parts {
		if strip(text) != "" {
			kept = append(kept, aaAbstract[index]+". "+strip(text))
		}
	}
	return strings.Join(kept, " ")
}

// opensGroup is text[position..].to_s.match?(/\A\s*\{/).
func opensGroup(text string, position int) bool {
	if position > len(text) {
		return false
	}
	position = skipSpace(text, position)
	return position < len(text) && text[position] == '{'
}

// argument reads a command's braced argument at position (after an optional [...]),
// returning it and where it ends.
func (t *TexMetadata) argument(text string, position int) (string, int) {
	position = skipSpace(text, position)
	if position < len(text) && text[position] == '[' {
		ending := strings.IndexByte(text[position:], ']')
		if ending < 0 {
			rec.Fail("Unterminated optional metadata argument")
		}
		position = skipSpace(text, position+ending+1)
	}
	if position >= len(text) || text[position] != '{' {
		rec.Fail("Metadata command requires a literal braced argument")
	}
	opening := position
	depth := 1
	position++
	// A backslash escapes exactly the next character: in \\} the brace closes the group.
	for ; position < len(text); position++ {
		switch text[position] {
		case '\\':
			position++
		case '{':
			depth++
		case '}':
			depth--
		}
		if depth == 0 {
			break
		}
	}
	if depth != 0 {
		rec.Fail("Unterminated literal metadata argument")
	}
	return text[opening+1 : position], position + 1
}

// withoutNotes: \thanks{...} in a title or author is a note, handled as one; it is not name
// text. Marks, spacing and affiliation fields go the same way. In a title a superscript is
// title text (Eu\textsuperscript{3+}), not an author mark. Counter changes
// (\addtocounter{footnote}{-1} before a shared \footnotemark) print nothing.
func (t *TexMetadata) withoutNotes(value string, title bool) string {
	// Primitive glue (\vskip 0.02in, \hskip 1em) prints a space at most.
	text := glueSkip.ReplaceAllLiteralString(counterChange.ReplaceAllLiteralString(t.expandMacros(value), " "), " ")
	for _, name := range dropped {
		if title && name == "textsuperscript" {
			continue
		}
		for {
			start, ending, _, ok := findCommand(text, 0, []string{name}, starSpace)
			if !ok {
				break
			}
			if ending < len(text) && text[ending] == '[' {
				if bracket := strings.IndexByte(text[ending:], ']'); bracket >= 0 {
					ending += bracket + 1
				}
			}
			if ending < len(text) && text[ending] == '{' {
				_, ending = t.argument(text, ending)
			}
			text = text[:start] + " " + text[ending:]
		}
	}
	// A line break (\\ or \\[2pt]) before control symbols, which would split its \\.
	text = lineBreak.ReplaceAllLiteralString(text, " ")
	text = replaceCommands(text, logoNames, letterEnd, func(name string) string { return logos[name] })
	text = hrefColor.ReplaceAllLiteralString(text, "")
	text = replaceCommands(text, spacing, spacingEnd, func(string) string { return " " })
	text = replaceCommands(text, marks, wordEndSpace, func(string) string { return "" })
	text = replaceCommands(text, []string{"xspace"}, wordEnd, func(string) string { return "" })
	text = controlSymbol.ReplaceAllLiteralString(text, " ")
	// A control space (\ followed by whitespace) prints one space; a fragment that ends with
	// the backslash alone (its whitespace was stripped: \\ \ \\ between authors and
	// affiliations) prints one space too.
	if strings.HasSuffix(text, `\`) {
		text = text[:len(text)-1] + " "
	}
	return text
}

// starSpace is \b\*?\s*.
func starSpace(text string, end int) (int, bool) {
	if !boundary(text, end) {
		return 0, false
	}
	if end < len(text) && text[end] == '*' {
		end++
	}
	return skipSpace(text, end), true
}

func (t *TexMetadata) withoutDefinitions(text string) string {
	for {
		location := definition.FindStringIndex(text)
		if location == nil {
			return text
		}
		end := location[1] - 1
		ending := t.argumentEnd(text, end)
		text = text[:location[0]] + " " + text[ending:]
	}
}

// argumentEnd is argument(text, position).last, or position when it fails.
func (t *TexMetadata) argumentEnd(text string, position int) (ending int) {
	defer recoverTo(&ending, position)
	_, ending = t.argument(text, position)
	return ending
}

// recoverTo is a rescue Failure that falls back to value.
func recoverTo(target *int, value int) {
	recovered := recover()
	if recovered == nil {
		return
	}
	if _, ok := recovered.(rec.Failure); !ok {
		panic(recovered)
	}
	*target = value
}

// textMacros: the document's own zero-argument text macros (\newcommand{\method}{Dojo}),
// expanded once; a body has up to two levels of groups (\newcommand{\DTS}{\ensuremath{\mathsf{DTS}}}).
func (t *TexMetadata) textMacros() map[string]string {
	found := map[string]string{}
	for _, match := range textMacro.FindAllStringSubmatch(t.definitions, -1) {
		found[match[1]] = match[2]
	}
	for _, match := range defMacro.FindAllStringSubmatch(t.definitions, -1) {
		if _, ok := found[match[1]]; !ok {
			found[match[1]] = match[2]
		}
	}
	return found
}

// argumentMacros: one-parameter text macros (\def\KR#1{{\color{black}#1}}), expanded once
// with their argument.
func (t *TexMetadata) argumentMacros() []macro {
	found := []macro{}
	seen := map[string]bool{}
	for _, match := range argumentHead.FindAllStringSubmatchIndex(t.definitions, -1) {
		name := ""
		if match[2] >= 0 {
			name = t.definitions[match[2]:match[3]]
		} else {
			name = t.definitions[match[4]:match[5]]
		}
		body, ok := t.tryArgument(t.definitions, match[1]-1)
		if !ok || seen[name] {
			continue
		}
		seen[name] = true
		found = append(found, macro{name: name, body: body})
	}
	result := []macro{}
	for _, item := range found {
		parameters := uniqueStrings(parameter.FindAllString(item.body, -1))
		if len(parameters) == 1 && parameters[0] == "#1" {
			result = append(result, item)
		}
	}
	return result
}

func uniqueStrings(values []string) []string {
	result := []string{}
	for _, value := range values {
		if !slices.Contains(result, value) {
			result = append(result, value)
		}
	}
	return result
}

// tryArgument is argument(text, position) rescuing a Failure.
func (t *TexMetadata) tryArgument(text string, position int) (body string, ok bool) {
	defer recoverFlag(&ok)
	body, _ = t.argument(text, position)
	return body, true
}

func recoverFlag(ok *bool) {
	recovered := recover()
	if recovered == nil {
		return
	}
	if _, isFailure := recovered.(rec.Failure); !isFailure {
		panic(recovered)
	}
	*ok = false
}

// substituteArgument is a macro body with #1 replaced by its argument. A control word before
// #1 ends there (\ntt#1 with amsmath is \ntt, then amsmath): a space keeps them apart.
func substituteArgument(body, inner string) string {
	var builder strings.Builder
	for {
		index := strings.Index(body, "#1")
		if index < 0 {
			builder.WriteString(body)
			return builder.String()
		}
		builder.WriteString(body[:index])
		if controlWordEnd.MatchString(body[:index]) && inner != "" && isASCIILetter(inner[0]) {
			builder.WriteByte(' ')
		}
		builder.WriteString(inner)
		body = body[index+2:]
	}
}

func (t *TexMetadata) expandMacros(value string) string {
	// Text macros before and after the argument macros: \newcommand{\HOL}{\systemname{HOL}}
	// calls one, and one's body may name text macros.
	text := t.expandTextMacros(t.expandEnvironments(value))
	if !t.argumentsOK {
		t.arguments, t.argumentsOK = t.argumentMacros(), true
	}
	for _, item := range t.arguments {
		check := func(text string, end int) (int, bool) {
			if !notLetter(text, end) {
				return 0, false
			}
			return beforeBrace(text, end)
		}
		for {
			start, end, _, ok := findCommand(text, 0, []string{item.name}, check)
			if !ok {
				break
			}
			inner, ending := t.argument(text, end)
			text = text[:start] + substituteArgument(item.body, inner) + text[ending:]
		}
	}
	return t.withoutSwitches(t.expandTextMacros(text))
}

// expandTextMacros is gsub(/\\([A-Za-z]+)\b(?:\{\})?/) { macros.fetch(name, match) }.
func (t *TexMetadata) expandTextMacros(text string) string {
	var builder strings.Builder
	position := 0
	for index := 0; index < len(text); index++ {
		if text[index] != '\\' {
			continue
		}
		end := index + 1
		for end < len(text) && isASCIILetter(text[end]) {
			end++
		}
		if end == index+1 || !boundary(text, end) {
			continue
		}
		if strings.HasPrefix(text[end:], "{}") {
			end += 2
		}
		builder.WriteString(text[position:index])
		name := strings.TrimSuffix(text[index+1:end], "{}")
		if body, ok := t.macros[name]; ok {
			builder.WriteString(body)
		} else {
			builder.WriteString(text[index:end])
		}
		position = end
		index = end - 1
	}
	builder.WriteString(text[position:])
	return builder.String()
}

// caseChanged: \MakeLowercase{...} and \MakeUppercase{...} print their argument in that case.
func (t *TexMetadata) caseChanged(text string) string {
	check := func(text string, end int) (int, bool) {
		if !strings.HasSuffix(text[:end], "case") {
			return 0, false
		}
		return beforeBrace(text, end)
	}
	for {
		start, end, name, ok := findCommand(text, 0, []string{"MakeLowercase", "MakeUppercase"}, check)
		if !ok {
			return text
		}
		value, ending := t.argument(text, end)
		if name == "MakeLowercase" {
			value = downcase(value)
		} else {
			value = upcase(value)
		}
		text = text[:start] + value + text[ending:]
	}
}
