'use strict';

// The drop-down terminal (decision 72), against Go: Use as Drop-down takes
// a terminal out of its pane, Escape at the prompt hides it, state.json
// keeps it between launches, the jump palette shows it again, Move into
// This Pane brings it back, and Settings ▸ Terminals' Drop-down height goes
// to config.toml.

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const {
  itemID, openPage, openSettings, paneOf, startPreview, switchTo, waitForToast, waitView,
} = require('./helpers/preview.js');

// waitForText waits until the drop-down's terminal shows text.
function waitForText(page, text) {
  return page.waitForFunction((t) => {
    const rows = document.querySelector('#dropdown .xterm-rows');
    return Boolean(rows && rows.textContent.includes(t));
  }, text);
}

// readState returns state.json in the preview's support folder.
function readState(home) {
  return JSON.parse(fs.readFileSync(path.join(home, 'state.json'), 'utf8'));
}

test('Use as Drop-down takes a terminal out of its pane, state.json keeps it, and Move into This Pane brings it back', async (t) => {
  const first = await startPreview({ t });
  const one = await openPage(first);
  let v = await switchTo(one.page, 'Code');
  const term = itemID(v, 'zsh — ~/code');
  const pane = paneOf(v, term);
  await one.page.click('[data-action="menu"][data-pane="' + pane + '"]');
  await one.page.waitForSelector('#window-menu');
  await one.page.click('#window-menu .menu-item:has-text("Use as Drop-down")');
  v = await waitView(one.page, (x, id) => x.dropdown && x.dropdown.id === id && x.dropdown.shown, term);
  // The preview draws it in the page.
  assert.deepEqual(v.dropdown, { id: term, shown: true, inPage: true });
  assert.equal(paneOf(v, term), '');
  assert.ok(v.floating.some((it) => it.id === term), 'the drop-down is out of the tabs');
  await waitForToast(one.page, 'Terminal — zsh — ~/code is the drop-down now. Show it again with View ▸ Show Drop-down, or set its shortcut in Settings ▸ Keyboard.');
  await one.page.waitForSelector('#dropdown .xterm');
  await one.page.waitForFunction(() => document.activeElement && document.activeElement.closest('#dropdown .term-host'));
  await one.page.keyboard.type('echo from the drop-down');
  await one.page.keyboard.press('Enter');
  await waitForText(one.page, 'from the drop-down');
  // Escape at the shell's prompt hides it; its session goes on.
  await one.page.keyboard.press('Escape');
  v = await waitView(one.page, (x) => x.dropdown && !x.dropdown.shown);
  await one.page.waitForSelector('#dropdown', { state: 'hidden' });
  await one.close();

  assert.equal(await first.stop(), 0);
  const state = readState(first.home);
  assert.equal(state.dropdown, term);

  const second = await startPreview({ home: first.home, t });
  const two = await openPage(second);
  t.after(two.close);
  v = await waitView(two.page, (x, id) => x.dropdown && x.dropdown.id === id, term);
  // Go leaves out shown while it is hidden.
  assert.equal(v.dropdown.shown, undefined);
  // The jump palette shows it again.
  await two.page.click('[data-action="open-palette"]');
  await two.page.waitForSelector('#palette-query');
  await two.page.fill('#palette-query', 'Show Drop-down');
  await two.page.press('#palette-query', 'Enter');
  await waitView(two.page, (x) => x.dropdown && x.dropdown.shown);
  await two.page.waitForSelector('#dropdown .xterm');
  await two.page.click('[data-action="hide-dropdown"]');
  v = await waitView(two.page, (x) => x.dropdown && !x.dropdown.shown);

  // Move into This Pane, in a pane's ⋯ menu, brings it back as a sub-tab.
  v = await switchTo(two.page, 'Code');
  const into = v.display.panes[0].id;
  await two.page.click('[data-action="menu"][data-pane="' + into + '"]');
  await two.page.waitForSelector('#window-menu');
  await two.page.click('#window-menu .menu-item:has-text("Move into This Pane")');
  v = await waitView(two.page, (x, id) => !x.dropdown && x.items[id] && x.items[id].tab >= 0, term);
  assert.equal(paneOf(v, term), into);
  await two.page.waitForSelector('.pane [data-term="' + term + '"] .xterm');
  await two.close();
  assert.equal(await second.stop(), 0);
  assert.equal(readState(second.home).dropdown, undefined);
});

test('Settings ▸ Terminals\' Drop-down height goes to config.toml, from 20 to 90 percent', async (t) => {
  const preview = await startPreview({ t });
  const { page, close } = await openPage(preview);
  t.after(close);
  const config = () => fs.readFileSync(path.join(preview.home, 'config.toml'), 'utf8');
  assert.match(config(), /^dropdown_height = 40\s+# Percent of the screen, from 20 to 90\.$/m);
  // Its shortcut is off until it's given one.
  assert.match(config(), /^dropdown = ""\s+# The drop-down terminal\. Off when empty\.$/m);

  await openSettings(page, 'terminals');
  await page.fill('#dropdown-height', '95');
  await page.click('[data-key="settings-save"]');
  await waitForToast(page, 'The drop-down\'s height must be from 20 to 90 percent of the screen.');
  assert.match(config(), /^dropdown_height = 40\s/m);
  await page.fill('#dropdown-height', '55');
  await page.click('[data-key="settings-save"]');
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
  await waitView(page, (v) => v.settings.dropdownHeight === 55);
  assert.match(config(), /^dropdown_height = 55\s/m);
});
