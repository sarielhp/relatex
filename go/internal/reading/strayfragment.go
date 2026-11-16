package reading

import "github.com/sarielhp/relatex/internal/rec"

// strayCopy: a paragraph typeset twice (a caption measured, then set) can ship a hyphen or a
// ligature of its first copy inside the second, which lacks those items: the first copy never
// ships its first slot, only such one-item fragments.
func (e *Evidence) strayCopy(owner rec.Map) bool {
	return rec.Fetch(owner, "group") == "paragraph" && e.neverShipped(owner, 1, 1)
}

// fragmentHost is the open block a stray copy's fragment belongs to: the paragraph of the same
// entity still in progress around it, which resumes after the fragment. Nil when there is none.
func (e *Evidence) fragmentHost(owner rec.Map) rec.Map {
	part := e.readingPartition(owner)
	last, key := e.lastBlocks[part], e.lastKeys[part]
	if last == nil || rec.Fetch(last, "type") == "display_math" || rec.Equal(key.owner, rec.Fetch(owner, "owner")) ||
		!rec.Equal(rec.Fetch(last, "entity"), rec.Fetch(owner, "entity")) || !e.strayCopy(owner) {
		return nil
	}
	host, found := e.owners.Get(key.owner)
	progress, _ := e.ownerProgress.Get(key.owner)
	if !found || rec.Fetch(host, "group") != "paragraph" || progress >= int64(len(rec.FetchList(host, "items"))) {
		return nil
	}
	return last
}
