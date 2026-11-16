package paperdoc

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// TestImageSpanTakesAFigureDescription: a span may carry a description from relatex/alt.toml
// (decision 086), always with a known provenance and never empty.
func TestImageSpanTakesAFigureDescription(t *testing.T) {
	base := func() rec.Map {
		return rec.Map{"type": "image", "id": "n-" + strings.Repeat("a", 64), "asset": strings.Repeat("b", 64),
			"width_sp": int64(10), "height_sp": int64(10)}
	}
	check := func(span rec.Map) (message string) {
		defer func() {
			if value := recover(); value != nil {
				message = value.(rec.Failure).Message
			}
		}()
		(&validation{data: rec.Map{"schema_version": "paper-0.2"}}).validateImageSpan(span)
		return ""
	}
	if got := check(base()); got != "" {
		t.Fatalf("a plain image span fails: %s", got)
	}
	described := base()
	described["alt"], described["alt_provenance"] = "A disk.", "reviewed"
	if got := check(described); got != "" {
		t.Errorf("a described span fails: %s", got)
	}
	for _, broken := range []rec.Map{{"alt": "A disk."}, {"alt": " ", "alt_provenance": "author"},
		{"alt": "A disk.", "alt_provenance": "guess"}, {"alt_provenance": "model"}} {
		span := base()
		for key, value := range broken {
			span[key] = value
		}
		if check(span) == "" {
			t.Errorf("accepted %v", broken)
		}
	}
}

// TestImageSpanTakesASourceDescription: the source's alt, long description and decorative mark
// (decision 096) are valid only in their shapes.
func TestImageSpanTakesASourceDescription(t *testing.T) {
	check := func(fields rec.Map) (valid bool) {
		span := rec.Map{"type": "image", "id": "n-" + strings.Repeat("a", 64), "asset": strings.Repeat("b", 64),
			"width_sp": int64(10), "height_sp": int64(10)}
		for key, value := range fields {
			span[key] = value
		}
		defer func() {
			if recover() != nil {
				valid = false
			}
		}()
		(&validation{data: rec.Map{"schema_version": "paper-0.2"}}).validateImageSpan(span)
		return true
	}
	for _, valid := range []rec.Map{
		{"alt": "A disk.", "alt_provenance": "source"},
		{"alt": "A disk.", "alt_provenance": "source", "long_description": "A disk of radius r around p."},
		{"alt": "A disk.", "alt_provenance": "reviewed", "long_description": "A disk.", "description_details": "Built in two steps."},
		{"alt": "A disk.", "alt_provenance": "source_macro", "long_description": "A disk.", "description_details": "From \\relatexalt."},
		{"alt": "Chess diagram, position in FEN: 8/8/8/8/8/8/8/8", "alt_provenance": "inferred"},
		{"decorative": true, "alt_provenance": "source"},
		{"decorative": true, "alt_provenance": "author"},
	} {
		if !check(valid) {
			t.Errorf("rejected %v", valid)
		}
	}
	for _, broken := range []rec.Map{
		{"decorative": true},
		{"decorative": false, "alt_provenance": "source"},
		{"decorative": true, "alt_provenance": "model"},
		{"decorative": true, "alt_provenance": "source_macro"},
		{"alt": "A disk.", "alt_provenance": "macro"},
		{"decorative": true, "alt": "A disk.", "alt_provenance": "source"},
		{"long_description": "Alone.", "alt_provenance": "source"},
		{"alt": "A disk.", "alt_provenance": "source", "long_description": " "},
		{"alt": "A disk.", "alt_provenance": "reviewed", "description_details": "Details without a short description."},
		{"alt": "A disk.", "alt_provenance": "reviewed", "long_description": "A disk.", "description_details": " "},
	} {
		if check(broken) {
			t.Errorf("accepted %v", broken)
		}
	}
}
