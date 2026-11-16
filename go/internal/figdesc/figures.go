package figdesc

import (
	"math"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/sarielhp/relatex/internal/figures"
	"github.com/sarielhp/relatex/internal/geometry"
	"github.com/sarielhp/relatex/internal/inline"
	"github.com/sarielhp/relatex/internal/mediamodel"
	"github.com/sarielhp/relatex/internal/rec"
)

// windowLimit bounds one citation window: the text around one reference to the figure.
const windowLimit = 1200

// citationHalf is the text taken on each side of a reference before cutting at sentence ends.
const citationHalf = 400

// citingLimit is how many windows a figure keeps: one per citing block, in reading order.
const citingLimit = 3

// contextLimit bounds the joined windows handed to a drafting command (RELATEX_CONTEXT).
const contextLimit = citingLimit * windowLimit

var (
	spaces            = regexp.MustCompile(`\s+`)
	beforePunctuation = regexp.MustCompile(` ([.,;:!?)])`)
)

// Figure is one figure file of the export and what describes it.
type Figure struct {
	Key    string // the sidecar's name for it: its file in figs/ without the extension
	File   string // the displayed file, relative to the export: figs/disk.svg
	SHA256 string // the bytes the author included (the original), else the displayed file's
	Page   int    // the page of that file the figure shows, 1-based; 1 when the placement names none
	// Box is the part of the page the figure shows: left, top, right, bottom in bp from its
	// top-left corner; nil when it is the page's own box.
	Box     []float64
	PageBox string // the page box Box is measured in: "mediabox", "cropbox", ...
	// Turns are the quarter turns, counterclockwise, the paper prints the figure turned by
	// (\includegraphics[angle=…], decision 123 amendment 3): 0 to 3; 0 also for any other angle.
	Turns   int
	IDs     []string // the image spans that show it
	Caption string   // the caption of its figure, when it has one
	Context string   // the windows around the references that cite it, else the paragraph before it
	// Citations are the text around each of the first references to the figure, in reading order
	// (decision 123): one window per citing block, the reference marked →…←.
	Citations []string
	// Places are where the source includes it, one per image span the conversion located (decision 115).
	Places []Place
	// Panel is where its first picture sits among the pictures of its figure (decision 133).
	Panel Panel
	span  rec.Map
	// document is the figure's document, and complexity its measure once taken (decision 110).
	document   rec.Map
	complexity *Complexity
}

// Identity is what identifies the picture for caching and draft reuse (decision 123): the original
// file's hash, then for a later page "#p<page>" and, when the figure shows part of it, its box. Page one
// is the file's hash alone, crop or not, so its entries keep the sha256 a sidecar written before
// decision 123 has (the sidecar format is unchanged).
func (f *Figure) Identity() string {
	if f.Page <= 1 {
		return f.SHA256
	}
	identity := f.SHA256 + "#p" + strconv.Itoa(f.Page)
	if len(f.Box) == 4 {
		identity += "#b" + f.PageBox
		for _, value := range f.Box {
			// A thousandth of a point: far below a pixel, and it keeps computation noise out of the name.
			identity += ":" + strconv.FormatFloat(math.Round(value*1000)/1000, 'f', -1, 64)
		}
	}
	return identity
}

// boxTolerance is how far a placement's viewport may differ from its page box and still be that box:
// the conversion records its round-off, a fraction of a thousandth of a point.
const boxTolerance = 0.01

// figurePage is the page a figure shows and the part of it, from the span's placement. The page is 1
// and the box nil when the placement is absent or its viewport is the page's own box.
func figurePage(span rec.Map) (page int, box []float64, pageBox string) {
	placement, ok := span["placement"].(rec.Map)
	if !ok {
		return 1, nil, ""
	}
	page = 1
	if number, ok := rec.Number(placement["page"]); ok && number >= 1 {
		page = int(number)
	}
	pageBox, _ = placement["pagebox"].(string)
	viewport, source := rec.ListOf(placement["viewport_bp"]), rec.ListOf(placement["source_box_bp"])
	if len(viewport) != 4 || len(source) != 4 {
		return page, nil, pageBox
	}
	if viewport, source := geometry.Box(viewport), geometry.Box(source); !sameBox(viewport, source) {
		return page, viewport, pageBox
	}
	return page, nil, pageBox
}

