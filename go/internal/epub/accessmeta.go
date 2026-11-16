package epub

import (
	"fmt"
	"strings"

	"github.com/sarielhp/relatex/internal/htmlkit"
)

// Accessibility is the evidence the EPUB Accessibility 1.1 discovery metadata is computed from.
type Accessibility struct {
	Images    int // displayed images (figures)
	Described int // images with a text alternative (an unreviewed draft is one)
	Drafts    int // figure descriptions a model drafted that nobody has reviewed yet (decision 086)
	Long      int // figures with a long description, a disclosure after the figure (decision 110)
	Formulas  int // formulas, written as MathML
	Headings  int // headings, the structure a reader navigates
	Pages     int // page-break markers of the print edition, listed in the page list
	// Claim is the WCAG conformance the accessibility report establishes (WCAGClaim); "" claims none.
	Claim string
}

// conformanceProfile prefixes the claim of the package (EPUB Accessibility 1.1, section 2.1).
const conformanceProfile = "EPUB Accessibility 1.1 - "

// certifier is who established the claim: the export's own checks, an evaluation by the producer.
const certifier = "relatex accessibility checks (axe-core and structural checks of the export)"

// conformance is the package's dcterms:conformsTo, or "" when it claims none: the report's claim,
// on a package that also offers the print edition's pages (a publication with a print source
// gives its page navigation).
func conformance(access Accessibility) string {
	if access.Claim != htmlkit.WCAGClaim || access.Pages == 0 || access.Drafts > 0 {
		return ""
	}
	return conformanceProfile + access.Claim
}

// accessibilityMetadata is the discovery metadata of EPUB Accessibility 1.1, each property
// stated only as far as the evidence carries it: visual access only with figures, alternative
// text only when every figure has one, MathML only with formulas, page navigation only with the
// page list, conformance only when the checks establish it.
func accessibilityMetadata(access Accessibility) [][2]string {
	pairs := [][2]string{{"schema:accessMode", "textual"}}
	described := access.Described >= access.Images
	if access.Images > 0 {
		pairs = append(pairs, [2]string{"schema:accessMode", "visual"}, [2]string{"schema:accessModeSufficient", "textual,visual"})
	}
	if described {
		pairs = append(pairs, [2]string{"schema:accessModeSufficient", "textual"})
	}
	for _, feature := range features(access, described) {
		pairs = append(pairs, [2]string{"schema:accessibilityFeature", feature})
	}
	pairs = append(pairs, [2]string{"schema:accessibilityHazard", "none"}, [2]string{"schema:accessibilitySummary", summary(access)})
	if claim := conformance(access); claim != "" {
		pairs = append(pairs, [2]string{"dcterms:conformsTo", claim}, [2]string{"a11y:certifiedBy", certifier})
	}
	return pairs
}

func features(access Accessibility, described bool) []string {
	features := []string{"tableOfContents", "readingOrder", "displayTransformability"}
	if access.Headings > 0 {
		features = append(features, "structuralNavigation")
	}
	if access.Formulas > 0 {
		features = append(features, "MathML")
	}
	if access.Images > 0 && described {
		features = append(features, "alternativeText")
	}
	if access.Long > 0 {
		features = append(features, "longDescription")
	}
	if access.Pages > 0 {
		features = append(features, "printPageNumbers", "pageNavigation")
	}
	return features
}

// summary is the accessibility summary: what the publication offers and what it lacks, and
// whether conformance is claimed.
func summary(access Accessibility) string {
	parts := []string{"Converted from LaTeX by relatex. The text is reflowable, in the reading order of the source, with a table of contents"}
	if access.Headings > 0 {
		parts[0] += " and navigation by headings"
	}
	parts[0] += "."
	if access.Formulas > 0 {
		parts = append(parts, fmt.Sprintf("Its %s written as MathML.", plural(access.Formulas, "formula is", "formulas are")))
	}
	if access.Pages > 0 {
		parts = append(parts, "The page numbers of the print edition (the PDF named as the source) are marked in the text and listed in a page list.")
	}
	parts = append(parts, figureSummary(access)...)
	switch claim := conformance(access); {
	case claim != "":
		parts = append(parts, "The publication conforms to "+claim+", as established by the export's accessibility checks.")
	case access.Claim == htmlkit.WCAGClaim && access.Pages == 0:
		parts = append(parts, "Conformance to EPUB Accessibility 1.1 is not claimed: the publication has no page list of its print edition.")
	default:
		parts = append(parts, "Conformance to EPUB Accessibility 1.1 and WCAG is not claimed: the accessibility checks have not established it.")
	}
	return strings.Join(parts, " ")
}

// figureSummary: the figures without a text alternative and the descriptions still to be reviewed.
func figureSummary(access Accessibility) []string {
	parts := []string{}
	if missing := access.Images - access.Described; missing > 0 {
		verb := "have"
		if missing == 1 {
			verb = "has"
		}
		parts = append(parts, fmt.Sprintf("%s of %d %s no text alternative yet.", plural(missing, "figure", "figures"), access.Images, verb))
	} else if access.Images > 0 {
		parts = append(parts, "Every figure has a text alternative.")
	}
	if access.Long > 0 {
		parts = append(parts, fmt.Sprintf("%s a long description after the figure, shown on demand.",
			plural(access.Long, "figure has", "figures have")))
	}
	if access.Drafts > 0 {
		verb := "were"
		if access.Drafts == 1 {
			verb = "was"
		}
		parts = append(parts, fmt.Sprintf("%s %s drafted by a model and not yet reviewed by the author.",
			plural(access.Drafts, "figure description", "figure descriptions"), verb))
	}
	return parts
}

func plural(count int, one, many string) string {
	if count == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", count, many)
}
