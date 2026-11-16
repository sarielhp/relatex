package pdfpaint

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/fontbinding"
	"github.com/sarielhp/relatex/internal/geometry"
	"github.com/sarielhp/relatex/internal/nativetrace"
	"github.com/sarielhp/relatex/internal/paintmodel"
	"github.com/sarielhp/relatex/internal/rec"
)

// The text paint proof of PDFTextPaint: a bounded native paint profile. Unsupported ink is
// rejected, never recolored.
const ()

var (
	unsupportedElements = []string{"clip_stroke_text", "mask"}
)

// CaptureText is PDFTextPaint.new(pdf, pdf_media_groups:).capture and its directory.
func CaptureText(pdf string, mediaGroups bool) (paint rec.Map, directory string) {
	pdf, _ = filepath.Abs(pdf)
	directory = filepath.Join(command.Scratch("text-paint-"), "evidence")
	if exists(directory) {
		rec.Fail("Text paint output must be new")
	}
	mkdir(directory)
	original, helper := command.FileDigest(pdf), command.FileDigest(tool())
	runText([]string{tool(), "-v"}, directory, "version", directory, 5*time.Second)
	if data, _ := os.ReadFile(filepath.Join(directory, "version.log")); !SupportedVersion(rec.Strip(string(data))) {
		rec.Fail("Unsupported text paint helper")
	}
	path := filepath.Join(directory, "text.xml")
	runText(command.MutoolDrawCommand(tool(), "-F", "trace", "-o", path, filepath.Base(pdf)), directory, "trace", filepath.Dir(pdf), 60*time.Second)
	data, err := os.ReadFile(path)
	if err != nil {
		rec.Fail("%v", err)
	}
	pages, glyphs, inks, canvas, backdrops := inspectTrace(data, mediaGroups)
	if command.FileDigest(pdf) != original || command.FileDigest(tool()) != helper {
		rec.Fail("Text paint PDF or helper changed")
	}
	paint = rec.Map{"pages": pages, "glyphs": glyphs, "status": "native_text_paint_verified", "pdf_sha256": original,
		"helper_sha256": helper, "trace_sha256": command.FileDigest(path), "color": paintmodel.Color}
	if mediaGroups {
		paint["schema_version"], paint["group_profile"] = paintmodel.TextVersion, paintmodel.MediaGroupProfile
	}
	if canvas != "" {
		paint["canvas"] = canvas
	}
	if len(inks) > 0 {
		paint["inks"] = inks
	}
	if len(backdrops) > 0 {
		paint["backdrops"] = backdrops
	}
	fontbinding.WriteJSON(filepath.Join(directory, "paint.json"), paint)
	return paint, directory
}

func runText(arguments []string, directory, name, workdir string, timeout time.Duration) {
	runTool(arguments, directory, name, workdir, timeout, "Native text paint extraction failed")
}

type traceState struct {
	pages    []int64
	texts    int64
	glyphs   int64
	clipText bool
	inks     map[string]bool
	canvas   canvasReader
	backdrop backdropReader
}

// addInk records a non-black fill the trace painted text in.
func (t *traceState) addInk(hex string) {
	if hex == "" {
		return
	}
	if t.inks == nil {
		t.inks = map[string]bool{}
	}
	t.inks[hex] = true
}

// inkList is the distinct non-black fills, sorted.
func (t *traceState) inkList() []any {
	list := make([]string, 0, len(t.inks))
	for hex := range t.inks {
		list = append(list, hex)
	}
	slices.Sort(list)
	result := make([]any, len(list))
	for index, hex := range list {
		result[index] = hex
	}
	return result
}

// linkLayerText: ocgx2's ocgcolorlinks sets a link's text as a clip (render mode 7) and
// fills through it inside optional-content layers (black for print, the link colour on
// screen): the glyphs are painted, in the layer's colour. A clip text is accepted only when
// a fill inside a layer follows before any other text or page; anything else stays
// unsupported.
func linkLayerText(name string, open []*node, state *traceState) bool {
	switch name {
	case "clip_text":
		if state.clipText {
			rec.Fail("Unsupported text paint operation")
		}
		state.clipText = true
		state.texts++
		return true
	case "fill_path":
		for _, row := range open {
			if row.Name == "layer" {
				state.clipText = false
				break
			}
		}
	case "fill_text", "page":
		if state.clipText {
			rec.Fail("Unsupported text paint operation")
		}
	}
	return false
}

