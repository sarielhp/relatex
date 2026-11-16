package browser

import (
	"bufio"
	"crypto/sha1" //nolint:gosec // RFC 6455's handshake digest in a test server.
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sarielhp/relatex/internal/rec"
)

func TestRewriteSocketURL(t *testing.T) {
	cases := []struct{ reported, base, want string }{
		{"ws://localhost/devtools/browser/abc", "http://192.0.2.10:9223", "ws://192.0.2.10:9223/devtools/browser/abc"},
		{"ws://localhost:9222/devtools/browser/abc", "http://100.92.239.12:9223", "ws://100.92.239.12:9223/devtools/browser/abc"},
		{"ws://127.0.0.1/devtools/browser/x", "http://10.1.2.3", "ws://10.1.2.3/devtools/browser/x"},
	}
	for _, c := range cases {
		got, err := rewriteSocketURL(c.reported, c.base)
		if err != nil || got != c.want {
			t.Errorf("rewriteSocketURL(%q, %q) = %q, %v; want %q", c.reported, c.base, got, err, c.want)
		}
	}
	if _, err := rewriteSocketURL("ws://localhost/x", "https://chrome.example"); err == nil {
		t.Error("only a plain http:// endpoint is supported")
	}
}

func TestSupported(t *testing.T) {
	for product, want := range map[string]bool{"Chrome/154.0.8037.92": true, "Chrome/156.0.8078.47": true,
		"HeadlessChrome/160.0.1.2": true, "Chrome/150.0.7000.1": false, "Firefox/140.0": false, "": false,
		// Edge (decision 129, D3): its major version is its Chromium's.
		"Edg/158.0.4370.16": true, "HeadlessEdg/158.0.4370.16": true, "Edg/150.0.1.2": false} {
		if supported(product) != want {
			t.Errorf("supported(%q) = %v", product, !want)
		}
	}
}

func TestEnvironmentEndpointOverridesConfiguration(t *testing.T) {
	Configure(Settings{Endpoint: "http://10.0.0.1:9223/", Sessions: 3})
	defer Configure(Settings{})
	t.Setenv(EndpointVariable, "")
	if settings := current(); settings.Endpoint != "http://10.0.0.1:9223" || settings.sessions() != 3 || settings.kind() != "remote" {
		t.Errorf("configured settings = %+v", settings)
	}
	t.Setenv(EndpointVariable, "http://192.0.2.10:9223")
	if settings := current(); settings.Endpoint != "http://192.0.2.10:9223" {
		t.Errorf("the environment's endpoint must win: %+v", settings)
	}
	Configure(Settings{})
	t.Setenv(EndpointVariable, "")
	if settings := current(); settings.remote() || settings.kind() != "local" || settings.sessions() < 2 {
		t.Errorf("default settings = %+v", settings)
	}
	if (Settings{Endpoint: "http://x:1"}).sessions() != remoteSessions {
		t.Error("a remote endpoint defaults to six sessions")
	}
}

func TestServedStaysInsideTheDirectory(t *testing.T) {
	root := t.TempDir()
	cases := map[string]string{Origin + "index.html": filepath.Join(root, "index.html"),
		Origin + "figs/a%20b.svg?x=1": filepath.Join(root, "figs", "a b.svg"), Origin + "../etc/passwd": filepath.Join(root, "etc", "passwd"),
		Origin: "", "https://example.com/index.html": "", "file:///etc/passwd": ""}
	for address, want := range cases {
		if got := served(root, address); got != want {
			t.Errorf("served(%q) = %q; want %q", address, got, want)
		}
	}
	if contentType("fonts/X.WOFF2") != "font/woff2" || contentType("a.bin") != "application/octet-stream" {
		t.Error("content types by extension")
	}
}

func TestFetchReplyAnswersFromTheDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<p>x</p>"), 0o644); err != nil {
		t.Fatal(err)
	}
	paused := func(address string) rec.Map {
		return rec.Map{"requestId": "r", "request": map[string]any{"url": address}}
	}
	method, params := fetchReply(root, paused(Origin+"index.html"))
	if method != "Fetch.fulfillRequest" || params["responseCode"] != int64(200) ||
		params["body"] != base64.StdEncoding.EncodeToString([]byte("<p>x</p>")) {
		t.Errorf("served file: %s %v", method, params)
	}
	if method, params = fetchReply(root, paused(Origin+"missing.png")); params["responseCode"] != int64(404) {
		t.Errorf("missing file: %s %v", method, params)
	}
	if method, _ = fetchReply(root, paused("https://example.com/")); method != "Fetch.failRequest" {
		t.Errorf("another origin must fail, got %s", method)
	}
}

func TestSessionSlotsCapConcurrentPages(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	settings := Settings{Endpoint: "http://192.0.2.10:9223", Sessions: 2}
	first, err := acquireSlot(settings)
	if err != nil {
		t.Fatal(err)
	}
	second, err := acquireSlot(settings)
	if err != nil {
		t.Fatal(err)
	}
	if held, _ := trySlots(slotDirectory(settings), settings.sessions()); held != nil {
		t.Fatal("a third page must wait while two slots are held")
	}
	first.release()
	third, err := trySlots(slotDirectory(settings), settings.sessions())
	if err != nil || third == nil {
		t.Fatalf("a released slot is free again: %v", err)
	}
	second.release()
	third.release()
	if !strings.HasSuffix(filepath.ToSlash(slotDirectory(settings)), "browser-sessions/http_192.0.2.10_9223") {
		t.Errorf("slot directory %s", slotDirectory(settings))
	}
}

// fakeCDP is a remote endpoint: /json/version naming ws://localhost as a proxied Chrome does, and
// a websocket that answers the commands a page needs and pauses two requests on navigation.
type fakeCDP struct {
	mu       sync.Mutex
	answers  map[string]rec.Map
	versions int
	dropOn   string // a method on which the connection ends unanswered, as a proxy or a crashed browser ends it
}

func (f *fakeCDP) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/json/version" {
			f.mu.Lock()
			f.versions++
			f.mu.Unlock()
			_, _ = io.WriteString(w, `{"Browser":"Chrome/156.0.8078.47","webSocketDebuggerUrl":"ws://localhost/devtools/browser/fake"}`)
			return
		}
		if r.URL.Path != "/devtools/browser/fake" {
			http.NotFound(w, r)
			return
		}
		conn, buffered, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		sum := sha1.Sum([]byte(r.Header.Get("Sec-WebSocket-Key") + handshakeGUID)) //nolint:gosec // RFC 6455.
		_, _ = io.WriteString(conn, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n"+
			"Sec-WebSocket-Accept: "+base64.StdEncoding.EncodeToString(sum[:])+"\r\n\r\n")
		f.serve(conn, buffered.Reader)
	})
}

var fakeResults = map[string]string{"Browser.getVersion": `{"product":"Chrome/156.0.8078.47"}`,
	"Target.createBrowserContext": `{"browserContextId":"C"}`, "Target.createTarget": `{"targetId":"T"}`,
	"Target.attachToTarget": `{"sessionId":"S"}`, "Page.navigate": `{"frameId":"T"}`}

func (f *fakeCDP) serve(conn net.Conn, reader *bufio.Reader) {
	for {
		payload, err := readClientFrame(reader)
		if err != nil {
			return
		}
		var message struct {
			ID     int64   `json:"id"`
			Method string  `json:"method"`
			Params rec.Map `json:"params"`
		}
		if json.Unmarshal(payload, &message) != nil || message.Method == f.dropOn {
			return
		}
		result, ok := fakeResults[message.Method]
		if !ok {
			result = "{}"
		}
		writeServerFrame(conn, `{"id":`+jsonNumber(message.ID)+`,"result":`+result+`}`)
		f.mu.Lock()
		f.answers[message.Method] = message.Params
		f.mu.Unlock()
		if message.Method == "Page.navigate" {
			writeServerFrame(conn, `{"method":"Fetch.requestPaused","sessionId":"S","params":{"requestId":"1","request":{"url":"`+Origin+`index.html"}}}`)
			writeServerFrame(conn, `{"method":"Fetch.requestPaused","sessionId":"S","params":{"requestId":"2","request":{"url":"https://example.com/x.css"}}}`)
		}
	}
}

