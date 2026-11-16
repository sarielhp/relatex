package artifacts

// AuthorBuildProducts are the extensions of the files an author's own LaTeX run leaves beside
// the main file (PaperCheck::STALE): <job>.aux, <job>.log, <job>.synctex.gz, ... The engine sets
// them aside before its own builds, so they are not sources of the paper.
var AuthorBuildProducts = []string{"aux", "log", "toc", "out", "lof", "lot", "fls", "fdb_latexmk", "synctex.gz", "bcf",
	"run.xml", "blg", "nav", "snm", "vrb"}

// IsAuthorBuildProduct is whether name, relative to the source root, is one of those files of
// the job.
func IsAuthorBuildProduct(name, job string) bool {
	for _, extension := range AuthorBuildProducts {
		if name == job+"."+extension {
			return true
		}
	}
	return false
}
