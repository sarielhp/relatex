package reading

import (
	"github.com/sarielhp/relatex/internal/links"
	"github.com/sarielhp/relatex/internal/rec"
)

// orcidLink: a picture inside a link to ORCID (\orcidlink's logo) keeps its link while the
// reading decides, from the picture's region on the page, whether it is the logo.
func orcidLink(item, link rec.Map) bool {
	return rec.Fetch(item, "type") == "graphic" && links.OrcidLogo(link["href"], 1, 1, 0)
}

// orcidSpan: the rendered picture of an ORCID link is the ORCID logo when it is logo-sized;
// the span keeps the link, and the export shows the shared logo inside it instead of a crop of
// the PDF. A larger picture is an ordinary rendered picture, its link dropped.
func (e *Evidence) orcidSpan(span, item rec.Map) {
	value, linked := item["external_link"]
	if !linked {
		return
	}
	link := e.externalLinks[rec.AsInt(value)]
	region := rec.FetchList(span, "region_sp")
	width := rec.AsInt(region[2]) - rec.AsInt(region[0])
	height := rec.AsInt(region[3]) - rec.AsInt(region[1])
	if links.OrcidLogo(link["href"], width, height, 0) {
		span["external_link"] = value
		return
	}
	e.dropExternalLink(link)
}
