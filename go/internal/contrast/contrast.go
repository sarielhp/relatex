// Package contrast measures and adjusts text colours against the reading page's backgrounds
// (WCAG 1.4.3, decision 080, plan accessibility §7.4 decision 1). A colour that reads too faintly
// on its background moves by the smallest change of its OKLCH lightness that reaches the
// required contrast ratio; its hue stays, and its chroma only shrinks where the lighter or
// darker colour would leave the sRGB gamut.
//
// It is in the model layer (decision 102): it reads and computes only over document.json. The
// core records the adjustments once (Field); the reading page and its check read them and
// the page's fills here.
package contrast

import (
	"fmt"
	"math"

	"github.com/sarielhp/relatex/internal/wcag"
)

// TextRatio is the minimum contrast of normal text (WCAG 1.4.3, AA; wcag.TextRatio).
const TextRatio = wcag.TextRatio

// RGB is a colour as sRGB channels in [0, 1].
type RGB [3]float64

// Parse reads #rrggbb (wcag.Parse).
func Parse(hex string) (RGB, bool) {
	color, ok := wcag.Parse(hex)
	return RGB(color), ok
}

// Hex is the colour as #rrggbb, each channel rounded to the nearest byte.
func (c RGB) Hex() string {
	bytes := c.bytes()
	return fmt.Sprintf("#%02x%02x%02x", bytes[0], bytes[1], bytes[2])
}

func (c RGB) bytes() [3]int {
	var bytes [3]int
	for index, value := range c {
		bytes[index] = int(math.Round(math.Max(0, math.Min(1, value)) * 255))
	}
	return bytes
}

// quantized is the colour as the page states it: whole bytes.
func (c RGB) quantized() RGB {
	var result RGB
	for index, value := range c.bytes() {
		result[index] = float64(value) / 255
	}
	return result
}

func linear(channel float64) float64 { return wcag.Linear(channel) }

func gamma(channel float64) float64 {
	if channel <= 0.0031308 {
		return 12.92 * channel
	}
	return float64(1.055*math.Pow(channel, 1/2.4)) - 0.055
}

// Luminance is the WCAG relative luminance (wcag).
func (c RGB) Luminance() float64 { return wcag.RGB(c).Luminance() }

// Ratio is the WCAG contrast ratio of two colours, at least 1 (wcag.Ratio).
func Ratio(a, b RGB) float64 { return wcag.Ratio(wcag.RGB(a), wcag.RGB(b)) }

// oklch is the colour's OKLCH lightness, chroma and hue (radians), after Björn Ottosson's OKLab.
func (c RGB) oklch() (lightness, chroma, hue float64) {
	r, g, b := linear(c[0]), linear(c[1]), linear(c[2])
	l := math.Cbrt(float64(0.4122214708*r) + float64(0.5363325363*g) + float64(0.0514459929*b))
	m := math.Cbrt(float64(0.2119034982*r) + float64(0.6806995451*g) + float64(0.1073969566*b))
	s := math.Cbrt(float64(0.0883024619*r) + float64(0.2817188376*g) + float64(0.6299787005*b))
	lightness = float64(0.2104542553*l) + float64(0.7936177850*m) - float64(0.0040720468*s)
	a := float64(1.9779984951*l) - float64(2.4285922050*m) + float64(0.4505937099*s)
	bb := float64(0.0259040371*l) + float64(0.7827717662*m) - float64(0.8086757660*s)
	return lightness, math.Hypot(a, bb), math.Atan2(bb, a)
}

// linearFromOKLCH is the colour's linear sRGB channels, possibly outside [0, 1].
func linearFromOKLCH(lightness, chroma, hue float64) [3]float64 {
	a, b := chroma*math.Cos(hue), chroma*math.Sin(hue)
	l := cube(lightness + float64(0.3963377774*a) + float64(0.2158037573*b))
	m := cube(lightness - float64(0.1055613458*a) - float64(0.0638541728*b))
	s := cube(lightness - float64(0.0894841775*a) - float64(1.2914855480*b))
	return [3]float64{
		float64(4.0767416621*l) - float64(3.3077115913*m) + float64(0.2309699292*s),
		float64(-1.2684380046*l) + float64(2.6097574011*m) - float64(0.3413193965*s),
		float64(-0.0041960863*l) - float64(0.7034186147*m) + float64(1.7076147010*s),
	}
}

func cube(x float64) float64 { return x * x * x }

const gamutTolerance = 1e-9

func inGamut(channels [3]float64) bool {
	for _, value := range channels {
		if value < -gamutTolerance || value > 1+gamutTolerance {
			return false
		}
	}
	return true
}

// fromOKLCH is the sRGB colour of that lightness and hue with the largest chroma up to chroma
// that the gamut holds.
func fromOKLCH(lightness, chroma, hue float64) RGB {
	if !inGamut(linearFromOKLCH(lightness, chroma, hue)) {
		low, high := 0.0, chroma
		for range 40 {
			middle := (low + high) / 2
			if inGamut(linearFromOKLCH(lightness, middle, hue)) {
				low = middle
			} else {
				high = middle
			}
		}
		chroma = low
	}
	channels := linearFromOKLCH(lightness, chroma, hue)
	var color RGB
	for index, value := range channels {
		color[index] = gamma(math.Max(0, math.Min(1, value)))
	}
	return color
}

// distinctStep is how far an adjusted colour moves on when its #rrggbb form is already taken.
const distinctStep = 0.002

// Distinct is adjusted, an adjustment of ink, moved on in the same direction by small lightness
// steps (ink's chroma and hue) until its #rrggbb form is not taken: a recorded adjustment must name
// one ink, so the checks can read it back.
func Distinct(adjusted, ink RGB, lighten bool, taken func(string) bool) RGB {
	lightness, _, _ := adjusted.oklch()
	_, chroma, hue := ink.oklch()
	step := -distinctStep
	if lighten {
		step = distinctStep
	}
	for range 500 {
		if !taken(adjusted.Hex()) {
			return adjusted
		}
		lightness = math.Max(0, math.Min(1, lightness+step))
		adjusted = fromOKLCH(lightness, chroma, hue).quantized()
	}
	return adjusted
}

// Worst is the lowest contrast of a colour on any of the backgrounds.
func Worst(color RGB, backgrounds []RGB) float64 {
	worst := math.Inf(1)
	for _, background := range backgrounds {
		worst = math.Min(worst, Ratio(color, background))
	}
	return worst
}

// Adjust is ink moved by the smallest OKLCH lightness change (darker when lighten is false) at
// which its stated #rrggbb form reaches minimum on every one of the backgrounds; ink itself when
// it already does. The search is a bisection between the ink's own lightness and black or white.
func Adjust(ink RGB, backgrounds []RGB, minimum float64, lighten bool) RGB {
	if Worst(ink, backgrounds) >= minimum {
		return ink
	}
	lightness, chroma, hue := ink.oklch()
	passes := func(value float64) bool {
		return Worst(fromOKLCH(value, chroma, hue).quantized(), backgrounds) >= minimum
	}
	low, high := lightness, 0.0
	if lighten {
		high = 1
	}
	if !passes(high) {
		return fromOKLCH(high, chroma, hue).quantized()
	}
	for range 50 {
		middle := (low + high) / 2
		if passes(middle) {
			high = middle
		} else {
			low = middle
		}
	}
	return fromOKLCH(high, chroma, hue).quantized()
}
