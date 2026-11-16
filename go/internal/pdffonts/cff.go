package pdffonts

import (
	"errors"
	"strings"
)

// A small CFF reader (Adobe Technical Note 5176): the built-in encoding of a FontFile3 /Type1C
// program, code -> glyph name, which a symbolic font without an Encoding shows its glyphs by.
// Ghostscript's PDF output embeds TeX's fonts so (books 0004: a figure's CMSY10 shows its prime,
// a name the Adobe Glyph List lacks, through the program's own encoding).

var errNotCFF = errors.New("not a CFF font program")

// cffReader reads a CFF font's structures.
type cffReader struct {
	data    []byte
	strings []string // the String INDEX: SIDs from 391 on
}

// cffIndex reads an INDEX at offset: its items and the offset after it.
func (r *cffReader) index(at int) ([][]byte, int, error) {
	if at+2 > len(r.data) {
		return nil, 0, errNotCFF
	}
	count := int(r.data[at])<<8 | int(r.data[at+1])
	if count == 0 {
		return nil, at + 2, nil
	}
	if at+3 > len(r.data) {
		return nil, 0, errNotCFF
	}
	size := int(r.data[at+2])
	offsets := at + 3
	base := offsets + (count+1)*size - 1
	if size < 1 || size > 4 || base >= len(r.data) {
		return nil, 0, errNotCFF
	}
	items := make([][]byte, 0, count)
	start := r.offset(offsets, size)
	for i := 1; i <= count; i++ {
		end := r.offset(offsets+i*size, size)
		if start < 1 || end < start || base+end > len(r.data) {
			return nil, 0, errNotCFF
		}
		items = append(items, r.data[base+start:base+end])
		start = end
	}
	return items, base + start, nil
}

func (r *cffReader) offset(at, size int) int {
	value := 0
	for i := range size {
		value = value<<8 | int(r.data[at+i])
	}
	return value
}

// cffTopDict reads the operators of a Top DICT the encoding needs: the integer operands of each
// one-byte operator, and whether the font is a CIDFont (ROS, 12 30).
func cffTopDict(dict []byte) (map[int][]int, bool) {
	ops, operands := map[int][]int{}, []int{}
	for at := 0; at < len(dict); {
		b0 := int(dict[at])
		switch {
		case b0 == 12:
			if at+1 < len(dict) && dict[at+1] == 30 {
				return ops, true
			}
			operands, at = operands[:0], at+2
		case b0 <= 21:
			ops[b0], operands, at = append([]int(nil), operands...), operands[:0], at+1
		case b0 == 30:
			operands, at = append(operands, 0), skipReal(dict, at+1)
		default:
			value, next := cffInteger(dict, at)
			operands, at = append(operands, value), next
		}
	}
	return ops, false
}

// cffInteger reads an integer operand at at (b0 28, 29, 32..254).
func cffInteger(dict []byte, at int) (int, int) {
	b0 := int(dict[at])
	byteAt := func(i int) int {
		if at+i < len(dict) {
			return int(dict[at+i])
		}
		return 0
	}
	switch {
	case b0 == 28:
		return int(int16(byteAt(1)<<8 | byteAt(2))), at + 3
	case b0 == 29:
		return int(int32(byteAt(1)<<24 | byteAt(2)<<16 | byteAt(3)<<8 | byteAt(4))), at + 5
	case b0 >= 32 && b0 <= 246:
		return b0 - 139, at + 1
	case b0 >= 247 && b0 <= 250:
		return (b0-247)*256 + byteAt(1) + 108, at + 2
	case b0 >= 251 && b0 <= 254:
		return -(b0-251)*256 - byteAt(1) - 108, at + 2
	}
	return 0, at + 1
}

// skipReal is the offset after a real operand's nibbles (which end with 0xf).
func skipReal(dict []byte, at int) int {
	for ; at < len(dict); at++ {
		if dict[at]&0x0f == 0x0f || dict[at]>>4 == 0x0f {
			return at + 1
		}
	}
	return at
}

// name is a SID's string.
func (r *cffReader) name(sid int) string {
	if sid < len(cffStandardStrings) {
		return cffStandardStrings[sid]
	}
	if sid-len(cffStandardStrings) < len(r.strings) {
		return r.strings[sid-len(cffStandardStrings)]
	}
	return ""
}

