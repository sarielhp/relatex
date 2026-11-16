package html

import (
	"math"
	"strconv"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/rubynum"
)

// Numbers here are Ruby's (rubynum): an int64 (Integer) or a float64 (Float). Integer arithmetic
// stays Integer (division floors), anything with a Float is a Float, as the Ruby reference
// computes the geometry the outputs print.

// roundInteger is Float#round (no digits): half away from zero.
func roundInteger(value float64) int64 { return int64(math.Round(value)) }

// toS is Ruby's to_s (string interpolation) of a JSON value.
func toS(value any) string { return rec.ToS(value) }

// fixed is format('%.6f', value).
func fixed(value any) string { return strconv.FormatFloat(rubynum.Float(value), 'f', 6, 64) }
