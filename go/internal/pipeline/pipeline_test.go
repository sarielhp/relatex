package pipeline

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
)

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func failure(t *testing.T, run func()) (message string) {
	t.Helper()
	defer func() {
		if value := recover(); value != nil {
			message = value.(rec.Failure).Message
		}
	}()
	run()
	return ""
}

func TestCompressionRoundTrip(t *testing.T) {
	if command.Executable("zstd") == "" {
		t.Skip("zstd not installed")
	}
	root := t.TempDir()
	large := strings.Repeat("relatex ", 4096)
	write(t, filepath.Join(root, "a/large.txt"), large)
	write(t, filepath.Join(root, "small.txt"), "small")
	write(t, filepath.Join(root, "figure.pdf"), large)
	CompressAtRest([]string{root})
	if isFile(filepath.Join(root, "a/large.txt")) || !isFile(filepath.Join(root, "a/large.txt.zst")) {
		t.Fatal("large file not compressed")
	}
	if !isFile(filepath.Join(root, "small.txt")) || !isFile(filepath.Join(root, "figure.pdf")) {
		t.Fatal("small or already compressed file changed")
	}
	ExpandAtRest(root)
	data, err := os.ReadFile(filepath.Join(root, "a/large.txt"))
	if err != nil || string(data) != large || isFile(filepath.Join(root, restManifest)) {
		t.Fatal("expansion did not restore the file")
	}
}

func TestStageCacheMissHitForceAndResume(t *testing.T) {
	cache := NewStageCache(filepath.Join(t.TempDir(), "cache"), "", "")
	key := strings.Repeat("a", 64)
	runs := 0
	produce := func(directory string) any {
		runs++
		write(t, filepath.Join(directory, "out.txt"), "product")
		return ojson.Object{{"made", true}}
	}
	first := cache.stage(key, filepath.Join(t.TempDir(), "one"), "layout", false, "", produce)
	second := cache.stage(key, filepath.Join(t.TempDir(), "two"), "layout", false, "", produce)
	forcedRun := cache.stage(key, filepath.Join(t.TempDir(), "three"), "layout", true, "", produce)
	if first.cache.Get("status") != "miss" || second.cache.Get("status") != "hit" || forcedRun.cache.Get("status") != "miss" || runs != 2 {
		t.Fatalf("statuses %v %v %v runs %d", first.cache.Get("status"), second.cache.Get("status"), forcedRun.cache.Get("status"), runs)
	}
	if forcedRun.cache.Get("reason") != "explicitly forced" {
		t.Fatalf("reason %v", forcedRun.cache.Get("reason"))
	}
	if !rec.Equal(ojson.Plain(second.result), rec.Map{"made": true}) {
		t.Fatalf("result %v", second.result)
	}
	// A previous attempt's completed stage is resumed when the cache lacks the entry.
	previous := t.TempDir()
	write(t, filepath.Join(previous, "out.txt"), "product")
	other := strings.Repeat("b", 64)
	metadata := cache.metadata(other, "layout", previous, ojson.Object{{"made", true}})
	writeOrdered(filepath.Join(previous, "stage.json"), ojson.Object{{"status", "complete"}, {"identity", other},
		{"cache_metadata", metadata}})
	isolated := NewStageCache(filepath.Join(t.TempDir(), "empty"), "", "")
	resumed := isolated.stage(other, filepath.Join(t.TempDir(), "four"), "layout", false, previous, produce)
	if resumed.cache.Get("status") != "resumed" || runs != 2 {
		t.Fatalf("resume status %v runs %d", resumed.cache.Get("status"), runs)
	}
}

func TestSettleKeepsTheRecentEntriesPerStage(t *testing.T) {
	root := filepath.Join(t.TempDir(), "cache")
	cache := NewStageCache(root, "", "")
	keys := []string{strings.Repeat("1", 64), strings.Repeat("2", 64), strings.Repeat("3", 64)}
	for index, key := range keys {
		cache.stage(key, filepath.Join(t.TempDir(), "run"), "layout", false, "", func(directory string) any {
			write(t, filepath.Join(directory, "out.txt"), "product")
			return ojson.Object{}
		})
		stamp := time.Now().Add(time.Duration(index-3) * time.Hour)
		if err := os.Chtimes(filepath.Join(root, key), stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	cache.Settle(2)
	if isDirectory(filepath.Join(root, keys[0])) || !isDirectory(filepath.Join(root, keys[1])) || !isDirectory(filepath.Join(root, keys[2])) {
		t.Fatal("settle kept the wrong entries")
	}
}

func TestForcedStagesIncludeTheirDependents(t *testing.T) {
	if got := forced([]string{"layout", "export.markdown"}); !slices.Equal(got,
		[]string{"layout", "synchronize", "export", "publish", "export.markdown"}) {
		t.Fatalf("forced %v", got)
	}
	for _, unknown := range []string{"nothing", "verify"} {
		if message := failure(t, func() { forced([]string{unknown}) }); message != "Unknown forced stage: "+unknown {
			t.Fatalf("forced(%q) message %q", unknown, message)
		}
	}
}
