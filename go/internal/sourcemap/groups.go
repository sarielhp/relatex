// Package sourcemap is the Go port of the source map's shared parts: SourceWordGroups (the
// inferred view that links a source-confirmed word across line and page breaks), SourceMap's
// lookup index, and SourceMapValidation (decision 066), which checks a source map 2 on its own
// terms.
package sourcemap

import (
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/rec"
)

// GroupVersion is SourceWordGroups::VERSION.
const GroupVersion = "source-word-group-1"

// maxFragments is SourceWordGroups::MAX_FRAGMENTS.
const maxFragments = 32

// hyphen is SourceWordGroups::HYPHEN: /[-­]\z/.
var hyphen = regexp.MustCompile(`[-\x{00ad}]\z`)

func digest(value any) string {
	text, err := canonjson.Digest(value)
	if err != nil {
		rec.Fail("%v", err)
	}
	return text
}

// JoinedText is SourceWordGroups.joined_text: the fragments' texts, each but the last without
// its line-end hyphen.
func JoinedText(chain []rec.Map) string {
	var builder strings.Builder
	for index, word := range chain {
		text := rec.FetchString(word, "text")
		if index < len(chain)-1 {
			text = hyphen.ReplaceAllString(text, "")
		}
		builder.WriteString(text)
	}
	return builder.String()
}

func copyList(value any) []any {
	list := rec.AsList(value)
	result := make([]any, len(list))
	for index, item := range list {
		if inner, ok := item.([]any); ok {
			result[index] = append([]any{}, inner...)
		} else {
			result[index] = item
		}
	}
	return result
}

