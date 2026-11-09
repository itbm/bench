'use strict';

// Tab colours (decision 74): a coloured tab's underline, the band along the
// top of its panes' headers, in dark and light, the swatches in Tab
// properties and New tab, which are a radio group that the keyboard and a
// screen reader use, the dots beside tabs' names in the overview and the
// palette, a rename that keeps the colour, and Bench's smallest window.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, openTabProperties, settle } = require('./helpers/harness.js');
const { defaultWorld, find } = require('./helpers/fixture.js');

// coloured returns the default world on Comms, with Comms blue and Code
// green, and Research with no colour.
function coloured(opts = {}) {
  const w = defaultWorld({ current: 0, ...opts });
  w.tabs[0].colour = 'blue';
  w.tabs[1].colour = 'green';
  return w;
}

// besideSwatches reports whether the name of the colour chosen is on the
// swatches' line, after them, as it is at the size Tab properties opens at.
const besideSwatches = (page) => page.evaluate(() => {
  const s = document.querySelector('.swatches').getBoundingClientRect();
  const p = document.getElementById('colour-text').getBoundingClientRect();
  return p.left > s.right && p.top < s.bottom && p.bottom > s.top;
});

const focusedKey = (page) => page.evaluate(() => document.activeElement && document.activeElement.dataset.key);

