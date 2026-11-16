package browser

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/sarielhp/relatex/internal/command"
)

// DefaultBrowser is the browser a check starts when RELATEX_CHROMIUM names none: Chromium on
// Linux; on Windows the Edge every installation has (decision 129, D3: CDP and --headless work
// the same, and its version is reported, not pinned).
func DefaultBrowser() string {
	if runtime.GOOS != "windows" {
		return "/usr/bin/chromium"
	}
	for _, base := range []string{os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles"), os.Getenv("LOCALAPPDATA")} {
		if base == "" {
			continue
		}
		if path := filepath.Join(base, "Microsoft", "Edge", "Application", "msedge.exe"); command.IsExecutable(path) {
			return path
		}
	}
	if path := command.Executable("msedge"); path != "" {
		return path
	}
	return "msedge.exe"
}
