'use strict';

// The Settings dialog, opened with the title bar's gear: the settings for
// every tab, saved in config.toml's [settings] table, and Open at login. Each
// tab's own properties are in Tab properties (editor.test.js).

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const {
  openPage, openSettings, openTabProperties, setScreen, startPreview, switchTo, view, waitForToast, waitUntil,
  waitView,
} = require('./helpers/preview.js');

async function open(t, screen = 'monitor') {
  const preview = await startPreview({ t });
  const { page, close } = await openPage(preview);
  t.after(close);
  await setScreen(page, screen);
  return { preview, page };
}

// readConfig returns config.toml, or the backup if bak is set.
function readConfig(preview, bak = false) {
  return fs.readFileSync(path.join(preview.home, bak ? 'config.toml.bak' : 'config.toml'), 'utf8');
}

// settingsSection returns the text of config.toml's [settings] table.
function settingsSection(toml) {
  return toml.split(/^\[shortcuts\]$/m)[0];
}

// tabsSection returns the text of config.toml from its first [[tab]] entry
// on.
function tabsSection(toml) {
  return toml.slice(toml.search(/^\[\[tab\]\]$/m));
}

test('Settings saves the settings for every tab in config.toml', async (t) => {
  const { preview, page } = await open(t);
  const before = readConfig(preview);
  assert.match(settingsSection(before), /^compact_below = 1800\s/m);
  assert.match(settingsSection(before), /^hide_other_tabs = true\s/m);
  assert.match(settingsSection(before), /^fill_screen = true\s/m);
  const was = (await view(page)).settings;

  await page.click('[data-action="open-settings"]');
  await page.waitForSelector('#settings-title:text("Settings")');
  assert.equal(await page.getAttribute('[data-action="open-settings"]', 'aria-label'), 'Settings');
  assert.deepEqual(await page.locator('.settings-group h3').allTextContents(), ['General', 'Appearance', 'Windows', 'Small screens']);
  assert.equal(await page.inputValue('#compact-below'), '1800');
  assert.equal(await page.isChecked('[data-setting="hideOtherTabs"]'), true);
  assert.equal(await page.isChecked('[data-setting="fillScreen"]'), true);
  await page.fill('#compact-below', '1600');
  await page.uncheck('[data-setting="hideOtherTabs"]');
  await page.uncheck('[data-setting="fillScreen"]');
  await page.click('[data-action="save-settings"]');

  await waitForToast(page, 'Saved settings');
  const v = await waitView(page, (x) => x.settings.compactBelow === 1600 && !x.settings.hideOtherTabs);
  assert.deepEqual(v.settings, { ...was, compactBelow: 1600, hideOtherTabs: false, fillScreen: false });
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
  await page.waitForSelector('[data-action="open-settings"]:focus');

  // config.toml has the new settings and the same tabs, and the backup has
  // the old settings.
  const after = await waitUntil(() => {
    const s = readConfig(preview);
    return /^compact_below = 1600\s/m.test(s) && s;
  }, 'config.toml to have the new threshold');
  assert.match(settingsSection(after), /^hide_other_tabs = false\s/m);
  assert.match(settingsSection(after), /^fill_screen = false\s/m);
  const others = (toml) => settingsSection(toml).split('\n').filter((l) => !/^(compact_below|hide_other_tabs|fill_screen) =/.test(l));
  assert.deepEqual(others(after), others(before));
  assert.equal(tabsSection(after), tabsSection(before));
  assert.equal(readConfig(preview, true), before);

  // The dialog opens with the saved settings.
  await openSettings(page);
  assert.equal(await page.inputValue('#compact-below'), '1600');
  assert.equal(await page.isChecked('[data-setting="hideOtherTabs"]'), false);
  assert.equal(await page.isChecked('[data-setting="fillScreen"]'), false);
});

