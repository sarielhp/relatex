package cli

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/fspath"
)

// TerminalReport: how a failure reads in the terminal. The long work directories it names
// are replaced by short links, ~/.cache/relatex/err/<name>, and the message is coloured when
// stderr is a terminal (and NO_COLOR is unset): only the level word, as a compiler writes it,
// the source location bold, the evidence dim. A file name too long to read inside a sentence
// is put on a line of its own. Only the newest links are kept.

const (
	keptLinks = 50
	longPath  = 40
	levelWord = "error"
	// pathWordish is what a path cannot follow: the tail of a home-shortened link (~/.cache).
	pathWordish = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_~"
)

var pathPattern = regexp.MustCompile(pathExpression())

// pathExpression finds absolute paths in a message: from "/" on Unix; on Windows also from a
// drive ("C:\Users\...").
func pathExpression() string {
	if runtime.GOOS == "windows" {
		return `(?:[A-Za-z]:\\|/)[^\t\n\v\f\r :;,'"]+`
	}
	return `/[^\t\n\v\f\r :;,'"]+`
}

var levelColours = map[string]string{"fatal": "1;31", "error": "31", "alert": "33", "warning": "36"}

var levelPattern = regexp.MustCompile(`: (fatal|error|alert|warning)\[`)

// colourful is io.tty? && !ENV.key?('NO_COLOR') for a file.
func colourful(file *os.File) bool {
	if _, set := os.LookupEnv("NO_COLOR"); set || file == nil {
		return false
	}
	return isTerminal(file)
}

func paint(text, code string, colour bool) string {
	if !colour {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

// failureText is TerminalReport.failure: `relatex: error: <message>`, its level word the only
// red.
func failureText(message string, colour bool) string {
	lines := messageLines(shorten(message))
	head := ""
	if len(lines) > 0 {
		head = lines[0]
	}
	result := []string{"relatex: " + paint(levelWord, levelColours[levelWord], colour) + ": " + head}
	for _, line := range lines[1:] {
		switch {
		case strings.HasPrefix(line, "Evidence:"):
			line = paint(line, "2", colour)
		case strings.HasPrefix(line, "  at "):
			line = paint(line, "1", colour)
		}
		result = append(result, line)
	}
	return strings.Join(result, "\n")
}

// messageLines is TerminalReport.lines: the message's lines, a long file name moved onto a
// line of its own.
func messageLines(message string) []string {
	raw := strings.Split(message, "\n")
	if strings.HasSuffix(message, "\n") {
		raw = raw[:len(raw)-1]
	}
	lines := []string{}
	for _, line := range raw {
		lines = append(lines, breakLongNames(line)...)
	}
	return lines
}

// breakLongNames is the file-name half of TerminalReport.lines.
func breakLongNames(line string) []string {
	if line == "" {
		return []string{""}
	}
	lines := []string{}
	last := 0
	for _, match := range pathMatches(line) {
		begin, end := match[0], match[1]
		path := line[begin:end]
		if !ownLine(path) {
			continue
		}
		lines = append(lines, strings.TrimRight(line[last:begin], " \t"))
		last = begin
	}
	lines = append(lines, strings.TrimRight(line[last:], " \t"))
	if len(lines) > 1 && lines[0] == "" {
		lines = lines[1:]
	}
	for index := 1; index < len(lines); index++ {
		lines[index] = "  " + lines[index]
	}
	return lines
}

// ownLine: a name too long to read inside a sentence, and not already a short link.
func ownLine(path string) bool {
	if len(path) <= longPath {
		return false
	}
	info, err := os.Stat(path)
	return err != nil || !info.IsDir()
}

// pathMatches is pathPattern's matches that begin a path: not the tail of a home-shortened
// link, whose leading ~ is not part of the pattern, and not a URL's "//host/..." after its
// scheme (books/0003's "http://cs.uiuc.edu/..." was broken onto a line of its own).
func pathMatches(line string) [][]int {
	matches := [][]int{}
	for _, match := range pathPattern.FindAllStringIndex(line, -1) {
		begin := match[0]
		if begin > 0 && strings.ContainsRune(pathWordish, rune(line[begin-1])) {
			continue
		}
		if begin > 0 && line[begin-1] == ':' && strings.HasPrefix(line[begin:], "//") {
			continue
		}
		matches = append(matches, match)
	}
	return matches
}

// diagnosticsText is TerminalReport.diagnostics: one per line, the level coloured.
func diagnosticsText(lines []string, colour bool) string {
	painted := make([]string, len(lines))
	for index, line := range lines {
		painted[index] = levelPattern.ReplaceAllStringFunc(line, func(match string) string {
			level := levelPattern.FindStringSubmatch(match)[1]
			return ": " + paint(level, levelColours[level], colour) + "["
		})
	}
	return strings.Join(painted, "\n")
}

// shorten: directories named in the message, when long, become short links to them.
func shorten(message string) string {
	var builder strings.Builder
	last := 0
	for _, match := range pathMatches(message) {
		path := message[match[0]:match[1]]
		if len(path) <= longPath {
			continue
		}
		if info, err := os.Stat(path); err != nil || !info.IsDir() {
			continue
		}
		builder.WriteString(message[last:match[0]])
		builder.WriteString(link(path))
		last = match[1]
	}
	if last == 0 {
		return message
	}
	builder.WriteString(message[last:])
	return builder.String()
}

func link(path string) string {
	root := filepath.Join(command.CacheRoot(), "err")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return path
	}
	target := filepath.Join(root, randomName())
	if err := os.Symlink(path, target); err != nil {
		return path
	}
	prune(root)
	home, err := os.UserHomeDir()
	if err == nil && fspath.Inside(home, target) {
		return "~" + target[len(home):]
	}
	return target
}

// randomName is SecureRandom.alphanumeric(6).downcase.
func randomName() string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	bytes := make([]byte, 6)
	if _, err := rand.Read(bytes); err != nil {
		return "link00"
	}
	for index, value := range bytes {
		bytes[index] = alphabet[int(value)%len(alphabet)]
	}
	return strings.ToLower(string(bytes))
}

func prune(root string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	type entry struct {
		path    string
		modTime int64
	}
	links := []entry{}
	for _, item := range entries {
		path := filepath.Join(root, item.Name())
		info, err := os.Lstat(path)
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			links = append(links, entry{path, info.ModTime().UnixNano()})
		}
	}
	sort.SliceStable(links, func(a, b int) bool { return links[a].modTime > links[b].modTime })
	for index, item := range links {
		if index >= keptLinks {
			_ = os.Remove(item.path)
		}
	}
}
