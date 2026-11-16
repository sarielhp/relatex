//go:build windows

package fspath

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// Real is the path with every link resolved. filepath.EvalSymlinks fails on a path through a
// directory junction (scoop's apps\NAME\current is one) with a bare "cannot find the path"; then
// the file system names the file itself: the final path of an open handle (decision 129).
func Real(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}
	if _, statErr := os.Stat(path); statErr != nil {
		return "", statErr
	}
	if final, finalErr := finalPath(path); finalErr == nil {
		return final, nil
	}
	return "", &os.PathError{Op: "realpath", Path: path, Err: err}
}

// finalPath is GetFinalPathNameByHandle of path opened for no access (a directory too), without
// the \\?\ prefix.
func finalPath(path string) (string, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	handle, err := windows.CreateFile(name, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(handle)
	buffer := make([]uint16, windows.MAX_LONG_PATH)
	length, err := windows.GetFinalPathNameByHandle(handle, &buffer[0], uint32(len(buffer)), 0)
	if err != nil {
		return "", err
	}
	final := windows.UTF16ToString(buffer[:length])
	if rest, ok := strings.CutPrefix(final, `\\?\UNC\`); ok {
		return `\\` + rest, nil
	}
	return strings.TrimPrefix(final, `\\?\`), nil
}
