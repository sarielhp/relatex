package mathexport

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/sarielhp/relatex/internal/authortex"
	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/mathtokens"
	"github.com/sarielhp/relatex/internal/rec"
)

// Root is the repository root, where the math accent font is read; set it before use.
var Root string

// MathFontFamily is HTML::MATH_FONT_FAMILY.
const MathFontFamily = `"Latin Modern Math", "STIX Two Math", math`

// AccentFamily is MathAccentFont::FAMILY.
const AccentFamily = "L2D Math Accents"

// AccentFile is the accent font's file name in the fonts/ folder beside the page: like the
// paper's own fonts it is linked as a file, never embedded as data (decision 058).
const AccentFile = "math-accents.otf"

var (
	accentFont     []byte
	accentFace     string
	accentFaceOnce sync.Once
	stretchedMark  = regexp.MustCompile(`<m(?:over|under|underover)\b`)
)

// loadAccentFont reads the fixed accent font, checked against its manifest, and its license.
func loadAccentFont() {
	directory := filepath.Join(Root, "lib", "relatex", "fonts", "math-accents")
	bytes, err := os.ReadFile(filepath.Join(directory, "math-accents.otf"))
	if err != nil {
		rec.Fail("%v", err)
	}
	manifestFile, err := os.Open(filepath.Join(directory, "math-accents.json"))
	if err != nil {
		rec.Fail("%v", err)
	}
	defer manifestFile.Close()
	manifest, err := canonjson.Decode(manifestFile)
	if err != nil {
		rec.Fail("%v", err)
	}
	sum := sha256.Sum256(bytes)
	if hex.EncodeToString(sum[:]) != rec.FetchString(rec.AsMap(manifest), "sha256") {
		rec.Fail("Math accent font resource hash mismatch")
	}
	license, err := os.ReadFile(filepath.Join(directory, "GUST-FONT-LICENSE.txt"))
	if err != nil {
		rec.Fail("%v", err)
	}
	accentFont = bytes
	accentFace = "/* Derived from Latin Modern Math; renamed L2D Math Accents.\n" + string(license) + "*/\n" +
		"@font-face{font-family:'" + AccentFamily + "';src:url(\"fonts/" + AccentFile +
		"\") format('opentype');font-weight:normal;font-style:normal;font-display:block;}"
}

// AccentFace is MathAccentFont::FACE: the accent font's license and its face, which links
// fonts/math-accents.otf beside the page (WriteAccentFont).
func AccentFace() string {
	accentFaceOnce.Do(loadAccentFont)
	return accentFace
}

// WriteAccentFont writes directory/fonts/math-accents.otf when the document's formulas use
// the accent font, beside the page whose stylesheet links it.
func WriteAccentFont(document rec.Map, directory string) {
	if !AccentFontUsed(document) {
		return
	}
	accentFaceOnce.Do(loadAccentFont)
	folder := filepath.Join(directory, "fonts")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		rec.Fail("%v", err)
	}
	if err := os.WriteFile(filepath.Join(folder, AccentFile), accentFont, 0o644); err != nil {
		rec.Fail("%v", err)
	}
}

// AccentFontUsed is MathAccentFont.used?: a formula stretches a mark over or under a base.
func AccentFontUsed(document rec.Map) bool {
	for _, value := range rec.FetchMap(document, "nodes") {
		node := rec.AsMap(value)
		if node["type"] == "math" && stretchedMark.MatchString(rec.FetchString(node, "mathml")) {
			return true
		}
	}
	return false
}

// AccentFontCSS is MathAccentFont.css.
func AccentFontCSS(document rec.Map) string {
	if AccentFontUsed(document) {
		return AccentFace()
	}
	return ""
}

var (
	tokens           = []string{"mi", "mn", "mo", "mtext", "ms"}
	cssUnits         = []string{"em", "ex", "px", "pt", "pc", "in", "cm", "mm"}
	signedAttributes = map[string]string{"mspace": "width", "mpadded": "lspace"}
	accentAttributes = map[string][]string{"mover": {"accent"}, "munder": {"accentunder"}, "munderover": {"accentunder", "accent"}}
	tablePadding     = map[string][2]string{"columnspacing": {"padding-inline-start", "padding-inline-end"},
		"rowspacing": {"padding-top", "padding-bottom"}}
)

// Project is BrowserMathML.project: older alphabet attributes projected to their Unicode
// characters (MathML Core only implements mathvariant="normal" on mi), on a detached copy, and
// cleaned for speech (decision 100).
func Project(root *Node) string { return ProjectAnnotated(root, "") }

