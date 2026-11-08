'use strict';

// Drawing the View: tabs, panes at Go's rectangles, the pane area, the
// status bar, the compact view, many tabs, and item statuses.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, openSettings, openTabProperties, settle } = require('./helpers/harness.js');
const { defaultWorld, find, manyTabs, pane } = require('./helpers/fixture.js');

test('renders the default three tabs from a fixture', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);

  const tabs = await page.$$eval('.tab', (els) => els.map((el) => ({
    text: el.querySelector('span').textContent,
    keys: el.querySelector('kbd') ? el.querySelector('kbd').textContent : '',
    pressed: el.getAttribute('aria-selected'),
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
    subtabs: Array.from(el.querySelectorAll('.subtab'), (s) => [s.querySelector('.mono-tile').textContent, s.querySelector('.subtab-title').textContent, s.getAttribute('aria-selected')]),
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
  assert.equal(view.display.panes[0].rect.x, 4, 'the first pane starts one gap in');

  // The title bar shows the current tab, so the status bar doesn't repeat it.
  assert.equal(await page.$('#statusbar .status-tab'), null);
  assert.equal(await page.textContent('#statusbar .status-count'), '4 windows in 2 panes');
  // The layout is named in short, with why in its tooltip and the layout
  // control's.
  assert.equal(await page.textContent('#statusbar .status-layout'), 'Auto: wide');
  assert.equal(await page.getAttribute('#statusbar .status-layout', 'title'), 'Wide layout, chosen for a window 2,560 pt wide');
  assert.equal(await page.getAttribute('#titlebar .seg', 'title'), 'Wide layout, chosen for a window 2,560 pt wide');
  assert.match(await page.textContent('#statusbar .status-keys'), /⌃⌥1–9 tabs/);
  // Clicking the layout says why too, for those who don't hover.
  await page.click('#statusbar .status-layout');
  await page.waitForFunction(() => document.getElementById('toast').classList.contains('show'));
  assert.equal(await page.textContent('#toast'), 'Wide layout, chosen for a window 2,560 pt wide.');
  // The status bar has no decorative dot, and nothing needs attention.
  assert.equal(await page.$('.status-dot'), null);
  assert.equal(await page.$('.status-attention'), null);

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
  const screens = await page.$$eval('#screen-picker button', (els) => els.map((el) => [el.textContent, el.getAttribute('aria-checked')]));
  assert.deepEqual(screens, [['Fit to browser', 'true'], ['Monitor · 2,560 pt', 'false'], ['Laptop · 1,512 pt', 'false']]);
  // In demo mode, the area is inside the stage's window, not the page.
  const area = (await fake.waitForCall('SetPaneArea')).args[0];
  assert.equal(area.x, 0);
  assert.equal(area.w, 1440 - 32 - 2);
  // Go can't measure the browser, so the preview tells it the width once
  // it's drawn "Fit to browser".
  assert.deepEqual(fake.callsTo('SetDemoScreen').map((c) => c.args), [['fit', 1440]]);

  // The demo bar's Simulate shows what the preview can pretend happens.
  // Its Minimise window minimises the focused pane's window, as Command-M
  // does in the app, and hides the list again.
  assert.equal(await page.isVisible('#demo-bar [data-action="demo-minimise"]'), false);
  await page.click('#demo-simulate');
  assert.equal(await page.getAttribute('#demo-simulate', 'aria-expanded'), 'true');
  assert.equal(await page.isVisible('#demo-bar [data-action="demo-minimise"]'), true);
  assert.deepEqual(await page.locator('#demo-simulate-menu button').allTextContents(),
    ['Minimise window', 'Bring Bench forward', 'VoiceOver', 'Rectangle', 'Keep moving window']);
  await page.click('#demo-bar [data-action="demo-minimise"]');
  assert.deepEqual((await fake.waitForCall('DemoMinimise')).args, []);
  assert.equal(await page.isVisible('#demo-simulate-menu'), false);
  assert.equal(await page.getAttribute('#demo-simulate', 'aria-expanded'), 'false');
  // Its Bring Bench forward does what a click on Bench does in the app,
  // while another app is in front: Bench hides the windows that cover it.
  await page.click('#demo-simulate');
  assert.equal(await page.textContent('#demo-bar [data-action="demo-come-forward"]'), 'Bring Bench forward');
  await page.click('#demo-bar [data-action="demo-come-forward"]');
  assert.deepEqual((await fake.waitForCall('DemoComeForward')).args, []);
  // From the keyboard, Simulate's list takes the focus, Escape hides it
  // and gives the focus back, and a click elsewhere hides it too.
  await page.focus('#demo-simulate');
  await page.keyboard.press('Enter');
  await page.waitForFunction(() => document.activeElement.dataset.action === 'demo-minimise');
  await page.keyboard.press('Escape');
  assert.equal(await page.isVisible('#demo-simulate-menu'), false);
  assert.equal(await page.evaluate(() => document.activeElement.id), 'demo-simulate');
  await page.click('#demo-simulate');
  await page.click('#demo-bar strong');
  assert.equal(await page.isVisible('#demo-simulate-menu'), false);
  // The bar stays one line at this width, as Bench's window is below it:
  // the hint ends in "…" where it doesn't fit, and its title has all of it.
  const bar = await page.$eval('#demo-bar', (el) => ({
    tops: [...el.children].filter((c) => !c.hidden).map((c) => Math.round(c.getBoundingClientRect().top + c.getBoundingClientRect().height / 2)),
    hint: [el.querySelector('#demo-hint').textContent, el.querySelector('#demo-hint').title],
  }));
  assert.equal(new Set(bar.tops).size, 1, 'the demo bar is one line: ' + bar.tops);
  assert.equal(bar.hint[1], bar.hint[0]);
  assert.match(bar.hint[0], /^Drag a sub-tab onto another pane/);
  // At this width, the hint has room for all of it, ⌃K too.
  assert.equal(await page.$eval('#demo-hint', (el) => el.scrollWidth <= el.clientWidth), true, 'the hint is cut short');

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

test('the compact view shows one pane, and Show Side by Side changes to the wide layout', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ current: 2, mode: 'compact' }) });
  t.after(close);
  const panes = await page.$$eval('.pane', (els) => els.map((el) => el.dataset.paneId));
  assert.deepEqual(panes, ['compact']);
  const titles = await page.$$eval('.subtab-title', (els) => els.map((el) => el.textContent));
  assert.deepEqual(titles, ['Wails v3 docs', 'Pull request #42 · bench', 'Go packages', 'Daily note']);
  // The status bar counts the panes as drawn: the tab's two are shown as
  // one.
  assert.equal(await page.textContent('#statusbar .status-count'), '4 windows, shown as one pane');
  assert.equal(await page.textContent('#statusbar .status-layout'), 'Compact');
  assert.equal(await page.getAttribute('#statusbar .status-layout', 'title'), 'Compact layout (one pane, every window as a sub-tab), set by hand');

  // The split button isn't off: its name and its tooltip, which has the
  // move keys' →, say that it uses the wide layout.
  const split = page.locator('[data-action="split-right"]');
  assert.equal(await split.getAttribute('aria-disabled'), null);
  assert.equal(await split.getAttribute('aria-label'), 'Show this window side by side, in the wide layout');
  assert.equal(await split.getAttribute('title'), 'Show this window side by side, in the wide layout (⌃⌥⇧→)');

  // The menu has one entry in place of the four splits, first after the
  // common actions, with nothing off but Move Left, for the first sub-tab,
  // and moving to a tab still works.
  await page.click('[data-action="menu"]');
  const labels = await page.$$eval('#window-menu .menu-item', (els) => els.map((el) => el.textContent));
  const side = labels.indexOf('Show Side by Side (Wide Layout)⌃⌥⇧→');
  assert.ok(side > 0 && labels[side - 1] === 'Close Window', labels.join(', '));
  assert.ok(!labels.some((l) => /^Split /.test(l)), labels.join(', '));
  const disabled = await page.$$eval('.menu-item[disabled]', (els) => els.map((el) => el.textContent));
  assert.deepEqual(disabled, ['Move Left']);
  assert.equal(await page.$('#window-menu .menu-note'), null);
  assert.equal(await page.$('.menu-heading:text("Move to Pane")'), null);
  await page.click('#window-menu [data-action="menu-sub"]:has-text("Move to Tab")');
  await page.click('#menu-sub .menu-item:has-text("Comms")');
  const move = await fake.waitForCall('MoveItemToTab');
  assert.deepEqual(move.args, ['w7', 0]);

  // Clicking a sub-tab shows it in the compact pane.
  await page.click('.subtab:has-text("Daily note")');
  const show = await fake.waitForCall('ShowItem');
  assert.deepEqual(show.args, ['compact', 'w10']);
  // Clicks in the compact pane don't focus a pane.
  assert.equal(fake.callsTo('FocusPane').length, 0);

  // Show side by side changes to the wide layout, which shows the tab's
  // panes, and says how to go back, with Undo, which goes back.
  await page.click('[data-action="menu"]');
  await page.click('#window-menu .menu-item:has-text("Show Side by Side (Wide Layout)")');
  assert.deepEqual((await fake.waitForCall('ShowSideBySide')).args, ['w10']);
  await page.waitForFunction(() => document.querySelectorAll('.pane').length === 2);
  assert.equal(await page.textContent('#statusbar .status-layout'), 'Wide');
  await page.waitForSelector('#toast.show .toast-action');
  assert.equal(await page.textContent('#toast .toast-text'), 'Changed to the wide layout, to show Obsidian — Daily note side by side. To go back, choose Compact at the right of the title bar.');
  await page.click('#toast .toast-action');
  await page.waitForFunction(() => document.querySelectorAll('.pane').length === 1);
  assert.equal(fake.world.mode, 'compact');

  // The split button does the same.
  await split.click();
  await page.waitForFunction(() => document.querySelectorAll('.pane').length === 2);
  assert.deepEqual(fake.callsTo('ShowSideBySide').map((c) => c.args), [['w10'], ['w10']]);
  assert.equal(fake.callsTo('MoveItem').length, 0);
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
  await page.waitForSelector('.tab[data-index="19"][aria-selected="true"]');
  await settle(page);
  const visible = await page.$eval('.tab[data-index="19"]', (el) => {
    const nav = el.parentElement.getBoundingClientRect();
    const r = el.getBoundingClientRect();
    return r.left >= nav.left - 1 && r.right <= nav.right + 1;
  });
  assert.ok(visible, 'the current tab is scrolled into view');

  // A new tab after the ninth has no shortcut.
  await page.click('[data-action="new-tab"]');
  assert.equal(await page.$eval('#tab-place', (el) => el.selectedOptions[0].textContent), '21st');
  assert.equal(await page.textContent('#tab-place-note'), 'Only the first nine tabs have shortcuts. Use ⌃⌥[ ] or Jump to… for this one.');
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
  await page.click('.menu-item:has-text("Dock Again")');
  assert.equal(fake.callsTo('Redock').length, 2);
});

