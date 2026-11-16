package bibclean

import (
	"fmt"
	"os"
	"strings"
)

// Status is the conversion outcome for one bibliography entry.
type Status string

const (
	// StatusConverted means all fields of the entry converted cleanly to Typst-ready markup.
	StatusConverted Status = "converted"
	// StatusUnconverted means the entry retains LaTeX that could not be safely converted.
	StatusUnconverted Status = "unconverted"
)

// Input is one in-memory .bib source.
type Input struct {
	Name string
	Data string
}

// Options controls which entries CleanFiles and CleanInputs select.
type Options struct {
	Keys []string
	All  bool
	// Math, when set, writes every inline formula of a printed field (decision 172, the plan's decision A).
	Math MathRoute
}

// MathRoute writes one inline formula of an entry another way: it gets the entry's key, the formula's
// index in the entry (fields in .bib order, from 0) and its TeX body, and returns the markup that replaces
// the whole $...$, or false to keep the cleaner's own reading of it.
type MathRoute func(key string, index int, tex string) (string, bool)

// Reason explains why an entry could not be cleanly converted.
type Reason struct {
	Kind   string `json:"kind"`
	Name   string `json:"name,omitempty"`
	Field  string `json:"field,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// EntryReport records the cleaning result for one included entry.
type EntryReport struct {
	Key     string   `json:"key"`
	Type    string   `json:"type"`
	Status  Status   `json:"status"`
	Reasons []Reason `json:"reasons,omitempty"`
}

// Report summarizes a bibclean run.
type Report struct {
	Converted      int           `json:"converted"`
	Unconverted    int           `json:"unconverted"`
	Entries        []EntryReport `json:"entries"`
	MissingKeys    []string      `json:"missing_keys,omitempty"`
	MissingParents []string      `json:"missing_parents,omitempty"`
}

// CleanFiles reads one or more .bib files and cleans the selected entries (plus needed crossref/xdata parents).
func CleanFiles(paths []string, opts Options) (string, Report, error) {
	inputs := make([]Input, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", Report{}, fmt.Errorf("read %s: %w", path, err)
		}
		inputs = append(inputs, Input{Name: path, Data: string(data)})
	}
	return CleanInputs(inputs, opts)
}

// CleanInputs parses and cleans one or more in-memory .bib inputs for the selected entries.
func CleanInputs(inputs []Input, opts Options) (string, Report, error) {
	env := newMacroEnv()
	entries, byLowerKey, err := collectInputs(inputs, env)
	if err != nil {
		return "", Report{}, err
	}
	selected, missingKeys, missingParents := selectEntries(entries, byLowerKey, env, opts)
	var out strings.Builder
	report := Report{
		Entries:        make([]EntryReport, 0, len(selected)),
		MissingKeys:    missingKeys,
		MissingParents: missingParents,
	}
	for i, entry := range selected {
		cleaned, er := cleanEntry(entry, env, opts.Math)
		if er.Status == StatusConverted {
			report.Converted++
		} else {
			report.Unconverted++
		}
		report.Entries = append(report.Entries, er)
		if i > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(cleaned)
	}
	return out.String(), report, nil
}

func collectInputs(inputs []Input, env *macroEnv) ([]Entry, map[string]int, error) {
	var entries []Entry
	byLowerKey := map[string]int{}
	for _, in := range inputs {
		parsed, err := parseBib(in.Name, in.Data)
		if err != nil {
			return nil, nil, err
		}
		for _, def := range parsed.strings {
			env.addString(def.name, def.parts)
		}
		for _, pre := range parsed.preambles {
			env.addPreamble(pre)
		}
		for _, entry := range parsed.entries {
			lowerKey := strings.ToLower(entry.Key)
			if _, exists := byLowerKey[lowerKey]; exists {
				continue
			}
			byLowerKey[lowerKey] = len(entries)
			entries = append(entries, entry)
		}
	}
	return entries, byLowerKey, nil
}

func selectEntries(entries []Entry, byLowerKey map[string]int, env *macroEnv, opts Options) ([]Entry, []string, []string) {
	included := make([]bool, len(entries))
	var queue []int
	var missingKeys []string
	if opts.All {
		for i := range entries {
			included[i] = true
			queue = append(queue, i)
		}
	} else {
		seenReq := map[string]bool{}
		for _, key := range opts.Keys {
			trimmed := strings.TrimSpace(key)
			if trimmed == "" {
				continue
			}
			lower := strings.ToLower(trimmed)
			if seenReq[lower] {
				continue
			}
			seenReq[lower] = true
			idx, ok := byLowerKey[lower]
			if !ok {
				missingKeys = append(missingKeys, trimmed)
				continue
			}
			if !included[idx] {
				included[idx] = true
				queue = append(queue, idx)
			}
		}
	}
	missingParents := expandParentRefs(entries, byLowerKey, env, included, queue)
	var selected []Entry
	for i, entry := range entries {
		if included[i] {
			selected = append(selected, entry)
		}
	}
	return selected, missingKeys, missingParents
}

func expandParentRefs(entries []Entry, byLowerKey map[string]int, env *macroEnv, included []bool, queue []int) []string {
	var missingParents []string
	seenMissing := map[string]bool{}
	for head := 0; head < len(queue); head++ {
		entry := entries[queue[head]]
		for _, ref := range entryParentRefs(entry, env) {
			lower := strings.ToLower(ref)
			idx, ok := byLowerKey[lower]
			if !ok {
				if !seenMissing[lower] {
					seenMissing[lower] = true
					missingParents = append(missingParents, ref)
				}
				continue
			}
			if !included[idx] {
				included[idx] = true
				queue = append(queue, idx)
			}
		}
	}
	return missingParents
}

func entryParentRefs(entry Entry, env *macroEnv) []string {
	var refs []string
	for _, field := range entry.Fields {
		if field.Name != "crossref" && field.Name != "xdata" {
			continue
		}
		expanded, _ := env.expandParts(field.Parts, field.Name)
		if field.Name == "crossref" {
			if k := strings.TrimSpace(expanded); k != "" {
				refs = append(refs, k)
			}
			continue
		}
		for _, part := range strings.Split(expanded, ",") {
			if k := strings.TrimSpace(part); k != "" {
				refs = append(refs, k)
			}
		}
	}
	return refs
}

func cleanEntry(entry Entry, env *macroEnv, route MathRoute) (string, EntryReport) {
	var b strings.Builder
	b.WriteByte('@')
	b.WriteString(entry.Type)
	b.WriteByte('{')
	b.WriteString(entry.Key)
	b.WriteString(",\n")

	var reasons []Reason
	var hook *mathHook
	if route != nil {
		hook = &mathHook{route: route, key: entry.Key}
	}
	for i, field := range entry.Fields {
		expanded, macroReasons := env.expandParts(field.Parts, field.Name)
		reasons = append(reasons, macroReasons...)
		cleaned, textReasons := cleanFieldText(env, field.Name, expanded, hook)
		reasons = append(reasons, textReasons...)

		b.WriteString("  ")
		b.WriteString(field.Name)
		b.WriteString(" = {")
		b.WriteString(cleaned)
		b.WriteByte('}')
		if i+1 < len(entry.Fields) {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("}\n")

	reasons = dedupReasons(reasons)
	status := StatusConverted
	if len(reasons) > 0 {
		status = StatusUnconverted
	}
	return b.String(), EntryReport{
		Key:     entry.Key,
		Type:    entry.Type,
		Status:  status,
		Reasons: reasons,
	}
}

func dedupReasons(reasons []Reason) []Reason {
	if len(reasons) <= 1 {
		return reasons
	}
	seen := make(map[Reason]bool, len(reasons))
	out := make([]Reason, 0, len(reasons))
	for _, r := range reasons {
		if seen[r] {
			continue
		}
		seen[r] = true
		out = append(out, r)
	}
	return out
}
