package typstsource

import (
	_ "embed"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/bibclean"
	"github.com/sarielhp/relatex/internal/bibstyle"
	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/typstvendor"
)

// The styled bibliography (decision 172; plans/archive/typst-bibliography.md phase 3). A document whose
// bibliography BibTeX or biber made from .bib files gets references.bib (relatex bibclean, the entries
// the paper printed), the style found along BSTINPUTS (bibstyle.typ), the vendored Pergamon (support/vendor/)
// and bibliography.typ, which prints the entries where the paper printed them. It ships only when every
// entry Pergamon prints equals the model's (bibcheck.go); otherwise the export keeps literal entries and
// its report says why (a warning when a style was found).

//go:embed bibliography.typ
var bibliographyTemplate string

// Files of a styled bibliography: references.bib beside main.typ, the others in support/.
const (
	referencesFile   = "references.bib"
	styleFile        = "bibstyle.typ"
	bibliographyFile = "bibliography.typ"
	checkFile        = "bibcheck.typ"
)

// Statuses of an export's bibliography.
const (
	statusStyled  = "styled"
	statusLiteral = "literal"
)

// maxReported bounds the differing entries a report lists.
const maxReported = 20

// Bibliography is what a styled bibliography is made from; nil (a bundle export) writes literal
// entries and records nothing.
type Bibliography struct {
	// Processor is biber or bibtex; "" when the build ran neither (a hand-written thebibliography).
	Processor string
	// Names are the style names the build loaded, most specific first.
	Names []string
	// Style is the style file found; nil when none was.
	Style *bibstyle.Style
	// Path is the BSTINPUTS path searched.
	Path string
	// Databases are the .bib files, resolved; Missing are the names not found.
	Databases []string
	Missing   []string
	// Typst is the typst executable; "" when there is none.
	Typst string
	// Root is the resource root (the vendored packages).
	Root string
	// Sources is the sources' root: a file inside it is recorded by its path there, not where the run put it.
	Sources string
}

// styledBibliography is a bibliography written by Pergamon: the block that prints it, every entry
// block, and each model key's .bib key.
type styledBibliography struct {
	first    string
	blocks   map[string]bool
	keys     map[string]string
	numbered bool
	// elements are the keys whose figure prints the model's number; the others are link targets.
	elements map[string]bool
}

// bibEntry is one entry the paper printed.
type bibEntry struct {
	key      string
	bibKey   string
	block    string
	node     rec.Map
	cited    bool
	numbered bool
	element  bool
}

// styleBibliography decides between a styled and a literal bibliography and records why.
func (e *exporter) styleBibliography(b *Bibliography) {
	if b == nil {
		return
	}
	e.bibReport = rec.Map{"status": statusLiteral, "processor": b.Processor, "names": stringsToAny(b.Names),
		"path": b.Path, "style": b.styleRecord(), "databases": b.databaseRecords()}
	entries, reason := e.bibliographyEntries()
	if reason == "" {
		reason = b.unusable()
	}
	if reason == "" {
		var differing []any
		reason, differing = e.attemptStyled(b, entries)
		if len(differing) > 0 {
			e.bibReport["differing"] = differing
		}
	}
	e.bibReport["entries"] = int64(len(maps(dig(e.document, "citations", "entries"))))
	if reason != "" {
		e.bibReport["reason"] = reason
		removeStyledFiles(e.directory)
		return
	}
	e.bibReport["status"] = statusStyled
	e.styledBib = newStyled(entries)
}

// unusable says why the inputs cannot make a styled bibliography, "" when they can.
func (b *Bibliography) unusable() string {
	switch {
	case b.Processor == "":
		return "the bibliography is not made from .bib files (no BibTeX or biber run)"
	case b.Style == nil:
		return fmt.Sprintf("no Typst style %s along BSTINPUTS (%s) or in relatex's library", typNames(b.Names), b.Path)
	case len(b.Missing) > 0:
		return "bibliography databases not found: " + strings.Join(b.Missing, ", ")
	case len(b.Databases) == 0:
		return "the build named no bibliography database"
	case b.Typst == "":
		return "typst is not available to check the styled bibliography"
	}
	return ""
}

