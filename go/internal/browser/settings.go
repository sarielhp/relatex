package browser

import (
	"os"
	"runtime"
	"strings"
	"sync"
)

// EndpointVariable overrides the configured endpoint, so the development tools (the cohort
// harness, the queue's workers) can point every conversion and check at one browser.
const EndpointVariable = "RELATEX_BROWSER_ENDPOINT"

// remoteSessions is the default number of concurrent pages on a remote endpoint.
const remoteSessions = 6

// Settings is the user configuration's "browser" section: the endpoint (empty for a local
// Chromium), the concurrent pages allowed (0 for the default), and an optional file holding
// a token the endpoint requires.
type Settings struct {
	Endpoint  string
	Sessions  int
	TokenFile string
}

var (
	configured Settings
	configMu   sync.Mutex
)

// Configure sets the browser the checks use, from the user configuration.
func Configure(settings Settings) {
	configMu.Lock()
	defer configMu.Unlock()
	configured = settings
}

// current is the configured settings with the environment's endpoint over them.
func current() Settings {
	configMu.Lock()
	settings := configured
	configMu.Unlock()
	if endpoint := strings.TrimSpace(os.Getenv(EndpointVariable)); endpoint != "" {
		settings.Endpoint = endpoint
	}
	settings.Endpoint = strings.TrimRight(settings.Endpoint, "/")
	return settings
}

// remote is whether the settings name a browser endpoint.
func (s Settings) remote() bool { return s.Endpoint != "" }

// kind is the endpoint kind the evidence records: "remote" or "local".
func (s Settings) kind() string {
	if s.remote() {
		return "remote"
	}
	return "local"
}

// sessions is the cap on concurrent pages: the configured number, else 6 on a remote endpoint
// and half the processors (at least two) for local Chromium processes.
func (s Settings) sessions() int {
	if s.Sessions > 0 {
		return s.Sessions
	}
	if s.remote() {
		return remoteSessions
	}
	return max(2, runtime.NumCPU()/2)
}
