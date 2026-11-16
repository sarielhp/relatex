package language

import (
	"slices"
	"testing"

	"github.com/sarielhp/relatex/internal/langtag"
	"github.com/sarielhp/relatex/internal/rec"
)

func TestTagMapsBabelAndPolyglossiaNames(t *testing.T) {
	cases := map[string]string{"english": "en", "ngerman": "de", "german": "de", "french": "fr", "USenglish": "en-US",
		"british": "en-GB", "naustrian": "de-AT", "brazil": "pt-BR", "norsk": "nb", "ancientgreek": "grc", "de-CH": "de-CH"}
	for name, want := range cases {
		if got, ok := langtag.Tag(name); !ok || got != want {
			t.Errorf("Tag(%q) = %q, %v; want %q", name, got, ok, want)
		}
	}
	for _, name := range []string{"klingon", "nil", "", "English language"} {
		if tag, ok := langtag.Tag(name); ok {
			t.Errorf("Tag(%q) = %q; want unknown", name, tag)
		}
	}
}

func TestReadDeclaredLanguage(t *testing.T) {
	evidence := Read([]rec.Map{{"code": "document_language", "source": "babel", "language": "ngerman", "class_options": ""},
		{"code": "text_language", "language": "english"}, {"code": "text_language", "language": "klingon"},
		{"code": "text_language", "language": "klingon"}})
	if !rec.Equal(evidence.Main, rec.Map{"tag": "de", "source": "babel", "name": "ngerman"}) || evidence.Undeclared {
		t.Errorf("main = %v, undeclared %v", evidence.Main, evidence.Undeclared)
	}
	if !slices.Equal(evidence.Unknown, []string{"klingon"}) {
		t.Errorf("unknown = %v", evidence.Unknown)
	}
}

// babel with nil declares no language: undeclared, with the class option or metadata still read.
func TestBabelNilDeclaresNothing(t *testing.T) {
	none := Read([]rec.Map{{"code": "document_language", "source": "babel", "language": "nil", "class_options": "11pt"}})
	if !none.Undeclared || len(none.Unknown) != 0 || none.Main["source"] != SourceDefault {
		t.Errorf("babel nil: %+v", none)
	}
	option := Read([]rec.Map{{"code": "document_language", "source": "babel", "language": "nil", "class_options": "11pt,french"}})
	if option.Undeclared || option.Main["tag"] != "fr" || option.Main["source"] != SourceClassOption {
		t.Errorf("babel nil with a class option: %+v", option)
	}
}

func TestReadUndeclaredAndClassOptionLanguage(t *testing.T) {
	for _, rows := range [][]rec.Map{nil, {{"code": "document_language", "source": "none", "class_options": "11pt,a4paper"}}} {
		evidence := Read(rows)
		if !evidence.Undeclared || !rec.Equal(evidence.Main, rec.Map{"tag": "en", "source": "default"}) {
			t.Errorf("rows %v: main = %v, undeclared %v", rows, evidence.Main, evidence.Undeclared)
		}
	}
	evidence := Read([]rec.Map{{"code": "document_language", "source": "none", "class_options": "11pt, french,a4paper"}})
	if evidence.Undeclared || !rec.Equal(evidence.Main, rec.Map{"tag": "fr", "source": "class_option", "name": "french"}) {
		t.Errorf("class option: main = %v", evidence.Main)
	}
	unknown := Read([]rec.Map{{"code": "document_language", "source": "polyglossia", "language": "klingon"}})
	if unknown.Undeclared || unknown.Main["tag"] != Undetermined || !slices.Equal(unknown.Unknown, []string{"klingon"}) {
		t.Errorf("unknown main language: %+v", unknown)
	}
}

// TestReadDocumentMetadataLanguage: \DocumentMetadata{lang=…} (decision 096) declares the language
// when no language package does; a class option yields to it, babel does not.
func TestReadDocumentMetadataLanguage(t *testing.T) {
	metadata := Read([]rec.Map{{"code": "document_language", "source": "none", "class_options": "french", "metadata_language": "de-AT"}})
	if metadata.Undeclared || !rec.Equal(metadata.Main, rec.Map{"tag": "de-AT", "source": "metadata", "name": "de-AT"}) {
		t.Errorf("metadata: main = %v", metadata.Main)
	}
	plain := Read([]rec.Map{{"code": "document_language", "source": "none", "metadata_language": "pl"}})
	if plain.Main["tag"] != "pl" || plain.Main["source"] != SourceMetadata {
		t.Errorf("a bare tag: main = %v", plain.Main)
	}
	babel := Read([]rec.Map{{"code": "document_language", "source": "babel", "language": "polish", "metadata_language": "en"}})
	if babel.Main["tag"] != "pl" || babel.Main["source"] != SourceBabel {
		t.Errorf("babel: main = %v", babel.Main)
	}
	unknown := Read([]rec.Map{{"code": "document_language", "source": "none", "metadata_language": "not a tag"}})
	if !unknown.Undeclared || !slices.Equal(unknown.Unknown, []string{"not a tag"}) {
		t.Errorf("unknown: %+v", unknown)
	}
}

func TestSpanTagDropsTheDocumentsOwnTag(t *testing.T) {
	if SpanTag("french", "en") != "fr" || SpanTag("american", "en-US") != "" || SpanTag("klingon", "en") != "" {
		t.Error("SpanTag must tag only known languages other than the document's")
	}
}
