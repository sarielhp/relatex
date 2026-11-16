package pipeline

import (
	"bytes"
	"os"
	"path/filepath"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/legacyinput"
	"github.com/sarielhp/relatex/internal/rec"
)

// builtSourceFiles are the input's files as the builds' working copy holds them, which the source map
// and the SyncTeX output read: the archive's own files, except that a file the probe re-encoded to
// UTF-8 (decision 143) has the digest of the archive's bytes with the recorded re-encoding applied.
func (c *conversion) builtSourceFiles() []any {
	files := rec.FetchList(c.archive.Manifest, "files")
	data, err := os.ReadFile(filepath.Join(c.probe, "probe.json"))
	if err != nil {
		rec.Fail("%v", err)
	}
	report, err := canonjson.Decode(bytes.NewReader(data))
	if err != nil {
		rec.Fail("%v", err)
	}
	return reencodedFiles(files, rec.AsMap(report), func(path string) []byte {
		original, readErr := os.ReadFile(filepath.Join(c.archive.Directory, "files", filepath.FromSlash(path)))
		if readErr != nil {
			rec.Fail("%v", readErr)
		}
		return original
	})
}

// reencodedFiles are files (the input manifest's rows) with each re-encoded file's digest replaced by
// that of legacyinput.Apply on the archive's bytes, the record checked against the row's own digest. A
// record for a file the manifest does not list is refused.
func reencodedFiles(files []any, report rec.Map, read func(path string) []byte) []any {
	records := legacyinput.Records(report)
	if len(records) == 0 {
		return files
	}
	result := make([]any, 0, len(files))
	found := 0
	for _, value := range files {
		row := rec.AsMap(value)
		entry, ok := records[rec.ToS(row["path"])]
		if !ok || row["directory"] == true {
			result = append(result, value)
			continue
		}
		if rec.ToS(row["sha256"]) != rec.ToS(entry["sha256"]) {
			rec.Fail("Re-encoded input record mismatch")
		}
		changed := rec.Map{}
		for key, field := range row {
			changed[key] = field
		}
		changed["sha256"] = legacyinput.Digest(legacyinput.Apply(read(rec.ToS(row["path"])), entry, report))
		result = append(result, changed)
		found++
	}
	if found != len(records) {
		rec.Fail("Re-encoded input record mismatch")
	}
	return result
}