// look returns, for each tab, its colour attribute, its underline, as the
// colour of its box-shadow (the current tab's) or of its ::after (a
// coloured tab's short one), or '' for none; and for each pane, the colour
// of the band along its header's top, and its border's colour. Colours are
// #RRGGBB, and so are the tokens it returns: the accent and the tab
// colours, as the page has them now.
function look(page) {
  return page.evaluate(() => {
    const hex = (css) => {
      const m = /rgba?\((\d+), (\d+), (\d+)/.exec(css || '');
      return m ? '#' + m.slice(1, 4).map((n) => Number(n).toString(16).padStart(2, '0').toUpperCase()).join('') : '';
    };
    const root = getComputedStyle(document.documentElement);
    const token = (n) => root.getPropertyValue(n).trim().toUpperCase();
    const tokens = { accent: token('--accent') };
    for (const c of ['red', 'yellow', 'green', 'cyan', 'blue', 'purple', 'pink', 'grey']) tokens[c] = token('--tab-' + c);
    const tabs = Array.from(document.querySelectorAll('#titlebar .tab'), (el) => {
      const after = getComputedStyle(el, '::after');
      const shadow = getComputedStyle(el).boxShadow;
      return {
        name: el.querySelector('.tab-name').textContent,
        colour: el.dataset.colour || '',
        current: el.getAttribute('aria-selected') === 'true',
        underline: shadow !== 'none' ? hex(shadow) : '',
        short: after.content !== 'none' && after.display !== 'none' ? hex(after.backgroundColor) : '',
        shortWidth: after.content !== 'none' && after.display !== 'none' ? parseFloat(after.width) : 0,
        width: el.getBoundingClientRect().width,
      };
    });
    const panes = Array.from(document.querySelectorAll('.pane'), (el) => {
      const header = el.querySelector(':scope > .pane-header');
      const style = header ? getComputedStyle(header) : null;
      const image = style ? style.backgroundImage : 'none';
      return {
        focused: el.dataset.focused === 'true',
        band: image !== 'none' ? hex(image) : '',
        // Where the band is in the header, and how large: [left, top],
        // and [the room it leaves at the ends, its height].
        bandAt: style ? style.backgroundPosition : '',
        bandSize: style ? style.backgroundSize : '',
        bandRepeats: style ? style.backgroundRepeat !== 'no-repeat' : false,
        headerShadow: style ? style.boxShadow : 'none',
        border: hex(getComputedStyle(el).borderTopColor),
      };
    });
    return { tokens, tabs, panes };
  });
}

test('a coloured tab has an underline in its colour, and its panes\' headers a band of it, in dark and light', async (t) => {
  const { page, fake, close } = await openBench({ world: coloured(), contextOptions: { colorScheme: 'dark' } });
  t.after(close);
  const dark = {};
  for (const scheme of ['dark', 'light']) {
    await page.emulateMedia({ colorScheme: scheme });
    await settle(page);
    const { tokens, tabs, panes } = await look(page);
    if (scheme === 'dark') Object.assign(dark, tokens);
    else assert.notEqual(tokens.blue, dark.blue, 'light has its own blue');
    // Comms, current, keeps its raised background, with its full
    // underline in blue in place of the accent; Code has a short one in
    // green; Research has none.
    assert.deepEqual(tabs.map((x) => [x.name, x.colour, x.current, x.underline, x.short]), [
      ['Comms', 'blue', true, tokens.blue, ''],
      ['Code', 'green', false, '', tokens.green],
      ['Research', '', false, '', ''],
    ], scheme);
    assert.ok(tabs[1].shortWidth > 0 && tabs[1].shortWidth < tabs[1].width, scheme + ': the short underline is inside the tab');
    // Each of Comms' panes has a blue band along its header's top, 1 pt
    // below the pane's border and 12 pt in from each end, so that it never
    // touches the border, and the focused one keeps the accent's border.
    assert.equal(panes.length, 2);
    for (const p of panes) {
      assert.equal(p.band, tokens.blue, scheme);
      assert.deepEqual([p.bandAt, p.bandSize, p.bandRepeats, p.headerShadow], ['12px 1px', 'calc(100% - 24px) 2px', false, 'none'], scheme);
    }
    assert.equal(panes.find((p) => p.focused).border, tokens.accent, scheme);
    assert.notEqual(panes.find((p) => !p.focused).border, tokens.blue, scheme);
    // The name always shows, and a screen reader hears it as before.
    assert.equal(await page.getAttribute('.tab[data-index="0"]', 'aria-label'), null);
  }

  // Research has no colour: its tab has the accent's underline, and its
  // panes' headers have no band.
  await page.click('.tab[data-index="2"]');
  await page.waitForFunction(() => window.bench.store.view.current === 2);
  await settle(page);
  const { tokens, tabs, panes } = await look(page);
  assert.deepEqual(tabs.map((x) => [x.name, x.underline, x.short]), [
    ['Comms', '', tokens.blue],
    ['Code', '', tokens.green],
    ['Research', tokens.accent, ''],
  ]);
  assert.ok(panes.length > 0);
  assert.deepEqual(panes.map((p) => p.band), panes.map(() => ''));

  // A colour that comes in a View, as a reload of config.toml sends it,
  // shows at once.
  fake.world.tabs[2].colour = 'pink';
  fake.changed();
  await page.waitForSelector('.tab[data-index="2"][data-colour="pink"]');
  const after = await look(page);
  assert.equal(after.tabs[2].underline, after.tokens.pink);
  assert.deepEqual(after.panes.map((p) => p.band), after.panes.map(() => after.tokens.pink));
});

test('Tab properties\' colours are a radio group that the keyboard and a screen reader use, saved with the tab', async (t) => {
  const { page, fake, close } = await openBench({ world: coloured() });
  t.after(close);
  await openTabProperties(page, 0);

  // A radio group named Colour, with None and the eight colours, each
  // named; the tab's own is chosen, and its name shows beside them.
  const group = page.getByRole('radiogroup', { name: 'Colour' });
  assert.equal(await group.count(), 1);
  const names = await group.getByRole('radio').evaluateAll((els) => els.map((el) => el.getAttribute('aria-label')));
  assert.deepEqual(names, ['None', 'Red', 'Yellow', 'Green', 'Cyan', 'Blue', 'Purple', 'Pink', 'Grey']);
  assert.equal(await group.getByRole('radio', { name: 'Blue', checked: true }).count(), 1);
  assert.equal(await group.getByRole('radio', { checked: true }).count(), 1);
  assert.match(await page.textContent('#colour-text'), /^Blue: /);
  assert.ok(await besideSwatches(page), 'the colour\'s name is beside the swatches');
  assert.equal(await page.getAttribute('.swatches', 'aria-describedby'), 'colour-text');

  // The group is one stop for the Tab key, on the colour chosen.
  await page.focus('#tab-place');
  await page.keyboard.press('Tab');
  assert.equal(await focusedKey(page), 'colour-blue');
  await page.keyboard.press('Tab');
  assert.match(await focusedKey(page), /^preset-/);
  await page.keyboard.press('Shift+Tab');
  assert.equal(await focusedKey(page), 'colour-blue');

  // The arrow keys, Home, and End choose as they move, as macOS radio
  // buttons do, and the name beside them follows.
  await page.keyboard.press('ArrowRight');
  assert.equal(await focusedKey(page), 'colour-purple');
  assert.equal(await page.getAttribute('[data-key="colour-purple"]', 'aria-checked'), 'true');
  assert.match(await page.textContent('#colour-text'), /^Purple: /);
  await page.keyboard.press('ArrowLeft');
  await page.keyboard.press('ArrowLeft');
  assert.equal(await focusedKey(page), 'colour-cyan');
  await page.keyboard.press('Home');
  assert.equal(await focusedKey(page), 'colour-none');
  assert.match(await page.textContent('#colour-text'), /^None\. /);
  await page.keyboard.press('End');
  assert.equal(await focusedKey(page), 'colour-grey');
  assert.equal(await group.getByRole('radio', { checked: true }).getAttribute('aria-label'), 'Grey');

  // A change is one Cancel would lose, so Escape asks first.
  await page.keyboard.press('Escape');
  await page.waitForSelector('.discard-sheet');
  await page.click('[data-key="discard-keep"]');
  await page.waitForSelector('.discard-sheet', { state: 'detached' });

  // A click chooses too. Save sends the colour with the rest of the tab.
  await page.click('[data-key="colour-pink"]');
  assert.match(await page.textContent('#colour-text'), /^Pink: /);
  await page.click('[data-key="editor-save"]');
  assert.deepEqual((await fake.waitForCall('SaveTab')).args, [0, { name: 'Comms', preset: 'side', smallScreens: 'tabs', colour: 'pink' }]);
  await page.waitForSelector('.editor-sheet', { state: 'detached' });

  // None takes the colour away.
  fake.clearCalls();
  await openTabProperties(page, 1);
  assert.equal(await page.getAttribute('[data-key="colour-green"]', 'aria-checked'), 'true');
  await page.click('[data-key="colour-none"]');
  await page.click('[data-key="editor-save"]');
  assert.deepEqual((await fake.waitForCall('SaveTab')).args, [1, { name: 'Code', preset: 'main-side', smallScreens: 'stack', colour: '' }]);
});

test('New tab starts with no colour, and gives the new tab the one chosen, with an empty layout or windows', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('[data-action="new-tab"]');
  await page.waitForSelector('.editor-sheet');
  assert.equal(await page.getAttribute('[data-key="colour-none"]', 'aria-checked'), 'true');
  assert.match(await page.textContent('#colour-text'), /^None\. A colour shows under the tab/);
  assert.ok(await besideSwatches(page), 'the note is beside the swatches');
  await page.fill('#tab-name', 'Design');
  await page.click('[data-key="colour-yellow"]');
  await page.click('[data-key="editor-save"]');
  assert.deepEqual((await fake.waitForCall('SaveTab')).args, [-1, { name: 'Design', preset: 'single', smallScreens: 'stack', colour: 'yellow' }]);
  await page.waitForSelector('.editor-sheet', { state: 'detached' });

  // A tab of windows that are open.
  await page.click('[data-action="new-tab"]');
  await page.waitForSelector('.editor-sheet');
  await page.click('[data-key="start-windows"]');
  await page.waitForSelector('.win-list');
  await page.click('[data-key="colour-cyan"]');
  await page.click('[data-key="editor-save"]');
  assert.deepEqual((await fake.waitForCall('NewTabFromWindows')).args[0], { name: 'Brave', preset: 'side', smallScreens: 'stack', colour: 'cyan' });
  await page.waitForSelector('.tab[data-index="3"][data-colour="cyan"]');
  assert.equal(await page.textContent('.tab[data-index="3"] .tab-name'), 'Brave');
});

