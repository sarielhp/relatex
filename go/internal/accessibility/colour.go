package accessibility

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/wcag"
)

// colourItems (WCAG 1.4.1, use of colour): a float caption that names colours ("the red line",
// "shown in blue") points the reader to a legend colour alone may carry. Whether a word, marker or
// pattern also carries it is not mechanically decidable (plan accessibility §7.2.4): each such
// caption is an open item for the author, with the colours it names. The author may attest a
// caption that already says it with more than colour (decision 188): relatex/hints.json keeps
// "colour_captions": {key: sha256 of the caption's text}, in the document as colour_captions. A
// caption whose float a key names and whose text hashes to the key's value is attested: listed in
// attested, never a gap. An edited caption no longer matches, so it is a gap again (its item says
// the attestation is voided); the check itself is unchanged and nothing is ever attested here.
func colourItems(document rec.Map, w *walk) (gaps, attested []any) {
	gaps, attested = []any{}, []any{}
	attestations := rec.MapOf(document["colour_captions"])
	texts := map[string]string{}
	for _, value := range rec.FetchList(document, "block_order") {
		id, _ := value.(string)
		if !slices.Contains(w.ancestors(id), "float_caption") {
			continue
		}
		text := blockText(rec.AsMap(w.nodes[id]))
		names := wcag.ColourNames(text)
		if len(names) == 0 {
			continue
		}
		colours := make([]any, len(names))
		for index, name := range names {
			colours[index] = name
		}
		item := placed(rec.Map{"block": id, "colours": colours, "text": clip(text, 160)}, w.places.Caption(id))
		float := w.places.FloatOf(id)
		if _, seen := texts[float]; !seen {
			texts[float] = captionText(document, w, float)
		}
		item["sha256"] = CaptionSHA(texts[float])
		key, state := attestation(attestations, w.places.FloatKeys(float), texts[float])
		switch state {
		case attestedState:
			item["key"] = key
			attested = append(attested, item)
		case voidedState:
			item["key"], item["attestation"] = key, "voided: the caption changed since it was attested"
			gaps = append(gaps, item)
		default:
			gaps = append(gaps, item)
		}
	}
	return gaps, attested
}

const (
	noAttestation = iota
	attestedState
	voidedState
)

// attestation finds the author's attestation for a float among its keys: the key that holds and the
// state, attested when its sha is the caption's, voided when a key names the float with another.
func attestation(attestations rec.Map, keys []string, text string) (string, int) {
	voided := ""
	sum := CaptionSHA(text)
	for _, key := range keys {
		value, ok := attestations[key].(string)
		switch {
		case !ok:
		case strings.EqualFold(strings.TrimSpace(value), sum):
			return key, attestedState
		case voided == "":
			voided = key
		}
	}
	if voided != "" {
		return voided, voidedState
	}
	return "", noAttestation
}

// CaptionSHA is the sha256 (lower-case hex) of a float caption's text as the document holds it:
// the texts of its blocks, one per line. The author's attestation binds to it, as lint.toml binds an
// accepted item to the sha of the suggestion.
func CaptionSHA(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// captionText is the text of a float's caption: its blocks with text, one per line.
func captionText(document rec.Map, w *walk, float string) string {
	lines := []string{}
	for _, value := range rec.FetchList(document, "block_order") {
		id, _ := value.(string)
		if !slices.Contains(w.ancestors(id), "float_caption") || w.places.FloatOf(id) != float {
			continue
		}
		if text := blockText(rec.AsMap(w.nodes[id])); text != "" {
			lines = append(lines, text)
		}
	}
	return strings.Join(lines, "\n")
}

// blockText is the text of a block's text and reference spans.
func blockText(node rec.Map) string {
	var text strings.Builder
	content, _ := node["content"].([]any)
	for _, value := range content {
		span := rec.AsMap(value)
		if span["type"] == "text" || span["type"] == "reference" {
			text.WriteString(str(span["text"]))
		}
	}
	return strings.TrimSpace(text.String())
}
