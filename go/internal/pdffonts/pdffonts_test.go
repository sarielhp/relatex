package pdffonts

import (
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/glyphnames"
)

// memoryStore is a Store over a map of objects, streams as {dict, data}.
type memoryStore struct {
	objects map[string]any
	data    map[string][]byte
	next    int
}

func newMemoryStore() *memoryStore {
	return &memoryStore{objects: map[string]any{}, data: map[string][]byte{}, next: 100}
}

func (s *memoryStore) Resolve(value any) any {
	if ref, ok := value.(string); ok && strings.HasSuffix(ref, " R") {
		return s.objects[ref]
	}
	return value
}

func (s *memoryStore) StreamData(ref string) ([]byte, error) {
	data, ok := s.data[ref]
	if !ok {
		return nil, fmt.Errorf("%s is no stream", ref)
	}
	return data, nil
}

func (s *memoryStore) Set(ref string, value map[string]any) { s.objects[ref] = value }

func (s *memoryStore) SetStream(ref string, dict map[string]any, data []byte) {
	clean := map[string]any{}
	for key, value := range dict {
		if key != "/Filter" && key != "/DecodeParms" && key != "/Length" {
			clean[key] = value
		}
	}
	s.objects[ref], s.data[ref] = clean, data
}

func (s *memoryStore) Add(value map[string]any) string {
	s.next++
	ref := fmt.Sprintf("%d 0 R", s.next)
	s.objects[ref] = value
	return ref
}

func (s *memoryStore) AddStream(dict map[string]any, data []byte) string {
	ref := s.Add(dict)
	s.data[ref] = data
	return ref
}

func TestNamedEncodings(t *testing.T) {
	if len(macRomanHigh) != 128 || len(latin1Names) != 95 {
		t.Fatalf("MacRoman has %d high codes, Latin-1 %d names", len(macRomanHigh), len(latin1Names))
	}
	standard, win, mac := NamedEncoding("/StandardEncoding"), NamedEncoding("/WinAnsiEncoding"), NamedEncoding("/MacRomanEncoding")
	checks := []struct {
		got, want string
	}{{standard[0x27], "quoteright"}, {standard[0xAE], "fi"}, {standard[0xFB], "germandbls"}, {win[0x27], "quotesingle"},
		{win[0x80], "Euro"}, {win[0xE9], "eacute"}, {win[0xFF], "ydieresis"}, {mac[0xCA], "space"}, {mac[0xDE], "fi"},
		{mac[0xF5], "dotlessi"}, {mac[0xFF], "caron"}, {standard[0x41], "A"}, {win[0x7E], "asciitilde"}}
	for index, check := range checks {
		if check.got != check.want {
			t.Errorf("check %d: %q, want %q", index, check.got, check.want)
		}
	}
}

func TestStandardFontNames(t *testing.T) {
	cases := map[string]string{"/Helvetica": "Helvetica", "Helvetica,Bold": "Helvetica-Bold", "Helvetica,Italic": "Helvetica-Oblique",
		"Times-Roman,BoldItalic": "Times-BoldItalic", "Symbol": "Symbol", "ArialMT": "Helvetica", "Helvetica,Light": "",
		"Arial-BoldMT,Italic": "Helvetica-BoldOblique", "TimesNewRomanPS-ItalicMT": "Times-Italic", "Tahoma": "",
		"Helvetica-Narrow": "", "CourierNewPSMT": "Courier", "Symbol,Bold": "", "Times New Roman,Bold": "Times-Bold",
		"Times New Roman": "Times-Roman", "Courier New,Italic": "Courier-Oblique"}
	for name, want := range cases {
		if got := StandardFont(name); got != want {
			t.Errorf("StandardFont(%s) = %q, want %q", name, got, want)
		}
	}
}

// standardProgram loads a URW program or skips the test without one.
func loadStandard(t *testing.T, standard string) *Type1 {
	t.Helper()
	var p programs
	program := p.load(standard)
	if program == nil {
		t.Skipf("no URW program for %s", standard)
	}
	return program
}

