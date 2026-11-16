package a11ylint

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/sarielhp/relatex/internal/langtag"
)

var (
	documentClass    = regexp.MustCompile(`\\documentclass\s*(?:\[([^\]]*)\])?\s*\{([^}]*)\}`)
	babel            = regexp.MustCompile(`\\usepackage\s*\[([^\]]*)\]\s*\{babel\}`)
	babelPackage     = regexp.MustCompile(`\\usepackage\s*(?:\[([^\]]*)\])?\s*\{babel\}`)
	polyglossiaUse   = regexp.MustCompile(`\\usepackage\s*(?:\[[^\]]*\])?\s*\{polyglossia\}`)
	polyglossia      = regexp.MustCompile(`\\set(?:default|main)language\s*(?:\[[^\]]*\])?\s*\{[^}]+\}`)
	babelProvide     = regexp.MustCompile(`\\babelprovide\s*\[[^\]]*main[^\]]*\]`)
	metadataLang     = regexp.MustCompile(`\\DocumentMetadata\s*\{[^}]*\blang\s*=`)
	documentMetadata = regexp.MustCompile(`\\DocumentMetadata\s*\{`)
	beginDocument    = regexp.MustCompile(`\\begin\s*\{document\}`)
)

// languageItems: the source declares its main language (babel, polyglossia, \DocumentMetadata's
// lang, or a class option naming a language). Without one, the item's note is the exact line to
// add for the language the text reads as, and where it goes.
func languageItems(files []*File) []Item {
	if len(files) == 0 {
		return nil
	}
	// The preamble may sit in files the main one inputs (a book's shared prefix): the declarations are read
	// from every file, and the suggestion goes in the file that holds \documentclass (books/0003).
	main := firstMatching(files, documentClass)
	options := []string{}
	for _, file := range files {
		if metadataLang.MatchString(file.Text) || polyglossia.MatchString(file.Text) || babelProvide.MatchString(file.Text) {
			return nil
		}
		if match := babel.FindStringSubmatch(file.Text); match != nil {
			options = append(options, strings.Split(match[1], ",")...)
		}
	}
	line := 1
	if match := documentClass.FindStringSubmatchIndex(main.Text); match != nil {
		line = main.Line(match[0])
		if match[2] >= 0 {
			options = append(options, strings.Split(main.Text[match[2]:match[3]], ",")...)
		}
	}
	for _, option := range options {
		if _, known := langtag.Tag(strings.TrimPrefix(strings.TrimSpace(option), "main=")); known {
			return nil
		}
	}
	item := Item{Kind: "language", File: main.Path, Line: line, Message: "the document declares no language; the export says English",
		Fix: `english as a \documentclass option, or \usepackage[english]{babel} (polyglossia: \setdefaultlanguage{english}; \DocumentMetadata{lang=en})`}
	return []Item{languageNote(item, main, detectLanguage(files))}
}

// firstMatching is the first file (in reading order) the pattern matches, else the main file.
func firstMatching(files []*File, pattern *regexp.Regexp) *File {
	for _, file := range files {
		if pattern.MatchString(file.Text) {
			return file
		}
	}
	return files[0]
}

// languageNote places the declaration for the detected language: in \DocumentMetadata when the
// document has one, as babel's option when babel is loaded without a language, polyglossia's
// \setdefaultlanguage after polyglossia, else babel after \documentclass.
func languageNote(item Item, main *File, language detected) Item {
	item.Place = PlaceAfter
	if match := documentMetadata.FindStringIndex(main.Text); match != nil {
		keys, _, _ := argument(main.Text, match[1]-1, '{', '}')
		item.Suggestion = `\DocumentMetadata{lang=` + language.tag + `}`
		if strings.TrimSpace(keys) != "" {
			item.Suggestion = `\DocumentMetadata{lang=` + language.tag + `, ` + strings.Join(strings.Fields(keys), " ") + `}`
		}
		item.NoteLine, item.Place = main.Line(match[0]), PlaceInstead
		item.Note = "add lang=" + language.tag + " to \\DocumentMetadata on this line: " + item.Suggestion + "; " + language.evidence()
		return item
	}
	if match := babelPackage.FindStringSubmatchIndex(main.Text); match != nil {
		given := ""
		if match[2] >= 0 {
			given = strings.TrimSpace(main.Text[match[2]:match[3]])
		}
		item.Suggestion = `\usepackage[` + strings.TrimPrefix(given+","+language.babel, ",") + `]{babel}`
		item.NoteLine, item.Place = main.Line(match[0]), PlaceInstead
		item.Note = "babel is loaded without a language: make this line " + item.Suggestion + "; " + language.evidence()
		return item
	}
	if match := polyglossiaUse.FindStringIndex(main.Text); match != nil {
		item.Suggestion = `\setdefaultlanguage{` + language.polyglossia + `}`
		item.NoteLine = main.Line(match[1] - 1)
		item.Note = "add " + item.Suggestion + " after this line (polyglossia names no language); " + language.evidence()
		return item
	}
	return classOptionNote(item, main, language)
}

