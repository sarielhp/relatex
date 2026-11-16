// Package structure is the Go port of Relatex::StructureEvidence: a bounded ownership tree
// over retained blocks, not a complete document model.
package structure

import (
	"slices"

	"github.com/sarielhp/relatex/internal/omap"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/stream"
)

// asideLimit is the longest run of blocks owned elsewhere that still counts as an aside.
const asideLimit = 8

type evidence struct {
	records   []rec.Map
	blocks    []rec.Map
	contexts  *omap.Map[int64, rec.Map]
	nodes     *omap.Map[int64, rec.Map]
	depth     map[int64]int64
	floats    map[int64]bool
	members   map[int64]bool
	displaced []any
	closed    map[int64]bool
	open      []int64
}

// Unavailable is the report of a stream without the structure protocol.
func Unavailable() rec.Map {
	return rec.Map{"status": "unavailable", "nodes": []any{}, "discarded_entities": []any{}}
}

// Report is StructureEvidence.new(records, blocks).report.
func Report(records []rec.Map, blocks []rec.Map) rec.Map {
	if rec.Fetch(records[0], "protocol") != "0.6" {
		return Unavailable()
	}
	e := &evidence{records: records, blocks: blocks, contexts: omap.New[int64, rec.Map](), nodes: omap.New[int64, rec.Map](),
		depth: map[int64]int64{0: 0}, closed: map[int64]bool{}}
	e.contexts.Set(0, rec.Map{"role": "document", "parent": int64(0)})
	e.inventory()
	e.ensureNode(0)
	e.placeBlocks()
	ids := make([]any, len(blocks))
	seen := map[any]bool{}
	unique := true
	for index, block := range blocks {
		ids[index] = rec.Fetch(block, "id")
		unique = unique && !seen[ids[index]]
		seen[ids[index]] = true
	}
	if !unique || !slices.Equal(e.flatten(0), ids) {
		rec.Fail("Duplicate or interleaved structural blocks")
	}
	discarded := []any{}
	for _, id := range e.contexts.Keys() {
		context, _ := e.contexts.Get(id)
		if rec.Fetch(context, "role") != "paragraph" && !e.nodes.Has(id) {
			discarded = append(discarded, id)
		}
	}
	nodes := []any{}
	e.nodes.Each(func(_ int64, node rec.Map) { nodes = append(nodes, node) })
	result := rec.Map{"status": "verified", "nodes": nodes, "discarded_entities": discarded}
	if len(e.displaced) > 0 {
		result["displaced"] = e.displaced
	}
	return result
}

func (e *evidence) context(entity int64) rec.Map {
	context, ok := e.contexts.Get(entity)
	if !ok {
		rec.Fail("key not found: %d", entity)
	}
	return context
}

func (e *evidence) parent(entity int64) int64 { return rec.FetchInt(e.context(entity), "parent") }

// placeBlocks: each block joins its owner's node in reading order. A block printed where
// its owner is not open joins the container open at that point instead, as a float does: an
// aside (revtex's page-1 affiliation note inside section 1, a later section's rule) is a
// block owned elsewhere that the reading leaves at once for the open container; a return to
// an owner already left behind is placed the same way. Such blocks are reported as displaced.
func (e *evidence) placeBlocks() {
	e.displaced = []any{}
	container := int64(0)
	targets := make([]int64, len(e.blocks))
	floating := make([]bool, len(e.blocks))
	for index, block := range e.blocks {
		targets[index] = e.target(block)
	}
	// Floats join the open container wherever they are printed; they never end an aside.
	for index, block := range e.blocks {
		floating[index] = e.inFloat(rec.FetchInt(block, "entity"))
	}
	for index, block := range e.blocks {
		entity := targets[index]
		var node rec.Map
		switch {
		case floating[index]:
			// A list or theorem in a float's body keeps its own structure, printed with the float.
			if member := e.floatMember(entity); member != 0 {
				node = e.ensureMemberNode(entity, member, container)
			}
			entity = container
		case e.isDisplaced(entity, container, following(targets, floating, index+1)):
			e.displaced = append(e.displaced, rec.Fetch(block, "id"))
			entity = container
		default:
			e.closeOutside(entity)
			container = entity
		}
		if node == nil {
			node = e.ensureNode(entity)
		}
		node["content"] = append(node["content"].([]any), rec.Map{"block": rec.Fetch(block, "id")})
	}
}

// floatMember is the outermost context set in a float's body (float_member) that holds entity,
// or 0.
func (e *evidence) floatMember(entity int64) int64 {
	member := int64(0)
	for ; entity != 0; entity = e.parent(entity) {
		if e.memberEntities()[entity] {
			member = entity
		}
	}
	return member
}

// ensureMemberNode is ensureNode for a context inside a float member: the member itself joins
// the container open where the float is printed, not its context's parent.
func (e *evidence) ensureMemberNode(entity, member, container int64) rec.Map {
	if node, ok := e.nodes.Get(entity); ok {
		return node
	}
	parent := e.ensureNode(container)
	if entity != member {
		parent = e.ensureMemberNode(e.parent(entity), member, container)
	}
	node := rec.Map{"id": entity, "role": rec.Fetch(e.context(entity), "role"), "content": []any{}}
	e.nodes.Set(entity, node)
	parent["content"] = append(parent["content"].([]any), rec.Map{"structure": entity})
	return node
}

// memberEntities are the lists and theorems set in a float's body (float_member).
func (e *evidence) memberEntities() map[int64]bool {
	if e.members != nil {
		return e.members
	}
	e.members = map[int64]bool{}
	for _, record := range e.records {
		if rec.Fetch(record, "kind") == "diagnostic" && record["code"] == "float_member" {
			if entity, ok := rec.Fetch(record, "entity").(int64); ok {
				e.members[entity] = true
			}
		}
	}
	return e.members
}

