// Package glyphowner is the export's glyph owners (decision 105): every glyph TeX shipped, page by
// page in shipout order, with its baseline origin and the document node it belongs to — the
// paragraph, heading or caption its flow item went into, or the formula it is part of. The core
// computes it at reconciliation from the collector's glyph map (the owner, slot and formula of
// every glyph) and the reading; the tagged PDF's route 2 binds the ordinary PDF's glyphs to the
// structure by it. It is a model file beside document.json.gz, which stays structure only.
package glyphowner

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// File is the owners' file in the stage and in the export directory.
const File = "glyph-owners.json.gz"

// Version is its schema.
const Version = "glyph-owners-1"

// Owners are the glyph owners of a document.
type Owners struct {
	SchemaVersion string `json:"schema_version"`
	// Leaves are the owning nodes' ids; a page's Leaf entries index them from 1 (0: no owner).
	Leaves []string `json:"leaves"`
	Pages  []Page   `json:"pages"`
	// Pictures are the drawn pictures (TikZ, picture mode, xy-pic: no image object of their own),
	// each with the box the export cropped it from.
	Pictures []Picture `json:"pictures,omitempty"`
}

// Page is one page's glyphs in shipout order: baseline origins in bp from the top-left corner,
// their text, their owner and the glyphs whose position TeX does not know (inside a
// literal-drawn picture).
type Page struct {
	Number  int       `json:"number"`
	X       []float64 `json:"x"`
	Y       []float64 `json:"y"`
	Text    []string  `json:"text"`
	Leaf    []int     `json:"leaf"`
	Unknown []int     `json:"unknown,omitempty"`
}

// Picture is a drawn picture: its image node, its page and its box (left, top, right, bottom in bp
// from the top-left corner, as the glyphs are), whose painting is the image node's (decision 105, drawn pictures).
type Picture struct {
	Node string     `json:"node"`
	Page int        `json:"page"`
	Box  [4]float64 `json:"box"`
}

// Owner is the node id of a page's glyph, "" when it has none.
func (o *Owners) Owner(p Page, index int) string {
	if index < 0 || index >= len(p.Leaf) || p.Leaf[index] <= 0 || p.Leaf[index] > len(o.Leaves) {
		return ""
	}
	return o.Leaves[p.Leaf[index]-1]
}

// Validate checks the columns agree.
func (o *Owners) Validate() error {
	if o.SchemaVersion != Version {
		return fmt.Errorf("glyph owners: unsupported schema %q", o.SchemaVersion)
	}
	for _, p := range o.Pages {
		size := len(p.Leaf)
		if len(p.X) != size || len(p.Y) != size || len(p.Text) != size {
			return fmt.Errorf("glyph owners: page %d: columns differ in length", p.Number)
		}
		for _, index := range p.Unknown {
			if index < 0 || index >= size {
				return fmt.Errorf("glyph owners: page %d: unknown glyph %d out of range", p.Number, index)
			}
		}
	}
	for _, picture := range o.Pictures {
		if picture.Node == "" || picture.Box[0] > picture.Box[2] || picture.Box[1] > picture.Box[3] {
			return fmt.Errorf("glyph owners: picture %q on page %d: no node or an inverted box", picture.Node, picture.Page)
		}
	}
	return nil
}

// Write writes the owners, gzipped, to path.
func Write(path string, owners *Owners) (err error) {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
	}()
	writer := gzip.NewWriter(file)
	if err := json.NewEncoder(writer).Encode(owners); err != nil {
		return err
	}
	return writer.Close()
}

// Read reads the owners of an export directory; an error satisfying os.IsNotExist when it has none.
func Read(directory string) (*Owners, error) {
	file, err := os.Open(filepath.Join(directory, File))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", File, err)
	}
	owners := &Owners{}
	if err := json.NewDecoder(reader).Decode(owners); err != nil {
		return nil, fmt.Errorf("%s: %w", File, err)
	}
	return owners, owners.Validate()
}
