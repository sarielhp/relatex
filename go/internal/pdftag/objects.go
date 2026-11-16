package pdftag

import (
	"bytes"
	"cmp"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
)

// pdfFile is a PDF as qpdf's JSON (version 2) shows it: every object by its key ("obj:N 0 R"),
// values in qpdf's notation (names "/N", text strings "u:…", binary "b:HEX", references
// "N 0 R"), stream data read on demand (streamData) and kept once read, images excluded. A
// conversion's PDF holds every figure's image data (papers4 0950: 5 GB inline, 24 GB in
// encoding/json's values); the tagger reads the text streams only. Changes are collected in patch
// and written by qpdf.
type pdfFile struct {
	qpdf       string
	path       string
	objects    map[string]any
	maxID      int
	patch      map[string]any
	cache      map[string][]byte // the streams read, by object key (images excluded)
	prefetched bool              // every non-image stream was fetched once
}

// page is one page of the document: its object, its resources and its content.
type page struct {
	ref       string
	dict      map[string]any
	resources map[string]any
}

// readCopy reads the PDF from a copy without object streams, which the patch is applied to:
// qpdf's --update-from-json drops the update of an object stored in an object stream when nothing
// listed before it in the patch refers to it (papers4 1005: a figure's CIDFont kept no
// CIDToGIDMap). The copy draws the same; only its object numbers differ from the original's.
func readCopy(qpdf, path, copyPath string) (*pdfFile, error) {
	output, err := exec.Command(qpdf, "--object-streams=disable", path, copyPath).CombinedOutput()
	var exit *exec.ExitError
	if err != nil && !(errors.As(err, &exit) && exit.ExitCode() == 3) {
		os.Remove(copyPath)
		return nil, fmt.Errorf("qpdf --object-streams=disable %s: %w: %s", path, err, bytes.TrimSpace(output))
	}
	file, err := readPDF(qpdf, copyPath)
	if err != nil {
		os.Remove(copyPath)
	}
	return file, err
}

