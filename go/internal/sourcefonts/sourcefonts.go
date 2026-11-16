package sourcefonts

import (
	"fmt"
	"math"
	"os"
	stdpath "path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/fontbinding"
	"github.com/sarielhp/relatex/internal/fontmodel"
	"github.com/sarielhp/relatex/internal/fspath"
	"github.com/sarielhp/relatex/internal/rec"
)

// Version is SourceFonts::VERSION.
const Version = "document-fonts-1"

// Root is the repository root, where profiles/font-families.json is read; the stage binary sets
// it (and fontmodel.Root, for the schema).
var Root string

var (
	loaderPrefix = regexp.MustCompile(`\A[a-z]+loaded:`)
	unsafeChars  = regexp.MustCompile(`[^A-Za-z0-9_.-]`)
	texFontName  = regexp.MustCompile(`\A[A-Za-z0-9_.-]+\z`)
)

func profile() rec.Map { return fontmodel.Load("sourcefonts", Root, "profiles", "font-families.json") }

func merge(maps ...rec.Map) rec.Map {
	result := rec.Map{}
	for _, m := range maps {
		for key, value := range m {
			result[key] = value
		}
	}
	return result
}

// Capture is SourceFonts.new(pdf, document, font_sources:).capture: every font the text uses
// named, nil without styled text. The directory receives the word-spacing evidence per font
// (font-N/binding.json, and font.json for file fonts).
func Capture(pdf string, document rec.Map, fontSources []rec.Map, working string) (fonts rec.Map, directory string) {
	styles := []rec.Map{}
	for _, span := range fontmodel.Spans(document) {
		style := rec.FetchMap(span, "style")
		if !slices.ContainsFunc(styles, func(seen rec.Map) bool { return rec.Equal(seen["font_name"], style["font_name"]) }) {
			styles = append(styles, style)
		}
	}
	slices.SortStableFunc(styles, func(a, b rec.Map) int {
		return strings.Compare(rec.FetchString(a, "font_name"), rec.FetchString(b, "font_name"))
	})
	if len(styles) == 0 {
		return nil, ""
	}
	directory = filepath.Join(command.Scratch("source-fonts-"), "evidence")
	if _, err := os.Lstat(directory); err == nil {
		rec.Fail("Source font output must be new")
	}
	mkdir(directory)
	resolver := fontbinding.New([]rec.Map{})
	tex := []string{}
	for _, style := range styles {
		name := rec.FetchString(style, "font_name")
		observed := slices.ContainsFunc(fontSources, func(row rec.Map) bool { return rec.Equal(row["font_name"], name) })
		if !observed && texFontName.MatchString(name) {
			tex = append(tex, name)
		}
	}
	resolver.PrefetchFonts(tex)
	list := make([]any, len(styles))
	for index, style := range styles {
		list[index] = describe(style, resolver, fontSources, working, filepath.Join(directory, fmt.Sprintf("font-%d", index)))
	}
	absolute, _ := filepath.Abs(pdf)
	result := rec.Map{"status": Version, "qualified": false, "pdf_sha256": command.FileDigest(absolute), "items": list}
	fontmodel.Validate(merge(document, rec.Map{"fonts": result}))
	writeJSON(filepath.Join(directory, "fonts.json"), result)
	return result, directory
}

func describe(style rec.Map, resolver *fontbinding.Binding, fontSources []rec.Map, working, evidence string) rec.Map {
	mkdir(evidence)
	name := rec.FetchString(style, "font_name")
	generic := "unknown"
	if value, ok := style["generic"]; ok {
		generic = rec.AsString(value)
	}
	if mapped, ok := rec.FetchMap(profile(), "generic")[generic]; ok {
		generic = rec.AsString(mapped)
	} else {
		generic = "serif"
	}
	observed := []rec.Map{}
	for _, row := range fontSources {
		if rec.Equal(row["font_name"], name) && row["filename"] != nil && row["filename"] != false {
			observed = append(observed, row)
		}
	}
	if len(observed) > 0 {
		return fileFont(name, generic, observed, working, evidence)
	}
	binding := recordTeXBinding(resolver, name, evidence)
	family := ""
	if value, ok := style["family"]; ok {
		family = rec.AsString(value)
	}
	item := rec.Map{"font_name": name, "origin": "tex", "families": texFamilies(family), "generic": generic}
	// The PDF's own name for the face (pdftex.map), which native PDF checks compare with.
	for _, key := range []string{"postscript_name", "postscript_names", "scales"} {
		if value, ok := binding[key]; ok {
			item[key] = value
		}
	}
	return item
}

// TeXFamilies is the CSS families profiles/font-families.json names for an NFSS family: its longest
// matching prefix, none without one.
func TeXFamilies(family string) []any { return texFamilies(family) }

func texFamilies(family string) []any {
	families := rec.FetchMap(profile(), "families")
	best := ""
	found := false
	keys := make([]string, 0, len(families))
	for key := range families {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		if strings.HasPrefix(family, key) && (!found || len(key) > len(best)) {
			best, found = key, true
		}
	}
	if !found {
		return []any{}
	}
	return rec.AsList(families[best])
}

