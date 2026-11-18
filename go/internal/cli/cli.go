// Package cli is the relatex command line (Ruby's ProductCLI): its commands, their options
// as Ruby's OptionParser reads them, the user's configuration, and the reports and failures
// as the Ruby program prints them, so the two programs answer the same command the same way.
package cli

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"github.com/sarielhp/clihelp"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/exportwriter"
	"github.com/sarielhp/relatex/internal/fspath"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
)

// legacyCommands is ProductCLI::LEGACY_COMMANDS: the development commands of the Ruby program.
var legacyCommands = []string{"experiment", "inspect", "html", "convert-fixture"}

// Root is the program's resources: the repository, or the unpacked package.
var Root string

// Name is the name the program was run under: as rlx (a link to relatex) it publishes its products
// beside the main file like latexmk; as relatex it leaves them in the output directory.
var Name = "relatex"

// systemFailure is a Ruby SystemCallError: its message as Ruby words it.
type systemFailure struct{ message string }

func (f systemFailure) Error() string { return f.message }

// systemCall is a Go file-system error as Ruby's SystemCallError message for the call Ruby
// makes (rb_sysopen for File.read, rb_file_s_size for File.size, ...).
func systemCall(err error) systemFailure { return systemFailureAt(err, "rb_sysopen") }

func systemFailureAt(err error, function string) systemFailure {
	var path *os.PathError
	if errors.As(err, &path) {
		var errno syscall.Errno
		if errors.As(path.Err, &errno) {
			return systemFailure{rubyErrno(errno) + " @ " + function + " - " + path.Path}
		}
	}
	var link *os.LinkError
	if errors.As(err, &link) {
		var errno syscall.Errno
		if errors.As(link.Err, &errno) {
			return systemFailure{rubyErrno(errno) + " @ " + function + " - (" + link.Old + ", " + link.New + ")"}
		}
	}
	return systemFailure{err.Error()}
}

// rubyErrno is strerror as Ruby's Errno messages capitalize it.
func rubyErrno(errno syscall.Errno) string {
	text := errno.Error()
	if text == "" {
		return text
	}
	return strings.ToUpper(text[:1]) + text[1:]
}

// messages is where a command warns while it runs (Kernel#warn's $stderr).
var messages io.Writer = os.Stderr

// Run is ProductCLI#run (with bin/relatex's choice between it and the development
// commands): the exit status, after writing the report to stdout and messages to stderr.
func Run(arguments []string, stdout io.Writer, stderr *os.File) (status int) {
	messages = stderr
	conversionHuman = nil
	command.Trace("relatex begins")
	defer command.Trace("relatex ends")
	colour := colourful(stderr)
	defer finish(stdout, stderr, colour, &status)
	exportwriter.SetRoots(Root)
	if !handles(arguments) {
		rec.Fail("%s was a development command of the archived Ruby relatex; relatex does not have it", arguments[0])
	}
	if len(arguments) > 0 && arguments[0] == "verify" {
		rec.Fail("%s", acceptanceRefusal)
	}
	current := newSession(stdout, stderr)
	app := current.app()
	if err := app.Execute(legacySynctex(arguments)); err != nil {
		app.PrintError(err)
		if clihelp.IsUsageError(err) {
			return 2
		}
		return 1
	}
	if !current.ran {
		return 0
	}
	return current.report(stdout, stderr, colour)
}

// report prints what a command produced, its diagnostics on stderr and its report on stdout, and
// is the exit status (exitStatus).
func (s *session) report(stdout io.Writer, stderr io.Writer, colour bool) int {
	result := s.result
	switch {
	case s.convert.human:
		report := reportFile(rec.ToS(result.Get("directory")))
		write(stderr, humanErrors(result, report, colour))
		write(stdout, humanSummary(result, colour)+"\n")
	default:
		if lines := diagnosticLines(result); len(lines) > 0 {
			write(stderr, diagnosticsText(lines, colour)+"\n")
		}
		if summary != "" {
			write(stdout, summary+"\n")
		} else if s.human != nil && !s.asReport {
			write(stdout, s.human(result))
		} else {
			write(stdout, string(ojson.Pretty(result))+"\n")
		}
	}
	human := summary != "" || s.convert.human || (s.human != nil && !s.asReport)
	return exitStatus(rec.ToS(result.Get("status")), human)
}

// exitStatus is 1 for a failed run. A run that wrote its products with diagnostics is 3 in a
// machine report (--report), which the dev tools tell apart, and 0 for a person, as rlx and
// latexmk succeed when they wrote the products; the warnings are printed above.
func exitStatus(status string, human bool) int {
	switch status {
	case "failed":
		return 1
	case "diagnostic", "degraded", "unverified":
		if human {
			return 0
		}
		return 3
	}
	return 0
}

