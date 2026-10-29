'use strict';

// Tab properties, opened from a tab's menu (right-click the tab): changing a
// tab's preset or name, adding a tab, and deleting one, with config.toml
// updated in the support folder. The settings for every tab are in the
// Settings dialog (settings.test.js).

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const {
  itemsOf, openPage, openTabProperties, panesOf, setScreen, startPreview, switchTo, tabIndex, view, waitForToast,
  waitUntil, waitView,
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

// settingsSection returns the text of config.toml's [settings] table.
function settingsSection(toml) {
  return toml.split(/^\[shortcuts\]$/m)[0];
}

test('Three columns keeps Code\'s windows in order and updates config.toml', async (t) => {
  const { preview, page } = await open(t);
  await switchTo(page, 'Code');
  const before = readConfig(preview);
  assert.match(tabSection(before, 'Code'), /layout = "main-side"/);

  // Right-clicking the tab opens its menu, whose first entry opens its
  // properties.
  await page.click('.tab[data-index="1"]', { button: 'right' });
  await page.waitForSelector('#tab-menu');
  assert.equal(await page.getAttribute('#tab-menu', 'aria-label'), 'Options for the Code tab');
  assert.deepEqual(await page.locator('#tab-menu .menu-item .menu-label').allTextContents(),
    ['Tab properties…', 'Rename…', 'Rearrange', 'Reset layout', 'Close tab…', 'Delete tab (keep windows)', 'New tab…']);
  await page.click('#tab-menu .menu-item:has-text("Tab properties…")');
  await page.waitForSelector('#editor-title:text("Tab properties")');
  assert.equal(await page.locator('#tab-menu').count(), 0);
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

test('Tab properties renames another tab without switching to it, and leaves [settings] alone', async (t) => {
  const { preview, page } = await open(t);
  const before = readConfig(preview);
  assert.equal((await view(page)).current, 0);

  await openTabProperties(page, 'Research');
  assert.equal(await page.textContent('#editor-title'), 'Tab properties');
  assert.equal(await page.inputValue('#tab-name'), 'Research');
  // The settings for every tab are in the Settings dialog, not here.
  assert.equal(await page.locator('.editor-sheet [data-setting], .editor-sheet #compact-below').count(), 0);
  assert.equal(await page.textContent('#small-screens-text'), 'For screens narrower than 1,800 points, as set in Settings.');
  await page.fill('#tab-name', 'Reading');
  await page.press('#tab-name', 'Enter');

  const v = await waitView(page, (x) => x.tabs[2].name === 'Reading');
  await waitForToast(page, 'Saved Reading');
  await page.waitForSelector('.editor-sheet', { state: 'detached' });
  assert.equal(v.current, 0);
  assert.deepEqual(v.tabs.map((x) => x.name), ['Comms', 'Code', 'Reading']);
  assert.equal(await page.textContent('.tab[data-index="2"]'), 'Reading⌃⌥3');
  // Focus goes back to the tab whose menu opened the dialog.
  await page.waitForSelector('.tab[data-index="2"]:focus');

  // config.toml has the new name, and its [settings] are as they were.
  const after = await waitUntil(() => {
    const s = readConfig(preview);
    return s.includes('name = "Reading"') && s;
  }, 'config.toml to have the new name');
  assert.ok(!after.includes('name = "Research"'));
  assert.equal(tabSection(after, 'Reading'), tabSection(before, 'Research').replace('name = "Research"', 'name = "Reading"'));
  assert.equal(settingsSection(after), settingsSection(before));
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
  // The editor closes when SaveTab returns, which can be after the View and
  // config.toml change. Until then, shortcuts go to the dialog.
  await page.waitForSelector('.editor-sheet', { state: 'detached' });

  // Its shortcut works.
  await page.keyboard.press('Control+Alt+Digit1');
  await waitView(page, (x) => x.current === 0);
  await page.keyboard.press('Control+Alt+Digit4');
  v = await waitView(page, (x) => x.current === 3);
  assert.equal(v.tabs[v.current].name, 'Notes');

  // A name that's taken, ignoring case, is numbered, as the dialog says
  // before it's saved.
  await page.click('[data-action="new-tab"]');
  await page.waitForSelector('#editor-title:text("New tab")');
  await page.fill('#tab-name', 'code');
  assert.equal(await page.textContent('#tab-name-note'), 'There\'s already a tab called Code, so this one will be called code (1).');
  await page.click('[data-action="save-tab"]');
  v = await waitView(page, (x) => x.tabs.length === 5);
  assert.equal(v.tabs[4].name, 'code (1)');
  await waitForToast(page, 'Saved code (1)');
  await page.waitForSelector('.editor-sheet', { state: 'detached' });
  await waitUntil(() => readConfig(preview).includes('name = "code (1)"'), 'config.toml to have the numbered name');
});

test('deleting a tab moves its windows to Not in a tab', async (t) => {
  const { preview, page } = await open(t);
  let v = await switchTo(page, 'Research');
  const windows = panesOf(v, 'Research').flatMap((p) => p.items);
  assert.equal(windows.length, 4);
  await openTabProperties(page);
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

test('Undo on Delete tab\'s toast puts the tab back, and Reopen closed tab brings it back later', async (t) => {
  const { preview, page } = await open(t);
  let v = await switchTo(page, 'Research');
  const windows = panesOf(v, 'Research').flatMap((p) => p.items);
  await openTabProperties(page);
  await page.click('[data-action="delete-tab"]');
  await waitForToast(page, 'Deleted Research. Its windows are in Jump to… under “Not in a tab”.');
  await waitUntil(() => !readConfig(preview).includes('name = "Research"'), 'config.toml to lose Research');

  // Undo puts the tab back where it was, with its windows, and its entry
  // back in config.toml.
  await page.click('#toast .toast-action');
  await waitForToast(page, 'Brought back Research.');
  v = await waitView(page, (x) => x.tabs.length === 3);
  assert.deepEqual(v.tabs.map((x) => x.name), ['Comms', 'Code', 'Research']);
  assert.deepEqual(panesOf(v, 'Research').flatMap((p) => p.items), windows);
  assert.equal((v.floating || []).filter((f) => windows.includes(f.id)).length, 0);
  assert.equal(v.closedTabs, undefined);
  await waitUntil(() => readConfig(preview).includes('name = "Research"'), 'config.toml to have Research again');

  // Deleted again, the tab's menu can reopen it.
  await page.click('.tab[data-index="2"]', { button: 'right' });
  await page.click('#tab-menu .menu-item:has-text("Delete tab (keep windows)")');
  v = await waitView(page, (x) => x.tabs.length === 2);
  assert.deepEqual(v.closedTabs, ['Research']);
  await page.click('.tab[data-index="0"]', { button: 'right' });
  await page.waitForSelector('#tab-menu');
  assert.equal(await page.textContent('#tab-menu .menu-item:has-text("Reopen closed tab") .menu-detail'), 'Research');
  await page.click('#tab-menu .menu-item:has-text("Reopen closed tab")');
  await waitForToast(page, 'Reopened Research.');
  v = await waitView(page, (x) => x.tabs.length === 3);
  assert.equal(v.tabs[v.current].name, 'Research');
  assert.deepEqual(panesOf(v, 'Research').flatMap((p) => p.items), windows);
  await waitUntil(() => readConfig(preview).includes('name = "Research"'), 'config.toml to have Research again');
});
