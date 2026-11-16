// Package langtag is the document model's language tags (decision 080, 102): the BCP 47 tag of
// a TeX language name (babel's and polyglossia's, with their old aliases) and the shape of a
// tag document.json accepts. The core (language) reads the document's languages from the records;
// the outputs and the author tools map names to tags here.
package langtag

import (
	"regexp"
	"strings"
)

// Default is the tag of a document that declares no language.
const Default = "en"

// tags maps babel and polyglossia language names (and their old aliases) to BCP 47 tags.
var tags = map[string]string{
	"english": "en", "american": "en-US", "usenglish": "en-US", "british": "en-GB", "ukenglish": "en-GB",
	"canadian": "en-CA", "australian": "en-AU", "newzealand": "en-NZ",
	"german": "de", "ngerman": "de", "austrian": "de-AT", "naustrian": "de-AT",
	"swissgerman": "de-CH", "nswissgerman": "de-CH",
	"french": "fr", "francais": "fr", "frenchb": "fr", "acadian": "fr-CA", "canadien": "fr-CA",
	"spanish": "es", "mexican": "es-MX", "catalan": "ca", "galician": "gl", "basque": "eu",
	"italian": "it", "portuguese": "pt", "portuges": "pt", "brazil": "pt-BR", "brazilian": "pt-BR",
	"dutch": "nl", "afrikaans": "af", "danish": "da", "swedish": "sv", "norsk": "nb", "norwegian": "nb",
	"nynorsk": "nn", "icelandic": "is", "finnish": "fi", "estonian": "et", "latvian": "lv", "lithuanian": "lt",
	"polish": "pl", "czech": "cs", "slovak": "sk", "slovene": "sl", "slovenian": "sl", "croatian": "hr",
	"serbian": "sr", "bosnian": "bs", "macedonian": "mk", "bulgarian": "bg", "russian": "ru",
	"ukrainian": "uk", "ukraineb": "uk", "belarusian": "be", "romanian": "ro", "hungarian": "hu", "magyar": "hu",
	"albanian": "sq", "greek": "el", "polutonikogreek": "el", "ancientgreek": "grc",
	"latin": "la", "classiclatin": "la", "medievallatin": "la", "ecclesiasticlatin": "la",
	"turkish": "tr", "hebrew": "he", "arabic": "ar", "farsi": "fa", "persian": "fa", "urdu": "ur",
	"hindi": "hi", "bengali": "bn", "sanskrit": "sa", "tamil": "ta", "telugu": "te", "marathi": "mr",
	"thai": "th", "vietnamese": "vi", "chinese": "zh", "japanese": "ja", "korean": "ko",
	"indonesian": "id", "bahasa": "id", "bahasai": "id", "malay": "ms", "bahasam": "ms",
	"esperanto": "eo", "interlingua": "ia", "irish": "ga", "scottish": "gd", "welsh": "cy", "breton": "br",
	"armenian": "hy", "georgian": "ka", "amharic": "am", "kurmanji": "kmr", "occitan": "oc",
	"romansh": "rm", "sorbian": "hsb", "uppersorbian": "hsb", "lowersorbian": "dsb", "northernsami": "se",
	"samin": "se", "friulan": "fur", "piedmontese": "pms", "turkmen": "tk", "mongolian": "mn",
}

// bcp47 is the shape of a tag document.json accepts.
var bcp47 = regexp.MustCompile(`\A[a-z]{2,3}(?:-[A-Za-z0-9]{2,8})*\z`)

// tagName is a tag written as a name, with a region or script (babel's \babelprovide{de-AT}); a
// bare name such as nil or latex is no tag.
var tagName = regexp.MustCompile(`\A[a-z]{2,3}(?:-[A-Za-z0-9]{2,8})+\z`)

// Tag is the BCP 47 tag of a TeX language name; false for a name the table does not know.
func Tag(name string) (string, bool) {
	if tag, ok := Named(name); ok {
		return tag, true
	}
	if tagName.MatchString(name) {
		return name, true
	}
	return "", false
}

// Valid is the shape document.json accepts for a tag.
func Valid(tag string) bool { return bcp47.MatchString(tag) }

// Named is the tag the table gives a language name (in any case); a tag written as a name is not
// read here (Tag reads it).
func Named(name string) (string, bool) {
	tag, ok := tags[strings.ToLower(name)]
	return tag, ok
}
