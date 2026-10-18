'use strict';

// Clicking: tabs, sub-tabs, panes, the split button, the ⋯ menu, and toasts
// for refused calls.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, settle } = require('./helpers/harness.js');
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

  // Other controls, and the rest of the pane, still focus it first.
  fake.clearCalls();
  await page.click('.pane[data-pane-id="p3"] [data-action="split-right"]');
  await fake.waitForCall('MoveItem');
  assert.deepEqual(fake.calls.map((c) => c.method).filter((m) => m === 'FocusPane' || m === 'MoveItem'), ['FocusPane', 'MoveItem']);
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
  await page.click('[data-action="edit-tab"]');
  await page.waitForSelector('.editor-sheet');
  await page.click('[data-key="editor-cancel"]');
  await page.click('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.click('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.waitForSelector('#window-menu', { state: 'detached' });
  await settle(page);
  assert.deepEqual(overlays(), [true, false, true, false, true, false]);
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
