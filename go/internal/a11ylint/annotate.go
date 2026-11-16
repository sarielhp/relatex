package a11ylint

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/figdesc"
)

// MarkPrefix starts every comment line the annotated copy adds, so the author greps for it.
const MarkPrefix = "%% relatex-a11y:"

// Mark is one item of the lint placed in the annotated copy: its number, the line of its comment
// in the copy (0 when the item has no place in the source, such as a drawing named only by the
// last conversion) and the placeholder written into the source ("" for a comment alone).
type Mark struct {
	Item
	Number      int    `json:"number"`
	CopyLine    int    `json:"copy_line"`
	Placeholder string `json:"placeholder,omitempty"`
}

// edit is a text inserted at an offset of a file's source; order keeps the items' order among
// insertions at the same offset.
type edit struct {
	offset int
	text   string
	order  int
	mark   *Mark // the mark whose comment line this is, nil for a placeholder
}

// annotation is what Annotate needs besides the items: the sidecar's drafts and whether
// \includegraphics takes graphicx's keys in this paper.
type annotation struct {
	sidecar  *figdesc.Sidecar
	keys     bool
	commands *regexp.Regexp // \includegraphics and the commands passing their options on to it
}

// annotateFile is file's source with the comment line of each mark before its line and each
// figure's placeholder in its \includegraphics; it fills in the marks' copy lines and placeholders.
func annotateFile(file *File, marks []*Mark, context annotation) string {
	edits := []edit{}
	placed := map[int]bool{} // one placeholder per call, though several marks (pages of one file) name it
	for index, mark := range marks {
		if mark.Line <= 0 || mark.Line > len(file.lines) {
			continue
		}
		// The placeholder first: it changes the fix the comment states.
		if placeholder, ok := placeholderEdit(file, mark, context); ok && !placed[placeholder.offset] {
			placed[placeholder.offset] = true
			placeholder.order = 3*index + 1
			edits = append(edits, placeholder)
		}
		edits = append(edits, edit{offset: file.lines[mark.Line-1], text: commentLines(mark), order: 3 * index, mark: mark})
		if suggestion, ok := suggestionEdit(file, mark); ok {
			suggestion.order = 3*(index-len(marks)) + 2 // before the next line's own marks
			edits = append(edits, suggestion)
		}
	}
	slices.SortStableFunc(edits, func(a, b edit) int {
		return cmp.Or(cmp.Compare(a.offset, b.offset), cmp.Compare(a.order, b.order))
	})
	var out strings.Builder
	position, added := 0, 0
	for _, change := range edits {
		out.WriteString(file.Source[position:change.offset])
		position = change.offset
		if change.mark != nil {
			change.mark.CopyLine = change.mark.Line + added
		}
		out.WriteString(withLineEnds(change.text, file.Source))
		added += strings.Count(change.text, "\n")
	}
	out.WriteString(file.Source[position:])
	return out.String()
}

// Places of an item's suggestion in the annotated copy, commented out after its note line: a
// line to add there, or a line to replace the note line by.
const (
	PlaceAfter   = "after"
	PlaceInstead = "instead"
)

// commentLines is a mark's comment line: comment lines change nothing TeX reads, so the copy
// typesets as the original.
func commentLines(mark *Mark) string {
	return fmt.Sprintf("%s %s %d — %s. Fix: %s\n", MarkPrefix, mark.Kind, mark.Number, oneLine(mark.Message), oneLine(mark.Fix))
}

// suggestionEdit is a mark's suggestion, commented out, on a line of its own after its note
// line: the author uncomments it (and, for one replacing the note line, deletes that line).
func suggestionEdit(file *File, mark *Mark) (edit, bool) {
	if mark.Place == "" || mark.Suggestion == "" || mark.NoteLine <= 0 || mark.NoteLine > len(file.lines) {
		return edit{}, false
	}
	hint := "uncomment to add it"
	if mark.Place == PlaceInstead {
		hint = "uncomment it in place of the line above"
	}
	text := fmt.Sprintf("%% %s  %% relatex-a11y %d: %s\n", mark.Suggestion, mark.Number, hint)
	if mark.NoteLine == len(file.lines) {
		if !strings.HasSuffix(file.Source, "\n") {
			text = "\n" + text
		}
		return edit{offset: len(file.Source), text: text}, true
	}
	return edit{offset: file.lines[mark.NoteLine], text: text}, true
}

// withLineEnds is inserted text with the line ends of the source it goes into: CRLF when the
// author's file has them (written on Windows, decision 129), so the file keeps one convention.
func withLineEnds(text, source string) string {
	if !strings.Contains(source, "\r\n") {
		return text
	}
	return strings.ReplaceAll(text, "\n", "\r\n")
}

func oneLine(text string) string {
	return strings.TrimRight(strings.Join(strings.Fields(text), " "), ".")
}

// placeholderEdit is a figure's alt key in its \includegraphics: a TODO, or the model's draft from
// the sidecar marked DRAFT. None when the graphic already has an alt key or the paper loads graphics
// without graphicx (no keys; the lint itself skips the bounding-box form [x,y][x,y]).
func placeholderEdit(file *File, mark *Mark, context annotation) (edit, bool) {
	if mark.Target == "" || mark.Offset < 0 || !context.keys || (mark.Kind != "figure" && mark.Kind != "draft") {
		return edit{}, false
	}
	value, fix := TodoPrefix+" describe "+escapeTeX(mark.Target), todoFix
	if entry := context.sidecar.Entries[mark.Key]; mark.Kind == "draft" && entry != nil {
		value, fix = DraftPrefix+" "+escapeTeX(entry.Alt), draftFix
	}
	key := "alt={" + value + "}"
	match := context.commands.FindStringIndex(file.Text[mark.Offset:])
	if match == nil || match[0] != 0 {
		return edit{}, false
	}
	end := mark.Offset + match[1]
	options, after, has := argument(file.Text, end, '[', ']')
	if !has {
		mark.Placeholder, mark.Fix = key, fix
		return edit{offset: end, text: "[" + key + "]"}, true
	}
	if _, given := keyValues(options)["alt"]; given {
		return edit{}, false
	}
	mark.Placeholder, mark.Fix = key, fix
	if strings.TrimSpace(options) != "" {
		key = ", " + key
	}
	return edit{offset: after - 1, text: key}, true
}

// texEscapes turn a plain text into TeX that typesets (and purifies) to it.
var texEscapes = strings.NewReplacer(`\`, `\textbackslash{}`, `{`, `\{`, `}`, `\}`, `%`, `\%`, `#`, `\#`, `&`, `\&`,
	`$`, `\$`, `_`, `\_`, `~`, `\textasciitilde{}`, `^`, `\textasciicircum{}`)

func escapeTeX(text string) string {
	return texEscapes.Replace(strings.Join(strings.Fields(text), " "))
}

var packageUse = regexp.MustCompile(`\\(?:usepackage|RequirePackage)\s*(?:\[[^\]]*\])?\s*\{([^}]*)\}`)

// takesKeys: \includegraphics accepts graphicx's keys unless the paper loads the graphics package
// and never graphicx (a class may load graphicx itself, so its absence alone says nothing).
func takesKeys(files []*File) bool {
	graphics, graphicx := false, false
	for _, file := range files {
		for _, match := range packageUse.FindAllStringSubmatch(file.Text, -1) {
			for _, name := range strings.Split(match[1], ",") {
				graphics = graphics || strings.TrimSpace(name) == "graphics"
				graphicx = graphicx || strings.TrimSpace(name) == "graphicx"
			}
		}
	}
	return graphicx || !graphics
}