func typNames(names []string) string {
	if len(names) == 0 {
		return "(the build named no style)"
	}
	files := make([]string, len(names))
	for index, name := range names {
		files[index] = name + ".typ"
	}
	return strings.Join(files, ", ")
}

func newStyled(entries []bibEntry) *styledBibliography {
	styled := &styledBibliography{first: entries[0].block, blocks: map[string]bool{}, keys: map[string]string{},
		numbered: entries[0].numbered, elements: map[string]bool{}}
	for _, entry := range entries {
		styled.blocks[entry.block] = true
		styled.elements[entry.key] = entry.element
		styled.keys[entry.key] = entry.bibKey
	}
	return styled
}

// bibliographyEntries are the entries the paper printed, in order, when they can be one styled
// bibliography: one key per item, unique keys, the items one run of siblings, all with a printed label
// or none.
func (e *exporter) bibliographyEntries() ([]bibEntry, string) {
	listed := maps(dig(e.document, "citations", "entries"))
	entries := []bibEntry{}
	seen := map[string]bool{}
	for _, entry := range listed {
		blocks := rec.FetchList(entry, "blocks")
		if len(blocks) == 0 || blocks[0] == nil {
			continue
		}
		block := toS(blocks[0])
		key := toS(rec.Fetch(entry, "key"))
		if len(e.bibliography[block]) != 1 || seen[strings.ToLower(key)] || key == "" {
			return nil, "an item holds several keys, or a key names several items"
		}
		seen[strings.ToLower(key)] = true
		node := e.node(block)
		entries = append(entries, bibEntry{key: key, block: block, node: node, cited: e.cited(entry),
			numbered: bracketLabel.MatchString(marked(node)), element: printedLabel(node, toS(rec.Fetch(entry, "number"))) >= 0})
	}
	if len(entries) == 0 {
		return nil, "the paper printed no bibliography entry"
	}
	if !e.oneRun(entries) {
		return nil, "the entries are not one run of paragraphs (several bibliographies or sections)"
	}
	for _, entry := range entries {
		if entry.numbered != entries[0].numbered {
			return nil, "some entries print a label and some do not"
		}
	}
	return entries, ""
}

// oneRun: the entries' blocks are consecutive children of one parent, in order.
func (e *exporter) oneRun(entries []bibEntry) bool {
	parent, ok := e.parents[entries[0].block]
	if !ok {
		return false
	}
	siblings := children(e.node(parent))
	start := slices.Index(siblings, entries[0].block)
	if start < 0 || start+len(entries) > len(siblings) {
		return false
	}
	for index, entry := range entries {
		if siblings[start+index] != entry.block {
			return false
		}
	}
	return true
}

// bracketLabel is a printed label at the start of an entry ([ADM25]).
var bracketLabel = regexp.MustCompile(`\A[ \t\n\v\f\r]*\[[^\]]+\]`)

// printedLabel is where the item's text starts after its printed label ([12] or 12), -1 without one.
func printedLabel(node rec.Map, printed string) int {
	pattern := regexp.MustCompile(`\A[ \t\n\v\f\r]*\[?` + regexp.QuoteMeta(printed) + `\]?[ \t\n\v\f\r]*`)
	text := marked(node)
	location := pattern.FindStringIndex(text)
	if location == nil {
		return -1
	}
	return runeOffset(text, location[1])
}

