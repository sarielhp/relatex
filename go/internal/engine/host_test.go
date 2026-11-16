package engine

import (
	"github.com/sarielhp/relatex/internal/command"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestHostMountsExistingDirectoriesAndLocalTime(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("TZ", "Europe/Paris")
	host := NewHost(command.SearchList(directory+"//", filepath.Join(directory, "missing"), ".", ""), "/nowhere")
	if host.LocalTime {
		t.Fatal("a TZ passes as it is; /etc/localtime is not needed")
	}
	if got := host.mounts(); !slices.Equal(got, []string{directory}) {
		t.Fatalf("mounts %q", got)
	}
	var none *Host
	if len(none.mounts()) != 0 {
		t.Fatal("an ordinary build mounts nothing more")
	}
}

func TestFingerprintFollowsTheSearchedFiles(t *testing.T) {
	directory := t.TempDir()
	nested := filepath.Join(directory, "sub")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(path, text string) {
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(directory, "a.sty"), "a")
	write(filepath.Join(nested, "b.sty"), "b")
	flat, deep := SearchHost(command.SearchList(directory, ""), "/x"), SearchHost(command.SearchList(directory+"//", ""), "/x")
	flatBefore, deepBefore := flat.Fingerprint(), deep.Fingerprint()
	write(filepath.Join(nested, "b.sty"), "newer b")
	if flat.Fingerprint() != flatBefore {
		t.Fatal("a directory without // is searched one level deep")
	}
	if deep.Fingerprint() == deepBefore {
		t.Fatal("a // directory is searched below: a changed style changes the fingerprint")
	}
	var none *Host
	if none.Fingerprint() != "" {
		t.Fatal("no host, no fingerprint")
	}
}
