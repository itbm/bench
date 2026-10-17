'use strict';

// Terminal items: one xterm.js view per item, kept across renders.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, settle } = require('./helpers/harness.js');

const b64 = (s) => Buffer.from(s, 'utf8').toString('base64');

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
  await fake.waitForCall('TermInput', (a) => a[1] === '\r');
  assert.deepEqual(fake.callsTo('TermInput').map((c) => c.args), [['w5', 'l'], ['w5', 's'], ['w5', '\r']]);

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
