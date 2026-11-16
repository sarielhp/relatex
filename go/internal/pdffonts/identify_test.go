package pdffonts

import (
	"encoding/binary"
	"strings"
	"testing"
)

// testGlyph is a synthetic glyph: contours of on-curve points and an advance width.
type testGlyph struct {
	contours [][][2]int16
	width    uint16
}

// glyfProgram builds a TrueType program of outlines (head, hhea, hmtx, maxp, loca, glyf) with the
// extra tables given.
func glyfProgram(upem uint16, glyphs []testGlyph, extra map[string][]byte) []byte {
	var glyf, loca, hmtx []byte
	for _, g := range glyphs {
		loca = binary.BigEndian.AppendUint32(loca, uint32(len(glyf)))
		glyf = append(glyf, glyfEntry(g)...)
		hmtx = binary.BigEndian.AppendUint16(hmtx, g.width)
		hmtx = binary.BigEndian.AppendUint16(hmtx, 0)
	}
	loca = binary.BigEndian.AppendUint32(loca, uint32(len(glyf)))
	head := make([]byte, 54)
	binary.BigEndian.PutUint16(head[18:], upem)
	binary.BigEndian.PutUint16(head[50:], 1)
	hhea := make([]byte, 36)
	binary.BigEndian.PutUint16(hhea[34:], uint16(len(glyphs)))
	tables := map[string][]byte{"head": head, "hhea": hhea, "hmtx": hmtx, "maxp": maxp(uint16(len(glyphs))), "loca": loca, "glyf": glyf}
	for tag, data := range extra {
		tables[tag] = data
	}
	return sfnt(tables)
}

// glyfEntry is a simple glyph's glyf data (every point on-curve, 16-bit coordinates).
func glyfEntry(g testGlyph) []byte {
	if len(g.contours) == 0 {
		return nil
	}
	out := binary.BigEndian.AppendUint16(nil, uint16(len(g.contours)))
	out = append(out, make([]byte, 8)...)
	var xs, ys []byte
	var flags []byte
	total, px, py := 0, int16(0), int16(0)
	for _, contour := range g.contours {
		total += len(contour)
		out = binary.BigEndian.AppendUint16(out, uint16(total-1))
		for _, p := range contour {
			flags = append(flags, 1)
			xs = binary.BigEndian.AppendUint16(xs, uint16(p[0]-px))
			ys = binary.BigEndian.AppendUint16(ys, uint16(p[1]-py))
			px, py = p[0], p[1]
		}
	}
	out = append(out, 0, 0)
	out = append(append(append(out, flags...), xs...), ys...)
	return out
}

// ligatureTable is a GSUB table with one ligature: glyph first followed by next forms glyph.
func ligatureTable(first, next, ligature uint16) []byte {
	words := []uint16{1, 0, 0, 0, 10, // header: version 1.0, no scripts, no features, lookup list at 10
		1, 4, // lookup list: one lookup at 4
		4, 0, 1, 8, // lookup: type 4, flags, one subtable at 8
		1, 8, 1, 14, // LigatureSubstFormat1: coverage at 8, one set at 14
		1, 1, first, // coverage format 1
		1, 4, // ligature set: one ligature at 4
		ligature, 2, next}
	var out []byte
	for _, word := range words {
		out = binary.BigEndian.AppendUint16(out, word)
	}
	return out
}

func square(side int16) [][2]int16 { return [][2]int16{{0, 0}, {side, 0}, {side, side}, {0, side}} }

func triangle(side int16) [][2]int16 { return [][2]int16{{0, 0}, {side, 0}, {0, side}} }

// fakeFonts is a Locator with one installed font.
type fakeFonts struct {
	name string
	data []byte
}

func (f fakeFonts) Locate(base string) *Installed {
	if base != f.name {
		return nil
	}
	return &Installed{File: "/fonts/" + f.name + ".ttf", Data: f.data}
}

// cmapOf is a Unicode cmap (format 6 from the lowest to the highest code) of the codes given.
func cmapOf(codes map[int]uint16) []byte {
	low, high := 0xFFFF, 0
	for code := range codes {
		low, high = min(low, code), max(high, code)
	}
	glyphs := make([]uint16, high-low+1)
	for code, glyph := range codes {
		glyphs[code-low] = glyph
	}
	return cmapTable(subtable{3, 1, format6(uint16(low), glyphs...)})
}