// quarterTurns are the counterclockwise quarter turns of an image span's placement: its rotate
// operations (graphicx's angle, \rotatebox) summed, when the sum is within a degree of a multiple
// of 90; 0 for none or any other angle.
func quarterTurns(span rec.Map) int {
	placement, _ := span["placement"].(rec.Map)
	degrees := 0.0
	for _, value := range rec.ListOf(placement["operations"]) {
		operation, _ := value.(rec.Map)
		if operation["kind"] != "rotate" {
			continue
		}
		cosine, isCosine := rec.Number(operation["cos"])
		sine, isSine := rec.Number(operation["sin"])
		if !isCosine || !isSine {
			return 0
		}
		degrees += float64(math.Atan2(sine, cosine) * 180 / math.Pi)
	}
	quarters := math.Round(degrees / 90)
	if math.Abs(degrees-float64(quarters*90)) > 1 {
		return 0
	}
	return ((int(quarters) % 4) + 4) % 4
}

func sameBox(first, second []float64) bool {
	for index := range first {
		if math.Abs(first[index]-second[index]) > boxTolerance {
			return false
		}
	}
	return true
}

// displayed is the figure's picture as the page shows it, with its MIME type.
func (f *Figure) displayed() ([]byte, string) {
	return figures.Displayed(f.document, f.span)
}

// Place is one inclusion of a figure: the source file and line the conversion recorded for its
// \includegraphics (the call of the author's macro that wraps it), and whether that image is described.
type Place struct {
	File      string
	Line      int
	Described bool
}

// occurrence is an image span and the block that holds it.
type occurrence struct {
	block string
	span  rec.Map
}

// Figures are the document's figure files in reading order. An image that is only a link's
// icon (the ORCID logo) is not a figure.
func Figures(document rec.Map) []*Figure {
	displayed, _ := figures.Plan(document)
	reader := newContextReader(document)
	byFile := map[string]*Figure{}
	listed := []*Figure{}
	for _, found := range reader.occurrences() {
		if _, linked := found.span["external_link"]; linked {
			continue
		}
		digest, _ := figures.Identity(document, found.span)
		file := displayed[digest]
		figure, seen := byFile[file]
		if !seen {
			figure = &Figure{File: file, SHA256: originalDigest(document, found.span, digest), span: found.span, document: document}
			figure.Page, figure.Box, figure.PageBox = figurePage(found.span)
			figure.Turns = quarterTurns(found.span)
			figure.Caption, figure.Citations = reader.describe(found.block)
			figure.Panel = reader.panel(found.block, found.span)
			figure.Context = clip(strings.Join(figure.Citations, "\n\n"))
			byFile[file] = figure
			listed = append(listed, figure)
		}
		figure.IDs = append(figure.IDs, rec.ToS(found.span["id"]))
		if file, ok := found.span["source_file"].(string); ok {
			line, _ := found.span["source_line"].(int64)
			figure.Places = append(figure.Places, Place{File: file, Line: int(line), Described: described(found.span)})
		}
	}
	nameFigures(listed)
	return listed
}

// nameFigures gives each figure its file name without the extension, or the whole file name
// when two files share a stem.
func nameFigures(figures []*Figure) {
	stems := map[string]int{}
	for _, figure := range figures {
		stems[stem(figure.File)]++
	}
	for _, figure := range figures {
		figure.Key = stem(figure.File)
		if stems[figure.Key] > 1 {
			figure.Key = path.Base(figure.File)
		}
	}
}

func stem(file string) string {
	base := path.Base(file)
	return strings.TrimSuffix(base, path.Ext(base))
}

// originalDigest is the SHA-256 of the file the author included, so a figure only scaled or
// clipped differently keeps its description; displayed when the asset keeps no original.
func originalDigest(document, span rec.Map, displayed string) string {
	asset := rec.AsMap(rec.FetchMap(rec.FetchMap(document, "assets"), "items")[rec.FetchString(span, "asset")])
	if asset["schema_version"] != mediamodel.MediaVersion {
		return displayed
	}
	original, ok := rec.FetchMap(asset, "representations")["original"].(rec.Map)
	if !ok {
		return displayed
	}
	if sha, ok := original["sha256"].(string); ok && sha != "" {
		return sha
	}
	return displayed
}

// Image is the figure's picture for a describer: the original the author included when it has
// one, else the displayed file; with its MIME type.
func (f *Figure) Image(document rec.Map) ([]byte, string) {
	asset := rec.AsMap(rec.FetchMap(rec.FetchMap(document, "assets"), "items")[rec.FetchString(f.span, "asset")])
	if asset["schema_version"] == mediamodel.MediaVersion {
		media := mediamodel.NewMediaAsset(asset)
		if original, ok := rec.FetchMap(asset, "representations")["original"].(rec.Map); ok {
			return media.Bytes(original), rec.FetchString(original, "mime_type")
		}
	}
	return figures.Displayed(document, f.span)
}

