package cli

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestResumedKeysDropAcceptanceAndKeepSplit(t *testing.T) {
	if Root == "" {
		Root, _ = filepath.Abs("../../..")
	}
	for _, stale := range []string{"verification_profile", "authored_proof", "frozen_pdf"} {
		if slices.Contains(resumedKeys, stale) {
			t.Fatalf("resumedKeys still contains %q", stale)
		}
	}
	if !slices.Contains(resumedKeys, "split") {
		t.Fatal("resumedKeys is missing split")
	}

	attempt := t.TempDir()
	runJSON := `{"configuration":{"formats":["html"],"split":"none","verification_profile":"draw12","authored_proof":"/tmp/p","frozen_pdf":"/tmp/f.pdf"},"main":"paper.tex"}`
	if err := os.WriteFile(filepath.Join(attempt, "run.json"), []byte(runJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	options := map[string]any{"cache": "auto"}
	resumedSettings(attempt, options)
	for _, stale := range []string{"verification_profile", "authored_proof", "frozen_pdf"} {
		if _, present := options[stale]; present {
			t.Fatalf("resumedSettings copied stale key %q", stale)
		}
	}
	if got := conversionOptions(options, t.TempDir(), t.TempDir(), t.TempDir(), nil); got.Split != "none" {
		t.Fatalf("Split = %q, want none", got.Split)
	}
}
