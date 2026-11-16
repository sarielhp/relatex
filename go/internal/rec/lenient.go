package rec

// The lenient readers: a value of another type, or none, reads as the zero value, for code that
// reads a document or report whose keys may be absent (the As and Fetch readers fail instead).

// StringOf is value as a string, "" when it is not one.
func StringOf(value any) string {
	text, _ := value.(string)
	return text
}

// MapOf is value as an object, nil when it is not one.
func MapOf(value any) Map {
	m, _ := value.(Map)
	return m
}

// SliceOf is value as an array, nil when it is not one.
func SliceOf(value any) []any {
	list, _ := value.([]any)
	return list
}

// IntOf is value as an integer, 0 when it is not one.
func IntOf(value any) int64 {
	number, _ := value.(int64)
	return number
}
