// Package glyphnames spells Type 1 glyph names as Unicode for the tagged PDF's ToUnicode maps
// (decision 104): relatex's reviewed symbol names, then TeX's glyphtounicode tables, then the
// Adobe Glyph List and its uniXXXX/uXXXX names. It is an output library, shared by both routes
// of the tagged PDF (internal/pdfua, internal/pdftag).
package glyphnames

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/sarielhp/relatex/internal/command"
)

// Table spells glyph names.
type Table struct {
	Adobe map[string]string            // the Adobe Glyph List: the names a validator reads without a map
	TeX   map[string]string            // name -> characters, the reviewed names first
	Fonts map[string]map[string]string // the reviewed names that hold for one font (tfm:FONT/NAME)
}

// fontAliases are the TFM names whose reviewed names a PDF font of another name takes: the
// standard fonts and their URW equivalents are the TFMs PSNFSS maps to them; bbding10.pfb's
// FontName is bbding (books 0004: its enc-97, \CircleSolid, read U+FFFD before).
var fontAliases = map[string]string{"ZapfDingbats": "pzdr", "Dingbats": "pzdr", "D050000L": "pzdr",
	"Symbol": "psyr", "StandardSymL": "psyr", "StandardSymbolsPS": "psyr", "bbding": "bbding10"}

var glyphToUnicode = regexp.MustCompile(`\\pdfglyphtounicode\{([^}]+)\}\{([0-9A-Fa-f ]+)\}`)

// Load reads the tables through kpsewhich and the reviewed names from reviewed
// (tex/relatex-symbol-glyphs.txt). As the collector reads them (relatex-glyph-names.lua), a name
// keeps its first printable reading: the reviewed names, cmex's table before the kernel's (whose
// brace pieces are private use), then TeX's extension of the Adobe Glyph List.
func Load(reviewed string) Table {
	table := Table{Adobe: map[string]string{}, TeX: map[string]string{}, Fonts: map[string]map[string]string{}}
	readLines(kpseFind("glyphlist.txt"), func(line string) {
		if name, codes, found := strings.Cut(line, ";"); found && !strings.HasPrefix(line, "#") {
			table.Adobe[name] = SpellCodes(strings.Fields(codes))
		}
	})
	readLines(reviewed, table.readReviewed)
	for _, name := range []string{"glyphtounicode-cmex.tex", "glyphtounicode.tex"} {
		readLines(kpseFind(name), func(line string) {
			if match := glyphToUnicode.FindStringSubmatch(line); match != nil && table.TeX[match[1]] == "" {
				table.TeX[match[1]] = SpellCodes(withoutSelectors(strings.Fields(match[2])))
			}
		})
	}
	readLines(kpseFind("texglyphlist.txt"), func(line string) {
		name, codes, found := strings.Cut(line, ";")
		if found && !strings.HasPrefix(line, "#") && table.TeX[name] == "" && table.Adobe[name] == "" {
			table.TeX[name] = SpellCodes(strings.Fields(codes))
		}
	})
	return table
}

// readReviewed reads one line of the reviewed names: NAME;HEX, tfm:FONT/NAME;HEX or NAME;none
// (no character: left to the published tables, else unspelled).
func (t Table) readReviewed(line string) {
	name, code, found := strings.Cut(strings.TrimSpace(line), ";")
	if !found || strings.HasPrefix(name, "#") || code == "none" {
		return
	}
	qualified, ok := strings.CutPrefix(name, "tfm:")
	if !ok {
		t.TeX[name] = SpellCodes([]string{code})
		return
	}
	font, glyph, _ := strings.Cut(qualified, "/")
	font = strings.ToLower(font)
	if t.Fonts[font] == nil {
		t.Fonts[font] = map[string]string{}
	}
	t.Fonts[font][glyph] = SpellCodes([]string{code})
}

// withoutSelectors drops variation selectors (cmex's parenleftbig is "(" with U+FE01).
func withoutSelectors(codes []string) []string {
	kept := make([]string, 0, len(codes))
	for _, code := range codes {
		if value, err := strconv.ParseUint(code, 16, 32); err != nil || value < 0xFE00 || value > 0xFE0F {
			kept = append(kept, code)
		}
	}
	return kept
}

func kpseFind(name string) string {
	output, err := exec.Command(command.TeXLiveBin("kpsewhich"), name).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// readLines calls each with every line of the file at path; a file that cannot be opened has none.
func readLines(path string, each func(string)) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		each(scanner.Text())
	}
}

// SpellCodes is the characters of hexadecimal code points, "" when one is not a character or is
// in the private use area (which a validator reads as unmapped).
func SpellCodes(codes []string) string {
	var spelled strings.Builder
	for _, code := range codes {
		value, err := strconv.ParseUint(code, 16, 32)
		if err != nil || value < 0x20 || privateUse(rune(value)) || value > 0x10FFFF {
			return ""
		}
		spelled.WriteRune(rune(value))
	}
	return spelled.String()
}

