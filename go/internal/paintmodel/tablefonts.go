package paintmodel

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/sarielhp/relatex/internal/fontmodel"
	"github.com/sarielhp/relatex/internal/omap"
	"github.com/sarielhp/relatex/internal/rec"
)

// TableFontsVersion is the portable evidence of PDFTableFonts, connecting native slots to PDF
// text. Several PDF subsets can share a PostScript name while assigning different meanings to
// a byte.
const TableFontsVersion = "pdf-table-fonts-1"

var (
	glyphNamePattern = regexp.MustCompile(`\A[A-Za-z0-9_.-]+\z`)
	faceFields       = []string{"reference", "postscript_name", "cmap", "encoding"}
	bindingFields    = []string{"native_font", "pdf_font", "source", "faces", "slots"}
)

// Requirement is one native font of the table glyphs: its name and the eight-bit slots used.
type Requirement struct {
	Font  string
	Slots []int64
}

// FontRequirements is PDFTableFonts.requirements: per normalized native font, its name and
// the sorted slots the glyphs use.
func FontRequirements(glyphs []rec.Map) *omap.Map[string, Requirement] {
	groups := omap.New[any, []int64]()
	for _, glyph := range glyphs {
		slot, ok := glyph["slot"]
		if !ok {
			continue
		}
		font := rec.Fetch(glyph, "font")
		slots, _ := groups.Get(font)
		number, isInt := slot.(int64)
		if !isInt {
			number = -1
		}
		groups.Set(font, append(slots, number))
	}
	result := omap.New[string, Requirement]()
	groups.Each(func(font any, slots []int64) {
		name, ok := font.(string)
		if !ok || name == "" {
			rec.Fail("Invalid native font name")
		}
		unique := []int64{}
		for _, slot := range slots {
			if slot < 0 || slot > 255 {
				rec.Fail("Invalid native font slots")
			}
			if !slices.Contains(unique, slot) {
				unique = append(unique, slot)
			}
		}
		slices.Sort(unique)
		result.Set(FontName(name), Requirement{Font: name, Slots: unique})
	})
	if result.Len() != groups.Len() {
		rec.Fail("Conflicting normalized native font names")
	}
	return result
}

type namedSlot struct {
	code string
	name string
}

// encodingVector is PDFTableFonts.encoding_vector: a font's /Differences as byte to glyph.
func encodingVector(value any) []namedSlot {
	encoding, ok := value.(rec.Map)
	differences, isList := encoding["/Differences"].([]any)
	for key := range encoding {
		ok = ok && (key == "/Type" || key == "/Differences")
	}
	if !ok || !isList {
		rec.Fail("Named slot binding requires explicit PDF encoding differences")
	}
	vector := []namedSlot{}
	index := int64(-1)
	for _, entry := range differences {
		if code, isInt := entry.(int64); isInt {
			if code < 0 || code > 255 {
				rec.Fail("Invalid PDF encoding byte")
			}
			index = code
			continue
		}
		name, isString := entry.(string)
		if index < 0 || index > 255 || !isString || !strings.HasPrefix(name, "/") ||
			!glyphNamePattern.MatchString(strings.TrimPrefix(name, "/")) {
			rec.Fail("Invalid PDF encoding glyph")
		}
		key := strconv.FormatInt(index, 10)
		for _, slot := range vector {
			if slot.code == key {
				rec.Fail("Repeated PDF encoding byte")
			}
		}
		vector = append(vector, namedSlot{code: key, name: strings.TrimPrefix(name, "/")})
		index++
	}
	return vector
}

