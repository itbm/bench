'use strict';

// Clicking: tabs, sub-tabs, panes, the split button, the ⋯ menu, a tab's
// menu, the Settings button, and toasts for refused calls.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, openSettings, openTabProperties, settle } = require('./helpers/harness.js');
const { defaultWorld } = require('./helpers/fixture.js');

test('clicking a tab and a sub-tab calls SwitchTab and ShowItem', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('.tab:has-text("Research")');
  const sw = await fake.waitForCall('SwitchTab');
  assert.deepEqual(sw.args, [2]);
  await page.waitForSelector('.tab[data-index="2"][aria-pressed="true"]');
  await page.click('.subtab:has-text("Go packages")');
  const show = await fake.waitForCall('ShowItem');
  assert.deepEqual(show.args, ['p5', 'w9']);
  await page.waitForSelector('.subtab[data-item="w9"][aria-pressed="true"]');
  // Showing an item focuses its pane, so there's no separate FocusPane.
  assert.equal(fake.callsTo('FocusPane').length, 0);
  // Focus stays on the sub-tab after the redraw.
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'sub-w9');
});

test('clicking a pane focuses it', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('.pane[data-pane-id="p4"] .pane-body');
  const call = await fake.waitForCall('FocusPane');
  assert.deepEqual(call.args, ['p4']);
  await page.waitForSelector('.pane[data-pane-id="p4"][data-focused="true"]');
  assert.equal(await page.getAttribute('.pane[data-pane-id="p3"]', 'data-focused'), 'false');
  // Clicking the focused pane again doesn't call it again.
  await page.click('.pane[data-pane-id="p4"] .pane-body');
  await settle(page);
  assert.equal(fake.callsTo('FocusPane').length, 1);
});

test('the mode buttons call SetMode', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('[data-action="mode"][data-mode="compact"]');
  const call = await fake.waitForCall('SetMode');
  assert.deepEqual(call.args, ['compact']);
  await page.waitForSelector('[data-mode="compact"][aria-pressed="true"]');
  // Code stacks on small screens.
  await page.waitForFunction(() => {
    const [a, b] = document.querySelectorAll('.pane');
    return a && b && a.getBoundingClientRect().left === b.getBoundingClientRect().left;
  });
});

test('the ⋯ menu shows names as they are, even with @NOTE@ in them', async (t) => {
  const world = defaultWorld();
  world.tabs[0].name = 'Docs @NOTE@ x';
  const { page, close } = await openBench({ world });
  t.after(close);
  await page.click('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.waitForSelector('#window-menu');
  const labels = await page.$$eval('#window-menu .menu-label', (els) => els.map((el) => el.textContent));
  assert.ok(labels.includes('Docs @NOTE@ x'), 'the tab\'s name is in ' + JSON.stringify(labels));
  // No entry is disabled, so none is described by a note.
  assert.equal(await page.$('#window-menu [aria-describedby]'), null);
});

test('the split button calls MoveItem with zone right', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('.pane[data-pane-id="p3"] [data-action="split-right"]');
  const call = await fake.waitForCall('MoveItem');
  assert.deepEqual(call.args, ['w3', 1, 'p3', 'right']);
});

test('a refused call shows Go\'s message as a toast', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  fake.on('MoveItem', () => { throw new Error('A pane can\'t be split by its only window.'); });
  await page.click('.pane[data-pane-id="p4"] [data-action="split-right"]');
  await fake.waitForCall('MoveItem');
  await page.waitForFunction(() => document.getElementById('toast').classList.contains('show'));
  assert.equal(await page.textContent('#toast'), 'A pane can\'t be split by its only window.');
  assert.equal(await page.getAttribute('#toast', 'role'), 'status');
  // Go can send a toast too.
  await fake.emit('bench:toast', 'Rearranged Code');
  await page.waitForFunction(() => document.getElementById('toast').textContent === 'Rearranged Code');
});

