package assets

import (
	_ "embed"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/links"
	"github.com/sarielhp/relatex/internal/mediamodel"
	"github.com/sarielhp/relatex/internal/rec"
)

// orcidLogo is the ORCID iD icon as a 16 bp vector PDF, written by tools/orcid_logo from the
// logo's path data in orcidlink.sty.
//
// Credit: the ORCID iD icon is a trademark of ORCID, Inc.; ORCID's brand guidelines permit the iD icon
// beside ORCID identifiers (https://info.orcid.org/brand-guidelines/).
//
//go:embed orcid.pdf
var orcidLogo []byte

// orcidSizeBP is the logo PDF's page side.
const orcidSizeBP = 16

// applyOrcid replaces each rendered picture that is the logo of an ORCID link (the reading
// kept that link on the span) with the shared ORCID logo, at the size and baseline the
// picture had on the page; the link stays. It returns the pictures still to crop.
func applyOrcid(spans []rec.Map, files map[string][]byte) []rec.Map {
	crops := []rec.Map{}
	for _, span := range spans {
		link, linked := span["external_link"]
		if !linked {
			crops = append(crops, span)
			continue
		}
		region := rec.FetchList(span, "region_sp")
		width := rec.AsInt(region[2]) - rec.AsInt(region[0])
		height := rec.AsInt(region[3]) - rec.AsInt(region[1])
		placement := orcidPlacement(width, height, rec.FetchInt(span, "depth_sp"))
		id := rec.Fetch(span, "id")
		for key := range span {
			delete(span, key)
		}
		sha := hexDigest(orcidLogo)
		for key, value := range (rec.Map{"type": "image", "id": id, "filename": links.OrcidSource, "sha256": sha,
			"bytes": int64(len(orcidLogo)), "mime_type": "application/pdf", "placement": placement,
			"width_sp": placement["width_sp"], "height_sp": placement["height_sp"], "depth_sp": placement["depth_sp"],
			"external_link": link}) {
			span[key] = value
		}
		files[links.OrcidSource] = orcidLogo
	}
	return crops
}

// orcidPlacement shows the whole logo page at the picture's size; the logo is square, so the
// larger side of the picture's box (height counting the depth) sets it, centred on that box.
func orcidPlacement(width, height, depth int64) rec.Map {
	side := max(width, height, 1)
	identity := []any{1.0, 0.0, 0.0, 1.0, 0.0, 0.0}
	box := []any{int64(0), int64(0), int64(orcidSizeBP), int64(orcidSizeBP)}
	data := rec.Map{"schema_version": mediamodel.PlacementVersion, "page": int64(1), "pagebox": "mediabox",
		"source_box_bp": box, "viewport_bp": box, "clip": false, "matrix": identity,
		"width_sp": side, "height_sp": side, "depth_sp": depth + (side-height)/2,
		"operations": []any{}}
	id, err := canonjson.Digest(data)
	if err != nil {
		rec.Fail("%v", err)
	}
	data["id"] = id
	mediamodel.ValidatePlacement(data)
	return data
}
