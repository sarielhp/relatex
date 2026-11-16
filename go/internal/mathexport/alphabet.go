package mathexport

import "github.com/sarielhp/relatex/internal/mathtokens"

// decode is MathAlphabet.decode: a character's base and its mathematical variant; ok is
// false when there is no variant (Ruby's nil), and the inherited variant applies.
func decode(character rune, inherited string, hasInherited bool) (string, string, bool) {
	base, variant, ok := mathtokens.DecodeAlphabet(character)
	if !ok {
		return base, inherited, hasInherited
	}
	return base, variant, true
}

// run is a stretch of text in one variant.
type run struct {
	text       string
	variant    string
	hasVariant bool
}

// runs is MathAlphabet.runs.
func runs(text string, inherited string, hasInherited bool) []run {
	result := []run{}
	for _, character := range text {
		value, variant, has := decode(character, inherited, hasInherited)
		if len(result) > 0 {
			last := &result[len(result)-1]
			if last.hasVariant == has && last.variant == variant {
				last.text += value
				continue
			}
		}
		result = append(result, run{text: value, variant: variant, hasVariant: has})
	}
	return result
}
