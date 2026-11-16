package a11ylint

import (
	"fmt"
	"sort"
	"strings"
)

// patchContext is the unchanged lines Patch keeps around each change.
const patchContext = 3

// patchBody is one line of a hunk: its diff prefix and the old or new line it shows.
type patchBody struct {
	prefix   byte
	text     string
	oldIndex int
	newIndex int
}

// patchSpan is a change's extent in old lines: [start, end), end == start for an insertion.
type patchSpan struct{ start, end int }

// Patch writes the source suggestions as a unified diff (git format), applied with patch -p1 in the
// paper directory. It carries the same conditions as the annotated copy's suggestion lines: Place
// says whether the suggestion follows NoteLine or replaces it. Figures, drafts and drawings stay
// out, as their decision is the sidecar's (decision 112), and so does advice.
func Patch(paper Paper, items []Item) (string, error) {
	files := make(map[string]*File, len(paper.Files))
	for _, file := range paper.Files {
		files[file.Path] = file
	}
	byFile := map[string][]Item{}
	paths := []string{}
	for _, item := range items {
		file, ok := files[item.File]
		if !ok || !patchable(item) || item.NoteLine > len(file.lines) {
			continue
		}
		if _, seen := byFile[item.File]; !seen {
			paths = append(paths, item.File)
		}
		byFile[item.File] = append(byFile[item.File], item)
	}
	sort.Strings(paths)
	var out strings.Builder
	for _, path := range paths {
		out.WriteString(patchSection(files[path], byFile[path]))
	}
	return out.String(), nil
}

// patchable: an item with a line to add or replace, not a figure, draft or drawing (their decision
// is the sidecar's) and not advice.
func patchable(item Item) bool {
	if item.Advisory || item.Suggestion == "" || item.NoteLine <= 0 {
		return false
	}
	switch item.Kind {
	case "figure", "draft", "drawing":
		return false
	}
	return item.Place == PlaceAfter || item.Place == PlaceInstead
}

// patchSection is one file's diff sections: its additions before each old line and the lines their
// items replace, merged into hunks of patchContext lines of context.
func patchSection(file *File, items []Item) string {
	old, noEOL := sourceLines(file.Source)
	added := make([][]string, len(old)+1)
	replaced := make([]string, len(old))
	replaces := make([]bool, len(old))
	for _, item := range items {
		patchAdd(item, len(old), added, replaced, replaces)
	}
	if strings.Contains(file.Source, "\r\n") {
		crlfLines(added, replaced, replaces)
	}
	keepLastLine(old, noEOL, added, replaced, replaces)
	before := patchNewBefore(old, added)
	newTotal := before[len(old)] + len(added[len(old)])
	var out strings.Builder
	groups := patchGroups(patchSpans(len(old), added, replaces), len(old))
	if len(groups) > 0 {
		fmt.Fprintf(&out, "--- a/%s\n+++ b/%s\n", file.Path, file.Path)
	}
	for _, group := range groups {
		body, addedLines := patchHunk(old, added, replaced, replaces, before, group)
		fmt.Fprintf(&out, "@@ -%d,%d +%d,%d @@\n", group[0]+1, group[1]-group[0], before[group[0]]+1, addedLines)
		patchWriteBody(&out, body, len(old), newTotal, noEOL)
	}
	return out.String()
}

// patchAdd places an item's suggestion: before old line NoteLine (PlaceAfter) or over old line
// NoteLine (PlaceInstead). A note line past the last source line appends the suggestion.
func patchAdd(item Item, count int, added [][]string, replaced []string, replaces []bool) {
	position := item.NoteLine
	if item.Place == PlaceInstead {
		position--
	}
	if position < 0 {
		return
	}
	instead := item.Place == PlaceInstead && position < count
	if position >= count || !instead {
		added[min(position, count)] = append(added[min(position, count)], item.Suggestion)
		return
	}
	replaced[position], replaces[position] = item.Suggestion, true
}

