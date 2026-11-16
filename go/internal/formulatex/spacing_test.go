package formulatex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

const (
	sinWord    = `<mrow><mi mathvariant="normal">s</mi><mi mathvariant="normal">i</mi><mi mathvariant="normal">n</mi></mrow>`
	limWord    = `<mrow><mi mathvariant="normal">l</mi><mi mathvariant="normal">i</mi><mi mathvariant="normal">m</mi></mrow>`
	thin       = `<mspace width="0.167em"/>`
	parenthesX = `<mrow><mo fence="true">(</mo><mi>𝑥</mi><mo fence="true">)</mo></mrow>`
	cellsAB    = `<mtr><mtd><mrow><mi>𝑎</mi></mrow></mtd><mtd><mrow><mi>𝑏</mi></mrow></mtd></mtr>`
	// bigRight is \bigr] as luamml writes it: the delimiter and the strut that sizes it.
	bigRight = `<mpadded depth="3.486985pt" height="8.468290pt" width="4.151120pt"><mpadded depth="0pt" height="0pt" ` +
		`lspace="0.000000pt" voffset="0.000000pt" width="0pt"><mrow><mo lspace="0" rspace="0">]</mo>` +
		`<mspace depth="0.000000pt" height="11.955123pt" width="0.000000pt"/><mspace width="0.000pt"/></mrow></mpadded></mpadded>`
)

// TestWriteSpacingAndTables covers TeX's automatic thin spaces after other glue and between
// \bigr and an operator, \underset around an operator name without an operator's spaces, and
// tables: an array keeps its column alignment, matrices their environments.
func TestWriteSpacingAndTables(t *testing.T) {
	cases := []struct{ name, body, want string }{
		// TeX's thin space comes after the author's \!, right before the inner atom.
		{"after negative space", `<mi>𝑓</mi><mspace width="-0.167em"/>` + thin + parenthesX, `f\!\left(x\right)`},
		{"author space before automatic", `<mi>𝑓</mi>` + thin + thin + parenthesX, `f\,\left(x\right)`},
		{"close then operator", bigRight + thin + sinWord + thin + `<mi>𝑦</mi>`, `\bigr]\sin y`},
		{"underset operator name", `<munder>` + limWord + `<mi>𝑥</mi></munder><mi>𝑏</mi>`, `\underset{x}{\lim}b`},
		{"operator limits", `<munder>` + limWord + `<mi>𝑥</mi></munder>` + thin + `<mi>𝑏</mi>`, `\lim_xb`},
		{"array", `<mi>𝑥</mi><mtable columnalign="left center">` + cellsAB + `</mtable><mi>𝑦</mi>`,
			`x\begin{array}{lc}a & b\end{array}y`},
		{"bare matrix", `<mspace width="-4.981pt"/><mtable columnalign="center center">` + cellsAB +
			`</mtable><mspace width="-4.981pt"/>`, `\begin{matrix}a & b\end{matrix}`},
		{"fenced matrix is inner", `<mi>𝑥</mi>` + thin + `<mrow><mo fence="true">[</mo><mspace width="-2.989pt"/>` +
			`<mtable>` + cellsAB + `</mtable><mspace width="-2.989pt"/><mo fence="true">]</mo></mrow>`,
			`x\begin{bmatrix}a & b\end{bmatrix}`},
	}
	checkWrites(t, cases)
}

func checkWrites(t *testing.T, cases []struct{ name, body, want string }) {
	t.Helper()
	for _, test := range cases {
		formula, err := Write(inline(test.body))
		if err != nil || formula.Text != test.want {
			t.Errorf("%s: Write = %q, %v; want %q", test.name, formula.Text, err, test.want)
		}
	}
}

// relbarArrow is \xrightarrow's fill: the bar \relbar and the arrow, with its kerns.
const relbarArrow = `<mpadded height="3.655041pt" width="10.867420pt"><mrow><mpadded width="0pt"><mspace width="0.000000pt"/>` +
	`</mpadded><mpadded width="0pt"><mrow><mstyle displaystyle="true" scriptlevel="0"><mpadded width="7.748744pt"><mrow>` +
	`<mpadded width="0pt"><mpadded depth="0.830215pt" height="5.811535pt" width="7.748744pt"><mpadded width="0pt"><mrow>` +
	`<mstyle displaystyle="true" scriptlevel="0"><mo>−</mo></mstyle></mrow></mpadded></mpadded></mpadded></mrow></mpadded>` +
	`<mspace width="-0.389em"/><mspace width="-0.389em"/><mo lspace="0" rspace="0">→</mo></mstyle></mrow></mpadded></mrow></mpadded>`

