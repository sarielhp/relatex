package browser

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// The check expects the style the HTML writes (html.Paper.imageSpan prints each size once to six
// places). 13114596 sp is exactly 265.8203125 px: printed once it is 265.820312, rounded first
// and printed after it was 265.820313 (fifth cohort 1262).
func TestImageStyleMatchesTheWriterOnATie(t *testing.T) {
	span := rec.Map{"id": "n-0-image", "type": "image", "width_sp": int64(13_114_596), "height_sp": int64(8_698_150),
		"depth_sp": int64(0), "placement": rec.Map{}}
	written := "width:265.820312px;height:176.303178px;vertical-align:0.000000px"
	if got := imageStyle(span); got != written {
		t.Fatalf("imageStyle = %q, the HTML writes %q", got, written)
	}
	validateImageSize(rec.Map{"style": written, "width": 265.8203125, "height": 176.303178}, span)
}
