// Package browser is the Go port of Relatex::BrowserCheck, PaperBrowserCheck with its page
// checks, and MathImageFallback: the offline HTML read back by Chromium, driven over the
// Chrome DevTools Protocol as the ferrum gem drives it.
package browser

import (
	"bufio"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // RFC 6455's handshake digest, not a security use.
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"
)

// handshakeGUID is RFC 6455's key suffix.
const handshakeGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

const (
	opContinuation = 0x0
	opText         = 0x1
	opClose        = 0x8
	opPing         = 0x9
	opPong         = 0xA
)

// socket is a WebSocket client connection (RFC 6455): text messages only, as CDP uses it.
type socket struct {
	conn   net.Conn
	reader *bufio.Reader
	write  sync.Mutex
}

// dialTimeout bounds opening the connection to the browser.
const dialTimeout = 45 * time.Second

// dial opens a WebSocket to a ws:// URL, with a bearer token when one is given.
func dial(address, token string) (*socket, error) {
	parsed, err := url.Parse(address)
	if err != nil {
		return nil, err
	}
	conn, err := connectTCP(parsed)
	if err != nil {
		return nil, err
	}
	reader, err := handshake(conn, parsed, token)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return &socket{conn: conn, reader: reader}, nil
}

// connectTCP opens the connection (the endpoint is plain HTTP by IP: ws:// only).
func connectTCP(parsed *url.URL) (net.Conn, error) {
	if parsed.Scheme != "ws" {
		return nil, fmt.Errorf("unsupported websocket scheme %q", parsed.Scheme)
	}
	return net.DialTimeout("tcp", parsed.Host, dialTimeout)
}

// handshake upgrades the connection: an HTTP/1.1 request written and its response read by
// hand (the status line and headers; a 101 has no body), without net/http.
func handshake(conn net.Conn, parsed *url.URL, token string) (*bufio.Reader, error) {
	nonce := make([]byte, 16)
	if _, randomErr := rand.Read(nonce); randomErr != nil {
		return nil, randomErr
	}
	key := base64.StdEncoding.EncodeToString(nonce)
	authorization := ""
	if token != "" {
		authorization = "Authorization: Bearer " + token + "\r\n"
	}
	request := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n"+
		"Sec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n%s\r\n", parsed.RequestURI(), parsed.Host, key, authorization)
	if _, writeErr := io.WriteString(conn, request); writeErr != nil {
		return nil, writeErr
	}
	reader := bufio.NewReaderSize(conn, 1<<20)
	status, headers, err := readHead(reader)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(status, "HTTP/1.1 101") || headers["sec-websocket-accept"] != accept(key) {
		return nil, fmt.Errorf("websocket handshake refused: %s", status)
	}
	return reader, nil
}

// readHead reads an HTTP response's status line and headers (names lowercased).
func readHead(reader *bufio.Reader) (string, map[string]string, error) {
	status, err := readLine(reader)
	if err != nil {
		return "", nil, err
	}
	headers := map[string]string{}
	for {
		line, lineErr := readLine(reader)
		if lineErr != nil {
			return "", nil, lineErr
		}
		if line == "" {
			return status, headers, nil
		}
		name, value, found := strings.Cut(line, ":")
		if !found {
			return "", nil, fmt.Errorf("malformed handshake header: %q", line)
		}
		headers[strings.ToLower(strings.TrimSpace(name))] = strings.TrimSpace(value)
	}
}

func readLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func accept(key string) string {
	sum := sha1.Sum([]byte(key + handshakeGUID)) //nolint:gosec // RFC 6455.
	return base64.StdEncoding.EncodeToString(sum[:])
}

// send writes one masked text frame.
func (s *socket) send(message []byte) error { return s.frame(opText, message) }

func (s *socket) frame(opcode byte, payload []byte) error {
	s.write.Lock()
	defer s.write.Unlock()
	header := []byte{0x80 | opcode}
	switch size := len(payload); {
	case size < 126:
		header = append(header, 0x80|byte(size))
	case size <= 0xFFFF:
		header = append(header, 0x80|126, byte(size>>8), byte(size))
	default:
		header = append(header, 0x80|127)
		header = binary.BigEndian.AppendUint64(header, uint64(size))
	}
	mask := make([]byte, 4)
	if _, err := rand.Read(mask); err != nil {
		return err
	}
	masked := make([]byte, len(payload))
	for index, value := range payload {
		masked[index] = value ^ mask[index%4]
	}
	_, err := s.conn.Write(append(append(header, mask...), masked...))
	return err
}

// receive reads the next whole text message, answering pings on the way.
func (s *socket) receive() ([]byte, error) {
	var message []byte
	for {
		final, opcode, payload, err := s.readFrame()
		if err != nil {
			return nil, err
		}
		switch opcode {
		case opPing:
			if err := s.frame(opPong, payload); err != nil {
				return nil, err
			}
			continue
		case opPong:
			continue
		case opClose:
			return nil, io.EOF
		case opText, opContinuation:
			message = append(message, payload...)
		default:
			return nil, fmt.Errorf("unexpected websocket opcode %d", opcode)
		}
		if final {
			return message, nil
		}
	}
}

func (s *socket) readFrame() (bool, byte, []byte, error) {
	head := make([]byte, 2)
	if _, err := io.ReadFull(s.reader, head); err != nil {
		return false, 0, nil, err
	}
	final, opcode := head[0]&0x80 != 0, head[0]&0x0F
	length := uint64(head[1] & 0x7F)
	switch length {
	case 126:
		extended := make([]byte, 2)
		if _, err := io.ReadFull(s.reader, extended); err != nil {
			return false, 0, nil, err
		}
		length = uint64(binary.BigEndian.Uint16(extended))
	case 127:
		extended := make([]byte, 8)
		if _, err := io.ReadFull(s.reader, extended); err != nil {
			return false, 0, nil, err
		}
		length = binary.BigEndian.Uint64(extended)
	}
	if head[1]&0x80 != 0 {
		return false, 0, nil, errors.New("masked websocket frame from the server")
	}
	if length > 1<<31 {
		return false, 0, nil, errors.New("websocket frame too large")
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(s.reader, payload); err != nil {
		return false, 0, nil, err
	}
	return final, opcode, payload, nil
}

func (s *socket) close() {
	_ = s.frame(opClose, nil)
	s.conn.Close()
}

// hostOf is a ws:// URL's host:port, for messages.
func hostOf(address string) string {
	if parsed, err := url.Parse(address); err == nil {
		return parsed.Host
	}
	return strings.TrimPrefix(address, "ws://")
}
