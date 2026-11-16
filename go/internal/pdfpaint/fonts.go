package pdfpaint

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/fontbinding"
	"github.com/sarielhp/relatex/internal/paintmodel"
	"github.com/sarielhp/relatex/internal/rec"
)

// captureFonts is PDFTableFonts#capture.
func captureFonts(pdf string, glyphs []rec.Map, directory string) rec.Map {
	if exists(directory) {
		rec.Fail("Table font output exists")
	}
	mkdir(directory)
	required := paintmodel.FontRequirements(glyphs)
	found := rec.Map{}
	proof := rec.Map{"schema_version": paintmodel.TableFontsVersion, "pdf_sha256": command.FileDigest(pdf), "bindings": found}
	if required.Len() > 0 {
		_, inventory := fontInventory(pdf, filepath.Join(directory, "pdf"))
		required.Each(func(name string, entry paintmodel.Requirement) {
			found[name] = bindFont(entry, inventory, directory)
		})
	}
	paintmodel.ValidateFonts(proof, glyphs)
	fontbinding.WriteJSON(filepath.Join(directory, "fonts.json"), proof)
	return proof
}

// faces is PDFTableFonts.faces: the Type1 font objects of the PDF with this normalized name.
func faces(inventory objects, name string) []any {
	result := []any{}
	for _, reference := range inventory.Keys() {
		if !paintmodel.ObjectRef.MatchString(reference) || !inventory.mentions(reference, []byte(`"/Type1"`)) {
			continue
		}
		row, _ := inventory.Get(reference)
		font, base, ok := fontRow(reference, row)
		if !ok || paintmodel.FontName(strings.TrimPrefix(base, "/")) != name {
			continue
		}
		data := cmapStream(inventory, font)
		encoding := font["/Encoding"]
		if text, isString := encoding.(string); isString && strings.HasSuffix(text, " R") {
			encoding = rec.Fetch(fetchObject(inventory, "obj:"+text), "value")
		}
		result = append(result, rec.Map{"reference": reference, "postscript_name": strings.TrimPrefix(base, "/"),
			"cmap": data, "encoding": encoding})
	}
	return result
}

func bindFont(entry paintmodel.Requirement, inventory objects, directory string) rec.Map {
	name := paintmodel.FontName(entry.Font)
	candidates := faces(inventory, name)
	var source any
	if len(candidates) == 0 {
		sum := sha256.Sum256([]byte(entry.Font))
		path := filepath.Join(directory, "source-"+hex.EncodeToString(sum[:])[:16])
		mkdir(path)
		resolved := fontbinding.New(nil).ResolveSlots(entry.Font, entry.Slots, path)
		source = resolved
		name = paintmodel.FontName(rec.FetchString(resolved, "postscript_name"))
		candidates = faces(inventory, name)
	}
	slots := make([]any, len(entry.Slots))
	for index, slot := range entry.Slots {
		slots[index] = strconv.FormatInt(slot, 10)
	}
	binding := rec.Map{"native_font": entry.Font, "pdf_font": name, "source": source, "faces": candidates, "slots": slots}
	paintmodel.ValidateFontBinding(binding)
	return binding
}
