'use strict';

// Closing tabs and sub-tabs: the × on each, a middle-click, the tab and
// window menus, the jump palette, and the View menu; the question before
// closing; and a window that's in other tabs too, which the × takes out of
// this tab only.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, openSettings, settle } = require('./helpers/harness.js');
const { defaultWorld } = require('./helpers/fixture.js');

// visible reports whether an element is drawn: the × is hidden with
// visibility, so that it takes no clicks.
const visible = (page, selector) => page.$eval(selector, (el) => getComputedStyle(el).visibility === 'visible');

// centre returns the middle of an element, hidden or not.
async function centre(page, selector) {
  const r = await page.$eval(selector, (el) => el.getBoundingClientRect().toJSON());
  return { x: r.x + r.width / 2, y: r.y + r.height / 2 };
}

const CROSS = (index) => '.tab[data-index="' + index + '"] .close-cross';
const SUB_CROSS = (id) => '.subtab[data-item="' + id + '"] .close-cross';

// sharedWorld has Zed's window (w3) in Code and in Research's second pane.
function sharedWorld() {
  const world = defaultWorld();
  world.tabs[2].root.children[1].items.push('w3');
  return world;
}

// UNDO_TAB ends the question before closing a tab.
const UNDO_TAB = ' Undo brings back the tab, but not what closes.';

const CODE_TEXT = 'This closes 2 windows and 2 terminals. Apps can still ask about unsaved changes.'
  + ' The tab goes from config.toml, so it won\'t come back when Bench next opens.' + UNDO_TAB;

test('a tab\'s × shows on the current tab, in its shortcut\'s place, and on others under the pointer', async (t) => {
  const { page, close } = await openBench();
  t.after(close);
  assert.equal(await visible(page, CROSS(1)), true);
  assert.equal(await visible(page, '.tab[data-index="1"] kbd'), false);
  assert.equal(await visible(page, CROSS(0)), false);
  assert.equal(await visible(page, '.tab[data-index="0"] kbd'), true);
  assert.equal(await page.getAttribute(CROSS(0), 'title'), 'Close tab');
  assert.equal(await page.getAttribute(CROSS(0), 'aria-hidden'), 'true');
  // Under the pointer, the × takes the shortcut's place, and the tab is no
  // wider for it.
  const before = await page.locator('.tab[data-index="0"]').boundingBox();
  await page.hover('.tab[data-index="0"] .tab-name');
  assert.equal(await visible(page, CROSS(0)), true);
  assert.equal(await visible(page, '.tab[data-index="0"] kbd'), false);
  const after = await page.locator('.tab[data-index="0"]').boundingBox();
  assert.deepEqual(after, before);
  // It's inside the tab, at its right end.
  const cross = await page.locator(CROSS(0)).boundingBox();
  assert.ok(cross.x + cross.width <= after.x + after.width && cross.x + cross.width >= after.x + after.width - 12, JSON.stringify({ cross, after }));
});

test('on a tab without its shortcut, the × has a slot of its own, clear of the name', async (t) => {
  const world = defaultWorld();
  world.settings.tabShortcutHints = 'never';
  const { page, close } = await openBench({ world });
  t.after(close);
  assert.equal(await page.$('.tab kbd'), null);
  // clearOfName reports whether a tab's name ends left of its ×'s fade.
  const clearOfName = (index) => page.$eval('.tab[data-index="' + index + '"]', (tab) => {
    const name = tab.querySelector('.tab-name').getBoundingClientRect();
    const cross = tab.querySelector('.close-cross');
    const fade = parseFloat(getComputedStyle(cross, '::before').width);
    return { ok: name.right <= cross.getBoundingClientRect().left - fade, name: name.right, cross: cross.getBoundingClientRect().left, fade };
  });
  const current = await clearOfName(1);
  assert.ok(current.ok, JSON.stringify(current));
  // Under the pointer, another tab's × shows in its slot, and the tab is
  // no wider for it.
  const before = await page.locator('.tab[data-index="0"]').boundingBox();
  await page.hover('.tab[data-index="0"] .tab-name');
  assert.equal(await visible(page, CROSS(0)), true);
  const other = await clearOfName(0);
  assert.ok(other.ok, JSON.stringify(other));
  assert.deepEqual(await page.locator('.tab[data-index="0"]').boundingBox(), before);
});

