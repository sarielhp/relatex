package cli

import "testing"

// A run that wrote its products with diagnostics succeeds for a person and is 3 in a machine report.
func TestExitStatusOfADiagnosticRunDependsOnTheReader(t *testing.T) {
	cases := []struct {
		status string
		human  bool
		want   int
	}{
		{"ok", true, 0},
		{"ok", false, 0},
		{"diagnostic", true, 0},
		{"degraded", true, 0},
		{"unverified", true, 0},
		{"diagnostic", false, 3},
		{"degraded", false, 3},
		{"unverified", false, 3},
		{"failed", true, 1},
		{"failed", false, 1},
	}
	for _, c := range cases {
		if got := exitStatus(c.status, c.human); got != c.want {
			t.Errorf("exitStatus(%q, human %v) = %d, want %d", c.status, c.human, got, c.want)
		}
	}
}
