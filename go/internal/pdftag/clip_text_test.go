package pdftag

import (
	"strings"
	"testing"
)

func clipTags(leaves []string) *pageTags {
	return &pageTags{leafOf: func(ordinal int) string { return leaves[ordinal] }, objects: map[int]string{},
		roleOf: func(string) string { return "Link" }}
}

func TestClipTextObjectIsMarkedOutsideItsBT(t *testing.T) {
	items := interpretTest(t, "q 7 Tr BT /F1 10 Tf 1 0 0 1 0 0 Tm [(ab)]TJ /F2 1 Tf (c) Tj ET "+
		"/OC /oc1 BDC 0 0 5 5 re f EMC /OC /oc2 BDC 0 0 5 5 re f EMC 0 Tr Q")
	content, marks := rewrite(items, clipTags([]string{"l1", "l1", ""}))
	text := string(content)
	want := "q\n7 Tr\n/Link <</MCID 0>> BDC\nBT\n"
	if !strings.Contains(text, want) {
		t.Fatalf("the sequence does not begin before BT:\n%s", text)
	}
	if strings.Count(text, "BDC") != 3 || strings.Contains(text, "/Artifact BMC\n(c)") {
		t.Fatalf("marked content inside the clip text object:\n%s", text)
	}
	for _, wantOC := range []string{"ET\nEMC\n/OC /oc1 BDC\n/Artifact BMC\n0 0 5 5 re\nf\nEMC\nEMC\n", "/OC /oc2 BDC\n"} {
		if !strings.Contains(text, wantOC) {
			t.Fatalf("lacks %q:\n%s", wantOC, text)
		}
	}
	if len(marks) != 1 || marks[0].leaf != "l1" || marks[0].first != 0 {
		t.Fatalf("marks %+v", marks)
	}
}

func TestOrdinaryTextObjectKeepsItsSequencesInside(t *testing.T) {
	items := interpretTest(t, "BT /F1 10 Tf 1 0 0 1 0 0 Tm [(ab)]TJ ET")
	content, _ := rewrite(items, clipTags([]string{"l1", "l1"}))
	if !strings.Contains(string(content), "BT\n/F1 10 Tf\n1 0 0 1 0 0 Tm\n/Link <</MCID 0>> BDC") {
		t.Fatalf("content:\n%s", content)
	}
}

func TestClipTextObjectWithTwoLeavesKeepsSequencesInside(t *testing.T) {
	items := interpretTest(t, "7 Tr BT /F1 10 Tf 1 0 0 1 0 0 Tm [(a)-250(b)]TJ ET")
	content, marks := rewrite(items, clipTags([]string{"l1", "l2"}))
	if len(marks) != 2 || strings.Contains(string(content), "BDC\nBT") {
		t.Fatalf("marks %+v:\n%s", marks, content)
	}
}
