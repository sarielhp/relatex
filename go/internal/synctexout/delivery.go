package synctexout

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/fspath"
	"github.com/sarielhp/relatex/internal/rec"
)

// generatedSources is SynctexDelivery::GENERATED: build products a SyncTeX file may name.
var generatedSources = []string{".tex", ".bbl", ".aux", ".toc", ".lof", ".lot", ".out"}

// Sources is how the delivery reads the paper's inventoried sources: the core's artifact resolver
// (ArtifactResolver), which the orchestration supplies so that this output never imports it.
type Sources interface {
	// Resolve is the inventoried file of a logical name, unchanged, inside the root.
	Resolve(name string) string
	// Translate is a path under a producer prefix, mapped to the directory that holds it now.
	Translate(path, relativeTo string) string
	// Relative is the logical name itself, failing on one with escapes.
	Relative(name string) string
	// AuthorBuildProduct is whether name is a file the author's own LaTeX run of job left.
	AuthorBuildProduct(name, job string) bool
}

// Options is what GeneralConversion#publish_native_synctex hands SynctexDelivery: the engine's
// PDF (its .synctex(.gz) beside it), the probe's external/working directory with the archive
// manifest's files, the layout and source map, the sources (made from the real source directory
// and its inventory, with the producer prefixes of a cached engine stage), the export directory
// and the mode (native or word).
type Options struct {
	PDF             string
	SourceDirectory string
	SourceFiles     []any
	Layout          rec.Map
	SourceMap       rec.Map
	Sources         func(root string, files map[string]string) Sources
	Directory       string
	Mode            string
	// OriginalDirectory is the author's own source directory when the input was one: the file a
	// viewer's reverse search opens is the real source, not the packaged copy.
	OriginalDirectory string
}

type delivery struct {
	pdf, source, directory, original string
	files                            map[string]string
	resolver                         Sources
	entries                          []any
}

// Publish is SynctexDelivery.new(...).write(directory, mode:): the sources packaged
// beside the PDF, the native SyncTeX retargeted to them, and in word mode a word-box SyncTeX.
// It writes synchronization.json and returns its content.
func Publish(options Options) Ordered {
	pdf, err := fspath.Real(options.PDF)
	check(err)
	source, err := fspath.Real(options.SourceDirectory)
	check(err)
	d := &delivery{pdf: pdf, source: source, original: options.OriginalDirectory, files: map[string]string{}}
	for _, value := range options.SourceFiles {
		row := rec.AsMap(value)
		if truthy(rec.Fetch(row, "directory")) {
			continue
		}
		d.files[rec.FetchString(row, "path")] = rec.FetchString(row, "sha256")
	}
	d.resolver = options.Sources(source, d.files)
	if options.Mode != "native" && options.Mode != "word" {
		rec.Fail("Unsupported SyncTeX delivery mode")
	}
	d.directory, err = fspath.Real(options.Directory)
	check(err)
	d.entries = []any{}
	// A directory input still holds its own files, which the SyncTeX names by absolute path: only an
	// archive's sources need a packaged copy to be readable.
	if d.original == "" {
		d.copySources(options.SourceFiles, strings.TrimSuffix(filepath.Base(pdf), ".pdf"))
	}
	return d.write(options)
}

func truthy(value any) bool { return value != nil && value != false }

func (d *delivery) write(options Options) Ordered {
	raw := strings.TrimSuffix(d.pdf, ".pdf") + ".synctex.gz"
	if !strings.HasSuffix(d.pdf, ".pdf") {
		raw = d.pdf
	}
	if info, err := os.Stat(raw); err != nil || !info.Mode().IsRegular() {
		raw = strings.TrimSuffix(raw, ".gz")
	}
	native := ReadSynctex(raw)
	// Every file is named after the PDF, as a SyncTeX viewer looks for <pdf name>.synctex.gz.
	job := strings.TrimSuffix(filepath.Base(d.pdf), ".pdf")
	// The word file names the sources itself; only the native file, or an archive input whose sources
	// are packaged, needs the native Input records retargeted.
	var portable *SynctexFile
	if options.Mode != "word" || d.original == "" {
		portable = native.Retarget(d.absoluteSource)
	}
	primary := filepath.Join(d.directory, job+".synctex.gz")
	result := Ordered{{"mode", options.Mode}, {"pdf", job + ".pdf"}, {"pdf_sha256", command.FileDigest(d.pdf)},
		{"companion", job + ".synctex.gz"}}
	if options.Mode == "word" {
		// The word-box file is the only SyncTeX beside the PDF; retargeting still packages the sources it names.
		result = append(result, Pair{"source_directory", "sources"}, Pair{"source_records", d.entries},
			Pair{"portable_original_sources", true}, Pair{"reader_compatibility", "unverified"},
			Pair{"word", d.word(options, primary)})
	} else {
		rawName := job + ".native.raw.synctex"
		if strings.HasSuffix(raw, ".gz") {
			rawName += ".gz"
		}
		copyFile(raw, filepath.Join(d.directory, rawName))
		report := portable.Write(primary)
		result = append(result, Pair{"raw_native", rawName}, Pair{"native", report}, Pair{"source_directory", "sources"},
			Pair{"source_records", d.entries}, Pair{"portable_original_sources", true},
			Pair{"reader_compatibility", "unverified"})
	}
	writeJSON(filepath.Join(d.directory, "synchronization.json"), result)
	return result
}

