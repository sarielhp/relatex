package html

import (
	"github.com/sarielhp/relatex/internal/extlinks"
	"github.com/sarielhp/relatex/internal/htmlkit"
	"github.com/sarielhp/relatex/internal/links"
	"github.com/sarielhp/relatex/internal/paperdoc"
	"github.com/sarielhp/relatex/internal/rec"
)

// linkedImage puts a linked image (only the ORCID logo is one) inside its link: one span per
// reading span, holding the link, holding the logo. The link is named "ORCID iD <iD>" (or
// "ORCID"), so the logo itself is decorative (alt="").
func (p *Paper) linkedImage(image string, span rec.Map) string {
	if _, linked := span["external_link"]; !linked {
		return image
	}
	if !p.described {
		p.descriptors, p.described = extlinks.Descriptors(p.document, false), true
	}
	href := paperdoc.ExternalHref(p.document, span)
	id := ""
	for _, link := range p.descriptors {
		if _, has := link["block"]; has && rec.Identity(extlinks.Resolve(p.document, link)) == rec.Identity(span) {
			id = toS(rec.Fetch(link, "id"))
		}
	}
	if id == "" {
		rec.Fail("Linked image has no external link descriptor")
	}
	hrefText := rec.AsString(href)
	return "<span data-kind=\"linked\" data-external-link-id=\"" + htmlkit.Escape(rec.AsString(span["external_link"])) +
		"\"><a data-external-link=\"" + id + "\" href=\"" + htmlkit.Escape(hrefText) + "\" rel=\"noopener noreferrer\" aria-label=\"" +
		htmlkit.Escape(links.OrcidName(hrefText)) + "\">" + image + "</a></span>"
}
