package pdftag

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sarielhp/relatex/internal/command"
)

// The PDF/UA-2 identification only when the checks earn it (decision 112). An author's
// \DocumentMetadata{pdfstandard=ua-2} makes the LaTeX kernel write the identification (pdfuaid:part
// and pdfuaid:rev) and the Well-Tagged PDF declarations (pdfd:declarations) into the ordinary PDF's
// XMP, which the tagged PDF keeps, and route 2 writes the identification itself, whatever veraPDF and relatex's checks
// later say. WithdrawClaim takes them out of a file that did not earn them; the rest of the
// metadata and every page stay as they are.

var (
	// claimElements are the identification's properties and the declarations block, with the
	// white space before them so the packet keeps its layout.
	claimElements = regexp.MustCompile(`(?s)\s*<pdfuaid:([A-Za-z]+)>[^<]*</pdfuaid:([A-Za-z]+)>|\s*<pdfd:declarations>.*?</pdfd:declarations>`)
	// claimAttributes are the identification written as attributes of rdf:Description.
	claimAttributes = regexp.MustCompile(`\s+pdfuaid:[A-Za-z]+="[^"]*"`)
)

// StripClaim is the XMP packet without the PDF/UA identification and the conformance declarations,
// and whether there was anything to take out.
func StripClaim(packet []byte) ([]byte, bool) {
	stripped := claimAttributes.ReplaceAll(claimElements.ReplaceAll(packet, nil), nil)
	return stripped, !bytes.Equal(stripped, packet)
}

// Claims tells whether an XMP packet identifies the file as PDF/UA.
func Claims(packet []byte) bool {
	return claimElements.Match(packet) || claimAttributes.Match(packet)
}

// Identification is the XMP metadata packet of the PDF's catalog, nil when it has none.
func Identification(pdf string) ([]byte, error) {
	qpdf := command.Resolve("qpdf")
	if qpdf == "" {
		return nil, fmt.Errorf("qpdf is required to read a PDF's metadata")
	}
	ref, _, err := metadataStream(qpdf, pdf)
	if err != nil || ref == "" {
		return nil, err
	}
	return streamBytes(qpdf, pdf, ref)
}

// WithdrawClaim removes the PDF/UA identification and the conformance declarations from the PDF's
// XMP metadata in place, with qpdf; withdrawn is false when the file states neither.
func WithdrawClaim(pdf string) (withdrawn bool, err error) {
	qpdf := command.Resolve("qpdf")
	if qpdf == "" {
		return false, fmt.Errorf("qpdf is required to change a PDF's metadata")
	}
	ref, dict, err := metadataStream(qpdf, pdf)
	if err != nil || ref == "" {
		return false, err
	}
	packet, err := streamBytes(qpdf, pdf, ref)
	if err != nil {
		return false, err
	}
	stripped, changed := StripClaim(packet)
	if !changed {
		return false, nil
	}
	file := &pdfFile{qpdf: qpdf, path: pdf, patch: map[string]any{}}
	file.setStream(ref, dict, stripped)
	data, err := file.patchJSON()
	if err != nil {
		return false, err
	}
	scratch, err := os.MkdirTemp(filepath.Dir(pdf), ".relatex-claim-")
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(scratch)
	patch, output := filepath.Join(scratch, "patch.json"), filepath.Join(scratch, filepath.Base(pdf))
	if err := os.WriteFile(patch, data, 0o644); err != nil {
		return false, err
	}
	if message, err := runQPDF(qpdf, pdf, "--update-from-json="+patch, output); err != nil {
		return false, fmt.Errorf("qpdf --update-from-json: %w: %s", err, message)
	}
	return true, os.Rename(output, pdf)
}

