package sourcealt

import (
	"testing"

	"github.com/sarielhp/relatex/internal/figdesc"
	"github.com/sarielhp/relatex/internal/rec"
)

// TestChessAltNamesTheDiagramAndItsPosition: the size is named only off 8 by 8, and the position
// only when it is a piece placement.
func TestChessAltNamesTheDiagramAndItsPosition(t *testing.T) {
	cases := []struct {
		files, ranks int64
		fen, want    string
	}{
		{8, 8, "4k3/4p3/8/8/8/8/8/3QK3", "Chess diagram, position in FEN: 4k3/4p3/8/8/8/8/8/3QK3"},
		{5, 5, "1Q3/5/5/5/Q4", "Chess diagram on a 5 by 5 board, position in FEN: 1Q3/5/5/5/Q4"},
		{10, 8, "10/10/10/10/10/10/10/Q9", "Chess diagram on a 10 by 8 board, position in FEN: 10/10/10/10/10/10/10/Q9"},
		{8, 8, "", "Chess diagram"},
		{0, 0, "8/8 x", "Chess diagram"},
	}
	for _, test := range cases {
		if got := ChessAlt(test.files, test.ranks, test.fen); got != test.want {
			t.Errorf("ChessAlt(%d, %d, %q) = %q, want %q", test.files, test.ranks, test.fen, got, test.want)
		}
	}
}

// TestAChessDiagramIsDescribedByItsPosition: a chess diagram's picture without a description of the
// author's gets the derived one (provenance inferred); the author's alt and the figure's
// \relatexalt still win, and a picture that is no chess diagram gets none.
func TestAChessDiagramIsDescribedByItsPosition(t *testing.T) {
	chessRow := func(graphic int64) rec.Map {
		return rec.Map{"code": "picture_alternative", "graphic": graphic, "drawing": "chess", "files": int64(5),
			"ranks": int64(5), "fen": "5/5/5/5/Q4"}
	}
	evidence := Read([]rec.Map{
		chessRow(1),
		chessRow(2), {"code": "picture_alternative", "graphic": int64(2), "alt": "The author's board"},
		chessRow(3), {"code": "picture_alternative", "graphic": int64(3), "figure": int64(7)},
		{"code": "figure_alt", "figure": int64(7), "alt": "Queens", "short": ""},
		{"code": "picture_alternative", "graphic": int64(4), "figure": int64(7)},
		{"code": "picture_alternative", "graphic": int64(5), "figure": int64(8)},
	})
	derived := Description{Alt: "Chess diagram on a 5 by 5 board, position in FEN: 5/5/5/5/Q4", Inferred: true}
	cases := []struct {
		graphic int64
		want    Description
		ok      bool
	}{
		{1, derived, true},
		{2, Description{Alt: "The author's board"}, true},
		{3, Description{Alt: "Queens", Macro: true}, true},
		{4, Description{Alt: "Queens", Macro: true}, true},
		{5, Description{}, false},
	}
	for _, test := range cases {
		if got, ok := evidence.ForPicture(test.graphic); ok != test.ok || got != test.want {
			t.Errorf("graphic %d: %+v %v, want %+v %v", test.graphic, got, ok, test.want, test.ok)
		}
	}
	span := rec.Map{}
	Apply(span, derived)
	if span["alt_provenance"] != figdesc.Inferred || span["alt"] != derived.Alt {
		t.Errorf("applied %v", span)
	}
}
