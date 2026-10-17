'use strict';

// Drawing the View: tabs, panes at Go's rectangles, the pane area, the
// status bar, the compact view, many tabs, and item statuses.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, settle } = require('./helpers/harness.js');
const { defaultWorld, manyTabs } = require('./helpers/fixture.js');

test('renders the default three tabs from a fixture', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);

  const tabs = await page.$$eval('.tab', (els) => els.map((el) => ({
    text: el.querySelector('span').textContent,
    keys: el.querySelector('kbd') ? el.querySelector('kbd').textContent : '',
    pressed: el.getAttribute('aria-pressed'),
  })));
  assert.deepEqual(tabs, [
    { text: 'Comms', keys: '⌃⌥1', pressed: 'false' },
    { text: 'Code', keys: '⌃⌥2', pressed: 'true' },
    { text: 'Research', keys: '⌃⌥3', pressed: 'false' },
  ]);

  const view = fake.view();
  const panes = await page.$$eval('.pane', (els) => els.map((el) => ({
    id: el.dataset.paneId,
    focused: el.dataset.focused,
    subtabs: Array.from(el.querySelectorAll('.subtab'), (s) => [s.querySelector('.mono-tile').textContent, s.querySelector('.subtab-title').textContent, s.getAttribute('aria-pressed')]),
    badge: el.querySelector('.badge') ? el.querySelector('.badge').textContent : '',
  })));
  assert.deepEqual(panes, [
    { id: 'p3', focused: 'true', subtabs: [['Zd', 'bench — switch.go', 'true'], ['VS', 'index.html — bench-ui', 'false']], badge: 'App' },
    { id: 'p4', focused: 'false', subtabs: [['>_', 'zsh — ~/code/bench', 'true'], ['>_', 'go test — ~/code/bench', 'false']], badge: 'Built-in' },
  ]);

  // Panes sit at Go's rectangles, relative to the pane area.
  const rects = await page.$$eval('.pane', (els) => {
    const win = document.getElementById('app-window').getBoundingClientRect();
    return els.map((el) => {
      const r = el.getBoundingClientRect();
      return { x: Math.round(r.left - win.left), y: Math.round(r.top - win.top), w: Math.round(r.width), h: Math.round(r.height) };
    });
  });
  assert.deepEqual(rects, view.display.panes.map((p) => p.rect));
  assert.equal(view.display.panes[0].rect.x, 8, 'the first pane starts one gap in');

  const status = await page.textContent('.statusbar');
  assert.match(status, /Code/);
  assert.match(status, /4 windows in 2 panes/);
  assert.match(status, /Wide layout, chosen for a 2,560-pt screen/);
  assert.match(status, /⌃⌥1–9 tabs/);

  // In the app there are no drawn traffic lights or demo bar, and the title
  // bar moves the window.
  assert.equal(await page.$('.traffic'), null);
  assert.equal(await page.isVisible('#demo-bar'), false);
  const drag = await page.$eval('.titlebar', (el) => getComputedStyle(el).getPropertyValue('--wails-draggable').trim());
  assert.equal(drag, 'drag');
  const noDrag = await page.$eval('.tab', (el) => getComputedStyle(el).getPropertyValue('--wails-draggable').trim());
  assert.equal(noDrag, 'no-drag');
  const pad = await page.$eval('.titlebar', (el) => getComputedStyle(el).paddingLeft);
  assert.equal(pad, '78px');
  assert.deepEqual(fake.errors, []);
});

test('SetPaneArea is called with the measured pane area', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  const measured = await page.$eval('#panes', (el) => {
    const win = document.getElementById('app-window').getBoundingClientRect();
    const r = el.getBoundingClientRect();
    return { x: Math.round(r.left - win.left), y: Math.round(r.top - win.top), w: Math.round(r.width), h: Math.round(r.height) };
  });
  const call = await fake.waitForCall('SetPaneArea');
  assert.deepEqual(call.args[0], measured);
  // Between the title bar and the status bar, with no padding.
  const title = await page.$eval('.titlebar', (el) => el.getBoundingClientRect().bottom);
  const status = await page.$eval('.statusbar', (el) => el.getBoundingClientRect().top);
  assert.equal(measured.y, Math.round(title));
  assert.equal(measured.h, Math.round(status - title));

  // A smaller window reports the new area.
  fake.clearCalls();
  await page.setViewportSize({ width: 1200, height: 700 });
  const next = await fake.waitForCall('SetPaneArea');
  assert.equal(next.args[0].w, 1200);
  assert.equal(next.args[0].h, measured.h - 200);
});

