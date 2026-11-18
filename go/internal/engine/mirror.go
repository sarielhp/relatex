package engine

import (
	"io/fs"
	"os"
	"path/filepath"
)

// mirrorDirectories makes the working copy's directories exist below the output directory. TeX with
// -output-directory opens an \openout file relative to the output directory, so a paper that writes
// fragment/myfile.tmp (as it would beside its source) needs fragment/ there.
func mirrorDirectories(working, output string) {
	_ = filepath.WalkDir(working, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.IsDir() || path == working {
			return nil
		}
		relative, relErr := filepath.Rel(working, path)
		if relErr == nil {
			_ = os.MkdirAll(filepath.Join(output, relative), 0o755)
		}
		return nil
	})
}