// installedOriginal is an installed font of five glyphs: A (a square), B (a triangle), the glyph of
// the ligature AB (no code), and a space.
func installedOriginal() []byte {
	return glyfProgram(1000, []testGlyph{{}, {[][][2]int16{square(400)}, 600}, {[][][2]int16{triangle(300)}, 500},
		{[][][2]int16{square(100), triangle(50)}, 800}, {nil, 250}},
		map[string][]byte{"cmap": cmapOf(map[int]uint16{0x20: 4, 0x41: 1, 0x42: 2}), "GSUB": ligatureTable(1, 2, 3)})
}

// figureFixture is a figure font cut from installedOriginal at 2000 units per em (a name-less,
// cmap-less Type 0 TrueType font): glyph 1 is A (points reversed), 2 the ligature AB, 3 a space,
// 4 an empty glyph without width, 5 a shape the original lacks, 6 an empty half-em glyph.
func figureFixture(base string, tables ...map[string][]byte) (*memoryStore, *Fixer) {
	var extra map[string][]byte
	if len(tables) > 0 {
		extra = tables[0]
	}
	reversed := [][2]int16{{0, 800}, {800, 800}, {800, 0}, {0, 0}}
	program := glyfProgram(2000, []testGlyph{{}, {[][][2]int16{reversed}, 1200}, {[][][2]int16{square(200), triangle(100)}, 1600},
		{nil, 500}, {nil, 0}, {[][][2]int16{triangle(777)}, 900}, {nil, 1000}}, extra)
	store := newMemoryStore()
	store.objects["1 0 R"] = map[string]any{"/Subtype": "/Type0", "/Encoding": "/Identity-H", "/BaseFont": "/ABCDEF+" + base,
		"/DescendantFonts": []any{"2 0 R"}}
	store.objects["2 0 R"] = map[string]any{"/Subtype": "/CIDFontType2", "/BaseFont": "/ABCDEF+" + base, "/FontDescriptor": "3 0 R"}
	store.objects["3 0 R"] = map[string]any{"/FontFile2": store.AddStream(map[string]any{}, program)}
	return store, NewFixer(store, testTable())
}

func figureUse() Use {
	return Use{Codes: map[int]bool{1: true, 2: true, 3: true, 4: true, 5: true, 6: true}}
}

func toUnicodeOf(store *memoryStore) map[int]string {
	data, _ := store.StreamData(store.objects["1 0 R"].(map[string]any)["/ToUnicode"].(string))
	mapped, _ := ReadCMap(string(data))
	return mapped
}

// A subset glyph is identified by its outline in the installed original (scaled, started at
// another point, drawn the other way): a code through the cmap, a ligature through GSUB, an empty
// glyph through its advance; the shape the original lacks stays U+FFFD and is reported.
func TestFigureFontIsIdentifiedFromTheInstalledOriginal(t *testing.T) {
	store, fixer := figureFixture("Fake")
	fixer.Locator = fakeFonts{"Fake", installedOriginal()}
	if !fixer.Fix("1 0 R", figureUse()) {
		t.Fatal("no map written")
	}
	want := map[int]string{1: "0041", 2: "00410042", 3: "0020", 4: "200B", 5: replacement, 6: "2002"}
	got := toUnicodeOf(store)
	for code, value := range want {
		if got[code] != value {
			t.Errorf("CID %d reads %s, want %s", code, got[code], value)
		}
	}
	report := fixer.Report
	if len(report.Identified) != 1 || report.Identified[0].Exact != 1 || report.Identified[0].Ligature != 1 ||
		report.Identified[0].Width != 3 || report.Identified[0].File != "Fake.ttf" || len(report.Identified[0].SHA256) != 64 ||
		report.Identified[0].Version == "" {
		t.Errorf("identified %+v", report.Identified)
	}
	if len(report.Unidentified) != 1 || report.Unidentified[0].Glyphs != 1 || !report.Unidentified[0].Installed ||
		!strings.Contains(report.Alerts()[0].Message, "installed Fake did not identify 1 of its glyphs (glyph 5:") {
		t.Errorf("unidentified %+v alerts %+v", report.Unidentified, report.Alerts())
	}
}

// Without the original every glyph stays U+FFFD, and the alert names the font to install; a
// generated name (Ghostscript's CIDFont+F3) names no original.
func TestFigureFontWithoutOriginalAlertsToInstall(t *testing.T) {
	store, fixer := figureFixture("Fake-Bold")
	fixer.Fix("1 0 R", figureUse())
	for code, value := range toUnicodeOf(store) {
		if value != replacement {
			t.Errorf("CID %d reads %s without its original", code, value)
		}
	}
	alerts := fixer.Report.Alerts()
	if len(alerts) != 1 || alerts[0].Glyphs != 6 || len(fixer.Report.Identified) != 0 ||
		alerts[0].Message != "figure font Fake has no character map; install Fake so relatex can identify its 6 glyphs (they read as U+FFFD until then)" {
		t.Fatalf("alerts %+v", alerts)
	}
	_, generated := figureFixture("CIDFont+F3-GBpc-EUC-H")
	generated.Fix("1 0 R", figureUse())
	if message := generated.Report.Alerts()[0].Message; !strings.Contains(message, "CIDFont+F3 has no character map and names no original font") {
		t.Fatalf("message %s", message)
	}
}