func TestURWProgramsReadTheirWidthsAndEncodings(t *testing.T) {
	cases := []struct {
		font, glyph string
		width       float64
		code        int
		name        string
	}{{"Helvetica", "A", 667, 65, "A"}, {"Times-Roman", "A", 722, 0xAE, "fi"}, {"Courier", "A", 600, 66, "B"},
		{"Symbol", "alpha", 631, 109, "mu"}, {"ZapfDingbats", "a1", 974, 33, "a1"}}
	for _, c := range cases {
		program := loadStandard(t, c.font)
		if program.Widths[c.glyph] != c.width || program.Encoding[c.code] != c.name {
			t.Errorf("%s: %s is %v wide (want %v), code %d is %q (want %s)", c.font, c.glyph, program.Widths[c.glyph], c.width,
				c.code, program.Encoding[c.code], c.name)
		}
		if len(program.Clear) == 0 || len(program.Binary) < 1000 || program.BBox[2] <= 0 {
			t.Errorf("%s: program parts %d/%d/%d, bbox %v", c.font, len(program.Clear), len(program.Binary), len(program.Trailer), program.BBox)
		}
		again, err := ParseType1(fontFileData(program), len(program.Clear), len(program.Binary))
		if err != nil || again.Widths[c.glyph] != c.width {
			t.Errorf("%s: the embedded stream reads back as %v (%v)", c.font, again, err)
		}
	}
}

func fontFileData(program *Type1) []byte {
	_, data := fontFileStream(program)
	return data
}

func TestToUnicodeMapsReadAndWritten(t *testing.T) {
	cmap := "1 begincodespacerange <00> <FF> endcodespacerange\n2 beginbfchar\n<41> <0041>\n<7E> <02F7>\nendbfchar\n" +
		"1 beginbfrange\n<61> <63> <0061>\nendbfrange\n1 beginbfrange\n<30> <31> [<2080> <D835DC00>]\nendbfrange\n"
	mapped, width := ReadCMap(cmap)
	want := map[int]string{0x41: "0041", 0x7E: "02F7", 0x61: "0061", 0x62: "0062", 0x63: "0063", 0x30: "2080",
		0x31: "D835DC00"}
	if len(mapped) != len(want) || width != 1 {
		t.Fatalf("mapped %v, width %d", mapped, width)
	}
	for code, hex := range want {
		if mapped[code] != hex {
			t.Fatalf("code %X maps to %q, want %q", code, mapped[code], hex)
		}
	}
	if again, _ := ReadCMap(WriteCMap(mapped, 1)); len(again) != len(want) || again[0x31] != "D835DC00" {
		t.Fatalf("written map reads back as %v", again)
	}
	if _, wide := ReadCMap(WriteCMap(map[int]string{0x123: "0041"}, 2)); wide != 2 {
		t.Fatalf("a two-byte map reads back %d bytes wide", wide)
	}
}

func TestUnusableValues(t *testing.T) {
	cases := map[string]bool{"0041": false, "0000": true, "FEFF": true, "FFFE": true, "F8F3": true, "": true,
		"D835DC00": false, "DB80DC00": true, "00410000": true, "FFFD": false}
	for value, want := range cases {
		if Unusable(value) != want {
			t.Errorf("Unusable(%q) = %v", value, !want)
		}
	}
}

func testTable() glyphnames.Table {
	return glyphnames.Table{Adobe: map[string]string{"A": "A", "B": "B", "C": "C", "mu": "µ", "minus": "−"},
		TeX: map[string]string{}, Fonts: map[string]map[string]string{"psyr": {"mu": "μ"}}}
}

// ghostscriptPrime is the CFF program Ghostscript 7.05 embedded for a figure's CMSY10 subset
// (books 0004, page 555): one glyph, prime, at code 48 of a custom encoding.
const ghostscriptPrime = "01000402000101010e4b514a4d474c2b434d535931300001010128f81b01f81c02f81d036efe54faf0f99b051d004c4e" +
	"740d8c0c01a4f78412f74511f7420ff73f1000040101464c5b60436f7079726967687420284329203139393720416d65" +
	"726963616e204d617468656d61746963616c20536f63696574792e20416c6c2052696768747320526573657276656443" +
	"4d53593130436f6d7075746572204d6f6465726e7072696d65000000013000018a00020101023a0ef7a7b8a0f86d9f01" +
	"8bf77d03f793f87315929a8b948b8f08a96fa16f63826a7e871efb0dfc488a8789868b861980a584938d908b99911e0e" +
	"75a1f93fa106fb109007b30ae00b1e0a04379f0c09b39a0c0c0000"