test('the ⋯ menu entries call the right methods', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  const open = async () => {
    await page.click('.pane[data-pane-id="p3"] [data-action="menu"]');
    await page.waitForSelector('#window-menu');
  };

  await open();
  await fake.waitForCall('SetOverlay', (a) => a[0] === true);
  assert.equal(await page.getAttribute('.pane[data-pane-id="p3"] [data-action="menu"]', 'aria-expanded'), 'true');
  const entries = await page.$$eval('#window-menu > *', (els) => els.map((el) => el.className + ':' + el.textContent));
  assert.deepEqual(entries, [
    'menu-item:Show side by side (split right)⌃⌥⇧→',
    'menu-item:Split down⌃⌥⇧↓',
    'menu-sep:',
    'menu-heading:Move to pane',
    'menu-item:With Terminal — zsh — ~/code/bench',
    'menu-sep:',
    'menu-heading:Move to tab',
    'menu-item:Comms',
    'menu-item:Research',
    'menu-sep:',
    'menu-item:Also show in Comms',
    'menu-item:Also show in Research',
    'menu-sep:',
    'menu-item:Pop out of Bench',
  ]);
  // The first entry has focus; arrows move it.
  assert.equal(await page.evaluate(() => document.activeElement.textContent), 'Show side by side (split right)⌃⌥⇧→');
  await page.keyboard.press('ArrowDown');
  assert.equal(await page.evaluate(() => document.activeElement.textContent), 'Split down⌃⌥⇧↓');
  await page.keyboard.press('ArrowUp');
  await page.keyboard.press('ArrowUp');
  assert.equal(await page.evaluate(() => document.activeElement.textContent), 'Pop out of Bench');
  // Escape closes it and puts focus back on the button.
  await page.keyboard.press('Escape');
  await page.waitForSelector('#window-menu', { state: 'detached' });
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'menu-p3');
  await fake.waitForLowered();

  const cases = [
    ['Show side by side (split right)', 'MoveItem', ['w3', 1, 'p3', 'right']],
    ['Split down', 'MoveItem', ['w3', 1, 'p3', 'bottom']],
    ['With Terminal — zsh — ~/code/bench', 'MoveItem', ['w3', 1, 'p4', 'center']],
    ['Comms', 'MoveItemToTab', ['w3', 0]],
    ['Research', 'MoveItemToTab', ['w3', 2]],
    ['Also show in Comms', 'AddItemToTab', ['w3', 0]],
    ['Also show in Research', 'AddItemToTab', ['w3', 2]],
    ['Pop out of Bench', 'PopOut', ['w3']],
  ];
  for (const [label, method, args] of cases) {
    fake.clearCalls();
    await open();
    await page.click('#window-menu .menu-item:has-text("' + label + '")');
    const call = await fake.waitForCall(method);
    assert.deepEqual(call.args, args, label);
    await page.waitForSelector('#window-menu', { state: 'detached' });
  }

  // Clicking outside closes it.
  await open();
  await page.click('.statusbar');
  await page.waitForSelector('#window-menu', { state: 'detached' });
});

test('right-clicking a sub-tab opens its menu, and shows it only when the menu closes', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  // Showing an App item first would activate its app, which would then
  // get the menu's keys, so it waits.
  await page.click('.subtab:has-text("index.html")', { button: 'right' });
  await page.waitForSelector('#window-menu');
  await fake.waitForCall('SetOverlay', (a) => a[0] === true);
  await settle(page);
  assert.equal(await page.$('#tab-menu'), null, 'a sub-tab isn\'t a tab');
  assert.equal(fake.callsTo('ShowItem').length, 0);
  assert.equal(fake.callsTo('FocusPane').length, 0);
  // Running an entry doesn't show it first: the entry moves it.
  await page.click('#window-menu .menu-item:has-text("Split down")');
  const move = await fake.waitForCall('MoveItem');
  assert.deepEqual(move.args, ['w4', 1, 'p3', 'bottom']);
  await settle(page);
  assert.equal(fake.callsTo('ShowItem').length, 0);

  // Closing the menu without choosing shows it, as the right-click asked.
  fake.clearCalls();
  await page.click('.subtab:has-text("index.html")', { button: 'right' });
  await page.waitForSelector('#window-menu');
  await page.keyboard.press('Escape');
  await page.waitForSelector('#window-menu', { state: 'detached' });
  const show = await fake.waitForCall('ShowItem');
  assert.deepEqual(show.args, ['p3', 'w4']);
  assert.deepEqual(fake.calls.map((c) => c.method).filter((m) => m === 'SetOverlay' || m === 'ShowItem'), ['SetOverlay', 'SetOverlay', 'ShowItem']);
});