test('renaming a tab in place keeps its colour', async (t) => {
  const { page, fake, close } = await openBench({ world: coloured() });
  t.after(close);
  await page.dblclick('.tab[data-index="1"]');
  await page.waitForSelector('.rename-input');
  assert.equal(await page.getAttribute('.tab.renaming', 'data-colour'), 'green');
  await page.fill('.rename-input', 'Coding');
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('SaveTab')).args, [1, { name: 'Coding', preset: 'custom', smallScreens: 'stack', colour: 'green' }]);
});

test('a colour that the page doesn\'t know goes back to Go as it came, from a rename and from Tab properties', async (t) => {
  // As a colour that a later Bench adds would come to a page that hasn't
  // got it: the tab shows none, and saving the tab mustn't lose it.
  const w = coloured();
  w.tabs[1].colour = 'teal';
  const { page, fake, close } = await openBench({ world: w });
  t.after(close);
  assert.equal(await page.getAttribute('.tab[data-index="1"]', 'data-colour'), null);
  await page.dblclick('.tab[data-index="1"]');
  await page.waitForSelector('.rename-input');
  await page.fill('.rename-input', 'Coding');
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('SaveTab')).args, [1, { name: 'Coding', preset: 'custom', smallScreens: 'stack', colour: 'teal' }]);

  fake.clearCalls();
  await openTabProperties(page, 1);
  await page.fill('#tab-name', 'Code 2');
  await page.click('[data-key="editor-save"]');
  assert.deepEqual((await fake.waitForCall('SaveTab')).args, [1, { name: 'Code 2', preset: 'main-side', smallScreens: 'stack', colour: 'teal' }]);
});

