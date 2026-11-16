package html

// The page's visual accessibility (plan accessibility §7.2.4, decision 095): a visible focus ring,
// no motion when the reader asks for none, legible rules and borders in forced colours, and
// non-text contrast of the page's own rules, borders and focus ring. CSS only: the EPUB, which
// holds no script, keeps all of it.

// lightTheme is the light palette: the ink is the PDF's text paint (rgb(0, 0, 0)), which the
// browser checks measure with the theme forced to light.
const lightTheme = `color-scheme: light; --bg: #faf9f6; --ink: rgb(0, 0, 0); --muted: #5c5f66; --link: #174f86; ` +
	`--rule: #858585; --card: #fff; --target: #fff1bd; --theorem: #2f6fd6; --theorem-bg: #eef3fb; --proof: #4f9a3c; ` +
	`--proof-bg: #eef7ea; --figure-bg: transparent; --focus: #174f86; --alert: #c0392b;`

// darkTheme is the dark palette, the page's own when the reader's system states no preference.
// Rules, borders and the focus ring reach 3:1 on every background they sit on (WCAG 1.4.11,
// visualContrastPairs), text 4.5:1.
const darkTheme = `color-scheme: dark; --bg: #1b1d22; --ink: #e9e6df; --muted: #a8a59e; --link: #8ab4f8; --rule: #767b85; ` +
	`--card: #262a31; --target: #5a4f1a; --theorem: #5b8def; --theorem-bg: #202836; --proof: #6fae5c; --proof-bg: #1f2a21; ` +
	`--figure-bg: #fff; --focus: #8ab4f8; --alert: #e06c5f;`

// visualStyle is the stylesheet's part for focus, motion and forced colours.
//   - Every focusable element (links, the skip link, the theme toggle, a scrollable display that
//     preview.js makes focusable) shows a 2 px ring in --focus, outside the element, when the
//     keyboard focuses it (WCAG 2.4.7, 2.4.13's size). scroll-padding keeps a focused element
//     clear of the fixed theme toggle (2.4.11).
//   - prefers-reduced-motion: the fading highlight of a link's target is replaced by a still
//     outline, which changes no text colour.
//   - forced-colors (Windows high contrast): the focus ring is the system Highlight; rules drawn in
//     the text colour (TeX rules without a colour of their own, table rules), which are painted
//     backgrounds the forced palette would drop, become CanvasText; structure borders
//     CanvasText; figures keep their own colours on white; text the author hid stays hidden.
const visualStyle = `html { scroll-padding-top: 2.5rem; }
main [aria-hidden="true"][data-kind="text"] { visibility: hidden; }
:focus-visible { outline: 2px solid var(--focus); outline-offset: 2px; }
a:focus-visible, button:focus-visible, [tabindex]:focus-visible { outline: 2px solid var(--focus); outline-offset: 2px; }
@media (prefers-reduced-motion: reduce) { :target { animation: none; outline: 2px dashed var(--muted); outline-offset: 4px; } html { scroll-behavior: auto; } }
@media (forced-colors: active) { :focus-visible, a:focus-visible, button:focus-visible, [tabindex]:focus-visible { outline: 2px solid Highlight; } ` +
	`.theorem, .proof, .theorem[class*="block-"] { border-left-color: CanvasText; } ` +
	`[data-kind="rule"]:not([data-color]), [data-rule], [data-rich-rule] { forced-color-adjust: none; background: CanvasText !important; } ` +
	`img, svg[data-kind="image"] { forced-color-adjust: none; background: #fff; } ` +
	`main [aria-hidden="true"][data-kind="text"] { forced-color-adjust: none; color: transparent !important; } }
`
