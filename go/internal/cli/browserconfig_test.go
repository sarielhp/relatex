package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
)

// failure is the message of the Failure run raises, or "".
func failure(run func()) (message string) {
	defer func() {
		if value := recover(); value != nil {
			message = value.(rec.Failure).Message
		}
	}()
	run()
	return ""
}

func writeConfig(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.jsonc")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTemplateCarriesTheBrowserDefaults(t *testing.T) {
	values := readConfig(writeConfig(t, configTemplate))
	section, ok := values.Get("browser").(ojson.Object)
	if !ok || section.Get("endpoint") != nil || section.Get("sessions") != nil || section.Get("token_file") != nil {
		t.Errorf("template browser section = %v", values.Get("browser"))
	}
	if !rec.Equal(ojson.Plain(values), ojson.Plain(configDefaults())) {
		t.Errorf("the template differs from the defaults:\n%v\n%v", ojson.Plain(values), ojson.Plain(configDefaults()))
	}
}

func TestBrowserSectionIsValidated(t *testing.T) {
	cases := map[string]string{
		`{"browser": {"endpoint": "http://192.0.2.10:9223", "sessions": 6, "token_file": "~/.config/t"}}`: "",
		`{"browser": {"endpoint": null}}`:                             "",
		`{"browser": {"endpoint": "192.0.2.10"}}`:                     `"browser.endpoint" must be null or an http:// address`,
		`{"browser": {"sessions": 0}}`:                                `"browser.sessions" must be null or a number from 1 to 64`,
		`{"browser": {"session": 6}}`:                                 `unknown setting "browser.session"`,
		`{"browser": "http://192.0.2.10:9223"}`:                       `"browser" must be an object`,
		`{"browser": {"token_file": ""}}`:                             `"browser.token_file" must be null or a file name`,
		`{"browser": {"ace_endpoint": "http://192.0.2.10:9223/ace"}}`: "",
		`{"browser": {"ace_endpoint": "qed/ace"}}`:                    `"browser.ace_endpoint" must be null or an http:// address`,
	}
	for text, want := range cases {
		path := writeConfig(t, text)
		got := failure(func() { readConfig(path) })
		if want == "" && got != "" || want != "" && !strings.HasSuffix(got, want) {
			t.Errorf("%s: got %q, want %q", text, got, want)
		}
	}
}

func TestProjectBrowserSettingsMergeOverGlobal(t *testing.T) {
	merged := mergeSettings(configDefaults(), readConfig(writeConfig(t, `{"browser": {"endpoint": "http://10.0.0.2:9223"}}`)))
	section := merged.Get("browser").(ojson.Object)
	if section.Get("endpoint") != "http://10.0.0.2:9223" || !section.Has("sessions") || !section.Has("token_file") {
		t.Errorf("merged browser section = %v", section)
	}
}
