'use strict';

// Dragging sub-tabs to move windows, and dividers to resize panes.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, settle } = require('./helpers/harness.js');

// order returns the methods called, in order, from those listed.
function order(fake, methods) {
  return fake.calls.filter((c) => methods.includes(c.method)).map((c) => c.method + ':' + JSON.stringify(c.args));
}

async function box(page, selector) {
  const b = await page.locator(selector).boundingBox();
  assert.ok(b, selector + ' is on the page');
  return b;
}

test('dragging a sub-tab to another pane\'s middle joins that pane', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await settle(page);
  fake.clearCalls();
  const target = await box(page, '.pane[data-pane-id="p4"]');
  await page.dragAndDrop('.subtab[data-item="w4"]', '.pane[data-pane-id="p4"]', {
    targetPosition: { x: target.width / 2, y: target.height / 2 },
  });
  await fake.waitForLowered();
  assert.deepEqual(order(fake, ['SetOverlay', 'MoveItem', 'MoveItemToTab']), [
    'SetOverlay:[true]',
    'MoveItem:["w4",1,"p4","center"]',
    'SetOverlay:[false]',
  ]);
});

test('dragging a sub-tab to a pane\'s right edge splits it', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await settle(page);
  fake.clearCalls();
  const target = await box(page, '.pane[data-pane-id="p4"]');
  await page.dragAndDrop('.subtab[data-item="w4"]', '.pane[data-pane-id="p4"]', {
    targetPosition: { x: target.width - 12, y: target.height / 2 },
  });
  await fake.waitForLowered();
  assert.deepEqual(order(fake, ['SetOverlay', 'MoveItem', 'MoveItemToTab']), [
    'SetOverlay:[true]',
    'MoveItem:["w4",1,"p4","right"]',
    'SetOverlay:[false]',
  ]);
});

test('dragging a sub-tab onto a tab moves it to that tab', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await settle(page);
  fake.clearCalls();
  await page.dragAndDrop('.subtab[data-item="w4"]', '.tab[data-index="2"]');
  await fake.waitForLowered();
  assert.deepEqual(order(fake, ['SetOverlay', 'MoveItem', 'MoveItemToTab']), [
    'SetOverlay:[true]',
    'MoveItemToTab:["w4",2]',
    'SetOverlay:[false]',
  ]);
});

test('dragging shows where the window will land', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  const src = await box(page, '.subtab[data-item="w3"]');
  const pane = await box(page, '.pane[data-pane-id="p3"]');
  await page.mouse.move(src.x + src.width / 2, src.y + src.height / 2);
  await page.mouse.down();
  // The bottom quarter of its own pane: split down.
  await page.mouse.move(pane.x + pane.width / 2, pane.y + pane.height - 20, { steps: 5 });
  await page.waitForSelector('.pane[data-pane-id="p3"] .drop-indicator:not([hidden])');
  assert.equal(await page.getAttribute('.pane[data-pane-id="p3"] .drop-indicator', 'data-zone'), 'bottom');
  // The middle of its own pane.
  await page.mouse.move(pane.x + pane.width / 2, pane.y + pane.height / 2, { steps: 5 });
  await page.waitForSelector('.pane[data-pane-id="p3"] .drop-indicator[data-zone="center"]:not([hidden])');
  // A tab: tinted.
  const tab = await box(page, '.tab[data-index="0"]');
  await page.mouse.move(tab.x + tab.width / 2, tab.y + tab.height / 2, { steps: 5 });
  await page.waitForSelector('.tab[data-index="0"].drop-target');
  assert.equal(await page.$('.drop-indicator:not([hidden])'), null);
  // The current tab isn't a target.
  const cur = await box(page, '.tab[data-index="1"]');
  await page.mouse.move(cur.x + cur.width / 2, cur.y + cur.height / 2, { steps: 5 });
  await page.waitForSelector('.tab.drop-target', { state: 'detached' });
  // Back to the middle of its own pane, and drop: nothing to do.
  await page.mouse.move(pane.x + pane.width / 2, pane.y + pane.height / 2, { steps: 5 });
  await page.mouse.up();
  await fake.waitForLowered();
  assert.equal(fake.callsTo('MoveItem').length, 0);
  assert.equal(await page.$('.subtab.dragging'), null);
});

test('a state that arrives during a drag is drawn when it ends', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  const src = await box(page, '.subtab[data-item="w3"]');
  await page.mouse.move(src.x + src.width / 2, src.y + src.height / 2);
  await page.mouse.down();
  await page.mouse.move(src.x + 80, src.y + 200, { steps: 5 });
  await fake.waitForCall('SetOverlay', (a) => a[0] === true);
  fake.world.items.w4.title = 'renamed.html — bench-ui';
  await fake.emitState();
  await settle(page);
  // The dragged sub-tab is still the same element.
  assert.equal(await page.$eval('.subtab[data-item="w3"]', (el) => el.classList.contains('dragging')), true);
  await page.mouse.move(src.x + 4000, src.y + 4000);
  await page.mouse.up();
  await page.waitForSelector('.subtab:has-text("renamed.html")');
});

