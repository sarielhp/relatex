(() => {
  const tree = element => ({
    tag: element.localName,
    attributes: Object.fromEntries([...element.attributes].filter(a => a.name !== 'id').map(a => [a.name, a.value]).sort()),
    children: [...element.childNodes].flatMap(child => {
      if (child.nodeType === Node.ELEMENT_NODE) return [tree(child)];
      if (child.nodeType === Node.TEXT_NODE && child.textContent.trim()) return [child.textContent];
      return [];
    })
  });
  const visible = element => {
    const box = element.getBoundingClientRect();
    return box.width > 0 && box.height > 0 && getComputedStyle(element).visibility === 'visible';
  };
  const bareSpan = element => {
    const type = element.dataset.kind;
if (type === 'table_grid') {
  if (element.dataset.schema === 'table-2') {
    const copies = [...element.tBodies].flatMap(body => [...body.rows]).flatMap(row => [...row.cells]).map(cell => ({
      ...JSON.parse(cell.dataset.cellMeta),
      content: spanList([...cell.querySelector(':scope > [data-cell-content]').children]),
      rules: [...cell.querySelectorAll(':scope > [data-rich-rule]')].map(rule => JSON.parse(rule.dataset.richRule))
    }));
    const unique = new Map();
    for (const cell of copies) {
      if (unique.has(cell.id) && JSON.stringify(unique.get(cell.id)) !== JSON.stringify(cell)) throw new Error('Repeated rich cell changed content or geometry');
      unique.set(cell.id, cell);
    }
    const cells = [...unique.values()].sort((a,b) => a.row - b.row || a.column - b.column);
    return {type, id: element.id, grid: {...JSON.parse(element.dataset.gridMeta), cells}};
  }
  const rule = e => Object.fromEntries((e.dataset.rule === 'vertical' ? ['width','height','depth','x'] : ['width','height','depth','after_row'])
    .map(key => [key, Number(e.getAttribute('data-' + key.replaceAll('_','-')))]));
  const rows = [...element.tBodies[0].rows].map(row => [...row.cells].map(cell => ({
    text: cell.querySelector(':scope > [data-cell-text]').textContent,
    width_sp: Number(cell.dataset.widthSp), height_sp: Number(cell.dataset.heightSp), depth_sp: Number(cell.dataset.depthSp),
    font_size_sp: Number(cell.dataset.fontSizeSp),
    borders: [...cell.querySelectorAll(':scope > [data-rule="vertical"]')].map(rule),
    ...(() => {
      const runs = [...cell.querySelector(':scope > [data-cell-text]').children].map(e => ({
        text: e.textContent, ...(e.dataset.textStyle ? {style: JSON.parse(e.dataset.textStyle)} : {}),
        ...(e.hasAttribute('lang') ? {lang: e.getAttribute('lang')} : {})
      }));
      return runs.length ? {runs} : {};
    })()
  })));
  return {type, id: element.id, grid: {preamble: element.dataset.preamble, width_sp: Number(element.dataset.widthSp), rows,
    horizontal_borders: [...element.querySelectorAll('[data-rule="horizontal"]')].map(rule)}};
}
if (type === 'image') return {type, id: element.id, asset: element.dataset.asset, width_sp: Number(element.dataset.widthSp), height_sp: Number(element.dataset.heightSp),
  ...(element.dataset.placement ? {placement: JSON.parse(element.dataset.placement), depth_sp: Number(element.dataset.depthSp)} : {}),
  // A figure description (relatex/alt.toml); data-undescribed marks a placeholder, never an alt. A
  // figure shown as inline SVG (decision 146) is an image (role img) its aria-label names.
  ...(element.localName === 'svg' ? (element.getAttribute('role') === 'img' && element.getAttribute('aria-label') && !element.hasAttribute('data-undescribed') ?
    {alt: element.getAttribute('aria-label')} : {}) :
    element.getAttribute('alt') && !element.hasAttribute('data-undescribed') ? {alt: element.getAttribute('alt')} : {}),
  // A decorative image (decision 096): an empty alternative screen readers skip.
  ...(element.getAttribute('alt') === '' && element.getAttribute('role') === 'presentation' ? {decorative: true} : {}),
  // A long description (decision 110): the text the image names with aria-describedby, inside the
  // disclosure it names with aria-details; a caption's aria-describedby is no long description.
  ...(element.hasAttribute('aria-details') ? {long_description: (() => {
    const text = document.getElementById(element.getAttribute('aria-describedby'));
    const disclosure = document.getElementById(element.getAttribute('aria-details') ?? '');
    return disclosure?.matches('details[data-kind="long-description"]') && disclosure.contains(text) ? text.textContent : '';
  })()} : {}),
  // A figure's details (decision 125): the text of the disclosure nested in the long description's,
  // read only there; the document's description_details must equal it, present iff the page shows one.
  ...(() => {
    const disclosure = document.getElementById(element.getAttribute('aria-details') ?? '');
    if (!disclosure?.matches('details[data-kind="long-description"]')) return {};
    const nested = disclosure.querySelectorAll(':scope > details.figure-details');
    return nested.length ? {description_details: [...nested].map(d => d.querySelector(':scope > p')?.textContent ?? '').join('\n')} : {};
  })()};
    // A formula inside a \hyperref text is the link itself: its label and the target it opens.
    if (type === 'math') return {type, node: element.querySelector('math').id,
      ...(element.dataset.referenceLabel ? {link_label: element.dataset.referenceLabel, link_target: element.getAttribute('href').slice(1)} : {})};
    if (type === 'note_reference') return {type, text: element.textContent, target: element.getAttribute('href').slice(1), callsite: element.id};
    // An author's QED mark (decision 176) shows its glyphs or its image; a drawn one shows the open square.
    if (type === 'symbol') return {type, name: element.dataset.name,
      ...(element.dataset.mark === 'text' ? {text: element.textContent} : {}),
      ...(element.dataset.mark === 'image' ? {image: bareSpan(element.querySelector(':scope > [data-kind="image"]'))} : {})};
    if (type === 'rule') {
      // The painted box must show at its TeX size; a different size reports what showed.
      const px = sp => sp * 96 / (65536 * 72.27), box = element.getBoundingClientRect();
      const [width_sp, height_sp, depth_sp] = [element.dataset.widthSp, element.dataset.heightSp, element.dataset.depthSp].map(Number);
      const fits = Math.abs(box.width - px(width_sp)) <= 1 && Math.abs(box.height - px(height_sp + depth_sp)) <= 1;
      const margins = element.dataset.marginLeftSp === undefined ? {} :
        {margin_left_sp: Number(element.dataset.marginLeftSp), margin_right_sp: Number(element.dataset.marginRightSp)};
      return {type, width_sp, height_sp, depth_sp, ...(element.dataset.color ? {color: element.dataset.color} : {}), ...margins,
        ...(fits ? {} : {rendered: [box.width, box.height]})};
    }
    if (type === 'reference') return {type, text: element.textContent, label: element.dataset.label, target: element.getAttribute('href').slice(1)};
    // A glyph with no character but a name in its symbol font (marvosym's Tumbler).
    return {type, text: element.textContent, ...(element.dataset.glyphName ? {glyph_name: element.dataset.glyphName} : {})};
  };
  const span = element => {
    // A linked image (the ORCID logo) is its image inside the link: the wrapper adds the link only.
    const shown = element.dataset.kind === 'linked' ? element.querySelector(':scope > a > img[data-kind="image"]') : element;
    const result = bareSpan(shown);
    if (element.dataset.externalLinkId) result.external_link = element.dataset.externalLinkId;
    if (element.dataset.textStyle) result.style = JSON.parse(element.dataset.textStyle);
    // A decorated span reads back as its decoration only when the browser paints that line
    // (underline, or line-through for a strike; decision 134); an unpainted one reads as such.
    if (element.dataset.decoration) {
      const line = {underline: 'underline', strike: 'line-through'}[element.dataset.decoration];
      const painted = getComputedStyle(element).textDecorationLine.split(' ');
      result.decoration = line && painted.includes(line) ? element.dataset.decoration : 'unpainted ' + element.dataset.decoration;
    }
    // The break between two lines of one heading (decision 126).
    if (element.hasAttribute('data-line-break')) result.line_break = true;
    // The span's language (plan accessibility 7.3) and text the author hid, hidden from screen
    // readers too (8.2, decision 1).
    if (element.hasAttribute('lang')) result.lang = element.getAttribute('lang');
    if (element.hasAttribute('aria-hidden')) result.aria_hidden = element.getAttribute('aria-hidden');
    // An inline fill color, as #rrggbb: the computed value of the declared one (a theme custom
    // property resolves to the ink itself in the light theme the checks measure).
    const fill = element.style.color ? getComputedStyle(element).color.match(/^rgb\((\d+), (\d+), (\d+)\)$/) : null;
    if (fill && (result.type === 'text' || result.type === 'reference')) result.color = '#' + fill.slice(1).map(v => Number(v).toString(16).padStart(2, '0')).join('');
    for (const field of ['citation','citation_link','bibliography_entry']) {
      const value = element.getAttribute('data-' + field.replaceAll('_','-'));
      if (value !== null) result[field] = Number(value);
    }
    return result;
  };
  const pageBreak = e => e.getAttribute('role') === 'doc-pagebreak';
  // A text piece cut after a formula's punctuation (data-join-cont) is part of the span before it: its text joins that span's.
  // A nowrap wrapper (data-join) is no span either: its children are the list's.
  const spanList = elements => elements.flatMap(c => c.hasAttribute('data-join') ? [...c.children] : [c]).reduce((out, c) => {
    if (c.hasAttribute('data-join-cont') && out.length) out[out.length - 1].text += c.textContent; else out.push(span(c));
    return out;
  }, []);
  const blocks = [...document.querySelectorAll('main [data-block]')].map(e => ({id: e.id, type: e.dataset.block, content: spanList([...e.children].flatMap(c => c.classList.contains('runin-heading') ? [...c.children] : [c]))}));
  // A figure's long description (decision 110) follows the block that holds the figure; it is no
  // node of the document.
  // A bibliography run's wrapper (decision 111) is no node of the document: its children are the
  // parent's, in their order.
  const structures = [...document.querySelectorAll('main [data-structure]')].map(e => ({id: e.id, type: e.dataset.structure,
    children: [...e.children].flatMap(c => c.matches('div[role="doc-bibliography"]') ? [...c.children] : [c])
      .filter(c => c.dataset.kind !== 'long-description' && !pageBreak(c)).map(c => c.id)}));
  const math = [...document.querySelectorAll('main math')].map(e => ({id: e.id, tree: tree(e), rows: [...e.querySelectorAll('mtr[id]')].map(r => r.id)}));
  const ids = [...document.querySelectorAll('[id]')].filter(e => !pageBreak(e)).map(e => e.id);
  // The print pages' markers (plan accessibility A8): no node of the document, checked on their own.
  const pageBreaks = [...document.querySelectorAll('[role="doc-pagebreak"]')].map(e => {
    const box = e.getBoundingClientRect();
    return {id: e.id, label: e.getAttribute('aria-label'), empty: e.childNodes.length === 0, main: !!e.closest('main'),
      flow: getComputedStyle(e).position === 'absolute', size: Math.max(box.width, box.height)};
  });
  const links = [...document.querySelectorAll('a')];
  const brokenLinks = links.filter(e => !e.getAttribute('href').startsWith('#') || !document.getElementById(e.getAttribute('href').slice(1))).length;
  const fractions = [...document.querySelectorAll('mfrac')].map(e => {
    const [a, b] = [...e.children].map(c => c.getBoundingClientRect());
    return !!a && !!b && a.top < b.top && a.bottom <= b.top + 2;
  });
  return {blocks, structures, math, ids, pageBreaks, brokenLinks, fractions,
    // Hidden content is content that does not show. An element showing through a descendant
    // (a zero-width TeX box: <mpadded width="0">) is visible; a blank one (an empty paragraph,
    // a blank symbol) has nothing to show.
    invisible: [...document.querySelectorAll('main [data-block], main math, main mtr[id]')].filter(e =>
      !visible(e) && ![...e.querySelectorAll('*')].some(visible) &&
      !(e.textContent.trim() === '' && !e.querySelector('img, svg, mglyph'))).map(e => e.id),
    mathNamespaces: [...document.querySelectorAll('math, math *')].every(e => e.namespaceURI === 'http://www.w3.org/1998/Math/MathML'),
    nativeMath: CSS.supports('display', 'math'),
    unsafeNodes: document.querySelectorAll('script, iframe, object, embed, img, link, merror, annotation-xml').length,
    resources: performance.getEntriesByType('resource').map(e => e.name),
    scripts: [...document.scripts].map(s => s.getAttribute('src'))};
})()
