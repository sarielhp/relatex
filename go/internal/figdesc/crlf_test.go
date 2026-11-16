package figdesc

import (
	"strings"
	"testing"
)

// An alt.toml written on Windows has CRLF line ends: it reads as the LF file does, and relatex
// writes it back with CRLF throughout, its own lines included (decision 129).
func TestACRLFSidecarReadsAndKeepsItsLineEnds(t *testing.T) {
	text := "# Figure descriptions\r\n\r\n[figs.plot]\r\nalt = \"My plot.\"\r\n"
	sidecar, err := Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	if entry := sidecar.Entries["plot"]; entry == nil || entry.Alt != "My plot." {
		t.Fatalf("entries %+v", sidecar.Entries)
	}
	if err = sidecar.Set("plot", "alt", "A rising curve."); err != nil {
		t.Fatal(err)
	}
	if err = sidecar.RecordApproval(Approval{Upload: true, Approved: "2026-11-12", Command: "describe"}); err != nil {
		t.Fatal(err)
	}
	written := string(sidecar.Bytes())
	if strings.Count(written, "\n") != strings.Count(written, "\r\n") || !strings.Contains(written, "alt = \"A rising curve.\"\r\n") {
		t.Fatalf("written with mixed line ends: %q", written)
	}
	again, err := Parse(written)
	if err != nil || again.Entries["plot"].Alt != "A rising curve." || !again.Approval.Upload {
		t.Fatalf("rereading: %+v, %v", again, err)
	}
	if lf, _ := Parse("[figs.plot]\nalt = \"x\"\n"); strings.Contains(string(lf.Bytes()), "\r") {
		t.Fatal("an LF file stays LF")
	}
}
