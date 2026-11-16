package fontbinding

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/fontmodel"
	"github.com/sarielhp/relatex/internal/rec"
)

func failure(run func()) (message string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			message = recovered.(rec.Failure).Message
		}
	}()
	run()
	return ""
}

const vpl = "(VTITLE x)\n(MAPFONT D 0\n   (FONTNAME cmr10)\n   )\n" +
	"(CHARACTER C A\n   (CHARWD R 0.5)\n   (MAP\n      (SETCHAR C B)\n      )\n   )\n" +
	"(CHARACTER O 102\n   (MAP\n      (SELECTFONT D 0)\n      (SETCHAR D 67)\n      )\n   )\n"

func TestVirtualPackets(t *testing.T) {
	packets := VirtualPackets(vpl, []int64{65, 66})
	slots := fontmodel.PacketSlots(packets)
	if !rec.Equal(slots, rec.Map{"65": int64(66), "66": int64(67)}) {
		t.Errorf("slots = %v", slots)
	}
	if got := failure(func() { VirtualPackets(vpl, []int64{67}) }); got != "Missing virtual font character" {
		t.Errorf("missing: %q", got)
	}
	composite := rec.Map{"65": "(CHARACTER C A\n   (MAP\n      (SETCHAR C B)\n      (SETCHAR C C)\n      )\n   )\n"}
	if got := failure(func() { fontmodel.PacketSlots(composite) }); got != "Unsupported transformed or composite virtual glyph" {
		t.Errorf("composite: %q", got)
	}
}

func TestVFFonts(t *testing.T) {
	data := []byte{247, 202, 0}
	data = append(data, make([]byte, 8)...)
	definition := append([]byte{243, 0}, make([]byte, 12)...)
	definition = append(definition, 0, 5)
	definition = append(definition, []byte("cmr10")...)
	data = append(append(data, definition...), 248)
	if got := VFFonts(data); len(got) != 1 || got[0] != "cmr10" {
		t.Errorf("VFFonts = %v", got)
	}
}

// txfonts' text fonts are virtual fonts on two unscaled bases (the Nimbus face and txfonts' own
// symbols): the binding names both PDF faces and keeps the virtual font's own TFM, and the slot
// binding, which needs one base, still refuses it.
func TestVirtualFontOnSeveralUnscaledBasesNamesEveryFace(t *testing.T) {
	skipWithoutTeX(t)
	binding := New(nil)
	if binding.lookup("t1xr.vf", true) == "" || command.Executable("vftovp") == "" {
		t.Skip("txfonts or vftovp is not installed")
	}
	directory := t.TempDir()
	for _, name := range []string{"t1xr", "t1xsl"} {
		got := New(nil).Resolve(name, directory)
		faces, _ := got["postscript_names"].([]any)
		if got["format"] != "virtual" || len(faces) != 2 || faces[0] != "NimbusRomNo9L-Regu" || faces[1] != "rtxr" {
			t.Errorf("%s: binding %v", name, got)
		}
		if got["tfm_sha256"] != command.FileDigest(rec.FetchString(got, "tfm_path")) {
			t.Errorf("%s: the virtual font's own TFM is not recorded", name)
		}
	}
	if message := failure(func() { New(nil).ResolveSlots("t1xr", []int64{65}, directory) }); message != "Unsupported multiple or scaled virtual font bases" {
		t.Errorf("slot binding: %q", message)
	}
}

// Times small caps (ptmrc8t) sets its lower case from its base at 0.8: the binding names the one
// PDF face and both scales (papers 16 and 49).
func TestSmallCapsVirtualFontNamesItsScales(t *testing.T) {
	skipWithoutTeX(t)
	if New(nil).lookup("ptmrc8t.vf", true) == "" || command.Executable("vftovp") == "" {
		t.Skip("psnfss or vftovp is not installed")
	}
	got := New(nil).Resolve("ptmrc8t", t.TempDir())
	scales, _ := got["scales"].([]any)
	faces, _ := got["postscript_names"].([]any)
	if got["format"] != "virtual" || len(scales) != 2 || scales[0] != "0.8" || scales[1] != "1.0" || len(faces) != 1 {
		t.Fatalf("binding %v", got)
	}
}

// Times slanted roman (ptmro8t) is the roman face under a SlantFont transform in the PDF font
// map: the binding names the roman PDF face, not nothing.
func TestSlantedFontMapEntryNamesItsFace(t *testing.T) {
	line := `ptmro8r NimbusRomNo9L-Regu " .167 SlantFont TeXBase1Encoding ReEncodeFont " <8r.enc <utmr8a.pfb`
	match := mapEntry.FindStringSubmatch(line)
	if match == nil || match[1] != "NimbusRomNo9L-Regu" || match[2] != "TeXBase1Encoding" || match[3] != "8r.enc" {
		t.Fatalf("match %v", match)
	}
}

func TestDigestsUseForwardSlashes(t *testing.T) {
	winPath := `C:\texlive\2026\texmf-dist\fonts\type1\public\amsfonts\cm\cmr10.pfb`
	got := digestsWith([]string{winPath}, func(p string) string { return strings.ReplaceAll(p, `\`, "/") }, func(string) string { return "abc" })
	want := "C:/texlive/2026/texmf-dist/fonts/type1/public/amsfonts/cm/cmr10.pfb"
	if got[want] != "abc" || len(got) != 1 {
		t.Errorf("digests = %v, want key %q", got, want)
	}
}
