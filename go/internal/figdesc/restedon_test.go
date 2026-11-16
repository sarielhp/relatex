package figdesc

import (
	"strings"
	"testing"
)

func TestRestedOnRoundTripsAndOldSidecarsLoad(t *testing.T) {
	old, err := Parse("[figs.a]\nalt = \"Old draft.\"\nprovenance = \"model\"\nmodel = \"m\"\nprompt = \"relatex-alt-2\"\n")
	if err != nil || old.Entries["a"].RestedOn != "" {
		t.Fatalf("a sidecar without rested_on: %v", err)
	}
	sidecar, err := Parse("")
	if err != nil {
		t.Fatal(err)
	}
	if err = sidecar.PutDraft(Entry{Key: "b", Alt: "New draft.", Model: "m", Prompt: "relatex-alt-3", Drafted: "2026-11-11",
		RestedOn: "trace"}); err != nil {
		t.Fatal(err)
	}
	text := string(sidecar.Bytes())
	if !strings.Contains(text, "drafted = \"2026-11-11\"\nrested_on = \"trace\"\n") {
		t.Fatalf("rendered:\n%s", text)
	}
	reread, err := Parse(text)
	if err != nil || reread.Entries["b"].RestedOn != "trace" {
		t.Fatalf("reread %+v %v", reread.Entries["b"], err)
	}
	if accepted, err := reread.Accept("b", "2026-11-12"); !accepted || err != nil || reread.Entries["b"].RestedOn != "trace" {
		t.Errorf("accepting dropped rested_on: %v %v", accepted, err)
	}
	if _, err := Parse("[figs.c]\nalt = \"x\"\nrested_on = 3\n"); err == nil {
		t.Errorf("a rested_on that is not a string was accepted")
	}
}
