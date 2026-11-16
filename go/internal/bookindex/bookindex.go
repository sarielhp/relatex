// Package bookindex is the linked index (decision 084): each \index entry an anchor in the
// paragraph that issued it, and each page number of the typeset index a link to the anchors of
// its term on that page. The collector reports where the entries' write nodes shipped
// (index_anchor), the .ind file theindex read (index_source), its entry commands (index_item)
// and the paragraphs typesetting each entry (index_item_owner); the printed index stays the
// paragraphs the reading made of it, its page numbers become links.
package bookindex

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

// Version names the document's index graph.
const Version = "index-1"

// Diagnostic is a loss the export reports: its code and message.
type Diagnostic struct{ Code, Message string }

var controlSpace = regexp.MustCompile(`(\\[A-Za-z@]+) +`)

// Comparable is a term or key level as compared: white space collapsed and no space after a
// control word (\write prints "\Term {ANN}" for \Term{ANN}).
func Comparable(text string) string {
	return controlSpace.ReplaceAllString(Normalize(text), "$1")
}

type anchor struct {
	id, block, page string
	span            int64
	path, key       string
	encap           string
	name            string
}

type source struct {
	file    string
	text    strings.Builder
	levels  []int64
	owners  map[int64][]int64
	offset  int64 // item numbers before this index
	missing bool
}

type projection struct {
	uid         func(scope string, key any) string
	blocks      map[int64][]rec.Map // reading blocks by flow owner, in order
	anchors     []anchor
	byPath      map[string][]anchor
	diagnostics []Diagnostic
}

// Project is the index graph of a paper document, nil when the paper issued no index entry and
// typeset no index. inspection holds the collector's records (diagnostics) and the reading's
// blocks; uid names a reading block's document node.
func Project(inspection rec.Map, uid func(scope string, key any) string) (rec.Map, []Diagnostic) {
	p := &projection{uid: uid, blocks: map[int64][]rec.Map{}, byPath: map[string][]anchor{}}
	for _, value := range rec.FetchList(rec.FetchMap(inspection, "reading"), "blocks") {
		block := rec.AsMap(value)
		if owner, ok := block["owner"].(int64); ok {
			p.blocks[owner] = append(p.blocks[owner], block)
		}
	}
	sources := p.read(rec.FetchList(inspection, "diagnostics"))
	if len(p.anchors) == 0 && len(sources) == 0 {
		return nil, nil
	}
	indexes := []any{}
	for _, item := range sources {
		indexes = append(indexes, p.index(item))
	}
	anchors := []any{}
	for _, a := range p.anchors {
		if a.block != "" {
			anchors = append(anchors, rec.Map{"id": a.id, "block": a.block, "span": a.span, "page": a.page, "key": a.key})
		}
	}
	return rec.Map{"schema_version": Version, "anchors": anchors, "indexes": indexes}, p.diagnostics
}

// read gathers the collector's index records in stream order.
func (p *projection) read(rows []any) []*source {
	sources := []*source{}
	var current *source
	items := int64(0)
	for _, value := range rows {
		row := rec.AsMap(value)
		switch row["code"] {
		case "index_anchor":
			p.addAnchor(row)
		case "index_source":
			part, _ := row["part"].(int64)
			if current == nil || part <= 1 {
				current = &source{file: rec.ToS(row["file"]), owners: map[int64][]int64{}, offset: items, missing: row["missing"] == true}
				sources = append(sources, current)
			}
			current.text.WriteString(rec.ToS(row["text"]))
		case "index_item":
			items++
			if current != nil {
				current.levels = append(current.levels, rec.AsInt(row["level"]))
			}
		case "index_item_owner":
			p.addOwner(sources, row)
		}
	}
	return sources
}

func (p *projection) addAnchor(row rec.Map) {
	path, encap := KeyPath(rec.ToS(row["key"]))
	compared := make([]string, len(path))
	for index, level := range path {
		compared[index] = Comparable(level)
	}
	a := anchor{id: fmt.Sprintf("idx-%d", rec.AsInt(row["anchor"])), page: rec.ToS(row["printed"]),
		path: strings.Join(compared, "\x00"), key: rec.ToS(row["key"]), encap: encap, name: rec.ToS(row["name"])}
	owner, _ := row["owner"].(int64)
	if block, span, ok := p.textSpan(owner); ok {
		a.block, a.span = block, span
	} else {
		p.diagnostics = append(p.diagnostics, Diagnostic{"index.anchor_unplaced",
			fmt.Sprintf("the index entry %q issued on page %s has no paragraph in the reading; its page links lead nowhere", rec.ToS(row["key"]), a.page)})
	}
	p.anchors = append(p.anchors, a)
	if a.block != "" {
		p.byPath[a.path] = append(p.byPath[a.path], a)
	}
}

// textSpan is the first text span of the paragraphs a flow owner made: an anchor sits inside it,
// so the block's children stay its spans.
func (p *projection) textSpan(owner int64) (string, int64, bool) {
	for _, block := range p.blocks[owner] {
		for index, value := range rec.FetchList(block, "content") {
			if rec.AsMap(value)["type"] == "text" {
				return p.uid("block", rec.Fetch(block, "id")), int64(index), true
			}
		}
	}
	return "", 0, false
}

// addOwner files a paragraph of an index item under the index that typeset it.
func (p *projection) addOwner(sources []*source, row rec.Map) {
	item, owner := rec.AsInt(row["item"]), rec.AsInt(row["owner"])
	for index := len(sources) - 1; index >= 0; index-- {
		if item > sources[index].offset {
			local := item - sources[index].offset
			if !slices.Contains(sources[index].owners[local], owner) {
				sources[index].owners[local] = append(sources[index].owners[local], owner)
			}
			return
		}
	}
}

// index is one typeset index: its entries aligned with the items the collector observed, each
// page number placed in its paragraph's text and linked to its term's anchors on that page.
func (p *projection) index(item *source) rec.Map {
	name := strings.TrimSuffix(filepath.Base(item.file), filepath.Ext(item.file))
	result := rec.Map{"source": filepath.Base(item.file), "status": "unaligned", "entries": []any{}}
	entries, ok := ParseInd(item.text.String())
	if item.missing || !ok || len(entries) != len(item.levels) {
		p.diagnostics = append(p.diagnostics, Diagnostic{"index.unstructured", fmt.Sprintf(
			"the typeset index (%s) does not read as its %d entries (%d observed); it prints as plain paragraphs without links",
			filepath.Base(item.file), len(entries), len(item.levels))})
		return result
	}
	for index, entry := range entries {
		if int64(entry.Level) != item.levels[index] {
			p.diagnostics = append(p.diagnostics, Diagnostic{"index.unstructured", fmt.Sprintf(
				"entry %d of the typeset index (%s) is set at level %d, the .ind says %d; the index prints without links",
				index+1, filepath.Base(item.file), item.levels[index], entry.Level)})
			return result
		}
	}
	result["status"] = "aligned"
	list := []any{}
	path := []string{}
	for index, entry := range entries {
		path = append(path[:min(len(path), entry.Level-1)], Comparable(entry.Term))
		list = append(list, p.entry(entry, strings.Join(path, "\x00"), item.owners[int64(index+1)], name))
	}
	result["entries"] = list
	return result
}
