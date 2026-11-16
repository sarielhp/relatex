package tryarxiv

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Limits on what one paper may unpack to.
const (
	maxFiles    = 5000
	maxUnpacked = 1 << 30
)

// ErrNoSource marks a paper arXiv distributes without its LaTeX source (a PDF only).
var ErrNoSource = errors.New("no LaTeX source (PDF only)")

// SafeName turns an arXiv identifier into a directory name (old-style identifiers contain a slash).
func SafeName(id string) string {
	return strings.ReplaceAll(id, "/", "_")
}

// Unpack writes the e-print body into dir: a tar archive (usually gzip-compressed) is extracted,
// a single gzip-compressed file is written as main.tex. Absolute paths, parent references and
// links are refused.
func Unpack(body []byte, dir string) error {
	if bytes.HasPrefix(body, []byte("%PDF")) {
		return ErrNoSource
	}
	data := body
	if bytes.HasPrefix(body, []byte{0x1f, 0x8b}) {
		reader, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return err
		}
		if data, err = io.ReadAll(io.LimitReader(reader, maxUnpacked+1)); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if isTar(data) {
		return extract(data, dir)
	}
	if bytes.HasPrefix(data, []byte("%PDF")) {
		return ErrNoSource
	}
	return os.WriteFile(filepath.Join(dir, "main.tex"), data, 0o644)
}

func isTar(data []byte) bool {
	return len(data) > 262 && string(data[257:262]) == "ustar"
}

func extract(data []byte, dir string) error {
	reader := tar.NewReader(bytes.NewReader(data))
	files := 0
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if files++; files > maxFiles {
			return fmt.Errorf("more than %d files", maxFiles)
		}
		if err := extractEntry(reader, header, dir); err != nil {
			return err
		}
	}
}

func extractEntry(reader *tar.Reader, header *tar.Header, dir string) error {
	name := path.Clean(header.Name)
	if path.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") {
		return fmt.Errorf("unsafe path %q in the archive", header.Name)
	}
	target := filepath.Join(dir, filepath.FromSlash(name))
	switch header.Typeflag {
	case tar.TypeDir:
		return os.MkdirAll(target, 0o755)
	case tar.TypeReg:
		return writeEntry(reader, target)
	default:
		return nil // links, devices and the like are skipped
	}
}

func writeEntry(reader io.Reader, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(file, io.LimitReader(reader, maxUnpacked)); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}
