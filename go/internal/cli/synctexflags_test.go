package cli

import "testing"

func synctexOf(t *testing.T, arguments ...string) (string, []string) {
	t.Helper()
	options, rest := parsedConvert(t, arguments...)
	mode, _ := options["synctex"].(string)
	return mode, rest
}

// --synctex is the most precise mode; --synctex-native and --no-synctex are the other two.
func TestSynctexFlagsSelectTheModes(t *testing.T) {
	for _, c := range []struct {
		arguments []string
		mode      string
	}{
		{[]string{"--synctex"}, "word"},
		{[]string{"--no-synctex"}, "none"},
		{[]string{"--synctex-native"}, "native"},
		{[]string{}, ""},
		{[]string{"--synctex", "--no-synctex"}, "none"},
	} {
		if mode, _ := synctexOf(t, c.arguments...); mode != c.mode {
			t.Errorf("%v: mode %q, want %q", c.arguments, mode, c.mode)
		}
	}
}

func TestSynctexFlagLeavesTheInputAlone(t *testing.T) {
	mode, rest := synctexOf(t, "--synctex", "./paper")
	if mode != "word" || len(rest) != 1 || rest[0] != "./paper" {
		t.Errorf("mode %q, rest %v, want word and the input ./paper", mode, rest)
	}
}

// The former `--synctex MODE` and `--synctex=MODE` spellings keep working.
func TestFormerSynctexModeSpellingsStillWork(t *testing.T) {
	for arguments, want := range map[string]string{"native": "native", "word": "word", "none": "none"} {
		for _, form := range [][]string{{"--synctex", arguments}, {"--synctex=" + arguments}} {
			if mode, rest := synctexOf(t, form...); mode != want || len(rest) != 0 {
				t.Errorf("%v: mode %q rest %v, want %q", form, mode, rest, want)
			}
		}
	}
}
