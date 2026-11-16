package formulatex

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"github.com/sarielhp/relatex/internal/authormath"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/texsandbox"
)

// Summary counts a document's formulas by the round trip's outcome. Failures are by class: the
// construct the serializer does not write, "not typeset", or where the MathML first differs.
type Summary struct {
	Formulas int
	Verified int
	Typeset  int // formulas typeset in this run; the rest came from the cache
	Failures map[string]int
	Seconds  float64

	typeset map[string]string // a failed formula's typeset MathML by its MathML, for reports
}

// result is one formula's outcome, as the cache keeps it.
type result struct {
	Status      string `json:"status"` // "verified" or "failed"
	Text        string `json:"text,omitempty"`
	Environment string `json:"environment,omitempty"`
	Display     bool   `json:"display,omitempty"`
	Class       string `json:"class,omitempty"`
	typeset     string // the MathML the round trip made, kept for reports only
}

// pending is a formula written but not yet proven, with the nodes that share its MathML.
type pending struct {
	key     string
	mathml  string
	formula Formula
}

// Check gives each math node whose TeX round-trips its formula_tex: the TeX written from its
// MathML, typeset again with Preamble alone in one LuaLaTeX run, and compared with the node's
// MathML by decision 064's projection. Formulas known from the cache are not typeset again.
func Check(nodes []rec.Map, work, engine string) Summary {
	started := time.Now()
	summary := Summary{Failures: map[string]int{}, typeset: map[string]string{}}
	doctor := texsandbox.Current(engine)
	prefix := keyPrefix(doctor)
	outcomes := map[string]result{}
	candidates := []pending{}
	for _, node := range nodes {
		mathml, ok := node["mathml"].(string)
		if node["type"] != "math" || !ok || mathml == "" {
			continue
		}
		key := cacheKey(prefix, mathml)
		if _, seen := outcomes[key]; seen {
			continue
		}
		if cached, found := readCache(key); found {
			outcomes[key] = cached
			continue
		}
		formula, err := Write(mathml)
		if err != nil {
			outcomes[key] = result{Status: "failed", Class: err.Error()}
			writeCache(key, outcomes[key])
			continue
		}
		outcomes[key] = result{Status: "pending"}
		candidates = append(candidates, pending{key: key, mathml: mathml, formula: formula})
	}
	for key, outcome := range roundTrip(candidates, work, doctor) {
		outcomes[key] = outcome
		writeCache(key, outcome)
	}
	summary.Typeset = len(candidates)
	for _, node := range nodes {
		mathml, ok := node["mathml"].(string)
		if node["type"] != "math" || !ok || mathml == "" {
			continue
		}
		summary.Formulas++
		outcome := outcomes[cacheKey(prefix, mathml)]
		if outcome.Status != "verified" {
			summary.Failures[outcome.Class]++
			if outcome.typeset != "" {
				summary.typeset[mathml] = outcome.typeset
			}
			continue
		}
		summary.Verified++
		node["formula_tex"] = texField(outcome)
	}
	summary.Seconds = time.Since(started).Seconds()
	return summary
}

func texField(outcome result) rec.Map {
	field := rec.Map{"text": outcome.Text}
	if outcome.Environment != "" {
		field["environment"] = outcome.Environment
	}
	return field
}

// roundTrip typesets the candidates in one run and judges each against its MathML.
func roundTrip(candidates []pending, work string, doctor *texsandbox.Doctor) map[string]result {
	outcomes := map[string]result{}
	if len(candidates) == 0 {
		return outcomes
	}
	if err := os.MkdirAll(work, 0o755); err != nil {
		rec.Fail("%v", err)
	}
	ids := make([]string, len(candidates))
	for index, item := range candidates {
		ids[index] = authormath.CandidateID(index)
		if err := os.WriteFile(filepath.Join(work, ids[index]+".tex"), []byte(candidateSource(item.formula)), 0o644); err != nil {
			rec.Fail("%v", err)
		}
	}
	typesetter := &authormath.Typesetter{Directory: work, Header: Preamble, Doctor: doctor, Attempt: "formula-tex"}
	typeset := typesetter.Typeset(ids)
	for index, item := range candidates {
		outcomes[item.key] = judge(item, typeset[ids[index]])
	}
	return outcomes
}

// judge compares a candidate's typeset MathML with its own: formulas nested in it (a \text's
// math, a \left's measured delimiter) come before it, so the last one is the candidate's.
func judge(item pending, typeset any) result {
	list, _ := typeset.([]any)
	if len(list) == 0 {
		return result{Status: "failed", Class: "not typeset"}
	}
	last, _ := list[len(list)-1].(string)
	if difference := authormath.Difference(item.mathml, last); difference != "" {
		return result{Status: "failed", Class: differenceClass(difference), typeset: last}
	}
	return result{Status: "verified", Text: item.formula.Text, Environment: item.formula.Environment, Display: item.formula.Display}
}

var (
	positions = regexp.MustCompile(`\[\d+\]`)
	quoted    = regexp.MustCompile(`"(?:[^"\\]|\\.)*"`)
)

// differenceClass is a difference without its positions and texts: where the trees differ.
func differenceClass(difference string) string {
	return "differs " + quoted.ReplaceAllString(positions.ReplaceAllString(difference, ""), `"…"`)
}

// Classes is the summary's failure classes, the most frequent first.
func (s Summary) Classes() []string {
	classes := make([]string, 0, len(s.Failures))
	for class := range s.Failures {
		classes = append(classes, class)
	}
	sort.Slice(classes, func(i, j int) bool {
		if s.Failures[classes[i]] != s.Failures[classes[j]] {
			return s.Failures[classes[i]] > s.Failures[classes[j]]
		}
		return classes[i] < classes[j]
	})
	return classes
}
