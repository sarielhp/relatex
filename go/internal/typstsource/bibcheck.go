package typstsource

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/rec"
)

// The check of a styled bibliography (decision 172, the plan's decision C): typst evaluates a document
// that prints the bibliography alone, each entry's cells read back as runs of text, and every entry
// must equal the model's, as tools/bibstyle_compare compares them: the same order, the same text (NFC,
// whitespace collapsed, ligatures expanded, dashes and quotes kept, the label included) and the same
// italic and bold runs. A formula compares by its characters.

// checkTimeout bounds the check's typst run (a book's bibliography has a thousand entries).
const checkTimeout = 300 * time.Second

// checkDocument prints the bibliography alone, as main.typ does; it lives in support/ beside bibliography.typ.
const checkDocument = `#import "` + bibliographyFile + `": pergamon, ltd-bib-style, ltd-bibliography
#pergamon.add-bib-resource(read("../` + referencesFile + `"))
#show: pergamon.refsection.with(style: ltd-bib-style)
#ltd-bibliography()
`

// checkQuery reads the entries back, in the order printed.
const checkQuery = "query(<ltd-bibcheck>).map(it => it.value)"

// Markers in the runs bibliography.typ writes.
const (
	doubleQuote = ''
	singleQuote = ''
	mathStart   = ''
	mathEnd     = ''
)

// printedEntry is one entry as Pergamon printed it: its key and each cell's runs.
type printedEntry struct {
	Key   string     `json:"key"`
	Cells [][][3]any `json:"cells"`
}

// segments are the entry's cells as text segments, a space between cells.
func (p printedEntry) segments() []segment {
	result := []segment{}
	for index, cell := range p.Cells {
		if index > 0 {
			result = append(result, segment{text: " "})
		}
		for _, run := range cell {
			text, _ := run[0].(string)
			italic, _ := run[1].(bool)
			bold, _ := run[2].(bool)
			result = append(result, splitMath(text, italic, bold)...)
		}
	}
	return result
}

// placeholders are the formula indexes the entry printed, in order.
func (p printedEntry) placeholders() []int {
	indexes := []int{}
	for _, part := range p.segments() {
		for _, char := range part.text {
			if char >= placeholderBase && char < placeholderBase+0x10000 {
				indexes = append(indexes, int(char-placeholderBase))
			}
		}
	}
	return indexes
}

func hasPlaceholders(printed []printedEntry) bool {
	return slices.ContainsFunc(printed, func(entry printedEntry) bool { return len(entry.placeholders()) > 0 })
}

// runCheck writes the check document, evaluates it offline and reads the printed entries; the reason
// it failed otherwise.
func runCheck(typst, directory string) ([]printedEntry, string) {
	document := filepath.Join(directory, SupportDirectory, checkFile)
	writeFile(document, checkDocument)
	defer os.Remove(document)
	empty, err := os.MkdirTemp("", "relatex-typst-packages-")
	if err != nil {
		rec.Fail("%v", err)
	}
	defer os.RemoveAll(empty)
	ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
	defer cancel()
	// No package directory and no network: the export must compile from its own files.
	cmd := exec.CommandContext(ctx, typst, "eval", "--root", directory, "--ignore-system-fonts",
		"--package-path", empty, "--package-cache-path", empty, "--input", "ltd-bibcheck=1", "--in", SupportDirectory+"/"+checkFile, checkQuery)
	cmd.Dir = directory
	cmd.Env = command.Environment(map[string]string{"HTTP_PROXY": offline, "HTTPS_PROXY": offline,
		"http_proxy": offline, "https_proxy": offline, "ALL_PROXY": offline})
	var out, errors bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errors
	if runErr := cmd.Run(); runErr != nil {
		return nil, "typst: " + firstLines(errors.String(), runErr)
	}
	printed := []printedEntry{}
	if err := json.Unmarshal(out.Bytes(), &printed); err != nil {
		return nil, "typst printed no entry list: " + err.Error()
	}
	return printed, ""
}

// offline is a proxy nothing listens on.
const offline = "http://127.0.0.1:9"

