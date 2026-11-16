package cli

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/sarielhp/relatex/internal/a11ylint"
	"github.com/sarielhp/relatex/internal/command"
)

// paperNeed is one TeX file a paper's source asks for: the command that names it, the file TeX looks
// for (caption.sty, revtex4-2.cls, plain.bst) and where the source says so.
type paperNeed struct {
	Command string `json:"command"`
	Name    string `json:"name"`
	File    string `json:"file"`
	Where   string `json:"where"`
}

// bibUse is what the source says about its bibliography: biblatex (and its backend, "" for the
// default biber) or classic BibTeX (\bibliography without biblatex).
type bibUse struct {
	Biblatex bool   `json:"biblatex"`
	Backend  string `json:"backend,omitempty"`
	Bibtex   bool   `json:"bibtex"`
}

// program is the bibliography program the source needs: "" when it needs none.
func (b bibUse) program() string {
	switch {
	case b.Biblatex && (b.Backend == "" || b.Backend == "biber"):
		return "biber"
	case b.Biblatex, b.Bibtex:
		return "bibtex"
	}
	return ""
}

// paperMissing is a needed file nobody provides: the paper's own directory and TeX Live do not have it.
type paperMissing struct {
	paperNeed
	Package string `json:"tex_live_package,omitempty"`
	Hint    string `json:"install"`
}

// paperProgram is the bibliography program a paper needs and whether it was found.
type paperProgram struct {
	Name  string `json:"name"`
	Path  string `json:"path,omitempty"`
	Found bool   `json:"found"`
}

// paperReport is relatex doctor PAPER's answer.
type paperReport struct {
	Main         string         `json:"main"`
	FilesRead    []string       `json:"files_read"`
	Checked      int            `json:"checked"`
	InPaper      int            `json:"in_paper"`
	Missing      []paperMissing `json:"missing"`
	Bibliography *bibUse        `json:"bibliography,omitempty"`
	Program      *paperProgram  `json:"bibliography_program,omitempty"`
	LookupError  string         `json:"lookup_error,omitempty"`
	Ready        bool           `json:"ready"`
}

var (
	needCommand = regexp.MustCompile(`\\(documentclass|LoadClassWithOptions|LoadClass|usepackage|RequirePackageWithOptions|RequirePackage|` +
		`bibliographystyle|usetheme|usecolortheme|usefonttheme|useinnertheme|useoutertheme|usetikzlibrary|usepgfplotslibrary)\*?` +
		`\s*(?:\[([^\]]*)\])?\s*\{([^}]*)\}`)
	bibliographyCommand = regexp.MustCompile(`\\bibliography\s*\{`)
	backendOption       = regexp.MustCompile(`backend\s*=\s*([A-Za-z0-9]+)`)
)

// needExtension is the file name a command's argument stands for: its prefix and suffix.
var needExtension = map[string][2]string{
	"documentclass": {"", ".cls"}, "LoadClass": {"", ".cls"}, "LoadClassWithOptions": {"", ".cls"},
	"usepackage": {"", ".sty"}, "RequirePackage": {"", ".sty"}, "RequirePackageWithOptions": {"", ".sty"},
	"bibliographystyle": {"", ".bst"},
	"usetheme":          {"beamertheme", ".sty"}, "usecolortheme": {"beamercolortheme", ".sty"},
	"usefonttheme": {"beamerfonttheme", ".sty"}, "useinnertheme": {"beamerinnertheme", ".sty"},
	"useoutertheme":      {"beamerouttertheme", ".sty"},
	"usetikzlibrary":     {"tikzlibrary", ".code.tex"},
	"usepgfplotslibrary": {"pgfplotslibrary", ".code.tex"},
}

// scanNeeds reads the files' text (comments already blanked) for the classes, packages, themes,
// libraries and bibliography styles it names, and what it says about the bibliography.
func scanNeeds(files []*a11ylint.File) (needs []paperNeed, bib bibUse) {
	for _, file := range files {
		for _, match := range needCommand.FindAllStringSubmatchIndex(file.Text, -1) {
			kind, options, names := group(file.Text, match, 1), group(file.Text, match, 2), group(file.Text, match, 3)
			where := fmt.Sprintf("%s:%d", file.Path, file.Line(match[0]))
			noteBibliography(&bib, kind, options, names)
			needs = append(needs, namedNeeds(kind, names, where)...)
		}
		if bibliographyCommand.MatchString(file.Text) {
			bib.Bibtex = true
		}
	}
	if bib.Biblatex {
		bib.Bibtex = false
	}
	return needs, bib
}

