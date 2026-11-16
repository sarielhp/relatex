package mathexport

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/mathintent"
	"github.com/sarielhp/relatex/internal/rec"
)

const speechMath = `<math xmlns="http://www.w3.org/1998/Math/MathML">`

func project(t *testing.T, body string) string {
	t.Helper()
	projected := Project(ParsePaper(speechMath + body + "</math>"))
	return strings.TrimSuffix(strings.TrimPrefix(projected, "<math xmlns='http://www.w3.org/1998/Math/MathML'>"), "</math>")
}

func TestSpeechRules(t *testing.T) {
	upright := func(letter string) string { return `<mi mathvariant="normal">` + letter + `</mi>` }
	fence := func(text string) string { return `<mo lspace="0" rspace="0" stretchy="false">` + text + `</mo>` }
	// The projection decorates an accent's mark with the accent font and stretchy.
	accentMark := func(text string) string {
		return "<mo stretchy='true' style='font-family:&apos;L2D Math Accents&apos;,&quot;Latin Modern Math&quot;, &quot;STIX Two Math&quot;, math'>" + text + "</mo>"
	}
	cases := []struct{ name, body, want string }{
		{"operator name", `<mrow>` + upright("l") + upright("o") + upright("g") + `</mrow><mspace width="0.167em"/><mi>x</mi>`,
			"<mrow><mi>log</mi></mrow><mo>\u2061</mo><mspace width='0.167em'/><mi>x</mi>"},
		{"a sans-serif function name is one identifier (sample 84)", `<mrow><mi>𝗅</mi><mi>𝗈</mi><mi>𝗀</mi></mrow><mspace width="0.167em"/><mi>x</mi>`,
			"<mrow><mi>𝗅𝗈𝗀</mi></mrow><mo>\u2061</mo><mspace width='0.167em'/><mi>x</mi>"},
		{"one sans-serif letter stays", `<mi>𝖳</mi>`, "<mi>𝖳</mi>"},
		{"a sans-serif word that is no function name stays", `<mi>𝗋𝖾𝖿𝗅</mi>`, "<mi intent='refl'>𝗋𝖾𝖿𝗅</mi>"},
		{"upright word", upright("e") + upright("n") + upright("s"), "<mi>ens</mi>"},
		{"one upright letter stays", upright("d") + `<mi>x</mi>`, "<mi mathvariant='normal'>d</mi><mi>x</mi>"},
		{"normal where it changes nothing", `<mi mathvariant="normal">∅</mi><mi mathvariant="normal">Ω</mi>`,
			"<mi>∅</mi><mi mathvariant='normal'>Ω</mi>"},
		{"no application before an operator", `<mi>max</mi><mo lspace="0.278em" rspace="0.278em">=</mo>`,
			"<mi>max</mi><mo lspace='0.278em' rspace='0.278em'>=</mo>"},
		{"application before a fence, scripted name", `<msub><mi>log</mi><mn>2</mn></msub>` + fence("(") + `<mi>n</mi>` + fence(")"),
			"<msub><mi>log</mi><mn>2</mn></msub><mo>\u2061</mo><mrow><mo lspace='0' rspace='0' stretchy='false'>(</mo>" +
				"<mi>n</mi><mo lspace='0' rspace='0' stretchy='false'>)</mo></mrow>"},
		{"lim sup is one name", `<munder><mrow><mi>lim</mi><mspace width="0.167em"/><mi>sup</mi></mrow><mi>n</mi></munder><mi>a</mi>`,
			"<munder><mrow><mi>lim</mi><mspace width='0.167em'/><mi>sup</mi></mrow><mi>n</mi></munder><mo>\u2061</mo><mi>a</mi>"},
		{"nested pairs", fence("[") + fence("(") + `<mi>a</mi>` + fence(")") + fence("]"),
			"<mrow><mo lspace='0' rspace='0' stretchy='false'>[</mo><mrow><mo lspace='0' rspace='0' stretchy='false'>(</mo>" +
				"<mi>a</mi><mo lspace='0' rspace='0' stretchy='false'>)</mo></mrow><mo lspace='0' rspace='0' stretchy='false'>]</mo></mrow>"},
		{"a half-open interval stays", fence("[") + `<mi>a</mi>` + fence(")"),
			"<mo lspace='0' rspace='0' stretchy='false'>[</mo><mi>a</mi><mo lspace='0' rspace='0' stretchy='false'>)</mo>"},
		{"a stretchy operator inside keeps the pair apart", fence("(") + `<mo>|</mo>` + fence(")"),
			"<mo lspace='0' rspace='0' stretchy='false'>(</mo><mo>|</mo><mo lspace='0' rspace='0' stretchy='false'>)</mo>"},
		{"a stretchy fence is not paired", `<mo>(</mo><mi>a</mi><mo>)</mo>`, "<mo>(</mo><mi>a</mi><mo>)</mo>"},
		{"juxtaposed letters", `<mi>𝑛</mi><mi>𝑚</mi><msub><mi>𝑘</mi><mi>𝑖</mi></msub>`,
			"<mi>𝑛</mi><mo>\u2062</mo><mi>𝑚</mi><mo>\u2062</mo><msub><mi>𝑘</mi><mi>𝑖</mi></msub>"},
		{"nabla beside a letter", `<mi>𝜇</mi><mi>∇</mi><mi>𝜙</mi>`, "<mi>𝜇</mi><mo>\u2062</mo><mi>∇</mi><mo>\u2062</mo><mi>𝜙</mi>"},
		{"a differential stays joined", `<mi>𝑓</mi><mi>𝑑</mi><mi>𝑡</mi>`, "<mi>𝑓</mi><mo>\u2062</mo><mi>𝑑</mi><mi>𝑡</mi>"},
		{"no invisible times beside a symbol", `<mi>‖</mi><mi>𝑢</mi><mi>‖</mi>`, "<mi>‖</mi><mi>𝑢</mi><mi>‖</mi>"},
		{"italic h", `<msub><mi>𝐸</mi><mi>ℎ</mi></msub>`, "<msub><mi>𝐸</mi><mi>h</mi></msub>"},
		{"a styled word", `<mi>𝗋𝖾𝖿𝗅</mi>`, "<mi intent='refl'>𝗋𝖾𝖿𝗅</mi>"},
		{"italic letters in one identifier", `<mi>𝑝𝑦</mi>`, "<mi intent='p-y'>𝑝𝑦</mi>"},
		{"a mixed identifier stays", `<mi>𝑝𝐲</mi>`, "<mi>𝑝𝐲</mi>"},
		{"a star in a script", `<msubsup><mi>𝑑</mi><mi>𝑇</mi><mi>∗</mi></msubsup><mi>∗</mi>`,
			"<msubsup><mi>𝑑</mi><mi>𝑇</mi><mi intent='star'>∗</mi></msubsup><mi>∗</mi>"},
		{"perp in a script row", `<msup><mi>𝑉</mi><mrow><mo>⊥</mo></mrow></msup>`, "<msup><mi>𝑉</mi><mrow><mo intent='perp'>⊥</mo></mrow></msup>"},
		{"an empty equation label drops its intent", `<mtable><mtr><mtd intent=":equation-label"><mtext></mtext></mtd><mtd><mi>x</mi></mtd></mtr></mtable>`,
			"<mtable><mtr><mtd style='text-align:center'><mtext/></mtd><mtd style='text-align:center'><mi>x</mi></mtd></mtr></mtable>"},
		{"a printed equation label keeps its intent", `<mtable><mtr><mtd intent=":equation-label"><mtext>(1)</mtext></mtd><mtd><mi>x</mi></mtd></mtr></mtable>`,
			"<mtable><mtr><mtd intent=':equation-label' style='text-align:center'><mtext>(1)</mtext></mtd><mtd style='text-align:center'><mi>x</mi></mtd></mtr></mtable>"},
		{"div applied is the divergence", `<mi>div</mi><mi>𝐮</mi>`, "<mi intent='divergence'>div</mi><mo>\u2061</mo><mi>𝐮</mi>"},
		{"supp applied is the support", `<mi>supp</mi><mi>ψ</mi>`, "<mi intent='support'>supp</mi><mo>\u2061</mo><mi>ψ</mi>"},
		{"dist applied is the distance", `<mi>dist</mi><mo lspace="0" rspace="0" stretchy="false">(</mo><mi>x</mi><mo>,</mo><mi>A</mi><mo lspace="0" rspace="0" stretchy="false">)</mo>`,
			"<mi intent='distance'>dist</mi><mo>\u2061</mo><mrow><mo lspace='0' rspace='0' stretchy='false'>(</mo><mi>x</mi><mo>,</mo><mi>A</mi>" +
				"<mo lspace='0' rspace='0' stretchy='false'>)</mo></mrow>"},
		{"diam applied is the diameter", `<mi>diam</mi><mi>S</mi>`, "<mi intent='diameter'>diam</mi><mo>\u2061</mo><mi>S</mi>"},
		{"vol applied is the volume", `<mi>vol</mi><mi>S</mi>`, "<mi intent='volume'>vol</mi><mo>\u2061</mo><mi>S</mi>"},
		{"diag applied is the diagonal", `<mi>diag</mi><mi>a</mi>`, "<mi intent='diagonal'>diag</mi><mo>\u2061</mo><mi>a</mi>"},
		{"sgn applied is the sign", `<mi>sgn</mi><mi>x</mi>`, "<mi intent='sign'>sgn</mi><mo>\u2061</mo><mi>x</mi>"},
		{"lcm applied is the least common multiple", `<mi>lcm</mi><mi>a</mi>`, "<mi intent='least-common-multiple'>lcm</mi><mo>\u2061</mo><mi>a</mi>"},
		{"gcd applied is the greatest common divisor", `<mi>gcd</mi><mi>a</mi>`, "<mi intent='greatest-common-divisor'>gcd</mi><mo>\u2061</mo><mi>a</mi>"},
		{"deg applied is the degree", `<mi>deg</mi><mi>p</mi>`, "<mi intent='degree'>deg</mi><mo>\u2061</mo><mi>p</mi>"},
		{"id applied is the identity", `<mi>id</mi><mi>x</mi>`, "<mi intent='identity'>id</mi><mo>\u2061</mo><mi>x</mi>"},
		{"Id applied is the identity", `<mi>Id</mi><mi>x</mi>`, "<mi intent='identity'>Id</mi><mo>\u2061</mo><mi>x</mi>"},
		{"a name followed by an operator gets no intent", `<mi>supp</mi><mspace width="0.167em"/><mo>+</mo><mn>1</mn>`,
			"<mi>supp</mi><mspace width='0.167em'/><mo>+</mo><mn>1</mn>"},
		{"italic letters are no name", `<mi>𝑑𝑖𝑠𝑡</mi><mi>𝑥</mi>`, "<mi intent='d-i-s-t'>𝑑𝑖𝑠𝑡</mi><mi>𝑥</mi>"},
		{"accent over an empty box", `<mrow><mover accent="true"><mspace width="8.302198pt" height="8.169341pt"/><mo stretchy="true">ˇ</mo></mover>` +
			`<mspace width="-8.3016pt"/></mrow><mspace/><mi mathvariant="normal">Λ</mi>`,
			"<mrow intent='check:postfix($speech-a1)'><mrow><mover accent='true'><mspace height='8.169341pt' width='8.302198pt'/>" +
				accentMark("ˇ") + "</mover><mspace style='margin-inline-start:-8.3016pt' width='0pt'/></mrow><mspace/>" +
				"<mi arg='speech-a1' mathvariant='normal'>Λ</mi></mrow>"},
		{"accent over an empty box without the space", `<mrow><mover accent="true"><mspace width="6.600262pt"/><mo>˜</mo></mover>` +
			`<mspace width="-6.6pt"/></mrow><mi>𝑘</mi>`,
			"<mrow intent='tilde:postfix($speech-a1)'><mrow><mover accent='true'><mspace width='6.600262pt'/>" + accentMark("˜") +
				"</mover><mspace style='margin-inline-start:-6.6pt' width='0pt'/></mrow><mi arg='speech-a1'>𝑘</mi></mrow>"},
		{"an empty box of several spaces", `<mrow><mover accent="true"><mrow><mspace width="7.918402pt" height="6.535483pt"/>` +
			`<mspace width="1.693200pt"/></mrow><mo>ˇ</mo></mover><mspace width="-1.6932pt"/><mspace width="-7.9188pt"/></mrow><mi>𝐺</mi>`,
			"<mrow intent='check:postfix($speech-a1)'><mrow><mover accent='true'><mrow><mspace height='6.535483pt' width='7.918402pt'/>" +
				"<mspace width='1.693200pt'/></mrow>" + accentMark("ˇ") + "</mover>" +
				"<mspace style='margin-inline-start:-1.6932pt' width='0pt'/><mspace style='margin-inline-start:-7.9188pt' width='0pt'/>" +
				"</mrow><mi arg='speech-a1'>𝐺</mi></mrow>"},
		{"a double accent over an empty box", `<mrow><mover accent="true"><mstyle displaystyle="true" scriptlevel="0">` +
			`<mover accent="true"><mspace depth="0.000000pt" height="6.807784pt" width="7.195247pt"/><mo>¯</mo></mover></mstyle>` +
			`<mo>˙</mo></mover><mspace width="-7.195pt"/></mrow><mi mathvariant="normal">Ω</mi>`,
			"<mrow intent='bar-dot:postfix($speech-a1)'><mrow><mover accent='true'><mstyle displaystyle='true' scriptlevel='0'>" +
				"<mover accent='true'><mspace depth='0.000000pt' height='6.807784pt' width='7.195247pt'/>" + accentMark("¯") +
				"</mover></mstyle>" + accentMark("˙") + "</mover><mspace style='margin-inline-start:-7.195pt' width='0pt'/></mrow>" +
				"<mi arg='speech-a1' mathvariant='normal'>Ω</mi></mrow>"},
		{"a plain accent over a letter", `<mover accent="true"><mi>x</mi><mo>^</mo></mover>`,
			"<mover accent='true'><mi>x</mi>" + accentMark("^") + "</mover>"},
		{"an accent box with nothing after it", `<mrow><mover accent="true"><mspace width="5pt"/><mo>˜</mo></mover>` +
			`<mspace width="-5pt"/></mrow>`,
			"<mrow><mover accent='true'><mspace width='5pt'/>" + accentMark("˜") +
				"</mover><mspace style='margin-inline-start:-5pt' width='0pt'/></mrow>"},
		{"an accent box before an mrow gets no intent", `<mrow><mover accent="true"><mspace width="5pt"/><mo>˜</mo></mover>` +
			`<mspace width="-5pt"/></mrow><mrow><mi>x</mi></mrow>`,
			"<mrow><mover accent='true'><mspace width='5pt'/>" + accentMark("˜") +
				"</mover><mspace style='margin-inline-start:-5pt' width='0pt'/></mrow><mrow><mi>x</mi></mrow>"},
		{"an accent box inside a fence pair", fence("(") + `<mrow><mover accent="true"><mspace width="5pt"/><mo>˜</mo></mover>` +
			`<mspace width="-5pt"/></mrow><mi>x</mi>` + fence(")"),
			"<mrow><mo lspace='0' rspace='0' stretchy='false'>(</mo><mrow intent='tilde:postfix($speech-a1)'><mrow>" +
				"<mover accent='true'><mspace width='5pt'/>" + accentMark("˜") +
				"</mover><mspace style='margin-inline-start:-5pt' width='0pt'/></mrow><mi arg='speech-a1'>x</mi></mrow>" +
				"<mo lspace='0' rspace='0' stretchy='false'>)</mo></mrow>"},
		{"a double-struck one as a subscript base is the indicator", `<msub><mn>𝟙</mn><mi>𝐴</mi></msub>`,
			"<msub><mn intent='indicator'>𝟙</mn><mi>𝐴</mi></msub>"},
		{"a double-struck one as the base of two scripts is the indicator", `<msubsup><mn>𝟙</mn><mi>𝐴</mi><mn>2</mn></msubsup>`,
			"<msubsup><mn intent='indicator'>𝟙</mn><mi>𝐴</mi><mn>2</mn></msubsup>"},
		{"a bare double-struck one stays", `<mn>𝟙</mn>`, "<mn>𝟙</mn>"},
		{"a double-struck one with a superscript only stays", `<msup><mn>𝟙</mn><mn>2</mn></msup>`, "<msup><mn>𝟙</mn><mn>2</mn></msup>"},
		{"a plain digit as a subscript base stays", `<msub><mn>1</mn><mi>A</mi></msub>`, "<msub><mn>1</mn><mi>A</mi></msub>"},
		// The 100-formula sample (decision 114, amendment 2026-11-15).
		{"a decimal in a list is one number (sample 53)", `<mn>0</mn><mo lspace="0" rspace="0">.</mo><mn>2</mn>` +
			`<mo lspace="0" rspace="0.167em">,</mo><mn>0</mn><mo lspace="0" rspace="0">.</mo><mn>4</mn>`,
			"<mn>0.2</mn><mo lspace='0' rspace='0.167em'>,</mo><mn>0.4</mn>"},
		{"a period with space around it is no decimal point", `<mn>1</mn><mo lspace="0.222em" rspace="0.222em">.</mo><mn>2</mn>`,
			"<mn>1</mn><mo lspace='0.222em' rspace='0.222em'>.</mo><mn>2</mn>"},
		{"a script on an empty box belongs to the element before it (sample 84)",
			`<mi>Σ</mi><msup><mrow/><mrow><mo lspace="0" rspace="0">−</mo><mn>1</mn></mrow></msup>`,
			"<msup><mi>Σ</mi><mrow><mo lspace='0' rspace='0'>−</mo><mn>1</mn></mrow></msup>"},
		{"a prime on an empty box after a scripted letter (sample 84)", `<msup><mi>X</mi><mo>′</mo></msup><msup><mrow/><mo>′</mo></msup>`,
			"<msup><msup><mi>X</mi><mo>′</mo></msup><mo>′</mo></msup>"},
		{"a prescript stays", `<msup><mrow/><mn>14</mn></msup><mi>C</mi>`, "<msup><mrow/><mn>14</mn></msup><mi>C</mi>"},
		{"a script on an empty box after an operator stays", `<mo>+</mo><msup><mrow/><mn>2</mn></msup>`,
			"<mo>+</mo><msup><mrow/><mn>2</mn></msup>"},
		{"an operator name set as text is applied and spoken (sample 61)",
			`<mpadded width="18pt"><mrow><mpadded width="0pt"><mtext>supp</mtext></mpadded></mrow></mpadded>` + fence("(") + `<mi>𝑃</mi>` + fence(")"),
			"<mpadded width='18pt'><mrow><mpadded width='0pt'><mi intent='support'>supp</mi></mpadded></mrow></mpadded><mo>\u2061</mo>" +
				"<mrow><mo lspace='0' rspace='0' stretchy='false'>(</mo><mi>𝑃</mi><mo lspace='0' rspace='0' stretchy='false'>)</mo></mrow>"},
		{"text that is no operator name stays", `<mpadded width="9pt"><mtext>if</mtext></mpadded><mi>𝑃</mi>`,
			"<mpadded width='9pt'><mtext>if</mtext></mpadded><mi>𝑃</mi>"},
		{"top in a script (sample 96)", `<msup><mi>A</mi><mo>⊤</mo></msup>`, "<msup><mi>A</mi><mo intent='top'>⊤</mo></msup>"},
		{"sharp in a script (sample 89)", `<msup><mi>P</mi><mi>♯</mi></msup>`, "<msup><mi>P</mi><mi intent='sharp'>♯</mi></msup>"},
		{"flat in a script", `<msub><mi>P</mi><mi>♭</mi></msub>`, "<msub><mi>P</mi><mi intent='flat'>♭</mi></msub>"},
		{"top outside a script stays", `<mi>⊤</mi>`, "<mi>⊤</mi>"},
		{"an accent over the base of scripts (sample 64)", `<mrow><mover accent="true"><mspace width="6.8pt"/><mo>ˇ</mo></mover>` +
			`<mspace width="-6.9pt"/></mrow><mspace width="0.278em"/><msubsup><mi mathvariant="normal">Λ</mi><mi>𝐺</mi>` +
			`<mrow><mi mathvariant="normal">p</mi><mi mathvariant="normal">o</mi><mi mathvariant="normal">s</mi></mrow></msubsup>`,
			"<msubsup><mrow intent='check:postfix($speech-a1)'><mrow><mover accent='true'><mspace width='6.8pt'/>" + accentMark("ˇ") +
				"</mover><mspace style='margin-inline-start:-6.9pt' width='0pt'/></mrow><mspace width='0.278em'/>" +
				"<mi arg='speech-a1' mathvariant='normal'>Λ</mi></mrow><mi>𝐺</mi><mrow><mi>pos</mi></mrow></msubsup>"},
	}
	for _, test := range cases {
		if got := project(t, test.body); got != test.want {
			t.Errorf("%s:\n got %s\nwant %s", test.name, got, test.want)
		}
	}
}