test('in the browser preview, draws the traffic lights and the screen picker', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ demo: true }) });
  t.after(close);
  assert.equal(await page.isVisible('.traffic'), true);
  assert.equal(await page.isVisible('#demo-bar'), true);
  const screens = await page.$$eval('#screen-picker button', (els) => els.map((el) => [el.textContent, el.getAttribute('aria-pressed')]));
  assert.deepEqual(screens, [['Fit to browser', 'true'], ['Monitor · 2,560 pt', 'false'], ['Laptop · 1,512 pt', 'false']]);
  // In demo mode, the area is inside the stage's window, not the page.
  const area = (await fake.waitForCall('SetPaneArea')).args[0];
  assert.equal(area.x, 0);
  assert.equal(area.w, 1440 - 32 - 2);
  // Go can't measure the browser, so the preview tells it the width once
  // it's drawn "Fit to browser".
  assert.deepEqual(fake.callsTo('SetDemoScreen').map((c) => c.args), [['fit', 1440]]);

  await page.click('[data-screen="monitor"]');
  const call = await fake.waitForCall('SetDemoScreen', (a) => a[0] === 'monitor');
  assert.deepEqual(call.args, ['monitor', 1440]);
  await page.waitForSelector('#app-window[data-screen="monitor"]');
  const box = await page.$eval('#app-window', (el) => [el.offsetWidth, el.offsetHeight]);
  assert.deepEqual(box, [1440, 900]);
  // A wide screen gives Code side by side.
  await page.waitForFunction(() => document.querySelectorAll('.pane').length === 2
    && document.querySelector('.pane[data-pane-id="p4"]').getBoundingClientRect().top
      === document.querySelector('.pane[data-pane-id="p3"]').getBoundingClientRect().top);
});

test('the compact view shows one pane and disables splits', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ current: 2, mode: 'compact' }) });
  t.after(close);
  const panes = await page.$$eval('.pane', (els) => els.map((el) => el.dataset.paneId));
  assert.deepEqual(panes, ['compact']);
  const titles = await page.$$eval('.subtab-title', (els) => els.map((el) => el.textContent));
  assert.deepEqual(titles, ['Wails v3 docs', 'Pull request #42 · bench', 'Go packages', 'Daily note']);
  assert.match(await page.textContent('.statusbar'), /Compact layout \(one pane, every window as a sub-tab\), set by hand/);

  // The split button explains instead of splitting.
  const split = page.locator('[data-action="split-right"]');
  assert.equal(await split.getAttribute('aria-disabled'), 'true');
  await split.click({ force: true });
  await page.waitForFunction(() => document.getElementById('toast').classList.contains('show'));
  assert.equal(await page.textContent('#toast'), 'Splits are hidden on small screens.');
  assert.equal(fake.callsTo('MoveItem').length, 0);

  // The menu disables split entries with the reason, and moving to a tab
  // still works.
  await page.click('[data-action="menu"]');
  const disabled = await page.$$eval('.menu-item[disabled]', (els) => els.map((el) => el.textContent));
  assert.deepEqual(disabled, ['Show side by side (split right)⌃⌥⇧→', 'Split down⌃⌥⇧↓']);
  assert.equal(await page.textContent('.menu-note'), 'Splits are hidden on small screens.');
  assert.equal(await page.$('.menu-heading:text("Move to pane")'), null);
  await page.click('.menu-item:has-text("Comms")');
  const move = await fake.waitForCall('MoveItemToTab');
  assert.deepEqual(move.args, ['w7', 0]);

  // Clicking a sub-tab shows it in the compact pane.
  await page.click('.subtab:has-text("Daily note")');
  const show = await fake.waitForCall('ShowItem');
  assert.deepEqual(show.args, ['compact', 'w10']);
  // Clicks in the compact pane don't focus a pane.
  assert.equal(fake.callsTo('FocusPane').length, 0);
});

