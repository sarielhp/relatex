//go:build !windows

package engine

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/texsandbox"
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
}

// Without TZ every build, a conversion with no host included, reads its dates in the host's zone.
func TestEveryBuildReadsTheLocalTimeZone(t *testing.T) {
	t.Setenv("TZ", "")
	t.Setenv("SOURCE_DATE_EPOCH", "")
	if _, err := os.Stat(localTimeFile); err != nil {
		t.Skip("no /etc/localtime")
	}
	if !LocalTime() || !strings.HasPrefix(TimeZone(), "local ") {
		t.Fatalf("zone %q: without TZ the build reads /etc/localtime", TimeZone())
	}
	var none *Host
	if texsandbox.Sandboxed && !slices.Contains(none.mounts(), localTimeFile) {
		t.Fatal("the sandbox shows /etc/localtime to a build without a host")
	}
}

// A pinned build date (the cohort pins each paper to its reference build) reads in UTC: the page
// decorations of a paper that prints \today must equal the reference, whatever the host zone.
func TestPinnedBuildDateReadsInUTC(t *testing.T) {
	t.Setenv("TZ", "")
	t.Setenv("SOURCE_DATE_EPOCH", "1793239200")
	var none *Host
	if LocalTime() || TimeZone() != "UTC" || slices.Contains(none.mounts(), localTimeFile) {
		t.Fatalf("zone %q: a pinned date is read in UTC, /etc/localtime stays hidden", TimeZone())
	}
}

func TestSearchHostIsTheCallersPath(t *testing.T) {
	t.Setenv("TZ", "")
	home, _ := os.UserHomeDir()
	if SearchHost("", "/d") != nil {
		t.Fatal("no TEXINPUTS, no host: the build is hermetic")
	}
	host := SearchHost(home+"/styles//:.:../:!!"+home+":/opt/tex:", "/papers/26/dog")
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
