package paperdoc

import (
	"strings"

	"github.com/sarielhp/relatex/internal/inline"
	"github.com/sarielhp/relatex/internal/rec"
)

// Decorative images relatex infers (decision 111), conservatively: only marks that show nothing a
// description could say. An image the source or the author already describes keeps that, and the
// author's sidecar replaces the inference.
const (
	// invisibleSP: an image at most 2 pt on both sides shows nothing (an empty picture's crop).
	invisibleSP = 2 * 65_536
	// ornamentSP: a drawn picture at most 6 pt on both sides, repeated, is a mark such as a
	// theme's item bullet (beamer's ball bullets are pgf shadings).
	ornamentSP = 6 * 65_536
	// ornamentRepeats: how often the same drawn mark must recur.
	ornamentRepeats = 3
	// ruleThinSP: a drawn rule's shorter side is at most 2 bp. The source crops a drawn picture to whole
	// big points, so a 2 pt line is boxed at 2 bp (2.007 pt).
	ruleThinSP = 2 * 65_782
	// barThinSP: a drawn bar of a theme (a frame's title rule or progress bar) is at most 4 bp thick.
	barThinSP = 4 * 65_782
	// barLongSP: a bar is at least 100 pt long, so no inline sparkline or bullet is one.
	barLongSP = 100 * 65_536
	// barRepeats: how often a bar of one size must recur in the document.
	barRepeats = 3
	// rowToleranceSP: the marks of a row differ by at most 1.5 pt in each side (the navigation dot of the
	// current frame is drawn a point narrower than the others).
	rowToleranceSP = 98_304
	// ruleLongSP: its longer side is at least 20 pt; a shorter thin mark is a dot or a bullet.
	ruleLongSP = 20 * 65_536
	// Inferred is the provenance of a decorative mark relatex inferred.
	Inferred = "inferred"
)

// InferDecorative marks the document's undescribed invisible images, repeated tiny drawn marks
// and drawn rules decorative with provenance "inferred"; it returns how many it marked.
func InferDecorative(nodes rec.Map) int {
	images := []rec.Map{}
	for _, value := range nodes {
		content, _ := rec.AsMap(value)["content"].([]any)
		images = append(images, inline.OfType(content, "image")...)
	}
	repeats := map[string]int{}
	bars := map[[2]int64]int{}
	for _, image := range images {
		asset, _ := image["asset"].(string)
		repeats[asset]++
		if bar(image) {
			bars[barSize(image)]++
		}
	}
	rows := ornamentRows(nodes)
	marked := 0
	for _, image := range images {
		if !undescribed(image) {
			continue
		}
		asset, _ := image["asset"].(string)
		id, _ := image["id"].(string)
		if invisible(image) || rule(image) || bar(image) && bars[barSize(image)] >= barRepeats || ornament(image) && (repeats[asset] >= ornamentRepeats || rows[id]) {
			image["decorative"], image["alt_provenance"] = true, Inferred
			marked++
		}
	}
	return marked
}

// ornamentRows are the ids of the tiny drawn marks that stand in a row of at least ornamentRepeats marks
// of about one size (within rowToleranceSP), next to each other in a paragraph with at most spaces between
// them: a theme's navigation dots, where the current frame's dot is drawn once (so not repeated) and a point
// narrower, but sits among its neighbours.
func ornamentRows(nodes rec.Map) map[string]bool {
	rows := map[string]bool{}
	for _, value := range nodes {
		content, _ := rec.AsMap(value)["content"].([]any)
		run := []rec.Map{}
		flush := func() {
			if len(run) >= ornamentRepeats {
				for _, image := range run {
					id, _ := image["id"].(string)
					if id != "" {
						rows[id] = true
					}
				}
			}
			run = run[:0]
		}
		for _, item := range content {
			span, _ := item.(rec.Map)
			text, _ := span["text"].(string)
			switch {
			case span["type"] == "text" && strings.TrimSpace(text) == "":
			case span["type"] == "image" && ornament(span):
				if len(run) > 0 && !sameSize(run[0], span) {
					flush()
				}
				run = append(run, span)
			default:
				flush()
			}
		}
		flush()
	}
	return rows
}

// sameSize: the two marks' sides agree to within rowToleranceSP.
func sameSize(a, b rec.Map) bool {
	near := func(key string) bool {
		x, _ := a[key].(int64)
		y, _ := b[key].(int64)
		return x-y <= rowToleranceSP && y-x <= rowToleranceSP
	}
	return near("width_sp") && near("height_sp")
}

// undescribed: no alternative, decorative mark or link of its own (the ORCID logo is its link's).
func undescribed(image rec.Map) bool {
	for _, key := range []string{"alt", "decorative", "long_description", "external_link"} {
		if _, has := image[key]; has {
			return false
		}
	}
	return true
}

func within(image rec.Map, limit int64) bool {
	width, wide := image["width_sp"].(int64)
	height, high := image["height_sp"].(int64)
	return wide && high && width <= limit && height <= limit
}

func invisible(image rec.Map) bool { return within(image, invisibleSP) }

// ornament: a picture the source draws (rendered from the PDF), not a file the author included.
func ornament(image rec.Map) bool {
	return drawn(image) && within(image, ornamentSP)
}

// rule: a drawn line thinner than 2 pt and at least 20 pt long, which shows nothing a
// description could say. A short thin mark is a dot (invisible or a bullet), not a rule.
func rule(image rec.Map) bool {
	if !drawn(image) {
		return false
	}
	width, wide := image["width_sp"].(int64)
	height, high := image["height_sp"].(int64)
	if !wide || !high {
		return false
	}
	short, long := width, height
	if height < width {
		short, long = height, width
	}
	return short <= ruleThinSP && long >= ruleLongSP
}

// bar: a drawn line at most 4 bp thick and 100 pt long: a theme's frame rule or progress bar when it
// recurs at one size (barRepeats). A progress bar shows a fraction the page already gives as its
// heading and position.
func bar(image rec.Map) bool {
	if !drawn(image) {
		return false
	}
	width, _ := image["width_sp"].(int64)
	height, _ := image["height_sp"].(int64)
	short, long := min(width, height), max(width, height)
	return short > 0 && short <= barThinSP && long >= barLongSP
}

// barSize is the picture's width and height in sp rounded to tenths of a point, so that bars of one
// size share a key.
func barSize(image rec.Map) [2]int64 {
	const tenth = 6_554
	width, _ := image["width_sp"].(int64)
	height, _ := image["height_sp"].(int64)
	return [2]int64{(width + tenth/2) / tenth, (height + tenth/2) / tenth}
}

// drawn: the source rendered the picture from the PDF, rather than including a file.
func drawn(image rec.Map) bool {
	source, _ := image["source"].(string)
	return strings.HasPrefix(source, "rendered/")
}

// detailsField is fields plus an image's details (decision 125) when it has them: a text beside the
// long description it extends (a span without one keeps the key unexpected, so it fails).
func detailsField(span rec.Map, fields []string) []string {
	value, has := span["description_details"]
	if !has {
		return fields
	}
	if text, isText := value.(string); !isText || strings.TrimSpace(text) == "" {
		rec.Fail("Invalid figure description")
	}
	return append(fields, "description_details")
}
