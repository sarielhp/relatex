package browser

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
)

// MinimumMajor is the oldest Chromium the checks support. The checks compare what a reader
// notices, not one build's exact boxes, so any later Chrome qualifies; the version each check
// ran on is recorded with its evidence (decision 079).
const MinimumMajor = 154

// ferrumVersion is BrowserCheck::FERRUM, which the Ruby reports name.
const ferrumVersion = "0.18.0"

// timeout is ferrum's timeout and protocol_timeout here (120 s).
const timeout = 120 * time.Second

// Viewport is the page size every check lays out in, on a local and a remote browser alike.
var viewport = rec.Map{"width": int64(1200), "height": int64(1000), "deviceScaleFactor": int64(1), "mobile": false}

// chromiumFlags are ferrum 0.18's Chrome defaults (Options::Chrome::DEFAULT_OPTIONS), which
// the Ruby checks launch Chromium with.
var chromiumFlags = []string{"--allow-pre-commit-input", "--disable-background-networking",
	"--disable-background-timer-throttling", "--disable-backgrounding-occluded-windows",
	"--disable-blink-features=AutomationControlled", "--disable-breakpad",
	"--disable-client-side-phishing-detection", "--disable-component-extensions-with-background-pages",
	"--disable-component-update", "--disable-crash-reporter", "--disable-default-apps", "--disable-dev-shm-usage",
	"--disable-extensions",
	"--disable-features=site-per-process,IsolateOrigins,TranslateUI,Translate,MacAppCodeSignClone," +
		"InterestFeedContentSuggestion,OptimizationHints,AcceptCHFrame,MediaRouter",
	"--disable-field-trial-config", "--disable-hang-monitor", "--disable-infobars", "--disable-ipc-flooding-protection",
	"--disable-popup-blocking", "--disable-prompt-on-repost", "--disable-renderer-backgrounding",
	"--disable-search-engine-choice-screen", "--disable-session-crashed-bubble", "--disable-site-isolation-trials",
	"--disable-smooth-scrolling", "--disable-sync", "--disable-translate", "--disable-web-security", "--enable-automation",
	"--enable-features=NetworkService,NetworkServiceInProcess", "--force-color-profile=srgb", "--headless",
	"--hide-scrollbars", "--keep-alive-for-test", "--metrics-recording-only", "--mute-audio", "--no-crash-upload",
	"--no-crashpad", "--no-default-browser-check", "--no-first-run", "--no-startup-window", "--password-store=basic",
	"--remote-allow-origins=*", "--safebrowsing-disable-auto-update", "--use-mock-keychain"}

var (
	listening = regexp.MustCompile(`DevTools listening on (ws://.*[a-zA-Z0-9-]{36})`)
	product   = regexp.MustCompile(`^(?:Headless)?(?:Chrome|Edg)/(\d+)\.\d+\.\d+\.\d+$`)
)

// errUnsupported is a browser older than MinimumMajor, or neither Chrome nor Edge: no retry helps.
var errUnsupported = errors.New("Browser outside the supported profile")

// errConnectionLost is a connection that ended while commands waited for their answers. Its message
// begins the failure, so a retry (transientFailures) recognizes it whatever command was waiting.
var errConnectionLost = errors.New("browser connection lost")

// event is a CDP event of a session.
type event struct {
	session, method string
	params          rec.Map
}

// reply is a command's response: its result, or its error.
type reply struct {
	result any
	err    error
}

// Browser is one page in a browser context of its own, driven over CDP: in a Chromium started for
// it, or on a remote endpoint shared with other checks. Every request of the page is answered
// from the served directory (serve.go).
type Browser struct {
	process  *exec.Cmd
	group    *command.Group
	userData string
	remote   *endpoint
	socket   *socket
	context  string
	session  string
	frame    string
	version  string
	root     string

	mu       sync.Mutex
	nextID   int64
	pending  map[int64]chan reply
	watchers []chan event
	done     chan struct{}
}

// supported is whether Browser.getVersion's product is a Chrome (or an Edge, decision 129) the checks support.
func supported(name string) bool {
	match := product.FindStringSubmatch(name)
	if match == nil {
		return false
	}
	major, err := strconv.Atoi(match[1])
	return err == nil && major >= MinimumMajor
}