test('a minimised window\'s pane says so, with Show again and Remove from this tab', async (t) => {
  const world = defaultWorld({ current: 2 });
  world.items.w10.status = 'minimised';
  world.items.w10.statusText = 'Minimised';
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  const body = page.locator('.pane[data-pane-id="p6"] .pane-status');
  assert.equal(await body.getAttribute('data-status'), 'minimised');
  assert.equal(await body.locator('.status-text').textContent(), 'Minimised');
  assert.deepEqual(await body.locator('button').allTextContents(), ['Show again', 'Remove from this tab']);
  // Its sub-tab has the marker for information, and the status bar doesn't
  // count it among the windows that need attention.
  assert.equal(await page.getAttribute('.subtab[data-item="w10"]', 'data-marker'), 'info');
  assert.equal(await page.getAttribute('.subtab[data-item="w10"]', 'aria-label'), 'Daily note, Obsidian (minimised)');
  assert.equal(await page.getAttribute('.subtab[data-item="w10"]', 'title'), 'Obsidian — Daily note\nMinimised.');
  assert.equal(await page.$('#statusbar [data-action="attention"]'), null);

  // Show again shows it, as clicking its sub-tab does.
  await body.locator('button:has-text("Show again")').click();
  assert.deepEqual((await fake.waitForCall('ShowItem')).args, ['p6', 'w10']);
  // Remove from this tab: a window in this tab alone goes out of Bench.
  await body.locator('button:has-text("Remove from this tab")').click();
  assert.deepEqual((await fake.waitForCall('PopOut')).args, ['w10']);
  assert.equal(fake.callsTo('RemoveItemFromTab').length, 0);

  // The menu says why, and offers Show again too.
  await page.click('.pane[data-pane-id="p6"] [data-action="menu"]');
  await page.waitForSelector('#window-menu');
  assert.equal(await page.textContent('#window-menu .menu-status'), 'Minimised.');
  await page.click('.menu-item:has-text("Show again")');
  await page.waitForFunction(() => !document.getElementById('window-menu'));
  assert.equal(fake.callsTo('ShowItem').length, 2);
});