// Record is SourceWordGroups.record.
func Record(chain []rec.Map, origins any) rec.Map {
	ids := make([]any, len(chain))
	for index, word := range chain {
		ids[index] = rec.Fetch(word, "id")
	}
	first := chain[0]
	text := JoinedText(chain)
	display := []string{}
	boxes := []any{}
	fragments := []any{}
	for _, word := range chain {
		display = append(display, rec.FetchString(word, "text"))
		if rec.Equal(rec.Fetch(word, "page"), rec.Fetch(first, "page")) {
			boxes = append(boxes, copyList(rec.Fetch(word, "boxes"))...)
		}
		fragments = append(fragments, fragment(word))
	}
	precisions := []string{}
	for _, origin := range rec.AsList(origins) {
		precision := fmt.Sprint(rec.Fetch(rec.AsMap(origin), "precision"))
		if !contains(precisions, precision) {
			precisions = append(precisions, precision)
		}
	}
	return rec.Map{"schema_version": GroupVersion, "id": "source-word-" + digest(ids)[:24],
		"text": text, "normalized_text": norm.NFKC.String(text),
		"display_text": strings.Join(display, ""), "word_ids": ids,
		"page": rec.Fetch(first, "page"), "baseline": append([]any{}, rec.FetchList(first, "baseline")...),
		"boxes": boxes, "fragments": fragments, "origins": origins, "source_precision": strings.Join(precisions, ","),
		"precision": "inferred source-confirmed continuation; native fragments and literal hyphen retained"}
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func fragment(word rec.Map) rec.Map {
	result := rec.Map{}
	for _, key := range []string{"id", "page"} {
		if value, ok := word[key]; ok {
			result[key] = value
		}
	}
	result["boxes"] = copyList(rec.Fetch(word, "boxes"))
	result["baseline"] = append([]any{}, rec.FetchList(word, "baseline")...)
	return result
}

// Groups is SourceWordGroups.groups: the logical words of a map that has them.
func Groups(m rec.Map) []rec.Map {
	if m["schema_version"] != "source-map-1.2" && m["schema_version"] != "source-map-2.0" {
		return nil
	}
	result := []rec.Map{}
	for _, group := range rec.FetchList(m, "logical_words") {
		result = append(result, rec.AsMap(group))
	}
	return result
}

func words(m rec.Map) []rec.Map {
	result := []rec.Map{}
	for _, word := range rec.FetchList(m, "words") {
		result = append(result, rec.AsMap(word))
	}
	return result
}

// Selection is SourceWordGroups.selection: the words, each group in place of its fragments.
func Selection(m rec.Map) []rec.Map {
	logical := Groups(m)
	if len(logical) == 0 {
		return words(m)
	}
	starts := map[any]rec.Map{}
	members := map[any]bool{}
	for _, group := range logical {
		ids := rec.FetchList(group, "word_ids")
		starts[rec.Index(ids, 0)] = group
		for _, id := range ids {
			members[id] = true
		}
	}
	result := []rec.Map{}
	for _, word := range words(m) {
		id := rec.Fetch(word, "id")
		if group, ok := starts[id]; ok {
			result = append(result, group)
		} else if !members[id] {
			result = append(result, word)
		}
	}
	return result
}

// Membership is SourceWordGroups.membership: each word id to its group, or itself.
func Membership(m rec.Map) map[string]rec.Map {
	result := map[string]rec.Map{}
	for _, word := range words(m) {
		result[rec.FetchString(word, "id")] = word
	}
	for _, group := range Groups(m) {
		for _, id := range rec.FetchList(group, "word_ids") {
			result[rec.AsString(id)] = group
		}
	}
	return result
}

// Index is SourceMap.index: per source file and line, the ids of the selected words there.
func Index(m rec.Map) rec.Map {
	index := rec.Map{}
	for _, word := range Selection(m) {
		id := rec.Fetch(word, "id")
		for _, value := range rec.FetchList(word, "origins") {
			origin := rec.AsMap(value)
			file := rec.FetchString(origin, "file")
			rows, ok := index[file].(rec.Map)
			if !ok {
				rows = rec.Map{}
				index[file] = rows
			}
			first := rec.FetchInt(origin, "line")
			last := first
			if end, present := origin["end_line"]; present {
				last = rec.AsInt(end)
			}
			for line := first; line <= last; line++ {
				key := fmt.Sprint(line)
				ids, _ := rows[key].([]any)
				if !includes(ids, id) {
					rows[key] = append(ids, id)
				} else {
					rows[key] = ids
				}
			}
		}
	}
	return index
}

func includes(list []any, value any) bool {
	for _, item := range list {
		if rec.Equal(item, value) {
			return true
		}
	}
	return false
}

// ValidateGroups is SourceWordGroups.validate!.
func ValidateGroups(m rec.Map) {
	all := words(m)
	positions := map[any]int{}
	for index, word := range all {
		positions[rec.Fetch(word, "id")] = index
	}
	if len(positions) != len(all) {
		rec.Fail("Duplicate native source word identity")
	}
	logical, ok := rec.Fetch(m, "logical_words").([]any)
	if !ok {
		rec.Fail("Logical source words must be an array")
	}
	used := map[any]bool{}
	for _, value := range logical {
		group, isMap := value.(rec.Map)
		if !isMap {
			rec.Fail("Logical source word must be an object")
		}
		ids := validMembership(rec.Fetch(group, "word_ids"), used)
		chain := []rec.Map{}
		previous := -1
		for _, id := range ids {
			position, found := positions[id]
			if !found {
				rec.Fail("Dangling source continuation fragment")
			}
			if previous >= 0 && position != previous+1 {
				rec.Fail("Source continuation fragments are not adjacent")
			}
			previous = position
			chain = append(chain, all[position])
		}
		validateBreaks(chain)
		if !rec.Equal(group, Record(chain, rec.Fetch(group, "origins"))) {
			rec.Fail("Source continuation changed native fragments or text")
		}
		if !literalEvidence(rec.FetchList(group, "origins")) {
			rec.Fail("Source continuation needs literal source evidence")
		}
		for _, id := range ids {
			used[id] = true
		}
	}
}

func validMembership(value any, used map[any]bool) []any {
	ids, ok := value.([]any)
	valid := ok && len(ids) >= 2 && len(ids) <= maxFragments
	for _, id := range ids {
		_, isString := id.(string)
		valid = valid && isString && !used[id]
	}
	if !valid {
		rec.Fail("Invalid source continuation membership")
	}
	return ids
}

func validateBreaks(chain []rec.Map) {
	for index := 0; index+1 < len(chain); index++ {
		left, right := chain[index], chain[index+1]
		if !hyphen.MatchString(rec.FetchString(left, "text")) ||
			(rec.Equal(left["page"], right["page"]) && rec.Equal(left["line"], right["line"])) {
			rec.Fail("Source continuation has no retained break")
		}
	}
}

func literalEvidence(origins []any) bool {
	for _, origin := range origins {
		if rec.AsMap(origin)["precision"] == "inferred_span" {
			return true
		}
	}
	return false
}
