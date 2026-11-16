package pdftag

import (
	"encoding/base64"
	"reflect"
	"testing"
)

func TestFigureFontsReadsTheFormsFontsAndCreator(t *testing.T) {
	collection := append([]byte("ttcf\x00\x01\x00\x00"), []byte("the rest of a fake collection")...)
	file := &pdfFile{
		objects: map[string]any{
			objectKey("10 0 R"): map[string]any{"value": map[string]any{
				"/Subtype": "/Form", "/PTEX.InfoDict": "11 0 R",
				"/Resources": map[string]any{"/Font": map[string]any{"/F1": "12 0 R"}}}},
			objectKey("11 0 R"): map[string]any{"value": map[string]any{"/Creator": "b:feff005700500053"}},
			objectKey("12 0 R"): map[string]any{"value": map[string]any{
				"/Type": "/Font", "/Subtype": "/Type0", "/BaseFont": "/ABCDEF+Fake",
				"/DescendantFonts": []any{"13 0 R"}}},
			objectKey("13 0 R"): map[string]any{"value": map[string]any{
				"/Type": "/Font", "/Subtype": "/CIDFontType2", "/BaseFont": "/ABCDEF+Fake",
				"/FontDescriptor": "14 0 R"}},
			objectKey("14 0 R"): map[string]any{"value": map[string]any{
				"/Type": "/FontDescriptor", "/FontFile2": "15 0 R"}},
			objectKey("15 0 R"): map[string]any{"stream": map[string]any{
				"data": base64.StdEncoding.EncodeToString(collection)}},
		},
		patch: map[string]any{},
	}
	tagger := &tagger{file: file, builder: &builder{sources: map[string]string{"fig": "Figures/f.pdf"}},
		prepared: []*prepared{{
			number: 1,
			items: []item{
				{kind: kindObject, xobject: "Form", ref: "10 0 R"},
				{kind: kindShow, font: "12 0 R"}, // a TeX font on the page itself
			},
			objects: map[int]string{0: "fig"},
		}}}
	want := []FigureFont{{Source: "Figures/f.pdf", Font: "ABCDEF+Fake", Creator: "WPS", Collection: true}}
	if got := tagger.figureFonts(); !reflect.DeepEqual(got, want) {
		t.Fatalf("figure fonts %+v, want %+v", got, want)
	}
}
