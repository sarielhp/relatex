// The page's visual accessibility checks (plan accessibility §7.2.4, decisions 095, 102): the
// declarations of the page's stylesheet the accessibility report looks for, and the contrast of
// its two palettes. The page (html) writes the stylesheet; the report (accessibility) reads it.

package htmlkit

import (
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/wcag"
	"regexp"
	"strings"
)

// VisualRules are the declarations of visualStyle the accessibility report looks for in a page,
// by check: a focus ring on every focusable element (2.4.7), a still target when the reader asks
// for reduced motion (2.3.3), and focus, rules and borders kept in forced colours.
var VisualRules = map[string][]string{
	"focus_visible":  {"a:focus-visible", "button:focus-visible", "[tabindex]:focus-visible", "outline: 2px solid var(--focus);"},
	"reduced_motion": {"@media (prefers-reduced-motion: reduce) { :target { animation: none;"},
	"forced_colors": {"@media (forced-colors: active)", "outline: 2px solid Highlight;",
		"background: CanvasText !important;", "border-left-color: CanvasText;"},
}

// MissingVisualRules are the declarations of a check (focus_visible, reduced_motion,
// forced_colors) that a stylesheet lacks.
func MissingVisualRules(check, style string) []string {
	missing := []string{}
	for _, rule := range VisualRules[check] {
		if !strings.Contains(style, rule) {
			missing = append(missing, rule)
		}
	}
	return missing
}

// ContrastPair is one foreground of the page's palette on the backgrounds it is shown on, with
// the ratio WCAG asks of it: 4.5:1 for text (1.4.3), 3:1 for rules, borders and the focus ring
// (1.4.11).
type ContrastPair struct {
	Foreground  string
	Backgrounds []string
	Minimum     float64
}

// visualContrastPairs are the palette's foregrounds and the backgrounds behind them, in both themes.
var visualContrastPairs = []ContrastPair{
	{"--muted", []string{"--bg", "--card"}, wcag.TextRatio},
	{"--link", []string{"--bg", "--card", "--theorem-bg", "--proof-bg"}, wcag.TextRatio},
	{"--rule", []string{"--bg", "--card"}, 3},
	{"--theorem", []string{"--bg", "--theorem-bg"}, 3},
	{"--proof", []string{"--bg", "--proof-bg"}, 3},
	{"--alert", []string{"--bg", "--theorem-bg"}, 3},
	{"--focus", []string{"--bg", "--card", "--theorem-bg", "--proof-bg", "--target"}, 3},
}

var (
	customProperty = regexp.MustCompile(`(--[a-z-]+):\s*(#[0-9a-fA-F]{6}|#[0-9a-fA-F]{3})\b`)
	darkRoot       = regexp.MustCompile(`(?m)^:root \{([^}]*)\}`)
	lightRoot      = regexp.MustCompile(`:root\[data-theme="light"\] \{([^}]*)\}`)
)

// ThemeContrastGaps reads the two palettes of a page's stylesheet (the dark :root and the light
// :root[data-theme="light"]) and lists each pair below its minimum: {"theme", "foreground",
// "background", "ratio"}. A palette or colour the page does not declare is a gap too.
func ThemeContrastGaps(page string) []any {
	gaps := []any{}
	for _, theme := range []struct {
		name    string
		pattern *regexp.Regexp
	}{{"dark", darkRoot}, {"light", lightRoot}} {
		match := theme.pattern.FindStringSubmatch(page)
		if match == nil {
			gaps = append(gaps, rec.Map{"theme": theme.name, "foreground": "", "background": "", "ratio": float64(0)})
			continue
		}
		gaps = append(gaps, paletteGaps(theme.name, match[1])...)
	}
	return gaps
}

func paletteGaps(theme, declarations string) []any {
	palette := map[string]wcag.RGB{}
	for _, match := range customProperty.FindAllStringSubmatch(declarations, -1) {
		if color, ok := wcag.Parse(expandHex(match[2])); ok {
			palette[match[1]] = color
		}
	}
	gaps := []any{}
	for _, pair := range visualContrastPairs {
		for _, background := range pair.Backgrounds {
			fore, foreKnown := palette[pair.Foreground]
			back, backKnown := palette[background]
			ratio := 0.0
			if foreKnown && backKnown {
				ratio = wcag.Ratio(fore, back)
			}
			if ratio < pair.Minimum {
				gaps = append(gaps, rec.Map{"theme": theme, "foreground": pair.Foreground, "background": background,
					"ratio": float64(int(ratio*100)) / 100})
			}
		}
	}
	return gaps
}

// expandHex is #rgb as #rrggbb; #rrggbb stays.
func expandHex(hex string) string {
	if len(hex) != 4 {
		return strings.ToLower(hex)
	}
	return strings.ToLower("#" + strings.Repeat(hex[1:2], 2) + strings.Repeat(hex[2:3], 2) + strings.Repeat(hex[3:4], 2))
}
