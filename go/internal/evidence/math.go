package evidence

import (
	"encoding/xml"
	"errors"
	"io"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/sarielhp/relatex/internal/omap"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/stream"
)

const mathMLNamespace = "http://www.w3.org/1998/Math/MathML"

// mathEvidence is the port of Relatex::MathEvidence: it reconciles execution identities and
// full fragment coverage, never formula strings.
type mathEvidence struct {
	records       []rec.Map
	candidates    *omap.Map[int64, rec.Map]
	fragments     *omap.Map[int64, rec.Map]
	math          *omap.Map[int64, rec.Map]
	diagnostics   []any
	open          *omap.Map[int64, *mathOccurrence]
	completed     []*mathOccurrence
	started       int
	choiceDiscard []any
	choiceTrials  map[int64]bool
}

type mathOccurrence struct {
	formula int64
	seen    *omap.Map[int64, *fragmentSeen]
	pages   []any
	start   int
}

type fragmentSeen struct {
	count int64
	last  int64
}

// MathReport is MathEvidence.new(records).report.
func MathReport(records []rec.Map) rec.Map {
	m := &mathEvidence{records: records, candidates: omap.New[int64, rec.Map](), fragments: omap.New[int64, rec.Map](),
		math: omap.New[int64, rec.Map](), open: omap.New[int64, *mathOccurrence](), choiceTrials: map[int64]bool{},
		diagnostics: []any{}}
	return m.report()
}

func (m *mathEvidence) report() rec.Map {
	if !stream.Retained(m.records[0]) {
		return rec.Map{"status": "unavailable", "occurrences": []any{}, "discarded_candidates": []any{},
			"trial_fragments": []any{}, "diagnostics": []any{}}
	}
	m.inventory()
	m.inventoryCompositions()
	m.inventoryChoices()
	for _, record := range m.records {
		if record["kind"] == "math_occurrence" {
			m.observe(record)
		}
	}
	for m.open.Len() > 0 {
		_, entry, _ := m.open.First()
		m.incomplete(entry)
	}
	occurrences, retained := m.occurrences()
	m.missingDiagnostics()
	status := "verified"
	// Formulas with no visible nodes (a paragraph inside the math box) are listed but do not
	// make the retention unresolved: the export prints them from their MathML and says so.
	for _, row := range m.diagnostics {
		if rec.AsMap(row)["code"] != "math_without_visible_nodes" {
			status = "unresolved"
		}
	}
	result := rec.Map{"status": status, "occurrences": occurrences,
		"discarded_candidates": ints(subtract(m.candidates.Keys(), retained)), "trial_fragments": ints(m.trialFragments()),
		"diagnostics": m.diagnostics}
	if len(m.choiceDiscard) > 0 {
		result["choice_discards"] = m.choiceDiscard
	}
	return result
}

func (m *mathEvidence) occurrences() ([]any, []int64) {
	sort.SliceStable(m.completed, func(i, j int) bool { return m.completed[i].start < m.completed[j].start })
	occurrences := make([]any, 0, len(m.completed))
	retained := []int64{}
	for index, entry := range m.completed {
		occurrences = append(occurrences, rec.Map{"id": "m" + strconv.Itoa(index+1), "formula": entry.formula,
			"fragments": ints(entry.seen.Keys()), "pages": uniqueValues(entry.pages)})
		if !slices.Contains(retained, entry.formula) {
			retained = append(retained, entry.formula)
		}
	}
	return occurrences, retained
}

