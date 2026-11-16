package alttext

import (
	"fmt"
	"html"
	"strings"

	"github.com/sarielhp/relatex/internal/authordir"
	"github.com/sarielhp/relatex/internal/figdesc"
)

// ReviewFile is the name the review page saves the author's decisions under (a download), read
// back by relatex alt import. It is not the sidecar: its [review] table makes the sidecar reader
// refuse it, so a review file saved over relatex/alt.toml by mistake describes nothing.
const ReviewFile = authordir.AltReview

// editable: the sidecar can describe the figure. A description from the LaTeX source wins over
// any entry, and a decorative figure has nothing to describe. A figure's \relatexalt is edited in the
// LaTeX source, or replaced by an entry written with relatex alt set (decision 125, amendment 2).
func editable(figure *figdesc.Figure, state Status) bool {
	return !figure.FromSource() && !figure.FromMacro() && (state.Entry == nil || !state.Entry.Decorative)
}

// writeDecision writes a figure's review controls: the description, the short description and its
// details (decision 125) as editable fields (each remembering the text shown), and the accept
// checkbox. Keys are every name the picture is included under: one decision writes one for each
// (import takes it per key).
func writeDecision(out *strings.Builder, figure *figdesc.Figure, text Description, keys []string) {
	key := html.EscapeString(figure.Key)
	id := fmt.Sprintf("d%d", out.Len())
	fmt.Fprintf(out, "<fieldset class=\"decision\" data-key=\"%s\" data-sha=\"%s\"><legend>Your decision on %s</legend>\n",
		key, html.EscapeString(figure.Identity()), key)
	for _, also := range keys {
		if also != figure.Key {
			fmt.Fprintf(out, "<input type=\"hidden\" class=\"group-key\" value=\"%s\">\n", html.EscapeString(also))
		}
	}
	for _, field := range [][4]string{{"alt", "Description (one sentence, at most 150 characters)", "2", text.Alt},
		{"long", "Short description (at most 40 words; may stay empty)", "3", text.Long},
		{"details", "Details (only when a reader following the text needs more; at most 150 words; may stay empty)", "4", text.Details}} {
		shownText := html.EscapeString(field[3])
		fmt.Fprintf(out, "<label for=\"%s-%s\">%s</label>\n<textarea id=\"%s-%s\" class=\"%s\" rows=\"%s\" data-shown=\"%s\">%s</textarea>\n",
			id, field[0], field[1], id, field[0], field[0], field[2], shownText, shownText)
	}
	fmt.Fprintf(out, "<p><input type=\"checkbox\" id=\"%s-accept\" class=\"accept\"> <label for=\"%s-accept\">Accept this "+
		"description: as shown it becomes reviewed, edited it becomes your text</label></p>\n</fieldset>\n", id, id)
}

// reviewControls are the page's buttons; without scripts they say how to review instead.
const reviewControls = `<div class="controls"><button type="button" class="tick-drafts">Tick every draft</button>
<button type="button" class="save-review">Save the review file</button> <span class="saved" role="status"></span>
<noscript><p>Scripts are off: edit relatex/alt.toml and run relatex alt accept instead.</p></noscript></div>
`

// reviewScript ticks a figure when its text is edited, ticks every draft on request, and saves
// the ticked figures as the review file: each one's text now, the text the page showed and the
// figure's SHA-256, in TOML (JSON's string escapes are TOML's).
const reviewScript = `(function(){
var q=JSON.stringify;
function decisions(){return Array.prototype.slice.call(document.querySelectorAll('fieldset.decision'));}
function covered(f){var keys=[f.dataset.key];f.querySelectorAll('input.group-key').forEach(function(i){keys.push(i.value);});return keys;}
decisions().forEach(function(f){f.querySelectorAll('textarea').forEach(function(t){
t.addEventListener('input',function(){f.querySelector('input.accept').checked=true;});});});
document.querySelectorAll('button.tick-drafts').forEach(function(b){b.addEventListener('click',function(){
decisions().forEach(function(f){if(f.closest('section.draft')){f.querySelector('input.accept').checked=true;}});});});
document.querySelectorAll('button.save-review').forEach(function(b){b.addEventListener('click',function(){
var lines=['# Figure decisions saved from ` + ReviewName + `. This is not ` + figdesc.FileName + `:',
'# take it in with: relatex alt import FILE (in the paper directory)','[` + figdesc.ReviewTable + `]',
'saved = '+q(new Date().toISOString().slice(0,10)),''];
var count=0;
decisions().forEach(function(f){if(!f.querySelector('input.accept').checked){return;}
var alt=f.querySelector('textarea.alt'),long=f.querySelector('textarea.long'),details=f.querySelector('textarea.details');
covered(f).forEach(function(key){lines.push('[figs.'+q(key)+']','alt = '+q(alt.value),'long = '+q(long.value),'details = '+q(details.value),
'shown = '+q(alt.dataset.shown),'shown_long = '+q(long.dataset.shown),'shown_details = '+q(details.dataset.shown),'sha256 = '+q(f.dataset.sha),'');count++;});});
var link=document.createElement('a');
link.href=URL.createObjectURL(new Blob([lines.join('\n')],{type:'application/toml'}));
link.download='` + authordir.AltReviewName + `';document.body.appendChild(link);link.click();link.remove();
document.querySelectorAll('span.saved').forEach(function(s){s.textContent=count+' figures saved in ` + authordir.AltReviewName + ` (keep it as ` + ReviewFile + `)` +
	`; run relatex alt import with that file';});});});
})();`
