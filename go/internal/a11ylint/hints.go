package a11ylint

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sarielhp/relatex/internal/authorhints"
	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/mathintent"
	"github.com/sarielhp/relatex/internal/paperdoc"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/tablehints"
)

// ambiguous are the operators whose common reading differs between papers: the lint proposes a
// speech hint wherever the paper defines one of them itself (decision 122).
var ambiguous = []string{"Im", "Re", "arg", "deg", "tr"}

// hintItems reads the author's hints (decision 122) and checks them: an unknown section, a speech
// hint for a macro the paper neither defines nor uses, a key two hints disagree on, and an operator
// with an ambiguous reading the author could name.
func hintItems(paper Paper) []Item {
	if len(paper.Files) == 0 {
		return nil
	}
	hints, err := readHints(paper)
	if err != nil {
		return []Item{{Kind: "hint", File: paper.Files[0].Path, Message: "relatex/hints.json could not be read: " + err.Error(),
			Fix: "correct relatex/hints.json; its sections are speech and alt (objects of strings) and tables (objects of header_rows and header_columns counts)"}}
	}
	if hints == nil {
		return nil
	}
	items := []Item{}
	for _, unknown := range hints.Unknown {
		items = append(items, Item{Kind: "hint", File: unknown.File, Line: unknown.Line,
			Message: fmt.Sprintf("unknown hint section %q", unknown.Section), Fix: "use the speech, alt or tables section (decisions 122, 131)"})
	}
	for _, conflict := range hints.Conflicts {
		items = append(items, Item{Kind: "hint", File: conflict.Winner.File, Line: conflict.Winner.Line,
			Message: fmt.Sprintf("two hints for %s %q say %q and %q; %q wins", conflict.Section, conflict.Key,
				conflict.Loser.Value, conflict.Winner.Value, conflict.Winner.Value),
			Fix: "remove one of them, or make them agree"})
	}
	for key, hint := range hints.Sections["alt"] {
		if !labelDefined(paper.Files, key) {
			items = append(items, Item{Kind: "hint", File: hint.File, Line: hint.Line,
				Message: fmt.Sprintf("alt hint for %q, a label the paper does not define", key),
				Fix:     "name the \\label of a figure, or remove the hint"})
		}
	}
	for key, hint := range hints.Sections["speech"] {
		if _, ok := mathintent.IntentName(hint.Value); !ok {
			items = append(items, Item{Kind: "hint", File: hint.File, Line: hint.Line,
				Message: fmt.Sprintf("speech hint for %q says %q, which cannot name a MathML intent, so it is not applied", key, hint.Value),
				Fix:     "write words of letters, digits, hyphens and spaces, starting with a letter"})
		}
		if !speechHintUsed(paper.Files, key) {
			items = append(items, Item{Kind: "hint", File: hint.File, Line: hint.Line,
				Message: fmt.Sprintf("speech hint for %q, which the paper neither defines nor uses", key),
				Fix:     "write the macro's name without its backslash, or remove the hint"})
		}
	}
	items = append(items, tableHintItems(paper, hints)...)
	items = append(items, hintedOperatorItems(paper, hints)...)
	return items
}

// tableNumber is a key that can be a table's caption number (3, A.1, S2, IV).
var tableNumber = regexp.MustCompile(`^(?:[A-Z]?[0-9]+(?:\.[0-9]+)*|[A-Z]\.[0-9]+(?:\.[0-9]+)*|[IVXLC]+)$`)

// tableHintItems: a table hint the conversion cannot read, or one keyed by neither a label the paper
// defines nor a caption number (decision 131).
func tableHintItems(paper Paper, hints *authorhints.Hints) []Item {
	items := []Item{}
	for key, hint := range hints.Sections[authorhints.TablesSection] {
		if _, err := authorhints.ParseTableHint(hint.Value); err != nil {
			items = append(items, Item{Kind: "hint", File: hint.File, Line: hint.Line,
				Message: fmt.Sprintf("table hint for %q cannot be read (%v), so it is not applied", key, err),
				Fix:     `write "header_rows=N header_columns=N" (either may be left out)`})
		}
		if base, _ := tablehints.Position(key); !tableNumber.MatchString(base) && !labelDefined(paper.Files, base) {
			items = append(items, Item{Kind: "hint", File: hint.File, Line: hint.Line,
				Message: fmt.Sprintf("table hint for %q, neither a label the paper defines nor a table number", key),
				Fix: "name the \\label of a table or the number its caption prints (for a table inside a figure, add its position: " +
					"fig:label#1), or remove the hint"})
		}
	}
	return items
}

