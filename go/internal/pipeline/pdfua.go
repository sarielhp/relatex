package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/exportwriter"
	"github.com/sarielhp/relatex/internal/glyphowner"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/pdfua"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/texsandbox"
)

// exportTagged writes the tagged PDF (pdf-ua, decisions 097 and 105) when it is selected, after every
// other format: the ordinary PDF tagged from the model (route 2, pdfua2.go).
// Its findings join the conversion's diagnostics (also when the result comes from the stage cache);
// a tagging that fails is a diagnostic, never a failed conversion. The results keep the order of the
// selected formats.
func (c *conversion) exportTagged(writer *exportwriter.Writer, payload rec.Map, root string, results ojson.Object) ojson.Object {
	if !slices.Contains(c.options.Formats, "pdf-ua") {
		return results
	}
	c.webClaim = webClaim(root, c.options)
	writer.Tagged = c.tagged
	tagged := c.exportFormat(writer, payload, "pdf-ua", root)
	for _, found := range pdfua.Diagnostics(tagged) {
		var magnitude any
		if found.Magnitude > 0 {
			magnitude = found.Magnitude
		}
		c.diagnostics.Add(found.Code, found.Message, magnitude)
	}
	if status := rec.ToS(tagged.Get("status")); status != "conforming" {
		fmt.Fprintf(os.Stderr, "Tagged PDF: %s (see pdf-ua/export.json)\n", status)
	}
	ordered := ojson.Object{}
	for _, format := range c.options.Formats {
		switch {
		case ordered.Has(format):
		case format == "pdf-ua":
			ordered = append(ordered, ojson.Pair{format, tagged})
		default:
			ordered = append(ordered, ojson.Pair{format, results.Get(format)})
		}
	}
	return ordered
}

// taggedInputs are what the tagged PDF is made from besides the document: the sources, the ordinary
// PDF, the layout and the glyph owners.
func (c *conversion) taggedInputs() ojson.Object {
	return ojson.Object{{"source", c.archive.Manifest["zip_sha256"]}, {"main", c.archive.Manifest["main"]},
		{"ordinary_pdf", fileDigestOrNil(c.pdf)}, {"layout", c.layoutDigest()},
		{"glyph_owners", fileDigestOrNil(filepath.Join(c.exportRoot, glyphowner.File))}}
}

// taggedConfiguration names the programs the tagged PDF and its checks run, and the glyph spellings
// route 2 reads.
func (c *conversion) taggedConfiguration() ojson.Object {
	glyphs := filepath.Join("tex", "relatex-symbol-glyphs.txt")
	return ojson.Object{{"toolchain", texsandbox.Current(c.options.Engine).Fingerprint()}, {"qpdf", c.helper("qpdf")},
		{"verapdf", c.helper("verapdf")}, {"mutool", c.helper("mutool")},
		{"files", ojson.Object{{glyphs, fileDigestOrNil(filepath.Join(c.options.Root, glyphs))}}}}
}

func fileDigestOrNil(path string) any {
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		return nil
	}
	return command.FileDigest(path)
}
