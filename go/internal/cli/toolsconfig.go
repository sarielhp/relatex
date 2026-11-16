package cli

import (
	"os"
	"slices"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/fspath"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
)

// The "tools" section (decision 129, amendment 2): explicit paths for the programs relatex
// starts, for a machine where they are not on PATH (a Windows laptop with MuPDF unpacked from a
// zip). A RELATEX_<NAME> variable (RELATEX_ENGINE for lualatex, RELATEX_CHROMIUM for the browser)
// still overrides a path set here; a path that names no program is reported missing, never
// replaced by another program. Global file only: a paper's file that chose the programs relatex
// runs would run what the paper ships.

// toolDefaults is the "tools" section of the defaults: every program unset.
func toolDefaults() ojson.Object {
	section := ojson.Object{}
	for _, name := range command.ToolNames {
		section = append(section, ojson.Pair{name, nil})
	}
	return section
}

// validateTools checks the "tools" section: only the known programs, each null or a path.
func validateTools(value any, path string) {
	section, ok := value.(ojson.Object)
	if !ok {
		rec.Fail("%s: \"tools\" must be an object", path)
	}
	for _, pair := range section {
		if !slices.Contains(command.ToolNames, pair.Key()) {
			rec.Fail("%s: unknown setting \"tools.%s\"", path, pair.Key())
		}
		if text, isString := pair.Value().(string); pair.Value() != nil && (!isString || text == "") {
			rec.Fail("%s: \"tools.%s\" must be null or the program's path", path, pair.Key())
		}
	}
}

// refuseProjectTools stops a paper's own file from choosing programs.
func refuseProjectTools(values ojson.Object, path string) {
	if values.Has("tools") {
		rec.Fail("%s: \"tools\" may be set only in the global configuration (%s)", path, globalConfigPath())
	}
}

// configureTools hands the global file's "tools" paths to the tool lookup, before a command looks
// for any program. Without a global file nothing changes.
func configureTools() {
	path := globalConfigPath()
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		return
	}
	section, _ := readConfig(path).Get("tools").(ojson.Object)
	paths := map[string]string{}
	for _, pair := range section {
		if text, isString := pair.Value().(string); isString {
			paths[pair.Key()] = fspath.ExpandHome(text)
		}
	}
	command.ConfigureTools(paths)
}
