'use strict';

// Terminal items: one xterm.js view per item, kept across renders.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, openTerminalWindow, settle } = require('./helpers/harness.js');

const b64 = (s) => Buffer.from(s, 'utf8').toString('base64');

// inputTo returns everything sent to a terminal with TermInput, in order.
function inputTo(fake, id) {
  return fake.callsTo('TermInput').filter((c) => c.args[0] === id).map((c) => c.args[1]).join('');
}

test('a terminal item has one xterm that survives a re-render and forwards input', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.waitForSelector('.pane[data-pane-id="p4"] .xterm');
  assert.equal(await page.$$eval('.xterm', (els) => els.length), 1);
  assert.deepEqual(fake.callsTo('TermSnapshot').map((c) => c.args), [['w5']]);
  // The snapshot is replayed.
  await page.waitForFunction(() => document.querySelector('.xterm-rows').textContent.includes('go run ./cmd/bench'));
  // The terminal is fitted to its pane and tells Go its size.
  const resize = await fake.waitForCall('TermResize');
  assert.equal(resize.args[0], 'w5');
  assert.ok(resize.args[1] > 20 && resize.args[2] > 10, 'columns and rows: ' + resize.args.slice(1));

  // Mark the element, then make a change that redraws the panes.
  await page.$eval('.xterm', (el) => { el.dataset.marker = 'kept'; });
  fake.world.items.w3.title = 'bench — layout.go';
  await fake.emitState();
  await page.waitForSelector('.subtab:has-text("bench — layout.go")');
  await settle(page);
  assert.equal(await page.$eval('.pane[data-pane-id="p4"] .xterm', (el) => el.dataset.marker), 'kept');
  assert.equal(await page.evaluate(() => window.bench.terminalCount()), 1);
  assert.equal(fake.callsTo('TermSnapshot').length, 1);

  // Typing goes to TermInput.
  await page.click('.pane[data-pane-id="p4"] .xterm');
  await page.keyboard.type('ls');
  await page.keyboard.press('Enter');
  await fake.waitForCall('TermInput', (a) => a[1].endsWith('\r'));
  assert.equal(inputTo(fake, 'w5'), 'ls\r');

  // Focus stays in the terminal across a redraw.
  fake.world.items.w3.title = 'bench — frame.go';
  await fake.emitState();
  await page.waitForSelector('.subtab:has-text("bench — frame.go")');
  assert.equal(await page.evaluate(() => Boolean(document.activeElement.closest('.term-host[data-term-host="w5"]'))), true);

  // Output from Go is written.
  await fake.emit('term:data', { id: 'w5', data: b64('\r\nhello from Go\r\n') });
  await page.waitForFunction(() => document.querySelector('.xterm-rows').textContent.includes('hello from Go'));
});

test('each terminal keeps its own view across sub-tab switches, and ends with its item', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.waitForSelector('.pane[data-pane-id="p4"] .xterm');
  await page.click('.subtab[data-item="w6"]');
  await page.waitForFunction(() => document.querySelector('.pane[data-pane-id="p4"] .term-host').dataset.termHost === 'w6');
  await page.waitForFunction(() => document.querySelector('.pane[data-pane-id="p4"] .xterm-rows').textContent.includes('go test ./...'));
  assert.deepEqual(fake.callsTo('TermSnapshot').map((c) => c.args), [['w5'], ['w6']]);
  assert.equal(await page.evaluate(() => window.bench.terminalCount()), 2);
  // Output for the hidden terminal is kept.
  await fake.emit('term:data', { id: 'w5', data: b64('\r\nwhile hidden\r\n') });
  await page.click('.subtab[data-item="w5"]');
  await page.waitForFunction(() => document.querySelector('.pane[data-pane-id="p4"] .term-host').dataset.termHost === 'w5');
  await page.waitForFunction(() => document.querySelector('.pane[data-pane-id="p4"] .xterm-rows').textContent.includes('while hidden'));
  assert.equal(fake.callsTo('TermSnapshot').length, 2);

  // A resized pane refits the terminal.
  fake.clearCalls();
  const before = await fake.waitForCall('TermResize', () => true, 10).catch(() => null);
  assert.equal(before, null);
  await page.setViewportSize({ width: 1000, height: 900 });
  const resize = await fake.waitForCall('TermResize');
  assert.equal(resize.args[0], 'w5');

  // Removing the item ends its terminal.
  const p4 = fake.world.tabs[1].root.children[1];
  p4.items = ['w5'];
  delete fake.world.items.w6;
  await fake.emitState();
  await page.waitForFunction(() => window.bench.terminalCount() === 1);
});