// TestWriteStackedAndText covers relations stacked by the author (\xrightarrow, \underset{x}{=},
// \mathop{\to}\limits made a relation), an accent over a word, a hyphen in an operator name and
// text that luamml split at a kern (math first, a space of its own), and \big around an empty group.
func TestWriteStackedAndText(t *testing.T) {
	thick := `<mspace width="0.278em"/>`
	cases := []struct{ name, body, want string }{
		{"extensible arrow", `<mi>𝑎</mi>` + thick + `<mover>` + relbarArrow + `<mrow><mi>𝑥</mi>` + thin + `</mrow></mover>` +
			thick + `<mi>𝑏</mi>`, `a\xrightarrow{x}b`},
		{"extensible arrow below", `<munderover>` + relbarArrow + `<mrow><mi>𝑦</mi>` + thin + `</mrow><mrow><mi>𝑥</mi>` + thin +
			`</mrow></munderover>`, `\xrightarrow[y]{x}`},
		{"underset relation", `<mi>𝑎</mi>` + thick + `<munder><mo lspace="0" rspace="0">=</mo><mi>𝑥</mi></munder>` + thick +
			`<mi>𝑏</mi>`, `a\underset{x}{=}b`},
		{"operator made a relation", `<mi>ℎ</mi><mover><mo lspace="0.278em" movablelimits="false" rspace="0.278em">→</mo>` +
			`<mo>≈</mo></mover><mi>𝐶</mi>`, `h\mathrel{\mathop{\to}\limits^{\approx}}C`},
		{"accent over a word", `<msub><mover>` + `<mrow><mi mathvariant="normal">d</mi><mi mathvariant="normal">x</mi></mrow>` +
			`<mo>‾</mo></mover><mi>𝑖</mi></msub>` + thin + `<mi>𝑑</mi>`, `{\overline{\mathrm{dx}}}_i\,d`},
		{"hyphen in an operator name", `<msup><mi mathvariant="normal">Δ</mi><mrow><mi mathvariant="normal">n</mi>` +
			`<mo>-</mo><mi mathvariant="normal">q</mi></mrow></msup>`, `\Delta^{\operatorname{n-q}}`},
		{"text split at a kern", `<mi>𝑎</mi><mpadded width="38.2pt"><mrow><mpadded lspace="3.3pt" width="0pt"><mtext>for ev</mtext>` +
			`</mpadded><mpadded lspace="24.6pt" width="0pt"><mtext>ery</mtext></mpadded></mrow></mpadded><mi>𝑏</mi>`,
			`a\text{ for every}b`},
		{"text starting with math", `<mpadded width="30pt"><mrow><mpadded width="0pt"><mrow><mstyle><mi>𝑛</mi></mstyle></mrow>` +
			`</mpadded><mpadded lspace="5pt" width="0pt"><mspace width="3.3pt"/></mpadded><mpadded lspace="8pt" width="0pt">` +
			`<mtext>items</mtext></mpadded></mrow></mpadded>`, `\text{$n$ items}`},
		{"big with an empty group", `<mpadded depth="3.486985pt" height="8.468290pt" width="4.151120pt"><mrow/>` +
			`<mpadded width="0pt"><mrow><mrow/><mo lspace="0" rspace="0">]</mo>` +
			`<mspace depth="0.000000pt" height="11.955123pt" width="0.000000pt"/></mrow></mpadded></mpadded>`, `\bigr]`},
	}
	checkWrites(t, cases)
}

// relbar is \relbar, the bar of \longrightarrow.
const relbar = `<mpadded width="7.748744pt"><mrow><mpadded width="0pt"><mpadded depth="0.830215pt" height="5.811535pt" ` +
	`width="7.748744pt"><mpadded width="0pt"><mrow><mo>−</mo></mrow></mpadded></mpadded></mpadded></mrow></mpadded>`

