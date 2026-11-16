package assets

import (
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/diagnostics"
	"github.com/sarielhp/relatex/internal/mediamodel"
	"github.com/sarielhp/relatex/internal/nativetrace"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/sourcefonts"
)

const repositoryRoot = "../../.."

// pictureSVG is a display SVG as MuPDF draws a crop: glyph outlines used at their origins.
const pictureSVG = `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="60pt" height="20pt" viewBox="0 0 60 20">` +
	`<defs><path id="font_1_1" d="M0 0H.5V.7H0Z"/><clipPath id="c"><use data-text="z" xlink:href="#font_1_1" transform="matrix(5,0,0,-5,1,1)"/></clipPath></defs>` +
	"<path d=\"M0 15H60\" stroke=\"#000\"/>\n" +
	`<use data-text="a" xlink:href="#font_1_1" fill="#ff0000" transform="matrix(7,0,0,-7,2,10)"/>` + "\n" +
	`<use data-text="b" xlink:href="#font_1_1" fill="#ff0000" transform="matrix(7,0,0,-7,5.5,10)"/>` + "\n" +
	`<use data-text="c" xlink:href="#font_1_1" fill="#ff0000" transform="matrix(7,0,0,-7,12,10)"/>` + "\n" +
	`<use data-text="x" xlink:href="#font_1_1" transform="matrix(5,0,0,-5,30,10)"/>` +
	`<use data-text="&lt;" xlink:href="#font_1_1" transform="matrix(0,-6,-6,0,50,18)"/></svg>`

func pictureGlyphs() []tracedGlyph {
	glyphs := []tracedGlyph{{font: "ABCDEF+CMR7", text: "a", adv: 0.5}, {font: "ABCDEF+CMR7", text: "b", adv: 0.5},
		{font: "ABCDEF+CMR7", text: "c", adv: 0.5}, {font: "GHIJKL+CMMI5", text: "x", adv: 0.6},
		{font: "MNOPQR+URWPalladioL-Bold", text: "<", adv: 0.5}}
	for index := range glyphs {
		glyphs[index].first = glyphs[index].text
	}
	return glyphs
}

func pictureDocumentFonts() rec.Map {
	return rec.Map{"fonts": rec.Map{"items": []any{rec.Map{"postscript_name": "URWPalladioL-Bold", "generic": "serif",
		"families": []any{"TeX Gyre Pagella", "Palatino"}},
		rec.Map{"postscript_name": "Roboto-Light", "generic": "sans-serif", "families": []any{"Roboto"}},
		rec.Map{"postscript_name": "Roboto-LightItalic", "generic": "sans-serif", "families": []any{"Roboto"}},
		rec.Map{"postscript_name": "Roboto-SemiBold", "generic": "sans-serif", "families": []any{"Roboto"}},
		rec.Map{"postscript_name": "SFTI0900", "generic": "serif", "families": []any{"Latin Modern Roman"}}}}}
}

// An outline's ink centre is read from MuPDF's absolute path data; other commands are not guessed.
func TestInkCentre(t *testing.T) {
	cases := []struct {
		data string
		want float64
		ok   bool
	}{
		{"M.31 .47C.32 .48 .32 .559 .26 .559L.066 .039Z", 0.299, true},
		{"M0 0H.5V.7H0Z", 0.35, true},
		{"M1e-1-.2L.3.4Z", 0.1, true},
		{"m0 0l1 1z", 0, false},
		{"", 0, false},
	}
	for _, test := range cases {
		got, ok := inkCentre(test.data)
		if ok != test.ok || math.Abs(got-test.want) > 1e-9 {
			t.Errorf("%q: %v %v, want %v %v", test.data, got, ok, test.want, test.ok)
		}
	}
}