// contextReader finds a figure's caption and the text around it.
type contextReader struct {
	document rec.Map
	nodes    rec.Map
	parents  map[string]string
	order    []string
}

func newContextReader(document rec.Map) *contextReader {
	reader := &contextReader{document: document, nodes: rec.FetchMap(document, "nodes"), parents: map[string]string{}}
	for id, value := range reader.nodes {
		children, _ := rec.AsMap(value)["children"].([]any)
		for _, child := range children {
			if name, ok := child.(string); ok {
				reader.parents[name] = id
			}
		}
	}
	for _, value := range rec.FetchList(document, "block_order") {
		reader.order = append(reader.order, rec.ToS(value))
	}
	return reader
}

func (r *contextReader) occurrences() []occurrence {
	found := []occurrence{}
	for _, id := range r.order {
		for _, span := range inline.Flat(rec.FetchList(rec.Node(r.nodes, id), "content"), true) {
			if rec.Fetch(span, "type") == "image" {
				found = append(found, occurrence{block: id, span: span})
			}
		}
	}
	return found
}

// describe is the caption of the figure holding block and the windows around the references to
// it, else the text before it (decision 123).
func (r *contextReader) describe(block string) (caption string, citations []string) {
	float := r.enclosing(block, "figure_float")
	if float == "" {
		return "", []string{clipWindow(r.before(block, map[string]bool{block: true}))}
	}
	inside := r.descendants(float)
	if found := r.firstOfType(float, "float_caption", 0); found != "" {
		caption = r.text(found)
	}
	citations = r.windows(inside)
	if len(citations) == 0 {
		citations = append(citations, clipWindow(r.before(float, inside)))
	}
	return clip(caption), citations
}

// enclosing is the nearest node of this type at or above id, or "".
func (r *contextReader) enclosing(id, kind string) string {
	for seen := 0; id != "" && seen < 1000; seen++ {
		if rec.AsMap(r.nodes[id])["type"] == kind {
			return id
		}
		id = r.parents[id]
	}
	return ""
}

// firstOfType is the first node of this type below id, depth first in child order, or "".
func (r *contextReader) firstOfType(id, kind string, depth int) string {
	if depth > 50 {
		return ""
	}
	children, _ := rec.AsMap(r.nodes[id])["children"].([]any)
	for _, child := range children {
		name, _ := child.(string)
		if rec.AsMap(r.nodes[name])["type"] == kind {
			return name
		}
		if found := r.firstOfType(name, kind, depth+1); found != "" {
			return found
		}
	}
	return ""
}

func (r *contextReader) descendants(id string) map[string]bool {
	found := map[string]bool{id: true}
	pending := []string{id}
	for len(pending) > 0 {
		next := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		children, _ := rec.AsMap(r.nodes[next])["children"].([]any)
		for _, child := range children {
			if name, ok := child.(string); ok && !found[name] {
				found[name] = true
				pending = append(pending, name)
			}
		}
	}
	return found
}

// markOpen and markClose delimit a reference while a block's text is normalised.
const (
	markOpen  = '\u0000'
	markClose = '\u0001'
)

// citationBlock is one block of the reading order in the stream that citation windows are cut
// from, with the spans of the references to the figure in it.
type citationBlock struct {
	start, end int      // rune offsets in the stream
	marks      [][2]int // each reference's span in the stream
}

// windows is the text around each of the first citingLimit references to the figure, in reading
// order: one window per citing block, about citationHalf runes on each side of the reference, cut
// at sentence boundaries and never inside a formula.
func (r *contextReader) windows(inside map[string]bool) []string {
	stream, blocks := r.citationStream(inside)
	found := []string{}
	for _, block := range blocks {
		if len(block.marks) == 0 {
			continue
		}
		if len(found) == citingLimit {
			break
		}
		found = append(found, clipWindow(cutWindow(stream, blocks, block)))
	}
	return found
}

