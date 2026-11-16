package figscan

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
)

// pdfFile is the little of a PDF the Ipe reader needs: its objects by number (the last definition
// wins, as incremental updates intend). Objects inside object streams are not read: Ipe writes none.
type pdfFile struct {
	data    []byte
	objects map[int]int // object number -> offset of its body (after "obj")
}

var (
	objectHeader = regexp.MustCompile(`(?m)(?:^|[\s>])(\d+)\s+\d+\s+obj\b`)
	rootRef      = regexp.MustCompile(`/Root\s+(\d+)\s+\d+\s+R`)
	pagesRef     = regexp.MustCompile(`/Pages\s+(\d+)\s+\d+\s+R`)
	kidsArray    = regexp.MustCompile(`/Kids\s*\[([^\]]*)\]`)
	reference    = regexp.MustCompile(`(\d+)\s+\d+\s+R`)
	typeName     = regexp.MustCompile(`/Type\s*/(\w+)`)
	lengthValue  = regexp.MustCompile(`/Length\s+(\d+)(\s+\d+\s+R)?`)
	ipeType      = regexp.MustCompile(`/Type\s*/Ipe\b`)
	catalogType  = regexp.MustCompile(`/Type\s*/Catalog\b`)
)

// maxPageDepth bounds the page tree walk against cycles.
const maxPageDepth = 32

func parsePDF(data []byte) *pdfFile {
	file := &pdfFile{data: data, objects: map[int]int{}}
	for _, match := range objectHeader.FindAllSubmatchIndex(data, -1) {
		number, err := strconv.Atoi(string(data[match[2]:match[3]]))
		if err == nil {
			file.objects[number] = match[1]
		}
	}
	return file
}

// dictionary is the text of an object up to its stream or its end.
func (f *pdfFile) dictionary(number int) []byte {
	offset, ok := f.objects[number]
	if !ok {
		return nil
	}
	rest := f.data[offset:]
	end := len(rest)
	for _, marker := range [][]byte{[]byte("endobj"), []byte("stream")} {
		if at := bytes.Index(rest, marker); at >= 0 && at < end {
			end = at
		}
	}
	return rest[:end]
}

// ipeSource is the embedded Ipe XML, or nil when the PDF carries none.
func (f *pdfFile) ipeSource() ([]byte, error) {
	for _, number := range sortedKeys(f.objects) {
		dict := f.dictionary(number)
		if !ipeType.Match(dict) {
			continue
		}
		return f.streamData(number, dict)
	}
	return nil, nil
}

func (f *pdfFile) streamData(number int, dict []byte) ([]byte, error) {
	match := lengthValue.FindSubmatch(dict)
	if match == nil {
		return nil, fmt.Errorf("object %d: stream without /Length", number)
	}
	length, _ := strconv.Atoi(string(match[1]))
	if len(match[2]) > 0 {
		length = f.integerObject(length)
	}
	start := f.objects[number] + len(dict) + len("stream")
	if start < len(f.data) && f.data[start] == '\r' {
		start++
	}
	if start < len(f.data) && f.data[start] == '\n' {
		start++
	}
	if length <= 0 || start+length > len(f.data) {
		return nil, fmt.Errorf("object %d: stream length %d out of range", number, length)
	}
	raw := f.data[start : start+length]
	if !bytes.Contains(dict, []byte("/FlateDecode")) {
		return raw, nil
	}
	reader, err := zlib.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("object %d: %w", number, err)
	}
	defer reader.Close()
	out, err := io.ReadAll(reader)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, fmt.Errorf("object %d: %w", number, err)
	}
	return out, nil
}

func (f *pdfFile) integerObject(number int) int {
	value, _ := strconv.Atoi(string(bytes.TrimSpace(f.dictionary(number))))
	return value
}

// pageBoxes lists each page's visible box (CropBox, else MediaBox, inherited down the tree).
func (f *pdfFile) pageBoxes() []Box {
	root := f.catalog()
	match := pagesRef.FindSubmatch(f.dictionary(root))
	if match == nil {
		return nil
	}
	pages, _ := strconv.Atoi(string(match[1]))
	var boxes []Box
	f.walkPages(pages, Box{}, false, 0, &boxes)
	return boxes
}

func (f *pdfFile) catalog() int {
	if matches := rootRef.FindAllSubmatch(f.data, -1); len(matches) > 0 {
		number, _ := strconv.Atoi(string(matches[len(matches)-1][1]))
		return number
	}
	for _, number := range sortedKeys(f.objects) {
		if catalogType.Match(f.dictionary(number)) {
			return number
		}
	}
	return -1
}

func (f *pdfFile) walkPages(number int, inherited Box, haveBox bool, depth int, boxes *[]Box) {
	dict := f.dictionary(number)
	if dict == nil || depth > maxPageDepth {
		return
	}
	if box, ok := boxEntry(dict, "/MediaBox"); ok {
		inherited, haveBox = box, true
	}
	kind := typeName.FindSubmatch(dict)
	if kind != nil && string(kind[1]) == "Page" {
		box, ok := boxEntry(dict, "/CropBox")
		if ok && haveBox {
			// A viewer (and mutool) shows the crop box clipped to the media box.
			box, ok = intersect(box, inherited)
		}
		if !ok {
			box, ok = inherited, haveBox
		}
		if !ok {
			box = Box{X1: 612, Y1: 792}
		}
		*boxes = append(*boxes, box)
		return
	}
	kids := kidsArray.FindSubmatch(dict)
	if kids == nil {
		return
	}
	for _, ref := range reference.FindAllSubmatch(kids[1], -1) {
		kid, _ := strconv.Atoi(string(ref[1]))
		f.walkPages(kid, inherited, haveBox, depth+1, boxes)
	}
}

func boxEntry(dict []byte, key string) (Box, bool) {
	at := bytes.Index(dict, []byte(key))
	if at < 0 {
		return Box{}, false
	}
	rest := dict[at+len(key):]
	open, end := bytes.IndexByte(rest, '['), bytes.IndexByte(rest, ']')
	if open < 0 || end < open {
		return Box{}, false
	}
	values := numbers(string(rest[open+1 : end]))
	if len(values) != 4 {
		return Box{}, false
	}
	box := Box{X0: min(values[0], values[2]), Y0: min(values[1], values[3]),
		X1: max(values[0], values[2]), Y1: max(values[1], values[3])}
	return box, box.Width() > 0 && box.Height() > 0
}

// intersect is the common part of two boxes; false when they do not overlap.
func intersect(a, b Box) (Box, bool) {
	box := Box{X0: max(a.X0, b.X0), Y0: max(a.Y0, b.Y0), X1: min(a.X1, b.X1), Y1: min(a.Y1, b.Y1)}
	return box, box.Width() > 0 && box.Height() > 0
}
