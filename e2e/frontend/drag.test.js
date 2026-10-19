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

test('a copy of the sub-tab follows the pointer, and the drop isn\'t a click', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await settle(page);
  fake.clearCalls();
  const src = await box(page, '.subtab[data-item="w6"]');
  await page.mouse.move(src.x + src.width / 2, src.y + src.height / 2);
  await page.mouse.down();
  // Less than the drag threshold: still a press.
  await page.mouse.move(src.x + src.width / 2 + 2, src.y + src.height / 2 + 1);
  assert.equal(await page.$('.drag-ghost'), null);
  await page.mouse.move(src.x + src.width / 2, src.y + src.height / 2 + 4);
  await page.mouse.move(src.x + 40, src.y + 120, { steps: 3 });
  await page.waitForSelector('.drag-ghost:has-text("' + (await page.textContent('.subtab[data-item="w6"] .subtab-title')) + '")');
  const ghost = await box(page, '.drag-ghost');
  assert.ok(Math.abs(ghost.x - (src.x + 40 + 12)) <= 1 && Math.abs(ghost.y - (src.y + 120 + 10)) <= 1, 'the copy is by the pointer');
  // Dropped in the middle of its own pane, which isn't focused: nothing
  // moves, and the button coming up there doesn't focus the pane.
  const pane = await box(page, '.pane[data-pane-id="p4"]');
  await page.mouse.move(pane.x + pane.width / 2, pane.y + pane.height / 2, { steps: 3 });
  await page.mouse.up();
  await fake.waitForLowered();
  await settle(page);
  assert.equal(await page.$('.drag-ghost'), null);
  assert.deepEqual(order(fake, ['MoveItem', 'MoveItemToTab', 'ShowItem', 'FocusPane']), []);
  // The next click is a click.
  await page.click('.subtab[data-item="w6"]');
  assert.deepEqual((await fake.waitForCall('ShowItem')).args, ['p4', 'w6']);
});

test('Escape cancels a sub-tab drag', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await settle(page);
  fake.clearCalls();
  const src = await box(page, '.subtab[data-item="w4"]');
  const tab = await box(page, '.tab[data-index="2"]');
  await page.mouse.move(src.x + src.width / 2, src.y + src.height / 2);
  await page.mouse.down();
  await page.mouse.move(tab.x + tab.width / 2, tab.y + tab.height / 2, { steps: 6 });
  await page.waitForSelector('.tab[data-index="2"].drop-target');
  await page.keyboard.press('Escape');
  await fake.waitForLowered();
  assert.equal(await page.$('.tab.drop-target'), null);
  assert.equal(await page.$('.drag-ghost'), null);
  assert.equal(await page.$('.subtab.dragging'), null);
  await page.mouse.up();
  await settle(page);
  assert.deepEqual(order(fake, ['SetOverlay', 'MoveItem', 'MoveItemToTab']), ['SetOverlay:[true]', 'SetOverlay:[false]']);
});

test('releasing the button after Escape cancels a drag isn\'t a click', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await settle(page);
  fake.clearCalls();
  const src = await box(page, '.subtab[data-item="w4"]');
  const x = src.x + src.width / 2;
  const y = src.y + src.height / 2;
  await page.mouse.move(x, y);
  await page.mouse.down();
  await page.mouse.move(x + 200, y + 300, { steps: 6 });
  await page.waitForSelector('.drag-ghost');
  // Back over the sub-tab, which isn't shown: a click there would show it.
  await page.mouse.move(x, y, { steps: 6 });
  await page.keyboard.press('Escape');
  await fake.waitForLowered();
  await page.mouse.up();
  await settle(page);
  assert.deepEqual(order(fake, ['ShowItem', 'FocusPane', 'MoveItem', 'MoveItemToTab']), []);
  // The next click is a click.
  await page.click('.subtab[data-item="w4"]');
  assert.deepEqual((await fake.waitForCall('ShowItem')).args, ['p3', 'w4']);
});

