package formulatex

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/authormath"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/texsandbox"
)

// TestProbeMathML prints the MathML the collector makes for each line of RELATEX_PROBE_TEX (a
// development aid: inline lines, or "D:" for a display, "E:env:" for an environment).
func TestProbeMathML(t *testing.T) {
	path := os.Getenv("RELATEX_PROBE_TEX")
	if path == "" {
		t.Skip("RELATEX_PROBE_TEX not set")
	}
	authormath.Root, _ = filepath.Abs("../../..")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	ids := make([]string, len(lines))
	for index, line := range lines {
		ids[index] = authormath.CandidateID(index)
		source := candidateSource(Formula{Text: line})
		switch {
		case strings.HasPrefix(line, "D:"):
			source = candidateSource(Formula{Text: line[2:], Display: true})
		case strings.HasPrefix(line, "E:"):
			parts := strings.SplitN(line[2:], ":", 2)
			source = candidateSource(Formula{Text: strings.ReplaceAll(parts[1], `\n`, "\n"), Display: true, Environment: parts[0]})
		}
		if err := os.WriteFile(filepath.Join(directory, ids[index]+".tex"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	typesetter := &authormath.Typesetter{Directory: directory, Header: Preamble,
		Doctor: texsandbox.Current(command.TeXLiveBin("lualatex")), Attempt: "formula-tex"}
	results := typesetter.Typeset(ids)
	for index, id := range ids {
		fmt.Printf("%s\n  %v\n", lines[index], results[id])
	}
}
