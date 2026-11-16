package cli

import (
	"io"
	"os"
	"strings"

	"github.com/sarielhp/clihelp"

	"github.com/sarielhp/relatex/internal/ojson"
)

// commandSpec is one command of the program: how its line is read and what it runs.
type commandSpec struct {
	name, description, long, usage string
	hidden                         bool
	validator                      clihelp.OptionsValidator
	options                        []clihelp.Option
	args                           clihelp.ArgsValidator
	// run runs the command on its positional words; it returns the report printed on stdout, or
	// stops with a panic (a failure, or an exitRequest that carries its own output and status).
	run func(words []string) ojson.Object
	// human is the short text the command prints without --report; nil keeps its own output.
	human       func(result ojson.Object) string
	subcommands []commandSpec
}

// session is one run of the program: the streams, and what the command that ran produced.
type session struct {
	stdout, stderr io.Writer
	convert        *convertFlags
	// parseOnly reads the command line without running a command (the tests' view of the parse):
	// path and words are then what the line named.
	parseOnly bool
	path      []string
	words     []string
	ran       bool
	result    ojson.Object
	// asReport is --report of a command with a human view; human is that view of the command that ran.
	asReport bool
	human    func(result ojson.Object) string
}

func newSession(stdout, stderr io.Writer) *session {
	return &session{stdout: stdout, stderr: stderr, convert: newConvertFlags()}
}

// specs are the commands of the program, in the order the help lists them.
func (s *session) specs() []commandSpec {
	convert := s.convert.spec(false)
	return []commandSpec{convert, s.convert.replaySpec(), s.withReport(exportSpec("export")), s.withReport(exportSpec("reconstruct")),
		s.withReport(validateSpec()), s.withReport(inspectSpec()), s.withReport(syncSpec()), s.withReport(capabilitiesSpec()),
		s.withReport(layersSpec()), s.withReport(configSpec()), s.withReport(altSpec()), lintSpec(), synctexSpec(), s.withReport(demoSyncSpec()),
		s.withReport(tagPDFSpec()), doctorSpec(), bibcleanSpec()}
}

// app is the program as clihelp reads it: the root converts (`relatex paper.tex`), every other
// command is named, and `convert` is also a command of its own with the same options.
func (s *session) app() *clihelp.App {
	specs := s.specs()
	commands := make([]clihelp.Command, 0, len(specs)+2)
	for _, spec := range specs {
		commands = append(commands, s.command(spec, nil))
	}
	commands = append(commands, clihelp.CompletionCommand(), clihelp.ManPageCommand())
	root := specs[0]
	return &clihelp.App{
		Name: Name, Description: root.description, GlobalNote: root.long + "\n\nRun as rlx (a link to relatex) it works like latexmk: `rlx paper.tex` writes the PDF and its SyncTeX file beside paper.tex.", UsageLine: Name + " [convert] [ZIP_OR_DIRECTORY] [options]",
		Version: programVersion(), Stdout: s.stdout, Stderr: s.stderr,
		Options: root.options, Args: clihelp.MaximumNArgs(1), PositionalFilter: inputWord, Commands: commands,
		Run: s.runner(root, nil), Examples: rootExamples(), EnableExamplesFlag: true,
	}
}

// command is a spec as a clihelp command; parents are the names above it.
func (s *session) command(spec commandSpec, parents []string) clihelp.Command {
	path := append(append([]string{}, parents...), spec.name)
	command := clihelp.Command{Name: spec.name, Description: spec.description, LongDescription: spec.long, UsageLine: spec.usage,
		Hidden: spec.hidden, Options: spec.options, Args: spec.args, Examples: examplesFor(path), OptionsValidator: spec.validator}
	if spec.run != nil {
		command.Run = s.runner(spec, path)
	}
	for _, sub := range spec.subcommands {
		command.Subcommands = append(command.Subcommands, s.command(sub, path))
	}
	return command
}

// runner is a command's Run: it runs the command on its words and keeps the report, unless the
// session only parses.
func (s *session) runner(spec commandSpec, path []string) func(*clihelp.Context) error {
	return func(context *clihelp.Context) error {
		s.path, s.words = path, context.Args
		if s.parseOnly {
			return nil
		}
		if spec.name != "config" {
			configureTools()
		}
		s.human = spec.human
		s.result, s.ran = spec.run(context.Args), true
		return nil
	}
}

// inputWord says whether the root's first word is an input rather than a mistyped command: a path
// (it has a dot or a slash) or something that exists.
func inputWord(word string) bool {
	if strings.ContainsAny(word, `./\`) {
		return true
	}
	_, err := os.Stat(word)
	return err == nil
}

// programVersion is the version the program reports, empty when its VERSION file is unreadable.
func programVersion() string {
	bytes, err := os.ReadFile(versionPath())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(bytes))
}
