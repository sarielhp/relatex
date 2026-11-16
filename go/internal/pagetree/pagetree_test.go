package pagetree

import "testing"

// A page written back as HTML reads as the same tree: attributes decoded and quoted again, void
// elements without end tags, the stylesheet's text as it stands, MathML and SVG kept.
func TestWriteHTMLRoundTrips(t *testing.T) {
	page := `<html lang="en"><head><style>nav > ol { margin: 0 }</style></head><body>` +
		`<p data-x='{"a":"b&amp;c"}' hidden>One &lt;two&gt; &amp;&nbsp;three<br>four<img src="a.svg" alt=""></p>` +
		`<math><mi>x</mi><mspace width="1em"/></math><svg><path d="M0 0"/></svg></body></html>`
	tree := Parse(page)
	written := HTML(tree.Children[0])
	want := `<html lang="en"><head><style>nav > ol { margin: 0 }</style></head><body>` +
		`<p data-x="{&quot;a&quot;:&quot;b&amp;c&quot;}" hidden="">One &lt;two&gt; &amp;` + " " +
		`three<br>four<img src="a.svg" alt=""></p><math><mi>x</mi><mspace width="1em"></mspace></math>` +
		`<svg><path d="M0 0"></path></svg></body></html>`
	if written != want {
		t.Fatalf("written\n%s\nwant\n%s", written, want)
	}
	if again := HTML(Parse(written).Children[0]); again != written {
		t.Fatalf("not stable:\n%s", again)
	}
	copied := Clone(tree)
	copied.Children[0].SetAttr("lang", "de")
	if value, _ := tree.Children[0].Attr("lang"); value != "en" {
		t.Errorf("Clone shares attributes")
	}
	if index, body := Child(tree.Children[0], "body"); index != 1 || body == nil {
		t.Errorf("Child found %d", index)
	}
}
