'use strict';

// Native menus (decision 71): in the app, View.nativeMenus is set, and the
// window, tab, empty pane, terminal, and layout menus show as native macOS
// menus through ShowMenu, without raised mode, with the same entries as
// the HTML menus that the browser preview draws. The frontend runs the
// entry whose ID comes back.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, settle } = require('./helpers/harness.js');
const { defaultWorld } = require('./helpers/fixture.js');

// appWorld is the default world as the app has it, with native menus.
const appWorld = (opts = {}) => defaultWorld({ nativeMenus: true, ...opts });

// shownMenus returns the menus that the frontend asked Go to show, in turn.
const shownMenus = (fake) => fake.callsTo('ShowMenu').map((c) => c.args[0]);

// waitForMenu waits for the nth menu (from 1) that the frontend asks Go to
// show, and returns it.
async function waitForMenu(fake, n = 1) {
  await fake.waitForCall('ShowMenu', () => shownMenus(fake).length >= n);
  return shownMenus(fake)[n - 1];
}

// lines describes a native menu's entries as htmlLines describes an HTML
// menu's: "-" for a separator, a heading, note, or status line as its
// text, and an entry as its label, with its keys after a space, "▸" after
// one with a submenu, and " (off)" after one that's off.
function lines(entries) {
  return entries.map((e) => {
    if (e.sep) return '-';
    if (e.heading || e.note || e.status) return e.heading || e.note || e.status;
    return e.label + (e.keys ? ' ' + e.keys : '') + (e.submenu ? ' ▸' : '') + (e.disabled ? ' (off)' : '');
  });
}

// htmlLines describes the HTML menu, or submenu, drawn at sel, as lines
// describes a native menu. An entry with a detail, which the native menu
// shows in its label, is described by its label alone.
const htmlLines = (page, sel) => page.$$eval(sel + ' > *', (els) => els.map((el) => {
  if (el.classList.contains('menu-sep')) return '-';
  if (!el.classList.contains('menu-item')) return el.textContent.trim();
  const main = el.querySelector('.menu-main') || el.querySelector('.menu-label');
  const kbd = el.querySelector('kbd');
  return main.textContent + (kbd ? ' ' + kbd.textContent : '') + (el.classList.contains('has-sub') ? ' ▸' : '') + (el.disabled ? ' (off)' : '');
}));

// ids lists the IDs of a native menu's entries, its submenus' too.
const ids = (entries) => entries.flatMap((e) => (e.submenu ? [e.id, ...ids(e.submenu)] : (e.id ? [e.id] : [])));

// raised reports whether the frontend has asked Go for raised mode.
const raised = (fake) => fake.callsTo('SetOverlay').some((c) => c.args[0] === true);

// sameMenus checks that a native menu has the entries that the HTML menu
// has in the browser preview, its submenus' too, opening the HTML menu
// with open, in a Bench without native menus.
async function sameMenus(t, native, sel, open, world = defaultWorld({ current: 0 })) {
  const { page, close } = await openBench({ world });
  t.after(close);
  await open(page);
  await page.waitForSelector(sel);
  assert.deepEqual(await htmlLines(page, sel), lines(native.entries));
  for (const [i, e] of native.entries.entries()) {
    if (!e.submenu) continue;
    await page.click(sel + ' > :nth-child(' + (i + 1) + ')');
    await page.waitForSelector('#menu-sub');
    assert.deepEqual(await htmlLines(page, '#menu-sub'), lines(e.submenu), e.label);
  }
}

