'use strict';

// A terminal's tools: its find bar (⌘F), links that programs print as plain
// addresses, its text size (⌘+, ⌘−, and ⌘0, and Settings), Option as Meta,
// and its right-click menu, in the main window and in a popped-out
// terminal's own window.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, openTerminalWindow, settle } = require('./helpers/harness.js');

const TERM = '.pane[data-pane-id="p4"] .term-host[data-term-host="w5"]';

// inputTo returns everything sent to a terminal with TermInput, in order.
function inputTo(fake, id) {
  return fake.callsTo('TermInput').filter((c) => c.args[0] === id).map((c) => c.args[1]).join('');
}

// ready waits until w5's terminal has drawn its snapshot and told Go its
// size, and gives it the keyboard.
async function ready(page, fake, root = TERM) {
  await page.waitForFunction((sel) => {
    const rows = document.querySelector(sel + ' .xterm-rows');
    return Boolean(rows && rows.textContent.includes('go run ./cmd/bench'));
  }, root);
  await fake.waitForCall('TermResize');
  await page.click(root + ' .xterm');
  await settle(page);
}

// print writes lines to w5's terminal, and waits until the last is drawn.
async function print(page, fake, lines, root = TERM) {
  await fake.termOut('w5', '\r\n' + lines.join('\r\n') + '\r\n');
  const last = lines[lines.length - 1];
  await page.waitForFunction(([sel, t]) => document.querySelector(sel + ' .xterm-rows').textContent.includes(t), [root, last]);
}

const options = (page, id = 'w5') => page.evaluate((i) => window.bench.terminalOptions(i), id);
const active = (page) => page.evaluate(() => document.activeElement.className);
const terminalHasKeyboard = (page, id = 'w5') => page.evaluate((i) => {
  const el = document.activeElement;
  return el.classList.contains('xterm-helper-textarea') && el.closest('.term-host').dataset.termHost === i;
}, id);

test('⌘F in a terminal opens its find bar, which finds as it\'s typed in', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await ready(page, fake);
  await print(page, fake, ['alpha one', 'beta', 'alpha two', 'alpha three']);
  const bar = page.locator(TERM + ' .term-find');
  assert.equal(await bar.count(), 0);

  await page.keyboard.press('Meta+f');
  await bar.waitFor({ state: 'visible' });
  assert.equal(await active(page), 'term-find-input');
  assert.equal(await bar.getAttribute('role'), 'search');
  assert.equal(await page.getAttribute(TERM + ' .term-find-input', 'aria-label'), 'Find in terminal');
  assert.deepEqual(await bar.locator('button').evaluateAll((els) => els.map((el) => el.getAttribute('aria-label'))),
    ['Previous match (⇧↩)', 'Next match (↩)', 'Close (⎋)']);

  // Typing finds, marks every match, and counts them.
  await page.keyboard.type('alpha');
  const count = bar.locator('.term-find-count');
  await page.waitForFunction((sel) => /^\d of 3$/.test(document.querySelector(sel).textContent), TERM + ' .term-find-count');
  const first = Number((await count.textContent())[0]);
  // The selected match has an outline of its own (#F2A33A).
  await page.waitForFunction((sel) => {
    const marks = Array.from(document.querySelectorAll(sel + ' .xterm-find-result-decoration'));
    return marks.length >= 3 && marks.filter((el) => el.style.outline.includes('rgb(242, 163, 58)')).length === 1;
  }, TERM);

  // Return finds the next match, and Shift-Return the previous one, in
  // turn; so do the bar's buttons, and ⌘G and ⇧⌘G.
  const after = (n) => (n % 3) + 1;
  await page.keyboard.press('Enter');
  await page.waitForFunction(([sel, t]) => document.querySelector(sel).textContent === t, [TERM + ' .term-find-count', after(first) + ' of 3']);
  await page.keyboard.press('Shift+Enter');
  await page.waitForFunction(([sel, t]) => document.querySelector(sel).textContent === t, [TERM + ' .term-find-count', first + ' of 3']);
  await bar.locator('[data-find="next"]').click();
  await page.waitForFunction(([sel, t]) => document.querySelector(sel).textContent === t, [TERM + ' .term-find-count', after(first) + ' of 3']);
  await bar.locator('[data-find="previous"]').click();
  await page.waitForFunction(([sel, t]) => document.querySelector(sel).textContent === t, [TERM + ' .term-find-count', first + ' of 3']);
  await page.focus(TERM + ' .term-find-input');
  await page.keyboard.press('Meta+g');
  await page.waitForFunction(([sel, t]) => document.querySelector(sel).textContent === t, [TERM + ' .term-find-count', after(first) + ' of 3']);

  // Text that isn't there says so.
  await page.fill(TERM + ' .term-find-input', 'zebra');
  await page.waitForFunction((sel) => document.querySelector(sel).textContent === 'No matches', TERM + ' .term-find-count');
  assert.equal(await count.evaluate((el) => el.classList.contains('none')), true);
  assert.equal(await page.locator(TERM + ' .xterm-find-result-decoration').count(), 0);
  await page.fill(TERM + ' .term-find-input', 'beta');
  await page.waitForFunction((sel) => document.querySelector(sel).textContent === '1 of 1', TERM + ' .term-find-count');

  // Escape closes it, takes the marks away, and gives the terminal the
  // keyboard back. Nothing typed in the bar reached the program.
  await page.keyboard.press('Escape');
  await bar.waitFor({ state: 'hidden' });
  assert.equal(await terminalHasKeyboard(page), true);
  await page.waitForFunction((sel) => document.querySelectorAll(sel + ' .xterm-find-result-decoration').length === 0, TERM);
  assert.equal(inputTo(fake, 'w5'), '');

  // ⌘F opens it again with what it found selected, so that typing
  // replaces it.
  await page.keyboard.press('Meta+f');
  await bar.waitFor({ state: 'visible' });
  assert.equal(await page.inputValue(TERM + ' .term-find-input'), 'beta');
  assert.deepEqual(await page.$eval(TERM + ' .term-find-input', (el) => [el.selectionStart, el.selectionEnd]), [0, 4]);
  // The bar's close button closes it too.
  await bar.locator('[data-find="close"]').click();
  await bar.waitFor({ state: 'hidden' });
  assert.equal(await terminalHasKeyboard(page), true);
  assert.equal(inputTo(fake, 'w5'), '');
});

