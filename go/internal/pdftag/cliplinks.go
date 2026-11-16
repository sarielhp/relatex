package pdftag

// A clip text object whose glyphs belong to several leaves (a link broken over lines: ocgx2 and
// ocgcolorlinks draw both lines in one `q 7 Tr BT ... ET`, two layer fills and `0 Tr Q`) cannot keep
// the leaves' sequences inside its BT, where MuPDF renders them as an empty clip (decision 121,
// amendment 3). It is written once per leaf instead: the opening q and state operators, a BT of
// that leaf's lines inside the leaf's sequence, the ET, and the layer fills through the closing Q.
// Each copy clips the fills to its own lines, which paint the union the one text object clipped to.

// clipGroup is the items of a clip text object that belong to one leaf, between BT and ET.
type clipGroup struct {
	key        string
	first      int // ordinal of the leaf's first glyph
	from, upto int // item indexes, upto excluded
}

// clipPlan is a clip text object cut by leaf with the items around it that repeat for every cut.
type clipPlan struct {
	groups       []clipGroup
	opening      int // index of the q the clip begins in
	text, ending int // indexes of the BT and of its ET
	closing      int // index of the Q that ends the q
}

// planClip cuts the clip text object beginning at index by leaf, when it can be: every show has
// one leaf, the next leaf's lines begin at a Tm (a text object restarts its matrix), and only state
// operators lie between the q and the BT and only paints, layers and state between ET and the Q.
func (e *emitter) planClip(index int) (clipPlan, bool) {
	plan := clipPlan{text: index, opening: e.openingQ(index)}
	if plan.opening < 0 {
		return plan, false
	}
	var ok bool
	if plan.groups, plan.ending, ok = e.cutByLeaf(index); !ok || len(plan.groups) < 2 {
		return plan, false
	}
	plan.closing = e.closingQ(plan.ending)
	return plan, plan.closing > 0
}

// openingQ is the q the text object at index sits in, -1 when only state operators do not lead to it.
func (e *emitter) openingQ(index int) int {
	depth := 0
	for at := index - 1; at >= 0; at-- {
		entry := e.items[at]
		if entry.kind != kindState {
			return -1
		}
		switch entry.op.operator {
		case "Q":
			depth++
		case "q":
			if depth == 0 {
				return at
			}
			depth--
		}
	}
	return -1
}

// closingQ is the Q that ends the q around the text object ending at ending, -1 when the items
// before it hold text or a text object.
func (e *emitter) closingQ(ending int) int {
	depth := 0
	for at := ending + 1; at < len(e.items); at++ {
		entry := e.items[at]
		switch entry.kind {
		case kindShow, kindTextBegin, kindTextEnd:
			return -1
		case kindState:
			switch entry.op.operator {
			case "q":
				depth++
			case "Q":
				if depth == 0 {
					return at
				}
				depth--
			}
		}
	}
	return -1
}

// cutByLeaf groups the shows of the text object at index by leaf, and returns the index of its ET.
func (e *emitter) cutByLeaf(index int) ([]clipGroup, int, bool) {
	var groups []clipGroup
	clip := false
	last := index // the item after which the next group may begin
	for at := index + 1; at < len(e.items); at++ {
		entry := e.items[at]
		switch entry.kind {
		case kindTextEnd:
			if len(groups) == 0 || !clip {
				return nil, 0, false
			}
			groups[len(groups)-1].upto = at
			return groups, at, true
		case kindShow:
			clip = clip || entry.clip
			next, ok := e.nextGroup(groups, at, last)
			if !ok {
				return nil, 0, false
			}
			groups = next
			last = at
		case kindState:
		default:
			return nil, 0, false
		}
	}
	return nil, 0, false
}

// nextGroup adds the show at index to the groups: to the last one, or as the first of a new one when
// its leaf differs. A new group begins at the Tm that follows the previous show at after.
func (e *emitter) nextGroup(groups []clipGroup, index, after int) ([]clipGroup, bool) {
	entry := e.items[index]
	key := e.tags.keyOf(entry.font, entry.first)
	for ordinal := entry.first + 1; ordinal < entry.first+entry.count; ordinal++ {
		if e.tags.keyOf(entry.font, ordinal) != key {
			return nil, false
		}
	}
	if entry.count == 0 || key == artifactKey {
		if len(groups) == 0 {
			return append(groups, clipGroup{key: artifactKey, first: 0, from: e.firstAfterBT(index)}), true
		}
		return groups, true
	}
	if len(groups) == 0 || groups[len(groups)-1].key == artifactKey {
		if len(groups) == 0 {
			groups = append(groups, clipGroup{from: e.firstAfterBT(index)})
		}
		groups[0].key, groups[0].first = key, entry.first
		return groups, true
	}
	if groups[len(groups)-1].key == key {
		return groups, true
	}
	for at := index - 1; at > after; at-- {
		if e.items[at].op.operator == "Tm" {
			groups[len(groups)-1].upto = at
			return append(groups, clipGroup{key: key, first: entry.first, from: at}), true
		}
	}
	return nil, false
}

// firstAfterBT is the index just after the BT before the item at index.
func (e *emitter) firstAfterBT(index int) int {
	for at := index; at > 0; at-- {
		if e.items[at-1].kind == kindTextBegin {
			return at
		}
	}
	return 0
}

// clipObjects writes the clip text object at index once per leaf and returns the index of the
// last item it wrote; false when the object is not cut and is written as any other.
func (e *emitter) clipObjects(index int) (int, bool) {
	plan, ok := e.planClip(index)
	if !ok {
		return 0, false
	}
	for number, group := range plan.groups {
		if number > 0 {
			for at := plan.opening; at < plan.text; at++ {
				e.item(at, e.items[at])
			}
		}
		e.close()
		e.begin(group.key, group.first)
		e.fixed = true
		e.write(e.items[plan.text].op.raw)
		if number > 0 {
			e.restoreState(plan.text+1, group.from)
		}
		for at := group.from; at < group.upto; at++ {
			e.item(at, e.items[at])
		}
		for at := plan.ending; at <= plan.closing; at++ {
			e.item(at, e.items[at])
		}
	}
	return plan.closing, true
}

// restoreState writes the state operators the text object set between from and upto again: the q
// of a copy forgot them (the font, the colour), and a text object restarts only its matrix.
func (e *emitter) restoreState(from, upto int) {
	for at := from; at < upto; at++ {
		if e.items[at].kind == kindState {
			e.item(at, e.items[at])
		}
	}
}
