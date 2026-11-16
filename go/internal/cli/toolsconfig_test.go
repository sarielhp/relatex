package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/authordir"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/papersource"
)

func TestToolsSectionIsValidated(t *testing.T) {
	cases := map[string]string{
		`{"tools": {"mutool": "C:\\mupdf\\mutool.exe", "qpdf": null, "browser": "/opt/chrome/chrome"}}`: "",
		`{"tools": {"verapdf": "~/verapdf/verapdf"}}`:                                                   "",
		`{"tools": {"bwrap": "/usr/bin/bwrap"}}`:                                                        `unknown setting "tools.bwrap"`,
		`{"tools": {"mutool": ""}}`:                                                                     `"tools.mutool" must be null or the program's path`,
		`{"tools": {"mutool": ["mutool"]}}`:                                                             `"tools.mutool" must be null or the program's path`,
		`{"tools": ["mutool"]}`:                                                                         `"tools" must be an object`,
	}
	for text, want := range cases {
		path := writeConfig(t, text)
		got := failure(func() { readConfig(path) })
		if want == "" && got != "" || want != "" && !strings.Contains(got, want) {
			t.Errorf("%s: got %q, want %q", text, got, want)
		}
	}
}

// A paper's own file must not choose the programs relatex runs.
func TestPaperMayNotChooseTools(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	paper := t.TempDir()
	if err := authordir.Ensure(filepath.Join(paper, projectConfig)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paper, projectConfig), []byte(`{"tools": {"lualatex": "./evil"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := failure(func() { loadConfig(paper) }); !strings.Contains(got, `"tools" may be set only in the global configuration`) {
		t.Errorf("a paper's tools were accepted: %q", got)
	}
	if strings.Contains(projectTemplate(), `"tools"`) {
		t.Error("the project template offers tools")
	}
}

// fakeProgram writes an executable file named name (name.exe on Windows) in directory.
func fakeProgram(t *testing.T, directory, name string) string {
	t.Helper()
	path := filepath.Join(directory, name)
	if runtime.GOOS == "windows" {
		path += ".exe"
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho version 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// globalTools writes a global configuration whose "tools" section is section and configures it,
// as dispatch does before every command but config.
func globalTools(t *testing.T, section string) {
	t.Helper()
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)
	if err := os.MkdirAll(filepath.Join(base, "relatex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "relatex", "config.jsonc"), []byte(`{"tools": `+section+`}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { command.ConfigureTools(nil) })
	configureTools()
}

// jsonPath writes a path as a JSON string.
func jsonPath(path string) string { return `"` + strings.ReplaceAll(path, `\`, `\\`) + `"` }

// The lookup takes the configured path before PATH; RELATEX_<NAME> still overrides it, and doctor
// names the source.
func TestConfiguredToolIsFoundBeforePathAndAfterTheVariable(t *testing.T) {
	directory := t.TempDir()
	onPath := fakeProgram(t, filepath.Join(directory, "path"), "qpdf")
	configured := fakeProgram(t, filepath.Join(directory, "config"), "qpdf")
	variable := fakeProgram(t, filepath.Join(directory, "variable"), "qpdf")
	t.Setenv("PATH", filepath.Dir(onPath))
	t.Setenv("RELATEX_QPDF", "")
	globalTools(t, `{"qpdf": `+jsonPath(configured)+`}`)
	if path, source := resolved("qpdf")(""); path != configured || source != "config" {
		t.Errorf("configured: found %q from %q", path, source)
	}
	if got := command.Resolve("qpdf"); got != configured {
		t.Errorf("Resolve = %q, want %q", got, configured)
	}
	t.Setenv("RELATEX_QPDF", variable)
	if path, source := resolved("qpdf")(""); path != variable || source != "RELATEX_QPDF" {
		t.Errorf("variable: found %q from %q", path, source)
	}
}

// A configured path that names no program is missing: never replaced by the one on PATH.
func TestConfiguredToolThatIsAbsentIsMissing(t *testing.T) {
	directory := t.TempDir()
	onPath := fakeProgram(t, directory, "mutool")
	t.Setenv("PATH", filepath.Dir(onPath))
	t.Setenv("RELATEX_MUTOOL", "")
	globalTools(t, `{"mutool": `+jsonPath(filepath.Join(directory, "gone", "mutool"))+`}`)
	spec := toolSpec{"mutool", "", resolved("mutool"), true, "", ""}
	check := spec.check("")
	if check.Path != "" || check.Source != "config" || !strings.Contains(check.Note, "config names no program") {
		t.Errorf("check %+v", check)
	}
	if !strings.Contains(check.text(), "note: config names no program here") {
		t.Errorf("text lacks the note:\n%s", check.text())
	}
}

// The configured lualatex is the default engine, and its tree is TeX Live's root (kpsewhich and
// the other TeX Live programs beside it).
func TestConfiguredEngineAndItsTree(t *testing.T) {
	tree := t.TempDir()
	engine := fakeProgram(t, filepath.Join(tree, "bin", "x86_64-linux"), "lualatex")
	t.Setenv("RELATEX_ENGINE", "")
	t.Setenv("RELATEX_TEXLIVE", "")
	globalTools(t, `{"lualatex": `+jsonPath(engine)+`}`)
	if got := papersource.DefaultEngine(); got != engine {
		t.Errorf("DefaultEngine = %q, want %q", got, engine)
	}
	if path, source := findEngine(""); path != engine || source != "config" {
		t.Errorf("findEngine = %q from %q", path, source)
	}
	if got := command.TeXLiveRoot(); got != tree {
		t.Errorf("TeXLiveRoot = %q, want %q", got, tree)
	}
}

func TestConfiguredBrowser(t *testing.T) {
	browserPath := fakeProgram(t, t.TempDir(), "chrome")
	t.Setenv("RELATEX_CHROMIUM", "")
	globalTools(t, `{"browser": `+jsonPath(browserPath)+`}`)
	if path, source := findBrowser(""); path != browserPath || source != "config" {
		t.Errorf("findBrowser = %q from %q", path, source)
	}
}