// slotValues is PDFTableFonts.slot_values: each slot's text, the same in every face.
func slotValues(binding rec.Map) (result rec.Map) {
	defer rescueArgumentOnly("Malformed PDF font binding: ")
	type face struct {
		unicode rec.Map
		names   []namedSlot
	}
	source, hasSource := binding["source"].(rec.Map)
	all := []face{}
	for _, item := range rec.FetchList(binding, "faces") {
		entry := rec.AsMap(item)
		current := face{unicode: parseCMap(decodeBase64(rec.FetchString(entry, "cmap")))}
		if truthy(binding["source"]) {
			current.names = encodingVector(rec.Fetch(entry, "encoding"))
		}
		all = append(all, current)
	}
	result = rec.Map{}
	for _, item := range rec.FetchList(binding, "slots") {
		slot := rec.AsString(item)
		values := []any{}
		for _, current := range all {
			for _, value := range slotCandidates(slot, source, hasSource && source != nil, current.unicode, current.names) {
				if !slices.Contains(values, value) {
					values = append(values, value)
				}
			}
		}
		if len(values) != 1 {
			rec.Fail("Missing or ambiguous PDF Unicode for %s:%s", rec.Fetch(binding, "native_font"), slot)
		}
		result[slot] = values[0]
	}
	return result
}

func slotCandidates(slot string, source rec.Map, hasSource bool, unicode rec.Map, names []namedSlot) []any {
	if !hasSource {
		if value, ok := unicode[slot]; ok {
			return []any{value}
		}
		return nil
	}
	physical := rec.Fetch(rec.FetchMap(source, "slot_map"), slot)
	glyph := rec.Fetch(rec.FetchMap(source, "source_encoding"), ToS(physical))
	if glyph == ".notdef" {
		rec.Fail("Unmapped source font glyph name")
	}
	result := []any{}
	for _, entry := range names {
		if entry.name == glyph {
			result = append(result, unicode[entry.code])
		}
	}
	return result
}

// ToS is to_s on a slot, id or version: an Integer in decimal, a string itself, nil empty.
func ToS(value any) string {
	switch typed := value.(type) {
	case int64:
		return strconv.FormatInt(typed, 10)
	case string:
		return typed
	case nil:
		return ""
	}
	return rec.AsString(value)
}

func validateDigest(value any) {
	text, ok := value.(string)
	if !ok || !digest.MatchString(text) {
		rec.Fail("Invalid evidence digest")
	}
}

// ValidateFonts is PDFTableFonts.validate!.
func ValidateFonts(value any, glyphs []rec.Map) {
	proof, ok := value.(rec.Map)
	if !ok || !rec.SameKeys(proof, []string{"bindings", "pdf_sha256", "schema_version"}) || proof["schema_version"] != TableFontsVersion {
		rec.Fail("Unsupported table font proof")
	}
	validateDigest(proof["pdf_sha256"])
	required := FontRequirements(glyphs)
	found, isMap := proof["bindings"].(rec.Map)
	if !isMap || !rec.SameKeys(found, required.Keys()) {
		rec.Fail("Table font binding coverage differs")
	}
	for name, item := range found {
		entry, _ := required.Get(name)
		binding, isBinding := item.(rec.Map)
		slots := make([]any, len(entry.Slots))
		for index, slot := range entry.Slots {
			slots[index] = strconv.FormatInt(slot, 10)
		}
		if !isBinding || !rec.SameKeys(binding, bindingFields) || binding["native_font"] != entry.Font ||
			!rec.Equal(binding["slots"], slots) {
			rec.Fail("Native slot binding identity differs")
		}
		ValidateFontBinding(binding)
	}
}

// ValidateFontBinding is PDFTableFonts.validate_binding!: one native font bound to its PDF
// faces, source font and slot texts.
func ValidateFontBinding(binding rec.Map) {
	font, ok := rec.Fetch(binding, "pdf_font").(string)
	if !ok || FontName(font) != font || font == "" {
		rec.Fail("Invalid PDF font face identity")
	}
	list, isList := rec.Fetch(binding, "faces").([]any)
	if !isList || len(list) == 0 {
		rec.Fail("Missing PDF font faces")
	}
	references := []any{}
	for _, face := range list {
		validateFace(face, font)
		if !slices.Contains(references, rec.AsMap(face)["reference"]) {
			references = append(references, rec.AsMap(face)["reference"])
		}
	}
	if len(references) != len(list) {
		rec.Fail("Duplicate PDF font face")
	}
	if truthy(rec.Fetch(binding, "source")) {
		validateSource(binding)
	}
	if !truthy(binding["source"]) && FontName(rec.FetchString(binding, "native_font")) != font {
		rec.Fail("Native/PDF alias has no source binding")
	}
	validateMaps(rec.Map{FontName(rec.FetchString(binding, "native_font")): slotValues(binding)})
}