test('a state that arrives while a sub-tab is pressed waits, and the drag that follows works', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  const src = await box(page, '.subtab[data-item="w4"]');
  const target = await box(page, '.pane[data-pane-id="p4"]');
  await page.mouse.move(src.x + src.width / 2, src.y + src.height / 2);
  await page.mouse.down();
  // A terminal's title changes, which would redraw the panes. WebKit
  // starts a drag from the element pressed, so it mustn't be replaced.
  fake.world.items.w5.title = 'make — ~/code/bench';
  await fake.emitState();
  await settle(page);
  assert.equal(await page.textContent('.subtab[data-item="w5"] .subtab-title'), 'zsh — ~/code/bench');
  await page.mouse.move(target.x + target.width / 2, target.y + target.height / 2, { steps: 8 });
  await page.mouse.up();
  const move = await fake.waitForCall('MoveItem');
  assert.deepEqual(move.args, ['w4', 1, 'p4', 'center']);
  await page.waitForSelector('.subtab:has-text("make — ~/code/bench")');
});

test('dragging a divider calls SetSizes, at most once a frame and once on release', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  const view = fake.view();
  const dv = view.display.dividers[0];
  assert.equal(dv.splitId, 's2');
  const el = await box(page, '.divider[data-split="s2"]');
  const cx = el.x + el.width / 2;
  const cy = el.y + el.height / 2;
  const win = await page.$eval('#app-window', (w) => w.getBoundingClientRect().left);
  assert.equal(win, 0);
  assert.equal(await page.getAttribute('.divider[data-split="s2"]', 'aria-valuenow'), '65');

  await page.mouse.move(cx, cy);
  await page.mouse.down();
  await page.mouse.move(cx - 100, cy, { steps: 4 });
  await page.mouse.move(cx - 200, cy, { steps: 4 });
  await settle(page, 50);
  await page.mouse.up();
  await settle(page);
  const calls = fake.callsTo('SetSizes');
  assert.ok(calls.length >= 2, 'SetSizes during the drag and on release');
  assert.ok(calls.length <= 9, 'no more than once a frame');
  for (const c of calls) assert.equal(c.args[0], 's2');
  // The pointer's position maps through the frame formula.
  const x = cx - 200;
  const gap = view.settings.gap;
  const a = Math.round((((x - dv.splitRect.x + gap / 2) / (dv.splitRect.w + gap)) * 100) * 100) / 100;
  const last = calls[calls.length - 1].args[1];
  assert.deepEqual(last, [a, Math.round((100 - a) * 100) / 100]);

  // Each child keeps at least 10%.
  fake.clearCalls();
  const el2 = await box(page, '.divider[data-split="s2"]');
  await page.mouse.move(el2.x + el2.width / 2, cy);
  await page.mouse.down();
  await page.mouse.move(5, cy, { steps: 3 });
  await page.mouse.up();
  await settle(page);
  const clamped = fake.callsTo('SetSizes').pop().args[1];
  assert.deepEqual(clamped, [10, 90]);
});

test('arrow keys move a focused divider', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.focus('.divider[data-split="s2"]');
  assert.equal(await page.getAttribute('.divider[data-split="s2"]', 'role'), 'separator');
  await page.keyboard.press('ArrowLeft');
  const call = await fake.waitForCall('SetSizes');
  assert.deepEqual(call.args, ['s2', [63, 37]]);
  await page.waitForSelector('.divider[data-split="s2"][aria-valuenow="63"]');
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'divider-s2-0');
});

test('a drag that starts with a menu open closes it and keeps Bench raised', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await settle(page);
  fake.clearCalls();
  await page.click('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.waitForSelector('#window-menu');
  await fake.waitForCall('SetOverlay', (a) => a[0] === true);
  const src = await box(page, '.subtab[data-item="w4"]');
  await page.mouse.move(src.x + src.width / 2, src.y + src.height / 2);
  await page.mouse.down();
  await page.mouse.move(src.x + 200, src.y + 300, { steps: 6 });
  await page.waitForSelector('#window-menu', { state: 'detached' });
  await page.mouse.move(src.x + 4000, src.y + 4000);
  await page.mouse.up();
  await fake.waitForLowered();
  await settle(page);
  assert.deepEqual(fake.callsTo('SetOverlay').map((c) => c.args[0]), [true, false]);
});

test('a divider drag ends if the button is released where Bench can\'t see it', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  const el = await box(page, '.divider[data-split="s2"]');
  const cx = el.x + el.width / 2;
  const cy = el.y + el.height / 2;
  await page.mouse.move(cx, cy);
  await page.mouse.down();
  await page.mouse.move(cx - 100, cy, { steps: 3 });
  await settle(page);
  // A move with no button pressed, as after a release outside the window.
  await page.evaluate(([x, y]) => window.dispatchEvent(new PointerEvent('pointermove', { clientX: x, clientY: y, buttons: 0 })), [cx - 120, cy]);
  await settle(page);
  assert.equal(await page.evaluate(() => window.bench.store.ui.divider), null);
  assert.equal(await page.evaluate(() => document.body.classList.contains('resizing-row')), false);
  const sent = fake.callsTo('SetSizes').length;
  await page.evaluate(([x, y]) => window.dispatchEvent(new PointerEvent('pointermove', { clientX: x, clientY: y, buttons: 1 })), [cx - 300, cy]);
  await settle(page);
  assert.equal(fake.callsTo('SetSizes').length, sent, 'no resizing after the drag ended');
  await page.mouse.up();
});