test('+ and ⋯ in an unfocused pane don\'t focus it until their dialog or menu closes', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  // Code: p3 (Zed) is focused, so focus p4 first.
  fake.world.tabs[1].focus = 'p4';
  await fake.emitState();
  await page.waitForSelector('.pane[data-pane-id="p4"][data-focused="true"]');
  fake.clearCalls();

  // + opens the add dialog for p3 without focusing p3, which would
  // activate Zed and send it the keys typed into the search field.
  await page.click('.pane[data-pane-id="p3"] [data-action="add"]');
  await page.waitForSelector('.palette');
  assert.deepEqual((await fake.waitForCall('AddOptions')).args, ['p3']);
  await page.keyboard.type('term');
  await settle(page);
  assert.equal(fake.callsTo('FocusPane').length, 0);
  // Adding focuses the pane by itself (in Go), so nothing else does.
  await page.keyboard.press('Enter');
  const add = await fake.waitForCall('AddToPane');
  assert.deepEqual(add.args, ['p3', { type: 'terminal', folder: '' }]);
  await settle(page);
  assert.equal(fake.callsTo('FocusPane').length, 0);

  // Cancelling focuses the pane, as the click would have.
  fake.clearCalls();
  await page.click('.pane[data-pane-id="p3"] [data-action="add"]');
  await page.waitForSelector('.palette');
  await page.keyboard.press('Escape');
  assert.deepEqual((await fake.waitForCall('FocusPane')).args, ['p3']);
  assert.deepEqual(fake.calls.map((c) => c.method).filter((m) => m === 'AddOptions' || m === 'FocusPane'), ['AddOptions', 'FocusPane']);
  await page.waitForSelector('.pane[data-pane-id="p3"][data-focused="true"]');

  // The same for the ⋯ menu, in p4 now.
  fake.clearCalls();
  await page.click('.pane[data-pane-id="p4"] [data-action="menu"]');
  await page.waitForSelector('#window-menu');
  await page.keyboard.press('ArrowDown');
  await settle(page);
  assert.equal(fake.callsTo('FocusPane').length, 0);
  await page.keyboard.press('Escape');
  assert.deepEqual((await fake.waitForCall('FocusPane')).args, ['p4']);
  await page.waitForSelector('.pane[data-pane-id="p4"][data-focused="true"]');

  // Choosing a menu entry leaves the focusing to Go.
  fake.clearCalls();
  await page.click('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.click('#window-menu .menu-item:has-text("Pop out of Bench")');
  await fake.waitForCall('PopOut');
  await settle(page);
  assert.equal(fake.callsTo('FocusPane').length, 0);

  // A click in another pane while the menu is open does what that click
  // does: p4 is focused already, so nothing is focused.
  fake.clearCalls();
  await page.click('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.waitForSelector('#window-menu');
  await page.click('.pane[data-pane-id="p4"] .pane-body');
  await page.waitForSelector('#window-menu', { state: 'detached' });
  await settle(page);
  assert.deepEqual(fake.callsTo('FocusPane').map((c) => c.args), []);

  // Showing the window side by side focuses its new pane, in Go, so the
  // click doesn't focus the pane it was in as well: Go could handle that
  // after the move, and focus the pane left behind.
  fake.clearCalls();
  await page.click('.pane[data-pane-id="p3"] [data-action="split-right"]');
  await fake.waitForCall('MoveItem');
  await settle(page);
  assert.deepEqual(fake.calls.map((c) => c.method).filter((m) => m === 'FocusPane' || m === 'MoveItem'), ['MoveItem']);

  // The rest of the pane still focuses it.
  fake.clearCalls();
  await page.click('.pane[data-pane-id="p3"] .pane-body');
  assert.deepEqual((await fake.waitForCall('FocusPane')).args, ['p3']);
});

test('a split that Go refuses focuses the pane clicked in', async (t) => {
  const { page, fake, close } = await openBench({ setup: (f) => {
    f.on('MoveItem', () => { throw new Error('That\'s the only window in its pane.'); });
  } });
  t.after(close);
  fake.world.tabs[1].root.children[1].items = ['w5'];
  fake.world.tabs[1].root.children[1].active = 'w5';
  await fake.emitState();
  await page.waitForSelector('.pane[data-pane-id="p4"] .subtab:only-child');
  fake.clearCalls();
  await page.click('.pane[data-pane-id="p4"] [data-action="split-right"]');
  assert.deepEqual((await fake.waitForCall('FocusPane')).args, ['p4']);
  assert.deepEqual(fake.calls.map((c) => c.method).filter((m) => m === 'FocusPane' || m === 'MoveItem'), ['MoveItem', 'FocusPane']);
  await page.waitForFunction(() => document.getElementById('toast').textContent === 'That\'s the only window in its pane.');
});

test('opening + or ⋯ from the keyboard leaves the pane unfocused', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  fake.world.tabs[1].focus = 'p4';
  await fake.emitState();
  await page.waitForSelector('.pane[data-pane-id="p4"][data-focused="true"]');
  fake.clearCalls();
  await page.focus('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#window-menu');
  await page.keyboard.press('Escape');
  await page.waitForSelector('#window-menu', { state: 'detached' });
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'menu-p3');
  await settle(page);
  assert.equal(fake.callsTo('FocusPane').length, 0);
});

