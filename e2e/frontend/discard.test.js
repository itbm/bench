'use strict';

// The question before a dialog's changes that aren't saved are thrown
// away: Settings, Tab properties, New tab, and the add dialog's web
// address and New terminal… form (decision 66).

const assert = require('node:assert/strict');
const test = require('node:test');
const {
  openBench, openSettings, openTabProperties, settle, showSection,
} = require('./helpers/harness.js');
const { defaultWorld } = require('./helpers/fixture.js');

const focused = (page) => page.evaluate(() => document.activeElement.dataset.key || document.activeElement.id);
const buttons = (page) => page.$$eval('.discard-sheet button', (els) => els.map((el) => el.textContent));
const toastIs = (page, text) => page.waitForFunction((t) => document.getElementById('toast').textContent === t, text);

test('the question\'s Save saves Settings and closes it, and a refused save keeps the changes to fix', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  fake.on('SaveSettings', () => { throw new Error('Bench couldn\'t save config.toml: permission denied'); });
  await openSettings(page, 'windows');
  await page.click('[data-setting="hideOtherApps"]');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.discard-sheet');
  // It's an alert, named by its title and text, and the Tab key stays in
  // it.
  assert.equal(await page.getAttribute('.discard-sheet', 'aria-labelledby'), 'discard-title');
  assert.equal(await page.getAttribute('.discard-sheet', 'aria-describedby'), 'discard-text');
  assert.equal(await focused(page), 'discard-save');
  await page.keyboard.press('Tab');
  assert.equal(await focused(page), 'discard-discard');
  await page.keyboard.press('Shift+Tab');
  assert.equal(await focused(page), 'discard-save');
  // Return presses Save, the default. Go refuses: Settings is back, with
  // the change, the reason, and the focus where it was.
  await page.keyboard.press('Enter');
  await toastIs(page, 'Bench couldn\'t save config.toml: permission denied');
  assert.equal(await page.isVisible('.discard-sheet'), false);
  assert.equal(await page.isVisible('.settings-sheet'), true);
  assert.equal(await page.isChecked('[data-setting="hideOtherApps"]'), true);
  assert.equal(await focused(page), 'setting-hideOtherApps');

  fake.on('SaveSettings', ([settings], f) => f.mSaveSettings(settings));
  await page.click('[data-key="settings-cancel"]');
  await page.click('[data-key="discard-save"]');
  await page.waitForSelector('#overlay-root .dialog', { state: 'detached' });
  assert.deepEqual(fake.callsTo('SaveSettings').map((c) => c.args[0].hideOtherApps), [true, true]);
  assert.equal(fake.world.settings.hideOtherApps, true);
  assert.equal(await focused(page), 'open-settings');
  assert.deepEqual(fake.errors, []);
});

test('Tab properties and New tab ask before Escape, Cancel, or a click outside discards their changes', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  // With no changes, Tab properties closes at once.
  await openTabProperties(page);
  await page.keyboard.press('Escape');
  await page.waitForSelector('.editor-sheet', { state: 'detached' });

  // A new name asks first, naming the tab, and Save saves it.
  await openTabProperties(page);
  await page.fill('#tab-name', 'Coding');
  await page.click('[data-key="editor-cancel"]');
  await page.waitForSelector('.discard-sheet');
  assert.equal(await page.textContent('#discard-title'), 'Discard changes to Tab properties?');
  assert.equal(await page.textContent('#discard-text'), 'Your changes to the Code tab aren\'t saved.');
  assert.deepEqual(await buttons(page), ['Discard', 'Keep editing', 'Save']);
  await page.click('[data-key="discard-keep"]');
  assert.equal(await page.inputValue('#tab-name'), 'Coding');
  await page.click('[data-preset="three"]');
  await page.keyboard.press('Escape');
  await page.click('[data-key="discard-save"]');
  const save = await fake.waitForCall('SaveTab');
  assert.deepEqual(save.args, [1, { name: 'Coding', preset: 'three', smallScreens: 'stack', colour: '' }]);
  await page.waitForSelector('#overlay-root .dialog', { state: 'detached' });

  // A new tab with a name typed asks before a click outside discards it.
  await page.click('[data-action="new-tab"]');
  await page.waitForSelector('.editor-sheet');
  await page.mouse.click(20, 880);
  await page.waitForSelector('.editor-sheet', { state: 'detached' });
  await page.click('[data-action="new-tab"]');
  await page.fill('#tab-name', 'Design');
  await page.mouse.click(20, 880);
  await page.waitForSelector('.discard-sheet');
  assert.equal(await page.textContent('#discard-title'), 'Discard the new tab?');
  assert.equal(await page.textContent('#discard-text'), 'Bench hasn\'t made the tab yet.');
  await page.click('[data-key="discard-discard"]');
  await page.waitForSelector('#overlay-root .dialog', { state: 'detached' });
  assert.equal(fake.callsTo('SaveTab').length, 1);

  // A new tab of windows with none ticked can't be made, so the question
  // has no button for that, and Keep editing is the default.
  await fake.emit('bench:command', { name: 'new-tab-from-windows' });
  await page.waitForSelector('.editor-sheet');
  await page.fill('#tab-name', 'Mail');
  for (let box = await page.$('input[data-win]:checked'); box; box = await page.$('input[data-win]:checked')) await box.click();
  await page.waitForSelector('[data-key="editor-save"][disabled]');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.discard-sheet');
  assert.deepEqual(await buttons(page), ['Discard', 'Keep editing']);
  assert.equal(await focused(page), 'discard-keep');
  await page.keyboard.press('Enter');
  await page.waitForSelector('.discard-sheet', { state: 'detached' });
  assert.equal(await page.inputValue('#tab-name'), 'Mail');
  assert.deepEqual(fake.errors, []);
});