// open connects a browser as the settings say, with a fresh context and page; a failure to
// reach or start it is an error (errUnsupported for a browser outside the profile).
func open(settings Settings) (browser *Browser, err error) {
	browser = &Browser{pending: map[int64]chan reply{}, done: make(chan struct{})}
	defer func() {
		if value := recover(); value != nil {
			failure, ok := value.(rec.Failure)
			if !ok {
				panic(value)
			}
			err = errors.New(failure.Message)
		}
		if err != nil {
			browser.Close()
			browser = nil
		}
	}()
	address, err := browser.address(settings)
	if err != nil {
		return browser, err
	}
	return browser, browser.connect(address)
}

// address starts a local Chromium or asks the remote endpoint, and returns the websocket address.
func (b *Browser) address(settings Settings) (string, error) {
	if !settings.remote() {
		return b.start()
	}
	remote, err := newEndpoint(settings)
	if err != nil {
		return "", err
	}
	b.remote = remote
	address, err := remote.socketURL()
	if err == nil {
		go remote.keepAlive()
	}
	return address, err
}

func (b *Browser) start() (string, error) {
	path := os.Getenv("RELATEX_CHROMIUM")
	if path == "" {
		path = command.ConfiguredTool("browser")
	}
	if path == "" {
		path = DefaultBrowser()
	}
	userData := command.Scratch("ferrum_user_data_dir_")
	b.userData = userData
	arguments := append(append([]string{}, chromiumFlags...), "--remote-debugging-port=0", "--remote-debugging-address=127.0.0.1",
		"--window-size=1200,1000", "--user-data-dir="+userData)
	b.process = exec.Command(path, arguments...)
	// Chromium gets relatex's own environment, as ferrum gives it: the workspace's TMPDIR is too long
	// a path for its singleton socket.
	b.process.Env = os.Environ()
	b.process.SysProcAttr = browserProcAttr()
	return b.spawn()
}

// spawn starts Chromium and reads its DevTools address from its output.
func (b *Browser) spawn() (string, error) {
	output, err := b.process.StderrPipe()
	if err != nil {
		return "", err
	}
	b.process.Stdout = nil
	group, err := command.StartGroup(b.process)
	if err != nil {
		return "", err
	}
	b.group = group
	found := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(output)
		for scanner.Scan() {
			if match := listening.FindStringSubmatch(scanner.Text()); match != nil {
				found <- match[1]
				break
			}
		}
		for scanner.Scan() {
			continue // Chromium's later output is not read.
		}
	}()
	select {
	case address := <-found:
		return address, nil
	case <-time.After(timeout):
		return "", errors.New("Browser process did not produce websocket url within timeout")
	}
}

func (b *Browser) connect(address string) error {
	token := ""
	if b.remote != nil {
		token = b.remote.token
	}
	socket, err := dial(address, token)
	if err != nil {
		return fmt.Errorf("browser connection lost: %s: %w", hostOf(address), err)
	}
	b.socket = socket
	go b.read()
	version := rec.AsMap(ojson.Plain(b.Command("", "Browser.getVersion", rec.Map{})))
	b.version = rec.ToS(version["product"])
	if !supported(b.version) {
		return fmt.Errorf("%w: %s", errUnsupported, b.version)
	}
	context := rec.AsMap(ojson.Plain(b.Command("", "Target.createBrowserContext", rec.Map{"disposeOnDetach": true})))
	b.context = rec.FetchString(context, "browserContextId")
	target := rec.AsMap(ojson.Plain(b.Command("", "Target.createTarget",
		rec.Map{"browserContextId": b.context, "url": "about:blank"})))
	attached := rec.AsMap(ojson.Plain(b.Command("", "Target.attachToTarget",
		rec.Map{"targetId": target["targetId"], "flatten": true})))
	b.session, b.frame = rec.FetchString(attached, "sessionId"), rec.FetchString(target, "targetId")
	for _, setup := range []struct {
		method string
		params rec.Map
	}{{"Fetch.enable", interceptPatterns}, {"Emulation.setDeviceMetricsOverride", viewport}, {"Page.enable", nil},
		{"Page.setLifecycleEventsEnabled", rec.Map{"enabled": true}}, {"Runtime.enable", nil},
		{"DOM.enable", rec.Map{"includeWhitespace": "all"}}, {"CSS.enable", nil}, {"Log.enable", nil}} {
		b.Page(setup.method, setup.params)
	}
	return nil
}

