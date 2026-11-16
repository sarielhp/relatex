package frontmatter

import (
	"reflect"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/extlinks"
	"github.com/sarielhp/relatex/internal/omap"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/stylemodel"
)

// Validate is PaperFrontmatter.validate!.
func Validate(document rec.Map) {
	if rows, ok := document["page_decorations"]; ok {
		validateDecorations(rows, document)
	}
	value := document["frontmatter"]
	if !truthy(value) {
		return
	}
	data, isMap := value.(rec.Map)
	_, bound := data["abstract_source"]
	extra := []string{}
	if isMap && bound {
		extra = []string{"abstract_blocks", "abstract_source"}
	}
	// A paper without an abstract types its title and authors alone.
	expected := append(slices.Clone(fields), extra...)
	if _, hasAbstract := data["abstract"]; isMap && !hasAbstract && len(extra) == 0 {
		expected = slices.DeleteFunc(expected, func(name string) bool { return name == "abstract" })
	}
	if !isMap || !validMetadata(data, document, expected) {
		rec.Fail("Invalid source-bound paper metadata")
	}
	validateAuthors(rec.Fetch(data, "authors"))
	if len(extra) > 0 {
		validateAbstract(data, document)
	}
	if !reflect.DeepEqual(rec.Fetch(data, "language"), languageAssertion()) {
		rec.Fail("Unsupported metadata language assertion")
	}
	validateDecorations(rec.Fetch(data, "decorations"), document)
	validateLinks(rec.Fetch(data, "links"), document)
}

func validMetadata(data, document rec.Map, expected []string) bool {
	if !rec.SameKeys(data, expected) || data["status"] != "source_bound" {
		return false
	}
	required := []string{"title"}
	if slices.Contains(expected, "abstract") {
		required = append(required, "abstract")
	}
	for _, key := range required {
		if text, ok := data[key].(string); !ok || text == "" {
			return false
		}
	}
	source, ok := data["source"].(rec.Map)
	if !ok || !rec.SameKeys(source, []string{"path", "sha256"}) ||
		source["sha256"] != rec.Fetch(rec.FetchMap(document, "identity"), "input_sha256") {
		return false
	}
	path, ok := source["path"].(string)
	return ok && sourcePath.MatchString(path) && !slices.Contains(rubySplit(path, "/"), "..")
}

// rubySplit is String#split(separator): trailing empty fields dropped.
func rubySplit(text, separator string) []string {
	parts := strings.Split(text, separator)
	for len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}

func validateAbstract(data, document rec.Map) {
	order := rec.FetchList(document, "block_order")
	blocks, isList := rec.Fetch(data, "abstract_blocks").([]any)
	found := isList && len(blocks) > 0 && slices.ContainsFunc(order, func(id any) bool { return rec.Equal(id, blocks[0]) })
	source, isString := data["abstract_source"].(string)
	valid := isString && source != "" && found && len(blocks) <= abstractBlocks
	if valid {
		valid = slices.ContainsFunc(abstractRuns(order, document), func(indices []int) bool {
			return rec.Equal(valuesAt(order, indices), blocks)
		})
	}
	if !valid {
		rec.Fail("Invalid structured AMS abstract association")
	}
	parser := NewTexMetadata("")
	prose := make([]string, len(blocks))
	for index, id := range blocks {
		prose[index] = blockProse(document, id)
	}
	if !abstractPattern(parser.AbstractProse(source)).MatchString(compactProse(normalizeAbstract(strings.Join(prose, " ")))) ||
		rec.Fetch(data, "abstract") != abstractSummary(document, blocks) {
		rec.Fail("Structured AMS abstract differs from source or retained summary")
	}
}

