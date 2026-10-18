'use strict';

// Keyboard shortcuts: handled by the UI in the browser preview, and by Go
// (through bench:command) in the app.

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
  await page.waitForSelector('#editor-title:text("Edit tab")');
  assert.equal(await page.inputValue('#tab-name'), 'Code');
  // Dialog keys work in the app.
  await page.keyboard.press('Escape');
  await page.waitForSelector('.editor-sheet', { state: 'detached' });
});
