package semantic

import (
	"path/filepath"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func TestMarkdownPairedDelimiterVariantsShipTeX(t *testing.T) {
	saved := Root
	Root = filepath.Join("..", "..", "..")
	defer func() { Root = saved }()

	doc := rec.Map{
		"math_macros": rec.Map{
			"absv": rec.Map{
				"parameters": int64(1),
				"body":       `\mathopen\lvert #1\mathclose\rvert`,
				"paired":     rec.Map{"before": "", "open": `\lvert`, "close": `\rvert`, "after": "", "inner": "#1"},
			},
			"nrm": rec.Map{
				"parameters": int64(1),
				"body":       `\mathopen\lVert #1\mathclose\rVert`,
				"paired":     rec.Map{"before": "", "open": `\lVert`, "close": `\rVert`, "after": "", "inner": "#1"},
			},
			"cases": rec.Map{
				"parameters": int64(1),
				"body":       `\mathopen\{#1`,
				"paired":     rec.Map{"before": "", "open": `\{`, "close": `.`, "after": "", "inner": "#1"},
			},
			"badwrap": rec.Map{
				"parameters": int64(1),
				"body":       `\absv*{#1}`,
			},
		},
	}
	node := func(tex string) rec.Map {
		return rec.Map{"type": "math", "tex": rec.Map{"status": "verified", "form": "inline", "text": tex}}
	}
	m := &markdown{}
	m.export = &export{w: m, document: doc}

	cases := []struct {
		name string
		tex  string
		want string
		ok   bool
	}{
		{"starred abs", `\absv*{\frac{a}{b}}`, `\left\lvert \frac{a}{b}\right\rvert`, true},
		{"sized abs", `\absv[\big]{y}`, `\bigl\lvert y\bigr\rvert`, true},
		{"nested sized norm inside starred abs", `\absv*{\nrm[\Big]{x}}`, `\left\lvert \Bigl\lVert x\Bigr\rVert\right\rvert`, true},
		{"unknown size still falls back", `\absv[\large]{y}`, "", false},
		{"sized null delimiter falls back", `\cases[\big]{x}`, "", false},
		{"macro body with paired variant falls back", `\badwrap{x}`, "", false},
	}
	for _, tc := range cases {
		got, ok := m.authorMath(node(tc.tex))
		if ok != tc.ok || got != tc.want {
			t.Errorf("%s: got (%q, %v), want (%q, %v)", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}

func TestMarkdownGroupMathExpandsPairedVariants(t *testing.T) {
	saved := Root
	Root = filepath.Join("..", "..", "..")
	defer func() { Root = saved }()

	doc := rec.Map{
		"math_macros": rec.Map{
			"absv": rec.Map{
				"parameters": int64(1),
				"body":       `\mathopen\lvert #1\mathclose\rvert`,
				"paired":     rec.Map{"before": "", "open": `\lvert`, "close": `\rvert`, "after": "", "inner": "#1"},
			},
		},
	}
	r1 := rec.Map{
		"type": "math",
		"tex":  rec.Map{"status": "verified", "group": "g1", "row": int64(1), "rows": int64(2), "environment": "align*", "text": `a &= \absv*{x}`},
	}
	r2 := rec.Map{
		"type": "math",
		"tex":  rec.Map{"status": "verified", "group": "g1", "row": int64(2), "rows": int64(2), "environment": "align*", "text": `b &= \absv[\big]{y}`},
	}
	m := &markdown{}
	m.export = &export{w: m, document: doc, nodes: rec.Map{"r1": r1, "r2": r2}}
	got, ok := m.authorMath(r1)
	want := "\\begin{aligned}\na &= \\left\\lvert x\\right\\rvert\\\\\nb &= \\bigl\\lvert y\\bigr\\rvert\n\\end{aligned}"
	if !ok || got != want {
		t.Fatalf("groupMath: got (%q, %v), want (%q, true)", got, ok, want)
	}
}
