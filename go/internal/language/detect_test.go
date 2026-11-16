package language

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/rec"
)

func testProfile(t *testing.T) Profile {
	t.Helper()
	file, err := os.Open(filepath.Join("..", "..", "..", "profiles", ProfileFile))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	value, err := canonjson.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	return NewProfile(rec.AsMap(value))
}

const (
	english = "We show that the problem can be solved in linear time. The algorithm is simple, and its analysis " +
		"uses only the fact that each step removes an edge from the graph, which has at most n edges. "
	spanish = "Dividimos el proceso de trabajo en cuatro etapas. Cada una de estas etapas tiene entradas y salidas, " +
		"y los nombres de los datos son consistentes porque el código está revisado hasta el final. "
)

func repeated(text string, times int) []string {
	return []string{strings.Repeat(text, times)}
}

func TestDetectEnglishIsCertain(t *testing.T) {
	detection := testProfile(t).Detect(repeated(english, 12))
	if detection.Tag != "en" || !detection.Certain || detection.RunnerUpHits != 0 {
		t.Fatalf("detection = %+v", detection)
	}
	if Detected(detection)["source"] != SourceDetected {
		t.Fatal("source")
	}
}

func TestDetectSpanishIsCertain(t *testing.T) {
	detection := testProfile(t).Detect(repeated(spanish, 12))
	if detection.Tag != "es" || !detection.Certain {
		t.Fatalf("detection = %+v", detection)
	}
}

func TestDetectShortTextIsNotCertain(t *testing.T) {
	if detection := testProfile(t).Detect(repeated(english, 2)); detection.Certain {
		t.Fatalf("too few words: %+v", detection)
	}
}

func TestDetectMixedTextIsNotCertain(t *testing.T) {
	if detection := testProfile(t).Detect(append(repeated(english, 10), repeated(spanish, 8)...)); detection.Certain {
		t.Fatalf("no decisive margin: %+v", detection)
	}
}

func TestDetectUnlistedLanguageIsNotCertain(t *testing.T) {
	polish := "Pokazujemy, że problem można rozwiązać w czasie liniowym. Algorytm jest prosty, a jego analiza korzysta tylko z faktu. "
	if detection := testProfile(t).Detect(repeated(polish, 30)); detection.Certain {
		t.Fatalf("no listed language: %+v", detection)
	}
}

func TestDetectOtherScriptIsNotCertain(t *testing.T) {
	greek := strings.Repeat("αβγ δεζ ", 40)
	if detection := testProfile(t).Detect([]string{strings.Repeat(english, 12), greek}); detection.Certain {
		t.Fatalf("script share: %+v", detection)
	}
}

func TestProfileWordListsAreDisjoint(t *testing.T) {
	profile := testProfile(t)
	total := 0
	file, _ := os.ReadFile(filepath.Join("..", "..", "..", "profiles", ProfileFile))
	value, _ := canonjson.Decode(strings.NewReader(string(file)))
	for _, entry := range rec.FetchList(rec.AsMap(value), "languages") {
		total += len(rec.FetchList(rec.AsMap(entry), "words"))
	}
	if total != len(profile.index) {
		t.Fatalf("a word is listed for two languages: %d words, %d distinct", total, len(profile.index))
	}
}

func TestTextsReadsCells(t *testing.T) {
	nodes := rec.Map{"a": rec.Map{"content": []any{
		rec.Map{"type": "text", "text": "one"}, rec.Map{"type": "math", "text": "x"},
		rec.Map{"type": "table_grid", "grid": rec.Map{"cells": []any{rec.Map{"content": []any{rec.Map{"type": "text", "text": "two"}}}}}},
	}}}
	if texts := Texts(nodes); strings.Join(texts, " ") != "one two" {
		t.Fatalf("texts = %v", texts)
	}
}