// Several glyphs with the same outline and different characters (Latin and Greek letters) are not
// guessed; the same character through several codes is.
func TestIdenticalOutlinesWithDifferentCharactersStayUnknown(t *testing.T) {
	original := glyfProgram(1000, []testGlyph{{}, {[][][2]int16{square(400)}, 600}, {[][][2]int16{square(400)}, 600}, {[][][2]int16{square(400)}, 600}},
		map[string][]byte{"cmap": cmapOf(map[int]uint16{0x41: 1, 0x391: 2, 0xA0: 3, 0x20: 3})})
	font, err := parseOutlines(original)
	if err != nil {
		t.Fatal(err)
	}
	program, _ := ParseTrueType(original)
	found := &originalFont{outlines: font, unicode: program.UnicodeOf(), spellName: func(string) string { return "" }}
	shape, _ := font.shape(1)
	if text, _, why := found.identify(shape); text != "" || !strings.Contains(why, "different characters") {
		t.Fatalf("read %q (%s)", text, why)
	}
	if text, _ := found.textOf(3); text != " " {
		t.Fatalf("glyph 3 reads %q", text)
	}
}

// A charstring gives its width (the nominal width plus the first operand) and its anchors: lines,
// then curves (only their end points), with the contour closed by the next moveto or the end.
func TestCharstringOutlineAndWidth(t *testing.T) {
	number := func(v int) byte { return byte(v + 139) }
	// width operand 20, then 10 20 rmoveto, 50 40 hlineto (right, up), 10 5 6 7 hvcurveto, endchar
	code := []byte{number(20), number(10), number(20), 21, number(50), number(40), 6, number(10), number(5), number(6), number(7), 31, 14}
	run := &charstring{private: cffPrivate{nominalWidth: 500}}
	if err := run.exec(code, 0); err != nil {
		t.Fatal(err)
	}
	run.close()
	if run.width != 520 || len(run.contours) != 1 {
		t.Fatalf("width %v contours %v", run.width, run.contours)
	}
	if got := run.contours[0]; len(got) != 4 || got[0] != (point{10, 20}) || got[1] != (point{60, 20}) || got[2] != (point{60, 60}) || got[3] != (point{75, 73}) {
		t.Fatalf("anchors %v", got)
	}
}

// The real Calibri, when installed, reads its own glyphs back: the letter A by its outline and its
// ti glyph by the ligature; nothing of it is kept in the repository.
func TestInstalledCalibriIdentifiesItsOwnGlyphs(t *testing.T) {
	found := SystemFonts().Locate("Calibri")
	if found == nil {
		t.Skip("Calibri is not installed")
	}
	original := loadOriginal(found, func(string) string { return "" })
	if original == nil {
		t.Fatal("installed Calibri not read")
	}
	var ti []int
	for gid, sequences := range original.ligature {
		if text := original.ligatureText(gid, 0); text == "ti" && len(sequences) > 0 {
			ti = append(ti, gid)
		}
	}
	if len(ti) == 0 {
		t.Fatal("no ti ligature found")
	}
	shape, _ := original.outlines.shape(ti[0])
	if text, method, _ := original.identify(shape); text != "ti" || method != "ligature" {
		t.Fatalf("ti glyph reads %q by %s", text, method)
	}
	shape, _ = original.outlines.shape(original.outlines.glyphs / 4)
	if text, _, why := original.identify(shape); why != "" && !strings.Contains(why, "different") && !strings.Contains(why, "no character") {
		t.Fatalf("glyph reads %q: %s", text, why)
	}
}

// cffIndex encodes an INDEX of the items (offset size 1).
func cffIndex(items ...[]byte) []byte {
	if len(items) == 0 {
		return []byte{0, 0}
	}
	out := []byte{byte(len(items) >> 8), byte(len(items)), 1, 1}
	end := 1
	for _, item := range items {
		end += len(item)
		out = append(out, byte(end))
	}
	for _, item := range items {
		out = append(out, item...)
	}
	return out
}

// cffInt encodes an integer operand in five bytes (so that offsets do not change sizes).
func cffInt(value int) []byte { return binary.BigEndian.AppendUint32([]byte{29}, uint32(value)) }