test('Remove from this tab leaves a minimised window that\'s in other tabs there', async (t) => {
  const world = defaultWorld({ current: 2 });
  world.items.w10.status = 'minimised';
  world.items.w10.statusText = 'Minimised';
  // Code shows it too.
  find(world.tabs[1].root, 'p4').items.push('w10');
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await page.click('.pane[data-pane-id="p6"] .pane-status button:has-text("Remove from this tab")');
  assert.deepEqual((await fake.waitForCall('RemoveItemFromTab')).args, ['w10', 2]);
  assert.equal(fake.callsTo('PopOut').length, 0);
});

test('a sub-tab with no window offers Remove from Bench', async (t) => {
  const world = defaultWorld({ current: 2 });
  world.items.w10.status = 'missing';
  world.items.w10.statusText = 'Obsidian isn\'t installed';
  world.items.w10.noWindow = true;
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  assert.equal(await page.textContent('.pane[data-pane-id="p6"] .status-text'), 'Obsidian isn\'t installed');
  assert.equal(await page.$('.pane[data-pane-id="p5"] [data-action="remove-item"]'), null);
  await page.click('.pane[data-pane-id="p6"] [data-action="remove-item"]');
  const call = await fake.waitForCall('PopOut');
  assert.deepEqual(call.args, ['w10']);

  // The menu says Remove from Bench in place of Pop Out of Bench.
  await page.click('.pane[data-pane-id="p6"] [data-action="menu"]');
  const labels = await page.$$eval('#window-menu .menu-item', (els) => els.map((el) => el.textContent));
  assert.ok(labels.includes('Remove from Bench'), labels.join(', '));
  assert.ok(!labels.includes('Pop Out of Bench'), labels.join(', '));
  await page.click('.menu-item:has-text("Remove from Bench")');
  assert.equal(fake.callsTo('PopOut').length, 2);
});

