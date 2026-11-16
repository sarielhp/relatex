package paperdoc

import "github.com/sarielhp/relatex/internal/rec"

// validateLineBreak: the break between two lines of a heading's title box (decision 126) is a text
// span of one space marked line_break; the space is what every text reader reads between the lines.
func validateLineBreak(span rec.Map, kind any) {
	value, marked := span["line_break"]
	if marked && (value != true || kind != "text" || span["text"] != " ") {
		rec.Fail("Invalid heading line break")
	}
}
