// Package wcag is the WCAG colour arithmetic (decision 080, 102): sRGB colours read from #rrggbb,
// their relative luminance and the contrast ratio of two of them. The core's ink adjustment
// (contrast) and the outputs' palette checks share it.
package wcag

import (
	"math"
	"strconv"
)

// TextRatio is the minimum contrast of normal text (WCAG 1.4.3, AA).
const TextRatio = 4.5

// RGB is a colour as sRGB channels in [0, 1].
type RGB [3]float64

// Parse reads #rrggbb.
func Parse(hex string) (RGB, bool) {
	var color RGB
	if len(hex) != 7 || hex[0] != '#' {
		return color, false
	}
	for index := range color {
		value, err := strconv.ParseUint(hex[1+2*index:3+2*index], 16, 8)
		if err != nil {
			return color, false
		}
		color[index] = float64(value) / 255
	}
	return color, true
}

// Linear is an sRGB channel's linear light.
func Linear(channel float64) float64 {
	if channel <= 0.04045 {
		return channel / 12.92
	}
	return math.Pow((channel+0.055)/1.055, 2.4)
}

// Luminance is the WCAG relative luminance.
func (c RGB) Luminance() float64 {
	return float64(0.2126*Linear(c[0])) + float64(0.7152*Linear(c[1])) + float64(0.0722*Linear(c[2]))
}

// Ratio is the WCAG contrast ratio of two colours, at least 1.
func Ratio(a, b RGB) float64 {
	high, low := a.Luminance(), b.Luminance()
	if low > high {
		high, low = low, high
	}
	return (high + 0.05) / (low + 0.05)
}
