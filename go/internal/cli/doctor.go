package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"

	"github.com/sarielhp/clihelp"

	"github.com/sarielhp/relatex/internal/browser"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/pdfpaint"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/texsandbox"
)

// toolSpec is one program relatex starts: how it is found, the flag that prints its version, and
// how to install it on Linux and Windows.
type toolSpec struct {
	name, flag     string
	find           func(engine string) (path, source string)
	required       bool
	linux, windows string
}

// toolCheck is what doctor found for one tool.
type toolCheck struct {
	Name     string `json:"name"`
	Path     string `json:"path,omitempty"`
	Source   string `json:"source,omitempty"`
	Version  string `json:"version,omitempty"`
	Required bool   `json:"required"`
	Note     string `json:"note,omitempty"`
	Without  string `json:"without_it,omitempty"`
	Linux    string `json:"install_linux"`
	Windows  string `json:"install_windows"`
}

// doctorReport is relatex doctor's whole answer.
type doctorReport struct {
	Version     string            `json:"version"`
	Platform    string            `json:"platform"`
	Ready       bool              `json:"ready"`
	Tools       []toolCheck       `json:"tools"`
	TeXLive     string            `json:"texlive_root"`
	Missing     []string          `json:"missing_tex_files"`
	Sandbox     string            `json:"sandbox"`
	Directories map[string]string `json:"directories"`
	Notes       []string          `json:"platform_notes,omitempty"`
}

const (
	texLiveLinux   = "TeX Live from https://tug.org/texlive/ (install-tl), or RELATEX_TEXLIVE / RELATEX_ENGINE"
	texLiveWindows = "TeX Live from https://tug.org/texlive/windows.html (install-tl-windows.exe), or RELATEX_TEXLIVE"
	popplerLinux   = "apt install poppler-utils"
	popplerWindows = "scoop install poppler"
)

func doctorTools() []toolSpec {
	return []toolSpec{
		{"lualatex", "--version", findEngine, true, texLiveLinux, texLiveWindows},
		{"kpsewhich", "--version", beside("kpsewhich"), true, texLiveLinux, texLiveWindows},
		{"luaotfload-tool", "--version", beside("luaotfload-tool"), true, texLiveLinux, texLiveWindows},
		{"biber", "--version", beside("biber"), false, texLiveLinux, texLiveWindows},
		{"bibtex", "--version", beside("bibtex"), false, texLiveLinux, texLiveWindows},
		{"makeindex", "", beside("makeindex"), false, texLiveLinux, texLiveWindows},
		{"epstopdf", "--version", beside("epstopdf"), false, texLiveLinux, texLiveWindows},
		{"vftovp", "--version", findVFtoVP, true, texLiveLinux, texLiveWindows},
		{"mutool", "-v", resolved("mutool"), true, "MuPDF " + pinnedMuPDF() + " (https://mupdf.com/releases)",
			"scoop install mupdf (MuPDF " + pinnedMuPDF() + ")"},
		{"qpdf", "--version", resolved("qpdf"), true, "apt install qpdf", "scoop install qpdf"},
		{"pdfinfo", "-v", resolved("pdfinfo"), true, popplerLinux, popplerWindows},
		{"pdftocairo", "-v", resolved("pdftocairo"), true, popplerLinux, popplerWindows},
		{"pdftotext", "-v", resolved("pdftotext"), true, popplerLinux, popplerWindows},
		{"pdftoppm", "-v", resolved("pdftoppm"), true, popplerLinux, popplerWindows},
		{"typst", "--version", resolved("typst"), false, "https://github.com/typst/typst/releases", "scoop install typst"},
		{"browser", "", findBrowser, false, "apt install chromium, or RELATEX_CHROMIUM",
			"Microsoft Edge (part of Windows), or RELATEX_CHROMIUM"},
		{"verapdf", "--version", resolved("verapdf"), false, "https://verapdf.org/software/", "https://verapdf.org/software/"},
		{"zstd", "--version", onPath("zstd"), false, "apt install zstd", "scoop install zstd"},
		{"rsvg-convert", "--version", resolved("rsvg-convert"), false, "apt install librsvg2-bin", "scoop install rsvg-convert"},
		{"tesseract", "--version", resolved("tesseract"), false, "apt install tesseract-ocr", "scoop install tesseract"},
		{"mktextfm", "", beside("mktextfm"), false, texLiveLinux, texLiveWindows},
		{"mf", "--version", findMetafont, false, texLiveLinux, texLiveWindows},
	}
}

