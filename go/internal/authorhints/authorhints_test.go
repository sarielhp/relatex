package authorhints

import (
	"os"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/rec"
)

func contract(t *testing.T) rec.Map {
	t.Helper()
	data, err := os.ReadFile("../../../schemas/relatex-hints.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	value, err := canonjson.Decode(strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	return rec.AsMap(value)
}

func parse(t *testing.T, sources []Source, json string) *Hints {
	t.Helper()
	hints, err := Parse(sources, []byte(json), contract(t))
	if err != nil {
		t.Fatal(err)
	}
	return hints
}

func TestOneLineAndBlock(t *testing.T) {
	hints := parse(t, []Source{{Path: "main.tex", Text: "%% relatex speech: \\Im = \"image\"\n" +
		"%% relatex alt: fig:a = \"a disk\"\n" +
		"%% relatex speech:\n" +
		"%%   \\Re = \"real part\"\n" +
		"%%   \\tr = \"trace\"\n" +
		"text\n" +
		"%%   \\deg = \"degree\"\n"}}, "")
	if got, _ := hints.Get("speech", "Im"); got.Value != "image" || got.File != "main.tex" || got.Line != 1 {
		t.Errorf("Im = %+v", got)
	}
	if got, _ := hints.Get("alt", "fig:a"); got.Value != "a disk" {
		t.Errorf("fig:a = %+v", got)
	}
	if got, _ := hints.Get("speech", "Re"); got.Value != "real part" {
		t.Errorf("Re = %+v", got)
	}
	if _, ok := hints.Get("speech", "deg"); ok {
		t.Error("deg was read after the block ended")
	}
}

func TestBlockEndsAtNextHeader(t *testing.T) {
	hints := parse(t, []Source{{Path: "main.tex", Text: "%% relatex speech:\n%%   \\Im = \"image\"\n" +
		"%% relatex alt:\n%%   fig:a = \"a disk\"\n" +
		"%%   \\Re = \"real part\"\n"}}, "")
	if _, ok := hints.Get("speech", "Re"); ok {
		t.Error("Re stayed in the speech section after the alt header")
	}
	if got, _ := hints.Get("alt", "Re"); got.Value != "real part" {
		t.Errorf("Re belongs to the section its header opened, got %+v", got)
	}
}

func TestJSONWinsAndLaterFileWins(t *testing.T) {
	hints := parse(t, []Source{
		{Path: "a.tex", Text: "%% relatex speech: \\Im = \"imaginary\"\n"},
		{Path: "b.tex", Text: "%% relatex speech: \\Im = \"image\"\n"},
	}, `{"speech": {"Im": "the image"}}`)
	if got, _ := hints.Get("speech", "Im"); got.Value != "the image" || got.File != FileName {
		t.Errorf("Im = %+v", got)
	}
	if len(hints.Conflicts) != 2 {
		t.Fatalf("conflicts = %+v", hints.Conflicts)
	}
	if hints.Conflicts[0].Winner.Value != "image" || hints.Conflicts[0].Loser.Value != "imaginary" {
		t.Errorf("first conflict = %+v", hints.Conflicts[0])
	}
}

func TestUnknownSection(t *testing.T) {
	hints := parse(t, []Source{{Path: "main.tex", Text: "%% relatex sound: \\Im = \"image\"\n"}}, "")
	if len(hints.Unknown) != 1 || hints.Unknown[0].Section != "sound" {
		t.Errorf("unknown = %+v", hints.Unknown)
	}
}

func TestBadJSONAgainstSchema(t *testing.T) {
	if _, err := Parse(nil, []byte(`{"speech": "image"}`), contract(t)); err == nil {
		t.Fatal("a speech section that is not an object was accepted")
	}
	if _, err := Parse(nil, []byte(`{"sound": {"Im": "image"}}`), contract(t)); err == nil {
		t.Fatal("an unknown section was accepted")
	}
}
