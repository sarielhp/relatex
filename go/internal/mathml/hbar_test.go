package mathml

import "testing"

func TestCollapseHbar(t *testing.T) {
	overstrike := `<mrow><mi mathvariant="normal">¯</mi><mspace width="-0.500em"/><mi>ℎ</mi></mrow>`
	for name, test := range map[string]struct{ source, want string }{
		"inline": {
			source: `<math><msub><mi>𝜎</mi><mn>0</mn></msub><mo>=</mo>` + overstrike + `</math>`,
			want:   `<math><msub><mi>𝜎</mi><mn>0</mn></msub><mo>=</mo><mi mathvariant="normal">ℏ</mi></math>`,
		},
		"superscript": {
			source: `<math><msup>` + overstrike + `<mn>2</mn></msup></math>`,
			want:   `<math><msup><mi mathvariant="normal">ℏ</mi><mn>2</mn></msup></math>`,
		},
		"another negative kern": {
			source: `<math><mrow><mi mathvariant="normal">¯</mi><mspace width="-0.167em"/><mi>ℎ</mi></mrow></math>`,
			want:   `<math><mi mathvariant="normal">ℏ</mi></math>`,
		},
		"bar accent": {
			source: `<math><mover><mi>ℎ</mi><mo>¯</mo></mover></math>`,
			want:   `<math><mover><mi>ℎ</mi><mo>¯</mo></mover></math>`,
		},
		"overline": {
			source: `<math><mover accent="true"><mi>h</mi><mo>¯</mo></mover></math>`,
			want:   `<math><mover accent="true"><mi>h</mi><mo>¯</mo></mover></math>`,
		},
		"amsfonts hbar": {
			source: `<math><mi mathvariant="normal">ℏ</mi></math>`,
			want:   `<math><mi mathvariant="normal">ℏ</mi></math>`,
		},
		"positive kern": {
			source: `<math><mrow><mi mathvariant="normal">¯</mi><mspace width="0.500em"/><mi>ℎ</mi></mrow></math>`,
			want:   `<math><mrow><mi mathvariant="normal">¯</mi><mspace width="0.500em"/><mi>ℎ</mi></mrow></math>`,
		},
		"upright h": {
			source: `<math><mrow><mi mathvariant="normal">¯</mi><mspace width="-0.500em"/><mi>h</mi></mrow></math>`,
			want:   `<math><mrow><mi mathvariant="normal">¯</mi><mspace width="-0.500em"/><mi>h</mi></mrow></math>`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := CollapseHbar(test.source); got != test.want {
				t.Fatalf("CollapseHbar() = %s, want %s", got, test.want)
			}
		})
	}
}