// citationStream joins the text of the blocks outside the figure in reading order and records
// where each block and each reference to the figure lies in it.
func (r *contextReader) citationStream(inside map[string]bool) ([]rune, []citationBlock) {
	stream := []rune{}
	blocks := []citationBlock{}
	for _, id := range r.order {
		if inside[id] {
			continue
		}
		clean, marks := normalizeRunes(r.markedText(id, inside))
		if len(clean) == 0 {
			continue
		}
		if len(stream) > 0 {
			stream = append(stream, '\n', '\n')
		}
		block := citationBlock{start: len(stream)}
		stream = append(stream, clean...)
		block.end = len(stream)
		for _, mark := range marks {
			block.marks = append(block.marks, [2]int{block.start + mark[0], block.start + mark[1]})
		}
		blocks = append(blocks, block)
	}
	return stream, blocks
}

// markedText is the plain text of a block with the references to targets delimited by markOpen and
// markClose; only the block's own content holds references, as the citation search did.
func (r *contextReader) markedText(id string, targets map[string]bool) []rune {
	node := rec.AsMap(r.nodes[id])
	out := []rune{}
	content, _ := node["content"].([]any)
	for _, value := range content {
		span := rec.AsMap(value)
		if target, ok := span["target"].(string); ok && span["type"] == "reference" && targets[target] {
			out = append(out, markOpen)
			out = append(out, r.rawSpan(span)...)
			out = append(out, markClose)
			continue
		}
		out = append(out, r.rawSpan(span)...)
	}
	for _, child := range rec.FetchList(node, "children") {
		name, ok := child.(string)
		if !ok || rec.AsMap(r.nodes[name])["type"] == "math" {
			continue
		}
		out = append(out, ' ')
		out = append(out, []rune(r.text(name))...)
	}
	return out
}

// normalizeRunes collapses whitespace, drops a space before punctuation and trims the ends, as
// text does, and records where the sentinel-delimited references lie in the result.
func normalizeRunes(raw []rune) ([]rune, [][2]int) {
	clean := []rune{}
	marks := [][2]int{}
	open, start, space := false, -1, false
	for _, char := range raw {
		switch char {
		case markOpen:
			open, start = true, -1
			continue
		case markClose:
			if open && start >= 0 {
				marks = append(marks, [2]int{start, len(clean)})
			}
			open = false
			continue
		}
		if unicode.IsSpace(char) {
			space = true
			continue
		}
		if space && len(clean) > 0 && !strings.ContainsRune(".,;:!?)", char) {
			clean = append(clean, ' ')
		}
		space = false
		if open && start < 0 {
			start = len(clean)
		}
		clean = append(clean, char)
	}
	return clean, marks
}

// cutWindow is the text around a block's first reference, widened to sentence boundaries and to
// formula ends, and at most windowLimit runes with the references marked →…←.
func cutWindow(stream []rune, blocks []citationBlock, block citationBlock) string {
	first := block.marks[0]
	start := max(first[0]-citationHalf, 0)
	end := min(first[1]+citationHalf, len(stream))
	start = sentenceStart(stream, blocks, start)
	end = sentenceEnd(stream, blocks, end)
	if open, _ := formulaAround(stream, start); open >= 0 && open < start {
		start = open
	}
	if _, shut := formulaAround(stream, end); shut >= end {
		end = shut + 1
	}
	if end-start > windowLimit {
		space := max(windowLimit-(first[1]-first[0]), 0)
		if left := first[0] - start; left > space/2 {
			start = first[0] - space/2
		}
		end = min(start+windowLimit, len(stream))
		if end < first[1] {
			end = first[1]
			start = max(end-windowLimit, 0)
		}
		start, end = outsideFormulas(stream, start, end)
	}
	return markWindow(stream, start, min(end, len(stream)), block.marks)
}

// outsideFormulas narrows a capped window so that neither end falls inside a $…$ formula.
func outsideFormulas(stream []rune, start, end int) (int, int) {
	if _, shut := formulaAround(stream, start); shut >= start {
		start = min(shut+1, end)
	}
	if open, _ := formulaAround(stream, end); open >= 0 && open < end {
		end = max(open, start)
	}
	return start, end
}

// sentenceStart is the start of the sentence at pos: the last sentence end at or before it, or a
// block's first rune.
func sentenceStart(stream []rune, blocks []citationBlock, pos int) int {
	start := 0
	for _, block := range blocks {
		if block.start > pos {
			break
		}
		start = max(start, block.start)
	}
	for i := 0; i < pos && i < len(stream); i++ {
		if !sentenceEndAt(stream, i) {
			continue
		}
		next := i + 1
		for next < len(stream) && unicode.IsSpace(stream[next]) {
			next++
		}
		if next <= pos {
			start = max(start, next)
		}
	}
	return start
}