// handles is ProductCLI.handles?.
func handles(arguments []string) bool {
	if len(arguments) > 0 && arguments[0] == "inspect" {
		for _, argument := range arguments {
			if argument == "--stage" || strings.HasPrefix(argument, "--stage=") {
				return false
			}
		}
		return true
	}
	return len(arguments) == 0 || !slices.Contains(legacyCommands, arguments[0])
}

func diagnosticLines(result ojson.Object) []string {
	list, _ := result.Get("diagnostic_lines").([]any)
	lines := make([]string, 0, len(list))
	for _, line := range list {
		lines = append(lines, rec.ToS(line))
	}
	return lines
}

// finish turns what stopped a command into its output and exit status: an officious help or
// version exit, an option error (2), or a failure (1).
func finish(stdout io.Writer, stderr *os.File, colour bool, status *int) {
	recovered := recover()
	if recovered == nil {
		return
	}
	switch typed := recovered.(type) {
	case *exitRequest:
		write(stdout, typed.stdout)
		write(stderr, typed.stderr)
		*status = typed.status
	case usageFailure:
		warn(stderr, "Error: "+typed.message)
		*status = 2
	case rec.Failure:
		warn(stderr, failureOutput(typed.Message, colour))
		*status = 1
	case systemFailure:
		warn(stderr, failureOutput(typed.message, colour))
		*status = 1
	case error:
		warn(stderr, failureOutput(typed.Error(), colour))
		*status = 1
	default:
		panic(recovered)
	}
}

// warn is Kernel#warn: the message and a newline unless it ends with one.
func warn(stderr io.Writer, message string) {
	if !strings.HasSuffix(message, "\n") {
		message += "\n"
	}
	write(stderr, message)
}

func write(out io.Writer, text string) {
	if text != "" {
		_, _ = io.WriteString(out, text)
	}
}

// version is Relatex::VERSION.
func version() string {
	bytes, err := os.ReadFile(versionPath())
	if err != nil {
		panic(systemCall(err))
	}
	return rec.Strip(string(bytes))
}

// versionPath is the file holding the program's version.
func versionPath() string { return filepath.Join(Root, "VERSION") }

// usageFailure is a command line that parses but cannot be run: the command's own check of its options and words.
type usageFailure struct{ message string }

// exitRequest is a command's end with its own output and status.
type exitRequest struct {
	stdout, stderr string
	status         int
}

func (e *exitRequest) Error() string { return "exit" }

// fail stops a command whose options or words do not make sense together: a usage error (status 2).
func fail(message string) { panic(usageFailure{message}) }

// realpath is File.realpath, failing as Ruby does.
func realpath(path string) string {
	resolved, err := fspath.Real(path)
	if err != nil {
		panic(systemFailureAt(err, "rb_check_realpath_internal"))
	}
	absolute, err := filepath.Abs(resolved)
	if err != nil {
		panic(systemCall(err))
	}
	return absolute
}

// readFile is File.read.
func readFile(path string) []byte {
	bytes, err := os.ReadFile(path)
	if err != nil {
		panic(systemCall(err))
	}
	return bytes
}

// readOrdered is JSON.parse(File.read(path)), keys in the file's order.
func readOrdered(path string) any {
	value, err := ojson.Decode(strings.NewReader(string(readFile(path))))
	if err != nil {
		rec.Fail("%s", jsonMessage(err))
	}
	return value
}

// workspaceRoot is RunWorkspace.new(root).root: the directory, made if missing, never a symlink.
func workspaceRoot(root string) string {
	path := expandPath(root)
	if err := os.MkdirAll(path, 0o755); err != nil {
		panic(systemFailureAt(err, "rb_dir_s_mkdir"))
	}
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		rec.Fail("Workspace root is a symlink")
	}
	return realpath(path)
}

// helperPath is RuntimeTools.resolve: the helper's path, "" when absent.
func helperPath(name string) string {
	found := command.Resolve(name)
	if found != "" || name != "typst" {
		return found
	}
	candidates := []string{}
	if cargo := os.Getenv("CARGO_HOME"); cargo != "" {
		candidates = append(candidates, filepath.Join(cargo, "bin", name))
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".cargo", "bin", name))
	}
	for _, candidate := range candidates {
		if path := command.Executable(candidate); path != "" {
			return path
		}
	}
	quarto := command.Executable("quarto")
	if quarto == "" {
		return ""
	}
	resolved, err := fspath.Real(quarto)
	if err != nil {
		return ""
	}
	return command.Executable(filepath.Join(filepath.Dir(resolved), "tools", "x86_64", name))
}

// failureOutput is a failure as it is printed: for a person during a conversion that prints for one.
func failureOutput(message string, colour bool) string {
	if conversionHuman != nil {
		return withTeXNotice(message, humanFailure(message, colour), colour)
	}
	return withTeXNotice(message, failureText(message, colour), colour)
}