test('the add dialog asks before Escape, Back, or a click outside discards a web address or a new terminal', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('.pane[data-pane-id="p3"] [data-action="add"]');
  await page.waitForSelector('.palette');
  await page.keyboard.type('Web page');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#palette-url');
  await page.keyboard.type('example.com');
  await page.mouse.click(20, 880);
  await page.waitForSelector('.discard-sheet');
  assert.equal(await page.textContent('#discard-title'), 'Discard the web address?');
  assert.equal(await page.textContent('#discard-text'), '“example.com” isn\'t added yet.');
  assert.deepEqual(await buttons(page), ['Discard', 'Keep editing', 'Add']);
  await page.keyboard.press('Escape');
  await page.waitForSelector('.discard-sheet', { state: 'detached' });
  assert.equal(await page.inputValue('#palette-url'), 'example.com');
  assert.equal(await focused(page), 'palette-url');
  // Escape, which goes back to the list, asks too, and Discard goes back
  // to the list without the address.
  await page.keyboard.press('Escape');
  await page.waitForSelector('.discard-sheet');
  assert.equal(await page.textContent('#discard-title'), 'Discard the web address?');
  await page.click('[data-key="discard-discard"]');
  await page.waitForSelector('#palette-query');
  assert.equal(await page.isVisible('.discard-sheet'), false);
  assert.equal(await page.isVisible('.palette'), true);
  assert.equal(await focused(page), 'palette-query');
  // With nothing typed, Escape goes back to the list at once.
  await page.fill('#palette-query', 'Web page');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#palette-url');
  assert.equal(await page.inputValue('#palette-url'), '');
  await page.keyboard.press('Escape');
  await page.waitForSelector('#palette-query');
  assert.equal(await page.isVisible('.discard-sheet'), false);

  // The New terminal… form's Back asks in the same way: Keep editing keeps
  // what's typed, and after Escape, Discard goes back to the list.
  await page.fill('#palette-query', 'New terminal');
  await page.click('.palette-item:has-text("New terminal…")');
  await page.waitForSelector('#terminal-command');
  await page.fill('#terminal-command', 'npm test');
  await page.click('[data-key="terminal-cancel"]');
  await page.waitForSelector('.discard-sheet');
  assert.equal(await page.textContent('#discard-title'), 'Discard the new terminal?');
  await page.click('[data-key="discard-keep"]');
  assert.equal(await page.inputValue('#terminal-command'), 'npm test');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.discard-sheet');
  await page.keyboard.press('Tab');
  assert.equal(await focused(page), 'discard-discard');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#palette-query');
  assert.equal(fake.callsTo('AddToPane').length, 0);

  // Add, in the question, adds the terminal.
  await page.fill('#palette-query', 'New terminal');
  await page.click('.palette-item:has-text("New terminal…")');
  await page.waitForSelector('#terminal-command');
  assert.equal(await page.inputValue('#terminal-command'), '');
  await page.fill('#terminal-command', 'npm run dev');
  await page.mouse.click(20, 880);
  await page.waitForSelector('.discard-sheet');
  assert.equal(await page.textContent('#discard-title'), 'Discard the new terminal?');
  await page.click('[data-key="discard-save"]');
  const add = await fake.waitForCall('AddToPane');
  assert.equal(add.args[1].command, 'npm run dev');
  await page.waitForSelector('#overlay-root .dialog', { state: 'detached' });

  // With nothing typed, a click outside closes the dialog at once.
  await page.click('.pane[data-pane-id="p3"] [data-action="add"]');
  await page.waitForSelector('.palette');
  await page.fill('#palette-query', 'New terminal');
  await page.click('.palette-item:has-text("New terminal…")');
  await page.waitForSelector('#terminal-command');
  await page.mouse.click(20, 880);
  await page.waitForSelector('.palette', { state: 'detached' });
  assert.deepEqual(fake.errors, []);
});

