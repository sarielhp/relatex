package pdffonts

import (
	"encoding/binary"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
)

// installedFontFile is a font file fontconfig would list: TrueType, OpenType and their collections.
var installedFontFile = regexp.MustCompile(`(?i)\.(?:ttf|otf|ttc|otc)\z`)

// maxCollectionFaces and maxNameTable bound what a scan reads of one file.
const (
	maxCollectionFaces = 1024
	maxNameTable       = 1 << 20
)

// windowsFontDirectories are the system's and the user's font folders: Windows has no fontconfig
// (decision 129).
func windowsFontDirectories() []string {
	return []string{filepath.Join(os.Getenv("WINDIR"), "Fonts"), filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "Windows", "Fonts")}
}

// scanFonts lists the fonts under the directories as fc-list does: each face's file, index in a
// collection, families (typographic, then legacy), styles and PostScript name, read from its name
// table without reading the rest of the file.
func scanFonts(directories []string) []fcEntry {
	var entries []fcEntry
	for _, directory := range directories {
		_ = filepath.WalkDir(directory, func(path string, entry fs.DirEntry, err error) error {
			if err == nil && !entry.IsDir() && installedFontFile.MatchString(path) {
				entries = append(entries, fontEntries(path)...)
			}
			return nil // an unreadable directory or file is no installed font
		})
	}
	return entries
}

// fontEntries are the faces of one font file.
func fontEntries(path string) []fcEntry {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	var entries []fcEntry
	for face, offset := range faceOffsets(file) {
		names := nameTableAt(file, offset)
		if names == nil {
			continue
		}
		entries = append(entries, fcEntry{file: path, face: face, postscript: sfntName(names, 6),
			families: distinctNames(sfntName(names, 16), sfntName(names, 1)), styles: distinctNames(sfntName(names, 17), sfntName(names, 2))})
	}
	return entries
}

// faceOffsets are where each face's table directory starts: 0 for a single font, the
// collection's offsets for a .ttc.
func faceOffsets(file *os.File) []int64 {
	header := make([]byte, 12)
	if _, err := file.ReadAt(header, 0); err != nil {
		return nil
	}
	if string(header[:4]) != "ttcf" {
		return []int64{0}
	}
	count := binary.BigEndian.Uint32(header[8:])
	if count == 0 || count > maxCollectionFaces {
		return nil
	}
	table := make([]byte, 4*count)
	if _, err := file.ReadAt(table, 12); err != nil {
		return nil
	}
	offsets := make([]int64, count)
	for index := range offsets {
		offsets[index] = int64(binary.BigEndian.Uint32(table[4*index:]))
	}
	return offsets
}

// nameTableAt is the name table of the face whose table directory starts at offset; nil without one.
func nameTableAt(file *os.File, offset int64) []byte {
	head := make([]byte, 12)
	if _, err := file.ReadAt(head, offset); err != nil {
		return nil
	}
	count := int(binary.BigEndian.Uint16(head[4:]))
	directory := make([]byte, 16*count)
	if _, err := file.ReadAt(directory, offset+12); err != nil {
		return nil
	}
	for index := range count {
		entry := directory[16*index:]
		if string(entry[:4]) != "name" {
			continue
		}
		length := binary.BigEndian.Uint32(entry[12:])
		if length > maxNameTable {
			return nil
		}
		table := make([]byte, length)
		if _, err := file.ReadAt(table, int64(binary.BigEndian.Uint32(entry[8:]))); err != nil {
			return nil
		}
		return table
	}
	return nil
}

// distinctNames are the non-empty names, each once, in order.
func distinctNames(names ...string) []string {
	result := []string{}
	for _, name := range names {
		if name != "" && (len(result) == 0 || result[len(result)-1] != name) {
			result = append(result, name)
		}
	}
	return result
}
