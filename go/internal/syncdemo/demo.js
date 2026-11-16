(() => {
  'use strict';
  // Click-to-source demo of word-level SyncTeX. Each word (.w) and formula (.sx) of the paper carries
  // its source ranges in data-o ("FILE:LINE:START:END", or "FILE:LINE" when only the line is known;
  // columns are 0-based character offsets); each source word or command (.t) carries its column.
  const paper = document.getElementById('paper');
  const source = document.getElementById('source');
  const status = document.getElementById('status');
  const panels = [...source.querySelectorAll('.panel')];
  const tabs = [...source.querySelectorAll('.tab')];
  const names = tabs.map((tab) => tab.textContent);
  const parse = (text) => text.split(' ').map((part) => {
    const [f, l, s, e] = part.split(':');
    return { f: +f, l: +l, s: s === undefined ? -1 : +s, e: e === undefined ? -1 : +e };
  });
  const items = [...paper.querySelectorAll('[data-o]')].map((el) => ({ el, ranges: parse(el.dataset.o) }));
  const tokens = [...source.querySelectorAll('.t')].map((el) => {
    const c = +el.dataset.c;
    return { el, f: +el.closest('.panel').dataset.f, l: +el.closest('.ln').dataset.l, c, d: c + [...el.textContent].length };
  });
  const lineOf = (f, l) => (panels[f] ? panels[f].children[l - 1] : null);
  let marked = [];
  let shown = -1;

  const clear = () => { marked.forEach((el) => el.classList.remove('hit', 'near')); marked = []; };
  const mark = (el, kind) => { if (el) { el.classList.add(kind); marked.push(el); } };
  const show = (f) => {
    if (f === shown || !panels[f]) return;
    shown = f;
    panels.forEach((panel, i) => { panel.hidden = i !== f; });
    tabs.forEach((tab, i) => tab.setAttribute('aria-selected', String(i === f)));
  };
  const say = (text) => { status.textContent = text; };
  const reveal = (el) => { if (el) el.scrollIntoView({ block: 'center', inline: 'nearest' }); };
  const where = (r) => names[r.f] + ':' + r.l + (r.s >= 0 ? ':' + (r.s + 1) : '');

  // From the paper: highlight the source words the ranges cover, or the line when only it is known.
  const toSource = (item) => {
    clear();
    const first = item.ranges[0];
    show(first.f);
    item.el.classList.add('hit'); marked.push(item.el);
    const found = [];
    item.ranges.forEach((r) => {
      show(first.f);
      const covered = r.s < 0 ? [] : tokens.filter((t) => t.f === r.f && t.l === r.l && t.c < r.e && t.d > r.s);
      if (covered.length > 0) {
        covered.forEach((t) => { mark(t.el, 'hit'); found.push(t.el); });
      } else {
        const line = lineOf(r.f, r.l);
        mark(line, 'near'); found.push(line);
      }
    });
    reveal(found[0]);
    say(where(first) + (first.s < 0 ? ' (line only)' : '') + (item.ranges.length > 1 ? ' and ' + (item.ranges.length - 1) + ' more' : ''));
  };

  // From the source: highlight the paper's words (formulas) whose ranges overlap the clicked word.
  const toPaper = (token) => {
    clear();
    mark(token.el, 'hit');
    const same = (r) => r.f === token.f && r.l === token.l;
    let found = items.filter((it) => it.ranges.some((r) => same(r) && r.s >= 0 && r.s < token.d && r.e > token.c));
    let kind = 'hit';
    if (found.length === 0) { found = items.filter((it) => it.ranges.some(same)); kind = 'near'; }
    found.forEach((it) => mark(it.el, kind));
    reveal(found.length > 0 ? found[0].el : null);
    say(names[token.f] + ':' + token.l + ':' + (token.c + 1) + (found.length === 0 ? ' has no counterpart in the paper' : kind === 'near' ? ' (line only)' : ''));
  };

  const itemOf = (el) => { const owner = el.closest('[data-o]'); return owner ? items.find((it) => it.el === owner) : null; };
  const tokenOf = (el) => { const owner = el.closest('.t'); return owner ? tokens.find((t) => t.el === owner) : null; };

  paper.addEventListener('click', (event) => {
    const item = itemOf(event.target);
    if (!item) return;
    toSource(item);
    // A word inside an external link finds its source instead of leaving the demo (Ctrl-click follows the link).
    const link = event.target.closest('a[href]');
    if (link && !link.getAttribute('href').startsWith('#') && !event.ctrlKey && !event.metaKey) event.preventDefault();
  });
  source.addEventListener('click', (event) => {
    const tab = event.target.closest('.tab');
    if (tab) { show(tabs.indexOf(tab)); return; }
    const token = tokenOf(event.target);
    if (token) toPaper(token);
  });
  document.addEventListener('keydown', (event) => {
    if (event.key !== 'Enter' || !event.target.matches('.w, .sx, .t')) return;
    event.preventDefault();
    event.target.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true }));
  });
  show(0);
  say('Click a word or a formula of the paper to find its LaTeX, or a word of the source to find it in the paper.');
  window.relatexSyncDemo = { toSource, toPaper, items, tokens };
})();