func TestCFFBuiltInEncodingNamesTheGlyphs(t *testing.T) {
	data, _ := hex.DecodeString(ghostscriptPrime)
	encoding, err := ParseCFFEncoding(data)
	if err != nil || len(encoding) != 1 || encoding[48] != "prime" {
		t.Fatalf("encoding %v, %v", encoding, err)
	}
	if _, err := ParseCFFEncoding([]byte("%!PS-AdobeFont")); err == nil {
		t.Fatal("parsed a Type 1 program as CFF")
	}
	store := newMemoryStore()
	store.objects["3 0 R"], store.data["3 0 R"] = map[string]any{"/Subtype": "/Type1C"}, data
	store.objects["2 0 R"] = map[string]any{"/Type": "/FontDescriptor", "/Flags": 5.0, "/FontFile3": "3 0 R"}
	store.objects["1 0 R"] = map[string]any{"/Type": "/Font", "/Subtype": "/Type1", "/BaseFont": "/KQJMGL+CMSY10",
		"/FontDescriptor": "2 0 R", "/FirstChar": 48.0, "/LastChar": 48.0, "/Widths": []any{275.0}}
	table := testTable()
	table.TeX["prime"] = "′"
	fixer := NewFixer(store, table)
	fixer.Fix("1 0 R", Use{Codes: map[int]bool{48: true}})
	toUnicode, _ := store.objects["1 0 R"].(map[string]any)["/ToUnicode"].(string)
	map48, _ := store.StreamData(toUnicode)
	if mapped, _ := ReadCMap(string(map48)); mapped[48] != "2032" {
		t.Fatalf("a symbolic CFF font's prime (no Adobe Glyph List name) mapped %v", mapped)
	}
}

func TestDrawingFontsAreKnownByProgramAndTFMName(t *testing.T) {
	for name, want := range map[string]bool{"/NTQRLY+LINEW10": true, "/LINE10": true, "/lcircle10": true,
		"/NSOCPI+XYATIP-Medium": true, "/xybtip10": true, "/XYCMAT12": true, "/XYATIP13": false, "/CMR10": false,
		"/ABCDEF+bbding": false, "/LINEW": false} {
		if Drawing(name) != want {
			t.Errorf("Drawing(%s) = %v", name, !want)
		}
	}
}

func TestDrawingPiecesReadReplacementNeverADingbat(t *testing.T) {
	store := newMemoryStore()
	// linew10's glyphs are named a1 ... a127, as Zapf Dingbats' are: its a9 is no scissors.
	store.objects["1 0 R"] = map[string]any{"/Type": "/Font", "/Subtype": "/Type1", "/BaseFont": "/NTQRLY+LINEW10",
		"/Encoding": map[string]any{"/Differences": []any{8.0, "/a8", "/a9"}}, "/ToUnicode": "2 0 R"}
	store.data["2 0 R"] = []byte("1 begincodespacerange <00> <FF> endcodespacerange " +
		"2 beginbfchar <A0> <0020> <09> <2702> endbfchar")
	table := testTable()
	table.TeX["a8"] = "✁"
	fixer := NewFixer(store, table)
	fixer.Fix("1 0 R", Use{Codes: map[int]bool{8: true, 9: true}})
	data, _ := store.StreamData(store.objects["1 0 R"].(map[string]any)["/ToUnicode"].(string))
	mapped, _ := ReadCMap(string(data))
	if mapped[8] != replacement || mapped[9] != replacement || mapped[0xA0] != "0020" {
		t.Fatalf("mapped %v", mapped)
	}
	if !fixer.Drawn["1 0 R"] || len(fixer.Report.Drawn) != 2 || len(fixer.Report.Unspelled) != 0 {
		t.Fatalf("drawn %v, report %+v", fixer.Drawn, fixer.Report)
	}
}