func validateFace(value any, font string) {
	face, ok := value.(rec.Map)
	reference, isReference := face["reference"].(string)
	name, isName := face["postscript_name"].(string)
	_, isCMap := face["cmap"].(string)
	if !ok || !rec.SameKeys(face, faceFields) || !isReference || !isName || !isCMap || !ObjectRef.MatchString(reference) ||
		FontName(name) != font {
		rec.Fail("PDF font face binding differs")
	}
}

func validateSource(binding rec.Map) {
	source, ok := rec.Fetch(binding, "source").(rec.Map)
	if !ok {
		rec.Fail("Invalid source font binding")
	}
	if rec.Fetch(source, "font_name") != rec.Fetch(binding, "native_font") ||
		FontName(rec.FetchString(source, "postscript_name")) != rec.Fetch(binding, "pdf_font") {
		rec.Fail("Source/PDF font names differ")
	}
	for _, key := range []string{"tfm_sha256", "full_source_sha256"} {
		validateDigest(rec.Fetch(source, key))
	}
	dependencies, isMap := rec.Fetch(source, "dependencies").(rec.Map)
	if !isMap {
		rec.Fail("Invalid source font dependencies")
	}
	for _, sha := range dependencies {
		validateDigest(sha)
	}
	if !rec.Equal(rec.Fetch(dependencies, rec.FetchString(source, "tfm_path")), source["tfm_sha256"]) ||
		!rec.Equal(rec.Fetch(dependencies, rec.FetchString(source, "full_source_path")), source["full_source_sha256"]) {
		rec.Fail("Source font dependency binding differs")
	}
	validateVector(rec.Fetch(source, "source_encoding"))
	expected := rec.Map{}
	slots := []string{}
	for _, item := range rec.FetchList(binding, "slots") {
		slot := rec.AsString(item)
		number, err := strconv.ParseInt(slot, 10, 64)
		if err != nil {
			rec.RaiseArgument("invalid value for Integer(): %q", slot)
		}
		expected[slot] = number
		slots = append(slots, slot)
	}
	packets, isPackets := rec.Fetch(source, "virtual_packets").(rec.Map)
	if !isPackets {
		rec.Fail("Invalid virtual font packet inventory")
	}
	if len(packets) > 0 {
		expected = fontmodel.PacketSlots(packets)
	}
	if rec.Equal(rec.Fetch(source, "slot_map"), expected) && rec.SameKeys(expected, slots) {
		return
	}
	rec.Fail("Native virtual slot mapping differs")
}

func validateVector(value any) {
	vector, ok := value.(rec.Map)
	valid := ok && len(vector) == 256
	for code := 0; valid && code < 256; code++ {
		name, isString := vector[strconv.Itoa(code)].(string)
		valid = isString && glyphNamePattern.MatchString(name)
	}
	if !valid {
		rec.Fail("Invalid source font encoding vector")
	}
}

// FontEncodings is PDFTableFonts.encodings.
func FontEncodings(proof rec.Map) rec.Map {
	result := rec.Map{}
	for name, binding := range rec.FetchMap(proof, "bindings") {
		result[name] = slotValues(rec.AsMap(binding))
	}
	return result
}

// FontNames is PDFTableFonts.font_names.
func FontNames(proof rec.Map) rec.Map {
	result := rec.Map{}
	for name, binding := range rec.FetchMap(proof, "bindings") {
		result[name] = rec.Fetch(rec.AsMap(binding), "pdf_font")
	}
	return result
}

var subsetPrefix = regexp.MustCompile(`\A[A-Z]{6}\+`)

var nameNoise = regexp.MustCompile(`[\s_-]`)

// FontName is PDFPaintInventory.font_name: a font name without its subset prefix,
// separators or case.
func FontName(name string) string {
	return strings.ToLower(nameNoise.ReplaceAllString(subsetPrefix.ReplaceAllString(name, ""), ""))
}