// sentenceEnd is the end of the sentence at pos: the first sentence or block end at or after it.
func sentenceEnd(stream []rune, blocks []citationBlock, pos int) int {
	end := len(stream)
	for _, block := range blocks {
		if block.end >= pos {
			end = min(end, block.end)
		}
	}
	for i := pos; i < len(stream) && i < end; i++ {
		if sentenceEndAt(stream, i) {
			end = i + 1
			break
		}
	}
	return end
}

// sentenceEndAt tells whether a sentence ends after the rune at i: a full stop, question mark or
// exclamation mark followed by a space or a newline.
func sentenceEndAt(stream []rune, i int) bool {
	if !strings.ContainsRune("?!.", stream[i]) {
		return false
	}
	return i+1 == len(stream) || stream[i+1] == ' ' || stream[i+1] == '\n'
}

// formulaAround is the span of the $…$ formula whose content holds the cut at pos, or -1, -1.
func formulaAround(stream []rune, pos int) (int, int) {
	for i := 0; i < len(stream); i++ {
		if stream[i] != '$' || escaped(stream, i) {
			continue
		}
		shut := i + 1
		for shut < len(stream) && (stream[shut] != '$' || escaped(stream, shut)) {
			shut++
		}
		if shut >= len(stream) {
			return -1, -1
		}
		if i < pos && pos <= shut {
			return i, shut
		}
		i = shut
	}
	return -1, -1
}

// escaped tells whether the rune at i is escaped by a backslash.
func escaped(stream []rune, i int) bool {
	return i > 0 && stream[i-1] == '\\'
}

// markWindow is stream[start:end] with → and ← around the references that lie whole in it.
func markWindow(stream []rune, start, end int, marks [][2]int) string {
	opens, closes := map[int]bool{}, map[int]bool{}
	for _, mark := range marks {
		if mark[0] >= start && mark[1] <= end {
			opens[mark[0]], closes[mark[1]] = true, true
		}
	}
	var out strings.Builder
	for i := start; i < end; i++ {
		if opens[i] {
			out.WriteString("→")
		}
		out.WriteRune(stream[i])
		if closes[i+1] {
			out.WriteString("←")
		}
	}
	return strings.TrimSpace(out.String())
}

// before is the text of the last paragraph in reading order before id or the first block of
// skip (a float is not in the reading order; its blocks are).
func (r *contextReader) before(id string, skip map[string]bool) string {
	last := ""
	for _, block := range r.order {
		if block == id || skip[block] {
			return last
		}
		if rec.AsMap(r.nodes[block])["type"] == "paragraph" {
			last = r.text(block)
		}
	}
	return last
}

// text is the plain text of a node and the nodes below it: formulas as their TeX between $.
func (r *contextReader) text(id string) string {
	var out strings.Builder
	r.write(&out, id, 0)
	return strings.TrimSpace(beforePunctuation.ReplaceAllString(spaces.ReplaceAllString(out.String(), " "), "$1"))
}

func (r *contextReader) write(out *strings.Builder, id string, depth int) {
	node := rec.AsMap(r.nodes[id])
	if depth > 50 || node == nil {
		return
	}
	content, _ := node["content"].([]any)
	for _, value := range content {
		out.WriteString(string(r.rawSpan(rec.AsMap(value))))
	}
	children, _ := node["children"].([]any)
	for _, child := range children {
		if name, ok := child.(string); ok && rec.AsMap(r.nodes[name])["type"] != "math" {
			out.WriteString(" ")
			r.write(out, name, depth+1)
		}
	}
}

// rawSpan is one span as the context's plain text: a formula as its TeX between $.
func (r *contextReader) rawSpan(span rec.Map) []rune {
	switch span["type"] {
	case "math":
		tex := rec.AsMap(rec.AsMap(r.nodes[rec.ToS(span["node"])])["tex"])
		if text, ok := tex["text"].(string); ok && tex["status"] == "verified" {
			return []rune(" $" + text + "$ ")
		}
		return []rune(" [formula] ")
	default:
		if text, ok := span["text"].(string); ok {
			return []rune(text)
		}
	}
	return nil
}

func clip(text string) string {
	return clipTo(text, contextLimit)
}

// clipWindow bounds one citation window, which is shorter than the joined context.
func clipWindow(text string) string {
	return clipTo(text, windowLimit)
}

func clipTo(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + " …"
}
