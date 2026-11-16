package bibclean

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/text/unicode/norm"
)

func TestParserFeaturesAndSelection(t *testing.T) {
	input := `
% leading comment
@COMMENT{ignored section {with braces}}
@PREAMBLE{"\newcommand{\Org}{Algo}" # " Corp."}
@STRING{conf = "Proc. " # "{ACM}" # " Symp."}
@string{part = "Discrete"}

@INPROCEEDINGS{Child20,
  author = "Ada Lovelace",
  title = {A } # part # { Study},
  booktitle = conf,
  month = jan,
  year = 2020,
  crossref = {PARENT20},
}

@Proceedings{parent20,
  booktitle = conf,
  year = 2020,
  xdata = "series20",
}

@XDATA(series20,
  series = {LNCS},
  publisher = "Springer",
)

@article{Unused99,
  author = {Nobody},
  title = {Should be omitted},
  year = 1999,
}
`
	out, rep, err := CleanInputs([]Input{{Name: "test.bib", Data: input}}, Options{Keys: []string{"child20", "MissingKey"}})
	if err != nil {
		t.Fatalf("CleanInputs returned error: %v", err)
	}
	if rep.Converted != 3 || rep.Unconverted != 0 {
		t.Fatalf("got converted=%d unconverted=%d, want 3, 0", rep.Converted, rep.Unconverted)
	}
	if !reflect.DeepEqual(rep.MissingKeys, []string{"MissingKey"}) {
		t.Fatalf("missing keys = %v, want [MissingKey]", rep.MissingKeys)
	}
	wantSubstrings := []string{
		"@inproceedings{Child20,",
		"title = {A Discrete Study},",
		"booktitle = {Proc. {#[ACM]} Symp.},",
		"month = {January},",
		"@proceedings{parent20,",
		"@xdata{series20,",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("cleaned output missing %q\noutput:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Unused99") {
		t.Errorf("cleaned output should omit unselected entry Unused99")
	}
}

func TestAllMonthMacros(t *testing.T) {
	months := [][2]string{
		{"jan", "January"}, {"feb", "February"}, {"mar", "March"}, {"apr", "April"},
		{"may", "May"}, {"jun", "June"}, {"jul", "July"}, {"aug", "August"},
		{"sep", "September"}, {"oct", "October"}, {"nov", "November"}, {"dec", "December"},
	}
	for _, pair := range months {
		src := "@misc{m, month = " + pair[0] + "}\n"
		out, rep, err := CleanInputs([]Input{{Name: "m.bib", Data: src}}, Options{All: true})
		if err != nil || rep.Unconverted != 0 {
			t.Fatalf("%s failed: err=%v unconverted=%d", pair[0], err, rep.Unconverted)
		}
		if !strings.Contains(out, "month = {"+pair[1]+"}") {
			t.Errorf("month %s: got:\n%s", pair[0], out)
		}
	}
}

func TestMissingParentsAndMultiInput(t *testing.T) {
	file1 := `@string{pub = "First"}` + "\n" + `@book{b1, title = {T}, publisher = pub, crossref = {missingP}, xdata = {missingX}}`
	file2 := `@string{pub = "Second"}` + "\n" + `@book{b1, title = {Updated}, publisher = pub, crossref = {missingP}, xdata = {missingX}}`
	out, rep, err := CleanInputs([]Input{
		{Name: "one.bib", Data: file1},
		{Name: "two.bib", Data: file2},
	}, Options{Keys: []string{"b1"}})
	if err != nil {
		t.Fatalf("CleanInputs: %v", err)
	}
	if !strings.Contains(out, "publisher = {Second}") || strings.Contains(out, "First") {
		t.Errorf("expected second file override:\n%s", out)
	}
	if !reflect.DeepEqual(rep.MissingParents, []string{"missingP", "missingX"}) {
		t.Errorf("missing parents = %v, want [missingP missingX]", rep.MissingParents)
	}
}

func TestPreambleMacroRules(t *testing.T) {
	src := `
@preamble{"\newcommand{\Barany}{B{\'a}r{\'a}ny}"}
@preamble{"\providecommand{\Erdos}{Erd{\H o}s}"}
@preamble{"\def\Badoiu{B\u{a}doiu}"}
@preamble{"\providecommand{\CNFX}[1]{{\em(#1)}}"}
@preamble{"\providecommand{\CNFLICS}{\CNFX{LICS}}"}
@preamble{"\providecommand{\tildegen}{{\protect\raisebox{-0.1cm}{\symbol{'176}}}}"}
@preamble{"\providecommand{\urlSarielPaper}[1]{\href{http://example.org/#1}{#1}}"}
@preamble{"\providecommand{\loopA}{\loopB}"}
@preamble{"\providecommand{\loopB}{\loopA}"}
@preamble{"\providecommand{\Opt}[2][x]{#1-#2}"}
@preamble{"\def\Delim#1.{#1}"}
@preamble{"\def\Pair#1#2{#2 and #1}"}

@article{ok,
  author = {I. \Barany{} and P. \Erdos{} and M. \Badoiu{}},
  title = {Plain Preamble Macros},
  year = 2021,
}

@inproceedings{withargs,
  title = {LICS Paper},
  booktitle = "Proc. Logic \CNFLICS",
  note = {\urlSarielPaper{05/paper}},
}

@misc{nonplain,
  note = {See http://example.org/\tildegen{}foo},
}

@misc{recursive,
  note = {\loopA{}},
}

@misc{pair,
  note = {\Pair{One}{Two}},
}

@misc{optional,
  note = {\Opt{y}},
}

@misc{delimited,
  note = {\Delim a.},
}

@misc{missingarg,
  note = {Proc. \CNFX},
}
`
	out, rep, err := CleanInputs([]Input{{Name: "p.bib", Data: src}}, Options{All: true})
	if err != nil {
		t.Fatalf("CleanInputs: %v", err)
	}
	if rep.Converted != 3 || rep.Unconverted != 5 {
		t.Fatalf("converted=%d unconverted=%d, want 3, 5\nreport: %+v", rep.Converted, rep.Unconverted, rep)
	}
	// Decision 173: a macro's plain mandatory arguments are substituted, also through another macro.
	for _, want := range []string{
		"author = {I. Bárány and P. Erdős and M. Bădoiu},",
		"booktitle = {Proc. Logic #emph[(LICS)]},",
		`note = {#link("http://example.org/05/paper")[05/paper]}`,
		"note = {Two and One}",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("preamble expansion missing %q:\n%s", want, out)
		}
	}
	byKey := map[string]EntryReport{}
	for _, entry := range rep.Entries {
		byKey[entry.Key] = entry
	}
	for key, name := range map[string]string{"optional": `\Opt`, "delimited": `\Delim`, "missingarg": `\CNFX`} {
		if !hasReasonKind(byKey[key].Reasons, "macro_with_args", name) {
			t.Errorf("expected %s macro_with_args reason for %s, got %+v", name, key, byKey[key].Reasons)
		}
	}
	if !hasReasonKind(byKey["nonplain"].Reasons, "non_plain_macro", `\tildegen`) {
		t.Errorf("expected \\tildegen non_plain_macro reason, got %+v", byKey["nonplain"].Reasons)
	}
	if !hasReasonKind(byKey["recursive"].Reasons, "non_plain_macro", `\loopA`) {
		t.Errorf("expected \\loopA recursive reason, got %+v", byKey["recursive"].Reasons)
	}
}

