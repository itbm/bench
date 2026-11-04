'use strict';

// Closing in the browser preview: a tab's ×, which asks first and closes
// the tab's windows, pages, and terminals, and deletes the tab from
// config.toml, and which is clear of the tab's name; and a sub-tab's ×,
// which closes one window.

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const {
  chooseMode, itemID, openPage, panesOf, setScreen, startPreview, switchTo, tabIndex, waitForToast, waitUntil, waitView,
} = require('./helpers/preview.js');

async function open(t) {
  const preview = await startPreview({ t });
  const { page, close } = await openPage(preview);
  t.after(close);
  await setScreen(page, 'monitor');
  return { preview, page };
}

function readConfig(preview) {
  return fs.readFileSync(path.join(preview.home, 'config.toml'), 'utf8');
}

test('a tab\'s × asks, then closes the tab\'s windows and deletes it from config.toml', async (t) => {
  const { preview, page } = await open(t);
  let v = await switchTo(page, 'Research');
  const ids = panesOf(v, 'Research').flatMap((p) => p.items);
  assert.equal(ids.length, 4);
  await page.click('.tab[data-index="' + tabIndex(v, 'Research') + '"] .close-cross');
  await page.waitForSelector('.question-sheet');
  assert.equal(await page.textContent('#question-title'), 'Close Research and its 4 windows?');
  // The details are behind a disclosure, and the question has no default
  // button, as other apps' windows close.
  assert.equal(await page.isVisible('#question-details'), false);
  await page.click('[data-action="question-details"]');
  assert.equal(await page.textContent('#question-details'), 'Apps can still ask about unsaved changes.'
    + ' The tab goes from config.toml, so it won\'t come back when Bench next opens.'
    + ' Undo brings back the tab, but not the windows that close.');
  assert.equal(await page.getAttribute('[data-action="close-run"]', 'class'), 'btn danger');
  await page.click('[data-action="close-run"]');
  // The toast says what Undo can't bring back, as the details do.
  await waitForToast(page, 'Closed Research. Undo brings back the tab, but not the windows that closed.');
  v = await waitView(page, (x) => x.tabs.length === 2);
  assert.deepEqual(v.tabs.map((x) => x.name), ['Comms', 'Code']);
  // Its windows closed: they aren't under Not in a tab.
  v = await waitView(page, (x, gone) => gone.every((id) => !x.items[id] && !(x.floating || []).some((f) => f.id === id)), ids);
  await waitUntil(() => !readConfig(preview).includes('name = "Research"'), 'config.toml to lose Research');
});

test('a sub-tab\'s × closes its window', async (t) => {
  const { page } = await open(t);
  let v = await switchTo(page, 'Research');
  const pr = itemID(v, 'Pull request #42 · bench');
  await page.hover('.subtab[data-item="' + pr + '"]');
  await page.click('.subtab[data-item="' + pr + '"] .close-cross');
  v = await waitView(page, (x, id) => !x.items[id] && !(x.floating || []).some((f) => f.id === id), pr);
  assert.equal(panesOf(v, 'Research').flatMap((p) => p.items).length, 3);
  assert.equal(await page.$('.question-sheet'), null);
});

test('a page closed with its × comes back in its place with Undo, and with Reopen Closed Page from Jump to…', async (t) => {
  const { page } = await open(t);
  let v = await switchTo(page, 'Comms');
  const slack = v.items.w1;
  assert.equal(slack.kind, 'web');
  const pane = panesOf(v, 'Comms').find((p) => p.items.includes('w1'));
  const panes = panesOf(v, 'Comms').map((p) => p.items);
  await page.hover('.subtab[data-item="w1"]');
  await page.click('.subtab[data-item="w1"] .close-cross');
  await waitForToast(page, /^Closed .+\.$/);
  // It's kept by the name that the toast gave it, with its app.
  const label = (await page.textContent('#toast .toast-text')).replace(/^Closed (.+)\.$/, '$1');
  v = await waitView(page, (x) => !x.items.w1 && (x.closedItems || []).length === 1);
  assert.deepEqual(v.closedItems[0], { id: 'w1', kind: 'web', label, tab: 'Comms' });
  assert.equal(v.undo.name, 'Close Page');
  assert.equal(await page.textContent('#toast .toast-action'), 'Undo');
  await page.click('#toast .toast-action');
  await waitForToast(page, /^Reopened .+\.$/);
  v = await waitView(page, (x) => x.items.w1 && !x.closedItems);
  assert.deepEqual(panesOf(v, 'Comms').find((p) => p.id === pane.id).items, pane.items);
  assert.equal(v.items.w1.url, slack.url);

  // Closed again, Jump to… reopens it, and Undo is first there. Its pane
  // went with it, as it had nothing else, so it comes back in a pane of
  // its own, on the side it was on.
  await page.hover('.subtab[data-item="w1"]');
  await page.click('.subtab[data-item="w1"] .close-cross');
  await waitView(page, (x) => !x.items.w1);
  await page.click('[data-action="open-palette"]');
  await page.waitForSelector('#palette-item-0');
  assert.equal(await page.textContent('#palette-item-0 .palette-label'), 'Undo Close Page');
  await page.fill('#palette-query', 'reopen closed page');
  await page.keyboard.press('Enter');
  v = await waitView(page, (x) => x.items.w1 && !x.closedItems);
  assert.equal(v.tabs[v.current].name, 'Comms');
  assert.deepEqual(panesOf(v, 'Comms').map((p) => p.items), panes);
  // The change that closed it can't be undone now that it's back.
  assert.equal(v.undo, undefined);
});

test('in the compact layout on a laptop\'s screen, where tabs don\'t show their shortcuts, a tab\'s × is clear of its name', async (t) => {
  const { page } = await open(t);
  await setScreen(page, 'laptop');
  await chooseMode(page, 'compact');
  const v = await waitView(page, (x) => x.sizeClass === 'compact');
  assert.equal(await page.$('.tab kbd'), null);
  // clearOfName says where a tab's name ends, and where its ×'s fade starts.
  const clearOfName = (index) => page.$eval('.tab[data-index="' + index + '"]', (tab) => {
    const name = tab.querySelector('.tab-name').getBoundingClientRect();
    const cross = tab.querySelector('.close-cross');
    const fade = cross.getBoundingClientRect().left - parseFloat(getComputedStyle(cross, '::before').width);
    return { ok: name.right <= fade, name: name.right, fade };
  });
  const current = await clearOfName(v.current);
  assert.ok(current.ok, JSON.stringify(current));
  const other = (v.current + 1) % v.tabs.length;
  await page.hover('.tab[data-index="' + other + '"] .tab-name');
  const hovered = await clearOfName(other);
  assert.ok(hovered.ok, JSON.stringify(hovered));
});
