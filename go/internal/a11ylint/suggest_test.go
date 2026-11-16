package a11ylint

import (
	"path/filepath"
	"strings"
	"testing"
)

const englishProse = `We show that the method is correct and that it is fast. This is the first result of its kind, and
the proof is short. We also give an algorithm which can be used for these problems, and we compare it with the
previous work by other authors. The results are not surprising, but they are new, and it is good to have them.
`

const germanProse = `Wir zeigen, dass die Methode korrekt ist und dass sie nicht langsam ist. Das ist auch für die
Praxis wichtig, und wir werden den Beweis mit einer einfachen Konstruktion von der Grundlage aus führen. Die
Ergebnisse werden in dem nächsten Abschnitt mit den früheren Arbeiten verglichen, die sich auf das Problem beziehen.
`

const frenchProse = `Nous montrons que la méthode est correcte et que les résultats sont nouveaux. Dans cette partie,
nous donnons une preuve pour le cas général, avec les notations du chapitre précédent, et nous discutons les
limites de cette approche qui sont par ailleurs bien connues dans les travaux sur ce sujet, pour être complet.
`

const russianProse = `Мы показываем, что метод корректен и что результаты новые. В этой части мы даём доказательство
для общего случая и обсуждаем ограничения подхода, которые хорошо известны в литературе по этой теме.
`

func only(t *testing.T, items []Item, kind string) Item {
	t.Helper()
	found := []Item{}
	for _, item := range items {
		if item.Kind == kind {
			found = append(found, item)
		}
	}
	if len(found) != 1 {
		t.Fatalf("want one %s item, got %+v", kind, found)
	}
	return found[0]
}

// TestLanguageNoteNamesTheLineForTheDetectedLanguage: the exact declaration for the language the
// text reads as, placed where it belongs: a class option on the \documentclass line, lang in an
// existing \DocumentMetadata, babel's option when babel names none, polyglossia's default.
func TestLanguageNoteNamesTheLineForTheDetectedLanguage(t *testing.T) {
	cases := []struct {
		name, source, suggestion, place string
		line                            int
	}{
		{"english", "\\documentclass[11pt,\n  a4paper]{article}\n\\begin{document}\n" + englishProse + "\\end{document}\n",
			`\documentclass[english,11pt,`, PlaceInstead, 1},
		{"german", "\\documentclass{article}\n\\begin{document}\n" + germanProse + "\\end{document}\n",
			`\documentclass[ngerman]{article}`, PlaceInstead, 1},
		{"metadata", "\\DocumentMetadata{pdfstandard=ua-2,\n  tagging=on}\n\\documentclass{article}\n\\begin{document}\n" + frenchProse + "\\end{document}\n",
			`\DocumentMetadata{lang=fr, pdfstandard=ua-2, tagging=on}`, PlaceInstead, 1},
		{"babel without a language", "\\documentclass{article}\n\\usepackage[T1]{fontenc}\n\\usepackage{babel}\n\\begin{document}\n" + englishProse + "\\end{document}\n",
			`\usepackage[english]{babel}`, PlaceInstead, 3},
		{"polyglossia", "\\documentclass{article}\n\\usepackage{polyglossia}\n\\begin{document}\n" + russianProse + "\\end{document}\n",
			`\setdefaultlanguage{russian}`, PlaceAfter, 2},
		{"too little text", "\\documentclass{article}\n\\begin{document}\nHi.\n\\end{document}\n",
			`\documentclass[english]{article}`, PlaceInstead, 1},
	}
	for _, test := range cases {
		item := only(t, lint(t, writePaper(t, map[string]string{"main.tex": test.source}), Options{}, nil), "language")
		if item.Suggestion != test.suggestion || item.NoteLine != test.line || item.Place != test.place {
			t.Errorf("%s: suggestion %q at %d (%s), want %q at %d (%s)", test.name, item.Suggestion, item.NoteLine, item.Place,
				test.suggestion, test.line, test.place)
		}
		if !strings.Contains(item.Note, test.suggestion) {
			t.Errorf("%s: note %q", test.name, item.Note)
		}
	}
	declared := writePaper(t, map[string]string{"main.tex": "\\documentclass{article}\n\\usepackage[main=english]{babel}\n\\begin{document}\\end{document}\n"})
	if items := lint(t, declared, Options{}, nil); len(items) != 0 {
		t.Errorf("main=english: %+v", items)
	}
	short := only(t, lint(t, writePaper(t, map[string]string{"main.tex": cases[5].source}), Options{}, nil), "language")
	if !strings.Contains(short.Note, "too little text") {
		t.Errorf("short note %q", short.Note)
	}
}

