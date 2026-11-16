package sourcetext

import (
	"reflect"
	"testing"
)

// The expected units and calls are Ruby's SourceVisibleText on the same text.
func TestVisibleMatchesRuby(t *testing.T) {
	text := "Caf\\'e -- x % c\n\\textbf{b}\\label{q} $y$ ``z''"
	visible := New(text, []Span{{Start: 36, Stop: 39}})
	want := []Unit{{"C", 0, 1, 1, true}, {"a", 1, 2, 1, false}, {"f", 2, 3, 1, false}, {"é", 3, 6, 1, false},
		{"–", 7, 9, 1, true}, {"x", 10, 11, 1, true}, {"b", 24, 25, 2, true}, {"“", 40, 42, 2, true},
		{"z", 42, 43, 2, false}, {"”", 43, 45, 2, false}}
	if !reflect.DeepEqual(visible.Units, want) {
		t.Errorf("units %v\nRuby %v", visible.Units, want)
	}
	calls := []Call{{"textbf", 16, 26}, {"label", 26, 35}}
	if !reflect.DeepEqual(visible.Calls, calls) {
		t.Errorf("calls %v, Ruby %v", visible.Calls, calls)
	}
}