func TestStandardFontsAreEmbeddedWithTheirWidths(t *testing.T) {
	loadStandard(t, "Helvetica")
	store := newMemoryStore()
	store.objects["1 0 R"] = map[string]any{"/Type": "/Font", "/Subtype": "/Type1", "/BaseFont": "/Helvetica"}
	store.objects["2 0 R"] = map[string]any{"/Type": "/Font", "/Subtype": "/Type1", "/BaseFont": "/Symbol"}
	fixer := NewFixer(store, testTable())
	fixer.Fix("1 0 R", Use{Codes: map[int]bool{65: true, 67: true}})
	fixer.Fix("2 0 R", Use{Codes: map[int]bool{109: true}})
	font := store.objects["1 0 R"].(map[string]any)
	descriptor := store.objects[font["/FontDescriptor"].(string)].(map[string]any)
	widths, _ := font["/Widths"].([]any)
	if descriptor["/FontFile"] == nil || descriptor["/Flags"] != 32 || font["/FirstChar"] != 65 || len(widths) != 3 ||
		widths[0] != 667 || widths[2] != 722 {
		t.Fatalf("Helvetica: %v, descriptor %v", font, descriptor)
	}
	if _, mapped := font["/ToUnicode"]; mapped {
		t.Fatal("Helvetica's StandardEncoding names are readable without a map")
	}
	symbol := store.objects["2 0 R"].(map[string]any)
	data, _ := store.StreamData(symbol["/ToUnicode"].(string))
	if mapped, _ := ReadCMap(string(data)); mapped[109] != "03BC" {
		t.Fatalf("Symbol's mu reads %q, want the Greek letter", mapped[109])
	}
	if len(fixer.Report.Embedded) != 2 {
		t.Fatalf("embedded %v", fixer.Report.Embedded)
	}
}

func TestToUnicodeValuesRespelledOrReplaced(t *testing.T) {
	store := newMemoryStore()
	store.objects["1 0 R"] = map[string]any{"/Type": "/Font", "/Subtype": "/Type1", "/BaseFont": "/ABCDEF+CMSY7",
		"/Encoding": map[string]any{"/Differences": []any{33.0, "/minus", "/unknownglyph"}}, "/ToUnicode": "2 0 R"}
	store.data["2 0 R"] = []byte("1 begincodespacerange <00> <FF> endcodespacerange 1 beginbfrange <21> <21> <0000> endbfrange")
	fixer := NewFixer(store, testTable())
	fixer.Fix("1 0 R", Use{Codes: map[int]bool{33: true, 34: true}})
	font := store.objects["1 0 R"].(map[string]any)
	data, _ := store.StreamData(font["/ToUnicode"].(string))
	mapped, _ := ReadCMap(string(data))
	if mapped[33] != "2212" || mapped[34] != replacement || fixer.Report.Replaced != 1 || fixer.Report.Codes != 1 {
		t.Fatalf("mapped %v, report %+v", mapped, fixer.Report)
	}
	if len(fixer.Report.Unspelled) != 1 || !strings.Contains(fixer.Report.Unspelled[0], "unknownglyph") {
		t.Fatalf("unspelled %v", fixer.Report.Unspelled)
	}
}

func TestWidthsCorrectedOnlyWhereNoGlyphMoves(t *testing.T) {
	program := loadStandard(t, "Helvetica")
	store := newMemoryStore()
	streamDict, data := fontFileStream(program)
	store.objects["3 0 R"], store.data["3 0 R"] = streamDict, data
	store.objects["2 0 R"] = map[string]any{"/Type": "/FontDescriptor", "/Flags": 32.0, "/FontFile": "3 0 R"}
	store.objects["1 0 R"] = map[string]any{"/Type": "/Font", "/Subtype": "/Type1", "/BaseFont": "/ABCDEF+NimbusSans-Regular",
		"/FontDescriptor": "2 0 R", "/FirstChar": 65.0, "/LastChar": 66.0, "/Widths": []any{600.0, 600.0}}
	fixer := NewFixer(store, testTable())
	fixer.Fix("1 0 R", Use{Codes: map[int]bool{65: true, 66: true}, Advanced: map[int]bool{65: true}})
	widths := store.objects["1 0 R"].(map[string]any)["/Widths"].([]any)
	if widths[0] != 600.0 || widths[1] != 667 || fixer.Report.Widths != 1 {
		t.Fatalf("widths %v (A positions a later glyph and keeps 600; B becomes 667)", widths)
	}
}

func TestParseType1RejectsOtherData(t *testing.T) {
	if _, err := ParseType1([]byte("%PDF-1.7 not a font"), 0, 0); err == nil {
		t.Fatal("parsed a non-font")
	}
}

