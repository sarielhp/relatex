package pipeline

import (
	"os"
	"path/filepath"
	"slices"
	"time"
)

const (
	// staleDirectory holds the products of the conversion before a failed one: relatex's own
	// directory (underscore prefix), so no later publish touches it except to clear it.
	staleDirectory = "_stale"
	// failureNote says where the last conversion failed and what became of the earlier products.
	failureNote = "_conversion-failed.txt"
)

// setAsideProducts keeps a failed conversion from leaving an earlier conversion's products where
// they look current: an accessibility claim (accessibility.json, accessibility.html, the PDF/UA
// verdict) would still read as this document's. The earlier products move into _stale/, with a
// short note beside them; the next successful publish clears both. It never fails: the failure
// being reported matters more than the clean-up.
func setAsideProducts(output, message string) {
	entries, err := os.ReadDir(output)
	if err != nil {
		return
	}
	var products []string
	for _, entry := range entries {
		if ownedEntry(entry.Name()) {
			products = append(products, entry.Name())
		}
	}
	setAsideNames(output, output, products, message)
}

// setAsideNames is setAsideProducts for the named entries of output, the ones relatex wrote; an
// empty output is no directory. The stale products and the note go to holder: the product itself,
// or, for an output root, the project cache, so a failure leaves nothing in the user's directory.
// It returns whether the note was written.
func setAsideNames(output, holder string, products []string, message string) bool {
	if output == "" {
		return false
	}
	if _, err := os.Stat(output); err != nil {
		return false
	}
	stale := filepath.Join(holder, staleDirectory)
	products = slices.DeleteFunc(slices.Clone(products), func(name string) bool {
		_, err := os.Lstat(filepath.Join(output, name))
		return err != nil
	})
	if len(products) > 0 {
		_ = os.RemoveAll(stale)
		if err := os.MkdirAll(stale, 0o755); err != nil {
			return false
		}
		for _, name := range products {
			moveAside(filepath.Join(output, name), filepath.Join(stale, name))
		}
	}
	note := "The conversion started at " + time.Now().Format(time.RFC3339) + " failed:\n" + message + "\n"
	if _, err := os.Stat(stale); err == nil {
		note += "\nThe products of the conversion before it, including its accessibility report and claim,\n" +
			"are in " + stale + " and do not describe the current source. Convert again to replace them.\n"
	}
	if os.MkdirAll(holder, 0o755) != nil {
		return false
	}
	return os.WriteFile(filepath.Join(holder, failureNote), []byte(note), 0o644) == nil
}

// moveAside moves a stale product into the stale directory: a rename, or, when the cache is on another
// file system than the output root, a copy and a removal. It never fails.
func moveAside(from, to string) {
	if os.Rename(from, to) == nil {
		return
	}
	defer func() { _ = recover() }()
	copyTree(from, to)
	removeAll(from)
}

// clearStale removes the failure note and the stale products of an earlier failed conversion from the
// directory that holds them.
func clearStale(holder string) {
	_ = os.RemoveAll(filepath.Join(holder, staleDirectory))
	_ = os.Remove(filepath.Join(holder, failureNote))
}