// attemptStyled writes the styled files and checks them; the reason it failed ("" when it did not)
// and the entries that differ.
func (e *exporter) attemptStyled(b *Bibliography, entries []bibEntry) (string, []any) {
	keys := make([]string, len(entries))
	for index, entry := range entries {
		keys[index] = entry.key
	}
	placeholders := func(_ string, index int, _ string) (string, bool) { return placeholder(index), true }
	cleaned, report, err := bibclean.CleanFiles(b.Databases, bibclean.Options{Keys: keys, Math: placeholders})
	if err != nil {
		return "bibclean: " + err.Error(), nil
	}
	if reason := cleanerReason(report); reason != "" {
		return reason, nil
	}
	bibKeys(entries, report)
	if reason := e.writeStyledFiles(b, entries, cleaned); reason != "" {
		return reason, nil
	}
	printed, reason := runCheck(b.Typst, e.directory)
	if reason != "" {
		return reason, nil
	}
	if hasPlaceholders(printed) {
		if printed, reason = e.substituteMath(b, entries, keys, printed); reason != "" {
			return reason, nil
		}
	}
	return compareEntries(e.modelTexts(entries), printed)
}

// substituteMath gives each formula of the .bib the model's formula printed in its place (decision A):
// the placeholders' printed order is the model's order of formulas in the entry.
func (e *exporter) substituteMath(b *Bibliography, entries []bibEntry, keys []string, printed []printedEntry) ([]printedEntry, string) {
	formulas, reason := e.mapFormulas(entries, printed)
	if reason != "" {
		return nil, reason
	}
	route := func(key string, index int, _ string) (string, bool) {
		return formulas[strings.ToLower(key)][index], true
	}
	cleaned, report, err := bibclean.CleanFiles(b.Databases, bibclean.Options{Keys: keys, Math: route})
	if err != nil {
		return nil, "bibclean: " + err.Error()
	}
	if reason := cleanerReason(report); reason != "" {
		return nil, reason
	}
	writeFile(filepath.Join(e.directory, referencesFile), cleaned)
	return runCheck(b.Typst, e.directory)
}