// A space between the letters is another atom: the run is not one identifier, so no function name
// is read (a kern is TeX's ordinary spacing, not a word).
func TestStyledFunctionNameNeedsOneRun(t *testing.T) {
	got := project(t, `<mi>𝗅</mi><mspace width="0.167em"/><mi>𝗈</mi><mi>𝗀</mi>`)
	if strings.Contains(got, "<mi>𝗅𝗈𝗀</mi>") {
		t.Fatalf("letters with a space between became one identifier: %s", got)
	}
	if strings.Count(got, "<mi>") != 3 {
		t.Fatalf("letters with a space between did not stay separate: %s", got)
	}
}

// A run of separate sans-serif letters that spells no function name (\mathsf{refl}) stays four
// identifiers: the run path, not a pre-merged mi.
func TestStyledNonFunctionRunStays(t *testing.T) {
	got := project(t, `<mi>𝗋</mi><mi>𝖾</mi><mi>𝖿</mi><mi>𝗅</mi>`)
	if strings.Contains(got, "𝗋𝖾𝖿𝗅") {
		t.Fatalf("a styled run that is no function name became one identifier: %s", got)
	}
	if strings.Count(got, "<mi>") != 4 {
		t.Fatalf("a styled run that is no function name did not stay separate: %s", got)
	}
}

