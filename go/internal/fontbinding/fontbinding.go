// Package fontbinding is the Go port of Relatex::SourceFontBinding: observed TeX font names
// resolved through the actual virtual font (vftovp) and the PDF font map (pdftex.map) to
// the Type 1 or OpenType file the PDF embeds, with every file read named by its SHA-256.
package fontbinding

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/fontmodel"
	"github.com/sarielhp/relatex/internal/fspath"
	"github.com/sarielhp/relatex/internal/rec"
)

var (
	mapEntry = regexp.MustCompile(`\A\S+\s+([A-Za-z0-9_.-]+)\s+"\s*(?:-?[0-9.]+\s+SlantFont\s+)?([A-Za-z0-9_.-]+)\s+ReEncodeFont\s*"\s+` +
		`<\[?([A-Za-z0-9_.-]+\.enc)\s+<([A-Za-z0-9_.-]+\.pfb)\s*\z`)
	builtinEntry   = regexp.MustCompile(`\A\S+\s+([A-Za-z0-9_.-]+)\s+<([A-Za-z0-9_.-]+\.pfb)\s*\z`)
	safeName       = regexp.MustCompile(`\A[A-Za-z0-9_.-]+\z`)
	encodingSlot   = regexp.MustCompile(`dup\s+(\d+)\s*/([A-Za-z0-9_.-]+)\s+put`)
	mapFont        = regexp.MustCompile(`\(MAPFONT ([CODH]) ([^\s()]+)`)
	fontName       = regexp.MustCompile(`\(FONTNAME ([A-Za-z0-9_.-]+)\)`)
	fontScale      = regexp.MustCompile(`\(FONTAT R ([0-9.]+)\)`)
	encodingNote   = regexp.MustCompile(`%[^\n]*`)
	encodingVector = regexp.MustCompile(`(?s)\[(.*?)\]`)
	glyphName      = regexp.MustCompile(`/([A-Za-z0-9_.-]+)`)
)

// A font's binding depends only on its TeX files, which every resolution re-verifies by
// hash; the export and acceptance of one paper resolve the same fonts in one process.
var (
	lock     sync.Mutex
	maps     = map[string][]string{}
	resolved = map[[3]string]cachedBinding{}
)

type cachedBinding struct {
	binding []byte
	vpl     *string
}

// Binding is a SourceFontBinding resolver.
type Binding struct {
	kpse        string
	fontSources []rec.Map
	mapPath     string
	entries     []string
	found       map[string]string // a path, or "" for a batch miss (Ruby's :unknown)
}

// New is SourceFontBinding.new(font_sources:), with kpsewhich from the TeX Live tree.
func New(fontSources []rec.Map) *Binding {
	return NewWith(command.TeXLiveBin("kpsewhich"), fontSources)
}

// NewWith is SourceFontBinding.new(kpse, font_sources:).
func NewWith(kpse string, fontSources []rec.Map) *Binding {
	b := &Binding{kpse: kpse, fontSources: fontSources, found: map[string]string{}}
	b.mapPath = b.lookup("pdftex.map", false)
	info, err := os.Stat(b.mapPath)
	if err != nil {
		rec.Fail("%v", err)
	}
	key := strings.Join([]string{b.mapPath, strconv.FormatUint(inode(info), 10), strconv.FormatInt(info.Size(), 10),
		strconv.FormatInt(info.ModTime().UnixNano(), 10)}, "\x00")
	lock.Lock()
	defer lock.Unlock()
	if cached, ok := maps[key]; ok {
		b.entries = cached
		return b
	}
	text, err := os.ReadFile(b.mapPath)
	if err != nil {
		rec.Fail("%v", err)
	}
	for _, line := range readLines(string(text)) {
		if !strings.HasPrefix(strings.TrimLeft(line, " \t\n\v\f\r\x00"), "%") {
			b.entries = append(b.entries, line)
		}
	}
	maps[key] = b.entries
	return b
}

// readLines is File.readlines: lines with their newlines.
func readLines(text string) []string {
	lines := []string{}
	for text != "" {
		end := strings.IndexByte(text, '\n')
		if end < 0 {
			lines = append(lines, text)
			break
		}
		lines = append(lines, text[:end+1])
		text = text[end+1:]
	}
	return lines
}

