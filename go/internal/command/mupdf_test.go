package command

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// mutool exits 1 when MuPDF drew a page past content it could not read (third cohort 0786: a
// shading's stitching function with 259 sub-functions); such a log is recovered, with its distinct
// errors. A log with an error line, or without the recovered-page warning, is a failure.
func TestRecoveredContentErrors(t *testing.T) {
	log := "warning: ICC support is not available\n" +
		"syntax error: too many sub-functions in stitching function\n" + recoveredWarning + "\n" +
		"syntax error: too many sub-functions in stitching function\n" + recoveredWarning + "\n"
	errors, ok := Recovered([]byte(log))
	if !ok || !slices.Equal(errors, []string{"too many sub-functions in stitching function"}) {
		t.Fatalf("Recovered = %v, %v", errors, ok)
	}
	for _, failed := range []string{
		"syntax error: too many sub-functions in stitching function\n" + recoveredWarning + "\nerror: cannot draw page\n",
		"syntax error: too many sub-functions in stitching function\n",
		recoveredWarning + "\n",
		"",
	} {
		if _, ok := Recovered([]byte(failed)); ok {
			t.Errorf("Recovered(%q) accepted", failed)
		}
	}
}

func TestMutoolDrawArgsIncludeNoICC(t *testing.T) {
	args := MutoolDrawArgs("-F", "trace", "-o", "out.xml", "paper.pdf")
	want := []string{"draw", "-q", "-N", "-F", "trace", "-o", "out.xml", "paper.pdf"}
	if !slices.Equal(args, want) {
		t.Fatalf("MutoolDrawArgs = %v, want %v", args, want)
	}
	if !slices.Contains(args, "-N") {
		t.Fatalf("MutoolDrawArgs missing -N: %v", args)
	}

	cmd := MutoolDrawCommand("/usr/bin/mutool", "-r", "50", "-o", "page-%d.pnm", "paper.pdf")
	wantCmd := []string{"/usr/bin/mutool", "draw", "-q", "-N", "-r", "50", "-o", "page-%d.pnm", "paper.pdf"}
	if !slices.Equal(cmd, wantCmd) {
		t.Fatalf("MutoolDrawCommand = %v, want %v", cmd, wantCmd)
	}
	if !slices.Contains(cmd, "-N") {
		t.Fatalf("MutoolDrawCommand missing -N: %v", cmd)
	}

	directory := t.TempDir()
	logPath := filepath.Join(directory, "draw.log")
	stage := Run(MutoolDrawCommand(os.Args[0], "-F", "trace", "-o", "out.xml", "paper.pdf"),
		directory, logPath, 5*time.Second, map[string]string{helperRole: "echo-args"})
	if stage["exit"] != int64(0) || stage["timeout"] != false {
		t.Fatalf("Run stage = %v", stage)
	}
	recorded, _ := stage["command"].([]any)
	if !slices.Contains(recorded, any("-N")) {
		t.Fatalf("stage command missing -N: %v", recorded)
	}
	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(strings.Split(strings.TrimSpace(string(logBytes)), "\n"), "-N") {
		t.Fatalf("executed args missing -N: %q", string(logBytes))
	}
}
