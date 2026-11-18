package html

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func frameRule(width, height, left, right any) rec.Map {
	span := rec.Map{"type": "rule", "width_sp": width, "height_sp": height, "depth_sp": int64(0)}
	if left != nil {
		span["margin_left_sp"], span["margin_right_sp"] = left, right
	}
	return span
}

func frameText(text string) rec.Map { return rec.Map{"type": "text", "text": text} }

// svjour3's \makeheadbox: top rule and left rule end the first paragraph, the right rule and the
// bottom rule end the last (p14/beer).
func TestRuleFrameWrapsTheBoxOfFourRules(t *testing.T) {
	const wide, stroke, tall = int64(22151168), int64(26214), int64(1707873)
	nodes := rec.Map{
		"p1": rec.Map{"id": "p1", "type": "paragraph", "content": []any{
			frameRule(wide, stroke, nil, nil), frameRule(stroke, tall, int64(0), -stroke)}},
		"p2": rec.Map{"id": "p2", "type": "paragraph", "content": []any{frameText("Noname manuscript No.")}},
		"p3": rec.Map{"id": "p3", "type": "paragraph", "content": []any{frameText("(will be inserted)"), frameText(" "),
			frameRule(stroke, tall, wide-stroke, -wide), frameText(" "), frameRule(wide, stroke, nil, nil)}},
		"p4": rec.Map{"id": "p4", "type": "paragraph", "content": []any{frameText("Title")}},
		"d":  rec.Map{"id": "d", "type": "document", "children": []any{"p1", "p2", "p3", "p4"}},
	}
	html := (&Paper{document: rec.Map{}, nodes: nodes}).RenderNode("d")
	if strings.Count(html, `data-frame=`) != 3 || strings.Count(html, `data-frame-hidden`) != 4 {
		t.Fatalf("frame is not one box without rules: %s", html)
	}
	open, closed := strings.Index(html, `data-frame="first"`), strings.Index(html, `data-frame="last"`)
	if !(open < strings.Index(html, "Noname") && closed < strings.Index(html, "inserted") && strings.Index(html, "inserted") < strings.Index(html, "Title") && !strings.Contains(html[strings.Index(html, `id="p4"`)-30:], "data-frame=")) {
		t.Fatalf("frame does not surround its paragraphs and only them: %s", html)
	}
}

func TestRuleFrameLeavesLoneRulesAlone(t *testing.T) {
	const wide, stroke, tall = int64(22151168), int64(26214), int64(1707873)
	nodes := rec.Map{
		"p1": rec.Map{"id": "p1", "type": "paragraph", "content": []any{
			frameRule(wide, stroke, nil, nil), frameRule(stroke, tall, int64(0), -stroke)}},
		"p2": rec.Map{"id": "p2", "type": "paragraph", "content": []any{frameText("text")}},
		"d":  rec.Map{"id": "d", "type": "document", "children": []any{"p1", "p2"}},
	}
	html := (&Paper{document: rec.Map{}, nodes: nodes}).RenderNode("d")
	if strings.Contains(html, "data-frame=") || strings.Count(html, `data-kind="rule"`) != 2 {
		t.Fatalf("unmatched rules were changed: %s", html)
	}
}

// An \fbox around a minipage reads as one paragraph: top and left rules, the text, the right and
// bottom rules (phd, walk Figure 9.1; master_thesis \shabox).
func TestRuleFrameBordersABoxedParagraph(t *testing.T) {
	const wide, stroke, tall = int64(8437104), int64(26214), int64(2752512)
	nodes := rec.Map{
		"p1": rec.Map{"id": "p1", "type": "paragraph", "content": []any{
			frameText(" "), frameRule(wide, stroke, nil, nil), frameRule(stroke, tall, nil, nil), frameText("boxed text"),
			frameText(" "), frameRule(stroke, tall, nil, nil), frameRule(wide, stroke, nil, nil)}},
		"p2": rec.Map{"id": "p2", "type": "paragraph", "content": []any{
			frameRule(wide, stroke, nil, nil), frameText("a lone rule before text")}},
		"p3": rec.Map{"id": "p3", "type": "paragraph", "content": []any{
			frameText("text read before the rules"), frameText(" "), frameText("more"), frameRule(wide, stroke, nil, nil),
			frameRule(stroke, tall, nil, nil), frameText(" "), frameRule(stroke, tall, nil, nil), frameRule(wide, stroke, nil, nil)}},
		"p4": rec.Map{"id": "p4", "type": "paragraph", "content": []any{
			frameRule(wide, stroke, nil, nil), frameRule(stroke, tall, nil, nil), frameText("shadowed"),
			frameText(" "), frameRule(stroke, tall, nil, nil), frameRule(wide, stroke, nil, nil),
			frameRule(4*stroke, tall-stroke, nil, nil), frameText(" "), frameRule(wide, 4*stroke, nil, nil)}},
		"d": rec.Map{"id": "d", "type": "document", "children": []any{"p1", "p2", "p3", "p4"}},
	}
	html := (&Paper{document: rec.Map{}, nodes: nodes}).RenderNode("d")
	if strings.Count(html, `data-frame="box"`) != 2 || strings.Count(html, `data-frame="shadow"`) != 1 ||
		strings.Count(html, `data-frame-hidden`) != 14 || strings.Count(html, `data-kind="rule"`) != 15 {
		t.Fatalf("boxed paragraph is not one border around its text with its four rules kept hidden: %s", html)
	}
}
