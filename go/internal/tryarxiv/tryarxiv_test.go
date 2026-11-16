package tryarxiv

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

const feedXML = `<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom">
<entry><id>http://arxiv.org/abs/2401.00001v2</id></entry>
<entry><id>http://arxiv.org/abs/hep-th/9901001v1</id></entry>
<entry><id>http://arxiv.org/abs/2401.00003</id></entry></feed>`

func tarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var raw bytes.Buffer
	writer := tar.NewWriter(&raw)
	for name, text := range files {
		if err := writer.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(text)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		writer.Write([]byte(text))
	}
	writer.Close()
	return gz(t, raw.Bytes())
}

func gz(t *testing.T, data []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	writer := gzip.NewWriter(&out)
	writer.Write(data)
	writer.Close()
	return out.Bytes()
}

// fakeArxiv serves the listing and e-prints and records each request's time and User-Agent.
func fakeArxiv(t *testing.T, eprints map[string][]byte) (*Client, *[]string) {
	t.Helper()
	seen := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path+" UA="+r.Header.Get("User-Agent"))
		if r.URL.Path == "/api/query" {
			fmt.Fprint(w, feedXML)
			return
		}
		body, ok := eprints[strings.TrimPrefix(r.URL.Path, "/e-print/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(body)
	}))
	t.Cleanup(server.Close)
	client := NewClient(MinDelay)
	client.APIBase = server.URL
	return client, &seen
}

func TestRequestsArePacedAndIdentified(t *testing.T) {
	client, seen := fakeArxiv(t, map[string][]byte{"2401.00001": []byte("x"), "2401.00002": []byte("y")})
	now := time.Unix(1000, 0)
	slept := []time.Duration{}
	client.Now = func() time.Time { return now }
	client.Sleep = func(d time.Duration) { slept = append(slept, d); now = now.Add(d) }
	for _, id := range []string{"2401.00001", "2401.00002"} {
		if _, err := client.EPrint(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(slept, []time.Duration{MinDelay}) {
		t.Fatalf("slept %v, want one pause of %v before the second request", slept, MinDelay)
	}
	for _, request := range *seen {
		if !strings.HasSuffix(request, "UA="+UserAgent) {
			t.Fatalf("request %q lacks the User-Agent", request)
		}
	}
	if strings.Contains(UserAgent, "@") {
		t.Fatal("the User-Agent must carry no email address")
	}
}

func TestDelayIsNeverBelowThreeSeconds(t *testing.T) {
	if NewClient(time.Second).Delay != MinDelay {
		t.Fatal("a delay below the minimum must be raised")
	}
}

func TestRecentParsesIdentifiersWithoutVersions(t *testing.T) {
	client, _ := fakeArxiv(t, nil)
	ids, err := client.Recent(context.Background(), "cs", 100)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"2401.00001", "hep-th/9901001", "2401.00003"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("Recent = %v, want %v", ids, want)
	}
}

func TestPickIsDistinctAndBounded(t *testing.T) {
	ids := []string{"a", "b", "c", "d"}
	got := Pick(ids, 2, rand.New(rand.NewPCG(1, 2)))
	if len(got) != 2 || got[0] == got[1] {
		t.Fatalf("Pick = %v", got)
	}
	if len(Pick(ids, 9, rand.New(rand.NewPCG(1, 2)))) != 4 {
		t.Fatal("Pick must return everything when asked for more")
	}
}

func TestUnpackKinds(t *testing.T) {
	dir := t.TempDir()
	if err := Unpack(tarGz(t, map[string]string{"main.tex": "\\x", "figs/a.png": "png"}), filepath.Join(dir, "tar")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "tar", "figs", "a.png")); err != nil {
		t.Fatal(err)
	}
	if err := Unpack(gz(t, []byte("\\documentclass{article}")), filepath.Join(dir, "single")); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "single", "main.tex")); !strings.Contains(string(data), "documentclass") {
		t.Fatal("a single gzip-compressed file becomes main.tex")
	}
	if err := Unpack([]byte("%PDF-1.5 ..."), filepath.Join(dir, "pdf")); !errors.Is(err, ErrNoSource) {
		t.Fatalf("PDF-only paper: %v", err)
	}
}

func TestUnpackRefusesUnsafePaths(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"../evil.tex", "/abs.tex", "a/../../evil.tex"} {
		if err := Unpack(tarGz(t, map[string]string{name: "x"}), filepath.Join(dir, "p")); err == nil {
			t.Fatalf("%q was accepted", name)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "evil.tex")); err == nil {
		t.Fatal("a file escaped the destination")
	}
}

func TestValidID(t *testing.T) {
	for id, want := range map[string]bool{"2401.00001": true, "2401.00001v3": true, "hep-th/9901001": true, "math.GT/0309136": true,
		"../x": false, "2401.1": false, "a b": false, "": false} {
		if ValidID(id) != want {
			t.Errorf("ValidID(%q) = %v, want %v", id, !want, want)
		}
	}
}

func TestRunDownloadsUnpacksAndConverts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake relatex is a shell script")
	}
	client, _ := fakeArxiv(t, map[string][]byte{"2401.00001": tarGz(t, map[string]string{"main.tex": "x"}), "2401.00003": []byte("%PDF-1.4")})
	client.Sleep = func(time.Duration) {}
	dir := t.TempDir()
	fake := filepath.Join(dir, "relatex")
	script := "#!/bin/sh\ntest -f main.tex || exit 2\necho noise\necho 'Conversion succeeded: output (0 errors, 0 warnings)'\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	options := Options{IDs: []string{"2401.00001"}, Random: 2, Category: "cs", Dest: filepath.Join(dir, "papers"), Relatex: fake}
	keepAll := func(all []string, n int) []string { return all[1 : 1+n] }
	if err := Run(context.Background(), client, options, &out, keepAll); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("want one line per paper, got %q", out.String())
	}
	if !strings.Contains(lines[0], "ok") || !strings.Contains(lines[0], "Conversion succeeded") {
		t.Fatalf("first line %q", lines[0])
	}
	if !strings.Contains(lines[1], "FAILED") || !strings.Contains(lines[1], "download") {
		t.Fatalf("hep-th/9901001 is not served: %q", lines[1])
	}
	if !strings.Contains(lines[2], "SKIPPED") {
		t.Fatalf("PDF-only paper: %q", lines[2])
	}
}

func TestRunRejectsBadIdentifiersBeforeAnyRequest(t *testing.T) {
	client, seen := fakeArxiv(t, nil)
	err := Run(context.Background(), client, Options{IDs: []string{"2401.00001", "../etc"}}, &bytes.Buffer{}, nil)
	if err == nil || len(*seen) != 0 {
		t.Fatalf("err=%v requests=%v", err, *seen)
	}
}