test('a tab\'s × asks before closing two or more things, and Close closes it', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  fake.clearCalls();
  await page.click(CROSS(1));
  assert.deepEqual((await fake.waitForCall('CloseTab')).args, [1, false, false]);
  await page.waitForSelector('.question-sheet');
  assert.equal(await page.textContent('#question-title'), 'Close Code?');
  assert.equal(await page.textContent('#question-text'), CODE_TEXT);
  assert.deepEqual(await page.$$eval('.question-sheet button', (els) => els.map((el) => el.textContent)), ['Cancel', 'Close tab']);
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'question-run');
  // The × isn't the tab: it doesn't switch to it.
  assert.equal(fake.callsTo('SwitchTab').length, 0);

  // Cancel closes nothing.
  await page.click('[data-key="question-cancel"]');
  await page.waitForSelector('.question-sheet', { state: 'detached' });
  assert.equal(fake.callsTo('CloseTab').length, 1);
  assert.equal(await page.$$eval('.tab', (els) => els.length), 3);

  await page.click(CROSS(1));
  await page.waitForSelector('.question-sheet');
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('CloseTab', (a) => a[1] === true)).args, [1, true, false]);
  await page.waitForSelector('.question-sheet', { state: 'detached' });
  await page.waitForFunction(() => document.querySelectorAll('.tab').length === 2);
  assert.deepEqual(await page.$$eval('.tab .tab-name', (els) => els.map((el) => el.textContent)), ['Comms', 'Research']);
  await page.waitForFunction(() => document.querySelector('#toast').textContent === 'Closed Code.');
});

test('the question\'s Don\'t ask again turns it off, and Settings shows it off', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click(CROSS(1));
  await page.waitForSelector('.question-sheet');
  await page.click('[data-question="dont-ask"]');
  await page.click('[data-action="close-run"]');
  assert.deepEqual((await fake.waitForCall('CloseTab', (a) => a[1] === true)).args, [1, true, true]);
  await page.waitForFunction(() => window.bench.store.view.settings.confirmClose === false);
  // Without the question, Comms closes at once.
  fake.clearCalls();
  await page.click('.tab[data-index="0"]', { button: 'middle' });
  assert.deepEqual((await fake.waitForCall('CloseTab')).args, [0, false, false]);
  await page.waitForFunction(() => document.querySelectorAll('.tab').length === 1);
  assert.equal(await page.$('.question-sheet'), null);
  await openSettings(page);
  assert.equal(await page.isChecked('[data-setting="confirmClose"]'), false);
});

test('a terminal that\'s running something asks first, from its × and in Close tab\'s question', async (t) => {
  const world = defaultWorld();
  world.busy = { w5: 'npm run dev' };
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await page.click(CROSS(1));
  await page.waitForSelector('.question-sheet');
  assert.equal(await page.textContent('#question-text'), 'This closes 2 windows and 2 terminals.'
    + ' “npm run dev” is still running in a terminal, and closing ends it. Apps can still ask about unsaved changes.'
    + ' The tab goes from config.toml, so it won\'t come back when Bench next opens.' + UNDO_TAB);
  await page.keyboard.press('Escape');
  await page.waitForSelector('.question-sheet', { state: 'detached' });

  fake.clearCalls();
  await page.click(SUB_CROSS('w5'));
  assert.deepEqual((await fake.waitForCall('CloseItem')).args, ['w5', false, false]);
  await page.waitForSelector('.question-sheet');
  assert.equal(await page.textContent('#question-title'), 'Close terminal?');
  assert.equal(await page.textContent('#question-text'),
    '“npm run dev” is still running in “zsh — ~/code/bench”. Close the terminal and end it?');
  assert.deepEqual(await page.$$eval('.question-sheet button', (els) => els.map((el) => el.textContent)), ['Cancel', 'Close']);
  await page.click('[data-question="dont-ask"]');
  await page.click('[data-action="close-run"]');
  assert.deepEqual((await fake.waitForCall('CloseItem', (a) => a[1] === true)).args, ['w5', true, true]);
  await page.waitForSelector('.question-sheet', { state: 'detached' });
  await page.waitForSelector('.subtab[data-item="w5"]', { state: 'detached' });
  await page.waitForFunction(() => window.bench.store.view.settings.confirmQuitBusyTerminals === false);
});