// inspectTrace is PDFTextPaint.inspect_trace!: one streaming pass over mutool's trace, every
// element checked as it is read.
func inspectTrace(data []byte, mediaGroups bool) (pages, glyphs int64, inks []any, canvas string, backdrops []any) {
	if nativetrace.Declared(data) {
		rec.Fail("Text paint trace declarations forbidden")
	}
	state := inspectPieces(data, mediaGroups)
	if state.clipText {
		rec.Fail("Unsupported text paint operation")
	}
	for index, page := range state.pages {
		if page != int64(index+1) {
			rec.Fail("Incomplete text paint pages")
		}
	}
	if len(state.pages) == 0 {
		rec.Fail("Incomplete text paint pages")
	}
	if state.texts == 0 {
		rec.Fail("Missing native text paint")
	}
	if state.glyphs <= 0 {
		rec.Fail("Empty native text paint")
	}
	canvas = canvasOf(state.canvas.pages, len(state.pages))
	return int64(len(state.pages)), state.glyphs, state.inkList(), canvas, state.backdrop.backdrops(canvas)
}

// inspectPieces reads the trace: a large one in runs of whole pages side by side, whose totals add up. A
// page may not end inside a clipped text run (the next element fails), so each run starts clean and one
// that ends in one fails as the whole read does; the first failing run in order is raised.
func inspectPieces(data []byte, mediaGroups bool) *traceState {
	pieces := nativetrace.Pieces(data)
	if pieces == nil {
		pieces = [][]byte{data}
	}
	states := make([]*traceState, len(pieces))
	failures := make([]any, len(pieces))
	var group sync.WaitGroup
	for index, piece := range pieces {
		group.Add(1)
		go func() {
			defer group.Done()
			defer func() { failures[index] = recover() }()
			state := &traceState{}
			eachElement(piece, func(name string, attrs map[string]string, open []*node) {
				state.canvas.visit(name, attrs, open)
				state.backdrop.visit(name, attrs, open)
				inspectElement(name, attrs, open, state, mediaGroups)
			})
			state.canvas.finish()
			state.backdrop.closePath()
			states[index] = state
		}()
	}
	group.Wait()
	for _, failure := range failures {
		if failure != nil {
			panic(failure)
		}
	}
	merged := &traceState{}
	for index, state := range states {
		if state.clipText && index < len(states)-1 {
			rec.Fail("Unsupported text paint operation")
		}
		merged.pages = append(merged.pages, state.pages...)
		merged.texts += state.texts
		merged.glyphs += state.glyphs
		merged.clipText = state.clipText
		for page, color := range state.canvas.pages {
			if merged.canvas.pages == nil {
				merged.canvas.pages = map[int64]string{}
			}
			merged.canvas.pages[page] = color
		}
		for hex := range state.inks {
			merged.addInk(hex)
		}
		merged.backdrop.merge(&state.backdrop)
	}
	return merged
}

func inspectElement(name string, attrs map[string]string, open []*node, state *traceState, mediaGroups bool) {
	if slices.Contains(unsupportedElements, name) {
		rec.Fail("Unsupported text paint operation")
	}
	if linkLayerText(name, open, state) {
		return
	}
	switch name {
	case "page":
		state.pages = append(state.pages, rec.ToI(attrs["number"]))
	case "fill_text", "stroke_text":
		for _, row := range open {
			if row.Name == "tile" {
				rec.Fail("Unsupported text paint operation")
			}
			if row.Name == "group" && alphaValue(row.Attrs) < 1 {
				rec.Fail("Unsupported PDF media compositing group")
			}
		}
		state.addInk(verifyInk(attrs))
		state.texts++
	case "g":
		if glyphRun(open) {
			state.glyphs++
		}
	case "group":
		var page map[string]string
		for index := len(open) - 1; index >= 0; index-- {
			if open[index].Name == "page" {
				page = open[index].Attrs
				break
			}
		}
		if ok := tryGroup(attrs, page); !ok {
			if !mediaGroups {
				verifyGroup(attrs, page)
			}
			parents := []map[string]string{}
			for _, row := range open {
				if row.Name == "group" {
					parents = append(parents, row.Attrs)
				}
			}
			verifyMediaGroup(attrs, page, parents)
		}
	}
}

