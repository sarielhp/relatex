package syncdemo

import (
	"fmt"
	"strings"
	"unicode"
)

// sourcePane is the source files as tabs: each file a panel of numbered lines, each word and each
// command a focusable span that carries its column (data-c, in characters from the line's start).
func sourcePane(sources []Source) string {
	var out strings.Builder
	out.WriteString(`<div class="tabs" role="tablist">`)
	for i, source := range sources {
		out.WriteString(fmt.Sprintf(`<button type="button" role="tab" class="tab" data-f="%d">%s</button>`, i, escapeText.Replace(source.Name)))
	}
	out.WriteString(`</div>`)
	for i, source := range sources {
		out.WriteString(fmt.Sprintf(`<div class="panel" role="tabpanel" data-f="%d" hidden>`, i))
		for number, line := range strings.Split(strings.TrimSuffix(source.Text, "\n"), "\n") {
			out.WriteString(fmt.Sprintf(`<div class="ln" data-l="%d"><span class="no">%d</span><span class="tx">`, number+1, number+1))
			out.WriteString(sourceLine(maskEmails(line)))
			out.WriteString(`</span></div>`)
		}
		out.WriteString(`</div>`)
	}
	return out.String()
}

// sourceLine is one line escaped, each word or command in a span with its start column.
func sourceLine(line string) string {
	runes := []rune(line)
	var out strings.Builder
	for i := 0; i < len(runes); {
		end := tokenEnd(runes, i)
		if end == i {
			out.WriteString(escapeText.Replace(string(runes[i])))
			i++
			continue
		}
		out.WriteString(fmt.Sprintf(`<span class="t" tabindex="0" data-c="%d">%s</span>`, i, escapeText.Replace(string(runes[i:end]))))
		i = end
	}
	return out.String()
}

// tokenEnd is the end of the word (letters and digits) or command (\ and letters) starting at i,
// i itself when the character starts neither.
func tokenEnd(runes []rune, i int) int {
	end := i
	if runes[i] == '\\' {
		end++
		for end < len(runes) && unicode.IsLetter(runes[end]) {
			end++
		}
		if end == i+1 {
			return i
		}
		return end
	}
	for end < len(runes) && (unicode.IsLetter(runes[end]) || unicode.IsDigit(runes[end])) {
		end++
	}
	return end
}
