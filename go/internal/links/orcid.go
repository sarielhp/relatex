package links

import "regexp"

// orcidHref is a link to ORCID; orcidID its iD, when the path holds one as \orcidlink writes
// it (https://orcid.org/0000-0001-9416-1742: four groups of four, the last a digit or the
// checksum X).
var (
	orcidHref = regexp.MustCompile(`\Ahttps?://(?:www\.)?orcid\.org/`)
	orcidID   = regexp.MustCompile(`\Ahttps?://(?:www\.)?orcid\.org/(\d{4}-\d{4}-\d{4}-\d{3}[\dX])/?\z`)
)

// OrcidSource is the reading filename of the shared ORCID logo (go/internal/assets/orcid.pdf).
// Its directory is no paper's, so a paper's own orcid.pdf keeps its bytes.
const OrcidSource = "relatex-logos/orcid.pdf"

// OrcidLogoLimitSP bounds the picture an ORCID link may hold to be read as the ORCID logo: the
// logo is drawn at the height of an X (\orcidlink) or of a line, never larger than 24 pt.
const OrcidLogoLimitSP = 24 * 65_536

// OrcidName is the accessible name of an ORCID logo link: "ORCID iD 0000-0001-9416-1742", or
// "ORCID" when the link names no iD.
func OrcidName(href string) string {
	if match := orcidID.FindStringSubmatch(href); match != nil {
		return "ORCID iD " + match[1]
	}
	return "ORCID"
}

// OrcidLogo reports whether a picture of this size, inside a link to href, is the ORCID logo.
func OrcidLogo(href any, width, height, depth int64) bool {
	text, ok := href.(string)
	return ok && orcidHref.MatchString(text) && width > 0 && width <= OrcidLogoLimitSP &&
		height+depth > 0 && height+depth <= OrcidLogoLimitSP
}
