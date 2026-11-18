package engine

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/fspath"
	"github.com/sarielhp/relatex/internal/rec"
)

var epsName = regexp.MustCompile(`(?i)\.(?:eps|pstex|ps|ipe)\z`)

// epsPDFName is the PDF a figure's file converts to: FILE-eps-converted-to.pdf, FILE-ps-converted-to.pdf or
// FILE-pstex-converted-to.pdf, the name epstopdf-base reads for the extension (.eps; .ps and xfig's .pstex through
// pstexRule).
func epsPDFName(path string) string {
	extension := strings.ToLower(filepath.Ext(path))
	return path[:len(path)-len(extension)] + "-" + extension[1:] + "-converted-to.pdf"
}

// namedPostScript reports whether a .ps file is one a paper includes: some .tex file names it with its extension
// (\includegraphics{Figs/oneball.ps}). Other .ps files are print copies beside their .eps or .pdf, not converted.
func namedPostScript(path string, sources []string) bool {
	name := filepath.Base(path)
	return slices.ContainsFunc(sources, func(text string) bool { return strings.Contains(text, name) })
}

// texTexts reads the .tex files under source.
func texTexts(source string) []string {
	texts := []string{}
	_ = filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && strings.HasSuffix(strings.ToLower(path), ".tex") {
			if data, readErr := os.ReadFile(path); readErr == nil {
				texts = append(texts, string(data))
			}
		}
		return nil
	})
	return texts
}

// pstexRule gives xfig's \includegraphics{X.pstex} and a plain \includegraphics{X.ps} (stock LaTeX names no graphics
// rule for them: "Unknown graphics extension: .pstex", the paper needs dvips) the rule epstopdf-base gives an .eps, so
// that they read the X-ps-converted-to.pdf or X-pstex-converted-to.pdf prepareEPS made beside them, named by it as an .eps is. Both builds
// carry it, the control one included; a rule the paper declares itself stays.
const pstexRule = `\makeatletter
\AddToHook{package/epstopdf-base/after}{%
  \@ifundefined{Gin@rule@.pstex}{\epstopdfDeclareGraphicsRule{.pstex}{pdf}{.pdf}{\ETE@epstopdf{#1}}}{}%
  \@ifundefined{Gin@rule@.ps}{\epstopdfDeclareGraphicsRule{.ps}{pdf}{.pdf}{\ETE@epstopdf{#1}}}{}%
  \@ifundefined{Gin@rule@.ipe}{\epstopdfDeclareGraphicsRule{.ipe}{pdf}{.pdf}{\ETE@epstopdf{#1}}}{}}
\makeatother
`

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
	p.ipeGraphics = ipeRecord(assets)
	if len(assets) > 0 {
		writeJSON(filepath.Join(p.Directory, "eps-assets.json"), assets)
	}
}

// prepareEPSAssets is EPSConversion.prepare!: each EPS figure becomes FILE-eps-converted-to.pdf,
// plus FILE.pdf unless the paper supplies one; a failed conversion leaves the EPS.
func prepareEPSAssets(source string, run func(tool string, arguments []string, name string) rec.Map) []any {
	assets := []any{}
	for _, path := range epsFiles(source) {
		converted := epsPDFName(path)
		if !exists(converted) {
			how, failure := convertFigure(path, converted, len(assets)+1, run)
			if failure != "" {
				entry := asset(source, path, "input")
				entry["conversion"] = failure + "; control determines whether this asset is required"
				assets = append(assets, entry)
				continue
			}
			entry := merged(asset(source, path, "input"), asset(source, converted, "output"))
			if how != "" {
				entry["ipe"] = how
			}
			assets = append(assets, entry)
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

// namedOnly reports whether a figure file is converted only when a .tex file names it: a .ps print copy beside an .eps
// or .pdf, an .ipe drawing a paper reads some other way.
func namedOnly(path string) bool {
	extension := strings.ToLower(filepath.Ext(path))
	return extension == ".ps" || extension == ".ipe"
}

// convertFigure is convertEPS for an .eps, .ps or .pstex, and convertIpe for an .ipe (decision 194); how names what an
// .ipe held ("PDF", "EPS" or "placeholder"), "" for the others.
func convertFigure(path, converted string, index int, run func(string, []string, string) rec.Map) (how, failure string) {
	if strings.ToLower(filepath.Ext(path)) != ".ipe" {
		return "", convertEPS(path, converted, index, run)
	}
	return convertIpe(path, converted, index, run), ""
}

// convertIpe makes the PDF of an Ipe graphic, which Ipe wrote as EPS (Ipe 5) or PDF and a paper named .ipe: a PDF is
// copied, an EPS converted like an .eps, anything else (Ipe's XML, a mail message saved as the file) or an EPS that does
// not convert becomes a placeholder box, so that the paper still builds. It returns what was made.
func convertIpe(path, converted string, index int, run func(string, []string, string) rec.Map) string {
	head := make([]byte, 1024)
	if file, err := os.Open(path); err == nil {
		count, _ := file.Read(head)
		head = head[:count]
		_ = file.Close()
	}
	switch {
	case bytes.HasPrefix(head, []byte("%PDF-")):
		check(copyFile(path, converted))
		return "PDF"
	case bytes.HasPrefix(head, []byte("%!PS")) && convertEPS(path, converted, index, run) == "":
		return "EPS"
	}
	check(os.WriteFile(converted, placeholderPDF(), 0o644))
	return "placeholder"
}

// placeholderPDF is a one-page PDF with a crossed box 144 by 108 points: a figure that could not be read.
func placeholderPDF() []byte {
	stream := "0.6 G 1 w 1 1 142 106 re S 1 1 m 143 107 l S 1 107 m 143 1 l S\n"
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 144 108] /Contents 4 0 R /Resources << >> >>",
		"<< /Length " + itoa(len(stream)) + " >>\nstream\n" + stream + "endstream",
	}
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects))
	for index, body := range objects {
		offsets[index] = out.Len()
		out.WriteString(itoa(index+1) + " 0 obj\n" + body + "\nendobj\n")
	}
	table := out.Len()
	out.WriteString("xref\n0 " + itoa(len(objects)+1) + "\n0000000000 65535 f \n")
	for _, offset := range offsets {
		out.WriteString(fmt.Sprintf("%010d 00000 n \n", offset))
	}
	out.WriteString("trailer\n<< /Size " + itoa(len(objects)+1) + " /Root 1 0 R >>\nstartxref\n" + itoa(table) + "\n%%EOF\n")
	return out.Bytes()
}

// ipeRecord is the report's ipe_graphics: each converted .ipe with what it held.
func ipeRecord(assets []any) []any {
	record := []any{}
	for _, value := range assets {
		if entry := rec.AsMap(value); entry["ipe"] != nil {
			record = append(record, rec.Map{"path": entry["input"], "how": entry["ipe"]})
		}
	}
	return record
}

func epsFiles(source string) []string {
	found := []string{}
	texts := texTexts(source)
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
		if epsName.MatchString(path) && isFile(path) && (!namedOnly(path) || namedPostScript(path, texts)) {
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
