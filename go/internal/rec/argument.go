package rec

import (
	"fmt"
	"strconv"
	"strings"
)

// ArgumentError is a Ruby ArgumentError or KeyError raised while reading a trace or a proof
// (Float("x"), a missing attribute); unlike a Failure, only the callers that rescue it say
// which.
type ArgumentError struct{ Message string }

// RaiseArgument panics with an ArgumentError.
func RaiseArgument(format string, args ...any) {
	panic(ArgumentError{Message: fmt.Sprintf(format, args...)})
}

// ToI is String#to_i: leading whitespace, an optional sign and the digits that follow; 0
// when there are none.
func ToI(text string) int64 {
	text = strings.TrimLeft(text, " \t\n\v\f\r")
	end := 0
	if end < len(text) && (text[end] == '-' || text[end] == '+') {
		end++
	}
	for end < len(text) && text[end] >= '0' && text[end] <= '9' {
		end++
	}
	value, _ := strconv.ParseInt(text[:end], 10, 64)
	return value
}
