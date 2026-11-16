package browser

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sarielhp/relatex/internal/rec"
)

// attempts is how often a check runs before a browser that times out or cannot be reached is
// given up: the first try and two retries, each on a fresh connection and context.
const attempts = 3

// backoff is the pause before each retry.
var backoff = []time.Duration{2 * time.Second, 6 * time.Second}

// transientFailures begin the messages of failures a fresh connection can cure: timeouts, a
// lost connection, an endpoint that is waking up or was restarted.
var transientFailures = []string{"Timed out waiting", "browser connection lost", "browser endpoint unavailable",
	"Browser process did not produce websocket url"}

// Session is the browser a check ran on, for its evidence.
type Session struct {
	Version  string
	Endpoint string
	Attempts int
}

// Evidence is the session as a report records it.
func (s Session) Evidence() rec.Map {
	return rec.Map{"browser": s.Version, "browser_endpoint": s.Endpoint, "browser_attempts": int64(s.Attempts)}
}

// Unavailable is a browser that timed out or could not be reached on every attempt.
type Unavailable struct {
	Endpoint string
	Reason   string
}

func (u *Unavailable) Error() string {
	return fmt.Sprintf("%s browser unavailable after %d attempts: %s", u.Endpoint, attempts, u.Reason)
}

func transient(message string) bool {
	for _, start := range transientFailures {
		if strings.HasPrefix(message, start) {
			return true
		}
	}
	return false
}

// withPage runs work on a fresh page that serves root, as the configured browser allows (a
// session slot held meanwhile). A transient failure is retried with backoff on a new connection;
// any other failure is raised as it is.
func withPage[T any](root string, work func(*Browser) T) (T, Session, error) {
	settings := current()
	var zero T
	reason := ""
	for attempt := 1; attempt <= attempts; attempt++ {
		if attempt > 1 {
			time.Sleep(backoff[attempt-2])
		}
		result, session, err := attemptPage(settings, root, work)
		if err == nil {
			session.Attempts = attempt
			return result, session, nil
		}
		if errors.Is(err, errUnsupported) {
			rec.Fail("%v", err)
		}
		reason = err.Error()
	}
	return zero, Session{Endpoint: settings.kind(), Attempts: attempts}, &Unavailable{Endpoint: settings.kind(), Reason: reason}
}

// attemptPage is one attempt: a transient failure is returned, any other one raised.
func attemptPage[T any](settings Settings, root string, work func(*Browser) T) (result T, session Session, err error) {
	held, err := acquireSlot(settings)
	if err != nil {
		return result, session, err
	}
	defer held.release()
	browser, err := open(settings)
	if err != nil {
		return result, session, err
	}
	defer func() {
		browser.Close()
		value := recover()
		if value == nil {
			return
		}
		failure, ok := value.(rec.Failure)
		if !ok || !transient(failure.Message) {
			panic(value)
		}
		err = errors.New(failure.Message)
	}()
	browser.Serve(root)
	session = Session{Version: browser.version, Endpoint: settings.kind()}
	return work(browser), session, nil
}
