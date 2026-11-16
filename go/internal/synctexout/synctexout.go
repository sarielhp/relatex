// Package synctexout is the SyncTeX output (decisions 074 and 101, plans/archive/modular_outputs.md phase 3):
// the native SyncTeX file retargeted to the packaged sources, the word-level SyncTeX file written
// from the layout and the source map the core produced, and relatex synctex's side of another
// tool's PDF (wait for it, match it with the PDF relatex built, install the SyncTeX file beside it).
// It reads the model and utilities only; the paper's sources reach it through Sources, which the
// orchestration supplies from the core's artifact resolver.
package synctexout

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
)

// Ordered is a JSON object whose keys keep the order Ruby inserted them in.
type Ordered = ojson.Object

// Pair is one key of an Ordered object.
type Pair = ojson.Pair

// systemError is an operating-system error, which Ruby raises as SystemCallError.
type systemError struct{ error }

func check(err error) {
	if err != nil {
		panic(systemError{err})
	}
}

// writeJSON is Relatex.write_json: pretty JSON and a newline, through a temporary file.
func writeJSON(path string, value any) { check(ojson.WriteFile(path, value)) }

func hexSum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// toS is Ruby's to_s on a JSON number or string.
func toS(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return string(marshal(value))
}

func marshal(value any) []byte {
	bytes, err := canonjson.Marshal(value)
	if err != nil {
		rec.Fail("%v", err)
	}
	return bytes
}
