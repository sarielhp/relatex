package browser

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/relatex/internal/rec"
)

// TestConnectionLostDuringACommandIsRetried: a connection that ends while a command waits (books/0003,
// 2026-11-08: "Runtime.evaluate: browser connection lost: EOF" after 1533 s) is a transient failure: the
// page is tried again on a fresh connection, and reported as Unavailable when every attempt loses it.
func TestConnectionLostDuringACommandIsRetried(t *testing.T) {
	fake := &fakeCDP{answers: map[string]rec.Map{}, dropOn: "Runtime.evaluate"}
	server := httptest.NewServer(fake.handler(t))
	defer server.Close()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv(EndpointVariable, server.URL)
	saved := backoff
	backoff = []time.Duration{time.Millisecond, time.Millisecond}
	defer func() { backoff = saved }()
	_, session, err := withPage(t.TempDir(), func(browser *Browser) any { return browser.Evaluate("1") })
	unavailable, ok := err.(*Unavailable)
	if !ok || session.Attempts != attempts || !strings.HasPrefix(unavailable.Reason, "browser connection lost") ||
		!strings.Contains(unavailable.Reason, "during Runtime.evaluate") {
		t.Fatalf("err = %v, session = %+v", err, session)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.versions != attempts {
		t.Errorf("%d connections for %d attempts", fake.versions, attempts)
	}
}

// TestCommandAfterTheConnectionEndedFailsAtOnce: a command sent after the connection ended gets the
// lost connection, not the 120 s timeout.
func TestCommandAfterTheConnectionEndedFailsAtOnce(t *testing.T) {
	fake := &fakeCDP{answers: map[string]rec.Map{}, dropOn: "Runtime.evaluate"}
	server := httptest.NewServer(fake.handler(t))
	defer server.Close()
	browser, err := open(Settings{Endpoint: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close()
	_, err = browser.commandWithin(browser.session, "Runtime.evaluate", rec.Map{}, time.Minute)
	if !errors.Is(err, errConnectionLost) {
		t.Fatalf("first command: %v", err)
	}
	started := time.Now()
	_, err = browser.commandWithin(browser.session, "DOM.getDocument", rec.Map{}, time.Minute)
	if !errors.Is(err, errConnectionLost) || !strings.HasSuffix(err.Error(), "(during DOM.getDocument)") ||
		time.Since(started) > 5*time.Second {
		t.Errorf("after the end: %v in %v", err, time.Since(started))
	}
}
