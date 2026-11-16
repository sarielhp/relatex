package html

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A colored span carries its fill as an inline color; an uncolored span carries none.
func TestRenderSpanWritesTheSpansColor(t *testing.T) {
	paper := &Paper{document: rec.Map{}, nodes: rec.Map{}}
	colored := paper.renderSpan(rec.Map{"type": "text", "text": "red", "color": "#ff0000"})
	if !strings.Contains(colored, `style="color:#ff0000"`) || !strings.Contains(colored, `data-kind="text">red<`) {
		t.Errorf("colored span = %s", colored)
	}
	plain := paper.renderSpan(rec.Map{"type": "text", "text": "black"})
	if strings.Contains(plain, "color") {
		t.Errorf("uncolored span has a color: %s", plain)
	}
	link := paper.renderSpan(rec.Map{"type": "reference", "text": "1", "label": "l", "target": "t", "color": "#0000ff"})
	if !strings.Contains(link, `style="color:#0000ff"`) || !strings.HasPrefix(link, "<a ") {
		t.Errorf("colored link = %s", link)
	}
}