test('without Accessibility, a sub-tab with no window offers to open its app', async (t) => {
  const world = defaultWorld({ current: 2, accessibility: false });
  world.items.w10.status = 'missing';
  world.items.w10.statusText = 'Obsidian isn\'t running';
  world.items.w10.noWindow = true;
  world.items.w10.canOpen = true;
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  const body = page.locator('.pane[data-pane-id="p6"] .pane-status');
  assert.deepEqual(await body.locator('button').allTextContents(), ['Open Obsidian', 'Remove from Bench']);
  await body.locator('button:has-text("Open Obsidian")').click();
  assert.deepEqual((await fake.waitForCall('OpenItem')).args, ['w10']);
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

test('a pane whose windows have closed lists the apps it is waiting for', async (t) => {
  const world = defaultWorld({ current: 2 });
  world.items.w10.hidden = true;
  world.remembered = {
    p6: [
      { itemId: 'w10', bundleId: 'md.obsidian', app: 'Obsidian', mono: 'Ob', text: 'Obsidian isn\'t running', canOpen: true },
      { itemId: 'w11', bundleId: 'notion.id', app: 'Notion', mono: 'No', text: 'Notion isn\'t installed', canOpen: false },
    ],
  };
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  const empty = page.locator('.pane[data-pane-id="p6"] .empty');
  const texts = await empty.locator('.status-text').allTextContents();
  assert.deepEqual(texts, ['Obsidian isn\'t running', 'Notion isn\'t installed']);
  assert.doesNotMatch(await empty.textContent(), /No windows in this pane yet/);
  // The hidden sub-tab isn't drawn, and the tab doesn't count it.
  assert.equal(await page.$('.pane[data-pane-id="p6"] .subtab'), null);
  assert.match(await page.textContent('.statusbar'), /3 windows in 2 panes/);

  // An app that isn't installed has nothing to open.
  assert.deepEqual(await empty.locator('[data-action="open-item"]').allTextContents(), ['Open Obsidian']);
  await empty.locator('button:has-text("Open Obsidian")').click();
  assert.deepEqual((await fake.waitForCall('OpenItem')).args, ['w10']);
  await empty.locator('.remembered[aria-label="Notion"] [data-action="remove-remembered"]').click();
  assert.deepEqual((await fake.waitForCall('RemoveRemembered')).args, ['p6', 'notion.id']);
  await empty.locator('button:has-text("Close this pane")').click();
  assert.deepEqual((await fake.waitForCall('ClosePane')).args, ['p6']);
  // These buttons focus what they show themselves, or close the pane.
  assert.equal(fake.callsTo('FocusPane').length, 0);

  // The ⋯ menu of the other pane doesn't offer them: they're this pane's.
  await page.click('.pane[data-pane-id="p5"] [data-action="menu"]');
  const labels = await page.$$eval('#window-menu .menu-item', (els) => els.map((el) => el.textContent));
  assert.ok(!labels.includes('Open Obsidian'), labels.join(', '));
});

test('a pane that shows other windows offers its closed ones in its menu', async (t) => {
  const world = defaultWorld({ current: 2 });
  world.items.w9.hidden = true;
  world.remembered = {
    p5: [{ itemId: 'w9', bundleId: 'com.brave.Browser', app: 'Brave', mono: 'Br', text: 'Brave has no window open for this', canOpen: true }],
  };
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  const subtabs = await page.$$eval('.pane[data-pane-id="p5"] .subtab-title', (els) => els.map((el) => el.textContent));
  assert.deepEqual(subtabs, ['Wails v3 docs', 'Pull request #42 · bench']);
  await page.click('.pane[data-pane-id="p5"] [data-action="menu"]');
  const labels = await page.$$eval('#window-menu .menu-item', (els) => els.map((el) => el.textContent));
  assert.ok(labels.includes('Open Brave'), labels.join(', '));
  await page.click('.menu-item:has-text("Remove Brave from this pane")');
  assert.deepEqual((await fake.waitForCall('RemoveRemembered')).args, ['p5', 'com.brave.Browser']);
});

test('the focus stays in the panes after Close this pane', async (t) => {
  const world = defaultWorld({ current: 2 });
  for (const id of ['w7', 'w8', 'w9', 'w10']) world.items[id].hidden = true;
  world.remembered = {
    p5: [{ itemId: 'w9', bundleId: 'com.brave.Browser', app: 'Brave', mono: 'Br', text: 'Brave isn\'t running', canOpen: true }],
    p6: [{ itemId: 'w10', bundleId: 'md.obsidian', app: 'Obsidian', mono: 'Ob', text: 'Obsidian isn\'t running', canOpen: true }],
  };
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  fake.on('ClosePane', ([paneId], f) => {
    const tab = f.world.tabs[2];
    tab.root = pane('p5', ['w7', 'w8', 'w9']);
    tab.focus = 'p5';
    delete f.world.remembered[paneId];
    f.changed();
    return null;
  });
  await page.focus('[data-key="close-pane-p6"]');
  await page.keyboard.press('Enter');
  await page.waitForSelector('.pane[data-pane-id="p6"]', { state: 'detached' });
  await page.waitForFunction(() => document.activeElement && document.activeElement.dataset.key === 'open-p5-com.brave.Browser');
});

test('the focus stays in its pane after Remove from this pane', async (t) => {
  const world = defaultWorld({ current: 2 });
  world.tabs[2].focus = 'p5';
  world.items.w10.hidden = true;
  world.remembered = {
    p6: [
      { itemId: 'w10', bundleId: 'md.obsidian', app: 'Obsidian', mono: 'Ob', text: 'Obsidian isn\'t running', canOpen: true },
      { itemId: 'w11', bundleId: 'notion.id', app: 'Notion', mono: 'No', text: 'Notion isn\'t installed', canOpen: false },
    ],
  };
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  fake.on('RemoveRemembered', ([paneId, bundleId], f) => {
    f.world.remembered[paneId] = f.world.remembered[paneId].filter((r) => r.bundleId !== bundleId);
    f.changed();
    return null;
  });
  await page.focus('[data-key="forget-p6-md.obsidian"]');
  await page.keyboard.press('Enter');
  await page.waitForSelector('[data-key="forget-p6-md.obsidian"]', { state: 'detached' });
  // Notion's button takes the place of Obsidian's, in the same pane, not
  // the focused one.
  await page.waitForFunction(() => document.activeElement && document.activeElement.dataset.key === 'forget-p6-notion.id');
});

test('a tab with one pane has no Close this pane', async (t) => {
  const world = defaultWorld({ current: 0 });
  world.tabs[0].root = { type: 'pane', id: 'p1', items: ['w1'], active: 'w1' };
  world.tabs[0].focus = 'p1';
  world.items.w1.hidden = true;
  delete world.items.w2;
  world.remembered = { p1: [{ itemId: 'w1', bundleId: 'com.tinyspeck.slackmacgap', app: 'Slack', mono: 'Sl', text: 'Slack isn\'t running', canOpen: true }] };
  const { page, close } = await openBench({ world });
  t.after(close);
  const empty = page.locator('.pane[data-pane-id="p1"] .empty');
  assert.match(await empty.textContent(), /Slack isn't running/);
  assert.equal(await empty.locator('[data-action="close-pane"]').count(), 0);
  assert.equal(await empty.locator('[data-action="add"]').count(), 1);
});

test('the status bar shows notices, and the accessibility banner asks for permission', async (t) => {
  const notices = ['Couldn\'t register ⌃⌥Space. Try ⌃⌥J.', 'config.toml: tab "Code": unknown preset "mainside".', 'Screen Recording is off.'];
  const { page, fake, close } = await openBench({ world: defaultWorld({ notices, accessibility: false, configWritable: false }) });
  t.after(close);
  const notice = page.locator('.status-notice');
  assert.equal(await notice.locator('.notice-text').textContent(), notices[0]);
  assert.equal(await notice.locator('.notice-more').textContent(), '2 more');
  // A short notice is as wide as its text, not stretched to fill the bar.
  assert.ok(await notice.evaluate((el) => {
    const r = el.getBoundingClientRect();
    return el.scrollWidth <= el.clientWidth && r.width < 400;
  }), 'the notice hugs its text');
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

  // Tab properties and Settings say changes aren't saved when config.toml
  // has problems.
  await openTabProperties(page);
  assert.match(await page.textContent('.editor-sheet .editor-note'), /aren't saved to it/);
  await page.keyboard.press('Escape');
  await openSettings(page);
  assert.match(await page.textContent('.settings-sheet .editor-note'), /aren't saved to it/);
});

test('a notice\'s buttons call Go, from the mouse or the keyboard', async (t) => {
  const changed = 'config.toml was changed outside Bench, so Bench didn\'t save its own changes over it.';
  const notices = [
    { text: changed, actions: [{ label: 'Reload', method: 'ReloadConfig' }, { label: 'Overwrite', method: 'OverwriteConfig' }] },
    'Couldn\'t register ⌃⌥Space.',
  ];
  const { page, fake, close } = await openBench({ world: defaultWorld({ notices }) });
  t.after(close);
  assert.equal(await page.textContent('.status-notice .notice-text'), changed);
  await page.click('[data-action="notices"]');
  await page.waitForSelector('#notices-menu');
  const listed = await page.$$eval('#notices-menu .menu-text', (els) => els.map((el) => ({
    text: el.firstChild.textContent,
    buttons: Array.from(el.querySelectorAll('button'), (b) => b.textContent),
  })));
  assert.deepEqual(listed, [
    { text: changed, buttons: ['Reload', 'Overwrite'] },
    { text: 'Couldn\'t register ⌃⌥Space.', buttons: [] },
  ]);

  // Tab and the arrow keys move between the buttons. Shift-Tab before the
  // first, like Tab past the last, closes the notices, and moves on from
  // their button, as Tab does in a menu.
  const focused = () => page.evaluate(() => document.activeElement.dataset.key);
  assert.equal(await focused(), 'notices-list');
  await page.keyboard.press('Tab');
  assert.equal(await focused(), 'notice-0-0');
  await page.keyboard.press('ArrowRight');
  assert.equal(await focused(), 'notice-0-1');
  await page.keyboard.press('ArrowDown');
  assert.equal(await focused(), 'notice-0-0');
  await page.keyboard.press('Shift+Tab');
  await page.waitForSelector('#notices-menu', { state: 'detached' });
  assert.equal(await focused(), 'layout-reason');
  assert.equal(fake.callsTo('ReloadConfig').length + fake.callsTo('OverwriteConfig').length, 0);

  // Overwrite, clicked, closes the notices and calls Go.
  await page.click('[data-action="notices"]');
  await page.click('#notices-menu button:has-text("Overwrite")');
  await fake.waitForCall('OverwriteConfig');
  await page.waitForSelector('#notices-menu', { state: 'detached' });
  await page.waitForFunction(() => document.querySelector('.status-notice .notice-text').textContent.startsWith('Couldn'));

  // Reload, from the keyboard.
  fake.world.notices = notices.slice();
  await fake.emitState();
  await page.click('[data-action="notices"]');
  await page.waitForSelector('#notices-menu');
  await page.keyboard.press('Tab');
  await page.keyboard.press('Enter');
  await fake.waitForCall('ReloadConfig');
  await page.waitForSelector('#notices-menu', { state: 'detached' });
});

test('the arrangement notice restores the arrangement, or goes when dismissed', async (t) => {
  const notice = {
    text: 'When Bench started, config.toml had no tab called Code, so Bench couldn\'t keep its arrangement.',
    actions: [{ label: 'Restore previous arrangement', method: 'RestoreArrangement' }, { label: 'Dismiss', method: 'DismissArrangementNotice' }],
  };
  const { page, fake, close } = await openBench({ world: defaultWorld({ notices: [notice] }) });
  t.after(close);
  const noNotices = () => page.waitForFunction(() => !(window.bench.store.view.notices || []).length);
  await page.click('[data-action="notices"]');
  await page.click('#notices-menu button:has-text("Restore previous arrangement")');
  await fake.waitForCall('RestoreArrangement');
  await noNotices();

  fake.world.notices = [notice];
  await fake.emitState();
  await page.click('[data-action="notices"]');
  await page.click('#notices-menu button:has-text("Dismiss")');
  await fake.waitForCall('DismissArrangementNotice');
  await noNotices();
});

test('the first-run guide explains permissions, and its last page\'s Done dismisses it', async (t) => {
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
  await page.waitForSelector('.guide-sheet[data-page="setup"]');
  // The world has three tabs, so the set-up offers to add none rather
  // than to start with one.
  await page.click('.guide-sheet button:has-text("Don\'t add tabs")');
  await page.waitForSelector('.guide-sheet[data-page="tips"]');
  assert.equal(fake.callsTo('DismissFirstRun').length, 0);
  await page.click('.guide-sheet button:has-text("Done")');
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

test('when Accessibility is granted, the focus stays in the first-run guide', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ firstRun: true, accessibility: false }) });
  t.after(close);
  await page.waitForSelector('.guide-sheet');
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'guide-allow');
  fake.world.permissions.accessibility = true;
  await fake.emitState();
  await page.waitForSelector('.guide-sheet .guide-ok');
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'guide-continue');
});

