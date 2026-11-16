package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/sarielhp/relatex/internal/legacyinput"
	"github.com/sarielhp/relatex/internal/rec"
)

func hexSHA(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// latin1Input is the input manifest and probe report of test/fixtures/compat/latin1.tex (a source in
// latin1) beside an ASCII file, as the probe re-encoded it.
func latin1Input(t *testing.T) ([]any, rec.Map, []byte, string) {
	t.Helper()
	original, err := os.ReadFile(filepath.Join("..", "..", "..", "test", "fixtures", "compat", "latin1.tex"))
	if err != nil {
		t.Fatal(err)
	}
	text, ok := legacyinput.Decode(original, "latin1")
	if !ok {
		t.Fatal("the fixture is not latin1")
	}
	files := []any{
		rec.Map{"path": "figs", "directory": true},
		rec.Map{"path": "main.tex", "sha256": hexSHA(original), "bytes": int64(len(original))},
		rec.Map{"path": "macros.sty", "sha256": hexSHA([]byte("ascii")), "bytes": int64(5)},
	}
	report := rec.Map{
		"compatibility": []any{rec.Map{"shim": "inputenc"}},
		"reencoded_inputs": []any{rec.Map{"path": "main.tex", "encoding": "latin1",
			"sha256": hexSHA(original), "reencoded_sha256": hexSHA(text)}},
	}
	return files, report, original, hexSHA(text)
}

func reencodedFilesFailure(files []any, report rec.Map, original []byte) (err error) {
	defer rec.Recover(&err)
	reencodedFiles(files, report, func(string) []byte { return original })
	return nil
}

// The source map and the SyncTeX output read the builds' working copy: a re-encoded file has the digest
// of its archive bytes with the recorded re-encoding applied (decision 143); every other row stays.
func TestBuiltSourceFilesNameTheReencodedCopy(t *testing.T) {
	files, report, original, reencoded := latin1Input(t)
	result := reencodedFiles(files, report, func(path string) []byte {
		if path != "main.tex" {
			t.Fatalf("read %s", path)
		}
		return original
	})
	if got := rec.AsMap(result[1])["sha256"]; got != reencoded {
		t.Fatalf("main.tex %v, want %s", got, reencoded)
	}
	if rec.AsMap(files[1])["sha256"] != hexSHA(original) {
		t.Fatal("the input manifest changed")
	}
	for _, index := range []int{0, 2} {
		if !rec.Equal(result[index], files[index]) {
			t.Fatalf("row %d changed: %v", index, result[index])
		}
	}
	if got := reencodedFiles(files, rec.Map{}, nil); len(got) != 3 || !rec.Equal(got[1], files[1]) {
		t.Fatalf("without a record the rows stay: %v", got)
	}
}

// A record that differs from the archive or names a file the input does not have is refused.
func TestBuiltSourceFilesRefuseARecordTheArchiveDoesNotBear(t *testing.T) {
	files, report, original, _ := latin1Input(t)
	if err := reencodedFilesFailure(files, report, []byte("other bytes\xe9")); err == nil {
		t.Error("bytes that differ from the record were re-encoded")
	}
	missing := []any{files[0], files[2]}
	if err := reencodedFilesFailure(missing, report, original); err == nil {
		t.Error("a record for a file the input does not list was believed")
	}
	changed := rec.Map{"path": "main.tex", "sha256": hexSHA([]byte("x"))}
	if err := reencodedFilesFailure([]any{changed}, report, original); err == nil {
		t.Error("a manifest row with another digest was believed")
	}
}
