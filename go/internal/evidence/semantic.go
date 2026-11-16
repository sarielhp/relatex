package evidence

import (
	"strconv"

	"github.com/sarielhp/relatex/internal/rec"
)

// semanticFields is SemanticEvidence::FIELDS: what the author's source said about each
// structural context, by diagnostic code.
var semanticFields = map[string]struct {
	kind   string
	fields []string
}{
	"heading_context":  {"heading", []string{"counter", "level", "runin"}},
	"theorem_context":  {"theorem", []string{"environment", "title", "counter", "numbered", "counter_register", "beamer_block", "untitled"}},
	"equation_context": {"equation", []string{"environment"}},
	"float_context":    {"float", []string{"category"}},
}

// SemanticReport is SemanticEvidence.new(records).report: entity (as a string key) to its
// kind and fields; the first record of an entity wins.
func SemanticReport(records []rec.Map) rec.Map {
	found := rec.Map{}
	for _, record := range records {
		code, _ := record["code"].(string)
		spec, known := semanticFields[code]
		entity, isInt := record["entity"].(int64)
		if record["kind"] != "diagnostic" || !known || !isInt {
			continue
		}
		key := strconv.FormatInt(entity, 10)
		if _, seen := found[key]; seen {
			continue
		}
		entry := rec.Map{"kind": spec.kind, "entity": entity}
		for _, field := range spec.fields {
			if value, ok := record[field]; ok {
				entry[field] = value
			}
		}
		found[key] = entry
	}
	return found
}