test('twenty tabs scroll, and only tabs with a shortcut show one', async (t) => {
  const world = manyTabs(defaultWorld({ current: 0 }), 20);
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  const keys = await page.$$eval('.tab', (els) => els.map((el) => (el.querySelector('kbd') ? el.querySelector('kbd').textContent : '')));
  assert.equal(keys.length, 20);
  assert.deepEqual(keys.slice(0, 9), ['⌃⌥1', '⌃⌥2', '⌃⌥3', '⌃⌥4', '⌃⌥5', '⌃⌥6', '⌃⌥7', '⌃⌥8', '⌃⌥9']);
  assert.deepEqual(keys.slice(9), Array(11).fill(''));
  const scroll = await page.$eval('.tabs', (el) => ({ client: el.clientWidth, scroll: el.scrollWidth, overflow: getComputedStyle(el).overflowX }));
  assert.ok(scroll.scroll > scroll.client, 'the tab strip overflows');
  assert.equal(scroll.overflow, 'auto');

  // Switching to the last tab scrolls it into view.
  fake.world.current = 19;
  await fake.emitState();
  await page.waitForSelector('.tab[data-index="19"][aria-pressed="true"]');
  await settle(page);
  const visible = await page.$eval('.tab[data-index="19"]', (el) => {
    const nav = el.parentElement.getBoundingClientRect();
    const r = el.getBoundingClientRect();
    return r.left >= nav.left - 1 && r.right <= nav.right + 1;
  });
  assert.ok(visible, 'the current tab is scrolled into view');

  // A new tab after the ninth has no shortcut.
  await page.click('[data-action="new-tab"]');
  assert.equal(await page.textContent('#shortcut-value'), 'None (use ⌃⌥[ ] or Jump to…)');
});

test('shows status text for items that aren\'t ready, and Dock again', async (t) => {
  const world = defaultWorld({ current: 2 });
  world.items.w7.status = 'undocked';
  world.items.w7.statusText = 'You moved this window out of Bench.';
  world.items.w10.status = 'launching';
  world.items.w10.statusText = 'Opening Obsidian…';
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  assert.equal(await page.textContent('.pane[data-pane-id="p5"] .status-text'), 'You moved this window out of Bench.');
  assert.equal(await page.textContent('.pane[data-pane-id="p6"] .status-text'), 'Opening Obsidian…');
  assert.equal(await page.$('.pane[data-pane-id="p6"] [data-action="redock"]'), null);
  await page.click('.pane[data-pane-id="p5"] [data-action="redock"]');
  const call = await fake.waitForCall('Redock');
  assert.deepEqual(call.args, ['w7']);

  // The menu offers Dock again too.
  await page.click('.pane[data-pane-id="p5"] [data-action="menu"]');
  await page.click('.menu-item:has-text("Dock again")');
  assert.equal(fake.callsTo('Redock').length, 2);
});

test('outside the preview, App and Web bodies are left empty for the native window', async (t) => {
  const { page, close } = await openBench({ world: defaultWorld({ current: 0 }) });
  t.after(close);
  const bodies = await page.$$eval('.pane-body', (els) => els.map((el) => [el.dataset.native, el.children.length]));
  assert.deepEqual(bodies, [['w1', 0], ['w2', 0]]);
});

test('an empty pane offers to add a window or app', async (t) => {
  const world = defaultWorld({ current: 0 });
  world.tabs[0].root.children[1].items = [];
  world.tabs[0].root.children[1].active = '';
  delete world.items.w2;
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  const empty = page.locator('.pane[data-pane-id="p2"] .empty');
  assert.match(await empty.textContent(), /No windows in this pane yet\./);
  await empty.locator('button:has-text("Add a window or app")').click();
  const call = await fake.waitForCall('AddOptions');
  assert.deepEqual(call.args, ['p2']);
});