// dots returns the dots in rows, each row's colour, or '-' for an empty
// slot, or null for a row with no dot.
function dots(page, rows) {
  return page.$$eval(rows, (els) => els.map((el) => {
    const dot = el.querySelector('.tab-dot');
    if (!dot) return null;
    if (dot.getAttribute('aria-hidden') !== 'true') return 'not hidden';
    return dot.dataset.colour || (getComputedStyle(dot).visibility === 'hidden' ? '-' : 'shown');
  }));
}

test('the overview\'s cards and the palette\'s tab rows have a dot of each tab\'s colour beside its name', async (t) => {
  const { page, fake, close } = await openBench({ world: coloured() });
  t.after(close);
  await fake.emit('bench:command', { name: 'show-all-tabs' });
  await page.waitForSelector('.overview-card');
  assert.deepEqual(await dots(page, '.overview-card'), ['blue', 'green', '-']);
  // The dot comes before the name, which a screen reader hears as before.
  assert.deepEqual(await page.$$eval('.overview-card-head', (els) => els.map((el) => el.firstElementChild.className)),
    ['tab-dot', 'tab-dot', 'tab-dot']);
  assert.match(await page.getAttribute('#overview-card-0', 'aria-label'), /^Comms ⌃⌥1, current tab/);
  // The dot is as far from each name, so the names line up.
  const lefts = await page.$$eval('.overview-name', (els) => els.map((el) => el.getBoundingClientRect().left - el.closest('.overview-card').getBoundingClientRect().left));
  assert.ok(lefts.every((x) => Math.abs(x - lefts[0]) < 0.5), String(lefts));
  await page.keyboard.press('Escape');
  await page.waitForSelector('.overview', { state: 'detached' });

  await page.click('[data-action="open-palette"]');
  await page.waitForSelector('.palette-item');
  const rows = await page.$$eval('.palette-item', (els) => els.map((el) => el.querySelector('.palette-label').textContent));
  const all = await dots(page, '.palette-item');
  const byName = Object.fromEntries(rows.map((r, i) => [r, all[i]]));
  assert.equal(byName.Comms, 'blue');
  assert.equal(byName.Code, 'green');
  assert.equal(byName.Research, '-');
  // Windows' rows have none.
  const windows = await dots(page, '.palette-item[data-item]');
  assert.ok(windows.length > 0);
  assert.deepEqual([...new Set(windows)], [null]);
});