func validateAuthors(value any) {
	authors, ok := value.([]any)
	if !ok || len(authors) == 0 {
		rec.Fail("Missing structured authors")
	}
	for _, item := range authors {
		author, isMap := item.(rec.Map)
		name, isString := author["name"].(string)
		if !isMap || !rec.SameKeys(author, []string{"affiliations", "emails", "name"}) || !isString || name == "" ||
			!authorFieldsValid(author) {
			rec.Fail("Invalid structured paper author")
		}
		for _, address := range rec.FetchList(author, "emails") {
			if !email.MatchString(address.(string)) {
				rec.Fail("Invalid metadata email")
			}
		}
	}
}

func authorFieldsValid(author rec.Map) bool {
	for _, field := range []string{"affiliations", "emails"} {
		values, ok := author[field].([]any)
		if !ok {
			return false
		}
		for _, value := range values {
			if text, isString := value.(string); !isString || text == "" {
				return false
			}
		}
	}
	return true
}

func validateDecorations(value any, document rec.Map) {
	decorations, ok := value.([]any)
	if !ok {
		rec.Fail("Invalid page-decoration inventory")
	}
	ids := []any{}
	for _, item := range decorations {
		row, isMap := item.(rec.Map)
		names := []string{"candidate", "glyphs", "id", "page", "source", "text"}
		if _, runs := row["runs"]; isMap && runs {
			names = append(names, "runs")
		}
		text, isString := row["text"].(string)
		valid := isMap && rec.SameKeys(row, names) && nodeID.MatchString(toS(row["id"])) &&
			slices.Contains(decorationSources, row["source"]) && isString && text != ""
		for _, key := range []string{"candidate", "glyphs", "page"} {
			valid = valid && rec.Positive(row[key])
		}
		if !valid {
			rec.Fail("Invalid retained page decoration")
		}
		ids = append(ids, rec.Fetch(row, "id"))
		key := []any{row["page"], row["source"], row["candidate"]}
		if rec.Fetch(row, "id") != identifier(rec.FetchMap(document, "identity"), "page-decoration", key) {
			rec.Fail("Page-decoration identity binding differs")
		}
		if _, runs := row["runs"]; runs {
			validateDecorationRuns(row)
		}
	}
	unique := len(uniqueAny(ids)) == len(ids)
	for _, id := range ids {
		if _, collides := nodes(document)[toS(id)]; collides {
			unique = false
		}
	}
	if !unique {
		rec.Fail("Colliding page-decoration anchors")
	}
}

// toS is to_s on a JSON value that is a string or not one.
func toS(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	if value == nil {
		return ""
	}
	return inspect(value)
}

func uniqueAny(values []any) []any {
	result := []any{}
	for _, value := range values {
		if !slices.ContainsFunc(result, func(seen any) bool { return reflect.DeepEqual(seen, value) }) {
			result = append(result, value)
		}
	}
	return result
}

func validateDecorationRuns(row rec.Map) {
	runs, ok := rec.Fetch(row, "runs").([]any)
	if !ok || len(runs) == 0 {
		rec.Fail("Missing styled decoration text")
	}
	var builder strings.Builder
	for _, item := range runs {
		run, isMap := item.(rec.Map)
		text, isString := run["text"].(string)
		keys := []string{"style", "text"}
		if isMathRun(run) {
			keys = []string{"math", "text"} // a math glyph has no text face; it says so
		}
		if !isMap || !rec.SameKeys(run, keys) || !isString || text == "" {
			rec.Fail("Invalid styled decoration run")
		}
		if !isMathRun(run) {
			stylemodel.Validate(rec.Fetch(run, "style"))
		}
		builder.WriteString(text)
	}
	if builder.String() != rec.Fetch(row, "text") {
		rec.Fail("Styled decoration text differs from candidate")
	}
}

// textLinkKeys: a text link names its block, the table cell holding it if any, its span
// and start.
func textLinkKeys(link rec.Map) []string {
	if _, ok := link["cell"]; ok {
		return []string{"block", "cell", "span", "start"}
	}
	return []string{"block", "span", "start"}
}

