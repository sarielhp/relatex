package mathexport

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/mathintent"
	"github.com/sarielhp/relatex/internal/rec"
)

// TestSpeechHint (decision 122): the author's hint gives \Im the intent of an operator applied to
// its argument; without one the page is unchanged.
func TestSpeechHint(t *testing.T) {
	node := rec.Map{"tex": rec.Map{"status": "verified", "text": `\Im f`}}
	math := speechMath + `<mi>Im</mi><mi>f</mi></math>`
	got := ProjectFormula(ParsePaper(math), node, mathintent.Macros{}, mathintent.Speech{"Im": "image"})
	want := "<mrow intent='image($a1)'><mi>Im</mi><mi arg='a1'>f</mi></mrow>"
	if !strings.Contains(got, want) {
		t.Fatalf("hinted formula:\n got %s\nwant %s", got, want)
	}
	if plain := ProjectFormula(ParsePaper(math), node, mathintent.Macros{}); strings.Contains(plain, "intent") {
		t.Errorf("no hint changed the page: %s", plain)
	}
}

// TestSpeechHintWrappedOperator: the collector writes \mathrm{Im} as an mrow of one mi, so the
// operator the hint wraps is that row, and its argument is the next sibling of the row.
func TestSpeechHintWrappedOperator(t *testing.T) {
	node := rec.Map{"tex": rec.Map{"status": "verified", "text": `\mathrm{Im} f`}}
	math := speechMath + `<mrow><mi>Im</mi></mrow><mspace width='0.167em'/><mi>f</mi></math>`
	got := ProjectFormula(ParsePaper(math), node, mathintent.Macros{}, mathintent.Speech{"Im": "image"})
	want := "<mrow intent='image($a1)'><mrow><mi>Im</mi></mrow><mspace width='0.167em'/><mi arg='a1'>f</mi></mrow>"
	if !strings.Contains(got, want) {
		t.Fatalf("hinted wrapped operator:\n got %s\nwant %s", got, want)
	}
}

// TestSpeechHintFromDocument: MacrosOf carries the document's hints into every ProjectFormula call,
// as the conversion does (document.json math_speech).
func TestSpeechHintFromDocument(t *testing.T) {
	node := rec.Map{"tex": rec.Map{"status": "verified", "text": `\Im f`}}
	macros := mathintent.MacrosOf(rec.Map{"math_speech": rec.Map{"Im": "image"}})
	got := ProjectFormula(ParsePaper(speechMath+`<mi>Im</mi><mi>f</mi></math>`), node, macros)
	if !strings.Contains(got, "intent='image($a1)'") {
		t.Fatalf("document hint: %s", got)
	}
}

// TestSpeechHintMathCAT: MathCAT reads the hinted formula "image of f", not "imaginary".
func TestSpeechHintMathCAT(t *testing.T) {
	mathcat := "../../../tools/mathcat"
	node := rec.Map{"tex": rec.Map{"status": "verified", "text": `\Im f`}}
	math := speechMath + `<mi>Im</mi><mi>f</mi></math>`
	projected := ProjectFormula(ParsePaper(math), node, mathintent.Macros{}, mathintent.Speech{"Im": "image"})
	input, err := json.Marshal([]string{projected})
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(mathcat)
	command.Stdin = strings.NewReader(string(input))
	out, err := command.Output()
	if err != nil {
		t.Skipf("tools/mathcat is not installed: %v", err)
	}
	var speech []string
	if err := json.Unmarshal(out, &speech); err != nil || len(speech) != 1 {
		t.Fatalf("MathCAT output %q: %v", out, err)
	}
	if !strings.Contains(speech[0], "image of f") || strings.Contains(speech[0], "imaginary") {
		t.Errorf("MathCAT says %q", speech[0])
	}
}

// TestSpeechHintNeedsArgument: a hinted operator with nothing to apply to keeps its plain reading.
func TestSpeechHintNeedsArgument(t *testing.T) {
	node := rec.Map{"tex": rec.Map{"status": "verified", "text": `\Im`}}
	math := speechMath + `<mi>Im</mi></math>`
	got := ProjectFormula(ParsePaper(math), node, mathintent.Macros{}, mathintent.Speech{"Im": "image"})
	if strings.Contains(got, "image(") {
		t.Errorf("a hint without an argument set an intent: %s", got)
	}
}

// TestSpeechHintWords: a spoken name of several words joins with hyphens, and words that cannot
// name an intent concept set none.
func TestSpeechHintWords(t *testing.T) {
	node := rec.Map{"tex": rec.Map{"status": "verified", "text": `\Im f`}}
	math := speechMath + `<mi>Im</mi><mi>f</mi></math>`
	got := ProjectFormula(ParsePaper(math), node, mathintent.Macros{}, mathintent.Speech{"Im": "image  set"})
	if !strings.Contains(got, "intent='image-set($a1)'") {
		t.Errorf("two-word hint: %s", got)
	}
	bad := ProjectFormula(ParsePaper(math), node, mathintent.Macros{}, mathintent.Speech{"Im": "image (of)"})
	if strings.Contains(bad, "intent") {
		t.Errorf("a hint that cannot name an intent set one: %s", bad)
	}
}
