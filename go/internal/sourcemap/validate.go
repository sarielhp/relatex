package sourcemap

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/sarielhp/relatex/internal/geometry"
	"github.com/sarielhp/relatex/internal/mathsource"
	"github.com/sarielhp/relatex/internal/mathtokens"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/sourcetext"
)

// Schema is GlyphSourceMap::SCHEMA.
const Schema = "source-map-2.0"

// Precisions is GlyphSourceMap::PRECISIONS.
var Precisions = []any{"exact", "macro", "math", "line", "none"}

// Coordinates is GlyphMap::COORDINATES.
var Coordinates = rec.Map{"unit": "bp", "origin": "top-left", "source": "TeX shipout glyph map (decision 066)"}

var hexDigest = regexp.MustCompile(`\A[0-9a-f]{64}\z`)

// typesetHyphen is /[-‐­]\z/: a hyphen TeX added at a line break.
var typesetHyphen = regexp.MustCompile(`[-\x{2010}\x{00ad}]\z`)

// ValidateDigest is BundleValidation.validate_digest!.
func ValidateDigest(value any) {
	text, ok := value.(string)
	if !ok || !hexDigest.MatchString(text) {
		rec.Fail("Invalid evidence digest")
	}
}

// sourceText is a source file read for placements: its text, its formula spans, its visible
// characters.
type sourceText struct {
	text    string
	spans   map[[2]int]bool
	visible *sourcetext.Visible
	// bodies are the ranges of the math inside the formulas (and rows of displays), the places
	// a token-level math origin may lie; tokens caches their reading.
	bodies [][2]int
	runs   []rune
	tokens map[[2]int][]mathtokens.Token
}

// Validate is SourceMapValidation.validate!: the map's sources, words, placements and groups,
// every placement re-derived from the source text.
func Validate(m rec.Map) {
	defer invalid()
	if rec.Fetch(m, "schema_version") != Schema {
		rec.Fail("Unsupported source map schema")
	}
	if !rec.Equal(rec.Fetch(m, "coordinates"), Coordinates) {
		rec.Fail("Source map coordinates differ")
	}
	for _, key := range []string{"pdf_sha256", "synctex_sha256", "glyph_map_sha256"} {
		ValidateDigest(rec.Fetch(m, key))
	}
	validateSources(m)
	texts := map[string]*sourceText{}
	ids := map[any]bool{}
	for _, word := range words(m) {
		id := rec.Fetch(word, "id")
		if ids[id] {
			rec.Fail("Duplicate source map word")
		}
		ids[id] = true
		validateWord(word, m)
		for _, origin := range rec.FetchList(word, "origins") {
			validatePlacement(word, rec.AsMap(origin), m, texts)
		}
		validateLetters(word, m, texts)
	}
	validateLogical(m, texts)
	if !rec.Equal(rec.Fetch(m, "index"), Index(m)) {
		rec.Fail("Source map lookup index differs")
	}
}

// invalid is the rescue of KeyError: a missing field is an invalid map.
func invalid() {
	recovered := recover()
	if recovered == nil {
		return
	}
	if failure, ok := recovered.(rec.Failure); ok && failure.Lookup {
		panic(rec.Failure{Message: "Invalid source map 2: " + failure.Message})
	}
	panic(recovered)
}

// UnsafeName is a source path that escapes the snapshot (name.split('/') keeps no trailing
// empty part, as Ruby's split does not).
func UnsafeName(name string) bool {
	if strings.HasPrefix(name, "/") {
		return true
	}
	parts := strings.Split(name, "/")
	for len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return true
		}
	}
	return false
}

func validateSources(m rec.Map) {
	files := rec.FetchMap(m, "source_files")
	for _, name := range sortedKeys(files) {
		if UnsafeName(name) {
			rec.Fail("Source map path escapes snapshot")
		}
		ValidateDigest(files[name])
	}
	ValidateCatalog(rec.FetchMap(m, "sources"), files, "Source catalog text differs from input")
}

// ValidateCatalog checks the source catalog against the source files' digests; textDiffers is
// the failure for a text whose digest is not its file's.
func ValidateCatalog(sources, files rec.Map, textDiffers string) {
	for _, name := range sortedKeys(sources) {
		record := rec.AsMap(sources[name])
		if !rec.Equal(rec.Fetch(record, "sha256"), rec.Fetch(files, name)) {
			rec.Fail("Source catalog identity differs")
		}
		text, ok := record["text"]
		if !ok {
			continue
		}
		sum := sha256.Sum256([]byte(rec.AsString(text)))
		if hex.EncodeToString(sum[:]) != record["sha256"] {
			rec.Fail("%s", textDiffers)
		}
	}
}