// Serve makes directory the one the page's requests are answered from.
func (b *Browser) Serve(directory string) {
	absolute, err := filepath.Abs(directory)
	if err != nil {
		rec.Fail("%v", err)
	}
	b.mu.Lock()
	b.root = absolute
	b.mu.Unlock()
}

// fulfill answers a paused request from the served directory.
func (b *Browser) fulfill(session string, params rec.Map) {
	b.mu.Lock()
	root := b.root
	b.mu.Unlock()
	method, answer := fetchReply(root, params)
	b.post(session, method, answer)
}

// read dispatches the browser's messages: replies to their commands, paused requests to fulfill,
// other events to watchers.
func (b *Browser) read() {
	defer close(b.done)
	for {
		message, err := b.socket.receive()
		if err != nil {
			b.failPending(fmt.Errorf("%w: %w", errConnectionLost, err))
			return
		}
		value, err := ojson.Decode(bytes.NewReader(message))
		if err != nil {
			continue
		}
		object, _ := value.(ojson.Object)
		if id, ok := object.Get("id").(int64); ok {
			b.answer(id, object)
			continue
		}
		method, _ := object.Get("method").(string)
		session, _ := object.Get("sessionId").(string)
		params, _ := ojson.Plain(object.Get("params")).(map[string]any)
		if method == "Fetch.requestPaused" {
			go b.fulfill(session, params)
			continue
		}
		b.notify(event{session, method, params})
	}
}

func (b *Browser) notify(seen event) {
	b.mu.Lock()
	watchers := append([]chan event{}, b.watchers...)
	b.mu.Unlock()
	for _, watcher := range watchers {
		select {
		case watcher <- seen:
		default:
		}
	}
}

func (b *Browser) answer(id int64, object ojson.Object) {
	b.mu.Lock()
	channel := b.pending[id]
	delete(b.pending, id)
	b.mu.Unlock()
	if channel == nil {
		return
	}
	if failure, ok := object.Get("error").(ojson.Object); ok {
		channel <- reply{err: fmt.Errorf("%v", failure.Get("message"))}
		return
	}
	channel <- reply{result: object.Get("result")}
}

func (b *Browser) failPending(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for id, channel := range b.pending {
		channel <- reply{err: err}
		delete(b.pending, id)
	}
}

// send writes a command and returns its id; the reply goes to channel when one is given.
func (b *Browser) send(session, method string, params rec.Map, channel chan reply) (int64, error) {
	if params == nil {
		params = rec.Map{}
	}
	b.mu.Lock()
	b.nextID++
	id := b.nextID
	if channel != nil {
		b.pending[id] = channel
	}
	b.mu.Unlock()
	message := rec.Map{"id": id, "method": method, "params": params}
	if session != "" {
		message["sessionId"] = session
	}
	payload, err := canonjson.Marshal(message)
	if err != nil {
		return id, err
	}
	return id, b.socket.send(payload)
}

// post sends a command without waiting for its reply (a paused request's answer).
func (b *Browser) post(session, method string, params rec.Map) {
	_, _ = b.send(session, method, params, nil)
}

// Command sends a CDP command (to a session; "" for the browser) and returns its result,
// objects in their key order (ojson.Object).
func (b *Browser) Command(session, method string, params rec.Map) any {
	result, err := b.commandWithin(session, method, params, timeout)
	if errors.Is(err, errTimedOut) {
		rec.Fail("Timed out waiting for response. It's possible that this happened because something took a very long time" +
			" (for example a page load was slow). If so, setting the :timeout option to a higher value might help.")
	}
	if err != nil {
		rec.Fail("%v", err)
	}
	return result
}

// errTimedOut is a command that got no answer within its limit.
var errTimedOut = errors.New("timed out")