// ProjectAnnotated is Project with the author's TeX, when known (""), as the formula's
// <annotation encoding="application/x-tex"> beside its presentation in <semantics>
// (decision 100). Browsers show a semantics element's first child only.
func ProjectAnnotated(root *Node, tex string) string {
	copied := root.Clone()
	applyVariant(copied, "", false)
	cleanSpeech(copied)
	if tex != "" {
		annotate(copied, tex)
	}
	return copied.String()
}

// TeXAnnotation is the encoding of the author's TeX annotation.
const TeXAnnotation = "application/x-tex"

// AuthorTeX is a math node's TeX as its author wrote it, for its annotation: only a verified
// cut (decision 064: typeset again, it gives the same MathML), as Markdown ships it (no
// comments, labels or blank lines); "" otherwise. Without --author-math every formula is
// unavailable and has none.
func AuthorTeX(node rec.Map) string {
	tex, ok := node["tex"].(rec.Map)
	if !ok || tex["status"] != "verified" {
		return ""
	}
	text, _ := tex["text"].(string)
	return authortex.Shipped(text)
}

// FormulaTeX is a math node's TeX written from its MathML, when the round trip proved it
// (decision 141): typeset again with amsmath, amssymb and bm alone, it gives the same MathML. An
// alignment comes with its environment; "" when the node has none.
func FormulaTeX(node rec.Map) string {
	field, ok := node["formula_tex"].(rec.Map)
	if !ok {
		return ""
	}
	text, _ := field["text"].(string)
	if environment, ok := field["environment"].(string); ok && environment != "" {
		return "\\begin{" + environment + "}\n" + text + "\n\\end{" + environment + "}"
	}
	return text
}

// AnnotationTeX is a math node's TeX annotation: the author's verified TeX, which wins where
// present, else the formula TeX; "" when it has neither.
func AnnotationTeX(node rec.Map) string {
	if tex := AuthorTeX(node); tex != "" {
		return tex
	}
	return FormulaTeX(node)
}

func annotate(math *Node, tex string) {
	children := math.Elements()
	semantics := &Node{Name: "semantics", Parent: math}
	presentation := &Node{Name: "mrow", Parent: semantics}
	if len(children) == 1 && children[0].Name == "mrow" {
		presentation = children[0]
	} else {
		presentation.setElements(children)
	}
	annotation := newToken(semantics, "annotation", tex)
	annotation.SetAttribute("encoding", TeXAnnotation)
	semantics.setElements([]*Node{presentation, annotation})
	math.setElements([]*Node{semantics})
}

func applyVariant(node *Node, outer string, hasOuter bool) {
	variant, has := node.Attribute("mathvariant")
	if !has {
		variant, has = outer, hasOuter
	}
	if slices.Contains(tokens, node.Name) && has {
		var builder strings.Builder
		for _, character := range strings.Join(node.Texts(), "") {
			base, _, encoded := decode(character, "", false)
			if encoded {
				builder.WriteRune(character)
				continue
			}
			if projected, ok := mathtokens.EncodeAlphabet(base, variant); ok {
				builder.WriteString(projected)
			} else {
				builder.WriteRune(character)
			}
		}
		node.SetText(builder.String())
		if node.Name == "mi" {
			node.SetAttribute("mathvariant", "normal")
		}
	}
	signedGeometry(node)
	fixedFence(node)
	if node.Name == "mtable" {
		tableAlignment(node)
	}
	accentAttribute(node)
	for _, child := range node.Elements() {
		applyVariant(child, variant, has)
	}
}

func signedGeometry(node *Node) {
	attribute, ok := signedAttributes[node.Name]
	if !ok {
		return
	}
	source, present := node.Attribute(attribute)
	if !present {
		return
	}
	length := NewLength(source, false)
	if !(length.Value < 0) {
		return
	}
	css := cssLength(length, 1)
	node.SetAttribute(attribute, "0pt")
	if node.Name == "mspace" {
		node.SetAttribute("style", "margin-inline-start:"+css)
	} else {
		node.SetAttribute("style", "position:relative;inset-inline-start:"+css)
	}
}

// cssLength is BrowserMathML.css_length.
func cssLength(length Length, scale float64) string {
	amount := length.Value * scale
	if length.Unit == "mu" {
		return FormatG(amount/18, 10) + "em"
	}
	if !slices.Contains(cssUnits, length.Unit) {
		rec.Fail("Unsupported browser mathematical length: %s", length.Unit)
	}
	return FormatG(amount, 10) + length.Unit
}

