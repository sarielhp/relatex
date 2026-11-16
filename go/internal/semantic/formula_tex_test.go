package semantic

import (
	"path/filepath"
	"testing"

	"github.com/sarielhp/relatex/internal/rec"
)

func TestMarkdownFormulaMath(t *testing.T) {
	saved := Root
	Root, _ = filepath.Abs("../../..")
	defer func() { Root = saved }()
	writer := &markdown{}
	writer.export = &export{w: writer, nodes: rec.Map{}, document: rec.Map{}}
	cases := []struct {
		field rec.Map
		want  string
		ok    bool
	}{
		{rec.Map{"text": "x_i+1"}, "x_i+1", true},
		{rec.Map{"text": "x & =1", "environment": "align*"}, "\\begin{aligned}\nx & =1\n\\end{aligned}", true},
		{rec.Map{"text": "x & =1 \\tag{3}", "environment": "align*"}, "", false},
		{rec.Map{"text": "\\text{a $b$}"}, "", false},
		{rec.Map{"text": "\\notacommand x"}, "", false},
	}
	for _, c := range cases {
		got, ok := writer.formulaMath(rec.Map{"formula_tex": c.field})
		if got != c.want || ok != c.ok {
			t.Errorf("formulaMath(%v) = %q, %v; want %q, %v", c.field, got, ok, c.want, c.ok)
		}
	}
	if _, ok := writer.formulaMath(rec.Map{}); ok {
		t.Error("formulaMath without formula_tex")
	}
}