// readPDF reads the PDF through qpdf's JSON.
func readPDF(qpdf, path string) (*pdfFile, error) {
	output, err := exec.Command(qpdf, "--json=2", "--json-key=qpdf", "--json-stream-data=none", path).Output()
	var exit *exec.ExitError
	if err != nil && !(errors.As(err, &exit) && exit.ExitCode() == 3) {
		return nil, fmt.Errorf("qpdf --json %s: %w", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(output))
	decoder.UseNumber()
	var parsed struct {
		Qpdf []json.RawMessage `json:"qpdf"`
	}
	if err := decoder.Decode(&parsed); err != nil || len(parsed.Qpdf) < 2 {
		return nil, fmt.Errorf("qpdf --json %s: unreadable output", path)
	}
	var header struct {
		MaxObjectID int `json:"maxobjectid"`
	}
	file := &pdfFile{qpdf: qpdf, path: path, patch: map[string]any{}}
	inner := json.NewDecoder(bytes.NewReader(parsed.Qpdf[1]))
	inner.UseNumber()
	if json.Unmarshal(parsed.Qpdf[0], &header) != nil || inner.Decode(&file.objects) != nil {
		return nil, fmt.Errorf("qpdf --json %s: unreadable objects", path)
	}
	file.maxID = header.MaxObjectID
	return file, nil
}

func objectKey(ref string) string { return "obj:" + ref }

// entry is an object's JSON entry, the patched one when there is one.
func (f *pdfFile) entry(ref string) map[string]any {
	if patched, ok := f.patch[objectKey(ref)].(map[string]any); ok {
		return patched
	}
	entry, _ := f.objects[objectKey(ref)].(map[string]any)
	return entry
}

// isRef reports whether a value is an indirect reference.
func isRef(value any) bool {
	text, ok := value.(string)
	return ok && strings.HasSuffix(text, " R") && !strings.HasPrefix(text, "/") && !strings.Contains(text, ":")
}

// resolve follows a reference to its value (a stream's dictionary for a stream).
func (f *pdfFile) resolve(value any) any {
	for range 32 {
		ref, ok := value.(string)
		if !ok || !isRef(ref) {
			return value
		}
		entry := f.entry(ref)
		if stream, isStream := entry["stream"].(map[string]any); isStream {
			return stream["dict"]
		}
		value = entry["value"]
	}
	return nil
}

// dict resolves a value to a dictionary (nil when it is none).
func (f *pdfFile) dict(value any) map[string]any {
	resolved, _ := f.resolve(value).(map[string]any)
	return resolved
}

// array resolves a value to an array.
func (f *pdfFile) array(value any) []any {
	resolved, _ := f.resolve(value).([]any)
	return resolved
}

// streamData is a stream's decoded data: read alone, from a PDF whose objects are already read
// (a patched stream's data is in the patch). The first read fetches every stream that is not an
// image in one qpdf call and keeps them (prefetchStreams): a form painted at many CTMs, a page
// read by every pass and the glyph procedures of a Type 3 font would otherwise run qpdf over the
// whole PDF for each read (papers2 0351: 2250 runs, 270 s). An image is read alone and never
// kept: papers4 0950's image data must not stay in memory (a11y-108).
func (f *pdfFile) streamData(ref string) ([]byte, error) {
	if patched, ok := f.patch[objectKey(ref)].(map[string]any); ok {
		stream, _ := patched["stream"].(map[string]any)
		encoded, _ := stream["data"].(string)
		return base64.StdEncoding.DecodeString(encoded)
	}
	if data, ok := f.cache[objectKey(ref)]; ok {
		return data, nil
	}
	if stream, ok := f.entry(ref)["stream"].(map[string]any); ok && stream["data"] != nil {
		encoded, _ := stream["data"].(string)
		return base64.StdEncoding.DecodeString(encoded)
	}
	if !f.prefetched {
		f.prefetched = true
		// A failed prefetch leaves the requested stream to be read alone below.
		_ = f.prefetchStreams()
		if data, ok := f.cache[objectKey(ref)]; ok {
			return data, nil
		}
	}
	return f.readStream(ref)
}

// readStream runs qpdf for one stream's decoded data; a stream that is not an image is kept.
func (f *pdfFile) readStream(ref string) ([]byte, error) {
	objects, err := jsonObjects(f.qpdf, f.path, true, objectKey(ref))
	if err != nil {
		return nil, err
	}
	entry, _ := objects[objectKey(ref)].(map[string]any)
	stream, ok := entry["stream"].(map[string]any)
	if !ok || stream["data"] == nil {
		return nil, fmt.Errorf("%s is not a stream", ref)
	}
	encoded, _ := stream["data"].(string)
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	if f.dict(ref)["/Subtype"] != "/Image" {
		f.store(ref, data)
	}
	return data, nil
}

// prefetchBatch is the objects one prefetch qpdf run asks for, which bounds its argument list.
const prefetchBatch = 1024

// prefetchStreams reads every stream that is not an image in as many qpdf runs as the batch
// needs (usually one) and keeps the data. Images are left out: they are the memory a11y-108
// bounded (papers4 0950).
func (f *pdfFile) prefetchStreams() error {
	var keys []string
	for key, value := range f.objects {
		if _, patched := f.patch[key]; patched {
			continue
		}
		entry, _ := value.(map[string]any)
		stream, ok := entry["stream"].(map[string]any)
		if !ok {
			continue
		}
		dict, _ := stream["dict"].(map[string]any)
		if dict["/Subtype"] != "/Image" {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	for start := 0; start < len(keys); start += prefetchBatch {
		end := min(start+prefetchBatch, len(keys))
		objects, err := jsonObjects(f.qpdf, f.path, true, keys[start:end]...)
		if err != nil {
			return err
		}
		for _, key := range keys[start:end] {
			entry, _ := objects[key].(map[string]any)
			stream, _ := entry["stream"].(map[string]any)
			encoded, ok := stream["data"].(string)
			if !ok {
				continue
			}
			data, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
			f.store(strings.TrimPrefix(key, "obj:"), data)
		}
	}
	return nil
}

// store keeps a stream's decoded data by its reference.
func (f *pdfFile) store(ref string, data []byte) {
	if f.cache == nil {
		f.cache = map[string][]byte{}
	}
	f.cache[objectKey(ref)] = data
}

// add creates an object and returns its reference.
func (f *pdfFile) add(value any) string {
	f.maxID++
	ref := strconv.Itoa(f.maxID) + " 0 R"
	f.patch[objectKey(ref)] = map[string]any{"value": value}
	return ref
}

// addStream creates a stream object.
func (f *pdfFile) addStream(dict map[string]any, data []byte) string {
	f.maxID++
	ref := strconv.Itoa(f.maxID) + " 0 R"
	f.setStream(ref, dict, data)
	return ref
}

// set replaces an object's value.
func (f *pdfFile) set(ref string, value any) {
	f.patch[objectKey(ref)] = map[string]any{"value": value}
}

// setStream replaces a stream; its data is given unfiltered and qpdf compresses it.
func (f *pdfFile) setStream(ref string, dict map[string]any, data []byte) {
	clean := map[string]any{}
	for key, value := range dict {
		if key != "/Filter" && key != "/DecodeParms" && key != "/Length" {
			clean[key] = value
		}
	}
	f.patch[objectKey(ref)] = map[string]any{"stream": map[string]any{"dict": clean,
		"data": base64.StdEncoding.EncodeToString(data)}}
}

// root is the document catalog's reference.
func (f *pdfFile) root() string {
	trailer, _ := f.objects["trailer"].(map[string]any)
	value, _ := trailer["value"].(map[string]any)
	ref, _ := value["/Root"].(string)
	return ref
}

// pages lists the pages in order with their inherited resources.
func (f *pdfFile) pages() []page {
	var pages []page
	var walk func(node any, resources map[string]any, depth int)
	walk = func(node any, resources map[string]any, depth int) {
		dict := f.dict(node)
		if dict == nil || depth > 64 {
			return
		}
		if own := f.dict(dict["/Resources"]); own != nil {
			resources = own
		}
		if dict["/Type"] == "/Page" {
			ref, _ := node.(string)
			pages = append(pages, page{ref: ref, dict: dict, resources: resources})
			return
		}
		for _, kid := range f.array(dict["/Kids"]) {
			walk(kid, resources, depth+1)
		}
	}
	walk(f.dict(f.root())["/Pages"], nil, 0)
	return pages
}

// contentData is a page's content, its streams joined.
func (f *pdfFile) contentData(p page) ([]byte, error) {
	contents := p.dict["/Contents"]
	refs := []any{contents}
	if list, ok := f.resolve(contents).([]any); ok {
		refs = list
	}
	var data []byte
	for _, ref := range refs {
		text, _ := ref.(string)
		part, err := f.streamData(text)
		if err != nil {
			return nil, err
		}
		data = append(append(data, part...), '\n')
	}
	return data, nil
}

// patchJSON is the patch as qpdf's JSON with the objects in ascending number: qpdf dropped
// updates of existing objects listed after new objects that refer to them (encoding/json's
// sorted keys put obj:1000 before obj:107; slides 0005 and 0018 lost pages' updates).
func (f *pdfFile) patchJSON() ([]byte, error) {
	keys := make([]string, 0, len(f.patch))
	for key := range f.patch {
		keys = append(keys, key)
	}
	// The pages first, then every other object in ascending number: qpdf dropped the updates of a
	// page's annotations listed before the page (their structure keys), as it dropped those of
	// pages listed after new objects referring to them.
	slices.SortFunc(keys, func(a, b string) int {
		return cmp.Or(cmp.Compare(f.patchRank(a), f.patchRank(b)), cmp.Compare(objectNumber(a), objectNumber(b)))
	})
	var out bytes.Buffer
	out.WriteString(`{"qpdf":[{"jsonversion":2,"pushedinheritedpageresources":false,"calledgetallpages":false},{`)
	for index, key := range keys {
		value, err := json.Marshal(f.patch[key])
		if err != nil {
			return nil, err
		}
		if index > 0 {
			out.WriteByte(',')
		}
		fmt.Fprintf(&out, "%q:%s", key, value)
	}
	out.WriteString("}]}")
	return out.Bytes(), nil
}

// patchRank orders the patch: 0 for a page, 1 for any other object.
func (f *pdfFile) patchRank(key string) int {
	entry, _ := f.patch[key].(map[string]any)
	if value, _ := entry["value"].(map[string]any); value["/Type"] == "/Page" {
		return 0
	}
	return 1
}

// objectNumber is the number of an "obj:N 0 R" key.
func objectNumber(key string) int {
	number, _ := strconv.Atoi(strings.Fields(strings.TrimPrefix(key, "obj:"))[0])
	return number
}

// write applies the patch with qpdf and writes the result as PDF 2.0; qpdf's warnings are
// returned.
func (f *pdfFile) write(output, scratch string) (string, error) {
	data, err := f.patchJSON()
	if err != nil {
		return "", fmt.Errorf("patch: %w", err)
	}
	patchFile := scratch + ".patch.json"
	defer os.Remove(patchFile)
	if err = os.WriteFile(patchFile, data, 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", patchFile, err)
	}
	run := exec.Command(f.qpdf, f.path, "--update-from-json="+patchFile, "--force-version=2.0",
		"--object-streams=generate", output)
	message, err := run.CombinedOutput()
	var exit *exec.ExitError
	if err != nil && !(errors.As(err, &exit) && exit.ExitCode() == 3) {
		return "", fmt.Errorf("qpdf --update-from-json: %w: %s", err, bytes.TrimSpace(message))
	}
	return string(bytes.TrimSpace(message)), nil
}
