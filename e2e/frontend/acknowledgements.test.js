'use strict';

// The Acknowledgements window's page, acknowledgements.html, at the
// window's size and at the smallest size that its page can be
// (internal/ui/shell.go). build/acknowledgements writes the page, so most
// checks read the entries from it rather than naming each one.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openPage } = require('./helpers/harness.js');

const PAGE = '/acknowledgements.html';
const SIZE = { width: 640, height: 720 };
// Wails sizes the window's web view with Width and Height, but its frame,
// title bar included, with MinWidth and MinHeight: the smallest page is the
// minimum less macOS's 28-point title bar.
const MINIMUM = { width: 420, height: 320 - 28 };

// entries reads the page's entries in order, each with its parts. Each one
// is { id, name, labelled, about, notes, texts }, and an entry also has
// parts; labelled is whether its section is labelled by its heading.
function entries(page) {
  return page.$$eval('main > section.ack-entry', (els) => {
    const read = (el) => {
      const heading = el.querySelector(':scope > h2, :scope > h3');
      const all = (sel) => Array.from(el.querySelectorAll(':scope > ' + sel), (p) => p.textContent);
      return {
        id: el.id,
        name: heading ? heading.textContent : '',
        labelled: Boolean(heading && heading.id && el.getAttribute('aria-labelledby') === heading.id),
        about: all('p.ack-about'),
        notes: all('p.ack-note'),
        texts: all('pre.ack-text'),
      };
    };
    return els.map((el) => ({ ...read(el), parts: Array.from(el.querySelectorAll(':scope > section.ack-part'), read) }));
  });
}

// find returns the entry or part in list with the given name.
function find(list, name) {
  const found = list.find((e) => e.name === name);
  assert.ok(found, 'no ' + name + ' in ' + list.map((e) => e.name).join(', '));
  return found;
}

test('the page has its title, one h1, and a link in the contents to each entry, in order', async (t) => {
  const { page, close } = await openPage(PAGE, { viewport: SIZE });
  t.after(close);
  assert.equal(await page.title(), 'Acknowledgements');
  assert.equal(await page.getAttribute('html', 'lang'), 'en-GB');
  assert.deepEqual(await page.$$eval('h1', (els) => els.map((el) => el.textContent)), ['Acknowledgements']);

  // Each entry has an h2, and the contents lists them in the same order.
  const list = await entries(page);
  const names = list.map((e) => e.name);
  assert.ok(names.length >= 6, 'entries: ' + names.join(', '));
  assert.deepEqual(await page.$$eval('h2', (els) => els.map((el) => el.textContent)), names);
  const links = await page.$$eval('.ack-contents a', (els) => els.map((a) => {
    const target = document.getElementById(a.getAttribute('href').slice(1));
    return {
      text: a.textContent,
      href: a.getAttribute('href'),
      target: target && target.matches('main > section.ack-entry') ? target.querySelector(':scope > h2').textContent : null,
    };
  }));
  assert.deepEqual(links, list.map((e) => ({ text: e.name, href: '#' + e.id, target: e.name })));

  // The entries are in order of name, ignoring case, and no two elements
  // share an id.
  const lower = (s) => s.toLowerCase();
  assert.deepEqual(names, [...names].sort((a, b) => (lower(a) < lower(b) ? -1 : lower(a) > lower(b) ? 1 : 0)));
  const ids = await page.$$eval('[id]', (els) => els.map((el) => el.id));
  assert.deepEqual(ids.filter((id, i) => ids.indexOf(id) !== i), []);
});

test('every entry and part has an about line and at least one licence text', async (t) => {
  const { page, close } = await openPage(PAGE, { viewport: SIZE });
  t.after(close);
  const list = await entries(page);
  const all = list.flatMap((e) => [e, ...e.parts]);
  assert.ok(list.some((e) => e.parts.length > 0), 'the page has parts');
  const problems = [];
  for (const e of all) {
    const name = e.name || '#' + e.id;
    if (!e.name.trim()) problems.push(name + ': no heading');
    if (!e.labelled) problems.push(name + ': not labelled by its heading');
    if (e.about.length !== 1 || !e.about[0].trim()) problems.push(name + ': about lines ' + JSON.stringify(e.about));
    if (e.texts.length === 0) problems.push(name + ': no licence text');
    if (e.texts.some((s) => !s.trim())) problems.push(name + ': an empty licence text');
  }
  assert.deepEqual(problems, []);

  // Nothing sits outside an entry or a part: every section and licence text
  // was read above.
  const counts = await page.evaluate(() => ({
    sections: document.querySelectorAll('section').length,
    texts: document.querySelectorAll('pre').length,
  }));
  assert.deepEqual(counts, { sections: all.length, texts: all.reduce((n, e) => n + e.texts.length, 0) });
});

