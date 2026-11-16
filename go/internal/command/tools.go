package command

import (
	"maps"
	"os"
	"path/filepath"
	"strings"
)

// ToolNames are the programs the configuration's "tools" section may name by path (decision 129,
// amendment 2): the TeX engine, the PDF helpers, OCR, Typst, the browser and veraPDF (amendment 4).
var ToolNames = []string{"lualatex", "mutool", "qpdf", "pdfinfo", "pdftocairo", "pdftotext", "pdftoppm", "tesseract",
	"typst", "browser", "verapdf"}

// configuredTools are the "tools" section's paths, set once before a command starts its work.
var configuredTools = map[string]string{}

// ConfigureTools takes the "tools" section's paths by program; a program left out is found
// as before.
func ConfigureTools(paths map[string]string) { configuredTools = maps.Clone(paths) }

// ConfiguredTool is the path the configuration names for a program, or "".
func ConfiguredTool(name string) string { return configuredTools[name] }

// Locate is Resolve with where it found the program: its RELATEX_<NAME> variable, then the
// configuration's "tools" section, then PATH. A variable or a configured path that names no
// program finds nothing: an explicit choice never falls back to another program silently.
func Locate(name string) (path, source string) {
	variable := "RELATEX_" + strings.ToUpper(name)
	if explicit := os.Getenv(variable); explicit != "" {
		return Executable(explicit), variable
	}
	if configured := configuredTools[name]; configured != "" {
		return Executable(configured), "config"
	}
	if path := Executable(name); path != "" {
		return path, "PATH"
	}
	return "", ""
}

// configuredTree is the TeX Live tree of the configured lualatex (TREE/bin/PLATFORM/lualatex), or
// "" when none is configured or it lies outside a bin directory.
func configuredTree() string {
	engine := configuredTools["lualatex"]
	if engine == "" {
		return ""
	}
	bin := filepath.Dir(filepath.Dir(engine))
	if !strings.EqualFold(filepath.Base(bin), "bin") {
		return ""
	}
	return filepath.Dir(bin)
}