test('if a state is drawn before Ready answers, the UI says again that Bench is raised, since Ready lowers it', async (t) => {
  const { page, fake, close } = await openBench({
    world: defaultWorld({ firstRun: true, accessibility: false }),
    // A state arrives while the UI waits for its fonts, before it calls
    // Ready, so the first render opens the guide and raises Bench first.
    setup: (f) => {
      let sent = false;
      f.page.route('**/vendor/fonts/*.woff2', async (route) => {
        if (!sent) {
          sent = true;
          await f.emitState();
          await new Promise((r) => setTimeout(r, 150));
        }
        await route.continue();
      });
    },
  });
  t.after(close);
  await page.waitForSelector('.guide-sheet');
  await settle(page);
  const seq = fake.calls.filter((c) => c.method === 'Ready' || c.method === 'SetOverlay')
    .map((c) => (c.method === 'Ready' ? 'Ready' : 'SetOverlay(' + c.args[0] + ')'));
  const ready = seq.indexOf('Ready');
  assert.ok(seq.indexOf('SetOverlay(true)') >= 0 && seq.indexOf('SetOverlay(true)') < ready, 'raised before Ready: ' + seq);
  assert.ok(seq.lastIndexOf('SetOverlay(true)') > ready, 'raised again after Ready: ' + seq);
  assert.equal(seq[seq.length - 1], 'SetOverlay(true)', seq.join(', '));
});

