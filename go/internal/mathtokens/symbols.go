package mathtokens

import "strings"

// symbols maps a TeX math command to the glyph string it typesets (after Fold).
var symbols = buildSymbols()

func buildSymbols() map[string]string {
	table := map[string]string{}
	add := func(glyph string, names ...string) {
		for _, name := range names {
			table[name] = glyph
		}
	}
	greek := "alpha α beta β gamma γ delta δ epsilon ϵ varepsilon ε zeta ζ eta η theta θ vartheta ϑ iota ι kappa κ lambda λ mu μ nu ν " +
		"xi ξ pi π varpi ϖ rho ρ varrho ϱ sigma σ varsigma ς tau τ upsilon υ phi ϕ varphi φ chi χ psi ψ omega ω " +
		"Gamma Γ Delta Δ Theta Θ Lambda Λ Xi Ξ Pi Π Sigma Σ Upsilon Υ Phi Φ Psi Ψ Omega Ω"
	fields := strings.Fields(greek)
	for index := 0; index+1 < len(fields); index += 2 {
		table[fields[index]] = fields[index+1]
	}
	add("∞", "infty")
	add("≤", "le", "leq", "leqslant")
	add("≥", "ge", "geq", "geqslant")
	add("≠", "ne", "neq")
	add("≈", "approx")
	add("∼", "sim")
	add("≃", "simeq")
	add("≅", "cong")
	add("≡", "equiv")
	add("∝", "propto")
	add("≪", "ll")
	add("≫", "gg")
	add("±", "pm")
	add("∓", "mp")
	add("×", "times")
	add("÷", "div")
	add("·", "cdot")
	add("∘", "circ")
	add("∗", "ast")
	add("⋆", "star")
	add("•", "bullet")
	add("→", "to", "rightarrow")
	add("←", "leftarrow", "gets")
	add("↔", "leftrightarrow")
	add("⇒", "Rightarrow", "implies")
	add("⇐", "Leftarrow")
	add("⇔", "Leftrightarrow", "iff")
	add("↦", "mapsto")
	add("∈", "in")
	add("∉", "notin")
	add("∋", "ni")
	add("⊂", "subset")
	add("⊆", "subseteq")
	add("⊃", "supset")
	add("⊇", "supseteq")
	add("∪", "cup")
	add("∩", "cap")
	add("∅", "emptyset", "varnothing")
	add("∀", "forall")
	add("∃", "exists")
	add("¬", "neg", "lnot")
	add("∧", "wedge", "land")
	add("∨", "vee", "lor")
	add("∂", "partial")
	add("∇", "nabla")
	add("∑", "sum")
	add("∏", "prod")
	add("∫", "int")
	add("∮", "oint")
	add("√", "sqrt")
	add("…", "ldots", "dots", "dotsc", "dotsb")
	add("⋯", "cdots")
	add("⋮", "vdots")
	add("⋱", "ddots")
	add("′", "prime")
	add("ℓ", "ell")
	add("ℏ", "hbar")
	add("ℜ", "Re")
	add("ℑ", "Im")
	add("∠", "angle")
	add("⊥", "perp")
	add("‖", "|", "Vert")
	add("|", "vert", "mid")
	add("{", "{", "lbrace")
	add("}", "}", "rbrace")
	add("⟨", "langle")
	add("⟩", "rangle")
	add("⌊", "lfloor")
	add("⌋", "rfloor")
	add("⌈", "lceil")
	add("⌉", "rceil")
	add("#", "#")
	add("%", "%")
	add("&", "&")
	add("$", "$")
	add("_", "_")
	add("†", "dagger")
	for _, name := range strings.Fields("sin cos tan cot sec csc sinh cosh tanh coth arcsin arccos arctan log ln lg exp lim limsup liminf " +
		"max min sup inf det dim ker deg gcd Pr hom arg") {
		table[name] = name
	}
	return table
}

// invisible commands typeset no glyph of their own; the arguments that follow them are read
// as ordinary math (\frac{a}{b} has the glyphs of a and b, the bar is a rule).
var invisible = map[string]bool{}

func init() {
	for _, name := range strings.Fields(`left right big Big bigg Bigg bigl bigr Bigl Bigr biggl biggr Biggl Biggr middle
		displaystyle textstyle scriptstyle scriptscriptstyle limits nolimits frac dfrac tfrac cfrac binom dbinom tbinom
		mathbf mathrm mathit mathsf mathtt mathcal mathscr mathfrak mathbb boldsymbol bm text textrm textbf textit
		textsf texttt operatorname mbox hbox mathop mathrel mathbin mathord mathopen mathclose mathpunct
		hat bar tilde vec dot ddot check breve acute grave widehat widetilde overline underline overbrace underbrace
		overrightarrow overleftarrow stackrel overset underset substack
		label nonumber notag quad qquad hspace hfill vspace smallskip medskip bigskip phantom hphantom vphantom
		begin end array cases split aligned gathered pmatrix bmatrix vmatrix Vmatrix matrix smallmatrix
		nobreak allowbreak relax protect color textcolor boxed`) {
		invisible[name] = true
	}
}