func (m *mathEvidence) missingDiagnostics() {
	missing := []int64{}
	for _, id := range m.candidates.Keys() {
		if !m.math.Has(id) {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		m.diagnostics = append(m.diagnostics, rec.Map{"code": "missing_mathml", "formulas": ints(missing)})
	}
	visible := []int64{}
	m.fragments.Each(func(_ int64, fragment rec.Map) {
		if !m.trial(fragment) {
			visible = append(visible, rec.FetchInt(fragment, "formula"))
		}
	})
	empty := subtract(subtract(subtract(m.candidates.Keys(), visible), m.externalTrials()), m.nativeInvisible())
	if len(empty) > 0 {
		m.diagnostics = append(m.diagnostics, rec.Map{"code": "math_without_visible_nodes", "formulas": ints(empty)})
	}
}

func (m *mathEvidence) trialFragments() []int64 {
	trials := []int64{}
	m.fragments.Each(func(id int64, fragment rec.Map) {
		if m.trial(fragment) {
			trials = append(trials, id)
		}
	})
	return trials
}

func (m *mathEvidence) nativeInvisible() []int64 {
	result := []int64{}
	for _, record := range m.records {
		if record["code"] != "external_math_without_ink" {
			continue
		}
		id := rec.FetchInt(record, "formula")
		candidate, known := m.candidates.Get(id)
		keys := sortedKeys(record)
		valid := m.records[0]["adapter"] == "external" && record["source"] == "native-math-boxes" &&
			slices.Equal(keys, []string{"code", "formula", "glyphs", "kind", "rules", "seq", "source"}) &&
			rec.Equal(record["glyphs"], int64(0)) && rec.Equal(record["rules"], int64(0)) && known && m.math.Has(id) &&
			rec.FetchInt(candidate, "seq") < rec.FetchInt(record, "seq") && !m.hasFragmentOf(id)
		if !valid {
			rec.Fail("Invalid native invisible-math provenance")
		}
		result = append(result, id)
	}
	return result
}

func (m *mathEvidence) hasFragmentOf(formula int64) bool {
	found := false
	m.fragments.Each(func(_ int64, fragment rec.Map) {
		if rec.FetchInt(fragment, "formula") == formula {
			found = true
		}
	})
	return found
}

func (m *mathEvidence) inventory() {
	for _, record := range m.records {
		switch rec.Fetch(record, "kind") {
		case "math_candidate":
			m.validateExternalTrial(record)
			unique(m.candidates, rec.FetchInt(record, "formula"), record)
		case "math_fragment":
			if !m.candidates.Has(rec.FetchInt(record, "formula")) {
				rec.Fail("Unknown formula in fragment")
			}
			unique(m.fragments, rec.FetchInt(record, "fragment"), record)
		case "math":
			id := rec.FetchInt(record, "formula")
			if !m.candidates.Has(id) {
				rec.Fail("MathML has no execution candidate")
			}
			validateMathML(rec.FetchString(record, "mathml"))
			unique(m.math, id, record)
		}
	}
}

func (m *mathEvidence) validateExternalTrial(record rec.Map) {
	_, trial := record["trial"]
	_, provenance := record["trial_provenance"]
	if !trial && !provenance {
		return
	}
	valid := m.records[0]["adapter"] == "external" && record["trial"] == true && record["trial_provenance"] == "amsmath-ifmeasuring"
	if !valid {
		rec.Fail("Invalid external math trial provenance")
	}
}

func (m *mathEvidence) externalTrials() []int64 {
	result := []int64{}
	m.candidates.Each(func(id int64, candidate rec.Map) {
		if candidate["trial"] != true {
			return
		}
		fragments := []rec.Map{}
		m.fragments.Each(func(_ int64, fragment rec.Map) {
			if rec.FetchInt(fragment, "formula") == id {
				fragments = append(fragments, fragment)
			}
		})
		if len(fragments) == 0 {
			return
		}
		for _, fragment := range fragments {
			if !truthy(rec.Fetch(fragment, "trial")) {
				rec.Fail("Declared external math trial has nontrial fragments")
			}
		}
		result = append(result, id)
	})
	return result
}

// validateMathML requires well-formed XML whose root is MathML's math element.
func validateMathML(text string) {
	if strings.Contains(text, "<!") {
		rec.Fail("MathML declarations are unsupported")
	}
	decoder := xml.NewDecoder(strings.NewReader(text))
	var root *xml.StartElement
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			rec.Fail("Invalid MathML: %s", err.Error())
		}
		if start, ok := token.(xml.StartElement); ok && root == nil {
			element := start
			root = &element
		}
	}
	if root == nil || root.Name.Local != "math" || root.Name.Space != mathMLNamespace {
		rec.Fail("Expected MathML root and namespace")
	}
}