func privateUse(r rune) bool {
	return (r >= 0xE000 && r <= 0xF8FF) || r >= 0xF0000
}

// Readable reports whether a validator maps the name without help: a name of the Adobe Glyph List
// outside the private use area. veraPDF does not read a uniXXXX name without a map (papers2 0439:
// MyriadPro's uni0394).
func (t Table) Readable(name string) bool {
	return t.Adobe[name] != ""
}

// Spell is the characters a glyph name stands for, "" when none is known: the reviewed and TeX's
// names, the Adobe Glyph List, uniXXXX and uXXXX names, then the list's rules for a suffix
// (aogonek.sc) and a ligature (f_i), as the collector reads names.
func (t Table) Spell(name string) string {
	if spelled := t.plain(name); spelled != "" {
		return spelled
	}
	return t.composed(name)
}

// SpellIn spells a glyph name of the PDF font named font (without a subset tag): the reviewed
// names that hold for that font, or for the TFM it stands for, first. The font's case does not
// matter: a PDF font names lasy10 "LASY10".
func (t Table) SpellIn(font, name string) string {
	if alias, ok := fontAliases[font]; ok {
		font = alias
	}
	if spelled := t.Fonts[strings.ToLower(font)][name]; spelled != "" {
		return spelled
	}
	return t.Spell(name)
}

func (t Table) plain(name string) string {
	if spelled := t.TeX[name]; spelled != "" {
		return spelled
	}
	if spelled := t.Adobe[name]; spelled != "" {
		return spelled
	}
	return CodeName(name)
}

// composed spells a name with a suffix that keeps the character (aogonek.sc, hyphen.alt) and a
// ligature of names joined by "_" (f_i); a superior or inferior figure is its super- or subscript,
// and another suffix that changes the character (.numerator) has none.
func (t Table) composed(name string) string {
	base, suffix, dotted := strings.Cut(name, ".")
	if dotted && (semanticSuffixes[suffix] || base == "") {
		return scriptFigure(base, suffix)
	}
	if !strings.Contains(base, "_") {
		if base == name {
			return ""
		}
		return t.plain(base)
	}
	var joined strings.Builder
	for _, part := range strings.Split(base, "_") {
		spelled := t.plain(part)
		if spelled == "" {
			return ""
		}
		joined.WriteString(spelled)
	}
	return joined.String()
}

// semanticSuffixes change the character: a superior figure is no plain one.
var semanticSuffixes = map[string]bool{"superior": true, "sups": true, "inferior": true, "sinf": true,
	"numerator": true, "numr": true, "denominator": true, "dnom": true}

var figureNames = map[string]int{"zero": 0, "one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6,
	"seven": 7, "eight": 8, "nine": 9}

// superiorFigures are the superscript digits 0-9.
var superiorFigures = []rune{0x2070, 0xB9, 0xB2, 0xB3, 0x2074, 0x2075, 0x2076, 0x2077, 0x2078, 0x2079}

// scriptFigure is a figure's superior or inferior form as its super- or subscript character, ""
// for anything else.
func scriptFigure(base, suffix string) string {
	digit, ok := figureNames[base]
	switch {
	case !ok:
		return ""
	case suffix == "superior" || suffix == "sups":
		return string(superiorFigures[digit])
	case suffix == "inferior" || suffix == "sinf":
		return string(rune(0x2080 + digit))
	}
	return ""
}

// CodeName spells the Adobe Glyph List's code-point names: uniXXXX (one or more groups of four hex
// digits) and uXXXX to uXXXXXX (math italic letters of an Ipe figure: u1D45D).
func CodeName(name string) string {
	if hex, found := strings.CutPrefix(name, "uni"); found && len(hex) >= 4 && len(hex)%4 == 0 {
		codes := []string{}
		for start := 0; start < len(hex); start += 4 {
			codes = append(codes, hex[start:start+4])
		}
		return SpellCodes(codes)
	}
	if hex, found := strings.CutPrefix(name, "u"); found && len(hex) >= 4 && len(hex) <= 6 {
		return SpellCodes([]string{hex})
	}
	return ""
}

// UTF16Hex is text as UTF-16BE hexadecimal, as a ToUnicode CMap writes it.
func UTF16Hex(text string) string {
	var out strings.Builder
	for _, r := range text {
		if r >= 0x10000 {
			r -= 0x10000
			fmt.Fprintf(&out, "%04X%04X", 0xD800+(r>>10), 0xDC00+(r&0x3FF))
			continue
		}
		fmt.Fprintf(&out, "%04X", r)
	}
	return out.String()
}