func TestCompositeFontsLoseTheirPrivateUseValues(t *testing.T) {
	store := newMemoryStore()
	store.objects["1 0 R"] = map[string]any{"/Type": "/Font", "/Subtype": "/Type0", "/ToUnicode": "2 0 R"}
	store.data["2 0 R"] = []byte("1 begincodespacerange <0000> <FFFF> endcodespacerange " +
		"2 beginbfchar <0001> <03D5> <0002> <F110> endbfchar")
	fixer := NewFixer(store, testTable())
	if !fixer.Fix("1 0 R", Use{Codes: map[int]bool{}}) {
		t.Fatal("the private-use value stayed")
	}
	data, _ := store.StreamData(store.objects["1 0 R"].(map[string]any)["/ToUnicode"].(string))
	mapped, width := ReadCMap(string(data))
	if width != 2 || mapped[1] != "03D5" || mapped[2] != replacement {
		t.Fatalf("mapped %v (%d bytes)", mapped, width)
	}
}

func TestType3NamesAreSpelledOrReplaced(t *testing.T) {
	store := newMemoryStore()
	store.objects["1 0 R"] = map[string]any{"/Type": "/Font", "/Subtype": "/Type3", "/BaseFont": "/ArialMT",
		"/Encoding": map[string]any{"/Differences": []any{32.0, "/uni00A0", "/uni00000003"}}}
	fixer := NewFixer(store, testTable())
	fixer.Fix("1 0 R", Use{Codes: map[int]bool{32: true, 33: true, 34: true}})
	data, _ := store.StreamData(store.objects["1 0 R"].(map[string]any)["/ToUnicode"].(string))
	mapped, _ := ReadCMap(string(data))
	// uni00A0 is a code point; uni00000003 a glyph index (matplotlib's Type 3 fonts), no character.
	if mapped[32] != "00A0" || mapped[33] != replacement || len(mapped) != 2 {
		t.Fatalf("mapped %v (code 34 has no name and stays unmapped)", mapped)
	}
}

// A figure's composite font with no map at all (a Mac Symbol, a Word Calibri; Identity-H) gets one
// that reads the codes the pages show as U+FFFD; a font with another encoding is left alone.
func TestCompositeFontWithoutMapGetsOne(t *testing.T) {
	store := newMemoryStore()
	store.objects["1 0 R"] = map[string]any{"/Type": "/Font", "/Subtype": "/Type0", "/Encoding": "/Identity-H"}
	store.objects["3 0 R"] = map[string]any{"/Type": "/Font", "/Subtype": "/Type0", "/Encoding": "/UniJIS-UCS2-H"}
	fixer := NewFixer(store, testTable())
	if fixer.Fix("3 0 R", Use{Codes: map[int]bool{3: true}}) || !fixer.Fix("1 0 R", Use{Codes: map[int]bool{105: true}}) {
		t.Fatal("the fonts without maps were not treated as expected")
	}
	data, _ := store.StreamData(store.objects["1 0 R"].(map[string]any)["/ToUnicode"].(string))
	mapped, width := ReadCMap(string(data))
	if width != 2 || len(mapped) != 1 || mapped[105] != replacement {
		t.Fatalf("mapped %v (%d bytes)", mapped, width)
	}
}

// A symbolic TrueType font with no Encoding names no glyph: its shown codes read U+FFFD.
func TestUnnamedTrueTypeFontGetsAMap(t *testing.T) {
	store := newMemoryStore()
	store.objects["1 0 R"] = map[string]any{"/Type": "/Font", "/Subtype": "/TrueType", "/BaseFont": "/ABCDEF+CIDFont+F3",
		"/FontDescriptor": "2 0 R"}
	store.objects["2 0 R"] = map[string]any{"/Type": "/FontDescriptor", "/Flags": 4, "/FontFile2": "9 0 R"}
	fixer := NewFixer(store, testTable())
	if !fixer.Fix("1 0 R", Use{Codes: map[int]bool{42: true, 45: true}}) {
		t.Fatal("the unnamed TrueType font stayed without a map")
	}
	data, _ := store.StreamData(store.objects["1 0 R"].(map[string]any)["/ToUnicode"].(string))
	mapped, width := ReadCMap(string(data))
	if width != 1 || len(mapped) != 2 || mapped[42] != replacement || len(fixer.Report.Unspelled) != 0 {
		t.Fatalf("mapped %v (%d bytes), unspelled %v", mapped, width, fixer.Report.Unspelled)
	}
}
