package papersource

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sarielhp/relatex/internal/legacyinput"
	"github.com/sarielhp/relatex/internal/rec"
)

// latin1Source is a probe directory holding test/fixtures/compat/latin1.tex (a source in latin1, the
// Ruby suite's fixture) as its archive file, with the probe report that re-encoded it as the build did.
func latin1Source(t *testing.T) (*Source, []byte, []byte) {
	t.Helper()
	original, err := os.ReadFile(filepath.Join("..", "..", "..", "test", "fixtures", "compat", "latin1.tex"))
	if err != nil {
		t.Fatal(err)
	}
	text, ok := legacyinput.Decode(original, "latin1")
	if !ok {
		t.Fatal("the fixture is not latin1")
	}
	directory := t.TempDir()
	if err := os.MkdirAll(filepath.Join(directory, "input", "files"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "input", "files", "paper.tex"), original, 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Source{directory: directory, main: "paper.tex", manifest: rec.Map{"input/files/paper.tex": digest(original)},
		report: rec.Map{
			"compatibility": []any{rec.Map{"shim": "inputenc", "cause": "inputenc is not designed for xetex or luatex"}},
			"reencoded_inputs": []any{rec.Map{"path": "paper.tex", "encoding": "latin1",
				"sha256": digest(original), "reencoded_sha256": digest(text)}},
		}}
	return s, original, text
}

func sourceArtifactFailure(s *Source) (err error) {
	defer rec.Recover(&err)
	s.sourceArtifact(s.main)
	return nil
}

// The input is the archive's file with the recorded re-encoding applied (decision 143): what the pass
// stream's header names (RELATEX_INPUT, the working copy's digest) and what the checkers read.
func TestTheInputIsTheArchiveFileReencodedAsRecorded(t *testing.T) {
	s, original, text := latin1Source(t)
	if got := s.sourceArtifact("paper.tex"); string(got) != string(text) {
		t.Fatalf("source %q", got)
	}
	s.report = rec.Map{}
	if got := s.sourceArtifact("paper.tex"); string(got) != string(original) {
		t.Fatal("without a record the archive's bytes are the input")
	}
}

// A record the archive's bytes do not bear, or one without the inputenc shim, is refused: the identity
// check stays as strong as before.
func TestAReencodingRecordMustMatchTheArchive(t *testing.T) {
	tamper := map[string]func(entry, report rec.Map){
		"archive digest": func(entry, _ rec.Map) { entry["sha256"] = digest([]byte("x")) },
		"result digest":  func(entry, _ rec.Map) { entry["reencoded_sha256"] = digest([]byte("x")) },
		"encoding":       func(entry, _ rec.Map) { entry["encoding"] = "latin2" },
		"no shim":        func(_, report rec.Map) { delete(report, "compatibility") },
	}
	for name, change := range tamper {
		s, _, _ := latin1Source(t)
		change(rec.AsMap(s.report["reencoded_inputs"].([]any)[0]), s.report)
		if err := sourceArtifactFailure(s); err == nil || err.Error() != "Re-encoded input record mismatch" {
			t.Errorf("%s: %v", name, err)
		}
	}
}
