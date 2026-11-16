//go:build windows

package texsandbox

import (
	"path/filepath"
)

// Sandboxed is false on Windows: there is no OS sandbox for TeX (decision 129, D1). A build
// runs with TeX's own limits instead (-no-shell-escape, and openin_any and openout_any set to
// "p": paranoid, no absolute paths, no "..", no dot files), and every run records the
// sandbox.none diagnostic.
const Sandboxed = false

// WrapWith on Windows runs the program as it is, in working (the caller's Run directory): the
// run's directories exist, the program's temporary directory is the run's own (TMPDIR, TMP and
// TEMP), and TeX's file limits are set in environment, which the caller passes to command.Run.
// The extra directories need no mount: without a sandbox every directory is readable.
func (d *Doctor) WrapWith(arguments []string, source, output, working string, environment map[string]string, extra []string) []string {
	_ = extra
	for _, path := range []string{source, output, environment["TEXMFVAR"], environment["TEXMFCACHE"]} {
		if path != "" {
			absolute, _ := filepath.Abs(path)
			mkdir(absolute)
		}
	}
	temporary := sandboxTemporary(environment)
	mkdir(temporary)
	if environment != nil {
		for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
			environment[name] = temporary
		}
		environment["openin_any"], environment["openout_any"] = "p", "p"
		// Paranoid writes go only below the output directory and TEXMFOUTPUT: the run's font
		// cache is the one other place TeX writes (luaotfload refuses to start without it).
		if cache := environment["TEXMFCACHE"]; cache != "" && environment["TEXMFOUTPUT"] == "" {
			environment["TEXMFOUTPUT"], _ = filepath.Abs(cache)
		}
	}
	_ = working
	return arguments
}
