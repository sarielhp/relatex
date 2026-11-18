// Package figures names the figure files an output writes: the LaTeX source the author
// included them from, numbered in reading order when the paper draws them itself, and made
// unique by the content each file holds (decision 068). Every output starts from the same
// stems, so a figure is called the same thing in the page, Markdown and Typst.
package figures

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/sarielhp/relatex/internal/inline"
	"github.com/sarielhp/relatex/internal/rec"
)

// Numbered is FigureNames::NUMBERED: the name a figure without a source name takes.
const Numbered = "fig"

// maxStem and minWord are FigureNames::MAX_STEM and MIN_WORD: a longer stem is cut at its last
// word separator (not before minWord), or at maxStem.
const (
	maxStem = 16
	minWord = 6
)

var (
	// contentName is FigureNames::CONTENT_NAME: a figure cropped from the paper is named by
	// its content, which is nobody's name.
	contentName = regexp.MustCompile(`^[0-9a-f]{32,}$`)
	unsafeStem  = regexp.MustCompile(`[^A-Za-z0-9._-]+`)
)

// Names is one output's naming state: the stem every asset's files share and the names its
// files have taken.
type Names struct {
	stems   map[string]string
	taken   map[string]string // file name, lower case, to the content that holds it
	named   map[string]string // file name, lower case, to the spelling its first claim gave it
	counter int
}

// Twin is two author files whose figure names differ only in case (decision 129, amendment 5):
// they share one exported name when their bytes are identical, and the second takes a number
// when they differ. First and Second are the source names as the paper writes them.
type Twin struct {
	First, Second, FirstAsset, SecondAsset string
	Identical                              bool // the two files hold the same bytes (one asset)
}

// New is FigureNames.new.
func New() *Names {
	return &Names{stems: map[string]string{}, taken: map[string]string{}, named: map[string]string{}}
}

// Occurrences is PaperImages.occurrences: the images of the reading order, rich cells in
// display order, QED marks' images included.
func Occurrences(document rec.Map) []rec.Map {
	nodes := rec.FetchMap(document, "nodes")
	images := []rec.Map{}
	for _, id := range rec.FetchList(document, "block_order") {
		for _, span := range inline.Flat(rec.FetchList(rec.Node(nodes, id), "content"), true) {
			if rec.Fetch(span, "type") == "image" {
				images = append(images, span)
			}
			// The image an author's QED mark shows (decision 176).
			if mark, ok := span["image"].(rec.Map); ok && span["type"] == "symbol" {
				images = append(images, mark)
			}
		}
	}
	return images
}

// SymbolImages are the images the document's symbols show: the author's QED marks (decision 176).
func SymbolImages(document rec.Map) []rec.Map {
	nodes := rec.FetchMap(document, "nodes")
	marks := []rec.Map{}
	for _, id := range rec.FetchList(document, "block_order") {
		for _, span := range inline.Flat(rec.FetchList(rec.Node(nodes, id), "content"), true) {
			if mark, ok := span["image"].(rec.Map); ok && span["type"] == "symbol" {
				marks = append(marks, mark)
			}
		}
	}
	return marks
}

// Stems is PaperImages.stems: every figure of the document named in reading order, the state
// each output starts from.
func Stems(document rec.Map) *Names {
	names := New()
	for _, span := range Occurrences(document) {
		names.Stem(rec.FetchString(span, "asset"), span)
	}
	return names
}

// Stem is the stem every file of a figure shares, assigned once per asset: its LaTeX source
// name, or the next figure number.
func (n *Names) Stem(asset string, span rec.Map) string {
	if stem, known := n.stems[asset]; known {
		return stem
	}
	stem, named := SourceStem(span)
	if !named {
		stem = n.nextNumber()
	}
	n.stems[asset] = stem
	return stem
}

// Claim is FigureNames#claim: the first free name for this content, the candidates in order
// and then the first numbered. Content already named in this output keeps its name. Names are
// compared without case, so no two files of figs/ differ only in case.
func (n *Names) Claim(digest string, candidates ...string) string {
	base := candidates[0]
	extension := path.Ext(base)
	stem := strings.TrimSuffix(base, extension)
	for counter := 1; ; counter++ {
		for _, candidate := range candidates {
			key := strings.ToLower(candidate)
			if held, found := n.taken[key]; !found || held == digest {
				n.taken[key] = digest
				if first, named := n.named[key]; named {
					return first
				}
				n.named[key] = candidate
				return candidate
			}
		}
		candidates = []string{fmt.Sprintf("%s-%d%s", stem, counter+1, extension)}
	}
}

// SourceStem is the figure's name as the author wrote it: the file it was included from,
// without its directory or extension. It reports false when the paper holds no such name.
func SourceStem(span rec.Map) (string, bool) {
	origin, ok := span["source"].(string)
	if !ok {
		return "", false
	}
	name := stemOf(origin)
	if name == "" || contentName.MatchString(name) {
		return "", false
	}
	return shorten(name), true
}

// SourceName is the name a figure included from origin (a file name as the source writes it, with
// or without its extension) takes when nothing else holds it: the sidecar key the lint suggests.
func SourceName(origin string) string {
	if !graphicExtension.MatchString(origin) {
		origin += ".pdf"
	}
	return shorten(stemOf(origin))
}

// graphicExtension is a file name ending in an extension \includegraphics finds by itself.
var graphicExtension = regexp.MustCompile(`(?i)\.(?:pdf|png|jpe?g|eps|ps|svg|jbig2|jb2|mps|gif|tiff?|webp)$`)

// shorten is FigureNames#shorten.
func shorten(name string) string {
	if len(name) <= maxStem {
		return name
	}
	head := name[:maxStem]
	if cut := strings.LastIndexAny(head, "-._"); cut >= minWord && !strings.ContainsRune("-._", rune(name[maxStem])) {
		head = head[:cut]
	}
	return strings.TrimRight(head, "-._")
}

// stemOf is File.basename(origin, '.*') then the slug the name is written under.
func stemOf(origin string) string {
	name := path.Base(origin)
	// File.basename(name, '.*') drops the last suffix, but keeps a leading dot.
	if index := strings.LastIndex(name[1:], "."); index >= 0 {
		name = name[:index+1]
	}
	name = unsafeStem.ReplaceAllString(name, "-")
	return strings.Trim(strings.TrimLeft(name, "-."), "-.")
}

func (n *Names) nextNumber() string {
	for {
		n.counter++
		candidate := fmt.Sprintf("%s-%d", Numbered, n.counter)
		if !n.inUse(candidate) {
			return candidate
		}
	}
}

// inUse is the numbering guard: a figure number no other figure already answers to.
func (n *Names) inUse(candidate string) bool {
	for _, stem := range n.stems {
		if strings.EqualFold(stem, candidate) {
			return true
		}
	}
	for name := range n.taken {
		if strings.EqualFold(path.Base(strings.TrimSuffix(name, path.Ext(name))), candidate) {
			return true
		}
	}
	return false
}