func firstLines(text string, err error) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return err.Error()
	}
	return strings.Join(lines[:min(len(lines), 3)], " | ")
}

// ---- texts ----

// segment is text with its style; a formula is one segment of its folded characters.
type segment struct {
	text         string
	italic, bold bool
	math         bool
}

// splitMath cuts a run's formulas (U+E002 ... U+E003) out as math segments.
func splitMath(text string, italic, bold bool) []segment {
	result := []segment{}
	for text != "" {
		start := strings.IndexRune(text, mathStart)
		if start < 0 {
			return append(result, segment{text: text, italic: italic, bold: bold})
		}
		if start > 0 {
			result = append(result, segment{text: text[:start], italic: italic, bold: bold})
		}
		rest := text[start+len(string(mathStart)):]
		end := strings.IndexRune(rest, mathEnd)
		if end < 0 {
			end = len(rest)
		}
		result = append(result, segment{text: foldMath(rest[:end]), math: true})
		text = strings.TrimPrefix(rest[end:], string(mathEnd))
	}
	return result
}

// foldMath is a formula's characters, compatibility-decomposed (math italic 𝑘 is k), without spaces
// or invisible operators, the minus sign as a hyphen, sorted: the writer substitutes the model's own
// formulas, so the comparison only has to tell them apart.
func foldMath(text string) string {
	runes := []rune{}
	for _, char := range norm.NFKC.String(text) {
		switch {
		case unicode.IsSpace(char), char >= '⁡' && char <= '⁤':
		case char == '−':
			runes = append(runes, '-')
		default:
			runes = append(runes, char)
		}
	}
	slices.Sort(runes)
	return string(runes)
}

// modelTexts are the entries as the model holds them: their spans, a formula by its MathML's text, an
// address url.sty set in math mode by the characters it shows (urlFormula).
func (e *exporter) modelTexts(entries []bibEntry) []modelText {
	texts := make([]modelText, len(entries))
	for index, entry := range entries {
		segments := []segment{}
		for _, span := range content(entry.node) {
			if shown, isURL := e.urlFormula(span); isURL {
				segments = append(segments, segment{text: shown})
				continue
			}
			if span["type"] == "math" {
				segments = append(segments, segment{text: foldMath(e.mathText(span)), math: true})
				continue
			}
			style, _ := span["style"].(rec.Map)
			shape, series := toS(dig(style, "shape")), toS(dig(style, "series"))
			segments = append(segments, segment{text: toS(span["text"]), italic: shape == "it" || shape == "sl",
				bold: strings.Contains(series, "b")})
		}
		texts[index] = modelText{key: entry.key, bibKey: entry.bibKey, text: normalize(segments)}
	}
	return texts
}

type modelText struct {
	key, bibKey string
	text        []styledRune
}

// styledRune is a character of an entry's normalized text.
type styledRune struct {
	char         rune
	italic, bold bool
}

// normalize is the entry text the harness compares (BibEntryText): ligatures expanded, soft hyphens and
// zero-width characters dropped, NFC, whitespace (no-break spaces too) collapsed to one space, trimmed;
// a quote marker becomes the quote Typst prints (opening after a space or an opening bracket).
func normalize(segments []segment) []styledRune {
	result := []styledRune{}
	for _, part := range segments {
		text := part.text
		if !part.math {
			text = norm.NFC.String(expandLigatures(text))
		}
		for _, char := range text {
			result = appendRune(result, char, part)
		}
	}
	for len(result) > 0 && result[len(result)-1].char == ' ' {
		result = result[:len(result)-1]
	}
	return result
}

func appendRune(result []styledRune, char rune, part segment) []styledRune {
	styled := !part.math
	switch {
	case char == '­' || char == '​' || char == '‌' || char == '‍' || char == '⁠' || char == '\uFEFF':
		return result
	case unicode.IsSpace(char) || char == ' ':
		if len(result) == 0 || result[len(result)-1].char == ' ' {
			return result
		}
		char = ' '
	case char == doubleQuote || char == singleQuote:
		char = resolveQuote(result, char == doubleQuote)
	}
	return append(result, styledRune{char: char, italic: styled && part.italic, bold: styled && part.bold})
}

