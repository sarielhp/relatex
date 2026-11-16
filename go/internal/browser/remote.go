package browser

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/sarielhp/relatex/internal/fspath"
	"io"
	"net"
	"net/url"
	"os"
	"strings"
	"time"
)

// handshakeTimeout covers a suspended endpoint waking up: Sablier holds the first request up
// to 30 s while the container starts.
const handshakeTimeout = 45 * time.Second

// keepAliveInterval is the pause between two keep-alive requests during a session: a proxy
// that counts requests (Sablier) sees a websocket as one, and would suspend the browser mid-run.
const keepAliveInterval = 2 * time.Minute

// endpoint is a remote browser reached over HTTP: its /json/version handshake and keep-alive.
type endpoint struct {
	base  string
	token string
	stop  chan struct{}
}

// versionReply is the part of /json/version a connection reads.
type versionReply struct {
	Browser string `json:"Browser"`
	Socket  string `json:"webSocketDebuggerUrl"`
}

func newEndpoint(settings Settings) (*endpoint, error) {
	if _, err := url.Parse(settings.Endpoint); err != nil {
		return nil, fmt.Errorf("browser endpoint unavailable: %w", err)
	}
	remote := &endpoint{base: settings.Endpoint, stop: make(chan struct{})}
	if settings.TokenFile != "" {
		token, err := os.ReadFile(expandHome(settings.TokenFile))
		if err != nil {
			return nil, fmt.Errorf("browser token: %w", err)
		}
		remote.token = strings.TrimSpace(string(token))
	}
	return remote, nil
}

func expandHome(path string) string {
	if path == "~" {
		return path
	}
	return fspath.ExpandHome(path)
}

// version asks the endpoint for its browser and websocket address, waking it when suspended.
func (e *endpoint) version() (versionReply, error) {
	var reply versionReply
	body, err := e.get("/json/version")
	if err != nil {
		return reply, fmt.Errorf("browser endpoint unavailable: %w", err)
	}
	if err := json.Unmarshal(body, &reply); err != nil || reply.Socket == "" {
		return reply, fmt.Errorf("browser endpoint unavailable: no websocket address in %q", prefix(string(body), 200))
	}
	return reply, nil
}

// get is an HTTP/1.0 GET written and read by hand, as the websocket handshake is: the endpoint is
// plain HTTP by IP, and an HTTP/1.0 answer is never chunked. Without net/http and TLS, the product
// links no certificate handling it does not need.
func (e *endpoint) get(path string) ([]byte, error) {
	base, err := url.Parse(e.base)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialTimeout("tcp", base.Host, handshakeTimeout)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err = conn.SetDeadline(time.Now().Add(handshakeTimeout)); err != nil {
		return nil, err
	}
	authorization := ""
	if e.token != "" {
		authorization = "Authorization: Bearer " + e.token + "\r\n"
	}
	if _, err = fmt.Fprintf(conn, "GET %s HTTP/1.0\r\nHost: %s\r\n%s\r\n", path, base.Host, authorization); err != nil {
		return nil, err
	}
	reader := bufio.NewReader(conn)
	status, _, err := readHead(reader)
	if err != nil {
		return nil, err
	}
	if fields := strings.Fields(status); len(fields) < 2 || fields[1] != "200" {
		return nil, fmt.Errorf("answered %q", status)
	}
	return io.ReadAll(io.LimitReader(reader, 1<<20))
}

// socketURL is the browser's websocket address, reached through the endpoint.
func (e *endpoint) socketURL() (string, error) {
	reply, err := e.version()
	if err != nil {
		return "", err
	}
	return rewriteSocketURL(reply.Socket, e.base)
}

// rewriteSocketURL puts the endpoint's host into the websocket address the browser reports: behind
// a proxy that rewrites Host to localhost, Chrome names ws://localhost/devtools/browser/<id>.
func rewriteSocketURL(reported, base string) (string, error) {
	socket, err := url.Parse(reported)
	if err != nil {
		return "", fmt.Errorf("browser endpoint unavailable: %w", err)
	}
	endpoint, err := url.Parse(base)
	if err != nil || endpoint.Scheme != "http" || endpoint.Host == "" {
		return "", fmt.Errorf("browser endpoint unavailable: %q is not an http:// address", base)
	}
	socket.Scheme = "ws"
	socket.Host = endpoint.Host
	return socket.String(), nil
}

// keepAlive asks for the version every few minutes until close.
func (e *endpoint) keepAlive() {
	ticker := time.NewTicker(keepAliveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-e.stop:
			return
		case <-ticker.C:
			_, _ = e.version()
		}
	}
}

func (e *endpoint) close() {
	if e != nil {
		close(e.stop)
	}
}