test('a command that opens another dialog asks first, and goes on once the changes are discarded or saved', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await openTabProperties(page);
  await page.fill('#tab-name', 'Coding');
  // Settings… from the menu bar asks before it takes Tab properties' place.
  await fake.emit('bench:command', { name: 'settings' });
  await page.waitForSelector('.discard-sheet');
  assert.equal(await page.textContent('#discard-title'), 'Discard changes to Tab properties?');
  // While the question is open, other commands wait for its answer.
  await fake.emit('bench:command', { name: 'toggle-palette' });
  await fake.emit('bench:command', { name: 'new-tab' });
  await settle(page);
  assert.equal(await page.isVisible('.discard-sheet'), true);
  assert.equal(await page.$('.palette'), null);
  await page.click('[data-key="discard-discard"]');
  await page.waitForSelector('.settings-sheet');
  assert.equal(await page.$('.editor-sheet'), null);
  assert.equal(fake.callsTo('SaveTab').length, 0);

  // New Tab from the menu bar, over Settings with a change: Save saves
  // Settings, then the new tab opens.
  await showSection(page, 'general');
  await page.click('[data-setting="fillScreen"]');
  await fake.emit('bench:command', { name: 'new-tab' });
  await page.waitForSelector('.discard-sheet');
  await page.click('[data-key="discard-save"]');
  await fake.waitForCall('SaveSettings');
  await page.waitForSelector('.editor-sheet');
  assert.equal(await page.textContent('#editor-title'), 'New tab');

  // The question before quitting takes the question's place, and Cancel
  // goes back to the dialog, with its changes.
  await page.fill('#tab-name', 'Design');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.discard-sheet');
  await fake.emit('bench:command', { name: 'confirm-quit', terminals: [{ title: 'npm run dev', tab: 'Code' }] });
  await page.waitForSelector('#question-title');
  assert.equal(await page.isVisible('.discard-sheet'), false);
  await page.click('[data-key="question-cancel"]');
  await page.waitForSelector('.editor-sheet');
  assert.equal(await page.inputValue('#tab-name'), 'Design');
  assert.deepEqual(fake.errors, []);
});

test('the question sits over the dialog it covers, which stays where it was', async (t) => {
  const { page, close } = await openBench();
  t.after(close);
  await openSettings(page);
  // Settings is moved by its title.
  const head = await page.locator('.settings-head h2').boundingBox();
  await page.mouse.move(head.x + 10, head.y + 5);
  await page.mouse.down();
  await page.mouse.move(head.x - 150, head.y + 60, { steps: 6 });
  await page.mouse.up();
  const at = await page.locator('.settings-sheet').boundingBox();
  await page.click('[data-setting="fillScreen"]');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.discard-sheet');
  await settle(page);
  const covered = await page.locator('.settings-sheet').boundingBox();
  const q = await page.locator('.discard-sheet').boundingBox();
  assert.deepEqual(covered, at);
  assert.ok(Math.abs((q.x + q.width / 2) - (covered.x + covered.width / 2)) <= 1, 'the question is in the middle of Settings');
  assert.ok(q.y > covered.y && q.y + q.height < covered.y + covered.height, 'the question is over Settings');
  // A press on the covered dialog's title doesn't move it.
  await page.mouse.move(covered.x + 30, covered.y + 20);
  await page.mouse.down();
  await page.mouse.move(covered.x + 130, covered.y + 120, { steps: 6 });
  await page.mouse.up();
  assert.deepEqual(await page.locator('.settings-sheet').boundingBox(), covered);
  assert.equal(await page.isVisible('.discard-sheet'), true);
});

test('while the question before quitting covers a dialog with changes, commands that open another dialog wait for its answer', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await openSettings(page, 'windows');
  await page.click('[data-setting="hideOtherApps"]');
  await fake.emit('bench:command', { name: 'confirm-quit', terminals: [{ title: 'npm run dev', tab: 'Code' }] });
  await page.waitForSelector('#question-title');
  for (const name of ['toggle-palette', 'new-tab', 'settings', 'add-to-pane', 'keyboard-settings', 'first-run', 'show-all-tabs', 'toggle-all-tabs']) {
    await fake.emit('bench:command', { name });
  }
  await page.keyboard.press('Control+k');
  await settle(page);
  assert.equal(await page.textContent('#question-title'), 'Quit Bench?');
  assert.equal(await page.$('.discard-sheet'), null);
  assert.equal(await page.$('.editor-sheet'), null);
  assert.equal(await page.$('.palette'), null);
  assert.equal(await page.$('.overview'), null);
  // Cancel goes back to Settings, with the change.
  await page.click('[data-key="question-cancel"]');
  await page.waitForSelector('.settings-sheet');
  assert.equal(await page.isChecked('[data-setting="hideOtherApps"]'), true);
  assert.equal(fake.callsTo('SaveSettings').length, 0);
  assert.deepEqual(fake.errors, []);
});