test('when another app takes the keyboard, an open menu closes, but a dialog stays', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  fake.world.tabs[1].focus = 'p4';
  await fake.emitState();
  await page.waitForSelector('.pane[data-pane-id="p4"][data-focused="true"]');
  await page.click('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.waitForSelector('#window-menu');
  await fake.waitForCall('SetOverlay', (a) => a[0] === true);
  fake.clearCalls();
  await page.evaluate(() => window.dispatchEvent(new FocusEvent('blur')));
  await page.waitForSelector('#window-menu', { state: 'detached' });
  await fake.waitForCall('SetOverlay', (a) => a[0] === false);
  // Closing it this way doesn't focus the pane, which would bring Bench
  // and Zed back in front of the other app.
  await settle(page);
  assert.equal(fake.callsTo('FocusPane').length, 0);

  await page.click('[data-action="open-palette"]');
  await page.waitForSelector('.palette');
  await page.evaluate(() => window.dispatchEvent(new FocusEvent('blur')));
  await settle(page);
  assert.ok(await page.$('.palette'), 'the palette stays open');
});

test('Tab in a menu closes it and moves on from its button', async (t) => {
  const { page, close } = await openBench();
  t.after(close);
  await page.focus('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#window-menu');
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'menu-item-0');
  await page.keyboard.press('Tab');
  await page.waitForSelector('#window-menu', { state: 'detached' });
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'sub-w5');
  await page.focus('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#window-menu');
  await page.keyboard.press('Shift+Tab');
  await page.waitForSelector('#window-menu', { state: 'detached' });
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'split-p3');
});

test('a menu or the add dialog closes when Go switches tabs', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.waitForSelector('#window-menu');
  await fake.waitForCall('SetOverlay', (a) => a[0] === true);
  // A global shortcut switches to Research.
  fake.world.current = 2;
  await fake.emitState();
  await page.waitForSelector('#window-menu', { state: 'detached' });
  await fake.waitForLowered();

  fake.world.current = 1;
  await fake.emitState();
  await page.waitForSelector('.tab[data-index="1"][aria-pressed="true"]');
  fake.clearCalls();
  await page.click('.pane[data-pane-id="p4"] [data-action="add"]');
  await page.waitForSelector('.palette');
  await fake.waitForCall('SetOverlay', (a) => a[0] === true);
  // Focus following switches to Comms.
  fake.world.current = 0;
  await fake.emitState();
  await page.waitForSelector('.palette', { state: 'detached' });
  await fake.waitForLowered();
  await settle(page);
  assert.equal(fake.callsTo('AddToPane').length, 0);
  assert.equal(fake.callsTo('FocusPane').length, 0);
});

