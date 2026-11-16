package rec

// Merge is Hash#merge over several hashes: a new hash, later keys replacing earlier ones.
func Merge(maps ...Map) Map {
	result := Map{}
	for _, m := range maps {
		for key, value := range m {
			result[key] = value
		}
	}
	return result
}

// Pick is Hash#slice: the given keys that are present, in a new hash.
func Pick(m Map, keys ...string) Map {
	result := Map{}
	for _, key := range keys {
		if value, ok := m[key]; ok {
			result[key] = value
		}
	}
	return result
}

// FetchOr is Hash#fetch(key, fallback).
func FetchOr(m Map, key string, fallback any) any {
	if value, ok := m[key]; ok {
		return value
	}
	return fallback
}

// ListOf is a value to iterate as a list: nil is empty, and so is an empty object, since
// lualibs writes an empty Lua table as {}.
func ListOf(value any) []any {
	if value == nil {
		return []any{}
	}
	if list, ok := value.([]any); ok {
		return list
	}
	if m, ok := value.(Map); ok && len(m) == 0 {
		return []any{}
	}
	return AsList(value)
}