// doctorEffects is, for a program a conversion can do without, what it loses without it.
var doctorEffects = map[string]string{
	"biber":        "papers whose biblatex uses the biber backend (the default) cannot build their bibliography",
	"bibtex":       "papers with \\bibliography or biblatex backend=bibtex cannot build their bibliography",
	"makeindex":    "papers with an index (\\printindex) get no index entries",
	"epstopdf":     "EPS figures are not converted and their drawings are missing",
	"typst":        "no Typst output and no Typst check of the bibliography",
	"browser":      "no browser rendering of the HTML (screenshots, MathML checks)",
	"verapdf":      "no PDF/UA validation of the tagged PDF",
	"zstd":         "stage caches stay uncompressed (larger on disk)",
	"rsvg-convert": "the check that a redrawn SVG figure matches its source reports a render failure instead of a verdict",
	"tesseract":    "no OCR of text inside bitmap figures; their alt-text drafts rest on the picture alone",
	"mktextfm":     "fonts with no ready TFM metrics (Metafont fonts) cannot be generated and the build fails on them",
	"mf":           "Metafont fonts cannot be generated (mktextfm needs it)",
	"fc-list":      "installed system fonts are not found; papers that load them by name (fontspec) fall back to TeX Live fonts",
}

// findMetafont is Metafont beside the engine: mf, else the windowless mf-nowin TeX Live ships.
func findMetafont(engine string) (string, string) {
	if path, source := beside("mf")(engine); path != "" {
		return path, source
	}
	return beside("mf-nowin")(engine)
}

// extraTools are the programs only some platforms have: fontconfig on Unix (Windows scans its font folders).
func extraTools() []toolSpec {
	if runtime.GOOS == "windows" {
		return nil
	}
	return []toolSpec{{"fc-list", "--version", onPath("fc-list"), false, "apt install fontconfig", ""}}
}

// doctorSpec is relatex doctor: every program relatex starts, with its path, version and where it was
// found (a RELATEX_* variable, TeX Live, beside the engine or PATH), and how to install a missing
// one; then the TeX files, the sandbox and the directories relatex uses. It exits 1 when a
// required program is missing. With a paper it checks that paper's TeX files instead.
func doctorSpec() commandSpec {
	asJSON, withTlmgr, main := false, false, ""
	options := []clihelp.Option{
		switchFlag("--json", "Print the JSON form instead of the text form", func() { asJSON = true }),
		switchFlag("--tlmgr", "With a paper: ask tlmgr (slow, needs the network) which TeX Live package has each missing file",
			func() { withTlmgr = true }),
		textFlag("--main <file>", "With a paper: the main .tex file, a path inside the paper", &main),
	}
	return commandSpec{name: "doctor", description: "List the programs relatex starts and how to install a missing one; check a paper's TeX files",
		long: "Lists the programs relatex starts (path, version, where it was found, how to install a missing one).\n" +
			"With a paper (a .tex file, a directory or a ZIP, as convert takes) it instead reads the main file and the files it\n" +
			"inputs and looks up the document class, packages, themes, TikZ libraries and bibliography styles they name\n" +
			"(unless the paper has them itself) with kpsewhich, lists the missing ones with how to install them, and says\n" +
			"whether the bibliography needs biber or BibTeX and whether it is installed. It is not complete by design:\n" +
			"packages that other packages load are not followed, so a TeX Live without them can still fail later.\n" +
			"Exits 1 when something is missing.",
		usage: Name + " doctor [PAPER] [--json] [--tlmgr] [--main FILE]", options: options, args: clihelp.MaximumNArgs(1),
		run: func(words []string) ojson.Object {
			if len(words) == 1 {
				doctorPaper(words[0], main, withTlmgr, asJSON)
			}
			if withTlmgr || main != "" {
				fail("--tlmgr and --main need a paper: relatex doctor PAPER")
			}
			return doctor(asJSON)
		}}
}

