package figscan

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// TestCorpus scans every page of every PDF in $FIGSCAN_CORPUS (a figs/ directory) and reports
// sources, failures, node counts and the time taken. It is a survey, skipped without the variable.
func TestCorpus(t *testing.T) {
	dir := os.Getenv("FIGSCAN_CORPUS")
	if dir == "" {
		t.Skip("FIGSCAN_CORPUS not set")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.pdf"))
	sort.Strings(files)
	survey := &corpusSurvey{counts: map[string]int{}}
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		survey.file(t, filepath.Base(path), data)
	}
	t.Logf("files %d: %v; pages %d, shape nodes %d (most %d), labels %d, pages over budget %d; time %v, slowest page %v",
		len(files), survey.counts, survey.pages, survey.nodes, survey.most, survey.labels, survey.cut,
		survey.elapsed, survey.slowest)
}

type corpusSurvey struct {
	counts                          map[string]int
	pages, nodes, most, labels, cut int
	elapsed, slowest                time.Duration
}

func (s *corpusSurvey) file(t *testing.T, name string, data []byte) {
	for page := 1; ; page++ {
		start := time.Now()
		figure, err := Scan(data, page, nil)
		took := time.Since(start)
		if err != nil {
			if page == 1 {
				s.counts["error"]++
				t.Logf("%s page %d: %v", name, page, err)
			}
			return
		}
		s.counts[figure.Source]++
		s.elapsed += took
		s.slowest = max(s.slowest, took)
		_, _ = figure.Text(), figure.JSON()
		s.pages++
		s.nodes += figure.Total
		s.most = max(s.most, figure.Total)
		s.labels += len(figure.Labels)
		if figure.Shown < figure.Total {
			s.cut++
		}
		if os.Getenv("FIGSCAN_VERBOSE") != "" {
			t.Logf("%s page %d: %s, %d shapes, %d labels, %v", name, page, figure.Source, figure.Total, len(figure.Labels), took)
		}
	}
}