test('a terminal\'s find bar keeps the keyboard across a redraw', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await ready(page, fake);
  await page.keyboard.press('Meta+f');
  await page.keyboard.type('go');
  fake.world.items.w3.title = 'bench — layout.go';
  await fake.emitState();
  await page.waitForSelector('.subtab:has-text("bench — layout.go")');
  await settle(page);
  assert.equal(await active(page), 'term-find-input');
  await page.keyboard.type(' run');
  assert.equal(await page.inputValue(TERM + ' .term-find-input'), 'go run');
  assert.equal(inputTo(fake, 'w5'), '');
});

test('⌘+, ⌘−, and ⌘0 change a terminal\'s text size, and tell Go its new size', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await ready(page, fake);
  assert.equal((await options(page)).fontSize, 12.5);
  const start = fake.callsTo('TermResize').at(-1).args;
  fake.clearCalls();

  await page.keyboard.press('Meta+Equal');
  assert.equal((await options(page)).fontSize, 13.5);
  const bigger = await fake.waitForCall('TermResize');
  assert.equal(bigger.args[0], 'w5');
  assert.ok(bigger.args[1] <= start[1] && bigger.args[2] < start[2], JSON.stringify({ start, bigger: bigger.args }));

  fake.clearCalls();
  await page.keyboard.press('Meta+Minus');
  await page.keyboard.press('Meta+Minus');
  assert.equal((await options(page)).fontSize, 11.5);
  await fake.waitForCall('TermResize', (a) => a[2] > start[2]);

  fake.clearCalls();
  await page.keyboard.press('Meta+Digit0');
  assert.equal((await options(page)).fontSize, 12.5);
  assert.deepEqual((await fake.waitForCall('TermResize')).args, start);

  // It goes no bigger than 24 points, and no smaller than 9.
  for (let i = 0; i < 15; i++) await page.keyboard.press('Meta+Equal');
  assert.equal((await options(page)).fontSize, 24);
  for (let i = 0; i < 20; i++) await page.keyboard.press('Meta+Minus');
  assert.equal((await options(page)).fontSize, 9);

  // The keys are the terminal's own: the program doesn't get them, and
  // the other terminal keeps its size.
  assert.equal(inputTo(fake, 'w5'), '');
  await page.click('.subtab[data-item="w6"]');
  await page.waitForFunction(() => document.querySelector('.pane[data-pane-id="p4"] .term-host').dataset.termHost === 'w6');
  assert.equal((await options(page, 'w6')).fontSize, 12.5);
});

