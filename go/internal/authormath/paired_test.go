package authormath

import (
	"testing"

	"github.com/sarielhp/relatex/internal/authortex"
	"github.com/sarielhp/relatex/internal/rec"
)

// TestPairedDelimiters: the meanings TeX Live's mathtools gives \DeclarePairedDelimiter,
// \DeclarePairedDelimiterX and \DeclarePairedDelimiterXPP macros (decision 114), as the collector
// reports them, keep the plain form's body.
func TestPairedDelimiters(t *testing.T) {
	choice := func(name string) string {
		return `->\@ifstar {\@nameuse {MT_delim_\MH_cs_to_str:N \` + name + ` _star:}}{\@nameuse {MT_delim_\MH_cs_to_str:N \` +
			name + ` _nostar:}}`
	}
	wrapper := func(name string) string {
		return `\@nameuse {MT_delim_\MH_cs_to_str:N \` + name + ` _star_wrapper:nnn}`
	}
	records := []rec.Map{}
	add := func(name, star string) {
		records = append(records, rec.Map{"name": name, "meaning": `->\protect \` + name + `  `},
			rec.Map{"name": name + " ", "meaning": choice(name)}, rec.Map{"name": name + "* ", "meaning": star})
	}
	add("abs", `#1->`+wrapper("abs")+`{\left \lvert }{#1}{\right \rvert }`)
	add("bars", `#1->`+wrapper("bars")+`{\left |}{#1}{\right |}`)
	add("inner", `#1#2->\begingroup \def \delimsize {\middle }`+wrapper("inner")+`{\left \langle }{#1,#2}{\right \rangle }\endgroup `)
	add("Prob", `#1->\begingroup \def \delimsize {\middle }\Pr `+wrapper("Prob")+`{\left [}{#1}{\right ]}\endgroup `)
	add("given", `#1#2->\begingroup \def \delimsize {\middle }`+wrapper("given")+`{\left (}{#1\delimsize \vert #2}{\right )}\endgroup `)
	add("broken", `#1->\begingroup \def \delimsize {\middle }`+wrapper("broken")+`{\left (}{#1}{\right )}`)
	kept, rejected := classify(records)
	want := map[string]Macro{
		"abs": {Parameters: 1, Body: `\mathopen\lvert #1\mathclose\rvert`,
			Paired: Paired{Open: `\lvert`, Close: `\rvert`, Inner: "#1"}},
		"bars": {Parameters: 1, Body: `\mathopen|#1\mathclose|`, Paired: Paired{Open: "|", Close: "|", Inner: "#1"}},
		"inner": {Parameters: 2, Body: `\mathopen\langle #1,#2\mathclose\rangle`,
			Paired: Paired{Open: `\langle`, Close: `\rangle`, Inner: "#1,#2"}},
		"Prob": {Parameters: 1, Body: `\Pr\mathopen[#1\mathclose]`, Paired: Paired{Before: `\Pr`, Open: "[", Close: "]", Inner: "#1"}},
	}
	for name, macro := range want {
		if got, ok := kept.Get(name); !ok || got != macro {
			t.Errorf("%s: kept %+v (%v), want %+v", name, got, ok, macro)
		}
	}
	for _, name := range []string{"given", "broken"} {
		if _, ok := rejected.Get(name); !ok {
			t.Errorf("%s is not rejected", name)
		}
	}
	kept.Set("plain", Macro{Parameters: 1, Body: `\lvert #1\rvert`})
	document := usedBy(kept, []string{`\abs*{x} \bars{y} \inner{a}{b} \Prob[\big]{A} \plain{z}`})
	// The verification declares each paired delimiter again as the paper did, the plain macro not.
	wantBlock := "\n" + `\let\Prob\relax\DeclarePairedDelimiterXPP{\Prob}[1]{\Pr}{[}{]}{}{#1}` + "\n" +
		`\let\abs\relax\DeclarePairedDelimiter{\abs}{\lvert}{\rvert}` + "\n" +
		`\let\bars\relax\DeclarePairedDelimiter{\bars}{|}{|}` + "\n" +
		`\let\inner\relax\DeclarePairedDelimiterXPP{\inner}[2]{}{\langle}{\rangle}{}{#1,#2}`
	if got := PairedBlock(document); got != wantBlock {
		t.Errorf("paired block\n got %q\nwant %q", got, wantBlock)
	}
	if got := PairedBlock(rec.Map{"plain": rec.Map{"parameters": int64(1), "body": "x"}}); got != "" {
		t.Errorf("no paired delimiter, block %q", got)
	}
}

// TestPairedVariant: Markdown ships no formula that uses a paired delimiter starred or sized, for
// its macro block defines only the plain form (decision 114, amendment 2026-11-13).
func TestPairedVariant(t *testing.T) {
	kept := rec.Map{"abs": rec.Map{"parameters": int64(1), "body": `\mathopen|#1\mathclose|`, "paired": rec.Map{"open": "|"}},
		"plain": rec.Map{"parameters": int64(1), "body": `|#1|`}}
	cases := map[string]bool{`\abs{x}`: false, `\abs*{x}`: true, `\abs [\big]{x}`: true, `\abs{x} + \abs[\Big]{y}`: true,
		`\absval*{x}`: false, `\plain*{x}`: false, `\abs{[0,1]}`: false}
	for text, want := range cases {
		if got := authortex.PairedVariant(text, kept); got != want {
			t.Errorf("%s: %v, want %v", text, got, want)
		}
	}
}
