package evidence

import (
	"fmt"

	"github.com/sarielhp/relatex/internal/rec"
)

// Report runs one evidence class by name, for comparison with its Ruby counterpart.
func Report(name string, records []rec.Map) (result rec.Map, err error) {
	defer rec.Recover(&err)
	switch name {
	case "math":
		return MathReport(records), nil
	case "citations":
		return CitationReport(records), nil
	case "semantic":
		return SemanticReport(records), nil
	}
	return nil, fmt.Errorf("unknown evidence %q", name)
}