test('with confirm_close off, Close tab still asks before ending a terminal that\'s running something', async (t) => {
  const world = defaultWorld();
  world.settings.confirmClose = false;
  world.busy = { w5: 'npm run dev' };
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await page.click(CROSS(1));
  assert.deepEqual((await fake.waitForCall('CloseTab')).args, [1, false, false]);
  await page.waitForSelector('.question-sheet');
  assert.equal(await page.textContent('#question-title'), 'Close Code?');
  // Its Don't ask again turns off asking about busy terminals, and leaves
  // confirm_close as it is.
  await page.click('[data-question="dont-ask"]');
  await page.click('[data-action="close-run"]');
  assert.deepEqual((await fake.waitForCall('CloseTab', (a) => a[1] === true)).args, [1, true, true]);
  await page.waitForFunction(() => window.bench.store.view.settings.confirmQuitBusyTerminals === false);
  await page.waitForFunction(() => document.querySelectorAll('.tab').length === 2);
  assert.equal(await page.evaluate(() => window.bench.store.view.settings.confirmClose), false);
});

test('a sub-tab\'s × shows on the shown sub-tab, and on others under the pointer, and closes without showing it', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  assert.equal(await visible(page, SUB_CROSS('w3')), true);
  assert.equal(await visible(page, SUB_CROSS('w4')), false);
  assert.equal(await page.getAttribute(SUB_CROSS('w3'), 'title'), 'Close window');
  assert.equal(await page.getAttribute(SUB_CROSS('w5'), 'title'), 'Close terminal');
  assert.equal(await page.getAttribute('.subtab[data-item="w4"]', 'aria-label'), 'VS Code — index.html — bench-ui');
  // It's at the sub-tab's right end, and the sub-tab is no wider for it.
  const before = await page.locator('.subtab[data-item="w4"]').boundingBox();
  await page.hover('.subtab[data-item="w4"] .subtab-title');
  assert.equal(await visible(page, SUB_CROSS('w4')), true);
  assert.deepEqual(await page.locator('.subtab[data-item="w4"]').boundingBox(), before);
  const cross = await page.locator(SUB_CROSS('w4')).boundingBox();
  assert.ok(cross.x + cross.width <= before.x + before.width && cross.x + cross.width >= before.x + before.width - 8, JSON.stringify({ cross, before }));

  fake.clearCalls();
  await page.click(SUB_CROSS('w4'));
  assert.deepEqual((await fake.waitForCall('CloseItem')).args, ['w4', false, false]);
  await page.waitForSelector('.subtab[data-item="w4"]', { state: 'detached' });
  await settle(page);
  // The × isn't the sub-tab: it doesn't show it, focus its pane, or drag.
  assert.equal(fake.callsTo('ShowItem').length, 0);
  assert.equal(fake.callsTo('FocusPane').length, 0);
  assert.equal(fake.callsTo('MoveItem').length, 0);
});

test('pressing a sub-tab\'s × and moving away doesn\'t drag the sub-tab', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  fake.clearCalls();
  const from = await centre(page, SUB_CROSS('w3'));
  const to = await centre(page, '.pane[data-pane-id="p4"] .subtabs');
  await page.mouse.move(from.x, from.y);
  await page.mouse.down();
  await page.mouse.move(from.x + 30, from.y + 4, { steps: 4 });
  assert.equal(await page.evaluate(() => document.body.classList.contains('dragging-subtab')), false);
  await page.mouse.move(to.x, to.y, { steps: 6 });
  assert.equal(await page.$('.drag-ghost'), null);
  await page.mouse.up();
  await settle(page);
  for (const method of ['MoveItem', 'AddItemToTab', 'CloseItem']) assert.equal(fake.callsTo(method).length, 0, method);
});

test('a sub-tab with no window: its × says Remove from Bench', async (t) => {
  const world = defaultWorld({ current: 2 });
  world.items.w10.status = 'missing';
  world.items.w10.statusText = 'Obsidian isn\'t running';
  world.items.w10.noWindow = true;
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  assert.equal(await page.getAttribute(SUB_CROSS('w10'), 'title'), 'Remove from Bench');
  fake.clearCalls();
  await page.click(SUB_CROSS('w10'));
  assert.deepEqual((await fake.waitForCall('CloseItem')).args, ['w10', false, false]);
});