func validateLinks(value any, document rec.Map) {
	links, ok := value.([]any)
	if !ok {
		rec.Fail("Invalid external-link inventory")
	}
	seen := []any{}
	ids := []any{}
	for _, item := range links {
		link, isMap := item.(rec.Map)
		extra := textLinkKeys(link)
		if _, math := link["math_node"]; isMap && math {
			extra = []string{"math_node"}
		}
		text, isString := link["text"].(string)
		if !isMap || !rec.SameKeys(link, append([]string{"href", "id", "text"}, extra...)) || !nodeID.MatchString(toS(link["id"])) ||
			!isString || text == "" {
			rec.Fail("Invalid retained external link")
		}
		extlinks.SafeHref(rec.Fetch(link, "href"))
		keys := append([]string{"href"}, textLinkKeys(link)...)
		if _, math := link["math_node"]; math {
			keys = []string{"href", "math_node"}
		}
		values := make([]any, len(keys))
		for index, key := range keys {
			values[index] = link[key]
		}
		if rec.Fetch(link, "id") != identifier(rec.FetchMap(document, "identity"), "external-link", values) {
			rec.Fail("External-link identity binding differs")
		}
		validateLinkPayload(link, document)
		seen = append(seen, values[1:])
		ids = append(ids, rec.Fetch(link, "id"))
	}
	if len(uniqueAny(seen)) != len(seen) {
		rec.Fail("Ambiguous external-link ownership")
	}
	if len(uniqueAny(ids)) != len(links) {
		rec.Fail("Duplicate external-link identity")
	}
	validateTextLinkRanges(links)
}

func validateLinkPayload(link, document rec.Map) {
	if id, math := link["math_node"]; math {
		item := node(document, id)
		if item["type"] == "math" && extlinks.MathText(rec.FetchString(item, "mathml")) == link["text"] {
			return
		}
		rec.Fail("External link payload differs from observed math text")
	}
	order := rec.FetchList(document, "block_order")
	valid := slices.ContainsFunc(order, func(id any) bool { return rec.Equal(id, link["block"]) })
	for _, key := range []string{"span", "start"} {
		number, isInt := link[key].(int64)
		valid = valid && isInt && number >= 0
	}
	if !valid {
		rec.Fail("Invalid external text link position")
	}
	span := extlinks.Resolve(document, link)
	_, cited := span["citation_link"]
	label := []rune(rec.FetchString(link, "text"))
	if span["type"] != "text" || cited || !substringAt([]rune(rec.FetchString(span, "text")), int(rec.FetchInt(link, "start")), label) {
		rec.Fail("External link payload differs from observed text span")
	}
}

// substringAt is text[start, label.size] == label.
func substringAt(text []rune, start int, label []rune) bool {
	if start > len(text) {
		return false
	}
	end := min(start+len(label), len(text))
	return equalRunes(text[start:end], label)
}

func validateTextLinkRanges(links []any) {
	groups := omap.New[string, []rec.Map]()
	for _, item := range links {
		link := rec.AsMap(item)
		if _, math := link["math_node"]; math {
			continue
		}
		key := inspect([]any{link["block"], link["cell"], link["span"]})
		rows, _ := groups.Get(key)
		groups.Set(key, append(rows, link))
	}
	groups.Each(func(_ string, rows []rec.Map) {
		sorted := slices.Clone(rows)
		slices.SortStableFunc(sorted, func(a, b rec.Map) int { return int(rec.FetchInt(a, "start") - rec.FetchInt(b, "start")) })
		ending := int64(0)
		for _, row := range sorted {
			if rec.FetchInt(row, "start") < ending {
				rec.Fail("Overlapping external text links")
			}
			ending = rec.FetchInt(row, "start") + int64(len([]rune(rec.FetchString(row, "text"))))
		}
	})
}

// isMathRun is a decoration run marked as math: it has no style and must say so.
func isMathRun(run rec.Map) bool {
	marked, _ := run["math"].(bool)
	return marked
}
