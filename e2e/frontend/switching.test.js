'use strict';

// Switching tabs responds at once (decision 67): a click on a tab, a row of
// the jump palette, a card of the overview, a key in the browser preview,
// or Go's switching command for a tab's shortcut from any app, shows the
// new tab as current in the title bar before Go has arranged it, and the
// View that follows settles it. A refused switch puts the title bar back.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, settle } = require('./helpers/harness.js');
const { defaultWorld } = require('./helpers/fixture.js');

// SLOW is how long, in milliseconds, Go takes to switch in these tests.
const SLOW = 800;

// wait waits ms milliseconds.
function wait(ms) {
  return new Promise((r) => setTimeout(r, ms));
}

// slowSwitches makes Go slow to switch tabs, and to step to the next or
// previous one, as with apps that take a while to arrange. It returns the
// calls' answers, which the test can wait for.
function slowSwitches(fake) {
  const answered = [];
  for (const method of ['SwitchTab', 'StepTab']) {
    fake.on(method, async (args, f) => {
      await wait(SLOW);
      const value = f['m' + method](...args);
      answered.push(method);
      return value;
    });
  }
  return answered;
}

// selected returns the index of each tab that the title bar shows as
// current.
function selected(page) {
  return page.evaluate(() => [...document.querySelectorAll('#titlebar .tab[role="tab"]')]
    .filter((el) => el.getAttribute('aria-selected') === 'true').map((el) => Number(el.dataset.index)));
}

// The tests start on Comms, tab 0.

// viewCurrent returns the current tab of the View that the UI has.
function viewCurrent(page) {
  return page.evaluate(() => window.bench.store.view.current);
}

test('a click on a tab shows it as current at once, and the View settles it', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ current: 0 }) });
  t.after(close);
  const answered = slowSwitches(fake);
  await settle(page);
  assert.deepEqual(await selected(page), [0]);

  const start = Date.now();
  await page.click('.tab[data-index="2"] .tab-name');
  await page.waitForSelector('.tab[data-index="2"][aria-selected="true"]');
  assert.ok(Date.now() - start < SLOW / 2, 'the tab showed as current only after ' + (Date.now() - start) + ' ms');
  assert.deepEqual(await selected(page), [2]);
  assert.equal(await viewCurrent(page), 0, 'the View has the old tab until Go answers');
  assert.equal(answered.length, 0);
  // Only the title bar changes at once: the panes are still the old tab's.
  assert.equal(await page.getAttribute('.tab[data-index="2"]', 'aria-controls'), 'panes');
  assert.ok(await page.$('.pane[data-pane-id="p1"]'), 'the old tab\'s panes went before Go switched');

  // Go answers, and its View has the tab current.
  await page.waitForFunction(() => window.bench.store.view.current === 2);
  assert.deepEqual(await selected(page), [2]);
  assert.equal(await page.evaluate(() => window.bench.store.ui.switching), null);
  await page.waitForSelector('.pane[data-pane-id="p5"]');
  assert.deepEqual(fake.callsTo('SwitchTab').map((c) => c.args), [[2]]);
});

test('a View sent before the switch doesn\'t put the old tab back, and a second click wins', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ current: 0 }) });
  t.after(close);
  slowSwitches(fake);
  await settle(page);
  await page.click('.tab[data-index="1"] .tab-name');
  await page.waitForSelector('.tab[data-index="1"][aria-selected="true"]');
  // Something else changes while Go arranges: its View still has Comms.
  await fake.emitState();
  await settle(page);
  assert.deepEqual(await selected(page), [1]);

  // A click on Research while the switch to Code is on its way: Code's
  // View comes first, then Research's.
  await page.click('.tab[data-index="2"] .tab-name');
  await page.waitForSelector('.tab[data-index="2"][aria-selected="true"]');
  await page.waitForFunction(() => window.bench.store.view.current === 1);
  assert.deepEqual(await selected(page), [2]);
  await page.waitForFunction(() => window.bench.store.view.current === 2);
  assert.deepEqual(await selected(page), [2]);
  assert.equal(await page.evaluate(() => window.bench.store.ui.switching), null);
});

// The tab you came from no longer shows as current once you've clicked
// another, so a click on it, before Go has switched, is a click on another
// tab: it shows at once and switches back, with no wait for a double-click,
// and isn't lost when the first switch's View comes in. Go takes 250 ms to
// switch to Research, and 600 ms to switch back, so that Research's View
// comes in between.
test('a click back on the tab you came from, before Go has switched, shows it at once and switches back', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ current: 0 }) });
  t.after(close);
  fake.on('SwitchTab', async (args, f) => {
    await wait(args[0] === 2 ? 250 : 600);
    return f.mSwitchTab(...args);
  });
  await settle(page);

  await page.click('.tab[data-index="2"] .tab-name');
  await page.waitForSelector('.tab[data-index="2"][aria-selected="true"]');
  const start = Date.now();
  await page.click('.tab[data-index="0"] .tab-name');
  assert.deepEqual(await page.evaluate(() => window.bench.store.ui.switching), { index: 0 });
  await page.waitForSelector('.tab[data-index="0"][aria-selected="true"]');
  assert.ok(Date.now() - start < 200, 'the tab showed as current only after ' + (Date.now() - start) + ' ms');
  assert.deepEqual(fake.callsTo('SwitchTab').map((c) => c.args), [[2], [0]]);

  // Research's View doesn't take the title bar back to Research.
  await page.waitForFunction(() => window.bench.store.view.current === 2);
  assert.deepEqual(await selected(page), [0]);
  await page.waitForFunction(() => window.bench.store.view.current === 0);
  assert.deepEqual(await selected(page), [0]);
  assert.equal(await page.evaluate(() => window.bench.store.ui.switching), null);
});