test('a menu closes when its window leaves the pane', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.waitForSelector('#window-menu');
  const [p3, p4] = fake.world.tabs[1].root.children;
  p3.items = ['w4'];
  p3.active = 'w4';
  p4.items.push('w3');
  await fake.emitState();
  await page.waitForSelector('#window-menu', { state: 'detached' });
});

test('menus and dialogs raise Bench while any is open', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  // The first render says Bench isn't raised, in case a page before this
  // one left it raised.
  assert.deepEqual(fake.callsTo('SetOverlay').map((c) => c.args), [[false]]);
  fake.clearCalls();
  const overlays = () => fake.callsTo('SetOverlay').map((c) => c.args[0]);
  await page.click('[data-action="open-palette"]');
  await page.waitForSelector('.palette');
  await page.keyboard.press('Escape');
  // A tab's menu hands Bench on to the Tab properties it opens without
  // lowering it in between, which would rearrange the docked windows in
  // front of it.
  await openTabProperties(page);
  await page.click('[data-key="editor-cancel"]');
  await openSettings(page);
  await page.click('[data-key="settings-cancel"]');
  await page.click('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.click('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.waitForSelector('#window-menu', { state: 'detached' });
  await settle(page);
  assert.deepEqual(overlays(), [true, false, true, false, true, false, true, false]);
  // Escape restored focus to the palette's opener.
  await page.click('[data-action="open-palette"]');
  await page.keyboard.press('Escape');
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'open-palette');
});

test('a menu whose window closes goes away, and Bench is lowered again', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.waitForSelector('#window-menu');
  await fake.waitForCall('SetOverlay', (a) => a[0] === true);
  const p3 = fake.world.tabs[1].root.children[0];
  p3.items = ['w4'];
  p3.active = 'w4';
  delete fake.world.items.w3;
  await fake.emitState();
  await page.waitForSelector('#window-menu', { state: 'detached' });
  await fake.waitForLowered();
  // Focus was in the menu, so it goes back to the menu button.
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'menu-p3');
});

test('another control that closes the ⋯ menu stops it focusing its pane later', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ notices: ['⌃⌥Space is taken by another app.'] }) });
  t.after(close);
  fake.world.tabs[1].focus = 'p4';
  await fake.emitState();
  await page.waitForSelector('.pane[data-pane-id="p4"][data-focused="true"]');
  fake.clearCalls();
  // The notices replace Zed's menu: closing them doesn't focus Zed's pane.
  await page.click('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.waitForSelector('#window-menu');
  await page.click('[data-action="notices"]');
  await page.waitForSelector('#notices-menu');
  await page.keyboard.press('Escape');
  await page.waitForSelector('#notices-menu', { state: 'detached' });
  await settle(page);
  assert.equal(fake.callsTo('FocusPane').length, 0);
});

test('in the browser preview, a shortcut that closes the ⋯ menu doesn\'t focus its pane first', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ demo: true }) });
  t.after(close);
  fake.world.tabs[1].focus = 'p4';
  await fake.emitState();
  await page.waitForSelector('.pane[data-pane-id="p4"][data-focused="true"]');
  fake.clearCalls();
  await page.click('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.waitForSelector('#window-menu');
  // Focusing p3 as well would race the shortcut's own call in Go.
  await page.keyboard.press('Control+Alt+0');
  await fake.waitForCall('Rearrange');
  await page.waitForSelector('#window-menu', { state: 'detached' });
  await settle(page);
  assert.equal(fake.callsTo('FocusPane').length, 0);
});

