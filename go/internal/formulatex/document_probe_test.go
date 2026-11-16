package formulatex

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/authormath"
	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/texsandbox"
)

// TestRoundTripDocument runs the round trip on the formulas of RELATEX_FORMULA_DOC (a
// document.json or an export bundle, gzipped or not) and prints the share verified and a few
// formulas of each failure class (a development aid). RELATEX_FORMULA_SAMPLES sets how many.
func TestRoundTripDocument(t *testing.T) {
	path := os.Getenv("RELATEX_FORMULA_DOC")
	if path == "" {
		t.Skip("RELATEX_FORMULA_DOC not set")
	}
	authormath.Root, _ = filepath.Abs("../../..")
	nodes := documentMath(t, path)
	summary := Check(nodes, t.TempDir(), command.TeXLiveBin("lualatex"))
	fmt.Printf("formulas %d verified %d (%.1f%%) typeset %d in %.1f s\n", summary.Formulas, summary.Verified,
		100*float64(summary.Verified)/float64(max(summary.Formulas, 1)), summary.Typeset, summary.Seconds)
	samples := 2
	if text := os.Getenv("RELATEX_FORMULA_SAMPLES"); text != "" {
		fmt.Sscan(text, &samples)
	}
	prefix := keyPrefix(texsandbox.Current(command.TeXLiveBin("lualatex")))
	shown := map[string]int{}
	for _, class := range summary.Classes() {
		fmt.Printf("%6d  %s\n", summary.Failures[class], class)
	}
	for _, node := range nodes {
		mathml, _ := node["mathml"].(string)
		outcome, _ := readCache(cacheKey(prefix, mathml))
		if outcome.Status != "failed" || shown[outcome.Class] >= samples {
			continue
		}
		shown[outcome.Class]++
		formula, _ := Write(mathml)
		fmt.Printf("\n== %s\n%s\nTeX: %s\nback: %s\n", outcome.Class, mathml, formula.Standalone(), summary.typeset[mathml])
	}
}

func documentMath(t *testing.T, path string) []rec.Map {
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var reader io.Reader = file
	if strings.HasSuffix(path, ".gz") {
		if reader, err = gzip.NewReader(file); err != nil {
			t.Fatal(err)
		}
	}
	value, err := canonjson.Decode(reader)
	if err != nil {
		t.Fatal(err)
	}
	document := rec.AsMap(value)
	if payload, ok := document["payload"].(rec.Map); ok {
		document = rec.AsMap(payload["document"])
	}
	nodes := []rec.Map{}
	for _, node := range rec.AsMap(document["nodes"]) {
		if math, ok := node.(rec.Map); ok && math["type"] == "math" {
			nodes = append(nodes, math)
		}
	}
	return nodes
}
