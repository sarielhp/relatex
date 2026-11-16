package evidence

import (
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/sarielhp/relatex/internal/omap"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/stream"
)

var citationSources = []any{"natbib-command-hooks", "latex-command-hooks", "biblatex-command-hooks"}

// bibliographyLabel: a printed bibliography label, a number or an alphabetic label (amsalpha's
// ADM25, BLT+25, Szé20; biblatex's G E97 for an author whose family name carries the initials),
// or a year a letter tells apart from another of the same year (\bibitem[2012b]{..}), or an
// author-year label as typeset (Cheng et al., 2021): plain text ending in a year.
var bibliographyLabel = regexp.MustCompile(`\A(?:[1-9][0-9]*[a-z]?|\p{L}[\p{L}0-9+.-]*(?: [\p{L}0-9+.-]*[0-9][\p{L}0-9+.-]*)?|\p{L}[^\\{}$^_~#%]*[0-9]{4}[a-z]?)\z`)

var citationFields = map[string][]string{
	"citation_call": {"citation", "requested"}, "citation_sorted": {"citation", "keys"},
	"citation_link": {"citation", "citation_link", "key", "number"}, "citation_link_end": {"citation_link"},
	"citation_end": {"citation"}, "bibliography_begin": {"bibliography"},
	"bibliography_entry": {"entry", "bibliography", "key"}, "bibliography_anchor": {"entry", "key", "number"},
	"bibliography_entry_end": {"entry"}, "bibliography_end": {"bibliography"},
	"citation_retained":     {"citation", "owner", "first", "last", "page"},
	"bibliography_retained": {"entry", "owner", "first", "last", "page"},
}

var unbalancedKey = regexp.MustCompile(`[\\{}]`)
var bracedKey = regexp.MustCompile(`\A\{(.*)\}\z`)

// citationEvidence is the port of Relatex::CitationEvidence.
type citationEvidence struct {
	records     []rec.Map
	diagnostics []any
	warnings    []any
	groups      *omap.Map[string, []rec.Map]
	owners      map[int64]rec.Map
	calls       *omap.Map[int64, rec.Map]
	callEnds    *omap.Map[int64, rec.Map]
	keys        *omap.Map[int64, []string]
	entries     *omap.Map[int64, rec.Map]
	byKey       map[string]rec.Map
	numbers     map[int64]string
	links       *omap.Map[int64, rec.Map]
}

// CitationReport is CitationEvidence.new(records).report. Ruby's KeyError and IndexError
// (a missing field, a slot outside a candidate) are reported as invalid citation evidence.
func CitationReport(records []rec.Map) (result rec.Map) {
	defer func() {
		if recovered := recover(); recovered != nil {
			if failure, ok := recovered.(rec.Failure); ok && failure.Lookup {
				rec.Fail("Invalid citation evidence: %s", failure.Message)
			}
			panic(recovered)
		}
	}()
	c := newCitationEvidence(records)
	return c.report()
}

func newCitationEvidence(records []rec.Map) *citationEvidence {
	c := &citationEvidence{records: records, diagnostics: []any{}, warnings: []any{}, groups: omap.New[string, []rec.Map](),
		owners: map[int64]rec.Map{}}
	for _, row := range records {
		if !slices.Contains(citationSources, row["source"]) {
			continue
		}
		code := rec.FetchString(row, "code")
		rows, _ := c.groups.Get(code)
		c.groups.Set(code, append(rows, row))
	}
	// An alphabetic label keeps the braces of an accented name (Sz{\'e}20 reads Sz{é}20).
	for _, code := range []string{"bibliography_anchor", "citation_link"} {
		rows, ok := c.groups.Get(code)
		if !ok {
			continue
		}
		relabeled := make([]rec.Map, len(rows))
		for index, row := range rows {
			copied := rec.Map{}
			for key, value := range row {
				copied[key] = value
			}
			copied["number"] = printedLabel(row["number"])
			relabeled[index] = copied
		}
		c.groups.Set(code, relabeled)
	}
	for _, code := range c.groups.Keys() {
		if strings.HasPrefix(code, "unsupported_") {
			rec.Fail("Unsupported citation observation")
		}
	}
	for _, row := range records {
		if rec.Fetch(row, "kind") == "flow_candidate" {
			c.owners[rec.FetchInt(row, "owner")] = row
		}
	}
	return c
}