// ParseCFFEncoding reads a bare CFF program's (FontFile3 /Type1C) built-in encoding: code -> glyph
// name. A CIDFont has none; a charset of the predefined Expert sets names nothing.
func ParseCFFEncoding(data []byte) (map[int]string, error) {
	if len(data) < 4 || data[0] != 1 {
		return nil, errNotCFF
	}
	r := &cffReader{data: data}
	_, next, err := r.index(int(data[2]))
	if err != nil {
		return nil, err
	}
	tops, next, err := r.index(next)
	if err != nil || len(tops) == 0 {
		return nil, errNotCFF
	}
	strs, _, err := r.index(next)
	if err != nil {
		return nil, err
	}
	for _, item := range strs {
		r.strings = append(r.strings, string(item))
	}
	ops, cid := cffTopDict(tops[0])
	if cid || len(ops[17]) != 1 {
		return nil, errNotCFF
	}
	glyphs, _, err := r.index(ops[17][0])
	if err != nil {
		return nil, err
	}
	names := r.charset(first(ops[15]), len(glyphs))
	return r.encoding(first(ops[16]), names), nil
}

func first(operands []int) int {
	if len(operands) == 0 {
		return 0
	}
	return operands[0]
}

// charset is the glyph names by glyph index (.notdef first).
func (r *cffReader) charset(at, count int) []string {
	names := make([]string, count)
	if count > 0 {
		names[0] = ".notdef"
	}
	if at <= 2 {
		if at == 0 { // ISOAdobe: glyph i is SID i
			for gid := 1; gid < count && gid < 229; gid++ {
				names[gid] = cffStandardStrings[gid]
			}
		}
		return names
	}
	r.readCharset(at, names)
	return names
}

func (r *cffReader) readCharset(at int, names []string) {
	if at >= len(r.data) {
		return
	}
	format, pos := r.data[at], at+1
	for gid := 1; gid < len(names) && pos+1 < len(r.data); {
		sid := int(r.data[pos])<<8 | int(r.data[pos+1])
		pos += 2
		if format == 0 {
			names[gid] = r.name(sid)
			gid++
			continue
		}
		left := 0
		if format == 1 && pos < len(r.data) {
			left, pos = int(r.data[pos]), pos+1
		} else if format == 2 && pos+1 < len(r.data) {
			left, pos = int(r.data[pos])<<8|int(r.data[pos+1]), pos+2
		} else {
			return
		}
		for i := 0; i <= left && gid < len(names); i, gid = i+1, gid+1 {
			names[gid] = r.name(sid + i)
		}
	}
}

// encoding is code -> glyph name: the Standard encoding's names the charset has (offset 0), none for
// the Expert encoding (1), else the custom encoding's codes with its supplements.
func (r *cffReader) encoding(at int, names []string) map[int]string {
	codes := map[int]string{}
	if at <= 1 {
		if at == 0 {
			present := map[string]bool{}
			for _, name := range names {
				present[name] = true
			}
			for code, name := range standardEncoding() {
				if present[name] {
					codes[code] = name
				}
			}
		}
		return codes
	}
	if at+1 >= len(r.data) {
		return codes
	}
	format, count, pos := r.data[at], int(r.data[at+1]), at+2
	pos = r.customCodes(format&0x7f, count, pos, names, codes)
	if format&0x80 != 0 && pos < len(r.data) {
		r.supplements(pos, codes)
	}
	return codes
}

// customCodes reads a custom encoding's codes (format 0: one per glyph; format 1: ranges), glyph 1
// on; it returns the offset after them.
func (r *cffReader) customCodes(format byte, count, pos int, names []string, codes map[int]string) int {
	gid := 1
	for i := 0; i < count; i++ {
		if format == 0 && pos < len(r.data) {
			if gid < len(names) {
				codes[int(r.data[pos])] = names[gid]
			}
			gid, pos = gid+1, pos+1
			continue
		}
		if format != 1 || pos+1 >= len(r.data) {
			return len(r.data)
		}
		code, left := int(r.data[pos]), int(r.data[pos+1])
		for j := 0; j <= left && gid < len(names); j, gid = j+1, gid+1 {
			codes[code+j] = names[gid]
		}
		pos += 2
	}
	return pos
}

// supplements reads an encoding's supplements: code -> SID.
func (r *cffReader) supplements(pos int, codes map[int]string) {
	count := int(r.data[pos])
	for i, at := 0, pos+1; i < count && at+2 < len(r.data); i, at = i+1, at+3 {
		codes[int(r.data[at])] = r.name(int(r.data[at+1])<<8 | int(r.data[at+2]))
	}
}

