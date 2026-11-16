package mathexport

// speechWords are the operator names whose written form a text-to-speech voice mispronounces, and
// the literal intent MathCAT reads for them (decision 114, amendment 2026-11-10): div -> "divergence
// of", supp -> "support of", dist -> "distance of". A literal is a bare word on the mi that carries
// the name; it is set only where the speech pass applies the name to an argument (applyFunctions
// writes U+2061 there), so a name standing alone keeps its own reading. Never a Core intent concept
// (determinant, norm, matrix, vector): MathCAT has special rules for those and fails on some
// ("ERROR Pattern match/replacement failure" for determinant).
var speechWords = map[string]string{
	"div":    "divergence",
	"supp":   "support",
	"dist":   "distance",
	"diam":   "diameter",
	"vol":    "volume",
	"diag":   "diagonal",
	"sgn":    "sign",
	"lcm":    "least-common-multiple",
	"gcd":    "greatest-common-divisor",
	"deg":    "degree",
	"id":     "identity",
	"Id":     "identity",
	"Stab":   "stabilizer",
	"rk":     "rank",
	"Aut":    "automorphism-group",
	"dom":    "domain",
	"argmax": "arg-max",
	"argmin": "arg-min",
	"im":     "image",
	"conv":   "convex-hull",
	"Lip":    "lipschitz-constant",
	"coker":  "cokernel",
	"codim":  "codimension",
	"Hess":   "hessian",
	"proj":   "projection",
	"cl":     "closure",
}

// spokenName gives the name applied to an argument (or the base of its scripts) its literal intent. A
// name the collector wrote as text becomes an mi (the same upright letters): the browser check and the
// acceptance accept a literal on an mi or an mo.
func spokenName(function *Node) {
	for baseWrapper(function) {
		function = function.Elements()[0]
	}
	if function.Name != "mi" && !operatorText(function) {
		return
	}
	if word, known := speechWords[functionWord(function)]; known {
		function.Name = "mi"
		setIntent(function, word)
	}
}