test('a terminal that comes back from its own window tells Go its size again', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.waitForSelector('.pane[data-pane-id="p4"] .xterm');
  const first = await fake.waitForCall('TermResize', (a) => a[0] === 'w5');
  await settle(page);
  // Pop out of Bench: w5 floats in its own window, which sets the
  // session's size to that window's.
  const p4 = fake.world.tabs[1].root.children[1];
  p4.items = ['w6'];
  p4.active = 'w6';
  fake.world.floating.push({ ...fake.world.items.w5, tab: -1 });
  await fake.emitState();
  await page.waitForFunction(() => document.querySelector('.pane[data-pane-id="p4"] .term-host').dataset.termHost === 'w6');
  await settle(page);
  assert.equal(await page.evaluate(() => window.bench.terminalCount()), 2, 'the main window keeps w5\'s view');
  fake.clearCalls();
  // Added back to the same pane, at the same size as before.
  fake.world.floating = fake.world.floating.filter((f) => f.id !== 'w5');
  p4.items = ['w5', 'w6'];
  p4.active = 'w5';
  await fake.emitState();
  const again = await fake.waitForCall('TermResize', (a) => a[0] === 'w5');
  assert.deepEqual(again.args, first.args);
  // A redraw that keeps it shown doesn't send it again.
  fake.clearCalls();
  fake.world.items.w3.title = 'bench — again.go';
  await fake.emitState();
  await page.waitForSelector('.subtab:has-text("bench — again.go")');
  await settle(page);
  assert.equal(fake.callsTo('TermResize').filter((c) => c.args[0] === 'w5').length, 0);
});

test('output that arrives before the snapshot is shown once, after it', async (t) => {
  const { page, fake, close } = await openBench({
    setup: (fake) => fake.on('TermSnapshot', async ([id], f) => {
      // Sent before the snapshot was taken, so it's in the snapshot.
      await f.emit('term:data', { id, data: b64('second line\r\n') });
      // Sent after it.
      await f.emit('term:data', { id, data: b64('third line\r\n') });
      return b64('first line\r\nsecond line\r\n');
    }),
  });
  t.after(close);
  await page.waitForFunction(() => {
    const rows = document.querySelector('.pane[data-pane-id="p4"] .xterm-rows');
    return rows && rows.textContent.includes('third line');
  });
  const text = await page.$eval('.pane[data-pane-id="p4"] .xterm-rows', (el) => el.textContent);
  const count = (s) => text.split(s).length - 1;
  assert.equal(count('first line'), 1);
  assert.equal(count('second line'), 1);
  assert.equal(count('third line'), 1);
  assert.ok(text.indexOf('first line') < text.indexOf('second line') && text.indexOf('second line') < text.indexOf('third line'));
  assert.equal(fake.callsTo('TermSnapshot').length, 1);
});

test('typing reaches Go in order, one call at a time', async (t) => {
  let busy = 0;
  let most = 0;
  const { page, fake, close } = await openBench({
    // Go is slow to answer, so keys are typed while a call is on its way.
    setup: (f) => f.on('TermInput', async () => {
      busy++;
      most = Math.max(most, busy);
      await new Promise((r) => setTimeout(r, 60));
      busy--;
    }),
  });
  t.after(close);
  await page.waitForSelector('.pane[data-pane-id="p4"] .xterm');
  await page.click('.pane[data-pane-id="p4"] .xterm');
  await page.keyboard.type('echo hello world');
  await page.keyboard.press('Enter');
  const end = Date.now() + 4000;
  while (!inputTo(fake, 'w5').endsWith('\r') && Date.now() < end) await new Promise((r) => setTimeout(r, 20));
  assert.equal(inputTo(fake, 'w5'), 'echo hello world\r');
  assert.equal(most, 1, 'one TermInput at a time');
  assert.ok(fake.callsTo('TermInput').length < 17, 'keys typed meanwhile go together');
});

