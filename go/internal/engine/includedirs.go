package engine

import (
	"os"
	"path/filepath"
	"strings"
)

// mirrorIncludeDirectories creates in the output directory the subdirectories of the working
// directory that hold TeX sources. LaTeX writes the .aux file of `\include{Appendix/main}` to
// output-directory/Appendix/main.aux and never creates that directory, so a build that sets an
// output directory needs them, as a plain run in the source directory has them.
func mirrorIncludeDirectories(working, output string) error {
	return filepath.WalkDir(working, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tex") {
			return err
		}
		relative, err := filepath.Rel(working, filepath.Dir(path))
		if err != nil || relative == "." || strings.HasPrefix(relative, "_relatex_probe") {
			return err
		}
		return os.MkdirAll(filepath.Join(output, relative), 0o755)
	})
}
