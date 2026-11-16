package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/rec"
)

func TestSettledBuildIsReusedOnlyWhenItsKeyAndFilesMatch(t *testing.T) {
	directory := t.TempDir()
	build := filepath.Join(directory, "build")
	if err := os.MkdirAll(build, 0o755); err != nil {
		t.Fatal(err)
	}
	aux := filepath.Join(build, "paper.aux")
	if err := os.WriteFile(aux, []byte("\\relax\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeJSON(filepath.Join(directory, "settled.json"), rec.Map{"schema_version": settleVersion, "key": "k1",
		"files": rec.Map{"paper.aux": command.FileDigest(aux)}})
	seed := reusedSeed(directory, "k1")
	if seed == nil || len(seed.names) != 1 || seed.names[0] != "paper.aux" {
		t.Fatalf("expected the settled build to be reused, got %#v", seed)
	}
	if reusedSeed(directory, "k2") != nil {
		t.Fatal("a different key must not reuse the settled build")
	}
	if err := os.WriteFile(aux, []byte("\\relax changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if reusedSeed(directory, "k1") != nil {
		t.Fatal("a changed settled file must not be reused")
	}
}

func TestAKeptSeedIsOnlyUsedWithTheBibliographyInputsItWasMadeFrom(t *testing.T) {
	directory := t.TempDir()
	build := filepath.Join(directory, "build")
	if err := os.MkdirAll(build, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(build, "paper.aux"), []byte("aux"), 0o644); err != nil {
		t.Fatal(err)
	}
	record := rec.Map{"schema_version": settleVersion, "key": "k", "bibliography": "one",
		"files": rec.Map{"paper.aux": command.FileDigest(filepath.Join(build, "paper.aux"))}}
	writeJSON(filepath.Join(directory, "settled.json"), record)
	if keptSeed(directory, "one") == nil {
		t.Error("a kept seed was refused for the bibliography inputs it was made from")
	}
	if keptSeed(directory, "two") != nil {
		t.Error("a kept seed was used with other bibliography inputs")
	}
}