// mapFormulas: for each entry (by lower-case .bib key), the Typst math for each formula index of the
// .bib, "" for one the entry does not print.
func (e *exporter) mapFormulas(entries []bibEntry, printed []printedEntry) (map[string]map[int]string, string) {
	byKey := map[string]printedEntry{}
	for _, entry := range printed {
		byKey[strings.ToLower(entry.Key)] = entry
	}
	formulas := map[string]map[int]string{}
	for _, entry := range entries {
		indexes := byKey[strings.ToLower(entry.bibKey)].placeholders()
		model := e.entryFormulas(entry.node)
		if len(indexes) != len(model) {
			return nil, fmt.Sprintf("the formulas of %s do not match the model's (%d printed, %d in the model)",
				entry.key, len(indexes), len(model))
		}
		mapped := map[int]string{}
		for position, index := range indexes {
			if _, repeated := mapped[index]; repeated {
				return nil, fmt.Sprintf("a formula of %s prints twice", entry.key)
			}
			typst, ok := e.bibMath(model[position])
			if !ok {
				return nil, fmt.Sprintf("a formula of %s has no Typst math", entry.key)
			}
			mapped[index] = "$" + strings.ReplaceAll(typst, `\`, `\\`) + "$"
		}
		formulas[strings.ToLower(entry.bibKey)] = mapped
	}
	return formulas, ""
}

// entryFormulas are the math nodes of an entry, in order; a URL url.sty set in math mode is text.
func (e *exporter) entryFormulas(node rec.Map) []rec.Map {
	formulas := []rec.Map{}
	for _, span := range content(node) {
		if _, isURL := e.urlFormula(span); span["type"] == "math" && !isURL {
			formulas = append(formulas, e.node(rec.Fetch(span, "node")))
		}
	}
	return formulas
}

// minURLText is the shortest link text read as an address: a title's one-letter formula inside the DOI
// link of its title is never one.
const minURLText = 4

// urlFormula: a math span that is an address url.sty typeset in math mode (biblatex's url and eprint
// fields, \url, \nolinkurl), with the characters it shows. Its text is an address (scheme://...), or the
// end of the target of the link it is in (an eprint's 2001.00001 in https://arxiv.org/abs/2001.00001).
// The .bib field holds no formula there, and Pergamon prints the characters as text (decision 173).
func (e *exporter) urlFormula(span rec.Map) (string, bool) {
	if span["type"] != "math" {
		return "", false
	}
	shown := strings.ReplaceAll(strings.ReplaceAll(e.mathText(span), " ", ""), "−", "-")
	if urlText.MatchString(shown) {
		return shown, true
	}
	link := span["external_link"]
	if len(shown) < minURLText || !truthy(link) {
		return "", false
	}
	if href := toS(e.externalHref(link)); href == "" || !strings.HasSuffix(href, shown) {
		return "", false
	}
	return shown, true
}

// bibMath is a formula as Typst math, false when the writer cannot express it.
func (e *exporter) bibMath(node rec.Map) (typst string, ok bool) {
	defer func() {
		if recovered := recover(); recovered != nil {
			if failure, isFailure := recovered.(rec.Failure); !isFailure || failure.Lookup {
				panic(recovered)
			}
			typst, ok = "", false
		}
	}()
	return e.math.convert(rec.FetchString(node, "mathml")), true
}

// placeholder is a formula's stand-in in the first check: one private-use character per index, which no
// style changes.
func placeholder(index int) string { return string(rune(placeholderBase + index)) }

const placeholderBase = 0xF0000

// cleanerReason: every entry and parent must convert.
func cleanerReason(report bibclean.Report) string {
	if len(report.MissingKeys) > 0 {
		return "entries missing from the .bib files: " + strings.Join(report.MissingKeys, ", ")
	}
	if len(report.MissingParents) > 0 {
		return "crossref parents missing from the .bib files: " + strings.Join(report.MissingParents, ", ")
	}
	failed := []string{}
	for _, entry := range report.Entries {
		if entry.Status == bibclean.StatusUnconverted {
			failed = append(failed, entry.Key+" ("+entry.Reasons[0].Kind+")")
		}
	}
	if len(failed) > 0 {
		return "bibclean left entries unconverted: " + strings.Join(failed[:min(len(failed), maxReported)], ", ")
	}
	return ""
}

// bibKeys: each entry's key as the .bib spells it (BibTeX matches keys without case).
func bibKeys(entries []bibEntry, report bibclean.Report) {
	spelled := map[string]string{}
	for _, entry := range report.Entries {
		spelled[strings.ToLower(entry.Key)] = entry.Key
	}
	for index := range entries {
		entries[index].bibKey = spelled[strings.ToLower(entries[index].key)]
	}
}

// writeStyledFiles writes the vendored packages, the style, references.bib and bibliography.typ.
func (e *exporter) writeStyledFiles(b *Bibliography, entries []bibEntry, cleaned string) string {
	if _, err := typstvendor.Write(b.Root, e.directory); err != nil {
		return err.Error()
	}
	pin, err := typstvendor.LoadPin(b.Root)
	if err != nil {
		return err.Error()
	}
	style, err := os.ReadFile(b.Style.Path)
	if err != nil {
		return err.Error()
	}
	if style, err = typstvendor.RewriteBeside(style, styleFile, pin.Packages); err != nil {
		return "the style " + b.Style.Name + ".typ: " + err.Error()
	}
	writeFile(filepath.Join(e.directory, SupportDirectory, styleFile), string(style))
	writeFile(filepath.Join(e.directory, referencesFile), cleaned)
	writeFile(filepath.Join(e.directory, SupportDirectory, bibliographyFile), e.bibliographySource(entries))
	return ""
}

// bibliographySource is bibliography.typ with this paper's keys and labels.
func (e *exporter) bibliographySource(entries []bibEntry) string {
	pairs := make([]string, len(entries))
	for index, entry := range entries {
		label := "none"
		if entry.cited {
			label = typstString(e.labels["cite:"+entry.key])
		}
		pairs[index] = typstString(entry.bibKey) + ": " + label
	}
	numbered := "false"
	if entries[0].numbered {
		numbered = "true"
	}
	source := strings.Replace(bibliographyTemplate, "/*LABELS*/", strings.Join(pairs, ", ")+trailingColon(len(pairs)), 1)
	return strings.Replace(source, "/*NUMBERED*/", numbered, 1)
}

// trailingColon: a Typst dictionary literal needs a key; an empty one is (:).
func trailingColon(count int) string {
	if count == 0 {
		return ":"
	}
	return ""
}

// removeStyledFiles leaves a literal export without the styled bibliography's files, and without support/
// (which only they use).
func removeStyledFiles(directory string) {
	for _, name := range []string{referencesFile, path.Join(SupportDirectory, styleFile), path.Join(SupportDirectory, bibliographyFile),
		path.Join(SupportDirectory, checkFile), typstvendor.ExportDirectory} {
		if err := os.RemoveAll(filepath.Join(directory, name)); err != nil {
			rec.Fail("%v", err)
		}
	}
	support := filepath.Join(directory, SupportDirectory)
	if left, err := os.ReadDir(support); err == nil && len(left) == 0 {
		if err := os.Remove(support); err != nil {
			rec.Fail("%v", err)
		}
	}
}

// ---- rendering ----

// bibliographySetup is main.typ's opening for a styled bibliography: Pergamon, the resource and the
// refsection around the paper.
func (e *exporter) bibliographySetup() string {
	if e.styledBib == nil {
		return ""
	}
	return "#import \"" + SupportDirectory + "/" + bibliographyFile + "\": pergamon, ltd-bib-style, ltd-bibliography\n" +
		"#pergamon.add-bib-resource(read(\"" + referencesFile + "\"))\n" +
		"#show: pergamon.refsection.with(style: ltd-bib-style)"
}

// styledItem: the first entry's block prints the whole bibliography; the others print nothing. Each
// cited entry's label is placed there (bibliography.typ labels its figure).
func (e *exporter) styledItem(id string) string {
	if id != e.styledBib.first {
		return ""
	}
	for key := range e.styledBib.keys {
		if _, ok := e.labels["cite:"+key]; ok {
			placement := anchor
			if e.styledBib.numbered && e.styledBib.elements[key] {
				placement = element
			}
			e.placed["cite:"+key] = placement
		}
	}
	return "#ltd-bibliography()"
}

// styledCitation is a citation of a styled entry: Pergamon prints its label.
func (e *exporter) styledCitation(key string) (string, bool) {
	if e.styledBib == nil {
		return "", false
	}
	bibKey, ok := e.styledBib.keys[key]
	if !ok {
		return "", false
	}
	return "#pergamon.citen(" + typstString(bibKey) + ");", true
}

// ---- records ----

func stringsToAny(values []string) []any {
	result := make([]any, len(values))
	for index, value := range values {
		result[index] = value
	}
	return result
}

func (b *Bibliography) styleRecord() any {
	style := b.Style
	if style == nil {
		return nil
	}
	path := b.recorded(style.Path)
	if style.Origin == bibstyle.OriginLibrary {
		// The library is a resource: its path names the resource, not where this binary unpacked it.
		path = bibstyle.Library + "/" + style.Name + ".typ"
	}
	return rec.Map{"name": style.Name, "path": path, "sha256": style.SHA256, "origin": style.Origin}
}

func (b *Bibliography) databaseRecords() []any {
	records := make([]any, len(b.Databases))
	for index, path := range b.Databases {
		records[index] = rec.Map{"path": b.recorded(path), "sha256": fileSHA256(path)}
	}
	return records
}

// recorded is how the report names a file: by its path in the sources when it is one of them, else
// with the home directory as "~".
func (b *Bibliography) recorded(path string) string {
	if b.Sources != "" {
		if relative, err := filepath.Rel(b.Sources, path); err == nil && filepath.IsLocal(relative) {
			return filepath.ToSlash(relative)
		}
	}
	return homeRelative(path)
}

// homeRelative writes the home directory "~", as the run's configuration does.
func homeRelative(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if rest, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok {
		return "~/" + filepath.ToSlash(rest)
	}
	return path
}