test('with no tab coloured, lists have no dots, as before', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  assert.equal(await page.locator('#titlebar .tab[data-colour]').count(), 0);
  assert.equal(await page.locator('.pane[data-colour]').count(), 0);
  await fake.emit('bench:command', { name: 'show-all-tabs' });
  await page.waitForSelector('.overview-card');
  assert.equal(await page.locator('.tab-dot').count(), 0);
  await page.keyboard.press('Escape');
  await page.click('[data-action="open-palette"]');
  await page.waitForSelector('.palette-item');
  assert.equal(await page.locator('.tab-dot').count(), 0);
});

test('in Bench\'s smallest window, the current tab shows its colour, and Tab properties\' colours fit, in view', async (t) => {
  const { page, close } = await openBench({ world: coloured({ screenWidth: 1512, current: 0 }), viewport: { width: 640, height: 480 } });
  t.after(close);
  await settle(page);
  const { tokens, tabs } = await look(page);
  assert.equal(tabs[0].underline, tokens.blue);
  // The "tabs" view's one pane has its header in the title bar, under the
  // tab's underline, so it has no band.
  assert.equal(await page.locator('.pane[data-header="title-bar"]').count(), 1);

  await openTabProperties(page, 0);
  await settle(page);
  const fit = await page.evaluate(() => {
    const body = document.querySelector('.editor-sheet .dialog-body').getBoundingClientRect();
    const sheet = document.querySelector('.editor-sheet').getBoundingClientRect();
    const swatches = Array.from(document.querySelectorAll('.swatch'), (el) => el.getBoundingClientRect());
    const text = document.getElementById('colour-text').getBoundingClientRect();
    return {
      inView: swatches.every((r) => r.top >= body.top && r.bottom <= body.bottom && r.left >= sheet.left && r.right <= sheet.right),
      oneRow: swatches.every((r) => Math.abs(r.top - swatches[0].top) < 1),
      text: text.bottom <= body.bottom && text.right <= sheet.right,
      size: [swatches[0].width, swatches[0].height],
      scrolls: document.querySelector('.editor-sheet .dialog-body').scrollWidth > document.querySelector('.editor-sheet .dialog-body').clientWidth,
    };
  });
  assert.deepEqual(fit, { inView: true, oneRow: true, text: true, size: [28, 28], scrolls: false });
  await page.focus('[data-key="colour-blue"]');
  await page.keyboard.press('ArrowLeft');
  assert.equal(await focusedKey(page), 'colour-cyan');
});

// The tab colours with the items before them: the tab that a switch is on
// its way to (decision 67), the warning sign of a window that keeps moving
// (decision 69), Move to Tab and Also Show In in the HTML and native menus
// (decision 71), the drop-down (decision 72), and files from the Finder over
// a pane's header (decision 73).

test('a tab that a switch is on its way to has its full underline in its colour at once, and the tab before it its short one', async (t) => {
  const { page, fake, close } = await openBench({ world: coloured() });
  t.after(close);
  // Go takes a while to switch, as with apps that take a while to arrange.
  let release;
  const held = new Promise((r) => { release = r; });
  fake.on('SwitchTab', async (args, f) => {
    await held;
    return f.mSwitchTab(...args);
  });
  await settle(page);
  await page.click('.tab[data-index="1"] .tab-name');
  await page.waitForSelector('.tab[data-index="1"][aria-selected="true"]');
  assert.equal(await page.evaluate(() => window.bench.store.view.current), 0, 'Go hasn\'t switched yet');
  let { tokens, tabs, panes } = await look(page);
  assert.deepEqual(tabs.map((x) => [x.name, x.current, x.underline, x.short]), [
    ['Comms', false, '', tokens.blue],
    ['Code', true, tokens.green, ''],
    ['Research', false, '', ''],
  ]);
  // The panes are still Comms' until Go has switched, with its band.
  assert.deepEqual(panes.map((p) => p.band), panes.map(() => tokens.blue));

  release();
  await page.waitForFunction(() => window.bench.store.view.current === 1);
  await settle(page);
  ({ tokens, tabs, panes } = await look(page));
  assert.deepEqual(tabs.map((x) => [x.name, x.current, x.underline, x.short]), [
    ['Comms', false, '', tokens.blue],
    ['Code', true, tokens.green, ''],
    ['Research', false, '', ''],
  ]);
  assert.ok(panes.length > 0);
  assert.deepEqual(panes.map((p) => p.band), panes.map(() => tokens.green));
});

