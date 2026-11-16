package typstvendor

import "testing"

// A style beside vendor/ (typst-source/support/bibstyle.typ) imports the vendored entry point by vendor/NAME/ENTRY.
func TestRewriteBeside(t *testing.T) {
	packages := []Package{{Name: "pergamon", Version: "0.8.0", Entrypoint: "lib.typ"}}
	got, err := RewriteBeside([]byte("#import \"@preview/pergamon:0.8.0\": *\n"), "bibstyle.typ", packages)
	if err != nil || string(got) != "#import \"vendor/pergamon/lib.typ\": *\n" {
		t.Errorf("RewriteBeside = %q, %v", got, err)
	}
	if _, err := RewriteBeside([]byte("#import \"@preview/pergamon:0.9.0\": *\n"), "bibstyle.typ", packages); err == nil {
		t.Error("RewriteBeside accepted a version that is not vendored")
	}
}