func sortedKeys(m rec.Map) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func positive(value any) bool {
	number, ok := rec.Number(value)
	if !ok {
		rec.Fail("undefined method 'positive?' for %T", value)
	}
	return number > 0
}

func validateWord(word, m rec.Map) {
	_, isText := rec.Fetch(word, "text").(string)
	page, isInt := rec.Fetch(word, "page").(int64)
	if !isInt || page <= 0 || !isText {
		rec.Fail("Invalid source map word")
	}
	for _, box := range rec.FetchList(word, "boxes") {
		geometry.Numbers(rec.AsList(box), 4)
	}
	geometry.Numbers(rec.FetchList(word, "baseline"), 2)
	if rec.Fetch(word, "normalized_text") != norm.NFKC.String(rec.FetchString(word, "text")) {
		rec.Fail("Normalized word text differs")
	}
	precision := rec.Fetch(word, "source_precision")
	if !slices.Contains(Precisions, precision) {
		rec.Fail("Unknown source precision")
	}
	origins := rec.FetchList(word, "origins")
	expected := 1
	if precision == "none" {
		expected = 0
	}
	if len(origins) != expected {
		rec.Fail("Source precision and origins differ")
	}
	if len(origins) == 0 {
		return
	}
	origin := rec.AsMap(origins[0])
	line := rec.Fetch(origin, "line")
	end := endLine(origin)
	if rec.Fetch(origin, "precision") != precision ||
		!rec.Equal(rec.FetchMap(m, "source_files")[rec.FetchString(origin, "file")], rec.Fetch(origin, "source_sha256")) ||
		!positive(line) || !atLeast(end, line) {
		rec.Fail("Invalid source map origin")
	}
	if _, characters := origin["characters"]; precision == "line" && characters {
		rec.Fail("Line precision claims characters")
	}
}

// validateLetters: the glyphs of a math word each claim the tokens that spell them, and their
// glyph texts make up the word.
func validateLetters(word, m rec.Map, texts map[string]*sourceText) {
	letters, ok := word["letters"].([]any)
	if !ok {
		return
	}
	var spelled strings.Builder
	for _, value := range letters {
		letter := rec.AsMap(value)
		spelled.WriteString(rec.FetchString(letter, "text"))
		for _, box := range rec.FetchList(letter, "boxes") {
			geometry.Numbers(rec.AsList(box), 4)
		}
		for _, origin := range rec.FetchList(letter, "origins") {
			validatePlacement(letter, rec.AsMap(origin), m, texts)
		}
	}
	if spelled.String() != rec.FetchString(word, "text") {
		rec.Fail("Letters differ from their word")
	}
}

func endLine(origin rec.Map) any {
	if end, ok := origin["end_line"]; ok {
		return end
	}
	return rec.Fetch(origin, "line")
}

func atLeast(left, right any) bool {
	a, ok := rec.Number(left)
	b, okRight := rec.Number(right)
	if !ok || !okRight {
		rec.Fail("comparison of %T with %T failed", left, right)
	}
	return a >= b
}

