package authormath

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/sarielhp/relatex/internal/mathsource"
	"github.com/sarielhp/relatex/internal/omap"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/xmltree"
)

// window: a formula in an absorbed argument (\footnote, \caption) reports the argument's last
// line: its text is looked for in the lines before it.
const window = 40

// fontPackages is MathSource::PACKAGES: math font and symbol packages the verification run
// loads when the paper does.
var fontPackages = []string{"mathptmx", "txfonts", "newtxmath", "pxfonts", "newpxmath", "mathpazo", "lmodern", "fourier",
	"kpfonts", "eucal", "upgreek", "bbm", "dsfont", "bbold", "stmaryrd", "wasysym", "MnSymbol", "esint"}

var (
	usePackage   = regexp.MustCompile(`(?m)^[^%\n]*?\\usepackage[ \t\n\v\f\r]*(\[[^\]]*\])?[ \t\n\v\f\r]*\{([^{}]+)\}`)
	blockDisplay = regexp.MustCompile(`\A<math[^>]*display="block"`)
	macroUse     = regexp.MustCompile(`\\([A-Za-z]+)`)
	sourceNoise  = regexp.MustCompile(`#\d|\\[A-Za-z]+|\\.`)
	alphanumeric = regexp.MustCompile(`[A-Za-z0-9]`)
)

// Reader returns a file's text as the collector named it, and whether it is the paper's.
type Reader func(file string) (string, bool)

// formulaRow is one math node being located.
type formulaRow struct {
	node    rec.Map
	formula int64
	display bool
	reason  string
	source  rec.Map
	group   any
	key     sourceKey
	tex     rec.Map
}

type sourceKey struct {
	file          string
	line, endLine int64
	display       bool
}

// source is MathSource: the author's TeX for each formula of a paper document. The collector
// recorded where each formula was read; the text is cut from that source file. A formula gets
// unverified with its text until verification decides, or unavailable with why.
type source struct {
	document     rec.Map
	inspection   rec.Map
	reader       Reader
	formulas     *omap.Map[any, rec.Map]
	occurrences  map[any]rec.Map
	order        map[any]int
	groups       map[any]rec.Map
	macros       *omap.Map[string, Macro]
	textMacros   []string
	texts        map[string]*mathsource.SourceText
	read         map[string]bool
	packages     []string
	claimed      map[string]map[*mathsource.Span]bool
	entityGroups map[any]any
}

// ApplySource is MathSource.new(inspection, document) { |file| ... }.apply!.
func ApplySource(inspection, document rec.Map, reader Reader) {
	s := &source{document: document, inspection: inspection, reader: reader, formulas: omap.New[any, rec.Map](),
		occurrences: map[any]rec.Map{}, order: map[any]int{}, groups: map[any]rec.Map{}, texts: map[string]*mathsource.SourceText{},
		read: map[string]bool{}, claimed: map[string]map[*mathsource.Span]bool{}}
	for _, value := range rec.FetchList(inspection, "math") {
		entry := rec.AsMap(value)
		if !s.formulas.Has(rec.Fetch(entry, "formula")) {
			s.formulas.Set(rec.Fetch(entry, "formula"), entry)
		}
	}
	for index, value := range rec.FetchList(rec.FetchMap(inspection, "math_retention"), "occurrences") {
		row := rec.AsMap(value)
		s.occurrences[rec.Fetch(row, "id")] = row
		s.order[rec.Fetch(row, "id")] = index
	}
	diagnostics, _ := inspection["diagnostics"].([]any)
	records := []rec.Map{}
	for _, value := range diagnostics {
		row := rec.AsMap(value)
		switch row["code"] {
		case "math_source_group":
			s.groups[rec.Fetch(row, "group")] = row
		case "math_macro":
			records = append(records, row)
		}
	}
	s.macros, _ = classify(records)
	for _, name := range s.macros.Keys() {
		if macro, _ := s.macros.Get(name); macro.TextMode {
			s.textMacros = append(s.textMacros, name)
		}
	}
	s.apply()
}

func (s *source) apply() {
	formulas := []*formulaRow{}
	for _, node := range s.mathNodes() {
		if row := s.entry(node); row != nil {
			formulas = append(formulas, row)
		}
	}
	grouped := omap.New[any, []*formulaRow]()
	for _, row := range formulas {
		if truthy(row.group) {
			rows, _ := grouped.Get(row.group)
			grouped.Set(row.group, append(rows, row))
		}
	}
	grouped.Each(func(group any, rows []*formulaRow) { s.assignGroup(group, rows) })
	singles := []*formulaRow{}
	for _, row := range formulas {
		if !truthy(row.group) && row.reason == "" {
			singles = append(singles, row)
		}
	}
	for _, rows := range byKey(singles) {
		s.assignExact(rows)
	}
	unplaced := []*formulaRow{}
	for _, row := range singles {
		if row.tex == nil {
			unplaced = append(unplaced, row)
		}
	}
	for _, rows := range byKey(unplaced) {
		s.assignWindow(rows)
	}
	texts := []string{}
	for _, row := range formulas {
		s.finish(row)
		if row.tex != nil {
			texts = append(texts, rec.FetchString(row.tex, "text"))
		}
	}
	s.document["math_macros"] = usedBy(s.macros, texts)
	packages := []any{}
	for _, name := range s.packages {
		if !slices.Contains(packages, any(name)) {
			packages = append(packages, name)
		}
	}
	s.document["math_packages"] = packages
}