func TestSpeechRowOfOnePair(t *testing.T) {
	got := project(t, `<mrow><mo lspace="0" rspace="0" stretchy="false">(</mo><mi>a</mi><mo lspace="0" rspace="0" stretchy="false">)</mo></mrow>`)
	if strings.Count(got, "<mrow>") != 1 {
		t.Fatalf("a row that is one pair is grouped again: %s", got)
	}
}

func TestAnnotation(t *testing.T) {
	root := ParsePaper(speechMath + `<mi>x</mi><mo>+</mo><mn>1</mn></math>`)
	got := ProjectAnnotated(root, `x+1 & <`)
	want := "<math xmlns='http://www.w3.org/1998/Math/MathML'><semantics><mrow><mi>x</mi><mo>+</mo><mn>1</mn></mrow>" +
		"<annotation encoding='application/x-tex'>x+1 &amp; &lt;</annotation></semantics></math>"
	if got != want {
		t.Fatalf("annotation:\n got %s\nwant %s", got, want)
	}
	single := ProjectAnnotated(ParsePaper(speechMath+`<mrow><mi>x</mi></mrow></math>`), "x")
	if !strings.Contains(single, "<semantics><mrow><mi>x</mi></mrow><annotation") {
		t.Fatalf("a single row is the presentation itself: %s", single)
	}
	if Project(root) != ProjectAnnotated(root, "") {
		t.Fatal("no TeX, no annotation")
	}
}