test('in the app, a web page\'s ⋯ menu is native, with the HTML menu\'s entries, its web app entry built in, and no raised mode', async (t) => {
  const { page, fake, close } = await openBench({ world: appWorld({ current: 0 }) });
  t.after(close);
  assert.equal(await page.evaluate(() => window.bench.store.view.nativeMenus), true);
  await page.click('[data-action="menu"][data-pane="p1"]');
  const menu = await waitForMenu(fake);
  // A native menu has no headings: Move to Pane is a submenu, as Move to
  // Tab is, so that each pane's item says what it does. The Page submenu
  // has what was a row of buttons; finding, the link, and the page's sound
  // and video stay in the menu itself.
  assert.deepEqual(lines(menu.entries), [
    'Reload Page', 'Rename…', 'Close Page', '-',
    'Split Right ⌃⌥⇧→', 'Split Down ⌃⌥⇧↓', 'Split Right with an Empty Pane', 'Split Down with an Empty Pane', 'Zoom Pane ⌃⌥↩', '-',
    'Move to Pane ▸', 'Move to Tab ▸', 'Also Show In ▸', '-',
    'Page ▸', 'Find in Page…', 'Copy Page Link', 'Stop Sound and Video', '-',
    'Open in Browser', 'Pop Out of Bench', '-',
    'Use the Slack Web App from Brave',
  ]);
  assert.ok(menu.entries.every((e) => !e.heading), JSON.stringify(menu.entries));
  const movePane = menu.entries.find((e) => e.id === 'move-to-pane');
  assert.deepEqual(lines(movePane.submenu), ['Pane with Outlook — Inbox']);
  const pageMenu = menu.entries.find((e) => e.id === 'page');
  assert.deepEqual(lines(pageMenu.submenu), ['Back (off)', 'Forward (off)', '-', 'Zoom In', 'Zoom Out', 'Actual Size (off)']);
  const moveTo = menu.entries.find((e) => e.id === 'move-to-tab');
  assert.deepEqual(lines(moveTo.submenu), ['Comms ⌃⌥⇧1 (off)', 'Code ⌃⌥⇧2', 'Research ⌃⌥⇧3', '-', 'New Tab']);
  // Each entry that can be chosen has an ID of its own; separators,
  // headings, and notes have none.
  const all = ids(menu.entries);
  assert.equal(new Set(all).size, all.length, JSON.stringify(all));
  assert.ok(menu.entries.filter((e) => e.sep || e.heading).every((e) => !e.id));
  // It opens below the ⋯ button, at its left, and nothing is drawn in HTML.
  const button = await page.locator('[data-action="menu"][data-pane="p1"]').boundingBox();
  assert.deepEqual([menu.x, menu.y], [Math.round(button.x), Math.round(button.y + button.height + 4)]);
  await settle(page);
  assert.equal(await page.$('#menu-root .menu'), null);
  // Bench isn't raised, so the web pages stay in view.
  assert.equal(raised(fake), false);
  assert.equal(await page.evaluate(() => window.bench.store.ui.nativeMenu), null);

  // The browser preview's HTML menu has the same entries.
  await sameMenus(t, menu, '#window-menu', async (p) => {
    await p.click('[data-action="menu"][data-pane="p1"]');
    await p.waitForSelector('#window-menu .menu-item:has-text("Use the Slack Web App from Brave")');
  });
});

test('the entry chosen in a native menu runs, in a submenu too, and closing it does what the click would have done', async (t) => {
  const { page, fake, close } = await openBench({ world: appWorld({ current: 0 }) });
  t.after(close);
  const open = async (n) => {
    await page.click('[data-action="menu"][data-pane="p1"]');
    await waitForMenu(fake, n);
  };
  fake.world.menuChoice = 'split-right';
  await open(1);
  assert.deepEqual((await fake.waitForCall('MoveItem')).args, ['w1', 0, 'p1', 'right']);
  fake.world.menuChoice = 'page-find';
  await open(2);
  assert.deepEqual((await fake.waitForCall('SetFindBar')).args, ['w1', true]);
  await page.waitForFunction(() => document.activeElement && document.activeElement.classList.contains('page-find-input'));
  await page.keyboard.press('Escape');
  fake.world.menuChoice = 'move-to-tab:2';
  await open(3);
  assert.deepEqual((await fake.waitForCall('MoveItemToTab')).args, ['w1', 2]);
  fake.world.menuChoice = 'move-to-pane:p2';
  await open(4);
  assert.deepEqual((await fake.waitForCall('MoveItem', (a) => a[3] === 'center')).args, ['w1', 0, 'p2', 'center']);

  // A right-click on a sub-tab that isn't shown opens the menu at the
  // pointer. Closed with nothing chosen, the sub-tab is shown, as a
  // click on it would.
  fake.world.current = 1;
  fake.world.menuChoice = '';
  await fake.emitState();
  const sub = page.locator('.subtab[data-item="w4"]');
  const box = await sub.boundingBox();
  await sub.click({ button: 'right', position: { x: 20, y: 8 } });
  const menu = await waitForMenu(fake, 5);
  assert.deepEqual([menu.x, menu.y], [Math.round(box.x + 20), Math.round(box.y + 8)]);
  assert.deepEqual((await fake.waitForCall('ShowItem')).args, ['p3', 'w4']);
  // With an entry chosen, it isn't.
  fake.clearCalls();
  fake.world.menuChoice = 'move-left';
  await sub.click({ button: 'right' });
  assert.deepEqual((await fake.waitForCall('MoveItemAt')).args, ['w4', 1, 'p3', 0]);
  await settle(page);
  assert.equal(fake.callsTo('ShowItem').length, 0);
  assert.equal(raised(fake), false);
});

