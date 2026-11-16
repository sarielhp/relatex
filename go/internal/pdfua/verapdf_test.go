package pdfua

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/command"
)

func TestReadValidation(t *testing.T) {
	passed := readValidation([]byte(`{"report":{"jobs":[{"validationResult":[{"compliant":true,"profileName":"PDF/UA-2",
		"details":{"ruleSummaries":[]}}]}]}}`))
	if passed.Status != "passed" || passed.Profile != "PDF/UA-2" {
		t.Fatalf("%+v", passed)
	}
	failed := readValidation([]byte(`{"report":{"jobs":[{"validationResult":{"compliant":false,"profileName":"PDF/UA-2",
		"details":{"ruleSummaries":[{"clause":"8.11.1","testNumber":1,"status":"failed","failedChecks":1,"description":"dc:title"}]}}}]}}`))
	if failed.Status != "failed" || len(failed.Failures) != 1 || !strings.Contains(failedRules(failed), "8.11.1-1 dc:title (1)") {
		t.Fatalf("%+v", failed)
	}
	if readValidation([]byte("not json")).Status != "" {
		t.Fatal("garbage read as a verdict")
	}
}

// TestValidatorFromConfiguration: the configuration's tools.verapdf finds veraPDF off PATH, RELATEX_VERAPDF
// overrides it, and a configured path that names no program finds nothing (decision 129, amendment 4).
func TestValidatorFromConfiguration(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake veraPDF is a shell script")
	}
	directory := t.TempDir()
	program := filepath.Join(directory, "verapdf")
	if err := os.WriteFile(program, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	t.Setenv("RELATEX_VERAPDF", "")
	defer command.ConfigureTools(nil)
	command.ConfigureTools(map[string]string{"verapdf": program})
	if got := validator(); got != program {
		t.Errorf("configured: got %q, want %q", got, program)
	}
	command.ConfigureTools(map[string]string{"verapdf": filepath.Join(directory, "missing")})
	if got := validator(); got != "" {
		t.Errorf("a configured path naming no program found %q", got)
	}
	other := filepath.Join(t.TempDir(), "verapdf")
	if err := os.WriteFile(other, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RELATEX_VERAPDF", other)
	if got := validator(); got != other {
		t.Errorf("RELATEX_VERAPDF did not override the configuration: %q", got)
	}
}