// TestFigureNoteIsAReadyToFillLine: the figure's own \includegraphics with the alt key, its sidecar
// key as the conversion names the file (shortened past 16 characters), \Description for acmart, and
// graphicx for a paper that loads graphics.
func TestFigureNoteIsAReadyToFillLine(t *testing.T) {
	source := "\\documentclass{article}\n\\usepackage[english]{babel}\n\\usepackage{graphicx}\n\\begin{document}\n" +
		"\\begin{figure}\n\\includegraphics[width=\\linewidth]{figs/throughput_vs_latency_all.pdf}\n\\end{figure}\n" +
		"\\includegraphics{plain}\n\\end{document}\n"
	items := lint(t, writePaper(t, map[string]string{"main.tex": source}), Options{}, nil)
	if len(items) != 2 {
		t.Fatalf("items %+v", items)
	}
	first, second := items[0], items[1]
	if first.Suggestion != `\includegraphics[width=\linewidth, alt={…}]{figs/throughput_vs_latency_all.pdf}` || first.NoteLine != 6 ||
		first.Key != "throughput_vs" || !strings.Contains(first.Note, "[figs.throughput_vs]") || !strings.Contains(first.Note, "relatex alt draft") {
		t.Errorf("first %+v", first)
	}
	if second.Suggestion != `\includegraphics[alt={…}]{plain}` || second.NoteLine != 8 {
		t.Errorf("second %+v", second)
	}
	acm := strings.Replace(source, "{article}", "[sigconf]{acmart}", 1)
	if item := lint(t, writePaper(t, map[string]string{"main.tex": acm}), Options{}, nil)[0]; !strings.Contains(item.Note, `\Description{…}`) {
		t.Errorf("acmart note %q", item.Note)
	}
	graphics := strings.Replace(source, "{graphicx}", "{graphics}", 1)
	if item := lint(t, writePaper(t, map[string]string{"main.tex": graphics}), Options{}, nil)[0]; !strings.Contains(item.Note, "load graphicx") {
		t.Errorf("graphics note %q", item.Note)
	}
}

// TestDrawingNoteIsAtTheTikzpicture: a drawn figure is placed at its tikzpicture, with the tagging
// project's alt key added to its options (written under it in the copy); a tikzpicture that has
// alt needs nothing more, since the export reads it too (decision 111), unless the alt is the
// annotated copy's placeholder.
func TestDrawingNoteIsAtTheTikzpicture(t *testing.T) {
	source := "\\documentclass[english]{article}\n\\begin{document}\n\\begin{figure}\n\\centering\n" +
		"\\begin{tikzpicture}[scale=2]\n\\draw (0,0) -- (1,1);\n\\end{tikzpicture}\n\\end{figure}\n" +
		"\\begin{figure}\\begin{tikzpicture}[alt={A line}]\\draw (0,0) -- (1,1);\\end{tikzpicture}\\end{figure}\n" +
		"\\begin{figure}\\begin{tikzpicture}[alt={TODO: describe}]\\draw (0,0) -- (1,1);\\end{tikzpicture}\\end{figure}\n\\end{document}\n"
	directory := writePaper(t, map[string]string{"main.tex": source})
	items := lint(t, directory, Options{}, nil)
	if len(items) != 2 {
		t.Fatalf("items %+v", items)
	}
	if items[0].Line != 5 || items[0].Suggestion != `\begin{tikzpicture}[scale=2, alt={…}]` || items[0].Place != PlaceInstead {
		t.Errorf("drawing %+v", items[0])
	}
	if items[1].Kind != "figure" || !strings.Contains(items[1].Message, "placeholder") || items[1].Line != 10 {
		t.Errorf("drawing with a placeholder alt %+v", items[1])
	}
	destination := filepath.Join(t.TempDir(), "review")
	annotate(t, directory, destination, Options{})
	want := "\\begin{tikzpicture}[scale=2]\n% \\begin{tikzpicture}[scale=2, alt={…}]  % relatex-a11y 1: uncomment it in place of the line above\n"
	if got := read(t, filepath.Join(destination, "main.tex")); !strings.Contains(got, want) {
		t.Errorf("copy:\n%s", got)
	}
}