test('a native menu\'s choice doesn\'t run once its subject has gone, and a menu that can\'t be shown natively opens in HTML', async (t) => {
  const { page, fake, close } = await openBench({ world: appWorld({ current: 0 }) });
  t.after(close);
  // Go switches tabs while the menu is open.
  fake.on('ShowMenu', async (args, f) => {
    f.world.current = 2;
    await f.emitState();
    return 'split-right';
  });
  await page.click('[data-action="menu"][data-pane="p1"]');
  await fake.waitForCall('ShowMenu');
  await page.waitForSelector('.pane[data-pane-id="p5"]');
  await settle(page);
  assert.equal(fake.callsTo('MoveItem').length, 0);

  // Go can't show it: the HTML menu opens in its place, raised.
  fake.on('ShowMenu', () => {
    throw new Error('Bench couldn\'t show the menu. Try again.');
  });
  await page.click('[data-action="menu"][data-pane="p5"]');
  await page.waitForSelector('#window-menu');
  await fake.waitForCall('SetOverlay', (a) => a[0] === true);
  await page.keyboard.press('Escape');
  await page.waitForSelector('#window-menu', { state: 'detached' });
});

test('a tab\'s menu is native in the app, below the tab from the keyboard, with the HTML menu\'s entries', async (t) => {
  const world = appWorld({ current: 1, closedTabs: ['Old'] });
  const { page, fake, close } = await openBench({ world });
  t.after(close);

  // A tab's menu, from the keyboard: below the tab. Choosing Tab
  // Properties… opens the dialog, which gives the focus back to the tab.
  await page.focus('.tab[data-index="2"]');
  await page.keyboard.press('Shift+F10');
  let menu = await waitForMenu(fake, 1);
  const tab = await page.locator('.tab[data-index="2"]').boundingBox();
  assert.deepEqual([menu.x, menu.y], [Math.round(tab.x), Math.round(tab.y + tab.height + 4)]);
  assert.deepEqual(lines(menu.entries), [
    'Tab Properties…', 'Rename…', '-', 'Move Tab Left', 'Move Tab Right (off)', '-', 'Rearrange', 'Reset Layout', '-',
    'Close Tab…', 'Delete Tab (Keep Windows)', '-', 'New Tab…', 'Reopen Closed Tab',
  ]);
  assert.equal(menu.entries.find((e) => e.id === 'reopen-tab').detail, 'Old');
  await sameMenus(t, menu, '#tab-menu', async (p) => {
    await p.click('.tab[data-index="2"]', { button: 'right' });
  }, defaultWorld({ current: 1, closedTabs: ['Old'] }));

  fake.world.menuChoice = 'tab-properties';
  await page.focus('.tab[data-index="2"]');
  await page.keyboard.press('Shift+F10');
  await waitForMenu(fake, 2);
  await page.waitForSelector('#editor-title:text("Tab properties")');
  await page.keyboard.press('Escape');
  await page.waitForSelector('#editor-title', { state: 'detached' });
  await page.waitForFunction(() => document.activeElement && document.activeElement.matches('.tab[data-index="2"]'));
  assert.equal(raised(fake), true, 'the dialog raises Bench');
});

test('a slim title bar\'s layout menu is native in the app, with the layout in use ticked and why as a note', async (t) => {
  const { page, fake, close } = await openBench({ world: appWorld({ mode: 'compact' }) });
  t.after(close);
  await page.waitForSelector('#titlebar[data-slim="true"]');
  fake.world.menuChoice = 'layout:auto';
  const button = await page.locator('[data-action="layout-menu"]').boundingBox();
  await page.click('[data-action="layout-menu"]');
  const menu = await waitForMenu(fake);
  assert.deepEqual([menu.x, menu.y], [Math.round(button.x), Math.round(button.y + button.height + 4)]);
  assert.equal(menu.entries.find((e) => e.id === 'layout:compact').checked, true);
  assert.equal(menu.entries.find((e) => e.id === 'layout:auto').checked, false);
  assert.ok(menu.entries.at(-1).note, JSON.stringify(menu.entries.at(-1)));
  assert.deepEqual((await fake.waitForCall('SetMode')).args, ['auto']);
  assert.equal(raised(fake), false);
  await sameMenus(t, menu, '#layout-menu', async (p) => {
    await p.click('[data-action="layout-menu"]');
  }, defaultWorld({ mode: 'compact' }));
});

