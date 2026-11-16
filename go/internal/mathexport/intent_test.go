package mathexport

import (
	"encoding/json"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/mathintent"
	"github.com/sarielhp/relatex/internal/rec"
)

// intentFixture is testdata/intent_fixture.json: the formulas of testdata/intent_fixture.tex as
// the collector typeset them, with the intents the page gives each (decision 109).
type intentFixture struct {
	MathMacros map[string]struct {
		Body       string
		Parameters int
		Paired     map[string]string
	} `json:"math_macros"`
	Formulas []struct {
		TeX       map[string]any `json:"tex"`
		MathML    string
		Intents   []string
		Projected string
	}
}

func readIntentFixture(t *testing.T) (intentFixture, mathintent.Macros) {
	t.Helper()
	data, err := os.ReadFile("testdata/intent_fixture.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture intentFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	macros := mathintent.Macros{}
	for name, macro := range fixture.MathMacros {
		macros[name] = mathintent.Macro{Body: macro.Body, Parameters: macro.Parameters, Paired: macro.Paired != nil}
	}
	return fixture, macros
}

var intentValue = regexp.MustCompile(`intent='([^']*)'`)

// TestIntentFixture: each macro of the fixture gives its concept, and nothing else changes: with
// the new rows unwrapped and intent and arg removed, the page's MathML is decision 100's.
func TestIntentFixture(t *testing.T) {
	t.Setenv("RELATEX_LEIBNIZ_INTENT", "1")
	fixture, macros := readIntentFixture(t)
	if len(fixture.Formulas) < 40 {
		t.Fatalf("the fixture has %d formulas", len(fixture.Formulas))
	}
	for _, formula := range fixture.Formulas {
		node := rec.Map{"tex": rec.Map(formula.TeX)}
		got := ProjectFormula(ParsePaper(formula.MathML), node, macros)
		intents := []string{}
		for _, match := range intentValue.FindAllStringSubmatch(got, -1) {
			intents = append(intents, match[1])
		}
		if !slices.Equal(intents, formula.Intents) {
			t.Errorf("%v: intents %v, want %v", formula.TeX["text"], intents, formula.Intents)
		}
		if got != formula.Projected {
			t.Errorf("%v: page MathML\n got %s\nwant %s", formula.TeX["text"], got, formula.Projected)
		}
		checkIntentReferences(t, ParseRaw(got))
		plain := ProjectAnnotated(ParsePaper(formula.MathML), AuthorTeX(node))
		if stripped := stripIntent(ParseRaw(got)).String(); stripped != plain {
			t.Errorf("%v: intent changed more than intent:\n got %s\nwant %s", formula.TeX["text"], stripped, plain)
		}
	}
}

// checkIntentReferences: each $name of an intent is the arg of exactly one element inside it.
func checkIntentReferences(t *testing.T, root *Node) {
	t.Helper()
	for _, node := range withSelf(root) {
		intent, ok := node.Attribute("intent")
		if !ok || strings.HasPrefix(intent, ":") {
			continue
		}
		for _, reference := range regexp.MustCompile(`\$([A-Za-z][\w.-]*)`).FindAllStringSubmatch(intent, -1) {
			count := 0
			for _, inner := range node.Descendants() {
				if inner.Get("arg") == reference[1] {
					count++
				}
			}
			if count != 1 {
				t.Errorf("%s: $%s names %d elements", intent, reference[1], count)
			}
		}
	}
}

// stripIntent unwraps the rows that carry intent or arg and removes both attributes elsewhere, but
// the speech pass's literals.
func stripIntent(node *Node) *Node {
	kept := []*Node{}
	for _, child := range node.Elements() {
		wrapper := child.Name == "mrow" && (child.Has("intent") || child.Has("arg"))
		child = stripIntent(child)
		if wrapper {
			kept = append(kept, child.Elements()...)
			continue
		}
		kept = append(kept, child)
	}
	if len(node.Elements()) > 0 {
		node.setElements(kept)
	}
	if intent, ok := node.Attribute("intent"); ok && !speechLiteral(intent) {
		node.RemoveAttribute("intent")
	}
	node.RemoveAttribute("arg")
	return node
}

// speechLiteral: a literal intent of the speech pass (star, p-y, divergence), which is no concept.
func speechLiteral(intent string) bool {
	if !literalIntent.MatchString(intent) {
		return false
	}
	return !slices.ContainsFunc(intentRules, func(rule intentRule) bool { return string(rule.concept) == intent })
}

var literalIntent = regexp.MustCompile(`^[A-Za-z][A-Za-z-]*$`)

func projectIntent(t *testing.T, tex, body string, macros mathintent.Macros) string {
	t.Helper()
	node := rec.Map{"tex": rec.Map{"status": "verified", "text": tex}}
	return ProjectFormula(ParsePaper(speechMath+body+"</math>"), node, macros)
}

// TestLeibnizIntentOffByDefault: the fraction of papers2/0310 (equation 11) reads "partial f of X X 1"
// in MathCAT 0.7.7 with the partial-derivative shape, so by default it carries none; the setting
// writes the shape.
func TestLeibnizIntentOffByDefault(t *testing.T) {
	const tex = `\frac{\partial f(\mat{X})}{\partial \mat{X}}`
	matrix := mathintent.Macros{"mat": {Body: `#1`, Parameters: 1}}
	const body = `<mfrac><mrow><mo lspace='0' rspace='0'>∂</mo><mrow><mi>𝑓</mi><mrow><mo lspace='0' rspace='0' stretchy='false'>(</mo>` +
		`<mi>𝑋</mi><mo lspace='0' rspace='0' stretchy='false'>)</mo></mrow></mrow></mrow><mrow><mo lspace='0' rspace='0'>∂</mo>` +
		`<mi>𝑋</mi></mrow></mfrac>`
	if got := projectIntent(t, tex, body, matrix); strings.Contains(got, "intent") {
		t.Errorf("default: %s", got)
	}
	t.Setenv("RELATEX_LEIBNIZ_INTENT", "1")
	if got := projectIntent(t, tex, body, matrix); !strings.Contains(got, "intent='partial-derivative($a1,$a2,1)'") {
		t.Errorf("setting: %s", got)
	}
}

func TestIntentNeedsAgreement(t *testing.T) {
	fixed := func(text string) string { return `<mo lspace="0" rspace="0" stretchy="false">` + text + `</mo>` }
	bar := fixed("|")
	cases := []struct{ name, tex, body string }{
		{"a source bar pair the MathML lacks", `\abs{x}+\abs{y}`, bar + `<mi>x</mi>` + bar + `<mo>+</mo><mi>y</mi>`},
		{"a bar in a script's base", `\abs{x}`, bar + `<mi>x</mi><msup>` + bar + `<mn>2</mn></msup>`},
		{"a binomial the source does not name", `x`, `<mo>(</mo><mfrac linethickness="0"><mi>n</mi><mi>k</mi></mfrac><mo>)</mo>`},
		{"a floor that pairs with nothing", `\lfloor x \rfloor`, fixed("⌊") + `<mi>x</mi>`},
		{"a transpose sign on a closing parenthesis that opens nowhere", `A)^	op`, `<mi>A</mi><msup>` + fixed(")") + `<mi>⊤</mi></msup>`},
		{"a transpose sign on a bracket closing a parenthesis", `(A]^	op`, fixed("(") + `<mi>A</mi><msup>` + fixed("]") + `<mi>⊤</mi></msup>`},
		{"a second factorial mark", `n!!`, `<mi>n</mi><mo>!</mo><mo>!</mo>`},
		{"an unverified formula", ``, `<mi>n</mi><mo>!</mo>`},
	}
	macros := mathintent.Macros{"abs": {Body: `\left|#1\right|`, Parameters: 1}}
	for _, test := range cases {
		if got := projectIntent(t, test.tex, test.body, macros); hasConceptIntent(got) {
			t.Errorf("%s: %s", test.name, got)
		}
	}
}

func TestIntentShapes(t *testing.T) {
	fixed := func(text string) string { return `<mo lspace="0" rspace="0" stretchy="false">` + text + `</mo>` }
	cases := []struct{ name, tex, body, want string }{
		{"absolute value of a sum", `\abs{a+b}`, fixed("|") + `<mi>a</mi><mo>+</mo><mi>b</mi>` + fixed("|"),
			"<mrow intent='absolute-value($a1)'><mo lspace='0' rspace='0' stretchy='false'>|</mo><mrow arg='a1'><mi>a</mi>" +
				"<mo>+</mo><mi>b</mi></mrow><mo lspace='0' rspace='0' stretchy='false'>|</mo></mrow>"},
		{"inverse of a named function", `\sin^{-1} x`, `<msup><mi>sin</mi><mrow><mo>−</mo><mn>1</mn></mrow></msup><mi>x</mi>`,
			"<msup intent='inverse($a1)'><mi arg='a1'>sin</mi><mrow><mo>−</mo><mn>1</mn></mrow></msup><mo>\u2061</mo><mi>x</mi>"},
		{"a row argument gets its own row", `\binom{a+b}{c}`,
			`<mo>(</mo><mfrac linethickness="0pt"><mrow><mi>a</mi><mo>+</mo><mi>b</mi></mrow><mi>c</mi></mfrac><mo>)</mo>`,
			"<mrow intent='binomial-coefficient($a1,$a2)'><mo>(</mo><mfrac linethickness='0pt'><mrow arg='a1'><mrow><mi>a</mi>" +
				"<mo>+</mo><mi>b</mi></mrow></mrow><mi arg='a2'>c</mi></mfrac><mo>)</mo></mrow>"},
		{"factorial", `n!`, `<mi>n</mi><mo>!</mo>`, "<mi>n</mi><mo intent='factorial'>!</mo>"},
	}
	for _, test := range cases {
		got := projectIntent(t, test.tex, test.body, mathintent.Macros{"abs": {Body: `\left|#1\right|`, Parameters: 1}})
		if !strings.Contains(got, test.want) {
			t.Errorf("%s:\n got %s\nwant %s", test.name, got, test.want)
		}
	}
}

// TestBareBarsAndOperators (decision 114): a bare |x| gets no intent (one meaning per document was
// removed, item 5), a macro's ordinary double bar gets its concept, and the expectation and
// variance signs of the paper's macros theirs.
func TestBareBarsAndOperators(t *testing.T) {
	fixed := func(text string) string { return `<mo lspace="0" rspace="0" stretchy="false">` + text + `</mo>` }
	macros := mathintent.Macros{"E": {Body: `\mathbb{E}`}, "Var": {Body: `\operatorname{Var}`}, "norm": {Body: `\|#1\|`, Parameters: 1}}
	node := func(tex string) rec.Map { return rec.Map{"tex": rec.Map{"status": "verified", "text": tex}} }
	bar := speechMath + fixed("|") + `<mi>x</mi>` + fixed("|") + `</math>`
	if got := ProjectFormula(ParsePaper(bar), node(`|x|`), macros); strings.Contains(got, "intent") {
		t.Errorf("bare bars: %s", got)
	}
	operators := ProjectFormula(ParsePaper(speechMath+`<mi>𝔼</mi><mi>𝑋</mi><mo>+</mo><mi>Var</mi><mi>𝑌</mi></math>`),
		node(`\E X + \Var Y`), macros)
	if !strings.Contains(operators, "<mi intent='expected-value'>𝔼</mi>") || !strings.Contains(operators, "<mi intent='variance'>Var</mi>") {
		t.Errorf("operators: %s", operators)
	}
	double := ParsePaper(speechMath + `<mi>‖</mi><mi>𝑢</mi><mi>‖</mi></math>`)
	if got := ProjectFormula(double, node(`\norm{u}`), macros); !strings.Contains(got,
		"<mrow intent='norm($a1)'><mi>‖</mi><mi arg='a1'>𝑢</mi><mi>‖</mi></mrow>") {
		t.Errorf("a macro's ordinary double bar: %s", got)
	}
	if got := ProjectFormula(double, node(`\|u\|`), macros); strings.Contains(got, "intent") {
		t.Errorf("a bare double bar: %s", got)
	}
	inner := mathintent.Macros{"inner": {Body: `\langle #1,#2\rangle`, Parameters: 2}}
	pair := speechMath + fixed("⟨") + `<mi>𝑢</mi><mo>+</mo><mi>𝑤</mi><mo>,</mo><mi>𝑣</mi>` + fixed("⟩") + `</math>`
	if got := ProjectFormula(ParsePaper(pair), node(`\inner{u+w}{v}`), inner); !strings.Contains(got,
		"intent='inner-product($a1,$a2)'") || !strings.Contains(got, "<mrow arg='a1'><mi>𝑢</mi><mo>+</mo><mi>𝑤</mi></mrow>") {
		t.Errorf("inner product: %s", got)
	}
}

// TestPairedWithoutAuthorMath checks why paired-delimiter intent requires --author-math: when the
// author-math pass did not run, document.json has neither math_macros nor verified node tex.text,
// and LuaMML MathML alone is identical for bare |x|, ⟨u, v⟩, ‖u‖ and named paired delimiters.
func TestPairedWithoutAuthorMath(t *testing.T) {
	fixed := func(text string) string { return `<mo lspace="0" rspace="0" stretchy="false">` + text + `</mo>` }
	absMML := speechMath + fixed("|") + `<mi>x</mi>` + fixed("|") + `</math>`
	innerMML := speechMath + fixed("⟨") + `<mi>𝑢</mi><mo>,</mo><mi>𝑣</mi>` + fixed("⟩") + `</math>`
	normMML := speechMath + `<mi>‖</mi><mi>𝑢</mi><mi>‖</mi></math>`

	unavailable := rec.Map{"tex": rec.Map{"status": "unavailable"}}
	emptyMacros := mathintent.MacrosOf(rec.Map{})
	for _, sample := range []struct{ name, mml string }{
		{"single bars", absMML},
		{"angle brackets", innerMML},
		{"ordinary double bars", normMML},
	} {
		if got := ProjectFormula(ParsePaper(sample.mml), unavailable, emptyMacros); strings.Contains(got, "intent") {
			t.Errorf("%s without --author-math got intent: %s", sample.name, got)
		}
	}

	// With --author-math, verified TeX and kept macros disambiguate the exact same MathML.
	doc := rec.Map{"math_macros": rec.Map{
		"absv":  rec.Map{"body": `\mathopen\lvert #1\mathclose\rvert`, "parameters": int64(1), "paired": rec.Map{}},
		"inner": rec.Map{"body": `\mathopen\langle #1\mathclose\rangle`, "parameters": int64(1), "paired": rec.Map{}},
		"vnorm": rec.Map{"body": `\mathopen\| #1\mathclose\|`, "parameters": int64(1), "paired": rec.Map{}},
	}}
	macros := mathintent.MacrosOf(doc)
	verified := func(tex string) rec.Map { return rec.Map{"tex": rec.Map{"status": "verified", "text": tex}} }
	if got := ProjectFormula(ParsePaper(absMML), verified(`\absv{x}`), macros); !strings.Contains(got, "intent='absolute-value($a1)'") {
		t.Errorf("verified \\absv: %s", got)
	}
	if got := ProjectFormula(ParsePaper(innerMML), verified(`\inner{u, v}`), macros); !strings.Contains(got, "intent='inner-product($a1,$a2)'") {
		t.Errorf("verified \\inner: %s", got)
	}
	if got := ProjectFormula(ParsePaper(normMML), verified(`\vnorm{u}`), macros); !strings.Contains(got, "intent='norm($a1)'") {
		t.Errorf("verified \\vnorm: %s", got)
	}
}

// TestNewcommandNormSpeechMathCAT verifies with tools/mathcat that a plain \newcommand double-bar
// macro (\vnorm = \|#1\|) reads as "norm of u", while an unmacroed bare \|u\| reads without norm intent.
func TestNewcommandNormSpeechMathCAT(t *testing.T) {
	mathcat := "../../../tools/mathcat"
	double := speechMath + `<mi>‖</mi><mi>u</mi><mi>‖</mi></math>`
	macros := mathintent.MacrosOf(rec.Map{"math_macros": rec.Map{
		"vnorm": rec.Map{"body": `\|#1\|`, "parameters": int64(1)},
	}})
	withMacro := ProjectFormula(ParsePaper(double), rec.Map{"tex": rec.Map{"status": "verified", "text": `\vnorm{u}`}}, macros)
	bare := ProjectFormula(ParsePaper(double), rec.Map{"tex": rec.Map{"status": "verified", "text": `\|u\|`}}, macros)
	input, err := json.Marshal([]string{bare, withMacro})
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
	if err := json.Unmarshal(out, &speech); err != nil || len(speech) != 2 {
		t.Fatalf("MathCAT output %q: %v", out, err)
	}
	t.Logf("bare=%q vnorm=%q", speech[0], speech[1])
	if strings.Contains(speech[0], "norm") {
		t.Errorf("bare double bar unexpectedly read as norm: %q", speech[0])
	}
	if !strings.Contains(speech[1], "norm of u") {
		t.Errorf("vnorm speech = %q, want norm of u", speech[1])
	}
}

// TestScriptedClosingBar locks a limit of the bar search (a11y round 2, item E): TeX attaches the
// script of \norm{u}^2, for \norm defined as \|#1\|, to the closing bar, so the page shows that bar as
// an msup's base; the pairs cannot be told apart and the formula gets no norm intent at all, never a
// wrong one. A \left\|#1\right\| body sets one row with the script on the whole, which keeps its intent.
func TestScriptedClosingBar(t *testing.T) {
	macros := mathintent.MacrosOf(rec.Map{"math_macros": rec.Map{
		"vnorm": rec.Map{"body": `\|#1\|`, "parameters": int64(1)},
		"lnorm": rec.Map{"body": `\left\|#1\right\|`, "parameters": int64(1)},
	}})
	verified := func(tex string) rec.Map { return rec.Map{"tex": rec.Map{"status": "verified", "text": tex}} }
	scripted := speechMath + `<mi>‖</mi><mi>𝑢</mi><msup><mi>‖</mi><mn>2</mn></msup><mo>+</mo>` +
		`<mi>‖</mi><mi>𝑣</mi><mi>‖</mi></math>`
	if got := ProjectFormula(ParsePaper(scripted), verified(`\vnorm{u}^2 + \vnorm{v}`), macros); strings.Contains(got, "norm(") {
		t.Errorf("a scripted closing bar got a norm intent: %s", got)
	}
	fence := `<mo fence="true" stretchy="true" symmetric="true">‖</mo>`
	left := speechMath + `<msup><mrow>` + fence + `<mi>𝑢</mi>` + fence + `</mrow><mn>2</mn></msup></math>`
	if got := ProjectFormula(ParsePaper(left), verified(`\lnorm{u}^2`), macros); !strings.Contains(got, "intent='norm($a1)'") {
		t.Errorf("a \\left\\|...\\right\\| norm with a script lost its intent: %s", got)
	}
}

// TestScriptedClosingBarSetting: with RELATEX_SCRIPTED_BAR=1 (experimental, off by default) the pair
// moves into the script's base and gets its norm intent; the second pair of the formula is unchanged.
func TestScriptedClosingBarSetting(t *testing.T) {
	t.Setenv("RELATEX_SCRIPTED_BAR", "1")
	macros := mathintent.MacrosOf(rec.Map{"math_macros": rec.Map{
		"vnorm": rec.Map{"body": `\|#1\|`, "parameters": int64(1)},
	}})
	node := rec.Map{"tex": rec.Map{"status": "verified", "text": `\vnorm{u}^2 + \vnorm{v}`}}
	scripted := speechMath + `<mi>‖</mi><mi>𝑢</mi><msup><mi>‖</mi><mn>2</mn></msup><mo>+</mo>` +
		`<mi>‖</mi><mi>𝑣</mi><mi>‖</mi></math>`
	got := ProjectFormula(ParsePaper(scripted), node, macros)
	if strings.Count(got, "intent='norm(") != 2 || !strings.Contains(got, "</mrow><mn>2</mn></msup>") {
		t.Errorf("scripted closing bar: %s", got)
	}
	many := speechMath + `<mi>‖</mi><mi>𝑢</mi><mo>−</mo><mi>𝑣</mi><msup><mi>‖</mi><mn>2</mn></msup></math>`
	node = rec.Map{"tex": rec.Map{"status": "verified", "text": `\vnorm{u-v}^2`}}
	got = ProjectFormula(ParsePaper(many), node, macros)
	if !strings.Contains(got, "intent='norm($a1)'") || !strings.Contains(got, "<mrow arg='a1'><mi>𝑢</mi><mo>−</mo><mi>𝑣</mi></mrow>") {
		t.Errorf("scripted closing bar with a multi-element argument: %s", got)
	}
}

// hasConceptIntent: the MathML carries an intent that names a concept; a literal of the speech pass
// (star, top, p-y) is no concept.
func hasConceptIntent(mathml string) bool {
	return slices.ContainsFunc(intentValue.FindAllStringSubmatch(mathml, -1), func(match []string) bool { return !speechLiteral(match[1]) })
}

// TestTransposeByMacroBody (decision 187): a macro whose body is a transpose script reads as transpose
// after a closing parenthesis, a letter or a scripted parenthesis; the scripted parentheses group first.
func TestTransposeByMacroBody(t *testing.T) {
	fixed := func(text string) string { return `<mo lspace="0" rspace="0" stretchy="false">` + text + `</mo>` }
	script := func(sign string) string { return `<mrow><mspace width="0pt"/>` + sign + `</mrow>` }
	macros := mathintent.Macros{
		"tran":  {Body: `^{\mkern -1.5mu\mathsf {T}}`},
		"trans": {Body: `^T`},
		"topm":  {Body: `^\top`},
		"name":  {Body: `^T x`},
	}
	cases := []struct{ name, tex, body, want string }{
		{"macro after a closing parenthesis", `(AB)\tran`,
			fixed("(") + `<mi>A</mi><mi>B</mi><msup>` + fixed(")") + script("<mi>𝖳</mi>") + `</msup>`,
			"<mrow intent='transpose($a1)'><mo lspace='0' rspace='0' stretchy='false'>(</mo><mrow arg='a1'><mi>A</mi><mo>\u2062</mo><mi>B</mi></mrow>" +
				"<msup><mo lspace='0' rspace='0' stretchy='false'>)</mo>"},
		{"macro on a letter", `A\tran`, `<msup><mi>A</mi>` + script("<mi>𝖳</mi>") + `</msup>`, "<msup intent='transpose($a1)'><mi arg='a1'>A</mi>"},
		{"macro of an italic T", `(v)\trans`, fixed("(") + `<mi>v</mi><msup>` + fixed(")") + `<mi>𝑇</mi></msup>`, "<mrow intent='transpose($a1)'>"},
		{"a nested group", `((A)\tran B)\tran`,
			fixed("(") + fixed("(") + `<mi>A</mi><msup>` + fixed(")") + `<mi>𝖳</mi></msup><mi>B</mi><msup>` + fixed(")") + `<mi>𝖳</mi></msup>`,
			"<mrow intent='transpose($a2)'><mo lspace='0' rspace='0' stretchy='false'>(</mo><mrow arg='a2'><mrow intent='transpose($a1)'>"},
		{"macro of the top sign", `(A)\topm`, fixed("(") + `<mi>A</mi><msup>` + fixed(")") + `<mi>⊤</mi></msup>`, "<mrow intent='transpose($a1)'>"},
	}
	for _, test := range cases {
		if got := projectIntent(t, test.tex, test.body, macros); !strings.Contains(got, test.want) {
			t.Errorf("%s:\n got %s\nwant %s", test.name, got, test.want)
		}
	}
	none := []struct{ name, tex, body string }{
		{"an italic T the source wrote itself", `A^T`, `<msup><mi>A</mi><mi>𝑇</mi></msup>`},
		{"a macro with more than a script in its body", `A\name`, `<msup><mi>A</mi><mi>𝑇</mi></msup><mi>x</mi>`},
		{"an italic T beside a macro one", `A^T B\trans`, `<msup><mi>A</mi><mi>𝑇</mi></msup><msup><mi>B</mi><mi>𝑇</mi></msup>`},
	}
	for _, test := range none {
		if got := projectIntent(t, test.tex, test.body, macros); hasConceptIntent(got) {
			t.Errorf("%s: %s", test.name, got)
		}
	}
}
