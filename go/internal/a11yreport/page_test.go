package a11yreport

import (
	"strings"
	"testing"

	"github.com/sarielhp/relatex/internal/a11ylint"
	"github.com/sarielhp/relatex/internal/htmlkit"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/pdfua"
	"github.com/sarielhp/relatex/internal/rec"
)

func reportDocument() rec.Map {
	grid := func(layout bool) rec.Map {
		return rec.Map{"type": "table_grid", "grid": rec.Map{"layout": layout, "cells": []any{}}}
	}
	return rec.Map{"language": rec.Map{"tag": "en", "source": "default"}, "frontmatter": rec.Map{"title": "Sums"},
		"assets": rec.Map{"items": rec.Map{}}, "block_order": []any{},
		"nodes": rec.Map{"t1": rec.Map{"type": "table", "content": []any{grid(false)}}, "t2": rec.Map{"type": "table",
			"content": []any{grid(false)}}, "t3": rec.Map{"type": "table", "content": []any{grid(true)}}}}
}

func openWeb() rec.Map {
	return rec.Map{"status": "measured", "claim": nil, "open_items": []any{
		rec.Map{"source": "structural", "id": "language_declared", "criterion": "3.1.1", "count": int64(1)},
		rec.Map{"source": "structural", "id": "table_headers", "criterion": "1.3.1", "count": int64(1)}},
		"structural": []any{rec.Map{"check": "table_headers", "status": "gap", "count": int64(1),
			"headers": rec.Map{"tagging": int64(1), "bold": int64(2), "layout_presumed": int64(3)}}}}
}

func taggedReport(status, pdfuaid string) ojson.Object {
	found := []pdfua.Diagnostic{{Code: "pdfua.undescribed", Message: "2 graphics", Magnitude: 2}}
	return ojson.Object{{"status", status}, {"file", "sums.pdf"}, {"diagnostics", pdfua.DiagnosticList(found)},
		{"identification", ojson.Object{{"pdfuaid", pdfuaid}}}}
}

func TestClaimsSayWhyNot(t *testing.T) {
	in := Inputs{Document: reportDocument(), Web: openWeb(), EPUB: true, Tagged: taggedReport("tagged", "withdrawn")}
	claims := Claims(in)
	for _, claim := range claims {
		if claim.Reached != "" || len(claim.Reasons) == 0 {
			t.Errorf("%s: claimed %q with reasons %v", claim.Output, claim.Reached, claim.Reasons)
		}
	}
	if reasons := strings.Join(claims[2].Reasons, "\n"); !strings.Contains(reasons, "pdfua.undescribed") {
		t.Errorf("tagged PDF reasons: %s", reasons)
	}
	in.Web = rec.Map{"status": "measured", "claim": htmlkit.WCAGClaim}
	in.Tagged = taggedReport("conforming", "kept")
	claims = Claims(in)
	if claims[0].Reached != htmlkit.WCAGClaim || claims[1].Reached != EPUBClaim || claims[2].Reached != "PDF/UA-2" {
		t.Errorf("earned claims: %+v", claims)
	}
	in.Tagged = taggedReport("conforming", "withdrawn")
	if alone := Claims(in)[2]; alone.Reached != "" || len(alone.Reasons) != 1 {
		t.Errorf("a conforming file whose identification was withdrawn: %+v", alone)
	}
	if missing := Claims(Inputs{Document: reportDocument()}); missing[0].Reached != "" || missing[2].File != "" {
		t.Errorf("nothing measured: %+v", missing)
	}
}

func TestProvenanceCountsTheOrigins(t *testing.T) {
	rows := ProvenanceRows(Inputs{Document: reportDocument(), Web: openWeb()})
	if rows[1].Counts["none"] != 1 {
		t.Errorf("default language counted %v", rows[1].Counts)
	}
	if tables := rows[2].Counts; tables["none"] != 1 || tables["source"] != 1 || tables["inferred"] != 2 {
		t.Errorf("tables %v (declared headers are the source, a bold first row inferred, presumed layouts no data tables)", tables)
	}
	detected := reportDocument()
	detected["language"] = rec.Map{"tag": "en", "source": "detected"}
	if rows := ProvenanceRows(Inputs{Document: detected}); rows[1].Counts["detected"] != 1 {
		t.Errorf("detected language counted %v", rows[1].Counts)
	}
	for provenance, want := range map[string]string{"": "none", "author": "sidecar", "model": "model", "reviewed": "reviewed",
		"source": "source", "heuristic": "inferred"} {
		if got := figureOrigin(provenance); got != want {
			t.Errorf("figureOrigin(%q) = %q, want %q", provenance, got, want)
		}
	}
}

func TestPageIsReadableWithoutScripts(t *testing.T) {
	lint := []a11ylint.Item{{Kind: "language", File: "sums.tex", Line: 1, Message: "no language <declared>",
		Fix: "babel", Suggestion: `\usepackage[english]{babel}`, NoteLine: 1}}
	page := Page(Inputs{Title: "Sums", Main: "sums.tex", Version: "0.1.53", Date: "2026-11-02", Document: reportDocument(),
		Web: openWeb(), EPUB: true, Tagged: taggedReport("tagged", "withdrawn"), Lint: lint})
	for _, want := range []string{`<html lang="en">`, "<title>Accessibility report: Sums</title>", "relatex 0.1.53 on 2026-11-02",
		"sums.tex:1", "no language &lt;declared&gt;", `\usepackage[english]{babel}`, "Not claimed", "prefers-color-scheme:dark",
		`<th scope="col">detected</th>`, "Data tables with header cells (WCAG 1.3.1)"} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	if strings.Contains(page, "<script") {
		t.Error("the report page holds a script")
	}
	// It is self-contained: no load of a file or a script from outside the export (decision 112).
	for _, external := range []string{"src=\"http", "src='http", "href=\"http", "href='http", "src=\"//", "href=\"//"} {
		if strings.Contains(page, external) {
			t.Errorf("the report page loads an external resource: %q", external)
		}
	}
	if note := Page(Inputs{Document: reportDocument(), LintNote: "archive"}); !strings.Contains(note, "<p>archive</p>") {
		t.Error("the lint's absence is not explained")
	}
}