// A CID-keyed CFF program is read: charset (CID -> glyph), the width from its Private DICT, the
// anchors of a charstring, FontMatrix 0.001.
func TestCFFOutlinesAreRead(t *testing.T) {
	number := func(v int) byte { return byte(v + 139) }
	notdef := []byte{14}
	box := []byte{number(40), number(10), number(20), 21, number(50), 6, number(30), 7, 14} // width 40 + nominal 100
	space := []byte{number(-60), 14}                                                        // width -60 + nominal 100
	strs, gsubrs := cffIndex(), cffIndex()
	header, names := []byte{1, 0, 4, 1}, cffIndex([]byte("T"))
	charset := []byte{0, 0, 7, 0, 9} // glyph 1 is CID 7, glyph 2 CID 9
	charStrings := cffIndex(notdef, box, space)
	private := []byte{number(300), 20, number(100), 21}
	topSize := len(cffIndex(make([]byte, 36)))
	base := len(header) + len(names) + topSize + len(strs) + len(gsubrs)
	fdArraySize := len(cffIndex(append(cffInt(len(private)), append(cffInt(0), 18)...)))
	top := append(cffInt(391), append(cffInt(392), cffInt(0)...)...)
	top = append(append(top, 12, 30), append(cffInt(base+len(charset)), 17)...)
	top = append(append(top, append(cffInt(base), 15)...), append(cffInt(base+len(charset)+len(charStrings)), 12, 36)...)
	privateAt := base + len(charset) + len(charStrings) + fdArraySize
	fd := append(cffInt(len(private)), append(cffInt(privateAt), 18)...)
	program := append(append(append(append(header, names...), cffIndex(top)...), strs...), gsubrs...)
	program = append(append(append(append(program, charset...), charStrings...), cffIndex(fd)...), private...)
	font, err := parseCFFOutlines(program)
	if err != nil {
		t.Fatal(err)
	}
	if font.cidGlyph[7] != 1 || font.cidGlyph[9] != 2 {
		t.Fatalf("charset %v", font.cidGlyph)
	}
	shape, ok := font.shape(1)
	if !ok || shape.width != 0.14 || len(shape.contours) != 1 || shape.contours[0][2] != (point{0.06, 0.05}) {
		t.Fatalf("box %+v %v", shape, ok)
	}
	if shape, ok = font.shape(2); !ok || !shape.empty() || shape.width != 0.04 {
		t.Fatalf("space %+v %v", shape, ok)
	}
}

// A name the PDF spells with #xx escapes (宋体 in papers/0217) is looked up decoded, for the glyphs
// as for the report.
func TestFigureFontWithEscapedNameIsLookedUpDecoded(t *testing.T) {
	store, fixer := figureFixture("#e5#ae#8b#e4#bd#93-GBpc-EUC-H")
	fixer.Locator = fakeFonts{"宋体", installedOriginal()}
	fixer.Fix("1 0 R", figureUse())
	if got := toUnicodeOf(store)[1]; got != "0041" {
		t.Errorf("CID 1 reads %s, want 0041", got)
	}
	if len(fixer.Report.Identified) != 1 || fixer.Report.Identified[0].Family != "宋体" {
		t.Errorf("identified %+v", fixer.Report.Identified)
	}
}

// nameTable is an sfnt name table with one Windows (UTF-16) PostScript name record.
func nameTable(postscript string) []byte {
	var value []byte
	for _, r := range postscript {
		value = binary.BigEndian.AppendUint16(value, uint16(r))
	}
	out := binary.BigEndian.AppendUint16(nil, 0)
	out = binary.BigEndian.AppendUint16(out, 1)
	out = binary.BigEndian.AppendUint16(out, 18)
	for _, field := range []int{3, 1, 0x409, 6, len(value), 0} {
		out = binary.BigEndian.AppendUint16(out, uint16(field))
	}
	return append(out, value...)
}

// A placeholder name (CIDFont+F3, papers/0217) is looked up by the PostScript name the embedded
// program keeps (SymbolMT there), and the report names that family.
func TestPlaceholderFigureFontIsLookedUpByItsOwnName(t *testing.T) {
	store, fixer := figureFixture("CIDFont+F3", map[string][]byte{"name": nameTable("QCLEEF+Fake")})
	fixer.Locator = fakeFonts{"Fake", installedOriginal()}
	fixer.Fix("1 0 R", figureUse())
	if got := toUnicodeOf(store)[1]; got != "0041" {
		t.Errorf("CID 1 reads %s, want 0041", got)
	}
	if len(fixer.Report.Identified) != 1 || fixer.Report.Identified[0].Family != "Fake" ||
		fixer.Report.Identified[0].Font != "CIDFont+F3" {
		t.Errorf("identified %+v", fixer.Report.Identified)
	}
}

