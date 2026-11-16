package html

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rubynum"
)

// The expectations are Ruby's (Array#sum, Float#round(n)).
func TestSumMatchesRuby(t *testing.T) {
	tenths := []any{}
	for range 10 {
		tenths = append(tenths, 0.1)
	}
	cases := []struct {
		values []any
		want   any
	}{{[]any{int64(1), 2.5, 0.1, 0.2}, 3.8}, {tenths, 1.0}, {[]any{int64(3), int64(4)}, int64(7)}, {[]any{1e16, 1.0, -1e16}, 1.0}}
	for _, item := range cases {
		if got := rubynum.Sum(item.values); got != item.want {
			t.Errorf("sum(%v) = %v, want %v", item.values, got, item.want)
		}
	}
}

func TestRoundMatchesRuby(t *testing.T) {
	cases := []struct {
		value  float64
		digits int
		want   float64
	}{{0.1234567895, 9, 0.12345679}, {2.675, 2, 2.68}, {-1.0000000005, 9, -1.000000001}, {1e-13, 9, 0}, {-2.3684757858670005e-15, 9, 0}}
	for _, item := range cases {
		if got := rubynum.RoundDigits(item.value, item.digits); got != item.want {
			t.Errorf("round(%v, %d) = %v, want %v", item.value, item.digits, got, item.want)
		}
	}
}
