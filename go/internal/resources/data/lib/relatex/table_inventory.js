(() => {
  const attributes = e => [...e.attributes].map(a => a.name).sort();
  const visible = e => {
    const style = getComputedStyle(e), box = e.getBoundingClientRect();
    if (style.visibility !== 'visible' || style.display === 'none' || box.width <= 0 || box.height <= 0) return false;
    for (let node = e; node; node = node.parentElement) if (getComputedStyle(node).opacity !== '1') return false;
    return true;
  };
  const size = e => ({width: e.getBoundingClientRect().width, height: e.getBoundingClientRect().height});
  const baseline = (text, cell) => {
    const marker = document.createElement('span');
    marker.style.cssText = 'display:inline-block;width:0;height:0;font-size:0;line-height:0;margin:0;padding:0;border:0;vertical-align:baseline';
    text.append(marker);
    const result = marker.getBoundingClientRect().top - cell.getBoundingClientRect().top;
    marker.remove();
    return result;
  };
  // Each table is measured with its top scrolled to the viewport's: client rects are single floats,
  // so far down a long page (a book, 2^18 px and more) a box rounds to 1/32 px or coarser.
  const [scrollX, scrollY] = [window.scrollX, window.scrollY];
  const tables = [...document.querySelectorAll('main table')].map(table => (window.scrollBy(0, table.getBoundingClientRect().top), {
    id: table.id, kind: table.dataset.kind, text: table.textContent, parent_text: table.parentElement.textContent, attributes: attributes(table), name: table.getAttribute('aria-labelledby'), ...size(table), visible: visible(table),
    shape: [...table.children].map(e => ({tag: e.tagName, attributes: attributes(e)})),
    ...(table.dataset.schema === 'table-2' ? {schema: 'table-2',
      columns: [...table.querySelectorAll(':scope > colgroup > col')].map(col => ({width_sp: Number(col.dataset.widthSp), ...size(col)}))} : {}),
    rows: table.tBodies.length === 1 ? [...table.tBodies[0].rows].map((row, rowIndex) => ({
      attributes: attributes(row), shape: [...row.children].map(e => e.tagName),
      ...(row.dataset.row !== undefined ? {rich_row: {row:Number(row.dataset.row),page:Number(row.dataset.page),
        occurrence:Number(row.dataset.occurrence),role:row.dataset.role}} : {}),
      cells: [...row.cells].map((cell, column) => {
        const text = cell.querySelector(':scope > [data-cell-text]'), style = text && getComputedStyle(text);
        const cellBox = cell.getBoundingClientRect();
        return {tag: cell.tagName, ...(cell.hasAttribute('scope') ? {scope: cell.getAttribute('scope')} : {}),
          text_content: cell.textContent, attributes: attributes(cell), ...size(cell), visible: visible(cell), left: cellBox.left - table.getBoundingClientRect().left,
          top: cellBox.top - table.getBoundingClientRect().top,
          ...(cell.dataset.richCell ? {rich: {
            metadata: JSON.parse(cell.dataset.cellMeta), rowspan: cell.rowSpan, colspan: cell.colSpan,
            alignment: getComputedStyle(cell).textAlign, vertical_align: getComputedStyle(cell).verticalAlign,
            wrapping: getComputedStyle(cell).whiteSpace, background: getComputedStyle(cell).backgroundColor,
            // A cell's inner offset: its padding plus its content's margin.
            ...(() => { const inner = cell.querySelector(':scope > [data-cell-content]'), own = getComputedStyle(cell), content = inner ? getComputedStyle(inner) : null;
              return {padding_left: parseFloat(own.paddingLeft) + (content ? parseFloat(content.marginLeft) : 0),
                padding_right: parseFloat(own.paddingRight) + (content ? parseFloat(content.marginRight) : 0)}; })(),
            rules: [...cell.querySelectorAll(':scope > [data-rich-rule]')].map(rule => {
              const box = rule.getBoundingClientRect(), style = getComputedStyle(rule);
              return {metadata: JSON.parse(rule.dataset.richRule), hidden: rule.getAttribute('aria-hidden'),
                visible: visible(rule), position: style.position, background: style.backgroundColor,
                text: rule.textContent, children: rule.children.length,
                left: box.left - cellBox.left, top: box.top - cellBox.top, width: box.width, height: box.height};
            })}} : {}),
          shape: [...cell.children].map(e => e.hasAttribute('data-cell-text') ? 'text' : e.dataset.rule),
          text: text && {attributes: attributes(text), children: text.children.length, visible: visible(text),
            ...size(text), font_size: parseFloat(style.fontSize), line_height: parseFloat(style.lineHeight), baseline: baseline(text, cell), alignment: style.textAlign, color: style.color,
            run_colors: [...text.children].map(run => getComputedStyle(run).color)},
          rules: [...cell.querySelectorAll(':scope > [data-rule]')].map(rule => {
            const box = rule.getBoundingClientRect(), style = getComputedStyle(rule);
            return {kind: rule.dataset.rule, row: rowIndex, column, attributes: attributes(rule),
              children: rule.children.length, text: rule.textContent, hidden: rule.getAttribute('aria-hidden'),
              visible: visible(rule), left: box.left - cellBox.left, top: box.top - cellBox.top,
              ...size(rule), background: style.backgroundColor, color: style.color, position: style.position};
          })};
      })
    })) : []
  }));
  window.scrollTo(scrollX, scrollY);
  return tables;
})()