func tableAlignment(table *Node) {
	if table.Has("columnspacing") || table.Has("rowspacing") {
		table.SetAttribute("style", "border-spacing:0")
	}
	rows := table.Elements()
	columns := 0
	for _, row := range rows {
		columns = max(columns, len(row.Elements()))
	}
	for rowIndex, row := range rows {
		for column, cell := range row.Elements() {
			align := Alignment(table, row, cell, column, "columnalign")
			style := "text-align:" + align
			// Chromium's math row handles CSS 'right' as left alignment. Its legacy value
			// works; other engines retain the preceding standard CSS.
			if align == "right" {
				style += ";text-align:-webkit-right"
			}
			if cell.Has("rowalign") || row.Has("rowalign") || table.Has("rowalign") {
				vertical := Alignment(table, row, cell, column, "rowalign")
				if vertical == "center" {
					vertical = "middle"
				}
				style += ";vertical-align:" + vertical
			}
			style += paddingStyle(table, "columnspacing", column, columns)
			style += paddingStyle(table, "rowspacing", rowIndex, len(rows))
			cell.SetAttribute("style", style)
		}
	}
}

func paddingStyle(table *Node, axis string, position, count int) string {
	if !table.Has(axis) {
		return ""
	}
	indexes := [2]int{position - 1, position}
	present := [2]bool{position != 0, position != count-1}
	var builder strings.Builder
	for side, name := range tablePadding[axis] {
		var length *Length
		if present[side] {
			length = Spacing(table, axis, indexes[side])
		}
		if length != nil && length.Value < 0 {
			rec.Fail("Negative matrix %s is unsupported by browser padding", axis)
		}
		value := "0pt"
		if length != nil {
			value = cssLength(*length, 0.5)
		}
		builder.WriteString(";" + name + ":" + value)
	}
	return builder.String()
}

func accentAttribute(node *Node) {
	names, ok := accentAttributes[node.Name]
	if !ok {
		return
	}
	for index, name := range names {
		decoration := node.Element(index + 2)
		if decoration == nil || decoration.Name != "mo" {
			continue
		}
		explicit, hasExplicit := node.Attribute(name)
		text, hasText := decoration.TextValue()
		inferred := hasText && strings.Contains(accentGlyphs[name], text)
		if hasExplicit && explicit != "true" || !hasExplicit && !inferred {
			continue
		}
		node.SetAttribute(name, "true")
		if !decoration.Has("stretchy") {
			decoration.SetAttribute("stretchy", "true")
		}
		decoration.SetAttribute("style", fmt.Sprintf("font-family:'%s',%s", AccentFamily, MathFontFamily))
	}
}

func fixedFence(node *Node) {
	fence, size := FenceBox(node)
	if fence == nil {
		return
	}
	css := cssLength(size, 1)
	fence.SetAttribute("minsize", css)
	fence.SetAttribute("maxsize", css)
}

var alignments = map[string][]string{"columnalign": {"left", "center", "right"}, "rowalign": {"top", "center", "bottom", "baseline"}}

// Alignment is MathTablePresentation.alignment: a cell overrides its row, a row its table.
func Alignment(table, row, cell *Node, column int, axis string) string {
	tableIndex, rowIndex := column, column
	if axis != "columnalign" {
		tableIndex, rowIndex = slices.Index(table.Elements(), row), 0
	}
	value, ok := cell.Attribute(axis)
	if !ok {
		value, ok = item(row, axis, rowIndex)
	}
	if !ok {
		value, ok = item(table, axis, tableIndex)
	}
	if !ok {
		value = map[string]string{"columnalign": "center", "rowalign": "baseline"}[axis]
	}
	if !slices.Contains(alignments[axis], value) {
		rec.Fail("Unsupported matrix %s: %s", axis, value)
	}
	return value
}

// Spacing is MathTablePresentation.spacing: the length at an index, or nil.
func Spacing(table *Node, axis string, index int) *Length {
	value, ok := item(table, axis, index)
	if !ok {
		return nil
	}
	length := NewLength(value, false)
	return &length
}

// item is MathTablePresentation.item: the index-th of a space-separated list, the last one
// past its end.
func item(node *Node, axis string, index int) (string, bool) {
	value, ok := node.Attribute(axis)
	if !ok {
		return "", false
	}
	values := strings.Fields(value)
	if len(values) == 0 {
		rec.Fail("Empty mathematical table attribute")
	}
	if index >= 0 && index < len(values) {
		return values[index], true
	}
	return values[len(values)-1], true
}
