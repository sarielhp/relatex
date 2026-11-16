//go:build !windows

package engine

import (
	"os"
	"slices"
	"testing"
)

// The author's TEXINPUTS on Unix: entries split at ":".
func TestHostSearchPathKeepsTheAuthorsOrder(t *testing.T) {
	t.Setenv("TZ", "")
	home, _ := os.UserHomeDir()
	host := NewHost("~/styles//:.:../:!!/opt/tex::", "/papers/26/dog")
	want := []string{home + "/styles//", ".", "/papers/26", "!!/opt/tex", "", ""}
	if !slices.Equal(host.SearchPath, want) {
		t.Fatalf("search path %q, want %q", host.SearchPath, want)
	}
	if got := host.searchPath("/c"); got != home+"/styles//:.:/papers/26:!!/opt/tex:/c::" {
		t.Fatalf("TEXINPUTS %q: the collector goes before kpathsea's default path", got)
	}
	if got := NewHost("/a:.", "/d").searchPath("/c"); got != "/a:.:/c" {
		t.Fatalf("TEXINPUTS without a default path %q", got)
	}
	if !host.LocalTime {
		t.Fatal("without TZ the build reads the host's local time")
	}
}

func TestSearchHostIsTheCallersPathWithoutTheTimeZone(t *testing.T) {
	t.Setenv("TZ", "")
	home, _ := os.UserHomeDir()
	if SearchHost("", "/d") != nil {
		t.Fatal("no TEXINPUTS, no host: the build is hermetic")
	}
	host := SearchHost(home+"/styles//:.:../:!!"+home+":/opt/tex:", "/papers/26/dog")
	if host.LocalTime {
		t.Fatal("a conversion reads the caller's search path, not its time zone")
	}
	if host.Given != "~/styles//:.:../:!!~:/opt/tex:" {
		t.Fatalf("given %q: the home directory is written ~", host.Given)
	}
	if !slices.Equal(host.SearchPath, []string{home + "/styles//", ".", "/papers/26", "!!" + home, "/opt/tex", ""}) {
		t.Fatalf("search path %q", host.SearchPath)
	}
	// The sandbox shows no /papers/26/dog: "dog/../styles" would not resolve there.
	if got := SearchHost("../styles//:../../", "/papers/26/dog").SearchPath; !slices.Equal(got, []string{"/papers/26/styles//", "/papers"}) {
		t.Fatalf("relative entries %q are made clean", got)
	}
}
