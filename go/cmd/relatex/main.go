// Command relatex is the relatex program in Go (decision 067): the command line of
// internal/cli. Its resources (tex/, schemas/, profiles/, the VERSION file, the export's
// files in lib/relatex/) are RELATEX_ROOT's; else, in a checkout's build/, the checkout's;
// else those embedded in the binary, unpacked once into the runtime cache.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"runtime/pprof"
	"strings"

	"github.com/sarielhp/relatex/internal/cli"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/fspath"
	"github.com/sarielhp/relatex/internal/resources"
)

func main() {
	tuneCollector()
	command.UseTempRoot()
	root, err := root()
	if err != nil {
		fmt.Fprintln(os.Stderr, "relatex: cannot unpack its resources:", err)
		os.Exit(1)
	}
	cli.Root = root
	cli.Name = programName(os.Args[0])
	os.Exit(profiled(func() int { return cli.Run(os.Args[1:], os.Stdout, os.Stderr) }))
}

func root() (string, error) {
	if explicit := os.Getenv("RELATEX_ROOT"); explicit != "" {
		return explicit, nil
	}
	if checkout := checkoutRoot(); checkout != "" {
		return checkout, nil
	}
	unpacked, err := resources.Root(command.CacheRoot())
	if err != nil || unpacked != "" {
		return unpacked, err
	}
	return ".", nil
}

// checkoutRoot is the repository a development build runs from: the parent of the binary's
// directory when it holds the Go sources and the resources.
func checkoutRoot() string {
	executable, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolved, resolveErr := fspath.Real(executable); resolveErr == nil {
		executable = resolved
	}
	parent := filepath.Dir(filepath.Dir(executable))
	for _, marker := range []string{"VERSION", "go/go.mod", "tex", "schemas"} {
		if _, statErr := os.Stat(filepath.Join(parent, marker)); statErr != nil {
			return ""
		}
	}
	return parent
}

// collectorPercent is the heap growth the collector allows before its next cycle: half the live
// heap, not the default whole of it. A book's conversion decodes large structures and drops them
// at every stage boundary; at the default the garbage between cycles inflated its resident peak
// (measured on books/0004, night run 2026-11-16b item W8). A caller's own GOGC wins.
const collectorPercent = 50

// A soft memory limit (GOMEMLIMIT) cut the peak further (books/0004: 2771 to 1783 MB) but made the
// export's browser check 55% slower, and books/0003 ran 25 minutes at 7 cores in that phase, so
// none is set by default; a caller may export one.
func tuneCollector() {
	if os.Getenv("GOGC") == "" {
		debug.SetGCPercent(collectorPercent)
	}
}

// profiled runs the command, writing a CPU profile to $RELATEX_CPUPROFILE when that is set.
func profiled(run func() int) int {
	path := os.Getenv("RELATEX_CPUPROFILE")
	if path == "" {
		return run()
	}
	file, err := os.Create(path)
	if err != nil {
		return run()
	}
	defer file.Close()
	if err := pprof.StartCPUProfile(file); err != nil {
		return run()
	}
	defer pprof.StopCPUProfile()
	return run()
}

// programName is the name relatex was started under, without the .exe a Windows program has
// (any case), lower-cased as Windows names compare: RLX.EXE is rlx (decision 129).
func programName(argument string) string {
	name := filepath.Base(argument)
	if extension := filepath.Ext(name); strings.EqualFold(extension, ".exe") {
		return strings.ToLower(name[:len(name)-len(extension)])
	}
	return name
}