func TestTextPictureWritesLabelsAsText(t *testing.T) {
	sourcefonts.Root = repositoryRoot
	svg, err := textPicture([]byte(pictureSVG), pictureGlyphs(), newPictureFonts(pictureDocumentFonts()))
	if err != nil {
		t.Fatal(err)
	}
	text := string(svg)
	for _, want := range []string{
		// One run in its optical size, the word space read from the gap, the outline's fill kept.
		`<text fill='#ff0000' font-family='"LM Roman 7", "Latin Modern Roman", "CMU Serif", "Computer Modern Serif", serif' font-size='7' xml:space='preserve' y='10'><tspan x='2'>a</tspan><tspan x='5.5'>b</tspan><tspan x='9'> </tspan><tspan x='12'>c</tspan></text>`,
		// Math italic as Mathematical Italic in the math font.
		`font-family='"Latin Modern Math", serif' font-size='5' xml:space='preserve' y='10'><tspan x='30'>𝑥</tspan></text>`,
		// A turned glyph: its size, turned by the transform; the document's stack and the name's weight.
		`<text font-family='"TeX Gyre Pagella", "Palatino", serif' font-size='6' font-weight='bold' xml:space='preserve' transform='matrix(0 -1 1 0 50 18)' y='0'><tspan x='0'>&lt;</tspan></text>`,
		// A clip path's glyph stays an outline.
		`<clipPath id='c'><use data-text='z'`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %s in\n%s", want, text)
		}
	}
	if strings.Count(text, "<use") != 1 {
		t.Errorf("painted outlines left: %s", text)
	}
}

func TestTextPictureReportsWhatItCannotWrite(t *testing.T) {
	sourcefonts.Root = repositoryRoot
	fonts := newPictureFonts(pictureDocumentFonts())
	cases := []struct {
		name   string
		glyphs func([]tracedGlyph) []tracedGlyph
		want   string
	}{
		{"count", func(g []tracedGlyph) []tracedGlyph { return g[:4] }, "trace: 4 glyphs painted, 5 outlines"},
		{"text", func(g []tracedGlyph) []tracedGlyph { g[1].text, g[1].first = "q", "q"; return g }, `glyph 2: the SVG's "b"`},
		{"font", func(g []tracedGlyph) []tracedGlyph { g[3].font = "ABCDEF+Unknown-Font"; return g }, "font Unknown-Font: no family"},
	}
	for _, test := range cases {
		_, err := textPicture([]byte(pictureSVG), test.glyphs(pictureGlyphs()), fonts)
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("%s: %v, want %q", test.name, err, test.want)
		}
	}
	unmapped := strings.Replace(pictureSVG, `data-text="x"`, `data-text="&#xFFFD;"`, 1)
	glyphs := pictureGlyphs()
	glyphs[3].text, glyphs[3].first = "�", "�"
	if _, err := textPicture([]byte(unmapped), glyphs, fonts); err == nil || !strings.Contains(err.Error(), "no character") {
		t.Errorf("unmapped glyph: %v", err)
	}
}

// A glyph that maps to several characters is traced as the glyph and a continuation per further
// character (no glyph name, no advance, its origin), as MuPDF traces a ligature or TeX's \Big
// parenthesis (papers/0191 fig-7): one glyph, its outline carrying the first character. A glyph
// without a name elsewhere, or with an advance, is a glyph of its own.
func TestTracedGlyphsJoinContinuations(t *testing.T) {
	page, err := nativetrace.ParseTree([]byte(`<document><page><fill_text>` +
		`<span font="AAAAAA+CMR8"><g unicode="f" glyph="fi" x="1" y="2" adv=".55"/><g unicode="i" x="1" y="2" adv="0"/>` +
		`<g unicode="x" glyph="x" x="5" y="2" adv=".5"/><g unicode="y" x="9" y="2" adv="0"/></span>` +
		`<span font="BBBBBB+CMEX10"><g unicode="︁" x="9" y="2" adv="0"/><g unicode=")" glyph="parenrightBig" x="3" y="4" adv=".6"/>` +
		`<g unicode="︂" x="3" y="4" adv="0"/></span></fill_text></page></document>`))
	if err != nil {
		t.Fatal(err)
	}
	got := tracedGlyphs(page.Descendants("page")[0])
	want := []tracedGlyph{{font: "AAAAAA+CMR8", text: "fi", adv: 0.55, first: "f"}, {font: "AAAAAA+CMR8", text: "x", adv: 0.5, first: "x"},
		{font: "AAAAAA+CMR8", text: "y", adv: 0, first: "y"}, {font: "BBBBBB+CMEX10", text: "︁", adv: 0, first: "︁"},
		{font: "BBBBBB+CMEX10", text: ")︂", adv: 0.6, first: ")"}}
	if len(got) != len(want) {
		t.Fatalf("glyphs %+v", got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Errorf("glyph %d: %+v, want %+v", index, got[index], want[index])
		}
	}
}