func TestTextMarkupAccentsAndBraces(t *testing.T) {
	src := strings.ReplaceAll(`
@article{fmt,
  author = {J. H{\r a}stad and F. {\c c}elik and {\v s}imon and P. M{\"o}ller and K. {\ss}er and {\'{\i}}van and J. O{\l}ek and {\ae}{\oe}{\o}},
  title = {{\em Alpha}, \emph{Beta}, \textit{Gamma}, {\bf Delta}, \textbf{Epsilon}, \textsc{SmallCaps} --- {DNA} and {Har{-}Peled}},
  journal = {J.~Comp.\ --- @BT@@BT@Quoted'' and @BT@single' -- pages 1--9},
  note = {Escapes: \#1 \$5 \_foo \{br\} @ref [x] \textdollar\ and 100\%},
  url = {http://example.org/~user/a--b},
}
`, "@BT@", "`")
	out, rep, err := CleanInputs([]Input{{Name: "t.bib", Data: src}}, Options{All: true})
	if err != nil {
		t.Fatalf("CleanInputs: %v", err)
	}
	if rep.Converted != 1 || rep.Unconverted != 0 {
		t.Fatalf("expected clean conversion, got %+v", rep)
	}
	if !norm.NFC.IsNormalString(out) {
		t.Errorf("cleaned output is not NFC normalized")
	}
	wants := []string{
		"author = {J. Håstad and F. çelik and šimon and P. Möller and K. ßer and ívan and J. Ołek and æœø},",
		"title = {#emph[Alpha], #emph[Beta], #emph[Gamma], #strong[Delta], #strong[Epsilon], #smallcaps[SmallCaps] — {#[DNA]} and {#[Har{#[-]}Peled]}},",
		"journal = {J.~Comp. — “Quoted” and ‘single’ – pages 1–9},",
		"url = {http://example.org/~user/a--b}",
		`\\#1`,
		`#sym.dollar;`,
		`\\_foo`,
		`#sym.brace.l;br#sym.brace.r;`,
	}
	for _, want := range wants {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out)
		}
	}
}

