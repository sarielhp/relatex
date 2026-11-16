package html

import (
	"errors"

	"github.com/sarielhp/relatex/internal/canonjson"
)

// crash is a Ruby error that is no Relatex::Failure (a NoMethodError): nothing that
// rescues Failures catches it.
func crash(message string) { panic(errors.New(message)) }

// rubyInspect is String#inspect for the identifiers in lookup messages.
func rubyInspect(text string) string {
	encoded, err := canonjson.Marshal(text)
	if err != nil {
		return text
	}
	return string(encoded)
}