// following is the owners of the blocks from start on, floats left out, at most asideLimit.
func following(targets []int64, floating []bool, start int) []int64 {
	result := []int64{}
	for index := start; index < len(targets) && len(result) < asideLimit; index++ {
		if !floating[index] {
			result = append(result, targets[index])
		}
	}
	return result
}

func (e *evidence) target(block rec.Map) int64 {
	entity := rec.FetchInt(block, "entity")
	context, ok := e.contexts.Get(entity)
	if !ok {
		rec.Fail("Unknown block context")
	}
	// Text a list owns before its first item (a theorem head held until the list's first
	// paragraph) reads in the list's parent, where it prints: before the items.
	if slices.Contains([]any{"unordered_list", "ordered_list", "paragraph"}, rec.Fetch(context, "role")) {
		return rec.FetchInt(context, "parent")
	}
	return entity
}

func (e *evidence) isDisplaced(entity, container int64, following []int64) bool {
	if entity == container {
		return false
	}
	// A node left behind stays closed even when the open container is an ancestor of it: a
	// block of the document printed between two sections must not reopen the first.
	if e.isClosed(entity) {
		return true
	}
	if e.within(entity, container) {
		return false
	}
	// An aside: within a few blocks owned elsewhere, the reading returns to the open container.
	for index, owner := range following {
		if owner == container || e.within(owner, container) {
			return !slices.Contains(following[:index], container)
		}
	}
	return false
}

// closeOutside: leaving the open path for entity's closes every open node not on it.
func (e *evidence) closeOutside(entity int64) {
	path := e.path(entity)
	if e.open == nil {
		e.open = []int64{0}
	}
	for _, node := range e.open {
		if !slices.Contains(path, node) {
			e.closed[node] = true
		}
	}
	e.open = path
}

func (e *evidence) path(entity int64) []int64 {
	chain := []int64{entity}
	for chain[len(chain)-1] != 0 {
		chain = append(chain, e.parent(chain[len(chain)-1]))
	}
	return chain
}

func (e *evidence) within(entity, ancestor int64) bool {
	return slices.Contains(e.path(entity), ancestor)
}

func (e *evidence) isClosed(entity int64) bool {
	return slices.ContainsFunc(e.path(entity), func(node int64) bool { return e.closed[node] })
}

func (e *evidence) inventory() {
	for _, record := range e.records {
		if rec.Fetch(record, "kind") != "context" {
			continue
		}
		id, parent := record["entity"], record["parent"]
		if e.has(id) {
			rec.Fail("Duplicate structural entity")
		}
		if !e.has(parent) || rec.Fetch(e.context(parent.(int64)), "role") == "paragraph" {
			rec.Fail("Invalid structural parent")
		}
		e.validateListContext(record, parent.(int64))
		entity := rec.AsInt(id)
		e.depth[entity] = e.depth[parent.(int64)] + 1
		if e.depth[entity] > 64 {
			rec.Fail("Structural depth limit")
		}
		e.contexts.Set(entity, record)
	}
}

func (e *evidence) has(value any) bool {
	id, ok := value.(int64)
	return ok && e.contexts.Has(id)
}

// floatEntities is the float environments, and the captions and display environments inside
// them, observed by the float and equation hooks.
func (e *evidence) floatEntities() map[int64]bool {
	if e.floats != nil {
		return e.floats
	}
	e.floats = map[int64]bool{}
	for _, record := range e.records {
		if rec.Fetch(record, "kind") != "diagnostic" {
			continue
		}
		number, _ := record["float"].(int64)
		inside := record["code"] == "caption_context" || record["code"] == "equation_context"
		if record["code"] == "float_context" || record["code"] == "float_member" || (inside && number > 0) {
			if entity, ok := rec.Fetch(record, "entity").(int64); ok {
				e.floats[entity] = true
			}
		}
	}
	return e.floats
}

func (e *evidence) inFloat(entity int64) bool {
	for entity != 0 {
		if e.floatEntities()[entity] {
			return true
		}
		entity = e.parent(entity)
	}
	return false
}

func (e *evidence) ensureNode(entity int64) rec.Map {
	if node, ok := e.nodes.Get(entity); ok {
		return node
	}
	context := e.context(entity)
	var parent rec.Map
	if entity != 0 {
		parent = e.ensureNode(rec.FetchInt(context, "parent"))
	}
	node := rec.Map{"id": entity, "role": rec.Fetch(context, "role"), "content": []any{}}
	e.nodes.Set(entity, node)
	if parent != nil {
		parent["content"] = append(parent["content"].([]any), rec.Map{"structure": entity})
	}
	return node
}

func (e *evidence) validateListContext(record rec.Map, parent int64) {
	role := rec.Fetch(record, "role")
	if !slices.Contains([]any{"unordered_list", "ordered_list", "list_item"}, role) {
		return
	}
	if !stream.Enabled(e.records[0], "lists-0.1") {
		rec.Fail("List context without external list protocol")
	}
	if role != "list_item" {
		return
	}
	if slices.Contains([]any{"unordered_list", "ordered_list"}, rec.Fetch(e.context(parent), "role")) {
		return
	}
	rec.Fail("List item without list parent")
}

func (e *evidence) flatten(entity int64) []any {
	node, _ := e.nodes.Get(entity)
	result := []any{}
	for _, value := range rec.FetchList(node, "content") {
		item := rec.AsMap(value)
		if block, ok := item["block"]; ok {
			result = append(result, block)
			continue
		}
		result = append(result, e.flatten(rec.FetchInt(item, "structure"))...)
	}
	return result
}
