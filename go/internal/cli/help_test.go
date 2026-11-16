package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/clihelp"
)

// run is Run with its streams captured: the status, stdout and stderr.
func run(t *testing.T, arguments ...string) (int, string, string) {
	t.Helper()
	if Root == "" {
		Root, _ = filepath.Abs("../../..")
	}
	stderr, err := os.Create(filepath.Join(t.TempDir(), "stderr"))
	if err != nil {
		t.Fatal(err)
	}
	defer stderr.Close()
	var stdout bytes.Buffer
	status := Run(arguments, &stdout, stderr)
	messages, err := os.ReadFile(stderr.Name())
	if err != nil {
		t.Fatal(err)
	}
	return status, stdout.String(), string(messages)
}

// commandWords are the words that name every command a user can run, subcommands included.
func commandWords() [][]string {
	var all [][]string
	var walk func(prefix []string, specs []commandSpec)
	walk = func(prefix []string, specs []commandSpec) {
		for _, spec := range specs {
			words := append(append([]string{}, prefix...), spec.name)
			all = append(all, words)
			walk(words, spec.subcommands)
		}
	}
	walk(nil, newSession(io.Discard, io.Discard).specs())
	return all
}

func TestEveryCommandHasAHelp(t *testing.T) {
	for _, words := range commandWords() {
		for _, flag := range []string{"--help", "-h"} {
			status, stdout, stderr := run(t, append(append([]string{}, words...), flag)...)
			if status != 0 || !strings.HasPrefix(stdout, "Usage:") || !strings.Contains(stdout, "relatex "+words[0]) || stderr != "" {
				t.Errorf("relatex %v %s: status %d, stdout %q, stderr %q", words, flag, status, stdout, stderr)
			}
		}
	}
	if status, stdout, _ := run(t, "--help"); status != 0 || !strings.HasPrefix(stdout, "Usage:  relatex [convert]") {
		t.Errorf("relatex --help: status %d, stdout %q", status, stdout)
	}
}

// TestCommandTreeIsAudited: clihelp's own check of the declarations (names, options, examples) passes,
// but for the width of one-line descriptions.
// TestEveryExampleIsValid: each example names a command and options that exist.
func TestEveryExampleIsValid(t *testing.T) {
	if err := newSession(io.Discard, io.Discard).app().ValidateAllExamples(); err != nil {
		t.Fatal(err)
	}
}

func TestCommandTreeIsAudited(t *testing.T) {
	err := clihelp.Audit(newSession(io.Discard, io.Discard).app())
	if err == nil {
		return
	}
	// The one-row rule for descriptions is clihelp's ideal; relatex's option texts are longer and wrap.
	for _, finding := range strings.Split(err.Error(), "\n") {
		if strings.Contains(finding, "layout problems:") || strings.Contains(finding, "columns, ") {
			continue
		}
		t.Error(strings.TrimSpace(finding))
	}
}

// TestHelpListsEveryCommandAndOption: the program's help names every command, the command's help
// every one of its options, and the archived acceptance suite's options are not shown.
func TestHelpListsEveryCommandAndOption(t *testing.T) {
	_, root, _ := run(t, "--help")
	for _, spec := range newSession(io.Discard, io.Discard).specs() {
		if !strings.Contains(root, "\n  "+spec.name+" ") {
			t.Errorf("the help does not list the command %s", spec.name)
		}
		_, help, _ := run(t, spec.name, "--help")
		for _, name := range longOptionNames(spec.options) {
			if !strings.Contains(help, name) {
				t.Errorf("relatex %s --help does not show %s", spec.name, name)
			}
		}
	}
	_, help, _ := run(t, "convert", "--help")
	for _, hidden := range []string{"--verify ", "--authored-proof", "--frozen-pdf", "verify,", "Ruby"} {
		if strings.Contains(help, hidden) {
			t.Errorf("the help shows %q", hidden)
		}
	}
}

// longOptionNames are the long names the options declare that the help shows (not the hidden ones).
func longOptionNames(options []clihelp.Option) []string {
	var names []string
	for _, option := range options {
		if option.Hidden {
			continue
		}
		for _, field := range strings.Fields(strings.ReplaceAll(option.Flags, ",", " ")) {
			if strings.HasPrefix(field, "--") {
				names = append(names, field)
			}
		}
	}
	return names
}

// TestVersionIsTheProgramAndItsVersion: --version names the program and the VERSION file's text.
func TestVersionIsTheProgramAndItsVersion(t *testing.T) {
	status, stdout, _ := run(t, "--version")
	if status != 0 || stdout != "relatex "+version()+"\n" {
		t.Errorf("--version: status %d, stdout %q", status, stdout)
	}
}

// TestUnknownCommandsAndOptionsAreUsageErrors: a mistyped command or option is status 2 with a
// suggestion, never a conversion.
func TestUnknownCommandsAndOptionsAreUsageErrors(t *testing.T) {
	for arguments, suggestion := range map[string]string{"conver": "convert", "lnt": "lint", "validat": "validate"} {
		status, _, stderr := run(t, arguments, "x")
		if status != 2 || !strings.Contains(stderr, `"`+suggestion+`"`) {
			t.Errorf("relatex %s x: status %d, stderr %q", arguments, status, stderr)
		}
	}
	if status, _, stderr := run(t, "convert", "--no-chache"); status != 2 || !strings.Contains(stderr, "no-chache") {
		t.Errorf("a mistyped option: status %d, stderr %q", status, stderr)
	}
}

// TestCompletionAndManualAreMounted: the shell scripts and the manual page come from clihelp.
func TestCompletionAndManualAreMounted(t *testing.T) {
	for _, arguments := range [][]string{{"completion", "bash"}, {"completion", "zsh"}, {"completion", "fish"}, {"manpage"}} {
		status, stdout, stderr := run(t, arguments...)
		if status != 0 || !strings.Contains(stdout, "relatex") || stderr != "" {
			t.Errorf("relatex %v: status %d, stderr %q", arguments, status, stderr)
		}
	}
	if _, stdout, _ := run(t, "completion", "bash"); !strings.Contains(stdout, "--hermetic") && !strings.Contains(stdout, "__complete") {
		t.Errorf("the bash completion does not complete relatex's options")
	}
}

func TestAcceptanceOptionsAreRefusedClearly(t *testing.T) {
	for _, arguments := range [][]string{{"--verify", "draw12"}, {"verify"}, {"verify", "--help"}, {"--frozen-pdf", "x.pdf"}} {
		status, _, stderr := run(t, arguments...)
		if status != 1 || !strings.Contains(stderr, "--verify-browser") || strings.Contains(stderr, "bin/relatex") {
			t.Errorf("relatex %v: status %d, stderr %q", arguments, status, stderr)
		}
	}
}
