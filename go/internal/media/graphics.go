package media

import (
	"fmt"
	"regexp"
	"slices"

	"github.com/sarielhp/relatex/internal/geometry"
	"github.com/sarielhp/relatex/internal/mediamodel"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/stream"
)

const pixelLimit = 268_435_456 // PNGImage::PIXEL_LIMIT

var (
	sha256Pattern = regexp.MustCompile(`\A[0-9a-f]{64}\z`)
)

// Graphics is GraphicEvidence: retained image slots bound to the native resources TeX
// loaded, PNG-only (graphics-0.1) or any media with its placement (graphics-0.2).
type Graphics struct {
	modern  *mediaGraphics
	sources map[any]rec.Map
	uses    map[any]rec.Map
	indices []any
	count   int
}

var legacyFields = map[string][]string{
	"graphic_source": {"seq", "kind", "code", "source", "filename", "sha256", "bytes", "pixel_width", "pixel_height",
		"resource", "index", "natural_width", "natural_height", "natural_depth"},
	"graphic_use": {"seq", "kind", "code", "source", "use", "resource", "width", "height"},
}

// NewGraphics is GraphicEvidence.new(records).
func NewGraphics(records []rec.Map) *Graphics {
	if stream.Enabled(records[0], "graphics-0.2") {
		return &Graphics{modern: newMediaGraphics(records)}
	}
	g := &Graphics{sources: map[any]rec.Map{}, uses: map[any]rec.Map{}}
	rows := []rec.Map{}
	for _, record := range records {
		if _, known := legacyFields[asString(record["code"])]; known && record["kind"] == "diagnostic" {
			rows = append(rows, record)
		}
	}
	if len(rows) > 0 && !stream.Enabled(records[0], "graphics-0.1") {
		rec.Fail("Graphic evidence requires its declared extension")
	}
	for _, row := range rows {
		if !rec.SameKeys(row, legacyFields[rec.FetchString(row, "code")]) || row["source"] != "graphics-def-luatex-png" {
			rec.Fail("Malformed graphic provenance")
		}
		if row["code"] == "graphic_source" {
			g.source(row)
		} else {
			g.use(row)
		}
	}
	return g
}

// Span is GraphicEvidence#span: the image span for a retained slot inside owner.
func (g *Graphics) Span(item, owner rec.Map) (span rec.Map) {
	if g.modern != nil {
		return g.modern.span(item, owner)
	}
	defer incomplete()
	if _, ok := item["form"]; ok {
		rec.Fail("Form image requires graphics-0.2")
	}
	use, ok := g.uses[key(rec.Fetch(item, "use"))]
	if !ok {
		rec.Fail("Dangling graphic use")
	}
	source := fetchKey(g.sources, rec.Fetch(use, "resource"))
	shape := values(item, "width", "height", "depth")
	if !(less(rec.Fetch(use, "seq"), rec.Fetch(owner, "seq")) && rec.Equal(rec.Fetch(item, "resource"), rec.Fetch(source, "resource")) &&
		rec.Equal(rec.Fetch(item, "index"), rec.Fetch(source, "index")) &&
		rec.Equal(shape, values(source, "natural_width", "natural_height", "natural_depth"))) {
		rec.Fail("Retained image differs from its native resource")
	}
	if _, ok := item["reference"]; ok {
		rec.Fail("Image cannot replace a reference token")
	}
	g.count++
	return rec.Map{"type": "image", "id": fmt.Sprintf("g%d", g.count), "graphic_use": rec.Fetch(item, "use"),
		"filename": rec.Fetch(source, "filename"), "sha256": rec.Fetch(source, "sha256"), "bytes": rec.Fetch(source, "bytes"),
		"pixel_width": rec.Fetch(source, "pixel_width"), "pixel_height": rec.Fetch(source, "pixel_height"),
		"width_sp": rec.Fetch(use, "width"), "height_sp": rec.Fetch(use, "height")}
}

// incomplete is the legacy span's rescue of KeyError: a missing field is incomplete evidence.
func incomplete() {
	recovered := recover()
	if recovered == nil {
		return
	}
	if failure, ok := recovered.(rec.Failure); ok && failure.Lookup {
		panic(rec.Failure{Message: "Incomplete graphic evidence: " + failure.Message})
	}
	panic(recovered)
}

