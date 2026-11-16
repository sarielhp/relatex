package pdftag

import (
	"encoding/base64"
	"testing"
)

// streamData reads a patched stream from the patch before the PDF's objects: a rewritten content
// stream must be interpreted as it was written, not as the source PDF has it.
func TestStreamDataReadsPatchFirst(t *testing.T) {
	const ref = "5 0 R"
	stream := func(data string) map[string]any {
		return map[string]any{"stream": map[string]any{"data": base64.StdEncoding.EncodeToString([]byte(data))}}
	}
	file := &pdfFile{objects: map[string]any{objectKey(ref): stream("source")},
		patch: map[string]any{objectKey(ref): stream("patched")}}
	data, err := file.streamData(ref)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "patched" {
		t.Fatalf("stream data %q, want the patch's", data)
	}
}
