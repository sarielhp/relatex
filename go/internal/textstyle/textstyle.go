// Package textstyle is the Go port of Relatex::TextStyle: the observed NFSS declarations of
// the text fonts. It does not certify exact font resources or color.
package textstyle

import (
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/stream"
	"github.com/sarielhp/relatex/internal/stylemodel"
)

// Catalog maps a font id to its observed style.
type Catalog map[int64]rec.Map

// NewCatalog is TextStyle.catalog(records).
func NewCatalog(records []rec.Map) Catalog {
	rows := []rec.Map{}
	for _, row := range records {
		if row["kind"] == "diagnostic" && row["code"] == "external_text_font" {
			rows = append(rows, row)
		}
	}
	if len(rows) > 0 && !stream.Enabled(records[0], "text-style-0.1") {
		rec.Fail("Font declarations without text-style extension")
	}
	result := Catalog{}
	declarations := map[any]int64{}
	for _, row := range rows {
		style := stylemodel.Validate(rec.Fetch(row, "style"))
		font, isInt := row["font"].(int64)
		_, seen := result[font]
		if !rec.SameKeys(row, []string{"seq", "kind", "code", "source", "font", "style"}) || row["source"] != "nfss-selectfont" ||
			!isInt || font != style["font_id"].(int64) || seen {
			rec.Fail("Invalid or duplicate text font declaration")
		}
		result[font] = style
	}
	for _, row := range rows {
		declarations[rec.Fetch(row, "font")] = rec.FetchInt(row, "seq")
	}
	validateSubstitutions(records, declarations)
	validateUsage(records, declarations)
	return result
}

// validateSubstitutions: NFSS substitution reuses one loaded font for later requests
// (TU/ptm/b/n served by TU/lmr/m/n): the glyphs keep the loading declaration, so a later
// differing selection of an already declared font is accepted. Undeclared or unexplained
// conflicts still fail.
func validateSubstitutions(records []rec.Map, declarations map[any]int64) {
	for _, row := range records {
		if row["kind"] != "diagnostic" || row["code"] != "conflicting_text_font_declaration" {
			continue
		}
		declared, ok := declarations[row["font"]]
		seq := int64(0)
		if value, present := row["seq"]; present {
			seq = rec.AsInt(value)
		}
		fields, isMap := row["fields"].(rec.Map)
		explained := ok && seq > declared && isMap && len(fields) > 0
		for key := range fields {
			explained = explained && slices.Contains(stylemodel.Fields, key)
		}
		if !explained {
			rec.Fail("Conflicting NFSS declarations for one font ID")
		}
	}
}

func validateUsage(records []rec.Map, declarations map[any]int64) {
	for _, owner := range records {
		if owner["kind"] != "flow_candidate" {
			continue
		}
		for _, value := range rec.FetchList(owner, "items") {
			item := rec.AsMap(value)
			if _, ok := item["style"]; !ok {
				continue
			}
			sequence, ok := declarations[rec.Fetch(item, "style")]
			if !ok || sequence >= rec.FetchInt(owner, "seq") {
				rec.Fail("Undeclared or premature text font use")
			}
		}
	}
}

// Has is whether the catalog declares font (the front matter's decorations ask it).
func (c Catalog) Has(font int64) bool {
	_, ok := c[font]
	return ok
}

// Runs is TextRuns(items, c, false): the front matter's styled decoration text.
func (c Catalog) Runs(items []any) []rec.Map { return TextRuns(items, c, false) }

// Style is catalog.fetch(id) { raise Failure, message }, for an item's style id.
func (c Catalog) Style(id any, message string) rec.Map {
	font, ok := id.(int64)
	style, found := c[font]
	if !ok || !found {
		rec.Fail("%s", message)
	}
	return style
}

// ItemStyle is item['style'] && catalog.fetch(item.fetch('style')) { raise Failure, message }:
// nil when the item has no style.
func (c Catalog) ItemStyle(item rec.Map, message string) rec.Map {
	id := item["style"]
	if id == nil || id == false {
		return nil
	}
	return c.Style(id, message)
}

// UnobservedStyle supplies the face properties of a direct TeX font when its TFM face name is
// known (an Adobe PostScript or Computer Modern face) or its suffix matches a family already
// observed through NFSS. Its encoding stays explicitly unknown.
func UnobservedStyle(row rec.Map, catalog Catalog) rec.Map {
	font, fontOK := row["font"].(int64)
	name, nameOK := row["font_name"].(string)
	size, sizeOK := row["size_sp"].(int64)
	if !fontOK || !nameOK || !sizeOK || font < 1 || size < 1 {
		return nil
	}
	base, family := familyStyle(name, catalog)
	var series, shape string
	var generic any
	if face, ok := postscriptFace(name); ok {
		family, series, shape, generic = face[0], face[1], face[2], face[3]
	} else if base != nil {
		var ok bool
		if series, shape, ok = faceStyle(name[len(family):], base); !ok {
			return nil
		}
		generic = rec.Fetch(base, "generic")
	} else {
		face, ok := computerModernFace(name)
		if !ok {
			return nil
		}
		family, series, shape, generic = face[0], face[1], face[2], face[3]
	}
	return stylemodel.Validate(rec.Map{"source": "unobserved-tex-font-id", "font_id": font, "font_name": name, "size_sp": size,
		"encoding": "unknown", "family": family, "series": series, "shape": shape, "generic": generic})
}