func (g *Graphics) source(row rec.Map) {
	for _, name := range []string{"resource", "index", "bytes", "pixel_width", "pixel_height", "natural_width", "natural_height"} {
		if !rec.Positive(row[name]) {
			rec.Fail("Invalid PNG source geometry or digest")
		}
	}
	bytes := rec.AsInt(row["bytes"])
	sha, isString := row["sha256"].(string)
	if !zero(row["natural_depth"]) || bytes < 33 || bytes > 67_108_864 ||
		rec.AsInt(row["pixel_width"])*rec.AsInt(row["pixel_height"]) > pixelLimit || !isString || !sha256Pattern.MatchString(sha) {
		rec.Fail("Invalid PNG source geometry or digest")
	}
	if !mediamodel.SafeSourcePath(rec.Fetch(row, "filename")) {
		rec.Fail("Unsafe graphic source path")
	}
	if _, seen := g.sources[key(row["resource"])]; seen || slices.ContainsFunc(g.indices, func(index any) bool { return rec.Equal(index, row["index"]) }) {
		rec.Fail("Duplicate native graphic resource")
	}
	g.sources[key(row["resource"])] = row
	g.indices = append(g.indices, row["index"])
}

func (g *Graphics) use(row rec.Map) {
	source, bound := g.sources[key(rec.Fetch(row, "resource"))]
	valid := true
	for _, name := range []string{"use", "width", "height"} {
		number, ok := row[name].(int64)
		valid = valid && ok && number >= 1 && number <= 1_073_741_823
	}
	_, seen := g.uses[key(row["use"])]
	if !bound || !less(rec.Fetch(source, "seq"), rec.Fetch(row, "seq")) || !valid || seen {
		rec.Fail("Invalid or premature graphic source use")
	}
	g.uses[key(row["use"])] = row
}

// mediaGraphics is GraphicEvidenceV2: versioned media evidence with the final placement.
type mediaGraphics struct {
	sources    map[any]rec.Map
	uses       map[any]rec.Map
	placements map[any]rec.Map
	count      int
}

var mediaFields = map[string][]string{
	"graphic_source": {"seq", "kind", "code", "source", "filename", "sha256", "bytes", "mime_type", "resource", "index",
		"natural_width", "natural_height", "natural_depth", "page", "pagebox"},
	"graphic_use":       {"seq", "kind", "code", "source", "use", "resource", "width", "height", "clip", "viewport"},
	"graphic_placement": {"seq", "kind", "code", "source", "use", "transforms"},
}

func newMediaGraphics(records []rec.Map) *mediaGraphics {
	g := &mediaGraphics{sources: map[any]rec.Map{}, uses: map[any]rec.Map{}, placements: map[any]rec.Map{}}
	for _, row := range records {
		allowed, known := mediaFields[asString(row["code"])]
		if row["kind"] != "diagnostic" || !known {
			continue
		}
		if _, form := row["form"]; form && row["code"] == "graphic_placement" {
			allowed = append(slices.Clone(allowed), "form")
		}
		if !rec.SameKeys(row, allowed) || row["source"] != "graphics-def-luatex-media" {
			rec.Fail("Malformed media provenance")
		}
		switch row["code"] {
		case "graphic_source":
			g.source(row)
		case "graphic_use":
			g.use(row)
		case "graphic_placement":
			g.placement(row)
		}
	}
	if len(g.uses) != len(g.placements) {
		rec.Fail("Missing final media placement evidence")
	}
	for use := range g.uses {
		if _, ok := g.placements[use]; !ok {
			rec.Fail("Missing final media placement evidence")
		}
	}
	return g
}

func (g *mediaGraphics) span(item, owner rec.Map) rec.Map {
	use, ok := g.uses[key(rec.Fetch(item, "use"))]
	if !ok {
		rec.Fail("Dangling media use")
	}
	source := fetchKey(g.sources, rec.Fetch(use, "resource"))
	placement := fetchKey(g.placements, rec.Fetch(item, "use"))
	form := truthy(item["form"])
	native := source
	keys := []string{"natural_width", "natural_height", "natural_depth"}
	if form {
		native = rec.FetchMap(placement, "form")
		keys = []string{"width", "height", "depth"}
	}
	if _, clipped := placement["form"]; clipped && !form {
		rec.Fail("Clipped media must retain its native form identity")
	}
	if !(less(rec.Fetch(use, "seq"), rec.Fetch(owner, "seq")) && rec.Equal(rec.Fetch(item, "resource"), rec.Fetch(source, "resource")) &&
		rec.Equal(rec.Fetch(native, "index"), rec.Fetch(item, "index")) &&
		rec.Equal(values(native, keys...), lookups(item, "width", "height", "depth"))) {
		rec.Fail("Retained media differs from its bound native resource")
	}
	if _, ok := item["reference"]; ok {
		rec.Fail("Media cannot replace a reference")
	}
	data := Placement(source, use, rec.FetchList(placement, "transforms"))
	g.count++
	span := rec.Map{"type": "image", "id": fmt.Sprintf("g%d", g.count), "graphic_use": rec.Fetch(item, "use"), "placement": data}
	for _, name := range []string{"filename", "sha256", "bytes", "mime_type"} {
		if value, ok := source[name]; ok {
			span[name] = value
		}
	}
	for _, name := range []string{"width_sp", "height_sp", "depth_sp"} {
		span[name] = data[name]
	}
	return span
}