func TestAuthorTeX(t *testing.T) {
	verified := rec.Map{"tex": rec.Map{"status": "verified", "text": "x^2 \\label{eq} % note\n"}}
	if got := AuthorTeX(verified); got != "x^2" {
		t.Fatalf("verified TeX as Markdown ships it: %q", got)
	}
	for _, node := range []rec.Map{{"tex": rec.Map{"status": "unverified", "text": "x"}}, {"tex": rec.Map{"status": "unavailable"}}, {}} {
		if got := AuthorTeX(node); got != "" {
			t.Fatalf("only verified TeX is annotated: %q", got)
		}
	}
}

// sampleEntry is one formula of testdata/speech_sample.json, in the file's key order: the first 40 are
// the reviewed sample of decision 114, the rest the seeded pick of tools/math_speech_pick (with TeX and
// macros where the paper was converted with the author's math).
type sampleEntry struct {
	Source          string                 `json:"source"`
	Category        string                 `json:"category"`
	Display         bool                   `json:"display,omitempty"`
	Pick            string                 `json:"pick,omitempty"`
	MathML          string                 `json:"mathml"`
	TeX             string                 `json:"tex,omitempty"`
	Macros          map[string]sampleMacro `json:"macros,omitempty"`
	Projected       string                 `json:"projected"`
	ProjectedAuthor string                 `json:"projected_author,omitempty"`
	Review          json.RawMessage        `json:"review,omitempty"`
	MathCAT         json.RawMessage        `json:"mathcat,omitempty"`
	MathCATAuthor   json.RawMessage        `json:"mathcat_author,omitempty"`
}

