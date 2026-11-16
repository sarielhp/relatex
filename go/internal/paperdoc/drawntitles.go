package paperdoc

import (
	"fmt"

	"github.com/sarielhp/relatex/internal/rec"
)

// drawnTitleCode is the collector's record of a frame title a beamer theme set while a picture was
// open (relatex-beamer.lua): the title is drawn in a TikZ node, not a heading (decision 128, d).
const drawnTitleCode = "frame_title_in_picture"

// reportDrawnTitles says how many frame titles a theme drew in a picture: they stay text where the
// slide prints them, and the export has no heading for them.
func (b *builder) reportDrawnTitles(rows []rec.Map) {
	drawn := countDrawnTitles(rows)
	if drawn == 0 {
		return
	}
	b.diagnostics.Add("heading.drawn_in_picture", fmt.Sprintf("%d frame titles are drawn in a picture by the theme; "+
		"they stay text where the slide prints them and are not headings", drawn), int64(drawn))
}

func countDrawnTitles(rows []rec.Map) int {
	drawn := 0
	for _, row := range rows {
		if rec.Fetch(row, "code") == drawnTitleCode {
			drawn++
		}
	}
	return drawn
}