test('a window that needs more room says so on its sub-tab and in its pane header', async (t) => {
  const world = defaultWorld();
  world.items.w3.status = 'needs-room';
  world.items.w3.statusText = 'Zed\'s window can\'t shrink to fit this pane. Try the compact layout';
  world.items.w4.status = 'full-screen';
  world.items.w4.statusText = 'In full screen';
  const { page, close } = await openBench({ world });
  t.after(close);
  const sub = await page.$eval('.subtab[data-item="w3"]', (el) => [el.dataset.status, el.title, el.getAttribute('aria-label')]);
  assert.deepEqual(sub, [
    'needs-room',
    'Zed — bench — switch.go\nZed\'s window can\'t shrink to fit this pane. Try the compact layout.',
    'bench — switch.go, Zed (needs more room)',
  ]);
  // Its window covers the pane body, so the header says so.
  const note = page.locator('.pane[data-pane-id="p3"] .pane-header .pane-note');
  assert.equal(await note.textContent(), 'Needs more room');
  assert.equal(await note.isVisible(), true);
  assert.equal(await note.getAttribute('title'), 'Zed\'s window can\'t shrink to fit this pane. Try the compact layout.');
  // A warning sign shows on the tile.
  assert.equal(await page.getAttribute('.subtab[data-item="w3"]', 'data-marker'), 'problem');
  assert.equal(await page.isVisible('.subtab[data-item="w3"] .mono-tile .status-mark[data-marker="problem"]'), true);
  // An inactive item's status shows on its sub-tab only, here with the
  // marker for information.
  assert.equal(await page.getAttribute('.subtab[data-item="w4"]', 'data-status'), 'full-screen');
  assert.equal(await page.getAttribute('.subtab[data-item="w4"]', 'data-marker'), 'info');
  assert.equal(await page.getAttribute('.subtab[data-item="w4"]', 'aria-label'), 'index.html — bench-ui, VS Code (in full screen)');
  assert.equal(await page.$$eval('.pane-note', (els) => els.length), 1);
  // Ready items have no status, and no marker.
  assert.equal(await page.getAttribute('.subtab[data-item="w5"]', 'data-status'), null);
  assert.equal(await page.$('.subtab[data-item="w5"] .status-mark'), null);
});

test('a redraw while the pointer is held down waits, so that the click still lands', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  const box = await page.locator('.subtab[data-item="w4"]').boundingBox();
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.down();
  // A terminal's title changes, which redraws the panes.
  fake.world.items.w5.title = 'make — ~/code/bench';
  await fake.emitState();
  await settle(page);
  assert.equal(await page.textContent('.subtab[data-item="w5"] .subtab-title'), 'zsh — ~/code/bench');
  await page.mouse.up();
  assert.deepEqual((await fake.waitForCall('ShowItem')).args, ['p3', 'w4']);
  // Then the redraw happens.
  await page.waitForSelector('.subtab[data-item="w5"]:has-text("make — ~/code/bench")');
  await page.waitForSelector('.subtab[data-item="w4"][aria-selected="true"]');
});