test('an empty pane\'s menu is native in the app, with the HTML menu\'s entries', async (t) => {
  const world = appWorld({ current: 1 });
  world.tabs[1].root.children[1].items = [];
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await page.click('.pane[data-pane-id="p4"] .pane-body', { button: 'right', position: { x: 40, y: 40 } });
  const menu = await waitForMenu(fake);
  assert.deepEqual(lines(menu.entries), ['Add to Pane…', 'Close Pane', '-', 'Zoom Pane ⌃⌥↩']);
  fake.world.menuChoice = 'close-pane';
  await page.click('.pane[data-pane-id="p4"] .pane-body', { button: 'right', position: { x: 40, y: 40 } });
  assert.deepEqual((await fake.waitForCall('ClosePane')).args, ['p4']);
  assert.equal(raised(fake), false);
  await sameMenus(t, menu, '#pane-menu', async (p) => {
    await p.click('.pane[data-pane-id="p4"] .pane-body', { button: 'right', position: { x: 40, y: 40 } });
  }, (() => {
    const w = defaultWorld({ current: 1 });
    w.tabs[1].root.children[1].items = [];
    return w;
  })());
});

test('a terminal\'s native menu has its entries, and the terminal gets the keyboard back', async (t) => {
  const { page, fake, close } = await openBench({ world: appWorld({ current: 1 }) });
  t.after(close);
  const term = '.pane[data-pane-id="p4"] .term-host[data-term-host="w5"]';
  await page.waitForFunction((sel) => {
    const rows = document.querySelector(sel + ' .xterm-rows');
    return Boolean(rows && rows.textContent.includes('go run ./cmd/bench'));
  }, term);
  await page.click(term + ' .xterm');
  fake.world.menuChoice = 'select-all';
  const box = await page.locator(term + ' .xterm-screen').boundingBox();
  await page.mouse.click(box.x + 60, box.y + 40, { button: 'right' });
  const menu = await waitForMenu(fake);
  // A terminal's ⌘ keys work only in the terminal, and Mac context menus
  // show no keys: only Bench's global shortcuts go with a native menu's
  // entries. The HTML menu shows them.
  assert.deepEqual(lines(menu.entries), ['Copy (off)', 'Paste', '-', 'Find…', '-', 'Clear', 'Select All']);
  assert.deepEqual([menu.x, menu.y], [Math.round(box.x + 60), Math.round(box.y + 40)]);
  await page.waitForFunction((sel) => {
    const el = document.querySelector(sel + ' .xterm-helper-textarea');
    return document.activeElement === el;
  }, term);
  // Select All ran: Copy is on now.
  fake.world.menuChoice = '';
  await page.mouse.click(box.x + 60, box.y + 40, { button: 'right' });
  assert.equal((await waitForMenu(fake, 2)).entries[0].disabled, undefined);
});

test('a menu opened while a native menu is on its way does nothing, and the first menu\'s click does what it would', async (t) => {
  const { page, fake, close } = await openBench({ world: appWorld({ current: 0 }) });
  t.after(close);
  // Go takes a second to say what a page can offer, so a web page's
  // native menu waits a moment before it opens.
  fake.on('WebAppChoice', async (args, f) => {
    await new Promise((r) => setTimeout(r, 1000));
    return f.mWebAppChoice(...args);
  });
  // Two quick right-clicks: on Slack's sub-tab, in the focused pane, and
  // then on Outlook's, in the other pane.
  await page.click('.subtab[data-item="w1"]', { button: 'right' });
  await page.waitForTimeout(50);
  await page.click('.subtab[data-item="w2"]', { button: 'right' });
  const menu = await waitForMenu(fake);
  assert.deepEqual(lines(menu.entries.find((e) => e.id === 'move-to-pane').submenu), ['Pane with Outlook — Inbox']);
  // Slack's menu closes with nothing chosen: its click shows Slack in its
  // pane, which it does already. The second click, whose menu didn't
  // open, doesn't focus Outlook's pane.
  await page.waitForFunction(() => window.bench.store.ui.nativeMenu === null);
  await settle(page, 300);
  assert.equal(shownMenus(fake).length, 1);
  assert.deepEqual(fake.callsTo('FocusPane').map((c) => c.args), []);
  assert.deepEqual(fake.callsTo('ShowItem').map((c) => c.args), []);

  // A right-click on Outlook's sub-tab now opens its menu, and its click
  // focuses Outlook's pane once the menu closes with nothing chosen.
  await page.click('.subtab[data-item="w2"]', { button: 'right' });
  await waitForMenu(fake, 2);
  assert.deepEqual((await fake.waitForCall('FocusPane')).args, ['p2']);
});

