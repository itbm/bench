'use strict';

// The layout editor: changing a tab's preset, adding a tab, and deleting
// one, with config.toml updated in the support folder.

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const {
  itemsOf, openPage, panesOf, setScreen, startPreview, switchTo, tabIndex, view, waitForToast, waitUntil,
  waitView,
} = require('./helpers/preview.js');

async function open(t) {
  const preview = await startPreview({ t });
  const { page, close } = await openPage(preview);
  t.after(close);
  await setScreen(page, 'monitor');
  return { preview, page };
}

// readConfig returns config.toml, or the backup if bak is set.
function readConfig(preview, bak = false) {
  return fs.readFileSync(path.join(preview.home, bak ? 'config.toml.bak' : 'config.toml'), 'utf8');
}

// tabSection returns the text of a tab's [[tab]] entry in config.toml.
function tabSection(toml, name) {
  const parts = toml.split(/^\[\[tab\]\]$/m).slice(1);
  return parts.find((p) => p.includes('name = "' + name + '"')) || '';
}

test('Three columns keeps Code\'s windows in order and updates config.toml', async (t) => {
  const { preview, page } = await open(t);
  await switchTo(page, 'Code');
  const before = readConfig(preview);
  assert.match(tabSection(before, 'Code'), /layout = "main-side"/);

  await page.click('[data-action="edit-tab"]');
  await page.waitForSelector('#editor-title:text("Edit tab")');
  assert.equal(await page.inputValue('#tab-name'), 'Code');
  assert.equal(await page.getAttribute('[data-preset="main-side"]', 'aria-pressed'), 'true');
  await page.click('[data-preset="three"]');
  // The preview shows where the windows will go, from Go's PreviewTree.
  await page.waitForFunction(() => document.querySelectorAll('.mini .mini-pane').length === 3);
  const chips = await page.locator('.mini .mini-pane').evaluateAll((els) => els.map((el) => el.textContent));
  assert.deepEqual(chips, ['Zed: bench — switch.goVS Code: index.html — bench-ui', 'Terminal: zsh — ~/code', 'Empty pane']);
  await page.click('[data-action="save-tab"]');

  const v = await waitView(page, (x) => x.tabs[1].preset === 'three' && x.display.panes.length === 3);
  assert.deepEqual(itemsOf(v, 'Code'), [
    [{ app: 'Zed', title: 'bench — switch.go' }, { app: 'VS Code', title: 'index.html — bench-ui' }],
    [{ app: 'Terminal', title: 'zsh — ~/code' }],
    [],
  ]);
  assert.deepEqual(v.tabs[1].root.sizes, [40, 30, 30]);
  // The dialog closes once Go's answer arrives, which can be after the
  // view that Go sent first.
  await page.waitForSelector('#overlay-root .dialog', { state: 'detached' });
  // The empty pane offers to add a window.
  const empty = v.display.panes[2].id;
  await page.waitForSelector('.pane[data-pane-id="' + empty + '"] .empty [data-action="add"]');

  // config.toml has the new layout, and the backup has the old one.
  const after = await waitUntil(() => {
    const s = readConfig(preview);
    return /layout = "three"/.test(tabSection(s, 'Code')) && s;
  }, 'config.toml to have the new layout');
  assert.match(tabSection(after, 'Code'), /app = "dev.zed.Zed"/);
  assert.equal(readConfig(preview, true), before);
});

test('a new tab gets the next shortcut', async (t) => {
  const { preview, page } = await open(t);
  await page.click('[data-action="new-tab"]');
  await page.waitForSelector('#editor-title:text("New tab")');
  assert.equal(await page.textContent('#shortcut-value'), '⌃⌥4');
  await page.fill('#tab-name', 'Notes');
  await page.click('[data-preset="side"]');
  await page.click('[data-action="save-tab"]');

  let v = await waitView(page, (x) => x.tabs.length === 4 && x.current === 3);
  assert.equal(v.tabs[3].name, 'Notes');
  assert.equal(v.tabs[3].shortcut, '⌃⌥4');
  assert.equal(v.tabs[3].preset, 'side');
  assert.equal(v.display.panes.length, 2);
  assert.equal(await page.textContent('.tab[data-index="3"]'), 'Notes⌃⌥4');
  await waitUntil(() => tabSection(readConfig(preview), 'Notes').includes('layout = "side"'), 'config.toml to have the new tab');

  // Its shortcut works.
  await page.keyboard.press('Control+Alt+Digit1');
  await waitView(page, (x) => x.current === 0);
  await page.keyboard.press('Control+Alt+Digit4');
  v = await waitView(page, (x) => x.current === 3);
  assert.equal(v.tabs[v.current].name, 'Notes');

  // A name that's taken is refused, and the editor stays open.
  await page.click('[data-action="new-tab"]');
  await page.fill('#tab-name', 'code');
  await page.click('[data-action="save-tab"]');
  await waitForToast(page, /Code/);
  assert.equal(await page.locator('.editor-sheet').count(), 1);
});

test('deleting a tab moves its windows to Not in a tab', async (t) => {
  const { preview, page } = await open(t);
  let v = await switchTo(page, 'Research');
  const windows = panesOf(v, 'Research').flatMap((p) => p.items);
  assert.equal(windows.length, 4);
  await page.click('[data-action="edit-tab"]');
  await page.click('[data-action="delete-tab"]');

  v = await waitView(page, (x) => x.tabs.length === 2);
  assert.deepEqual(v.tabs.map((x) => x.name), ['Comms', 'Code']);
  assert.throws(() => tabIndex(v, 'Research'));
  const floating = (v.floating || []).map((f) => f.app + ': ' + f.title);
  for (const title of ['Brave: Wails v3 docs', 'Brave: Pull request #42 · bench', 'Brave: Go packages', 'Obsidian: Daily note']) {
    assert.ok(floating.includes(title), title + ' is under Not in a tab: ' + floating);
  }
  await waitUntil(() => !readConfig(preview).includes('name = "Research"'), 'config.toml to lose Research');

  await page.click('[data-action="open-palette"]');
  await page.fill('#palette-query', 'add to this tab');
  const groups = await page.locator('.palette-group').allTextContents();
  assert.deepEqual(groups, ['Not in a tab']);
  const labels = await page.locator('.palette-item .palette-label').allTextContents();
  assert.ok(labels.includes('Daily note'));
  assert.equal((await view(page)).tabs.length, 2);
});
