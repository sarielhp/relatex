package mediamodel

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/rec"
)

// MediaVersion is MediaAsset::VERSION.
const MediaVersion = "media-1"

// MediaAsset is MediaAsset: portable original bytes and explicit target derivatives share
// one identity. A placement belongs to the document; it is never the identity of its source.
type MediaAsset struct {
	Data rec.Map
}

var (
	verifiedMedia     = map[string]bool{}
	verifiedMediaLock sync.Mutex
)

// NewMediaAsset is MediaAsset.new(descriptor): a media-1 descriptor, or a legacy PNG one.
func NewMediaAsset(descriptor rec.Map) *MediaAsset {
	asset := &MediaAsset{Data: descriptor}
	if descriptor["schema_version"] != MediaVersion {
		asset.Data = legacyPNG(descriptor)
	}
	asset.validate()
	return asset
}

// Encoded is MediaAsset.encoded.
func Encoded(data []byte, mime string) rec.Map {
	if _, ok := Extensions[mime]; !ok {
		rec.Fail("Unknown media type")
	}
	return rec.Map{"mime_type": mime, "sha256": hexDigest(data), "bytes": int64(len(data)),
		"data_uri": "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)}
}

// Representation is MediaAsset#representation; placement is nil for the asset itself.
func (a *MediaAsset) Representation(format string, placement rec.Map) rec.Map {
	representations := rec.FetchMap(a.Data, "representations")
	if placement != nil {
		ValidatePlacement(placement)
		id := rec.FetchString(placement, "id")
		retained, ok := rec.FetchMap(a.Data, "placements")[id]
		if !ok {
			panic(rec.Failure{Message: fmt.Sprintf("key not found: %q", id), Lookup: true})
		}
		if !rec.Equal(retained, placement) {
			rec.Fail("Media placement differs from its derivative")
		}
		// One display form serves every target; older models stored it once per target.
		if item, ok := representations[format+":"+id]; ok {
			return rec.AsMap(item)
		}
		return rec.FetchMap(representations, "display:"+id)
	}
	if item, ok := representations[format]; ok {
		return rec.AsMap(item)
	}
	original := rec.FetchMap(representations, "original")
	if format != "original" && original["mime_type"] == "application/pdf" {
		rec.Fail("Media %s has no %s representation", rec.FetchString(a.Data, "sha256"), format)
	}
	return original
}

// Bytes is MediaAsset#bytes: the verified bytes of one representation.
func (a *MediaAsset) Bytes(item rec.Map) []byte { return decodeMedia(item) }

// Materialize is MediaAsset#materialize: the representation's file in directory, under the
// name the writer gave it (a figure's source, decision 068), or its content name when the
// name is empty.
func (a *MediaAsset) Materialize(directory, format string, placement rec.Map, name string) string {
	item := a.Representation(format, placement)
	data := decodeMedia(item)
	if name == "" {
		name = rec.FetchString(item, "sha256") + "." + Extensions[rec.FetchString(item, "mime_type")]
	}
	path := filepath.Join(directory, name)
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			rec.Fail("Materialized media path is not an owned file")
		}
		if command.FileDigest(path) != item["sha256"] {
			rec.Fail("Materialized media differs from requested content")
		}
		return name
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		rec.Fail("%v", err)
	}
	return name
}

func (a *MediaAsset) validate() {
	if rec.Fetch(a.Data, "schema_version") != MediaVersion {
		rec.Fail("Unknown media schema")
	}
	representations, ok := rec.Fetch(a.Data, "representations").(rec.Map)
	if !ok || len(representations) == 0 {
		rec.Fail("Media has no representations")
	}
	original := rec.FetchMap(representations, "original")
	if !rec.Equal(rec.Fetch(a.Data, "sha256"), rec.Fetch(original, "sha256")) {
		rec.Fail("Media source identity differs from original representation")
	}
	for _, item := range representations {
		verifyMedia(rec.AsMap(item))
	}
	placements, _ := a.Data["placements"].(rec.Map)
	for id, value := range placements {
		placement := rec.AsMap(value)
		ValidatePlacement(placement)
		if rec.Fetch(placement, "id") != id {
			rec.Fail("Media placement inventory differs")
		}
		for _, format := range []string{"html", "markdown", "typst"} {
			if !rec.Equal(rec.Fetch(a.Representation(format, placement), "placement"), id) {
				rec.Fail("Media representation belongs to another placement")
			}
		}
	}
}

// verifyMedia decodes and checks each exact representation once per process.
func verifyMedia(item rec.Map) {
	uri, _ := item["data_uri"].(string)
	key := fmt.Sprintf("%v\x00%v\x00%v\x00%s", item["mime_type"], item["sha256"], item["bytes"], uri)
	verifiedMediaLock.Lock()
	seen := verifiedMedia[key]
	verifiedMediaLock.Unlock()
	if seen {
		return
	}
	decodeMedia(item)
	verifiedMediaLock.Lock()
	verifiedMedia[key] = true
	verifiedMediaLock.Unlock()
}

func decodeMedia(item rec.Map) []byte {
	mime := rec.FetchString(item, "mime_type")
	if _, ok := Extensions[mime]; !ok {
		rec.Fail("Unknown media representation type")
	}
	prefix := "data:" + mime + ";base64,"
	uri, ok := rec.Fetch(item, "data_uri").(string)
	if !ok || !strings.HasPrefix(uri, prefix) {
		rec.Fail("Media MIME and data URI differ")
	}
	size, isInt := rec.Fetch(item, "bytes").(int64)
	if !isInt || size < 1 || size > MaxBytes {
		rec.Fail("Media representation exceeds limit")
	}
	data, err := strictDecode(uri[len(prefix):])
	if err != nil {
		rec.Fail("Invalid embedded media encoding: %s", err.Error())
	}
	if int64(len(data)) != size || hexDigest(data) != item["sha256"] {
		rec.Fail("Embedded media bytes differ from identity")
	}
	if mime == "image/svg+xml" {
		ValidateSVG(data)
	}
	return data
}

func legacyPNG(descriptor rec.Map) rec.Map {
	ValidatePNGDescriptor(descriptor)
	source := DataURIBytes(rec.FetchString(descriptor, "source_data_uri"))
	display := DataURIBytes(rec.FetchString(descriptor, "data_uri"))
	representation := Encoded(display, "image/png")
	metadata := rec.Map{}
	for _, key := range []string{"pixel_width", "pixel_height", "trailing_bytes"} {
		if value, ok := descriptor[key]; ok {
			metadata[key] = value
		}
	}
	return rec.Map{"schema_version": MediaVersion, "sha256": rec.Fetch(descriptor, "sha256"), "metadata": metadata,
		"representations": rec.Map{"original": Encoded(source, "image/png"), "html": representation, "markdown": representation,
			"typst": representation}}
}

// DataURIBytes is the strictly decoded data of a base64 data URI (what follows its comma).
func DataURIBytes(uri string) []byte {
	_, data, _ := strings.Cut(uri, ",")
	decoded, err := strictDecode(data)
	if err != nil {
		rec.Fail("Invalid embedded media encoding: %s", err.Error())
	}
	return decoded
}
