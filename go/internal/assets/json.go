package assets

import (
	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/rec"
)

// writeJSON is Relatex.write_json for evidence files beside a capture: indented JSON with
// Ruby's numbers. Keys are sorted, where Ruby keeps insertion order; readers parse it.
func writeJSON(path string, value any) {
	if err := canonjson.WriteFile(path, value); err != nil {
		rec.Fail("%v", err)
	}
}
