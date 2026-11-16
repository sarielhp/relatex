package canonjson

import (
	"github.com/sarielhp/relatex/internal/fspath"
	"os"
	"path/filepath"
)

// WriteFile is Relatex.write_json of a canonical value: Pretty, a final newline, written to
// a temporary file and renamed into place.
func WriteFile(path string, value any) error {
	bytes, err := Pretty(value)
	if err != nil {
		return err
	}
	return WriteAtomic(path, append(bytes, '\n'))
}

// WriteAtomic writes data to a temporary file of its own beside path and renames it over path,
// so a reader sees the old file or the new one. The temporary name is unique: two conversions of
// one source writing the same cache entry at once (a settled build's settle.json) each rename
// their own file, where a fixed NAME.tmp let one rename the other's away ("no such file").
func WriteAtomic(path string, data []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	name := temporary.Name()
	_, err = temporary.Write(data)
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Chmod(name, 0o644)
	}
	if err == nil {
		// Windows refuses to replace a file another writer is replacing or a reader holds open:
		// fspath.Replace waits for it (decision 129).
		err = fspath.Replace(name, path)
	}
	if err != nil {
		_ = os.Remove(name)
	}
	return err
}
