package extlinks

import (
	"github.com/sarielhp/relatex/internal/links"
	"github.com/sarielhp/relatex/internal/rec"
)

// OrcidImage reports whether a linked image span is the ORCID logo: the shared logo file, at
// most logo-sized, inside a link to ORCID. It is the only image a link may hold.
func OrcidImage(document, span rec.Map) bool {
	return span["type"] == "image" && span["source"] == links.OrcidSource &&
		links.OrcidLogo(href(document, span), rec.FetchInt(span, "width_sp"), rec.FetchInt(span, "height_sp"), 0)
}

// href is the address of a span's external link: paperdoc.ExternalHref, which the outputs read;
// paperdoc imports this package, so the core reads it here.
func href(document, span rec.Map) any {
	items := rec.FetchMap(rec.FetchMap(document, "external_links"), "items")
	return rec.Fetch(rec.AsMap(fetchKey(items, rec.Fetch(span, Field))), "href")
}