test('a move that reports no button held still waits for the release, so that the click lands', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  const box = await page.locator('.subtab[data-item="w4"]').boundingBox();
  const x = box.x + box.width / 2;
  const y = box.y + box.height / 2;
  await page.mouse.move(x, y);
  await page.mouse.down();
  fake.world.items.w5.title = 'make — ~/code/bench';
  await fake.emitState();
  await settle(page);
  // WebKit on macOS sends the last move of a quick release with no button
  // held, just before the release.
  await page.evaluate(([cx, cy]) => {
    window.dispatchEvent(new PointerEvent('pointermove', { clientX: cx, clientY: cy, buttons: 0, pointerId: 1, pointerType: 'mouse', isPrimary: true, bubbles: true }));
  }, [x, y]);
  await settle(page, 60);
  assert.equal(await page.textContent('.subtab[data-item="w5"] .subtab-title'), 'zsh — ~/code/bench');
  await page.mouse.up();
  assert.deepEqual((await fake.waitForCall('ShowItem')).args, ['p3', 'w4']);
  await page.waitForSelector('.subtab[data-item="w5"]:has-text("make — ~/code/bench")');

  // With no release after it, the button came up where Bench couldn't see
  // it, and the redraw goes ahead.
  await page.mouse.move(x, y);
  await page.mouse.down();
  fake.world.items.w5.title = 'zsh — ~/code/bench';
  await fake.emitState();
  await settle(page);
  assert.equal(await page.textContent('.subtab[data-item="w5"] .subtab-title'), 'make — ~/code/bench');
  await page.evaluate(([cx, cy]) => {
    window.dispatchEvent(new PointerEvent('pointermove', { clientX: cx, clientY: cy, buttons: 0, pointerId: 1, pointerType: 'mouse', isPrimary: true, bubbles: true }));
  }, [x, y]);
  await page.waitForSelector('.subtab[data-item="w5"]:has-text("zsh — ~/code/bench")', { timeout: 1000 });
  await page.mouse.up();
});

test('in the browser preview, a web item named like an Object method draws its placeholder', async (t) => {
  const world = defaultWorld({ demo: true });
  // A single-label host is its own app name, as for http://constructor/.
  world.items.w4 = { ...world.items.w4, kind: 'web', app: 'constructor', title: 'constructor', url: 'http://constructor/' };
  world.tabs[1].root.children[0].active = 'w4';
  const { page, close } = await openBench({ world });
  t.after(close);
  assert.equal(await page.textContent('.pane[data-pane-id="p3"] .urlbar .field'), 'http://constructor/');
  assert.equal(await page.textContent('#statusbar .status-count'), '4 windows in 2 panes');
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
  await openTabProperties(page);
});

test('a press on the title bar\'s empty space, which moves the window, doesn\'t hold redraws', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  // In the app, Wails moves the window from here (--wails-draggable), and
  // the page never sees the button come up: between + and the layout
  // control.
  const plus = await page.locator('#titlebar [data-action="new-tab"]').boundingBox();
  const end = await page.locator('#titlebar .titlebar-end').boundingBox();
  const at = { x: (plus.x + plus.width + end.x) / 2, y: plus.y + plus.height / 2 };
  assert.equal(await page.evaluate(({ x, y }) => {
    const el = document.elementFromPoint(x, y);
    return getComputedStyle(el).getPropertyValue('--wails-draggable').trim();
  }, at), 'drag');
  await page.mouse.move(at.x, at.y);
  await page.mouse.down();
  fake.world.items.w5.title = 'make — ~/code/bench';
  await fake.emitState();
  await page.waitForSelector('.subtab[data-item="w5"]:has-text("make — ~/code/bench")', { timeout: 2000 });
  await page.mouse.up();
});

test('a terminal asked to take the focus while the pointer is held down takes it on release', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  fake.world.tabs[1].focus = 'p4';
  await fake.emitState();
  await page.waitForSelector('.pane[data-pane-id="p4"][data-focused="true"] .xterm');
  const bar = await page.locator('#statusbar').boundingBox();
  await page.mouse.move(bar.x + bar.width / 2, bar.y + bar.height / 2);
  await page.mouse.down();
  await fake.emit('bench:command', { name: 'focus-terminal', itemId: 'w5' });
  await settle(page);
  await page.mouse.up();
  await page.waitForFunction(() => document.activeElement.closest('.term-host[data-term-host="w5"]'), null, { timeout: 2000 });
});

test('the new-tab + sits right after the tabs, and stays in view at the end of a strip that scrolls', async (t) => {
  const { page, fake, close } = await openBench({ viewport: { width: 1280, height: 800 } });
  t.after(close);
  const last = await page.locator('#titlebar .tab').last().boundingBox();
  const plus = await page.locator('#titlebar [data-action="new-tab"]').boundingBox();
  const end = await page.locator('#titlebar .titlebar-end').boundingBox();
  const bar = await page.locator('#titlebar').boundingBox();
  const gap = plus.x - (last.x + last.width);
  assert.ok(gap >= 0 && gap <= 8, 'the + is ' + gap + ' px after the last tab');
  // The layout control, Jump to…, and Settings stay at the right, with the
  // title bar's own space between.
  assert.ok(Math.abs(bar.x + bar.width - 12 - (end.x + end.width)) <= 1, 'the end group is at the right');
  assert.ok(end.x - (plus.x + plus.width) > 200, 'the space between is free');
  assert.equal(await page.getAttribute('#titlebar [data-action="new-tab"]', 'title'), 'New tab');

  // With more tabs than fit, the strip scrolls, + stays at its end, and
  // the title bar stays one row.
  const height = bar.height;
  for (let i = 3; i < 13; i++) fake.world.tabs.push({ ...fake.world.tabs[0], name: 'Another tab ' + (i + 1) });
  await fake.emitState();
  await page.waitForSelector('#titlebar .tab[data-index="12"]');
  await settle(page);
  const strip = await page.$eval('#titlebar .tabs', (el) => ({ client: el.clientWidth, scroll: el.scrollWidth, right: el.getBoundingClientRect().right }));
  assert.ok(strip.scroll > strip.client, 'the strip overflows');
  const more = await page.locator('#titlebar [data-action="new-tab"]').boundingBox();
  const after = await page.locator('#titlebar .titlebar-end').boundingBox();
  assert.ok(more.x >= strip.right && more.x - strip.right <= 8, 'the + follows the strip');
  assert.ok(more.x + more.width <= after.x, 'the + is in view, before the layout control');
  assert.equal((await page.locator('#titlebar').boundingBox()).height, height);
  assert.equal(after.y, end.y);
});

