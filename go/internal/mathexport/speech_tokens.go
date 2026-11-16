package mathexport

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The speech fixes of tokens and juxtaposed identifiers (plan accessibility_completion M4, decision
// 114), found by MathCAT (the engine of NVDA and JAWS) on the reviewed speech sample. Like decision
// 100's pass they change no glyph, space or stretch: an invisible operator, a literal intent on a
// token, or an h the math-auto transform shows as the ℎ it replaces.

const (
	// invisibleTimes is U+2062 INVISIBLE TIMES.
	invisibleTimes = "\u2062"
	// planck is U+210E, the italic h of the letterlike block.
	planck = "ℎ"
)

// speechScriptIntents are the symbols a speech engine reads as operators where they stand in a
// script (d^∗ "d super times", t^⊥ "t to the is perpendicular to-th power", A^⊤ "a to the top-th
// power", P^♯ "p to the sharp-th power"), with the word a reader says.
var speechScriptIntents = map[string]string{"∗": "star", "⋆": "star", "⊥": "perp", "⊤": "top", "♯": "sharp", "♭": "flat"}

// speechToken fixes one token: an italic h as h, a word of styled letters as a literal, a symbol in
// a script as its name, the double-struck one of a subscript as the indicator.
func speechToken(node *Node) {
	text := tokenString(node)
	if node.Name == "mi" && text == planck && len(node.Attrs) == 0 {
		node.SetText("h")
		return
	}
	if literal, ok := styledWord(node, text); ok {
		setIntent(node, literal)
		return
	}
	if word, ok := speechScriptIntents[text]; ok && inScript(node) {
		setIntent(node, word)
	}
	speechIndicator(node)
}

// styledWord: an mi of several letters of one mathematical alphabet (TeX's 𝑝𝑦, \mathsf{refl},
// \mathbf{SRW}) as the literal a speech engine reads: italic letters one by one (p-y: "p y", as
// TeX's italic juxtaposes variables), the other alphabets as the word they spell (refl, SRW, as
// decision 100 reads an upright word). MathCAT leaves such an identifier unread.
func styledWord(node *Node, text string) (string, bool) {
	if node.Name != "mi" || len(node.Attrs) != 0 || utf8.RuneCountInString(text) < 2 {
		return "", false
	}
	letters, alphabet := []string{}, ""
	for _, character := range text {
		base, variant, ok := decode(character, "", false)
		if !ok || len(base) != 1 || !asciiLetter(base[0]) || alphabet != "" && variant != alphabet {
			return "", false
		}
		letters, alphabet = append(letters, base), variant
	}
	if strings.Contains(alphabet, "italic") {
		return strings.Join(letters, "-"), true
	}
	return strings.Join(letters, ""), true
}

func asciiLetter(character byte) bool {
	return character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z'
}

// inScript: the token is a script (not the base) of a script element, alone or as the only
// element of a row there.
func inScript(node *Node) bool {
	for node.Parent != nil && node.Parent.Name == "mrow" && len(node.Parent.Elements()) == 1 {
		node = node.Parent
	}
	parent := node.Parent
	return parent != nil && slices.Contains(speechScripts, parent.Name) && slices.Index(parent.Elements(), node) > 0
}

// oneCharacter: an mi of one letter (any alphabet), or of ∇ or ∂, which speech engines join to
// a letter beside them.
func oneCharacter(node *Node) bool {
	if node == nil || node.Name != "mi" {
		return false
	}
	text := tokenString(node)
	character, size := utf8.DecodeRuneInString(text)
	return size > 0 && size == len(text) && (unicode.IsLetter(character) || character == 0x2207 || character == 0x2202)
}

// scriptedCharacter: an mi of one character, or a script element whose base is one.
func scriptedCharacter(node *Node) bool {
	if slices.Contains([]string{"msub", "msup", "msubsup"}, node.Name) && len(node.Elements()) > 0 {
		return oneCharacter(leaf(node.Elements()[0]))
	}
	return oneCharacter(node)
}

// separateIdentifiers puts U+2062 between an identifier of one character and the identifier (or
// scripted identifier) after it: the MathML for juxtaposed factors. Without it MathCAT reads a run
// of three or more such letters as one word it cannot speak (𝑓𝑑𝑡, 𝑛𝑚𝑘), and ∇ or ∂ beside a
// letter likewise. A d (italic or upright) is left joined to what follows: it is a differential as
// often as a factor, and Speech Rule Engine reads "f times d t" when U+2062 stands after it, while
// a pair of letters is never a word MathCAT garbles.
func separateIdentifiers(row *Node) {
	children := row.Elements()
	result := []*Node{}
	for index, child := range children {
		result = append(result, child)
		if index+1 < len(children) && oneCharacter(child) && !differential(child) && scriptedCharacter(children[index+1]) {
			result = append(result, newToken(row, "mo", invisibleTimes))
		}
	}
	row.setElements(result)
}

// differential: an italic or upright d.
func differential(node *Node) bool {
	text := tokenString(node)
	return text == "𝑑" || text == "d" && node.Get("mathvariant") == "normal"
}

// invisibleOperator: an mo of U+2061–U+2064, which shows nothing.
func invisibleOperator(node *Node) bool {
	if node.Name != "mo" {
		return false
	}
	character, size := utf8.DecodeRuneInString(tokenString(node))
	return size > 0 && size == len(tokenString(node)) && character >= 0x2061 && character <= 0x2064
}

// dropEmptyLabel removes the label intent from an equation-label cell with no text: an unnumbered
// row of an align still has its label cell (the row model needs it), but MathCAT reads every
// label cell "with label", also an empty one (papers4 1126).
func dropEmptyLabel(cell *Node) {
	if cell.Get("intent") == ":equation-label" && strings.TrimSpace(strings.Join(textsOf(cell), "")) == "" {
		cell.RemoveAttribute("intent")
	}
}

func textsOf(node *Node) []string {
	texts := node.Texts()
	for _, child := range node.Descendants() {
		texts = append(texts, child.Texts()...)
	}
	return texts
}