test('terminals follow Settings\' text size and Option key', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await ready(page, fake);
  assert.deepEqual(await options(page), { fontSize: 12.5, optionAsMeta: false });
  await page.keyboard.press('Meta+Equal');
  const start = fake.callsTo('TermResize').at(-1).args;
  fake.clearCalls();

  // A new size replaces the one ⌘+ gave the terminal.
  fake.world.settings.terminalFontSize = 16;
  fake.world.settings.terminalOptionAsMeta = true;
  await fake.emitState();
  await page.waitForFunction(() => window.bench.terminalOptions('w5').fontSize === 16);
  assert.deepEqual(await options(page), { fontSize: 16, optionAsMeta: true });
  const r = await fake.waitForCall('TermResize');
  assert.ok(r.args[2] < start[2], JSON.stringify({ start, now: r.args }));

  // ⌘0 goes back to Settings' size.
  await page.keyboard.press('Meta+Equal');
  assert.equal((await options(page)).fontSize, 17);
  await page.keyboard.press('Meta+Digit0');
  assert.equal((await options(page)).fontSize, 16);

  // A terminal drawn later has them too.
  await page.click('.subtab[data-item="w6"]');
  await page.waitForFunction(() => document.querySelector('.pane[data-pane-id="p4"] .term-host').dataset.termHost === 'w6');
  assert.deepEqual(await options(page, 'w6'), { fontSize: 16, optionAsMeta: true });

  // Option off again leaves the size alone.
  fake.world.settings.terminalOptionAsMeta = false;
  await fake.emitState();
  await page.waitForFunction(() => window.bench.terminalOptions('w6').optionAsMeta === false);
  assert.equal((await options(page, 'w6')).fontSize, 16);
});

// cellAt returns the middle of the cell of w5's terminal at column col
// and row row, both from 1.
async function cellAt(page, fake, col, row) {
  const [, cols, rows] = fake.callsTo('TermResize').at(-1).args;
  const screen = await page.locator(TERM + ' .xterm-screen').boundingBox();
  const w = screen.width / cols;
  const h = screen.height / rows;
  return { x: screen.x + (col - 0.5) * w, y: screen.y + (row - 0.5) * h };
}

// hover moves the pointer onto a point, as xterm.js needs to find the link
// there: from just to its left.
async function hover(page, p) {
  await page.mouse.move(p.x - 4, p.y);
  await page.mouse.move(p.x + 4, p.y);
}

test('an address that a program prints as plain text opens through Go on Command-click, and shows its host', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await ready(page, fake);
  await fake.termOut('w5', '\x1b[2J\x1b[Hsee https://docs.example.com/guide/start for more');
  await page.waitForFunction((sel) => document.querySelector(sel + ' .xterm-rows').textContent.includes('for more'), TERM);
  await settle(page);
  const p = await cellAt(page, fake, 12, 1);

  // Holding the pointer over it shows its address, with its host in bold,
  // as for links printed with OSC 8.
  const tip = page.locator(TERM + ' .term-link');
  await hover(page, p);
  await tip.waitFor({ state: 'visible' });
  assert.equal(await tip.locator('.term-link-address').textContent(), 'https://docs.example.com/guide/start');
  assert.equal(await tip.locator('.term-link-address strong').textContent(), 'docs.example.com');
  assert.equal(await tip.locator('.term-link-hint').textContent(), 'Command-click to open');

  // A plain click opens nothing.
  await page.mouse.click(p.x + 4, p.y);
  assert.equal(await fake.waitForCall('OpenLink', () => true, 300).catch(() => null), null);

  // Command-click opens it, through Go.
  await hover(page, p);
  await page.keyboard.down('Meta');
  await page.mouse.click(p.x + 4, p.y);
  await page.keyboard.up('Meta');
  assert.deepEqual((await fake.waitForCall('OpenLink')).args, ['https://docs.example.com/guide/start']);
  assert.equal(fake.callsTo('OpenLink').length, 1);

  // Moving off it hides the address.
  await page.mouse.move(p.x, p.y + 200);
  await tip.waitFor({ state: 'hidden' });
});

