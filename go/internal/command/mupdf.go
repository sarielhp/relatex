package command

import (
	"bytes"
	"slices"
	"strings"
)

// recoveredWarning is the line MuPDF prints after a page whose content had syntax errors it
// recovered from by leaving out the operator that failed.
const recoveredWarning = "warning: encountered syntax errors; page may not be correct"

// MutoolDrawArgs builds the argument list for a `mutool draw` invocation (starting with
// "draw"), always passing `-N` to disable MuPDF's ICC workflow so every platform traces and
// renders like Debian's no-ICC build (decision 151).
func MutoolDrawArgs(arguments ...string) []string {
	result := make([]string, 0, 3+len(arguments))
	result = append(result, "draw", "-q", "-N")
	return append(result, arguments...)
}

// MutoolDrawCommand builds a complete command slice for `mutool draw` starting with tool and
// always passing `-N` (decision 151).
func MutoolDrawCommand(tool string, arguments ...string) []string {
	return append([]string{tool}, MutoolDrawArgs(arguments...)...)
}

// Recovered is the distinct content errors mutool recovered from, when its log (both streams of a
// draw that exited 1) holds nothing else: each error a "syntax error: " line, each page with one
// followed by MuPDF's recovered-page warning, other lines warnings. MuPDF then drew every page and
// left out only the operators that failed (third cohort 0786: a shading whose stitching function
// has 259 sub-functions, more than MuPDF reads, so the shading is not painted); mutool still exits
// 1. ok is false for any other log: an error line, or no recovered page.
func Recovered(log []byte) (messages []string, ok bool) {
	recovered := false
	for _, line := range strings.Split(string(bytes.TrimSpace(log)), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == recoveredWarning:
			recovered = true
		case strings.HasPrefix(line, "syntax error: "):
			if message := strings.TrimPrefix(line, "syntax error: "); !slices.Contains(messages, message) {
				messages = append(messages, message)
			}
		case strings.HasPrefix(line, "warning: "):
		default:
			return nil, false
		}
	}
	if !recovered || len(messages) == 0 {
		return nil, false
	}
	return messages, true
}
