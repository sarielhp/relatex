package cli

import (
	"strings"

	"github.com/sarielhp/clihelp"
)

// example is one command line (written with the program's name) and what it does.
func example(line, description string) clihelp.Example {
	return clihelp.Example{Line: Name + " " + line, Description: description}
}

// rootExamples are the program's own examples (-E, --examples): converting a paper in the ways people ask for.
func rootExamples() []clihelp.Example {
	return []clihelp.Example{
		example("paper.tex --typst", "Convert one paper to Typst; the result is in typst/ here"),
		example("paper.tex --html --epub", "HTML and EPUB, and nothing else"),
		example("paper.zip", "A ZIP archive as the input (an archive always builds from its own files); the configured formats, Markdown and HTML"),
		example("paper/ --main main.tex", "A multi-file paper in a directory: name the main file when more than one .tex could be it"),
		example("book.tex --html --split chapters", "A book (a document with \\chapter): a contents page and one HTML file per chapter"),
		example("paper.tex --pdf-ua", "The tagged PDF (PDF/UA-2), validated by veraPDF when installed"),
		example("paper.tex --output reading", "Put the results in reading/ (reading/html/ and so on) instead of the current directory"),
		example("paper.tex --no-cache", "Run every stage afresh and write no stage cache"),
		example("paper.tex --force export,publish", "Rerun the export and publish stages (and the ones after them) on the cached earlier stages"),
		example("convert --resume ATTEMPT", "Continue an earlier attempt directory (the path of its run report's folder) with its settings and finished stages"),
		example("paper.tex --synctex", "Also write the word-level SyncTeX file beside the PDF"),
		example("paper.tex --report", "Print the JSON run report on stdout, as tools read it"),
		example("lint --accessibility paper/", "List what only the author can supply for an accessible export"),
		example("export json-gz/document.json.gz --format markdown --output markdown", "Write another format from a finished conversion"),
		example("inspect json-gz/document.json.gz --node ID", "Look at one node of a finished conversion's document"),
		example("capabilities", "What this build implements, as JSON"),
		example("doctor", "Check the programs relatex starts and how to install a missing one"),
		example("completion install", "Install tab completion for your shell"),
	}
}

