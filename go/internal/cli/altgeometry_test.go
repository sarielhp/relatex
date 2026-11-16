package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/figscan"
	"github.com/sarielhp/relatex/internal/mediamodel"
	"github.com/sarielhp/relatex/internal/rec"
)

func TestAltGeometryKeyReadsTheFiguresPage(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "figscan", "testdata", "balanced.pdf"))
	if os.IsNotExist(err) {
		t.Skip("the figure fixtures are not in this tree (they are not published)")
	}
	if err != nil {
		t.Fatal(err)
	}
	original := mediamodel.Encoded(data, "application/pdf")
	sha := original["sha256"].(string)
	asset := rec.Map{"schema_version": mediamodel.MediaVersion, "sha256": sha, "representations": rec.Map{"original": original,
		"html": mediamodel.Encoded([]byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), "image/svg+xml")}}
	document := rec.Map{"assets": rec.Map{"items": rec.Map{sha: asset}}, "block_order": []any{"p1"},
		"nodes": rec.Map{"p1": rec.Map{"id": "p1", "type": "paragraph", "content": []any{
			rec.Map{"type": "image", "id": "i1", "asset": sha, "source": "figs/balanced.pdf"}}}}}
	direct, err := figscan.Scan(data, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	keyed := scanKey("balanced", document, "out")
	if keyed.Text() != direct.Text() || keyed.Source != figscan.SourceIpe {
		t.Errorf("the KEY form differs from the --pdf form:\n%s\n---\n%s", keyed.Text(), direct.Text())
	}
	if got := failure(func() { scanKey("missing", document, "out") }); !strings.Contains(got, "no figure missing") {
		t.Errorf("unknown key: %q", got)
	}
}
