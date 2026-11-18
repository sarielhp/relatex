package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

// Decision 082: a source written for pdfLaTeX or XeLaTeX is built again with the compatibility
// layer (tex/relatex-compat.sty) after its plain first pass failed on something the layer supplies.
// A source whose plain build compiles never gets the layer.

// compatibilityShims are the layer's options in the order the wrapper passes them.
var compatibilityShims = []string{"pdftex", "xetex", "fontspec", "t1text", "inputenc", "textmap", "microtype", "hyperref", "fontawesome", "psfig", "ltexpprt"}

// pdfPrimitives are the pdfTeX names the pdftex shim defines.
var pdfPrimitives = strings.Fields(`pdfoutput pdfpagewidth pdfpageheight pdfminorversion pdfcompresslevel
	pdfobjcompresslevel pdfinfo pdfcatalog pdfpageattr pdfpagesattr pdfpageresources pdfadjustspacing pdfprotrudechars
	pdfmapfile pdfmapline pdfsuppresswarningpagegroup pdfstrcmp pdffilesize pdfliteral pdfsavepos pdflastxpos pdflastypos
	pdfelapsedtime pdfresettimer pdfshellescape pdfpkresolution pdfdraftmode pdfinclusionerrorlevel pdfinclusioncopyfonts
	pdfimageresolution pdfdecimaldigits pdfhorigin pdfvorigin pdfximage pdfrefximage pdflastximage pdfxform pdfrefxform
	pdflastxform pdfannot pdfdest pdfobj pdfrefobj pdflastobj pdfuniformdeviate pdfnormaldeviate pdfrandomseed
	pdfsetrandomseed pdfsave pdfrestore pdfsetmatrix pdfcolorstack pdfstartlink pdfendlink pdfoutline pdfnames pdftrailer
	pdfpkmode pdfprimitive pdfcreationdate pdfpxdimen pdfoptionpdfminorversion pdfoptionpdfinclusionerrorlevel`)

// fontCommands are fontspec's commands the fontspec shim stands in for.
var fontCommands = []string{"setmainfont", "setsansfont", "setmonofont", "newfontfamily", "defaultfontfeatures"}

var (
	// xetexPrimitive is an XeTeX primitive other than the two that identify the engine.
	xetexPrimitive = regexp.MustCompile(`\AXeTeX[a-z]+\z`)
	// lastName is the undefined name that ends the first context line after the error.
	lastName = regexp.MustCompile(`\\([A-Za-z@]+)\s*\z`)
	// packageRefusals are packages that refuse LuaTeX, with the shim that satisfies them.
	packageRefusals = []struct {
		shim    string
		pattern *regexp.Regexp
	}{
		{"pdftex", regexp.MustCompile(`Package xypdf Error: pdfTeX version[^\n]*`)},
		{"xetex", regexp.MustCompile(`this package currently works only with XeTeX[^\n]*`)},
		// An 8-bit input encoding: the builds re-encode the source to UTF-8 (decision 143, legacyinput.go).
		{"inputenc", regexp.MustCompile(`Package inputenc Error: inputenc is not designed for xetex or luatex[^\n]*`)},
		// microtype's pdfTeX-only features: switched off with a warning (decision 143).
		{"microtype", regexp.MustCompile(`Package microtype Error: The (?:spacing|kerning) feature only works with pdftex[^\n]*`)},
		// hyperref's DVI driver options (dvipdfmx, dvips, hypertex): dropped from its options (decision 143,
		// amendment 1).
		{"hyperref", regexp.MustCompile("Package hyperref Error: Wrong DVI mode driver option `[a-z]+'")},
		// fontawesome5's [pro] option: its commercial Pro fonts are not in TeX Live; Free stands in.
		{"fontawesome", regexp.MustCompile(`! Font \\TU/fontawesomepro/[^\n]*`)},
		// LaTeX 2.09 packages TeX Live no longer has: psfig (epsfig's \psfig) and ltexpprt (the SIAM proceedings
		// option) are taken as loaded (decision 194).
		{"psfig", regexp.MustCompile("LaTeX Error: File `psfig\\.sty' not found[^\n]*")},
		{"ltexpprt", regexp.MustCompile("LaTeX Error: File `ltexpprt\\.sty' not found[^\n]*")},
	}
)

const undefinedControlSequence = "! Undefined control sequence.\n"

// tuFallback is NFSS's substitution of an 8-bit text family with no TU shapes (times' ptm, txfonts' txr)
// by Latin Modern regular: a warning in a pass that compiles (decision 082, amendment 1).
var tuFallback = regexp.MustCompile("Font shape `TU/[A-Za-z0-9]+/[^']*' undefined")

