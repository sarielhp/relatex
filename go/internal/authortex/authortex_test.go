package authortex

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func testPairedMacros() rec.Map {
	return rec.Map{
		"absv": rec.Map{
			"parameters": int64(1),
			"body":       `\mathopen\lvert #1\mathclose\rvert`,
			"paired":     rec.Map{"before": "", "open": `\lvert`, "close": `\rvert`, "after": "", "inner": "#1"},
		},
		"pfloor": rec.Map{
			"parameters": int64(1),
			"body":       `\mathopen\lfloor #1\mathclose\rfloor`,
			"paired":     rec.Map{"before": "", "open": `\lfloor`, "close": `\rfloor`, "after": "", "inner": "#1"},
		},
		"pceil": rec.Map{
			"parameters": int64(1),
			"body":       `\mathopen\lceil #1\mathclose\rceil`,
			"paired":     rec.Map{"before": "", "open": `\lceil`, "close": `\rceil`, "after": "", "inner": "#1"},
		},
		"nrm": rec.Map{
			"parameters": int64(1),
			"body":       `\mathopen\lVert #1\mathclose\rVert`,
			"paired":     rec.Map{"before": "", "open": `\lVert`, "close": `\rVert`, "after": "", "inner": "#1"},
		},
		"barnorm": rec.Map{
			"parameters": int64(1),
			"body":       `\mathopen\|#1\mathclose\|`,
			"paired":     rec.Map{"before": "", "open": `\|`, "close": `\|`, "after": "", "inner": "#1"},
		},
		"ip": rec.Map{
			"parameters": int64(2),
			"body":       `\mathopen\langle #1, #2\mathclose\rangle`,
			"paired":     rec.Map{"before": "", "open": `\langle`, "close": `\rangle`, "after": "", "inner": "#1, #2"},
		},
		"Prob": rec.Map{
			"parameters": int64(1),
			"body":       `\mathbb{P}\mathopen[#1\mathclose]`,
			"paired":     rec.Map{"before": `\mathbb{P}`, "open": `[`, "close": `]`, "after": "", "inner": "#1"},
		},
		"cases": rec.Map{
			"parameters": int64(1),
			"body":       `\mathopen\{#1`,
			"paired":     rec.Map{"before": "", "open": `\{`, "close": `.`, "after": "", "inner": "#1"},
		},
		"plain": rec.Map{
			"parameters": int64(1),
			"body":       `\|#1\|`,
		},
	}
}

func TestExpandPairedVariants(t *testing.T) {
	kept := testPairedMacros()
	cases := []struct {
		name string
		tex  string
		want string
		ok   bool
	}{
		{"starred abs", `\absv*{\frac{a}{b}}`, `\left\lvert \frac{a}{b}\right\rvert`, true},
		{"sized abs big", `\absv[\big]{y}`, `\bigl\lvert y\bigr\rvert`, true},
		{"sized abs spaced Bigg", `\absv [\Bigg] {z}`, `\Biggl\lvert z\Biggr\rvert`, true},
		{"starred ceil", `\pceil*{\frac{n}{2}}`, `\left\lceil \frac{n}{2}\right\rceil`, true},
		{"sized floor", `\pfloor[\big]{x}`, `\bigl\lfloor x\bigr\rfloor`, true},
		{"sized norm", `\nrm[\Big]{w}`, `\Bigl\lVert w\Bigr\rVert`, true},
		{"symbol delimiters", `\barnorm*{v}`, `\left\|v\right\|`, true},
		{"two-parameter inner product", `\ip*{x}{y}`, `\left\langle x, y\right\rangle`, true},
		{"before prefix", `\Prob*{X}`, `\mathbb{P}\left[X\right]`, true},
		{"letter after closing control word", `\absv*{x}y + \absv*{x}^2`, `\left\lvert x\right\rvert y + \left\lvert x\right\rvert^2`, true},
		{"nested variants", `\absv*{\nrm[\big]{x} + 1}`, `\left\lvert \bigl\lVert x\bigr\rVert + 1\right\rvert`, true},
		{"escaped braces inside argument", `\absv*{\{x, y\}}`, `\left\lvert \{x, y\}\right\rvert`, true},
		{"unbraced single token arguments", `\absv* x + \absv*\alpha`, `\left\lvert x\right\rvert + \left\lvert \alpha\right\rvert`, true},
		{"plain use left untouched", `\absv{z} + \absv{\nrm*{w}}`, `\absv{z} + \absv{\left\lVert w\right\rVert}`, true},
		{"line break before macro name not misread", `a \\absv*{x}`, `a \\absv*{x}`, true},
		{"non-paired macro left untouched", `\plain{x}`, `\plain{x}`, true},
		{"starred null delimiter is accepted", `\cases*{x}`, `\left\{x\right.`, true},
		{"sized null delimiter falls back", `\cases[\big]{x}`, "", false},
		{"unknown size command falls back", `\absv[\large]{x}`, "", false},
		{"pre-sided command rejected", `\absv[\bigl]{x}`, "", false},
		{"empty size rejected", `\absv[]{x}`, "", false},
		{"unclosed size bracket falls back", `\absv[\big{x}`, "", false},
		{"unclosed argument falls back", `\absv*{x`, "", false},
	}
	for _, tc := range cases {
		got, ok := ExpandPairedVariants(tc.tex, kept)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("%s: got (%q, %v), want (%q, %v)", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}

func TestPairedVariantBoundary(t *testing.T) {
	kept := rec.Map{
		"abs": rec.Map{"parameters": int64(1), "body": `\mathopen|#1\mathclose|`, "paired": rec.Map{}},
	}
	if PairedVariant(`\absv*{x}`, kept) {
		t.Fatal(`\absv* should not match \abs`)
	}
	if PairedVariant(`a \\abs*{x}`, kept) {
		t.Fatal(`\\abs* is a linebreak plus text, not \abs`)
	}
	if !PairedVariant(`\abs*{x}`, kept) || !PairedVariant(`\abs [\big]{x}`, kept) {
		t.Fatal(`\abs* and \abs [\big] should match \abs`)
	}
}