// metadataStream is the reference and dictionary of the catalog's /Metadata stream ("" when the
// catalog has none).
func metadataStream(qpdf, pdf string) (string, map[string]any, error) {
	objects, err := jsonObjects(qpdf, pdf, false, "trailer")
	if err != nil {
		return "", nil, err
	}
	root, _ := valueOf(objects["trailer"])["/Root"].(string)
	if !isRef(root) {
		return "", nil, fmt.Errorf("%s: no document catalog", pdf)
	}
	if objects, err = jsonObjects(qpdf, pdf, false, objectKey(root)); err != nil {
		return "", nil, err
	}
	ref, _ := valueOf(objects[objectKey(root)])["/Metadata"].(string)
	if !isRef(ref) {
		return "", nil, nil
	}
	if objects, err = jsonObjects(qpdf, pdf, false, objectKey(ref)); err != nil {
		return "", nil, err
	}
	stream, _ := objects[objectKey(ref)].(map[string]any)["stream"].(map[string]any)
	dict, _ := stream["dict"].(map[string]any)
	if dict == nil {
		return "", nil, fmt.Errorf("%s: the catalog's /Metadata is not a stream", pdf)
	}
	return ref, dict, nil
}

// streamBytes is a stream's decoded data.
func streamBytes(qpdf, pdf, ref string) ([]byte, error) {
	objects, err := jsonObjects(qpdf, pdf, true, objectKey(ref))
	if err != nil {
		return nil, err
	}
	file := &pdfFile{objects: objects, patch: map[string]any{}}
	return file.streamData(ref)
}

// valueOf is an object entry's dictionary value.
func valueOf(entry any) map[string]any {
	object, _ := entry.(map[string]any)
	value, _ := object["value"].(map[string]any)
	return value
}

// jsonObjects reads the named objects ("trailer", "obj:N 0 R") through qpdf's JSON, with their
// stream data decoded when inline.
func jsonObjects(qpdf, pdf string, inline bool, keys ...string) (map[string]any, error) {
	arguments := []string{"--json=2", "--json-key=qpdf", "--json-stream-data=none"}
	if inline {
		arguments = []string{"--json=2", "--json-key=qpdf", "--json-stream-data=inline", "--decode-level=generalized"}
	}
	for _, key := range keys {
		if ref, isObject := cutObjectKey(key); isObject {
			key = ref
		}
		arguments = append(arguments, "--json-object="+key)
	}
	output, err := exec.Command(qpdf, append(arguments, pdf)...).Output()
	var exit *exec.ExitError
	if err != nil && !(errors.As(err, &exit) && exit.ExitCode() == 3) {
		return nil, fmt.Errorf("qpdf --json %s: %w", pdf, err)
	}
	var parsed struct {
		Qpdf []json.RawMessage `json:"qpdf"`
	}
	decoder := json.NewDecoder(bytes.NewReader(output))
	decoder.UseNumber()
	if err := decoder.Decode(&parsed); err != nil || len(parsed.Qpdf) < 2 {
		return nil, fmt.Errorf("qpdf --json %s: unreadable output", pdf)
	}
	objects := map[string]any{}
	inner := json.NewDecoder(bytes.NewReader(parsed.Qpdf[1]))
	inner.UseNumber()
	if err := inner.Decode(&objects); err != nil {
		return nil, fmt.Errorf("qpdf --json %s: unreadable objects", pdf)
	}
	return objects, nil
}

// cutObjectKey is an "obj:N G R" key as --json-object takes it, "N,G".
func cutObjectKey(key string) (string, bool) {
	ref, isObject := strings.CutPrefix(key, "obj:")
	fields := strings.Fields(ref)
	if !isObject || len(fields) < 2 {
		return key, false
	}
	return fields[0] + "," + fields[1], true
}

// runQPDF runs qpdf; exit status 3 (warnings) is success.
func runQPDF(qpdf string, arguments ...string) (string, error) {
	message, err := exec.Command(qpdf, arguments...).CombinedOutput()
	var exit *exec.ExitError
	if err != nil && !(errors.As(err, &exit) && exit.ExitCode() == 3) {
		return string(bytes.TrimSpace(message)), err
	}
	return "", nil
}
