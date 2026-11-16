package typstvendor

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func loadPin(t *testing.T) Pin {
	t.Helper()
	pin, err := LoadPin(repositoryRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	return pin
}

// TestFilesMatchPin checks every vendored file against its pinned SHA-256, that no unpinned file
// sits in a package directory, and that each package has its LICENSE, typst.toml and entry point.
func TestFilesMatchPin(t *testing.T) {
	root := repositoryRoot(t)
	pin := loadPin(t)
	if len(pin.Packages) == 0 {
		t.Fatal("the pin lists no packages")
	}
	for _, p := range pin.Packages {
		for _, required := range []string{"LICENSE", "typst.toml", p.Entrypoint} {
			if _, ok := p.Files[required]; !ok {
				t.Errorf("%s %s does not vendor %s", p.Name, p.Version, required)
			}
		}
		for _, name := range p.SortedFiles() {
			if _, err := ReadVerified(root, p, name); err != nil {
				t.Error(err)
			}
		}
		base := filepath.Join(root, filepath.FromSlash(p.Source("")))
		err := filepath.WalkDir(base, func(file string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil || entry.IsDir() {
				return walkErr
			}
			name, _ := filepath.Rel(base, file)
			if _, ok := p.Files[filepath.ToSlash(name)]; !ok {
				t.Errorf("%s is vendored but not pinned", p.Source(filepath.ToSlash(name)))
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

// TestPinnedVersionsInSeries checks each version is in its series (~> MAJOR.MINOR.0): a patch
// update keeps the series, a new series needs the author.
func TestPinnedVersionsInSeries(t *testing.T) {
	series := regexp.MustCompile(`^~> ([0-9]+\.[0-9]+)\.0$`)
	names := map[string]bool{}
	for _, p := range loadPin(t).Packages {
		names[p.Name] = true
		match := series.FindStringSubmatch(p.Series)
		if match == nil || !strings.HasPrefix(p.Version, match[1]+".") {
			t.Errorf("%s %s is not in its series %q", p.Name, p.Version, p.Series)
		}
		manifest, err := os.ReadFile(filepath.Join(repositoryRoot(t), filepath.FromSlash(p.Source("typst.toml"))))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(manifest, []byte(`version = "`+p.Version+`"`)) {
			t.Errorf("%s's typst.toml does not name version %s", p.Name, p.Version)
		}
	}
	for _, required := range []string{"pergamon", "citegeist"} {
		if !names[required] {
			t.Errorf("%s is not vendored", required)
		}
	}
}

// TestVendoredSizeCap is decision D's cap: the vendored set stays under 1 MB.
func TestVendoredSizeCap(t *testing.T) {
	pin := loadPin(t)
	if pin.MaxBytes != 1<<20 {
		t.Errorf("the cap is %d bytes, not decision D's 1 MB", pin.MaxBytes)
	}
	var total int64
	for _, p := range pin.Packages {
		for _, name := range p.SortedFiles() {
			info, err := os.Stat(filepath.Join(repositoryRoot(t), filepath.FromSlash(p.Source(name))))
			if err != nil {
				t.Fatal(err)
			}
			total += info.Size()
		}
	}
	if total >= 1<<20 {
		t.Errorf("the vendored packages are %d bytes, over decision D's 1 MB cap", total)
	}
}

var previewLine = regexp.MustCompile(`(?m)^[^/\n]*@preview/`)

// TestWriteRewritesImports writes the tree and checks no @preview import remains, that each
// rewritten import names a written file, and that the reported hashes are the written bytes'.
func TestWriteRewritesImports(t *testing.T) {
	directory := t.TempDir()
	written, err := Write(repositoryRoot(t), directory)
	if err != nil {
		t.Fatal(err)
	}
	quoted := regexp.MustCompile(`(?m)^[^/\n]*import\s+"([^"]+)"`)
	for _, file := range written {
		data, readErr := os.ReadFile(filepath.Join(directory, filepath.FromSlash(file.Path)))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if digest(data) != file.SHA256 || len(data) != file.Bytes {
			t.Errorf("%s: the reported hash or size is not the written file's", file.Path)
		}
		if !strings.HasSuffix(file.Path, ".typ") {
			continue
		}
		if line := previewLine.Find(data); line != nil {
			t.Errorf("%s keeps a @preview import: %s", file.Path, line)
		}
		for _, match := range quoted.FindAllSubmatch(data, -1) {
			target := filepath.Join(filepath.Dir(filepath.Join(directory, file.Path)), filepath.FromSlash(string(match[1])))
			if _, statErr := os.Stat(target); statErr != nil {
				t.Errorf("%s imports %s, which was not written", file.Path, match[1])
			}
		}
	}
}

func TestRewrite(t *testing.T) {
	packages := []Package{{Name: "citegeist", Version: "0.2.2", Entrypoint: "lib.typ"}, {Name: "bullseye", Version: "0.1.0", Entrypoint: "src/lib.typ"}}
	source := "#import \"@preview/citegeist:0.2.2\": load-bibliography\n/// #import \"@preview/zebraw:0.5.5\": *\n  #import \"@preview/bullseye:0.1.0\": *\n"
	got, err := Rewrite([]byte(source), "pergamon/src/bibtypst.typ", packages)
	if err != nil {
		t.Fatal(err)
	}
	want := "#import \"../../citegeist/lib.typ\": load-bibliography\n/// #import \"@preview/zebraw:0.5.5\": *\n  #import \"../../bullseye/src/lib.typ\": *\n"
	if string(got) != want {
		t.Errorf("Rewrite = %q, want %q", got, want)
	}
	for _, bad := range []string{"#import \"@preview/citegeist:0.2.3\": *\n", "#import \"@preview/zebraw:0.5.5\": *\n"} {
		if _, err := Rewrite([]byte(bad), "pergamon/lib.typ", packages); err == nil {
			t.Errorf("Rewrite(%q) accepted a package that is not vendored", bad)
		}
	}
}

const sampleBib = `@article{knuth84,
  author = {Donald E. Knuth},
  title = {Literate Programming},
  journal = {The Computer Journal},
  volume = {27},
  number = {2},
  pages = {97--111},
  year = {1984},
}

@book{lamport94,
  author = {Leslie Lamport},
  title = {{\LaTeX}: A Document Preparation System},
  publisher = {Addison-Wesley},
  year = {1994},
}
`

const sampleMain = `#import "support/vendor/pergamon/lib.typ": *

#add-bib-resource(read("references.bib"))

#refsection(style: alphabetic-style())[
  See #cite("knuth84") and #cite("lamport94").

  #print-bibliography()
]
`

// TestCompileOffline writes the vendor tree, a main.typ and a two-entry .bib, and compiles them
// with empty package directories and no reachable proxy: the export needs no network.
func TestCompileOffline(t *testing.T) {
	typst, err := exec.LookPath("typst")
	if err != nil {
		t.Skip("typst is not on PATH")
	}
	directory := t.TempDir()
	if _, err = Write(repositoryRoot(t), directory); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"main.typ": sampleMain, "references.bib": sampleBib} {
		if err = os.WriteFile(filepath.Join(directory, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	empty := t.TempDir()
	command := exec.Command(typst, "compile", "--root", directory, "--package-path", empty, "--package-cache-path", empty,
		"--ignore-system-fonts", "main.typ", "main.pdf")
	command.Dir = directory
	command.Env = append(os.Environ(), "HTTPS_PROXY=http://127.0.0.1:9", "https_proxy=http://127.0.0.1:9",
		"HTTP_PROXY=http://127.0.0.1:9", "http_proxy=http://127.0.0.1:9", "ALL_PROXY=http://127.0.0.1:9", "NO_PROXY=")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("typst compile failed: %v\n%s", err, output)
	}
	if strings.Contains(strings.ToLower(string(output)), "downloading") {
		t.Errorf("typst downloaded a package:\n%s", output)
	}
	if info, statErr := os.Stat(filepath.Join(directory, "main.pdf")); statErr != nil || info.Size() == 0 {
		t.Errorf("typst wrote no main.pdf: %v", statErr)
	}
}