// TestWriteSymbolsAndClasses covers negated relations, composite commands (with the spaces TeX
// puts around them), characters the author gave another class, a strut as an atom and an
// operator symbol that carries its own spaces.
func TestWriteSymbolsAndClasses(t *testing.T) {
	thick := `<mspace width="0.278em"/>`
	sum := `<munder><mo lspace="0.167em" movablelimits="true" rspace="0.167em">∑</mo><mi>𝑖</mi></munder>`
	cases := []struct{ name, body, want string }{
		{"not in", `<mi>𝑎</mi><mo lspace="0.278em" rspace="0.278em">∉</mo><mi>𝑏</mi>`, `a\not\in b`},
		{"not before a relation", `<mi>𝑎</mi><mo lspace="0.278em" rspace="0">` + "̸" + `</mo><mo lspace="0" rspace="0.278em">∋</mo><mi>𝑏</mi>`,
			`a\not\ni b`},
		{"long arrow", `<mi>𝑥</mi>` + thick + relbar + `<mspace width="-0.167em"/><mo lspace="0" rspace="0.278em">→</mo><mi>𝑦</mi>`,
			`x\longrightarrow y`},
		{"at as a binary operator", `<mi mathvariant="normal">a</mi><mo lspace="0.222em" rspace="0.222em">@</mo>` +
			`<mi mathvariant="normal">b</mi>`, `\mathrm{a}\mathbin{@}\mathrm{b}`},
		{"strut after an inner atom", parenthesX + thin + `<mspace depth="0pt" height="25.5pt" width="0.000000pt"/>`,
			`\left(x\right)\rule[-0bp]{0bp}{25.5bp}`},
		{"operator spaces on its symbol", sum + thin + sinWord + thin + `<mi>𝑥</mi>`, `\sum_i\,\sin x`},
	}
	checkWrites(t, cases)
}

// TestWriteEqnarray writes eqnarray's rows without the empty group align sets before a cell.
func TestWriteEqnarray(t *testing.T) {
	cell := func(align, body string) string {
		return `<mtd columnalign="` + align + `"><mrow>` + body + `</mrow></mtd>`
	}
	mathml := mathNS + ` display="block"><mtable class="eqnarray"><mtr><mtd intent=":equation-label"><mtext></mtext></mtd>` +
		cell("right", `<mi>𝑥</mi>`) + cell("center", `<mi>≤</mi>`) + cell("left", `<mi>𝑦</mi>`) + `</mtr></mtable></math>`
	formula, err := Write(mathml)
	if err != nil || formula.Environment != "eqnarray*" || formula.Text != `x & \le & y` {
		t.Errorf("Write = %+v, %v", formula, err)
	}
}

// TestEnvironmentsInSchemas keeps every environment the formula TeX may name in the document schemas'
// formula_tex enum (papers4/0879's eqnarray* failed the export's schema check).
func TestEnvironmentsInSchemas(t *testing.T) {
	for _, name := range []string{"document-node", "paper-document-node"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "..", "schemas", name+".schema.json"))
		if err != nil {
			t.Fatal(err)
		}
		var schema struct {
			Properties struct {
				FormulaTeX struct {
					Properties struct {
						Environment struct {
							Enum []string `json:"enum"`
						} `json:"environment"`
					} `json:"properties"`
				} `json:"formula_tex"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(data, &schema); err != nil {
			t.Fatal(err)
		}
		enum := schema.Properties.FormulaTeX.Properties.Environment.Enum
		if len(enum) == 0 {
			t.Fatalf("%s: no formula_tex.environment enum", name)
		}
		for _, environment := range alignmentEnvironments {
			if !slices.Contains(enum, environment) {
				t.Errorf("%s: formula_tex.environment lacks %s", name, environment)
			}
		}
	}
}

// TestWriteMultlineGap leaves multline's own indent of its first and last rows to multline.
func TestWriteMultlineGap(t *testing.T) {
	gap := `<mspace width="9.963pt"/>`
	row := func(align, body string) string {
		return `<mtr><mtd intent=":equation-label"><mtext></mtext></mtd><mtd columnalign="` + align + `"><mrow>` +
			`<mstyle displaystyle="true" scriptlevel="0">` + body + `</mstyle></mrow></mtd></mtr>`
	}
	mathml := mathNS + ` display="block"><mtable class="multline">` + row("left", gap+`<mi>𝑎</mi>`) +
		row("right", `<mi>𝑏</mi>`+gap) + `</mtable></math>`
	formula, err := Write(mathml)
	if err != nil || formula.Environment != "multline*" || formula.Text != "a \\\\\nb" {
		t.Errorf("Write = %+v, %v", formula, err)
	}
}
