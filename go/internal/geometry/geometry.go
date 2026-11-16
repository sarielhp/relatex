// Package geometry is the Go port of Relatex::Geometry: affine matrices [a,b,c,d,e,f] mapping
// (x,y) to (a*x+c*y+e, b*x+d*y+f), boxes [x0,y0,x1,y1] and TeX unit conversions. The
// arithmetic is the Ruby reference's, operation for operation, so results agree bit for bit;
// every product is rounded with float64(...) so no architecture fuses it into a multiply-add
// (decision 132).
package geometry

import (
	"math"

	"github.com/sarielhp/relatex/internal/rec"
)

// Identity is Geometry::IDENTITY.
var Identity = []float64{1, 0, 0, 1, 0, 0}

const spPerTeXPoint = 65_536.0

// bpPerTeXPoint is divided at run time, as Ruby does: a constant expression would be folded
// exactly and round differently in the last bit.
var bpPerTeXPoint = bigPoints / texPoints

var bigPoints, texPoints = 72.0, 72.27

// Numbers is Geometry.numbers: a vector of count finite numbers, as floats.
func Numbers(values []any, count int) []float64 {
	if len(values) != count {
		rec.Fail("Invalid geometry vector")
	}
	result := make([]float64, count)
	for index, value := range values {
		number, ok := rec.Number(value)
		if !ok || math.IsInf(number, 0) || math.IsNaN(number) {
			rec.Fail("Nonfinite geometry coordinate")
		}
		result[index] = number
	}
	return result
}

// Floats converts a float vector to JSON values.
func Floats(values []float64) []any {
	result := make([]any, len(values))
	for index, value := range values {
		result[index] = value
	}
	return result
}

// Point is Geometry.point.
func Point(matrix []float64, x, y float64) (float64, float64) {
	a, b, c, d, e, f := matrix[0], matrix[1], matrix[2], matrix[3], matrix[4], matrix[5]
	return float64(a*x) + float64(c*y) + e, float64(b*x) + float64(d*y) + f
}

// Inverse is Geometry.inverse.
func Inverse(matrix []float64) []float64 {
	a, b, c, d, e, f := matrix[0], matrix[1], matrix[2], matrix[3], matrix[4], matrix[5]
	determinant := float64(a*d) - float64(b*c)
	if determinant == 0 {
		rec.Fail("Singular placement transform")
	}
	return []float64{d / determinant, -b / determinant, -c / determinant, a / determinant,
		(float64(c*f) - float64(d*e)) / determinant, (float64(b*e) - float64(a*f)) / determinant}
}

// Compose is Geometry.compose: outer after inner.
func Compose(outer, inner []float64) []float64 {
	a, b, c, d, e, f := outer[0], outer[1], outer[2], outer[3], outer[4], outer[5]
	g, h, i, j, k, l := inner[0], inner[1], inner[2], inner[3], inner[4], inner[5]
	return []float64{float64(a*g) + float64(c*h), float64(b*g) + float64(d*h),
		float64(a*i) + float64(c*j), float64(b*i) + float64(d*j),
		float64(a*k) + float64(c*l) + e, float64(b*k) + float64(d*l) + f}
}

// Box is Geometry.box: four numbers with x0 <= x1 and y0 <= y1.
func Box(values []any) []float64 {
	box := Numbers(values, 4)
	if !(box[0] <= box[2] && box[1] <= box[3]) {
		rec.Fail("Reversed geometry box")
	}
	return box
}

// TransformedBox is Geometry.transformed_box: the bounds of a box's four transformed corners.
func TransformedBox(matrix, box []float64) []float64 {
	if !(box[0] <= box[2] && box[1] <= box[3]) {
		rec.Fail("Reversed geometry box")
	}
	x0, y0, x1, y1 := box[0], box[1], box[2], box[3]
	corners := [4][2]float64{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}}
	xs, ys := make([]float64, 4), make([]float64, 4)
	for index, corner := range corners {
		xs[index], ys[index] = Point(matrix, corner[0], corner[1])
	}
	return []float64{Minimum(xs), Minimum(ys), Maximum(xs), Maximum(ys)}
}

// Minimum is Ruby's Array#min on floats (the first of equal values).
func Minimum(values []float64) float64 {
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}

// Maximum is Ruby's Array#max on floats (the first of equal values).
func Maximum(values []float64) float64 {
	result := values[0]
	for _, value := range values[1:] {
		if value > result {
			result = value
		}
	}
	return result
}

// SPToBP is Geometry.sp_to_bp.
func SPToBP(value float64) float64 {
	return float64(value / spPerTeXPoint * bpPerTeXPoint)
}

// BPToSP rounds a length in bp to sp as MediaPlacement#to_sp does (half away from zero).
func BPToSP(value float64) int64 {
	return int64(math.Round(value / bpPerTeXPoint * spPerTeXPoint))
}
