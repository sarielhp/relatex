package diagnostics

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// Every code the product adds by literal (diagnostics.Add("figure.icon_named", ...)) is registered in
// profiles/diagnostics.json: an unregistered code fails the conversion at reconcile ("Unregistered
// diagnostic code"), and only on the papers that reach it (2026-11-14: figure.icon_named failed 8 exports).
func TestEveryAddedCodeIsRegistered(t *testing.T) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	profile, err := LoadProfile(root)
	if err != nil {
		t.Fatal(err)
	}
	codes := rec.FetchMap(profile, "codes")
	added := regexp.MustCompile(`\.Add\("([a-z][a-z0-9_]*\.[a-z0-9_.]+)"`)
	missing := map[string]string{}
	walkErr := filepath.WalkDir(filepath.Join(root, "go"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		source, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, match := range added.FindAllSubmatch(source, -1) {
			if _, known := codes[string(match[1])]; !known {
				missing[string(match[1])] = path
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatal(walkErr)
	}
	names := make([]string, 0, len(missing))
	for code := range missing {
		names = append(names, code+" ("+missing[code]+")")
	}
	sort.Strings(names)
	if len(names) > 0 {
		t.Fatalf("codes added but not registered in profiles/diagnostics.json:\n%s", strings.Join(names, "\n"))
	}
}