test('a window that\'s in other tabs too: its × takes it out of this tab only, and says so', async (t) => {
  const { page, fake, close } = await openBench({ world: sharedWorld() });
  t.after(close);
  assert.equal(await page.getAttribute(SUB_CROSS('w3'), 'title'), 'Remove from this tab');
  assert.equal(await page.getAttribute(SUB_CROSS('w4'), 'title'), 'Close window');
  fake.clearCalls();
  await page.click(SUB_CROSS('w3'));
  assert.deepEqual((await fake.waitForCall('RemoveItemFromTab')).args, ['w3', 1]);
  await settle(page);
  assert.equal(fake.callsTo('CloseItem').length, 0);
  // Close tab… leaves it in the other tab. (FakeGo's RemoveItemFromTab
  // changes nothing, so it's in Code still.)
  await page.click('.tab[data-index="2"]', { button: 'right' });
  await page.click('#tab-menu .menu-item:has-text("Close tab…")');
  await page.waitForSelector('.question-sheet');
  assert.equal(await page.textContent('#question-title'), 'Close Research?');
  assert.equal(await page.textContent('#question-text'), 'This closes 4 windows. The one that\'s also in another tab stays there.'
    + ' Apps can still ask about unsaved changes. The tab goes from config.toml, so it won\'t come back when Bench next opens.' + UNDO_TAB);
});

test('a middle-click closes a tab or a sub-tab, as its × does', async (t) => {
  const { page, fake, close } = await openBench({ world: sharedWorld() });
  t.after(close);
  fake.clearCalls();
  await page.click('.tab[data-index="0"]', { button: 'middle' });
  assert.deepEqual((await fake.waitForCall('CloseTab')).args, [0, false, false]);
  await page.waitForSelector('.question-sheet');
  assert.equal(await page.textContent('#question-title'), 'Close Comms?');
  assert.equal(await page.textContent('#question-text'),
    'This closes 2 pages. The tab goes from config.toml, so it won\'t come back when Bench next opens.' + UNDO_TAB);
  // A middle-click while the question is open does nothing.
  await page.click('.question-sheet h2', { button: 'middle' });
  await page.keyboard.press('Escape');
  await page.waitForSelector('.question-sheet', { state: 'detached' });
  assert.equal(fake.callsTo('SwitchTab').length, 0);

  await page.click('.subtab[data-item="w4"]', { button: 'middle' });
  assert.deepEqual((await fake.waitForCall('CloseItem')).args, ['w4', false, false]);
  // In other tabs too, it leaves this tab only.
  await page.click('.subtab[data-item="w3"]', { button: 'middle' });
  assert.deepEqual((await fake.waitForCall('RemoveItemFromTab')).args, ['w3', 1]);
  await settle(page, 300);
  // Once each, though both mouseup and auxclick come.
  assert.equal(fake.callsTo('CloseTab').length, 1);
  assert.equal(fake.callsTo('CloseItem').length, 1);
  assert.equal(fake.callsTo('RemoveItemFromTab').length, 1);
  assert.equal(fake.callsTo('ShowItem').length, 0);
});

test('one thing closes without asking, and the window menu closes a page', async (t) => {
  const world = defaultWorld({ current: 0 });
  world.tabs[0].root.children[1].items = [];
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  // Comms has one page now: it closes at once.
  await page.click(CROSS(0));
  assert.deepEqual((await fake.waitForCall('CloseTab')).args, [0, false, false]);
  await page.waitForFunction(() => document.querySelectorAll('.tab').length === 2);
  assert.equal(await page.$('.question-sheet'), null);

  // In Code, the window menu's last entry is Close window, or Close page,
  // or Close terminal.
  await page.click('.pane[data-pane-id="p4"] [data-action="menu"]');
  await page.waitForSelector('#window-menu');
  const items = await page.$$eval('#window-menu .menu-item', (els) => els.map((el) => el.textContent));
  assert.equal(items[items.length - 1], 'Close terminal');
  await page.click('#window-menu .menu-item:has-text("Close terminal")');
  assert.deepEqual((await fake.waitForCall('CloseItem')).args, ['w5', false, false]);
  await page.waitForSelector('.subtab[data-item="w5"]', { state: 'detached' });
});

