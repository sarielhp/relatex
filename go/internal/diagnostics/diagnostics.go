// Package diagnostics is the Go port of Relatex::Diagnostics: compiler-style export
// diagnostics, each a stable code, a severity, a message and the affected element.
// profiles/diagnostics.json gives each code its default severity and, for a measured
// deviation, thresholds that choose the severity from its magnitude. Strict mode
// reproduces fail-closed export: anything above "whatever" stops the export.
package diagnostics

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/rec"
)

// Levels is Diagnostics::LEVELS, most severe first.
var Levels = []string{"fatal", "error", "alert", "warning", "whatever"}

// Version is Diagnostics::VERSION.
const Version = "diagnostics-1"

// Place is one key of an entry's location, in the order it was given.
type Place struct {
	Key   string
	Value any
}

// Diagnostics collects the entries of one export.
type Diagnostics struct {
	profile   rec.Map
	overrides map[string]string
	strict    bool
	entries   []rec.Map
	// orders keeps each entry's location keys in the order given, for Lines.
	orders [][]Place
}

// LoadProfile reads ROOT/profiles/diagnostics.json.
func LoadProfile(root string) (rec.Map, error) {
	file, err := os.Open(filepath.Join(root, "profiles", "diagnostics.json"))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	value, err := canonjson.Decode(file)
	if err != nil {
		return nil, err
	}
	profile, ok := value.(rec.Map)
	if !ok {
		return nil, fmt.Errorf("diagnostics profile is not an object")
	}
	return profile, nil
}

// New is Diagnostics.new(overrides:, strict:, profile:).
func New(profile rec.Map, overrides map[string]string, strict bool) *Diagnostics {
	for _, level := range overrides {
		if !slices.Contains(Levels, level) {
			rec.Fail("Unknown diagnostic level: %s", level)
		}
	}
	return &Diagnostics{profile: profile, overrides: overrides, strict: strict, entries: []rec.Map{}}
}

// Add is Diagnostics#add; magnitude is nil without one.
func (d *Diagnostics) Add(code, message string, magnitude any, location ...Place) rec.Map {
	level := d.Level(code, magnitude)
	if d.strict && level != "whatever" {
		rec.Fail("%s: %s", code, message)
	}
	entry := rec.Map{"code": code, "level": level, "message": message}
	if truthy(magnitude) {
		entry["magnitude"] = magnitude
	}
	if len(location) > 0 {
		places := rec.Map{}
		for _, place := range location {
			places[place.Key] = place.Value
		}
		entry["location"] = places
	}
	d.entries = append(d.entries, entry)
	d.orders = append(d.orders, location)
	return entry
}

func truthy(value any) bool { return value != nil && value != false }

// Level is Diagnostics#level.
func (d *Diagnostics) Level(code string, magnitude any) string {
	spec, ok := rec.FetchMap(d.profile, "codes")[code].(rec.Map)
	if !ok {
		rec.Fail("Unregistered diagnostic code: %s", code)
	}
	if level, overridden := d.overrides[code]; overridden {
		return level
	}
	if truthy(magnitude) {
		size, _ := rec.Number(magnitude)
		thresholds, _ := spec["thresholds"].([]any)
		for _, value := range thresholds {
			row := rec.AsMap(value)
			bound, _ := rec.Number(rec.Fetch(row, "up_to"))
			if size <= bound {
				return rec.FetchString(row, "level")
			}
		}
	}
	return rec.FetchString(spec, "level")
}

// Entries are the entries as JSON values.
func (d *Diagnostics) Entries() []any {
	result := make([]any, len(d.entries))
	for index, entry := range d.entries {
		result[index] = entry
	}
	return result
}

// Counts is Diagnostics#counts.
func (d *Diagnostics) Counts() rec.Map {
	counts := rec.Map{}
	for _, level := range Levels {
		count := int64(0)
		for _, entry := range d.entries {
			if entry["level"] == level {
				count++
			}
		}
		counts[level] = count
	}
	return counts
}

// Lines is Diagnostics#lines: "page 3 · label sec:x: alert[label.unresolved_owner]:
// message", most severe first; identical diagnostics print once with their count.
func (d *Diagnostics) Lines(minimum string) []string {
	type tally struct {
		line  string
		level int
		count int
	}
	tallies := []*tally{}
	seen := map[string]*tally{}
	for position, entry := range d.entries {
		level := slices.Index(Levels, entry["level"].(string))
		if level > slices.Index(Levels, minimum) {
			continue
		}
		line := describe(entry, d.orders[position])
		if found, ok := seen[line]; ok {
			found.count++
			continue
		}
		item := &tally{line: line, level: level, count: 1}
		seen[line] = item
		tallies = append(tallies, item)
	}
	slices.SortStableFunc(tallies, func(a, b *tally) int { return a.level - b.level })
	lines := make([]string, len(tallies))
	for index, item := range tallies {
		lines[index] = item.line
		if item.count > 1 {
			lines[index] = fmt.Sprintf("%s (×%d)", item.line, item.count)
		}
	}
	return lines
}

func describe(entry rec.Map, order []Place) string {
	places := make([]string, len(order))
	for index, place := range order {
		places[index] = fmt.Sprintf("%s %v", place.Key, place.Value)
	}
	place := strings.Join(places, " · ")
	if place == "" {
		place = "paper"
	}
	return fmt.Sprintf("%s: %s[%s]: %s", place, entry["level"], entry["code"], entry["message"])
}

// Map is Diagnostics#to_h.
func (d *Diagnostics) Map() rec.Map {
	return rec.Map{"schema_version": Version, "counts": d.Counts(), "entries": d.Entries()}
}