func TestMathRules(t *testing.T) {
	src := `
@article{plainmath,
  title = {Bounds on $k$-sets, {$O(n^2)$}, and \(x + y_1 \)},
  year = 2020,
}

@article{cmdmath,
  title = {Within {$n^{1-\epsilon}$} in {${\bf R}\sp{n}$}},
  year = 2020,
}

@article{displaymath,
  title = {Display $$x+1$$ check},
  year = 2020,
}

@article{unclosedmath,
  title = {Broken $x+1 check},
  year = 2020,
}
`
	out, rep, err := CleanInputs([]Input{{Name: "math.bib", Data: src}}, Options{All: true})
	if err != nil {
		t.Fatalf("CleanInputs: %v", err)
	}
	if rep.Converted != 1 || rep.Unconverted != 3 {
		t.Fatalf("converted=%d unconverted=%d, want 1, 3\nreport=%+v", rep.Converted, rep.Unconverted, rep)
	}
	if !strings.Contains(out, `title = {Bounds on $k$-sets, {#[$O(n^2)$]}, and $x + y_1 $},`) {
		t.Errorf("plain math changed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, `title = {Within {#[$n^{1-\epsilon}$]} in {#[${\bf R}\sp{n}$]}},`) {
		t.Errorf("command math should remain untouched:\n%s", out)
	}
	byKey := map[string]EntryReport{}
	for _, entry := range rep.Entries {
		byKey[entry.Key] = entry
	}
	for _, cmd := range []string{`\epsilon`, `\bf`, `\sp`} {
		if !hasReasonKind(byKey["cmdmath"].Reasons, "math_command", cmd) {
			t.Errorf("cmdmath missing reason %s: %+v", cmd, byKey["cmdmath"].Reasons)
		}
	}
	if !hasReasonKind(byKey["displaymath"].Reasons, "math_command", "") {
		t.Errorf("displaymath missing math_command reason: %+v", byKey["displaymath"].Reasons)
	}
	if !hasReasonKind(byKey["unclosedmath"].Reasons, "unclosed_math", "") {
		t.Errorf("unclosedmath missing unclosed_math reason: %+v", byKey["unclosedmath"].Reasons)
	}
}

// Plain math that Typst would print differently from TeX stays unconverted for decision A's route.
func TestMathTypstReadsDifferently(t *testing.T) {
	cases := map[string]string{
		`$kd$-trees`:      "identifier",
		`the 17$th$ case`: "identifier",
		`$n2$`:            "identifier",
		`{$^*$}`:          "no base",
		`$x + _1$`:        "no base",
		`Vol$'$berg`:      "no base",
		`$R^{3}$`:         "braces",
		`${0,...,n}$ set`: "braces",
		`$n^{5/2}$`:       "braces",
		`$5/2$`:           "fraction",
		`$a<=b$`:          "shorthand",
		`$0...n$`:         "shorthand",
		`$a~b$`:           "space",
	}
	for title, want := range cases {
		src := "@article{m,\n  title = {" + title + "},\n  year = 2020,\n}\n"
		_, rep, err := CleanInputs([]Input{{Name: "m.bib", Data: src}}, Options{All: true})
		if err != nil {
			t.Fatalf("%s: CleanInputs: %v", title, err)
		}
		if want == "" {
			continue // a math command: unconverted for another reason
		}
		reasons := rep.Entries[0].Reasons
		if !hasReasonKind(reasons, "math_typst", "") || !strings.Contains(reasons[0].Detail, want) {
			t.Errorf("%s: reasons %+v, want math_typst mentioning %q", title, reasons, want)
		}
	}
	for _, plain := range []string{`$k$-sets`, `$O(n^2)$`, `$L_1$`, `$x^*$`, `$f'(x)$`, `$(p, q)$`, `$2n-2$`, `$a_1b$`} {
		src := "@article{p,\n  title = {" + plain + "},\n  year = 2020,\n}\n"
		_, rep, err := CleanInputs([]Input{{Name: "p.bib", Data: src}}, Options{All: true})
		if err != nil || rep.Converted != 1 {
			t.Errorf("%s: want converted, got %+v (%v)", plain, rep.Entries, err)
		}
	}
}

// A protecting group in a title is also a Typst content block, which survives citegeist; so is a name
// token with a space, hyphen or tie, whose braces citegeist also needs for parsing; other fields keep
// plain braces.
func TestTitleProtectionSurvivesAsContentBlock(t *testing.T) {
	src := `@article{p,
  author = {S. {Har{-}Peled}},
  title = {The {VC}(x) of {E}rdős},
  journal = {{SIAM} J.},
  note = {{Kept} plain},
  year = 2020,
}
`
	out, rep, err := CleanInputs([]Input{{Name: "p.bib", Data: src}}, Options{All: true})
	if err != nil || rep.Converted != 1 {
		t.Fatalf("CleanInputs: %+v %v", rep, err)
	}
	for _, want := range []string{
		"author = {S. {#[Har{#[-]}Peled]}},",
		"title = {The {#[VC]};(x) of {#[E]}rdős},",
		"journal = {{#[SIAM]} J.},",
		"note = {{Kept} plain},",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n%s", want, out)
		}
	}
}

