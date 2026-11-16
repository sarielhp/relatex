package language

import (
	"strings"
	"unicode"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/rubynum"
)

// Detection (decision 111). When the source declares no language, relatex reads the language
// from the document's own text: the frequent function words of each language in
// profiles/language-detection.json, counted over the letter words of the text the conversion
// read (formulas are not text). It is used only when certain; otherwise the export keeps the
// English default and the language stays an author item.

// SourceDetected is the language source when the language was detected from the text, certainly (decision 111).
const SourceDetected = "detected"

// ProfileFile is the detection profile under profiles/.
const ProfileFile = "language-detection.json"

// Certainty are the thresholds a detection must meet, all of them, to be used.
type Certainty struct {
	// MinimumWords: letter words in the text.
	MinimumWords int
	// MinimumHits: frequent words of the winner.
	MinimumHits int
	// MinimumShare: the winner's frequent words as a share of all words.
	MinimumShare float64
	// Margin: the winner has at least this many times the runner-up's frequent words.
	Margin int
	// ScriptShare: the share of the words written in the winner's script.
	ScriptShare float64
}

// Profile is the detection profile: the thresholds and the languages' frequent words.
type Profile struct {
	Certainty Certainty
	languages []spoken
	index     map[string]int
}

type spoken struct {
	tag, name, script string
}

// NewProfile reads the profile's data (profiles/language-detection.json).
func NewProfile(data rec.Map) Profile {
	certainty := rec.FetchMap(data, "certainty")
	profile := Profile{Certainty: Certainty{
		MinimumWords: int(rubynum.Float(rec.Fetch(certainty, "minimum_words"))),
		MinimumHits:  int(rubynum.Float(rec.Fetch(certainty, "minimum_hits"))),
		MinimumShare: rubynum.Float(rec.Fetch(certainty, "minimum_share")),
		Margin:       int(rubynum.Float(rec.Fetch(certainty, "margin"))),
		ScriptShare:  rubynum.Float(rec.Fetch(certainty, "script_share")),
	}, index: map[string]int{}}
	for number, value := range rec.FetchList(data, "languages") {
		entry := rec.AsMap(value)
		profile.languages = append(profile.languages, spoken{tag: rec.FetchString(entry, "tag"), name: rec.FetchString(entry, "name"),
			script: rec.FetchString(entry, "script")})
		for _, word := range rec.FetchList(entry, "words") {
			profile.index[rec.AsString(word)] = number
		}
	}
	return profile
}

// Detection is what the text says: the winning language, its count, the runner-up's and the
// words in the winner's script.
type Detection struct {
	Tag, Name, RunnerUp       string
	Words, Hits, RunnerUpHits int
	ScriptWords               int
	Certain                   bool
}

// Evidence is the detection as document.json's language.evidence.
func (d Detection) Evidence() rec.Map {
	return rec.Map{"words": int64(d.Words), "hits": int64(d.Hits), "runner_up": d.RunnerUp,
		"runner_up_hits": int64(d.RunnerUpHits), "script_words": int64(d.ScriptWords)}
}

// Detect counts the frequent words of each language in texts and tells whether the winner is
// certain by the profile's thresholds.
func (p Profile) Detect(texts []string) Detection {
	if len(p.languages) == 0 {
		return Detection{}
	}
	hits := make([]int, len(p.languages))
	scripts := map[string]int{}
	words := 0
	for _, text := range texts {
		for _, word := range strings.FieldsFunc(text, func(r rune) bool { return !unicode.IsLetter(r) }) {
			words++
			if script := wordScript(word); script != "" {
				scripts[script]++
			}
			if number, ok := p.index[strings.ToLower(word)]; ok {
				hits[number]++
			}
		}
	}
	best, second := 0, -1
	for number := 1; number < len(hits); number++ {
		switch {
		case hits[number] > hits[best]:
			best, second = number, best
		case second < 0 || hits[number] > hits[second]:
			second = number
		}
	}
	winner := p.languages[best]
	detection := Detection{Tag: winner.tag, Name: winner.name, Words: words, Hits: hits[best], ScriptWords: scripts[winner.script]}
	if second >= 0 {
		detection.RunnerUp, detection.RunnerUpHits = p.languages[second].tag, hits[second]
	}
	detection.Certain = p.certain(detection)
	return detection
}

// certain: every threshold holds.
func (p Profile) certain(d Detection) bool {
	c := p.Certainty
	return d.Words >= c.MinimumWords && d.Hits >= c.MinimumHits &&
		float64(d.Hits) >= c.MinimumShare*float64(d.Words) &&
		d.Hits >= c.Margin*max(d.RunnerUpHits, 1) &&
		float64(d.ScriptWords) >= c.ScriptShare*float64(d.Words)
}

// wordScript is the script all the word's letters are written in, "" when they mix scripts or
// use one the profile does not name.
func wordScript(word string) string {
	script := ""
	for _, letter := range word {
		name := letterScript(letter)
		if name == "" || script != "" && name != script {
			return ""
		}
		script = name
	}
	return script
}

func letterScript(letter rune) string {
	switch {
	case unicode.Is(unicode.Latin, letter):
		return "Latin"
	case unicode.Is(unicode.Cyrillic, letter):
		return "Cyrillic"
	case unicode.Is(unicode.Greek, letter):
		return "Greek"
	}
	return ""
}

// Detected is document.json's language for a certain detection: the tag, its source and the
// evidence.
func Detected(detection Detection) rec.Map {
	return rec.Map{"tag": detection.Tag, "source": SourceDetected, "evidence": detection.Evidence()}
}

// Texts are the text spans' texts of a document's nodes, table cells included, in no
// particular order (the counts do not depend on it).
func Texts(nodes rec.Map) []string {
	texts := []string{}
	for _, value := range nodes {
		content, _ := rec.AsMap(value)["content"].([]any)
		texts = spanTexts(content, texts)
	}
	return texts
}

func spanTexts(content []any, texts []string) []string {
	for _, value := range content {
		span, _ := value.(rec.Map)
		switch span["type"] {
		case "text":
			if text, ok := span["text"].(string); ok {
				texts = append(texts, text)
			}
		case "table_grid":
			grid, _ := span["grid"].(rec.Map)
			cells, _ := grid["cells"].([]any)
			for _, cell := range cells {
				cellContent, _ := rec.AsMap(cell)["content"].([]any)
				texts = spanTexts(cellContent, texts)
			}
		}
	}
	return texts
}
