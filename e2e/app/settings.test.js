'use strict';

// The Settings dialog, opened with the title bar's gear: the settings for
// every tab, saved in config.toml's [settings] table, and Open at login. Each
// tab's own properties are in Tab properties (editor.test.js).

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const {
  itemID, openPage, openSettings, openTabProperties, setScreen, showSection, startPreview, switchTo, view, waitForToast,
  waitUntil, waitView,
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

// shortcutsSection returns the text of config.toml's [shortcuts] table.
function shortcutsSection(toml) {
  return toml.split(/^\[shortcuts\]$/m)[1].split(/^\[/m)[0];
}

// tabsSection returns the text of config.toml from its first [[tab]] entry
// on.
function tabsSection(toml) {
  return toml.slice(toml.search(/^\[\[tab\]\]$/m));
}

test('Settings saves the settings for every tab in config.toml', async (t) => {
  const { preview, page } = await open(t);
  const before = readConfig(preview);
  assert.match(settingsSection(before), /^compact_below = 1280\s/m);
  assert.match(settingsSection(before), /^hide_other_tabs = true\s/m);
  assert.match(settingsSection(before), /^fill_screen = true\s/m);
  const was = (await view(page)).settings;

  await page.click('[data-action="open-settings"]');
  await page.waitForSelector('#settings-title:text("Settings")');
  assert.equal(await page.getAttribute('[data-action="open-settings"]', 'aria-label'), 'Settings');
  // Its sections are listed down its side, and it shows one at a time.
  assert.deepEqual(await page.locator('.settings-nav [role="tab"]').allTextContents(), ['General', 'Appearance', 'Windows', 'Keyboard', 'Small screens', 'Terminals', 'Launchers', 'Advanced']);
  assert.deepEqual(await page.locator('.settings-group h3').allTextContents(), ['General']);
  // Save saves the changes in every section.
  assert.equal(await page.isChecked('[data-setting="fillScreen"]'), true);
  await page.uncheck('[data-setting="fillScreen"]');
  await showSection(page, 'windows');
  assert.equal(await page.isChecked('[data-setting="hideOtherTabs"]'), true);
  await page.uncheck('[data-setting="hideOtherTabs"]');
  await showSection(page, 'screens');
  assert.equal(await page.inputValue('#compact-below'), '1280');
  await page.fill('#compact-below', '1600');
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

  // The dialog opens with the saved settings, at the section it showed.
  await openSettings(page);
  assert.equal(await page.inputValue('#compact-below'), '1600');
  await showSection(page, 'windows');
  assert.equal(await page.isChecked('[data-setting="hideOtherTabs"]'), false);
  await showSection(page, 'general');
  assert.equal(await page.isChecked('[data-setting="fillScreen"]'), false);
});

test('Settings saves what the menu bar item shows, and whether tab shortcuts bring back a hidden Bench', async (t) => {
  const { preview, page } = await open(t);
  const before = readConfig(preview);
  assert.match(settingsSection(before), /^menu_bar = "name"\s/m);
  assert.match(settingsSection(before), /^show_on_tab_shortcut = true\s/m);

  await openSettings(page);
  assert.equal(await page.getAttribute('[data-key="menu-bar-name"]', 'aria-checked'), 'true');
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
  assert.equal(await page.getAttribute('[data-key="menu-bar-icon"]', 'aria-checked'), 'true');
  assert.equal(await page.isChecked('[data-setting="showOnTabShortcut"]'), false);
});

test('Settings turns on looking for a new version once a day, which is off by default', async (t) => {
  const { preview, page } = await open(t);
  assert.match(settingsSection(readConfig(preview)), /^check_for_updates = false\s/m);
  await openSettings(page);
  assert.equal(await page.isChecked('[data-setting="checkForUpdates"]'), false);
  await page.check('[data-setting="checkForUpdates"]');
  await page.click('[data-action="save-settings"]');

  await waitForToast(page, 'Saved settings');
  const v = await waitView(page, (x) => x.settings.checkForUpdates);
  // The preview looks for no new version, so there's no notice.
  assert.equal(v.notices.some((n) => /is out/.test(n.text)), false);
  await waitUntil(() => /^check_for_updates = true\s/m.test(settingsSection(readConfig(preview))), 'config.toml to have the new setting');
  await openSettings(page);
  assert.equal(await page.isChecked('[data-setting="checkForUpdates"]'), true);
});

test('Settings turns off asking before quitting while a terminal is running something, and back on', async (t) => {
  const { preview, page } = await open(t);
  assert.match(settingsSection(readConfig(preview)), /^confirm_quit_busy_terminals = true\s/m);

  await openSettings(page);
  assert.equal(await page.isChecked('[data-setting="confirmQuitBusyTerminals"]'), true);
  assert.equal(await page.getAttribute('[data-setting="confirmQuitBusyTerminals"]', 'aria-describedby'), 'confirm-quit-help');
  await page.uncheck('[data-setting="confirmQuitBusyTerminals"]');
  await page.click('[data-action="save-settings"]');
  await waitForToast(page, 'Saved settings');
  await waitView(page, (x) => x.settings.confirmQuitBusyTerminals === false);
  await waitUntil(() => /^confirm_quit_busy_terminals = false\s/m.test(settingsSection(readConfig(preview))),
    'config.toml to stop asking before quitting');

  await openSettings(page);
  assert.equal(await page.isChecked('[data-setting="confirmQuitBusyTerminals"]'), false);
  await page.check('[data-setting="confirmQuitBusyTerminals"]');
  await page.click('[data-action="save-settings"]');
  await waitView(page, (x) => x.settings.confirmQuitBusyTerminals === true);
  await waitUntil(() => /^confirm_quit_busy_terminals = true\s/m.test(settingsSection(readConfig(preview))),
    'config.toml to ask before quitting again');
});

test('Settings saves the terminals\' text size and Option key, which every terminal follows', async (t) => {
  const { preview, page } = await open(t);
  assert.match(settingsSection(readConfig(preview)), /^terminal_font_size = 12\.5\s/m);
  const v = await switchTo(page, 'Code');
  const term = itemID(v, 'zsh — ~/code');
  await page.waitForSelector('[data-term="' + term + '"] .xterm');
  assert.deepEqual(await page.evaluate((id) => window.bench.terminalOptions(id), term), { fontSize: 12.5, optionAsMeta: false });

  await openSettings(page, 'terminals');
  assert.equal(await page.inputValue('#terminal-font-size'), '12.5');
  await page.fill('#terminal-font-size', '15');
  await page.check('[data-setting="terminalOptionAsMeta"]');
  await page.click('[data-action="save-settings"]');
  await waitForToast(page, 'Saved settings');
  await waitView(page, (x) => x.settings.terminalFontSize === 15 && x.settings.terminalOptionAsMeta === true);
  await waitUntil(() => {
    const s = settingsSection(readConfig(preview));
    return /^terminal_font_size = 15\s/m.test(s) && /^terminal_option_as_meta = true\s/m.test(s);
  }, 'config.toml to have the terminals\' settings');
  await page.waitForFunction((id) => window.bench.terminalOptions(id).fontSize === 15, term);
  assert.deepEqual(await page.evaluate((id) => window.bench.terminalOptions(id), term), { fontSize: 15, optionAsMeta: true });

  // Go refuses a size it can't use, and Settings stays open.
  await openSettings(page);
  await page.fill('#terminal-font-size', '40');
  await page.click('[data-action="save-settings"]');
  await waitForToast(page, 'The terminals\' text size must be from 9 to 24 points.');
  assert.equal(await page.isVisible('.settings-sheet'), true);
});

test('Settings turns off asking before closing a tab\'s windows', async (t) => {
  const { preview, page } = await open(t);
  assert.match(settingsSection(readConfig(preview)), /^confirm_close = true\s/m);

  await openSettings(page);
  assert.equal(await page.isChecked('[data-setting="confirmClose"]'), true);
  assert.equal(await page.getAttribute('[data-setting="confirmClose"]', 'aria-describedby'), 'confirm-close-help');
  await page.uncheck('[data-setting="confirmClose"]');
  await page.click('[data-action="save-settings"]');
  await waitForToast(page, 'Saved settings');
  await waitView(page, (x) => x.settings.confirmClose === false);
  await waitUntil(() => /^confirm_close = false\s/m.test(settingsSection(readConfig(preview))),
    'config.toml to stop asking before closing a tab');
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

  await openSettings(page, 'appearance');
  assert.equal(await page.getAttribute('[data-key="gap-4"]', 'aria-checked'), 'true');
  assert.equal(await page.getAttribute('[data-key="tab-shortcut-hints-auto"]', 'aria-checked'), 'true');
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
  // The current tab shows its × in its shortcut's place.
  await page.waitForSelector('#titlebar .tab[aria-selected="false"] kbd');

  const after = await waitUntil(() => {
    const s = settingsSection(readConfig(preview));
    return /^show_status_bar = false\s/m.test(s) && s;
  }, 'config.toml to have the status bar hidden');
  assert.match(after, /^gap = 2\s/m);
  assert.match(after, /^tab_shortcut_hints = "always"\s/m);

  await openSettings(page);
  assert.equal(await page.isChecked('[data-setting="showStatusBar"]'), false);
  assert.equal(await page.getAttribute('[data-key="gap-2"]', 'aria-checked'), 'true');
  assert.equal(await page.getAttribute('[data-key="tab-shortcut-hints-always"]', 'aria-checked'), 'true');
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

  await openSettings(page, 'appearance');
  assert.equal(await page.getAttribute('[data-key="scroll-bars-auto"]', 'aria-checked'), 'true');
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
  assert.equal(await page.getAttribute('[data-key="scroll-bars-always"]', 'aria-checked'), 'true');
});

test('Settings saves the colours, which Bench takes at once, while the panes stay where they were', async (t) => {
  const { preview, page } = await open(t);
  // A new config matches macOS, and the browser, like a Mac, is light.
  assert.match(settingsSection(readConfig(preview)), /^appearance = "system"\s/m);
  const drawn = await page.$eval('#panes', (el) => Math.round(el.getBoundingClientRect().height));
  const was = await waitView(page, (x, h) => x.display.area.h === h, drawn);
  assert.equal(was.settings.appearance, 'system');
  const theme = () => page.evaluate(() => document.documentElement.dataset.theme);
  assert.equal(await theme(), 'light');

  // Dark, even in a browser that prefers light.
  await openSettings(page, 'appearance');
  assert.equal(await page.getAttribute('[data-key="appearance-system"]', 'aria-checked'), 'true');
  await page.click('[data-key="appearance-dark"]');
  await page.click('[data-action="save-settings"]');
  await waitForToast(page, 'Saved settings');
  await waitView(page, (x) => x.settings.appearance === 'dark');
  assert.equal(await theme(), 'dark');
  assert.equal(await page.evaluate(() => getComputedStyle(document.documentElement).getPropertyValue('--bg-pane').trim()), '#0E1014');
  await waitUntil(() => /^appearance = "dark"\s/m.test(settingsSection(readConfig(preview))),
    'config.toml to have the dark colours');

  // Light, even in a browser that prefers dark.
  await page.emulateMedia({ colorScheme: 'dark' });
  await openSettings(page);
  await page.click('[data-key="appearance-light"]');
  await page.click('[data-action="save-settings"]');
  await waitForToast(page, 'Saved settings');
  const v = await waitView(page, (x) => x.settings.appearance === 'light');
  assert.equal(await theme(), 'light');
  assert.equal(await page.evaluate(() => getComputedStyle(document.documentElement).getPropertyValue('--bg-pane').trim()), '#FFFFFF');
  assert.deepEqual(v.display.panes.map((p) => [p.rect, p.body]), was.display.panes.map((p) => [p.rect, p.body]));
  await waitUntil(() => /^appearance = "light"\s/m.test(settingsSection(readConfig(preview))),
    'config.toml to have the light colours');

  // Match macOS follows the browser's colour scheme in the preview.
  await openSettings(page);
  await page.click('[data-key="appearance-system"]');
  await page.click('[data-action="save-settings"]');
  await waitView(page, (x) => x.settings.appearance === 'system');
  await page.waitForFunction(() => document.documentElement.dataset.theme === 'dark');
  await page.emulateMedia({ colorScheme: 'light' });
  await page.waitForFunction(() => document.documentElement.dataset.theme === 'light');
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

  await openSettings(page, 'appearance');
  assert.equal(await page.getAttribute('[data-key="density-auto"]', 'aria-checked'), 'true');
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

test('Settings ▸ Keyboard records a shortcut, which works as soon as Settings saves it', async (t) => {
  const { preview, page } = await open(t);
  assert.match(shortcutsSection(readConfig(preview)), /^jump = "ctrl\+alt\+space"\s/m);
  const was = await view(page);
  assert.equal(was.keys.jump, '⌃⌥Space');
  // The preview registers every shortcut, and macOS keeps none of them.
  assert.deepEqual(was.shortcuts.map((r) => [r.field, r.display || '', r.problem || '']), [
    ['tab_modifiers', '⌃⌥', ''], ['next_tab', '⌃⌥]', ''], ['previous_tab', '⌃⌥[', ''], ['last_tab', '⌃⌥`', ''],
    ['jump', '⌃⌥Space', ''], ['show_all_tabs', '', ''], ['rearrange', '⌃⌥0', ''], ['focus_pane_modifiers', '⌃⌥', ''],
    ['move_window_modifiers', '⌃⌥⇧', ''], ['send_to_tab_modifiers', '⌃⌥⇧', ''], ['next_window', '⌃⌥Tab', ''], ['zoom_pane', '⌃⌥↩', ''],
    ['grow_pane', '', ''], ['shrink_pane', '', ''], ['equal_panes', '', ''], ['next_layout', '', ''], ['previous_layout', '', ''],
    ['add_front_window', '', ''], ['undo', '', ''],
  ]);
  assert.equal(was.notices.some((n) => /shortcut|register/i.test(n.text)), false);

  await openSettings(page, 'keyboard');
  assert.equal(await page.textContent('#key-jump-state'), 'Works.');
  await page.click('[data-key="key-jump"]');
  assert.equal(await page.textContent('[data-key="key-jump"]'), 'Press keys…');
  // While the field records, Bench's own jump shortcut is a key like any
  // other: it doesn't open the palette.
  await page.keyboard.press('Control+Alt+Space');
  assert.equal(await page.textContent('[data-key="key-jump"]'), '⌃⌥Space');
  assert.equal(await page.isVisible('#palette-query'), false);
  await page.click('[data-key="key-jump"]');
  await page.keyboard.press('Control+Alt+KeyJ');
  assert.equal(await page.textContent('[data-key="key-jump"]'), '⌃⌥J');
  await page.waitForSelector('#key-jump-state:text-is("Works.")');
  await page.click('[data-key="key-zoom_pane-off"]');
  assert.equal(await page.textContent('[data-key="key-zoom_pane"]'), 'Off');
  await page.click('[data-action="save-settings"]');

  await waitForToast(page, 'Saved settings');
  const v = await waitView(page, (x) => x.keys.jump === '⌃⌥J');
  assert.equal(v.keys.zoomPane, '');
  assert.equal(v.shortcuts.find((r) => r.field === 'jump').value, 'ctrl+alt+j');
  const after = await waitUntil(() => {
    const s = readConfig(preview);
    return /^jump = "ctrl\+alt\+j"\s/m.test(shortcutsSection(s)) && s;
  }, 'config.toml to have the new jump shortcut');
  assert.match(shortcutsSection(after), /^zoom_pane = ""\s/m);
  assert.match(shortcutsSection(after), /^next_tab = "ctrl\+alt\+]"\s/m);

  // The new shortcut works at once, and the old one doesn't.
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
  await page.keyboard.press('Control+Alt+Space');
  assert.equal(await page.isVisible('#palette-query'), false);
  await page.keyboard.press('Control+Alt+KeyJ');
  await page.waitForSelector('#palette-query');
  await page.keyboard.press('Escape');
  await page.waitForSelector('#palette-query', { state: 'detached' });

  // Restore defaults puts them back.
  await openSettings(page);
  await page.click('[data-action="keys-defaults"]');
  assert.equal(await page.textContent('[data-key="key-jump"]'), '⌃⌥Space');
  assert.equal(await page.textContent('[data-key="key-zoom_pane"]'), '⌃⌥↩');
  await page.click('[data-action="save-settings"]');
  await waitView(page, (x) => x.keys.jump === '⌃⌥Space' && x.keys.zoomPane === '⌃⌥↩');
  await waitUntil(() => /^jump = "ctrl\+alt\+space"\s/m.test(shortcutsSection(readConfig(preview))), 'config.toml to have the default jump shortcut');
});

test('Settings ▸ Keyboard says that ⌘Z for Undo the latest change, or ⌘, for any field, would take it from every app, and offers another', async (t) => {
  const { page } = await open(t);
  await openSettings(page, 'keyboard');
  await page.click('[data-key="key-undo"]');
  assert.equal(await page.textContent('[data-key="key-undo"]'), 'Press keys…');
  await page.keyboard.press('Meta+KeyZ');
  assert.equal(await page.textContent('[data-key="key-undo"]'), '⌘Z');
  await page.waitForSelector('#key-undo-state:has-text("Mac apps use ⌘Z for their own commands, so Bench would take it from every app.")');
  assert.equal(await page.textContent('[data-key="key-undo-use"]'), 'Use ⌃⇧⌘Z');
  await page.click('[data-key="key-undo-use"]');
  assert.equal(await page.textContent('[data-key="key-undo"]'), '⌃⇧⌘Z');
  await page.waitForSelector('#key-undo-state:text-is("Works.")');

  // ⌘, opens every app's Settings, Bench's included.
  await page.click('[data-key="key-show_all_tabs"]');
  assert.equal(await page.textContent('[data-key="key-show_all_tabs"]'), 'Press keys…');
  await page.keyboard.press('Meta+Comma');
  assert.equal(await page.textContent('[data-key="key-show_all_tabs"]'), '⌘,');
  await page.waitForSelector('#key-show-all-tabs-state:has-text("Mac apps use ⌘, for their own commands, so Bench would take it from every app.")');
  assert.match(await page.textContent('[data-key="key-show_all_tabs-use"]'), /^Use /);
});

test('Settings saves where new windows go, what another tab\'s window does, hiding other apps, dragging windows on, and how long to wait', async (t) => {
  const { preview, page } = await open(t);
  const before = settingsSection(readConfig(preview));
  assert.match(before, /^new_windows = "app-pane"\s/m);
  assert.match(before, /^follow_focus = "switch-tab"\s/m);
  assert.match(before, /^hide_other_apps = false\s/m);
  assert.match(before, /^drag_to_add = true\s/m);
  assert.match(before, /^launch_timeout = "15s"\s/m);

  await openSettings(page, 'windows');
  assert.equal(await page.getAttribute('[data-key="new-windows-app-pane"]', 'aria-checked'), 'true');
  assert.equal(await page.getAttribute('[data-key="follow-focus-switch-tab"]', 'aria-checked'), 'true');
  assert.equal(await page.isChecked('[data-setting="hideOtherApps"]'), false);
  assert.equal(await page.isChecked('[data-setting="dragToAdd"]'), true);
  await page.click('[data-key="new-windows-focused-pane"]');
  await page.click('[data-key="follow-focus-bring-here"]');
  await page.check('[data-setting="hideOtherApps"]');
  await page.uncheck('[data-setting="dragToAdd"]');
  await showSection(page, 'advanced');
  assert.equal(await page.inputValue('#launch-wait'), '15');
  await page.fill('#launch-wait', '30');
  await page.click('[data-action="save-settings"]');

  await waitForToast(page, 'Saved settings');
  const v = await waitView(page, (x) => x.settings.followFocus === 'bring-here');
  assert.equal(v.settings.newWindows, 'focused-pane');
  assert.equal(v.settings.hideOtherApps, true);
  assert.equal(v.settings.dragToAdd, false);
  assert.equal(v.settings.launchTimeout, '30s');
  const after = await waitUntil(() => {
    const s = settingsSection(readConfig(preview));
    return /^follow_focus = "bring-here"\s/m.test(s) && s;
  }, 'config.toml to have the new follow_focus');
  assert.match(after, /^new_windows = "focused-pane"\s/m);
  assert.match(after, /^hide_other_apps = true\s/m);
  assert.match(after, /^drag_to_add = false\s/m);
  assert.match(after, /^launch_timeout = "30s"\s/m);

  await openSettings(page);
  assert.equal(await page.inputValue('#launch-wait'), '30');
  await showSection(page, 'windows');
  assert.equal(await page.getAttribute('[data-key="follow-focus-bring-here"]', 'aria-checked'), 'true');
  assert.equal(await page.isChecked('[data-setting="dragToAdd"]'), false);
});

test('Settings previews the space between panes and the density, and Escape asks before it discards them, saving nothing', async (t) => {
  const { preview, page } = await open(t);
  const before = readConfig(preview);
  // apart reports whether two of the panes are a gap apart.
  const apart = (v, gap) => v.display.panes.some((a) => v.display.panes.some((b) => b.rect.x - (a.rect.x + a.rect.w) === gap));
  const drawn = await page.$eval('#panes', (el) => Math.round(el.getBoundingClientRect().height));
  const was = await waitView(page, (x, h) => x.display.area.h === h, drawn);
  assert.ok(apart(was, 4), 'two panes are 4 pt apart');

  // Go draws the panes as the gap and density chosen would place them, at
  // once, while the settings stay as they were.
  await openSettings(page, 'appearance');
  await page.click('[data-key="gap-8"]');
  await page.click('[data-key="density-compact"]');
  const spaced = (x, a) => x.density === a.density
    && x.display.panes.some((p) => x.display.panes.some((q) => q.rect.x - (p.rect.x + p.rect.w) === a.gap));
  let v = await waitView(page, spaced, { gap: 8, density: 'compact' });
  assert.equal(v.settings.gap, 4);
  assert.equal(v.settings.density, 'auto');
  await showSection(page, 'windows');
  await page.uncheck('[data-setting="openMissingApps"]');

  // Escape asks first, naming the sections changed. Keep editing goes back
  // to the dialog; Discard throws the changes away, and the panes go back.
  await page.keyboard.press('Escape');
  await page.waitForSelector('.discard-sheet');
  assert.equal(await page.textContent('#discard-title'), 'Discard changes to Settings?');
  assert.equal(await page.textContent('#discard-text'), 'Your changes to Appearance and Windows aren\'t saved.');
  await page.click('[data-key="discard-keep"]');
  assert.equal(await page.isChecked('[data-setting="openMissingApps"]'), false);
  await page.keyboard.press('Escape');
  await page.click('[data-key="discard-discard"]');
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
  // The preview's title bar, which follows the density at once, is back
  // to its height too, and with it the pane area.
  v = await waitView(page, (x, a) => x.density === 'standard' && x.display.area.y === a.y && x.display.area.h === a.h, was.display.area);
  assert.deepEqual(v.display, was.display);
  assert.equal(v.settings.openMissingApps, true);
  assert.equal(readConfig(preview), before);
  assert.equal(fs.existsSync(path.join(preview.home, 'config.toml.bak')), false);
});

test('the threshold in Settings decides when Bench\'s window is small', async (t) => {
  const { page } = await open(t, 'laptop');
  // Over the default threshold of 1,280 points, Bench's window, which
  // fills the 1,512-point laptop screen, is wide, so Research shows its
  // windows side by side.
  let v = await switchTo(page, 'Research');
  assert.equal(v.sizeClass, 'wide');
  assert.equal(v.display.panes.length, 2);
  await openTabProperties(page);
  assert.equal(await page.textContent('#small-screens-text'), 'When Bench is narrower than 1,280 points, as set in Settings.');
  await page.click('[data-key="editor-cancel"]');
  await page.waitForSelector('.editor-sheet', { state: 'detached' });

  // Settings is in the jump palette's actions too. Enter in the threshold
  // field saves.
  await page.click('[data-action="open-palette"]');
  await page.fill('#palette-query', 'settings');
  await page.click('.palette-item:has-text("Settings…")');
  await page.waitForSelector('.settings-sheet');
  await showSection(page, 'screens');
  // Over the window's width, the threshold makes it small, so Research
  // shows its windows as sub-tabs of one pane.
  await page.fill('#compact-below', '1600');
  await page.press('#compact-below', 'Enter');

  v = await waitView(page, (x) => x.sizeClass === 'compact');
  assert.equal(v.settings.compactBelow, 1600);
  assert.equal(v.display.compact, true);
  assert.equal(v.display.panes.length, 1);
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
  await openTabProperties(page);
  assert.equal(await page.textContent('#small-screens-text'), 'When Bench is narrower than 1,600 points, as set in Settings.');
});

test('Open at login isn\'t available in the browser preview', async (t) => {
  const { preview, page } = await open(t);
  await openSettings(page);
  assert.equal(await page.isChecked('[data-setting="openAtLogin"]'), false);
  await page.check('[data-setting="openAtLogin"]');
  await showSection(page, 'windows');
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
  await openSettings(page, 'general');
  assert.equal(await page.isChecked('[data-setting="openAtLogin"]'), false);
});
