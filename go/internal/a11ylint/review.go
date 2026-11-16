package a11ylint

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/sarielhp/relatex/internal/figdesc"
)

// ReviewFile is the summary the annotated copy carries at its top level.
const ReviewFile = "REVIEW.md"

// reviewTitle starts ReviewFile; a destination holding such a file is an earlier copy, replaced.
const reviewTitle = "# Accessibility review (relatex)"

// Annotate writes the annotated review copy (decision 098): the paper directory copied to
// destination (never changing the original; .git and the skipped directories left out), each
// item marked before its line by a MarkPrefix comment, each figure given an alt placeholder, and
// ReviewFile listing the marks with their places in the copy.
func Annotate(paper Paper, items []Item, directory, destination string, skip []string) ([]Mark, error) {
	if err := prepareDestination(directory, destination); err != nil {
		return nil, err
	}
	if err := copyTree(directory, destination, skip); err != nil {
		return nil, err
	}
	marks := make([]Mark, len(items))
	byFile := map[string][]*Mark{}
	for index, item := range items {
		marks[index] = Mark{Item: item, Number: index + 1}
		byFile[item.File] = append(byFile[item.File], &marks[index])
	}
	context := annotation{sidecar: paper.Sidecar, keys: takesKeys(paper.Files), commands: graphicCommands(macros(paper.Files))}
	if context.sidecar == nil {
		context.sidecar = &figdesc.Sidecar{Entries: map[string]*figdesc.Entry{}}
	}
	for _, file := range paper.Files {
		if len(byFile[file.Path]) == 0 {
			continue
		}
		text := annotateFile(file, byFile[file.Path], context)
		if err := os.WriteFile(filepath.Join(destination, file.Path), []byte(text), 0o644); err != nil {
			return nil, err
		}
	}
	return marks, os.WriteFile(filepath.Join(destination, ReviewFile), []byte(reviewSummary(directory, marks)), 0o644)
}

// prepareDestination refuses the paper directory or a directory holding it, and an existing
// directory with other content than an earlier review copy; an earlier copy is removed.
func prepareDestination(directory, destination string) error {
	source, err := filepath.Abs(directory)
	if err != nil {
		return err
	}
	target, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	if relative, relErr := filepath.Rel(target, source); relErr == nil && (relative == "." || !strings.HasPrefix(relative, "..")) {
		return fmt.Errorf("the review copy %s cannot be the paper directory or hold it", destination)
	}
	entries, err := os.ReadDir(target)
	if errors.Is(err, fs.ErrNotExist) || err == nil && len(entries) == 0 {
		return os.MkdirAll(target, 0o755)
	}
	if err != nil {
		return err
	}
	summary, err := os.ReadFile(filepath.Join(target, ReviewFile))
	if err != nil || !strings.HasPrefix(string(summary), reviewTitle) {
		return fmt.Errorf("%s exists and is not an earlier review copy; name a new directory", destination)
	}
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	return os.MkdirAll(target, 0o755)
}

// copyTree copies the regular files, directories and symbolic links under directory to
// destination, leaving out .git, destination itself and the skipped directories.
func copyTree(directory, destination string, skip []string) error {
	excluded := map[string]bool{}
	for _, path := range append(skip, destination) {
		if absolute, err := filepath.Abs(path); err == nil {
			excluded[absolute] = true
		}
	}
	return filepath.WalkDir(directory, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		absolute, _ := filepath.Abs(path)
		if entry.IsDir() && (excluded[absolute] || entry.Name() == ".git") {
			return filepath.SkipDir
		}
		relative, err := filepath.Rel(directory, path)
		if err != nil {
			return err
		}
		return copyEntry(path, filepath.Join(destination, relative), entry)
	})
}

func copyEntry(source, target string, entry fs.DirEntry) error {
	switch {
	case entry.IsDir():
		return os.MkdirAll(target, 0o755)
	case entry.Type()&fs.ModeSymlink != 0:
		link, err := os.Readlink(source)
		if err != nil {
			return err
		}
		// Windows creates symbolic links only in developer mode or as an administrator: the copy
		// then holds the target's content (decision 129).
		if err := os.Symlink(link, target); err == nil || runtime.GOOS != "windows" {
			return err
		}
	case !entry.Type().IsRegular():
		return nil
	}
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// reviewSummary is ReviewFile: how to use the copy, then each mark with its place in the copy.
func reviewSummary(directory string, marks []Mark) string {
	var out strings.Builder
	out.WriteString(reviewTitle + "\n\n")
	fmt.Fprintf(&out, "An annotated copy of `%s`, written by `relatex lint --accessibility --annotate`; the original is "+
		"unchanged. Each place below is marked in the copy by a comment line starting `%s`. A figure carries "+
		"`alt={%s …}` to replace by its description, or `alt={%s …}` holding a model's draft to check, correct "+
		"and keep without the prefix. A line to add (the language, the rule under a table's header row, a picture's "+
		"alt key) is written commented out under its place, ending `%% relatex-a11y N: uncomment …`. Comment lines and alt keys change nothing on the page, so the copy typesets "+
		"as the original. When done, rebuild and run `relatex lint --accessibility` on the copy: it lists what is "+
		"left, placeholders included.\n\n", filepath.ToSlash(directory), MarkPrefix, TodoPrefix, DraftPrefix)
	if len(marks) == 0 {
		out.WriteString("Nothing for the author to supply was found in the source.\n")
		return out.String()
	}
	fmt.Fprintf(&out, "%d marks.\n\n", len(marks))
	for _, mark := range marks {
		place := mark.File + " (no line in the source)"
		if mark.CopyLine > 0 {
			place = fmt.Sprintf("%s:%d", mark.File, mark.CopyLine)
		}
		fmt.Fprintf(&out, "%d. `%s` %s: %s\n", mark.Number, place, mark.Kind, oneLine(mark.Message))
		if mark.Placeholder != "" {
			fmt.Fprintf(&out, "   - placeholder: `%s`\n", mark.Placeholder)
		}
		fmt.Fprintf(&out, "   - fix: %s\n", oneLine(mark.Fix))
		if mark.Note != "" {
			fmt.Fprintf(&out, "   - note (line %d of the original): %s\n", mark.NoteLine, oneLine(mark.Note))
		}
	}
	return out.String()
}
