package contrast

import (
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

// Backdrops are the fills the PDF paints behind a faint ink's text (decision 113, the text paint
// proof's backdrops): for each ink, each fill with the ink's text on it, spaces left out.
type Backdrops map[string][]Backdrop

// Backdrop is one fill an ink is set on and the ink's text there.
type Backdrop struct {
	Fill string
	Text string
}

// DocumentBackdrops are the document's recorded backdrops; nil when it has none.
func DocumentBackdrops(document rec.Map) Backdrops {
	paint, _ := document["text_paint"].(rec.Map)
	list, _ := paint["backdrops"].([]any)
	if len(list) == 0 {
		return nil
	}
	result := Backdrops{}
	for _, value := range list {
		entry := rec.AsMap(value)
		ink, _ := entry["ink"].(string)
		fill, _ := entry["fill"].(string)
		text, _ := entry["text"].(string)
		if _, valid := Parse(fill); valid && ink != "" {
			result[ink] = append(result[ink], Backdrop{Fill: fill, Text: text})
		}
	}
	return result
}

// Of is the fill the PDF paints behind a text span: among its ink's backdrops, the one whose text
// holds the most of the span's characters, word by word (a word may hold a picture's two colours,
// a title page's band and the frame below it), the one the ink reads worst on among equals; ""
// when the span is not text in a recorded ink or one of its words is on no backdrop (the paper).
func (b Backdrops) Of(span rec.Map) string {
	color, colored := span["color"].(string)
	text, texted := span["text"].(string)
	backdrops := b[color]
	if !colored || !texted || span["type"] == "rule" || len(backdrops) == 0 {
		return ""
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return ""
	}
	held := make([]int, len(backdrops))
	for _, word := range words {
		found := false
		for index, backdrop := range backdrops {
			if strings.Contains(backdrop.Text, word) {
				held[index] += len([]rune(word))
				found = true
			}
		}
		if !found {
			return ""
		}
	}
	return b.chosen(color, held)
}

// chosen is the backdrop that holds the most characters, the ink reading worst on it among equals.
func (b Backdrops) chosen(color string, held []int) string {
	ink, _ := Parse(color)
	best, ratio := -1, 0.0
	for index, backdrop := range b[color] {
		fill, _ := Parse(backdrop.Fill)
		value := Ratio(ink, fill)
		if best < 0 || held[index] > held[best] || held[index] == held[best] && value < ratio {
			best, ratio = index, value
		}
	}
	return b[color][best].Fill
}

// fills are the backdrops' fills.
func (b Backdrops) fills() []string {
	result := []string{}
	for _, list := range b {
		for _, backdrop := range list {
			result = append(result, backdrop.Fill)
		}
	}
	return result
}