test('a yellow tab\'s band stays clear of the warning sign of a window that keeps moving, in each density', async (t) => {
  for (const density of ['standard', 'compact']) {
    const world = defaultWorld({ current: 2 });
    world.tabs[2].colour = 'yellow';
    world.settings.density = density;
    world.items.w7.status = 'moving';
    world.items.w7.statusText = 'Brave or Rectangle keeps moving this window';
    const { page, close } = await openBench({ world });
    t.after(close);
    await page.waitForSelector('.subtab[data-item="w7"] .status-mark[data-marker="problem"]');
    await settle(page);
    const { tokens, panes } = await look(page);
    assert.deepEqual([...new Set(panes.map((p) => p.band))], [tokens.yellow], density);
    // The band is the header's top 1 pt to 3 pt; the sign is below it.
    const gap = await page.evaluate(() => {
      const header = document.querySelector('.pane[data-pane-id="p5"] > .pane-header').getBoundingClientRect();
      const mark = document.querySelector('.subtab[data-item="w7"] .status-mark').getBoundingClientRect();
      return mark.top - (header.top + 3);
    });
    assert.ok(gap >= 1, density + ': the warning sign is ' + gap + ' pt below the band');
  }
});

test('Move to Tab and Also Show In have the dot of each tab\'s colour before its name, in the HTML menu and the native one', async (t) => {
  const { page, close } = await openBench({ world: coloured() });
  t.after(close);
  await page.click('[data-action="menu"][data-pane="p1"]');
  await page.waitForSelector('#window-menu');
  for (const name of ['Move to Tab', 'Also Show In']) {
    await page.click('#window-menu .menu-item.has-sub:has-text("' + name + '")');
    await page.waitForSelector('#menu-sub');
    // Comms, Code, Research, and New Tab, which isn't a tab.
    assert.deepEqual(await dots(page, '#menu-sub .menu-item'), ['blue', 'green', '-', null], name);
    // The names of the tabs line up.
    const lefts = await page.$$eval('#menu-sub .menu-item .menu-label', (els) => els.slice(0, 3).map((el) => el.getBoundingClientRect().left));
    assert.ok(lefts.every((x) => Math.abs(x - lefts[0]) < 0.5), name + ': ' + lefts);
  }

  // In the app, the native menu's entries have the same dots, which Go
  // shows as the items' images: "none" for Research, which has no colour.
  const app = await openBench({ world: coloured({ nativeMenus: true }) });
  t.after(app.close);
  await app.page.click('[data-action="menu"][data-pane="p1"]');
  await app.fake.waitForCall('ShowMenu');
  const { entries } = app.fake.callsTo('ShowMenu')[0].args[0];
  for (const id of ['move-to-tab', 'also-show-in']) {
    assert.deepEqual(entries.find((e) => e.id === id).submenu.map((e) => [e.label || '-', e.dot || '']), [
      ['Comms', 'blue'], ['Code', 'green'], ['Research', 'none'], ['-', ''], ['New Tab', ''],
    ], id);
  }
  // Other entries have none.
  const flat = (list) => list.flatMap((e) => [e, ...flat(e.submenu || [])]);
  assert.deepEqual(flat(entries).filter((e) => e.dot).map((e) => e.id).sort(),
    ['also-show-in:0', 'also-show-in:1', 'also-show-in:2', 'move-to-tab:0', 'move-to-tab:1', 'move-to-tab:2']);
});

