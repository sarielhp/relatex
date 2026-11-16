// Package affine composes PDF transformation matrices [a b c d e f].
package affine

// Matrix is a PDF transformation matrix: x' = a·x + c·y + e, y' = b·x + d·y + f.
type Matrix [6]float64

// Then is m followed by n: a point m maps is then mapped by n (n ∘ m, PDF's m × n).
func (m Matrix) Then(n Matrix) Matrix {
	return Matrix{
		mul(m[0], n[0]) + mul(m[1], n[2]), mul(m[0], n[1]) + mul(m[1], n[3]),
		mul(m[2], n[0]) + mul(m[3], n[2]), mul(m[2], n[1]) + mul(m[3], n[3]),
		mul(m[4], n[0]) + mul(m[5], n[2]) + n[4], mul(m[4], n[1]) + mul(m[5], n[3]) + n[5],
	}
}

// mul is a rounded product: the conversion forbids fusing it into a multiply-add, so the
// matrix is the same on every architecture (decision 132).
func mul(x, y float64) float64 { return float64(x * y) }
