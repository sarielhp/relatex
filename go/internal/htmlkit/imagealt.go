package htmlkit

import (
	"regexp"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

// UndescribedAttribute marks on the page an image whose alt is a placeholder: nobody described
// it (decision 086) and it is not decorative (decision 111). The EPUB keys its evidence on the
// same flag, never on the alt text.
const UndescribedAttribute = "data-undescribed"

// UndescribedImage is the alternative of an image nobody described outside a captioned float.
const UndescribedImage = "Undescribed image"

// Undescribed is whether an image has no text alternative of its own: no author, sidecar or hint
// description and not decorative. The page's alt is then a placeholder, not an alternative.
func Undescribed(span rec.Map) bool {
	if span["decorative"] == true {
		return false
	}
	alt, ok := span["alt"].(string)
	return !ok || strings.TrimSpace(alt) == ""
}

// ImageAlt is an image's text alternative: the description the paper's relatex/alt.toml gave it
// (decision 086), else the label its float's caption is typeset with, else UndescribedImage.
func ImageAlt(span rec.Map, captionLabel string) string {
	if alt, ok := span["alt"].(string); ok && strings.TrimSpace(alt) != "" {
		return alt
	}
	if label := strings.TrimSpace(captionLabel); label != "" {
		return label
	}
	return UndescribedImage
}

// captionLabelPattern is the label a caption begins with: a float's label word and the number or
// letter LaTeX sets ("Figure 3", "Fig. 2", "Table 12", "TABLE I"). Any other opening words are
// caption text, never a label: the image then keeps UndescribedImage.
var captionLabelPattern = regexp.MustCompile(`^((?i:Figure|Fig\.?|Table|Tab\.|Algorithm|Alg\.|Listing|Scheme|Chart|Plate|Box|Exhibit)\s*` +
	`(?:[0-9]+[A-Za-z]?(?:\.[0-9]+)*|[IVXLCDM]+|[A-Z](?:\.[0-9]+)*))([\s.:]|$)`)

// CaptionLabel is the label a float's caption is typeset with, read from the caption node: the
// words and number at its start ("Figure 3", "Fig. 2", "Table 12"), or "" when it has none.
func CaptionLabel(caption rec.Map, nodes rec.Map) string {
	if caption == nil {
		return ""
	}
	var text strings.Builder
	for _, child := range rec.FetchList(caption, "children") {
		block := rec.Node(nodes, child)
		content, _ := block["content"].([]any)
		for _, value := range content {
			span := rec.AsMap(value)
			if span["type"] == "text" {
				run, _ := span["text"].(string)
				text.WriteString(run)
			}
		}
	}
	match := captionLabelPattern.FindStringSubmatch(strings.TrimSpace(text.String()))
	if match == nil {
		return ""
	}
	return strings.Join(strings.Fields(match[1]), " ")
}
