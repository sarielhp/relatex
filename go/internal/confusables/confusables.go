// Package confusables is Unicode's confusable characters (UTS #39, Unicode Security Mechanisms) and
// their skeletons: two strings are confusable when their skeletons are equal.
//
// The data is confusables.txt of UTS #39 version 18.0.0 (Date: 2026-08-06, 01:05:35 GMT), from
// https://www.unicode.org/Public/security/latest/confusables.txt (fetched 2026-11-13; that version
// is also at https://www.unicode.org/Public/security/18.0.0/confusables.txt), SHA-256
// 6ed3ee967c9dfdf6677d563c9985182fbc50a2efb7d6059cd57b2e2ce18f5b92, shipped unchanged beside this
// file under Unicode's terms of use (https://www.unicode.org/terms_of_use.html) and never fetched
// at run time (decision 148).
package confusables

import (
	_ "embed"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/text/unicode/norm"
)

//go:embed confusables.txt
var data string

var (
	once       sync.Once
	prototypes map[rune]string
)

// load reads the mappings: "source ; target ; MA # comment", code points in hex.
func load() {
	prototypes = map[rune]string{}
	for line := range strings.SplitSeq(data, "\n") {
		line, _, _ = strings.Cut(line, "#")
		fields := strings.Split(line, ";")
		if len(fields) < 2 {
			continue
		}
		source, err := strconv.ParseUint(strings.TrimSpace(fields[0]), 16, 32)
		if err != nil {
			continue
		}
		var target strings.Builder
		valid := true
		for _, code := range strings.Fields(fields[1]) {
			value, err := strconv.ParseUint(code, 16, 32)
			if err != nil {
				valid = false
				break
			}
			target.WriteRune(rune(value))
		}
		if valid {
			prototypes[rune(source)] = target.String()
		}
	}
}

// Skeleton is UTS #39's skeleton of text: NFD, each character replaced by its prototype, NFD again.
func Skeleton(text string) string {
	once.Do(load)
	var out strings.Builder
	for _, r := range norm.NFD.String(text) {
		if prototype, ok := prototypes[r]; ok {
			out.WriteString(prototype)
			continue
		}
		out.WriteRune(r)
	}
	return norm.NFD.String(out.String())
}

// Confusable reports whether a and b have the same skeleton.
func Confusable(a, b string) bool { return Skeleton(a) == Skeleton(b) }
