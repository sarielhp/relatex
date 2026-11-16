package frontmatter

import (
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

// genericNames: authors are separated by \and (\And, \AND in some classes), and by
// sig-alternate's \alignauthor, which starts the next author block.
func (t *TexMetadata) genericNames(head string) []string {
	names := []string{}
	for _, row := range t.commands(head, []string{"author"}) {
		for _, name := range topLevelSplit(row.argument, andSeparator) {
			if strip(name) != "" {
				names = append(names, name)
			}
		}
	}
	// Interspeech lists its authors, comma-separated, in \name{...}. (Ruby's separator,
	// /,|\band\b/, is only tried at a backslash, so the argument stays whole.)
	if len(names) == 0 {
		for _, row := range t.commands(head, []string{"name"}) {
			names = append(names, topLevelSplit(row.argument, neverSeparator)...)
		}
	}
	return names
}

func (t *TexMetadata) genericAuthors(head string, names []string) []any {
	if authors, ok := t.jmlrAuthors(head); ok {
		return authors
	}
	if authors, ok := t.minipageAuthors(head); ok {
		return authors
	}
	result := []any{}
	for _, name := range names {
		result = append(result, t.authorLines(name)...)
	}
	return result
}

// separator matches at a position of top_level_split, returning where it ends.
type separator func(text string, position int) (int, bool)

func andSeparator(text string, position int) (int, bool) {
	_, end, ok := commandAt(text, position, []string{"and", "And", "AND", "alignauthor"}, wordEnd)
	return end, ok
}

func jmlrSeparator(text string, position int) (int, bool) {
	_, end, ok := commandAt(text, position, []string{"name"}, wordEnd)
	return end, ok
}

func lineSeparator(text string, position int) (int, bool) {
	location := lineBreakAt.FindStringIndex(text[position:])
	if location == nil {
		return 0, false
	}
	return position + location[1], true
}

func neverSeparator(string, int) (int, bool) { return 0, false }

// topLevelSplit splits at separators outside any group (\quad inside
// \IEEEauthorblockA{...} separates nothing).
func topLevelSplit(text string, split separator) []string {
	pieces := []string{}
	depth, start, position := 0, 0, 0
	for position < len(text) {
		character := text[position]
		if character == '\\' {
			if depth == 0 {
				if end, ok := split(text, position); ok {
					pieces = append(pieces, text[start:position])
					position, start = end, end
					continue
				}
			}
			position += 2
			continue
		}
		switch character {
		case '{':
			depth++
		case '}':
			depth--
		}
		position++
	}
	if start > len(text) {
		start = len(text)
	}
	return append(pieces, text[start:])
}

// minipageAuthors: authors laid out in minipages (one per author): the name line, then
// affiliation lines.
func (t *TexMetadata) minipageAuthors(head string) ([]any, bool) {
	argument, found := "", false
	for _, row := range t.commands(head, []string{"author"}) {
		if strings.Contains(row.argument, `\begin{minipage}`) {
			argument, found = row.argument, true
			break
		}
	}
	if !found {
		return nil, false
	}
	blocks := []string{}
	for _, block := range scanFirst(minipage, argument) {
		blocks = append(blocks, centeringEnvironment.ReplaceAllLiteralString(block, ""))
	}
	names := emptyFontGroup.ReplaceAllLiteralString(minipage.ReplaceAllLiteralString(argument, " "), " ")
	if lineBreakOrSpace.ReplaceAllLiteralString(names, "") != "" {
		return t.namesOverMinipages(names, blocks), true
	}
	result := []any{}
	for _, block := range blocks {
		for _, value := range t.authorLines(block) {
			author := rec.AsMap(value)
			emails, affiliations := []any{}, []any{}
			for _, line := range rec.FetchList(author, "affiliations") {
				if emailLine.MatchString(line.(string)) {
					emails = append(emails, line)
				} else {
					affiliations = append(affiliations, line)
				}
			}
			result = append(result, rec.Map{"name": author["name"], "affiliations": affiliations, "emails": emails})
		}
	}
	return result, true
}

var (
	centeringEnvironment = rx(`\\(?:begin|end)\{(?:center|flushleft|flushright)\}`)
	emptyFontGroup       = rx(`\{\s*(?:\\[A-Za-z]+\s*)*\}`)
	lineBreakOrSpace     = rx(`\\\\(?:\[[^\]]*\])?|\s`)
	lineBreakAnywhere    = rx(`\\\\(?:\[[^\]]*\])?`)
)

// namesOverMinipages: names set above one minipage that holds the shared affiliation (ICME:
// the names, then a centred university and address list): every name has that block's lines.
func (t *TexMetadata) namesOverMinipages(names string, blocks []string) []any {
	affiliations := []any{}
	for _, block := range blocks {
		for _, line := range topLevelSplit(block, lineSeparator) {
			line = strip(line)
			if line == "" || contactLine(line) || strings.Contains(line, "@") {
				continue
			}
			if text := t.plain(line, false); text != "" {
				affiliations = append(affiliations, text)
			}
		}
	}
	result := []any{}
	for _, author := range t.authorLine(lineBreakAnywhere.ReplaceAllLiteralString(names, ",")) {
		author["affiliations"] = slices.Clone(affiliations)
		result = append(result, author)
	}
	return result
}

// jmlrAuthors: JMLR's \author{\name A \email a@x \\ affiliation \AND \name B ...}.
func (t *TexMetadata) jmlrAuthors(head string) ([]any, bool) {
	argument, found := "", false
	for _, row := range t.commands(head, []string{"author"}) {
		if _, _, _, ok := findCommand(row.argument, 0, []string{"name"}, wordEnd); ok {
			argument, found = row.argument, true
			break
		}
	}
	if !found {
		return nil, false
	}
	result := []any{}
	for _, chunk := range topLevelSplit(argument, jmlrSeparator)[1:] {
		if start, _, _, ok := findCommand(chunk, 0, []string{"And", "AND"}, wordEnd); ok {
			chunk = chunk[:start]
		}
		lines := topLevelSplit(chunk, lineSeparator)
		name, email := lines[0], ""
		if start, end, _, ok := findCommand(name, 0, []string{"email"}, wordEnd); ok {
			name, email = name[:start], name[end:]
		}
		affiliations := []any{}
		for _, line := range lines[1:] {
			if start, end, _, ok := findCommand(line, 0, []string{"addr"}, wordEnd); ok {
				line = line[:start] + line[end:]
			}
			if text := t.plain(line, false); text != "" {
				affiliations = append(affiliations, text)
			}
		}
		emails := []any{}
		for _, field := range awkSplit(email) {
			if value := strip(field); value != "" && len(emails) == 0 {
				emails = append(emails, value)
			}
		}
		result = append(result, rec.Map{"name": t.plain(name, false), "affiliations": affiliations, "emails": emails})
	}
	return result, true
}

// authorLines: in an article's \author entry the first lines are names; the lines after
// \\ are the affiliation. A name line listing several people (A, B and C) is one author
// each. A line that is only a contact address list opens the affiliation block.
func (t *TexMetadata) authorLines(value string) []any {
	// IEEE ordinals (1\textsuperscript{st}) and membership grades are not names; \quad and
	// \hspace separate names.
	value = ordinal.ReplaceAllLiteralString(value, "")
	value = replaceCommands(value, []string{"qquad", "quad"}, wordEnd, func(string) string { return "," })
	value = hspaceCall.ReplaceAllLiteralString(value, ",")
	// Authors set in a tabular: its rows are lines, its cells names.
	value = tabular.ReplaceAllLiteralString(value, "")
	value = replaceUnescaped(value, '&', ",")
	lines := []string{}
	for _, line := range topLevelSplit(value, lineSeparator) {
		if text := strip(line); text != "" {
			lines = append(lines, text)
		}
	}
	head, rest := authorLineGroups(lines)
	affiliations := []any{}
	for _, line := range rest {
		if text := t.plain(line, false); text != "" {
			affiliations = append(affiliations, text)
		}
	}
	result := []any{}
	for _, line := range head {
		for _, author := range t.authorLine(line) {
			author["affiliations"] = slices.Clone(affiliations)
			result = append(result, author)
		}
	}
	return result
}

// authorLineGroups: leading lines list authors (a name line carries an email, \and, or an
// author mark); the first line that is none of those opens the affiliation block.
func authorLineGroups(lines []string) (head, rest []string) {
	for position := 1; position < len(lines); position++ {
		if !authorLineText(lines[position]) {
			return lines[:position], lines[position:]
		}
	}
	return lines, []string{}
}

// authorLineText is author_line?: a name line carries an email, \and or an author mark.
func authorLineText(line string) bool {
	if affiliationLineText(line) || contactLine(line) {
		return false
	}
	return strings.Contains(line, "@") || andWord.MatchString(line) ||
		typesetNameMark.MatchString(line) || authorMarkLine.MatchString(line)
}

// affiliationLineText: a line opening with an affiliation mark (\textsuperscript{\dag}Georgia
// Institute ...) lists affiliations, as one opening with a math mark (${}^1$ Computation Lab) does.
func affiliationLineText(line string) bool {
	return affiliationStart.MatchString(line) || typesetMarkStart.MatchString(line) ||
		footnoteMarkStart.MatchString(line)
}

// contactLine is a line that is only one address list (\{a, b, c\}@example.org): the
// authors' contact address prints there, not their names.
func contactLine(line string) bool {
	text := contactCommand.ReplaceAllLiteralString(line, " ")
	text = contactSymbol.ReplaceAllLiteralString(text, "")
	text = strings.NewReplacer("{", "", "}", "").Replace(text)
	return contact.MatchString(strip(squeezeSpace(text, " ")))
}

// authorLine: one author line: names separated by commas or \and, with parenthesized or
// \texttt emails.
// notesAsSeparators: a note ends the name it follows (Sonia Marsh\sthanks{...} Hugo Ertel
// lists two people).
func (t *TexMetadata) notesAsSeparators(line string) string {
	check := func(text string, end int) (int, bool) {
		if !notLetter(text, end) {
			return 0, false
		}
		return beforeBrace(text, end)
	}
	for {
		start, end, _, ok := findCommand(line, 0, []string{"thanks", "sthanks", "footnote"}, check)
		if !ok {
			return line
		}
		_, ending := t.argument(line, end)
		line = line[:start] + "," + line[ending:]
	}
}

func (t *TexMetadata) authorLine(line string) []rec.Map {
	emails := authorEmails(line)
	// A \thanks note is a footnote, not an email of the author line.
	names := textttOneLevel.ReplaceAllLiteralString(t.notesAsSeparators(line), " ")
	names = parenEmail.ReplaceAllLiteralString(names, " ")
	names = bareEmail.ReplaceAllLiteralString(names, " ")
	first := membership.ReplaceAllLiteralString(t.plain(names, false), "")
	listed := []string{}
	for _, name := range nameSeparator.Split(first, -1) {
		name = leadingAnd.ReplaceAllLiteralString(strip(name), "")
		if name != "" {
			listed = append(listed, name)
		}
	}
	// An email belongs to the one author whose line holds it; a line listing several names
	// and several emails does not say whose is whose.
	if len(listed) != 1 {
		emails = []any{}
	}
	result := []rec.Map{}
	for _, name := range listed {
		result = append(result, rec.Map{"name": authorMark.ReplaceAllLiteralString(name, ""),
			"affiliations": []any{}, "emails": emails})
	}
	return result
}

// authorEmails are the addresses an author line holds.
func authorEmails(line string) []any {
	line = thanksOneLevel.ReplaceAllLiteralString(line, " ")
	result := []any{}
	for _, match := range emailPattern.FindAllStringSubmatch(line, -1) {
		if match[1] != "" {
			result = append(result, match[1])
		} else {
			result = append(result, match[2])
		}
	}
	return result
}

// replaceUnescaped is gsub(/(?<!\\)c/, replacement).
func replaceUnescaped(text string, c byte, replacement string) string {
	var builder strings.Builder
	position := 0
	for _, index := range unescaped(text, c) {
		builder.WriteString(text[position:index])
		builder.WriteString(replacement)
		position = index + 1
	}
	builder.WriteString(text[position:])
	return builder.String()
}

func (t *TexMetadata) authors(head string, affiliations []string) []any {
	rows := []rec.Map{}
	members := [][]int{}
	emails := [][]any{}
	for _, row := range t.commands(head, append([]string{"author", "email"}, affiliations...)) {
		if row.name == "author" {
			// A nested affiliation (\author{Name \address{...}}) is that author's affiliation,
			// read as a following \address is; the name is read without it.
			argument := t.withoutAffiliationArguments(row.argument, affiliations)
			value := t.plain(argument, false)
			group, addresses := t.appendAuthors(&rows, argument, value)
			members, emails = append(members, group), append(emails, addresses)
			continue
		}
		argument := row.argument
		if row.name == "affiliation" && t.class() == "acmart" {
			argument = t.acmAddress(argument, acmObeyPunctuation(row.options))
		}
		value := t.plain(argument, row.name == "email")
		// An empty field (a template's \email{ }) declares nothing.
		if value == "" {
			continue
		}
		if len(rows) == 0 {
			rec.Fail("ACM affiliation or email precedes author")
		}
		if row.name == "email" {
			// Ruby's authors share the email list of their author line: a following \email
			// belongs to every author that line listed.
			emails[len(emails)-1] = append(emails[len(emails)-1], value)
			continue
		}
		last := rows[len(rows)-1]
		last["affiliations"] = append(last["affiliations"].([]any), value)
	}
	if len(rows) == 0 {
		rec.Fail("Literal ACM metadata requires authors")
	}
	for index, group := range members {
		for _, member := range group {
			rows[member]["emails"] = emails[index]
		}
	}
	result := make([]any, len(rows))
	for index, row := range rows {
		result[index] = row
	}
	return result
}

// withoutAffiliationArguments removes an affiliation command and its argument from an
// author's own argument: \author{Name \address{...}} reads the address as a following
// \address does, so the name is read without it.
func (t *TexMetadata) withoutAffiliationArguments(value string, names []string) string {
	position := 0
	for {
		start, end, _, ok := findCommand(value, position, names, wordEndSpace)
		if !ok {
			return value
		}
		if end >= len(value) || value[end] != '{' {
			position = end
			continue
		}
		_, ending := t.argument(value, end)
		value = value[:start] + " " + value[ending:]
		position = start + 1
	}
}

// appendAuthors adds one \author row's authors and returns their positions and the email
// list they share.
func (t *TexMetadata) appendAuthors(rows *[]rec.Map, argument, value string) (members []int, emails []any) {
	for _, author := range t.authorRows(argument, value) {
		item := rec.AsMap(author)
		if len(members) == 0 {
			emails, _ = item["emails"].([]any)
		}
		members = append(members, len(*rows))
		*rows = append(*rows, item)
	}
	return members, emails
}

// authorRows: AMS's \author{Name, Name \\ email} lists several authors; one name stays
// one author.
func (t *TexMetadata) authorRows(argument, value string) []any {
	expand := t.AMS() &&
		(lineBreak.MatchString(argument) || commaOnlyArgument.MatchString(argument))
	one := []any{rec.Map{"name": value, "affiliations": []any{}, "emails": []any{}}}
	if !expand {
		return one
	}
	rows := t.authorLines(argument)
	if len(rows) == 0 {
		return one
	}
	return rows
}