// alphaValue is a group's alpha; an unreadable one counts as transparent.
func alphaValue(attrs map[string]string) float64 {
	text, ok := attrs["alpha"]
	if !ok {
		return 1
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0
	}
	return value
}

// glyphRun: a glyph inside a span of a filled, clipped or stroked text run.
func glyphRun(open []*node) bool {
	if len(open) < 2 || open[len(open)-1].Name != "span" {
		return false
	}
	return slices.Contains([]string{"fill_text", "clip_text", "stroke_text"}, open[len(open)-2].Name)
}

// verifyInk is the paint of one painted text run: opaque, in DeviceGray, DeviceRGB, DeviceCMYK
// or a named process separation. It
// returns the color as #rrggbb, "" for black (the default, which is not recorded). Translucent
// ink and other color spaces are rejected, never recolored.
func verifyInk(attrs map[string]string) string {
	alpha, ok := attrs["alpha"]
	if !ok {
		alpha = "1"
	}
	space := attrs["colorspace"]
	hex, supported := InkHex(space, strings.Fields(attrs["color"]))
	if strings.HasPrefix(space, "Separation(") && !supported {
		rec.Fail("Unsupported native text separation ink")
	}
	if !supported || alpha != "1" {
		rec.Fail("Text paint profile requires native opaque ink")
	}
	return hex
}

// processSeparations: a process ink named as a separation colorant; its tint 1 is the full
// process color. The trace reports the tint but not the tint transform, so only these
// standard names are modelled.
var processSeparations = map[string][]string{
	"Cyan": {"1", "0", "0", "0"}, "Magenta": {"0", "1", "0", "0"},
	"Yellow": {"0", "0", "1", "0"}, "Black": {"0", "0", "0", "1"},
}

var separationPattern = regexp.MustCompile(`\ASeparation\((DeviceCMYK|DeviceGray|DeviceRGB),([^)]+)\)\z`)

// InkHex converts a DeviceGray, DeviceRGB, DeviceCMYK or named-process Separation fill to
// #rrggbb, rounding as the collector's fill_color does; "" for black. The second result is
// false for any other fill.
func InkHex(space string, values []string) (string, bool) {
	if strings.HasPrefix(space, "Separation(") {
		match := separationPattern.FindStringSubmatch(space)
		ink, known := []string(nil), false
		if match != nil {
			ink, known = processSeparations[match[2]]
		}
		if !known || match[1] != "DeviceCMYK" || len(values) != 1 || values[0] != "1" {
			return "", false
		}
		return InkHex("DeviceCMYK", ink)
	}
	want := map[string]int{"DeviceGray": 1, "DeviceRGB": 3, "DeviceCMYK": 4}[space]
	if want == 0 || len(values) != want {
		return "", false
	}
	channels := make([]float64, want)
	for index, text := range values {
		value, err := strconv.ParseFloat(text, 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			return "", false
		}
		channels[index] = math.Min(math.Max(value, 0), 1)
	}
	bytes := rgbBytes(channels)
	hex := fmt.Sprintf("#%02x%02x%02x", bytes[0], bytes[1], bytes[2])
	if hex == "#000000" {
		return "", true
	}
	return hex, true
}

func rgbBytes(channels []float64) [3]int {
	switch len(channels) {
	case 1:
		return [3]int{inkByte(channels[0]), inkByte(channels[0]), inkByte(channels[0])}
	case 3:
		return [3]int{inkByte(channels[0]), inkByte(channels[1]), inkByte(channels[2])}
	}
	key := channels[3]
	return [3]int{inkByte((1 - channels[0]) * (1 - key)), inkByte((1 - channels[1]) * (1 - key)), inkByte((1 - channels[2]) * (1 - key))}
}