test('a refused switch puts the current tab back, and says why', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ current: 0 }) });
  t.after(close);
  fake.on('SwitchTab', async () => {
    await wait(300);
    throw new Error('Bench can\'t switch tabs while it quits.');
  });
  await settle(page);
  await page.click('.tab[data-index="1"] .tab-name');
  await page.waitForSelector('.tab[data-index="1"][aria-selected="true"]');
  await page.waitForSelector('.tab[data-index="0"][aria-selected="true"]');
  assert.deepEqual(await selected(page), [0]);
  await page.waitForFunction(() => document.getElementById('toast').textContent === 'Bench can\'t switch tabs while it quits.');
});

test('a switch whose View doesn\'t come doesn\'t leave the wrong tab shown', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ current: 0 }) });
  t.after(close);
  await settle(page);
  // Go answers, but its View is lost.
  fake.on('SwitchTab', () => null);
  await page.click('.tab[data-index="2"] .tab-name');
  await page.waitForSelector('.tab[data-index="2"][aria-selected="true"]');
  await fake.waitForCall('SwitchTab');
  await page.waitForSelector('.tab[data-index="0"][aria-selected="true"]', { timeout: 3000 });
  assert.deepEqual(await selected(page), [0]);
});

test('the palette\'s tab rows and the overview\'s cards show the tab at once', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ current: 0 }) });
  t.after(close);
  slowSwitches(fake);
  await settle(page);

  await page.click('[data-action="open-palette"]');
  await page.fill('#palette-query', 'Research');
  await page.waitForFunction(() => {
    const row = document.querySelector('.palette-item[aria-selected="true"] .palette-label');
    return row && row.textContent === 'Research';
  });
  await page.keyboard.press('Enter');
  await page.waitForSelector('.tab[data-index="2"][aria-selected="true"]');
  assert.equal(await viewCurrent(page), 0);
  await page.waitForFunction(() => window.bench.store.view.current === 2);

  await fake.emit('bench:command', { name: 'show-all-tabs' });
  await page.waitForSelector('.overview');
  await page.click('#overview-card-1');
  await page.waitForSelector('.overview', { state: 'detached' });
  await page.waitForSelector('.tab[data-index="1"][aria-selected="true"]');
  assert.equal(await viewCurrent(page), 2);
  await page.waitForFunction(() => window.bench.store.view.current === 1);
  assert.deepEqual(await selected(page), [1]);
});

test('a tab\'s key in the browser preview shows the tab at once, and next_tab steps from it', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ demo: true, current: 0 }) });
  t.after(close);
  slowSwitches(fake);
  await page.click('.statusbar');
  await settle(page);

  await page.keyboard.press('Control+Alt+Digit2');
  await page.waitForSelector('.tab[data-index="1"][aria-selected="true"]');
  assert.equal(await viewCurrent(page), 0);
  // next_tab, before Go has switched, goes on from the tab shown.
  await page.keyboard.press('Control+Alt+BracketRight');
  await page.waitForSelector('.tab[data-index="2"][aria-selected="true"]');
  assert.deepEqual(fake.callsTo('SwitchTab').map((c) => c.args), [[1]]);
  await fake.waitForCall('StepTab');
  assert.deepEqual(fake.callsTo('StepTab').map((c) => c.args), [[1]]);
  await page.waitForFunction(() => window.bench.store.view.current === 2);
  assert.deepEqual(await selected(page), [2]);

  // previous_tab wraps.
  await page.keyboard.press('Control+Alt+Digit1');
  await page.waitForFunction(() => window.bench.store.view.current === 0);
  await page.keyboard.press('Control+Alt+BracketLeft');
  await page.waitForSelector('.tab[data-index="2"][aria-selected="true"]');
  assert.equal(await viewCurrent(page), 0);
});

test('Go\'s switching command shows the tab of a shortcut from any app at once, and switch-refused puts it back', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ current: 0 }) });
  t.after(close);
  await settle(page);

  // Tab 0 has no tab field in Go's command, as Go leaves it out.
  fake.world.current = 2;
  await fake.emitState();
  await page.waitForSelector('.tab[data-index="2"][aria-selected="true"]');
  await fake.emit('bench:command', { name: 'switching' });
  await page.waitForSelector('.tab[data-index="0"][aria-selected="true"]');
  fake.world.current = 0;
  await fake.emitState();
  await settle(page);
  assert.deepEqual(await selected(page), [0]);
  assert.equal(await page.evaluate(() => window.bench.store.ui.switching), null);

  await fake.emit('bench:command', { name: 'switching', tab: 1 });
  await page.waitForSelector('.tab[data-index="1"][aria-selected="true"]');
  assert.equal(await viewCurrent(page), 0);
  await fake.emit('bench:command', { name: 'switch-refused' });
  await page.waitForSelector('.tab[data-index="0"][aria-selected="true"]');

  // A command for the current tab, or for one that isn't there, changes
  // nothing.
  await fake.emit('bench:command', { name: 'switching', tab: 0 });
  await fake.emit('bench:command', { name: 'switching', tab: 7 });
  await settle(page);
  assert.deepEqual(await selected(page), [0]);
  assert.equal(await page.evaluate(() => window.bench.store.ui.switching), null);
});