test('Close Tab from the menu bar leaves Settings open unless Go asks first, and then asks about its changes', async (t) => {
  const world = defaultWorld({ current: 1 });
  world.settings.confirmClose = false;
  world.busy = { w5: 'npm run dev' };
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await openSettings(page, 'windows');
  await page.click('[data-setting="hideOtherApps"]');
  // Closing the Code tab would end a terminal that's running something,
  // so Go asks, and the question would take Settings' place: Bench asks
  // about Settings' changes first.
  await fake.emit('bench:command', { name: 'close-tab' });
  await page.waitForSelector('.discard-sheet');
  assert.equal(await page.textContent('#discard-title'), 'Discard changes to Settings?');
  await page.click('[data-key="discard-keep"]');
  await page.waitForSelector('.discard-sheet', { state: 'detached' });
  assert.equal(await page.isChecked('[data-setting="hideOtherApps"]'), true);
  assert.equal(await page.$('.question-sheet'), null);
  await fake.emit('bench:command', { name: 'close-tab' });
  await page.waitForSelector('.discard-sheet');
  await page.click('[data-key="discard-discard"]');
  await page.waitForSelector('#question-title');
  assert.equal(await page.$('.settings-sheet'), null);
  await page.click('[data-key="question-cancel"]');
  await page.waitForSelector('.question-sheet', { state: 'detached' });
  assert.equal(fake.world.tabs.length, 3);

  // With nothing running, Go closes the tab without asking, and Settings
  // stays open with its change.
  fake.world.busy = {};
  await openSettings(page, 'windows');
  await page.click('[data-setting="hideOtherApps"]');
  fake.clearCalls();
  await fake.emit('bench:command', { name: 'close-tab' });
  await fake.waitForCall('CloseTab');
  await page.waitForFunction(() => window.bench.store.view.tabs.length === 2);
  await settle(page);
  assert.equal(await page.$('.discard-sheet'), null);
  assert.equal(await page.isVisible('.settings-sheet'), true);
  assert.equal(await page.isChecked('[data-setting="hideOtherApps"]'), true);
  assert.deepEqual(fake.errors, []);
});

test('Show All Tabs asks before the overview takes the place of Settings with changes, and Go\'s hold doesn\'t open it there', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ demo: true, lastTab: 2, keys: { showAllTabs: '⌃⌥O' } }) });
  t.after(close);
  await openSettings(page, 'appearance');
  await page.click('[data-key="gap-8"]');
  await fake.waitForCall('PreviewSettings', (a) => a[0].gap === 8);
  // Go doesn't start a hold while a dialog is open, and the overview of a
  // hold doesn't open over one either (decision 60).
  fake.world.hold = { id: 1, tabs: [1, 2, 0], selected: 2 };
  await fake.emitState();
  await fake.emit('bench:command', { name: 'hold-tabs' });
  await settle(page);
  assert.equal(await page.$('.overview'), null);
  assert.equal(await page.isVisible('.settings-sheet'), true);
  fake.world.hold = null;
  await fake.emitState();

  // View ▸ Show All Tabs asks first, and Keep editing keeps the preview.
  await fake.emit('bench:command', { name: 'show-all-tabs' });
  await page.waitForSelector('.discard-sheet');
  assert.equal(await page.textContent('#discard-title'), 'Discard changes to Settings?');
  // While the question is open, the show_all_tabs shortcut does nothing.
  await fake.emit('bench:command', { name: 'toggle-all-tabs' });
  await settle(page);
  assert.equal(await page.$('.overview'), null);
  await page.click('[data-key="discard-keep"]');
  await page.waitForSelector('.discard-sheet', { state: 'detached' });
  assert.equal(await page.isChecked('[data-key="gap-8"]'), true);
  assert.deepEqual(fake.world.preview, { appearance: '', density: '', gap: 8 });

  // The preview's show_all_tabs shortcut asks too, and Discard opens the
  // overview, which is open alone, with Settings' preview ended.
  await page.keyboard.press('Control+Alt+KeyO');
  await page.waitForSelector('.discard-sheet');
  await page.click('[data-key="discard-discard"]');
  await page.waitForSelector('.overview');
  assert.equal(await page.$('.settings-sheet'), null);
  await fake.waitForCall('PreviewSettings', (a) => a[0].gap === -1);
  assert.equal(fake.world.preview, null);
  assert.equal(fake.callsTo('SaveSettings').length, 0);
  await page.keyboard.press('Escape');
  await page.waitForSelector('.overview', { state: 'detached' });
  await fake.waitForLowered();
  assert.deepEqual(fake.errors, []);
});