// fileFont is a font LaTeX loaded from a file; a relative file name is LaTeX's, relative to its
// working directory, not to relatex's.
func fileFont(name, generic string, observed []rec.Map, working, evidence string) rec.Map {
	paths := []string{}
	postscripts := []string{}
	for _, row := range observed {
		path := loaderPrefix.ReplaceAllString(rec.FetchString(row, "filename"), "")
		if !slices.Contains(paths, path) {
			paths = append(paths, path)
		}
		if psname, ok := row["psname"].(string); ok && !slices.Contains(postscripts, psname) {
			postscripts = append(postscripts, psname)
		}
	}
	if len(paths) != 1 || len(postscripts) > 1 {
		rec.Fail("Conflicting observed font files for %s", name)
	}
	path := paths[0]
	if !filepath.IsAbs(path) && working != "" {
		path = filepath.Join(working, path)
	}
	path, err := fspath.Real(path)
	if err != nil {
		rec.Fail("%v", err)
	}
	path, _ = filepath.Abs(path)
	observedName := ""
	if len(postscripts) == 1 {
		observedName = postscripts[0]
	}
	font := OpenFont(path, observedName)
	postscript, ok := font.PostScriptName()
	if !ok {
		postscript = observedName
		if postscript == "" {
			postscript = name
		}
	}
	item := rec.Map{"font_name": name, "origin": "file", "families": []any{font.Family()}, "generic": generic,
		"postscript_name": postscript}
	recordFileBinding(font, name, observed, evidence)
	if !ship(font) {
		return item
	}
	sha := font.SHA256()
	item["css_family"] = "ltd-" + sha[:16]
	item["file"] = linkedFontFile(path, sha, font.Format, font.Size(), fspath.Slash)
	return item
}

func ship(font *Font) bool {
	limits := rec.FetchMap(profile(), "ship")
	maximum, _ := rec.Number(rec.Fetch(limits, "maximum_bytes"))
	return font.Shippable() && float64(font.Size()) <= maximum && (truthy(rec.Fetch(limits, "collections")) || !font.Collection())
}

func truthy(value any) bool { return value != nil && value != false }

// recordTeXBinding: TeX fonts keep their TFM binding, which the word-boundary check reads
// letter kerns and glue from. A font without a resolvable TFM has no word-spacing evidence.
func recordTeXBinding(resolver *fontbinding.Binding, name, evidence string) (binding rec.Map) {
	defer unbound(name, evidence, &binding)
	binding = resolver.Resolve(name, evidence)
	writeJSON(filepath.Join(evidence, "binding.json"), binding)
	return binding
}

// unbound records a Relatex::Failure of the binding as unbound.json and no binding.
func unbound(name, evidence string, binding *rec.Map) {
	recovered := recover()
	if recovered == nil {
		return
	}
	failure, ok := recovered.(rec.Failure)
	if !ok || failure.Lookup {
		panic(recovered)
	}
	writeJSON(filepath.Join(evidence, "unbound.json"), rec.Map{"font_name": name, "reason": failure.Message})
	*binding = nil
}

func recordFileBinding(font *Font, name string, observed []rec.Map, evidence string) {
	sha := font.SHA256()
	observations := []any{}
	for _, row := range observed {
		observation := rec.Map{}
		for _, key := range []string{"font", "font_name", "format", "psname", "size_sp", "parameters"} {
			if value, ok := row[key]; ok {
				observation[key] = value
			}
		}
		observations = append(observations, observation)
	}
	var postscript any
	if value, ok := font.PostScriptName(); ok {
		postscript = value
	}
	binding := fileFontBinding(name, postscript, font.Path, sha, observations, fspath.Slash)
	writeJSON(filepath.Join(evidence, "binding.json"), binding)
	if spacing := wordSpacing(font, observations); spacing != nil {
		writeJSON(filepath.Join(evidence, "font.json"), rec.Map{"font_name": name, "spacing": spacing})
	}
}

func linkedFontFile(path, sha, format string, size int64, slash func(string) string) rec.Map {
	slashed := slash(path)
	return rec.Map{"name": sha[:16] + "-" + unsafeChars.ReplaceAllString(stdpath.Base(slashed), "_"), "sha256": sha,
		"format": format, "bytes": size, "source": slashed}
}

func fileFontBinding(name string, postscript any, path, sha string, observations []any, slash func(string) string) rec.Map {
	slashed := slash(path)
	return rec.Map{"format": "opentype", "font_name": name, "postscript_name": postscript, "full_source_path": slashed,
		"full_source_sha256": sha, "observations": observations, "dependencies": rec.Map{slashed: sha}}
}

// toF is Ruby's to_f on a JSON value.
func toF(value any) float64 {
	number, _ := rec.Number(value)
	return number
}

// wordSpacing is the narrowest observed interword glue against the widest positive letter
// kerning, both in ems; nil when the two cannot be told apart.
func wordSpacing(font *Font, observations []any) rec.Map {
	bounds := []float64{}
	for _, value := range observations {
		row := rec.AsMap(value)
		size := toF(row["size_sp"])
		parameters, _ := row["parameters"].(rec.Map)
		if !(size > 0) || !(toF(parameters["quad"]) > 0) {
			continue
		}
		bounds = append(bounds, (toF(parameters["space"])-toF(parameters["space_shrink"]))/size)
	}
	if len(bounds) == 0 || slices.Max(bounds)-slices.Min(bounds) > 0.00001 {
		return nil
	}
	maximum := float64(font.PositivePositioningBound()) / float64(font.UnitsPerEm())
	minimum := slices.Min(bounds)
	if !(maximum < minimum) || math.IsNaN(minimum) {
		return nil
	}
	return rec.Map{"maximum_letter_kern_em": maximum, "minimum_word_space_em": minimum,
		"source": "opentype-positioning-and-observed-glue-parameters"}
}

func mkdir(path string) {
	if err := os.MkdirAll(path, 0o755); err != nil {
		rec.Fail("%v", err)
	}
}

// writeJSON is Relatex.write_json for evidence files: indented JSON with Ruby's numbers (a
// Float stays one). Keys are sorted, where Ruby keeps insertion order; readers parse it.
func writeJSON(path string, value any) {
	if err := canonjson.WriteFile(path, value); err != nil {
		rec.Fail("%v", err)
	}
}
