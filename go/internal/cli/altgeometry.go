package cli

import (
	"os"

	"github.com/sarielhp/clihelp"

	"github.com/sarielhp/relatex/internal/alttext"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/figdesc"
	"github.com/sarielhp/relatex/internal/figscan"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
)

// altGeometrySpec is relatex alt geometry: what one page of a figure PDF draws, from its embedded
// source (decision 123), printed as text or JSON. KEY names a figure of the last conversion and
// reads its own page and box, as alt draft sends it to the describer; --pdf reads a file's page.
func altGeometrySpec() commandSpec {
	pdf, output, page, asJSON := "", "", int64(1), false
	options := []clihelp.Option{
		textFlag("--pdf <file>", "The figure PDF", &pdf),
		integerFlag("--page <n>", "The page (default 1); an Ipe file's pages are its views, in order", func(value int64) { page = value }),
		textFlag("-o, --output <dir>", "With KEY: the conversion's product directory (default: the project cache's)", &output),
		switchFlag("--json", "Print the JSON form instead of the text form", func() { asJSON = true }),
	}
	return commandSpec{name: "geometry", description: "Print what a figure page draws: groups, paths, marks and labels",
		long:    "Prints what a figure page draws: groups, paths, marks and labels, coordinates 0-100 in the page box",
		usage:   Name + " alt geometry KEY [DIRECTORY] [--output DIR] [--json]\n       " + Name + " alt geometry --pdf FILE [--page N] [--json]",
		options: options, args: clihelp.MinimumNArgs(0),
		run: func(rest []string) ojson.Object { return altGeometry(rest, pdf, output, int(page), asJSON) }}
}

func altGeometry(rest []string, pdf, output string, page int, asJSON bool) ojson.Object {
	var figure *figscan.Figure
	switch {
	case pdf != "" && len(rest) == 0:
		figure = scanPDF(pdf, page)
	case pdf == "" && len(rest) >= 1:
		place := newAltPlace(rest[1:], output)
		figure = scanKey(rest[0], place.document(), place.output)
	default:
		fail("give a figure KEY, or --pdf FILE and no key")
	}
	if asJSON {
		panic(&exitRequest{stdout: string(figure.JSON())})
	}
	panic(&exitRequest{stdout: figure.Text()})
}

func scanPDF(pdf string, page int) *figscan.Figure {
	data, err := os.ReadFile(expandPath(pdf))
	if err != nil {
		panic(systemCall(err))
	}
	figure, err := figscan.Scan(data, page, nil)
	if err != nil {
		rec.Fail("%v", err)
	}
	return figure
}

// scanKey is the geometry of the last conversion's figure KEY, at its page and box.
func scanKey(key string, document rec.Map, output string) *figscan.Figure {
	temp := command.Scratch("alt-geometry-")
	defer os.RemoveAll(temp)
	for _, figure := range figdesc.Figures(document) {
		if figure.Key != key {
			continue
		}
		scanned, err := alttext.ScanFigure(document, figure, temp)
		if err != nil {
			rec.Fail("figure %s: %v", key, err)
		}
		if scanned == nil {
			rec.Fail("figure %s is a raster image: it has no geometry", key)
		}
		return scanned
	}
	rec.Fail("no figure %s in the last conversion (%s)", key, output)
	return nil
}
