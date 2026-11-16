package input

import (
	"regexp"
	"runtime"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

// storableOnly refuses names this platform's file system cannot store (decision 129, D9): on
// Windows, the names NTFS refuses. Unix stores every name the archive check lets through.
var storableOnly = runtime.GOOS == "windows"

var (
	// ntfsCharacters are the characters NTFS refuses in a name (the control characters and "\"
	// are refused everywhere already).
	ntfsCharacters = regexp.MustCompile(`[<>:"|?*]`)
	// reservedDevice is a Windows device name, with any extension: aux.tex is the device AUX.
	reservedDevice = regexp.MustCompile(`(?i)\A(?:CON|PRN|AUX|NUL|COM[1-9¹²³]|LPT[1-9¹²³])(?:\.|\z)`)
)

// unstorable is why NTFS cannot store a name (one part of a path), or "" when it can. Nothing
// is renamed: a renamed file is not the file TeX would read (D9).
func unstorable(part string) string {
	switch {
	case ntfsCharacters.MatchString(part):
		return `it contains one of < > : " | ? *`
	case strings.HasSuffix(part, ".") || strings.HasSuffix(part, " "):
		return "it ends with a dot or a space"
	case reservedDevice.MatchString(part):
		return "it is a reserved device name (CON, PRN, AUX, NUL, COM1-9, LPT1-9, with any extension)"
	}
	return ""
}

// refuseUnstorable fails with input.unsafe, naming the file, when this platform cannot store one
// of the path's parts.
func refuseUnstorable(name string, parts []string) {
	if !storableOnly {
		return
	}
	for _, part := range parts {
		if reason := unstorable(part); reason != "" {
			rec.Fail("input.unsafe: %s cannot be stored on Windows: %s; rename it in the source", rec.InspectString(name), reason)
		}
	}
}
