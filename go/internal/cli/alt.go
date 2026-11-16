package cli

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sarielhp/clihelp"

	"github.com/sarielhp/relatex/internal/alttext"
	"github.com/sarielhp/relatex/internal/figdesc"
	"github.com/sarielhp/relatex/internal/fspath"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
)

// altLong is the alt command's help (decision 086).
const altLong = "Figure descriptions in DIRECTORY/" + figdesc.FileName + ", read by the next conversion:\n" +
	"  relatex alt draft [DIRECTORY]     draft the missing ones with the configured command (alt_text.command)\n" +
	"  relatex alt review [DIRECTORY]    write " + alttext.ReviewName + " in the output directory: each figure with its description\n" +
	"  relatex alt accept NAME...|--all [DIRECTORY]  mark drafts reviewed: their text becomes yours and is never replaced\n" +
	"  relatex alt import FILE [DIRECTORY]  apply the review page's saved decisions: accepted drafts reviewed, edits yours\n" +
	"  relatex alt set NAME [DIRECTORY] --alt|--long|--details TEXT  write a figure's alternative, short description or details\n" +
	"  relatex alt geometry --pdf FILE [--page N] [--json]  what a figure page draws (groups, paths, marks, labels)"

// altSpec is relatex alt: drafting, reviewing and accepting figure descriptions.
func altSpec() commandSpec {
	return commandSpec{name: "alt", description: "Draft, review and accept the descriptions of figures", long: altLong,
		usage:       Name + " alt draft|review|accept|import|set|geometry ...",
		subcommands: []commandSpec{altDraftSpec(), altReviewSpec(), altAcceptSpec(), altImportSpec(), altSetSpec(), altGeometrySpec()}}
}

// altPlace is a paper directory and its output directory, from the arguments and the settings.
type altPlace struct {
	directory, output string
	settings          ojson.Object
}

func newAltPlace(rest []string, output string) altPlace {
	if len(rest) > 1 {
		fail("at most one paper directory is expected, not " + strings.Join(rest, " "))
	}
	place := altPlace{directory: ".", output: output}
	if len(rest) == 1 {
		place.directory = rest[0]
	}
	place.settings, _ = loadConfig(place.directory)
	if place.output == "" {
		place.output = lastProduct(place.directory, place.settings)
	}
	place.directory, place.output = expandPath(place.directory), expandPath(place.output)
	return place
}

func (p altPlace) sidecar() *figdesc.Sidecar {
	sidecar, err := figdesc.Read(filepath.Join(p.directory, figdesc.FileName))
	if err != nil {
		rec.Fail("%v", err)
	}
	return sidecar
}

// document is the last conversion's document, from the output directory's bundle.
func (p altPlace) document() rec.Map {
	path := filepath.Join(p.output, "document.json.gz")
	if _, err := os.Stat(path); err != nil {
		rec.Fail("No conversion in %s: convert the paper first (relatex), then run relatex alt", p.output)
	}
	return rec.AsMap(readBundle(path)["document"])
}

// altDraftRequest is what an alt draft command line asked for.
type altDraftRequest struct {
	output               string
	allowUpload, redraft bool
	limit                int64
}

func altDraftSpec() commandSpec {
	var request altDraftRequest
	options := []clihelp.Option{
		textFlag("-o, --output <dir>", "The conversion's product directory (default: the project cache's)", &request.output),
		switchFlag("--allow-upload", "Record your approval to send this paper's figures (with their text, geometry and labels) "+
			"to the command's service, then draft", func() { request.allowUpload = true }),
		integerFlag("--limit <n>", "Send at most N figures in this run; the rest wait for the next", func(value int64) { request.limit = value }),
		switchFlag("--redraft", "Draft again figures whose draft is current (never an entry you wrote or accepted)",
			func() { request.redraft = true }),
	}
	return commandSpec{name: "draft", description: "Draft a description for each figure of the last conversion that has none",
		long:  "Drafts a description for each figure of the last conversion that has none, with alt_text.command",
		usage: Name + " alt draft [DIRECTORY] [options]", options: options, args: clihelp.MaximumNArgs(1),
		human: altHuman("draft"), run: func(rest []string) ojson.Object { return altDraft(rest, request) }}
}