func (s *sourceText) honest(word rec.Map, precision any, start, stop int) bool {
	switch precision {
	case "exact":
		return exact(word, s.visible, start, stop)
	case "macro":
		return s.visible.At(start) == `\`
	case "math":
		return s.spans[[2]int{start, stop}] || s.spellsTokens(word, start, stop)
	}
	return false
}

// spellsTokens: a math origin inside a formula is honest when it starts and ends on token
// boundaries and the tokens it covers spell the word, as the glyphs of the word do.
func (s *sourceText) spellsTokens(word rec.Map, start, stop int) bool {
	best := [2]int{-1, -1}
	for _, body := range s.bodies {
		if body[0] <= start && stop <= body[1] && (best[0] < 0 || body[1]-body[0] < best[1]-best[0]) {
			best = body
		}
	}
	if best[0] < 0 {
		return false
	}
	tokens, cached := s.tokens[best]
	if !cached {
		tokens = mathtokens.Tokenize(s.runs, best[0], best[1])
		s.tokens[best] = tokens
	}
	return mathtokens.Spells(tokens, start, stop, rec.FetchString(word, "text"))
}
func loadSource(m rec.Map, file string, texts map[string]*sourceText) *sourceText {
	if entry, ok := texts[file]; ok {
		return entry
	}
	record, ok := rec.FetchMap(m, "sources")[file].(rec.Map)
	if !ok {
		panic(rec.Failure{Message: "key not found: " + quote(file), Lookup: true})
	}
	text, has := record["text"].(string)
	if !has || record["text"] == nil {
		rec.Fail("Placed word in a source without text")
	}
	spans := mathsource.NewSourceText(text, nil).Spans
	entry := &sourceText{text: text, spans: map[[2]int]bool{}, runs: []rune(text), tokens: map[[2]int][]mathtokens.Token{}}
	masks := []sourcetext.Span{}
	for _, span := range spans {
		entry.spans[[2]int{span.Start, span.Stop}] = true
		rows := mathsource.RowSpans(text, span)
		for _, row := range rows {
			entry.spans[row] = true
		}
		entry.bodies = append(entry.bodies, rows...)
		if first, last, ok := mathsource.BodyRange(text, span); ok {
			entry.bodies = append(entry.bodies, [2]int{first, last})
		}
		masks = append(masks, sourcetext.Span{Start: span.Start, Stop: span.Stop})
	}
	entry.visible = sourcetext.New(text, masks)
	texts[file] = entry
	return entry
}

func quote(text string) string { return `"` + text + `"` }

func validatePlacement(word, origin, m rec.Map, texts map[string]*sourceText) {
	precision := rec.Fetch(origin, "precision")
	if precision == "line" {
		return
	}
	entry := loadSource(m, rec.FetchString(origin, "file"), texts)
	characters := rec.FetchList(origin, "characters")
	first, last := int(rec.AsInt(rec.Index(characters, 0))), int(rec.AsInt(rec.Index(characters, 1)))
	line, end := int(rec.FetchInt(origin, "line")), int(rec.AsInt(endLine(origin)))
	lineStart, ok := entry.visible.LineStart(line)
	endStart, endOK := entry.visible.LineStart(end)
	if !ok || !endOK {
		panic(rec.Failure{Message: "index outside of array bounds", Lookup: true})
	}
	start, stop := lineStart+first, endStart+last
	if start < 0 || stop <= start || stop > entry.visible.Length() {
		rec.Fail("Invalid source span")
	}
	firstBytes, lastBytes, _ := entry.visible.ByteOffsets(line, end, first, last)
	if !rec.Equal(origin["byte_offsets"], []any{int64(firstBytes), int64(lastBytes)}) {
		rec.Fail("Source span byte offsets differ")
	}
	if !entry.honest(word, precision, start, stop) {
		rec.Fail("Source span does not support %v precision: %v", precision, rec.Fetch(word, "id"))
	}
}

// exact: the visible source characters inside the span, read the way the aligner reads them,
// equal the word (a hyphen TeX added at a line break is not in the source). The span starts
// and ends on a visible character: no surrounding space or markup is claimed.
func exact(word rec.Map, visible *sourcetext.Visible, start, stop int) bool {
	units := visible.UnitsWithin(start, stop)
	if len(units) == 0 || units[0].Start != start || units[len(units)-1].Stop != stop {
		return false
	}
	var source strings.Builder
	for _, unit := range units {
		source.WriteString(unit.Char)
	}
	typeset := rec.FetchString(word, "text")
	if _, continues := word["continues"]; continues {
		typeset = typesetHyphen.ReplaceAllString(typeset, "")
	}
	return dropBlanks(sourcetext.Fold(source.String())) == dropBlanks(sourcetext.Fold(typeset))
}

func dropBlanks(text string) string { return strings.NewReplacer(" ", "", "\t", "").Replace(text) }

// validateLogical: hyphenated words, adjacent fragments linked by continues, recorded as
// SourceWordGroups does, with one exact origin over the whole source word.
func validateLogical(m rec.Map, texts map[string]*sourceText) {
	all := words(m)
	positions := map[any]int{}
	for index, word := range all {
		positions[rec.Fetch(word, "id")] = index
	}
	for _, value := range rec.FetchList(m, "logical_words") {
		group := rec.AsMap(value)
		chain := []rec.Map{}
		indexes := []int{}
		for _, id := range rec.FetchList(group, "word_ids") {
			position, ok := positions[id]
			if !ok {
				panic(rec.Failure{Message: "key not found: " + quote(rec.AsString(id)), Lookup: true})
			}
			indexes = append(indexes, position)
			chain = append(chain, all[position])
		}
		valid := len(chain) >= 2
		for index := 0; index+1 < len(chain); index++ {
			valid = valid && rec.Equal(chain[index]["continues"], rec.Fetch(chain[index+1], "id")) && indexes[index+1] == indexes[index]+1
		}
		origins := rec.FetchList(group, "origins")
		valid = valid && rec.Equal(group, Record(chain, rec.Fetch(group, "origins"))) && len(origins) == 1 &&
			rec.AsMap(origins[0])["precision"] == "exact"
		if !valid {
			rec.Fail("Invalid hyphenated source word")
		}
		validatePlacement(group, rec.AsMap(origins[0]), m, texts)
	}
}
