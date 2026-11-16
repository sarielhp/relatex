//go:build windows

package cli

import "golang.org/x/sys/windows/registry"

// platformNotes are doctor's lines for Windows: whether long paths are enabled. relatex itself
// handles long paths, but TeX and the PDF tools stop at 260 characters without the setting, and a
// deep project folder with a long cache path can pass it.
func platformNotes() []string {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\FileSystem`, registry.QUERY_VALUE)
	if err != nil {
		return []string{"Long paths  unknown (" + err.Error() + ")"}
	}
	defer key.Close()
	if value, _, err := key.GetIntegerValue("LongPathsEnabled"); err == nil && value == 1 {
		return []string{"Long paths  enabled"}
	}
	return []string{"Long paths  disabled: TeX and the PDF tools stop at 260 characters; enable LongPathsEnabled " +
		"(HKLM\\SYSTEM\\CurrentControlSet\\Control\\FileSystem) or keep the project and cache paths short"}
}