func (m *mathEvidence) inventoryChoices() {
	choices := map[int64]bool{}
	for _, record := range m.records {
		if record["kind"] != "diagnostic" || rec.Fetch(record, "code") != "math_choice_discard" {
			continue
		}
		formula, known := m.candidates.Get(asInt(record["formula"]))
		style, styled := record["chosen_style"].(int64)
		choice, chosen := record["choice"].(int64)
		valid := record["source"] == "luatex-mlist-style" && styled && style >= 0 && style <= 7 && chosen && choice > 0 &&
			!choices[choice] && known && rec.FetchInt(formula, "seq") < rec.FetchInt(record, "seq")
		if !valid {
			rec.Fail("Invalid math-choice provenance")
		}
		discarded := m.choiceFragments(record, "discarded_fragments")
		kept := m.choiceFragments(record, "kept_fragments")
		if len(discarded) == 0 || intersects(discarded, kept) {
			rec.Fail("Empty or overlapping math-choice discard")
		}
		for _, id := range discarded {
			m.choiceTrials[id] = true
		}
		m.choiceDiscard = append(m.choiceDiscard, record)
		choices[rec.FetchInt(record, "choice")] = true
	}
}

func (m *mathEvidence) inventoryCompositions() {
	for _, record := range m.records {
		if record["code"] != "external_math_composition" {
			continue
		}
		parent, known := m.candidates.Get(asInt(record["formula"]))
		children, isList := record["children"].([]any)
		valid := m.records[0]["adapter"] == "external" && record["source"] == "native-alignment-cells" && known &&
			rec.FetchInt(parent, "seq") < rec.FetchInt(record, "seq") && isList && len(children) > 0 && uniqueList(children)
		if !valid {
			rec.Fail("Invalid native math composition")
		}
		for _, child := range children {
			m.validateChild(child, parent)
		}
	}
}

func (m *mathEvidence) validateChild(child any, parent rec.Map) {
	id, isInt := child.(int64)
	candidate, known := m.candidates.Get(id)
	if !isInt || !known || id >= rec.FetchInt(parent, "formula") || rec.FetchInt(candidate, "seq") >= rec.FetchInt(parent, "seq") ||
		!m.adoptable(candidate, parent) {
		rec.Fail("Foreign or premature native matrix cell")
	}
}

// adoptable reports whether a composed formula may take the candidate as a cell: it lies in the
// parent's entity, or in an equation context of its own that closed before the parent (empheq sets its
// alignat in a box, then the box in an equation of the enclosing paragraph, paper 414).
func (m *mathEvidence) adoptable(candidate, parent rec.Map) bool {
	entity := rec.Fetch(candidate, "entity")
	if rec.Equal(entity, rec.Fetch(parent, "entity")) {
		return true
	}
	for _, record := range m.records {
		if record["code"] == "equation_context" && rec.Equal(rec.Fetch(record, "entity"), entity) &&
			rec.FetchInt(record, "seq") < rec.FetchInt(candidate, "seq") {
			return true
		}
	}
	return false
}

func (m *mathEvidence) choiceFragments(record rec.Map, field string) []int64 {
	list, ok := record[field].([]any)
	if !ok || !uniqueList(list) {
		rec.Fail("Invalid math-choice fragment list")
	}
	ids := make([]int64, 0, len(list))
	for _, item := range list {
		id, isInt := item.(int64)
		fragment, known := m.fragments.Get(id)
		if !isInt || id <= 0 || !known || !rec.Equal(rec.Fetch(fragment, "formula"), rec.Fetch(record, "formula")) ||
			rec.FetchInt(fragment, "seq") >= rec.FetchInt(record, "seq") {
			rec.Fail("Dangling, foreign, or premature math-choice fragment")
		}
		ids = append(ids, id)
	}
	return ids
}