test('the status bar shows notices, and the accessibility banner asks for permission', async (t) => {
  const notices = ['Couldn\'t register ⌃⌥Space. Try ⌃⌥J.', 'config.toml: tab "Code": unknown preset "mainside".', 'Screen Recording is off.'];
  const { page, fake, close } = await openBench({ world: defaultWorld({ notices, accessibility: false, configWritable: false }) });
  t.after(close);
  const notice = page.locator('.status-notice');
  assert.equal(await notice.locator('.notice-text').textContent(), notices[0]);
  assert.equal(await notice.locator('.notice-more').textContent(), '2 more');
  await notice.click();
  const listed = await page.$$eval('#notices-menu .menu-text', (els) => els.map((el) => el.textContent));
  assert.deepEqual(listed, notices);
  await fake.waitForCall('SetOverlay', (a) => a[0] === true);
  await page.keyboard.press('Escape');
  await page.waitForSelector('#notices-menu', { state: 'detached' });
  await fake.waitForLowered();

  assert.equal(await page.isVisible('#banner'), true);
  assert.match(await page.textContent('#banner'), /Accessibility permission/);
  await page.click('#banner button');
  await fake.waitForCall('RequestAccessibility');

  // The editor says changes aren't saved when config.toml has problems.
  await page.click('[data-action="edit-tab"]');
  assert.match(await page.textContent('.editor-note'), /aren't saved to it/);
});

test('the first-run guide explains permissions and Continue dismisses it', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ firstRun: true, accessibility: false }) });
  t.after(close);
  await page.waitForSelector('.guide-sheet');
  await fake.waitForCall('SetOverlay', (a) => a[0] === true);
  const text = await page.textContent('.guide-sheet');
  assert.match(text, /Accessibility permission to arrange other apps' windows/);
  assert.match(text, /Screen Recording is only for live previews/);
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'guide-allow');
  await page.click('.guide-sheet [data-action="request-accessibility"]');
  await fake.waitForCall('RequestAccessibility');
  await page.click('.guide-sheet button:has-text("Continue")');
  await fake.waitForCall('DismissFirstRun');
  await page.waitForSelector('.guide-sheet', { state: 'detached' });
  await fake.waitForLowered();

  // The first-run command opens it again.
  await fake.emit('bench:command', { name: 'first-run' });
  await page.waitForSelector('.guide-sheet');
});

test('a state that arrives while Ready is on its way isn\'t drawn over by Ready\'s answer', async (t) => {
  const { page, close } = await openBench({
    setup: (fake) => fake.on('Ready', async (_args, f) => {
      const old = f.view();
      f.world.items.w3.title = 'bench — newer.go';
      await f.emitState();
      await new Promise((r) => setTimeout(r, 200));
      return old;
    }),
  });
  t.after(close);
  await settle(page, 350);
  assert.equal(await page.textContent('.subtab[data-item="w3"] .subtab-title'), 'bench — newer.go');
  // The panes are still drawn in the measured area.
  const h = await page.$eval('.pane[data-pane-id="p3"]', (el) => el.getBoundingClientRect().height);
  assert.ok(h > 100, 'pane height ' + h);
});

test('in the app, a toast sits where docked windows can\'t cover it', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await fake.emit('bench:toast', 'Rearranged Code');
  await page.waitForFunction(() => document.getElementById('toast').classList.contains('show'));
  const toast = await page.locator('#toast').boundingBox();
  const bodies = await page.$$eval('.pane-body', (els) => els.map((el) => el.getBoundingClientRect().bottom));
  for (const bottom of bodies) assert.ok(toast.y >= bottom, 'toast top ' + toast.y + ' is below a pane body ending at ' + bottom);
  assert.equal(await page.$eval('#toast', (el) => getComputedStyle(el).whiteSpace), 'nowrap');
});

test('a View with Go\'s nulls for empty lists draws without errors', async (t) => {
  const world = defaultWorld({ current: 0 });
  world.tabs[0].root.children[1].items = [];
  world.tabs[0].root.children[1].active = '';
  delete world.items.w2;
  world.floating = [];
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  // Go's encoding/json sends nil slices as null.
  const v = fake.view();
  v.floating = null;
  v.notices = null;
  v.tabs[0].apps = null;
  v.tabs[0].root.children[1].items = null;
  v.display.panes[1].items = null;
  await fake.emit('bench:state', v);
  await settle(page);
  assert.match(await page.textContent('.pane[data-pane-id="p2"] .empty'), /No windows in this pane yet/);
  await page.click('[data-action="open-palette"]');
  await page.waitForSelector('.palette');
  await page.keyboard.press('Escape');
  await page.click('.pane[data-pane-id="p1"] [data-action="menu"]');
  await page.waitForSelector('#window-menu');
  await page.keyboard.press('Escape');
  fake.on('AddOptions', () => null);
  await page.click('.pane[data-pane-id="p2"] [data-action="add"]');
  await page.waitForSelector('.palette-empty');
  assert.equal(await page.textContent('.palette-empty'), 'There\'s nothing to choose from yet.');
  await page.keyboard.press('Escape');
  await page.click('[data-action="edit-tab"]');
  await page.waitForSelector('.editor-sheet');
});
