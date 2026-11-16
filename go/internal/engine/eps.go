package engine

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/fspath"
	"github.com/sarielhp/relatex/internal/rec"
)

var epsName = regexp.MustCompile(`(?i)\.eps\z`)

// prepareEPS is SourceProbe#prepare_eps!: EPS figures converted once, before both builds,
// in the sandbox.
func (p *Probe) prepareEPS() {
	files := filepath.Join(p.Directory, "input", "files")
	output := filepath.Join(p.Directory, "eps")
	environment := map[string]string{"PATH": command.ToolPath(p.doctor.Engine),
		"TEXMFVAR": filepath.Join(output, "texmf-var"), "TEXMFCACHE": filepath.Join(output, "texmf-cache")}
	tools := map[string]string{"epstopdf": command.Sibling(p.doctor.Engine, "epstopdf"), "pdfinfo": command.Resolve("pdfinfo"),
		"qpdf": command.Resolve("qpdf")}
	run := func(tool string, arguments []string, name string) rec.Map {
		path := tools[tool]
		if path == "" || !command.IsExecutable(path) {
			rec.Fail("EPS figures need %s", tool)
		}
		wrapped := p.doctor.Wrap(append([]string{path}, arguments...), files, output, environment)
		return command.Run(wrapped, files, filepath.Join(output, name+".log"), p.remaining(), environment)
	}
	assets := prepareEPSAssets(files, run)
	if len(assets) > 0 {
		writeJSON(filepath.Join(p.Directory, "eps-assets.json"), assets)
	}
}

// prepareEPSAssets is EPSConversion.prepare!: each EPS figure becomes FILE-eps-converted-to.pdf,
// plus FILE.pdf unless the paper supplies one; a failed conversion leaves the EPS.
func prepareEPSAssets(source string, run func(tool string, arguments []string, name string) rec.Map) []any {
	assets := []any{}
	for _, path := range epsFiles(source) {
		converted := epsName.ReplaceAllString(path, "-eps-converted-to.pdf")
		if !exists(converted) {
			if failure := convertEPS(path, converted, len(assets)+1, run); failure != "" {
				entry := asset(source, path, "input")
				entry["conversion"] = failure + "; control determines whether this asset is required"
				assets = append(assets, entry)
				continue
			}
			assets = append(assets, merged(asset(source, path, "input"), asset(source, converted, "output")))
		}
		plain := epsName.ReplaceAllString(path, ".pdf")
		if exists(plain) {
			continue
		}
		check(copyFile(converted, plain))
		entry := merged(asset(source, converted, "input"), asset(source, plain, "output"))
		entry["conversion"] = "copy of standard converted asset; supplied PDFs preserved"
		assets = append(assets, entry)
	}
	return assets
}

func epsFiles(source string) []string {
	found := []string{}
	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != source && strings.HasPrefix(entry.Name(), ".") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if epsName.MatchString(path) && isFile(path) {
			found = append(found, path)
		}
		return nil
	})
	check(err)
	slices.Sort(found)
	return found
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// convertEPS is EPSConversion.convert: "" on success, else why; a failed conversion removes
// its partial output.
func convertEPS(path, converted string, index int, run func(string, []string, string) rec.Map) string {
	failure := ""
	stage := run("epstopdf", []string{"--outfile=" + converted, path}, "eps-"+itoa(index))
	if stage["timeout"] == true || !rec.Equal(stage["exit"], int64(0)) {
		failure = "failed"
	}
	if failure == "" {
		failure = normalizeEPS(converted, index, run)
	}
	if failure == "" {
		info := run("pdfinfo", []string{converted}, "eps-info-"+itoa(index))
		if !succeeded(info) {
			failure = "unreadable output"
		}
	}
	if failure != "" {
		_ = os.Remove(converted)
	}
	return failure
}

// normalizeEPS: Ghostscript dates the PDF it writes and gives it a random ID, and the ICC
// profile it makes for a CIE colour space (R's sRGB plots) keeps whatever its memory held after
// a text tag's terminator. Its pages expanded, those bytes cleared, and copied into an empty
// document with an ID derived from the content are the same bytes in every run. "" on success,
// else why the output could not be read.
func normalizeEPS(converted string, index int, run func(string, []string, string) rec.Map) string {
	expanded := converted + ".expanded"
	normalized := converted + ".normalized"
	failure := copyPages([]string{"--qdf", "--object-streams=disable"}, converted, expanded, "eps-expand-"+itoa(index), run)
	if failure == "" {
		check(clearProfilePadding(expanded))
		failure = copyPages([]string{"--deterministic-id"}, expanded, normalized, "eps-normalize-"+itoa(index), run)
	}
	_ = os.Remove(expanded)
	if failure != "" {
		_ = os.Remove(normalized)
		return failure
	}
	check(os.Rename(normalized, converted))
	return ""
}

// copyPages copies input's pages into output with qpdf (exit code 3 is success with warnings).
func copyPages(options []string, input, output, name string, run func(string, []string, string) rec.Map) string {
	stage := run("qpdf", append(slices.Clone(options), "--empty", "--pages", input, "--", output), name)
	exit, _ := stage["exit"].(int64)
	if stage["timeout"] == true || stage["exit"] == nil || (exit != 0 && exit != 3) || !isFile(output) {
		return "unreadable output"
	}
	return ""
}

var streamStart = regexp.MustCompile(`stream\r?\n`)

// clearProfilePadding clears, in each ICC profile of an uncompressed PDF (a stream whose data
// has the signature acsp at byte 36), the bytes of every text tag after its terminator.
func clearProfilePadding(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for _, match := range streamStart.FindAllIndex(data, -1) {
		clearTextTags(data, match[1])
	}
	return os.WriteFile(path, data, 0o644)
}

func clearTextTags(data []byte, start int) {
	if len(data) < start+132 || string(data[start+36:start+40]) != "acsp" {
		return
	}
	size := int(binary.BigEndian.Uint32(data[start:]))
	count := int(binary.BigEndian.Uint32(data[start+128:]))
	if start+size > len(data) || 132+12*count > size {
		return
	}
	for tag := range count {
		entry := data[start+132+12*tag:]
		offset, length := int(binary.BigEndian.Uint32(entry[4:])), int(binary.BigEndian.Uint32(entry[8:]))
		if length <= 8 || offset+length > size || string(data[start+offset:start+offset+4]) != "text" {
			continue
		}
		text := data[start+offset+8 : start+offset+length]
		if terminator := bytes.IndexByte(text, 0); terminator >= 0 {
			clear(text[terminator:])
		}
	}
}

func itoa(value int) string { return rec.ToS(int64(value)) }

// asset is EPSConversion.asset, its keys named for the side ("input" or "output").
func asset(source, path, side string) rec.Map {
	return rec.Map{side: fspath.Name(source, path), side + "_sha256": command.FileDigest(path)}
}

func merged(left, right rec.Map) rec.Map {
	result := rec.Map{}
	for _, part := range []rec.Map{left, right} {
		for key, value := range part {
			result[key] = value
		}
	}
	return result
}
