package syncdemo

import "strings"

// maxCells bounds the alignment table of one node; a longer node is aligned greedily.
const maxCells = 16_000_000

// align pairs the HTML words of one node (their normalized texts) with the PDF words of the same
// node, in order: the longest common subsequence, so a word the HTML or the PDF has alone (a list
// marker, a hyphen's second half, a footnote mark) is skipped. It returns, per HTML word, the index
// of its PDF word or -1.
func align(html []string, pdf []string) []int {
	pairs := make([]int, len(html))
	for i := range pairs {
		pairs[i] = -1
	}
	if len(html) == 0 || len(pdf) == 0 {
		return pairs
	}
	if len(html)*len(pdf) > maxCells {
		return greedy(html, pdf, pairs)
	}
	width := len(pdf) + 1
	table := make([]int32, (len(html)+1)*width)
	for i := len(html) - 1; i >= 0; i-- {
		for j := len(pdf) - 1; j >= 0; j-- {
			switch {
			case same(html[i], pdf[j]):
				table[i*width+j] = table[(i+1)*width+j+1] + 1
			case table[(i+1)*width+j] >= table[i*width+j+1]:
				table[i*width+j] = table[(i+1)*width+j]
			default:
				table[i*width+j] = table[i*width+j+1]
			}
		}
	}
	for i, j := 0, 0; i < len(html) && j < len(pdf); {
		switch {
		case same(html[i], pdf[j]):
			pairs[i] = j
			i++
			j++
		case table[(i+1)*width+j] >= table[i*width+j+1]:
			i++
		default:
			j++
		}
	}
	return pairs
}

// greedy pairs each HTML word with the next equal PDF word within a short window.
func greedy(html []string, pdf []string, pairs []int) []int {
	const window = 8
	j := 0
	for i, word := range html {
		for k := j; k < len(pdf) && k < j+window; k++ {
			if same(word, pdf[k]) {
				pairs[i], j = k, k+1
				break
			}
		}
	}
	return pairs
}

// same is whether an HTML word and a PDF word are the same word: equal, or the PDF's carries a
// footnote mark (digits) after it, as "very1" does for "very" and its mark in the HTML.
func same(html, pdf string) bool {
	if html == pdf {
		return true
	}
	if len(pdf) <= len(html) || pdf[:len(html)] != html || strings.Trim(html, "0123456789") == "" {
		return false
	}
	return strings.Trim(pdf[len(html):], "0123456789") == ""
}
