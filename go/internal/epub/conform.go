package epub

import (
	"regexp"
	"strings"

	"github.com/sarielhp/relatex/internal/pagetree"
)

// Adjustments count the changes that make the page valid EPUB 3 content where the browser page
// relies on more than EPUB's schemas allow; each keeps what a reader sees.
type Adjustments struct {
	// IntentsAsData: MathML 4 intent and arg attributes, which the MathML 3 of EPUB 3 does not have
	// (epubcheck: RSC-005), kept as data-intent and data-arg, which it accepts (decision 109).
	// Reading systems speak the formula from its markup, as before; the author's meaning stays in the
	// package for a reading system or a later EPUB that reads MathML 4.
	IntentsAsData int
	// ColorsAsStyle: a MathML color the MathML 3 schema does not accept (currentColor), written as
	// the CSS it means.
	ColorsAsStyle int
	// FencesAsData: fence and symmetric on mo (MathML 3 core has neither; epubcheck: RSC-005), kept as
	// data-fence and data-symmetric.
	FencesAsData int
	// CaptionsAsDivisions: a figure caption between the figure's other blocks, where HTML allows a
	// caption only first or last, kept in place as a division of class figcaption.
	CaptionsAsDivisions int
}

// mathColor is a color value of MathML 3: #rgb, #rrggbb or one of the HTML 4 color names.
var mathColor = regexp.MustCompile(`(?i)^\s*(#[0-9a-f]{3}([0-9a-f]{3})?|aqua|black|blue|fuchsia|gray|green|lime|maroon|navy|olive|` +
	`purple|red|silver|teal|white|yellow|transparent)\s*$`)

// colorProperties are the CSS properties of MathML's color attributes.
var colorProperties = map[string]string{"mathbackground": "background-color", "mathcolor": "color", "background": "background-color",
	"color": "color"}

// conform adjusts the page's markup to the content models of EPUB 3 and counts what it changed.
func conform(root *pagetree.Node) Adjustments {
	adjustments := Adjustments{}
	root.Walk(func(n *pagetree.Node) {
		if n.Name == "math" {
			n.Walk(func(m *pagetree.Node) { conformMath(m, &adjustments) })
		}
		if n.Name == "figure" {
			conformFigure(n, &adjustments)
		}
		if class, _ := n.Attr("class"); n.Name == "div" && class == "display_math" {
			focusableDisplay(n)
		}
	})
	return adjustments
}

func conformMath(n *pagetree.Node, adjustments *Adjustments) {
	kept := n.Attrs[:0]
	style := []string{}
	for _, a := range n.Attrs {
		property, isColor := colorProperties[a.Name]
		switch {
		case a.Name == "intent" || a.Name == "arg":
			kept = append(kept, pagetree.Attribute{Name: "data-" + a.Name, Value: a.Value})
			if a.Name == "intent" {
				adjustments.IntentsAsData++
			}
		case a.Name == "fence" || a.Name == "symmetric":
			kept = append(kept, pagetree.Attribute{Name: "data-" + a.Name, Value: a.Value})
			adjustments.FencesAsData++
		case isColor && !mathColor.MatchString(a.Value):
			style = append(style, property+":"+a.Value)
			adjustments.ColorsAsStyle++
		default:
			kept = append(kept, a)
		}
	}
	n.Attrs = kept
	if len(style) == 0 {
		return
	}
	if existing, _ := n.Attr("style"); existing != "" {
		style = append([]string{strings.TrimSuffix(existing, ";")}, style...)
	}
	n.SetAttr("style", strings.Join(style, ";"))
}

// conformFigure keeps one caption of a figure as figcaption where HTML allows it, the figure's
// first or last element.
func conformFigure(figure *pagetree.Node, adjustments *Adjustments) {
	elements := []*pagetree.Node{}
	for _, child := range figure.Children {
		if child.Name != "" {
			elements = append(elements, child)
		}
	}
	kept := false
	for index, child := range elements {
		if child.Name != "figcaption" {
			continue
		}
		if !kept && (index == 0 || index == len(elements)-1) {
			kept = true
			continue
		}
		child.Name = "div"
		class, _ := child.Attr("class")
		child.SetAttr("class", strings.TrimSpace(class+" figcaption"))
		// The figure names its caption, so a screen reader announces it with the figure.
		// A figure the writer made a plain group names its caption at its table (decision 183).
		if id, ok := child.Attr("id"); ok {
			_, grouped := figure.Attr("role")
			if _, named := figure.Attr("aria-labelledby"); !named && !grouped {
				figure.SetAttr("aria-labelledby", id)
			}
		}
		adjustments.CaptionsAsDivisions++
	}
}

// focusableDisplay names a display formula as a scrollable region the keyboard reaches (WCAG
// 2.1.1, scrollable-region-focusable): "Equation (3)" by its number, else "Display formula", as
// preview.js names the ones that overflow in the browser. A display scrolls when it is wider than
// the page; the EPUB holds no script to find which, so every display is marked (decision 095).
func focusableDisplay(display *pagetree.Node) {
	name := "Display formula"
	for _, child := range display.Children {
		if kind, _ := child.Attr("data-kind"); kind == "text" {
			if number := strings.TrimSpace(child.TextContent()); number != "" {
				name = "Equation " + number
			}
		}
	}
	display.SetAttr("tabindex", "0")
	display.SetAttr("role", "group") // not a region: unnumbered displays would be many landmarks of one name
	display.SetAttr("aria-label", name+", scrollable")
}
