package tryarxiv

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Options configure a try_arxiv run.
type Options struct {
	IDs      []string
	Random   int
	Category string // arXiv category group for --random, "cs" by default
	Dest     string
	Relatex  string // the relatex executable
}

// Try downloads one paper, unpacks it under dest and runs relatex there. It returns the one-line
// result to print: the identifier, ok or FAILED and relatex's closing line.
func Try(ctx context.Context, client *Client, id, dest, relatex string) string {
	dir := filepath.Join(dest, SafeName(id))
	body, err := client.EPrint(ctx, id)
	if err != nil {
		return line(id, "FAILED", "download: "+err.Error())
	}
	if err := Unpack(body, dir); err != nil {
		return line(id, "SKIPPED", err.Error())
	}
	return convert(ctx, id, dir, relatex)
}

func convert(ctx context.Context, id, dir, relatex string) string {
	var arguments []string
	if main := MainFile(dir); main != "" {
		arguments = []string{"--main", main}
	}
	command := exec.CommandContext(ctx, relatex, arguments...)
	command.Dir = dir
	var output bytes.Buffer
	command.Stdout, command.Stderr = &output, &output
	err := command.Run()
	summary := lastLine(output.String())
	if err != nil {
		return line(id, "FAILED", summary)
	}
	return line(id, "ok", summary+"  -> "+filepath.Join(dir, "html", "index.html"))
}

func line(id, status, detail string) string {
	return fmt.Sprintf("%-18s %-8s %s", id, status, detail)
}

// lastLine is the last non-empty line of relatex's output, which says how the conversion ended.
func lastLine(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// Run resolves the papers to try (the explicit identifiers, or n random recent ones), tries each
// and writes one line per paper to out.
func Run(ctx context.Context, client *Client, options Options, out io.Writer, pick func([]string, int) []string) error {
	ids := options.IDs
	if options.Random > 0 {
		recent, err := client.Recent(ctx, options.Category, 100)
		if err != nil {
			return err
		}
		ids = append(ids, pick(recent, options.Random)...)
	}
	if len(ids) == 0 {
		return errors.New("nothing to do: give --id ID or --random N")
	}
	for _, id := range ids {
		if !ValidID(id) {
			return fmt.Errorf("%q is not an arXiv identifier (like 2401.00001 or hep-th/9901001)", id)
		}
	}
	for _, id := range ids {
		fmt.Fprintln(out, Try(ctx, client, id, options.Dest, options.Relatex))
	}
	return nil
}

var idPattern = regexp.MustCompile(`^(\d{4}\.\d{4,5}|[a-z-]+(\.[A-Z]{2})?/\d{7})(v\d+)?$`)

// ValidID reports whether id is an arXiv identifier (new style 2401.00001 or old style hep-th/9901001).
func ValidID(id string) bool {
	return idPattern.MatchString(id)
}
