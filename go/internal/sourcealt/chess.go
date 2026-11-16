package sourcealt

import (
	"fmt"
	"regexp"

	"github.com/sarielhp/relatex/internal/rec"
)

// fenPlacement is FEN's piece placement field as chessboard stores a board: ranks of piece letters
// and counts of empty squares, separated by slashes.
var fenPlacement = regexp.MustCompile(`\A[KQRBNPkqrbnp0-9]+(/[KQRBNPkqrbnp0-9]+)*\z`)

// standardSide is the side of a standard chessboard, which ChessAlt does not name.
const standardSide = 8

// chess is the description relatex derives for a chess diagram (decision 163) from the position its
// picture_alternative record gives: false when the picture is no chess diagram.
func chess(row rec.Map) (Description, bool) {
	if row["drawing"] != "chess" {
		return Description{}, false
	}
	files, _ := row["files"].(int64)
	ranks, _ := row["ranks"].(int64)
	fen, _ := row["fen"].(string)
	return Description{Alt: ChessAlt(files, ranks, fen), Inferred: true}, true
}

// ChessAlt is a chess diagram's text alternative (decision 163): what it is, the board's size when
// it is not 8 by 8, and the position in FEN's piece placement when the source gives it.
func ChessAlt(files, ranks int64, fen string) string {
	alt := "Chess diagram"
	if files > 0 && ranks > 0 && (files != standardSide || ranks != standardSide) {
		alt += fmt.Sprintf(" on a %d by %d board", files, ranks)
	}
	if fenPlacement.MatchString(fen) {
		alt += ", position in FEN: " + fen
	}
	return alt
}
