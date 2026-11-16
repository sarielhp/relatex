package paperdoc

import (
	"fmt"
	"strings"

	"github.com/sarielhp/relatex/internal/langtag"
	"github.com/sarielhp/relatex/internal/language"
	"github.com/sarielhp/relatex/internal/rec"
)

// language is document.json's language (decision 080): the main language the source declares.
// When it declares none, the language the text reads as, if the detection is certain (decision
// 111, language.source "detected" with its evidence); else the default with a warning. Names the
// table cannot map are reported; their spans carry no tag.
func (b *builder) language(inspection, data rec.Map) rec.Map {
	evidence := language.Read(diagnosticRows(inspection))
	if len(evidence.Unknown) > 0 {
		b.diagnostics.Add("language.unknown", fmt.Sprintf("language %s has no known BCP 47 tag; its text is not marked with a language",
			strings.Join(evidence.Unknown, ", ")), int64(len(evidence.Unknown)))
	}
	if !evidence.Undeclared {
		return evidence.Main
	}
	detection := language.NewProfile(rootFile("profiles", language.ProfileFile)).Detect(language.Texts(rec.FetchMap(data, "nodes")))
	if detection.Certain {
		b.diagnostics.Add("language.detected", fmt.Sprintf("the source declares no language; the text reads as %s "+
			"(%d of its %d words are frequent %s words, %d of another language), so the export is marked %s: declare it",
			detection.Name, detection.Hits, detection.Words, detection.Name, detection.RunnerUpHits, detection.Tag), nil)
		return language.Detected(detection)
	}
	b.diagnostics.Add("language.undeclared", "the source declares no language with a known tag (babel, polyglossia or a class option), "+
		"and the text is not certainly in one; the export is marked as English ("+langtag.Default+")", nil)
	return evidence.Main
}

// diagnosticRows are an inspection's diagnostic records.
func diagnosticRows(inspection rec.Map) []rec.Map {
	rows := []rec.Map{}
	for _, value := range rec.FetchList(inspection, "diagnostics") {
		if row, ok := value.(rec.Map); ok {
			rows = append(rows, row)
		}
	}
	return rows
}

// validateSpanLanguage: a text or reference span may carry the BCP 47 tag of the language it is
// set in, when that differs from the document's.
func validateSpanLanguage(span rec.Map, kind any) {
	value, tagged := span["lang"]
	if !tagged {
		return
	}
	tag, isString := value.(string)
	if kind != "text" && kind != "reference" || !isString || !langtag.Valid(tag) {
		rec.Fail("Invalid span language")
	}
}
