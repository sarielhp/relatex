package gaplocate

import "testing"

// A float's keys are its labels, its printed number and its caption's name for it; a caption block
// places its float's label too, and a block without a float has no keys.
func TestFloatKeys(t *testing.T) {
	document, sourceMap := fixture()
	locator := New(document, sourceMap)
	keys := locator.FloatKeys("f1")
	if len(keys) != 3 || keys[0] != "tab:one" || keys[1] != "7" || keys[2] != "Table 7" {
		t.Errorf("keys = %v", keys)
	}
	if got := locator.FloatKeys(""); len(got) != 0 {
		t.Errorf("keys without a float = %v", got)
	}
	if place := locator.Caption("k1"); place.Label != "tab:one" || place.Float != "table_float" {
		t.Errorf("caption place = %+v", place)
	}
	if ids := locator.FloatTables("f1"); len(ids) != 2 || ids[0] != "outer" || locator.FloatOf("p1") != "f1" {
		t.Errorf("tables = %v", ids)
	}
}
