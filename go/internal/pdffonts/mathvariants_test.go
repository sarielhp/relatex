package pdffonts

import (
	"encoding/binary"
	"os"
	"testing"
)

func be16(values ...int) []byte {
	var out []byte
	for _, v := range values {
		out = binary.BigEndian.AppendUint16(out, uint16(v))
	}
	return out
}

func coverage1(glyphs ...int) []byte { return append(be16(1, len(glyphs)), be16(glyphs...)...) }

// construction is a MathGlyphConstruction: variants and assembly parts (glyph ids).
func construction(variants, parts []int) []byte {
	assembly := 0
	head := 4 + 4*len(variants)
	if len(parts) > 0 {
		assembly = head
	}
	out := be16(assembly, len(variants))
	for _, v := range variants {
		out = append(out, be16(v, 1000)...)
	}
	if len(parts) > 0 {
		out = append(out, 0, 0, 0, 0) // italics correction
		out = append(out, be16(len(parts))...)
		for _, p := range parts {
			out = append(out, be16(p, 0, 0, 500, 0)...)
		}
	}
	return out
}

// buildMath is a MATH table with the vertical constructions of the base glyphs, in order.
func buildMath(bases []int, constructions [][]byte) []byte {
	n := len(bases)
	offsets := be16()
	body := coverage1(bases...)
	at := 10 + 2*n + len(body)
	for _, c := range constructions {
		offsets = append(offsets, be16(at)...)
		body = append(body, c...)
		at += len(c)
	}
	out := be16(1, 0, 0, 0, 10)
	out = append(out, be16(0, 10+2*n, 0, n, 0)...)
	return append(append(out, offsets...), body...)
}

// buildGSUB is a GSUB table of single substitutions (list 1 -> 31 and 10 -> 60, delta +50 on 1) and
// an alternate substitution (2 -> 41, 42).
func buildGSUB() []byte {
	single2 := append(be16(2, 6+2*2, 2, 31, 60), coverage1(1, 10)...)
	single1 := append(be16(1, 6, 50), coverage1(1)...)
	alternate := append(be16(1, 14, 1, 8, 2, 41, 42), coverage1(2)...)
	lookup := func(kind int, subs ...[]byte) []byte {
		head := be16(kind, 0, len(subs))
		offset := 6 + 2*len(subs)
		var body []byte
		for _, sub := range subs {
			head = append(head, be16(offset)...)
			body = append(body, sub...)
			offset += len(sub)
		}
		return append(head, body...)
	}
	first, second := lookup(1, single2, single1), lookup(3, alternate)
	list := be16(2, 6, 6+len(first))
	list = append(append(list, first...), second...)
	return append(be16(1, 0, 0, 0, 10), list...)
}

func TestDerivationsReverseMathVariantsAndSubstitutions(t *testing.T) {
	const left, right = 1, 2
	bases := []int{left, right}
	cons := [][]byte{construction([]int{10, 11, 30, 31}, []int{12, 13, 14}), construction([]int{20, 30}, []int{13, 22, 23})}
	derived := derivations(buildMath(bases, cons), buildGSUB())
	o := &originalFont{program: &TrueType{}, derived: derived, unicode: map[int]int{left: 0x28, right: 0x29}, spellName: func(s string) string { return s }}
	cases := []struct {
		gid  int
		text string
		why  string
	}{
		{10, "(", ""}, {11, "(", ""}, {20, ")", ""},
		{30, "", "several different characters"}, // a variant of both ( and )
		{31, "(", ""},                            // a variant of ( and, by GSUB, of ( too
		{12, "", "only an assembly part"},        // top of a tall (
		{13, "", "only an assembly part"},        // extender shared by ( and )
		{60, "(", ""},                            // GSUB single of the variant 10
		{41, ")", ""},                            // GSUB alternate of )
		{51, "(", ""},                            // GSUB single by delta
		{99, "", ""},
	}
	for _, c := range cases {
		text, why := o.derivedText(c.gid, 0)
		if text != c.text || (c.why == "") != (why == "") || c.why != "" && !contains(why, c.why) {
			t.Errorf("glyph %d: got %q, %q; want %q, %q", c.gid, text, why, c.text, c.why)
		}
	}
	if text, how := o.textOf(60); text != "(" || how != "variant" {
		t.Errorf("textOf(60) = %q, %q", text, how)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestInstalledLatinModernMathReadsVariantsAsTheirBase reads TeX Live's Latin Modern Math (skipped
// when it is not installed, nothing is kept): a size variant of "(" is not in the cmap, its MATH
// construction leads back to "(", and the extender of a stacked delimiter reads nothing.
func TestInstalledLatinModernMathReadsVariantsAsTheirBase(t *testing.T) {
	data, err := os.ReadFile("/usr/share/texmf/fonts/opentype/public/lm-math/latinmodern-math.otf")
	if err != nil {
		t.Skip("Latin Modern Math is not installed")
	}
	tables := tableDirectory(data)
	program, err := ParseTrueType(data)
	if err != nil {
		t.Fatal(err)
	}
	o := &originalFont{program: program, unicode: program.UnicodeOf(), derived: derivations(tables["MATH"], tables["GSUB"]),
		spellName: func(s string) string { return s }}
	paren := program.Cmaps[[2]int{3, 1}]['(']
	variants := 0
	for gid, routes := range o.derived {
		if len(routes) == 1 && routes[0].from == paren && !routes[0].part && o.exactText(gid) == "" {
			variants++
			if text, why := o.derivedText(gid, 0); text != "(" {
				t.Errorf("variant glyph %d of ( reads %q (%s)", gid, text, why)
			}
		}
	}
	if variants == 0 {
		t.Error("no unencoded variant of ( found")
	}
}