// A tie in a name field stays a tie: biber reads J.~MacQueen as one family name (label [J M67],
// printed J. MacQueen), so the name parts citegeist sees must be the same (phase 2, decision 164).
func TestNameTieIsKept(t *testing.T) {
	src := `@article{mq,
  author = {J.~MacQueen and A.\nobreakspace{}B. Cee},
  editor = {E.~Ditor},
  title = {Some~methods},
  year = 1967,
}
`
	out, rep, err := CleanInputs([]Input{{Name: "tie.bib", Data: src}}, Options{All: true})
	if err != nil || rep.Converted != 1 {
		t.Fatalf("CleanInputs: %+v %v", rep, err)
	}
	for _, want := range []string{"author = {J.~MacQueen and A.~B. Cee},", "editor = {E.~Ditor},", "title = {Some~methods},"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n%s", want, out)
		}
	}
}

func TestGeometryFixture(t *testing.T) {
	fixturePath := filepath.Join("testdata", "geometry_hard.bib")
	expectedBibBytes, err := os.ReadFile(filepath.Join("testdata", "geometry_hard.expected.bib"))
	if err != nil {
		t.Fatal(err)
	}
	expectedJSONBytes, err := os.ReadFile(filepath.Join("testdata", "geometry_hard.expected.json"))
	if err != nil {
		t.Fatal(err)
	}
	var expectedReport Report
	if unmarshalErr := json.Unmarshal(expectedJSONBytes, &expectedReport); unmarshalErr != nil {
		t.Fatal(unmarshalErr)
	}

	out, rep, err := CleanFiles([]string{fixturePath}, Options{All: true})
	if err != nil {
		t.Fatalf("CleanFiles: %v", err)
	}
	if out != string(expectedBibBytes) {
		t.Fatalf("fixture .bib mismatch:\ngot:\n%s\nwant:\n%s", out, string(expectedBibBytes))
	}
	if !reflect.DeepEqual(rep, expectedReport) {
		t.Fatalf("fixture report mismatch:\ngot:  %+v\nwant: %+v", rep, expectedReport)
	}

	subOut, subRep, err := CleanFiles([]string{fixturePath}, Options{Keys: []string{"PT-PQRPD-11", "gl-fpamn-04"}})
	if err != nil {
		t.Fatalf("CleanFiles subset: %v", err)
	}
	if len(subRep.Entries) != 4 || len(subRep.MissingParents) != 0 {
		t.Fatalf("expected 4 entries (2 children + 2 parents), got entries=%d missingParents=%v",
			len(subRep.Entries), subRep.MissingParents)
	}
	for _, wantKey := range []string{"pt-pqrpd-11", "DBLP:conf/wads/2011", "gl-fpamn-04", "akt-jcdcg-05"} {
		if !strings.Contains(subOut, "{"+wantKey+",") {
			t.Errorf("subset output missing %q", wantKey)
		}
	}
}

