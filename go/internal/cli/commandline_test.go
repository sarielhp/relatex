package cli

import (
	"io"

	"github.com/sarielhp/relatex/internal/ojson"
)

// commandResult runs a command line as the program does and returns the report its command made;
// a failure or a usage error is a panic, as inside the program.
func commandResult(arguments ...string) ojson.Object {
	current := newSession(io.Discard, io.Discard)
	if err := current.app().Execute(arguments); err != nil {
		panic(err)
	}
	return current.result
}