func altDraft(rest []string, request altDraftRequest) ojson.Object {
	output, allowUpload, limit, redraft := request.output, request.allowUpload, int(request.limit), request.redraft
	place := newAltPlace(rest, output)
	settings := altTextSettings(place.settings)
	if len(settings.command) == 0 {
		rec.Fail("No drafting command: set alt_text.command in %s (see the guide's Figure descriptions page)", globalConfigPath())
	}
	settings.checkKeyFile()
	document := place.document()
	sidecar := place.sidecar()
	approve(sidecar, settings, allowUpload)
	drafter := &alttext.Drafter{Command: settings.command, Model: settings.model, Timeout: settings.timeout, Today: today(),
		Env:   settings.environment(),
		Limit: limit, Redraft: redraft, Notify: func(key, outcome string) { warn(messages, "figure "+key+": "+outcome) }}
	outcome, err := drafter.Run(document, sidecar)
	if err != nil {
		rec.Fail("%v", err)
	}
	status := "success"
	if len(outcome.Failed) > 0 || len(outcome.TooLong) > 0 {
		status = "diagnostic"
	}
	for _, key := range outcome.TooLong {
		warn(messages, "alt.too_long: figure "+key+": the draft is over the length limits after asking again; shorten it in review")
	}
	return ojson.Object{{"status", status}, {"sidecar", sidecar.Path}, {"drafted", names(outcome.Drafted)},
		{"reused", names(outcome.Reused)}, {"cached", names(outcome.Cached)}, {"kept", names(outcome.Kept)},
		{"stale", names(outcome.Stale)}, {"waiting", names(outcome.Waiting)}, {"failed", anyList(outcome.Failed)}, {"too_long", names(outcome.TooLong)},
		{"next", "relatex alt review, then relatex alt accept NAME (or --all), then convert again"}}
}

// approve stops a command that sends figures off the machine until the author has approved it
// for this paper; --allow-upload records that approval in the sidecar.
func approve(sidecar *figdesc.Sidecar, settings altSettings, allowUpload bool) {
	if !settings.uploads || sidecar.Approval.Upload {
		return
	}
	if !allowUpload {
		rec.Fail("The drafting command sends figures off this machine (alt_text.uploads is true), each with its caption, the text "+
			"around its references and, for a vector figure, its geometry and label text. Approve it once for this "+
			"paper with relatex alt draft --allow-upload (recorded in %s), or set alt_text.uploads to false for a local command",
			figdesc.FileName)
	}
	approval := figdesc.Approval{Upload: true, Approved: today(), Command: filepath.Base(settings.command[0])}
	if err := sidecar.RecordApproval(approval); err != nil {
		rec.Fail("%v", err)
	}
	if err := sidecar.Write(); err != nil {
		panic(systemCall(err))
	}
}

func altReviewSpec() commandSpec {
	output := ""
	options := []clihelp.Option{textFlag("-o, --output <dir>", "The conversion's product directory (default: the project cache's)", &output)}
	return commandSpec{name: "review", description: "Write the page of every figure with its description, for review",
		long:  "Writes " + alttext.ReviewName + " in the output directory (the current directory unless the output setting says otherwise): each figure with its caption, the text citing it and its description",
		usage: Name + " alt review [DIRECTORY] [--output DIR]", options: options, args: clihelp.MaximumNArgs(1),
		human: altHuman("review"), run: func(rest []string) ojson.Object { return altReview(rest, output) }}
}

