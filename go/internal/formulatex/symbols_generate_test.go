package formulatex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/authormath"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/texsandbox"
	"github.com/sarielhp/relatex/internal/xmltree"
)

// asciiCandidates are the characters a formula may hold as themselves.
const asciiCandidates = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789+-=<>()[]/|*,.;:!?'@\"`"

// escapedCandidates are TeX's special characters as math writes them.
var escapedCandidates = []string{`\{`, `\}`, `\|`, `\%`, `\#`, `\&`, `\$`, `\_`, `\backslash`}

// TestGenerateSymbols writes symbols.go and composites.go (RELATEX_GENERATE_SYMBOLS=1): every
// command KaTeX knows (profiles/katex-support.json), each negated by \not, and every printable ASCII
// character, typeset alone with the round trip's preamble. Those that typeset as one token element
// give the table from that element back to the TeX; the shortest TeX wins, so "x" beats a command
// and \le beats \leq. Those that typeset as several elements (\longrightarrow, \notin) and take no
// argument give the composites, by their elements' signature.
func TestGenerateSymbols(t *testing.T) {
	if os.Getenv("RELATEX_GENERATE_SYMBOLS") == "" {
		t.Skip("RELATEX_GENERATE_SYMBOLS not set")
	}
	root, _ := filepath.Abs("../../..")
	authormath.Root = root
	candidates := symbolCandidates(t, root)
	results := typesetEach(t, candidates, 1)
	table := map[symbolKey]string{}
	compound := []string{}
	for index, list := range results {
		if len(list) == 0 {
			continue
		}
		key, ok := singleToken(list[len(list)-1])
		if !ok {
			// \not before anything but a relation is two elements, not a composite.
			if !strings.HasPrefix(candidates[index], `\not`) {
				compound = append(compound, candidates[index])
			}
			continue
		}
		if len(list) != 1 {
			continue
		}
		if known, seen := table[key]; seen && !shorter(candidates[index], known) {
			continue
		}
		table[key] = candidates[index]
	}
	// \not alone is the slash TeX lays over the relation after it (\not\ni).
	table[symbolKey{"mo", "\u0338", ""}] = `\not`
	writeSymbols(t, table)
	writeComposites(t, compositeTable(t, compound))
}

// compositeTable is each composite's signatures: alone, after an x, before a y and between them
// (with the spaces TeX puts around it), the shortest TeX first. A command before a y that
// typesets otherwise with an empty group between them takes an argument and is left out; so are
// spaces alone (the serializer writes spaces itself) and empty groups (an infix \atop).
func compositeTable(t *testing.T, compound []string) map[string]string {
	texts := []string{}
	for _, text := range compound {
		texts = append(texts, text, text+"{} y", "x"+text, text+" y", "x"+text+" y")
	}
	results := typesetEach(t, texts, 0)
	table := map[string]string{}
	for index := 0; index < len(texts); index += 5 {
		before, grouped := lastFormula(results[index+3]), lastFormula(results[index+1])
		if before == "" || grouped == "" || authormath.Difference(before, grouped) != "" {
			continue
		}
		for offset, trim := range [][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
			elements, ok := trimmed(lastFormula(results[index+[]int{0, 2, 3, 4}[offset]]), trim[0], trim[1])
			if !ok || allSpaces(elements) || writtenAlready(elements) {
				continue
			}
			key := signature(elements)
			if strings.Contains(key, "mrow:;") {
				continue
			}
			// A glyph without a Unicode mapping comes out as a control character or ^^ notation.
			control := strings.ContainsFunc(key, func(character rune) bool { return character < 0x20 })
			if control || strings.Contains(key, "^^") {
				continue
			}
			if known, seen := table[key]; !seen || shorter(compound[index/5], known) {
				table[key] = compound[index/5]
			}
		}
	}
	return table
}

// writtenAlready: the serializer writes these elements itself, settling their spaces by their
// neighbors (operator names, letters, dots), so they are no composite.
func writtenAlready(elements []*xmltree.Node) bool {
	for _, node := range elements {
		_, dots := dotsCommand(node)
		switch {
		case isGlue(node), isWord(node), dots:
		case node.Name == "mi" && node.Attrs["mathvariant"] == "normal" && isPlain(node.Content()):
		case node.Name == "mo" && node.Content() == ".":
		default:
			return false
		}
	}
	return true
}

func lastFormula(list []string) string {
	if len(list) == 0 {
		return ""
	}
	return list[len(list)-1]
}

// trimmed is a formula's elements without the x before it and the y after it.
func trimmed(mathml string, before, after int) ([]*xmltree.Node, bool) {
	root, err := xmltree.Parse(mathml)
	if err != nil {
		return nil, false
	}
	elements := root.Elements()
	if len(elements) < before+after {
		return nil, false
	}
	if before == 1 && elements[0].Content() != "𝑥" || after == 1 && elements[len(elements)-1].Content() != "𝑦" {
		return nil, false
	}
	return elements[before : len(elements)-after], true
}

