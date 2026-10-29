'use strict';

// Keyboard shortcuts: handled by the UI in the browser preview, and by Go
// (through bench:command) in the app; and the keys in a tab's menu.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, settle } = require('./helpers/harness.js');
const { defaultWorld } = require('./helpers/fixture.js');

test('keyboard shortcuts in the browser preview', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ demo: true }) });
  t.after(close);
  await page.click('.statusbar');
  const cases = [
    ['Control+Alt+Digit1', 'SwitchTab', [0]],
    ['Control+Alt+Digit3', 'SwitchTab', [2]],
    ['Control+Alt+BracketRight', 'StepTab', [1]],
    ['Control+Alt+BracketLeft', 'StepTab', [-1]],
    ['Control+Alt+Digit0', 'Rearrange', []],
    ['Control+Alt+ArrowRight', 'FocusDirection', ['right']],
    ['Control+Alt+ArrowUp', 'FocusDirection', ['up']],
    ['Control+Alt+Shift+ArrowDown', 'MoveDirection', ['down']],
    ['Control+Alt+Shift+ArrowLeft', 'MoveDirection', ['left']],
    ['Control+Alt+Tab', 'CycleSubtab', [1]],
    ['Control+Alt+Shift+Tab', 'CycleSubtab', [-1]],
  ];
  for (const [keys, method, args] of cases) {
    fake.clearCalls();
    await page.keyboard.press(keys);
    const call = await fake.waitForCall(method);
    assert.deepEqual(call.args, args, keys);
  }

  await page.keyboard.press('Control+Alt+Space');
  await page.waitForSelector('.palette');
  // Shortcuts that act on panes wait while a dialog is open.
  fake.clearCalls();
  await page.keyboard.press('Control+Alt+Digit2');
  await settle(page);
  assert.equal(fake.callsTo('SwitchTab').length, 0);
  await page.keyboard.press('Control+KeyK');
  await page.waitForSelector('.palette', { state: 'detached' });
  await page.keyboard.press('Control+KeyK');
  await page.waitForSelector('.palette');
});

test('shortcuts reach Bench before a focused terminal, except Control+K', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ demo: true, mode: 'wide' }) });
  t.after(close);
  await page.click('.pane[data-pane-id="p4"] .xterm');
  await page.waitForFunction(() => document.activeElement && document.activeElement.closest('.term-host'));
  fake.clearCalls();
  await page.keyboard.press('Control+Alt+Digit1');
  assert.deepEqual((await fake.waitForCall('SwitchTab')).args, [0]);
  assert.equal(fake.callsTo('TermInput').length, 0);
  fake.world.current = 1;
  await fake.emitState();
  await page.click('.pane[data-pane-id="p4"] .xterm');
  fake.clearCalls();
  await page.keyboard.press('Control+KeyK');
  assert.deepEqual((await fake.waitForCall('TermInput')).args, ['w5', '\x0b']);
  assert.equal(await page.$('.palette'), null);
});

test('in the app, Go handles shortcuts and sends commands', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('.statusbar');
  await page.keyboard.press('Control+Alt+Digit1');
  await page.keyboard.press('Control+KeyK');
  await settle(page);
  assert.equal(fake.callsTo('SwitchTab').length, 0);
  assert.equal(await page.$('.palette'), null);

  await fake.emit('bench:command', { name: 'toggle-palette' });
  await page.waitForSelector('.palette');
  await fake.emit('bench:command', { name: 'toggle-palette' });
  await page.waitForSelector('.palette', { state: 'detached' });
  await fake.emit('bench:command', { name: 'open-palette' });
  await fake.emit('bench:command', { name: 'open-palette' });
  await page.waitForSelector('.palette');
  await fake.emit('bench:command', { name: 'new-tab' });
  await page.waitForSelector('#editor-title:text("New tab")');
  await fake.emit('bench:command', { name: 'edit-tab' });
  await page.waitForSelector('#editor-title:text("Tab properties")');
  assert.equal(await page.inputValue('#tab-name'), 'Code');
  await fake.emit('bench:command', { name: 'settings' });
  await page.waitForSelector('#settings-title:text("Settings")');
  assert.equal(await page.$('.editor-sheet'), null);
  // Dialog keys work in the app.
  await page.keyboard.press('Escape');
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
});

test('a tab\'s menu works from the keyboard', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  const focused = () => page.evaluate(() => document.activeElement.dataset.key);
  const open = async (i) => {
    await page.click('.tab[data-index="' + i + '"]', { button: 'right' });
    await page.waitForSelector('#tab-menu');
  };

  // The first entry has the focus. The arrows move it, round from either
  // end, and Home and End go to the ends. There are seven entries.
  await open(2);
  assert.equal(await focused(), 'menu-item-0');
  for (const [key, want] of [['ArrowDown', 1], ['ArrowUp', 0], ['ArrowUp', 6], ['ArrowDown', 0], ['End', 6], ['Home', 0]]) {
    await page.keyboard.press(key);
    assert.equal(await focused(), 'menu-item-' + want, key);
  }
  // Escape closes it and focuses the tab.
  await page.keyboard.press('Escape');
  await page.waitForSelector('#tab-menu', { state: 'detached' });
  assert.equal(await focused(), 'tab-2');

  // Return runs the focused entry. Tab properties are for the tab whose
  // menu it was, and give the focus back to it.
  await open(2);
  await page.keyboard.press('Enter');
  await page.waitForSelector('#editor-title:text("Tab properties")');
  assert.equal(await page.inputValue('#tab-name'), 'Research');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.editor-sheet', { state: 'detached' });
  assert.equal(await focused(), 'tab-2');
  // New tab… is the last entry.
  await open(0);
  await page.keyboard.press('End');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#editor-title:text("New tab")');
  assert.deepEqual((await fake.waitForCall('PreviewTree', (a) => a[0] === -1)).args, [-1, 'single']);
  await page.keyboard.press('Escape');
  await page.waitForSelector('.editor-sheet', { state: 'detached' });
  assert.equal(await focused(), 'tab-0');

  // Tab closes it and moves on from the tab, and Shift+Tab moves back.
  await open(2);
  await page.keyboard.press('Tab');
  await page.waitForSelector('#tab-menu', { state: 'detached' });
  assert.equal(await focused(), 'new-tab');
  await open(2);
  await page.keyboard.press('Shift+Tab');
  await page.waitForSelector('#tab-menu', { state: 'detached' });
  assert.equal(await focused(), 'tab-1');
  // None of it switched tabs.
  assert.equal(fake.callsTo('SwitchTab').length, 0);
});