func jsonNumber(value int64) string { bytes, _ := json.Marshal(value); return string(bytes) }

func readClientFrame(reader *bufio.Reader) ([]byte, error) {
	head := make([]byte, 2)
	if _, err := io.ReadFull(reader, head); err != nil {
		return nil, err
	}
	length := uint64(head[1] & 0x7F)
	switch length {
	case 126:
		extended := make([]byte, 2)
		if _, err := io.ReadFull(reader, extended); err != nil {
			return nil, err
		}
		length = uint64(binary.BigEndian.Uint16(extended))
	case 127:
		extended := make([]byte, 8)
		if _, err := io.ReadFull(reader, extended); err != nil {
			return nil, err
		}
		length = binary.BigEndian.Uint64(extended)
	}
	mask := make([]byte, 4)
	payload := make([]byte, length)
	if _, err := io.ReadFull(reader, mask); err != nil {
		return nil, err
	}
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, err
	}
	for index := range payload {
		payload[index] ^= mask[index%4]
	}
	if head[0]&0x0F == opClose {
		return nil, io.EOF
	}
	return payload, nil
}

func writeServerFrame(conn net.Conn, text string) {
	header := []byte{0x81}
	if size := len(text); size < 126 {
		header = append(header, byte(size))
	} else {
		header = append(header, 126, byte(size>>8), byte(size))
	}
	_, _ = conn.Write(append(header, text...))
}

// TestRemotePageIsServedThroughInterception connects to the fake endpoint as to the remote Chrome: the
// handshake's ws://localhost is rewritten, the page gets a context of its own and the viewport, and
// the paused requests are answered from the served directory or failed.
func TestRemotePageIsServedThroughInterception(t *testing.T) {
	fake := &fakeCDP{answers: map[string]rec.Map{}}
	server := httptest.NewServer(fake.handler(t))
	defer server.Close()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<p>served</p>"), 0o644); err != nil {
		t.Fatal(err)
	}
	browser, err := open(Settings{Endpoint: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	browser.Serve(root)
	browser.Page("Page.navigate", rec.Map{"url": pageURL("index.html")})
	fulfilled, failed := waitFor(fake, "Fetch.fulfillRequest"), waitFor(fake, "Fetch.failRequest")
	browser.Close()
	if fulfilled["body"] != base64.StdEncoding.EncodeToString([]byte("<p>served</p>")) || fulfilled["requestId"] != "1" {
		t.Errorf("index.html answered with %v", fulfilled)
	}
	if failed["requestId"] != "2" {
		t.Errorf("another origin answered with %v", failed)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.answers["Target.createTarget"]["browserContextId"] != "C" || fake.answers["Emulation.setDeviceMetricsOverride"] == nil ||
		fake.answers["Fetch.enable"] == nil || fake.answers["Target.disposeBrowserContext"]["browserContextId"] != "C" {
		t.Errorf("page set-up: %v", fake.answers)
	}
	if browser.version != "Chrome/156.0.8078.47" || fake.versions != 1 {
		t.Errorf("version %q after %d handshakes", browser.version, fake.versions)
	}
}

func waitFor(fake *fakeCDP, method string) rec.Map {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		fake.mu.Lock()
		params, ok := fake.answers[method]
		fake.mu.Unlock()
		if ok {
			return params
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil
}

// TestUnreachableEndpointIsRetriedThenReported: a browser that cannot be reached is tried three
// times and then reported as Unavailable, not raised.
func TestUnreachableEndpointIsRetriedThenReported(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv(EndpointVariable, "http://"+address)
	saved := backoff
	backoff = []time.Duration{time.Millisecond, time.Millisecond}
	defer func() { backoff = saved }()
	_, session, err := withPage(t.TempDir(), func(*Browser) int { return 1 })
	unavailable, ok := err.(*Unavailable)
	if !ok || session.Attempts != attempts || unavailable.Endpoint != "remote" ||
		!strings.Contains(unavailable.Error(), "browser endpoint unavailable") {
		t.Errorf("err = %v, session = %+v", err, session)
	}
}
