'use strict';
(() => {
  const data = window.RelatexData;
  const words = new Map(data.words.map(word => [word.id, word]));
  const sourceFile = document.getElementById('source-file');
  const sourceText = document.getElementById('source-text');
  const status = document.getElementById('source-selection');
  const lineIndex = data.index || {};
  function selection(query) {
    const first = Number(query.line), last = Number(query.end_line ?? query.line);
    const column = Number(query.column ?? 1);
    const end = query.end_column != null ? Number(query.end_column)
      : query.column != null && query.end_line == null ? column + 1 : Infinity;
    if (!Number.isInteger(first) || first < 1 || !Number.isInteger(last) || last < first ||
        !Number.isInteger(column) || column < 1 ||
        (query.end_column != null && (!Number.isInteger(end) || end < 1)) || (last === first && end <= column)) {
      throw new Error('Invalid source selection');
    }
    return {first, last, column, end};
  }
  function forward(query) {
    const range = selection(query);
    const fileLines = lineIndex[query.file] || {};
    const ids = new Set();
    Object.entries(fileLines).forEach(([line, matches]) => {
      if (Number(line) >= range.first && Number(line) <= range.last) matches.forEach(id => ids.add(id));
    });
    return data.words.filter(word => ids.has(word.id) && word.origins.some(origin => {
      if (origin.file !== query.file || origin.line < range.first || origin.line > range.last) return false;
      if (!origin.characters) return true;
      const lower = origin.line === range.first ? range.column : 1;
      const upper = origin.line === range.last ? range.end : Infinity;
      return origin.characters[0] + 1 < upper && origin.characters[1] + 1 > lower;
    }));
  }
  function inverse(query) {
    return data.words.filter(word => {
      const fragments = word.fragments || [{page: word.page, boxes: word.boxes}];
      return fragments.some(fragment => fragment.page === Number(query.page) && fragment.boxes.some(box =>
        query.horizontal >= box[0] && query.horizontal <= box[2] && query.vertical >= box[1] && query.vertical <= box[3]));
    });
  }
  function highlight(matches) {
    const ids = new Set(matches.map(word => word.id));
    document.querySelectorAll('[data-word]').forEach(button => button.classList.toggle('selected', ids.has(button.dataset.word)));
    document.dispatchEvent(new CustomEvent('relatex-highlight', {detail: {words: [...ids]}}));
    return matches;
  }
  function showSource(query) {
    sourceFile.value = query.file;
    const source = (data.sources || {})[query.file];
    sourceText.replaceChildren();
    if (!source || source.text === undefined) {
      sourceText.textContent = 'Source text unavailable in this bundle.';
      return;
    }
    const range = selection(query);
    source.text.split('\n').forEach((line, index) => {
      const number = index + 1;
      if (number < range.first || number > range.last) {
        sourceText.append(document.createTextNode(line + '\n'));
        return;
      }
      const chars = Array.from(line);
      const start = number === range.first ? range.column - 1 : 0;
      const end = number === range.last ? range.end - 1 : chars.length;
      const mark = document.createElement('mark');
      mark.textContent = chars.slice(start, end).join('');
      sourceText.append(document.createTextNode(chars.slice(0, start).join('')), mark,
        document.createTextNode(chars.slice(end).join('') + '\n'));
    });
  }
  function selectSource(query) {
    const matches = highlight(forward(query));
    showSource(query);
    status.textContent = `${matches.length} matching word boxes. Line-only source hints remain candidates.`;
    return matches;
  }
  function selectWord(id) {
    const word = words.get(id);
    if (!word) throw new Error('Unknown word');
    highlight([word]);
    status.textContent = word.origins.length
      ? word.origins.map(origin => `${origin.file}:${origin.line} (${origin.precision})`).join('\n')
      : 'Source origin unavailable';
    if (word.origins.length) {
      const origin = word.origins[0];
      const query = {file: origin.file, line: origin.line};
      if (origin.characters) {
        query.column = origin.characters[0] + 1;
        query.end_column = origin.characters[1] + 1;
      }
      showSource(query);
    }
    document.dispatchEvent(new CustomEvent('relatex-source', {detail: {word: id, origins: word.origins}, bubbles: true}));
    return word;
  }
  Object.keys(data.sources || {}).forEach(file => {
    const option = document.createElement('option');
    option.value = option.textContent = file;
    sourceFile.append(option);
  });
  document.getElementById('source-form').addEventListener('submit', event => {
    event.preventDefault();
    const query = {file: sourceFile.value};
    ['line', 'column', 'end_line', 'end_column'].forEach(field => {
      const value = document.getElementById('source-' + field).value;
      if (value) query[field] = Number(value);
    });
    try { selectSource(query); } catch (error) { status.textContent = error.message; }
  });
  document.addEventListener('click', event => {
    const button = event.target.closest('[data-word]');
    if (button) selectWord(button.dataset.word);
  });
  window.RelatexSync = Object.freeze({forward, inverse, selectSource, selectWord});
})();