test('on the only tab, Close all windows… leaves the tab, empty, and it can\'t be deleted', async (t) => {
  const world = defaultWorld({ current: 0 });
  world.tabs = [world.tabs[1]];
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  assert.equal(await page.getAttribute(CROSS(0), 'title'), 'Close all windows');
  await page.click('.tab[data-index="0"]', { button: 'right' });
  await page.waitForSelector('#tab-menu');
  const entries = await page.$$eval('#tab-menu > *', (els) => els.map((el) => el.className + ':' + el.textContent));
  assert.deepEqual(entries.slice(-3), ['menu-item:Close all windows…', 'menu-sep:', 'menu-item:New tab…']);
  await page.click('#tab-menu .menu-item:has-text("Close all windows…")');
  assert.deepEqual((await fake.waitForCall('CloseTab')).args, [0, false, false]);
  await page.waitForSelector('.question-sheet');
  assert.equal(await page.textContent('#question-title'), 'Close all windows in Code?');
  assert.equal(await page.textContent('#question-text'), 'This closes 2 windows and 2 terminals. Apps can still ask about unsaved changes.'
    + ' What config.toml lists for this tab won\'t open again when Bench next opens; Reset layout brings it back.'
    + ' Undo brings back the layout, but not what closes.');
  assert.deepEqual(await page.$$eval('.question-sheet button', (els) => els.map((el) => el.textContent)), ['Cancel', 'Close all']);
  await page.click('[data-action="close-run"]');
  assert.deepEqual((await fake.waitForCall('CloseTab', (a) => a[1] === true)).args, [0, true, false]);
  await page.waitForFunction(() => document.querySelectorAll('.subtab').length === 0);
  assert.deepEqual(await page.$$eval('.tab .tab-name', (els) => els.map((el) => el.textContent)), ['Code']);
  await page.waitForFunction(() => document.querySelector('#toast').textContent === 'Closed everything in Code.');
});

test('the jump palette and the View menu close the shown window and the current tab', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('[data-action="open-palette"]');
  await page.waitForSelector('.palette');
  await page.keyboard.type('close');
  const rows = await page.$$eval('#palette-list .palette-item', (els) => els.map((el) =>
    el.querySelector('.palette-label').textContent + ' | ' + el.querySelector('.palette-hint').textContent));
  assert.ok(rows.includes('Close window | Zed — bench — switch.go'), JSON.stringify(rows));
  assert.ok(rows.includes('Close tab… | Code'), JSON.stringify(rows));
  fake.clearCalls();
  await page.click('.palette-item:has-text("Close tab…")');
  assert.deepEqual((await fake.waitForCall('CloseTab')).args, [1, false, false]);
  await page.waitForSelector('.question-sheet');
  assert.equal(await page.$('.palette'), null);
  await page.keyboard.press('Escape');
  await page.waitForSelector('.question-sheet', { state: 'detached' });

  // The View menu's Close Tab… and Close Window.
  await fake.emit('bench:command', { name: 'close-tab' });
  await page.waitForSelector('.question-sheet');
  assert.equal(await page.textContent('#question-title'), 'Close Code?');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.question-sheet', { state: 'detached' });
  fake.clearCalls();
  await fake.emit('bench:command', { name: 'close-window' });
  assert.deepEqual((await fake.waitForCall('CloseItem')).args, ['w3', false, false]);
  await page.waitForSelector('.subtab[data-item="w3"]', { state: 'detached' });
});

test('the pointer on a sub-tab\'s × cancels its preview', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ livePreviews: true, screenRecording: true }) });
  t.after(close);
  fake.clearCalls();
  const title = await centre(page, '.subtab[data-item="w4"] .subtab-title');
  const cross = await centre(page, SUB_CROSS('w4'));
  await page.mouse.move(title.x, title.y);
  await page.mouse.move(cross.x, cross.y, { steps: 3 });
  await page.waitForTimeout(700);
  assert.equal(fake.callsTo('ShowPreview').length, 0);
  // Back on the title, it shows.
  await page.mouse.move(title.x, title.y, { steps: 3 });
  assert.equal((await fake.waitForCall('ShowPreview')).args[0], 'w4');
});

test('without Accessibility permission, an app\'s Close window is off, and says why', async (t) => {
  const { page, close } = await openBench({ world: defaultWorld({ accessibility: false }) });
  t.after(close);
  await page.click('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.waitForSelector('#window-menu');
  const disabled = await page.$$eval('#window-menu [disabled]', (els) => els.map((el) => el.textContent));
  assert.ok(disabled.includes('Close window'), JSON.stringify(disabled));
  const notes = await page.$$eval('#window-menu .menu-note', (els) => els.map((el) => el.textContent));
  assert.ok(notes.includes('Closing an app\'s window needs Accessibility permission.'), JSON.stringify(notes));
  // A terminal closes without it.
  await page.keyboard.press('Escape');
  await page.click('.pane[data-pane-id="p4"] [data-action="menu"]');
  await page.waitForSelector('#window-menu');
  assert.equal(await page.$eval('#window-menu .menu-item:has-text("Close terminal")', (el) => el.disabled), false);
});
