package artifacts

import "testing"

func TestIsAuthorBuildProduct(t *testing.T) {
	for name, want := range map[string]bool{
		"hello.aux": true, "hello.synctex.gz": true, "hello.log": true,
		"hello.tex": false, "other.aux": false, "sub/hello.aux": false,
	} {
		if got := IsAuthorBuildProduct(name, "hello"); got != want {
			t.Errorf("IsAuthorBuildProduct(%q) = %v, want %v", name, got, want)
		}
	}
}
