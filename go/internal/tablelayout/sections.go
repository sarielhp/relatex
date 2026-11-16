package tablelayout

import (
	"slices"
	"sort"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/rubynum"
)

// Sections is TableSections: canonical section rows stay source-bound; a target may
// paginate them anew. Pairs first and continuation cells without copying body content.
type Sections struct {
	layout     *Layout
	sections   map[string][]int64
	Heights    []any
	Cells      []SectionCell
	Captions   []rec.Map
	RepeatHead bool
	RepeatFoot bool
}

// SectionCell is one cell of a section, with its alternate (continuation) variant.
type SectionCell struct {
	Cell      rec.Map
	Alternate rec.Map
	Row       int
	Section   string
	First     bool
	Last      bool
}

var sectionRoles = []string{"caption", "first_head", "repeat_head", "body", "repeat_foot", "last_foot"}

// NewSections is TableSections.new(layout).
func NewSections(layout *Layout) *Sections {
	s := &Sections{layout: layout, sections: map[string][]int64{}}
	rows := layout.Model.Rows
	if value, ok := layout.Model.Data["sections"]; ok {
		for role, indices := range rec.AsMap(value) {
			list := []int64{}
			for _, index := range rec.AsList(indices) {
				list = append(list, rec.AsInt(index))
			}
			s.sections[role] = list
		}
	} else {
		body := []int64{}
		for index := range rows {
			body = append(body, int64(index))
		}
		s.sections["body"] = body
	}
	s.validate()
	for _, row := range s.indices("caption") {
		s.Captions = append(s.Captions, rows[row]...)
	}
	first, repeat := s.indices("first_head"), s.indices("repeat_head")
	s.RepeatHead = len(repeat) > 0
	if len(first) == 0 {
		first = repeat
	}
	if len(repeat) == 0 {
		repeat = first
	}
	s.addVariants(first, repeat, "header")
	s.addBody()
	continuation, last := s.indices("repeat_foot"), s.indices("last_foot")
	s.RepeatFoot = len(continuation) > 0
	if len(last) == 0 {
		last = continuation
	}
	if len(continuation) == 0 {
		continuation = last
	}
	s.addVariants(continuation, last, "footer")
	return s
}

func (s *Sections) indices(role string) []int64 { return s.sections[role] }

func (s *Sections) validate() {
	all := []int64{}
	for role, indices := range s.sections {
		if !slices.Contains(sectionRoles, role) {
			rec.Fail("Unknown table section role")
		}
		all = append(all, indices...)
	}
	slices.Sort(all)
	expected := make([]int64, len(s.layout.Model.Rows))
	for index := range expected {
		expected[index] = int64(index)
	}
	if !slices.Equal(all, expected) {
		rec.Fail("Table sections overlap or omit logical rows")
	}
	for _, indices := range s.sections {
		sorted := slices.Compact(slices.Sorted(slices.Values(indices)))
		if !slices.Equal(sorted, indices) {
			rec.Fail("Unordered table section rows")
		}
	}
}

type shapeKey struct{ offset, column, rowspan, colspan int64 }

type shapeEntry struct {
	key  shapeKey
	cell rec.Map
}

func (s *Sections) shape(rows []int64) []shapeEntry {
	result := []shapeEntry{}
	for offset, row := range rows {
		for _, cell := range s.layout.Model.Rows[row] {
			rowspan := rec.FetchInt(cell, "rowspan")
			for step := int64(0); step < rowspan; step++ {
				index := int64(offset) + step
				if index >= int64(len(rows)) || rows[index] != row+step {
					rec.Fail("Cell %s crosses a table section boundary", rec.ToS(rec.Fetch(cell, "id")))
				}
			}
			result = append(result, shapeEntry{shapeKey{int64(offset), rec.FetchInt(cell, "column"), rowspan, rec.FetchInt(cell, "colspan")}, cell})
		}
	}
	return result
}

func sortedKeys(entries []shapeEntry) []shapeKey {
	keys := []shapeKey{}
	for _, entry := range entries {
		keys = append(keys, entry.key)
	}
	sort.Slice(keys, func(a, b int) bool {
		x, y := keys[a], keys[b]
		return x.offset < y.offset || x.offset == y.offset && (x.column < y.column || x.column == y.column &&
			(x.rowspan < y.rowspan || x.rowspan == y.rowspan && x.colspan < y.colspan))
	})
	return keys
}

func (s *Sections) addVariants(primary, alternate []int64, section string) {
	if len(primary) == 0 && len(alternate) == 0 {
		return
	}
	if len(primary) != len(alternate) {
		rec.Fail("First/repeated table sections have differing row counts")
	}
	before, after := s.shape(primary), s.shape(alternate)
	if !slices.Equal(sortedKeys(before), sortedKeys(after)) {
		rec.Fail("First/repeated table sections have incompatible cell spans")
	}
	offset := len(s.Heights)
	for index := range primary {
		s.Heights = append(s.Heights, rubynum.Maximum(s.layout.RowHeights[primary[index]], s.layout.RowHeights[alternate[index]]))
	}
	alternates := map[shapeKey]rec.Map{}
	for _, entry := range after {
		alternates[entry.key] = entry.cell
	}
	for _, entry := range before {
		s.Cells = append(s.Cells, SectionCell{Cell: entry.cell, Alternate: alternates[entry.key], Row: offset + int(entry.key.offset), Section: section})
	}
}

func (s *Sections) addBody() {
	rows := s.indices("body")
	offset := len(s.Heights)
	for _, row := range rows {
		s.Heights = append(s.Heights, s.layout.RowHeights[row])
	}
	for _, entry := range s.shape(rows) {
		first := entry.key.offset == 0 && rec.FetchInt(entry.cell, "column") == 0
		last := entry.key.offset+rec.FetchInt(entry.cell, "rowspan") == int64(len(rows)) && rec.FetchInt(entry.cell, "column") == 0
		s.Cells = append(s.Cells, SectionCell{Cell: entry.cell, Row: offset + int(entry.key.offset), Section: "body", First: first, Last: last})
	}
	if len(rows) == 0 && (len(s.indices("repeat_head")) > 0 || len(s.indices("repeat_foot")) > 0) {
		rec.Fail("Repeated table sections need body location anchors")
	}
}
