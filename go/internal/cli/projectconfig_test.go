package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/ojson"
)

func TestProjectTemplateSetsNothing(t *testing.T) {
	if values := readConfig(writeConfig(t, projectTemplate())); len(values) != 0 {
		t.Errorf("the project template sets %v", ojson.Plain(values))
	}
	uncommented := strings.Replace(projectTemplate(), `// "jobs": 2,`, `"jobs": 4,`, 1)
	if values := readConfig(writeConfig(t, uncommented)); len(values) != 1 || values.Get("jobs") != int64(4) {
		t.Errorf("an uncommented line sets %v", ojson.Plain(values))
	}
}

// TestInitProjectKeepsTheGlobalSettings: a paper's fresh relatex/config.jsonc leaves the global
// browser endpoint and formats in force.
func TestInitProjectKeepsTheGlobalSettings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	global := `{"formats": ["epub"], "browser": {"endpoint": "http://192.0.2.10:9223"}}`
	if err := os.MkdirAll(filepath.Join(home, "relatex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "relatex", "config.jsonc"), []byte(global), 0o644); err != nil {
		t.Fatal(err)
	}
	paper := t.TempDir()
	report := commandResult("config", paper, "--init-project")
	if report.Get("written") != filepath.Join(paper, projectConfig) {
		t.Fatalf("config --init-project reported %v", ojson.Plain(report))
	}
	settings, files := loadConfig(paper)
	browser, _ := settings.Get("browser").(ojson.Object)
	if len(files) != 2 || browser.Get("endpoint") != "http://192.0.2.10:9223" {
		t.Errorf("the project file overrode the global endpoint: %v", ojson.Plain(settings))
	}
	if formats, _ := settings.Get("formats").([]any); len(formats) != 1 || formats[0] != "epub" {
		t.Errorf("the project file overrode the global formats: %v", formats)
	}
}
