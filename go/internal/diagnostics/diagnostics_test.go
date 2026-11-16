package diagnostics

import (
	"path/filepath"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func TestShippedProfileIncludesInputUnsafe(t *testing.T) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	profile, loadErr := LoadProfile(root)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	codes := rec.FetchMap(profile, "codes")
	entry, ok := codes["input.unsafe"].(rec.Map)
	if !ok {
		t.Fatal("profiles/diagnostics.json is missing input.unsafe")
	}
	if entry["level"] != "fatal" {
		t.Fatalf("input.unsafe level = %v, want fatal", entry["level"])
	}
	if summary, _ := entry["summary"].(string); summary == "" {
		t.Fatal("input.unsafe summary is empty")
	}
}