func TestGeometryBibFull(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot resolve home dir: %v", err)
	}
	bibPath := filepath.Join(home, "papers", "bib", "geometry.bib")
	if _, statErr := os.Stat(bibPath); statErr != nil {
		t.Skipf("geometry.bib not readable (%s): %v", bibPath, statErr)
	}
	out, rep, err := CleanFiles([]string{bibPath}, Options{All: true})
	if err != nil {
		t.Fatalf("CleanFiles(geometry.bib): %v", err)
	}
	if rep.Converted+rep.Unconverted < 2000 || out == "" {
		t.Fatalf("unexpectedly small parse result: converted=%d unconverted=%d len=%d",
			rep.Converted, rep.Unconverted, len(out))
	}
	t.Logf("geometry.bib parsed cleanly: %d converted, %d unconverted (%d total)",
		rep.Converted, rep.Unconverted, len(rep.Entries))
}

func hasReasonKind(reasons []Reason, kind, name string) bool {
	for _, r := range reasons {
		if r.Kind == kind && (name == "" || r.Name == name) {
			return true
		}
	}
	return false
}

// A bare _ or ^ outside math is a TeX error that typesets a script; the entry stays unconverted.
func TestTextScriptIsUnconverted(t *testing.T) {
	src := "@proceedings{j,\n  title = {JCDCG_2004},\n  year = 2005,\n}\n"
	_, rep, err := CleanInputs([]Input{{Name: "j.bib", Data: src}}, Options{All: true})
	if err != nil || rep.Unconverted != 1 || !hasReasonKind(rep.Entries[0].Reasons, "text_script", "") {
		t.Fatalf("want text_script, got %+v %v", rep, err)
	}
	src = "@inproceedings{n,\n  title = {T},\n  note = {SIGGRAPH_1992},\n  year = 1992,\n}\n" +
		"@inproceedings{a,\n  title = {T},\n  acceptrate = {SOCG_1999_AR},\n  year = 1999,\n}\n"
	_, rep, err = CleanInputs([]Input{{Name: "n.bib", Data: src}}, Options{All: true})
	if err != nil || rep.Entries[0].Status != "unconverted" || rep.Entries[1].Status != "converted" {
		t.Fatalf("want the note unconverted and the unprinted acceptrate converted, got %+v %v", rep, err)
	}
}

