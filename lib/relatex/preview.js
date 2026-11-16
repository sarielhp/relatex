// Reference previews for the reading page: hovering or focusing an in-page link (a reference,
// a citation or a footnote mark) shows a card with a copy of its target, the theorem statement,
// equation, caption or bibliography entry. Without this script the links work as plain links.
(() => {
  'use strict';
  // Theme: the system's (prefers-color-scheme) unless the reader chose one with the toggle; the
  // choice is kept in this browser.
  const root = document.documentElement;
  const themeKey = 'relatex-theme';
  const systemLight = window.matchMedia ? window.matchMedia('(prefers-color-scheme: light)') : null;
  const applyTheme = (theme) => {
    if (theme === 'light' || theme === 'dark') root.dataset.theme = theme;
    else delete root.dataset.theme;
  };
  const shownTheme = () => root.dataset.theme || (systemLight && systemLight.matches ? 'light' : 'dark');
  try { applyTheme(localStorage.getItem(themeKey)); } catch (error) { /* no storage: the system's theme */ }
  const toggle = document.createElement('button');
  toggle.type = 'button';
  toggle.className = 'theme-toggle';
  const labelTheme = () => { toggle.textContent = shownTheme() === 'light' ? 'Dark theme' : 'Light theme'; };
  if (systemLight && systemLight.addEventListener) systemLight.addEventListener('change', labelTheme);
  toggle.addEventListener('click', () => {
    const next = shownTheme() === 'light' ? 'dark' : 'light';
    applyTheme(next);
    try { localStorage.setItem(themeKey, next); } catch (error) { /* not kept */ }
    labelTheme();
  });
  labelTheme();
  document.body.append(toggle);

  const card = document.createElement('div');
  // A class, not an id: the page anchors are its document targets alone (the browser check).
  card.className = 'preview-card';
  card.setAttribute('role', 'tooltip');
  card.hidden = true;
  document.body.append(card);
  let timer = null;
  let shownFor = null;

  // The target of a link: a heading's preview also shows the text that follows it.
  // A chapter file of a split book (decision 081) keeps copies of its links' targets in other
  // files in a template, since a page read from disk cannot fetch another file.
  const copies = document.querySelector('template[data-split-previews]');
  const copyOf = (link) => {
    const href = link.getAttribute('href');
    const hash = href.indexOf('#');
    if (!copies || hash <= 0) return null;
    const id = decodeURIComponent(href.slice(hash + 1));
    const entry = [...copies.content.children].find((child) => child.getAttribute('data-preview-for') === id);
    if (!entry) return null;
    const fragment = document.createDocumentFragment();
    for (const child of entry.children) fragment.append(child.cloneNode(true));
    for (const back of fragment.querySelectorAll('[data-note-backref]')) back.remove();
    return fragment;
  };

  const contentOf = (link) => {
    if (!link.getAttribute('href').startsWith('#')) return copyOf(link);
    const target = document.getElementById(decodeURIComponent(link.getAttribute('href').slice(1)));
    if (!target) return null;
    const copy = target.cloneNode(true);
    for (const node of [copy, ...copy.querySelectorAll('[id]')]) node.removeAttribute('id');
    for (const back of copy.querySelectorAll('[data-note-backref]')) back.remove();
    if (/^H[1-6]$/.test(target.tagName)) {
      const fragment = document.createDocumentFragment();
      fragment.append(copy);
      let next = target.nextElementSibling;
      for (let count = 0; next && count < 2 && !/^H[1-6]$/.test(next.tagName); count += 1) {
        const following = next.cloneNode(true);
        for (const node of [following, ...following.querySelectorAll('[id]')]) node.removeAttribute('id');
        fragment.append(following);
        next = next.nextElementSibling;
      }
      return fragment;
    }
    return copy;
  };

  const place = (link) => {
    const box = link.getBoundingClientRect();
    const gutter = 8;
    card.style.left = '0px';
    card.style.top = '0px';
    const width = Math.min(card.offsetWidth, window.innerWidth - 2 * gutter);
    let left = box.left + window.scrollX;
    if (left + width > window.scrollX + window.innerWidth - gutter) left = window.scrollX + window.innerWidth - gutter - width;
    left = Math.max(left, window.scrollX + gutter);
    const below = box.bottom + window.scrollY + 6;
    const fits = box.bottom + 6 + card.offsetHeight <= window.innerHeight;
    const top = fits ? below : Math.max(box.top + window.scrollY - card.offsetHeight - 6, window.scrollY + gutter);
    card.style.left = `${left}px`;
    card.style.top = `${top}px`;
  };

  const show = (link) => {
    const content = contentOf(link);
    if (!content) return;
    card.replaceChildren(content);
    card.hidden = false;
    shownFor = link;
    place(link);
  };

  const hide = () => {
    card.hidden = true;
    shownFor = null;
  };

  const schedule = (action, delay) => {
    clearTimeout(timer);
    timer = setTimeout(action, delay);
  };

  const links = [...document.querySelectorAll('main a[href]:not([data-note-backref])')]
    .filter((link) => link.getAttribute('href').startsWith('#') || copyOf(link));
  for (const link of links) {
    link.addEventListener('mouseenter', () => schedule(() => show(link), 150));
    link.addEventListener('mouseleave', () => schedule(hide, 250));
    link.addEventListener('focus', () => show(link));
    link.addEventListener('blur', hide);
  }
  // Moving onto the card keeps it open, so a long statement can be scrolled.
  card.addEventListener('mouseenter', () => clearTimeout(timer));
  card.addEventListener('mouseleave', () => schedule(hide, 250));
  document.addEventListener('keydown', (event) => { if (event.key === 'Escape') hide(); });
  window.addEventListener('scroll', () => { if (shownFor) place(shownFor); }, { passive: true });

  // A display formula wider than the page scrolls sideways. Only after layout is it known which
  // ones do: each that overflows becomes a named, focusable region, so the keyboard can scroll it
  // (WCAG 2.1.1, axe scrollable-region-focusable); a display that fits gets no tab stop. The EPUB
  // package holds no script, so its displays stay as written (plan accessibility §8.1).
  const scrollable = new WeakSet();
  const markScrollable = () => {
    for (const display of document.querySelectorAll('main .display_math')) {
      // overflow-x: auto makes the other axis scroll too: a tall formula overflows downwards.
      const overflows = display.scrollWidth > display.clientWidth || display.scrollHeight > display.clientHeight;
      if (overflows === scrollable.has(display)) continue;
      if (!overflows) {
        scrollable.delete(display);
        for (const name of ['tabindex', 'role', 'aria-label']) display.removeAttribute(name);
        continue;
      }
      scrollable.add(display);
      const number = display.querySelector(':scope > [data-kind="text"]')?.textContent.trim();
      display.setAttribute('tabindex', '0');
      display.setAttribute('role', 'region');
      display.setAttribute('aria-label', number ? `Equation ${number}, scrollable` : 'Display formula, scrollable');
    }
  };
  markScrollable();
  window.addEventListener('load', markScrollable);
  if (document.fonts) document.fonts.ready.then(markScrollable);
  // A formula grows when its fonts arrive, a display shrinks with the window: each change of
  // either size marks the displays again, after layout.
  if (typeof ResizeObserver === 'function') {
    const observer = new ResizeObserver(markScrollable);
    for (const element of document.querySelectorAll('main .display_math, main .display_math math')) observer.observe(element);
  } else {
    window.addEventListener('resize', markScrollable);
  }
})();