// commandWithin is Command with its own time limit, the failure returned: errTimedOut when the
// answer did not come in time.
func (b *Browser) commandWithin(session, method string, params rec.Map, limit time.Duration) (any, error) {
	channel := make(chan reply, 1)
	id, err := b.send(session, method, params, channel)
	if err != nil {
		b.forget(id)
		return nil, fmt.Errorf("browser connection lost: %v", err)
	}
	select {
	case answer := <-channel:
		return answered(method, answer)
	case <-b.done:
		select {
		case answer := <-channel: // answered just before the connection ended
			return answered(method, answer)
		default:
		}
		// The connection ended before this command was sent or answered: no answer will come.
		b.forget(id)
		return nil, fmt.Errorf("%w (during %s)", errConnectionLost, method)
	case <-time.After(limit):
		b.forget(id)
		return nil, errTimedOut
	}
}

// answered is a command's answer as commandWithin returns it. A lost connection keeps its reason
// first and names the command after it: books/0003 (2026-11-08) failed its export on "Runtime.evaluate:
// browser connection lost: EOF", which the retry did not recognize as the method came first.
func answered(method string, answer reply) (any, error) {
	if errors.Is(answer.err, errConnectionLost) {
		return nil, fmt.Errorf("%w (during %s)", answer.err, method)
	}
	if answer.err != nil {
		return nil, fmt.Errorf("%s: %v", method, answer.err)
	}
	return answer.result, nil
}

func (b *Browser) forget(id int64) {
	b.mu.Lock()
	delete(b.pending, id)
	b.mu.Unlock()
}

// Page sends a command to the page's session.
func (b *Browser) Page(method string, params rec.Map) any {
	return b.Command(b.session, method, params)
}

// watch subscribes to the browser's events until the returned stop is called.
func (b *Browser) watch() (chan event, func()) {
	channel := make(chan event, 4096)
	b.mu.Lock()
	b.watchers = append(b.watchers, channel)
	b.mu.Unlock()
	return channel, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		for index, watcher := range b.watchers {
			if watcher == channel {
				b.watchers = append(b.watchers[:index], b.watchers[index+1:]...)
				return
			}
		}
	}
}

// GoTo is ferrum's Page#go_to: navigation, then the main frame's load to its end.
func (b *Browser) GoTo(address string) {
	events, stop := b.watch()
	defer stop()
	response := rec.AsMap(ojson.Plain(b.Page("Page.navigate", rec.Map{"url": address})))
	if text, ok := response["errorText"].(string); ok && text != "" && text != "net::ERR_ABORTED" {
		rec.Fail("Request to %s failed (%s)", address, text)
	}
	frame, _ := response["frameId"].(string)
	if frame == "" {
		frame = b.frame
	}
	deadline := time.After(timeout)
	for {
		select {
		case seen := <-events:
			if seen.session == b.session && seen.method == "Page.frameStoppedLoading" && seen.params["frameId"] == frame {
				return
			}
		case <-b.done:
			rec.Fail("browser connection lost while loading %s", address)
		case <-deadline:
			rec.Fail("Timed out waiting for response")
		}
	}
}

// Evaluate is ferrum's Browser#evaluate: the expression's value (as JSON values, objects in
// their key order).
func (b *Browser) Evaluate(expression string) any {
	response := b.Page("Runtime.evaluate", rec.Map{"expression": "(function() { return " + expression + " })()",
		"returnByValue": true})
	object, _ := response.(ojson.Object)
	if details := object.Get("exceptionDetails"); details != nil {
		rec.Fail("JavaScript error: %v", rec.Inspect(ojson.Plain(details)))
	}
	result, _ := object.Get("result").(ojson.Object)
	return result.Get("value")
}

// Close ends the page: its context disposed on a remote browser, a local Chromium and its user
// data directory gone.
func (b *Browser) Close() {
	if b.remote != nil && b.context != "" && b.socket != nil {
		b.disposeContext()
	}
	if b.socket != nil {
		b.socket.close()
	}
	b.remote.close()
	if b.group != nil {
		b.group.Kill()
		b.group.Close()
		_ = b.process.Wait()
	}
	if b.userData != "" {
		_ = os.RemoveAll(filepath.Clean(b.userData))
	}
}

// disposeContext closes the page's context on a shared browser, waiting a little for the answer.
func (b *Browser) disposeContext() {
	channel := make(chan reply, 1)
	if _, err := b.send("", "Target.disposeBrowserContext", rec.Map{"browserContextId": b.context}, channel); err != nil {
		return
	}
	select {
	case <-channel:
	case <-b.done:
	case <-time.After(5 * time.Second):
	}
}