// compatibilityRequests are the shims a failed pass's log asks for, each with the error that asked.
func compatibilityRequests(log string) []rec.Map {
	requests := []rec.Map{}
	add := func(shim, cause string) {
		for _, request := range requests {
			if request["shim"] == shim {
				return
			}
		}
		requests = append(requests, rec.Map{"shim": shim, "cause": cause})
	}
	for _, name := range undefinedNames(log) {
		if shim := shimForName(name); shim != "" {
			add(shim, `undefined \`+name)
		}
	}
	for _, refusal := range packageRefusals {
		if match := refusal.pattern.FindString(log); match != "" {
			add(refusal.shim, match)
		}
	}
	return append(requests, warningRequests(log)...)
}

// unmappedCharacter is a typed character LuaTeX passed unmapped to an 8-bit font (a TFM name, not an
// OpenType one in brackets or with features), which pdfLaTeX's inputenc mapped (decision 143).
var unmappedCharacter = regexp.MustCompile(`(?m)^Missing character: There is no .+? \(U\+([0-9A-F]{4,6})\) in font [a-z0-9-]+!$`)

// warningRequests are the shims a first pass's warnings ask for, whether or not it compiled: t1text for a
// text family that fell back to Latin Modern, textmap for a typed character an 8-bit font got unmapped.
func warningRequests(log string) []rec.Map {
	requests := []rec.Map{}
	for _, match := range tuFallback.FindAllString(log, -1) {
		if !strings.HasPrefix(match, "Font shape `TU/lmr/") {
			requests = append(requests, rec.Map{"shim": "t1text", "cause": match})
			break
		}
	}
	if cause := unmappedNonASCII(log); cause != "" {
		requests = append(requests, rec.Map{"shim": "textmap", "cause": cause})
	}
	return requests
}

// unmappedNonASCII is the first missing character beyond ASCII in an 8-bit font, else "". U+FFFD is
// LuaTeX's stand-in for bytes that are not UTF-8, which no mapping can print.
func unmappedNonASCII(log string) string {
	for _, match := range unmappedCharacter.FindAllStringSubmatch(log, -1) {
		if code, err := strconv.ParseUint(match[1], 16, 32); err == nil && code >= 0x80 && code != 0xFFFD {
			return match[0]
		}
	}
	return ""
}

func undefinedNames(log string) []string {
	names := []string{}
	for rest := log; ; {
		index := strings.Index(rest, undefinedControlSequence)
		if index < 0 {
			return names
		}
		rest = rest[index+len(undefinedControlSequence):]
		context, _, _ := strings.Cut(rest, "\n")
		if match := lastName.FindStringSubmatch(context); match != nil && !slices.Contains(names, match[1]) {
			names = append(names, match[1])
		}
	}
}

func shimForName(name string) string {
	switch {
	case slices.Contains(pdfPrimitives, name):
		return "pdftex"
	case xetexPrimitive.MatchString(name) && name != "XeTeXrevision" && name != "XeTeXversion":
		return "xetex"
	case slices.Contains(fontCommands, name):
		return "fontspec"
	}
	return ""
}

// compatibilityLine loads the layer with the shims applied so far; "" without any.
func (p *Probe) compatibilityLine() string {
	options := []string{}
	for _, shim := range compatibilityShims {
		for _, request := range p.compatibility {
			if request["shim"] == shim {
				options = append(options, shim)
				break
			}
		}
	}
	if len(options) == 0 {
		return ""
	}
	return "\\RequirePackage[" + strings.Join(options, ",") + "]{relatex-compat}\n"
}

// newCompatibilityRequests reads the first pass of each build that did not compile for shims not
// yet applied.
func (p *Probe) newCompatibilityRequests(built []rec.Map) []rec.Map {
	found := []rec.Map{}
	for index, strategy := range p.strategies() {
		path := filepath.Join(p.Directory, strategy, "output", "pass-1.log")
		if !isFile(path) {
			continue
		}
		// A build that compiled asks only for what its first pass's warnings show (decision 082, amendment 1).
		asked := warningRequests
		if built[index]["compiled"] != true {
			asked = compatibilityRequests
		}
		for _, request := range asked(readLog(path)) {
			if !hasShim(p.compatibility, request["shim"]) && !hasShim(found, request["shim"]) {
				found = append(found, request)
			}
		}
	}
	return found
}

func hasShim(requests []rec.Map, shim any) bool {
	for _, request := range requests {
		if request["shim"] == shim {
			return true
		}
	}
	return false
}

// buildWithCompatibility is the plain builds, then, while their failed first passes ask for shims
// not yet applied, the builds again from nothing with the layer.
func (p *Probe) buildWithCompatibility() []rec.Map {
	built := p.buildAll()
	for range compatibilityShims {
		requests := p.newCompatibilityRequests(built)
		if len(requests) == 0 {
			break
		}
		p.compatibility = append(p.compatibility, requests...)
		for _, request := range requests {
			fmt.Fprintf(os.Stderr, "engine: compatibility layer %s (%s)\n", request["shim"], request["cause"])
		}
		p.removeBuilds()
		before, shims := built, p.compatibility[:len(p.compatibility)-len(requests)]
		built = p.buildAll()
		if allCompiled(before) && !allCompiled(built) {
			// A layer that only a warning asked for must not break a build that compiled (cite keys with
			// accented letters under t1text's luainputenc): the plain build stands.
			fmt.Fprintln(os.Stderr, "engine: compatibility layer dropped, the build with it does not compile")
			p.compatibility = shims
			p.removeBuilds()
			built = p.buildAll()
			break
		}
	}
	return built
}

func allCompiled(built []rec.Map) bool {
	for _, build := range built {
		if build["compiled"] != true {
			return false
		}
	}
	return true
}

// removeBuilds clears every strategy's working and output directories.
func (p *Probe) removeBuilds() {
	for _, strategy := range p.strategies() {
		for _, part := range []string{"working", "output"} {
			check(os.RemoveAll(filepath.Join(p.Directory, strategy, part)))
		}
	}
}

// compatibilityRecord is probe.json's compatibility: each shim applied with its cause.
func (p *Probe) compatibilityRecord() []any {
	record := []any{}
	for _, request := range p.compatibility {
		record = append(record, request)
	}
	return record
}