test('Settings saves what the menu bar item shows, and whether tab shortcuts bring back a hidden Bench', async (t) => {
  const { preview, page } = await open(t);
  const before = readConfig(preview);
  assert.match(settingsSection(before), /^menu_bar = "name"\s/m);
  assert.match(settingsSection(before), /^show_on_tab_shortcut = true\s/m);

  await openSettings(page);
  assert.equal(await page.getAttribute('[data-key="menu-bar-name"]', 'aria-pressed'), 'true');
  assert.equal(await page.isChecked('[data-setting="showOnTabShortcut"]'), true);
  await page.click('[data-key="menu-bar-icon"]');
  await page.uncheck('[data-setting="showOnTabShortcut"]');
  await page.click('[data-action="save-settings"]');

  await waitForToast(page, 'Saved settings');
  const v = await waitView(page, (x) => x.settings.menuBar === 'icon');
  assert.equal(v.settings.showOnTabShortcut, false);
  const after = await waitUntil(() => {
    const s = settingsSection(readConfig(preview));
    return /^menu_bar = "icon"\s/m.test(s) && s;
  }, 'config.toml to have the new menu bar setting');
  assert.match(after, /^show_on_tab_shortcut = false\s/m);

  await openSettings(page);
  assert.equal(await page.getAttribute('[data-key="menu-bar-icon"]', 'aria-pressed'), 'true');
  assert.equal(await page.isChecked('[data-setting="showOnTabShortcut"]'), false);
});

test('Settings hides the status bar, narrows the space between panes, and shows tab shortcuts always', async (t) => {
  const { preview, page } = await open(t);
  const before = settingsSection(readConfig(preview));
  assert.match(before, /^gap = 4\s/m);
  assert.match(before, /^show_status_bar = true\s/m);
  assert.match(before, /^tab_shortcut_hints = "auto"\s/m);
  // apart reports whether two of the panes are a gap apart.
  const apart = (v, gap) => v.display.panes.some((a) => v.display.panes.some((b) => b.rect.x - (a.rect.x + a.rect.w) === gap));
  // Once Go has the pane area as drawn on the simulated monitor.
  const drawn = await page.$eval('#panes', (el) => Math.round(el.getBoundingClientRect().height));
  const was = await waitView(page, (x, h) => x.display.area.h === h, drawn);
  assert.ok(apart(was, 4), 'two panes are 4 pt apart');
  const bar = await page.locator('#statusbar').boundingBox();

  await openSettings(page);
  assert.equal(await page.getAttribute('[data-key="gap-4"]', 'aria-pressed'), 'true');
  assert.equal(await page.getAttribute('[data-key="tab-shortcut-hints-auto"]', 'aria-pressed'), 'true');
  assert.equal(await page.isChecked('[data-setting="showStatusBar"]'), true);
  await page.uncheck('[data-setting="showStatusBar"]');
  await page.click('[data-key="gap-2"]');
  await page.click('[data-key="tab-shortcut-hints-always"]');
  await page.click('[data-action="save-settings"]');

  // With the status bar hidden, the toast is in the title bar, and the
  // panes take the status bar's height.
  await waitForToast(page, 'Saved settings');
  assert.equal(await page.$eval('#toast', (el) => el.classList.contains('in-titlebar')), true);
  const v = await waitView(page, (x, h) => !x.settings.showStatusBar && x.display.area.h === h, was.display.area.h + Math.round(bar.height));
  assert.equal(v.display.area.y, was.display.area.y);
  assert.equal(await page.isVisible('#statusbar'), false);
  assert.ok(apart(v, 2), 'two panes are 2 pt apart');
  assert.equal(v.settings.tabShortcutHints, 'always');
  await page.waitForSelector('#titlebar .tab kbd');

  const after = await waitUntil(() => {
    const s = settingsSection(readConfig(preview));
    return /^show_status_bar = false\s/m.test(s) && s;
  }, 'config.toml to have the status bar hidden');
  assert.match(after, /^gap = 2\s/m);
  assert.match(after, /^tab_shortcut_hints = "always"\s/m);

  await openSettings(page);
  assert.equal(await page.isChecked('[data-setting="showStatusBar"]'), false);
  assert.equal(await page.getAttribute('[data-key="gap-2"]', 'aria-pressed'), 'true');
  assert.equal(await page.getAttribute('[data-key="tab-shortcut-hints-always"]', 'aria-pressed'), 'true');
});

