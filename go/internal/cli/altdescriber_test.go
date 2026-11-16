package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDescriberSettingsValidated(t *testing.T) {
	config := `{"alt_text": {"command": ["describe"], "endpoint": "http://tqed:4000/v1/chat/completions", ` +
		`"models": ["lumen/a", "lumen/b"], "key_file": "~/.config/auth/key"}}`
	got := altTextSettings(mergeSettings(configDefaults(), readConfig(writeConfig(t, config))))
	home, _ := os.UserHomeDir()
	if got.model != "lumen/a" || got.keyFile != filepath.Join(home, ".config/auth/key") || len(got.models) != 2 ||
		got.endpoint != "http://tqed:4000/v1/chat/completions" {
		t.Errorf("settings %+v", got)
	}
	bad := map[string]string{
		`{"alt_text": {"endpoint": "tqed:4000"}}`: `"alt_text.endpoint" must be an http`,
		`{"alt_text": {"endpoint": "ftp://h/x"}}`: `"alt_text.endpoint" must be an http`,
		`{"alt_text": {"models": []}}`:            `"alt_text.models" must be a non-empty list`,
		`{"alt_text": {"models": ["a", 3]}}`:      `"alt_text.models" must be a non-empty list`,
		`{"alt_text": {"key_file": ""}}`:          `"alt_text.key_file" must be a file name`,
		`{"alt_text": {"key_file": ["x"]}}`:       `"alt_text.key_file" must be a file name`,
	}
	for config, want := range bad {
		if message := failure(func() { readConfig(writeConfig(t, config)) }); !strings.Contains(message, want) {
			t.Errorf("%s: %q, want %q", config, message, want)
		}
	}
}

func TestDescriberEnvironment(t *testing.T) {
	full := describerSettings{endpoint: "https://h/v1", models: []string{"a", "b"}, keyFile: "/k"}
	want := "RELATEX_DESCRIBER_ENDPOINT=https://h/v1|RELATEX_DESCRIBER_MODELS=a,b|RELATEX_DESCRIBER_KEY_FILE=/k"
	if got := strings.Join(full.environment(), "|"); got != want {
		t.Errorf("environment %s", got)
	}
	if got := (describerSettings{}).environment(); len(got) != 0 {
		t.Errorf("unset settings set variables: %v", got)
	}
}
