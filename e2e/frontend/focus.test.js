'use strict';

// Keyboard focus and terminals: Go's focus-terminal command, a terminal
// letting go of the focus when its pane does, and dialogs giving the focus
// back to the terminal that had it.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, settle } = require('./helpers/harness.js');

// focusedTerminal returns the item ID of the terminal with the keyboard
// focus, or the focused element's tag.
const focusedTerminal = (page) => page.evaluate(() => {
  const el = document.activeElement;
  const host = el && el.closest('.term-host[data-term-host]');
  return host ? host.dataset.termHost : el.tagName;
});

test('focus-terminal gives a terminal the keyboard focus, waiting for it to be drawn', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.waitForSelector('.pane[data-pane-id="p4"] .xterm');
  // Go focuses p4, which shows w5: the command can come before the View.
  await fake.emit('bench:command', { name: 'focus-terminal', itemId: 'w5' });
  await settle(page);
  assert.equal(await focusedTerminal(page), 'BODY', 'p4 isn\'t the focused pane yet');
  fake.world.tabs[1].focus = 'p4';
  await fake.emitState();
  await page.waitForFunction(() => document.activeElement.closest('.term-host[data-term-host="w5"]'));
  // Typing goes to the terminal.
  await page.keyboard.type('ls');
  await fake.waitForCall('TermInput', (a) => a[0] === 'w5' && a[1].endsWith('s'));

  // Go cycles the sub-tab to w6, which hasn't been drawn yet.
  await fake.emit('bench:command', { name: 'focus-terminal', itemId: 'w6' });
  fake.world.tabs[1].root.children[1].active = 'w6';
  await fake.emitState();
  await page.waitForFunction(() => document.activeElement.closest('.term-host[data-term-host="w6"]'));
});

test('a terminal lets go of the focus when the focus moves to another pane or sub-tab', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('.pane[data-pane-id="p4"] .xterm');
  await fake.waitForCall('FocusPane');
  await page.waitForSelector('.pane[data-pane-id="p4"][data-focused="true"]');
  assert.equal(await focusedTerminal(page), 'w5');
  // A redraw that doesn't move the focus keeps it.
  fake.world.items.w3.title = 'bench — main.go';
  await fake.emitState();
  await page.waitForSelector('.subtab:has-text("bench — main.go")');
  assert.equal(await focusedTerminal(page), 'w5');

  // Control+Option+Left focuses Zed's pane.
  fake.world.tabs[1].focus = 'p3';
  await fake.emitState();
  await page.waitForSelector('.pane[data-pane-id="p3"][data-focused="true"]');
  assert.equal(await focusedTerminal(page), 'BODY');
  fake.clearCalls();
  await page.keyboard.type('x');
  await settle(page);
  assert.equal(fake.callsTo('TermInput').length, 0, 'typing doesn\'t reach the terminal');

  // Control+Option+Tab in p4 shows w6 instead of w5, which had the focus.
  await page.click('.pane[data-pane-id="p4"] .xterm');
  await page.waitForSelector('.pane[data-pane-id="p4"][data-focused="true"]');
  assert.equal(await focusedTerminal(page), 'w5');
  fake.world.tabs[1].root.children[1].active = 'w6';
  await fake.emitState();
  await page.waitForFunction(() => document.querySelector('.pane[data-pane-id="p4"] .term-host').dataset.termHost === 'w6');
  assert.equal(await focusedTerminal(page), 'BODY');
});

test('focus-terminal waits while a dialog has the focus', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  fake.world.tabs[1].focus = 'p4';
  await fake.emitState();
  await page.waitForSelector('.pane[data-pane-id="p4"][data-focused="true"] .xterm');
  await page.click('[data-action="open-palette"]');
  await page.waitForSelector('.palette');
  await fake.emit('bench:command', { name: 'focus-terminal', itemId: 'w5' });
  await settle(page);
  assert.equal(await page.evaluate(() => document.activeElement.id), 'palette-query');
});

test('closing the palette gives the focus back to the terminal that had it', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('.pane[data-pane-id="p4"] .xterm');
  await page.waitForSelector('.pane[data-pane-id="p4"][data-focused="true"]');
  assert.equal(await focusedTerminal(page), 'w5');
  // The jump shortcut, in the app.
  await fake.emit('bench:command', { name: 'toggle-palette' });
  await page.waitForSelector('.palette');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.palette', { state: 'detached' });
  assert.equal(await focusedTerminal(page), 'w5');
  fake.clearCalls();
  await page.keyboard.type('pwd');
  await fake.waitForCall('TermInput', (a) => a[0] === 'w5' && a[1].endsWith('d'));

  // The layout editor too.
  await fake.emit('bench:command', { name: 'edit-tab' });
  await page.waitForSelector('.editor-sheet');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.editor-sheet', { state: 'detached' });
  assert.equal(await focusedTerminal(page), 'w5');
});