type sampleMacro struct {
	Body       string         `json:"body"`
	Parameters int            `json:"parameters"`
	Paired     map[string]any `json:"paired,omitempty"`
}

func readSample(t *testing.T) []sampleEntry {
	t.Helper()
	data, err := os.ReadFile("testdata/speech_sample.json")
	if err != nil {
		t.Fatal(err)
	}
	var sample []sampleEntry
	if err := json.Unmarshal(data, &sample); err != nil {
		t.Fatal(err)
	}
	return sample
}

// projectAuthor is the page's MathML of the entry with the author's math: the verified TeX and the
// paper's macros, as a conversion with --author-math gives the formula.
func projectAuthor(entry sampleEntry) string {
	macros := mathintent.Macros{}
	for name, macro := range entry.Macros {
		macros[name] = mathintent.Macro{Body: macro.Body, Parameters: macro.Parameters, Paired: macro.Paired != nil}
	}
	node := rec.Map{"tex": rec.Map{"status": "verified", "text": entry.TeX}}
	return ProjectFormula(ParsePaper(entry.MathML), node, macros)
}

// TestSpeechSample locks the cleaned MathML of the reviewed speech sample (plans/math-speech-sample.md):
// formulas of the stores, each projected for the page as the review heard it, and, where the entry has the
// author's TeX and macros, projected as the author's math gives it.
func TestSpeechSample(t *testing.T) {
	sample := readSample(t)
	if len(sample) < 100 {
		t.Fatalf("the reviewed sample has %d formulas", len(sample))
	}
	for _, entry := range sample {
		if got := Project(ParsePaper(entry.MathML)); got != entry.Projected {
			t.Errorf("%s:\n got %s\nwant %s", entry.Source, got, entry.Projected)
		}
		if entry.TeX == "" {
			continue
		}
		if got := projectAuthor(entry); got != entry.ProjectedAuthor {
			t.Errorf("%s with author math:\n got %s\nwant %s", entry.Source, got, entry.ProjectedAuthor)
		}
	}
}

// TestSpeechSampleFill rewrites the projections of testdata/speech_sample.json after
// tools/math_speech_pick (RELATEX_SPEECH_SAMPLE_FILL=1); it never runs otherwise.
func TestSpeechSampleFill(t *testing.T) {
	if os.Getenv("RELATEX_SPEECH_SAMPLE_FILL") == "" {
		t.Skip("set RELATEX_SPEECH_SAMPLE_FILL=1 to rewrite the sample's projections")
	}
	sample := readSample(t)
	for index := range sample {
		entry := &sample[index]
		entry.Projected = Project(ParsePaper(entry.MathML))
		if entry.TeX != "" {
			entry.ProjectedAuthor = projectAuthor(*entry)
		}
	}
	var out strings.Builder
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(sample); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("testdata/speech_sample.json", []byte(out.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}
