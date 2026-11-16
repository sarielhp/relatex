package synctexout

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
	"github.com/sarielhp/relatex/internal/zgzip"
)

// synctexMaxBytes is SynctexFile::MAX_BYTES.
const synctexMaxBytes = 128 * 1024 * 1024

var (
	inputRecord   = regexp.MustCompile(`\AInput:(\d+):([^\r\n\x00]+)\n\z`)
	anchorRecord  = regexp.MustCompile(`\A!(\d+)\n\z`)
	unsafeSynctex = regexp.MustCompile(`[\r\n\x00]`)
)

// SynctexFile is the bounded version-1 SyncTeX adapter: only Input records and byte anchors
// change; native geometry and opaque extension records are retained verbatim.
type SynctexFile struct {
	lines  [][]byte
	inputs map[int64]string
}

// splitLines is String#lines: each line with its newline.
func splitLines(data []byte) [][]byte {
	lines := [][]byte{}
	for len(data) > 0 {
		index := bytes.IndexByte(data, '\n')
		if index < 0 {
			lines = append(lines, data)
			break
		}
		lines = append(lines, data[:index+1])
		data = data[index+1:]
	}
	return lines
}

// NewSynctexFile is SynctexFile.new(bytes).
func NewSynctexFile(data []byte) *SynctexFile {
	if len(data) > synctexMaxBytes {
		rec.Fail("SyncTeX exceeds expanded limit")
	}
	if !bytes.HasPrefix(data, []byte("SyncTeX Version:1\n")) {
		rec.Fail("Unsupported SyncTeX version")
	}
	s := &SynctexFile{lines: splitLines(data), inputs: map[int64]string{}}
	content, postamble, count := false, false, false
	for _, line := range s.lines {
		content = content || string(line) == "Content:\n"
		postamble = postamble || string(line) == "Postamble:\n"
		count = count || bytes.HasPrefix(line, []byte("Count:"))
	}
	if !content || !postamble || !count {
		rec.Fail("Incomplete SyncTeX stream")
	}
	for _, line := range s.lines {
		if !bytes.HasPrefix(line, []byte("Input:")) {
			continue
		}
		match := inputRecord.FindSubmatch(line)
		if match == nil {
			rec.Fail("Malformed SyncTeX Input record")
		}
		tag := decimal(match[1])
		if known, ok := s.inputs[tag]; ok && known != string(match[2]) {
			rec.Fail("Conflicting SyncTeX source tag")
		}
		s.inputs[tag] = string(match[2])
	}
	s.validateAnchors()
	return s
}

// decimal is Integer(text, 10) on a run of digits.
func decimal(digits []byte) int64 {
	value, err := strconv.ParseInt(string(digits), 10, 64)
	if err != nil {
		rec.Fail("invalid value for Integer(): %q", digits)
	}
	return value
}

// ReadSynctex is SynctexFile.read: a .synctex or .synctex.gz file.
func ReadSynctex(path string) *SynctexFile {
	info, err := os.Stat(path)
	check(err)
	if info.Size() > synctexMaxBytes {
		rec.Fail("SyncTeX compressed input exceeds limit")
	}
	data, err := os.ReadFile(path)
	check(err)
	if strings.HasSuffix(path, ".gz") {
		data, err = zgzip.Decompress(data, synctexMaxBytes)
		if err != nil {
			rec.Fail("Invalid compressed SyncTeX: %v", err)
		}
	}
	return NewSynctexFile(data)
}

// Retarget is SynctexFile#retarget: each Input record's file replaced by what replace
// returns for its tag ("" keeps it), the byte anchors recomputed.
func (s *SynctexFile) Retarget(replace func(tag int64, path string) string) *SynctexFile {
	records := make([][]byte, len(s.lines))
	for index, line := range s.lines {
		match := inputRecord.FindSubmatch(line)
		if match == nil {
			records[index] = line
			continue
		}
		replacement := replace(decimal(match[1]), string(match[2]))
		if replacement == "" {
			replacement = string(match[2])
		}
		if unsafeSynctex.MatchString(replacement) {
			rec.Fail("Unsafe SyncTeX source filename")
		}
		records[index] = []byte(fmt.Sprintf("Input:%s:%s\n", match[1], replacement))
	}
	return NewSynctexFile(synctexAnchors(records))
}

// synctexAnchors is SynctexFile.anchors: the records joined, each byte anchor the distance
// from the previous one.
func synctexAnchors(records [][]byte) []byte {
	var output bytes.Buffer
	previous := 0
	for _, line := range records {
		if anchorRecord.Match(line) {
			offset := output.Len()
			fmt.Fprintf(&output, "!%d\n", offset-previous)
			previous = offset
			continue
		}
		output.Write(line)
	}
	return output.Bytes()
}

// Write is SynctexFile#write: the file (gzip-compressed for .gz) and its report.
func (s *SynctexFile) Write(path string) Ordered {
	if _, err := os.Lstat(path); err == nil {
		rec.Fail("SyncTeX destination already exists")
	}
	data := bytes.Join(s.lines, nil)
	if strings.HasSuffix(path, ".gz") {
		var err error
		data, err = zgzip.Compress(data)
		check(err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	check(err)
	_, err = file.Write(data)
	check(err)
	check(file.Close())
	return Ordered{{"file", filepath.Base(path)}, {"sha256", hexSum(data)}, {"input_records", int64(len(s.inputs))}}
}

func (s *SynctexFile) validateAnchors() {
	offset, previous := 0, 0
	for _, line := range s.lines {
		if match := anchorRecord.FindSubmatch(line); match != nil {
			if decimal(match[1]) != int64(offset-previous) {
				rec.Fail("SyncTeX byte anchor differs from record positions")
			}
			previous = offset
		}
		offset += len(line)
	}
}
