package pdftag

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/sarielhp/relatex/internal/command"
)

// The checks of the written file (decision 105 item 5): its structure tree read back in order
// with each element's marked content, compared with the model's text of the same element, and
// its pages rasterised beside the ordinary PDF's.

// OrderCheck compares the tagged PDF's structure, read back, with the model.
type OrderCheck struct {
	Elements   int      `json:"elements"`
	TextLeaves int      `json:"text_leaves"`
	TextEqual  int      `json:"text_equal"`
	Mismatches []string `json:"mismatches,omitempty"`
	// Normalized counts the leaves equal only after a normalization (textcheck.go), by kind.
	Normalized map[string]int `json:"normalized,omitempty"`
	Failure    string         `json:"failure,omitempty"`
}

// AppearanceCheck compares the pages of both PDFs as images.
type AppearanceCheck struct {
	Pages     int    `json:"pages"`
	Identical int    `json:"identical"`
	Differing []int  `json:"differing,omitempty"`
	Failure   string `json:"failure,omitempty"`
}

// readBack is an element of the written tree: its role and its marked content's text.
type readBack struct {
	role string
	text readText
}

// checkOrder reads the written structure tree back and compares each element's text.
func (t *tagger) checkOrder(root *selem) OrderCheck {
	file, err := readPDF(t.file.qpdf, t.options.Target)
	if err != nil {
		return OrderCheck{Failure: err.Error()}
	}
	texts, err := t.mcidTexts(file)
	if err != nil {
		return OrderCheck{Failure: err.Error()}
	}
	pageIndex := map[string]int{}
	for index, p := range file.pages() {
		pageIndex[p.ref] = index + 1
	}
	reader := &treeReader{file: file, texts: texts, pageIndex: pageIndex}
	tree := file.dict(file.dict(file.root())["/StructTreeRoot"])
	reader.walk(tree["/K"], 0)
	var expected []*selem
	collectLeaves(root, &expected)
	return t.compare(reader.found, expected)
}

// compare pairs the read elements with the model's, in order.
func (t *tagger) compare(found []readBack, expected []*selem) OrderCheck {
	check := OrderCheck{Elements: len(found)}
	if len(found) != len(expected) {
		check.Failure = fmt.Sprintf("%d elements with content read back, %d written", len(found), len(expected))
		return check
	}
	for index, element := range expected {
		if found[index].role != element.role {
			check.Failure = fmt.Sprintf("element %d is %s, written as %s", index, found[index].role, element.role)
			return check
		}
		if !textRoles[element.role] {
			continue
		}
		check.TextLeaves++
		want := t.leafText[element.leaf]
		kind := matchKind(found[index].text, want, t.report.Fonts.Codes+t.report.Fonts.Replaced > 0)
		switch kind {
		case "":
			if len(check.Mismatches) < 20 {
				check.Mismatches = append(check.Mismatches, fmt.Sprintf("%s %q: read %q", element.role, want.text, found[index].text.plain))
			}
			continue
		case "exact":
		default:
			if check.Normalized == nil {
				check.Normalized = map[string]int{}
			}
			check.Normalized[kind]++
		}
		check.TextEqual++
	}
	return check
}

// collectLeaves lists the elements with marked content in tree order. A Link is not one: its
// marked content is read as part of its host's text (decision 121 item 3). An element whose text
// is all in its Link kids has no MCID of its own and is listed all the same.
func collectLeaves(element *selem, into *[]*selem) {
	if element.role != "Link" && holdsText(element) {
		*into = append(*into, element)
	}
	for _, kid := range element.kids {
		collectLeaves(kid, into)
	}
}

// holdsText reports whether an element has marked content of its own or through its Link kids.
func holdsText(element *selem) bool {
	if len(element.mcrs) > 0 {
		return true
	}
	for _, kid := range element.kids {
		if kid.role == "Link" && holdsText(kid) {
			return true
		}
	}
	return false
}

// treeReader walks a written structure tree.
type treeReader struct {
	file      *pdfFile
	texts     map[int]map[int]readText
	pageIndex map[string]int
	found     []readBack
}

// walk collects the elements with marked content. A text leaf's text is its own marked content and
// its Link kids' in /K order (the host's MCIDs and its links interleaved as written); a Link is not
// an element of its own, and Formula and Figure kids stay out of their host's text.
func (r *treeReader) walk(value any, depth int) {
	if depth > 200 {
		return
	}
	if list, ok := r.file.resolve(value).([]any); ok {
		for _, kid := range list {
			r.walk(kid, depth+1)
		}
		return
	}
	dict := r.file.dict(value)
	if dict == nil || dict["/Type"] == "/MCR" || dict["/Type"] == "/OBJR" {
		return
	}
	kids := r.file.array(dict["/K"])
	if kids == nil && dict["/K"] != nil {
		kids = []any{dict["/K"]}
	}
	role, _ := dict["/S"].(string)
	text, marked := r.content(kids, depth)
	if marked && role != "/Link" && role != "" {
		r.found = append(r.found, readBack{role: role[1:], text: text})
	}
	for _, kid := range kids {
		r.walk(kid, depth+1)
	}
}