// typesetEach typesets each text alone with the round trip's preamble: its formulas' MathML.
// In chunks: a command that stops TeX or swallows the files after it (\iffalse) costs its chunk
// only; a text without want formulas (any, for 0) is tried again in small chunks.
func typesetEach(t *testing.T, texts []string, want int) [][]string {
	directory := t.TempDir()
	ids := make([]string, len(texts))
	for index, text := range texts {
		ids[index] = authormath.CandidateID(index)
		source := candidateSource(Formula{Text: text})
		if err := os.WriteFile(filepath.Join(directory, ids[index]+".tex"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	typesetter := &authormath.Typesetter{Directory: directory, Header: Preamble,
		Doctor: texsandbox.Current(command.TeXLiveBin("lualatex")), Attempt: "formula-tex"}
	results := map[string]any{}
	for _, size := range []int{40, 4} {
		missing := []string{}
		for _, id := range ids {
			if list, _ := results[id].([]any); len(list) == 0 || want > 0 && len(list) != want {
				missing = append(missing, id)
			}
		}
		for start := 0; start < len(missing); start += size {
			for id, value := range typesetter.Typeset(missing[start:min(start+size, len(missing))]) {
				results[id] = value
			}
		}
	}
	lists := make([][]string, len(texts))
	for index, id := range ids {
		list, _ := results[id].([]any)
		for _, value := range list {
			lists[index] = append(lists[index], rec.AsString(value))
		}
	}
	return lists
}

func writeComposites(t *testing.T, table map[string]string) {
	keys := make([]string, 0, len(table))
	for key := range table {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var builder strings.Builder
	builder.WriteString("// Code generated by TestGenerateSymbols (RELATEX_GENERATE_SYMBOLS=1); DO NOT EDIT.\n\npackage formulatex\n\n")
	builder.WriteString("// composites maps the signature of a run of elements to the TeX command that typesets it.\n")
	builder.WriteString("var composites = map[string]string{\n")
	for _, key := range keys {
		fmt.Fprintf(&builder, "\t%q: %q,\n", key, table[key])
	}
	builder.WriteString("}\n")
	if err := os.WriteFile("composites.go", []byte(builder.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func symbolCandidates(t *testing.T, root string) []string {
	data, err := os.ReadFile(filepath.Join(root, "profiles", "katex-support.json"))
	if err != nil {
		t.Fatal(err)
	}
	var support struct {
		Commands []string `json:"commands"`
	}
	if err := json.Unmarshal(data, &support); err != nil {
		t.Fatal(err)
	}
	candidates := []string{}
	for _, character := range asciiCandidates {
		candidates = append(candidates, string(character))
	}
	candidates = append(candidates, escapedCandidates...)
	for _, character := range "=<>|" {
		candidates = append(candidates, `\not`+string(character))
	}
	for _, name := range support.Commands {
		// \char and \not alone typeset a character their missing argument or base leaves.
		if name == "char" || name == "not" {
			continue
		}
		if len(name) > 1 && strings.Trim(name, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") == "" {
			candidates = append(candidates, `\`+name, `\not\`+name)
		}
	}
	return candidates
}

func shorter(candidate, known string) bool {
	if len(candidate) != len(known) {
		return len(candidate) < len(known)
	}
	return candidate < known
}

// singleToken is the key of a formula made of one token element.
func singleToken(mathml string) (symbolKey, bool) {
	root, err := xmltree.Parse(mathml)
	if err != nil {
		return symbolKey{}, false
	}
	children := root.Elements()
	if len(children) != 1 || len(children[0].Elements()) > 0 {
		return symbolKey{}, false
	}
	element := children[0]
	if element.Name != "mi" && element.Name != "mo" && element.Name != "mn" || element.Content() == "" {
		return symbolKey{}, false
	}
	// A glyph without a Unicode mapping comes out as a control character or ^^ notation.
	if strings.ContainsFunc(element.Content(), func(character rune) bool { return character < ' ' }) ||
		strings.Contains(element.Content(), "^^") {
		return symbolKey{}, false
	}
	return symbolKey{element.Name, element.Content(), element.Attrs["mathvariant"]}, true
}

func writeSymbols(t *testing.T, table map[symbolKey]string) {
	keys := make([]symbolKey, 0, len(table))
	for key := range table {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.element != b.element {
			return a.element < b.element
		}
		if a.text != b.text {
			return a.text < b.text
		}
		return a.variant < b.variant
	})
	var builder strings.Builder
	builder.WriteString("// Code generated by TestGenerateSymbols (RELATEX_GENERATE_SYMBOLS=1); DO NOT EDIT.\n\npackage formulatex\n\n")
	builder.WriteString("// symbols maps a token element (name, text, mathvariant) to the TeX that typesets it alone.\n")
	builder.WriteString("var symbols = map[symbolKey]string{\n")
	for _, key := range keys {
		fmt.Fprintf(&builder, "\t{%q, %q, %q}: %q,\n", key.element, key.text, key.variant, table[key])
	}
	builder.WriteString("}\n")
	if err := os.WriteFile("symbols.go", []byte(builder.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}