test('a web page\'s native menu waits for its web app entry only until Go has answered once', async (t) => {
  const { page, fake, close } = await openBench({ world: appWorld({ current: 0 }) });
  t.after(close);
  let answered = 0;
  fake.on('WebAppChoice', async (args, f) => {
    await new Promise((r) => setTimeout(r, 1000));
    answered++;
    return f.mWebAppChoice(...args);
  });
  // timeToMenu clicks Slack's ⋯ and returns the nth native menu, and how
  // long, in milliseconds, it took to reach Go after the click.
  const timeToMenu = async (n) => {
    const start = Date.now();
    await page.click('[data-action="menu"][data-pane="p1"]');
    const menu = await waitForMenu(fake, n);
    return { ms: fake.callsTo('ShowMenu')[n - 1].at - start, menu };
  };
  const ids = (menu) => menu.entries.map((e) => e.id).filter(Boolean);

  // The first time, the menu waits for Go, a moment at most, and then
  // opens without the entry.
  const first = await timeToMenu(1);
  assert.ok(first.ms >= 250, 'opened after ' + first.ms + ' ms, before the wait was up');
  assert.ok(!ids(first.menu).includes('use-web-app'), JSON.stringify(ids(first.menu)));
  await page.waitForFunction(() => window.bench.store.ui.nativeMenu === null);
  // Go's answer comes later, and is kept for the next time.
  while (answered < 1) await new Promise((r) => setTimeout(r, 20));

  // The next time, the menu opens at once with the answer Go gave, and
  // asks again for the time after.
  const asked = fake.callsTo('WebAppChoice').length;
  const second = await timeToMenu(2);
  assert.ok(second.ms < 200, 'opened after ' + second.ms + ' ms, though Go had answered');
  assert.equal(ids(second.menu).at(-1), 'use-web-app');
  assert.equal(fake.callsTo('WebAppChoice').length, asked + 1);
});

test('a native menu has Dock Again for a window that keeps moving, as the HTML menu and the palette have it', async (t) => {
  // movingWorld has Brave's window keep moving out of its pane, as a
  // window tool would make it (decision 69).
  const movingWorld = (opts) => {
    const world = defaultWorld({ current: 2, ...opts });
    world.items.w7.status = 'moving';
    world.items.w7.statusText = 'Brave or Rectangle keeps moving this window';
    return world;
  };
  const { page, fake, close } = await openBench({ world: movingWorld({ nativeMenus: true }) });
  t.after(close);
  fake.world.menuChoice = 'dock-again';
  await page.click('[data-action="menu"][data-pane="p5"]');
  const menu = await waitForMenu(fake);
  // Its first line says why, and Dock Again is in the last group.
  assert.equal(menu.entries[0].status, 'Brave or Rectangle keeps moving this window.');
  assert.ok(ids(menu.entries).includes('dock-again'), JSON.stringify(ids(menu.entries)));
  assert.deepEqual((await fake.waitForCall('Redock')).args, ['w7']);
  assert.equal(raised(fake), false);
  await sameMenus(t, menu, '#window-menu', async (p) => {
    await p.click('[data-action="menu"][data-pane="p5"]');
  }, movingWorld());

  // The jump palette's This window has it too.
  await page.click('[data-action="open-palette"]');
  await page.fill('#palette-query', 'Dock Again');
  await page.waitForFunction(() => {
    const row = document.querySelector('.palette-item[aria-selected="true"] .palette-label');
    return row && row.textContent === 'Dock Again';
  });
  await page.keyboard.press('Enter');
  await fake.waitForCall('Redock', () => fake.callsTo('Redock').length === 2);
});

test('a switch from a native menu shows the tab as current at once', async (t) => {
  const { page, fake, close } = await openBench({ world: appWorld({ current: 2 }) });
  t.after(close);
  // Go takes a while to switch, as with apps that are slow to arrange.
  fake.on('SwitchTab', async (args, f) => {
    await new Promise((r) => setTimeout(r, 800));
    return f.mSwitchTab(...args);
  });
  await settle(page);
  // A tab's Rearrange, for a tab that isn't current, switches to it
  // first: the title bar shows it as current before Go has switched
  // (decision 67), and then Bench arranges it.
  fake.world.menuChoice = 'rearrange';
  await page.click('.tab[data-index="0"]', { button: 'right' });
  await waitForMenu(fake);
  await page.waitForSelector('.tab[data-index="0"][aria-selected="true"]', { timeout: 400 });
  assert.equal(await page.evaluate(() => window.bench.store.view.current), 2, 'the View has the old tab until Go answers');
  await fake.waitForCall('Rearrange');
  assert.deepEqual(fake.callsTo('SwitchTab').map((c) => c.args), [[0]]);
  await page.waitForFunction(() => window.bench.store.view.current === 0);
  assert.equal(raised(fake), false);
});
