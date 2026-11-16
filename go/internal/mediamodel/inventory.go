package mediamodel

import (
	"math"
	"regexp"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/geometry"
	"github.com/sarielhp/relatex/internal/rec"
)

// ValidatePNGAssets is PaperPNGAssets.validate!(assets, images): the document's asset inventory
// holds exactly the retained images' assets, each verified against its bytes.
func ValidatePNGAssets(assets rec.Map, images []rec.Map) {
	if len(images) == 0 && rec.SameKeys(assets, []string{"status"}) && assets["status"] == "unverified" {
		return
	}
	items, isMap := assets["items"].(rec.Map)
	if !rec.SameKeys(assets, []string{"items", "status"}) || assets["status"] != "source_verified" || !isMap {
		rec.Fail("Paper images require verified source assets")
	}
	expected := map[string]bool{}
	for _, image := range images {
		expected[rec.FetchString(image, "asset")] = true
	}
	if len(expected) != len(items) {
		rec.Fail("Paper asset inventory differs from retained images")
	}
	for digest := range items {
		if !expected[digest] {
			rec.Fail("Paper asset inventory differs from retained images")
		}
	}
	keys := make([]string, 0, len(items))
	for digest := range items {
		keys = append(keys, digest)
	}
	slices.Sort(keys)
	for _, digest := range keys {
		descriptor := rec.AsMap(items[digest])
		if rec.Fetch(descriptor, "sha256") != digest {
			rec.Fail("Paper asset digest key mismatch")
		}
		if descriptor["schema_version"] == MediaVersion {
			NewMediaAsset(descriptor)
		} else {
			ValidatePNGDescriptor(descriptor)
		}
	}
}

// SimplePlacement is MediaPlacement.simple?: a whole first page, unclipped, untransformed,
// on the baseline.
func SimplePlacement(data rec.Map) bool {
	ValidatePlacement(data)
	depth, _ := rec.Number(rec.Fetch(data, "depth_sp"))
	if len(rec.FetchList(data, "operations")) > 0 || rec.Fetch(data, "clip") == true || !rec.Equal(rec.Fetch(data, "page"), int64(1)) ||
		depth != 0 {
		return false
	}
	source, viewport := rec.FetchList(data, "source_box_bp"), rec.FetchList(data, "viewport_bp")
	tolerance := geometry.SPToBP(2)
	for index := range source {
		if index >= len(viewport) {
			return true // zip pairs a missing value with nil; validation already bounds both at four.
		}
		a, _ := rec.Number(source[index])
		b, _ := rec.Number(viewport[index])
		if math.Abs(a-b) > tolerance {
			return false
		}
	}
	return true
}

var unsafeSourcePath = regexp.MustCompile(`[\\\x00-\x1f]`)

// SafeSourcePath is MediaAsset.safe_source_path?: a figure's source as the author included
// it, relative and inside the paper.
func SafeSourcePath(value any) bool {
	path, ok := value.(string)
	if !ok || path == "" || strings.HasPrefix(path, "/") || unsafeSourcePath.MatchString(path) {
		return false
	}
	// A doubled slash ("figures//a.pdf", as an author wrote it) names the same file; only a
	// trailing slash, "." and ".." are unsafe.
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if (part == "" && i == len(parts)-1) || part == "." || part == ".." {
			return false
		}
	}
	return true
}
