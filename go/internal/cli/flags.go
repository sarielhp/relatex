package cli

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/sarielhp/clihelp"
	"github.com/spf13/pflag"
)

// The command line is declared with clihelp: each option below is a clihelp.Option whose handler
// runs once per appearance, in command-line order (the order of --markdown --html is the order of
// the formats written, and the last of --synctex and --no-synctex wins). A handler records what
// was asked; the command runs after the whole line parsed.

// funcValue is a pflag value that hands each appearance of its option to a function.
type funcValue struct {
	set  func(string) error
	text string
}

func (v *funcValue) String() string { return v.text }

func (v *funcValue) Set(text string) error {
	v.text = text
	return v.set(text)
}

func (v *funcValue) Type() string { return "string" }

// valueFlag is an option taking one value ("-o, --output <dir>"); a value the function refuses is a
// usage error.
func valueFlag(spec, description string, set func(string) error) clihelp.Option {
	return clihelp.Var(&funcValue{set: set}, spec, description)
}

// textFlag is an option whose value is a string, kept as given.
func textFlag(spec, description string, target *string) clihelp.Option {
	return valueFlag(spec, description, func(text string) error { *target = text; return nil })
}

// choiceFlag is an option whose value is one of values (exactly, no prefixes); the shell completes them.
func choiceFlag(spec, description string, values []string, set func(string)) clihelp.Option {
	option := valueFlag(spec, description, func(text string) error {
		if !slices.Contains(values, text) {
			return fmt.Errorf("must be one of [%s]", strings.Join(values, ", "))
		}
		set(text)
		return nil
	})
	option.Complete = func(string) []string { return values }
	return option
}

// textChoice is choiceFlag storing the chosen value.
func textChoice(spec, description string, values []string, target *string) clihelp.Option {
	return choiceFlag(spec, description, values, func(value string) { *target = value })
}

// integerFlag is an option whose value is a decimal integer.
func integerFlag(spec, description string, set func(int64)) clihelp.Option {
	return valueFlag(spec, description, func(text string) error {
		number, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return fmt.Errorf("%q is not an integer", text)
		}
		set(number)
		return nil
	})
}

// numberFlag is an option whose value is a decimal number.
func numberFlag(spec, description string, set func(float64)) clihelp.Option {
	return valueFlag(spec, description, func(text string) error {
		number, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return fmt.Errorf("%q is not a number", text)
		}
		set(number)
		return nil
	})
}

// listFlag is an option whose value is comma-separated words (empty ones dropped); each appearance
// replaces the list.
func listFlag(spec, description string, set func([]string)) clihelp.Option {
	return valueFlag(spec, description, func(text string) error {
		set(nonEmpty(strings.Split(text, ",")...))
		return nil
	})
}

// switchFlag is an option without a value ("--html"): set runs at each appearance.
func switchFlag(spec, description string, set func()) clihelp.Option {
	name := strings.TrimPrefix(spec, "--")
	return clihelp.Option{Flags: spec, Description: description, Binder: func(fs *pflag.FlagSet) error {
		fs.BoolFunc(name, description, func(text string) error { return needsNoValue(spec, text, set) })
		return nil
	}}
}

// toggleFlag is "--[no-]name": set runs with true for --name and false for --no-name.
func toggleFlag(spec, description string, set func(bool)) clihelp.Option {
	name := strings.TrimPrefix(spec, "--[no-]")
	return clihelp.Option{Flags: spec, Description: description, Binder: func(fs *pflag.FlagSet) error {
		fs.BoolFunc(name, description, func(text string) error {
			return needsNoValue("--"+name, text, func() { set(true) })
		})
		fs.BoolFunc("no-"+name, "Disable "+description, func(text string) error {
			return needsNoValue("--no-"+name, text, func() { set(false) })
		})
		_ = fs.MarkHidden("no-" + name)
		return nil
	}}
}

// needsNoValue runs set for a bare switch; pflag hands a BoolFunc "true" when none was written, and
// "--html=false" or "--html=1" is refused rather than read.
func needsNoValue(spec, text string, set func()) error {
	if text != "true" {
		return fmt.Errorf("%s takes no value", spec)
	}
	set()
	return nil
}

// optionalFlag is an option whose value may be omitted ("--pdf-ua[=route]"): bare, it reads bare.
func optionalFlag(spec, description, bare string, set func(string) error) clihelp.Option {
	return clihelp.Optional(valueFlag(spec, description, set), bare)
}

// nonEmpty drops the empty words of a list.
func nonEmpty(list ...string) []string {
	kept := make([]string, 0, len(list))
	for _, word := range list {
		if word != "" {
			kept = append(kept, word)
		}
	}
	return kept
}