test('another button coming up during a drag doesn\'t drop', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await settle(page);
  fake.clearCalls();
  const src = await box(page, '.subtab[data-item="w4"]');
  const target = await box(page, '.pane[data-pane-id="p4"]');
  const x = target.x + target.width / 2;
  const y = target.y + target.height / 2;
  await page.mouse.move(src.x + src.width / 2, src.y + src.height / 2);
  await page.mouse.down();
  await page.mouse.move(x, y, { steps: 8 });
  await page.waitForSelector('.pane[data-pane-id="p4"] .drop-indicator[data-zone="center"]:not([hidden])');
  // A two-finger tap on the trackpad, with the primary button still held.
  await page.evaluate(([cx, cy]) => {
    document.elementFromPoint(cx, cy).dispatchEvent(new MouseEvent('mouseup', { clientX: cx, clientY: cy, button: 2, buttons: 1, bubbles: true }));
  }, [x, y]);
  await settle(page);
  assert.notEqual(await page.$('.drag-ghost'), null, 'still dragging');
  assert.equal(fake.callsTo('MoveItem').length, 0);
  await page.mouse.up();
  await fake.waitForLowered();
  assert.deepEqual(order(fake, ['SetOverlay', 'MoveItem']), ['SetOverlay:[true]', 'MoveItem:["w4",1,"p4","center"]', 'SetOverlay:[false]']);
});

// staleMove dispatches what WebKit on macOS sends just before a quick
// release: a move at the drop point that reports no button held, because
// WebKit reads the buttons as it handles the event.
async function staleMove(page, x, y) {
  await page.evaluate(([cx, cy]) => {
    const init = { clientX: cx, clientY: cy, buttons: 0, bubbles: true };
    window.dispatchEvent(new PointerEvent('pointermove', { ...init, pointerId: 1, pointerType: 'mouse', isPrimary: true }));
    window.dispatchEvent(new MouseEvent('mousemove', init));
  }, [x, y]);
}

for (const [where, target, expected] of [
  ['a tab', (b) => b.tab, 'MoveItemToTab:["w4",2]'],
  ['a pane\'s right edge', (b) => ({ x: b.pane.x + b.pane.width - 24, y: b.pane.y, width: 0, height: b.pane.height }), 'MoveItem:["w4",1,"p4","right"]'],
]) {
  test('a drop on ' + where + ' lands when the last move reports no button held', async (t) => {
    const { page, fake, close } = await openBench();
    t.after(close);
    await settle(page);
    fake.clearCalls();
    const src = await box(page, '.subtab[data-item="w4"]');
    const b = target({ tab: await box(page, '.tab[data-index="2"]'), pane: await box(page, '.pane[data-pane-id="p4"]') });
    const x = b.x + b.width / 2;
    const y = b.y + b.height / 2;
    await page.mouse.move(src.x + src.width / 2, src.y + src.height / 2);
    await page.mouse.down();
    await page.mouse.move(x, y, { steps: 8 });
    await staleMove(page, x, y);
    await page.mouse.up();
    await fake.waitForLowered();
    assert.deepEqual(order(fake, ['SetOverlay', 'MoveItem', 'MoveItemToTab']), ['SetOverlay:[true]', expected, 'SetOverlay:[false]']);
  });
}

test('a move with no button held and no release after it cancels the drag', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await settle(page);
  fake.clearCalls();
  const src = await box(page, '.subtab[data-item="w4"]');
  const tab = await box(page, '.tab[data-index="2"]');
  await page.mouse.move(src.x + src.width / 2, src.y + src.height / 2);
  await page.mouse.down();
  await page.mouse.move(tab.x + tab.width / 2, tab.y + tab.height / 2, { steps: 6 });
  await staleMove(page, tab.x + tab.width / 2, tab.y + tab.height / 2);
  // Still dragging until the wait is over.
  assert.notEqual(await page.$('.drag-ghost'), null);
  await fake.waitForLowered();
  assert.equal(await page.$('.drag-ghost'), null);
  assert.deepEqual(order(fake, ['SetOverlay', 'MoveItem', 'MoveItemToTab']), ['SetOverlay:[true]', 'SetOverlay:[false]']);
  await page.mouse.up();
});

