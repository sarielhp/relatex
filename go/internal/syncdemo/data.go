// Package syncdemo writes the click-to-source demo of word-level SyncTeX: one self-contained
// directory with the paper's HTML and its LaTeX source side by side, a click on a word of either
// finding the other (decision 192). It reads a finished product (document.json.gz's source map and
// glyph-owners.json.gz) and never changes what the HTML export writes.
package syncdemo

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/sarielhp/relatex/internal/glyphowner"
)

// DocumentFile is the product's document bundle.
const DocumentFile = "document.json.gz"

// Origin is a source range a typeset word came from: Start and End are character offsets in the
// line (0-based, End exclusive), -1 when the map knows the line only.
type Origin struct {
	File       int
	Line       int
	Start, End int
}

// Word is a typeset word of the PDF with its normalized text and its source ranges.
type Word struct {
	Norm    string
	Origins []Origin
}

// Source is a source file the words point into.
type Source struct {
	Name string
	Text string
}

// Data is what the demo reads of a product: the source files (numbered in the order the words
// first use them, the main file first) and each node's words in shipout order.
type Data struct {
	Sources []Source
	Leaves  map[string][]Word
}

type rawOrigin struct {
	File       string `json:"file"`
	Line       int    `json:"line"`
	Characters []int  `json:"characters"`
}

type rawWord struct {
	ID         string      `json:"id"`
	Text       string      `json:"text"`
	Normalized string      `json:"normalized_text"`
	Origins    []rawOrigin `json:"origins"`
}

type rawLogical struct {
	Normalized string      `json:"normalized_text"`
	Origins    []rawOrigin `json:"origins"`
	WordIDs    []string    `json:"word_ids"`
}

type rawSource struct {
	Text string `json:"text"`
}

type rawSourceMap struct {
	Words        []rawWord            `json:"words"`
	LogicalWords []rawLogical         `json:"logical_words"`
	Sources      map[string]rawSource `json:"sources"`
}

const needsProduct = " (the demo needs the full product: convert with --product DIR)"

// Load reads the product directory: the source map of document.json.gz and the glyph owners.
func Load(product string) (*Data, error) {
	sourceMap, err := readSourceMap(filepath.Join(product, DocumentFile))
	if err != nil {
		return nil, err
	}
	owners, err := glyphowner.Read(product)
	if err != nil {
		return nil, fmt.Errorf("%w%s", err, needsProduct)
	}
	return build(sourceMap, owners), nil
}

func readSourceMap(path string) (*rawSourceMap, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%w%s", err, needsProduct)
	}
	defer file.Close()
	reader, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var bundle struct {
		Payload struct {
			SourceMap *rawSourceMap `json:"source_map"`
		} `json:"payload"`
	}
	if err := json.NewDecoder(reader).Decode(&bundle); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if bundle.Payload.SourceMap == nil {
		return nil, fmt.Errorf("%s has no source map: the conversion left out its synchronization", path)
	}
	return bundle.Payload.SourceMap, nil
}

// build joins the source map's words to the owners: each hyphenated word is one logical word, and a
// word belongs to the node owning its first glyph (the word's id is gPAGE-GLYPH).
func build(sourceMap *rawSourceMap, owners *glyphowner.Owners) *Data {
	files := fileIndex{numbers: map[string]int{}}
	merged := map[string]rawLogical{}
	absorbed := map[string]bool{}
	for _, logical := range sourceMap.LogicalWords {
		if len(logical.WordIDs) == 0 {
			continue
		}
		merged[logical.WordIDs[0]] = logical
		for _, id := range logical.WordIDs[1:] {
			absorbed[id] = true
		}
	}
	data := &Data{Leaves: map[string][]Word{}}
	for _, word := range sourceMap.Words {
		if absorbed[word.ID] {
			continue
		}
		origins, text := word.Origins, word.Text
		if logical, ok := merged[word.ID]; ok {
			origins, text = logical.Origins, logical.Normalized
		}
		if leaf := ownerOf(owners, word.ID); leaf != "" {
			data.Leaves[leaf] = append(data.Leaves[leaf], Word{Norm: Normalize(text), Origins: files.convert(origins)})
		}
	}
	for _, name := range files.names {
		data.Sources = append(data.Sources, Source{Name: name, Text: sourceMap.Sources[name].Text})
	}
	return data
}

// ownerOf is the node owning the glyph a word id (gPAGE-GLYPH) names.
func ownerOf(owners *glyphowner.Owners, id string) string {
	var page, glyph int
	if n, err := fmt.Sscanf(id, "g%d-%d", &page, &glyph); n != 2 || err != nil {
		return ""
	}
	for _, candidate := range owners.Pages {
		if candidate.Number == page {
			return owners.Owner(candidate, glyph)
		}
	}
	return ""
}

// fileIndex numbers the source files in the order their words first use them.
type fileIndex struct {
	numbers map[string]int
	names   []string
}

func (f *fileIndex) convert(raw []rawOrigin) []Origin {
	out := make([]Origin, 0, len(raw))
	for _, origin := range raw {
		number, ok := f.numbers[origin.File]
		if !ok {
			number = len(f.names)
			f.numbers[origin.File] = number
			f.names = append(f.names, origin.File)
		}
		converted := Origin{File: number, Line: origin.Line, Start: -1, End: -1}
		if len(origin.Characters) == 2 {
			converted.Start, converted.End = origin.Characters[0], origin.Characters[1]
		}
		out = append(out, converted)
	}
	return out
}

// Normalize is a word as the alignment compares it: NFKC, letters and digits only, lower case.
func Normalize(text string) string {
	var out strings.Builder
	for _, r := range norm.NFKC.String(text) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			out.WriteRune(unicode.ToLower(r))
		}
	}
	return out.String()
}
