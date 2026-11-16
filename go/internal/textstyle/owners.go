package textstyle

import "github.com/sarielhp/relatex/internal/rec"

// UnobservedOwnerStyles is the style of a direct TeX font (\font\x=cmti10) for each flow owner
// that prints it: from the first owner holding the font's first character to the next such font.
func UnobservedOwnerStyles(records []rec.Map, catalog Catalog) map[any]rec.Map {
	result := map[any]rec.Map{}
	conflicting := map[any]bool{}
	for index, row := range records {
		if row["kind"] != "diagnostic" || row["code"] != "unobserved_text_font" {
			continue
		}
		style := UnobservedStyle(row, catalog)
		firstChar, hasChar := firstCharacter(row)
		if style == nil || !hasChar {
			continue
		}
		expectedEntity, hasEntity := precedingEntity(records, index)
		start, role := firstMatchingOwner(records, index+1, firstChar, expectedEntity, hasEntity)
		if start < 0 {
			continue
		}
		for offset, candidate := range records[start:] {
			if candidate["kind"] == "diagnostic" && candidate["code"] == "unobserved_text_font" {
				break
			}
			if candidate["kind"] != "flow_candidate" || !rec.Equal(candidate["role"], role) {
				continue
			}
			// The first owner may mix the font with declared ones ({\uno Abstract}. text): its
			// unstyled items are the direct font's; a later owner must print nothing else.
			if offset == 0 || candidateHasNoStyles(candidate) {
				owner := rec.Fetch(candidate, "owner")
				// An owner printing two direct fonts (a heading whose word is set in another
				// face) has no single style: its items name their own fonts.
				if conflicting[owner] {
					continue
				}
				if known, seen := result[owner]; seen && !rec.Equal(known, style) {
					conflicting[owner] = true
					delete(result, owner)
					continue
				}
				result[owner] = style
			}
		}
	}
	return result
}

func precedingEntity(records []rec.Map, before int) (any, bool) {
	for index := before - 1; index >= 0; index-- {
		if entity, ok := records[index]["entity"]; ok {
			return entity, true
		}
	}
	return nil, false
}

func firstMatchingOwner(records []rec.Map, start int, firstChar int64, expectedEntity any, hasEntity bool) (int, any) {
	for index := start; index < len(records); index++ {
		candidate := records[index]
		if candidate["kind"] != "flow_candidate" || hasEntity && !rec.Equal(candidate["entity"], expectedEntity) ||
			!candidateHasCharacter(candidate, firstChar) {
			continue
		}
		return index, rec.Fetch(candidate, "role")
	}
	return -1, nil
}

func candidateHasNoStyles(candidate rec.Map) bool {
	for _, value := range rec.FetchList(candidate, "items") {
		if _, ok := rec.AsMap(value)["style"]; ok {
			return false
		}
	}
	return true
}

// candidateHasCharacter: an item that names no font prints the character.
func candidateHasCharacter(candidate rec.Map, character int64) bool {
	for _, value := range rec.FetchList(candidate, "items") {
		if _, styled := rec.AsMap(value)["style"]; styled {
			continue
		}
		text, _ := rec.AsMap(value)["text"].(string)
		for _, printed := range text {
			if int64(printed) == character {
				return true
			}
		}
	}
	return false
}

// firstCharacter is the font's first printed character as the flow text spells it: the decoded
// character when the collector decoded one (a TFM slot such as cmti10's 16 prints U+0131), else
// the raw slot.
func firstCharacter(row rec.Map) (int64, bool) {
	if text, ok := row["first_text"].(string); ok {
		for _, character := range text {
			return int64(character), true
		}
	}
	character, ok := row["first_char"].(int64)
	return character, ok
}
