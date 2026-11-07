'use strict';

// Renaming in place, and new tabs for a window, with Go: a tab renamed by
// double-clicking it is saved in config.toml, numbered if another tab has
// its name; a terminal's own name is kept in state.json; and Move to new
// tab, or a drop on the tabs' +, makes a tab named after the window's app.

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const {
  itemID, openPage, setScreen, startPreview, switchTo, view, waitForToast, waitView,
} = require('./helpers/preview.js');

// configText returns config.toml as the preview saved it.
function configText(preview) {
  return fs.readFileSync(path.join(preview.home, 'config.toml'), 'utf8');
}

test('double-clicking a tab renames it, keeps its layout, and saves config.toml', async (t) => {
  const preview = await startPreview({ t });
  const { page, close } = await openPage(preview);
  t.after(close);
  const before = await view(page);
  await page.dblclick('.tab[data-index="2"] .tab-name');
  await page.waitForSelector('.tab.renaming[data-index="2"] #rename-input');
  assert.equal(await page.inputValue('#rename-input'), 'Research');
  // Another tab has the name: Go numbers it, as the field says it will.
  await page.keyboard.type('code');
  await page.waitForSelector('#rename-note:has-text("→ code (1)")');
  await page.keyboard.press('Enter');
  const v = await waitView(page, (x) => x.tabs[2].name === 'code (1)');
  await page.waitForSelector('#rename-input', { state: 'detached' });
  assert.equal(await page.textContent('.tab[data-index="2"] .tab-name'), 'code (1)');
  assert.equal(v.tabs[2].preset, before.tabs[2].preset);
  assert.equal(v.tabs[2].smallScreens, before.tabs[2].smallScreens);
  assert.deepEqual(v.tabs[2].root, before.tabs[2].root);
  assert.match(configText(preview), /name = "code \(1\)"/);
  assert.doesNotMatch(configText(preview), /name = "Research"/);
});

test('a terminal\'s own name wins over its shell\'s title, survives a restart, and can be taken away', async (t) => {
  const first = await startPreview({ t });
  const one = await openPage(first);
  await setScreen(one.page, 'monitor');
  let v = await switchTo(one.page, 'Code');
  const term = itemID(v, 'zsh — ~/code');
  await one.page.click('.subtab[data-item="' + term + '"]', { button: 'right' });
  await one.page.click('#window-menu .menu-item:has-text("Rename…")');
  await one.page.waitForSelector('.subtab.renaming[data-item="' + term + '"] #rename-input');
  await one.page.keyboard.type('API server');
  await one.page.keyboard.press('Enter');
  v = await waitView(one.page, (x, id) => x.items[id].title === 'API server', term);
  assert.equal(v.items[term].label, 'API server');
  assert.equal(v.items[term].autoTitle, 'zsh — ~/code');
  await one.page.waitForSelector('.subtab[data-item="' + term + '"] .subtab-title:text("API server")');
  await one.close();
  assert.equal(await first.stop(), 0);
  const state = fs.readFileSync(path.join(first.home, 'state.json'), 'utf8');
  assert.match(state, /"label":\s*"API server"/);

  const second = await startPreview({ home: first.home, t });
  const two = await openPage(second);
  t.after(two.close);
  await setScreen(two.page, 'monitor');
  await switchTo(two.page, 'Code');
  v = await waitView(two.page, (x) => Object.values(x.items).some((it) => it.label === 'API server'));
  const again = Object.values(v.items).find((it) => it.label === 'API server').id;
  await two.page.waitForSelector('.subtab[data-item="' + again + '"] .subtab-title:text("API server")');
  await two.page.click('.subtab[data-item="' + again + '"]', { button: 'right' });
  await two.page.click('#window-menu .menu-item:has-text("Use Automatic Title")');
  v = await waitView(two.page, (x, id) => x.items[id].title === 'zsh — ~/code' && !x.items[id].label, again);
  // An app's window takes its title from the app.
  const zed = itemID(v, 'bench — switch.go');
  await two.page.click('.subtab[data-item="' + zed + '"]', { button: 'right' });
  await two.page.waitForSelector('#window-menu');
  assert.equal(await two.page.locator('#window-menu .menu-item:has-text("Rename…")').count(), 0);
});

test('Move to Tab ▸ New Tab, and a drop on + with Option, make tabs named after the app', async (t) => {
  const preview = await startPreview({ t });
  const { page, close } = await openPage(preview);
  t.after(close);
  await setScreen(page, 'monitor');
  let v = await switchTo(page, 'Code');
  const code = itemID(v, 'index.html — bench-ui');
  await page.click('.subtab[data-item="' + code + '"]', { button: 'right' });
  await page.click('#window-menu [data-action="menu-sub"]:has-text("Move to Tab")');
  await page.click('#menu-sub .menu-item:has(.menu-label:text-is("New Tab"))');
  v = await waitView(page, (x) => x.tabs.length === 4 && x.current === 3);
  assert.equal(v.tabs[3].name, 'VS Code');
  assert.equal(v.tabs[3].preset, 'single');
  assert.deepEqual(v.items[code].tabs || [v.items[code].tab], [3]);
  await waitForToast(page, 'Moved VS Code — index.html — bench-ui to a new tab, VS Code.');
  assert.match(configText(preview), /name = "VS Code"/);
  // Undo takes the tab away again, from config.toml too.
  await page.click('#toast .toast-action');
  v = await waitView(page, (x) => x.tabs.length === 3 && x.current === 1);
  assert.deepEqual(v.items[code].tabs || [v.items[code].tab], [1]);
  await waitForToast(page, 'Moved VS Code — index.html — bench-ui back.');
  assert.doesNotMatch(configText(preview), /name = "VS Code"/);

  // A terminal dropped on + with Option held stays in Code, and is shown
  // in a new tab too.
  await switchTo(page, 'Code');
  v = await view(page);
  const term = itemID(v, 'zsh — ~/code');
  const src = await page.locator('.subtab[data-item="' + term + '"]').boundingBox();
  const plus = await page.locator('.new-tab').boundingBox();
  await page.mouse.move(src.x + src.width / 2, src.y + src.height / 2);
  await page.mouse.down();
  await page.mouse.move(src.x + src.width / 2 + 20, src.y, { steps: 4 });
  await page.keyboard.down('Alt');
  await page.mouse.move(plus.x + plus.width / 2, plus.y + plus.height / 2, { steps: 6 });
  await page.waitForSelector('.new-tab.drop-target.drop-add');
  await page.mouse.up();
  await page.keyboard.up('Alt');
  v = await waitView(page, (x) => x.tabs.length === 4 && x.current === 3);
  assert.equal(v.tabs[3].name, 'Terminal');
  assert.deepEqual(v.items[term].tabs, [1, 3]);

  // Again, from the jump palette: the name is numbered.
  await page.click('[data-action="open-palette"]');
  await page.waitForSelector('#palette-query');
  await page.fill('#palette-query', 'Also Show In New Tab');
  await page.press('#palette-query', 'Enter');
  v = await waitView(page, (x) => x.tabs.length === 5);
  assert.equal(v.tabs[4].name, 'Terminal (1)');
  assert.deepEqual(v.items[term].tabs, [1, 3, 4]);
});
