package paperdoc

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// Decision 176, amendment 1: an image QED mark is named "End of proof" (inferred), never decorative and
// never without a name.
func TestQEDMarkImageIsNamedEndOfProof(t *testing.T) {
	mark := func(fields rec.Map) (valid bool) {
		image := rec.Map{"type": "image", "id": "n-" + strings.Repeat("a", 64), "asset": strings.Repeat("b", 64),
			"width_sp": int64(10), "height_sp": int64(10)}
		for key, value := range fields {
			image[key] = value
		}
		defer func() {
			if recover() != nil {
				valid = false
			}
		}()
		(&validation{data: rec.Map{"schema_version": "paper-0.2"}}).validateSymbolMark(rec.Map{"type": "symbol", "name": "qed", "image": image})
		return true
	}
	if !mark(rec.Map{"alt": EndOfProof, "alt_provenance": Inferred}) {
		t.Errorf("a named image mark fails")
	}
	if !mark(rec.Map{"decorative": true, "alt_provenance": Inferred}) {
		t.Errorf("an export converted before amendment 1 (decorative mark) is no longer read")
	}
	for _, broken := range []rec.Map{{}, {"decorative": true, "alt": "", "alt_provenance": Inferred}, {"alt": "A duck.", "alt_provenance": Inferred},
		{"alt": EndOfProof, "alt_provenance": "author"}} {
		if mark(broken) {
			t.Errorf("accepted %v", broken)
		}
	}
}
