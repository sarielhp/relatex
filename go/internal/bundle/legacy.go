package bundle

import (
	"slices"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/sourcemap"
)

// The source maps before decision 066 (source-map-1.0 to 1.2), bound to a layout's words.

var legacyVersions = []any{"source-map-1.0", "source-map-1.1", "source-map-1.2"}

func layoutWords(layout rec.Map) []rec.Map {
	result := []rec.Map{}
	EachLayoutPages(layout, func(_ int, pages []any) {
		for _, page := range pages {
			for _, word := range rec.FetchList(rec.AsMap(page), "words") {
				result = append(result, rec.AsMap(word))
			}
		}
	})
	return result
}

func validateMap(m rec.Map, layoutValue any) {
	layout, _ := layoutValue.(rec.Map)
	if layout == nil || !rec.Equal(rec.Fetch(m, "pdf_sha256"), rec.Fetch(layout, "pdf_sha256")) {
		rec.Fail("Source map has no bound layout")
	}
	if !slices.Contains(legacyVersions, rec.Fetch(m, "schema_version")) {
		rec.Fail("Unsupported source map schema")
	}
	if !rec.Equal(rec.Fetch(m, "coordinates"), rec.Fetch(layout, "coordinates")) {
		rec.Fail("Source map coordinates differ")
	}
	sourcemap.ValidateDigest(rec.Fetch(m, "synctex_sha256"))
	files := rec.FetchMap(m, "source_files")
	for _, name := range sortedKeys(files) {
		if sourcemap.UnsafeName(name) {
			rec.Fail("Source map path escapes snapshot")
		}
		sourcemap.ValidateDigest(files[name])
	}
	original := layoutWords(layout)
	mapped := rec.FetchList(m, "words")
	sameIDs := len(mapped) == len(original)
	for index := 0; sameIDs && index < len(mapped); index++ {
		sameIDs = rec.Equal(rec.Fetch(rec.AsMap(mapped[index]), "id"), rec.Fetch(original[index], "id"))
	}
	if !sameIDs {
		rec.Fail("Source map word coverage differs from layout")
	}
	for index, value := range mapped {
		word := rec.AsMap(value)
		kept := rec.Map{}
		for key := range original[index] {
			if field, ok := word[key]; ok {
				kept[key] = field
			}
		}
		if !rec.Equal(kept, original[index]) {
			rec.Fail("Source map changed native word geometry")
		}
		for _, origin := range rec.FetchList(word, "origins") {
			validateSourceOrigin(rec.AsMap(origin), m)
		}
	}
	if rec.Fetch(m, "schema_version") == "source-map-1.0" {
		return
	}
	validateCatalog(m)
	if rec.Fetch(m, "schema_version") == "source-map-1.2" {
		validateRefinements(m)
	}
}

func validateCatalog(m rec.Map) {
	if !rec.Equal(rec.Fetch(m, "index"), sourcemap.Index(m)) {
		rec.Fail("Source map lookup index differs")
	}
	sourcemap.ValidateCatalog(rec.FetchMap(m, "sources"), rec.FetchMap(m, "source_files"),
		"Source catalog text differs from immutable input")
}

func validateSourceOrigin(origin, m rec.Map) {
	files := rec.FetchMap(m, "source_files")
	file := rec.FetchString(origin, "file")
	known, ok := files[file]
	if !ok {
		panic(rec.Failure{Message: "key not found: " + rec.FetchString(origin, "file"), Lookup: true})
	}
	precision := rec.Fetch(origin, "precision")
	line, isInt := rec.Fetch(origin, "line").(int64)
	if !rec.Equal(known, rec.Fetch(origin, "source_sha256")) || (precision != "line" && precision != "inferred_span") || !isInt || line <= 0 {
		rec.Fail("Source map origin identity/precision differs")
	}
	if precision != "inferred_span" {
		return
	}
	characters := rec.FetchList(origin, "characters")
	var start, finish any
	if len(characters) > 0 {
		start = characters[0]
	}
	if len(characters) > 1 {
		finish = characters[1]
	}
	first, firstInt := start.(int64)
	last, lastInt := finish.(int64)
	if !firstInt || !lastInt || first < 0 || last <= first || rec.Fetch(origin, "span_units") != "Unicode scalar offsets, zero-based half-open" {
		rec.Fail("Invalid inferred source span")
	}
}

func validateRefinements(m rec.Map) {
	sourcemap.ValidateGroups(m)
	indexes := map[string]*sourcemap.TextIndex{}
	all := append(append([]any{}, rec.FetchList(m, "words")...), rec.FetchList(m, "logical_words")...)
	for _, value := range all {
		word := rec.AsMap(value)
		for _, item := range rec.FetchList(word, "origins") {
			origin := rec.AsMap(item)
			validateSourceOrigin(origin, m)
			validateNativeHint(origin)
			if origin["precision"] != "inferred_span" {
				continue
			}
			validateInferredSpan(word, origin, m, indexes)
		}
	}
}

func validateInferredSpan(word, origin, m rec.Map, indexes map[string]*sourcemap.TextIndex) {
	file := rec.FetchString(origin, "file")
	record, ok := rec.FetchMap(m, "sources")[file].(rec.Map)
	if !ok {
		panic(rec.Failure{Message: "key not found: " + file, Lookup: true})
	}
	text, has := record["text"].(string)
	if !has {
		rec.Fail("Inferred source span in a source without catalogued text")
	}
	index, ok := indexes[file]
	if !ok {
		index = sourcemap.NewTextIndex(text)
		indexes[file] = index
	}
	hint := rec.FetchMap(origin, "native_hint")
	expected := index.Locate(rec.FetchString(word, "normalized_text"), int(rec.FetchInt(hint, "line")))
	actual := rec.Map{}
	for _, key := range []string{"line", "characters", "byte_offsets"} {
		if value, present := origin[key]; present {
			actual[key] = value
		}
	}
	if expected == nil || !rec.Equal(rec.Map(expected), actual) {
		rec.Fail("Inferred source span differs from its bound literal/native hint")
	}
}

func validateNativeHint(origin rec.Map) {
	hint, ok := rec.Fetch(origin, "native_hint").(rec.Map)
	line, lineInt := hint["line"].(int64)
	_, columnInt := hint["column"].(int64)
	if !ok || !rec.SameKeys(hint, []string{"column", "line"}) || !lineInt || line <= 0 || !columnInt {
		rec.Fail("Invalid retained native source hint")
	}
	if column, has := origin["native_column"]; has &&
		(!rec.Equal(column, hint["column"]) || !rec.Equal(rec.Fetch(origin, "line"), hint["line"])) {
		rec.Fail("Inferred source position was relabelled as a native column")
	}
}
