package tables

import (
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

// A dashed rule's end halves (arydshln \hdashline) are rules with a fixed height and a running
// depth: the flow observes them as running rules and the table's painted rules cover them.
func TestRunningRuleWithRunningDepthIsCovered(t *testing.T) {
	observed := rec.Map{"code": "unclassified_running_rule", "table": int64(14), "page": int64(22), "width": int64(131072),
		"height": int64(26214), "depth": int64(runningSize), "box_height": int64(26214), "box_depth": int64(0), "box_kind": "hlist"}
	painted := rec.Map{"code": "rich_table_rule", "table": int64(14), "page": int64(22), "width": int64(131072),
		"height": int64(26214), "depth": int64(runningSize), "box_height": int64(26214), "box_depth": int64(0), "box_kind": "hlist"}
	e := &rich{events: []rec.Map{painted}, diagnostics: []rec.Map{observed}}
	e.removeProvenDiagnostics(int64(14))
	if len(e.diagnostics) != 0 {
		t.Fatalf("diagnostics left: %v", e.diagnostics)
	}
}
