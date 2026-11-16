package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --keys-file reads one key per line, for selections longer than a command line (geom.bib's 13,000 keys).
func TestBibcleanCLIKeysFile(t *testing.T) {
	dir := t.TempDir()
	bibPath := filepath.Join(dir, "sample.bib")
	keysPath := filepath.Join(dir, "keys.txt")
	src := "@article{a1,\n  title = {A},\n  year = 2020,\n}\n@article{b2,\n  title = {B},\n  year = 2021,\n}\n"
	if err := os.WriteFile(bibPath, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keysPath, []byte("b2\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	status, stdout, stderr := run(t, "bibclean", "--keys-file", keysPath, bibPath)
	if status != 0 || !strings.Contains(stdout, "@article{b2,") || strings.Contains(stdout, "a1") {
		t.Fatalf("bibclean --keys-file: status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
}

func TestBibcleanCLIWithKeysAndReport(t *testing.T) {
	dir := t.TempDir()
	bibPath := filepath.Join(dir, "sample.bib")
	reportPath := filepath.Join(dir, "report.json")
	src := `
@string{SODA = "Proc. {\em(SODA)}"}
@inproceedings{a1,
  author = {J. H{\r a}stad},
  title = {Bounds on $k$-sets},
  booktitle = SODA,
  crossref = {p1},
}
@proceedings{p1,
  booktitle = SODA,
  year = 2020,
}
@article{u1,
  title = {With \epsilon math $\epsilon$},
}
`
	if err := os.WriteFile(bibPath, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	status, stdout, stderr := run(t, "bibclean", "--keys", "a1", bibPath)
	if status != 0 {
		t.Fatalf("bibclean --keys a1 status=%d stderr=%q", status, stderr)
	}
	if !strings.Contains(stdout, "@inproceedings{a1,") || !strings.Contains(stdout, "@proceedings{p1,") {
		t.Errorf("expected a1 and crossref p1 in stdout:\n%s", stdout)
	}
	if strings.Contains(stdout, "u1") {
		t.Errorf("unselected entry u1 should be omitted:\n%s", stdout)
	}
	if !strings.Contains(stderr, "bibclean: 2 converted, 0 unconverted") {
		t.Errorf("unexpected stderr summary: %q", stderr)
	}

	status, stdout, stderr = run(t, "bibclean", "--all", "--report", reportPath, bibPath)
	if status != 0 || stderr != "" {
		t.Fatalf("bibclean --all --report failed: status=%d stderr=%q", status, stderr)
	}
	if !strings.Contains(stdout, "@article{u1,") {
		t.Errorf("--all missing u1:\n%s", stdout)
	}
	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if unmarshalErr := json.Unmarshal(data, &parsed); unmarshalErr != nil {
		t.Fatalf("invalid report JSON: %v\n%s", unmarshalErr, string(data))
	}
	if parsed["converted"] != float64(2) || parsed["unconverted"] != float64(1) {
		t.Errorf("unexpected report counts: %v", parsed)
	}
}

func TestBibcleanCLIValidationErrors(t *testing.T) {
	dir := t.TempDir()
	bibPath := filepath.Join(dir, "empty.bib")
	if err := os.WriteFile(bibPath, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := [][]string{
		{"bibclean", bibPath},
		{"bibclean", "--all", "--keys", "a", bibPath},
		{"bibclean", "--all"},
	}
	for _, args := range cases {
		status, _, stderr := run(t, args...)
		if status == 0 || stderr == "" {
			t.Errorf("relatex %v unexpectedly succeeded: status=%d stderr=%q", args, status, stderr)
		}
	}
}
