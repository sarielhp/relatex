package sourcealt

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/figdesc"
	"github.com/sarielhp/relatex/internal/rec"
)

func evidence() *Evidence {
	return Read([]rec.Map{
		{"code": "graphic_alternative", "use": int64(1), "alt": "A red  square", "figure": int64(1)},
		{"code": "graphic_alternative", "use": int64(2), "artifact": true},
		{"code": "graphic_alternative", "use": int64(3), "alt": ""},
		{"code": "graphic_alternative", "use": int64(4), "actualtext": "XY"},
		{"code": "graphic_alternative", "use": int64(5), "figure": int64(2)},
		{"code": "graphic_alternative", "use": int64(6), "figure": int64(3)},
		{"code": "figure_description", "figure": int64(1), "description": "Ignored: the key wins."},
		{"code": "figure_description", "figure": int64(2), "short": "Two squares", "description": "Two grey squares side by side."},
		{"code": "graphic_use", "use": int64(7)},
	})
}

func TestForTakesKeysBeforeTheFiguresDescription(t *testing.T) {
	cases := []struct {
		use  int64
		want Description
		ok   bool
	}{
		{1, Description{Alt: "A red square"}, true},
		{2, Description{Decorative: true}, true},
		{3, Description{Decorative: true}, true},
		{4, Description{Alt: "XY"}, true},
		{5, Description{Alt: "Two squares", Long: "Two grey squares side by side."}, true},
		{6, Description{}, false},
		{7, Description{}, false},
	}
	source := evidence()
	for _, test := range cases {
		got, ok := source.For(test.use)
		if got != test.want || ok != test.ok {
			t.Errorf("use %d: %+v %v, want %+v %v", test.use, got, ok, test.want, test.ok)
		}
	}
	if _, ok := source.For("1"); ok {
		t.Error("a use that is no number has no description")
	}
}

func TestSplitKeepsAShortTextAndSplitsALongOne(t *testing.T) {
	if got, _ := Split("", "A disk."); got != (Description{Alt: "A disk."}) {
		t.Errorf("short text: %+v", got)
	}
	long := "A plot of the error against n. " + strings.Repeat("It falls quickly and then levels off. ", 6)
	got, ok := Split("", long)
	if !ok || got.Alt != "A plot of the error against n." || got.Long != Normalize(long) {
		t.Errorf("long text: %+v", got)
	}
	if got, ok := Split("", "  "); ok || got != (Description{}) {
		t.Errorf("empty: %+v", got)
	}
	if got, _ := Split("Same", "Same"); got != (Description{Alt: "Same"}) {
		t.Errorf("equal short and long: %+v", got)
	}
}

func TestLeadClipsALongSentenceAtAWord(t *testing.T) {
	sentence := strings.Repeat("word ", 60)
	lead := Lead(Normalize(sentence))
	if len([]rune(lead)) > ShortLimit || !strings.HasSuffix(lead, "…") || strings.Contains(lead, "wo…") {
		t.Errorf("lead %q", lead)
	}
}

func TestApplySetsTheSpanFields(t *testing.T) {
	span := rec.Map{"type": "image"}
	Apply(span, Description{Alt: "A", Long: "A long text."})
	if span["alt"] != "A" || span["long_description"] != "A long text." || !figdesc.FromSource(span) {
		t.Errorf("described: %v", span)
	}
	decorative := rec.Map{"type": "image"}
	Apply(decorative, Description{Decorative: true})
	if decorative["decorative"] != true || decorative["alt"] != nil || !figdesc.FromSource(decorative) {
		t.Errorf("decorative: %v", decorative)
	}
}

func TestOriginOfIsWhereTheSourceIncludesTheGraphic(t *testing.T) {
	evidence := Read([]rec.Map{
		{"code": "graphic_origin", "use": int64(1), "file": "36_frechet/frechet.tex", "line": int64(120)},
		{"code": "graphic_origin", "use": int64(2), "file": "a.tex", "line": int64(0)},
	})
	if origin, ok := evidence.OriginOf(int64(1)); !ok || origin != (Origin{File: "36_frechet/frechet.tex", Line: 120}) {
		t.Fatalf("origin %+v %v", origin, ok)
	}
	for _, use := range []any{int64(2), int64(3), "1"} {
		if _, ok := evidence.OriginOf(use); ok {
			t.Errorf("use %v has an origin", use)
		}
	}
}
