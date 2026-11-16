package legacyinput

import "testing"

func TestLatin9ReadsTheEuroSign(t *testing.T) {
	decoded, ok := Decode([]byte{'5', ' ', 0xA4, ' ', 0xBD}, "latin9")
	if !ok || string(decoded) != "5 € œ" {
		t.Fatalf("decoded %q %v", decoded, ok)
	}
}

func TestCp1252ReadsTypographicQuotes(t *testing.T) {
	decoded, ok := Decode([]byte{0x93, 'a', 0x94, ' ', 0xE9}, "ansinew")
	if !ok || string(decoded) != "“a” é" {
		t.Fatalf("decoded %q %v", decoded, ok)
	}
}

func TestDecodeRefusesUTF8UnknownEncodingsAndUndefinedBytes(t *testing.T) {
	cases := []struct {
		data     []byte
		encoding string
	}{
		{[]byte("caf\xc3\xa9"), "latin1"},
		{[]byte{'a', 0xE9}, "latin2"},
		{[]byte{'a', 0x85}, "latin1"},
		{[]byte{'a', 0x81}, "cp1252"},
	}
	for _, item := range cases {
		if decoded, ok := Decode(item.data, item.encoding); ok {
			t.Errorf("%q in %s decoded to %q", item.data, item.encoding, decoded)
		}
	}
}