// crlfLines ends each added and replacing line with "\r": a file written with CRLF (on Windows)
// keeps them on the lines the patch adds, as its own lines (read up to "\n") carry theirs.
func crlfLines(added [][]string, replaced []string, replaces []bool) {
	for _, lines := range added {
		for index := range lines {
			lines[index] += "\r"
		}
	}
	for index := range replaced {
		if replaces[index] {
			replaced[index] += "\r"
		}
	}
}

// keepLastLine: a line appended after a last line that has no newline changes that line's ending,
// so the diff deletes and re-adds it before the added line (the standard no-newline marker).
func keepLastLine(old []string, noEOL bool, added [][]string, replaced []string, replaces []bool) {
	last := len(old) - 1
	if !noEOL || last < 0 || len(added[len(old)]) == 0 || replaces[last] {
		return
	}
	replaced[last], replaces[last] = old[last], true
}

// sourceLines is the file's lines without their newline, and whether its last line has none.
func sourceLines(source string) ([]string, bool) {
	if source == "" {
		return nil, false
	}
	return strings.Split(strings.TrimSuffix(source, "\n"), "\n"), !strings.HasSuffix(source, "\n")
}

// patchNewBefore is the new line index before each old line boundary.
func patchNewBefore(old []string, added [][]string) []int {
	before := make([]int, len(old)+1)
	for index := range old {
		before[index+1] = before[index] + len(added[index]) + 1
	}
	return before
}

// patchSpans is each replaced line and each insertion, in old-line order.
func patchSpans(count int, added [][]string, replaces []bool) []patchSpan {
	spans := []patchSpan{}
	for index := 0; index < count; index++ {
		if replaces[index] {
			spans = append(spans, patchSpan{index, index + 1})
		}
	}
	for index := 0; index <= count; index++ {
		if len(added[index]) > 0 {
			spans = append(spans, patchSpan{index, index})
		}
	}
	sort.SliceStable(spans, func(a, b int) bool { return spans[a].start < spans[b].start })
	return spans
}

// patchGroups widens each change by patchContext lines and merges the hunks whose context overlaps.
func patchGroups(spans []patchSpan, count int) [][2]int {
	groups := [][2]int{}
	for _, span := range spans {
		start, end := max(0, span.start-patchContext), min(count, span.end+patchContext)
		if last := len(groups) - 1; last >= 0 && start <= groups[last][1] {
			groups[last][1] = max(groups[last][1], end)
			continue
		}
		groups = append(groups, [2]int{start, end})
	}
	return groups
}

// patchHunk is the body of the hunk over group and how many lines it adds to the new file.
func patchHunk(old []string, added [][]string, replaced []string, replaces []bool, before []int, group [2]int) ([]patchBody, int) {
	body := []patchBody{}
	addedLines := 0
	for position := group[0]; position < group[1]; position++ {
		for offset, line := range added[position] {
			body = append(body, patchBody{'+', line, -1, before[position] + offset})
			addedLines++
		}
		if replaces[position] {
			body = append(body, patchBody{'-', old[position], position, -1})
			body = append(body, patchBody{'+', replaced[position], -1, before[position] + len(added[position])})
			addedLines++
			continue
		}
		body = append(body, patchBody{' ', old[position], position, before[position] + len(added[position])})
		addedLines++
	}
	if group[1] == len(old) {
		for offset, line := range added[len(old)] {
			body = append(body, patchBody{'+', line, -1, before[len(old)] + offset})
			addedLines++
		}
	}
	return body, addedLines
}

// patchWriteBody writes a hunk's lines with the standard marker for a file that ends without a
// newline, once for an unchanged last line and on each side when the last line changes.
func patchWriteBody(out *strings.Builder, body []patchBody, oldTotal, newTotal int, noEOL bool) {
	for _, line := range body {
		out.WriteByte(line.prefix)
		out.WriteString(line.text)
		out.WriteByte('\n')
		if noEOL && line.oldIndex == oldTotal-1 {
			out.WriteString("\\ No newline at end of file\n")
		} else if noEOL && line.newIndex == newTotal-1 {
			out.WriteString("\\ No newline at end of file\n")
		}
	}
}
