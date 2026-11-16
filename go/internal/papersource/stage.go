package papersource

import (
	"os"
	"path/filepath"

	"github.com/sarielhp/relatex/internal/authormath"
	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/diagnostics"
	"github.com/sarielhp/relatex/internal/fontmodel"
	"github.com/sarielhp/relatex/internal/glyphowner"
	"github.com/sarielhp/relatex/internal/paperdoc"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/schema"
	"github.com/sarielhp/relatex/internal/sourcefonts"
	"github.com/sarielhp/relatex/internal/tablemodel"
)

// Reconcile is the 03-reconcile stage's block (GeneralConversion#ruby_reconcile): the probe
// reinspected, document.json, provenance.json and diagnostics.json written into out as
// Relatex.write_canonical_json writes them, and the stage result.
func Reconcile(root, probe, out string, options Options) rec.Map {
	authormath.Root = root
	tablemodel.Root, sourcefonts.Root, fontmodel.Root, paperdoc.Root = root, root, root, root
	profile, err := diagnostics.LoadProfile(root)
	if err != nil {
		rec.Fail("%v", err)
	}
	diags := diagnostics.New(profile, nil, false)
	source := New(root, probe, options, diags)
	document := source.Document()
	provenance := source.Provenance()
	ValidateContracts(root, diags.Map(), provenance)
	for name, value := range map[string]rec.Map{"document.json": document, "provenance.json": provenance,
		"diagnostics.json": diags.Map(), ChapterPagesFile: source.ChapterPages(document), PageListFile: source.PageList(document)} {
		if writeErr := canonjson.WriteFile(filepath.Join(out, name), value); writeErr != nil {
			rec.Fail("%v", writeErr)
		}
	}
	if owners := source.GlyphOwners(document); owners != nil {
		if writeErr := glyphowner.Write(filepath.Join(out, glyphowner.File), owners); writeErr != nil {
			rec.Fail("%v", writeErr)
		}
	}
	return rec.Map{"nodes": int64(len(rec.FetchMap(document, "nodes"))), "qualified": false}
}

// ValidateContracts is ReconcileContracts.validate!: diagnostics.json and provenance.json
// against their schemas (docs/stage-contracts.md).
func ValidateContracts(root string, diagnosticsData, provenance rec.Map) {
	validateContract(root, "reconcile-diagnostics.schema.json", diagnosticsData)
	validateContract(root, "reconcile-provenance.schema.json", provenance)
}

func validateContract(root, name string, value rec.Map) {
	file, err := os.Open(filepath.Join(root, "schemas", name))
	if err != nil {
		rec.Fail("%v", err)
	}
	defer file.Close()
	contract, err := canonjson.Decode(file)
	if err != nil {
		rec.Fail("%v", err)
	}
	schema.Validate(value, rec.AsMap(contract), "$")
}