func altReview(rest []string, output string) ojson.Object {
	place := newAltPlace(rest, output)
	document := place.document()
	sidecar := place.sidecar()
	figures := figdesc.Figures(document)
	// The page is what the command was asked for: it goes to the output directory, not the product.
	directory := expandPath(rec.AsString(place.settings.Get("output")))
	if err := os.MkdirAll(directory, 0o755); err != nil {
		panic(systemCall(err))
	}
	page := filepath.Join(directory, alttext.ReviewName)
	if err := os.WriteFile(page, []byte(alttext.ReviewPage(figures, sidecar)), 0o644); err != nil {
		panic(systemCall(err))
	}
	counts := map[string][]any{}
	for _, state := range alttext.States(figures, sidecar) {
		counts[state.Class] = append(counts[state.Class], state.Key)
	}
	return ojson.Object{{"status", "success"}, {"page", page}, {"sidecar", sidecar.Path}, {"figures", int64(len(figures))},
		{"drafts", nonNil(counts["draft"])}, {"missing", nonNil(counts["missing"])}, {"stale", nonNil(counts["stale"])},
		{"done", nonNil(counts["done"])}}
}

// splitDirectory takes the paper directory out of the positional arguments, as alt draft and alt review
// read it: with --all the one positional is the directory; with names a last argument that is an
// existing directory is (after at least one name). --paper keeps working.
func splitDirectory(rest []string, directory string, all bool) ([]string, string) {
	if all && len(rest) == 1 {
		return nil, rest[0]
	}
	if !all && len(rest) > 1 && isDirectory(rest[len(rest)-1]) {
		return rest[:len(rest)-1], rest[len(rest)-1]
	}
	return rest, directory
}

func isDirectory(path string) bool {
	info, err := os.Stat(expandPath(path))
	return err == nil && info.IsDir()
}

func altAcceptSpec() commandSpec {
	directory, all := ".", false
	options := []clihelp.Option{
		textFlag("--paper <directory>", "The paper directory holding "+figdesc.FileName+" (default .)", &directory),
		switchFlag("--all", "Accept every draft", func() { all = true }),
	}
	return commandSpec{name: "accept", description: "Mark model drafts reviewed: their text becomes yours",
		long:  "Marks model drafts reviewed (provenance = \"reviewed\"): their text becomes yours",
		usage: Name + " alt accept NAME... | --all [DIRECTORY] [--paper DIRECTORY]", options: options, args: clihelp.MinimumNArgs(0),
		human: altHuman("accept"), run: func(rest []string) ojson.Object { return altAccept(rest, directory, all) }}
}

func altAccept(rest []string, directory string, all bool) ojson.Object {
	rest, directory = splitDirectory(rest, directory, all)
	if all == (len(rest) > 0) {
		fail("name the figures to accept, or give --all and no names")
	}
	place := altPlace{directory: expandPath(directory)}
	sidecar := place.sidecar()
	if all {
		rest = sidecar.Keys()
	}
	accepted, unchanged := []any{}, []any{}
	for _, key := range rest {
		done, err := sidecar.Accept(key, today())
		if err != nil {
			rec.Fail("%v", err)
		}
		if done {
			accepted = append(accepted, key)
		} else {
			unchanged = append(unchanged, key)
		}
	}
	if len(accepted) > 0 {
		if err := sidecar.Write(); err != nil {
			panic(systemCall(err))
		}
	}
	return ojson.Object{{"status", "success"}, {"sidecar", sidecar.Path}, {"accepted", accepted}, {"unchanged", unchanged}}
}

