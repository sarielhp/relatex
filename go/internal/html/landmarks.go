package html

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/sarielhp/relatex/internal/htmlkit"
	"github.com/sarielhp/relatex/internal/rec"
)

// Landmarks and named groups (plan accessibility §7.2.3, decision 095): the page's banner holds
// the status notice and the contents link, the bibliography's section is a doc-bibliography
// landmark named by its heading, and each theorem-like structure or proof is a group named by
// its title and number.

// pageHeader is the page's banner: the status notice and the link to the contents.
func (p *Paper) pageHeader() string {
	return "<header>" + statusNotice + p.contentsLink() + "</header>"
}

// noteNumber is a note number a name can show: a few letters, digits or note marks (a number
// recorded as TeX source, such as \TextOrMath{…}, is not).
var noteNumber = regexp.MustCompile(`^[0-9A-Za-z∗*⋆†‡§¶]{1,4}$`)

// noteRole makes a note's aside a DPUB-ARIA footnote, not a complementary landmark: a page of
// notes would otherwise hold many landmarks of one name (Ace's landmark-unique). Its name is
// "Footnote N" when the number is plain, else "Footnote".
func noteRole(number string) string {
	name := "Footnote"
	if noteNumber.MatchString(number) {
		name += " " + number
	}
	return " role=\"doc-footnote\" aria-label=\"" + htmlkit.Escape(name) + "\""
}

// groupNumber is a structure's printed number as its first paragraph shows it after the title:
// digits or one capital letter, then dotted or dashed parts ("3", "2.4", "A.1").
var groupNumber = regexp.MustCompile(`^(?:[0-9]+|[A-Z])(?:[.\-][0-9A-Za-z]+)*$`)

// groupAttributes are a theorem-like structure's or a proof's role and name: role="group" and
// aria-label "<title> <number>" ("Theorem 3.2", "Proof"). A theorem without a title is named by
// its first paragraph when that opens with a word and a number ("Theorem 3.1 (…)", set by hand in
// bold, decision 167 amendment 4); with neither it has no name to give and stays a plain container.
func (p *Paper) groupAttributes(node rec.Map, kind string) string {
	title := "Proof"
	if kind == "theorem" {
		semantics, _ := node["semantics"].(rec.Map)
		title, _ = semantics["title"].(string)
		title = strings.TrimSpace(title)
	}
	if title == "" {
		title = typedTitle(p.firstParagraphText(node))
	}
	if title == "" {
		return ""
	}
	return " role=\"group\" aria-label=\"" + htmlkit.Escape(GroupName(title, p.firstParagraphText(node))) + "\""
}

// typedTitle is the name a hand-typed theorem opens with: a theorem-like kind word (typedKind)
// and a number with a digit ("Theorem 1.", "Lemma 3.2 (Zorn)"), else "".
func typedTitle(text string) string {
	words := strings.FieldsFunc(text, unicode.IsSpace)
	if len(words) < 2 || !typedKind.MatchString(words[0]) {
		return ""
	}
	number := strings.TrimRight(words[1], ".:,")
	if !groupNumber.MatchString(number) || !strings.ContainsAny(number, "0123456789") {
		return ""
	}
	return words[0] + " " + number
}

var typedKind = regexp.MustCompile(`^(?:Theorem|Lemma|Proposition|Corollary|Claim|Conjecture|Definition|Remark|Example|Fact|` +
	`Observation|Assumption|Property|Axiom|Notation|Problem|Question|Hypothesis|Exercise|Principle|Satz|Lemme)$`)

// GroupName is the name of a structure titled title whose first paragraph reads text: the title,
// and the number when the paragraph starts with the title and a number.
func GroupName(title, text string) string {
	words := strings.FieldsFunc(text, unicode.IsSpace)
	titleWords := strings.FieldsFunc(title, unicode.IsSpace)
	if len(words) <= len(titleWords) || strings.Join(words[:len(titleWords)], " ") != strings.Join(titleWords, " ") {
		return title
	}
	number := strings.TrimRight(words[len(titleWords)], ".:,")
	if !groupNumber.MatchString(number) {
		return title
	}
	return title + " " + number
}

// firstParagraphText is the text of a structure's first child when it is a paragraph, else "".
func (p *Paper) firstParagraphText(node rec.Map) string {
	children, _ := node["children"].([]any)
	if len(children) == 0 {
		return ""
	}
	first := p.node(children[0])
	if first["type"] != "paragraph" {
		return ""
	}
	return paragraphText(first)
}

// sectionAttributes are a section's landmark attributes: the bibliography's section is
// role="doc-bibliography", labelled by its heading.
func (p *Paper) sectionAttributes(identifier string, node rec.Map) string {
	if identifier != p.bibliographySection() {
		return ""
	}
	children, _ := node["children"].([]any)
	heading, _ := children[0].(string)
	return " role=\"doc-bibliography\" aria-labelledby=\"" + htmlkit.Escape(heading) + "\""
}

// bibliographySection is the id of the section that holds the bibliography, or "": the nearest
// section around every entry block of the citations graph, the same for all, opening with a
// heading followed by the entry blocks (in lists or not) and then only end matter (entriesLead).
func (p *Paper) bibliographySection() string {
	if p.bibliography != nil {
		return *p.bibliography
	}
	found := htmlkit.BibliographySection(p.document)
	p.bibliography = &found
	return found
}

// bibliographyRun is the bibliography's run of entries when no section holds it alone (decision
// 111), found once: the wrapper spans the run's children in their parent. The page is English,
// or says no language, because the wrapper's name is the English word (the long descriptions'
// rule).
type bibliographyRun struct {
	parent string
	first  int
	last   int
	ok     bool
}

func (p *Paper) bibliographyRun() (string, int, int, bool) {
	if p.bibRun == nil {
		run := bibliographyRun{}
		if p.english() {
			run.parent, run.first, run.last, run.ok = htmlkit.BibliographyRun(p.document)
		}
		p.bibRun = &run
	}
	return p.bibRun.parent, p.bibRun.first, p.bibRun.last, p.bibRun.ok
}

// bibliographyOpen is the wrapper around a bibliography run: a doc-bibliography landmark named
// References, its id the page's own (BibliographyID).
func (p *Paper) bibliographyOpen() string {
	if p.bibID == "" {
		p.bibID = htmlkit.BibliographyID(p.document)
	}
	return "<div id=\"" + htmlkit.Escape(p.bibID) + "\" role=\"doc-bibliography\" aria-label=\"References\">"
}