test('a mouseup with no pointerup still drops', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await settle(page);
  fake.clearCalls();
  const src = await box(page, '.subtab[data-item="w4"]');
  const tab = await box(page, '.tab[data-index="2"]');
  const x = tab.x + tab.width / 2;
  const y = tab.y + tab.height / 2;
  await page.mouse.move(src.x + src.width / 2, src.y + src.height / 2);
  await page.mouse.down();
  await page.mouse.move(x, y, { steps: 6 });
  await page.waitForSelector('.tab[data-index="2"].drop-target');
  // WebKit sends a release it reads as still held on as a pointermove, and
  // only the mouseup says the button came up.
  await page.evaluate(([cx, cy]) => {
    document.elementFromPoint(cx, cy).dispatchEvent(new MouseEvent('mouseup', { clientX: cx, clientY: cy, button: 0, buttons: 0, bubbles: true }));
  }, [x, y]);
  await fake.waitForLowered();
  assert.deepEqual(order(fake, ['SetOverlay', 'MoveItem', 'MoveItemToTab']), ['SetOverlay:[true]', 'MoveItemToTab:["w4",2]', 'SetOverlay:[false]']);
  await page.mouse.up();
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
  // A terminal's title changes, which would redraw the panes. The drag
  // starts from the element pressed, so it mustn't be replaced.
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

test('a mouseup with no pointerup ends a divider drag', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  const el = await box(page, '.divider[data-split="s2"]');
  const cx = el.x + el.width / 2;
  const cy = el.y + el.height / 2;
  await page.mouse.move(cx, cy);
  await page.mouse.down();
  await page.mouse.move(cx - 100, cy, { steps: 3 });
  await settle(page);
  // WebKit sends a release it reads as still held on as a pointermove, and
  // only the mouseup says the button came up. The browser's own mouseup
  // must come, so the press mustn't have stopped it.
  await page.evaluate(() => window.addEventListener('pointerup', (e) => e.stopImmediatePropagation(), { capture: true, once: true }));
  await page.mouse.up();
  await settle(page);
  assert.equal(await page.evaluate(() => window.bench.store.ui.divider), null);
  assert.equal(await page.evaluate(() => document.body.classList.contains('resizing-row')), false);
  const sent = fake.callsTo('SetSizes').length;
  await page.evaluate(([x, y]) => window.dispatchEvent(new PointerEvent('pointermove', { clientX: x, clientY: y, buttons: 1 })), [cx - 300, cy]);
  await settle(page);
  assert.equal(fake.callsTo('SetSizes').length, sent, 'no resizing after the drag ended');
});

test('dragging a divider gives the keyboard back to the terminal that had it', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('.pane[data-pane-id="p4"] .xterm');
  await page.waitForSelector('.pane[data-pane-id="p4"][data-focused="true"]');
  const el = await box(page, '.divider[data-split="s2"]');
  const cx = el.x + el.width / 2;
  const cy = el.y + el.height / 2;
  await page.mouse.move(cx, cy);
  await page.mouse.down();
  await page.mouse.move(cx - 100, cy, { steps: 4 });
  await page.mouse.up();
  await settle(page);
  assert.equal(await page.evaluate(() => {
    const host = document.activeElement.closest('.term-host[data-term-host]');
    return host ? host.dataset.termHost : document.activeElement.tagName;
  }), 'w5');
  fake.clearCalls();
  await page.keyboard.type('ls');
  await fake.waitForCall('TermInput', (a) => a[0] === 'w5' && a[1].endsWith('s'));
  // Arrow keys reach the terminal too, rather than moving the divider.
  await page.keyboard.press('ArrowLeft');
  await fake.waitForCall('TermInput', (a) => a[0] === 'w5' && a[1] === '\x1b[D');
  assert.equal(fake.callsTo('SetSizes').length, 0);
});

test('a divider drag doesn\'t give the keyboard back to a terminal outside the focused pane', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('.pane[data-pane-id="p4"] .xterm');
  await page.waitForSelector('.pane[data-pane-id="p4"][data-focused="true"]');
  const el = await box(page, '.divider[data-split="s2"]');
  const cx = el.x + el.width / 2;
  const cy = el.y + el.height / 2;
  await page.mouse.move(cx, cy);
  await page.mouse.down();
  await page.mouse.move(cx - 100, cy, { steps: 4 });
  // Focus following, say, focuses Zed's pane during the drag.
  fake.world.tabs[1].focus = 'p3';
  await fake.emitState();
  await page.waitForSelector('.pane[data-pane-id="p3"][data-focused="true"]');
  await page.mouse.up();
  await settle(page);
  assert.equal(await page.evaluate(() => document.activeElement.tagName), 'BODY');
  fake.clearCalls();
  await page.keyboard.type('x');
  await settle(page);
  assert.equal(fake.callsTo('TermInput').length, 0, 'typing doesn\'t reach the terminal');
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
