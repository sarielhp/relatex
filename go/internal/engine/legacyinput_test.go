package engine

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// legacyFixture copies test/fixtures/compat/NAME (the Ruby suite's fixtures) into a new tree as main.tex.
func legacyFixture(t *testing.T, name string) (string, []byte) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "test", "fixtures", "compat", name))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.tex"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return root, data
}

func readMain(t *testing.T, root string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "main.tex"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestCompatibilityRequestsReadTheInputencRefusal(t *testing.T) {
	log := "! Package inputenc Error: inputenc is not designed for xetex or luatex.\n(inputenc)                only UTF-8 supported.\n"
	if got := shimsOf(compatibilityRequests(log)); len(got) != 1 || got[0] != "inputenc" {
		t.Fatalf("shims %v", got)
	}
}

func TestLatin1SourceIsReencodedToUTF8(t *testing.T) {
	root, original := legacyFixture(t, "latin1.tex")
	changed := reencodeLegacyInput(root)
	if len(changed) != 1 || changed[0]["path"] != "main.tex" || changed[0]["encoding"] != "latin1" ||
		changed[0]["sha256"] != hexDigest(original) {
		t.Fatalf("changed %v", changed)
	}
	text := readMain(t, root)
	if !strings.Contains(text, "Café naïve «guillemets» Straße § 2, 3°C, ½.") || changed[0]["reencoded_sha256"] != hexDigest([]byte(text)) {
		t.Fatalf("text %q", text)
	}
	if again := reencodeLegacyInput(root); len(again) != 0 {
		t.Fatalf("a UTF-8 file is re-encoded again: %v", again)
	}
}

func TestWindows1252SourceIsReencodedToUTF8(t *testing.T) {
	root, _ := legacyFixture(t, "cp1252.tex")
	if changed := reencodeLegacyInput(root); len(changed) != 1 || changed[0]["encoding"] != "ansinew" {
		t.Fatalf("changed %v", changed)
	}
	if text := readMain(t, root); !strings.Contains(text, "\u201cQuoted\u201d \u2013 \u20ac 5, naïve \u2026") {
		t.Fatalf("text %q", text)
	}
}

func TestAnUndefinedByteLeavesTheFileAsItIs(t *testing.T) {
	root, original := legacyFixture(t, "latin1.tex")
	data := bytes.Replace(original, []byte("Caf"), []byte("Caf\x93"), 1)
	if err := os.WriteFile(filepath.Join(root, "main.tex"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if changed := reencodeLegacyInput(root); len(changed) != 0 || readMain(t, root) != string(data) {
		t.Fatalf("changed %v", changed)
	}
}

func TestAnAsciiSourceOrTwoEncodingsReencodeNothing(t *testing.T) {
	root, original := legacyFixture(t, "ascii_latin9.tex")
	if changed := reencodeLegacyInput(root); len(changed) != 0 || readMain(t, root) != string(original) {
		t.Fatalf("changed %v", changed)
	}
	root, latin1 := legacyFixture(t, "latin1.tex")
	other := "\\usepackage[cp1252]{inputenc}\n"
	if err := os.WriteFile(filepath.Join(root, "other.tex"), []byte(other), 0o644); err != nil {
		t.Fatal(err)
	}
	if changed := reencodeLegacyInput(root); len(changed) != 0 || readMain(t, root) != string(latin1) {
		t.Fatalf("two declared encodings re-encoded %v", changed)
	}
}

func TestADeclarationInACommentDoesNotCount(t *testing.T) {
	root := t.TempDir()
	source := "% \\usepackage[latin1]{inputenc}\n\\usepackage[utf8]{inputenc}\n\\inputencoding{latin1}\n"
	if err := os.WriteFile(filepath.Join(root, "main.tex"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := declaredInputEncodings(root); len(got) != 1 || got[0] != "latin1" {
		t.Fatalf("declared %v", got)
	}
}

func TestCompatibilityRequestsReadMicrotypeAndOlderPdfTeXNames(t *testing.T) {
	log := "! Package microtype Error: The spacing feature only works with pdftex 1.40\n" +
		"! Undefined control sequence.\nl.350 \\pdfoptionpdfminorversion\n"
	if got := shimsOf(compatibilityRequests(log)); len(got) != 2 || got[0] != "pdftex" || got[1] != "microtype" {
		t.Fatalf("shims %v", got)
	}
}

// graphicx's pdftex driver (\usepackage[pdftex]{graphicx}) fails on \pdffilesize under LuaLaTeX.
func TestCompatibilityRequestsReadPdfFilesize(t *testing.T) {
	log := "! Undefined control sequence.\n\\Gread@pdftex ...\\expandafter \\relax \\pdffilesize \n  {#1}\n"
	if got := shimsOf(compatibilityRequests(log)); len(got) != 1 || got[0] != "pdftex" {
		t.Fatalf("shims %v", got)
	}
}

// fontawesome5's [pro] option needs fonts TeX Live does not carry; the layer asks for the Free stand-ins.
func TestCompatibilityRequestsReadAMissingFontAwesomePro(t *testing.T) {
	log := "! Font \\TU/fontawesomepro/solid/n/10=FontAwesome5Pro-Solid:script=latn; at 10pt\n not loadable: metric data not found or bad.\n"
	if got := shimsOf(compatibilityRequests(log)); len(got) != 1 || got[0] != "fontawesome" {
		t.Fatalf("shims %v", got)
	}
	if got := compatibilityRequests("! Font \\TU/lmr/m/n/10=x not loadable\n"); len(got) != 0 {
		t.Fatalf("another font asks %v", got)
	}
}

func TestWarningRequestsReadACharacterAnEightBitFontLacks(t *testing.T) {
	log := "Missing character: There is no A (U+0041) in font ec-lmr10!\n"
	if got := warningRequests(log); len(got) != 0 {
		t.Fatalf("ASCII asks %v", got)
	}
	log += "Missing character: There is no \u2019 (U+2019) in font ec-lmr10!\n"
	if got := shimsOf(warningRequests(log)); len(got) != 1 || got[0] != "textmap" {
		t.Fatalf("shims %v", got)
	}
	opentype := "Missing character: There is no \u4e2d (U+4E2D) in font [lmroman10-regular]:+tlig;!\n"
	if got := warningRequests(opentype); len(got) != 0 {
		t.Fatalf("an OpenType font asks %v", got)
	}
}
