'use strict';

// Starting up: the default config's tabs, with the demo platform's windows
// matched into their panes.

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const {
  itemsOf, openPage, setScreen, startPreview, switchTo, view,
} = require('./helpers/preview.js');

test('the default tabs come from the default config, with the demo windows in place', async (t) => {
  const preview = await startPreview({ t });
  const { page, close } = await openPage(preview);
  t.after(close);

  // Bench writes the default config on first run.
  const config = fs.readFileSync(path.join(preview.home, 'config.toml'), 'utf8');
  assert.match(config, /name = "Comms"/);

  // The browser's own screen is small, so the tabs show only their names,
  // with their shortcuts in their tooltips.
  const tabs = () => page.locator('#titlebar .tab').evaluateAll((els) => els.map((el) => [el.textContent, el.title]));
  assert.deepEqual(await tabs(), [['Comms', 'Comms (⌃⌥1)'], ['Code', 'Code (⌃⌥2)'], ['Research', 'Research (⌃⌥3)']]);

  const v = await setScreen(page, 'monitor');
  assert.equal(v.sizeClass, 'wide');
  // On a monitor, the tabs show their shortcuts too, other than the
  // current tab, which shows its × in its shortcut's place.
  await page.waitForSelector('#titlebar .tab[aria-pressed="false"] kbd');
  assert.deepEqual((await tabs()).map(([text]) => text), ['Comms⌃⌥1', 'Code⌃⌥2', 'Research⌃⌥3']);
  assert.deepEqual(itemsOf(v, 'Comms'), [
    [{ app: 'Slack', title: 'Slack' }],
    [{ app: 'Outlook', title: 'Outlook' }],
  ]);
  assert.deepEqual(itemsOf(v, 'Code'), [
    [{ app: 'Zed', title: 'bench — switch.go' }, { app: 'VS Code', title: 'index.html — bench-ui' }],
    [{ app: 'Terminal', title: 'zsh — ~/code' }],
  ]);
  assert.deepEqual(itemsOf(v, 'Research'), [
    [
      { app: 'Brave', title: 'Wails v3 docs' },
      { app: 'Brave', title: 'Pull request #42 · bench' },
      { app: 'Brave', title: 'Go packages' },
    ],
    [{ app: 'Obsidian', title: 'Daily note' }],
  ]);
  assert.ok(Object.values(v.items).every((it) => it.status === 'ready'), 'every item is ready');

  // Code's first pane shows Zed, with VS Code as its other sub-tab.
  const code = await switchTo(page, 'Code');
  const first = code.display.panes[0];
  const subtabs = await page.locator('.pane[data-pane-id="' + first.id + '"] .subtab').evaluateAll(
    (els) => els.map((el) => [el.textContent, el.getAttribute('aria-pressed')]),
  );
  assert.deepEqual(subtabs, [['Zdbench — switch.go', 'true'], ['VSindex.html — bench-ui', 'false']]);
  assert.equal(await page.textContent('.pane[data-pane-id="' + first.id + '"] .badge'), 'App');

  // Finder isn't in any tab, so the palette lists it under Not in a tab.
  await page.click('[data-action="open-palette"]');
  await page.fill('#palette-query', 'Downloads');
  const rows = await page.locator('#palette-list').evaluate((list) => Array.from(list.children, (el) => el.textContent));
  assert.deepEqual(rows.slice(0, 2), ['Not in a tab', 'FnDownloadsFinder · add to this tab↵']);
  assert.deepEqual((await view(page)).floating.map((f) => f.app + ': ' + f.title), ['Finder: Downloads']);
});
