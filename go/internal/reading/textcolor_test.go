package reading

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func TestTextPaintIsRecordedPerSlotAndConflictsStayBlack(t *testing.T) {
	e := &Evidence{textPaint: map[ruleKey]string{}, activeSlot: 3}
	owner := rec.Map{"owner": int64(7)}
	e.recordTextPaint(rec.Map{"owner": int64(7), "slot": int64(3), "color": "#ff0000"})
	e.recordTextPaint(rec.Map{"owner": int64(7), "slot": int64(4), "color": "#00ff00"})
	if got := e.textColor(owner); got != "#ff0000" {
		t.Errorf("slot 3 color = %q, want #ff0000", got)
	}
	e.activeSlot = 9
	if got := e.textColor(owner); got != "" {
		t.Errorf("an unrecorded slot has color %q, want black (empty)", got)
	}
	e.recordTextPaint(rec.Map{"owner": int64(7), "slot": int64(3), "color": "#0000ff"})
	e.activeSlot = 3
	if got := e.textColor(owner); got != "" {
		t.Errorf("a slot shipped in two colors has color %q, want none", got)
	}
}

func TestColorValueIsAbsentForBlack(t *testing.T) {
	if colorValue("") != nil || colorValue("#ff0000") != "#ff0000" {
		t.Error("colorValue must be nil for black and the color otherwise")
	}
}
