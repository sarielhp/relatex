// Package bundle is the Go port of the export stage's bundle side: DocumentBundle (the
// portable, compressed JSON envelope), BundleValidation (the cross-layer checks of a bundle's
// document, layout and source map), PDFReconstruction and LayoutHTML.
package bundle

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/paperdoc"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/zgzip"
)

// Version is the bundle schema written: bundle-1.2 keeps the layout's pages in part files beside
// the bundle (layoutparts.go, decision 087); bundle-1.1 stored the layout inside, without the glyph
// fields its page SVGs hold (layoutcodec.go); bundle-1.0 stored it whole. Both are still read.
const Version = "bundle-1.2"

// inlineVersions are the bundle schemas holding their layout inside; Read accepts them and Version.
var inlineVersions = []any{"bundle-1.0", "bundle-1.1"}

// maxBytes bounds a bundle, compressed or expanded, and each layout part file (decisions 085 and
// 087). The bundle is read whole into memory; since its layout lives in part files, it holds the
// document and the source map only (Notes on Diffy Qs, 462 pages: 61 and 77 MB).
const maxBytes = 1024 * 1024 * 1024

// expandedLimit begins the failure of a bundle beyond maxBytes expanded.
const expandedLimit = "Bundle exceeds expanded byte limit"

// Fields is DocumentBundle::FIELDS, in the order Ruby's payload holds them.
var Fields = []string{"document", "layout", "source_map", "losses", "capabilities"}

// Root is the repository root (source_viewer.js); set it before WriteLayoutHTML.
var Root string

func hexSum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func marshal(value any) []byte {
	bytes, err := canonjson.Marshal(value)
	if err != nil {
		rec.Fail("%v", err)
	}
	return bytes
}

// Payload is the bundle payload Ruby's write builds: document, layout, source map, losses
// and capabilities, with Ruby's defaults for the absent ones.
func Payload(payload rec.Map) rec.Map {
	losses, ok := payload["losses"]
	if !ok {
		losses = []any{}
	}
	capabilities, ok := payload["capabilities"]
	if !ok {
		capabilities = rec.Map{}
	}
	return rec.Map{"document": rec.Fetch(payload, "document"), "layout": payload["layout"], "source_map": payload["source_map"],
		"losses": losses, "capabilities": capabilities}
}

// CanonicalEnvelope is DocumentBundle.canonical_envelope: the canonical JSON of { payload,
// schema_version, sha256 }, each part serialized once.
func CanonicalEnvelope(payload rec.Map) []byte {
	parts := map[string][]byte{}
	hashes := rec.Map{}
	for _, key := range Fields {
		parts[key] = marshal(payload[key])
		hashes[key] = hexSum(parts[key])
	}
	keys := slices.Clone(Fields)
	slices.Sort(keys)
	var body []byte
	for index, key := range keys {
		if index > 0 {
			body = append(body, ',')
		}
		body = append(body, marshal(key)...)
		body = append(body, ':')
		body = append(body, parts[key]...)
	}
	envelope := append([]byte(`{"payload":{`), body...)
	envelope = append(envelope, `},"schema_version":`...)
	envelope = append(envelope, marshal(Version)...)
	envelope = append(envelope, `,"sha256":`...)
	envelope = append(envelope, marshal(hashes)...)
	return append(envelope, '}')
}

// Write is DocumentBundle.write: the envelope, gzip-compressed when path ends in .gz, with its
// layout's part files beside it (WriteBook without chapters).
func Write(path string, payload rec.Map) rec.Map { return WriteBook(path, payload, nil) }

// WriteBook is Write with the pages a book's parts and chapters begin on, where its layout is cut
// into part files (decision 087).
func WriteBook(path string, payload rec.Map, chapters []int64) rec.Map {
	payload = Payload(payload)
	paperdoc.Validate(rec.AsMap(payload["document"]))
	if _, err := os.Lstat(path); err == nil {
		rec.Fail("Bundle destination exists")
	}
	absolute, err := filepath.Abs(path)
	check(err)
	check(os.MkdirAll(filepath.Dir(absolute), 0o755))
	payload["layout"] = storeLayout(payload["layout"], filepath.Dir(absolute), chapters)
	bytes := append(CanonicalEnvelope(payload), '\n')
	if len(bytes) > maxBytes {
		rec.Fail("%s: %d bytes, limit %d", expandedLimit, len(bytes), maxBytes)
	}
	stored := bytes
	if strings.HasSuffix(path, ".gz") {
		stored, err = zgzip.Compress(bytes)
		check(err)
	}
	writeNew(path, stored)
	info, err := os.Stat(path)
	check(err)
	written, err := os.ReadFile(path)
	check(err)
	return rec.Map{"sha256": hexSum(written), "bytes": info.Size(), "expanded_bytes": int64(len(bytes))}
}

// systemError is an operating-system error, which Ruby raises as SystemCallError.
type systemError struct{ error }

func check(err error) {
	if err != nil {
		panic(systemError{err})
	}
}

// Read is DocumentBundle.read: the payload of a bundle, or of a bare paper document.
func Read(path string) (payload rec.Map) {
	defer invalidBundle()
	info, err := os.Stat(path)
	check(err)
	if info.Size() > maxBytes {
		rec.Fail("Bundle exceeds compressed byte limit")
	}
	bytes, err := os.ReadFile(path)
	check(err)
	if strings.HasSuffix(path, ".gz") {
		bytes, err = zgzip.Decompress(bytes, maxBytes)
		if err != nil {
			panic(bundleError{err})
		}
	}
	if len(bytes) > maxBytes {
		rec.Fail("Bundle exceeds expanded byte limit")
	}
	parsed, err := canonjson.Decode(strings.NewReader(string(bytes)))
	if err != nil {
		panic(bundleError{err})
	}
	value := rec.AsMap(parsed)
	if version := value["schema_version"]; version == "paper-0.1" || version == "paper-0.2" {
		paperdoc.Validate(value)
		return rec.Map{"document": value, "layout": nil, "source_map": nil, "losses": []any{}, "capabilities": rec.Map{}}
	}
	version := rec.Fetch(value, "schema_version")
	if version != Version && !slices.Contains(inlineVersions, version) {
		rec.Fail("Unsupported bundle schema version")
	}
	payload = rec.FetchMap(value, "payload")
	hashes := rec.FetchMap(value, "sha256")
	if !rec.SameKeys(payload, Fields) || !rec.SameKeys(hashes, Fields) {
		rec.Fail("Bundle payload fields differ")
	}
	for _, key := range Fields {
		if hexSum(marshal(payload[key])) != rec.Fetch(hashes, key) {
			rec.Fail("Bundle content hash differs: %s", key)
		}
	}
	paperdoc.Validate(rec.FetchMap(payload, "document"))
	if version != Version {
		payload["layout"] = expandLayout(payload["layout"])
		return payload
	}
	directory, err := filepath.Abs(filepath.Dir(path))
	check(err)
	payload["layout"] = attachLayout(payload["layout"], directory)
	return payload
}

// bundleError is a JSON or zlib error, which Ruby's read rescues.
type bundleError struct{ error }

// invalidBundle is DocumentBundle.read's rescue of JSON::ParserError, Zlib::Error, KeyError
// and TypeError.
func invalidBundle() {
	recovered := recover()
	if recovered == nil {
		return
	}
	switch failure := recovered.(type) {
	case bundleError:
		panic(rec.Failure{Message: "Invalid document bundle: " + failure.Error()})
	case rec.Failure:
		if failure.Lookup {
			panic(rec.Failure{Message: "Invalid document bundle: " + failure.Message})
		}
	}
	panic(recovered)
}
