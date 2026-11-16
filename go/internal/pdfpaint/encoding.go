package pdfpaint

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/fontbinding"
	"github.com/sarielhp/relatex/internal/paintmodel"
	"github.com/sarielhp/relatex/internal/rec"
)

// objects is qpdf's JSON object inventory, in the order qpdf writes it. Values are decoded
// when first read: most of an inventory is image streams no font check looks at.
type objects = *inventoryObjects

type inventoryObjects struct {
	keys    []string
	raw     map[string]json.RawMessage
	decoded map[string]any
}

// Keys are the object references in qpdf's order.
func (o *inventoryObjects) Keys() []string { return o.keys }

// Get is the decoded object.
func (o *inventoryObjects) Get(key string) (any, bool) {
	if value, ok := o.decoded[key]; ok {
		return value, true
	}
	raw, ok := o.raw[key]
	if !ok {
		return nil, false
	}
	value, err := canonjson.Decode(bytes.NewReader(raw))
	if err != nil {
		rec.Fail("%v", err)
	}
	o.decoded[key] = value
	return value, true
}

// mentions reports whether the object's JSON holds text, without decoding it.
func (o *inventoryObjects) mentions(key string, text []byte) bool {
	return bytes.Contains(o.raw[key], text)
}

// fontInventory is PDFFontEncoding#capture_inventory: the ordinary PDF's objects, decoded
// by qpdf, with the identities of the PDF, the helper and the inventory.
func fontInventory(pdf, directory string) (rec.Map, objects) {
	if _, err := os.Lstat(directory); err == nil {
		rec.Fail("Font encoding output exists")
	}
	mkdir(directory)
	tool := command.Resolve("qpdf")
	if tool == "" {
		rec.Fail("PDF font encoding helper unavailable")
	}
	source, helper := command.FileDigest(pdf), command.FileDigest(tool)
	path := filepath.Join(directory, "ordinary.json")
	stage := command.Run([]string{tool, "--json", "--json-stream-data=inline", "--decode-level=all", "--json-key=qpdf", pdf, path},
		directory, filepath.Join(directory, "qpdf.log"), 60*time.Second, nil)
	fontbinding.WriteJSON(filepath.Join(directory, "stage.json"), stage)
	exit, _ := stage["exit"].(int64)
	if stage["exit"] == nil || (exit != 0 && exit != 3) || stage["timeout"] == true {
		rec.Fail("PDF font encoding extraction failed")
	}
	inventory := readObjects(path)
	if source != command.FileDigest(pdf) || helper != command.FileDigest(tool) {
		rec.Fail("PDF font encoding source or helper changed")
	}
	return rec.Map{"pdf_sha256": source, "helper_sha256": helper, "inventory_sha256": command.FileDigest(path)}, inventory
}

func mkdir(directory string) {
	if err := os.MkdirAll(directory, 0o755); err != nil {
		rec.Fail("%v", err)
	}
}

// readObjects is JSON.parse(File.read(path)).fetch('qpdf').last, keeping the objects'
// order. A byte scanner finds each value's extent; values are decoded only when read.
func readObjects(path string) objects {
	data, err := os.ReadFile(path)
	if err != nil {
		rec.Fail("%v", err)
	}
	if !json.Valid(data) {
		rec.Fail("Invalid qpdf JSON")
	}
	scan := &jsonScanner{data: data}
	var last *inventoryObjects
	found := false
	scan.members(func(key string, start, end int) {
		if key != "qpdf" {
			return
		}
		found = true
		inner := &jsonScanner{data: data[start:end]}
		inner.elements(func(start, end int) {
			last = objectsOf(inner.data[start:end])
		})
	})
	if !found {
		rec.Fail("key not found: \"qpdf\"")
	}
	if last == nil {
		rec.Fail("Invalid qpdf object inventory")
	}
	return last
}

func objectsOf(data []byte) *inventoryObjects {
	result := &inventoryObjects{raw: map[string]json.RawMessage{}, decoded: map[string]any{}}
	scan := &jsonScanner{data: data}
	scan.skipSpace()
	if scan.at >= len(data) || data[scan.at] != '{' {
		return result
	}
	scan.members(func(key string, start, end int) {
		if _, seen := result.raw[key]; !seen {
			result.keys = append(result.keys, key)
		}
		result.raw[key] = data[start:end]
	})
	return result
}