func (c *citationEvidence) report() rec.Map {
	if !stream.Enabled(c.records[0], "citations-0.1") {
		if c.metadataPresent() {
			rec.Fail("Citation metadata without external citation protocol")
		}
		return rec.Map{"status": "unavailable"}
	}
	c.validateRecords()
	c.validateLifecycle("citation_call", "citation_end", "citation")
	c.validateLifecycle("citation_link", "citation_link_end", "citation_link")
	c.validateLifecycle("bibliography_begin", "bibliography_end", "bibliography")
	c.validateLifecycle("bibliography_entry", "bibliography_entry_end", "entry")
	result := c.verify()
	result["status"] = "unresolved"
	if len(c.diagnostics) == 0 {
		result["status"] = "verified"
	}
	result["diagnostics"] = c.diagnostics
	result["warnings"] = c.warnings
	return result
}

func (c *citationEvidence) metadataPresent() bool {
	if c.groups.Len() > 0 {
		return true
	}
	for _, owner := range c.owners {
		for _, item := range rec.FetchList(owner, "items") {
			if hasAnyKey(rec.AsMap(item), "citation", "citation_link", "bibliography_entry") {
				return true
			}
		}
	}
	return false
}

func hasAnyKey(m rec.Map, keys ...string) bool {
	for _, key := range keys {
		if _, ok := m[key]; ok {
			return true
		}
	}
	return false
}

func (c *citationEvidence) validateRecords() {
	c.groups.Each(func(code string, rows []rec.Map) {
		fields, known := citationFields[code]
		if !known {
			rec.Fail("Unknown citation event")
		}
		for _, row := range rows {
			validateCitationRecord(row, code, fields)
		}
	})
}

func validateCitationRecord(row rec.Map, code string, fields []string) {
	if row["kind"] != "diagnostic" || !rec.Positive(row["seq"]) {
		rec.Fail("Invalid citation observation record")
	}
	optional := []string{}
	if _, linked := row["citation_link"]; code == "citation_retained" && linked {
		optional = []string{"citation_link"}
	}
	expected := append(append([]string{"seq", "kind", "code", "source"}, fields...), optional...)
	sort.Strings(expected)
	if !slices.Equal(sortedKeys(row), expected) {
		rec.Fail("Unknown or missing citation event fields")
	}
	for _, field := range append(append([]string{}, fields...), optional...) {
		value := rec.Fetch(row, field)
		var valid bool
		if field == "key" || field == "keys" || field == "requested" || field == "number" {
			_, valid = value.(string)
		} else {
			valid = rec.Positive(value)
		}
		if !valid {
			rec.Fail("Invalid citation event field type")
		}
	}
}

func (c *citationEvidence) validateLifecycle(opening, closing, field string) {
	stack := []any{}
	for _, row := range c.records {
		if !slices.Contains(citationSources, row["source"]) {
			continue
		}
		switch rec.Fetch(row, "code") {
		case opening:
			stack = append(stack, rec.Fetch(row, field))
		case closing:
			var popped any
			if len(stack) > 0 {
				popped = stack[len(stack)-1]
				stack = stack[:len(stack)-1]
			}
			if !rec.Equal(popped, rec.Fetch(row, field)) || popped == nil {
				rec.Fail("Crossed citation event lifetimes")
			}
		}
	}
	if len(stack) > 0 {
		rec.Fail("Unclosed citation event lifetime")
	}
}

// superscriptLabel: alpha's \etalchar{+} expands to a superscript formula ($^{+}$), which
// prints its characters raised: FHM{$^{+}$}20 prints FHM+20.
var superscriptLabel = regexp.MustCompile(`\$\^(?:\{([^${}\\]*)\}|([^${}\\]))\$`)