// TeX's prime is a full-size symbol-font glyph set at script size; the math font's U+2032 is the
// script prime already scaled and raised (decision 157): a prime above and after its base (papers/0191's
// j′ and i′′) is written at its size over the font's script scale, at its own position, its ink centred
// on TeX's (the outline's points run 0 to 0.7 em: centre 0.35); a prime on the line keeps its size and place.
func TestTextPictureRaisesPrimesToTheirBase(t *testing.T) {
	sourcefonts.Root = repositoryRoot
	svg := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="40pt" height="20pt" viewBox="0 0 40 20">` +
		`<defs><path id="g" d="M0 0H.5V.7H0Z"/></defs>` +
		`<use data-text="j" xlink:href="#g" transform="matrix(8,0,0,-8,2,15)"/>` +
		`<use data-text="&#x2032;" xlink:href="#g" transform="matrix(6,0,0,-6,6,12)"/>` +
		`<use data-text="&#x2032;" xlink:href="#g" transform="matrix(6,0,0,-6,8.2,12)"/>` +
		`<use data-text="&#x2032;" xlink:href="#g" transform="matrix(6,0,0,-6,20,15)"/></svg>`
	traced := []tracedGlyph{{font: "AAAAAA+CMMI8", text: "j", first: "j", adv: 0.43}, {font: "BBBBBB+CMSY6", text: "′", first: "′", adv: 0.37},
		{font: "BBBBBB+CMSY6", text: "′", first: "′", adv: 0.37}, {font: "BBBBBB+CMSY6", text: "′", first: "′", adv: 0.37}}
	got, err := textPicture([]byte(svg), traced, newPictureFonts(rec.Map{}))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`font-size='8.5714' xml:space='preserve' y='14.9486'><tspan x='6'>′</tspan><tspan x='8.2'>′</tspan></text>`,
		`font-size='6' xml:space='preserve' y='15'><tspan x='20'>′</tspan></text>`,
	} {
		if !strings.Contains(string(got), want) {
			t.Errorf("missing %s in\n%s", want, got)
		}
	}
}

// A line font's glyph is a piece of the drawing: it stays an outline, the labels around it convert.
func TestTextPictureKeepsDrawingGlyphs(t *testing.T) {
	sourcefonts.Root = repositoryRoot
	glyphs := pictureGlyphs()
	glyphs[3].font = "ABCDEF+LINE10"
	svg, err := textPicture([]byte(pictureSVG), glyphs, newPictureFonts(pictureDocumentFonts()))
	if err != nil || !strings.Contains(string(svg), "<use data-text='x'") || !strings.Contains(string(svg), "<tspan x='2'>a</tspan>") {
		t.Errorf("%v %s", err, svg)
	}
	for index := range glyphs {
		glyphs[index].font = "LCIRCLE10"
	}
	if _, err := textPicture([]byte(pictureSVG), glyphs, newPictureFonts(pictureDocumentFonts())); err != errNoText {
		t.Errorf("a picture of drawing glyphs only: %v", err)
	}
}

func TestPictureFontFaces(t *testing.T) {
	sourcefonts.Root = repositoryRoot
	fonts := newPictureFonts(pictureDocumentFonts())
	cases := []struct{ font, family, weight, style string }{
		{"ABCDEF+SFRM0800", `"LM Roman 8", "Latin Modern Roman", "CMU Serif", "Computer Modern Serif", serif`, "", ""},
		{"SFBX1440", `"Latin Modern Roman", "CMU Serif", "Computer Modern Serif", serif`, "bold", ""},
		{"CMTI10", `"LM Roman 10", "Latin Modern Roman", "CMU Serif", "Computer Modern Serif", serif`, "", "italic"},
		{"LMSans9-Oblique", `"LM Sans 9", "Latin Modern Sans", "CMU Sans Serif", "Computer Modern Sans", serif`, "", "oblique"},
		{"URWPalladioL-Bold", `"TeX Gyre Pagella", "Palatino", serif`, "bold", ""},
		// Decision 157: the weight words of the style part; a TeX program the document names keeps its table style.
		{"Roboto-Light", `"Roboto", sans-serif`, "300", ""},
		{"ABCDEF+Roboto-LightItalic", `"Roboto", sans-serif`, "300", "italic"},
		{"Roboto-SemiBold", `"Roboto", sans-serif`, "600", ""},
		{"ABCDEF+SFTI0900", `"LM Roman 9", "Latin Modern Roman", serif`, "", "italic"},
	}
	for _, test := range cases {
		face, ok := fonts.face(test.font)
		if !ok || face.family != test.family || face.weight != test.weight || face.style != test.style {
			t.Errorf("%s: %+v %v", test.font, face, ok)
		}
	}
}

func TestMathLetters(t *testing.T) {
	if got := mathLetters("hxαΓ1µϕ", "italic"); got != "ℎ𝑥𝛼𝛤1𝜇𝜙" {
		t.Errorf("italic %q", got)
	}
	if got := mathLetters("XBa", "script"); got != "𝒳ℬa" {
		t.Errorf("script %q", got)
	}
}

// labelPage is a PDF picture whose label is Helvetica (MuPDF's built-in Nimbus Sans) above a rule.
const labelPage = "%%MediaBox 0 0 120 40\n%%Font F1 Helvetica\nBT /F1 12 Tf 10 18 Td (Edge set) Tj ET\n0 0 1 RG 2 w 5 8 m 110 8 l S\n"

// The whole step on a real crop: MuPDF's display SVG becomes text when the stack draws the same
// glyphs, and stays outlines with a diagnostic when it does not.
func TestTextPicturesOnACrop(t *testing.T) {
	for _, tool := range []string{"mutool", "rsvg-convert"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not installed", tool)
		}
	}
	sourcefonts.Root = repositoryRoot
	directory := t.TempDir()
	source := filepath.Join(directory, "page.txt")
	if err := os.WriteFile(source, []byte(labelPage), 0o644); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("mutool", "create", "-o", filepath.Join(directory, "crop.pdf"), source).CombinedOutput(); err != nil {
		t.Fatalf("mutool create: %v %s", err, output)
	}
	crop, err := os.ReadFile(filepath.Join(directory, "crop.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		families  []any
		converted bool
		code      string
	}{
		{[]any{"Nimbus Sans", "TeX Gyre Heros"}, true, "figure.picture_text"},
		{[]any{"Latin Modern Mono"}, false, "figure.picture_text_fallback"},
	}
	for _, test := range cases {
		display, code := textPicturesOnce(t, crop, test.families)
		if strings.Contains(display, "<tspan") != test.converted || code != test.code {
			t.Errorf("%v: converted %v, diagnostic %s", test.families, strings.Contains(display, "<tspan"), code)
		}
	}
}

func textPicturesOnce(t *testing.T, crop []byte, families []any) (string, string) {
	t.Helper()
	sha := hexDigest(crop)
	asset := NewDerivatives(crop, rec.Map{"bytes": int64(len(crop)), "sha256": sha, "mime_type": "application/pdf"}).
		Add(renderedPlacement(120, 40, 0))
	document := rec.Map{"assets": rec.Map{"items": rec.Map{sha: asset}}, "fonts": rec.Map{"items": []any{
		rec.Map{"postscript_name": "Helvetica", "generic": "sans-serif", "families": families}}}}
	profile, err := diagnostics.LoadProfile(repositoryRoot)
	if err != nil {
		t.Fatal(err)
	}
	diags := diagnostics.New(profile, nil, false)
	TextPictures(document, map[string][]byte{"rendered/" + sha + ".pdf": crop}, diags)
	for key, value := range rec.FetchMap(asset, "representations") {
		if strings.HasPrefix(key, "display:") {
			code := rec.FetchString(rec.AsMap(diags.Entries()[0]), "code")
			return string(mediamodel.DataURIBytes(rec.FetchString(rec.AsMap(value), "data_uri"))), code
		}
	}
	t.Fatal("no display representation")
	return "", ""
}
