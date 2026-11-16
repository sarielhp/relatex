package htmlkit

import (
	"github.com/sarielhp/relatex/internal/figdesc"
	"github.com/sarielhp/relatex/internal/figures"
	"github.com/sarielhp/relatex/internal/fontmodel"
	"github.com/sarielhp/relatex/internal/langtag"
	"github.com/sarielhp/relatex/internal/rec"
	"slices"
	"strings"
)

// The reading page's names that the page writer (html), its check (browser) and the EPUB share:
// captions, image descriptions and sources, and font families (decision 102).

// CaptionTags are the elements of a float's captions: figcaption for the first caption that is the
// float's first or last child, where HTML allows one; div (class figcaption) for a caption between
// the float's other blocks, kept in place so the reading order holds. The figure names its first
// caption with aria-labelledby either way.
func CaptionTags(float rec.Map, nodes rec.Map) map[string]string {
	tags := map[string]string{}
	children := rec.FetchList(float, "children")
	kept := false
	for index, child := range children {
		if rec.Node(nodes, child)["type"] != "float_caption" {
			continue
		}
		if !kept && (index == 0 || index == len(children)-1) {
			kept = true
			tags[rec.ToS(child)] = "figcaption"
			continue
		}
		tags[rec.ToS(child)] = "div"
	}
	return tags
}

// LongDescription is an image's long description (decision 096), false when it has none.
func LongDescription(span rec.Map) (string, bool) {
	long, ok := span["long_description"].(string)
	return long, ok && strings.TrimSpace(long) != ""
}

// DescriptionID is the id of the element that holds an image's long description's text, which the
// image names with aria-describedby.
func DescriptionID(span rec.Map) string {
	return rec.ToS(rec.Fetch(span, "id")) + "-description"
}

// DetailsID is the id of the disclosure (<details>) that shows an image's long description on
// demand, which the image names with aria-details (decision 110).
func DetailsID(span rec.Map) string {
	return rec.ToS(rec.Fetch(span, "id")) + "-details"
}

// LongDescriptionSummary is the disclosure's visible label (decision 110).
const LongDescriptionSummary = "Description of the figure"

// LongDescriptionHTML is an image's long description as the WAI pattern for complex images has it
// (decision 110): a disclosure after the block that holds the image, closed until the reader opens
// it, its text the element aria-describedby names. The label is English, and says so on a page in
// another language.
func LongDescriptionHTML(document, span rec.Map, long string) string {
	lang := ""
	if !strings.HasPrefix(DocumentLanguage(document), "en") {
		lang = " lang=\"en\""
	}
	return "<details id=\"" + DetailsID(span) + "\" data-kind=\"long-description\" class=\"long-description\"><summary" + lang +
		">" + LongDescriptionSummary + "</summary><p id=\"" + DescriptionID(span) + "\">" + Escape(long) + "</p>" +
		moreHTML(span, lang) + "</details>"
}

// MoreSummary is the visible label of a figure's details (decision 125).
const MoreSummary = "More about this figure"

// moreHTML is a figure's details (decision 125), "" when it has none: a disclosure of its own inside
// the description's, after the short text, so the image's aria-details names both and its
// aria-describedby still names the short text only.
func moreHTML(span rec.Map, lang string) string {
	details, _ := span[figdesc.DetailsField].(string)
	if strings.TrimSpace(details) == "" {
		return ""
	}
	return "<details class=\"figure-details\"><summary" + lang + ">" + MoreSummary + "</summary><p>" + Escape(details) + "</p></details>"
}

// ImageSource is PaperImages.source: the relative path the HTML uses for an image.
func ImageSource(document, span rec.Map) string {
	displayed, _ := figures.Plan(document)
	return FigurePath(displayed, document, span)
}

// FigurePath is ImageSource for a plan already built for the document.
func FigurePath(displayed map[string]string, document, span rec.Map) string {
	digest, _ := figures.Identity(document, span)
	relative, named := displayed[digest]
	if !named {
		rec.Fail("Figure has no exported file: %s", digest)
	}
	return relative
}

func cssIdentifier(name string) bool {
	rest := strings.TrimPrefix(name, "-")
	if rest == "" || !(rest[0] == '_' || isASCIILetter(rest[0])) {
		return false
	}
	for index := 1; index < len(rest); index++ {
		c := rest[index]
		if !(isASCIILetter(c) || (c >= '0' && c <= '9') || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

func isASCIILetter(c byte) bool { return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') }

// FontFamily is SourceFonts.font_family: linked file first, then named families, then the
// generic class, written as browsers serialize a computed font-family.
func FontFamily(document, style rec.Map) string {
	font := fontmodel.ForStyle(document, style)
	if font == nil {
		return ""
	}
	names := []string{}
	if family, ok := font["css_family"].(string); ok {
		names = append(names, family)
	}
	for _, family := range rec.FetchList(font, "families") {
		if name, ok := family.(string); ok {
			names = append(names, name)
		}
	}
	parts := []string{}
	for _, name := range names {
		if cssIdentifier(name) && !slices.Contains(genericFamilies, strings.ToLower(name)) {
			parts = append(parts, name)
		} else {
			parts = append(parts, `"`+strings.NewReplacer(`"`, "", `\`, "").Replace(name)+`"`)
		}
	}
	return strings.Join(append(parts, rec.ToS(rec.Fetch(font, "generic"))), ", ")
}

// cssIdentifier and genericFamilies are SourceFonts::IDENTIFIER and GENERIC (FORMATS is
// htmlkit's).
var (
	genericFamilies = []string{"serif", "sans-serif", "monospace", "cursive", "fantasy", "system-ui", "math", "emoji",
		"fangsong", "inherit", "initial", "unset", "default"}
)

// DocumentLanguage is the page's lang: the document's language tag (decision 080), English for a
// document from before the field existed.
func DocumentLanguage(document rec.Map) string {
	if declared, ok := document["language"].(rec.Map); ok {
		if tag, ok := declared["tag"].(string); ok {
			return tag
		}
	}
	return langtag.Default
}

// English is whether the page writes the English words: the document's language tag says English,
// or says no language (decision 111, the bibliography run's name).
func English(document rec.Map) bool {
	tag := strings.ToLower(DocumentLanguage(document))
	return tag == "" || strings.HasPrefix(tag, "en")
}

// WCAGClaim is the only conformance the accessibility report states and the EPUB package claims
// (decision 080 item 5).
const WCAGClaim = "WCAG 2.2 Level AA"
