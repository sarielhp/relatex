package assets

import (
	"runtime"
	"slices"
	"sync"

	"github.com/sarielhp/relatex/internal/inline"
	"github.com/sarielhp/relatex/internal/mediamodel"
	"github.com/sarielhp/relatex/internal/omap"
	"github.com/sarielhp/relatex/internal/rec"
)

// readingImages are the image spans of a reading's blocks and note blocks, table cells included.
func readingImages(reading rec.Map) []rec.Map {
	blocks := rec.FetchList(reading, "blocks")
	for _, note := range rec.FetchList(rec.FetchMap(reading, "notes"), "notes") {
		blocks = append(slices.Clip(blocks), rec.FetchList(rec.AsMap(note), "blocks")...)
	}
	content := []any{}
	for _, block := range blocks {
		content = append(content, rec.FetchList(rec.AsMap(block), "content")...)
	}
	images := []rec.Map{}
	for _, span := range inline.Deep(content) {
		if rec.Fetch(span, "type") == "image" {
			images = append(images, span)
		}
	}
	return images
}

// FromReading is PaperMediaAssets.from_reading: the verified assets of a reading's images,
// read through read (a filename the probe recorded).
func FromReading(reading rec.Map, read func(name string) []byte) rec.Map {
	images := readingImages(reading)
	placed := slices.ContainsFunc(images, func(image rec.Map) bool { _, ok := image["placement"]; return ok })
	if !placed {
		return pngFromReading(images, read)
	}
	groups := omap.New[any, []rec.Map]()
	for _, image := range images {
		sha := rec.Fetch(image, "sha256")
		uses, _ := groups.Get(sha)
		groups.Set(sha, append(uses, image))
	}
	type group struct {
		sha  any
		uses []rec.Map
	}
	ordered := []group{}
	groups.Each(func(sha any, uses []rec.Map) { ordered = append(ordered, group{sha, uses}) })
	// Each source is turned into its representations by programs of its own: the groups run side by
	// side, and the first failure in order is the one raised.
	descriptors := make([]rec.Map, len(ordered))
	failures := make([]any, len(ordered))
	limit := make(chan struct{}, runtime.NumCPU())
	var wait sync.WaitGroup
	for index, item := range ordered {
		wait.Add(1)
		go func() {
			defer wait.Done()
			limit <- struct{}{}
			defer func() { <-limit }()
			defer func() { failures[index] = recover() }()
			descriptors[index] = describeUses(item.uses, read)
		}()
	}
	wait.Wait()
	for _, failure := range failures {
		if failure != nil {
			panic(failure)
		}
	}
	items := rec.Map{}
	for index, item := range ordered {
		items[rec.AsString(item.sha)] = descriptors[index]
	}
	return rec.Map{"status": "source_verified", "items": items}
}

// describeUses is the asset of one source: its descriptor with every placement of its uses.
func describeUses(uses []rec.Map, read func(name string) []byte) rec.Map {
	for _, image := range uses {
		if _, ok := image["placement"]; !ok {
			rec.Fail("Mixed legacy and placement-aware media in one producer")
		}
	}
	data := readSources(uses, read)
	if allSimplePNG(uses) {
		return mediamodel.PNGFromBytes(data, uses[0]).Descriptor()
	}
	builder := NewDerivatives(data, uses[0])
	var asset rec.Map
	for _, image := range uses {
		asset = builder.Add(rec.FetchMap(image, "placement"))
	}
	return asset
}

func allSimplePNG(uses []rec.Map) bool {
	for _, image := range uses {
		if rec.Fetch(image, "mime_type") != "image/png" || !mediamodel.SimplePlacement(rec.FetchMap(image, "placement")) {
			return false
		}
	}
	return true
}

func readSources(uses []rec.Map, read func(name string) []byte) []byte {
	files := map[string][]byte{}
	for _, image := range uses {
		path := rec.FetchString(image, "filename")
		if _, ok := files[path]; !ok {
			files[path] = read(path)
		}
	}
	for _, image := range uses {
		data := files[rec.FetchString(image, "filename")]
		if !rec.Equal(int64(len(data)), rec.Fetch(image, "bytes")) || hexDigest(data) != rec.Fetch(image, "sha256") {
			rec.Fail("Repeated media source declaration differs from original bytes")
		}
		if !rec.Equal(rec.Fetch(image, "mime_type"), rec.Fetch(uses[0], "mime_type")) {
			rec.Fail("Repeated media MIME declarations differ")
		}
	}
	return files[rec.FetchString(uses[0], "filename")]
}

// pngFromReading is PaperPNGAssets.from_reading: each observed file resolved through
// immutable probe evidence.
func pngFromReading(images []rec.Map, read func(name string) []byte) rec.Map {
	if len(images) == 0 {
		return rec.Map{"status": "unverified"}
	}
	files := map[string]*mediamodel.PNG{}
	items := rec.Map{}
	for _, image := range images {
		path := rec.FetchString(image, "filename")
		png, ok := files[path]
		if !ok {
			png = mediamodel.NewPNG(read(path), image)
			files[path] = png
		}
		for _, key := range []string{"sha256", "bytes", "pixel_width", "pixel_height"} {
			declared, has := image[key]
			if !has || !rec.Equal(png.Metadata[key], declared) {
				rec.Fail("Conflicting PNG source declarations")
			}
		}
		sha := rec.FetchString(image, "sha256")
		if _, seen := items[sha]; !seen {
			items[sha] = png.Descriptor()
		}
	}
	return rec.Map{"status": "source_verified", "items": items}
}