// commandExamples are the examples of each command, by its words.
func commandExamples() map[string][]clihelp.Example {
	return map[string][]clihelp.Example{
		"convert": {
			example("convert paper.tex --typst", "One paper to Typst"),
			example("convert paper.zip --main paper.tex --format html --output reading", "A ZIP, its main file, HTML only, into reading/"),
			example("convert paper.tex --html --epub", "HTML and EPUB"),
			example("convert book.tex --html --split none", "A book as one HTML page"),
			example("convert paper.tex --pdf-ua", "The tagged PDF"),
			example("convert paper.tex --accessible", "The accessibility conversion: HTML, EPUB, the tagged PDF and accessibility.html"),
			example("convert paper/ --main main.tex --hermetic", "A multi-file paper built from its own files only, not your TEXINPUTS"),
			example("convert paper.tex --no-cache", "Run every stage afresh"),
			example("convert paper.tex --force layout", "Rerun the layout stage and the ones after it"),
			example("convert --resume ATTEMPT", "Continue an earlier attempt"),
			example("convert paper.tex --synctex", "Write the SyncTeX file from the word map"),
			example("convert paper.tex --no-layout", "Skip the page graphics (faster, no word-to-source map)"),
			example("convert paper.tex --all", "Every format but the separate builds"),
			example("convert paper.tex --report", "The JSON run report on stdout"),
		},
		"replay": {
			example("replay ATTEMPT", "Continue an earlier attempt with its settings, reusing its finished stages"),
			example("replay ATTEMPT --force export,publish", "Rerun the export and publish stages"),
		},
		"export": {
			example("export json-gz/document.json.gz --output markdown --format markdown", "Markdown from a finished conversion"),
			example("export json-gz/document.json.gz --output typst --format typst-source", "Typst source"),
			example("export json-gz/document.json.gz --output epub --format epub", "An EPUB package"),
		},
		"reconstruct": {
			example("reconstruct json-gz/document.json.gz --output pdf", "Rebuild the PDF of a finished conversion"),
		},
		"validate": {example("validate json-gz/document.json.gz", "Check a bundle against its schema and report what it holds")},
		"inspect": {
			example("inspect json-gz/document.json.gz --node ID", "One node of the document"),
			example("inspect json-gz/document.json.gz --word ID", "One word, with its place on the page and in the source"),
			example("inspect formula.mathml --mathml --projection tex", "A standalone MathML file, shown as TeX"),
		},
		"sync": {
			example("sync json-gz/document.json.gz --source main.tex --line 42", "The words a source line produced"),
			example("sync json-gz/document.json.gz --page 1 --x 120 --y 300", "The source of a point on a PDF page"),
		},
		"capabilities": {example("capabilities", "What this build implements, as JSON")},
		"layers": {
			example("layers ATTEMPT", "An attempt's stages and files"),
			example("layers ATTEMPT --stage engine", "One stage's receipt"),
		},
		"config": {
			example("config", "The settings a conversion here would use, and their files"),
			example("config --init", "Write the global configuration file from the template"),
			example("config paper/ --init-project", "Write paper/'s own configuration file, every setting commented out"),
		},
		"alt": {example("alt review paper/", "Write the page of every figure with its description")},
		"alt draft": {
			example("alt draft paper/", "Draft descriptions for the figures that have none (needs alt_text.command)"),
			example("alt draft paper/ --allow-upload --limit 5", "Approve sending the figures and draft at most five"),
		},
		"alt review": {example("alt review paper/", "Write alt-review.html here: the review page of the figures")},
		"alt accept": {
			example("alt accept --all paper/", "Mark every draft reviewed"),
			example("alt accept fig1 fig2 paper/", "Mark two drafts reviewed"),
		},
		"alt import": {example("alt import saved.toml paper/", "Take the review page's saved decisions into the sidecar")},
		"alt set": {
			example("alt set fig1 --alt \"A binary tree of depth three.\"", "Write a figure's alternative"),
			example("alt set fig1 --long \"The root splits into two subtrees.\" paper/", "Write its short description"),
		},
		"alt geometry": {
			example("alt geometry --pdf figure.pdf --page 1", "What a figure PDF page draws"),
			example("alt geometry fig1 paper/ --json", "The same for a figure of the last conversion, as JSON"),
		},
		"lint": {
			example("lint --accessibility paper/", "What only the author can supply, read from the source"),
			example("lint --accessibility paper/ --patch fixes.diff", "Also write the suggestions as a unified diff (patch -p1)"),
			example("lint --accessibility paper/ --annotate review/ --review-page page/", "A marked copy of the source and an HTML review page"),
			example("lint --accessibility paper/ --strict --json", "JSON items, exit status 1 when anything is listed"),
		},
		"synctex": {
			example("synctex paper/ --main main.tex --for paper.pdf", "Give the PDF another tool built relatex's word-level SyncTeX file"),
			example("synctex paper/ --main main.tex --for paper.pdf --replace-pdf", "Replace a preview built by another engine with relatex's pair"),
		},
		"demo-sync": {
			example("demo-sync -o demo/", "A page with the last conversion beside its LaTeX source, a click on a word finding the other"),
		},
		"tag-pdf": {example("tag-pdf --web-report accessibility.json", "Tag a finished conversion's PDF, judged against the web claim")},
		"doctor":  {example("doctor", "The programs relatex starts, their versions and how to install a missing one"), example("doctor --json", "The same as JSON"), example("doctor paper/", "The TeX files a paper needs that TeX Live lacks, and its bibliography program"), example("doctor paper.zip --tlmgr", "Also ask tlmgr which TeX Live package has each missing file (slow, needs the network)")},
		"bibclean": {
			example("bibclean --all refs.bib > clean.bib", "Clean every entry for Typst's bibliography engine"),
			example("bibclean --keys knuth84,lamport94 --report report.json refs.bib", "Only these entries, the per-entry report in a file"),
		},
	}
}

// examplesFor are the examples of the command at path.
func examplesFor(path []string) []clihelp.Example {
	return commandExamples()[strings.Join(path, " ")]
}