// A glyph whose own font names no original (Acrobat's nameless CIDFont+F3-GBpc-EUC-H in papers/0217)
// is matched against the originals of the other fonts the document names, exactly, and the report
// says which font matched.
func TestNamelessFigureFontIsMatchedByAFontOfTheDocument(t *testing.T) {
	store, fixer := figureFixture("CIDFont+F3-GBpc-EUC-H")
	store.objects["9 0 R"] = map[string]any{"/Subtype": "/TrueType", "/BaseFont": "/ABCDEF+Fake"}
	fixer.Locator = fakeFonts{"Fake", installedOriginal()}
	fixer.Prepare([]string{"1 0 R", "9 0 R"})
	fixer.Fix("1 0 R", figureUse())
	got := toUnicodeOf(store)
	if got[1] != "0041" || got[3] != replacement {
		t.Errorf("CID 1 reads %s (want 0041), the empty CID 3 %s (want no width match: %s)", got[1], got[3], replacement)
	}
	if len(fixer.Report.Identified) != 1 || fixer.Report.Identified[0].File != "Fake.ttf" ||
		!strings.Contains(strings.Join(fixer.Report.Identified[0].Glyphs, ";"), "in Fake, a font of the document") {
		t.Errorf("identified %+v", fixer.Report.Identified)
	}
}

// Without Prepare, or when no font of the document matches, the glyph stays U+FFFD.
func TestNamelessFigureFontWithoutDocumentFontsStaysUnknown(t *testing.T) {
	store, fixer := figureFixture("CIDFont+F3-GBpc-EUC-H")
	fixer.Locator = fakeFonts{"Fake", installedOriginal()}
	fixer.Fix("1 0 R", figureUse())
	if got := toUnicodeOf(store)[1]; got != replacement {
		t.Errorf("CID 1 reads %s, want %s", got, replacement)
	}
}

// The PDF's own ToUnicode value settles outline matches with different characters only when it is exactly
// one of them (decision 118, amendment 2): confirmed, disagreeing, or absent.
func TestToUnicodeArbitratesAmbiguousMatches(t *testing.T) {
	original := glyfProgram(1000, []testGlyph{{}, {[][][2]int16{square(400)}, 600}, {[][][2]int16{square(400)}, 600}},
		map[string][]byte{"cmap": cmapOf(map[int]uint16{0x1D45C: 1, 0x1D70A: 2})})
	font, err := parseOutlines(original)
	if err != nil {
		t.Fatal(err)
	}
	program, _ := ParseTrueType(original)
	found := &originalFont{outlines: font, unicode: program.UnicodeOf(), spellName: func(string) string { return "" }}
	shape, _ := font.shape(1)
	if text, method, why := found.identifyWith(shape, "D835DC5C"); text != "\U0001D45C" || method != "tounicode-confirmed" || why != "" {
		t.Errorf("confirmed: %q %s (%s)", text, method, why)
	}
	if text, _, why := found.identifyWith(shape, "0041"); text != "" || !strings.Contains(why, "ToUnicode U+0041 is none of them") {
		t.Errorf("disagreeing: %q (%s)", text, why)
	}
	for _, none := range []string{"", "E000"} {
		if text, _, why := found.identifyWith(shape, none); text != "" || !strings.Contains(why, "different characters") {
			t.Errorf("no ToUnicode %q: %q (%s)", none, text, why)
		}
	}
}

// A ToUnicode destination may split its UTF-16 units with a space (<d835 dc5c>, Word's maps).
func TestToUnicodeDestinationWithSpaces(t *testing.T) {
	mapped, _ := ReadCMap("1 begincodespacerange <00> <FF> endcodespacerange\n1 beginbfchar\n<22><d835 dc5c>\nendbfchar\n" +
		"1 beginbfrange\n<30> <31> <d835 dc00>\nendbfrange\n")
	if mapped[0x22] != "D835DC5C" || mapped[0x30] != "D835DC00" || mapped[0x31] != "D835DC01" {
		t.Fatalf("mapped %v", mapped)
	}
}

func TestIdentityHintsConflict(t *testing.T) {
	i := &identity{}
	i.hint(2, "0041")
	i.hint(3, "0041")
	i.hint(3, "0042")
	i.hint(4, "E000")
	if i.hints[2] != "0041" || i.hints[3] != "" || len(i.hints) != 2 {
		t.Fatalf("hints %v", i.hints)
	}
}
