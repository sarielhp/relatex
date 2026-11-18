// Package input is the Go port of the 01-input stage: Relatex::Archive (a ZIP checked and
// extracted into a fresh snapshot with its inventory) and SourceSnapshot (a paper directory
// zipped first, as rubyzip zips it).
package input

import (
	"bytes"
	"compress/flate"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"golang.org/x/text/unicode/norm"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/rec"
)

// Limits are Archive::LIMITS.
var Limits = map[string]int64{"entries": 100_000, "zip_bytes": 256 * 1024 * 1024, "expanded_bytes": 2 * 1024 * 1024 * 1024,
	"entry_bytes": 256 * 1024 * 1024, "depth": 64}

// Companions is Archive::COMPANIONS: files a paper keeps beside its main file, never the main
// file.
var Companions = []string{"prefix.tex", "prelim.tex"}

var (
	// A colon is fine inside a name (def_median:estimator.tex, 2026-10-30); only a drive prefix is unsafe.
	unsafeName = regexp.MustCompile(`[\\\x00-\x1f\x7f]|^[A-Za-z]:`)
	// beginsDocument is /^[^%]*\\begin\s*\{document\}/n: no % before the \begin on its line.
	beginsDocument = regexp.MustCompile(`(?m)^[^%\n]*\\begin[ \t\n\v\f\r]*\{document\}`)
)

// Archive is one checked snapshot: Directory holds source.zip, files/ and inventory.json.
type Archive struct {
	Directory string
	Manifest  rec.Map
	expanded  int64
}

// centralEntry is a central directory record, as rubyzip reads it.
type centralEntry struct {
	name            string
	madeBy          uint16
	flags           uint16
	method          uint16
	crc             uint32
	compressedSize  int64
	size            int64
	externalAttrs   uint32
	localOffset     int64
	directory       bool
	symlink         bool
	regularFileType bool
}

// NewArchive is Archive.new(path, directory:): the ZIP copied (within the byte limit),
// checked entry by entry and extracted, with its inventory.
func NewArchive(path, directory string) *Archive {
	absolute, err := filepath.Abs(directory)
	check(err)
	a := &Archive{Directory: absolute}
	if _, statErr := os.Lstat(absolute); statErr == nil {
		rec.Fail("Archive snapshot already exists")
	}
	check(os.Mkdir(absolute, 0o700))
	snapshot := filepath.Join(absolute, "source.zip")
	copyLimited(path, snapshot, Limits["zip_bytes"]+1)
	info, err := os.Stat(snapshot)
	check(err)
	if info.Size() > Limits["zip_bytes"] {
		rec.Fail("ZIP byte limit exceeded")
	}
	limits := rec.Map{}
	for key, value := range Limits {
		limits[key] = value
	}
	a.Manifest = rec.Map{"zip_sha256": fileDigest(snapshot), "zip_bytes": info.Size(), "zip_reader": "go " + runtime.Version(),
		"limits": limits, "files": []any{}}
	a.extract(snapshot)
	a.write()
	return a
}

func (a *Archive) write() {
	check(canonjson.WriteFile(filepath.Join(a.Directory, "inventory.json"), a.Manifest))
}

func copyLimited(from, to string, limit int64) {
	input, err := os.Open(from)
	check(err)
	defer input.Close()
	output, err := os.Create(to)
	check(err)
	_, err = io.Copy(output, io.LimitReader(input, limit))
	check(errors.Join(err, output.Close()))
}

