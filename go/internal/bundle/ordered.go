package bundle

import (
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
)

// Ordered is a JSON object whose keys keep the order Ruby inserted them in, for the files
// Ruby writes with Relatex.write_json (JSON.pretty_generate of a Hash, not sorted).
type Ordered = ojson.Object

// Pair is one key of an Ordered object.
type Pair = ojson.Pair

// stageOrder is Command.run's result in Ruby's key order.
func stageOrder(stage rec.Map) Ordered {
	return Ordered{{"command", stage["command"]}, {"exit", stage["exit"]}, {"signal", stage["signal"]},
		{"timeout", stage["timeout"]}, {"seconds", stage["seconds"]}}
}

// writeJSON is Relatex.write_json: pretty JSON and a newline, through a temporary file.
func writeJSON(path string, value any) { check(ojson.WriteFile(path, value)) }