// classOptionNote names the language as a class option on the \documentclass line: a global option
// defines no command and changes no typesetting, where babel or \DocumentMetadata can stop a document
// that compiled (the author's book: babel defines its \frq, and \DocumentMetadata loads latex-lab's
// enumitem support, clashing with its enumitem [inline]). babel, if loaded, takes the option too.
func classOptionNote(item Item, main *File, language detected) Item {
	match := documentClass.FindStringIndex(main.Text)
	if match == nil {
		item.Note = "add " + language.babel + " to the \\documentclass options; " + language.evidence()
		return item
	}
	start := strings.LastIndexByte(main.Text[:match[0]], '\n') + 1
	end := len(main.Text)
	if newline := strings.IndexByte(main.Text[match[0]:], '\n'); newline >= 0 {
		end = match[0] + newline
	}
	line := strings.TrimSuffix(main.Text[start:end], "\r")
	at := match[0] - start + len(`\documentclass`)
	rest := strings.TrimLeft(line[at:], " \t")
	if strings.HasPrefix(rest, "[") {
		rest = "[" + language.babel + "," + strings.TrimLeft(rest[1:], " \t")
		rest = strings.Replace(rest, ",]", "]", 1)
	} else {
		rest = "[" + language.babel + "]" + rest
	}
	item.Suggestion = line[:at] + rest
	item.NoteLine, item.Place = main.Line(match[0]), PlaceInstead
	item.Note = "add " + language.babel + " to the class options on this line: " + item.Suggestion +
		" (a class option defines no command; babel, if loaded, takes it); " + language.evidence()
	return item
}

// spokenLanguage is a language the lint recognises in the text: its tag, babel's and
// polyglossia's names, and frequent words of it.
type spokenLanguage struct {
	tag, babel, polyglossia string
	words                   string
}

// spoken are the languages told apart by their frequent words (the Latin-script ones) and by
// their script (Russian, Greek). English is first: it wins ties, as it is relatex's default.
var spoken = []spokenLanguage{
	{"en", "english", "english", "the and of is that we this with are which be it for from by not can have these our"},
	{"de", "ngerman", "german", "der die und das ist nicht mit ein eine wir für auf sich auch von zu dem den wird werden"},
	{"fr", "french", "french", "le les et est une du que dans pour nous sur pas par avec ce sont qui cette aux être"},
	{"es", "spanish", "spanish", "el los las y del una por con para se como más su al lo este son pero entre puede"},
	{"it", "italian", "italian", "il di che è della sono non con gli per dei nel questo anche delle alla essere tra come"},
	{"pt", "portuguese", "portuguese", "os do da em um uma para com não dos das é mais ao pela pelo são também como que"},
	{"nl", "dutch", "dutch", "het een en van dat op te voor niet met zijn wordt ook door aan bij als worden deze er"},
	{"ru", "russian", "russian", ""},
	{"el", "greek", "greek", ""},
}

// minimumHits is how many frequent words a language needs before the lint names it.
const minimumHits = 10

// detected is the language the text reads as, with the count it was told by.
type detected struct {
	spokenLanguage
	hits, words int
	script      string // "" or the script that decided it
}

func (d detected) evidence() string {
	name := strings.ToUpper(d.babel[:1]) + d.babel[1:]
	if d.babel == "ngerman" {
		name = "German"
	}
	switch {
	case d.script != "":
		return fmt.Sprintf("the text reads as %s (%d of %d words in %s script)", name, d.hits, d.words, d.script)
	case d.hits > 0:
		return fmt.Sprintf("the text reads as %s (%d of its %d words are frequent %s words)", name, d.hits, d.words, name)
	}
	return "too little text to tell its language; English is relatex's default, change it if the paper is in another language"
}

var (
	mathSpan    = regexp.MustCompile(`(?s)\$\$.*?\$\$|\$[^$]*\$|\\\[.*?\\\]|\\\(.*?\\\)`)
	keyCommand  = regexp.MustCompile(`\\(?:[a-zA-Z]*cite[a-zA-Z]*|[a-zA-Z]*ref|label|includegraphics|input|include|url|href|begin|end|usepackage|bibliography[a-zA-Z]*)\*?(?:\s*\[[^\]]*\])*\s*\{[^}]*\}`)
	controlWord = regexp.MustCompile(`\\[a-zA-Z@]+`)
	letters     = regexp.MustCompile(`\p{L}+`)
	wordIndex   = func() map[string][]int {
		index := map[string][]int{}
		for number, language := range spoken {
			for _, word := range strings.Fields(language.words) {
				index[word] = append(index[word], number)
			}
		}
		return index
	}()
)

// detectLanguage reads the document's text (after \begin{document}, without math and commands'
// keys) and names its language by frequent words or script; English when too little is there.
func detectLanguage(files []*File) detected {
	hits := make([]int, len(spoken))
	words, cyrillic, greek := 0, 0, 0
	for index, file := range files {
		text := file.Text
		if match := beginDocument.FindStringIndex(text); index == 0 && match != nil {
			text = text[match[1]:]
		}
		text = controlWord.ReplaceAllString(keyCommand.ReplaceAllString(mathSpan.ReplaceAllString(text, " "), " "), " ")
		for _, word := range letters.FindAllString(text, -1) {
			words++
			first := []rune(word)[0]
			cyrillic += boolInt(unicode.Is(unicode.Cyrillic, first))
			greek += boolInt(unicode.Is(unicode.Greek, first))
			for _, number := range wordIndex[strings.ToLower(word)] {
				hits[number]++
			}
		}
	}
	switch {
	case words > 0 && cyrillic*2 > words:
		return detected{spokenLanguage: spoken[7], hits: cyrillic, words: words, script: "Cyrillic"}
	case words > 0 && greek*2 > words:
		return detected{spokenLanguage: spoken[8], hits: greek, words: words, script: "Greek"}
	}
	best := 0
	for number := range hits {
		if hits[number] > hits[best] {
			best = number
		}
	}
	if hits[best] < minimumHits {
		return detected{spokenLanguage: spoken[0], words: words}
	}
	return detected{spokenLanguage: spoken[best], hits: hits[best], words: words}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