// TestTableNoteNamesTheHeaderRow: tables are judged as the conversion reads header rows (a full
// rule under row 1 or 2 that sets them apart, or a bold first row over a body that is not), or by the tagging project's header-rows setting;
// the note names the row that looks like the header, the rule to add after it and the setting.
func TestTableNoteNamesTheHeaderRow(t *testing.T) {
	preamble := "\\documentclass[english]{article}\n\\usepackage{booktabs}\n\\begin{document}\n"
	cases := []struct {
		name, table, suggestion, note string
		line                          int // the note's line in the file; 0: no item
	}{
		{"bold first column too", "\\begin{tabular}{ll}\n\\toprule\n\\textbf{Method} & \\textbf{Time} \\\\\n\\textbf{A} & 1 \\\\\n\\textbf{B} & 2 \\\\\n\\bottomrule\n\\end{tabular}\n",
			`\midrule`, "“\\textbf{Method} & \\textbf{Time}” looks like the header (the first row, bold)", 6},
		{"grid", "\\begin{tabular}{|l|l|}\\hline\nName & Size \\\\ \\hline\nA & 1 \\\\ \\hline\nB & 2 \\\\ \\hline\n\\end{tabular}\n",
			`\hline`, "make the rule under it double", 5},
		{"double rule", "\\begin{tabular}{|l|l|}\\hline\nName & Size \\\\ \\hline\\hline\nA & 1 \\\\ \\hline\nB & 2 \\\\ \\hline\n\\end{tabular}\n", "", "", 0},
		{"booktabs", "\\begin{tabular}{ll}\\toprule\nName & Size \\\\ \\midrule\nA & 1 \\\\\nB & 2 \\\\ \\bottomrule\n\\end{tabular}\n", "", "", 0},
		{"two header rows", "\\begin{tabular}{lll}\\toprule\n & \\multicolumn{2}{c}{Size} \\\\ \\cmidrule(lr){2-3}\nName & min & max \\\\ \\midrule\nA & 1 & 2 \\\\\nB & 2 & 3 \\\\ \\bottomrule\n\\end{tabular}\n", "", "", 0},
		{"tagging setting", "\\tagpdfsetup{table/header-rows={1}}\n\\begin{tabular}{ll}\nName & Size \\\\\nA & 1 \\\\\nB & 2 \\\\\n\\end{tabular}\n", "", "", 0},
		{"bold first row", "\\begin{tabular}{ll}\n\\toprule\n\\textbf{Method} & \\textbf{Time} \\\\\nA & 1 \\\\\nB & 2 \\\\\n\\bottomrule\n\\end{tabular}\n", "", "", 0},
		{"one column", "\\begin{tabular}{l}\nName \\\\\nA \\\\\n\\end{tabular}\n", "", "", 0},
	}
	for _, test := range cases {
		items := lint(t, writePaper(t, map[string]string{"main.tex": preamble + test.table + "\\end{document}\n"}), Options{}, nil)
		if test.line == 0 {
			if len(items) != 0 {
				t.Errorf("%s: items %+v", test.name, items)
			}
			continue
		}
		item := only(t, items, "table")
		if item.Line != 4 || item.NoteLine != test.line || item.Suggestion != test.suggestion || !strings.Contains(item.Note, test.note) ||
			!strings.Contains(item.Note, `\tagpdfsetup{table/header-rows={1}} before \begin on line 4 (it needs \DocumentMetadata{tagging=on})`) {
			t.Errorf("%s: %+v", test.name, item)
		}
	}
	// A table nested in a cell of another: the outer one ends at its own \end and its first row
	// holds the inner one's rows; the inner one has one column.
	nested := preamble + "\\begin{tabular}{ll}\\toprule\nName & \\begin{tabular}{c}a\\\\b\\end{tabular} \\\\ \\midrule\nA & 1 \\\\\nB & 2 \\\\\n\\end{tabular}\n\\end{document}\n"
	if items := lint(t, writePaper(t, map[string]string{"main.tex": nested}), Options{}, nil); len(items) != 0 {
		t.Errorf("nested: %+v", items)
	}
}