func group(text string, match []int, index int) string {
	if match[2*index] < 0 {
		return ""
	}
	return text[match[2*index]:match[2*index+1]]
}

// noteBibliography records biblatex and its backend option.
func noteBibliography(bib *bibUse, kind, options, names string) {
	if kind != "usepackage" && kind != "RequirePackage" && kind != "RequirePackageWithOptions" {
		return
	}
	for _, name := range strings.Split(names, ",") {
		if strings.TrimSpace(name) != "biblatex" {
			continue
		}
		bib.Biblatex = true
		if found := backendOption.FindStringSubmatch(options); found != nil {
			bib.Backend = found[1]
		}
	}
}

// namedNeeds are the files a command's comma list names.
func namedNeeds(kind, names, where string) []paperNeed {
	parts := needExtension[kind]
	var needs []paperNeed
	for _, name := range strings.Split(names, ",") {
		name = strings.Join(strings.Fields(name), "")
		if name == "" || strings.ContainsAny(name, `#\`) {
			continue
		}
		file := name
		if !strings.HasSuffix(name, parts[1]) {
			file = parts[0] + name + parts[1]
		}
		needs = append(needs, paperNeed{Command: kind, Name: name, File: file, Where: where})
	}
	return needs
}

// kpsewhichFor is the TeX Live lookup program: beside the engine, else on PATH; "" when neither is there.
func kpsewhichFor(engine string) string {
	if engine != "" {
		if tool := command.Sibling(engine, "kpsewhich"); command.IsExecutable(tool) {
			return tool
		}
	}
	return command.Executable("kpsewhich")
}

// lookupTeXFiles is, for each file name, the path kpsewhich finds (absent when it finds none).
func lookupTeXFiles(kpsewhich string, files []string) (map[string]string, error) {
	found := map[string]string{}
	if len(files) == 0 {
		return found, nil
	}
	run := exec.Command(kpsewhich, append([]string{"-engine=luahbtex", "--"}, files...)...)
	run.Dir = os.TempDir()
	output, err := run.Output()
	if err != nil {
		if _, exited := err.(*exec.ExitError); !exited {
			return nil, err
		}
	}
	for _, line := range strings.Split(string(output), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			found[path.Base(filepath.ToSlash(line))] = line
		}
	}
	return found, nil
}

// inPaper reports whether the paper provides the file itself: beside the main file or beside one of
// the files read.
func inPaper(directory string, read []*a11ylint.File, file string) bool {
	directories := map[string]bool{directory: true}
	for _, source := range read {
		directories[filepath.Join(directory, filepath.FromSlash(path.Dir(source.Path)))] = true
	}
	for place := range directories {
		if info, err := os.Stat(filepath.Join(place, filepath.FromSlash(file))); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

// parseTlmgrSearch is the TeX Live package that owns file in the output of
// `tlmgr search --global --file /file` ("package:" lines, each followed by its indented paths).
func parseTlmgrSearch(output, file string) string {
	owner := ""
	for _, line := range strings.Split(output, "\n") {
		switch {
		case strings.HasPrefix(line, "tlmgr:"), strings.TrimSpace(line) == "":
		case line[0] != ' ' && line[0] != '\t':
			owner = strings.TrimSuffix(strings.TrimSpace(line), ":")
		case strings.HasSuffix(strings.TrimSpace(line), "/"+file) && owner != "":
			return owner
		}
	}
	return ""
}

// tlmgrOwner asks tlmgr (slow, and it needs the network) which package has file.
func tlmgrOwner(tlmgr, file string) string {
	run := exec.Command(tlmgr, "search", "--global", "--file", "/"+file)
	done := make(chan []byte, 1)
	go func() { output, _ := run.Output(); done <- output }()
	select {
	case output := <-done:
		return parseTlmgrSearch(string(output), file)
	case <-time.After(2 * time.Minute):
		_ = run.Process.Kill()
		return ""
	}
}

// installHint is how to get the missing file's package.
func installHint(name, owner string, searched bool) string {
	if owner == "" && searched {
		return "no TeX Live package found with this file (tlmgr search --global; offline, or not in TeX Live): look for \"" + name + "\" on CTAN"
	}
	if owner != "" {
		return "tlmgr install " + owner
	}
	return "tlmgr install " + strings.TrimSuffix(name, path.Ext(name)) + " (the TeX Live package may be named differently; --tlmgr looks it up)"
}

// paperSource is the directory and main file of the paper input: a .tex file, a directory or a ZIP
// (its source files are extracted into a scratch directory, returned to be removed).
func paperSource(input, requested string) (directory, main, scratch string) {
	options := map[string]any{}
	if requested != "" {
		options["main"] = requested
	}
	input = texFileInput(input, options)
	info, err := os.Stat(input)
	if err != nil {
		fail("cannot read " + input + ": " + err.Error())
	}
	directory = input
	if info.Mode().IsRegular() {
		scratch = command.Scratch("doctor-")
		if err = extractSources(input, scratch); err != nil {
			os.RemoveAll(scratch)
			fail(err.Error())
		}
		directory = scratch
	}
	main, _ = options["main"].(string)
	if main == "" {
		if main, err = a11ylint.FindMain(directory, ""); err != nil {
			os.RemoveAll(scratch)
			fail(err.Error())
		}
	}
	return directory, main, scratch
}

// extractSources writes the TeX source files of a ZIP (.tex, .sty, .cls, .bst, .def, .cfg, .clo) into
// directory, without the one folder that holds them all, refusing names that leave it.
func extractSources(archive, directory string) error {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return fmt.Errorf("%s is neither a directory, a .tex file nor a ZIP: %w", archive, err)
	}
	defer reader.Close()
	prefix := commonFolder(reader.File)
	for _, entry := range reader.File {
		name := path.Clean(strings.TrimPrefix(entry.Name, prefix))
		if entry.FileInfo().IsDir() || name == ".." || strings.HasPrefix(name, "../") || path.IsAbs(name) || !sourceExtension(name) {
			continue
		}
		if err := extractEntry(entry, filepath.Join(directory, filepath.FromSlash(name))); err != nil {
			return err
		}
	}
	return nil
}

func sourceExtension(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".tex", ".sty", ".cls", ".bst", ".def", ".cfg", ".clo", ".ltx":
		return true
	}
	return false
}

// commonFolder is "dir/" when every entry is inside the folder dir, else "".
func commonFolder(entries []*zip.File) string {
	prefix := ""
	for _, entry := range entries {
		head, _, nested := strings.Cut(entry.Name, "/")
		if !nested {
			return ""
		}
		if prefix != "" && prefix != head+"/" {
			return ""
		}
		prefix = head + "/"
	}
	return prefix
}

func extractEntry(entry *zip.File, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	source, err := entry.Open()
	if err != nil {
		return err
	}
	defer source.Close()
	out, err := os.Create(target)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, io.LimitReader(source, 64<<20))
	return err
}

// diagnosePaper reads the paper's sources and checks each file they ask for.
func diagnosePaper(input, requested string, withTlmgr bool) paperReport {
	directory, main, scratch := paperSource(input, requested)
	if scratch != "" {
		defer os.RemoveAll(scratch)
	}
	files, err := a11ylint.Load(directory, main)
	if err != nil {
		fail(err.Error())
	}
	report := paperReport{Main: main, Ready: true, Missing: []paperMissing{}}
	for _, file := range files {
		report.FilesRead = append(report.FilesRead, file.Path)
	}
	needs, bib := scanNeeds(files)
	engine, _ := findEngine("")
	report.checkNeeds(needs, directory, files, engine, withTlmgr)
	report.checkBibliography(bib, engine)
	return report
}

// checkNeeds looks each needed file up: in the paper's directories, then in TeX Live.
func (r *paperReport) checkNeeds(needs []paperNeed, directory string, files []*a11ylint.File, engine string, withTlmgr bool) {
	var wanted []string
	seen := map[string]bool{}
	for _, need := range needs {
		if seen[need.File] {
			continue
		}
		seen[need.File] = true
		r.Checked++
		if inPaper(directory, files, need.File) {
			r.InPaper++
			continue
		}
		wanted = append(wanted, need.File)
	}
	kpsewhich := kpsewhichFor(engine)
	if kpsewhich == "" && len(wanted) > 0 {
		r.LookupError = "kpsewhich was not found: TeX Live is missing (relatex doctor)"
		r.Ready = false
		return
	}
	found, err := lookupTeXFiles(kpsewhich, wanted)
	if err != nil {
		r.LookupError = "kpsewhich failed: " + err.Error()
		r.Ready = false
		return
	}
	tlmgr := ""
	if withTlmgr {
		tlmgr = tlmgrFor(engine)
	}
	reported := map[string]bool{}
	for _, need := range needs {
		if reported[need.File] || found[need.File] != "" || !slices.Contains(wanted, need.File) {
			continue
		}
		reported[need.File] = true
		owner := ""
		if tlmgr != "" {
			owner = tlmgrOwner(tlmgr, need.File)
		}
		r.Missing = append(r.Missing, paperMissing{paperNeed: need, Package: owner, Hint: installHint(need.File, owner, tlmgr != "")})
		r.Ready = false
	}
}

func tlmgrFor(engine string) string {
	if engine != "" {
		if tool := command.Sibling(engine, "tlmgr"); command.IsExecutable(tool) {
			return tool
		}
	}
	return command.Executable("tlmgr")
}

// checkBibliography records the bibliography program the paper needs and whether it is present.
func (r *paperReport) checkBibliography(bib bibUse, engine string) {
	name := bib.program()
	if name == "" {
		return
	}
	r.Bibliography = &bib
	program := &paperProgram{Name: name}
	program.Path, _ = beside(name)(engine)
	program.Found = program.Path != ""
	r.Program = program
	r.Ready = r.Ready && program.Found
}

// text is the report as doctor PAPER prints it.
func (r paperReport) text() string {
	var out strings.Builder
	fmt.Fprintf(&out, "Paper     %s (%d source files read)\n", r.Main, len(r.FilesRead))
	fmt.Fprintf(&out, "Checked   %d classes, packages and styles: %d in the paper, %d missing\n", r.Checked, r.InPaper, len(r.Missing))
	if r.LookupError != "" {
		fmt.Fprintf(&out, "  MISSING  %s\n", r.LookupError)
	}
	for _, missing := range r.Missing {
		fmt.Fprintf(&out, "  MISSING  %-24s %s (\\%s)\n", missing.File, missing.Where, missing.Command)
		fmt.Fprintf(&out, "  %-8s %-24s install: %s\n", "", "", missing.Hint)
	}
	r.bibliographyText(&out)
	out.WriteString("Not complete by design: only the files the source names are checked; the packages those load are not followed.\n")
	if r.Ready {
		out.WriteString("\nReady.\n")
	} else {
		out.WriteString("\nNot ready: install the missing files and programs above.\n")
	}
	return out.String()
}

func (r paperReport) bibliographyText(out *strings.Builder) {
	if r.Bibliography == nil {
		out.WriteString("Bibliography  none needs a program\n")
		return
	}
	style := "BibTeX (\\bibliography)"
	if r.Bibliography.Biblatex {
		style = "biblatex, backend " + map[bool]string{true: r.Bibliography.Backend, false: "biber (default)"}[r.Bibliography.Backend != ""]
	}
	status := fmt.Sprintf("ok %s", r.Program.Path)
	if !r.Program.Found {
		status = "MISSING " + r.Program.Name + " install: " + doctorInstallHint(r.Program.Name)
	}
	fmt.Fprintf(out, "Bibliography  %s: %s\n", style, status)
}

// doctorInstallHint is the install hint of a program in the program list, for this platform.
func doctorInstallHint(name string) string {
	for _, spec := range doctorTools() {
		if spec.name == name {
			if runtime.GOOS == "windows" {
				return spec.windows
			}
			return spec.linux
		}
	}
	return ""
}

// doctorPaper is relatex doctor PAPER: the report as text or JSON, exit status 1 when anything is missing.
func doctorPaper(input, requested string, withTlmgr, asJSON bool) {
	report := diagnosePaper(input, requested, withTlmgr)
	status := 0
	if !report.Ready {
		status = 1
	}
	if asJSON {
		data, _ := json.MarshalIndent(report, "", "  ")
		panic(&exitRequest{stdout: string(data) + "\n", status: status})
	}
	panic(&exitRequest{stdout: report.text(), status: status})
}
