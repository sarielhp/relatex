package typstsource

import (
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"github.com/sarielhp/relatex/internal/rec"
)

// A listing, a rule and its tag (books/0001): the rule writes nothing, so the tag's "(1)"
// follows the #raw call directly and must not become its arguments.
func TestCallBeforeParenthesisAcrossAnEmptyRunIsEnded(t *testing.T) {
	mono := rec.Map{"generic": "monospace"}
	spans := []rec.Map{
		{"type": "text", "text": ` \begin{equation} a=b \end{equation}`, "style": mono},
		{"type": "rule"},
		{"type": "text", "text": "(1)"},
	}
	got := (&exporter{}).inline(spans, rec.Map{"id": "p"}, 0, false)
	if want := ` #raw("\\begin{equation} a=b \\end{equation}");(1)`; got != want {
		t.Errorf("inline = %s, want %s", got, want)
	}
	spans = []rec.Map{
		{"type": "text", "text": "word", "style": rec.Map{"shape": "it"}},
		{"type": "rule"},
		{"type": "rule"},
		{"type": "text", "text": ".Then"},
	}
	if got, want := (&exporter{}).inline(spans, rec.Map{"id": "p"}, 0, false), `#emph[word];.Then`; got != want {
		t.Errorf("inline = %s, want %s", got, want)
	}
}

// fuzzAlphabet: letters, spaces and every character Typst markup gives a meaning, with the
// openers that continue an embedded call.
const fuzzAlphabet = "abcxyz  0123456789#$*_@<>\\`[]~\"'/=+-.:;()"

// TestEscapingFuzz is decision 063's escaping fuzz: random runs (plain, italic, bold, monospace,
// with rules between them) go through the writer's inline markup, are compiled with Typst, and
// the extracted text must equal the input (whitespace aside).
func TestEscapingFuzz(t *testing.T) {
	typst, errTypst := exec.LookPath("typst")
	pdftotext, errText := exec.LookPath("pdftotext")
	if errTypst != nil || errText != nil {
		t.Skip("typst or pdftotext is not installed")
	}
	random := rand.New(rand.NewPCG(63, 154))
	cases := make([][]rec.Map, 400)
	var source strings.Builder
	source.WriteString("#set page(width: 40cm, height: auto)\n\n")
	for index := range cases {
		cases[index] = fuzzSpans(random)
		fmt.Fprintf(&source, "ZQZ%d\n\nx %s QE\n\n", index, (&exporter{}).inline(cases[index], rec.Map{"id": "p"}, 0, false))
	}
	source.WriteString("ZQZend\n")
	directory := t.TempDir()
	main := filepath.Join(directory, "main.typ")
	if err := os.WriteFile(main, []byte(source.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(typst, "compile", "--ignore-system-fonts", main).CombinedOutput(); err != nil {
		t.Fatalf("typst compile failed: %v\n%s\n%s", err, output, source.String())
	}
	text, err := exec.Command(pdftotext, "-q", filepath.Join(directory, "main.pdf"), "-").Output()
	if err != nil {
		t.Fatal(err)
	}
	extracted := strings.Split(withoutSpace(string(text)), "ZQZ")
	for index, spans := range cases {
		want := fmt.Sprintf("%dx%sQE", index, withoutSpace(fuzzText(spans)))
		if index+1 >= len(extracted) || extracted[index+1] != want {
			got := ""
			if index+1 < len(extracted) {
				got = extracted[index+1]
			}
			t.Errorf("case %d: text %q, want %q from %q", index, got, want, (&exporter{}).inline(spans, rec.Map{"id": "p"}, 0, false))
		}
	}
}

func fuzzSpans(random *rand.Rand) []rec.Map {
	styles := []rec.Map{nil, {"shape": "it"}, {"series": "bx"}, {"generic": "monospace"}}
	spans := []rec.Map{}
	for range 1 + random.IntN(5) {
		if random.IntN(3) == 0 {
			spans = append(spans, rec.Map{"type": "rule"})
		}
		runes := make([]rune, 1+random.IntN(8))
		for at := range runes {
			runes[at] = rune(fuzzAlphabet[random.IntN(len(fuzzAlphabet))])
		}
		span := rec.Map{"type": "text", "text": string(runes)}
		if style := styles[random.IntN(len(styles))]; style != nil {
			span["style"] = style
		}
		spans = append(spans, span)
	}
	return spans
}

func fuzzText(spans []rec.Map) string {
	var builder strings.Builder
	for _, span := range spans {
		if text, ok := span["text"].(string); ok {
			builder.WriteString(text)
		}
	}
	return builder.String()
}

func withoutSpace(text string) string {
	return strings.Map(func(char rune) rune {
		if unicode.IsSpace(char) {
			return -1
		}
		return char
	}, text)
}
