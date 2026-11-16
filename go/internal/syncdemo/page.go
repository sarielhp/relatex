package syncdemo

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

//go:embed demo.js
var demoScript string

//go:embed demo.css
var demoStyle string

var policy = regexp.MustCompile(`<meta http-equiv="Content-Security-Policy"[^>]*>`)

var bodyOpen = regexp.MustCompile(`<body[^>]*>`)

// demoPolicy is the export's content security policy with the demo's one inline script allowed.
const demoPolicy = `<meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src data: 'self' file:; ` +
	`style-src 'unsafe-inline'; script-src 'self' 'unsafe-inline' file:; base-uri 'none'; form-action 'none'"/>`

// Page is the demo page for the export's index.html: the annotated paper beside the source, with the
// demo's style and script inline. A page without a body or head is refused.
func Page(index string, data *Data) (string, Stats, error) {
	if !bodyOpen.MatchString(index) || !strings.Contains(index, "</body>") || !strings.Contains(index, "</head>") {
		return "", Stats{}, fmt.Errorf("the HTML has no head and body to build the demo page on")
	}
	annotated, stats := Annotate(redactPage(index), data)
	page := policy.ReplaceAllLiteralString(annotated, demoPolicy)
	page = strings.Replace(page, "</head>", "<style>\n"+demoStyle+"</style></head>", 1)
	page = bodyOpen.ReplaceAllStringFunc(page, func(open string) string { return open + `<div id="demo"><div id="paper">` })
	tail := `</div><aside id="source" aria-label="LaTeX source">` + sourcePane(data.Sources) +
		`<p id="status" role="status" aria-live="polite"></p></aside></div><script>` + "\n" + demoScript + `</script></body>`
	cut := strings.LastIndex(page, "</body>")
	return page[:cut] + tail + page[cut+len("</body>"):], stats, nil
}

// skipped are the export's receipts, which the demo directory does not repeat.
var skipped = map[string]bool{"index.html": true, "export.json": true, "stage.json": true}

// Write builds the demo of a finished product in out: index.html with the page, and every other
// file of the product's html directory (figs/, fonts/, preview.js, accessibility.html) beside it.
func Write(product, out string) (Stats, error) {
	exported := filepath.Join(product, "html")
	data, err := Load(product)
	if err != nil {
		return Stats{}, err
	}
	index, err := os.ReadFile(filepath.Join(exported, "index.html"))
	if err != nil {
		return Stats{}, fmt.Errorf("%w (the demo reads the product's html directory)", err)
	}
	page, stats, err := Page(string(index), data)
	if err != nil {
		return Stats{}, err
	}
	if err := copyTree(exported, out); err != nil {
		return Stats{}, err
	}
	return stats, os.WriteFile(filepath.Join(out, "index.html"), []byte(page), 0o644)
}

// copyTree copies the files of from into to, the receipts aside.
func copyTree(from, to string) error {
	if err := os.MkdirAll(to, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(from)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		source, target := filepath.Join(from, entry.Name()), filepath.Join(to, entry.Name())
		switch {
		case entry.IsDir():
			err = copyTree(source, target)
		case skipped[entry.Name()]:
		default:
			err = copyFile(source, target)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func copyFile(from, to string) error {
	bytes, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return os.WriteFile(to, bytes, 0o644)
}
