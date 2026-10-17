'use strict';

// Clicking: tabs, sub-tabs, panes, the split button, the ⋯ menu, and toasts
// for refused calls.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, settle } = require('./helpers/harness.js');

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

test('right-clicking a sub-tab shows it and opens its menu', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('.subtab:has-text("index.html")', { button: 'right' });
  const show = await fake.waitForCall('ShowItem');
  assert.deepEqual(show.args, ['p3', 'w4']);
  await page.waitForSelector('#window-menu');
  await page.click('#window-menu .menu-item:has-text("Split down")');
  const move = await fake.waitForCall('MoveItem');
  assert.deepEqual(move.args, ['w4', 1, 'p3', 'bottom']);
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