// readHints is authorhints.Read over the files the lint already loaded.
func readHints(paper Paper) (*authorhints.Hints, error) {
	sources := make([]authorhints.Source, 0, len(paper.Files))
	for _, file := range paper.Files {
		sources = append(sources, authorhints.Source{Path: file.Path, Text: file.Source})
	}
	var data []byte
	if paper.Directory != "" {
		var err error
		data, err = os.ReadFile(filepath.Join(paper.Directory, authorhints.FileName))
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}
	bytes, err := os.ReadFile(filepath.Join(paperdoc.Root, "schemas", "relatex-hints.schema.json"))
	if err != nil {
		return nil, nil
	}
	value, err := canonjson.Decode(strings.NewReader(string(bytes)))
	if err != nil {
		return nil, err
	}
	return authorhints.Parse(sources, data, rec.AsMap(value))
}

// labelDefined: the paper labels something key (\label{key}); comments are blanked in Text, so a
// commented-out label does not count.
func labelDefined(files []*File, key string) bool {
	label := regexp.MustCompile(`\\label\s*\{\s*` + regexp.QuoteMeta(key) + `\s*\}`)
	for _, file := range files {
		if label.MatchString(file.Text) {
			return true
		}
	}
	return false
}

// speechHintUsed: the paper defines or uses the macro a speech hint names, or writes the name the
// hint speaks for (\mathrm{Im} is the operator a hint names Im).
func speechHintUsed(files []*File, key string) bool {
	command := regexp.MustCompile(`\\` + regexp.QuoteMeta(key) + `\b`)
	name := regexp.MustCompile(`\b` + regexp.QuoteMeta(key) + `\b`)
	for _, file := range files {
		if command.MatchString(file.Text) || name.MatchString(file.Text) {
			return true
		}
	}
	return false
}

// hintedOperatorItems proposes a speech hint for an ambiguous operator the paper defines itself.
func hintedOperatorItems(paper Paper, hints *authorhints.Hints) []Item {
	items := []Item{}
	for _, name := range ambiguous {
		if _, ok := hints.Get("speech", name); ok {
			continue
		}
		for _, file := range paper.Files {
			line := operatorDefinition(file.Text, name)
			if line == 0 {
				continue
			}
			items = append(items, Item{Kind: "hint", File: file.Path, Line: line,
				Message:    fmt.Sprintf(`\%s has more than one common reading; the author can name it`, name),
				Fix:        fmt.Sprintf(`add %s relatex speech: \%s = "…"`, "%%", name),
				Suggestion: fmt.Sprintf(`%s relatex speech: \%s = "…"`, "%%", name), NoteLine: line, Place: PlaceAfter,
				Note: "after the definition, so the reading travels with the paper (decision 122)"})
			break
		}
	}
	return items
}

// setSymbol is a definition body that is one bold or double-struck letter (\mathbb{R}): a set's
// symbol, read as such, never the operator its name abbreviates.
var setSymbol = regexp.MustCompile(`\A\s*\\(?:mathbb|mathbbm|mathds|varmathbb|mathbf|bm|boldsymbol)\s*\{\s*[A-Za-z]\s*\}\s*\z`)

// typedName is a definition body that is one text command around letters: \operatorname{Im}, \textup{Im}.
var typedName = regexp.MustCompile(`\A\s*\\(?:operatorname\*?|textup|textrm|text|mathrm|mathsf|textsf)\s*\{\s*([A-Za-z]+)\s*\}\s*\z`)

// typesItsName: the body of the definition of the operator name is the name itself, typed in a
// text command (\renewcommand{\Im}{\operatorname{Im}}, \newcommand{\tr}{\text{Tr}}): the paper has
// spelled the reading the hint would name, so the definition is no gap (decision 167 amendment).
func typesItsName(body, name string) bool {
	match := typedName.FindStringSubmatch(body)
	return match != nil && strings.EqualFold(match[1], name)
}

// operatorDefinition is the line defining the operator in text, 0 when it does not define it. A
// definition as a set's symbol (\renewcommand{\Re}{\mathbb{R}}) has one reading and is skipped
// (decision 167).
func operatorDefinition(text, name string) int {
	definition := regexp.MustCompile(`\\(?:newcommand|renewcommand|providecommand|DeclareRobustCommand|DeclareMathOperator)\*?\s*\{?\s*\\` +
		name + `\b\s*\}?`)
	for _, match := range definition.FindAllStringIndex(text, -1) {
		after := match[1]
		for range 2 { // the argument count and the optional default
			if _, end, ok := argument(text, after, '[', ']'); ok {
				after = end
			}
		}
		if body, _, ok := argument(text, after, '{', '}'); ok && (setSymbol.MatchString(body) || typesItsName(body, name)) &&
			!strings.Contains(text[match[0]:after], "DeclareMathOperator") {
			continue
		}
		return lineAt(text, match[0])
	}
	return 0
}

// lineAt is the 1-based line of offset in text.
func lineAt(text string, offset int) int {
	return strings.Count(text[:offset], "\n") + 1
}
