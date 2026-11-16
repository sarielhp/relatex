package papersource

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/sarielhp/relatex/internal/frontmatter"
	"github.com/sarielhp/relatex/internal/rec"
)

func noticeRecords() []rec.Map {
	row := func(char string, position int64) rec.Map {
		return rec.Map{"code": "excluded_page_decoration", "char": char, "text": char, "candidate": int64(1),
			"position": position, "page": int64(1), "source": "acmart-copyright"}
	}
	return []rec.Map{{"kind": "candidate", "candidate": int64(1), "text": "ab", "glyphs": int64(2)}, row("a", 1), row("b", 2)}
}

func TestFailedFrontMatterKeepsPageDecorations(t *testing.T) {
	document := rec.Map{"identity": rec.Map{"input_sha256": "abc"}, "nodes": rec.Map{}}
	(&Source{}).keepPageDecorations(noticeRecords(), document)
	rows, ok := document["page_decorations"].([]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("page_decorations %v", document["page_decorations"])
	}
	if _, has := document["frontmatter"]; has {
		t.Error("front matter came back")
	}
	if got := frontmatter.Decorations(document); len(got) != 1 {
		t.Errorf("accessor %v", got)
	}
}

func TestNoDecorationRowsStoreNothing(t *testing.T) {
	document := rec.Map{"identity": rec.Map{"input_sha256": "abc"}, "nodes": rec.Map{}}
	(&Source{}).keepPageDecorations(nil, document)
	if _, has := document["page_decorations"]; has {
		t.Error("an empty list was stored")
	}
}

func TestUnreadableDecorationsStoreNothing(t *testing.T) {
	document := rec.Map{"identity": rec.Map{"input_sha256": "abc"}, "nodes": rec.Map{}}
	(&Source{}).keepPageDecorations(noticeRecords()[:2], document)
	if _, has := document["page_decorations"]; has {
		t.Error("decorations that do not validate were stored")
	}
}

// A source that declares no front matter at all (no \title the reader knows) still keeps its
// page headers and footers as page_decorations.
func TestUndeclaredFrontMatterKeepsPageDecorations(t *testing.T) {
	directory := t.TempDir()
	source := "\\documentclass{article}\n\\begin{document}\nText.\n\\end{document}\n"
	if err := os.MkdirAll(filepath.Join(directory, "input", "files"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "input", "files", "main.tex")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	directory, _ = filepath.EvalSymlinks(directory)
	sum := sha256.Sum256([]byte(source))
	s := &Source{directory: directory, main: "main.tex", manifest: rec.Map{"input/files/main.tex": hex.EncodeToString(sum[:])}}
	document := rec.Map{"identity": rec.Map{"input_sha256": hex.EncodeToString(sum[:])}, "nodes": rec.Map{}}
	s.projectFrontmatter(noticeRecords(), document)
	if _, has := document["frontmatter"]; has {
		t.Fatal("front matter typed from an undeclared source")
	}
	if rows, ok := document["page_decorations"].([]any); !ok || len(rows) != 1 {
		t.Errorf("page_decorations %v", document["page_decorations"])
	}
}