// altImport is relatex alt import: the decisions saved from the review page (decision 112) applied to
// the sidecar. Only the figures the author ticked are in the file; a decision about a text that
// changed since the page was written is refused.
func altImportSpec() commandSpec {
	directory := "."
	options := []clihelp.Option{textFlag("--paper <directory>", "The paper directory holding "+figdesc.FileName+" (default .)", &directory)}
	return commandSpec{name: "import", description: "Take the decisions saved from the review page into the sidecar",
		long: "Takes the decisions saved from " + alttext.ReviewName + " (" + alttext.ReviewFile + ") into " + figdesc.FileName + ":\n" +
			"a draft accepted as shown becomes reviewed, a text you edited or wrote becomes yours; nothing else changes",
		usage: Name + " alt import FILE [DIRECTORY] [--paper DIRECTORY]", options: options, args: clihelp.RangeArgs(1, 2),
		human: altHuman("import"), run: func(rest []string) ojson.Object { return altImport(rest, directory) }}
}

func altImport(rest []string, directory string) ojson.Object {
	if len(rest) == 2 {
		directory = rest[1]
	}
	decisions, err := figdesc.ReadReview(expandPath(rest[0]))
	if err != nil {
		rec.Fail("%v", err)
	}
	sidecar := altPlace{directory: expandPath(directory)}.sidecar()
	outcome, err := sidecar.Import(decisions, today())
	if err != nil {
		rec.Fail("%v", err)
	}
	if len(outcome.Accepted)+len(outcome.Edited) > 0 {
		if err := sidecar.Write(); err != nil {
			panic(systemCall(err))
		}
	}
	refused := ojson.Object{}
	for _, decision := range decisions {
		if reason, ok := outcome.Refused[decision.Key]; ok {
			refused = append(refused, ojson.Pair{decision.Key, reason})
		}
	}
	return ojson.Object{{"status", "success"}, {"sidecar", sidecar.Path}, {"accepted", names(outcome.Accepted)},
		{"edited", names(outcome.Edited)}, {"unchanged", names(outcome.Unchanged)}, {"refused", refused}}
}

// altSetSpec is relatex alt set: a figure's alternative, short description (decision 110) or details
// (decision 125) written into the sidecar, the rest of its entry kept.
func altSetSpec() commandSpec {
	directory := "."
	fields := map[string]string{}
	field := func(key string) func(string) error {
		return func(value string) error { fields[key] = value; return nil }
	}
	options := []clihelp.Option{
		textFlag("--paper <directory>", "The paper directory holding "+figdesc.FileName+" (default .)", &directory),
		valueFlag("--alt <text>", "The alternative: one sentence of at most 150 characters", field("alt")),
		valueFlag("--long <text>", "The short description: at most 40 words, purpose first", field("long")),
		valueFlag("--details <text>", "Details beyond the short description, only when a reader needs them (HTML and EPUB show them)",
			field("details")),
	}
	return commandSpec{name: "set", description: "Write a figure's alternative, short description or details into the sidecar",
		long:  "Writes a figure's text into " + figdesc.FileName + "; --long \"\" or --details \"\" removes it. A draft stays a draft until accepted",
		usage: Name + " alt set NAME [DIRECTORY] [--alt TEXT] [--long TEXT] [--details TEXT] [--paper DIRECTORY]", options: options,
		args: clihelp.RangeArgs(1, 2), human: altHuman("set"), run: func(rest []string) ojson.Object { return altSet(rest, directory, fields) }}
}

func altSet(rest []string, directory string, fields map[string]string) ojson.Object {
	if len(fields) == 0 {
		fail("give --alt, --long or --details")
	}
	if len(rest) == 2 {
		directory = rest[1]
	}
	sidecar := altPlace{directory: expandPath(directory)}.sidecar()
	if err := sidecar.SetTexts(rest[0], fields); err != nil {
		rec.Fail("%v", err)
	}
	if err := sidecar.Write(); err != nil {
		panic(systemCall(err))
	}
	entry := sidecar.Entries[rest[0]]
	result := ojson.Object{{"status", "success"}, {"sidecar", sidecar.Path}, {"figure", rest[0]}}
	if entry != nil {
		result = append(result, ojson.Object{{"alt", entry.Alt}, {"long", entry.Long}, {"details", entry.Details}, {"provenance", entry.Provenance}}...)
	}
	return result
}

