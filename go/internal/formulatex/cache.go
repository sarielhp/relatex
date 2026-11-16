package formulatex

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/sarielhp/relatex/internal/authormath"
	"github.com/sarielhp/relatex/internal/canonjson"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/texsandbox"
)

// Version names the serializer's output: a change to what Write writes for some MathML changes it,
// so cached outcomes of the old serializer are not used.
const Version = "4"

func digest(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// keyPrefix is what every outcome depends on besides the MathML: the serializer, the preamble,
// the collector and the TeX installation.
func keyPrefix(doctor *texsandbox.Doctor) string {
	return digest(strings.Join([]string{Version, Preamble, authormath.CollectorDigest(), doctor.Fingerprint()}, "\x00"))
}

// cacheKey is a formula's key: its MathML after the prefix.
func cacheKey(prefix, mathml string) string { return digest(prefix + "\x00" + mathml) }

// cachePath is the outcome's file in relatex's cache (XDG_CACHE_HOME), never in an export.
func cachePath(key string) string {
	return filepath.Join(command.CacheRoot(), "formula-tex", key[:2], key+".json")
}

func readCache(key string) (result, bool) {
	data, err := os.ReadFile(cachePath(key))
	if err != nil {
		return result{}, false
	}
	var outcome result
	if json.Unmarshal(data, &outcome) != nil || outcome.Status != "verified" && outcome.Status != "failed" {
		return result{}, false
	}
	return outcome, true
}

// writeCache keeps an outcome; a cache that cannot be written only costs a later run its reuse.
func writeCache(key string, outcome result) {
	path := cachePath(key)
	if os.MkdirAll(filepath.Dir(path), 0o755) != nil {
		return
	}
	data, err := json.Marshal(outcome)
	if err != nil {
		return
	}
	_ = canonjson.WriteAtomic(path, data) // the cache is an optimization; the next run checks again
}
