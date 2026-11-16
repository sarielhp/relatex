package reading

import (
	"regexp"

	"github.com/sarielhp/relatex/internal/rec"
)

var scopeFields = map[string][]string{
	"float_context":   {"seq", "kind", "code", "source", "entity", "parent", "category"},
	"caption_context": {"seq", "kind", "code", "source", "entity", "parent", "category", "numbered", "float"},
}

var scopeSources = map[string]string{"float_context": "latex-float-environment-hooks", "caption_context": "caption-package-public-hooks"}

var category = regexp.MustCompile(`\A[a-z][a-z0-9]*\z`)

// partition is a reading partition: independently formatted float, caption and note
// paragraphs read apart from the surrounding flow. The zero value is the main flow.
type partition struct {
	kind string
	id   any
}

// scopes is ReadingScopes: independently formatted float/caption paragraphs remain distinct
// when packed into a surrounding paragraph. Claims require the collector's observed hooks.
type scopes struct {
	external bool
	contexts map[any]rec.Map
	scopes   map[any]rec.Map
	// members are the lists and theorems set in a float's body (float_member): entity -> float.
	members map[any]any
}

func newScopes(records []rec.Map) *scopes {
	s := &scopes{contexts: map[any]rec.Map{}, scopes: map[any]rec.Map{}, members: map[any]any{}}
	s.external = len(records) > 0 && records[0]["adapter"] == "external"
	for _, row := range records {
		if rec.Fetch(row, "kind") == "context" {
			s.contexts[rec.Fetch(row, "entity")] = row
		}
	}
	for _, record := range records {
		if rec.Fetch(record, "kind") != "diagnostic" {
			continue
		}
		if record["code"] == "float_member" {
			s.members[rec.Fetch(record, "entity")] = rec.Fetch(record, "float")
		}
		if _, known := scopeFields[toS(rec.Fetch(record, "code"))]; known {
			s.scope(record)
		}
	}
	return s
}

func (s *scopes) partition(owner rec.Map) (partition, bool) {
	scope, ok := s.scopes[rec.Fetch(owner, "entity")]
	if !ok {
		return s.memberFloat(owner)
	}
	if !less(rec.Fetch(scope, "seq"), rec.Fetch(owner, "seq")) {
		rec.Fail("Premature independent reading scope")
	}
	if rec.Fetch(scope, "code") == "caption_context" && positive(rec.Fetch(scope, "float")) {
		return partition{"float", rec.Fetch(scope, "float")}, true
	}
	if rec.Fetch(scope, "code") == "float_context" {
		return partition{"float", rec.Fetch(scope, "entity")}, true
	}
	return partition{"caption", rec.Fetch(scope, "entity")}, true
}

// memberFloat is the float partition of an owner with no scope of its own that belongs to a list or
// theorem set in a float's body (float_member), itself or through its structural parents (a list item
// in a table cell of a table float, papers2/0309). It is read with the float's paragraphs, so a cell's
// label paragraph that resumes after the list starts a new block in order.
func (s *scopes) memberFloat(owner rec.Map) (partition, bool) {
	entity := rec.Fetch(owner, "entity")
	for depth := 0; depth < 64 && entity != nil; depth++ {
		if float, member := s.members[entity]; member {
			scope, ok := s.scopes[float]
			if ok && rec.Fetch(scope, "code") == "float_context" && less(rec.Fetch(scope, "seq"), rec.Fetch(owner, "seq")) {
				return partition{"float", rec.Fetch(scope, "entity")}, true
			}
			return partition{}, false
		}
		context, found := s.contexts[entity]
		if !found {
			break
		}
		entity = context["parent"]
	}
	return partition{}, false
}

func (s *scopes) scope(record rec.Map) {
	code := rec.FetchString(record, "code")
	context, found := s.contexts[record["entity"]]
	sourceValid := record["source"] == scopeSources[code] ||
		(s.external && code == "caption_context" && record["source"] == "latex-native-caption-hooks-1")
	_, seen := s.scopes[record["entity"]]
	valid := rec.SameKeys(record, scopeFields[code]) && sourceValid && found && rec.Fetch(context, "role") == "paragraph" &&
		rec.Equal(rec.Fetch(context, "parent"), record["parent"]) && less(rec.Fetch(context, "seq"), rec.Fetch(record, "seq")) &&
		category.MatchString(toS(record["category"])) && !seen
	if !valid {
		rec.Fail("Invalid independent reading scope provenance")
	}
	if code == "caption_context" {
		s.caption(record)
	}
	s.scopes[rec.Fetch(record, "entity")] = record
}

func (s *scopes) caption(record rec.Map) {
	float, isInt := record["float"].(int64)
	numbered := record["numbered"]
	valid := (numbered == true || numbered == false) && isInt && float >= 0
	if valid && float > 0 {
		parent, ok := s.scopes[float]
		// A float may hold a caption of another type (\captionof{figure} inside a table).
		valid = ok && rec.Fetch(parent, "code") == "float_context" && less(rec.Fetch(parent, "seq"), rec.Fetch(record, "seq"))
	}
	if !valid {
		rec.Fail("Invalid independent caption scope")
	}
}