func doctor(asJSON bool) ojson.Object {
	report := diagnose()
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

func diagnose() doctorReport {
	report := doctorReport{Version: version(), Platform: runtime.GOOS + "/" + runtime.GOARCH, Ready: true,
		TeXLive: command.TeXLiveRoot(), Sandbox: sandboxMode(), Directories: doctorDirectories(),
		Notes: platformNotes()}
	engine, _ := findEngine("")
	for _, spec := range append(doctorTools(), extraTools()...) {
		check := spec.check(engine)
		report.Ready = report.Ready && (check.Path != "" || !check.Required)
		report.Tools = append(report.Tools, check)
	}
	if runtime.GOOS != "windows" {
		check := toolSpec{"bwrap", "--version", onPath("bwrap"), true, "apt install bubblewrap", ""}.check(engine)
		report.Ready = report.Ready && check.Path != ""
		report.Tools = append(report.Tools, check)
	}
	report.Missing = missingTeXFiles(engine)
	report.Ready = report.Ready && len(report.Missing) == 0
	return report
}

func (spec toolSpec) check(engine string) toolCheck {
	check := toolCheck{Name: spec.name, Required: spec.required, Linux: spec.linux, Windows: spec.windows,
		Without: doctorEffects[spec.name]}
	check.Path, check.Source = spec.find(engine)
	if check.Path == "" {
		if check.Source != "" {
			check.Note = check.Source + " names no program here"
		}
		return check
	}
	if spec.flag != "" {
		if line := texsandbox.FirstLine(check.Path, spec.flag); line != nil {
			check.Version = *line
		}
	}
	if spec.name == "mutool" && !pdfpaint.SupportedVersion(check.Version) {
		check.Note = "relatex needs MuPDF " + pinnedMuPDF() + "x; another release series changes figures and tables"
	}
	return check
}

func pinnedMuPDF() string { return strings.TrimPrefix(pdfpaint.ToolSeries, "mutool version ") }

// findEngine is papersource.DefaultEngine, with where it found the engine.
func findEngine(string) (string, string) {
	if engine := os.Getenv("RELATEX_ENGINE"); engine != "" {
		return command.Executable(engine), "RELATEX_ENGINE"
	}
	if engine := command.ConfiguredTool("lualatex"); engine != "" {
		return command.Executable(engine), "config"
	}
	if engine := command.TeXLiveBin("lualatex"); command.IsExecutable(engine) {
		return engine, "TeX Live"
	}
	return command.Executable("lualatex"), "PATH"
}

// beside is a TeX Live program relatex starts from the engine's own directory.
func beside(name string) func(string) (string, string) {
	return func(engine string) (string, string) {
		if engine == "" {
			return "", ""
		}
		if path := command.Sibling(engine, name); command.IsExecutable(path) {
			return path, "beside lualatex"
		}
		return "", ""
	}
}

func findVFtoVP(string) (string, string) {
	if path := command.Executable("vftovp"); path != "" {
		return path, "PATH"
	}
	if path := command.TeXLiveBin("vftovp"); command.IsExecutable(path) {
		return path, "TeX Live"
	}
	return "", ""
}

// resolved is command.Resolve: RELATEX_<NAME>, else the configuration's "tools" path, else PATH.
func resolved(name string) func(string) (string, string) {
	return func(string) (string, string) { return command.Locate(name) }
}

func onPath(name string) func(string) (string, string) {
	return func(string) (string, string) {
		if path := command.Executable(name); path != "" {
			return path, "PATH"
		}
		return "", ""
	}
}

func findBrowser(string) (string, string) {
	if path := os.Getenv("RELATEX_CHROMIUM"); path != "" {
		return command.Executable(path), "RELATEX_CHROMIUM"
	}
	if path := command.ConfiguredTool("browser"); path != "" {
		return command.Executable(path), "config"
	}
	if endpoint := os.Getenv(browser.EndpointVariable); endpoint != "" {
		return endpoint, browser.EndpointVariable
	}
	if path := browser.DefaultBrowser(); command.IsExecutable(path) {
		return path, "default"
	}
	return "", ""
}

// missingTeXFiles are the TeX files relatex needs that kpsewhich does not find once (all of them
// without an engine).
func missingTeXFiles(engine string) (missing []string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			if _, ok := recovered.(rec.Failure); !ok {
				panic(recovered)
			}
			missing = []string{"(the TeX installation cannot be inspected)"}
		}
	}()
	if engine == "" {
		return []string{"(no lualatex)"}
	}
	for name, value := range rec.AsMap(texsandbox.Current(engine).Report["files"]) {
		if value == nil {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	return missing
}

func sandboxMode() string {
	if texsandbox.Sandboxed {
		return "bubblewrap: TeX sees the source, its output and the TeX tree only"
	}
	return "none: TeX runs with its own file limits only (openin_any=p, openout_any=p)"
}

func doctorDirectories() map[string]string {
	temporary, _ := command.TempRoot()
	return map[string]string{"cache": command.CacheRoot(), "temporary": temporary, "config": globalConfigPath()}
}

// text is the report as doctor prints it.
func (r doctorReport) text() string {
	var out strings.Builder
	fmt.Fprintf(&out, "relatex %s (%s)\n\nPrograms\n", r.Version, r.Platform)
	for _, check := range r.Tools {
		out.WriteString(check.text())
	}
	fmt.Fprintf(&out, "\nTeX Live  %s\n", r.TeXLive)
	if len(r.Missing) > 0 {
		fmt.Fprintf(&out, "  missing TeX files: %s\n", strings.Join(r.Missing, ", "))
	}
	fmt.Fprintf(&out, "Sandbox   %s\n", r.Sandbox)
	for _, name := range []string{"cache", "temporary", "config"} {
		fmt.Fprintf(&out, "%-9s %s\n", strings.ToUpper(name[:1])+name[1:], r.Directories[name])
	}
	for _, note := range r.Notes {
		fmt.Fprintf(&out, "%s\n", note)
	}
	if r.Ready {
		out.WriteString("\nReady.\n")
	} else {
		out.WriteString("\nNot ready: install the missing required programs above.\n")
	}
	return out.String()
}

func (c toolCheck) text() string {
	if c.Path == "" {
		kind, hint := "optional", c.Linux
		if c.Required {
			kind = "MISSING"
		}
		if runtime.GOOS == "windows" {
			hint = c.Windows
		}
		text := fmt.Sprintf("  %-8s %-16s install: %s\n", kind, c.Name, hint)
		if c.Without != "" {
			text += fmt.Sprintf("  %-8s %-16s without it: %s\n", "", "", c.Without)
		}
		if c.Note != "" {
			text += fmt.Sprintf("  %-8s %-16s note: %s\n", "", "", c.Note)
		}
		return text
	}
	text := fmt.Sprintf("  %-8s %-16s %s (%s)\n", "ok", c.Name, c.Path, c.Source)
	if c.Version != "" {
		text += fmt.Sprintf("  %-8s %-16s %s\n", "", "", c.Version)
	}
	if c.Note != "" {
		text += fmt.Sprintf("  %-8s %-16s note: %s\n", "", "", c.Note)
	}
	return text
}
