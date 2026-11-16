package assets

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/mediamodel"
	"github.com/sarielhp/relatex/internal/rec"
)

func failureOf(fn func()) string { return run(fn) }

// TestREXMLWrite pins REXML's serialization: attributes sorted by local name in single
// quotes (an apostrophe as &apos;), raw text and entities, childless elements closed.
func TestREXMLWrite(t *testing.T) {
	source := `<?xml version="1.0"?>` + "\n" + `<svg xmlns="http://www.w3.org/2000/svg" width="10pt" xmlns:xlink="x" height="5pt">` +
		`<g id="it's"></g>a &amp; b<!-- c --><use xlink:href="#g"/></svg>`
	root := parseREXML(source)
	root.setAttribute("width", "3.5")
	var out strings.Builder
	root.write(&out)
	want := `<svg height='5pt' width='3.5' xlink='x' xmlns='http://www.w3.org/2000/svg'><g id='it&apos;s'/>a &amp; b<!-- c --><use xlink:href='#g'/></svg>`
	want = strings.Replace(want, "xlink='x'", "xmlns:xlink='x'", 1)
	if out.String() != want {
		t.Errorf("got  %s\nwant %s", out.String(), want)
	}
}

func TestValidateSVG(t *testing.T) {
	cases := map[string]string{
		`<svg><script/></svg>`:                    "Active SVG content is forbidden",
		`<svg onload="x"/>`:                       "Active SVG attribute is forbidden",
		`<svg><image href="http://x"/></svg>`:     "External SVG resource is forbidden",
		`<svg><g style="fill:url(x.svg)"/></svg>`: "External SVG style resource is forbidden",
		`<html/>`:                           "Expected an SVG page",
		`<!DOCTYPE svg><svg/>`:              "SVG declarations/entities are forbidden",
		`<svg><use xlink:href="#a"/></svg>`: "",
		`<svg><image href="data:image/png;base64,"/></svg>`: "",
	}
	for source, want := range cases {
		if got := failureOf(func() { mediamodel.ValidateSVG([]byte(source)) }); got != want {
			t.Errorf("ValidateSVG(%s) = %q, want %q", source, got, want)
		}
	}
}

func TestPNGHeader(t *testing.T) {
	if got := failureOf(func() { mediamodel.PNGFromBytes([]byte("not a png at all, really not one"), rec.Map{}) }); got != "Missing PNG header" {
		t.Errorf("got %q", got)
	}
}
