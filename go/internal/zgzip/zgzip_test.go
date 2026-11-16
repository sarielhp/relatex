package zgzip

import (
	"bytes"
	"compress/flate"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"testing"
)

func TestCompressWritesDefaultLevelHeaderAndRoundTrips(t *testing.T) {
	plain := bytes.Repeat([]byte("relatex bundle "), 1000)
	compressed, err := Compress(plain)
	if err != nil {
		t.Fatal(err)
	}
	if want := []byte{0x1f, 0x8b, 8, 0, 0, 0, 0, 0, 0, 3}; !bytes.Equal(compressed[:10], want) {
		t.Fatalf("header %v, want %v", compressed[:10], want)
	}
	back, err := Decompress(compressed, int64(len(plain)))
	if err != nil || !bytes.Equal(back, plain) {
		t.Fatalf("round trip failed: %v", err)
	}
}

// The compressed bytes are compress/flate's, the same on every platform (decision 129, D4): a
// digest that changes means a Go release changed them, and every hash of a compressed file with it.
func TestCompressBytesAreFixed(t *testing.T) {
	plain := bytes.Repeat([]byte("relatex bundle, gzip at level 3 with mtime 0\n"), 500)
	compressed, err := Compress(plain)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(compressed)
	if got := hex.EncodeToString(digest[:]); got != "f3838aeb44b0d923458b50a90435be0c4072fb42b1a43a1a073fc2b1f99529d0" {
		t.Fatalf("compressed digest %s", got)
	}
}

func TestDeflateRoundTripsAtEachLevel(t *testing.T) {
	plain := bytes.Repeat([]byte("abc relatex "), 300)
	for _, level := range []int{Default, Quick, Best} {
		packed, err := Deflate(plain, level)
		if err != nil {
			t.Fatal(err)
		}
		back, err := io.ReadAll(flate.NewReader(bytes.NewReader(packed)))
		if err != nil || !bytes.Equal(back, plain) {
			t.Fatalf("level %d: round trip failed: %v", level, err)
		}
	}
	if _, err := Deflate(plain, 12); err == nil {
		t.Fatal("level 12 accepted")
	}
}
