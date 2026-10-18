'use strict';

// Calls whose order matters reach Go in order. Go can handle calls out of
// order, so the UI sends one at a time for each piece of state, and only
// the latest while one is on its way.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, settle } = require('./helpers/harness.js');

// slow makes a method answer after ms, and counts how many calls to it are
// on their way at once.
function slow(fake, method, ms) {
  const stats = { busy: 0, most: 0 };
  fake.on(method, async (args, f) => {
    stats.busy++;
    stats.most = Math.max(stats.most, stats.busy);
    await new Promise((r) => setTimeout(r, ms));
    stats.busy--;
    const own = f['m' + method];
    return own ? own.apply(f, args) : undefined;
  });
  return stats;
}

async function until(fn, timeout = 4000) {
  const end = Date.now() + timeout;
  while (!fn()) {
    if (Date.now() > end) throw new Error('timed out');
    await new Promise((r) => setTimeout(r, 20));
  }
}

test('raised mode ends lowered when menus open and close faster than Go answers', async (t) => {
  let stats;
  const { page, fake, close } = await openBench({ setup: (f) => { stats = slow(f, 'SetOverlay', 80); } });
  t.after(close);
  await settle(page);
  await until(() => stats.busy === 0);
  fake.clearCalls();
  for (let i = 0; i < 3; i++) {
    await page.click('[data-action="menu"][data-pane="p3"]');
    await page.keyboard.press('Escape');
  }
  await settle(page, 400);
  await until(() => stats.busy === 0);
  const seq = fake.callsTo('SetOverlay').map((c) => c.args[0]);
  assert.equal(stats.most, 1, 'one SetOverlay at a time');
  assert.equal(seq[seq.length - 1], false, 'the last word is lowered: ' + JSON.stringify(seq));
  assert.ok(seq.length < 6, 'calls made while one was on its way are replaced: ' + JSON.stringify(seq));
});

test('a divider\'s sizes reach Go in order, ending with where it was let go', async (t) => {
  let stats;
  const { page, fake, close } = await openBench({ setup: (f) => { stats = slow(f, 'SetSizes', 80); } });
  t.after(close);
  await settle(page);
  fake.clearCalls();
  const d = await page.locator('.divider[data-split="s2"]').boundingBox();
  const x = d.x + d.width / 2;
  const y = d.y + d.height / 2;
  await page.mouse.move(x, y);
  await page.mouse.down();
  for (let i = 1; i <= 8; i++) await page.mouse.move(x - i * 30, y);
  await page.mouse.up();
  await until(() => stats.busy === 0 && fake.callsTo('SetSizes').length > 0);
  await settle(page, 200);
  await until(() => stats.busy === 0);
  const firsts = fake.callsTo('SetSizes').map((c) => c.args[1][0]);
  assert.equal(stats.most, 1, 'one SetSizes at a time');
  assert.ok(firsts.length < 9, 'sizes made while one was on its way are replaced: ' + firsts);
  // The pointer only moved left, so no call is bigger than the one before.
  for (let i = 1; i < firsts.length; i++) assert.ok(firsts[i] <= firsts[i - 1], 'in order: ' + firsts);
  const root = fake.world.tabs[1].root;
  assert.equal(root.sizes[0], firsts[firsts.length - 1]);
});
