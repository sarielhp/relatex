package fontmodel

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func failure(run func()) (message string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			message = recovered.(rec.Failure).Message
		}
	}()
	run()
	return ""
}

func TestVPLNumber(t *testing.T) {
	cases := map[[2]string]int64{{"C", "A"}: 65, {"O", "101"}: 65, {"D", "65"}: 65, {"H", "41"}: 65}
	for input, want := range cases {
		if got := VPLNumber(input[0], input[1]); got != want {
			t.Errorf("VPLNumber(%q, %q) = %d, want %d", input[0], input[1], got, want)
		}
	}
	if got := failure(func() { VPLNumber("D", "256") }); got != "Unsupported virtual font slot" {
		t.Errorf("slot 256: %q", got)
	}
	if got := failure(func() { VPLNumber("O", "9") }); got != "Malformed virtual font slot" {
		t.Errorf("octal 9: %q", got)
	}
}
