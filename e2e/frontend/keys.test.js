'use strict';

// Shortcut hints and the browser preview's keys follow the configured
// shortcuts (View.keys), not fixed text.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, openTabProperties, settle } = require('./helpers/harness.js');
const { defaultWorld, manyTabs } = require('./helpers/fixture.js');

// KEYS is a [shortcuts] config unlike the default one, in display form.
const KEYS = {
  tabs: '⌃⌘',
  nextTab: '⌃⌘→',
  previousTab: '⌃⌘←',
  lastTab: '⌥⌘L',
  jump: '⌃⌥J',
  showAllTabs: '⌃⌥O',
  rearrange: '',
  focusPane: '⌥⌘',
  moveWindow: '⌥⇧',
  sendToTab: '⌃⌥⌘',
  nextWindow: '⌃`',
  zoomPane: '⌥⌘↩',
  growPane: '⌃⌥=',
  shrinkPane: '⌃⌥-',
  equalPanes: '⌃⌥⇧=',
  nextLayout: '⌃⌥L',
  previousLayout: '⌃⌥⇧L',
  addFrontWindow: '⌃⌥A',
};

test('the hints show the configured shortcuts', async (t) => {
  const { page, close } = await openBench({ world: manyTabs(defaultWorld({ keys: KEYS }), 10) });
  t.after(close);

  assert.equal(await page.textContent('[data-action="open-palette"] kbd'), '⌃⌥J');
  assert.equal(await page.textContent('#statusbar .status-keys'), '⌃⌘1–9 tabs · ⌃⌥J jump · ⌥⇧ arrows move window');
  assert.equal(await page.textContent('.tab[data-index="1"] kbd'), '⌃⌘2');

  // The ⋯ menu's splits use the move modifiers.
  await page.click('[data-action="menu"][data-pane="p3"]');
  const keys = await page.locator('#window-menu .menu-item').evaluateAll((els) => els
    .filter((el) => el.textContent.startsWith('Split') && !el.textContent.includes('Empty')).map((el) => el.textContent));
  assert.deepEqual(keys, ['Split Right⌥⇧→', 'Split Down⌥⇧↓']);
  // Its Move to Tab entries use the send modifiers, for the first nine
  // tabs only.
  await page.click('#window-menu .menu-item:has-text("Move to Tab")');
  await page.waitForSelector('#menu-sub');
  const sends = await page.locator('#menu-sub .menu-item').evaluateAll((els) => els
    .filter((el) => ['Comms', 'Research', 'Tab 10'].includes((el.querySelector('.menu-label') || {}).textContent)).map((el) => el.textContent));
  assert.deepEqual(sends, ['Comms⌃⌥⌘1', 'Research⌃⌥⌘3', 'Tab 10']);
  await page.keyboard.press('Escape');
  await page.keyboard.press('Escape');

  // Rearrange has no shortcut, so the palette shows none for it.
  await page.click('[data-action="open-palette"]');
  await page.fill('#palette-query', 'rearrange');
  assert.equal(await page.textContent('.palette-item .palette-keys'), '↵');
  await page.keyboard.press('Escape');

  // The tenth tab has no shortcut; its properties name the next and
  // previous tab shortcuts instead.
  await openTabProperties(page, 9);
  assert.equal(await page.$eval('#tab-place', (el) => el.selectedOptions[0].textContent), '10th');
  assert.equal(await page.textContent('#tab-place-note'), 'Only the first nine tabs have shortcuts. Use ⌃⌘← → or Jump to… for this one.');
  assert.equal(await page.textContent('#tab-place option[value="9"]'), '9th (⌃⌘9)');
});

test('shortcuts that are turned off have no hints', async (t) => {
  const off = { ...KEYS, tabs: '', jump: '', moveWindow: '', sendToTab: '', nextTab: '', previousTab: '', lastTab: '', zoomPane: '' };
  const { page, close } = await openBench({ world: defaultWorld({ keys: off }) });
  t.after(close);
  assert.equal(await page.locator('[data-action="open-palette"] kbd').count(), 0);
  assert.equal(await page.locator('#statusbar .status-keys').count(), 0);
  assert.equal(await page.locator('.tab kbd').count(), 0);
  await page.click('[data-action="menu"][data-pane="p3"]');
  assert.equal(await page.locator('#window-menu .menu-item kbd').count(), 0);
  await page.keyboard.press('Escape');
  await page.click('[data-action="new-tab"]');
  assert.equal(await page.$eval('#tab-place', (el) => el.selectedOptions[0].textContent), '4th');
  assert.equal(await page.textContent('#tab-place-note'), 'Tabs have no shortcuts. Settings ▸ Keyboard can give them some.');
  assert.equal(await page.$('#tab-shortcut-text'), null);
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
    ['Alt+Meta+KeyL', 'LastTab', []],
    ['Alt+Meta+ArrowUp', 'FocusDirection', ['up']],
    ['Alt+Shift+ArrowLeft', 'MoveDirection', ['left']],
    ['Control+Backquote', 'CycleSubtab', [1]],
    ['Control+Shift+Backquote', 'CycleSubtab', [-1]],
    ['Alt+Meta+Enter', 'ZoomPane', ['']],
    ['Control+Alt+Equal', 'GrowPane', []],
    ['Control+Alt+Minus', 'ShrinkPane', []],
    ['Control+Alt+Shift+Equal', 'EqualPanes', []],
    ['Control+Alt+KeyL', 'StepLayout', [1]],
    ['Control+Alt+Shift+KeyL', 'StepLayout', [-1]],
    ['Control+Alt+KeyA', 'AddFrontWindow', []],
    ['Control+Alt+Meta+Digit3', 'SendToTab', [2]],
    ['Control+Alt+Meta+Backquote', 'SendToLastTab', []],
  ];
  for (const [keys, method, args] of cases) {
    fake.clearCalls();
    await page.keyboard.press(keys);
    const call = await fake.waitForCall(method);
    assert.deepEqual(call.args, args, keys);
  }
  // The default shortcuts do nothing now.
  fake.clearCalls();
  for (const keys of ['Control+Alt+Digit1', 'Control+Alt+Digit0', 'Control+Alt+Shift+ArrowLeft', 'Control+Alt+Space', 'Control+Alt+Enter',
    'Control+Alt+Backquote', 'Control+Alt+Shift+Digit2', 'Control+Alt+Shift+Backquote']) {
    await page.keyboard.press(keys);
  }
  await settle(page);
  assert.deepEqual(fake.calls.filter((c) => !['SetOverlay', 'SetPaneArea'].includes(c.method)).map((c) => c.method), []);
  assert.equal(await page.$('.palette'), null);
  await page.keyboard.press('Control+Alt+KeyJ');
  await page.waitForSelector('.palette');
});
