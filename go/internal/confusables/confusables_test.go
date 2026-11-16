package confusables

import "testing"

// The pairs decision 148 respells are confusable under UTS #39; mathematically different symbols
// are not.
func TestSkeletons(t *testing.T) {
	same := [][2]string{{"‖", "∥"}, {"–", "-"}, {"∗", "*"}, {"−", "-"}}
	for _, pair := range same {
		if !Confusable(pair[0], pair[1]) {
			t.Errorf("%q and %q: skeletons %q and %q", pair[0], pair[1], Skeleton(pair[0]), Skeleton(pair[1]))
		}
	}
	different := [][2]string{{"+", "−"}, {"<", "≤"}, {"=", "≠"}, {"∥", "|"}, {"–", "—"}}
	for _, pair := range different {
		if Confusable(pair[0], pair[1]) {
			t.Errorf("%q and %q are not the same character, skeleton %q", pair[0], pair[1], Skeleton(pair[0]))
		}
	}
}
