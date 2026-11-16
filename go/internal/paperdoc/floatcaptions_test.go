package paperdoc

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func captionBlock(text string) []rec.Map {
	return []rec.Map{{"content": []any{rec.Map{"type": "text", "text": text}}}}
}

func TestNamedCaptions(t *testing.T) {
	caption := func(entity int64, numbered bool) rec.Map {
		return rec.Map{"entity": entity, "numbered": numbered}
	}
	f := &floats{byEntity: map[any][]rec.Map{
		int64(1): captionBlock("Figure 2: First caption."),
		int64(2): captionBlock("(a) Left sub"),
		int64(3): captionBlock("Fig. 3. Second caption."),
		int64(4): captionBlock("TABLE IV Results"),
		int64(5): captionBlock("Figure 9: not numbered"),
	}}
	cases := map[string]struct {
		captions []rec.Map
		want     int
	}{
		"two captions of a float":        {[]rec.Map{caption(1, true), caption(3, true)}, 2},
		"a subcaption is not one":        {[]rec.Map{caption(2, true), caption(1, true)}, 1},
		"roman numbered table caption":   {[]rec.Map{caption(4, true)}, 1},
		"an unnumbered caption":          {[]rec.Map{caption(5, false)}, 0},
		"a caption with no block at all": {[]rec.Map{caption(7, true)}, 0},
	}
	for name, test := range cases {
		if got := len(f.namedCaptions(test.captions)); got != test.want {
			t.Errorf("%s: %d named captions, want %d", name, got, test.want)
		}
	}
}
