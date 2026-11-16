package paperdoc

import (
	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/rec"
)

// ExternalHref is PaperExternalLinks.href: the address of the external link a span carries
// (its external_link key in the document's external_links items), as every output reads it.
func ExternalHref(document, span rec.Map) any {
	items := rec.FetchMap(rec.FetchMap(document, "external_links"), "items")
	key := rec.Fetch(span, "external_link")
	name, _ := key.(string)
	item, ok := items[name]
	if !ok {
		panic(rec.Failure{Message: "key not found: " + inspectKey(key), Lookup: true})
	}
	return rec.Fetch(rec.AsMap(item), "href")
}

// inspectKey is a lookup key in a failure message, as JSON ("nil" when it has none).
func inspectKey(value any) string {
	bytes, err := canonjson.Marshal(value)
	if err != nil {
		return "nil"
	}
	return string(bytes)
}
