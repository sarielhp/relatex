package synctexout

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/fspath"
)

// Path is the SyncTeX file a viewer looks for beside a PDF.
func Path(pdf string) string {
	return strings.TrimSuffix(pdf, filepath.Ext(pdf)) + ".synctex.gz"
}

// TimedOut is another tool's PDF that did not come within the wait.
type TimedOut struct {
	PDF  string
	Wait time.Duration
}

func (t *TimedOut) Error() string {
	return fmt.Sprintf("timed out: no %s written within %s of the start", filepath.Base(t.PDF), t.Wait)
}

// Refused is another tool's PDF that is not the one relatex built.
type Refused struct {
	PDF    string
	Reason string
}

func (r *Refused) Error() string {
	return fmt.Sprintf("refused: %s is not the PDF relatex built (%s); %s left as it was", filepath.Base(r.PDF), r.Reason,
		filepath.Base(Path(r.PDF)))
}

// Await waits for another tool's PDF: written after started and the same size and time on two
// looks 200 ms apart. A *TimedOut when it does not come within wait.
func Await(pdf string, started time.Time, wait time.Duration) error {
	deadline := started.Add(wait)
	var last os.FileInfo
	for {
		info, err := os.Stat(pdf)
		fresh := err == nil && info.Mode().IsRegular() && info.ModTime().After(started)
		if fresh && last != nil && info.Size() == last.Size() && info.ModTime().Equal(last.ModTime()) {
			return nil
		}
		last = nil
		if fresh {
			last = info
		}
		if time.Now().After(deadline) {
			return &TimedOut{PDF: pdf, Wait: wait}
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// Built is what relatex's own build made, to match another tool's PDF against.
type Built struct {
	Input   string            // the paper's directory, absolute
	Sources map[string]string // the sources' digests as the build read them, by path relative to Input
	PDF     []byte            // the PDF relatex built
	Scratch func() string     // a directory for the comparison's files
}

// Verify is nil when pdf is, byte for byte but its /ID, the PDF relatex built; else a *Refused.
func (b Built) Verify(pdf string) error {
	theirs, err := os.ReadFile(pdf)
	if err != nil {
		return err
	}
	if Same(theirs, b.PDF) {
		return nil
	}
	return &Refused{PDF: pdf, Reason: b.difference(pdf)}
}

// difference is why the PDFs differ, as far as it can tell: sources that changed while relatex
// built (the bibliography the other tool writes last, most often), else where the PDFs part.
func (b Built) difference(pdf string) string {
	changed := ChangedSources(b.Input, b.Sources)
	if len(changed) > 0 {
		reason := "changed after relatex read them: " + strings.Join(changed, ", ")
		for _, name := range changed {
			if strings.HasSuffix(name, ".bbl") {
				return reason + " (the bibliography: the next build will match)"
			}
		}
		return reason
	}
	scratch, err := os.MkdirTemp(b.Scratch(), "synctex-compare-")
	if err != nil {
		return "the PDFs differ"
	}
	defer os.RemoveAll(scratch)
	ours := filepath.Join(scratch, "relatex.pdf")
	if os.WriteFile(ours, b.PDF, 0o644) != nil {
		return "the PDFs differ"
	}
	if where := Difference(pdf, ours); where != "" {
		return where
	}
	return "the PDFs differ"
}

// ChangedSources are the sources (at most three named) whose content is no longer what the build read.
func ChangedSources(directory string, sources map[string]string) []string {
	changed := []string{}
	for _, path := range sortedNames(sources) {
		file := filepath.Join(directory, filepath.FromSlash(path))
		if !regular(file) || command.FileDigest(file) != sources[path] {
			changed = append(changed, path)
		}
	}
	if len(changed) > 3 {
		changed = append(changed[:3], fmt.Sprintf("and %d more", len(changed)-3))
	}
	return changed
}

func sortedNames(set map[string]string) []string {
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// Install writes the SyncTeX file beside the PDF as NAME.synctex.gz.tmp and renames it over
// NAME.synctex.gz, so a viewer reads the old file or the new one, never a part. partial is told
// the temporary file while it exists ("" once renamed), for a signal to remove it.
func Install(pdf string, synctex []byte, partial func(path string)) (string, error) {
	destination := Path(pdf)
	temporary := destination + ".tmp"
	partial(temporary)
	if err := os.WriteFile(temporary, synctex, 0o644); err != nil {
		_ = os.Remove(temporary)
		return "", err
	}
	if err := fspath.Replace(temporary, destination); err != nil {
		_ = os.Remove(temporary)
		return "", err
	}
	partial("")
	return filepath.Base(destination), nil
}