// jsonScanner walks valid JSON (checked before) by byte: the extents of an object's
// members or an array's elements, without decoding them.
type jsonScanner struct {
	data []byte
	at   int
}

func (s *jsonScanner) skipSpace() {
	for s.at < len(s.data) && (s.data[s.at] == ' ' || s.data[s.at] == '\t' || s.data[s.at] == '\n' || s.data[s.at] == '\r') {
		s.at++
	}
}

// members calls visit with each key of the object at the cursor and its value's extent.
func (s *jsonScanner) members(visit func(key string, start, end int)) {
	s.skipSpace()
	s.at++ // {
	for {
		s.skipSpace()
		if s.data[s.at] == '}' {
			s.at++
			return
		}
		if s.data[s.at] == ',' {
			s.at++
			s.skipSpace()
		}
		keyStart := s.at
		s.skipString()
		var key string
		if err := json.Unmarshal(s.data[keyStart:s.at], &key); err != nil {
			rec.Fail("%v", err)
		}
		s.skipSpace()
		s.at++ // :
		s.skipSpace()
		start := s.at
		s.skipValue()
		visit(key, start, s.at)
	}
}

// elements calls visit with each element's extent of the array at the cursor.
func (s *jsonScanner) elements(visit func(start, end int)) {
	s.skipSpace()
	s.at++ // [
	for {
		s.skipSpace()
		if s.data[s.at] == ']' {
			s.at++
			return
		}
		if s.data[s.at] == ',' {
			s.at++
			s.skipSpace()
		}
		start := s.at
		s.skipValue()
		visit(start, s.at)
	}
}

func (s *jsonScanner) skipString() {
	s.at++ // opening quote
	for {
		next := bytes.IndexAny(s.data[s.at:], "\"\\")
		s.at += next
		if s.data[s.at] == '\\' {
			s.at += 2
			continue
		}
		s.at++
		return
	}
}

func (s *jsonScanner) skipValue() {
	depth := 0
	for {
		switch s.data[s.at] {
		case '"':
			s.skipString()
		case '{', '[':
			depth++
			s.at++
		case '}', ']':
			depth--
			s.at++
		case ',', ' ', '\t', '\n', '\r', ':':
			if depth == 0 {
				return
			}
			s.at++
		default:
			s.at++
		}
		if depth == 0 && s.at < len(s.data) && !continues(s.data[s.at-1]) {
			return
		}
		if s.at >= len(s.data) {
			return
		}
	}
}

// continues reports whether a scalar may go on after this byte (a number or literal).
func continues(previous byte) bool {
	return previous != '"' && previous != '}' && previous != ']'
}

// fontRow is a Type1 font object of the inventory: its value and its base font name.
func fontRow(reference string, row any) (rec.Map, string, bool) {
	entry, ok := row.(rec.Map)
	if !paintmodel.ObjectRef.MatchString(reference) || !ok {
		return nil, "", false
	}
	font, ok := entry["value"].(rec.Map)
	if !ok || font["/Subtype"] != "/Type1" {
		return nil, "", false
	}
	base, ok := font["/BaseFont"].(string)
	return font, base, ok
}

func fetchObject(inventory objects, reference string) rec.Map {
	value, ok := inventory.Get(reference)
	if !ok {
		panic(rec.Failure{Message: "key not found: " + strconv.Quote(reference), Lookup: true})
	}
	return rec.AsMap(value)
}

// cmapStream is the font's ToUnicode stream data (base64).
func cmapStream(inventory objects, font rec.Map) string {
	cmap, ok := font["/ToUnicode"]
	if !ok {
		// A font without a ToUnicode map cannot bind its painted slots to text.
		rec.Fail("PDF font has no ToUnicode map")
	}
	return rec.FetchString(rec.FetchMap(fetchObject(inventory, "obj:"+rec.AsString(cmap)), "stream"), "data")
}