test('each kind of status has its own marker, and the status bar says how many windows need attention', async (t) => {
  const world = defaultWorld();
  world.items.w3.status = 'missing';
  world.items.w3.statusText = 'Zed isn\'t running';
  world.items.w4.status = 'unavailable';
  world.items.w4.statusText = 'On another desktop';
  world.items.w6.status = 'launching';
  world.items.w6.statusText = 'Opening Terminal…';
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  const marks = await page.$$eval('.subtab', (els) => els.map((el) => {
    const mark = el.querySelector('.mono-tile .status-mark');
    return [el.dataset.item, el.dataset.marker || '', mark ? mark.dataset.marker : '', mark ? mark.querySelector('svg').innerHTML.includes('circle') : null];
  }));
  // A warning sign, an i in a circle, nothing, and a ring.
  assert.deepEqual(marks, [['w3', 'problem', 'problem', false], ['w4', 'info', 'info', true], ['w5', '', '', null], ['w6', 'opening', 'opening', true]]);
  // The labels keep saying why, for screen readers.
  assert.equal(await page.getAttribute('.subtab[data-item="w4"]', 'aria-label'), 'index.html — bench-ui, VS Code (on another desktop)');
  assert.equal(await page.getAttribute('.subtab[data-item="w6"]', 'aria-label'), 'go test — ~/code/bench, Terminal (opening)');
  // The marker colours differ too: amber for a problem, grey otherwise.
  const colours = await page.$$eval('.subtab .status-mark', (els) => els.map((el) => getComputedStyle(el).color));
  assert.notEqual(colours[0], colours[1]);
  assert.equal(colours[1], colours[2]);

  // Only windows with a problem need attention: not those opening, nor
  // those on another desktop or in full screen, which are for information.
  const note = page.locator('#statusbar .status-attention');
  assert.equal(await note.textContent(), '1 window needs attention');
  assert.equal(await note.getAttribute('aria-haspopup'), 'menu');
  fake.world.items.w6.status = 'needs-room';
  await fake.emitState();
  await page.waitForSelector('#statusbar .status-attention:has-text("2 windows need attention")');

  // With no problems left, there's no note.
  fake.world.items.w3.status = 'ready';
  fake.world.items.w6.status = 'ready';
  fake.world.items.w4.status = 'full-screen';
  fake.world.items.w4.statusText = 'In full screen';
  await fake.emitState();
  await page.waitForSelector('#statusbar .status-attention', { state: 'detached' });
  assert.equal(await page.getAttribute('.subtab[data-item="w4"]', 'data-marker'), 'info');
  assert.deepEqual(fake.errors, []);
});

test('the status bar stays on one line, cutting the notice short and leaving out the key hints', async (t) => {
  const notices = ['config.toml: tab "A tab with a long name for a small screen": unknown preset "mainside". Bench uses its default layout until you fix it.'];
  const world = defaultWorld({ notices });
  world.tabs[1].name = 'A tab with a long name for a small screen';
  world.items.w3.status = 'needs-room';
  world.items.w4.status = 'missing';
  const { page, close } = await openBench({ world, viewport: { width: 900, height: 600 } });
  t.after(close);
  const bar = await page.locator('#statusbar').boundingBox();
  assert.equal(bar.height, 30);
  const parts = await page.$eval('#statusbar', (el) => {
    const box = (sel) => el.querySelector(sel).getBoundingClientRect();
    const cut = (sel) => {
      const n = el.querySelector(sel);
      return n.scrollWidth > n.clientWidth;
    };
    const b = el.getBoundingClientRect();
    return {
      // Everything else is on the bar's one line.
      inside: ['.status-count', '.status-attention', '.status-layout', '.status-notice']
        .every((sel) => box(sel).top >= b.top && box(sel).bottom <= b.bottom && box(sel).right <= b.right),
      keys: box('.status-keys').top >= b.bottom,
      // The notice is cut short, and the rest is whole.
      notice: cut('.status-notice .notice-text'),
      whole: ['.status-count', '.status-attention', '.status-layout'].every((sel) => !cut(sel)),
    };
  });
  assert.deepEqual(parts, { inside: true, keys: true, notice: true, whole: true });
  assert.equal(await page.textContent('#statusbar .status-count'), '4 windows in 2 panes');
  assert.equal(await page.textContent('#statusbar .status-attention'), '2 windows need attention');
  assert.equal(await page.textContent('#statusbar .status-layout'), 'Auto: wide');

  // With room, the key hints come back, at the right.
  await page.setViewportSize({ width: 1800, height: 900 });
  await page.waitForFunction(() => {
    const b = document.getElementById('statusbar').getBoundingClientRect();
    return document.querySelector('.status-keys').getBoundingClientRect().bottom <= b.bottom;
  });
  assert.equal((await page.locator('#statusbar').boundingBox()).height, 30);
});