func (m *mathEvidence) trial(fragment rec.Map) bool {
	return truthy(rec.Fetch(fragment, "trial")) || m.choiceTrials[rec.FetchInt(fragment, "fragment")]
}

func unique(inventory *omap.Map[int64, rec.Map], id int64, record rec.Map) {
	if inventory.Has(id) {
		rec.Fail("Duplicate math identity")
	}
	inventory.Set(id, record)
}

func (m *mathEvidence) observe(record rec.Map) {
	fragment, known := m.fragments.Get(rec.FetchInt(record, "fragment"))
	if !known || !rec.Equal(rec.Fetch(fragment, "formula"), rec.Fetch(record, "formula")) {
		rec.Fail("Dangling math fragment")
	}
	if m.trial(fragment) {
		rec.Fail("Trial mathematics survived shipout")
	}
	validateRange(record, fragment)
	formula := rec.FetchInt(record, "formula")
	entry, open := m.open.Get(formula)
	if !open {
		entry = m.startOccurrence(formula)
		m.open.Set(formula, entry)
	}
	fragmentID := rec.FetchInt(record, "fragment")
	first, last := rec.FetchInt(record, "first"), rec.FetchInt(record, "last")
	// A range that does not continue the open occurrence starts a new one (a box shown
	// partly, then again in full); the partial one is incomplete.
	if seenFragment(entry, fragmentID).last >= first {
		if first != 1 {
			rec.Fail("Overlapping or reversed math ranges before a complete occurrence")
		}
		m.incomplete(entry)
		entry = m.startOccurrence(formula)
		m.open.Set(formula, entry)
	}
	seen := seenFragment(entry, fragmentID)
	seen.count += last - first + 1
	seen.last = last
	entry.pages = append(entry.pages, rec.Fetch(record, "page"))
	if m.covered(entry) {
		m.completed = append(m.completed, entry)
		m.open.Delete(entry.formula)
	}
}

func seenFragment(entry *mathOccurrence, id int64) *fragmentSeen {
	seen, ok := entry.seen.Get(id)
	if !ok {
		rec.Fail("key not found: %d", id)
	}
	return seen
}

func (m *mathEvidence) covered(entry *mathOccurrence) bool {
	complete := true
	entry.seen.Each(func(id int64, found *fragmentSeen) {
		fragment, _ := m.fragments.Get(id)
		if found.count != rec.FetchInt(fragment, "nodes") {
			complete = false
		}
	})
	return complete
}

// startOccurrence: occurrences of several formulas can be open at once (a formula inside
// \text{} inside another formula is printed between the outer formula's pieces); each
// completes on its own, and occurrences keep the order in which they started.
func (m *mathEvidence) startOccurrence(formula int64) *mathOccurrence {
	m.started++
	entry := &mathOccurrence{formula: formula, seen: omap.New[int64, *fragmentSeen](), pages: []any{}, start: m.started}
	m.fragments.Each(func(id int64, fragment rec.Map) {
		if rec.FetchInt(fragment, "formula") == formula && !m.trial(fragment) {
			entry.seen.Set(id, &fragmentSeen{})
		}
	})
	return entry
}

func validateRange(record, fragment rec.Map) {
	first, last := rec.FetchInt(record, "first"), rec.FetchInt(record, "last")
	if first <= 0 || first > last || last > rec.FetchInt(fragment, "nodes") {
		rec.Fail("Invalid retained math range")
	}
	if rec.FetchInt(record, "seq") <= rec.FetchInt(fragment, "seq") {
		rec.Fail("Math occurrence precedes fragment definition")
	}
}

func (m *mathEvidence) incomplete(entry *mathOccurrence) {
	missing := rec.Map{}
	entry.seen.Each(func(id int64, seen *fragmentSeen) {
		fragment, _ := m.fragments.Get(id)
		missing[strconv.FormatInt(id, 10)] = rec.FetchInt(fragment, "nodes") - seen.count
	})
	m.diagnostics = append(m.diagnostics, rec.Map{"code": "incomplete_math_occurrence", "formula": entry.formula,
		"missing_nodes": missing})
	m.open.Delete(entry.formula)
}