test('the page includes Go, Wails and its parts, golang.org/x/net with the Public Suffix List, xterm.js with Visual Studio Code, and the IBM Plex fonts', async (t) => {
  const { page, close } = await openPage(PAGE, { viewport: SIZE });
  t.after(close);
  const list = await entries(page);

  const go = find(list, 'Go');
  assert.match(go.about[0], /BSD 3-Clause License/);
  assert.match(go.texts[0], /^Copyright \d{4} The Go Authors\./);

  const wails = find(list, 'Wails');
  assert.match(wails.about[0], /^github\.com\/wailsapp\/wails\/v3 v3\.\S+ · MIT License/);
  for (const name of ['Chromium', 'htmx', 'is-callable', 'nanoid', 'ringqueue']) find(wails.parts, name);

  // The Public Suffix List says which revision Bench includes, and has the
  // whole of the Mozilla Public License.
  const psl = find(find(list, 'golang.org/x/net').parts, 'Public Suffix List');
  assert.match(psl.about[0], /Mozilla Public License 2\.0/);
  assert.equal(psl.notes.length, 1);
  assert.match(psl.notes[0], /at revision [0-9a-f]{40}\./);
  assert.ok(psl.texts.some((s) => s.startsWith('Mozilla Public License Version 2.0') && s.includes('Exhibit B - "Incompatible With Secondary Licenses" Notice')), 'the MPL text');

  const vscode = find(find(list, 'xterm.js').parts, 'Visual Studio Code');
  assert.match(vscode.texts[0], /Copyright \(c\) 2015 - present Microsoft Corporation/);

  // There's an entry for each font the page loads.
  const families = await page.evaluate(() => [...new Set(Array.from(document.fonts, (f) => f.family.replace(/^["']|["']$/g, '')))].sort());
  assert.deepEqual(families, ['IBM Plex Mono', 'IBM Plex Sans']);
  for (const family of families) {
    const font = find(list, family);
    assert.match(font.about[0], /SIL Open Font License 1\.1/);
    assert.ok(font.texts.some((s) => s.includes('SIL OPEN FONT LICENSE Version 1.1')), family + ': the OFL text');
  }
});

for (const viewport of [SIZE, MINIMUM]) {
  const size = viewport.width + ' by ' + viewport.height;

  test('at ' + size + ', clicking a contents link brings its entry into view', async (t) => {
    const { page, close } = await openPage(PAGE, { viewport });
    t.after(close);
    const hrefs = await page.$$eval('.ack-contents a', (els) => els.map((a) => a.getAttribute('href')));
    for (const href of hrefs) {
      await page.evaluate(() => window.scrollTo(0, 0));
      await page.click('.ack-contents a[href="' + href + '"]');
      await page.waitForFunction((h) => location.hash === h, href, { timeout: 2000 });
      const view = await page.evaluate((h) => {
        const entry = document.getElementById(h.slice(1));
        return {
          scrolled: window.scrollY > 0,
          top: entry.getBoundingClientRect().top,
          headingBottom: entry.querySelector(':scope > h2').getBoundingClientRect().bottom,
          height: window.innerHeight,
        };
      }, href);
      assert.ok(view.scrolled && view.top >= 0 && view.headingBottom <= view.height, href + ': ' + JSON.stringify(view));
    }
  });

  test('at ' + size + ', the page scrolls down but not sideways', async (t) => {
    const { page, close } = await openPage(PAGE, { viewport });
    t.after(close);
    const fit = await page.evaluate(() => {
      const doc = document.scrollingElement;
      const name = (el) => el.tagName.toLowerCase() + (el.id ? '#' + el.id : '') + (el.className ? '.' + el.className : '');
      return {
        wider: doc.scrollWidth > doc.clientWidth,
        taller: doc.scrollHeight > doc.clientHeight,
        // Nothing reaches past the sides, and no licence text scrolls on its own.
        outside: Array.from(document.querySelectorAll('body *'), (el) => [el, el.getBoundingClientRect()])
          .filter(([, r]) => r.width > 0 && (r.left < -0.5 || r.right > doc.clientWidth + 0.5)).map(([el]) => name(el)),
        scrolling: Array.from(document.querySelectorAll('pre'), (el) => [el, el.scrollWidth > el.clientWidth])
          .filter(([, wide]) => wide).map(([el]) => name(el.closest('section'))),
      };
    });
    assert.deepEqual(fit, { wider: false, taller: true, outside: [], scrolling: [] });

    // The wheel scrolls the document down, and there's nothing to scroll
    // sideways.
    await page.mouse.move(viewport.width / 2, viewport.height / 2);
    await page.mouse.wheel(0, 400);
    await page.waitForFunction(() => window.scrollY > 0, null, { timeout: 2000 });
    await page.evaluate(() => window.scrollBy(400, 0));
    assert.equal(await page.evaluate(() => window.scrollX), 0);
  });
}

test('the page has no scripts, links only within itself, and loads only from its own origin', async (t) => {
  const { page, url, close } = await openPage(PAGE, { viewport: SIZE });
  t.after(close);
  assert.equal(await page.$$eval('script', (els) => els.length), 0);
  const handlers = await page.$$eval('*', (els) => els.flatMap((el) => Array.from(el.attributes, (a) => a.name).filter((n) => n.startsWith('on'))));
  assert.deepEqual(handlers, []);
  const hrefs = await page.$$eval('[href]', (els) => els.filter((el) => el.tagName !== 'LINK').map((el) => el.getAttribute('href')));
  assert.ok(hrefs.length > 0);
  assert.deepEqual(hrefs.filter((h) => !h.startsWith('#')), []);

  // It loads its stylesheets and fonts from its own origin. close fails if
  // any request went elsewhere.
  const origin = new URL(url).origin;
  const loaded = await page.evaluate(() => [location.href, ...performance.getEntriesByType('resource').map((e) => e.name)]);
  assert.deepEqual(loaded.filter((u) => new URL(u).origin !== origin), []);
  const paths = loaded.map((u) => new URL(u).pathname);
  for (const p of [PAGE, '/vendor/fonts/fonts.css', '/styles.css']) assert.ok(paths.includes(p), p + ' in ' + paths.join(', '));
  assert.ok(paths.some((p) => /^\/vendor\/fonts\/ibm-plex-sans-.+\.woff2$/.test(p)), 'IBM Plex Sans in ' + paths.join(', '));
  assert.ok(paths.some((p) => /^\/vendor\/fonts\/ibm-plex-mono-.+\.woff2$/.test(p)), 'IBM Plex Mono in ' + paths.join(', '));
});

test('the page is in IBM Plex Sans and its licence texts in IBM Plex Mono, and the fonts load', async (t) => {
  const { page, close } = await openPage(PAGE, { viewport: SIZE });
  t.after(close);
  // Each kind of text: its first font, and whether that font's face for
  // its weight has loaded.
  const fonts = await page.evaluate(() => Object.fromEntries(['body', 'h1', 'h2', 'h3', '.ack-about', 'pre.ack-text'].map((sel) => {
    const style = getComputedStyle(document.querySelector(sel));
    const family = style.fontFamily.split(',')[0].trim().replace(/^["']|["']$/g, '');
    const face = Array.from(document.fonts).find((f) => f.family.replace(/^["']|["']$/g, '') === family && f.weight === style.fontWeight);
    const loaded = document.fonts.check(style.fontWeight + ' ' + style.fontSize + ' "' + family + '"') && Boolean(face) && face.status === 'loaded';
    return [sel, family + (loaded ? '' : ' (not loaded)')];
  })));
  assert.deepEqual(fonts, {
    body: 'IBM Plex Sans',
    h1: 'IBM Plex Sans',
    h2: 'IBM Plex Sans',
    h3: 'IBM Plex Sans',
    '.ack-about': 'IBM Plex Sans',
    'pre.ack-text': 'IBM Plex Mono',
  });
});

test('licence texts can be selected', async (t) => {
  const { page, close } = await openPage(PAGE, { viewport: SIZE });
  t.after(close);
  // No text on the page is kept from being selected.
  const unselectable = await page.$$eval('body, body *', (els) => els.filter((el) => getComputedStyle(el).userSelect === 'none')
    .map((el) => el.tagName.toLowerCase() + (el.className ? '.' + el.className : '')));
  assert.deepEqual(unselectable, []);

  // A Range over each licence text selects all of it.
  const differ = await page.$$eval('pre.ack-text', (els) => els.filter((el) => {
    const range = document.createRange();
    range.selectNodeContents(el);
    const selection = window.getSelection();
    selection.removeAllRanges();
    selection.addRange(range);
    return selection.toString() !== el.textContent;
  }).length);
  assert.equal(differ, 0);

  // Dragging across the first line of a licence text selects that line.
  await page.evaluate(() => window.getSelection().removeAllRanges());
  const pre = page.locator('pre.ack-text').first();
  await pre.scrollIntoViewIfNeeded();
  const box = await pre.boundingBox();
  await page.mouse.move(box.x + 14, box.y + 20);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width - 14, box.y + 20, { steps: 5 });
  await page.mouse.up();
  const line = (await pre.textContent()).split('\n')[0];
  assert.ok(line.length > 10, 'the first line: ' + JSON.stringify(line));
  assert.equal(await page.evaluate(() => window.getSelection().toString()), line);
});
