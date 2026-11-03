'use strict';

// A new tab of windows that are open, against the browser preview's
// simulated Mac: Finder's Downloads window is in no tab, and Zed's is in
// Code. Make tab docks Finder's in a tab of its own, which config.toml
// gets, and shows Zed's there as well. Undo takes the tab away, puts
// Finder's window back under Not in a tab, with the same ID, and leaves
// Zed's in Code.

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const {
  itemID, itemsOf, openPage, setScreen, startPreview, tabIndex, view, waitForToast, waitView,
} = require('./helpers/preview.js');

test('a tab of the open windows docks them, and Undo puts them back', async (t) => {
  const preview = await startPreview({ t });
  const { page, close } = await openPage(preview);
  t.after(close);
  await setScreen(page, 'monitor');
  const before = await view(page);
  const finder = before.floating.find((f) => f.app === 'Finder' && f.title === 'Downloads');
  assert.ok(finder, 'Finder\'s Downloads is under Not in a tab: ' + JSON.stringify(before.floating.map((f) => f.title)));
  const tabs = before.tabs.map((x) => x.name);
  const code = itemsOf(before, 'Code');
  const zed = itemID(before, 'bench — switch.go');

  await page.click('[data-action="new-tab"]');
  await page.waitForSelector('.editor-sheet');
  await page.click('[data-key="start-windows"]');
  await page.waitForSelector('.win-list');
  assert.equal(await page.isChecked('[data-key="win-' + finder.id + '"]'), true);
  assert.equal(await page.inputValue('#tab-name'), 'Finder');
  // Go's preview has the window in the one pane.
  await page.waitForFunction(() => Array.from(document.querySelectorAll('.mini .chips span'), (el) => el.textContent).join() === 'Finder: Downloads');
  assert.equal(await page.getAttribute('[data-key="preset-single"]', 'aria-pressed'), 'true');
  // Zed's window, in Code, makes a second app, and a second pane.
  await page.click('[data-key="win-others"]');
  await page.click('[data-key="win-' + zed + '"]');
  await page.waitForFunction(() => document.querySelectorAll('.mini .mini-pane').length === 2);
  assert.equal(await page.getAttribute('[data-key="preset-side"]', 'aria-pressed'), 'true');
  await page.click('[data-key="editor-save"]');

  let v = await waitView(page, (x, n) => x.tabs.length === n + 1 && x.tabs[x.current].name === 'Finder', tabs.length);
  assert.deepEqual(itemsOf(v, 'Finder'), [[{ app: 'Finder', title: 'Downloads' }], [{ app: 'Zed', title: 'bench — switch.go' }]]);
  assert.equal((v.floating || []).some((f) => f.id === finder.id), false);
  assert.deepEqual(itemsOf(v, 'Code'), code);
  await waitForToast(page, 'Made a new tab, Finder, with 2 windows.');
  const toml = fs.readFileSync(path.join(preview.home, 'config.toml'), 'utf8');
  assert.match(toml, /name = "Finder"/);

  await page.click('#toast .toast-action');
  await waitForToast(page, 'Deleted the new tab, Finder, and put its windows back.');
  v = await waitView(page, (x, id) => (x.floating || []).some((f) => f.id === id) && !x.tabs.some((tab) => tab.name === 'Finder'), finder.id);
  assert.deepEqual(v.tabs.map((x) => x.name), tabs);
  // Zed's window is in Code, as it was, and in no other tab.
  assert.deepEqual(itemsOf(v, 'Code'), code);
  assert.equal(v.items[zed].tab, tabIndex(v, 'Code'));
  assert.ok(!(v.items[zed].tabs || []).some((i) => i !== tabIndex(v, 'Code')), 'Zed is in tabs ' + JSON.stringify(v.items[zed].tabs));
  assert.doesNotMatch(fs.readFileSync(path.join(preview.home, 'config.toml'), 'utf8'), /name = "Finder"/);
});
