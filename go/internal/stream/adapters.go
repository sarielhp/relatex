package stream

import (
	"fmt"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

// order is StreamExtensions::ORDER: extensions must be declared in this order.
var order = []string{"notes-0.1", "graphics-0.1", "graphics-0.2", "tables-0.1", "tables-0.2", "page-totals-0.1",
	"external-math-0.1", "text-style-0.1", "lists-0.1", "citations-0.1", "external-links-0.1"}

// externalOnly are the extensions only the external adapter may declare.
var externalOnly = []string{"external-math-0.1", "text-style-0.1", "lists-0.1", "citations-0.1", "external-links-0.1"}

// extensions returns the header's declared extensions and whether they are a list of strings.
func extensions(header rec.Map) ([]string, bool) {
	declared, ok := header["extensions"]
	if !ok {
		return nil, true
	}
	list, ok := declared.([]any)
	if !ok {
		return nil, false
	}
	names := make([]string, 0, len(list))
	for _, item := range list {
		name, isString := item.(string)
		if !isString {
			return nil, false
		}
		names = append(names, name)
	}
	return names, true
}

// ValidExtensions is StreamExtensions.valid?.
func ValidExtensions(header rec.Map) bool {
	names, ok := extensions(header)
	if !ok {
		return false
	}
	if slices.Contains(names, "graphics-0.1") && slices.Contains(names, "graphics-0.2") {
		return false
	}
	for _, name := range externalOnly {
		if slices.Contains(names, name) && header["adapter"] != "external" {
			return false
		}
	}
	if len(names) == 0 {
		return true
	}
	previous := -1
	for _, name := range names {
		position := slices.Index(order, name)
		if position <= previous {
			return false
		}
		previous = position
	}
	return header["protocol"] == "0.6" && (header["adapter"] == "bridge" || header["adapter"] == "external")
}

// Enabled is StreamExtensions.enabled?.
func Enabled(header rec.Map, extension string) bool {
	names, _ := extensions(header)
	return ValidExtensions(header) && slices.Contains(names, extension)
}

// Retained is StreamExtensions.retained?.
func Retained(header rec.Map) bool {
	return header["adapter"] == "bridge" || (header["adapter"] == "external" && Enabled(header, "external-math-0.1"))
}

func (s *Stream) validateAdapter(record rec.Map) {
	header := s.Header()
	kind := fmt.Sprint(rec.Fetch(record, "kind"))
	validateNoteAdapter(header, record, kind)
	validateGraphicAdapter(header, record)
	validateTableAdapter(header, record)
	validatePageAdapter(header, record)
	validateTextStyleAdapter(header, record)
	bridge := Retained(header)
	validateExternalMath(header, record, bridge)
	if kind == "diagnostic" && record["code"] == "math_choice_discard" && !bridge {
		rec.Fail("Math-choice provenance without bridge adapter")
	}
	protocol := fmt.Sprint(rec.Fetch(header, "protocol"))
	reading := bridge && slices.Contains([]string{"0.3", "0.4", "0.5", "0.6"}, protocol)
	if kind == "symbol_candidate" && !(bridge && slices.Contains([]string{"0.4", "0.5", "0.6"}, protocol)) {
		rec.Fail("Symbol event without protocol 0.4 bridge adapter")
	}
	if slices.Contains([]string{"flow_candidate", "flow_occurrence", "reference"}, kind) && !reading {
		rec.Fail("Reading event without protocol 0.3 bridge adapter")
	}
	if strings.HasPrefix(kind, "math_") && !bridge {
		rec.Fail("Math bridge event without bridge adapter")
	}
	if reading {
		requireReadingFields(record, kind, protocol)
	}
}

func requireReadingFields(record rec.Map, kind, protocol string) {
	fields := []string{}
	switch kind {
	case "math_candidate":
		fields = []string{"display"}
	case "label":
		fields = []string{"counter", "anchor", "target_entity"}
	}
	if kind == "label" && truthy(record["formula"]) && (protocol == "0.5" || protocol == "0.6") {
		fields = append(fields, "math_environment")
	}
	if kind == "context" && protocol == "0.6" {
		fields = append(fields, "parent")
	}
	for _, field := range fields {
		if _, ok := record[field]; !ok {
			rec.Fail("Missing reading adapter fields")
		}
	}
}

// truthy is Ruby's truthiness: everything but nil and false.
func truthy(value any) bool {
	return value != nil && value != false
}

func items(record rec.Map) []any {
	return rec.FetchList(record, "items")
}

func anyItem(record rec.Map, test func(rec.Map) bool) bool {
	for _, item := range items(record) {
		if test(rec.AsMap(item)) {
			return true
		}
	}
	return false
}

func hasKey(item rec.Map, keys ...string) bool {
	for _, key := range keys {
		if _, ok := item[key]; ok {
			return true
		}
	}
	return false
}

func validateTextStyleAdapter(header, record rec.Map) {
	if record["kind"] != "flow_candidate" {
		return
	}
	if !anyItem(record, func(item rec.Map) bool { return hasKey(item, "style") }) {
		return
	}
	if !Enabled(header, "text-style-0.1") {
		rec.Fail("Text styles without declared extension")
	}
}

func validateExternalMath(header, record rec.Map, retained bool) {
	if record["kind"] != "math" {
		return
	}
	external := rec.Fetch(header, "adapter") == "external"
	standalone := record["provenance"] == "luamml-mlist"
	if standalone && !retained {
		rec.Fail("External math without its retained-observer extension")
	}
	if standalone != external {
		rec.Fail("Math provenance disagrees with collector adapter")
	}
}

func validatePageAdapter(header, record rec.Map) {
	code := record["code"]
	if record["kind"] == "diagnostic" && (code == "document_page_total" || code == "unsupported_document_page_total") &&
		!Enabled(header, "page-totals-0.1") {
		rec.Fail("Document page evidence without declared extension")
	}
}

func validateTableAdapter(header, record rec.Map) {
	flow := record["kind"] == "flow_candidate"
	diagnostic := record["kind"] == "diagnostic"
	linked := flow && anyItem(record, func(item rec.Map) bool { return hasKey(item, "external_link") })
	definition := diagnostic && record["code"] == "external_link"
	if (linked || definition) && !Enabled(header, "external-links-0.1") {
		rec.Fail("External link evidence without declared extension")
	}
	modern := diagnostic && strings.HasPrefix(rec.FetchString(record, "code"), "rich_table_")
	cell := flow && anyItem(record, func(item rec.Map) bool { return hasKey(item, "table_cell", "table") })
	if (modern || cell) && !Enabled(header, "tables-0.2") {
		rec.Fail("Rich table evidence without declared extension")
	}
	code := record["code"]
	event := diagnostic && (code == "table_snapshot" || code == "table_presence" || code == "unsupported_table_layout")
	_, hasTable := record["table"]
	bound := diagnostic && code == "unclassified_running_rule" && hasTable
	if (event || bound) && !Enabled(header, "tables-0.1") {
		rec.Fail("Table evidence without declared extension")
	}
}

func validateGraphicAdapter(header, record rec.Map) {
	item := record["kind"] == "flow_candidate" && anyItem(record, func(slot rec.Map) bool { return slot["type"] == "image" })
	code := record["code"]
	event := record["kind"] == "diagnostic" && slices.Contains([]any{"graphic_source", "graphic_use", "graphic_placement",
		"unsupported_graphic_source", "unsupported_graphic_transform", "graphic_alternative", "figure_description", "figure_alt",
		"picture_alternative", "graphic_origin"}, code)
	enabled := Enabled(header, "graphics-0.1") || Enabled(header, "graphics-0.2")
	if (item || event) && !enabled {
		rec.Fail("Graphic evidence without declared extension")
	}
}

func validateNoteAdapter(header, record rec.Map, kind string) {
	notes := Enabled(header, "notes-0.1")
	fields := kind == "flow_candidate" && anyItem(record, func(item rec.Map) bool { return hasKey(item, "note_mark", "note_number") })
	role := (kind == "context" || kind == "candidate" || kind == "flow_candidate") && record["role"] == "footnote"
	code := record["code"]
	noted := role || (kind == "diagnostic" && (code == "note_mark" || code == "note_body"))
	if !notes && (fields || noted) {
		rec.Fail("Note evidence without declared extension")
	}
}
