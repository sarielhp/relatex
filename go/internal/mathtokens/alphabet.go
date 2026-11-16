package mathtokens

import (
	"strings"

	"golang.org/x/text/unicode/norm"
)

// alphabetRange is a block of mathematical alphanumeric characters and its variant.
type alphabetRange struct {
	first, last rune
	variant     string
}

// latin and greek are MathAlphabet::LATIN and GREEK.
var (
	latin = []alphabetRange{
		{0x1D400, 0x1D433, "bold"}, {0x1D434, 0x1D467, "italic"},
		{0x1D468, 0x1D49B, "bold-italic"}, {0x1D49C, 0x1D4CF, "script"},
		{0x1D4D0, 0x1D503, "bold-script"}, {0x1D504, 0x1D537, "fraktur"},
		{0x1D538, 0x1D56B, "double-struck"}, {0x1D56C, 0x1D59F, "bold-fraktur"},
		{0x1D5A0, 0x1D5D3, "sans-serif"}, {0x1D5D4, 0x1D607, "bold-sans-serif"},
		{0x1D608, 0x1D63B, "sans-serif-italic"}, {0x1D63C, 0x1D66F, "sans-serif-bold-italic"},
		{0x1D670, 0x1D6A3, "monospace"}, {0x1D6A4, 0x1D6A5, "italic"},
		{0x1D7CE, 0x1D7D7, "bold"}, {0x1D7D8, 0x1D7E1, "double-struck"},
		{0x1D7E2, 0x1D7EB, "sans-serif"}, {0x1D7EC, 0x1D7F5, "bold-sans-serif"}, {0x1D7F6, 0x1D7FF, "monospace"},
	}
	greek = []alphabetRange{
		{0x1D6A8, 0x1D6E1, "bold"}, {0x1D6E2, 0x1D71B, "italic"},
		{0x1D71C, 0x1D755, "bold-italic"}, {0x1D756, 0x1D78F, "bold-sans-serif"},
		{0x1D790, 0x1D7C9, "sans-serif-bold-italic"},
	}
	greekBases = []rune("ΑΒΓΔΕΖΗΘΙΚΛΜΝΞΟΠΡϴΣΤΥΦΧΨΩ∇αβγδεζηθικλμνξοπρςστυφχψω∂ϵϑϰϕϱϖ")
	// letterlike is MathAlphabet::LETTERLIKE, in its order.
	letterlike = []struct{ characters, variant string }{
		{"ℂℍℕℙℚℝℤ", "double-struck"}, {"ℋℐℒℛℬℰℱℳℴℯℊℓ", "script"}, {"ℭℌℑℜℨ", "fraktur"}, {"ℎ", "italic"},
	}
	digamma = map[rune]string{0x1D7CA: "Ϝ", 0x1D7CB: "ϝ"}
)

// DecodeAlphabet is MathAlphabet.decode: a mathematical alphanumeric character's base and variant
// ("bold", "italic", "double-struck"...); ok is false for any other character, whose base is itself.
func DecodeAlphabet(character rune) (base, variant string, ok bool) {
	for _, block := range greek {
		if character >= block.first && character <= block.last {
			return string(greekBases[character-block.first]), block.variant, true
		}
	}
	if base, ok := digamma[character]; ok {
		return base, "bold", true
	}
	for _, block := range latin {
		if character >= block.first && character <= block.last {
			return norm.NFKC.String(string(character)), block.variant, true
		}
	}
	for _, group := range letterlike {
		if strings.ContainsRune(group.characters, character) {
			return norm.NFKC.String(string(character)), group.variant, true
		}
	}
	return string(character), "", false
}

// alphabets is BrowserMathML::ALPHABETS: (base, variant) to the mathematical character.
var alphabets = buildAlphabets()

type alphabetKey struct{ base, variant string }

func buildAlphabets() map[alphabetKey]string {
	characters := []rune{}
	for _, block := range append(append([]alphabetRange{}, latin...), greek...) {
		for codepoint := block.first; codepoint <= block.last; codepoint++ {
			characters = append(characters, codepoint)
		}
	}
	for _, group := range letterlike {
		characters = append(characters, []rune(group.characters)...)
	}
	characters = append(characters, 0x1D7CA, 0x1D7CB)
	result := map[alphabetKey]string{}
	for _, character := range characters {
		base, variant, _ := DecodeAlphabet(character)
		result[alphabetKey{base, variant}] = string(character)
	}
	return result
}

// EncodeAlphabet is the mathematical alphanumeric character of a base in a variant, if Unicode has one.
func EncodeAlphabet(base, variant string) (string, bool) {
	character, ok := alphabets[alphabetKey{base, variant}]
	return character, ok
}