// cffStandardStrings are the CFF standard strings, SIDs 0 to 390 (TN 5176, Appendix A).
var cffStandardStrings = strings.Fields(
	".notdef space exclam quotedbl numbersign dollar percent ampersand quoteright parenleft " +
		"parenright asterisk plus comma hyphen period slash zero one two three four five six seven " +
		"eight nine colon semicolon less equal greater question at A B C D E F G H I J K L M N O P Q " +
		"R S T U V W X Y Z bracketleft backslash bracketright asciicircum underscore quoteleft a b c " +
		"d e f g h i j k l m n o p q r s t u v w x y z braceleft bar braceright asciitilde " +
		"exclamdown cent sterling fraction yen florin section currency quotesingle quotedblleft " +
		"guillemotleft guilsinglleft guilsinglright fi fl endash dagger daggerdbl periodcentered " +
		"paragraph bullet quotesinglbase quotedblbase quotedblright guillemotright ellipsis " +
		"perthousand questiondown grave acute circumflex tilde macron breve dotaccent dieresis ring " +
		"cedilla hungarumlaut ogonek caron emdash AE ordfeminine Lslash Oslash OE ordmasculine ae " +
		"dotlessi lslash oslash oe germandbls onesuperior logicalnot mu trademark Eth onehalf " +
		"plusminus Thorn onequarter divide brokenbar degree thorn threequarters twosuperior " +
		"registered minus eth multiply threesuperior copyright Aacute Acircumflex Adieresis Agrave " +
		"Aring Atilde Ccedilla Eacute Ecircumflex Edieresis Egrave Iacute Icircumflex Idieresis " +
		"Igrave Ntilde Oacute Ocircumflex Odieresis Ograve Otilde Scaron Uacute Ucircumflex " +
		"Udieresis Ugrave Yacute Ydieresis Zcaron aacute acircumflex adieresis agrave aring atilde " +
		"ccedilla eacute ecircumflex edieresis egrave iacute icircumflex idieresis igrave ntilde " +
		"oacute ocircumflex odieresis ograve otilde scaron uacute ucircumflex udieresis ugrave " +
		"yacute ydieresis zcaron exclamsmall Hungarumlautsmall dollaroldstyle dollarsuperior " +
		"ampersandsmall Acutesmall parenleftsuperior parenrightsuperior twodotenleader " +
		"onedotenleader zerooldstyle oneoldstyle twooldstyle threeoldstyle fouroldstyle fiveoldstyle " +
		"sixoldstyle sevenoldstyle eightoldstyle nineoldstyle commasuperior threequartersemdash " +
		"periodsuperior questionsmall asuperior bsuperior centsuperior dsuperior esuperior isuperior " +
		"lsuperior msuperior nsuperior osuperior rsuperior ssuperior tsuperior ff ffi ffl " +
		"parenleftinferior parenrightinferior Circumflexsmall hyphensuperior Gravesmall Asmall " +
		"Bsmall Csmall Dsmall Esmall Fsmall Gsmall Hsmall Ismall Jsmall Ksmall Lsmall Msmall Nsmall " +
		"Osmall Psmall Qsmall Rsmall Ssmall Tsmall Usmall Vsmall Wsmall Xsmall Ysmall Zsmall " +
		"colonmonetary onefitted rupiah Tildesmall exclamdownsmall centoldstyle Lslashsmall " +
		"Scaronsmall Zcaronsmall Dieresissmall Brevesmall Caronsmall Dotaccentsmall Macronsmall " +
		"figuredash hypheninferior Ogoneksmall Ringsmall Cedillasmall questiondownsmall oneeighth " +
		"threeeighths fiveeighths seveneighths onethird twothirds zerosuperior foursuperior " +
		"fivesuperior sixsuperior sevensuperior eightsuperior ninesuperior zeroinferior oneinferior " +
		"twoinferior threeinferior fourinferior fiveinferior sixinferior seveninferior eightinferior " +
		"nineinferior centinferior dollarinferior periodinferior commainferior Agravesmall " +
		"Aacutesmall Acircumflexsmall Atildesmall Adieresissmall Aringsmall AEsmall Ccedillasmall " +
		"Egravesmall Eacutesmall Ecircumflexsmall Edieresissmall Igravesmall Iacutesmall " +
		"Icircumflexsmall Idieresissmall Ethsmall Ntildesmall Ogravesmall Oacutesmall " +
		"Ocircumflexsmall Otildesmall Odieresissmall OEsmall Oslashsmall Ugravesmall Uacutesmall " +
		"Ucircumflexsmall Udieresissmall Yacutesmall Thornsmall Ydieresissmall 001.000 001.001 " +
		"001.002 001.003 Black Bold Book Light Medium Regular Roman Semibold",
)
