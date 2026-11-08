'use strict';

// Keyboard shortcuts: handled by the UI in the browser preview, and by Go
// (through bench:command) in the app; the keys in a tab's menu; and what
// the keyboard's focus looks like on tabs, sub-tabs, and layout buttons.

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
  // In the browser preview, whose demo bar comes before the title bar.
  const { page, fake, close } = await openBench({ world: defaultWorld({ demo: true }) });
  t.after(close);
  const focused = () => page.evaluate(() => document.activeElement.dataset.key);
  const open = async (i) => {
    await page.click('.tab[data-index="' + i + '"]', { button: 'right' });
    await page.waitForSelector('#tab-menu');
  };

  // The first entry has the focus. The arrows move it, round from either
  // end, and Home and End go to the ends. There are eight entries that
  // work: the last tab's Move right doesn't, and the keys pass over it.
  await open(2);
  assert.equal(await focused(), 'menu-item-0');
  for (const [key, want] of [['ArrowDown', 1], ['ArrowUp', 0], ['ArrowUp', 7], ['ArrowDown', 0], ['End', 7], ['Home', 0]]) {
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

  // Tab closes it and moves on from the tabs, which are one stop for the
  // Tab key, and Shift+Tab comes back to the current tab. Shift+Tab from
  // the menu closes it too, and moves back from the tabs, to the last
  // button of the demo bar, Simulate, whose list is hidden.
  await open(2);
  await page.keyboard.press('Tab');
  await page.waitForSelector('#tab-menu', { state: 'detached' });
  assert.equal(await focused(), 'new-tab');
  await page.keyboard.press('Shift+Tab');
  assert.equal(await focused(), 'tab-1');
  await open(2);
  await page.keyboard.press('Shift+Tab');
  await page.waitForSelector('#tab-menu', { state: 'detached' });
  assert.equal(await page.evaluate(() => document.activeElement.id), 'demo-simulate');
  // None of it switched tabs.
  assert.equal(fake.callsTo('SwitchTab').length, 0);
});

test('with the keyboard focus, tabs, sub-tabs, and layout buttons have their whole ring inside, and a tab and a sub-tab show their ×', async (t) => {
  // VS Code's window is missing, so its sub-tab has a marker on its tile.
  const world = defaultWorld();
  world.items.w4.status = 'missing';
  // On a dark Mac, as the ring's colour below is the dark set's accent.
  const { page, close } = await openBench({ world, contextOptions: { colorScheme: 'dark' } });
  t.after(close);
  // ring describes the focus: what has it, whether it shows the keyboard's
  // ring, whether the ring is inside the element's edges, and the element
  // inside its strip, which cuts off what's outside it, and whether its ×
  // and shortcut show. A sub-tab's ring is a layer over it (::after), an
  // inset shadow, rather than its outline.
  const ring = () => page.evaluate(() => {
    const el = document.activeElement;
    const s = getComputedStyle(el);
    const layer = getComputedStyle(el, '::after');
    const strip = el.closest('.tabs, .subtabs');
    const r = el.getBoundingClientRect();
    const n = strip ? strip.getBoundingClientRect() : r;
    const shows = (sel) => {
      const part = el.querySelector(sel);
      return part ? getComputedStyle(part).visibility === 'visible' : null;
    };
    const outline = s.outlineStyle === 'solid' && parseFloat(s.outlineWidth) >= 2;
    const shadow = s.outlineStyle === 'none' && layer.position === 'absolute' && layer.inset === '0px'
      && / 0px 0px 0px 2px inset$/.test(layer.boxShadow);
    return {
      on: el.dataset.index || el.dataset.item || el.dataset.mode,
      ring: el.matches(':focus-visible') && (outline || shadow),
      inside: (shadow || parseFloat(s.outlineOffset) + parseFloat(s.outlineWidth) <= 0)
        && r.top >= n.top - 0.5 && r.bottom <= n.bottom + 0.5 && r.left >= n.left - 0.5 && r.right <= n.right + 0.5,
      cross: shows('.close-cross'),
      kbd: shows('kbd'),
    };
  });
  // Right Arrow from the current tab to the next, Research, which shows
  // its × in place of its shortcut while it has the focus.
  await page.focus('.tab[data-index="1"]');
  await page.keyboard.press('ArrowRight');
  assert.deepEqual(await ring(), { on: '2', ring: true, inside: true, cross: true, kbd: false });
  // From the shown sub-tab to the next, whose × shows too.
  await page.focus('.subtab[data-item="w3"]');
  await page.keyboard.press('ArrowRight');
  assert.deepEqual(await ring(), { on: 'w4', ring: true, inside: true, cross: true, kbd: null });
  assert.equal(await page.$eval('.subtab[data-item="w4"] .close-cross', (el) => el.getAttribute('aria-hidden')), 'true');
  // The sub-tab's marker sits on its tile's corner, at the sub-tab's top
  // edge, cut out of what's under it. The focus ring is over the marker:
  // the top edge, beside the marker's tip, is the accent colour.
  const mark = await page.locator('.subtab[data-item="w4"] .status-mark').boundingBox();
  const top = (await page.locator('.subtab[data-item="w4"]').boundingBox()).y;
  const png = await page.screenshot({ clip: { x: Math.floor(mark.x + mark.width / 2) - 3, y: Math.floor(top) + 1, width: 1, height: 1 } });
  const pixel = await page.evaluate(async (data) => {
    const img = new Image();
    img.src = 'data:image/png;base64,' + data;
    await img.decode();
    const canvas = document.createElement('canvas');
    canvas.width = 1;
    canvas.height = 1;
    const g = canvas.getContext('2d');
    g.drawImage(img, 0, 0);
    return Array.from(g.getImageData(0, 0, 1, 1).data.slice(0, 3));
  }, png.toString('base64'));
  const accent = [0xF2, 0xA3, 0x3A];
  assert.ok(pixel.every((c, i) => Math.abs(c - accent[i]) <= 8), 'the ring over the marker is ' + pixel);
  // A layout button's ring is inside it too. Right Arrow moves the focus
  // to the next.
  await page.focus('.layout-seg [data-mode="auto"]');
  await page.keyboard.press('ArrowRight');
  assert.deepEqual(await ring(), { on: 'wide', ring: true, inside: true, cross: null, kbd: null });
  // The focus moved on, the × goes again, and the shortcut comes back.
  await page.focus('.tab[data-index="1"]');
  assert.deepEqual(await page.$eval('.tab[data-index="2"]', (el) => [
    getComputedStyle(el.querySelector('.close-cross')).visibility, getComputedStyle(el.querySelector('kbd')).visibility,
  ]), ['hidden', 'visible']);
});
