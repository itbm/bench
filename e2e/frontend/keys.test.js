'use strict';

// Shortcut hints and the browser preview's keys follow the configured
// shortcuts (View.keys), not fixed text.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, settle } = require('./helpers/harness.js');
const { defaultWorld, manyTabs } = require('./helpers/fixture.js');

// KEYS is a [shortcuts] config unlike the default one, in display form.
const KEYS = {
  tabs: '⌃⌘',
  nextTab: '⌃⌘→',
  previousTab: '⌃⌘←',
  jump: '⌃⌥J',
  rearrange: '',
  focusPane: '⌥⌘',
  moveWindow: '⌥⇧',
  nextWindow: '⌃`',
};

test('the hints show the configured shortcuts', async (t) => {
  const { page, close } = await openBench({ world: manyTabs(defaultWorld({ keys: KEYS }), 10) });
  t.after(close);

  assert.equal(await page.textContent('[data-action="open-palette"] kbd'), '⌃⌥J');
  assert.equal(await page.textContent('#statusbar .status-keys'), '⌃⌘1–9 tabs · ⌃⌥J jump · ⌥⇧ arrows move window');
  assert.equal(await page.textContent('.tab[data-index="1"] kbd'), '⌃⌘2');

  // The ⋯ menu's splits use the move modifiers.
  await page.click('[data-action="menu"][data-pane="p3"]');
  const keys = await page.locator('#window-menu .menu-item').evaluateAll((els) => els.slice(0, 2).map((el) => el.textContent));
  assert.deepEqual(keys, ['Show side by side (split right)⌥⇧→', 'Split down⌥⇧↓']);
  await page.keyboard.press('Escape');

  // Rearrange has no shortcut, so the palette shows none for it.
  await page.click('[data-action="open-palette"]');
  await page.fill('#palette-query', 'rearrange');
  assert.equal(await page.textContent('.palette-item .palette-keys'), '↵');
  await page.keyboard.press('Escape');

  // The tenth tab has no shortcut; the editor names the next and previous
  // tab shortcuts instead.
  await page.click('.tab[data-index="9"]');
  await page.waitForFunction(() => window.bench.store.view.current === 9);
  await page.click('[data-action="edit-tab"]');
  assert.equal(await page.textContent('#shortcut-value'), 'None (use ⌃⌘← → or Jump to…)');
  await page.click('[data-action="shortcut"]');
  await page.waitForFunction(() => document.getElementById('toast').textContent
    === 'Shortcuts follow tab order: the first nine tabs get ⌃⌘1 to ⌃⌘9.');
});

test('shortcuts that are turned off have no hints', async (t) => {
  const off = { ...KEYS, tabs: '', jump: '', moveWindow: '', nextTab: '', previousTab: '' };
  const { page, close } = await openBench({ world: defaultWorld({ keys: off }) });
  t.after(close);
  assert.equal(await page.locator('[data-action="open-palette"] kbd').count(), 0);
  assert.equal(await page.locator('#statusbar .status-keys').count(), 0);
  assert.equal(await page.locator('.tab kbd').count(), 0);
  await page.click('[data-action="menu"][data-pane="p3"]');
  assert.equal(await page.locator('#window-menu .menu-item kbd').count(), 0);
  await page.keyboard.press('Escape');
  await page.click('[data-action="new-tab"]');
  assert.equal(await page.textContent('#shortcut-value'), 'None (use Jump to…)');
  await page.click('[data-action="shortcut"]');
  await page.waitForFunction(() => document.getElementById('toast').textContent
    === 'Tabs have no shortcuts, because tab_modifiers is empty in config.toml.');
});

test('the browser preview handles the configured shortcuts', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ demo: true, keys: KEYS }) });
  t.after(close);
  assert.match(await page.textContent('#demo-hint'), /⌃⌥J or ⌃K jump · ⌥⇧ arrows move the window$/);
  await page.click('.statusbar');
  const cases = [
    ['Control+Meta+Digit3', 'SwitchTab', [2]],
    ['Control+Meta+ArrowRight', 'StepTab', [1]],
    ['Control+Meta+ArrowLeft', 'StepTab', [-1]],
    ['Alt+Meta+ArrowUp', 'FocusDirection', ['up']],
    ['Alt+Shift+ArrowLeft', 'MoveDirection', ['left']],
    ['Control+Backquote', 'CycleSubtab', [1]],
    ['Control+Shift+Backquote', 'CycleSubtab', [-1]],
  ];
  for (const [keys, method, args] of cases) {
    fake.clearCalls();
    await page.keyboard.press(keys);
    const call = await fake.waitForCall(method);
    assert.deepEqual(call.args, args, keys);
  }
  // The default shortcuts do nothing now.
  fake.clearCalls();
  for (const keys of ['Control+Alt+Digit1', 'Control+Alt+Digit0', 'Control+Alt+Shift+ArrowLeft', 'Control+Alt+Space']) {
    await page.keyboard.press(keys);
  }
  await settle(page);
  assert.deepEqual(fake.calls.filter((c) => !['SetOverlay', 'SetPaneArea'].includes(c.method)).map((c) => c.method), []);
  assert.equal(await page.$('.palette'), null);
  await page.keyboard.press('Control+Alt+KeyJ');
  await page.waitForSelector('.palette');
});
