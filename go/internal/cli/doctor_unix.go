//go:build !windows

package cli

// platformNotes are doctor's lines for this platform: none on Unix.
func platformNotes() []string { return nil }
