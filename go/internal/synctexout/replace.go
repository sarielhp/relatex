// Replace puts relatex's own PDF and its SyncTeX file in place of another tool's pair (relatex
// synctex --replace-pdf, decision 101): for a fast preview built by another engine (XeLaTeX), whose
// PDF relatex's LuaLaTeX build can never equal byte for byte.

package synctexout

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/sarielhp/relatex/internal/fspath"
	"os"
	"path/filepath"
	"strings"
)

// Superseded is another tool's PDF that a newer build replaced after relatex saw it, or sources
// that changed after relatex's build read them: relatex's pair is older than what is in place.
type Superseded struct {
	PDF    string
	Reason string
}

func (s *Superseded) Error() string {
	return fmt.Sprintf("superseded: %s; nothing installed, %s and %s left as they were", s.Reason,
		filepath.Base(s.PDF), filepath.Base(Path(s.PDF)))
}

// Digest is the SHA-256 of a file's content, "" when it cannot be read.
func Digest(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Current is nil while pdf still has the digest seen (the PDF relatex waited for) and every source
// still holds what the build read; else a *Superseded.
func (b Built) Current(pdf, seen string) error {
	if Digest(pdf) != seen {
		return &Superseded{PDF: pdf, Reason: filepath.Base(pdf) + " changed after relatex saw it"}
	}
	if changed := ChangedSources(b.Input, b.Sources); len(changed) > 0 {
		return &Superseded{PDF: pdf, Reason: "changed after relatex read them: " + strings.Join(changed, ", ")}
	}
	return nil
}

// Replacement is relatex's pair for another tool's PDF, and the hooks that make putting it there safe.
type Replacement struct {
	PDF     string // the other tool's PDF, absolute
	Ours    []byte // relatex's PDF
	Synctex []byte // its word-level SyncTeX file
	// Current is checked once both temporary files are written, right before the renames.
	Current func() error
	// Partial is told the temporary files while they exist (none once renamed), for a signal to remove them.
	Partial func(paths ...string)
	// Uninterrupted runs the check and the two renames so that a signal waits for them.
	Uninterrupted func(body func() error) error
}

// Replace writes NAME.pdf.tmp, then NAME.synctex.gz.tmp, beside the PDF; checks Current; then renames
// the SyncTeX file into place first and the PDF second. A viewer that reloads the PDF when it changes
// reads the SyncTeX file then, so it finds the new one already there and loads a matching pair; one
// that re-reads a changed SyncTeX file (the patched Okular) sees the new SyncTeX beside the old PDF
// only between the two renames (microseconds) and reloads again for the PDF. The other order would
// leave a viewer that reads the SyncTeX with the PDF holding the old SyncTeX for the new PDF until
// the next build. Written in that order, the SyncTeX file's mtime is not older than the PDF's, as
// TeX's own outputs are. A failed check removes both temporary files and installs nothing.
func Replace(r Replacement) (string, error) {
	pdfTemporary, synctex := r.PDF+".tmp", Path(r.PDF)
	synctexTemporary := synctex + ".tmp"
	r.Partial(pdfTemporary, synctexTemporary)
	defer r.Partial()
	discard := func() {
		_ = os.Remove(pdfTemporary)
		_ = os.Remove(synctexTemporary)
	}
	if err := os.WriteFile(pdfTemporary, r.Ours, 0o644); err != nil {
		discard()
		return "", err
	}
	if err := os.WriteFile(synctexTemporary, r.Synctex, 0o644); err != nil {
		discard()
		return "", err
	}
	err := r.Uninterrupted(func() error {
		if err := r.Current(); err != nil {
			return err
		}
		// A viewer holding the PDF open blocks the replacement on Windows: fspath.Replace waits
		// about two seconds for it, then says the file is in use (decision 129).
		if err := fspath.Replace(synctexTemporary, synctex); err != nil {
			return err
		}
		return fspath.Replace(pdfTemporary, r.PDF)
	})
	if err != nil {
		discard()
		return "", err
	}
	return filepath.Base(r.PDF) + " and " + filepath.Base(synctex), nil
}
