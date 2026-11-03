'use strict';

// Minimising a docked window with the real engine. The demo bar's
// Minimise window minimises the window that the focused pane shows, on the
// demo platform, as Command-M does on a Mac. Its pane shows another of its
// windows, or else says it's minimised, and the window stays in the Dock
// until its sub-tab, Show again, or Jump to… brings it back.

const assert = require('node:assert/strict');
const test = require('node:test');
const {
  itemID, openPage, paneOf, setScreen, startPreview, switchTo, waitForToast, waitView,
} = require('./helpers/preview.js');

// openTab starts a preview showing a tab on a simulated monitor.
async function openTab(t, name) {
  const preview = await startPreview({ t });
  const { page, close } = await openPage(preview);
  t.after(close);
  await setScreen(page, 'monitor');
  const v = await switchTo(page, name);
  return { page, v };
}

// shownIn returns the item that a displayed pane shows.
function shownIn(v, pane) {
  const p = v.display.panes.find((x) => x.id === pane);
  return p ? p.active : '';
}

// minimise shows an item in its pane, focusing it, and minimises its window
// from the demo bar.
async function minimise(page, item, pane) {
  await page.click('.subtab[data-item="' + item + '"]');
  await waitView(page, (x, a) => x.display.focus === a.pane && x.display.panes.find((p) => p.id === a.pane).active === a.item, { item, pane });
  await page.click('#demo-bar [data-action="demo-minimise"]');
  return waitView(page, (x, id) => x.items[id].status === 'minimised', item);
}

test('a minimised window\'s pane shows its other window, and its sub-tab brings it back', async (t) => {
  const { page, v } = await openTab(t, 'Code');
  const zed = itemID(v, 'bench — switch.go');
  const code = itemID(v, 'index.html — bench-ui');
  const pane = paneOf(v, zed);
  let w = await minimise(page, zed, pane);
  assert.equal(shownIn(w, pane), code);
  assert.equal(await page.getAttribute('.subtab[data-item="' + zed + '"]', 'data-marker'), 'info');
  assert.match(await page.getAttribute('.subtab[data-item="' + zed + '"]', 'aria-label'), /\(minimised\)$/);
  // It's for information: the status bar doesn't count it.
  assert.equal(await page.locator('#statusbar [data-action="attention"]').count(), 0);

  // Another tab and back leaves it in the Dock.
  await switchTo(page, 'Research');
  w = await switchTo(page, 'Code');
  assert.equal(w.items[zed].status, 'minimised');
  assert.equal(shownIn(w, pane), code);

  // Its sub-tab brings it back.
  await page.click('.subtab[data-item="' + zed + '"]');
  w = await waitView(page, (x, id) => x.items[id].status === 'ready', zed);
  assert.equal(shownIn(w, pane), zed);
});

test('a pane whose only window is minimised says so, and Show again brings it back', async (t) => {
  const { page, v } = await openTab(t, 'Research');
  const notes = itemID(v, 'Daily note');
  const pane = paneOf(v, notes);
  await minimise(page, notes, pane);
  const body = page.locator('.pane[data-pane-id="' + pane + '"] .pane-status');
  await body.waitFor();
  assert.equal(await body.getAttribute('data-status'), 'minimised');
  assert.equal(await body.locator('.status-text').textContent(), 'Minimised');
  assert.deepEqual(await body.locator('button').allTextContents(), ['Show again', 'Remove from this tab']);
  await body.locator('button:has-text("Show again")').click();
  const w = await waitView(page, (x, id) => x.items[id].status === 'ready', notes);
  assert.equal(shownIn(w, pane), notes);
  assert.equal(await page.locator('.pane[data-pane-id="' + pane + '"] .pane-status').count(), 0);

  // Remove from this tab takes a window that's in no other tab out of
  // Bench, still minimised, which it can't be dragged back from.
  await minimise(page, notes, pane);
  await page.locator('.pane[data-pane-id="' + pane + '"] .pane-status button:has-text("Remove from this tab")').click();
  await waitForToast(page, 'Moved Obsidian — Daily note out of Bench. Its window stays in the Dock. Add it back with +.');
  await waitView(page, (x, id) => (x.floating || []).some((f) => f.id === id && f.status === 'ready'), notes);
});

test('Minimise window says when the focused pane has no window to minimise', async (t) => {
  const { page, v } = await openTab(t, 'Code');
  const terminal = paneOf(v, Object.values(v.items).find((i) => i.kind === 'builtin' && i.tab === v.current).id);
  await page.click('.pane[data-pane-id="' + terminal + '"] .pane-header');
  await waitView(page, (x, id) => x.display.focus === id, terminal);
  await page.click('#demo-bar [data-action="demo-minimise"]');
  await waitForToast(page, 'The focused pane doesn\'t show an app\'s window to minimise.');
});
