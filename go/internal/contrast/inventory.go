package contrast

import (
	"fmt"

	"github.com/sarielhp/relatex/internal/rec"
)

// Restore is a browser inventory measured in the light palette with each recorded adjusted ink
// read as the PDF ink it stands for, so the paint checks compare the PDF's colours: a colour the
// record does not name is compared as observed. The record must be the one the document's inks
// give (Adjustments); any other record fails.
func Restore(inventory, document rec.Map) rec.Map {
	if !rec.Equal(document[Field], nilIfEmpty(Adjustments(document))) {
		rec.Fail("Ink adjustments differ from the document's inks")
	}
	record, ok := document[Field].(rec.Map)
	if !ok {
		return inventory
	}
	originals := map[string]string{}
	for _, value := range rec.FetchList(record, "items") {
		ink, adjusted := rec.FetchString(rec.AsMap(value), "ink"), rec.FetchString(rec.AsMap(value), "adjusted")
		originals[adjusted] = ink
		originals[cssRGB(adjusted)] = cssRGB(ink)
	}
	return rec.AsMap(restore(inventory, originals))
}

func nilIfEmpty(record rec.Map) any {
	if record == nil {
		return nil
	}
	return record
}

// restore walks the inventory: the colour fields (color, background, run_colors) are restored.
func restore(value any, originals map[string]string) any {
	switch typed := value.(type) {
	case rec.Map:
		result := rec.Map{}
		for key, field := range typed {
			result[key] = restoreField(key, field, originals)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = restore(item, originals)
		}
		return result
	}
	return value
}

func restoreField(key string, field any, originals map[string]string) any {
	switch key {
	case "color", "background":
		if text, ok := field.(string); ok {
			return original(text, originals)
		}
	case "run_colors":
		if list, ok := field.([]any); ok {
			result := make([]any, len(list))
			for index, item := range list {
				result[index] = item
				if text, isText := item.(string); isText {
					result[index] = original(text, originals)
				}
			}
			return result
		}
	}
	return restore(field, originals)
}

func original(color string, originals map[string]string) string {
	if ink, ok := originals[color]; ok {
		return ink
	}
	return color
}

// cssRGB is how the browser reports a #rrggbb colour it computed.
func cssRGB(hex string) string {
	color, ok := Parse(hex)
	if !ok {
		return hex
	}
	bytes := color.bytes()
	return fmt.Sprintf("rgb(%d, %d, %d)", bytes[0], bytes[1], bytes[2])
}