func today() string { return time.Now().Format("2006-01-02") }

func names(list []string) []any {
	result := make([]any, len(list))
	for index, name := range list {
		result[index] = name
	}
	return result
}

func anyList(maps []rec.Map) []any {
	result := make([]any, len(maps))
	for index, item := range maps {
		result[index] = ojson.Object{{"figure", item["figure"]}, {"reason", item["reason"]}}
	}
	return result
}

func nonNil(list []any) []any {
	if list == nil {
		return []any{}
	}
	return list
}

// altSettings is the alt_text section of the global configuration.
type altSettings struct {
	command []string
	uploads bool
	model   string
	timeout time.Duration
	describerSettings
}

func altTextSettings(settings ojson.Object) altSettings {
	section, _ := settings.Get("alt_text").(ojson.Object)
	result := altSettings{uploads: section.Get("uploads") != false, timeout: 120 * time.Second}
	list, _ := section.Get("command").([]any)
	for _, item := range list {
		result.command = append(result.command, rec.AsString(item))
	}
	if len(result.command) > 0 {
		result.command[0] = expandHome(result.command[0])
		result.model = filepath.Base(result.command[0])
	}
	result.describerSettings = readDescriberSettings(section)
	if len(result.models) > 0 {
		result.model = result.models[0]
	}
	if model, ok := section.Get("model").(string); ok {
		result.model = model
	}
	if seconds, ok := section.Get("timeout_seconds").(int64); ok {
		result.timeout = time.Duration(seconds) * time.Second
	}
	return result
}

// expandHome is a leading ~/ as the home directory; a bare program name stays one, for PATH.
func expandHome(path string) string {
	if path != "~" && fspath.ExpandHome(path) != path {
		return expandPath(path)
	}
	return path
}

// validateAltText checks the "alt_text" section of the global configuration.
func validateAltText(value any, path string) {
	section, ok := value.(ojson.Object)
	if !ok {
		rec.Fail("%s: \"alt_text\" must be an object", path)
	}
	for _, pair := range section {
		valid := true
		switch pair.Key() {
		case "command":
			valid = pair.Value() == nil || stringList(pair.Value())
		case "uploads":
			valid = pair.Value() == true || pair.Value() == false
		case "model":
			text, isString := pair.Value().(string)
			valid = pair.Value() == nil || isString && text != ""
		case "endpoint":
			valid = httpURL(pair.Value())
		case "models":
			valid = stringList(pair.Value())
		case "key_file":
			valid = nonEmptyString(pair.Value())
		case "timeout_seconds":
			number, isNumber := pair.Value().(int64)
			valid = isNumber && number >= 1 && number <= 3600
		default:
			rec.Fail("%s: unknown setting \"alt_text.%s\"", path, pair.Key())
		}
		if !valid {
			rec.Fail("%s: \"alt_text.%s\" must be %s", path, pair.Key(), altExpected[pair.Key()])
		}
	}
}

var altExpected = map[string]string{"command": "null or a list of strings (the program, then its arguments)",
	"uploads": "true or false", "model": "null or a name", "timeout_seconds": "a number of seconds from 1 to 3600",
	"endpoint": "an http:// or https:// address", "models": "a non-empty list of model names",
	"key_file": "a file name (the file holding the API key, mode 600)"}

func stringList(value any) bool {
	list, ok := value.([]any)
	if !ok || len(list) == 0 {
		return false
	}
	for _, item := range list {
		if text, isString := item.(string); !isString || text == "" {
			return false
		}
	}
	return true
}

// refuseProjectAltText: a paper's own relatex/config.jsonc may not choose a program to run or say that
// it keeps figures local; a paper from someone else could otherwise run code or skip the approval.
func refuseProjectAltText(values ojson.Object, path string) {
	if values.Has("alt_text") {
		rec.Fail("%s: \"alt_text\" may be set only in the global configuration (%s)", path, globalConfigPath())
	}
}
