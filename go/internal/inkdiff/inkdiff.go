// Package inkdiff compares two renderings of one picture by their ink (dark pixels), with decision
// 119's shift-tolerant rule (lib/relatex/shift_tolerant_diff.rb) and its constants, but without its
// glyph exemption: every differing region must be a near copy of the other rendering's ink, moved
// by at most ShiftPixels in each direction. Decision 137 applies it to pictures drawn as SVG with
// text (rendered at 144 dpi, twice the reference pages' 72, so each pixel constant is half the
// distance on paper it is there).
package inkdiff

import (
	"image"
	"image/color"
	"slices"
)

// The constants of decision 119 (ShiftTolerantDiff).
const (
	ShiftPixels   = 5
	growPixels    = 3
	minArea       = 4
	badShare      = 0.03
	minBadPixels  = 2
	edgePixels    = 1
	darkThreshold = 128
)

// Ink is a rendering reduced to its dark pixels.
type Ink struct {
	Width, Height int
	dark          []bool
}

// FromImage is an image's ink: a pixel is dark when its gray value, composited on white, is below
// half (decision 119's PGM threshold).
func FromImage(picture image.Image) Ink {
	bounds := picture.Bounds()
	ink := Ink{Width: bounds.Dx(), Height: bounds.Dy(), dark: make([]bool, bounds.Dx()*bounds.Dy())}
	for row := range ink.Height {
		for col := range ink.Width {
			ink.dark[(row*ink.Width)+col] = darkPixel(picture.At(bounds.Min.X+col, bounds.Min.Y+row))
		}
	}
	return ink
}

// darkPixel composites a pixel on white and compares its gray with the threshold.
func darkPixel(pixel color.Color) bool {
	r, g, b, a := pixel.RGBA()
	white := 0xffff - a
	gray := color.GrayModel.Convert(color.RGBA64{R: uint16(r + white), G: uint16(g + white), B: uint16(b + white),
		A: 0xffff}).(color.Gray)
	return gray.Y < darkThreshold
}

// Crop is the ink's top-left width by height pixels.
func (i Ink) Crop(width, height int) Ink {
	cropped := Ink{Width: width, Height: height, dark: make([]bool, width*height)}
	for row := range height {
		copy(cropped.dark[row*width:(row+1)*width], i.dark[row*i.Width:(row*i.Width)+width])
	}
	return cropped
}

// Count is the number of dark pixels.
func (i Ink) Count() int {
	count := 0
	for _, dark := range i.dark {
		if dark {
			count++
		}
	}
	return count
}

func (i Ink) at(col, row int) bool {
	return col >= 0 && row >= 0 && col < i.Width && row < i.Height && i.dark[(row*i.Width)+col]
}

// Box is a region's pixel box, inclusive: left, top, right, bottom.
type Box [4]int

// Changed is the boxes of the regions where the two renderings differ by more than a shift: decision
// 119's changed regions with no glyph box covering any. The renderings must be of one size.
func Changed(before, after Ink) []Box {
	if before.Width != after.Width || before.Height != after.Height {
		return []Box{{0, 0, max(before.Width, after.Width) - 1, max(before.Height, after.Height) - 1}}
	}
	d := &differ{old: before, new: after, nearOld: grow(before), nearNew: grow(after)}
	changed := []Box{}
	for _, box := range d.regions() {
		if !tiny(box) && !d.shifted(box) {
			changed = append(changed, box)
		}
	}
	return changed
}

type differ struct {
	old, new         Ink
	nearOld, nearNew Ink
}

func tiny(box Box) bool { return (box[2]-box[0]+1)*(box[3]-box[1]+1) < minArea }

// regions is the box of every connected region of differing pixels (connected within growPixels).
func (d *differ) regions() []Box {
	seen := make([]bool, len(d.old.dark))
	found := []Box{}
	for key := range d.old.dark {
		if seen[key] || d.old.dark[key] == d.new.dark[key] {
			continue
		}
		found = append(found, d.region(key, seen))
	}
	return found
}

func (d *differ) region(start int, seen []bool) Box {
	width := d.old.Width
	stack := []int{start}
	seen[start] = true
	box := Box{start % width, start / width, start % width, start / width}
	for len(stack) > 0 {
		key := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		col, row := key%width, key/width
		box = Box{min(box[0], col), min(box[1], row), max(box[2], col), max(box[3], row)}
		stack = d.neighbours(col, row, seen, stack)
	}
	return box
}

func (d *differ) neighbours(col, row int, seen []bool, stack []int) []int {
	for nrow := max(row-growPixels, 0); nrow <= min(row+growPixels, d.old.Height-1); nrow++ {
		for ncol := max(col-growPixels, 0); ncol <= min(col+growPixels, d.old.Width-1); ncol++ {
			key := (nrow * d.old.Width) + ncol
			if seen[key] || d.old.dark[key] == d.new.dark[key] {
				continue
			}
			seen[key] = true
			stack = append(stack, key)
		}
	}
	return stack
}

// shifted: some offset within ShiftPixels maps the region's old ink onto new ink almost exactly,
// nearest offsets first.
func (d *differ) shifted(box Box) bool {
	area := Box{box[0] - ShiftPixels, box[1] - ShiftPixels, box[2] + ShiftPixels, box[3] + ShiftPixels}
	offsets := [][2]int{}
	for dy := -ShiftPixels; dy <= ShiftPixels; dy++ {
		for dx := -ShiftPixels; dx <= ShiftPixels; dx++ {
			offsets = append(offsets, [2]int{dx, dy})
		}
	}
	slices.SortStableFunc(offsets, func(a, b [2]int) int { return manhattan(a) - manhattan(b) })
	return slices.ContainsFunc(offsets, func(offset [2]int) bool { return d.matches(area, offset[0], offset[1]) })
}

func manhattan(offset [2]int) int { return abs(offset[0]) + abs(offset[1]) }

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func (d *differ) matches(area Box, dcol, drow int) bool {
	total, bad := 0, 0
	areaPixels := float64((area[2] - area[0] + 1) * (area[3] - area[1] + 1))
	bound := max(float64(minBadPixels), areaPixels*badShare)
	for row := area[1]; row <= area[3]; row++ {
		for col := area[0]; col <= area[2]; col++ {
			inkOld, inkNew := d.old.at(col, row), d.new.at(col+dcol, row+drow)
			if inkOld || inkNew {
				total++
			}
			if (inkOld && !d.nearNew.at(col+dcol, row+drow)) || (inkNew && !d.nearOld.at(col, row)) {
				bad++
			}
		}
		if float64(bad) > bound {
			return false
		}
	}
	return total == 0 || float64(bad) <= max(float64(minBadPixels), float64(total)*badShare)
}

// grow is the ink widened by edgePixels in every direction.
func grow(ink Ink) Ink {
	grown := Ink{Width: ink.Width, Height: ink.Height, dark: slices.Clone(ink.dark)}
	for row := range ink.Height {
		for col := range ink.Width {
			if ink.dark[(row*ink.Width)+col] {
				mark(grown, col, row)
			}
		}
	}
	return grown
}

func mark(grown Ink, col, row int) {
	for nrow := max(row-edgePixels, 0); nrow <= min(row+edgePixels, grown.Height-1); nrow++ {
		for ncol := max(col-edgePixels, 0); ncol <= min(col+edgePixels, grown.Width-1); ncol++ {
			grown.dark[(nrow*grown.Width)+ncol] = true
		}
	}
}
