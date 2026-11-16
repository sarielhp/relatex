// Same and Difference decide whether two PDFs are the same build (relatex synctex, decision 101):
// byte for byte, but for the trailer's /ID. LuaTeX hashes the working directory and the output
// file's name into /ID, so two builds of one source in two directories differ there and nowhere
// else; every page, font, resource and the document information (dates included) must match.

package synctexout

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/sarielhp/relatex/internal/command"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// trailerID is a /ID array of two hexadecimal strings, in a trailer or a cross-reference stream's
// dictionary (which is never compressed).
var trailerID = regexp.MustCompile(`/ID\s*\[\s*<([0-9A-Fa-f]*)>\s*<([0-9A-Fa-f]*)>\s*\]`)

// Normalized is the PDF with the digits of every /ID array zeroed: the same length, so no offset moves.
func Normalized(data []byte) []byte {
	return trailerID.ReplaceAllFunc(data, func(match []byte) []byte {
		zeroed := bytes.Clone(match)
		inside := false
		for index, character := range zeroed {
			switch {
			case character == '<':
				inside = true
			case character == '>':
				inside = false
			case inside:
				zeroed[index] = '0'
			}
		}
		return zeroed
	})
}

// Same is whether two PDFs are the same build: identical bytes once their /ID arrays are zeroed.
func Same(first, second []byte) bool {
	return bytes.Equal(Normalized(first), Normalized(second))
}

// pageMark begins one of a page's objects in qpdf's QDF form: its dictionary or a content stream.
var pageMark = regexp.MustCompile(`(?m)^%% (?:Page|Contents for page) (\d+)\n`)

// objectHeader and lengthEntry are what moves when an earlier object grows: the object numbers
// and the lengths of the streams.
var (
	objectHeader = regexp.MustCompile(`(?m)^\d+ \d+ obj\n|\d+ \d+ R`)
	lengthEntry  = regexp.MustCompile(`(?m)^\d+\n`)
)

// Difference says, for the refusal message, where two different PDFs part: the page counts, the
// first page whose content differs, or the document outside the pages. It needs qpdf; "" without it.
func Difference(first, second string) string {
	pagesFirst, errFirst := pages(first)
	pagesSecond, errSecond := pages(second)
	if errFirst != nil || errSecond != nil {
		return ""
	}
	if len(pagesFirst) != len(pagesSecond) {
		return fmt.Sprintf("%d pages against %d", len(pagesFirst), len(pagesSecond))
	}
	for index := range pagesFirst {
		if pagesFirst[index] != pagesSecond[index] {
			return fmt.Sprintf("page %d of %d differs", index+1, len(pagesFirst))
		}
	}
	return "the pages match; the document information or resources differ"
}

// pages is each page's objects in qpdf's QDF form, with the object numbers and stream lengths left out.
func pages(path string) ([]string, error) {
	qpdf := command.Resolve("qpdf")
	if qpdf == "" {
		return nil, errors.New("qpdf not found (RELATEX_QPDF, PATH)")
	}
	output, err := exec.Command(qpdf, "--qdf", "--object-streams=disable", "--deterministic-id", path, "-").Output()
	if err != nil && len(output) == 0 {
		return nil, err
	}
	text := string(output)
	marks := pageMark.FindAllStringSubmatchIndex(text, -1)
	objects := map[int]string{}
	count := 0
	for _, mark := range marks {
		number, _ := strconv.Atoi(text[mark[2]:mark[3]])
		count = max(count, number)
		object := text[mark[1]:]
		if end := strings.Index(object, "endobj"); end >= 0 {
			object = object[:end]
		}
		objects[number] += lengthEntry.ReplaceAllString(objectHeader.ReplaceAllString(object, ""), "")
	}
	result := make([]string, count)
	for number, object := range objects {
		result[number-1] = object
	}
	return result, nil
}