// A bare & prints nothing in TeX (an alignment tab error): unconverted. Three periods stay three periods
// rather than becoming Typst's ellipsis.
func TestAlignmentTabAndThreePeriods(t *testing.T) {
	src := "@unpublished{s,\n  title = {T},\n  note = {submitted & withdrawn},\n  year = 2001,\n}\n" +
		"@misc{d,\n  title = {Don't Panic... just do it},\n  year = 1998,\n}\n"
	out, rep, err := CleanInputs([]Input{{Name: "s.bib", Data: src}}, Options{All: true})
	if err != nil || !hasReasonKind(rep.Entries[0].Reasons, "text_alignment", "") || rep.Entries[1].Status != "converted" {
		t.Fatalf("want s unconverted (text_alignment) and d converted, got %+v %v", rep, err)
	}
	if !strings.Contains(out, `title = {Don’t Panic.\\.. just do it},`) {
		t.Errorf("three periods not escaped:\n%s", out)
	}
}

// A // after a scheme is escaped too: a capitalized title (Https://) is no Typst autolink and would
// start a comment.
func TestAddressSlashesAreEscaped(t *testing.T) {
	src := "@inproceedings{u,\n  title = {https://doi.example.org/x/},\n  year = 2025,\n}\n"
	out, rep, err := CleanInputs([]Input{{Name: "u.bib", Data: src}}, Options{All: true})
	if err != nil || rep.Converted != 1 || !strings.Contains(out, `title = {https:\\//doi.example.org/x/},`) {
		t.Fatalf("want the slashes escaped, got %+v %v\n%s", rep, err, out)
	}
}

// biber splits a name list at AND in any case outside braces; citegeist only at the lower-case word. A
// TeX ' is a right quote, which Typst's smart quote would open at a word start.
func TestNameSeparatorsAndRightQuotes(t *testing.T) {
	src := `@inproceedings{g,
  author = {S. Gottschalk AND M. C. Lin And {Barnes AND Noble}},
  title = {Proc. SIGGRAPH '92},
  year = 1996,
}
`
	out, rep, err := CleanInputs([]Input{{Name: "g.bib", Data: src}}, Options{All: true})
	if err != nil || rep.Converted != 1 {
		t.Fatalf("CleanInputs: %+v %v", rep, err)
	}
	for _, want := range []string{"author = {S. Gottschalk and M. C. Lin and {#[Barnes AND Noble]}},", "title = {Proc. SIGGRAPH ’92},"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n%s", want, out)
		}
	}
}

// \textrm and \textsf change the family only: their text keeps the shape and weight around it
// (books/0003's ah-ncsa-10 "\textsf{SSPD}s", the CNFX macro's "{\em{\textrm{(#1)}}}").
func TestFamilyCommandsKeepTheirText(t *testing.T) {
	src := `@inproceedings{ah10,
  title = {New Constructions of \textsf{SSPD}s},
  booktitle = {Proc. {\em{\textrm{(SoCG)}}}},
}
@misc{bare, note = {\textsf}}
@misc{ok, title = {A \textrm{B} c}}
`
	out, rep, err := CleanInputs([]Input{{Name: "f.bib", Data: src}}, Options{All: true})
	if err != nil {
		t.Fatal(err)
	}
	// In a title the text is protected from the style's sentence case, as biblatex protects a command's argument.
	for _, want := range []string{"title = {New Constructions of {#[SSPD]}s},", "booktitle = {Proc. #emph[{#[{#[(SoCG)]}]}]}",
		"title = {A {#[B]} c}"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if rep.Converted != 2 || !hasReasonKind(rep.Entries[1].Reasons, "unknown_command", `\textsf`) {
		t.Errorf("report %+v", rep.Entries)
	}
}