// printedLabel removes the grouping braces of a literal label (no macro inside).
func printedLabel(number any) any {
	text, ok := number.(string)
	if !ok || strings.Contains(text, `\`) || strings.Count(text, "{") != strings.Count(text, "}") {
		return number
	}
	text = superscriptLabel.ReplaceAllString(text, "$1$2")
	return strings.NewReplacer("{", "", "}", "").Replace(text)
}

func (c *citationEvidence) index(code, field string) *omap.Map[int64, rec.Map] {
	rows, _ := c.groups.Get(code)
	result := omap.New[int64, rec.Map]()
	valid := true
	for _, row := range rows {
		id, ok := rec.Fetch(row, field).(int64)
		if !ok || id <= 0 {
			valid = false
		}
		result.Set(id, row)
	}
	if !valid || result.Len() != len(rows) {
		rec.Fail("Duplicate or invalid citation identity")
	}
	return result
}

// citationKeys: keys as LaTeX reads them. One level of braces around a key is grouping
// ({MR4085124}), mcite's leading * merges the entry into the previous one, and a key may
// contain spaces (\bibitem{Feigenbaum 01}). A backslash or a remaining brace is a macro.
func citationKeys(value any) []string {
	text, ok := value.(string)
	if !ok {
		rec.Fail("Citation keys must be text")
	}
	parts := strings.Split(text, ",")
	for len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1] // Ruby's split drops trailing empty fields.
	}
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		key := rubyStrip(part)
		if match := bracedKey.FindStringSubmatch(key); match != nil {
			key = match[1]
		}
		key = strings.TrimPrefix(rubyStrip(key), "*")
		if key == "" || unbalancedKey.MatchString(key) {
			rec.Fail("Unsupported citation key tokens")
		}
		result = append(result, key)
	}
	if len(result) == 0 {
		rec.Fail("Unsupported citation key tokens")
	}
	return result
}

// rubyStrip is Ruby's String#strip: leading whitespace, trailing whitespace and NULs.
func rubyStrip(text string) string {
	return strings.TrimRight(strings.TrimLeft(text, " \t\n\v\f\r"), " \t\n\v\f\r\x00")
}

func (c *citationEvidence) catalogs() {
	c.calls = c.index("citation_call", "citation")
	sorted := c.index("citation_sorted", "citation")
	ends := c.index("citation_end", "citation")
	c.callEnds = ends
	if !slices.Equal(c.calls.Keys(), sorted.Keys()) || !sameSorted(c.calls.Keys(), ends.Keys()) {
		rec.Fail("Incomplete citation lifecycle")
	}
	c.keys = omap.New[int64, []string]()
	c.calls.Each(func(id int64, call rec.Map) {
		requested := citationKeys(rec.Fetch(call, "requested"))
		order, _ := sorted.Get(id)
		resolved := citationKeys(rec.Fetch(order, "keys"))
		if !sameSorted(uniqueStrings(requested), uniqueStrings(resolved)) {
			rec.Fail("Citation cluster lost or invented keys")
		}
		end, _ := ends.Get(id)
		if !(rec.FetchInt(call, "seq") < rec.FetchInt(order, "seq") && rec.FetchInt(order, "seq") < rec.FetchInt(end, "seq")) {
			rec.Fail("Invalid citation lifecycle order")
		}
		c.keys.Set(id, resolved)
	})
	c.bibliography()
	c.hyperlinks()
}

func sameSorted[T int64 | string](left, right []T) bool {
	a := slices.Clone(left)
	b := slices.Clone(right)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}

func uniqueStrings(values []string) []string {
	result := []string{}
	for _, value := range values {
		if !slices.Contains(result, value) {
			result = append(result, value)
		}
	}
	return result
}

func fetchEntry(m *omap.Map[int64, rec.Map], id int64) rec.Map {
	value, ok := m.Get(id)
	if !ok {
		panic(rec.Failure{Message: "key not found: " + itoa(id), Lookup: true})
	}
	return value
}

func (c *citationEvidence) bibliography() {
	frames := c.index("bibliography_begin", "bibliography")
	frameEnds := c.index("bibliography_end", "bibliography")
	c.entries = c.index("bibliography_entry", "entry")
	anchors := c.index("bibliography_anchor", "entry")
	ends := c.index("bibliography_entry_end", "entry")
	if !slices.Equal(c.entries.Keys(), anchors.Keys()) || !sameSorted(c.entries.Keys(), ends.Keys()) {
		rec.Fail("Incomplete bibliography lifecycle")
	}
	numbers := c.mergedNumbers(anchors)
	c.byKey = map[string]rec.Map{}
	c.numbers = map[int64]string{}
	c.entries.Each(func(id int64, entry rec.Map) {
		anchor := fetchEntry(anchors, id)
		frame := fetchEntry(frames, rec.FetchInt(entry, "bibliography"))
		frameEnd := fetchEntry(frameEnds, rec.FetchInt(entry, "bibliography"))
		end := fetchEntry(ends, id)
		if !(rec.FetchInt(frame, "seq") < rec.FetchInt(entry, "seq") && rec.FetchInt(end, "seq") < rec.FetchInt(frameEnd, "seq")) {
			rec.Fail("Bibliography item outside its environment lifetime")
		}
		if !rec.Equal(rec.Fetch(entry, "key"), rec.Fetch(anchor, "key")) {
			rec.Fail("Bibliography key/label mismatch")
		}
		number, present := numbers[id]
		if !present {
			c.diagnostics = append(c.diagnostics, rec.Map{"code": "unresolved_bibliography_number", "entry": id})
		} else if !bibliographyLabel.MatchString(number) {
			rec.Fail("Unsupported native bibliography number")
		}
		if !(rec.FetchInt(entry, "seq") < rec.FetchInt(anchor, "seq") && rec.FetchInt(anchor, "seq") < rec.FetchInt(end, "seq")) {
			rec.Fail("Invalid bibliography lifecycle order")
		}
		key := rec.FetchString(entry, "key")
		if previous, repeated := c.byKey[key]; repeated {
			c.warnings = append(c.warnings, rec.Map{"code": "bibliography_key_repeated", "key": key,
				"entry": id, "shadowed": previous["entry"]})
		}
		// LaTeX keeps the label of a key's last item (\bibitem{a} twice, or empty keys): a
		// citation of a repeated key cites that item.
		c.byKey[key] = rec.Map{"entry": id, "number": number}
		c.numbers[id] = number
	})
}

// mergedNumbers: REVTeX merges entries (\cite{a,*b}); b prints inside a's item, under a's
// number, and has no number of its own. An entry without a number takes its bibliography's
// preceding one. A missing number is absent from the result.
func (c *citationEvidence) mergedNumbers(anchors *omap.Map[int64, rec.Map]) map[int64]string {
	ids := c.entries.Keys()
	sort.SliceStable(ids, func(i, j int) bool {
		a, _ := c.entries.Get(ids[i])
		b, _ := c.entries.Get(ids[j])
		return rec.FetchInt(a, "seq") < rec.FetchInt(b, "seq")
	})
	previous := map[int64]*string{}
	result := map[int64]string{}
	for _, id := range ids {
		entry, _ := c.entries.Get(id)
		recorded := rec.FetchString(fetchEntry(anchors, id), "number")
		frame := rec.FetchInt(entry, "bibliography")
		number := &recorded
		if recorded == "" {
			number = previous[frame]
		}
		if number != nil {
			previous[frame] = number
			result[id] = *number
		}
	}
	return result
}

func (c *citationEvidence) hyperlinks() {
	c.links = c.index("citation_link", "citation_link")
	ends := c.index("citation_link_end", "citation_link")
	if !sameSorted(c.links.Keys(), ends.Keys()) {
		rec.Fail("Incomplete citation hyperlink lifecycle")
	}
	c.links.Each(func(id int64, link rec.Map) {
		citation := rec.FetchInt(link, "citation")
		call := fetchEntry(c.calls, citation)
		keys, _ := c.keys.Get(citation)
		key := rec.FetchString(link, "key")
		if !slices.Contains(keys, key) {
			rec.Fail("Hyperlink key outside citation cluster")
		}
		// natbib's compressed ranges ([3-5]) link their first key while \NAT@num already holds
		// the last number; the bibliography's own number is authoritative.
		number := rec.Fetch(c.target(key), "number")
		if !rec.Equal(number, rec.Fetch(link, "number")) {
			c.warnings = append(c.warnings, rec.Map{"code": "citation_link_number_differs", "citation_link": id, "key": key,
				"recorded": rec.Fetch(link, "number"), "bibliography": number})
		}
		end := fetchEntry(ends, id)
		if !(rec.FetchInt(call, "seq") < rec.FetchInt(link, "seq") && rec.FetchInt(link, "seq") < rec.FetchInt(end, "seq")) {
			rec.Fail("Invalid hyperlink lifecycle order")
		}
		if rec.FetchInt(end, "seq") >= rec.FetchInt(fetchEntry(c.callEnds, citation), "seq") {
			rec.Fail("Hyperlink closes outside citation call")
		}
	})
}

func (c *citationEvidence) target(key string) rec.Map {
	target, ok := c.byKey[key]
	if !ok {
		panic(rec.Failure{Message: "key not found: \"" + key + "\"", Lookup: true})
	}
	return target
}

type citationSlot struct {
	owner, slot int64
	page        any
	identity    any
	link        any
	linked      bool
}

func (s citationSlot) equal(other citationSlot) bool {
	return s.owner == other.owner && s.slot == other.slot && rec.Equal(s.page, other.page) &&
		rec.Equal(s.identity, other.identity) && s.linked == other.linked && rec.Equal(s.link, other.link)
}

func (c *citationEvidence) expectedSlots() ([]citationSlot, []citationSlot) {
	citations, bibliography := []citationSlot{}, []citationSlot{}
	for _, run := range c.records {
		if rec.Fetch(run, "kind") != "flow_occurrence" {
			continue
		}
		owner := c.owner(rec.FetchInt(run, "owner"))
		items := rec.FetchList(owner, "items")
		for slot := rec.FetchInt(run, "first"); slot <= rec.FetchInt(run, "last"); slot++ {
			item := rec.AsMap(rec.Index(items, int(slot-1)))
			base := citationSlot{owner: rec.FetchInt(run, "owner"), slot: slot, page: rec.Fetch(run, "page")}
			if truthy(item["citation_link"]) && !truthy(item["citation"]) {
				rec.Fail("Citation hyperlink without cluster")
			}
			if truthy(item["citation_link"]) &&
				!rec.Equal(rec.Fetch(fetchEntry(c.links, rec.AsInt(item["citation_link"])), "citation"), rec.Fetch(item, "citation")) {
				rec.Fail("Retained hyperlink belongs to a different citation cluster")
			}
			if truthy(item["citation"]) {
				entry := base
				entry.identity, entry.link, entry.linked = rec.Fetch(item, "citation"), item["citation_link"], true
				citations = append(citations, entry)
			}
			if truthy(item["bibliography_entry"]) {
				entry := base
				entry.identity = rec.Fetch(item, "bibliography_entry")
				bibliography = append(bibliography, entry)
			}
		}
	}
	return citations, bibliography
}

func (c *citationEvidence) owner(id int64) rec.Map {
	owner, ok := c.owners[id]
	if !ok {
		panic(rec.Failure{Message: "key not found: " + itoa(id), Lookup: true})
	}
	return owner
}

func (c *citationEvidence) observedSlots(code, identity string) []citationSlot {
	rows, _ := c.groups.Get(code)
	result := []citationSlot{}
	for _, run := range rows {
		owner := c.owner(rec.FetchInt(run, "owner"))
		first, okFirst := run["first"].(int64)
		last, okLast := run["last"].(int64)
		if !okFirst || !okLast || first <= 0 || last < first || last > int64(len(rec.FetchList(owner, "items"))) {
			rec.Fail("Invalid citation retained range")
		}
		if rec.FetchInt(owner, "seq") >= rec.FetchInt(run, "seq") {
			rec.Fail("Premature citation retained range")
		}
		for slot := first; slot <= last; slot++ {
			entry := citationSlot{owner: rec.FetchInt(run, "owner"), slot: slot, page: rec.Fetch(run, "page"),
				identity: rec.Fetch(run, identity)}
			if identity == "citation" {
				entry.link, entry.linked = run["citation_link"], true
			}
			result = append(result, entry)
		}
	}
	return result
}

func equalSlots(left, right []citationSlot) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !left[index].equal(right[index]) {
			return false
		}
	}
	return true
}

func (c *citationEvidence) verify() rec.Map {
	c.catalogs()
	expectedCitations, expectedBibliography := c.expectedSlots()
	citations := c.observedSlots("citation_retained", "citation")
	bibliography := c.observedSlots("bibliography_retained", "entry")
	if !equalSlots(expectedCitations, citations) {
		rec.Fail("Retained citation slot coverage differs")
	}
	if !equalSlots(expectedBibliography, bibliography) {
		rec.Fail("Retained bibliography slot coverage differs")
	}
	retainedCalls := uniqueIdentities(citations)
	retainedEntries := uniqueIdentities(bibliography)
	for _, id := range retainedCalls {
		keys, _ := c.keys.Get(id)
		if !c.keys.Has(id) {
			panic(rec.Failure{Message: "key not found: " + itoa(id), Lookup: true})
		}
		for _, key := range keys {
			if !slices.Contains(retainedEntries, rec.AsInt(rec.Fetch(c.target(key), "entry"))) {
				rec.Fail("Citation target is not retained")
			}
		}
	}
	return c.summary(citations, bibliography, retainedCalls, retainedEntries)
}

func uniqueIdentities(slots []citationSlot) []int64 {
	result := []int64{}
	for _, slot := range slots {
		id := rec.AsInt(slot.identity)
		if !slices.Contains(result, id) {
			result = append(result, id)
		}
	}
	return result
}

func (c *citationEvidence) summary(citations, bibliography []citationSlot, retainedCalls, retainedEntries []int64) rec.Map {
	calls := slices.Clone(retainedCalls)
	slices.Sort(calls)
	entries := slices.Clone(retainedEntries)
	slices.Sort(entries)
	callRecords := make([]any, len(calls))
	for index, id := range calls {
		callRecords[index] = c.citationCall(id)
	}
	entryRecords := make([]any, len(entries))
	for index, id := range entries {
		entryRecords[index] = c.bibliographyEntry(id)
	}
	links := []any{}
	seen := []any{}
	for _, slot := range citations {
		if !truthy(slot.link) || slices.ContainsFunc(seen, func(id any) bool { return rec.Equal(id, slot.link) }) {
			continue
		}
		seen = append(seen, slot.link)
		links = append(links, c.citationLink(rec.AsInt(slot.link)))
	}
	return rec.Map{"status": "observed_keys_labels_and_slot_coverage_verified", "qualified": false,
		"scope":          "Citation keys, native labels and retained reading-slot coverage",
		"executed_calls": int64(c.calls.Len()), "retained_calls": int64(len(retainedCalls)),
		"unretained_calls": int64(len(subtract(c.calls.Keys(), retainedCalls))), "hyperlink_executions": int64(c.links.Len()),
		"bibliography_entries": int64(c.entries.Len()), "retained_entries": int64(len(retainedEntries)),
		"retained_citation_slots": int64(len(citations)), "retained_bibliography_slots": int64(len(bibliography)),
		"calls": callRecords, "entries": entryRecords, "links": links,
		"discarded_calls": ints(subtract(c.calls.Keys(), retainedCalls))}
}

func (c *citationEvidence) retainedRanges(code, field string, id int64) []any {
	rows, _ := c.groups.Get(code)
	result := []any{}
	for _, row := range rows {
		if !rec.Equal(rec.Fetch(row, field), id) {
			continue
		}
		slice := rec.Map{}
		for _, key := range []string{"owner", "first", "last", "page", "citation_link"} {
			if value, ok := row[key]; ok {
				slice[key] = value
			}
		}
		result = append(result, slice)
	}
	return result
}

func (c *citationEvidence) citationCall(id int64) rec.Map {
	keys, _ := c.keys.Get(id)
	keyValues := make([]any, len(keys))
	targets := make([]any, len(keys))
	for index, key := range keys {
		keyValues[index] = key
		targets[index] = rec.Fetch(c.target(key), "entry")
	}
	return rec.Map{"id": id, "keys": keyValues, "targets": targets, "ranges": c.retainedRanges("citation_retained", "citation", id)}
}

func (c *citationEvidence) bibliographyEntry(id int64) rec.Map {
	entry := fetchEntry(c.entries, id)
	key := rec.FetchString(entry, "key")
	return rec.Map{"id": id, "key": key, "number": c.numbers[id],
		"ranges": c.retainedRanges("bibliography_retained", "entry", id)}
}

func (c *citationEvidence) citationLink(id int64) rec.Map {
	link := fetchEntry(c.links, id)
	target := c.target(rec.FetchString(link, "key"))
	return rec.Map{"citation": rec.Fetch(link, "citation"), "key": rec.Fetch(link, "key"), "number": rec.Fetch(target, "number"),
		"id": id, "entry": rec.Fetch(target, "entry")}
}