test('the drop-down terminal is in no tab, so it has no colour, and the palette\'s row for it no dot', async (t) => {
  const world = coloured({ current: 1 });
  const p4 = find(world.tabs[1].root, 'p4');
  p4.items = p4.items.filter((id) => id !== 'w5');
  p4.active = 'w6';
  world.floating.push({ ...world.items.w5, tab: -1 });
  delete world.items.w5;
  world.dropdown = { id: 'w5', shown: true, inPage: true };
  const { page, close } = await openBench({ world, contextOptions: { reducedMotion: 'reduce' } });
  t.after(close);
  await page.waitForSelector('#dropdown:not([hidden]) .xterm');
  assert.equal(await page.locator('#dropdown[data-colour], #dropdown [data-colour], #dropdown .tab-dot').count(), 0);
  // Code's panes, under it, keep their green band.
  const { tokens, panes } = await look(page);
  assert.deepEqual([...new Set(panes.map((p) => p.band))], [tokens.green]);
  // The drop-down covers Jump to… in the preview, as in dropdown.test.js.
  await page.$eval('[data-action="open-palette"]', (el) => el.click());
  await page.waitForSelector('.palette-item');
  const row = page.locator('.palette-item:has-text("Hide Drop-down")');
  assert.equal(await row.count(), 1);
  assert.equal(await row.locator('.tab-dot').count(), 0);
});

test('a coloured pane\'s header that files from the Finder are over keeps its band, below the frame, with Increase contrast too', async (t) => {
  for (const [contrast, frame] of [['no-preference', 2], ['more', 3]]) {
    const { page, close } = await openBench({ world: coloured(), contextOptions: { contrast, forcedColors: 'none' } });
    t.after(close);
    await settle(page);
    const header = '.pane[data-pane-id="p1"] > .pane-header';
    const box = await page.locator(header).boundingBox();
    await page.evaluate(([x, y]) => { window._wails.handleDragEnter(); window._wails.handleDragOver(x, y); }, [box.x + box.width / 2, box.y + box.height / 2]);
    await page.waitForSelector(header + '.file-drop-target-active');
    const { tokens, panes } = await look(page);
    const lit = panes[(await page.$$eval('.pane', (els) => els.findIndex((el) => el.dataset.paneId === 'p1')))];
    // The frame is the accent's, and the band, in Comms' blue, starts a
    // point below it.
    assert.match(lit.headerShadow, new RegExp('^rgb\\(.*\\) 0px 0px 0px ' + frame + 'px inset$'), contrast);
    assert.equal(lit.band, tokens.blue, contrast);
    assert.deepEqual([lit.bandAt, lit.bandSize], ['12px ' + (frame + 1) + 'px', 'calc(100% - 24px) 2px'], contrast);
    // The other pane's band is where it was.
    assert.ok(panes.some((p) => p !== lit && p.bandAt === '12px 1px'), contrast);
    // Once they leave, the band is back where it was.
    await page.evaluate(() => window._wails.handleDragLeave());
    await page.waitForSelector(header + ':not(.file-drop-target-active)');
    const after = await look(page);
    assert.deepEqual(after.panes.map((p) => [p.band, p.bandAt, p.headerShadow]), after.panes.map(() => [after.tokens.blue, '12px 1px', 'none']), contrast);
  }
});

test('a colour chosen alone is a change that Tab properties asks about before Escape discards it', async (t) => {
  const { page, fake, close } = await openBench({ world: coloured() });
  t.after(close);
  await openTabProperties(page, 0);
  await page.click('[data-key="colour-pink"]');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.discard-sheet');
  assert.equal(await page.textContent('#discard-text'), 'Your changes to the Comms tab aren\'t saved.');
  await page.click('[data-key="discard-keep"]');
  // Choosing the colour it had again is no change: Escape closes it at once.
  await page.click('[data-key="colour-blue"]');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.editor-sheet', { state: 'detached' });
  assert.equal(fake.callsTo('SaveTab').length, 0);
});
