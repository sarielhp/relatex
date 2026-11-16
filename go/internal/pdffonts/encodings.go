package pdffonts

import "strings"

// The simple fonts' named encodings (ISO 32000-2, Annex D): code -> glyph name.

// asciiNames are the printable ASCII glyph names from 0x21; the quote and grave slots differ
// between the encodings and are set by each.
var asciiNames = strings.Fields(`exclam quotedbl numbersign dollar percent ampersand quotesingle
	parenleft parenright asterisk plus comma hyphen period slash zero one two three four five six seven
	eight nine colon semicolon less equal greater question at A B C D E F G H I J K L M N O P Q R S T U V
	W X Y Z bracketleft backslash bracketright asciicircum underscore grave a b c d e f g h i j k l m n o
	p q r s t u v w x y z braceleft bar braceright asciitilde`)

// latin1Names are the glyph names of ISO Latin-1's 0xA1-0xFF.
var latin1Names = strings.Fields(`exclamdown cent sterling currency yen brokenbar section dieresis
	copyright ordfeminine guillemotleft logicalnot hyphen registered macron degree plusminus twosuperior
	threesuperior acute mu paragraph periodcentered cedilla onesuperior ordmasculine guillemotright
	onequarter onehalf threequarters questiondown Agrave Aacute Acircumflex Atilde Adieresis Aring AE
	Ccedilla Egrave Eacute Ecircumflex Edieresis Igrave Iacute Icircumflex Idieresis Eth Ntilde Ograve
	Oacute Ocircumflex Otilde Odieresis multiply Oslash Ugrave Uacute Ucircumflex Udieresis Yacute Thorn
	germandbls agrave aacute acircumflex atilde adieresis aring ae ccedilla egrave eacute ecircumflex
	edieresis igrave iacute icircumflex idieresis eth ntilde ograve oacute ocircumflex otilde odieresis
	divide oslash ugrave uacute ucircumflex udieresis yacute thorn ydieresis`)

// standardHigh is StandardEncoding above 0x7E: code, name.
var standardHigh = `241 exclamdown 242 cent 243 sterling 244 fraction 245 yen 246 florin 247 section
	250 currency 251 quotesingle 252 quotedblleft 253 guillemotleft 254 guilsinglleft 255 guilsinglright
	256 fi 257 fl 261 endash 262 dagger 263 daggerdbl 264 periodcentered 266 paragraph 267 bullet
	270 quotesinglbase 271 quotedblbase 272 quotedblright 273 guillemotright 274 ellipsis 275 perthousand
	277 questiondown 301 grave 302 acute 303 circumflex 304 tilde 305 macron 306 breve 307 dotaccent
	310 dieresis 312 ring 313 cedilla 315 hungarumlaut 316 ogonek 317 caron 320 emdash 341 AE
	343 ordfeminine 350 Lslash 351 Oslash 352 OE 353 ordmasculine 361 ae 365 dotlessi 370 lslash
	371 oslash 372 oe 373 germandbls`

// winAnsiHigh is WinAnsiEncoding's 0x80-0x9F: code, name.
var winAnsiHigh = `200 Euro 202 quotesinglbase 203 florin 204 quotedblbase 205 ellipsis 206 dagger
	207 daggerdbl 210 circumflex 211 perthousand 212 Scaron 213 guilsinglleft 214 OE 216 Zcaron
	221 quoteleft 222 quoteright 223 quotedblleft 224 quotedblright 225 bullet 226 endash 227 emdash
	230 tilde 231 trademark 232 scaron 233 guilsinglright 234 oe 236 zcaron 237 Ydieresis`

// macRomanHigh is MacRomanEncoding above 0x7E, from 0x80, in order ("-" for an unused code).
var macRomanHigh = strings.Fields(`Adieresis Aring Ccedilla Eacute Ntilde Odieresis Udieresis aacute
	agrave acircumflex adieresis atilde aring ccedilla eacute egrave ecircumflex edieresis iacute igrave
	icircumflex idieresis ntilde oacute ograve ocircumflex odieresis otilde uacute ugrave ucircumflex
	udieresis dagger degree cent sterling section bullet paragraph germandbls registered copyright
	trademark acute dieresis - AE Oslash - plusminus - - yen mu - - - - - ordfeminine ordmasculine -
	ae oslash questiondown exclamdown logicalnot - florin - - guillemotleft guillemotright ellipsis space
	Agrave Atilde Otilde OE oe endash emdash quotedblleft quotedblright quoteleft quoteright divide -
	ydieresis Ydieresis fraction currency guilsinglleft guilsinglright fi fl daggerdbl periodcentered
	quotesinglbase quotedblbase perthousand Acircumflex Ecircumflex Aacute Edieresis Egrave Iacute
	Icircumflex Idieresis Igrave Oacute Ocircumflex - Ograve Uacute Ucircumflex Ugrave dotlessi
	circumflex tilde macron breve dotaccent ring cedilla hungarumlaut ogonek caron`)

var namedEncodings = map[string]map[int]string{
	"/StandardEncoding": standardEncoding(),
	"/WinAnsiEncoding":  winAnsiEncoding(),
	"/MacRomanEncoding": macRomanEncoding(),
}

// NamedEncoding is one of the named simple-font encodings by its PDF name (/WinAnsiEncoding), nil
// for another name.
func NamedEncoding(name string) map[int]string {
	return namedEncodings[name]
}

// withASCII is a new encoding with the printable ASCII names, quote and grave as given.
func withASCII(quote, grave string) map[int]string {
	codes := map[int]string{32: "space"}
	for index, name := range asciiNames {
		codes[0x21+index] = name
	}
	codes[0x27], codes[0x60] = quote, grave
	return codes
}

// addOctal adds "code name" pairs with octal codes.
func addOctal(codes map[int]string, pairs string) {
	fields := strings.Fields(pairs)
	for index := 0; index+1 < len(fields); index += 2 {
		code := 0
		for _, digit := range fields[index] {
			code = code*8 + int(digit-'0')
		}
		codes[code] = fields[index+1]
	}
}

func standardEncoding() map[int]string {
	codes := withASCII("quoteright", "quoteleft")
	addOctal(codes, standardHigh)
	return codes
}

func winAnsiEncoding() map[int]string {
	codes := withASCII("quotesingle", "grave")
	addOctal(codes, winAnsiHigh)
	codes[0xA0] = "space"
	for index, name := range latin1Names {
		codes[0xA1+index] = name
	}
	return codes
}

func macRomanEncoding() map[int]string {
	codes := withASCII("quotesingle", "grave")
	for index, name := range macRomanHigh {
		if name != "-" {
			codes[0x80+index] = name
		}
	}
	return codes
}