func inkByte(value float64) int { return int(float64(value*255) + 0.5) }

// tryGroup is verify_group! rescue Failure: whether the group passes.
func tryGroup(attrs, page map[string]string) (ok bool) {
	defer rescueFailure(&ok)
	verifyGroup(attrs, page)
	return true
}

// verifyGroup: attrs are the group's attributes; page its page's, or nil.
func verifyGroup(attrs, page map[string]string) {
	defer rescueArgument("Invalid native text compositing group: ")
	expected := map[string]string{"isolated": "1", "knockout": "0", "blendmode": "Normal", "alpha": "1"}
	if !sameExcept(attrs, expected, "bbox") {
		rec.Fail("Unsupported native text compositing group")
	}
	box := floats(fetchAttribute(attrs, "bbox"))
	finite := len(box) == 4
	for _, value := range box {
		finite = finite && !isInfinite(value)
	}
	if !finite || !(box[0] < box[2]) || !(box[1] < box[3]) {
		rec.Fail("Invalid native text compositing bounds")
	}
	if page == nil || !floatsEqual(box, floats(page["mediabox"])) {
		rec.Fail("Text compositing group must cover the ordinary PDF page")
	}
}

func isInfinite(value float64) bool { return math.IsInf(value, 0) || math.IsNaN(value) }

func fetchAttribute(attrs map[string]string, name string) string {
	value, ok := attrs[name]
	if !ok {
		rec.RaiseArgument("key not found: %q", name)
	}
	return value
}

// sameExcept is attrs.except(key) == expected.
func sameExcept(attrs, expected map[string]string, key string) bool {
	count := 0
	for name, value := range attrs {
		if name == key {
			continue
		}
		count++
		if want, ok := expected[name]; !ok || want != value {
			return false
		}
	}
	return count == len(expected)
}

// verifyMediaGroup: media groups come from an embedded PDF figure's own transparency and
// soft-mask groups: isolated or knockout, with bounds that may exceed the page when an
// enclosing group lies within it. parents are the attributes of the enclosing groups.
func verifyMediaGroup(attrs, page map[string]string, parents []map[string]string) {
	defer rescueArgument("Invalid PDF media group: ")
	expected := map[string][]string{"isolated": {"0", "1"}, "knockout": {"0", "1"}, "blendmode": {"Normal"}}
	supported := len(attrs)-boolInt(hasKey(attrs, "bbox"))-boolInt(hasKey(attrs, "alpha")) == len(expected) &&
		hasKey(attrs, "alpha") && alphaValue(attrs) > 0 && alphaValue(attrs) <= 1
	for key, values := range expected {
		supported = supported && slices.Contains(values, attrs[key])
	}
	if !supported {
		rec.Fail("Unsupported PDF media compositing group")
	}
	box := geometry.Box(geometry.Floats(floats(fetchAttribute(attrs, "bbox"))))
	if page == nil {
		rec.Fail("PDF media group has no page")
	}
	bounds := geometry.Box(geometry.Floats(floats(fetchAttribute(page, "mediabox"))))
	inside := inPage(box, bounds) && box[0] < box[2] && box[1] < box[3]
	covered := false
	for _, parent := range parents {
		covered = covered || enclosing(parent, bounds)
	}
	if !inside && !covered {
		rec.Fail("Unbound PDF media group extent")
	}
}

func hasKey(attrs map[string]string, key string) bool {
	_, ok := attrs[key]
	return ok
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func inPage(box, bounds []float64) bool {
	return box[0] >= bounds[0] && box[1] >= bounds[1] && box[2] <= bounds[2] && box[3] <= bounds[3]
}

// enclosing: a figure group that is itself clipped within the page, rather than the ordinary
// page-covering group.
func enclosing(attrs map[string]string, bounds []float64) (ok bool) {
	defer rescueFailure(&ok)
	defer rescueArgument("")
	box := geometry.Box(geometry.Floats(floats(fetchAttribute(attrs, "bbox"))))
	return inPage(box, bounds) && !floatsEqual(box, bounds) && box[0] < box[2] && box[1] < box[3]
}