test('Show scroll bars follows the preview, which shows them only while scrolling, and Always changes no heights', async (t) => {
  const { preview, page } = await open(t, 'laptop');
  assert.match(settingsSection(readConfig(preview)), /^scroll_bars = "auto"\s/m);
  const drawn = await page.$eval('#panes', (el) => Math.round(el.getBoundingClientRect().height));
  const was = await waitView(page, (x, h) => x.display.area.h === h, drawn);
  assert.equal(was.systemScrollBars, false);
  assert.equal(was.settings.scrollBars, 'auto');
  const shown = () => page.evaluate(() => document.body.classList.contains('scroll-bars'));
  assert.equal(await shown(), false);
  // heights returns the title bar's and the pane headers' heights.
  const heights = () => page.evaluate(() => [...document.querySelectorAll('#titlebar, .pane-header')]
    .map((el) => el.getBoundingClientRect().height));
  const before = await heights();

  await openSettings(page);
  assert.equal(await page.getAttribute('[data-key="scroll-bars-auto"]', 'aria-pressed'), 'true');
  assert.match(await page.textContent('#scroll-bars-help'), /only while scrolling now/);
  await page.click('[data-key="scroll-bars-always"]');
  await page.click('[data-action="save-settings"]');
  await waitForToast(page, 'Saved settings');
  const v = await waitView(page, (x) => x.settings.scrollBars === 'always');
  assert.equal(await shown(), true);
  assert.deepEqual(await heights(), before);
  assert.deepEqual(v.display.area, was.display.area);
  assert.deepEqual(v.display.panes.map((p) => [p.rect, p.body]), was.display.panes.map((p) => [p.rect, p.body]));
  await waitUntil(() => /^scroll_bars = "always"\s/m.test(settingsSection(readConfig(preview))),
    'config.toml to have the scroll bars shown');

  await openSettings(page);
  assert.equal(await page.getAttribute('[data-key="scroll-bars-always"]', 'aria-pressed'), 'true');
});

test('Settings saves the density and hiding the sub-tab row, which change where Go docks windows', async (t) => {
  const { preview, page } = await open(t);
  const before = settingsSection(readConfig(preview));
  assert.match(before, /^density = "auto"\s/m);
  assert.match(before, /^pane_headers = "always"\s/m);
  // On the monitor, Automatic is the standard density, and each pane's
  // window is docked below a 40 pt header.
  let v = await switchTo(page, 'Research');
  assert.equal(v.density, 'standard');
  assert.deepEqual(v.display.panes.map((p) => p.header), [40, 40]);
  for (const p of v.display.panes) assert.equal(p.body.y, p.rect.y + 1 + p.header + 1);

  await openSettings(page);
  assert.equal(await page.getAttribute('[data-key="density-auto"]', 'aria-pressed'), 'true');
  assert.equal(await page.isChecked('[data-setting="paneHeaders"]'), false);
  // The preview has no native title bar, so its title bar changes at once.
  assert.doesNotMatch(await page.textContent('#density-help'), /next opens/);
  await page.click('[data-key="density-compact"]');
  await page.check('[data-setting="paneHeaders"]');
  await page.click('[data-action="save-settings"]');
  await waitForToast(page, 'Saved settings');
  v = await waitView(page, (x) => x.density === 'compact' && x.settings.paneHeaders === 'auto' && x.display.panes.some((p) => p.header === 0));
  assert.equal(v.titleBar, 'compact');
  // Brave's pane, with three windows, has a compact header. Obsidian's,
  // with one app window, has none: its window is docked just inside the
  // pane's border.
  const [brave, obsidian] = v.display.panes;
  assert.equal(brave.header, 32);
  assert.equal(brave.body.y, brave.rect.y + 34);
  assert.equal(obsidian.header, 0);
  assert.equal(obsidian.body.y, obsidian.rect.y + 1);
  assert.equal(await page.getAttribute('.pane[data-pane-id="' + obsidian.id + '"]', 'data-header'), 'none');
  assert.equal(Math.round((await page.locator('#titlebar').boundingBox()).height), 39);
  const after = await waitUntil(() => {
    const s = settingsSection(readConfig(preview));
    return /^density = "compact"\s/m.test(s) && s;
  }, 'config.toml to have the compact density');
  assert.match(after, /^pane_headers = "auto"\s/m);
});

