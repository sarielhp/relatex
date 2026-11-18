package tryarxiv

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// MainFile is the main file arXiv states for the unpacked paper in dir: the one .tex file its
// 00README.json marks as top level. It is "" when there is no README, it names no file or
// several (a paper with its supplement), or the file is missing: relatex then chooses by its
// own rules, and asks when those leave it open.
func MainFile(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "00README.json"))
	if err != nil {
		return ""
	}
	var readme struct {
		Sources []struct {
			Usage    string `json:"usage"`
			Filename string `json:"filename"`
		} `json:"sources"`
	}
	if json.Unmarshal(data, &readme) != nil {
		return ""
	}
	names := []string{}
	for _, source := range readme.Sources {
		if source.Usage == "toplevel" {
			names = append(names, path.Clean(source.Filename))
		}
	}
	if len(names) != 1 || !strings.HasSuffix(strings.ToLower(names[0]), ".tex") || strings.HasPrefix(names[0], "../") {
		return ""
	}
	if info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(names[0]))); err != nil || !info.Mode().IsRegular() {
		return ""
	}
	return names[0]
}
