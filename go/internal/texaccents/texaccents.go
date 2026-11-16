// Package texaccents holds the LaTeX accent commands and the Unicode combining marks they print, shared by the
// source-text reader and the .bib cleaner.
package texaccents

// Combining maps a LaTeX accent command (without its backslash) to its Unicode combining mark.
var Combining = map[string]string{
	"`": "̀", // grave
	"'": "́", // acute
	"^": "̂", // circumflex
	"~": "̃", // tilde
	"=": "̄", // macron
	"u": "̆", // breve
	".": "̇", // dot above
	`"`: "̈", // diaeresis
	"r": "̊", // ring above
	"H": "̋", // double acute
	"v": "̌", // caron
	"c": "̧", // cedilla
	"d": "̣", // dot below
	"b": "̱", // macron below
	"k": "̨", // ogonek
}