test('right-clicking a tab opens its menu below it, without switching tabs', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  fake.clearCalls();
  // Code is the current tab; the menu is for Research.
  await page.click('.tab[data-index="2"]', { button: 'right' });
  await page.waitForSelector('#tab-menu');
  await fake.waitForCall('SetOverlay', (a) => a[0] === true);
  assert.equal(await page.getAttribute('#tab-menu', 'role'), 'menu');
  assert.equal(await page.getAttribute('#tab-menu', 'aria-label'), 'Options for the Research tab');
  const entries = await page.$$eval('#tab-menu > *', (els) => els.map((el) => el.className + ':' + el.textContent));
  assert.deepEqual(entries, ['menu-item:Tab properties…', 'menu-sep:', 'menu-item:New tab…']);
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'menu-item-0');
  // It lines up with the tab, just below it.
  const tab = await page.locator('.tab[data-index="2"]').boundingBox();
  const menu = await page.locator('#tab-menu').boundingBox();
  assert.equal(Math.round(menu.x), Math.round(tab.x));
  assert.ok(menu.y >= tab.y + tab.height && menu.y <= tab.y + tab.height + 8, 'below the tab: ' + JSON.stringify({ tab, menu }));
  await settle(page);
  assert.equal(fake.callsTo('SwitchTab').length, 0);
  assert.equal(await page.getAttribute('.tab[data-index="1"]', 'aria-pressed'), 'true');

  // Clicking outside closes it without running anything, and lowers Bench.
  await page.click('.statusbar');
  await page.waitForSelector('#tab-menu', { state: 'detached' });
  await fake.waitForLowered();
  await settle(page);
  assert.equal(await page.$('.overlay'), null);
  assert.deepEqual(fake.calls.map((c) => c.method).filter((m) => m !== 'SetOverlay'), []);
});

test('clicking Tab properties… in a tab\'s menu opens that tab\'s properties', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  const focused = () => page.evaluate(() => document.activeElement.dataset.key);
  // Research isn't the current tab.
  await page.click('.tab[data-index="2"]', { button: 'right' });
  await page.click('#tab-menu .menu-item:has-text("Tab properties…")');
  await page.waitForSelector('.editor-sheet');
  assert.equal(await page.$('#tab-menu'), null);
  assert.equal(await page.textContent('#editor-title'), 'Tab properties');
  assert.equal(await page.inputValue('#tab-name'), 'Research');
  assert.deepEqual((await fake.waitForCall('PreviewTree')).args, [2, 'side']);
  // Cancel gives the focus back to the tab.
  await page.click('[data-key="editor-cancel"]');
  await page.waitForSelector('.editor-sheet', { state: 'detached' });
  assert.equal(await focused(), 'tab-2');

  // Save sends that tab's index, and none of the settings for every tab.
  await openTabProperties(page, 2);
  await page.fill('#tab-name', 'Reading');
  await page.click('[data-action="save-tab"]');
  assert.deepEqual((await fake.waitForCall('SaveTab')).args, [2, { name: 'Reading', preset: 'side', smallScreens: 'tabs' }]);
  await page.waitForSelector('.editor-sheet', { state: 'detached' });
  assert.equal(await focused(), 'tab-2');
  await settle(page);
  assert.equal(fake.callsTo('SwitchTab').length, 0);
  assert.equal(fake.callsTo('SetOpenAtLogin').length, 0);
});

test('right-clicking another tab while a tab\'s menu is open opens that tab\'s menu instead', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('.tab[data-index="2"]', { button: 'right' });
  await page.waitForSelector('#tab-menu[aria-label="Options for the Research tab"]');
  await page.click('.tab[data-index="0"]', { button: 'right' });
  await page.waitForSelector('#tab-menu[aria-label="Options for the Comms tab"]');
  assert.equal(await page.locator('#menu-root .menu').count(), 1);
  const tab = await page.locator('.tab[data-index="0"]').boundingBox();
  const menu = await page.locator('#tab-menu').boundingBox();
  assert.equal(Math.round(menu.x), Math.round(tab.x));
  await page.click('#tab-menu .menu-item:has-text("Tab properties…")');
  await page.waitForSelector('.editor-sheet');
  assert.equal(await page.inputValue('#tab-name'), 'Comms');
  assert.deepEqual(fake.callsTo('PreviewTree').map((c) => c.args), [[0, 'side']]);
});