func truthy(value any) bool { return value != nil && value != false }

// byKey is group_by(key), in the order keys first appear.
func byKey(rows []*formulaRow) [][]*formulaRow {
	index := map[sourceKey]int{}
	groups := [][]*formulaRow{}
	for _, row := range rows {
		at, ok := index[row.key]
		if !ok {
			at = len(groups)
			index[row.key] = at
			groups = append(groups, nil)
		}
		groups[at] = append(groups[at], row)
	}
	return groups
}

// mathNodes are the document's math nodes in the order the document made them: the
// retained occurrences' order.
func (s *source) mathNodes() []rec.Map {
	nodes := []rec.Map{}
	for _, value := range rec.FetchMap(s.document, "nodes") {
		if node := rec.AsMap(value); node["type"] == "math" {
			nodes = append(nodes, node)
		}
	}
	position := func(node rec.Map) int {
		key := rec.AsMap(node["provenance"])["inspection_key"]
		if at, ok := s.order[key]; ok {
			return at
		}
		return len(s.order)
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		if position(nodes[i]) != position(nodes[j]) {
			return position(nodes[i]) < position(nodes[j])
		}
		return rec.FetchString(nodes[i], "id") < rec.FetchString(nodes[j], "id")
	})
	return nodes
}

// MathNodes are a document's math nodes in the order the document made them.
func MathNodes(inspection, document rec.Map) []rec.Map {
	s := &source{document: document, order: map[any]int{}}
	for index, value := range rec.FetchList(rec.FetchMap(inspection, "math_retention"), "occurrences") {
		s.order[rec.Fetch(rec.AsMap(value), "id")] = index
	}
	return s.mathNodes()
}

func (s *source) entry(node rec.Map) *formulaRow {
	provenance, _ := node["provenance"].(rec.Map)
	occurrence, ok := s.occurrences[provenance["inspection_key"]]
	if !ok {
		return nil
	}
	record, found := s.formulas.Get(rec.Fetch(occurrence, "formula"))
	if !found {
		panic(rec.Failure{Message: fmt.Sprintf("key not found: %v", occurrence["formula"]), Lookup: true})
	}
	location, _ := record["source"].(rec.Map)
	if location == nil {
		location = s.entityGroup(record)
	}
	row := &formulaRow{node: node, formula: rec.FetchInt(record, "formula"),
		display: blockDisplay.MatchString(rec.FetchString(node, "mathml"))}
	if location == nil {
		row.reason = "math.source_missing"
		return row
	}
	if s.text(rec.FetchString(location, "file")) == nil {
		row.reason = "math.source_outside_paper"
		return row
	}
	row.source, row.group = location, location["group"]
	row.key = sourceKey{file: rec.FetchString(location, "file"), line: rec.FetchInt(location, "line"),
		endLine: rec.FetchInt(location, "end_line"), display: row.display}
	return row
}

// entityGroup: rows of an amsmath display are formulas made after the environment closed;
// they share its entity with the cells that carry the group.
func (s *source) entityGroup(record rec.Map) rec.Map {
	if s.entityGroups == nil {
		s.entityGroups = map[any]any{}
		s.formulas.Each(func(_ any, row rec.Map) {
			location, _ := row["source"].(rec.Map)
			group := location["group"]
			if _, seen := s.entityGroups[rec.Fetch(row, "entity")]; truthy(group) && !seen {
				s.entityGroups[rec.Fetch(row, "entity")] = group
			}
		})
	}
	group, ok := s.groups[s.entityGroups[rec.Fetch(record, "entity")]]
	if !ok {
		return nil
	}
	result := rec.Map{}
	for _, key := range []string{"file", "line", "end_line", "environment", "group"} {
		if value, present := group[key]; present {
			result[key] = value
		}
	}
	return result
}

func (s *source) text(file string) *mathsource.SourceText {
	if s.read[file] {
		return s.texts[file]
	}
	s.read[file] = true
	content, ok := s.reader(file)
	if !ok {
		s.texts[file] = nil
		return nil
	}
	s.collectPackages(content)
	s.texts[file] = mathsource.NewSourceText(content, s.textMacros)
	return s.texts[file]
}

func (s *source) collectPackages(content string) {
	for _, match := range usePackage.FindAllStringSubmatch(content, -1) {
		for _, name := range strings.Split(match[2], ",") {
			if name = rec.Strip(name); slices.Contains(fontPackages, name) {
				s.packages = append(s.packages, match[1]+"{"+name+"}")
			}
		}
	}
}

func sortByFormula(rows []*formulaRow) []*formulaRow {
	sorted := slices.Clone(rows)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].formula < sorted[j].formula })
	return sorted
}

