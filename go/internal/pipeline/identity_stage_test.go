package pipeline

import "testing"

// A stage with an embedded source digest is identified by that digest, not by the binary: another stage's
// digest changing leaves it alone, its own changing does not.
func TestFingerprintFollowsTheStagesOwnSources(t *testing.T) {
	stageSources() // the embedded table is read once; the test then replaces it
	saved := sourcesDigests
	defer func() { sourcesDigests = saved }()
	root := t.TempDir()

	sourcesDigests = map[string]string{"engine": "aaa", "reconcile": "bbb"}
	engine := digest(fingerprint(root, "engine", nil))
	sourcesDigests = map[string]string{"engine": "aaa", "reconcile": "ccc"}
	if digest(fingerprint(root, "engine", nil)) != engine {
		t.Fatal("the engine's identity followed the reconciliation's sources")
	}
	sourcesDigests = map[string]string{"engine": "zzz", "reconcile": "ccc"}
	if digest(fingerprint(root, "engine", nil)) == engine {
		t.Fatal("the engine's identity ignored its own sources")
	}
	sourcesDigests = map[string]string{}
	if got := fingerprint(root, "engine", nil).Get("files").(interface{ Has(string) bool }); !got.Has(binaryKey) {
		t.Fatal("a stage without a digest does not fall back to the binary")
	}
}