// firstField is line.split.first ("" for a blank line).
func firstField(line string) string {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// Resolve is SourceFontBinding#resolve.
func (b *Binding) Resolve(name, output string) rec.Map {
	if !safeName.MatchString(name) {
		rec.Fail("Unsafe observed font name")
	}
	observed := []rec.Map{}
	for _, row := range b.fontSources {
		filename, _ := row["filename"].(string)
		if _, has := row["filename"]; !has {
			filename = ""
		}
		if row["font_name"] == name && strings.HasSuffix(filename, ".otf") {
			observed = append(observed, row)
		}
	}
	if len(observed) > 0 {
		return opentypeBinding(name, observed)
	}
	key := [3]string{b.kpse, b.mapPath, name}
	lock.Lock()
	cached, ok := resolved[key]
	lock.Unlock()
	if ok {
		return b.reuse(cached, name, output)
	}
	binding := b.resolveTeX(name, output)
	entry := cachedBinding{binding: encode(binding)}
	if text, err := os.ReadFile(filepath.Join(output, name+".vpl")); err == nil {
		vpl := string(text)
		entry.vpl = &vpl
	}
	lock.Lock()
	resolved[key] = entry
	lock.Unlock()
	return binding
}

func encode(value rec.Map) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		rec.Fail("%v", err)
	}
	return data
}

// decode reads back a cached binding: its values are strings and string maps only.
func decode(data []byte) rec.Map {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		rec.Fail("%v", err)
	}
	return normalize(raw).(rec.Map)
}

func normalize(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := rec.Map{}
		for key, item := range typed {
			result[key] = normalize(item)
		}
		return result
	case []any:
		for index, item := range typed {
			typed[index] = normalize(item)
		}
		return typed
	case float64:
		return int64(typed)
	}
	return value
}

func (b *Binding) resolveTeX(name, output string) rec.Map {
	tfm := b.lookup(name+".tfm", false)
	dependencies := []string{b.mapPath, tfm}
	bases, scales := b.virtualBases(name, output, &dependencies)
	if len(bases) > 1 {
		return b.virtualBinding(name, tfm, bases, scales, dependencies)
	}
	if len(scales) > 0 {
		rec.Fail("Unsupported multiple or scaled virtual font bases")
	}
	return b.parseEntry(b.entryFor(bases[0]), name, tfm, dependencies)
}

// entryFor is the one line of the PDF font map that names the font.
func (b *Binding) entryFor(base string) string {
	entries := []string{}
	for _, line := range b.entries {
		if firstField(line) == base {
			entries = append(entries, line)
		}
	}
	if len(entries) != 1 {
		rec.Fail("Ambiguous or absent PDF font map: %s", base)
	}
	return entries[0]
}

// virtualBinding: a virtual font that sets its glyphs from several unscaled base fonts (txfonts:
// the text face and the font of its own symbols) paints with any of them: its binding names every
// PDF face of its bases, and keeps its own TFM, which carries the kerns and the glue.
//
// A small caps font (ptmrc8t) sets its lower case from its base at a smaller size: scales lists
// the sizes, relative to the font, its glyphs may be painted at.
func (b *Binding) virtualBinding(name, tfm string, bases, scales []string, dependencies []string) rec.Map {
	faces := []string{}
	for _, base := range bases {
		fields := strings.Fields(b.entryFor(base))
		if len(fields) < 2 || !safeName.MatchString(fields[1]) {
			rec.Fail("Unsupported PDF font name: %s", base)
		}
		if !slices.Contains(faces, fields[1]) {
			faces = append(faces, fields[1])
		}
	}
	slices.Sort(faces)
	list := make([]any, len(faces))
	for index, face := range faces {
		list[index] = face
	}
	binding := rec.Map{"format": "virtual", "font_name": name, "postscript_names": list, "tfm_path": fspath.Slash(tfm),
		"tfm_sha256": command.FileDigest(tfm), "dependencies": digests(dependencies)}
	if len(scales) > 0 {
		values := make([]any, len(scales))
		for index, scale := range scales {
			values[index] = scale
		}
		binding["scales"] = values
	}
	return binding
}

