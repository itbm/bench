'use strict';

// A window in several tabs: its sub-tab's mark, the ⋯ menu's entries, and
// adding a window to a tab by dragging with Option held.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, settle } = require('./helpers/harness.js');
const { defaultWorld } = require('./helpers/fixture.js');

// sharedWorld has Zed's window (w3) in Code and in Research's second pane.
function sharedWorld() {
  const world = defaultWorld();
  world.tabs[2].root.children[1].items.push('w3');
  return world;
}

async function box(page, selector) {
  const b = await page.locator(selector).boundingBox();
  assert.ok(b, selector + ' is on the page');
  return b;
}

test('a window in several tabs has a mark naming the other tabs', async (t) => {
  const { page, close } = await openBench({ world: sharedWorld() });
  t.after(close);
  assert.equal(await page.getAttribute('.subtab[data-item="w3"]', 'aria-label'), 'Zed — bench — switch.go (also in Research)');
  assert.equal(await page.getAttribute('.subtab[data-item="w3"] .subtab-shared', 'title'), 'Also in Research');
  assert.equal(await page.$('.subtab[data-item="w4"] .subtab-shared'), null);
  // In Research, the mark names Code.
  await page.click('.tab[data-index="2"]');
  await page.waitForSelector('.pane[data-pane-id="p6"] .subtab[data-item="w3"]');
  assert.equal(await page.getAttribute('.subtab[data-item="w3"] .subtab-shared', 'title'), 'Also in Code');
});

test('the ⋯ menu offers the tabs a window isn\'t in, and removes it from this one', async (t) => {
  const { page, fake, close } = await openBench({ world: sharedWorld() });
  t.after(close);
  await page.click('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.waitForSelector('#window-menu');
  const entries = await page.$$eval('#window-menu > *', (els) => els.map((el) => el.className + ':' + el.textContent));
  assert.deepEqual(entries.slice(-6), [
    'menu-item:Research',
    'menu-sep:',
    'menu-item:Also show in Comms',
    'menu-sep:',
    'menu-item:Remove from this tab',
    'menu-item:Pop out of Bench',
  ]);
  await page.click('#window-menu .menu-item:has-text("Remove from this tab")');
  const call = await fake.waitForCall('RemoveItemFromTab');
  assert.deepEqual(call.args, ['w3', 1]);
});

test('a window in one tab can\'t be removed from it with the ⋯ menu', async (t) => {
  const { page, close } = await openBench();
  t.after(close);
  await page.click('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.waitForSelector('#window-menu');
  assert.equal(await page.$('#window-menu .menu-item:has-text("Remove from this tab")'), null);
});

test('dropping a sub-tab on a tab with Option held shows it there as well', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await settle(page);
  fake.clearCalls();
  const src = await box(page, '.subtab[data-item="w4"]');
  const tab = await box(page, '.tab[data-index="2"]');
  await page.mouse.move(src.x + src.width / 2, src.y + src.height / 2);
  await page.mouse.down();
  await page.mouse.move(tab.x + tab.width / 2, tab.y + tab.height / 2, { steps: 6 });
  await page.waitForSelector('.tab[data-index="2"].drop-target:not(.drop-add)');
  // Option marks the drop as adding, and letting go of it as moving again.
  await page.keyboard.down('Alt');
  await page.waitForSelector('.tab[data-index="2"].drop-target.drop-add');
  await page.waitForSelector('.drag-ghost.adding');
  await page.keyboard.up('Alt');
  await page.waitForSelector('.tab[data-index="2"].drop-target:not(.drop-add)');
  await page.keyboard.down('Alt');
  await page.mouse.move(tab.x + tab.width / 2 + 2, tab.y + tab.height / 2);
  await page.mouse.up();
  await page.keyboard.up('Alt');
  await fake.waitForLowered();
  const calls = fake.calls.filter((c) => ['AddItemToTab', 'MoveItemToTab'].includes(c.method)).map((c) => c.method + ':' + JSON.stringify(c.args));
  assert.deepEqual(calls, ['AddItemToTab:["w4",2]']);
});

test('the add dialog adds a window from another tab', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('.pane[data-pane-id="p4"] [data-action="add"]');
  await page.waitForSelector('.palette');
  await page.keyboard.type('daily');
  await page.keyboard.press('Enter');
  const call = await fake.waitForCall('AddToPane');
  assert.deepEqual(call.args, ['p4', { type: 'item', id: 'w10' }]);
});
