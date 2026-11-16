// Package authordir names the files a paper's author owns or approves for relatex (decision 124):
// all of them live in one visible directory, relatex/, beside the paper's main .tex file, and are
// committed with the paper. Generated results stay in output/.
package authordir

import (
	"os"
	"path/filepath"
)

// Dir is the author's relatex directory, relative to the paper directory.
const Dir = "relatex"

// The author's files, relative to the paper directory.
const (
	Config    = Dir + "/config.jsonc"     // per-paper settings over ~/.config/relatex/config.jsonc
	Hints     = Dir + "/hints.json"       // author hints (decision 122)
	Alt       = Dir + "/alt.toml"         // figure descriptions (decision 086)
	AltReview = Dir + "/" + AltReviewName // figure decisions saved from the review page
	Lint      = Dir + "/" + LintName      // accessibility lint items the author accepted

	// The names a review page saves under (a browser download cannot name a directory).
	AltReviewName = "alt-review.toml"
	LintName      = "lint.toml"
)

// Ensure creates the directory that holds path (relatex/ on the first write there).
func Ensure(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o755)
}
