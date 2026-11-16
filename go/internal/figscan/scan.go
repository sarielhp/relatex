package figscan

import (
	"errors"
	"fmt"
	"math"
)

// Scan reads page (1-based) of a figure PDF. box is the region to normalize to, in PDF points;
// nil takes the page's CropBox (else MediaBox). A PDF with embedded Ipe source is read from it;
// any other is read from mutool's trace of the page (Source "trace", or "bitmap" for one image
// covering the box).
func Scan(pdf []byte, page int, box *Box) (*Figure, error) {
	if page < 1 {
		return nil, fmt.Errorf("figscan: page %d: pages count from 1", page)
	}
	file := parsePDF(pdf)
	source, err := file.ipeSource()
	if err != nil {
		// A damaged embedded source (a truncated stream, a wrong /Length) is a loss, not a stop:
		// the page itself is still drawn, so read that.
		figure, traceErr := scanTrace(pdf, file, page, box)
		if traceErr != nil {
			return nil, fmt.Errorf("figscan: embedded Ipe source: %w; trace: %v", err, traceErr)
		}
		figure.Detail += "; embedded Ipe source unreadable: " + err.Error()
		return figure, nil
	}
	if source == nil {
		return scanTrace(pdf, file, page, box)
	}
	boxes := file.pageBoxes()
	if box == nil {
		if page > len(boxes) {
			return nil, fmt.Errorf("figscan: page %d: the PDF has %d pages", page, len(boxes))
		}
		box = &boxes[page-1]
	}
	if box.Width() <= 0 || box.Height() <= 0 {
		return nil, errors.New("figscan: empty box")
	}
	figure, err := readIpe(source, page, len(boxes), *box)
	if err != nil {
		return nil, fmt.Errorf("figscan: %w", err)
	}
	printedLabels(pdf, file, page, figure, *box)
	return finished(figure, *box), nil
}

// printedLabels attaches to each Ipe label the printed text the page shows at its place, read from
// the trace of the same page and box; a failed trace leaves every label marked "source".
func printedLabels(pdf []byte, file *pdfFile, page int, figure *Figure, box Box) {
	traced, err := traceFigure(pdf, file, page, &box)
	if err != nil {
		attachPrinted(figure.Nodes, nil, box.scale())
		figure.Detail += "; printed labels unread: " + err.Error()
		return
	}
	attachPrinted(figure.Nodes, traced.Nodes, box.scale())
}

// scanTrace reads a page from mutool's trace. The page's box comes from the PDF's page tree when
// it can be read (object streams hide it) and the page is not rotated, else from the trace with
// its origin at 0 0.
func scanTrace(pdf []byte, file *pdfFile, page int, box *Box) (*Figure, error) {
	figure, err := traceFigure(pdf, file, page, box)
	if err != nil {
		return nil, err
	}
	return finished(figure, figure.box), nil
}

// traceFigure reads a traced page, not yet ranked.
func traceFigure(pdf []byte, file *pdfFile, page int, box *Box) (*Figure, error) {
	traced, err := tracePage(pdf, page)
	if err != nil {
		return nil, fmt.Errorf("figscan: %w", err)
	}
	bounds, err := traceBounds(traced)
	if err != nil {
		return nil, fmt.Errorf("figscan: %w", err)
	}
	crop := Box{X1: bounds.Width(), Y1: bounds.Height()}
	if boxes := file.pageBoxes(); page <= len(boxes) && sameSize(boxes[page-1], crop) {
		crop = boxes[page-1]
	}
	if box == nil {
		box = &crop
	}
	if box.Width() <= 0 || box.Height() <= 0 {
		return nil, errors.New("figscan: empty box")
	}
	return readTrace(traced, page, crop, bounds, *box), nil
}

// sameSize reports whether two boxes have the same size: a rotated page's box does not match
// mutool's device space, which shows the page turned, and then the device space is the box.
func sameSize(a, b Box) bool {
	return math.Abs(a.Width()-b.Width()) < 0.5 && math.Abs(a.Height()-b.Height()) < 0.5
}

func finished(figure *Figure, box Box) *Figure {
	figure.Size = [2]float64{round1(box.Width()), round1(box.Height())}
	figure.Extent = [2]float64{round1(box.Width() * box.scale()), round1(box.Height() * box.scale())}
	finish(figure)
	if figure.Nodes == nil {
		figure.Nodes = []*Node{}
	}
	return figure
}