// reuse: an earlier resolution in this process, valid while every file it read is
// unchanged; the virtual font listing it wrote is written again into this output.
func (b *Binding) reuse(cached cachedBinding, name, output string) rec.Map {
	binding := decode(cached.binding)
	for path, sha := range rec.FetchMap(binding, "dependencies") {
		native := dependencyPath(path)
		if info, err := os.Stat(native); err != nil || !info.Mode().IsRegular() || command.FileDigest(native) != sha {
			return b.resolveTeX(name, output)
		}
	}
	if cached.vpl != nil {
		if err := os.WriteFile(filepath.Join(output, name+".vpl"), []byte(*cached.vpl), 0o644); err != nil {
			rec.Fail("%v", err)
		}
	}
	return binding
}

// ResolveSlots is SourceFontBinding#resolve_slots.
func (b *Binding) ResolveSlots(name string, slots []int64, output string) rec.Map {
	output, _ = filepath.Abs(output)
	binding := b.Resolve(name, output)
	if binding["format"] == "virtual" {
		rec.Fail("Unsupported multiple or scaled virtual font bases")
	}
	encoding, _ := binding["source_encoding"].(rec.Map)
	if len(encoding) != 256 {
		rec.Fail("Encoded font binding requires a complete source vector")
	}
	virtual := false
	for path := range rec.FetchMap(binding, "dependencies") {
		virtual = virtual || strings.HasSuffix(path, ".vf")
	}
	packets := rec.Map{}
	mapping := rec.Map{}
	if virtual {
		text, err := os.ReadFile(filepath.Join(output, name+".vpl"))
		if err != nil {
			rec.Fail("%v", err)
		}
		packets = VirtualPackets(string(text), slots)
		mapping = fontmodel.PacketSlots(packets)
	} else {
		for _, slot := range slots {
			mapping[strconv.FormatInt(slot, 10)] = slot
		}
	}
	result := rec.Map{}
	for key, value := range binding {
		result[key] = value
	}
	result["slot_map"] = mapping
	result["virtual_packets"] = packets
	return result
}

// BuiltinEncoding is SourceFontBinding.builtin_encoding: a Type 1 font's own encoding, from
// the cleartext part before eexec: "/Encoding 256 array" then "dup CODE /NAME put" for each
// encoded slot, .notdef elsewhere. StandardEncoding and anything else stay empty: no vector.
func BuiltinEncoding(path string) rec.Map {
	data, err := os.ReadFile(path)
	if err != nil {
		rec.Fail("%v", err)
	}
	if len(data) > 65_536 {
		data = data[:65_536]
	}
	if at := bytes.Index(data, []byte("eexec")); at >= 0 {
		data = data[:at]
	}
	if !bytes.Contains(data, []byte("/Encoding 256 array")) {
		return rec.Map{}
	}
	slots := map[int64]string{}
	for _, match := range encodingSlot.FindAllSubmatch(data, -1) {
		code, err := strconv.ParseInt(string(match[1]), 10, 64)
		if err != nil {
			rec.Fail("invalid value for Integer(): %q", match[1])
		}
		slots[code] = string(match[2])
	}
	if len(slots) == 0 {
		return rec.Map{}
	}
	for code := range slots {
		if code < 0 || code > 255 {
			return rec.Map{}
		}
	}
	result := rec.Map{}
	for code := int64(0); code <= 255; code++ {
		glyph, ok := slots[code]
		if !ok {
			glyph = ".notdef"
		}
		result[strconv.FormatInt(code, 10)] = glyph
	}
	return result
}

