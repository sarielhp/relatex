package browser

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
)

// AxeVersion is the axe-core release vendored as lib/relatex/axe.min.js (recorded in axe.json). The
// injected file must report it, so a swapped file fails.
const AxeVersion = "4.14.0"

// AxeTags are the axe-core rules run: WCAG 2.0, 2.1 and 2.2, levels A and AA (decision 080).
var AxeTags = []string{"wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"}

// axeBase bounds one axe run on a page of up to axeBaseElements elements. A long book can take
// minutes; a run that takes longer leaves the accessibility unmeasured, never the page unverified.
const axeBase = 300 * time.Second

// axeBaseElements is the page size axeBase covers; every axeStep elements beyond it add axeGrowth.
const axeBaseElements = 5000

// axeStep is how many elements one axeGrowth of extra time covers.
const axeStep = 5000

// axeGrowth is the extra time each axeStep elements beyond axeBaseElements buys.
const axeGrowth = 30 * time.Second

// axeCap bounds any axe run, however large the page.
const axeCap = 30 * time.Minute

// axeLimitFor is the limit of one axe run on a page of elements elements: axeBase, plus axeGrowth
// for each axeStep elements beyond axeBaseElements, capped at axeCap.
func axeLimitFor(elements int) time.Duration {
	limit := axeBase
	if elements > axeBaseElements {
		limit += time.Duration((elements-axeBaseElements+axeStep-1)/axeStep) * axeGrowth
	}
	return min(limit, axeCap)
}

// axeNodes is how many violating elements a rule keeps in the report; its count keeps them all.
const axeNodes = 50

// axeFailure is how much of axe's failure summary a node keeps.
const axeFailure = 500

// axeScript runs axe on the whole page and returns its violations as JSON, in the report's shape.
var axeScript = `(async () => {
  const result = await axe.run(document, {runOnly: {type: 'tag', values: %s}, resultTypes: ['violations']});
  return JSON.stringify(result.violations.map(v => ({id: v.id, impact: v.impact, tags: v.tags, help: v.help,
    count: v.nodes.length,
    nodes: v.nodes.slice(0, %d).map(n => ({target: n.target, failure: (n.failureSummary || '').slice(0, %d)}))})));
})()`

// themes are the page's two themes in the order axe checks them: the light one the checks force
// (the PDF's paint), then the dark one, each chosen as the theme toggle chooses it. Colour contrast differs.
var themes = []struct{ name, expression string }{
	{"light", "document.documentElement.setAttribute('data-theme', 'light')"},
	{"dark", "document.documentElement.setAttribute('data-theme', 'dark')"},
}

// Accessibility runs axe-core on the loaded page, in the light and the dark theme, and returns
// {"axe", "tags", "themes": {"light": [violation...], "dark": [...]}}. A violation is {"id",
// "impact", "tags", "help", "count", "nodes": [{"target", "failure"}]}. It runs after the inventory,
// which never sees axe; the page gets no script element (CDP evaluation is outside its
// Content-Security-Policy). A run that throws or takes longer than axeLimitFor allows is returned
// as an error: phase 0 measures, so the page's verification stands without it.
func (c *Checker) Accessibility() (rec.Map, error) {
	c.browser.Page("Runtime.evaluate", rec.Map{"expression": script("axe.min.js")})
	if version, _ := c.browser.Evaluate("typeof axe === 'object' ? axe.version : null").(string); version != AxeVersion {
		rec.Fail("Injected axe-core is %q, not the vendored %s", version, AxeVersion)
	}
	tags := make([]any, len(AxeTags))
	for index, tag := range AxeTags {
		tags[index] = tag
	}
	results := rec.Map{}
	elements, _ := rec.Number(c.browser.Evaluate("document.getElementsByTagName('*').length"))
	limit := axeLimitFor(int(elements))
	for _, theme := range themes {
		c.browser.Page("Runtime.evaluate", rec.Map{"expression": theme.expression})
		violations, err := c.axeRun(limit)
		if err != nil {
			return nil, fmt.Errorf("axe-core in the %s theme: %w", theme.name, err)
		}
		results[theme.name] = violations
	}
	return rec.Map{"axe": AxeVersion, "tags": tags, "themes": results}, nil
}

// axeRun is one awaited axe.run, bounded by limit: Browser.Evaluate does not await a promise.
func (c *Checker) axeRun(limit time.Duration) ([]any, error) {
	quoted := `["` + strings.Join(AxeTags, `","`) + `"]`
	expression := fmt.Sprintf(axeScript, quoted, axeNodes, axeFailure)
	response, err := c.browser.commandWithin(c.browser.session, "Runtime.evaluate",
		rec.Map{"expression": expression, "awaitPromise": true, "returnByValue": true}, limit)
	if errors.Is(err, errTimedOut) {
		return nil, fmt.Errorf("no result within %s", limit)
	}
	if err != nil {
		rec.Fail("%v", err)
	}
	object, _ := response.(ojson.Object)
	if details := object.Get("exceptionDetails"); details != nil {
		return nil, fmt.Errorf("JavaScript error: %v", rec.Inspect(ojson.Plain(details)))
	}
	result, _ := object.Get("result").(ojson.Object)
	text, ok := result.Get("value").(string)
	if !ok {
		return nil, errors.New("axe-core returned no JSON")
	}
	value, err := ojson.Decode(bytes.NewReader([]byte(text)))
	if err != nil {
		return nil, err
	}
	violations, ok := ojson.Plain(value).([]any)
	if !ok {
		return nil, errors.New("axe-core violations are not a list")
	}
	return violations, nil
}