// openMenu right-clicks w5's terminal, and waits for its menu.
async function openMenu(page, root = TERM) {
  const box = await page.locator(root + ' .xterm-screen').boundingBox();
  await page.mouse.click(box.x + 60, box.y + 40, { button: 'right' });
  await page.waitForSelector('#terminal-menu');
  return { x: box.x + 60, y: box.y + 40 };
}

const menuItems = (page) => page.$$eval('#terminal-menu .menu-item', (els) => els.map((el) => [
  el.querySelector('.menu-label').textContent, el.querySelector('kbd') ? el.querySelector('kbd').textContent : '', el.disabled]));

test('a right-click in a terminal opens its menu: Copy, Paste, Find…, Clear, and Select all', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await ready(page, fake);
  await print(page, fake, ['hello from the program']);

  // It opens at the pointer, with its first entry focused. Copy needs
  // text selected.
  const at = await openMenu(page);
  const box = await page.locator('#terminal-menu').boundingBox();
  assert.ok(Math.abs(box.x - at.x) <= 2 && Math.abs(box.y - at.y) <= 2, JSON.stringify({ box, at }));
  assert.equal(await page.getAttribute('#terminal-menu', 'aria-label'), 'Terminal');
  assert.deepEqual(await menuItems(page), [
    ['Copy', '⌘C', true], ['Paste', '⌘V', false], ['Find…', '⌘F', false], ['Clear', '⌘K', false], ['Select all', '⌘A', false],
  ]);
  assert.equal(await page.evaluate(() => document.activeElement.textContent), 'Paste⌘V');

  // Select all, then Copy, copies the terminal's text.
  await page.click('#terminal-menu .menu-item:has-text("Select all")');
  await page.waitForSelector('#terminal-menu', { state: 'detached' });
  assert.equal(await terminalHasKeyboard(page), true);
  await openMenu(page);
  assert.deepEqual((await menuItems(page))[0], ['Copy', '⌘C', false]);
  await page.click('#terminal-menu .menu-item:has-text("Copy")');
  const copied = await fake.waitForCall('ClipboardSetText');
  assert.ok(copied.args[0].includes('go run ./cmd/bench') && copied.args[0].includes('hello from the program'), copied.args[0]);

  // Paste types the clipboard's text into the terminal. With text
  // selected, Copy is first.
  fake.clipboard = 'echo pasted';
  await openMenu(page);
  assert.equal(await page.evaluate(() => document.activeElement.textContent), 'Copy⌘C');
  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('Enter');
  await fake.waitForCall('TermInput', (a) => a[1].includes('echo pasted'));
  assert.equal(inputTo(fake, 'w5'), 'echo pasted');
  assert.equal(await terminalHasKeyboard(page), true);

  // Find… opens the find bar.
  await openMenu(page);
  await page.click('#terminal-menu .menu-item:has-text("Find…")');
  await page.locator(TERM + ' .term-find').waitFor({ state: 'visible' });
  assert.equal(await active(page), 'term-find-input');
  await page.keyboard.press('Escape');

  // Clear clears the screen and what has scrolled off it, and Go forgets
  // that output too.
  await openMenu(page);
  await page.click('#terminal-menu .menu-item:has-text("Clear")');
  await page.waitForFunction((sel) => !document.querySelector(sel + ' .xterm-rows').textContent.includes('hello from the program'), TERM);
  await fake.waitForCall('TermClear', (a) => a[0] === 'w5');
  assert.equal(inputTo(fake, 'w5'), 'echo pasted');

  // Escape closes the menu, giving the terminal the keyboard back.
  await openMenu(page);
  await page.keyboard.press('Escape');
  await page.waitForSelector('#terminal-menu', { state: 'detached' });
  assert.equal(await terminalHasKeyboard(page), true);
});