// resolveQuote is Typst's smart quote: opening at the start, after a space or an opening bracket or
// quote; closing otherwise.
func resolveQuote(before []styledRune, double bool) rune {
	opening := len(before) == 0 || strings.ContainsRune(" ([{“‘", before[len(before)-1].char)
	switch {
	case double && opening:
		return '“'
	case double:
		return '”'
	case opening:
		return '‘'
	}
	return '’'
}

var ligatures = strings.NewReplacer("ﬀ", "ff", "ﬁ", "fi", "ﬂ", "fl", "ﬃ", "ffi", "ﬄ", "ffl", "ﬅ", "st", "ﬆ", "st")

func expandLigatures(text string) string { return ligatures.Replace(text) }

func plainText(runes []styledRune) string {
	var builder strings.Builder
	for _, char := range runes {
		builder.WriteRune(char.char)
	}
	return builder.String()
}

// runs are the maximal runs of characters with the style; a space between two styled characters stays
// in the run.
func runs(runes []styledRune, styled func(styledRune) bool) []string {
	result := []string{}
	var current []rune
	pending := 0
	for _, char := range runes {
		switch {
		case char.char == ' ':
			if len(current) > 0 {
				current = append(current, ' ')
			}
		case styled(char):
			current = append(current, char.char)
			pending = len(current)
		default:
			if pending > 0 {
				result = append(result, string(current[:pending]))
			}
			current, pending = nil, 0
		}
	}
	if pending > 0 {
		result = append(result, string(current[:pending]))
	}
	return result
}

func italic(char styledRune) bool { return char.italic }
func bold(char styledRune) bool   { return char.bold }

// ---- the comparison ----

// compareEntries: the printed entries against the model's, in order; the reason they differ ("" when
// they do not) and the first differing entries.
func compareEntries(model []modelText, printed []printedEntry) (string, []any) {
	if len(printed) != len(model) {
		return fmt.Sprintf("Pergamon printed %d entries, the paper %d", len(printed), len(model)), nil
	}
	differing := []any{}
	count := 0
	for index, want := range model {
		got := printed[index]
		difference := compareEntry(want, got)
		if difference == nil {
			continue
		}
		count++
		if len(differing) < maxReported {
			differing = append(differing, difference)
		}
	}
	if count == 0 {
		return "", nil
	}
	return fmt.Sprintf("%d of %d entries differ from the paper's", count, len(model)), differing
}

// compareEntry is the first difference of one entry (order, text, italic, bold), nil when equal.
func compareEntry(want modelText, got printedEntry) rec.Map {
	if !strings.EqualFold(got.Key, want.bibKey) {
		return rec.Map{"key": want.key, "field": "order", "latex": want.key, "typst": got.Key}
	}
	text := normalize(got.segments())
	if latex, typst := plainText(want.text), plainText(text); latex != typst {
		before, after := differingSpan(latex, typst)
		return rec.Map{"key": want.key, "field": "text", "latex": before, "typst": after}
	}
	for _, field := range []struct {
		name  string
		style func(styledRune) bool
	}{{"italic", italic}, {"bold", bold}} {
		latex, typst := runs(want.text, field.style), runs(text, field.style)
		if !slices.Equal(latex, typst) {
			return rec.Map{"key": want.key, "field": field.name, "latex": strings.Join(latex, " | "),
				"typst": strings.Join(typst, " | ")}
		}
	}
	return nil
}

// differingSpan: both texts from 20 characters before their first difference, 50 characters long.
func differingSpan(latex, typst string) (string, string) {
	a, b := []rune(latex), []rune(typst)
	at := 0
	for at < len(a) && at < len(b) && a[at] == b[at] {
		at++
	}
	start := max(0, at-20)
	return string(a[start:min(len(a), at+50)]), string(b[start:min(len(b), at+50)])
}

func fileSHA256(path string) any {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
