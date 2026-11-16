package bibstyle

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A biblatex build: the package that loads biblatex comes first, then the first .bbx; the .bcf names
// the data sources.
func TestBiberRequestNamesTheStylePackageThenTheBbx(t *testing.T) {
	output, working := t.TempDir(), t.TempDir()
	write(t, filepath.Join(output, "paper.bcf"), `<bcf:controlfile><bcf:bibdata section="0">`+
		`<bcf:datasource type="file" datatype="bibtex" glob="false">refs.bib</bcf:datasource>`+
		`<bcf:datasource type="file" datatype="bibtex" glob="false">more&amp;.bib</bcf:datasource></bcf:bibdata></bcf:controlfile>`)
	write(t, filepath.Join(working, "mystyle.sty"), "% \\usepackage{biblatex} in a comment only\n\\RequirePackage[style=alphabetic]{biblatex}\n")
	write(t, filepath.Join(working, "relatex-external.sty"), "\\RequirePackage{biblatex}\n")
	write(t, filepath.Join(working, "other.sty"), "% \\RequirePackage{biblatex}\n\\RequirePackage{xcolor}\n")
	write(t, filepath.Join(output, "paper.fls"), "PWD /x\nINPUT ./relatex-external.sty\nINPUT ./other.sty\nINPUT ./mystyle.sty\n"+
		"INPUT /texmf/biblatex.sty\nINPUT /texmf/bbx/alphabetic.bbx\nINPUT /texmf/bbx/standard.bbx\n")
	request := ReadRequest(output, "paper", working)
	if request.Processor != Biber || !slices.Equal(request.Styles, []string{"mystyle", "alphabetic"}) ||
		!slices.Equal(request.Databases, []string{"refs.bib", "more&.bib"}) {
		t.Errorf("request %+v", request)
	}
}

func TestBibtexRequestReadsTheAux(t *testing.T) {
	output := t.TempDir()
	write(t, filepath.Join(output, "paper.aux"), "\\relax\n\\citation{a}\n\\bibstyle{plainnat}\n\\bibdata{refs, shortcuts}\n")
	request := ReadRequest(output, "paper")
	if request.Processor != BibTeX || !slices.Equal(request.Styles, []string{"plainnat"}) ||
		!slices.Equal(request.Databases, []string{"refs", "shortcuts"}) {
		t.Errorf("request %+v", request)
	}
	if empty := ReadRequest(t.TempDir(), "paper"); empty.Processor != "" {
		t.Errorf("no build, request %+v", empty)
	}
}

// The library is the path's last entry: it answers when kpsewhich finds nothing; a name with a
// directory is never looked up.
func TestLibraryIsTheLastEntry(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, Library, "house.typ"), "#let style = none\n")
	style, _, ok := Search{Root: root, Directory: t.TempDir()}.Find([]string{"../house", "missing", "house"})
	if !ok || style.Name != "house" || style.Origin != OriginLibrary || len(style.SHA256) != 64 {
		t.Errorf("style %+v %v", style, ok)
	}
	if _, _, ok := (Search{Root: root}).Find([]string{"plain"}); ok {
		t.Error("found a style the library does not have")
	}
}

// Along BSTINPUTS, kpathsea's order: the caller's directory first, then ".", the document's directory;
// both win over the library.
func TestBstInputsComeBeforeTheLibrary(t *testing.T) {
	kpsewhich, err := exec.LookPath("kpsewhich")
	if err != nil {
		t.Skip("kpsewhich is not on PATH")
	}
	root, document, caller := t.TempDir(), t.TempDir(), t.TempDir()
	write(t, filepath.Join(root, Library, "house.typ"), "library\n")
	write(t, filepath.Join(document, "house.typ"), "document\n")
	search := Search{Kpsewhich: kpsewhich, Directory: document, Root: root}
	if style, _, _ := search.Find([]string{"house"}); style.Origin != OriginPath || style.Path != filepath.Join(document, "house.typ") {
		t.Errorf("document directory: %+v", style)
	}
	write(t, filepath.Join(caller, "house.typ"), "caller\n")
	search.BstInputs = caller + ":"
	if style, path, _ := search.Find([]string{"house"}); style.Path != filepath.Join(caller, "house.typ") || path == "" {
		t.Errorf("BSTINPUTS: %+v (path %s)", style, path)
	}
}
