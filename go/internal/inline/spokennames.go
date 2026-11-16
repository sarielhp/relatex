package inline

// spokenNames are the glyphs of txfonts, stmaryrd and wasysym that paint no Unicode character
// (tex/relatex-symbol-glyphs.txt) and whose all-lower-case Type 1 name a speech engine would run
// together. Each is the name's own words, abbreviations written out (dbl: double, tp, bt, ex: top,
// bottom, extension of a stretchy piece, eql: equal, inv: inverse); nothing is read from the shape
// that the name does not say, so a piece of a composite symbol (the bars of \Mapsto) is a piece.
var spokenNames = map[string]string{
	"mapsfrom": "Maps from bar", "Mapsto": "Double maps to bar", "Mapsfrom": "Double maps from bar",
	"mmapsto": "Maps to bar", "mmapsfrom": "Maps from bar", "Mmapsto": "Double maps to bar",
	"Mmapsfrom": "Double maps from bar", "colonapprox": "Colon approx", "colonsim": "Colon sim",
	"coloncolonapprox": "Double colon approx", "coloncolonsim": "Double colon sim",
	"solidmultimap": "Solid multimap", "solidmultimapinv": "Solid multimap inverse",
	"solidmultimapboth": "Solid multimap both", "notprecedesorsimilar": "Not precedes or similar",
	"notfollowsorsimilar": "Not follows or similar", "notlessorapproxeql": "Not less or approx equal",
	"notgreaterorapproxeql": "Not greater or approx equal",
	"notprecedesorequal":    "Not precedes or equal",
	"notfollowsorequal":     "Not follows or equal", "strictboth": "Strict both",
	"dashedarrowboth": "Dashed arrow both", "leadstoext": "Leads to extension",
	"colonbar": "Colon bar", "barcolon": "Bar colon", "Colonbar": "Double colon bar", "Barcolon": "Bar double colon",
	"Equalcolon": "Equal double colon", "leftrightarroweq": "Left right arrow equal",
	"Ydown": "Y down", "Yleft": "Y left", "Yright": "Y right",
	"llfloor": "Double left floor", "rrfloor": "Double right floor",
	"llceil": "Double left ceiling", "rrceil": "Double right ceiling",
	"dblbracketlefttp": "Double bracket left top", "dblbracketrighttp": "Double bracket right top",
	"dblbracketleftbt": "Double bracket left bottom", "dblbracketrightbt": "Double bracket right bottom",
	"dblbracketleftex": "Double bracket left extension", "dblbracketrightex": "Double bracket right extension",
	"Leftcircle": "Left circle", "Rightcircle": "Right circle", "ataribox": "Atari box",
}