// VirtualPackets is SourceFontBinding.virtual_packets: each slot's CHARACTER packet of a
// virtual font's property list.
func VirtualPackets(text string, slots []int64) rec.Map {
	if len(text) > 4_194_304 {
		rec.Fail("Virtual font description exceeds size bound")
	}
	bases := mapFont.FindAllStringSubmatch(text, -1)
	if len(bases) != 1 || fontmodel.VPLNumber(bases[0][1], bases[0][2]) != 0 {
		rec.Fail("Virtual slot binding requires one font at index zero")
	}
	records := characterRecords(text)
	packets := map[string]string{}
	for _, packet := range records {
		header := fontmodel.CharacterHead.FindStringSubmatch(packet)
		if header == nil {
			rec.Fail("Malformed virtual font character")
		}
		packets[strconv.FormatInt(fontmodel.VPLNumber(header[1], header[2]), 10)] = packet
	}
	if len(packets) != len(records) {
		rec.Fail("Duplicate virtual font character")
	}
	result := rec.Map{}
	for _, slot := range slots {
		packet, ok := packets[strconv.FormatInt(slot, 10)]
		if !ok {
			rec.Fail("Missing virtual font character")
		}
		result[strconv.FormatInt(slot, 10)] = packet
	}
	return result
}

// characterRecords is text.split(/(?=^\(CHARACTER )/).drop(1).
func characterRecords(text string) []string {
	starts := []int{}
	for at := 1; at < len(text); at++ {
		if text[at-1] == '\n' && strings.HasPrefix(text[at:], "(CHARACTER ") {
			starts = append(starts, at)
		}
	}
	records := make([]string, len(starts))
	for index, start := range starts {
		end := len(text)
		if index+1 < len(starts) {
			end = starts[index+1]
		}
		records[index] = text[start:end]
	}
	return records
}

func opentypeBinding(name string, observed []rec.Map) rec.Map {
	paths := []string{}
	consistent := true
	for _, row := range observed {
		path, err := fspath.Real(rec.FetchString(row, "filename"))
		if err != nil {
			rec.Fail("%v", err)
		}
		absolute, _ := filepath.Abs(path)
		if !slices.Contains(paths, absolute) {
			paths = append(paths, absolute)
		}
		psname, has := row["psname"]
		consistent = consistent && (!has || psname == name)
	}
	if len(paths) != 1 || !consistent {
		rec.Fail("Conflicting observed OpenType source identity")
	}
	source := paths[0]
	hash := command.FileDigest(source)
	observations := []any{}
	for _, row := range observed {
		slice := rec.Map{}
		for _, key := range []string{"font", "font_name", "format", "psname", "size_sp", "parameters"} {
			if value, ok := row[key]; ok {
				slice[key] = value
			}
		}
		observations = append(observations, slice)
	}
	slashed := fspath.Slash(source)
	return rec.Map{"format": "opentype", "font_name": name, "postscript_name": name, "full_source_path": slashed,
		"full_source_sha256": hash, "observations": observations, "dependencies": rec.Map{slashed: hash}}
}

// Prefetch resolves many files with one kpsewhich start (each costs ~50 ms); lookup then
// answers from these results. kpsewhich prints only the files it finds, in any order.
func (b *Binding) Prefetch(names []string) {
	pending := []string{}
	for _, name := range names {
		if _, known := b.found[name]; !known && !slices.Contains(pending, name) {
			pending = append(pending, name)
		}
	}
	if len(pending) == 0 {
		return
	}
	cmd := exec.Command(b.kpse, append([]string{"--"}, pending...)...)
	cmd.Env = command.Environment(nil)
	output, _ := cmd.Output()
	paths := map[string][]string{}
	for _, line := range strings.Split(string(output), "\n") {
		if path := rec.Strip(line); path != "" {
			paths[filepath.Base(path)] = append(paths[filepath.Base(path)], path)
		}
	}
	for _, name := range pending {
		b.found[name] = ""
		if candidates := paths[name]; len(candidates) == 1 && regular(candidates[0]) {
			b.found[name] = realpath(candidates[0])
		}
	}
}