func (d *delivery) word(options Options, primary string) Ordered {
	if options.SourceMap == nil || (options.Layout == nil && options.SourceMap["pages"] == nil) {
		rec.Fail("Word SyncTeX requires source synchronization")
	}
	writer := newWordSynctex(options.Layout, options.SourceMap)
	writer.inputName = d.sourceName
	report := writer.generate().Write(primary)
	return append(report, Pair{"mapped_words", writer.mapped}, Pair{"unmapped_words", writer.unmapped},
		Pair{"boxes", writer.boxCount}, Pair{"precision", "Measured word boxes; native source line/column hints. " +
			"Inferred spans and ambiguity remain in JSON."})
}

func copyFile(from, to string) {
	input, err := os.Open(from)
	check(err)
	defer input.Close()
	output, err := os.Create(to)
	check(err)
	_, err = io.Copy(output, input)
	check(err)
	check(output.Close())
}

// copySources copies every inventoried source under sources/, in the manifest's order, except the
// author's own build products of the job.
func (d *delivery) copySources(rows []any, job string) {
	for _, value := range rows {
		row := rec.AsMap(value)
		if truthy(row["directory"]) {
			continue
		}
		name := rec.FetchString(row, "path")
		// A build product the author left beside the paper was set aside by the engine: not a source.
		if d.resolver.AuthorBuildProduct(name, job) {
			continue
		}
		destination := filepath.Join(d.directory, "sources", filepath.FromSlash(d.resolver.Relative(name)))
		check(os.MkdirAll(filepath.Dir(destination), 0o755))
		copyFile(d.resolver.Resolve(name), destination)
	}
}

// sourceRecord is ArtifactResolver#source.
func (d *delivery) sourceRecord(name string) Ordered {
	path := d.resolver.Resolve(name)
	hash := command.FileDigest(path)
	info, err := os.Stat(path)
	check(err)
	return Ordered{{"id", "source:" + hexSum(marshal([]any{name, hash}))}, {"file", name}, {"sha256", hash}, {"bytes", info.Size()}}
}

// sourceName is the absolute name of one source file (relative to the source directory): the
// author's own file where it still exists, else the packaged copy.
func (d *delivery) sourceName(name string) string {
	if d.original != "" {
		if candidate := filepath.Join(d.original, filepath.FromSlash(name)); regular(candidate) {
			return fspath.Slash(candidate)
		}
	}
	return fspath.Slash(filepath.Join(d.directory, "sources", filepath.FromSlash(name)))
}

// absoluteSource names a source by absolute path where it can: the author's own file when the input
// was a directory that still holds it, else the packaged copy beside the SyncTeX file. What is not
// packaged (an external file) keeps the name TeX recorded.
func (d *delivery) absoluteSource(tag int64, original string) string {
	packaged := d.packagedSource(tag, original)
	if packaged == "" {
		return ""
	}
	if name, found := strings.CutPrefix(packaged, "sources/"); found && !strings.HasPrefix(name, "_generated/") && d.original != "" {
		if candidate := filepath.Join(d.original, filepath.FromSlash(name)); regular(candidate) {
			return fspath.Slash(candidate)
		}
	}
	return fspath.Slash(filepath.Join(d.directory, filepath.FromSlash(packaged)))
}

func (d *delivery) packagedSource(tag int64, original string) string {
	path := d.resolver.Translate(original, d.source)
	if name, inside := fspath.Cut(d.source, path); inside {
		if _, ok := d.files[name]; ok {
			entry := append(d.sourceRecord(name), Pair{"tag", tag}, Pair{"role", "original"}, Pair{"packaged", "sources/" + name})
			d.entries = append(d.entries, entry)
			return "sources/" + name
		}
	}
	output := filepath.Dir(d.pdf)
	if fspath.Inside(output, path) && slices.Contains(generatedSources, rec.Extname(path)) && regular(path) {
		return d.packageGenerated(tag, path)
	}
	d.entries = append(d.entries, externalSourceEntry(tag, original, fspath.Slash))
	return ""
}

func externalSourceEntry(tag int64, original string, slash func(string) string) Ordered {
	return Ordered{{"tag", tag}, {"role", "external"}, {"native_path", slash(original)}}
}

func regular(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func (d *delivery) packageGenerated(tag int64, path string) string {
	resolved, err := fspath.Real(path)
	check(err)
	if !fspath.Inside(filepath.Dir(d.pdf), resolved) {
		rec.Fail("Generated SyncTeX source escapes the output snapshot")
	}
	info, err := os.Stat(path)
	check(err)
	if info.Size() > 16*1024*1024 {
		rec.Fail("Generated SyncTeX source exceeds limit")
	}
	hash := command.FileDigest(path)
	relative := "sources/_generated/" + hash[:16] + "-" + filepath.Base(path)
	destination := filepath.Join(d.directory, filepath.FromSlash(relative))
	check(os.MkdirAll(filepath.Dir(destination), 0o755))
	copyFile(path, destination)
	d.entries = append(d.entries, Ordered{{"tag", tag}, {"role", "generated"}, {"sha256", hash}, {"packaged", relative}})
	return relative
}