test('a tab\'s menu that replaces a window\'s menu stops that one focusing its pane or showing its window later', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  fake.world.tabs[1].focus = 'p4';
  await fake.emitState();
  await page.waitForSelector('.pane[data-pane-id="p4"][data-focused="true"]');
  const overlays = () => fake.callsTo('SetOverlay').map((c) => c.args[0]);
  fake.clearCalls();
  // Zed's ⋯ menu, in the unfocused pane.
  await page.click('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.waitForSelector('#window-menu');
  await page.click('.tab[data-index="2"]', { button: 'right' });
  await page.waitForSelector('#tab-menu');
  assert.equal(await page.$('#window-menu'), null);
  assert.equal(await page.getAttribute('.pane[data-pane-id="p3"] [data-action="menu"]', 'aria-expanded'), 'false');
  await page.keyboard.press('Escape');
  await page.waitForSelector('#tab-menu', { state: 'detached' });
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'tab-2');
  await settle(page);
  assert.equal(fake.callsTo('FocusPane').length, 0);
  // Bench stayed raised from one menu to the other.
  assert.deepEqual(overlays(), [true, false]);

  // A right-clicked sub-tab's menu, whose window shows when it closes.
  fake.clearCalls();
  await page.click('.subtab:has-text("index.html")', { button: 'right' });
  await page.waitForSelector('#window-menu');
  await page.click('.tab[data-index="0"]', { button: 'right' });
  await page.waitForSelector('#tab-menu');
  await page.keyboard.press('Escape');
  await page.waitForSelector('#tab-menu', { state: 'detached' });
  await settle(page);
  assert.deepEqual(fake.calls.map((c) => c.method).filter((m) => m === 'ShowItem' || m === 'FocusPane'), []);
});

test('a tab\'s menu follows its tab when a tab before it is deleted, and closes when its tab is', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('.tab[data-index="2"]', { button: 'right' });
  await page.waitForSelector('#tab-menu');
  // Comms is deleted elsewhere, such as in config.toml: Research is the
  // second tab now.
  fake.world.tabs.splice(0, 1);
  fake.world.current = 0;
  await fake.emitState();
  await page.waitForSelector('.tab[data-index="1"]:has-text("Research")');
  assert.equal(await page.getAttribute('#tab-menu', 'aria-label'), 'Options for the Research tab');
  await page.click('#tab-menu .menu-item:has-text("Tab properties…")');
  await page.waitForSelector('.editor-sheet');
  assert.equal(await page.inputValue('#tab-name'), 'Research');
  assert.deepEqual((await fake.waitForCall('PreviewTree')).args, [1, 'side']);
  await page.keyboard.press('Escape');
  await page.waitForSelector('.editor-sheet', { state: 'detached' });

  // Deleting Research closes its menu, and lowers Bench.
  await page.click('.tab[data-index="1"]', { button: 'right' });
  await page.waitForSelector('#tab-menu');
  await fake.waitForCall('SetOverlay', (a) => a[0] === true);
  fake.clearCalls();
  fake.world.tabs.splice(1, 1);
  await fake.emitState();
  await page.waitForSelector('#tab-menu', { state: 'detached' });
  await fake.waitForCall('SetOverlay', (a) => a[0] === false);
  await settle(page);
  assert.equal(await page.$('.overlay'), null);
  // Focus was in the menu, and its tab's button has gone, so it goes to
  // the current tab.
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'tab-0');
});

test('the title bar\'s gear opens Settings', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  // The button that edited the current tab's layout has gone: each tab's
  // properties are in its menu.
  assert.equal(await page.$('[data-action="edit-tab"]'), null);
  const gear = page.locator('#titlebar [data-action="open-settings"]');
  assert.equal(await gear.getAttribute('aria-label'), 'Settings');
  assert.equal(await gear.getAttribute('title'), 'Settings');
  fake.clearCalls();
  await gear.click();
  await page.waitForSelector('.settings-sheet');
  await fake.waitForCall('SetOverlay', (a) => a[0] === true);
  assert.equal(await page.textContent('#settings-title'), 'Settings');
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'setting-openAtLogin');
  // Escape closes it without saving, and gives the focus back to the gear.
  await page.keyboard.press('Escape');
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
  await fake.waitForLowered();
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'open-settings');
  assert.equal(fake.callsTo('SaveSettings').length, 0);
});