// computerModernFaces are the faces of Knuth's Computer Modern text fonts, named outside the
// NFSS family-series-shape scheme (cmti10 is cmr/m/it, as ot1cmr.fd declares): family, series,
// shape and generic family.
var computerModernFaces = map[string][4]string{
	"cmr": {"cmr", "m", "n", "serif"}, "cmti": {"cmr", "m", "it", "serif"}, "cmsl": {"cmr", "m", "sl", "serif"},
	"cmcsc": {"cmr", "m", "sc", "serif"}, "cmbx": {"cmr", "bx", "n", "serif"}, "cmbxti": {"cmr", "bx", "it", "serif"},
	"cmbxsl": {"cmr", "bx", "sl", "serif"}, "cmb": {"cmr", "b", "n", "serif"},
	"cmss": {"cmss", "m", "n", "sans-serif"}, "cmssi": {"cmss", "m", "sl", "sans-serif"},
	"cmssbx": {"cmss", "bx", "n", "sans-serif"}, "cmtt": {"cmtt", "m", "n", "monospace"},
	"cmitt": {"cmtt", "m", "it", "monospace"}, "cmsltt": {"cmtt", "m", "sl", "monospace"},
	"cmtcsc": {"cmtt", "m", "sc", "monospace"},
}

// computerModernFace reads a Computer Modern font name: a face name and its design size (cmti10).
func computerModernFace(name string) ([4]string, bool) {
	face := strings.TrimRight(name, "0123456789")
	if face == name {
		return [4]string{}, false
	}
	value, ok := computerModernFaces[face]
	return value, ok
}

// postscriptFamilies are the Adobe PostScript text families under their psnfss names, with the
// generic family each stands for.
var postscriptFamilies = map[string]string{
	"ptm": "serif", "ppl": "serif", "phv": "sans-serif", "pcr": "monospace",
}

// postscriptFaces are the face suffixes of those families: family-series-shape, as psnfss
// declares them. A direct font keeps the family name, so ptmb is ptm/b/n and ptmbc (the caps
// font of \scshape, Times-BoldSC) is ptm/b/sc.
var postscriptFaces = map[string][2]string{
	"": {"m", "n"}, "r": {"m", "n"}, "b": {"b", "n"}, "rc": {"m", "sc"}, "bc": {"b", "sc"},
	"ri": {"m", "it"}, "bi": {"b", "it"}, "ro": {"m", "sl"}, "o": {"m", "sl"}, "bo": {"b", "sl"},
}

// postscriptFace reads a PostScript family's TFM face name (ptmb, phvbc8t): family, series,
// shape and generic family.
func postscriptFace(name string) ([4]string, bool) {
	if len(name) < 4 {
		return [4]string{}, false
	}
	family := name[:3]
	generic, ok := postscriptFamilies[family]
	if !ok {
		return [4]string{}, false
	}
	suffix := name[3:]
	if index := strings.IndexAny(suffix, "0123456789"); index >= 0 {
		suffix = suffix[:index]
	}
	face, ok := postscriptFaces[suffix]
	if !ok {
		return [4]string{}, false
	}
	return [4]string{family, face[0], face[1], generic}, true
}

func familyStyle(name string, catalog Catalog) (rec.Map, string) {
	var selected rec.Map
	family := ""
	for _, style := range catalog {
		candidate := rec.FetchString(style, "family")
		if len(candidate) > len(family) && strings.HasPrefix(name, candidate) {
			selected, family = style, candidate
		}
	}
	return selected, family
}

func faceStyle(suffix string, base rec.Map) (string, string, bool) {
	series := ""
	for _, candidate := range []string{"eb", "ub", "bx", "sb", "el", "ul", "b", "m", "l"} {
		if strings.HasPrefix(suffix, candidate) {
			series, suffix = candidate, strings.TrimPrefix(suffix, candidate)
			break
		}
	}
	if series == "" {
		return "", "", false
	}
	shape := rec.FetchString(base, "shape")
	if suffix != "" {
		if !slices.Contains([]string{"n", "it", "sl", "sc", "scit", "scsl"}, suffix) {
			return "", "", false
		}
		shape = suffix
	}
	return series, shape, true
}

// TextRuns is TextStyle.text_runs: consecutive items of one style joined into text runs.
func TextRuns(items []any, catalog Catalog, trim bool) []rec.Map {
	runs := []rec.Map{}
	for _, value := range items {
		item := rec.AsMap(value)
		style := catalog.ItemStyle(item, "Undeclared cell text font")
		text := rec.FetchString(item, "space") + rec.FetchString(item, "text")
		if len(runs) > 0 {
			last := runs[len(runs)-1]
			if previous, _ := last["style"].(rec.Map); sameStyle(previous, style) {
				last["text"] = last["text"].(string) + text
				continue
			}
		}
		run := rec.Map{"type": "text", "text": text}
		if style != nil {
			run["style"] = style
		}
		runs = append(runs, run)
	}
	if trim {
		runs = trimRuns(runs)
	}
	return runs
}

func sameStyle(left, right rec.Map) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return rec.Equal(left, right)
}

func trimRuns(runs []rec.Map) []rec.Map {
	for len(runs) > 0 && rec.Strip(runs[0]["text"].(string)) == "" {
		runs = runs[1:]
	}
	for len(runs) > 0 && rec.Strip(runs[len(runs)-1]["text"].(string)) == "" {
		runs = runs[:len(runs)-1]
	}
	if len(runs) == 0 {
		return runs
	}
	runs[0]["text"] = rec.LStrip(runs[0]["text"].(string))
	runs[len(runs)-1]["text"] = rec.RStrip(runs[len(runs)-1]["text"].(string))
	return runs
}
