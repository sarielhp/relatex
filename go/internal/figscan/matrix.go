package figscan

import (
	"github.com/sarielhp/relatex/internal/affine"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
)

// matrix is an affine map [a b c d e f]: (x, y) -> (a x + c y + e, b x + d y + f), Ipe's and
// PDF's order.
type matrix [6]float64

var identity = matrix{1, 0, 0, 1, 0, 0}

// then is m followed by n: the point m maps is then mapped by n (n ∘ m).
func (m matrix) then(n matrix) matrix {
	return matrix(affine.Matrix(m).Then(affine.Matrix(n)))
}

func (m matrix) apply(x, y float64) Point {
	return Point{float64(m[0]*x) + float64(m[2]*y) + m[4], float64(m[1]*x) + float64(m[3]*y) + m[5]}
}

func (m matrix) determinant() float64 { return float64(m[0]*m[3]) - float64(m[1]*m[2]) }

// inverse of m; identity when m is singular.
func (m matrix) inverse() matrix {
	det := m.determinant()
	if det == 0 {
		return identity
	}
	a, b, c, d := m[3]/det, -m[1]/det, -m[2]/det, m[0]/det
	return matrix{a, b, c, d, -(float64(a*m[4]) + float64(c*m[5])), -(float64(b*m[4]) + float64(d*m[5]))}
}

// axes are the semi-axes of the ellipse m maps the unit circle to, and the direction (degrees,
// in [0, 180)) of the first, longer one.
func (m matrix) axes() (major, minor, angle float64) {
	a, b, c, d := m[0], m[1], m[2], m[3]
	// Singular values of [[a c] [b d]].
	s := float64(a*a) + float64(b*b) + float64(c*c) + float64(d*d)
	det := math.Abs(float64(a*d) - float64(b*c))
	root := math.Sqrt(math.Max(float64(s*s/4)-float64(det*det), 0))
	major = math.Sqrt(math.Max(float64(s/2)+root, 0))
	minor = math.Sqrt(math.Max(float64(s/2)-root, 0))
	// Direction of the major axis: eigenvector of M Mᵀ.
	p, q, r := float64(a*a)+float64(c*c), float64(a*b)+float64(c*d), float64(b*b)+float64(d*d)
	angle = 0.5 * math.Atan2(2*q, p-r) * 180 / math.Pi
	if angle < 0 {
		angle += 180
	}
	if major-minor < 1e-9*math.Max(major, 1) {
		angle = 0
	}
	return major, minor, angle
}

// restricted is m as ipelib's Painter::untransform leaves it for an object whose transformations
// attribute is kind: "translations" keeps only m's translation, "rigid" also the rotation that
// turns (1, 0) into m's image of it, anything else (affine) all of m.
func (m matrix) restricted(kind string) matrix {
	switch kind {
	case "translations":
		return matrix{1, 0, 0, 1, m[4], m[5]}
	case "rigid":
		angle := math.Atan2(m[1], m[0])
		cos, sin := math.Cos(angle), math.Sin(angle)
		return matrix{cos, sin, -sin, cos, m[4], m[5]}
	}
	return m
}

// parseMatrix reads "a b c d e f"; identity when absent or malformed.
func parseMatrix(text string) matrix {
	values := numbers(text)
	if len(values) != 6 {
		return identity
	}
	return matrix(values)
}

// numbers reads the whitespace-separated numbers of text, skipping anything else.
func numbers(text string) []float64 {
	var values []float64
	for _, field := range strings.Fields(text) {
		if value, err := strconv.ParseFloat(field, 64); err == nil {
			values = append(values, value)
		}
	}
	return values
}

func sortedKeys[V any](m map[int]V) []int { return slices.Sorted(maps.Keys(m)) }
