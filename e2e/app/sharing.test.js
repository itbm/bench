'use strict';

// Showing one window in several tabs, end to end: the ⋯ menu, the add
// dialog, and taking it out of one tab again.

const assert = require('node:assert/strict');
const test = require('node:test');
const {
  itemID, openPage, panesOf, setScreen, startPreview, switchTo, tabIndex, waitForToast, waitView,
} = require('./helpers/preview.js');

// openOn starts a preview showing a tab on a simulated monitor.
async function openOn(t, name) {
  const preview = await startPreview({ t });
  const { page, close } = await openPage(preview);
  t.after(close);
  await setScreen(page, 'monitor');
  const v = await switchTo(page, name);
  return { page, v };
}

test('the ⋯ menu shows a window in another tab as well, and takes it out again', async (t) => {
  const { page, v } = await openOn(t, 'Code');
  const zed = itemID(v, 'bench — switch.go');
  const code = tabIndex(v, 'Code');
  const research = tabIndex(v, 'Research');
  const [mainPane] = panesOf(v, 'Code');
  await page.click('[data-action="menu"][data-pane="' + mainPane.id + '"]');
  await page.locator('#window-menu .menu-item', { hasText: 'Also show in Research' }).click();
  await waitForToast(page, 'Showing Zed — bench — switch.go in Research too.');
  let w = await waitView(page, (x, a) => (x.items[a].tabs || []).length === 2, zed);
  assert.deepEqual(w.items[zed].tabs, [code, research]);
  assert.equal(w.items[zed].tab, code);
  assert.equal(await page.getAttribute('.subtab[data-item="' + zed + '"] .subtab-shared', 'title'), 'Also in Research');

  // In Research, it's shown in the focused pane.
  w = await switchTo(page, 'Research');
  assert.equal(w.items[zed].tab, research);
  const pane = w.display.panes.find((p) => p.id === w.items[zed].pane);
  assert.equal(pane.active, zed);
  await page.waitForSelector('.pane[data-pane-id="' + pane.id + '"] .subtab[data-item="' + zed + '"]');

  await page.click('[data-action="menu"][data-pane="' + pane.id + '"]');
  await page.locator('#window-menu .menu-item', { hasText: 'Remove from this tab' }).click();
  await waitForToast(page, 'Removed Zed — bench — switch.go from Research. It\'s still in Code.');
  w = await waitView(page, (x, a) => x.items[a.zed].tab === a.code && !x.items[a.zed].tabs, { zed, code });
  assert.equal(await page.$('.subtab[data-item="' + zed + '"]'), null);
});

test('the add dialog shows a window from another tab in this pane as well', async (t) => {
  const { page, v } = await openOn(t, 'Research');
  const [, notesPane] = panesOf(v, 'Research');
  await page.click('[data-action="add"][data-pane="' + notesPane.id + '"]');
  await page.waitForSelector('#palette-list');
  await page.locator('.palette-item', { hasText: 'bench — switch.go' }).click();
  const w = await waitView(page, (x) => Object.values(x.items).some((it) => it.title === 'bench — switch.go' && it.tab === x.current));
  const zed = itemID(w, 'bench — switch.go');
  assert.equal(w.items[zed].pane, notesPane.id);
  assert.deepEqual(w.items[zed].tabs, [tabIndex(w, 'Code'), tabIndex(w, 'Research')]);
  assert.equal(w.display.panes.find((p) => p.id === notesPane.id).active, zed);
});