func regular(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func realpath(path string) string {
	resolvedPath, err := fspath.Real(path)
	if err != nil {
		rec.Fail("%v", err)
	}
	absolute, _ := filepath.Abs(resolvedPath)
	return absolute
}

// PrefetchFonts prefetches what resolving these TeX fonts will look up: their TFM and VF
// files, then the encoding and Type1 files their map entries name (fonts without a virtual
// layer).
func (b *Binding) PrefetchFonts(names []string) {
	files := []string{}
	for _, name := range names {
		files = append(files, name+".tfm", name+".vf")
	}
	b.Prefetch(files)
	bases := []string{}
	virtual := []string{}
	for _, name := range names {
		if b.found[name+".vf"] != "" {
			virtual = append(virtual, name)
		}
	}
	for _, name := range names {
		if !slices.Contains(virtual, name) {
			bases = append(bases, name)
		}
	}
	for _, name := range virtual {
		data, err := os.ReadFile(b.found[name+".vf"])
		if err != nil {
			rec.Fail("%v", err)
		}
		bases = append(bases, VFFonts(data)...)
	}
	tfms := []string{}
	for _, base := range bases {
		tfms = append(tfms, base+".tfm")
	}
	b.Prefetch(tfms)
	b.Prefetch(b.mappedFiles(bases))
}

func (b *Binding) mappedFiles(bases []string) []string {
	files := []string{}
	for _, line := range b.entries {
		if !slices.Contains(bases, firstField(line)) {
			continue
		}
		match := mapEntry.FindStringSubmatch(line)
		if match == nil {
			match = builtinEntry.FindStringSubmatch(line)
		}
		for _, file := range match[min(1, len(match)):] {
			if strings.HasSuffix(file, ".enc") || strings.HasSuffix(file, ".pfb") {
				files = append(files, file)
			}
		}
	}
	return files
}

// VFFonts is SourceFontBinding.vf_fonts: the names of the fonts a virtual font builds on,
// from its fnt_def records (opcodes 243-246), which follow the preamble. Only used to
// prefetch; vftovp stays authoritative.
func VFFonts(data []byte) []string {
	at, ok := vfStart(data)
	if !ok {
		return []string{}
	}
	names := []string{}
	for at < len(data) && data[at] >= 243 && data[at] <= 246 {
		at += 1 + int(data[at]-242) + 12
		if at+1 >= len(data) {
			break
		}
		area, length := int(data[at]), int(data[at+1])
		start := min(at+2+area, len(data))
		names = append(names, string(data[start:min(start+length, len(data))]))
		at += 2 + area + length
	}
	safe := []string{}
	for _, name := range names {
		if safeName.MatchString(name) {
			safe = append(safe, name)
		}
	}
	return safe
}

func vfStart(data []byte) (int, bool) {
	if len(data) < 3 || data[0] != 247 || data[1] != 202 {
		return 0, false
	}
	return 3 + int(data[2]) + 8, true
}

func (b *Binding) lookup(name string, optional bool) string {
	known, cached := b.found[name]
	if cached && known != "" {
		return known
	}
	// A batch miss answers an optional lookup; a required one is asked again for its error.
	if cached && optional {
		return ""
	}
	cmd := exec.Command(b.kpse, "--", name)
	cmd.Env = command.Environment(nil)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	runErr := cmd.Run()
	if optional && runErr != nil && out.Len() == 0 && errOut.Len() == 0 {
		return ""
	}
	if runErr != nil || errOut.Len() > 0 || len(readLines(out.String())) != 1 {
		rec.Fail("Cannot resolve source font dependency: %s", name)
	}
	path := rec.Strip(out.String())
	if !regular(path) {
		rec.Fail("Missing source font dependency: %s", name)
	}
	return realpath(path)
}

// scales is nil when every base is unscaled, else each distinct scale ("0.8", "1.0"), sorted.
func (b *Binding) virtualBases(name, output string, dependencies *[]string) (bases, scales []string) {
	vf := b.lookup(name+".vf", true)
	if vf == "" {
		return []string{name}, nil
	}
	*dependencies = append(*dependencies, vf)
	vpl := filepath.Join(output, name+".vpl")
	tool := command.Executable("vftovp")
	if texLive := command.TeXLiveBin("vftovp"); tool == "" && command.IsExecutable(texLive) {
		tool = texLive // Windows: TeX Live's bin\windows is seldom on PATH (decision 129)
	}
	if tool == "" {
		rec.Fail("Missing virtual font inspection helper")
	}
	stage := command.Run([]string{tool, vf, b.lookup(name+".tfm", false), vpl}, output,
		filepath.Join(output, name+"-vftovp.log"), 60*time.Second, nil)
	if exit, ok := stage["exit"].(int64); !ok || exit != 0 {
		rec.Fail("Virtual font inspection failed")
	}
	WriteJSON(filepath.Join(output, name+"-vftovp.json"), stage)
	*dependencies = append(*dependencies, tool)
	data, err := os.ReadFile(vpl)
	if err != nil {
		rec.Fail("%v", err)
	}
	text := string(data)
	fonts := fontName.FindAllStringSubmatch(text, -1)
	sizes := fontScale.FindAllStringSubmatch(text, -1)
	if len(fonts) == 0 || len(sizes) != len(fonts) {
		rec.Fail("Unsupported multiple or scaled virtual font bases")
	}
	scaled := false
	for _, size := range sizes {
		if !slices.Contains(scales, size[1]) {
			scales = append(scales, size[1])
		}
		scaled = scaled || size[1] != "1.0"
	}
	if !scaled {
		scales = nil
	}
	slices.Sort(scales)
	bases = make([]string, len(fonts))
	for index, font := range fonts {
		bases[index] = font[1]
		*dependencies = append(*dependencies, b.lookup(font[1]+".tfm", false))
	}
	return bases, scales
}

// WriteJSON is Relatex.write_json for evidence files beside the capture (indented JSON).
func WriteJSON(path string, value any) {
	if err := canonjson.WriteFile(path, value); err != nil {
		rec.Fail("%v", err)
	}
}

func digests(paths []string) rec.Map {
	return digestsWith(paths, fspath.Slash, command.FileDigest)
}

func digestsWith(paths []string, slash func(string) string, digest func(string) string) rec.Map {
	result := rec.Map{}
	for _, path := range paths {
		result[slash(path)] = digest(path)
	}
	return result
}

func dependencyPath(path string) string { return filepath.FromSlash(path) }

func (b *Binding) parseEntry(entry, name, tfm string, dependencies []string) rec.Map {
	match := mapEntry.FindStringSubmatch(entry)
	if match == nil {
		return b.builtinBinding(entry, name, tfm, dependencies)
	}
	encoding := b.lookup(match[3], false)
	source := b.lookup(match[4], false)
	dependencies = append(dependencies, encoding, source)
	data, err := os.ReadFile(encoding)
	if err != nil {
		rec.Fail("%v", err)
	}
	vector := encodingVector.FindStringSubmatch(encodingNote.ReplaceAllString(string(data), ""))
	if vector == nil {
		rec.Fail("Unsupported source encoding vector")
	}
	names := glyphName.FindAllStringSubmatch(vector[1], -1)
	if len(names) != 256 {
		rec.Fail("Unsupported source encoding vector")
	}
	slots := rec.Map{}
	for code, glyph := range names {
		slots[strconv.Itoa(code)] = glyph[1]
	}
	return rec.Map{"font_name": name, "postscript_name": match[1], "tfm_path": fspath.Slash(tfm), "tfm_sha256": command.FileDigest(tfm),
		"full_source_path": fspath.Slash(source), "full_source_sha256": command.FileDigest(source), "source_encoding": slots,
		"dependencies": digests(dependencies)}
}

func (b *Binding) builtinBinding(entry, name, tfm string, dependencies []string) rec.Map {
	match := builtinEntry.FindStringSubmatch(entry)
	if match == nil {
		rec.Fail("Unsupported font map transformation or format")
	}
	source := b.lookup(match[2], false)
	dependencies = append(dependencies, source)
	return rec.Map{"format": "type1-builtin", "font_name": name, "postscript_name": match[1], "tfm_path": fspath.Slash(tfm),
		"tfm_sha256": command.FileDigest(tfm), "full_source_path": fspath.Slash(source), "full_source_sha256": command.FileDigest(source),
		"source_encoding": BuiltinEncoding(source), "dependencies": digests(dependencies)}
}