test('Escape closes Settings without saving', async (t) => {
  const { preview, page } = await open(t);
  const before = readConfig(preview);
  await openSettings(page);
  await page.fill('#compact-below', '1600');
  await page.uncheck('[data-setting="openMissingApps"]');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.settings-sheet', { state: 'detached' });

  const v = await view(page);
  assert.equal(v.settings.compactBelow, 1800);
  assert.equal(v.settings.openMissingApps, true);
  assert.equal(readConfig(preview), before);
  assert.equal(fs.existsSync(path.join(preview.home, 'config.toml.bak')), false);
});

test('the threshold in Settings decides which screens are small', async (t) => {
  const { page } = await open(t, 'laptop');
  // Under the default threshold of 1,800 points, the 1,512-point laptop is
  // small, so Research shows its windows as sub-tabs of one pane.
  let v = await switchTo(page, 'Research');
  assert.equal(v.sizeClass, 'compact');
  assert.equal(v.display.panes.length, 1);
  await openTabProperties(page);
  assert.equal(await page.textContent('#small-screens-text'), 'For screens narrower than 1,800 points, as set in Settings.');
  await page.click('[data-key="editor-cancel"]');
  await page.waitForSelector('.editor-sheet', { state: 'detached' });

  // Settings is in the jump palette's actions too. Enter in the threshold
  // field saves.
  await page.click('[data-action="open-palette"]');
  await page.fill('#palette-query', 'settings');
  await page.click('.palette-item:has-text("Settings…")');
  await page.waitForSelector('.settings-sheet');
  await page.fill('#compact-below', '1400');
  await page.press('#compact-below', 'Enter');

  v = await waitView(page, (x) => x.sizeClass === 'wide');
  assert.equal(v.settings.compactBelow, 1400);
  assert.equal(v.display.compact, false);
  assert.equal(v.display.panes.length, 2);
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
  await openTabProperties(page);
  assert.equal(await page.textContent('#small-screens-text'), 'For screens narrower than 1,400 points, as set in Settings.');
});

test('Open at login isn\'t available in the browser preview', async (t) => {
  const { preview, page } = await open(t);
  await openSettings(page);
  assert.equal(await page.isChecked('[data-setting="openAtLogin"]'), false);
  await page.check('[data-setting="openAtLogin"]');
  await page.uncheck('[data-setting="openMissingApps"]');
  await page.click('[data-action="save-settings"]');

  // The other settings are saved; then Go refuses Open at login, which the
  // preview has no login item for.
  await waitForToast(page, 'Open at login isn\'t available here.');
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
  const v = await waitView(page, (x) => !x.settings.openMissingApps);
  assert.equal(v.openAtLogin, false);
  await waitUntil(() => /^open_missing_apps = false\s/m.test(readConfig(preview)), 'config.toml to have the new setting');

  // The dialog shows Open at login as it is: off.
  await openSettings(page);
  assert.equal(await page.isChecked('[data-setting="openAtLogin"]'), false);
});