test('a right-click in a terminal\'s find bar leaves the usual menu for text', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await ready(page, fake);
  await page.keyboard.press('Meta+f');
  const prevented = await page.$eval(TERM + ' .term-find-input', (el) => {
    const e = new MouseEvent('contextmenu', { bubbles: true, cancelable: true });
    el.dispatchEvent(e);
    return e.defaultPrevented;
  });
  assert.equal(prevented, false);
  assert.equal(await page.$('#terminal-menu'), null);
});

test('⌘K clears a terminal, and ⌘A selects all its text, without the program seeing them', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await ready(page, fake);
  await print(page, fake, ['before clearing']);
  await page.keyboard.press('Meta+a');
  await openMenu(page);
  assert.deepEqual((await menuItems(page))[0], ['Copy', '⌘C', false]);
  await page.keyboard.press('Escape');
  await page.keyboard.press('Meta+k');
  await page.waitForFunction((sel) => !document.querySelector(sel + ' .xterm-rows').textContent.includes('before clearing'), TERM);
  assert.equal(inputTo(fake, 'w5'), '');
  // Go forgets the output too, so that it doesn't come back when the
  // terminal is drawn again, as in a window of its own.
  await fake.waitForCall('TermClear', (a) => a[0] === 'w5');
  assert.equal(fake.callsTo('TermClear').length, 1);
});

test('a popped-out terminal has the same tools, and follows Settings too', async (t) => {
  const root = '.terminal-window';
  const { page, fake, close } = await openTerminalWindow({ id: 'w5' });
  t.after(close);
  await ready(page, fake, root);

  // Settings' text size applies here too, and the window's own keys.
  fake.world.settings.terminalFontSize = 15;
  await fake.emitState();
  await page.waitForFunction(() => window.bench.terminalOptions('w5').fontSize === 15);
  await page.keyboard.press('Meta+Equal');
  assert.equal((await options(page)).fontSize, 16);

  // Its find bar.
  await page.keyboard.press('Meta+f');
  await page.locator(root + ' .term-find').waitFor({ state: 'visible' });
  await page.keyboard.type('cmd/bench');
  await page.waitForFunction((sel) => document.querySelector(sel).textContent === '1 of 1', root + ' .term-find-count');
  await page.keyboard.press('Escape');
  await page.locator(root + ' .term-find').waitFor({ state: 'hidden' });

  // Its menu, at the pointer, within the window.
  const at = await openMenu(page, root);
  const box = await page.locator('#terminal-menu').boundingBox();
  assert.ok(Math.abs(box.x - at.x) <= 2 && Math.abs(box.y - at.y) <= 2, JSON.stringify({ box, at }));
  // What find found stays selected, so Copy is first. Back to Bench, last,
  // is the window's own.
  assert.deepEqual((await menuItems(page)).map((e) => e[0]), ['Copy', 'Paste', 'Find…', 'Clear', 'Select all', 'Back to Bench']);
  assert.equal(await page.evaluate(() => document.activeElement.textContent), 'Copy⌘C');
  await page.keyboard.press('ArrowDown');
  assert.equal(await page.evaluate(() => document.activeElement.textContent), 'Paste⌘V');
  await page.keyboard.press('ArrowUp');
  await page.keyboard.press('ArrowUp');
  assert.equal(await page.evaluate(() => document.activeElement.textContent), 'Back to Bench');
  await page.keyboard.press('Escape');
  await page.waitForSelector('#terminal-menu', { state: 'detached' });
  assert.equal(await terminalHasKeyboard(page), true);

  await openMenu(page, root);
  await page.click('#terminal-menu .menu-item:has-text("Copy")');
  assert.equal((await fake.waitForCall('ClipboardSetText')).args[0], 'cmd/bench');
  fake.clipboard = 'ls -la';
  await openMenu(page, root);
  await page.click('#terminal-menu .menu-item:has-text("Paste")');
  await fake.waitForCall('TermInput', (a) => a[1].includes('ls -la'));
  await page.waitForSelector('#terminal-menu', { state: 'detached' });

  // A click outside it closes it.
  await openMenu(page, root);
  await page.mouse.click(20, 20);
  await page.waitForSelector('#terminal-menu', { state: 'detached' });
  assert.equal(inputTo(fake, 'w5'), 'ls -la');
});
