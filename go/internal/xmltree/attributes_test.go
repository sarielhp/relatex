package xmltree

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestAttributeWhitespaceReadsAsXMLParsersReadIt(t *testing.T) {
	data := "<?xml version=\"1.0\"?>\n<!-- a\n\"b -->\n<svg a=\"x\ny\tz\r\nw\rv&#10;u\" b='p\nq'>\n<text>c\nd \"e\n</text><![CDATA[f\"\ng]]></svg>"
	want := "<?xml version=\"1.0\"?>\n<!-- a\n\"b -->\n<svg a=\"x y z w v&#10;u\" b='p q'>\n<text>c\nd \"e\n</text><![CDATA[f\"\ng]]></svg>"
	got := string(AttributeWhitespace([]byte(data)))
	if got != want {
		t.Fatalf("AttributeWhitespace = %q, want %q", got, want)
	}
	decoder := xml.NewDecoder(strings.NewReader(got))
	for {
		token, err := decoder.RawToken()
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if start, ok := token.(xml.StartElement); ok {
			if value := start.Attr[0].Value; value != "x y z w v\nu" {
				t.Fatalf("attribute = %q, want the character reference kept", value)
			}
			return
		}
	}
}