// content is an element's own marked content's text and its Link kids', in /K order; depth
// bounds a Link nested in a Link as walk bounds the tree.
func (r *treeReader) content(kids []any, depth int) (readText, bool) {
	var text readText
	marked := false
	for _, kid := range kids {
		dict := r.file.dict(kid)
		switch {
		case dict == nil, dict["/Type"] == "/OBJR":
		case dict["/Type"] == "/MCR":
			page := r.pageIndex[fmt.Sprint(dict["/Pg"])]
			text.add(r.texts[page][int(numberOf(dict["/MCID"]))])
			marked = true
		case dict["/S"] == "/Link" && depth < 200:
			links := r.file.array(dict["/K"])
			if links == nil && dict["/K"] != nil {
				links = []any{dict["/K"]}
			}
			if link, ok := r.content(links, depth+1); ok {
				text.add(link)
				marked = true
			}
		}
	}
	return text, marked
}

// mcidTexts is the normalized text of every MCID of every page, from the layout's glyphs.
func (t *tagger) mcidTexts(file *pdfFile) (map[int]map[int]readText, error) {
	texts := map[int]map[int]readText{}
	for index, p := range file.pages() {
		number := index + 1
		data, err := file.contentData(p)
		if err != nil {
			return nil, err
		}
		operations, err := parseContent(data)
		if err != nil {
			return nil, fmt.Errorf("page %d: %w", number, err)
		}
		items, err := newInterpreter(file, p.resources).run(operations)
		if err != nil {
			return nil, fmt.Errorf("page %d: %w", number, err)
		}
		var mapping []int
		if prepared := t.prepared[index]; prepared.mapper != nil && prepared.failure == "" {
			mapping = prepared.mapper.mapping
		}
		texts[number] = pageTexts(items, t.painted[number], mapping)
	}
	return texts, nil
}

// pageTexts reads each MCID's text as a reader does: through the glyphs' /ActualText where the
// tagged PDF writes one (readGlyphs, decision 148), else as the layout spells them.
func pageTexts(items []item, glyphs []paintedGlyph, mapping []int) map[int]readText {
	glyphs = readGlyphs(items, glyphs, mapping)
	indices := map[int][]int{}
	for _, entry := range items {
		if entry.mcid < 0 || (entry.kind != kindShow && entry.kind != kindObject) {
			continue
		}
		for ordinal := entry.first; ordinal < entry.first+entry.count && ordinal < len(mapping); ordinal++ {
			if index := mapping[ordinal]; index >= 0 {
				indices[entry.mcid] = append(indices[entry.mcid], index)
			}
		}
	}
	roles := newGlyphRoles(glyphs)
	texts := map[int]readText{}
	for mcid, list := range indices {
		texts[mcid] = glyphText(glyphs, roles, list)
	}
	return texts
}

// checkAppearance rasterises both PDFs with mutool and compares every page's pixels.
func checkAppearance(source, target string, pages int) AppearanceCheck {
	check := AppearanceCheck{Pages: pages}
	mutool := command.Resolve("mutool")
	if mutool == "" {
		check.Failure = "mutool not found"
		return check
	}
	scratch, err := os.MkdirTemp(filepath.Dir(target), "appearance-")
	if err != nil {
		check.Failure = err.Error()
		return check
	}
	defer os.RemoveAll(scratch)
	for _, side := range []struct{ name, pdf string }{{"a", source}, {"b", target}} {
		pattern := filepath.Join(scratch, side.name+"-%d.pnm")
		output, err := exec.Command(mutool, command.MutoolDrawArgs("-r", "50", "-o", pattern, side.pdf)...).CombinedOutput()
		// Content MuPDF cannot read (papers3/0786's shading) is left out of both PDFs' pages alike.
		var exit *exec.ExitError
		if _, recovered := command.Recovered(output); err != nil && !(errors.As(err, &exit) && exit.ExitCode() == 1 && recovered) {
			check.Failure = fmt.Sprintf("mutool draw: %v: %s", err, bytes.TrimSpace(output))
			return check
		}
	}
	for number := 1; number <= pages; number++ {
		a, errA := os.ReadFile(filepath.Join(scratch, fmt.Sprintf("a-%d.pnm", number)))
		b, errB := os.ReadFile(filepath.Join(scratch, fmt.Sprintf("b-%d.pnm", number)))
		if errA == nil && errB == nil && bytes.Equal(a, b) {
			check.Identical++
			continue
		}
		check.Differing = append(check.Differing, number)
	}
	return check
}

// withoutHyphens drops hyphens, which the reading removes at line breaks and the glyphs keep.
// sameText compares the text read back with the model's. The read text comes through the
// ordinary PDF's layout, where a glyph its font did not map reads U+FFFD even when the tagging
// spelled it (fonts.codes_added): there it stands for one character of the model's.
func sameText(read, want string, spelled bool) bool {
	if read == want {
		return true
	}
	got, expected := []rune(read), []rune(want)
	if !spelled || len(got) != len(expected) || !strings.ContainsRune(read, unicode.ReplacementChar) {
		return false
	}
	for index, r := range got {
		if r != expected[index] && r != unicode.ReplacementChar {
			return false
		}
	}
	return true
}

func withoutHyphens(text string) string {
	return strings.Map(func(r rune) rune {
		if isHyphen(r) {
			return -1
		}
		return r
	}, text)
}
