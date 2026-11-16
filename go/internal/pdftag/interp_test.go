package pdftag

import (
	"encoding/base64"
	"testing"
)

// formGlyphs caches a form by reference, CTM and rendering mode: mode 2 strokes the text it fills,
// so the modes must keep separate interpretations, while a repeat visit reuses its own.
func TestFormCacheSeparatesTextRenderingModes(t *testing.T) {
	const ref = "7 0 R"
	content := []byte("BT /F1 10 Tf (a) Tj ET")
	file := &pdfFile{objects: map[string]any{},
		patch: map[string]any{objectKey(ref): map[string]any{"stream": map[string]any{
			"data": base64.StdEncoding.EncodeToString(content)}}}}
	resources := map[string]any{"/Font": map[string]any{"/F1": map[string]any{"/Subtype": "/Type1"}}}
	dict := map[string]any{"/Resources": resources}
	in := newInterpreter(file, resources)
	fill, err := in.formGlyphs(ref, dict)
	if err != nil {
		t.Fatal(err)
	}
	in.state.render = 2
	stroke, err := in.formGlyphs(ref, dict)
	if err != nil {
		t.Fatal(err)
	}
	if fill == stroke || fill.trace.total == stroke.trace.total {
		t.Fatalf("rendering modes share the form cache: fill %+v, stroke %+v", fill.trace, stroke.trace)
	}
	again, err := in.formGlyphs(ref, dict)
	if err != nil {
		t.Fatal(err)
	}
	if again != stroke {
		t.Fatal("a repeat visit reinterpreted the form instead of reusing its cache entry")
	}
}