func (s *source) assignGroup(group any, rows []*formulaRow) {
	record, ok := s.groups[group]
	var span *mathsource.Span
	if ok {
		text := s.text(rec.FetchString(record, "file"))
		if text == nil {
			rec.Fail("undefined method 'spans' for nil")
		}
		for _, candidate := range text.Spans {
			if candidate.Form == "environment" && rec.Equal(candidate.Line, record["line"]) && environmentOf(candidate) == record["environment"] {
				span = candidate
				break
			}
		}
	}
	if span == nil {
		for _, row := range rows {
			row.reason = "math.source_unlocated"
		}
		return
	}
	rows = sortByFormula(rows)
	parts := mathsource.Rows(span.Body)
	if len(rows) == 1 {
		s.locate(rows[0], span, rec.Strip(span.Body), 0, len(parts))
		return
	}
	if len(parts) != len(rows) {
		for _, row := range rows {
			row.reason = "math.source_rows_differ"
		}
		return
	}
	for index, row := range rows {
		s.locate(row, span, parts[index], index+1, len(parts))
	}
}

// environmentOf is a span's environment as a JSON value (nil outside an environment).
func environmentOf(span *mathsource.Span) any {
	if span.Environment == "" {
		return nil
	}
	return span.Environment
}

func (s *source) unclaimed(file string, span *mathsource.Span) bool { return !s.claimed[file][span] }

// assignExact: formulas read from exactly these lines, matched to the spans there in order.
func (s *source) assignExact(rows []*formulaRow) {
	key := rows[0].key
	spans := []*mathsource.Span{}
	for _, span := range s.text(key.file).Spans {
		if span.Line == key.line && span.EndLine == key.endLine && (span.Form != "inline") == key.display && s.unclaimed(key.file, span) {
			spans = append(spans, span)
		}
	}
	if len(spans) != len(rows) {
		return
	}
	for index, row := range sortByFormula(rows) {
		s.locate(row, spans[index], rec.Strip(spans[index].Body), 0, -1)
	}
}

// assignWindow: formulas the exact lines did not settle: an absorbed argument reports its
// last line, and some formulas there have no text of their own (a class's "et al"). Each
// formula, in order, takes the next unclaimed span before its line with the same letters and
// digits.
func (s *source) assignWindow(rows []*formulaRow) {
	key := rows[0].key
	spans := []*mathsource.Span{}
	for _, span := range s.text(key.file).Spans {
		if span.EndLine <= key.endLine && span.Line > key.line-window && (span.Form != "inline") == key.display &&
			s.unclaimed(key.file, span) {
			spans = append(spans, span)
		}
	}
	start := 0
	for _, row := range sortByFormula(rows) {
		signature := mathmlSignature(rec.FetchString(row.node, "mathml"))
		found := -1
		for at := start; at < len(spans); at++ {
			if s.sourceSignature(spans[at].Body) == signature {
				found = at
				break
			}
		}
		if found < 0 {
			row.reason = "math.source_unlocated"
			continue
		}
		s.locate(row, spans[found], rec.Strip(spans[found].Body), 0, -1)
		spans = slices.Delete(spans, found, found+1)
		start = found
	}
}

// mathmlSignature: the Latin letters and digits a formula shows, to tell spans apart.
func mathmlSignature(mathml string) string {
	root, err := xmltree.Parse(mathml)
	if err != nil {
		rec.Fail("undefined method 'text' for nil")
	}
	return strings.Join(alphanumeric.FindAllString(norm.NFKC.String(root.Content()), -1), "")
}

func (s *source) sourceSignature(text string) string {
	expanded := macroUse.ReplaceAllStringFunc(text, func(match string) string {
		if macro, ok := s.macros.Get(match[1:]); ok {
			return macro.Body
		}
		return match
	})
	return strings.Join(alphanumeric.FindAllString(sourceNoise.ReplaceAllString(expanded, ""), -1), "")
}

// locate gives a row its TeX; rowIndex is 0 and rows -1 when the formula is not a row of an
// alignment (Ruby's nil, which compact drops; a row count of 0 stays).
func (s *source) locate(row *formulaRow, span *mathsource.Span, text string, rowIndex, rows int) {
	file := rec.FetchString(row.source, "file")
	if rowIndex <= 1 {
		if s.claimed[file] == nil {
			s.claimed[file] = map[*mathsource.Span]bool{}
		}
		s.claimed[file][span] = true
	}
	tex := rec.Map{"status": "unverified", "reason": "math.source_untested", "text": text, "form": span.Form}
	if span.Environment != "" {
		tex["environment"] = span.Environment
	}
	if rowIndex > 0 {
		tex["row"] = int64(rowIndex)
		if group := row.source["group"]; group != nil {
			tex["group"] = group
		}
	}
	if rows >= 0 {
		tex["rows"] = int64(rows)
	}
	row.tex = tex
}

func (s *source) finish(row *formulaRow) {
	if row.tex != nil {
		row.node["tex"] = row.tex
		return
	}
	reason := row.reason
	if reason == "" {
		reason = "math.source_unlocated"
	}
	row.node["tex"] = rec.Map{"status": "unavailable", "reason": reason}
}
