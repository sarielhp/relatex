package bookindex

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

const sampleInd = `\begin{theindex}

  \item \textbf{bold}, \hyperxindexformat{\textbf}{5}

  \indexspace

  \item graph, \hyperpage{1, 2}
    \subitem tree, \hyperpage{1}, \hyperpage{5}

  \indexspace

  \item net, \hyperpage{1--3}
  \item sample, \hyperpage{1},
		\hyperxindexformat{\see{random sample}}{3}
  \item 2-SAT, 12, iv

\end{theindex}
`

func TestParseInd(t *testing.T) {
	entries, ok := ParseInd(sampleInd)
	if !ok || len(entries) != 6 {
		t.Fatalf("entries %v ok %v", entries, ok)
	}
	want := []Entry{
		{1, `\textbf{bold}`, []Locator{{Kind: "page", Text: "5", Page: "5"}}},
		{1, "graph", []Locator{{Kind: "page", Text: "1", Page: "1"}, {Kind: "page", Text: "2", Page: "2"}}},
		{2, "tree", []Locator{{Kind: "page", Text: "1", Page: "1"}, {Kind: "page", Text: "5", Page: "5"}}},
		{1, "net", []Locator{{Kind: "page", Text: "1–3", Page: "1"}}},
		{1, "sample", []Locator{{Kind: "page", Text: "1", Page: "1"}, {Kind: "see", Target: "random sample"}}},
		{1, "2-SAT", []Locator{{Kind: "page", Text: "12", Page: "12"}, {Kind: "page", Text: "iv", Page: "iv"}}},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Fatalf("got %#v", entries)
	}
}

func TestKeyPath(t *testing.T) {
	cases := map[string][]string{
		`graph!tree|hyperpage`:        {"graph", "tree"},
		`epsilon-net@$\epsilon$-net`:  {`$\epsilon$-net`},
		`ANN@\Term {ANN}|see{approx}`: {`\Term {ANN}`},
		`a"!b!c`:                      {"a!b", "c"},
		`net|(`:                       {"net"},
		`x@y!z@w`:                     {"y", "w"},
	}
	for key, want := range cases {
		if got, _ := KeyPath(key); !reflect.DeepEqual(got, want) {
			t.Errorf("KeyPath(%q) = %q, want %q", key, got, want)
		}
	}
	if _, encap := KeyPath(`net|(hyperpage`); encap != "(hyperpage" {
		t.Errorf("encap %q", encap)
	}
	if Comparable(`\Term {ANN}`) != Comparable(`\Term{ANN}`) {
		t.Error("control word spacing differs")
	}
}

func block(id string, owner int64, texts ...string) rec.Map {
	content := []any{}
	for _, text := range texts {
		content = append(content, rec.Map{"type": "text", "text": text})
	}
	return rec.Map{"id": id, "owner": owner, "content": content}
}

