// Package assets is the Go port of the reconcile stage's media: media assets' per-placement
// derivatives (MediaDerivatives), the paper's asset inventory (PaperMediaAssets,
// PaperPNGAssets), figures cropped from the PDF (RenderedGraphics) and the independent page count
// (PDFPageCount). The descriptors they produce (MediaAsset, PNGImage, the SVG safety check) are
// the document model's, in mediamodel (decision 102).
package assets

import (
	"crypto/sha256"
	"encoding/hex"
)

func hexDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
