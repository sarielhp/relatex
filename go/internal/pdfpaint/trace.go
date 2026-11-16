// Package pdfpaint is the Go port of relatex's PDF paint capture: the proofs that a table's
// rules and glyphs (PDFTablePaint, PDFRichTablePaint and their geometry, inventory, font
// and precision checks) and the body text (PDFTextPaint) are painted in the ordinary PDF as
// the reading says, read from mutool's paint traces and qpdf's object inventory. The proofs'
// validation and the precision model the capture shares are the model's (paintmodel).
package pdfpaint

import (
	"math/big"
	"strings"

	"github.com/sarielhp/relatex/internal/nativetrace"
	"github.com/sarielhp/relatex/internal/rec"
)

// The trace reader and its Ruby conversions are shared with the layout capture
// (nativetrace); these names keep this package's code as it reads.
type node = nativetrace.Node

func parseTrace(data []byte) *node         { return nativetrace.Parse(data) }
func parseTree(data []byte) (*node, error) { return nativetrace.ParseTree(data) }
func firstLine(text string) string         { return nativetrace.FirstLine(text) }
func rubyFloat(text string) float64        { return nativetrace.Float(text) }
func floats(text string) []float64         { return nativetrace.Floats(text) }

func eachElement(data []byte, visit func(name string, attrs map[string]string, open []*node)) {
	nativetrace.EachElement(data, visit)
}

// rescueArgument turns an ArgumentError into a Failure with prefix; others continue.
func rescueArgument(prefix string) {
	recovered := recover()
	if recovered == nil {
		return
	}
	if failure, ok := recovered.(rec.ArgumentError); ok {
		panic(rec.Failure{Message: prefix + failure.Message})
	}
	if failure, ok := recovered.(rec.Failure); ok && failure.Lookup {
		panic(rec.Failure{Message: prefix + failure.Message})
	}
	panic(recovered)
}

// opaque is Rational(alpha) == 1.
func opaque(alpha string) bool {
	value, ok := new(big.Rat).SetString(strings.TrimSpace(alpha))
	if !ok {
		rec.RaiseArgument("invalid value for convert(): %q", alpha)
	}
	return value.Cmp(big.NewRat(1, 1)) == 0
}

func identityPrefix(matrix []float64) bool {
	return len(matrix) >= 4 && matrix[0] == 1.0 && matrix[1] == 0.0 && matrix[2] == 0.0 && matrix[3] == -1.0
}

func floatsEqual(left, right []float64) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