func (g *mediaGraphics) source(row rec.Map) {
	if !mediamodel.SafeSourcePath(rec.Fetch(row, "filename")) {
		rec.Fail("Unsafe media source path")
	}
	valid := true
	for _, name := range []string{"resource", "index", "bytes", "natural_width", "natural_height", "page"} {
		valid = valid && rec.Positive(row[name])
	}
	if valid {
		sha, isString := rec.Fetch(row, "sha256").(string)
		_, known := mediamodel.Extensions[asString(rec.Fetch(row, "mime_type"))]
		valid = zero(rec.Fetch(row, "natural_depth")) && rec.AsInt(row["bytes"]) <= 67_108_864 && isString &&
			sha256Pattern.MatchString(sha) && known
	}
	if !valid {
		rec.Fail("Invalid media source geometry or identity")
	}
	if _, seen := g.sources[key(row["resource"])]; seen {
		rec.Fail("Duplicate native media resource")
	}
	g.sources[key(row["resource"])] = row
}

func (g *mediaGraphics) use(row rec.Map) {
	source, bound := g.sources[key(rec.Fetch(row, "resource"))]
	if !bound {
		rec.Fail("Unbound media use")
	}
	_, seen := g.uses[key(rec.Fetch(row, "use"))]
	if !less(rec.Fetch(source, "seq"), rec.Fetch(row, "seq")) || !rec.Positive(row["use"]) ||
		!rec.Positive(row["width"]) || !rec.Positive(row["height"]) || seen {
		rec.Fail("Invalid or premature media use")
	}
	geometry.Box(rec.AsList(rec.Fetch(row, "viewport")))
	g.uses[key(row["use"])] = row
}

func (g *mediaGraphics) placement(row rec.Map) {
	use, bound := g.uses[key(rec.Fetch(row, "use"))]
	if !bound {
		rec.Fail("Unbound final media placement")
	}
	// lualibs encodes an empty Lua sequence as an empty JSON object. Normalize only that
	// exact representation at this versioned evidence boundary.
	if transforms, ok := rec.Fetch(row, "transforms").(rec.Map); ok && len(transforms) == 0 {
		merged := rec.Map{}
		for name, value := range row {
			merged[name] = value
		}
		merged["transforms"] = []any{}
		row = merged
	}
	transforms, isList := row["transforms"].([]any)
	_, seen := g.placements[key(row["use"])]
	if !less(rec.Fetch(use, "seq"), rec.Fetch(row, "seq")) || !isList || len(transforms) > 32 || seen {
		rec.Fail("Invalid final media placement")
	}
	if value, ok := row["form"]; ok {
		form := rec.AsMap(value)
		if !rec.SameKeys(form, []string{"depth", "height", "index", "width"}) || !zero(rec.Fetch(form, "depth")) ||
			!rec.Positive(form["index"]) || !rec.Positive(form["width"]) || !rec.Positive(form["height"]) {
			rec.Fail("Invalid clipped media form identity")
		}
	}
	g.placements[key(row["use"])] = row
}

// key makes a JSON number a map key that compares as Ruby's Hash#eql? does on Integers.
func key(value any) any { return value }

// fetchKey is Hash#fetch on a map keyed by JSON values.
func fetchKey(m map[any]rec.Map, name any) rec.Map {
	value, ok := m[key(name)]
	if !ok {
		panic(rec.Failure{Message: fmt.Sprintf("key not found: %v", name), Lookup: true})
	}
	return value
}

func values(m rec.Map, names ...string) []any {
	result := make([]any, len(names))
	for index, name := range names {
		result[index] = rec.Fetch(m, name)
	}
	return result
}

// lookups is Hash#values_at: a missing key is nil.
func lookups(m rec.Map, names ...string) []any {
	result := make([]any, len(names))
	for index, name := range names {
		result[index] = m[name]
	}
	return result
}

func less(left, right any) bool {
	a, ok := rec.Number(left)
	b, isNumber := rec.Number(right)
	if !ok || !isNumber {
		rec.Fail("comparison of %T with %T failed", left, right)
	}
	return a < b
}

func zero(value any) bool {
	number, ok := rec.Number(value)
	if !ok {
		rec.Fail("undefined method 'zero?' for %T", value)
	}
	return number == 0
}

func truthy(value any) bool { return value != nil && value != false }

func asString(value any) string {
	text, _ := value.(string)
	return text
}
