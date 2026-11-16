package mathsource

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func naiveByteOffset(text string, scalar int) int {
	count := 0
	for index := range text {
		if count == scalar {
			return index
		}
		count++
	}
	if count == scalar {
		return len(text)
	}
	return -1
}

func TestOffsetsAgreeWithTheScanForAsciiAndMultibyteText(t *testing.T) {
	for _, text := range []string{"", "abc $x$ def", "α β $γ$ δ é", strings.Repeat("ab é ", 50)} {
		for scalar := -1; scalar <= utf8.RuneCountInString(text)+2; scalar++ {
			if got, want := byteOffset(text, scalar), naiveByteOffset(text, scalar); got != want {
				t.Fatalf("byteOffset(%q, %d) = %d, want %d", text, scalar, got, want)
			}
		}
		for offset := 0; offset <= len(text); offset++ {
			if !utf8.ValidString(text[:offset]) {
				continue
			}
			if got, want := scalarCount(text, offset), utf8.RuneCountInString(text[:offset]); got != want {
				t.Fatalf("scalarCount(%q, %d) = %d, want %d", text, offset, got, want)
			}
		}
	}
}
