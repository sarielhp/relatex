package paperdoc

import (
	"path/filepath"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/schema"
)

func TestHeadingLineBreakIsOneSpaceOfText(t *testing.T) {
	validateLineBreak(rec.Map{"type": "text", "text": " ", "line_break": true}, "text")
	validateLineBreak(rec.Map{"type": "text", "text": "plain"}, "text")
	for _, span := range []rec.Map{
		{"type": "text", "text": "Proof", "line_break": true},
		{"type": "text", "text": " ", "line_break": "yes"},
		{"type": "reference", "text": " ", "line_break": true},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("accepted %v", span)
				}
			}()
			validateLineBreak(span, span["type"])
		}()
	}
}

func TestBuiltSpanKeepsTheHeadingLineBreak(t *testing.T) {
	b := &builder{}
	if got := b.buildSpan(rec.Map{"type": "text", "text": " ", "line_break": true}); got["line_break"] != true {
		t.Errorf("the line break is lost: %v", got)
	}
	if got := b.buildSpan(rec.Map{"type": "text", "text": "a"}); got["line_break"] != nil {
		t.Errorf("an ordinary span has no line break: %v", got)
	}
}

// The reading's text spans may carry the line break in the inspection contract (a reading with one
// failed at reconcile, "Schema alternative", before the contract knew it).
func TestInspectionContractAcceptsTheHeadingLineBreak(t *testing.T) {
	Root = filepath.Join("..", "..", "..")
	block := rec.FetchMap(rec.FetchMap(rec.FetchMap(rec.FetchMap(contract("inspection.schema.json"), "properties"), "reading"), "properties"), "blocks")
	item := rec.FetchMap(rec.FetchMap(rec.FetchMap(rec.FetchMap(block, "items"), "properties"), "content"), "items")
	text := rec.AsMap(rec.FetchList(item, "oneOf")[0])
	schema.Validate(rec.Map{"type": "text", "text": " ", "line_break": true}, text, "$")
	defer func() {
		if recover() == nil {
			t.Error("a line break that is not true was accepted")
		}
	}()
	schema.Validate(rec.Map{"type": "text", "text": " ", "line_break": "yes"}, text, "$")
}
