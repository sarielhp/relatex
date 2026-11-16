package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/bundle"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/sourcemap"
)

func TestCapabilitiesOmitRemovedVerifyStage(t *testing.T) {
	if Root == "" {
		Root, _ = filepath.Abs("../../..")
	}
	if slices.Contains(layerOrder, "verify") {
		t.Fatalf("layerOrder still contains verify: %v", layerOrder)
	}
	report := capabilityReport(true)
	layers, _ := report.Get("layers").(ojson.Object)
	if layers.Has("verify") {
		t.Fatal("capabilityLayers still reports verify")
	}
	reuse, _ := report.Get("reuse").(ojson.Object)
	stages, _ := reuse.Get("stages").([]any)
	if slices.Contains(stages, any("verify")) {
		t.Fatalf("reuse.stages still lists verify: %v", stages)
	}
	status, _, stderr := run(t, "layers", t.TempDir(), "--stage", "verify")
	if status != 2 || !strings.Contains(stderr, "invalid argument") {
		t.Fatalf("layers --stage verify: status %d, stderr %q", status, stderr)
	}
}

func TestCapabilityReadersIncludeCurrentBundleAndSourceMapSchemas(t *testing.T) {
	if Root == "" {
		Root, _ = filepath.Abs("../../..")
	}
	readers := capabilityReaders()
	bundles, _ := readers.Get("bundle").([]any)
	if !slices.Contains(bundles, any(bundle.Version)) {
		t.Fatalf("readers.bundle = %v, missing %q", bundles, bundle.Version)
	}
	maps, _ := readers.Get("source_map").([]any)
	if !slices.Contains(maps, any(sourcemap.Schema)) {
		t.Fatalf("readers.source_map = %v, missing %q", maps, sourcemap.Schema)
	}
	checkSchemaEnum(t, filepath.Join(Root, "schemas/bundle.schema.json"), bundle.Version)
	checkSchemaEnum(t, filepath.Join(Root, "schemas/source-map.schema.json"), sourcemap.Schema)
}

func checkSchemaEnum(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Properties struct {
			SchemaVersion struct {
				Enum []string `json:"enum"`
			} `json:"schema_version"`
		} `json:"properties"`
	}
	if err = json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(doc.Properties.SchemaVersion.Enum, want) {
		t.Fatalf("%s schema_version enum = %v, missing %q", filepath.Base(path), doc.Properties.SchemaVersion.Enum, want)
	}
}
