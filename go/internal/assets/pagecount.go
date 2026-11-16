package assets

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/texsandbox"
)

var pagesLine = regexp.MustCompile(`\APages:\s+([1-9][0-9]*)\s*\z`)

// CapturePageCount is PDFPageCount.new.capture(pdf, output:): the PDF's physical page count,
// read by the recorded pdfinfo inside the TeX sandbox, with the proof of how.
func CapturePageCount(pdf, output string) rec.Map {
	if _, err := os.Lstat(output); err == nil {
		rec.Fail("Page-count output must be new")
	}
	mkdir(output)
	input := filepath.Join(output, "input.pdf")
	digest := command.FileDigest(pdf)
	copyFile(pdf, input)
	if command.FileDigest(input) != digest {
		rec.Fail("PDF changed while counting pages")
	}
	helper := texsandbox.HelperIdentity("pdfinfo", "-v")
	if command.FileDigest(rec.FetchString(helper, "path")) != helper["sha256"] {
		rec.Fail("PDF helper identity changed")
	}
	log := filepath.Join(output, "pdfinfo.log")
	engine := command.TeXLiveBin("lualatex")
	environment := map[string]string{"PATH": command.ToolPath(engine), "TEXMFVAR": filepath.Join(output, "var"),
		"TEXMFCACHE": filepath.Join(output, "cache")}
	wrapped := texsandbox.Current(engine).Wrap([]string{rec.FetchString(helper, "path"), input}, output, output, environment)
	stage := command.Run(wrapped, output, log, 30*time.Second, environment)
	if !rec.Equal(stage["exit"], int64(0)) || stage["timeout"] == true {
		rec.Fail("Independent PDF page counting failed")
	}
	text, err := os.ReadFile(log)
	if err != nil {
		rec.Fail("%v", err)
	}
	pages := parsePages(string(text))
	if command.FileDigest(pdf) != digest {
		rec.Fail("Original PDF changed during page counting")
	}
	if command.FileDigest(rec.FetchString(helper, "path")) != helper["sha256"] {
		rec.Fail("PDF helper changed during counting")
	}
	proof := rec.Map{"pdf_sha256": digest, "pages": pages, "helper": helper, "stage": stage}
	writeJSON(filepath.Join(output, "proof.json"), proof)
	return proof
}

// parsePages is PDFPageCount.parse.
func parsePages(text string) int64 {
	lines := []string{}
	for _, line := range strings.SplitAfter(text, "\n") {
		if strings.HasPrefix(line, "Pages:") {
			lines = append(lines, line)
		}
	}
	if len(lines) != 1 {
		rec.Fail("Missing or ambiguous physical PDF page count")
	}
	match := pagesLine.FindStringSubmatch(lines[0])
	if match == nil {
		rec.Fail("Missing or ambiguous physical PDF page count")
	}
	pages, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil || pages > 100_000 {
		rec.Fail("Missing or ambiguous physical PDF page count")
	}
	return pages
}

func mkdir(path string) {
	if err := os.MkdirAll(path, 0o755); err != nil {
		rec.Fail("%v", err)
	}
}

func copyFile(from, to string) {
	data, err := os.ReadFile(from)
	if err != nil {
		rec.Fail("%v", err)
	}
	if err := os.WriteFile(to, data, 0o644); err != nil {
		rec.Fail("%v", err)
	}
}
