package browser

import (
	"encoding/base64"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/sarielhp/relatex/internal/rec"
)

// Origin is where the browser finds the page under check: every request is paused by CDP's Fetch
// domain and answered from the export directory, the same way for a local and a remote browser.
// Any other address fails as it would offline.
const Origin = "https://relatex.local/"

// browserFavicon is the icon Chrome asks an http(s) origin for by itself, which the page never
// links: the one request at the origin that is not the page's own.
const browserFavicon = Origin + "favicon.ico"

// contentTypes are the media types of the files an export holds, by extension.
var contentTypes = map[string]string{".html": "text/html; charset=utf-8", ".css": "text/css; charset=utf-8",
	".js": "text/javascript; charset=utf-8", ".json": "application/json", ".svg": "image/svg+xml", ".png": "image/png",
	".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif", ".webp": "image/webp", ".pdf": "application/pdf",
	".woff2": "font/woff2", ".woff": "font/woff", ".otf": "font/otf", ".ttf": "font/ttf", ".xhtml": "application/xhtml+xml"}

// interceptPatterns pause every request of the page.
var interceptPatterns = rec.Map{"patterns": []any{rec.Map{"urlPattern": "*", "requestStage": "Request"}}}

// pageURL is the address of a file of the served directory.
func pageURL(name string) string { return Origin + name }

// served is the file a request's address names inside root, or "" when it names none (another
// origin, or a path that leaves the directory).
func served(root, address string) string {
	if root == "" || !strings.HasPrefix(address, Origin) {
		return ""
	}
	parsed, err := url.Parse(address)
	if err != nil {
		return ""
	}
	name := path.Clean("/" + parsed.Path)
	if name == "/" {
		return ""
	}
	return filepath.Join(root, filepath.FromSlash(name[1:]))
}

// contentType is the media type of a served file.
func contentType(file string) string {
	if kind, ok := contentTypes[strings.ToLower(filepath.Ext(file))]; ok {
		return kind
	}
	return "application/octet-stream"
}

// fetchReply is the Fetch command that answers a paused request: the file's bytes, a 404 for a missing
// file of the origin, or a failure for any other address.
func fetchReply(root string, params rec.Map) (string, rec.Map) {
	id, _ := params["requestId"].(string)
	request, _ := params["request"].(map[string]any)
	address, _ := request["url"].(string)
	file := served(root, address)
	if file == "" {
		return "Fetch.failRequest", rec.Map{"requestId": id, "errorReason": "InternetDisconnected"}
	}
	body, err := os.ReadFile(file)
	if err != nil {
		return "Fetch.fulfillRequest", rec.Map{"requestId": id, "responseCode": int64(404),
			"responseHeaders": []any{rec.Map{"name": "Content-Type", "value": "text/plain"}}, "body": ""}
	}
	return "Fetch.fulfillRequest", rec.Map{"requestId": id, "responseCode": int64(200),
		"responseHeaders": []any{rec.Map{"name": "Content-Type", "value": contentType(file)},
			rec.Map{"name": "Cache-Control", "value": "no-store"}},
		"body": base64.StdEncoding.EncodeToString(body)}
}