test('mouse reports in the default encoding reach TermInput', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.waitForFunction(() => document.querySelector('.pane[data-pane-id="p4"] .xterm-rows').textContent.includes('go run'));
  // The program turns on X10 mouse reporting.
  await fake.emit('term:data', { id: 'w5', data: b64('\x1b[?1000h') });
  await settle(page);
  fake.clearCalls();
  const screen = await page.locator('.pane[data-pane-id="p4"] .xterm-screen').boundingBox();
  await page.mouse.click(screen.x + 30, screen.y + 30);
  const call = await fake.waitForCall('TermInput', (a) => a[1].startsWith('\x1b[M'));
  assert.equal(call.args[0], 'w5');
  assert.equal(call.args[1].length, 6);
});

test('a popped-out terminal\'s window shows just that terminal, filling the window', async (t) => {
  const { page, fake, close } = await openTerminalWindow({ id: 'w5' });
  t.after(close);
  // None of the main window's parts are there, and the main window's
  // business (Ready, the pane area, raised mode) is left alone.
  for (const sel of ['#titlebar', '#panes', '#statusbar', '.pane']) assert.equal(await page.$(sel), null, sel);
  assert.equal(await page.isVisible('#demo-bar'), false);
  await page.waitForFunction(() => document.querySelector('.terminal-window .xterm-rows').textContent.includes('go run ./cmd/bench'));
  const resize = await fake.waitForCall('TermResize');
  assert.equal(resize.args[0], 'w5');
  const host = await page.locator('.terminal-window .term-host').boundingBox();
  assert.deepEqual([host.x, host.y, host.width, host.height], [0, 0, 760, 480]);
  assert.ok(resize.args[1] > 60 && resize.args[2] > 15, 'columns and rows: ' + resize.args.slice(1));
  for (const m of ['Ready', 'SetPaneArea', 'SetOverlay', 'SetDemoScreen']) assert.equal(fake.callsTo(m).length, 0, m);

  // It has focus, so typing goes straight to the terminal.
  await page.keyboard.type('ls');
  await fake.waitForCall('TermInput', (a) => a[1].endsWith('s'));
  assert.equal(inputTo(fake, 'w5'), 'ls');
  assert.deepEqual(fake.callsTo('TermInput').filter((c) => c.args[0] !== 'w5'), []);
  await fake.emit('term:data', { id: 'w5', data: b64('\r\nfrom Go\r\n') });
  await fake.emit('term:data', { id: 'w6', data: b64('\r\nanother terminal\r\n') });
  await page.waitForFunction(() => document.querySelector('.terminal-window .xterm-rows').textContent.includes('from Go'));
  assert.ok(!(await page.textContent('.terminal-window .xterm-rows')).includes('another terminal'));

  // Its title follows the terminal's.
  fake.world.items.w5.title = 'vim — ~/code/bench';
  await fake.emitState();
  await page.waitForFunction(() => document.title === 'Terminal — vim — ~/code/bench');

  // When the item goes, the window says so.
  const p4 = fake.world.tabs[1].root.children[1];
  p4.items = ['w6'];
  delete fake.world.items.w5;
  await fake.emitState();
  await page.waitForSelector('.terminal-window .empty');
  assert.equal(await page.textContent('.terminal-window .empty'), 'This terminal has closed.');
  assert.equal(await page.$('.xterm'), null);
});

test('a terminal window for a terminal that has gone says so', async (t) => {
  const { page, close } = await openTerminalWindow({
    id: 'w99',
    setup: (fake) => fake.on('TermSnapshot', () => { throw new Error('There\'s no such terminal.'); }),
  });
  t.after(close);
  assert.equal(await page.textContent('.terminal-window .empty'), 'There\'s no such terminal.');
  assert.equal(await page.$('.xterm'), null);
});
