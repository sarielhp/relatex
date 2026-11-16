// Package language reads the document's language and its spans in other languages from the
// collector's records and maps TeX language names to BCP 47 tags (decision 080, plan A1).
//
// The collector records names as babel and polyglossia spell them (\languagename): the
// document_language record once, after \begin{document}, and one text_language record per
// collected glyph set in a language other than the document's.
//
// It is in the model layer (decision 102): paperdoc projects the language with it; the tags are
// langtag's.
package language

import (
	"strings"

	"github.com/sarielhp/relatex/internal/langtag"
	"github.com/sarielhp/relatex/internal/rec"
)

// Undetermined is the tag of a document whose declared main language has no known tag (decision
// 080, the author's choice): a screen reader then uses the reader's own default instead of
// reading the text with English rules.
const Undetermined = "und"

// Sources of the document's language in document.json.
const (
	SourceBabel       = "babel"
	SourcePolyglossia = "polyglossia"
	SourceClassOption = "class_option"
	// SourceMetadata: \DocumentMetadata{lang=…} (the LaTeX tagging project, decision 096).
	SourceMetadata = "metadata"
	SourceDefault  = "default"
)

// A babel main language of nil (babel loaded without a language, or \selectlanguage{nil}) declares
// none: the declaration falls through to the metadata and the class options, and the document stays
// undeclared when they say nothing, so the text decides (decision 111).

// Evidence is what the records say about languages.
type Evidence struct {
	// Main is document.json's language: tag, source and, when declared, the TeX name.
	Main rec.Map
	// Undeclared: nothing in the source names a language; Main is the default.
	Undeclared bool
	// Unknown lists the names (main or span) the table cannot map, in order of first use.
	Unknown []string
}

// Read reads the document_language and text_language records among rows (stream records or
// an inspection's diagnostics).
func Read(rows []rec.Map) Evidence {
	evidence := Evidence{Main: rec.Map{"tag": langtag.Default, "source": SourceDefault}, Undeclared: true}
	seen := map[string]bool{}
	unknown := func(name string) {
		if !seen[name] {
			seen[name] = true
			evidence.Unknown = append(evidence.Unknown, name)
		}
	}
	for _, row := range rows {
		switch row["code"] {
		case "document_language":
			evidence.declare(row, unknown)
		case "text_language":
			if name := text(row["language"]); name != "" {
				if _, ok := langtag.Tag(name); !ok {
					unknown(name)
				}
			}
		}
	}
	return evidence
}

func (e *Evidence) declare(row rec.Map, unknown func(string)) {
	source, name := text(row["source"]), text(row["language"])
	if (source == SourceBabel || source == SourcePolyglossia) && !strings.EqualFold(name, "nil") {
		if tag, ok := langtag.Tag(name); ok {
			e.Main, e.Undeclared = rec.Map{"tag": tag, "source": source, "name": name}, false
			return
		}
		if name != "" {
			unknown(name)
			e.Main, e.Undeclared = rec.Map{"tag": Undetermined, "source": source, "name": name}, false
		}
		return
	}
	// Without a language package, \DocumentMetadata{lang=…} names it as a tag (or a name).
	if declared := strings.TrimSpace(text(row["metadata_language"])); declared != "" {
		tag, ok := langtag.Tag(declared)
		if !ok && langtag.Valid(declared) {
			tag, ok = declared, true
		}
		if ok {
			e.Main, e.Undeclared = rec.Map{"tag": tag, "source": SourceMetadata, "name": declared}, false
			return
		}
		unknown(declared)
	}
	// Without a language package a class option may still name the language (babel's rule: the
	// last language option is the main one).
	for _, option := range strings.Split(text(row["class_options"]), ",") {
		option = strings.TrimSpace(option)
		if tag, ok := langtag.Named(option); ok {
			e.Main, e.Undeclared = rec.Map{"tag": tag, "source": SourceClassOption, "name": option}, false
		}
	}
}

// SpanTag is the tag a span set in the named language carries: "" when the name is unknown or
// maps to the document's own tag.
func SpanTag(name, mainTag string) string {
	tag, ok := langtag.Tag(name)
	if !ok || tag == mainTag {
		return ""
	}
	return tag
}

// text is a record's optional string field: "" when absent.
func text(value any) string {
	s, _ := value.(string)
	return s
}