func fileDigest(path string) string {
	data, err := os.ReadFile(path)
	check(err)
	return digest(data)
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func check(err error) {
	if err != nil {
		rec.Fail("%v", err)
	}
}

func invalidZIP(format string, args ...any) {
	rec.Fail("Invalid ZIP: "+format, args...)
}

func (a *Archive) extract(snapshot string) {
	data, err := os.ReadFile(snapshot)
	check(err)
	entries := centralDirectory(data)
	// rubyzip reads a name with its backslashes as slashes and keys its entries by name without
	// a trailing slash: a repeated key is one entry, a rewritten name a normalized one.
	names := map[string]bool{}
	for _, entry := range entries {
		key := strings.TrimSuffix(strings.ReplaceAll(entry.name, `\`, "/"), "/")
		if names[key] {
			rec.Fail("Duplicate or incomplete ZIP directory")
		}
		names[key] = true
	}
	for _, entry := range entries {
		if strings.Contains(entry.name, `\`) {
			rec.Fail("ZIP filenames were normalized by the reader")
		}
	}
	a.inventory(entries)
	for _, entry := range entries {
		verifyLocalHeader(data, entry)
	}
	check(os.MkdirAll(filepath.Join(a.Directory, "files"), 0o755))
	for _, entry := range entries {
		a.extractEntry(data, entry)
	}
}

// centralDirectory is Archive#entry_count and #directory_names! with what rubyzip reads of
// each record.
func centralDirectory(data []byte) []centralEntry {
	tail := data[max(len(data)-65_557, 0):]
	offset := bytes.LastIndex(tail, []byte("PK\x05\x06"))
	if offset < 0 || len(tail)-offset < 22 {
		rec.Fail("Truncated or missing ZIP end record")
	}
	record := tail[offset : offset+22]
	if offset+22+int(binary.LittleEndian.Uint16(record[20:])) != len(tail) {
		rec.Fail("Truncated ZIP comment or trailing data")
	}
	disk, startDisk := binary.LittleEndian.Uint16(record[4:]), binary.LittleEndian.Uint16(record[6:])
	onDisk, count := binary.LittleEndian.Uint16(record[8:]), binary.LittleEndian.Uint16(record[10:])
	size, start := binary.LittleEndian.Uint32(record[12:]), binary.LittleEndian.Uint32(record[16:])
	if disk != 0 || startDisk != 0 || onDisk != count {
		rec.Fail("Multi-volume ZIP is unsupported")
	}
	if count == 0xffff || size == 0xffffffff || start == 0xffffffff {
		rec.Fail("ZIP64 central directory is outside the initial archive profile")
	}
	if int64(count) > Limits["entries"] {
		rec.Fail("ZIP entry limit exceeded")
	}
	if int64(start)+int64(size) > int64(len(data)) {
		rec.Fail("ZIP central directory exceeds archive")
	}
	return centralEntries(data, int(start), int(size), int(count))
}

func centralEntries(data []byte, start, size, count int) []centralEntry {
	position := start
	entries := make([]centralEntry, 0, count)
	for range count {
		if position+46 > len(data) || !bytes.HasPrefix(data[position:], []byte("PK\x01\x02")) {
			rec.Fail("Invalid ZIP central directory header")
		}
		header := data[position : position+46]
		nameSize := int(binary.LittleEndian.Uint16(header[28:]))
		extraSize, commentSize := int(binary.LittleEndian.Uint16(header[30:])), int(binary.LittleEndian.Uint16(header[32:]))
		if nameSize > 4096 {
			rec.Fail("ZIP filename size limit")
		}
		nameEnd := min(position+46+nameSize, len(data))
		entry := newCentralEntry(header, string(data[position+46:nameEnd]))
		extraEnd := min(nameEnd+extraSize, len(data))
		entries = append(entries, withZIP64(entry, header, data[nameEnd:extraEnd]))
		position += 46 + nameSize + extraSize + commentSize
	}
	if position != start+size {
		rec.Fail("ZIP central directory size/count mismatch")
	}
	return entries
}

// newCentralEntry reads rubyzip's view of a record: its file type comes from the Unix mode
// when a Unix tool made it, else from the trailing slash of its name.
func newCentralEntry(header []byte, name string) centralEntry {
	entry := centralEntry{name: name, madeBy: binary.LittleEndian.Uint16(header[4:]), flags: binary.LittleEndian.Uint16(header[8:]),
		method: binary.LittleEndian.Uint16(header[10:]), crc: binary.LittleEndian.Uint32(header[16:]),
		compressedSize: int64(binary.LittleEndian.Uint32(header[20:])), size: int64(binary.LittleEndian.Uint32(header[24:])),
		externalAttrs: binary.LittleEndian.Uint32(header[38:]), localOffset: int64(binary.LittleEndian.Uint32(header[42:]))}
	byName := strings.HasSuffix(name, "/")
	entry.directory, entry.regularFileType = byName, true
	if entry.madeBy>>8 != 3 {
		return entry
	}
	switch kind := entry.externalAttrs >> 28; kind {
	case 0o04:
		entry.directory = true
	case 0o10:
		entry.directory = false
	case 0o12:
		entry.directory, entry.symlink = false, true
	default:
		entry.regularFileType = kind == 0
	}
	return entry
}

// withZIP64 reads the ZIP64 extra field (0x0001) as rubyzip does for a central record: the
// 64-bit size, compressed size and header offset, each present when its 32-bit field is full.
func withZIP64(entry centralEntry, header, extra []byte) centralEntry {
	for len(extra) >= 4 {
		tag, size := binary.LittleEndian.Uint16(extra), int(binary.LittleEndian.Uint16(extra[2:]))
		if 4+size > len(extra) {
			break
		}
		if tag == 1 {
			return zip64Fields(entry, header, extra[4:4+size])
		}
		extra = extra[4+size:]
	}
	return entry
}

func zip64Fields(entry centralEntry, header, field []byte) centralEntry {
	next := func(present bool, value int64) int64 {
		if !present || len(field) < 8 {
			return value
		}
		read := int64(binary.LittleEndian.Uint64(field))
		field = field[8:]
		return read
	}
	entry.size = next(binary.LittleEndian.Uint32(header[24:]) == 0xffffffff, entry.size)
	entry.compressedSize = next(binary.LittleEndian.Uint32(header[20:]) == 0xffffffff, entry.compressedSize)
	entry.localOffset = next(binary.LittleEndian.Uint32(header[42:]) == 0xffffffff, entry.localOffset)
	return entry
}

// checkedName is Archive#checked_name.
func checkedName(entry centralEntry) string {
	name := entry.name
	if !utf8.ValidString(name) {
		rec.Fail("Invalid ZIP filename encoding")
	}
	path := strings.TrimSuffix(name, "/")
	parts := []string{}
	if path != "" {
		parts = strings.Split(path, "/")
	}
	if unsafeName.MatchString(name) || slices.ContainsFunc(parts, func(part string) bool { return part == "" || part == "." || part == ".." }) {
		rec.Fail("Unsafe ZIP path: %s", rec.InspectString(name))
	}
	refuseUnstorable(name, parts)
	if len(name) > 4096 || int64(len(parts)) > Limits["depth"] {
		rec.Fail("ZIP path limit exceeded")
	}
	if entry.symlink || !entry.regularFileType {
		rec.Fail("ZIP symlinks and special files are forbidden")
	}
	if entry.flags&1 != 0 {
		rec.Fail("Encrypted ZIP entries are unsupported")
	}
	if entry.method != 0 && entry.method != 8 {
		rec.Fail("Unsupported ZIP compression")
	}
	if entry.size > Limits["entry_bytes"] {
		rec.Fail("ZIP entry byte limit exceeded")
	}
	if entry.directory && entry.size > 0 {
		rec.Fail("ZIP directory contains data")
	}
	if entry.directory != strings.HasSuffix(name, "/") {
		rec.Fail("ZIP directory type/name mismatch")
	}
	return path
}

// inventory is Archive#inventory!: safe names, no NFC-equal duplicates (names differing only in letter case are distinct files, as on Linux where LuaLaTeX reads both), no file under a
// file, the expanded size within the limit.
func (a *Archive) inventory(entries []centralEntry) {
	paths := map[string]bool{}
	keys := []string{}
	total := int64(0)
	for _, entry := range entries {
		path := checkedName(entry)
		key := norm.NFC.String(path)
		if _, seen := paths[key]; seen {
			rec.Fail("Colliding ZIP path: %s", path)
		}
		paths[key] = entry.directory
		keys = append(keys, key)
		total += entry.size
	}
	for _, key := range keys {
		parts := strings.Split(key, "/")
		for length := 1; length < len(parts); length++ {
			if directory, ok := paths[strings.Join(parts[:length], "/")]; ok && !directory {
				rec.Fail("ZIP file/directory collision: %s", key)
			}
		}
	}
	if total > Limits["expanded_bytes"] {
		rec.Fail("Expanded ZIP byte limit exceeded")
	}
}

func verifyLocalHeader(data []byte, entry centralEntry) {
	offset := entry.localOffset
	if offset+30 > int64(len(data)) || !bytes.HasPrefix(data[offset:], []byte("PK\x03\x04")) {
		rec.Fail("Truncated ZIP local header")
	}
	header := data[offset : offset+30]
	nameSize := int64(binary.LittleEndian.Uint16(header[26:]))
	name := data[offset+30 : min(offset+30+nameSize, int64(len(data)))]
	if string(name) != entry.name || binary.LittleEndian.Uint16(header[8:]) != entry.method {
		rec.Fail("ZIP local/central header mismatch or normalized filename")
	}
}

// entryData is the entry's stored bytes after its local header.
func entryData(data []byte, entry centralEntry) io.Reader {
	header := data[entry.localOffset : entry.localOffset+30]
	start := entry.localOffset + 30 + int64(binary.LittleEndian.Uint16(header[26:])) + int64(binary.LittleEndian.Uint16(header[28:]))
	// A stored entry is read for its declared size (rubyzip's PassThruDecompressor), whatever
	// its compressed size says; a deflated one until its stream ends.
	if entry.method == 0 {
		return bytes.NewReader(data[min(start, int64(len(data))):min(start+entry.size, int64(len(data)))])
	}
	end := start + entry.compressedSize
	if end > int64(len(data)) {
		invalidZIP("entry data exceeds the archive")
	}
	return flate.NewReader(bytes.NewReader(data[start:end]))
}

func (a *Archive) extractEntry(data []byte, entry centralEntry) {
	path := checkedName(entry)
	target := filepath.Join(a.Directory, "files", path)
	files := rec.FetchList(a.Manifest, "files")
	if entry.directory {
		check(os.MkdirAll(target, 0o755))
		a.Manifest["files"] = append(files, rec.Map{"path": path, "directory": true, "bytes": int64(0)})
		return
	}
	check(os.MkdirAll(filepath.Dir(target), 0o755))
	output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	check(err)
	written := a.copyEntry(data, entry, output)
	check(output.Close())
	content, err := os.ReadFile(target)
	check(err)
	a.Manifest["files"] = append(files, rec.Map{"path": path, "directory": false, "bytes": written, "sha256": digest(content)})
}

// copyEntry is Archive#copy_entry: the inflated bytes within the declared size and the
// limits, with the declared CRC.
func (a *Archive) copyEntry(data []byte, entry centralEntry, output io.Writer) int64 {
	reader := entryData(data, entry)
	buffer := make([]byte, 65_536)
	size := int64(0)
	crc := uint32(0)
	for {
		count, err := reader.Read(buffer)
		if count > 0 {
			size += int64(count)
			a.expanded += int64(count)
			if size > entry.size || size > Limits["entry_bytes"] || a.expanded > Limits["expanded_bytes"] {
				rec.Fail("Actual ZIP expansion exceeds declared size or limits")
			}
			crc = crc32.Update(crc, crc32.IEEETable, buffer[:count])
			_, writeErr := output.Write(buffer[:count])
			check(writeErr)
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			invalidZIP("%v", err)
		}
	}
	if size != entry.size || crc != entry.crc {
		rec.Fail("ZIP size or CRC mismatch")
	}
	return size
}

var lower = cases.Lower(language.Und)

// SelectMain is Archive#select_main: the requested main file; else the only top-level .tex
// file besides the ignored ones; else the only .tex file that begins a document.
func (a *Archive) SelectMain(requested string, ignore []string) string {
	ignored := make([]string, len(ignore))
	for index, name := range ignore {
		ignored[index] = lower.String(name)
	}
	files := []string{}
	for _, value := range rec.FetchList(a.Manifest, "files") {
		file := rec.AsMap(value)
		if file["directory"] != true {
			files = append(files, rec.FetchString(file, "path"))
		}
	}
	topLevel, candidates := []string{}, []any{}
	for _, path := range files {
		lowered := lower.String(path)
		if !strings.Contains(path, "/") && strings.HasSuffix(lowered, ".tex") && !slices.Contains(ignored, lowered) {
			topLevel = append(topLevel, path)
		}
		if strings.HasSuffix(lowered, ".tex") && !slices.Contains(ignored, lower.String(filepath.Base(path))) && a.beginsDocument(path) {
			candidates = append(candidates, path)
		}
	}
	selected := chooseMain(requested, topLevel, candidates)
	narrowed, reason := "", ""
	pool := anyStrings(candidates)
	if selected == "" && len(candidates) > 1 {
		narrowed, reason, pool = a.narrowMain(files, pool, ignored)
		selected = narrowed
	}
	besides := ""
	if len(ignore) > 0 {
		besides = " besides " + strings.Join(ignore, ", ")
	}
	if selected == "" {
		listed := topLevel
		if len(candidates) > 0 {
			listed = pool
		}
		rec.Fail("%s", ambiguousMain(listed, besides))
	}
	if !slices.Contains(files, selected) || !strings.HasSuffix(lower.String(selected), ".tex") {
		rec.Fail("Requested main is outside the archive, missing, or not a .tex file")
	}
	selection := "unique textual candidate; not compilation evidence"
	if requested != "" {
		selection = "explicit"
	} else if len(topLevel) == 1 {
		selection = "only top-level .tex file" + besides
	} else if narrowed != "" {
		selection = reason + " (several documents); not compilation evidence"
	}
	a.Manifest["main"], a.Manifest["main_candidates"], a.Manifest["selection"] = selected, candidates, selection
	a.write()
	return selected
}

func chooseMain(requested string, topLevel []string, candidates []any) string {
	switch {
	case requested != "":
		return requested
	case len(topLevel) == 1:
		return topLevel[0]
	case len(candidates) == 1:
		return rec.AsString(candidates[0])
	}
	return ""
}

func anyStrings(values []any) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = rec.AsString(value)
	}
	return result
}

func (a *Archive) beginsDocument(path string) bool {
	text, err := os.ReadFile(filepath.Join(a.Directory, "files", path))
	check(err)
	return beginsDocument.Match(text)
}

// ambiguousMain is Archive#ambiguous_main: which files could be the main one and how to choose.
func ambiguousMain(candidates []string, besides string) string {
	if len(candidates) == 0 {
		return fmt.Sprintf("No .tex file%s begins a document; choose the main file with --main FILE", besides)
	}
	sorted := slices.Sorted(slices.Values(candidates))
	var builder strings.Builder
	builder.WriteString("Cannot tell which file is the main one. Candidates:\n")
	for _, path := range sorted {
		builder.WriteString("  " + path + "\n")
	}
	builder.WriteString("Choose one with --main FILE, or list the others under main.ignore in relatex/config.jsonc")
	return builder.String()
}