func TestProject(t *testing.T) {
	ind := "\\begin{theindex}\n  \\item graph, \\hyperpage{1, 2}\n    \\subitem tree, 2\n  \\item net, 3\n\\end{theindex}\n"
	rows := []any{
		rec.Map{"code": "index_anchor", "anchor": int64(1), "key": "graph", "printed": "1", "owner": int64(1), "name": ""},
		rec.Map{"code": "index_anchor", "anchor": int64(2), "key": "graph!tree", "printed": "2", "owner": int64(2), "name": ""},
		rec.Map{"code": "index_anchor", "anchor": int64(3), "key": "graph", "printed": "2", "owner": int64(2), "name": ""},
		rec.Map{"code": "index_anchor", "anchor": int64(4), "key": "net", "printed": "4", "owner": int64(99), "name": ""},
		rec.Map{"code": "index_source", "file": "/x/book.ind", "part": int64(1), "parts": int64(1), "text": ind},
		rec.Map{"code": "index_item", "item": int64(1), "level": int64(1)},
		rec.Map{"code": "index_item", "item": int64(2), "level": int64(2)},
		rec.Map{"code": "index_item", "item": int64(3), "level": int64(1)},
		rec.Map{"code": "index_item_owner", "item": int64(1), "owner": int64(10)},
		rec.Map{"code": "index_item_owner", "item": int64(2), "owner": int64(11)},
		rec.Map{"code": "index_item_owner", "item": int64(3), "owner": int64(12)},
	}
	blocks := []any{block("b1", 1, "Graphs are here."), block("b2", 2, "Trees too."),
		block("b10", 10, "graph, 1, 2"), block("b11", 11, " tree, 2"), block("b12", 12, "net, 3")}
	inspection := rec.Map{"diagnostics": rows, "reading": rec.Map{"blocks": blocks}}
	uid := func(scope string, key any) string { return fmt.Sprintf("%s-%v", scope, key) }
	graph, losses := Project(inspection, uid)
	if graph == nil {
		t.Fatal("no graph")
	}
	if len(rec.FetchList(graph, "anchors")) != 3 {
		t.Fatalf("anchors %v", graph["anchors"])
	}
	codes := []string{}
	for _, loss := range losses {
		codes = append(codes, loss.Code)
	}
	if !reflect.DeepEqual(codes, []string{"index.anchor_unplaced", "index.page_unmatched"}) {
		t.Fatalf("losses %v", losses)
	}
	index := rec.AsMap(rec.FetchList(graph, "indexes")[0])
	if index["status"] != "aligned" {
		t.Fatalf("index %v", index)
	}
	entries := rec.FetchList(index, "entries")
	second := rec.AsMap(rec.FetchList(rec.AsMap(entries[0]), "locators")[1])
	if second["block"] != "block-b10" || second["start"] != int64(10) || !reflect.DeepEqual(second["anchors"], []any{"idx-3"}) {
		t.Fatalf("locator %v", second)
	}
	tree := rec.AsMap(rec.FetchList(rec.AsMap(entries[1]), "locators")[0])
	if !reflect.DeepEqual(tree["anchors"], []any{"idx-2"}) || tree["start"] != int64(7) {
		t.Fatalf("subentry %v", tree)
	}
	document := rec.Map{"index": graph, "nodes": rec.Map{}}
	for _, value := range blocks {
		b := rec.AsMap(value)
		document["nodes"].(rec.Map)[uid("block", b["id"])] = rec.Map{"type": "paragraph", "content": b["content"]}
	}
	Validate(document)
	if links := Links(document)["block-b10"]; len(links) != 2 {
		t.Fatalf("links %v", links)
	}
}

func TestProjectMisaligned(t *testing.T) {
	rows := []any{
		rec.Map{"code": "index_source", "file": "book.ind", "part": int64(1), "text": "\\item a, 1\n\\item b, 2\n"},
		rec.Map{"code": "index_item", "item": int64(1), "level": int64(1)},
	}
	graph, losses := Project(rec.Map{"diagnostics": rows, "reading": rec.Map{"blocks": []any{}}}, func(string, any) string { return "" })
	if len(losses) != 1 || losses[0].Code != "index.unstructured" || rec.AsMap(rec.FetchList(graph, "indexes")[0])["status"] != "unaligned" {
		t.Fatalf("graph %v losses %v", graph, losses)
	}
}

func TestRangeWithDashApart(t *testing.T) {
	rows := []any{
		rec.Map{"code": "index_anchor", "anchor": int64(1), "key": "net|(", "printed": "3", "owner": int64(1), "name": ""},
		rec.Map{"code": "index_source", "file": "b.ind", "part": int64(1), "text": "\\item net, \\hyperpage{3--5}\n"},
		rec.Map{"code": "index_item", "item": int64(1), "level": int64(1)},
		rec.Map{"code": "index_item_owner", "item": int64(1), "owner": int64(10)},
	}
	blocks := []any{block("b1", 1, "Nets."), block("b10", 10, "net, 3", "–", "5")}
	graph, losses := Project(rec.Map{"diagnostics": rows, "reading": rec.Map{"blocks": blocks}},
		func(scope string, key any) string { return fmt.Sprintf("%s-%v", scope, key) })
	entry := rec.AsMap(rec.FetchList(rec.AsMap(rec.FetchList(graph, "indexes")[0]), "entries")[0])
	locator := rec.AsMap(rec.FetchList(entry, "locators")[0])
	if len(losses) != 0 || locator["text"] != "3" || locator["start"] != int64(5) || locator["span"] != int64(0) {
		t.Fatalf("locator %v losses %v", locator, losses)
	}
}
