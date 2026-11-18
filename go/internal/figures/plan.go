package figures

import (
	"encoding/base64"
	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/mediamodel"
	"github.com/sarielhp/relatex/internal/rec"
	"sort"
	"strings"
)

// Directory is PaperImages::DIRECTORY: the folder beside the page that holds the
// figures (the EPUB 3 layout, decision 058).
const Directory = "figs"

// Original is one figure's original: the source PDF or the cropped JPEG it was made from.
type Original struct {
	Asset     string // the asset id in document.json
	Digest    string // the original's SHA-256
	Extension string // its file extension
}

// Identity is PaperImages.identity: the content and extension of the form the HTML
// shows for an occurrence. No bytes are decoded.
func Identity(document, span rec.Map) (string, string) {
	items := rec.FetchMap(rec.FetchMap(document, "assets"), "items")
	asset, ok := items[rec.FetchString(span, "asset")].(rec.Map)
	if !ok {
		panic(rec.Failure{Message: "key not found: " + rubyInspect(rec.FetchString(span, "asset")), Lookup: true})
	}
	placement, _ := span["placement"].(rec.Map)
	item := mediamodel.NewMediaAsset(asset).Representation("html", placement)
	extension, ok := mediamodel.Extensions[rec.FetchString(item, "mime_type")]
	if !ok {
		panic(rec.Failure{Message: "key not found: " + rubyInspect(rec.FetchString(item, "mime_type")), Lookup: true})
	}
	return rec.FetchString(item, "sha256"), extension
}

// Plan is PaperImages.plan: the name of every figure file, keyed by the content it
// holds. Figures made from one LaTeX source share its name and take a number when that name
// is held by other bytes; a figure with no source name is numbered in reading order.
func Plan(document rec.Map) (displayed, originals map[string]string) {
	names := Stems(document)
	displayed, originals = map[string]string{}, map[string]string{}
	for _, span := range Occurrences(document) {
		digest, extension := Identity(document, span)
		if _, named := displayed[digest]; named {
			continue
		}
		stem := names.Stem(rec.FetchString(span, "asset"), span)
		displayed[digest] = Directory + "/" + names.Claim(digest, stem+"."+extension)
	}
	for _, source := range Originals(document) {
		if _, named := originals[source.Digest]; named {
			continue
		}
		stem := names.Stem(source.Asset, rec.Map{})
		originals[source.Digest] = Directory + "/" + names.Claim(source.Digest, stem+"."+source.Extension,
			stem+"-source."+source.Extension)
	}
	return displayed, originals
}

// Originals is PaperImages.originals_of: each figure's original, in ascending identity
// order so an asset nothing displays still takes the same name every run.
func Originals(document rec.Map) []Original {
	items, ok := rec.FetchMap(document, "assets")["items"].(rec.Map)
	if !ok {
		return nil
	}
	sha := make([]string, 0, len(items))
	for key := range items {
		sha = append(sha, key)
	}
	sort.Strings(sha)
	sources := []Original{}
	for _, key := range sha {
		asset := rec.AsMap(items[key])
		if asset["schema_version"] != mediamodel.MediaVersion {
			continue
		}
		item := mediamodel.NewMediaAsset(asset).Representation("original", nil)
		extension, ok := mediamodel.Extensions[rec.FetchString(item, "mime_type")]
		if !ok {
			panic(rec.Failure{Message: "key not found: " + rubyInspect(rec.FetchString(item, "mime_type")), Lookup: true})
		}
		sources = append(sources, Original{Asset: key, Digest: rec.FetchString(item, "sha256"), Extension: extension})
	}
	return sources
}

// Displayed is PaperImages.displayed: the bytes and MIME type the HTML shows.
func Displayed(document, span rec.Map) ([]byte, string) {
	items := rec.FetchMap(rec.FetchMap(document, "assets"), "items")
	asset, ok := items[rec.FetchString(span, "asset")].(rec.Map)
	if !ok {
		panic(rec.Failure{Message: "key not found: " + rubyInspect(rec.FetchString(span, "asset")), Lookup: true})
	}
	if asset["schema_version"] == "media-1" {
		media := mediamodel.NewMediaAsset(asset)
		placement, _ := span["placement"].(rec.Map)
		item := media.Representation("html", placement)
		return media.Bytes(item), rec.FetchString(item, "mime_type")
	}
	mediamodel.ValidatePNGDescriptor(asset)
	bytes, err := base64.StdEncoding.Strict().DecodeString(strings.TrimPrefix(rec.FetchString(asset, "data_uri"), "data:image/png;base64,"))
	if err != nil {
		panic(err)
	}
	return bytes, "image/png"
}

// rubyInspect is String#inspect for the identifiers in lookup messages.
func rubyInspect(text string) string {
	encoded, err := canonjson.Marshal(text)
	if err != nil {
		return text
	}
	return string(encoded)
}

// CaseTwins are the author files of the document whose figure names differ only in case, with
// whether their bytes are identical (one asset: they then share one name; decision 129,
// amendment 5). Each name is compared with the first file that took it, whatever the extension.
func CaseTwins(document rec.Map) []Twin {
	type holder struct{ stem, origin, asset string }
	held := map[string]holder{}
	reported := map[[2]string]bool{}
	twins := []Twin{}
	for _, span := range Occurrences(document) {
		stem, named := SourceStem(span)
		if !named {
			continue
		}
		origin, asset := rec.FetchString(span, "source"), rec.FetchString(span, "asset")
		first, found := held[strings.ToLower(stem)]
		if !found {
			held[strings.ToLower(stem)] = holder{stem, origin, asset}
			continue
		}
		pair := [2]string{first.origin, origin}
		if first.stem != stem && !reported[pair] {
			reported[pair] = true
			twins = append(twins, Twin{First: first.origin, Second: origin, FirstAsset: first.asset, SecondAsset: asset,
				Identical: first.asset == asset})
		}
	}
	return twins
}
