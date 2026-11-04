'use strict';

// The overview of every tab (decision 60): a card for each tab, which
// typing filters, the arrow keys and 1 to 9 choose from, and Return or a
// click switches to; and the overview Go's hold after last_tab's shortcut
// shows (View.hold), whose choice Go keeps.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, settle } = require('./helpers/harness.js');
const { defaultWorld } = require('./helpers/fixture.js');

// names returns the cards' tab names, in order.
const names = (page) => page.$$eval('#overview-list .overview-card .overview-name', (els) => els.map((el) => el.textContent));

// chosen returns the chosen card's tab name.
const chosen = (page) => page.textContent('#overview-list .overview-card[aria-selected="true"] .overview-name');

const focusedId = (page) => page.evaluate(() => document.activeElement && document.activeElement.id);

// controlClick sends the contextmenu event that macOS makes of a
// Control-click on an element, where Chromium on Linux makes a click.
const controlClick = (page, selector) => page.$eval(selector, (el) => {
  el.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true, ctrlKey: true, button: 0 }));
});

// openFromPalette opens the overview with Jump to…'s Show All Tabs.
async function openFromPalette(page) {
  await page.click('[data-action="open-palette"]');
  await page.fill('#palette-query', 'show all tabs');
  assert.equal(await page.textContent('.palette-item[aria-selected="true"] .palette-label'), 'Show All Tabs');
  await page.keyboard.press('Enter');
  await page.waitForSelector('.overview');
}

test('Show All Tabs in Jump to… shows a card for each tab, with the last used tab chosen', async (t) => {
  const world = defaultWorld({ lastTab: 2 });
  world.items.w6.attention = 'Rang the bell';
  world.items.w9.status = 'missing';
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await openFromPalette(page);
  await fake.waitForCall('SetOverlay', (a) => a[0] === true);
  assert.equal(await page.getAttribute('.overview', 'role'), 'dialog');
  assert.equal(await page.getAttribute('.overview', 'aria-labelledby'), 'overview-title');
  assert.equal(await page.textContent('#overview-title'), 'All tabs');
  // The cards are in the tab bar's order, and the current tab's is marked.
  assert.deepEqual(await names(page), ['Comms', 'Code', 'Research']);
  assert.equal(await page.textContent('.overview-card[data-current] .overview-current'), 'Current');
  assert.equal(await page.getAttribute('.overview-card[data-current]', 'data-index'), '1');
  assert.equal(await chosen(page), 'Research');
  // The search field has the keyboard, and points at the card chosen.
  assert.equal(await focusedId(page), 'overview-query');
  assert.equal(await page.getAttribute('#overview-query', 'aria-activedescendant'), 'overview-card-2');
  assert.equal(await page.getAttribute('#overview-list', 'role'), 'listbox');
  assert.equal(await page.getAttribute('#overview-card-2', 'role'), 'option');

  // Each card has the tab's shortcut, and its panes to scale, each with
  // the window it shows and its other windows' tiles.
  const code = '#overview-card-1';
  assert.equal(await page.textContent(code + ' .overview-card-head kbd'), '⌃⌥2');
  assert.equal(await page.locator(code + ' .overview-pane').count(), 2);
  assert.deepEqual(await page.$$eval(code + ' .overview-window', (els) => els.map((el) => el.textContent)),
    ['bench — switch.go', 'zsh — ~/code/bench']);
  assert.equal(await page.locator(code + ' .overview-pane').first().locator('.overview-other').count(), 1);
  assert.equal(await page.getAttribute(code + ' .overview-mini', 'aria-hidden'), 'true');
  // The bell and ⚠ marks, which a screen reader reads from the card's
  // label, after its name.
  assert.equal(await page.locator(code + ' .overview-card-head .overview-bell').count(), 1);
  assert.equal(await page.locator('#overview-card-2 .overview-card-head .overview-problem').count(), 1);
  assert.match(await page.getAttribute(code, 'aria-label'),
    /^Code ⌃⌥2, current tab, 4 windows, showing .*bench — switch\.go.*, 1 window needs you$/);
  assert.match(await page.getAttribute('#overview-card-2', 'aria-label'), /^Research ⌃⌥3, last used, 4 windows, .*1 window has a problem$/);
  // Its panes have the shape of the pane area, as laid out side by side.
  const ratio = await page.$eval(code + ' .overview-mini', (el) => el.style.aspectRatio);
  const area = await page.evaluate(() => window.bench.store.view.display.area);
  assert.equal(ratio.replace(/\s/g, ''), Math.round(area.w) + '/' + Math.round(area.h));
});

test('typing filters the cards, the arrow keys move the choice, and Return switches to it', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ lastTab: 2 }) });
  t.after(close);
  await openFromPalette(page);
  // A window's app or title counts, as does a tab's name.
  await page.keyboard.type('term');
  await settle(page);
  assert.deepEqual(await names(page), ['Code']);
  assert.equal(await chosen(page), 'Code');
  assert.equal(await page.getAttribute('#overview-query', 'aria-activedescendant'), 'overview-card-1');
  await page.fill('#overview-query', 'daily note');
  await settle(page);
  assert.deepEqual(await names(page), ['Research']);
  await page.fill('#overview-query', 'nothing like it');
  await settle(page);
  assert.equal(await page.textContent('.overview-empty'), 'No tab matches “nothing like it”.');
  assert.equal(await page.getAttribute('#overview-query', 'aria-activedescendant'), null);
  await page.fill('#overview-query', '');
  await settle(page);
  assert.deepEqual(await names(page), ['Comms', 'Code', 'Research']);

  // Left and right move from card to card, and stop at the ends; up and
  // down move by a row, which has every card in a wide window.
  await page.keyboard.press('ArrowLeft');
  assert.equal(await chosen(page), 'Code');
  await page.keyboard.press('ArrowLeft');
  await page.keyboard.press('ArrowLeft');
  assert.equal(await chosen(page), 'Comms');
  await page.keyboard.press('ArrowDown');
  assert.equal(await chosen(page), 'Research');
  await page.keyboard.press('ArrowUp');
  assert.equal(await chosen(page), 'Comms');
  assert.equal(await focusedId(page), 'overview-query');

  await page.keyboard.press('ArrowRight');
  fake.clearCalls();
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('SwitchTab')).args, [1]);
  await page.waitForSelector('.overview', { state: 'detached' });
  await fake.waitForCall('SetOverlay', (a) => a[0] === false);
});

test('1 to 9 and a click switch tabs, and Escape closes the overview and gives the keyboard back', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld() });
  t.after(close);
  // View ▸ Show All Tabs opens it.
  await fake.emit('bench:command', { name: 'show-all-tabs' });
  await page.waitForSelector('.overview');
  await page.keyboard.press('3');
  assert.deepEqual((await fake.waitForCall('SwitchTab')).args, [2]);
  await page.waitForSelector('.overview', { state: 'detached' });

  // Once something is typed, digits are text.
  fake.clearCalls();
  await fake.emit('bench:command', { name: 'show-all-tabs' });
  await page.waitForSelector('#overview-query:focus');
  await page.keyboard.type('c1');
  await settle(page);
  assert.equal(await page.inputValue('#overview-query'), 'c1');
  assert.equal(fake.callsTo('SwitchTab').length, 0);
  await page.keyboard.press('Escape');
  await page.waitForSelector('.overview', { state: 'detached' });

  await fake.emit('bench:command', { name: 'show-all-tabs' });
  await page.waitForSelector('.overview');
  await page.click('#overview-card-0');
  assert.deepEqual((await fake.waitForCall('SwitchTab')).args, [0]);
  await page.waitForSelector('.overview', { state: 'detached' });

  // The show_all_tabs shortcut opens and closes it, and Escape gives the
  // keyboard back: Bench lowers its window again.
  await fake.emit('bench:command', { name: 'toggle-all-tabs' });
  await page.waitForSelector('.overview');
  await fake.emit('bench:command', { name: 'toggle-all-tabs' });
  await page.waitForSelector('.overview', { state: 'detached' });
  fake.clearCalls();
  await fake.emit('bench:command', { name: 'toggle-all-tabs' });
  await page.waitForSelector('.overview');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.overview', { state: 'detached' });
  await fake.waitForLowered();
  assert.equal(fake.callsTo('SwitchTab').length, 0);
});

test('the preview\'s show_all_tabs shortcut opens and closes the overview', async (t) => {
  const { page, close } = await openBench({ world: defaultWorld({ demo: true, keys: { showAllTabs: '⌃⌥O' } }) });
  t.after(close);
  await page.click('.statusbar');
  await page.keyboard.press('Control+Alt+KeyO');
  await page.waitForSelector('.overview');
  await page.keyboard.press('Control+Alt+KeyO');
  await page.waitForSelector('.overview', { state: 'detached' });
});

test('Go\'s hold shows the tabs most recently used first, and its choice, which the arrow keys move', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ lastTab: 2 }) });
  t.after(close);
  // Go sends the hold once it shows the tabs, and the hold-tabs command.
  fake.world.hold = { id: 1, tabs: [1, 2, 0], selected: 2 };
  await fake.emitState();
  await fake.emit('bench:command', { name: 'hold-tabs' });
  await page.waitForSelector('.overview[data-hold]');
  assert.equal(await page.textContent('#overview-title'), 'Switch to a tab');
  assert.equal(await page.textContent('.overview-hint'), 'Let go of ⌃⌥ to switch');
  assert.equal(await page.locator('#overview-query').count(), 0);
  assert.deepEqual(await names(page), ['Code', 'Research', 'Comms']);
  assert.equal(await chosen(page), 'Research');
  // The list has the keyboard, and points at the card chosen.
  await page.waitForFunction(() => document.activeElement && document.activeElement.id === 'overview-list');
  assert.equal(await page.getAttribute('#overview-list', 'aria-activedescendant'), 'overview-card-2');

  // The arrow keys ask Go to move the choice.
  await page.keyboard.press('ArrowRight');
  assert.deepEqual((await fake.waitForCall('StepHold')).args, [1]);
  await page.waitForFunction(() => document.querySelector('#overview-card-0[aria-selected="true"]'));
  assert.equal(await page.getAttribute('#overview-list', 'aria-activedescendant'), 'overview-card-0');
  // Up and Down move by a row, which has every card in a wide window, so
  // Up goes to the first card, Code's.
  fake.clearCalls();
  await page.keyboard.press('ArrowUp');
  assert.deepEqual((await fake.waitForCall('StepHold')).args, [-3]);
  await page.waitForFunction(() => document.querySelector('#overview-card-1[aria-selected="true"]'));
  // So do Go's ⌃⌥↓ and ⌃⌥↑, which reach Go as global shortcuts: Go asks.
  fake.clearCalls();
  await fake.emit('bench:command', { name: 'hold-down' });
  assert.deepEqual((await fake.waitForCall('StepHold')).args, [3]);
  await page.waitForFunction(() => document.querySelector('#overview-card-0[aria-selected="true"]'));
  fake.clearCalls();
  await fake.emit('bench:command', { name: 'hold-up' });
  assert.deepEqual((await fake.waitForCall('StepHold')).args, [-3]);
  await page.waitForFunction(() => document.querySelector('#overview-card-1[aria-selected="true"]'));
  assert.match(await page.textContent('.overview-foot'), /←→↑↓ move/);

  // Escape ends the hold without switching, and the overview stays closed
  // while Go's View still has it.
  fake.on('CancelHold', () => null);
  await page.keyboard.press('Escape');
  await fake.waitForCall('CancelHold');
  await page.waitForSelector('.overview', { state: 'detached' });
  await fake.emitState();
  await settle(page);
  assert.equal(await page.$('.overview'), null);
  assert.equal(fake.callsTo('SwitchTab').length, 0);

  // The next hold opens it again, and goes when Go ends the hold, as the
  // user lets go: Go switches tabs.
  fake.world.hold = { id: 2, tabs: [1, 2, 0], selected: 2 };
  await fake.emitState();
  await page.waitForSelector('.overview[data-hold]');
  fake.clearCalls();
  fake.switchTo(2);
  await fake.emitState();
  await page.waitForSelector('.overview', { state: 'detached' });
  assert.equal(fake.callsTo('CancelHold').length, 0);
});

test('Return or a click during Go\'s hold switches at once', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ lastTab: 2 }) });
  t.after(close);
  fake.world.hold = { id: 1, tabs: [1, 2, 0], selected: 2 };
  await fake.emitState();
  await page.waitForSelector('.overview[data-hold]');
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('SwitchTab')).args, [2]);
  await page.waitForSelector('.overview', { state: 'detached' });
  assert.equal(fake.world.hold, null);

  // The user is holding Control: a click with it switches.
  fake.world.hold = { id: 2, tabs: [2, 1, 0], selected: 1 };
  await fake.emitState();
  await page.waitForSelector('.overview[data-hold]');
  fake.clearCalls();
  await page.click('#overview-card-0', { modifiers: ['Control'] });
  assert.deepEqual((await fake.waitForCall('SwitchTab')).args, [0]);
  await page.waitForSelector('.overview', { state: 'detached' });

  // On a Mac, a Control-click is a contextmenu event, not a click: on a
  // card it switches too.
  fake.world.hold = { id: 3, tabs: [0, 2, 1], selected: 2 };
  await fake.emitState();
  await page.waitForSelector('.overview[data-hold]');
  fake.clearCalls();
  await controlClick(page, '#overview-card-1 .overview-name');
  assert.deepEqual((await fake.waitForCall('SwitchTab')).args, [1]);
  await page.waitForSelector('.overview', { state: 'detached' });
  assert.equal(fake.callsTo('SwitchTab').length, 1);

  // On the backdrop, it stays on the current tab.
  fake.world.hold = { id: 4, tabs: [1, 2, 0], selected: 2 };
  await fake.emitState();
  await page.waitForSelector('.overview[data-hold]');
  fake.clearCalls();
  fake.on('CancelHold', () => null);
  await controlClick(page, '.overlay[data-action="backdrop"]');
  await fake.waitForCall('CancelHold');
  await page.waitForSelector('.overview', { state: 'detached' });
  assert.equal(fake.callsTo('SwitchTab').length, 0);
});

test('a hold that Go gives up gives the keyboard back, and one that switches doesn\'t', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ lastTab: 2 }) });
  t.after(close);
  const focusedKey = () => page.evaluate(() => document.activeElement && document.activeElement.dataset.key);
  await page.focus('[data-key="open-palette"]');
  fake.world.hold = { id: 1, tabs: [1, 2, 0], selected: 2 };
  await fake.emitState();
  await page.waitForFunction(() => document.activeElement && document.activeElement.id === 'overview-list');
  // Go gives up, as when the Mac woke: Bench stays on Code.
  fake.world.hold = null;
  await fake.emitState();
  await page.waitForSelector('.overview', { state: 'detached' });
  await page.waitForFunction(() => document.activeElement && document.activeElement.dataset.key === 'open-palette');

  // Go switches, which gives the keyboard to the tab's window: the button
  // doesn't take it back.
  fake.world.hold = { id: 2, tabs: [1, 2, 0], selected: 2 };
  await fake.emitState();
  await page.waitForFunction(() => document.activeElement && document.activeElement.id === 'overview-list');
  fake.switchTo(2);
  await fake.emitState();
  await page.waitForSelector('.overview', { state: 'detached' });
  await settle(page);
  assert.notEqual(await focusedKey(), 'open-palette');
});

test('Go\'s hold doesn\'t open the overview over a dialog', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ lastTab: 2 }) });
  t.after(close);
  await page.click('[data-action="open-palette"]');
  await page.waitForSelector('.palette');
  fake.world.hold = { id: 1, tabs: [1, 2, 0], selected: 2 };
  await fake.emitState();
  await settle(page);
  assert.equal(await page.$('.overview'), null);
  assert.ok(await page.$('.palette'));
});

test('the overview fits Bench\'s smallest window, and its list scrolls to the card chosen', async (t) => {
  const world = defaultWorld({ screenWidth: 640, lastTab: 2 });
  const { page, close } = await openBench({ world, viewport: { width: 640, height: 480 } });
  t.after(close);
  await openFromPalette(page);
  await settle(page);
  const m = await page.evaluate(() => {
    const r = document.querySelector('.overview').getBoundingClientRect();
    const list = document.querySelector('#overview-list');
    const l = list.getBoundingClientRect();
    const c = document.querySelector('.overview-card[aria-selected="true"]').getBoundingClientRect();
    return {
      top: r.top, bottom: r.bottom, left: r.left, right: r.right, scrolls: list.scrollHeight > list.clientHeight,
      chosenTop: c.top, chosenBottom: c.bottom, listTop: l.top, listBottom: l.bottom,
      foot: document.querySelector('.overview-foot').getBoundingClientRect().bottom,
    };
  });
  assert.ok(m.top >= 0 && m.bottom <= 480 && m.left >= 0 && m.right <= 640, JSON.stringify(m));
  assert.ok(m.foot <= 480, 'the keys below the cards are in view: ' + JSON.stringify(m));
  // Two cards to a row, so Research is below; Down reaches it, in view.
  await page.keyboard.press('ArrowUp');
  assert.equal(await chosen(page), 'Comms');
  await page.keyboard.press('ArrowDown');
  assert.equal(await chosen(page), 'Research');
  await settle(page);
  const shown = await page.evaluate(() => {
    const l = document.querySelector('#overview-list').getBoundingClientRect();
    const c = document.querySelector('.overview-card[aria-selected="true"]').getBoundingClientRect();
    return c.top >= l.top - 1 && c.bottom <= l.bottom + 1;
  });
  assert.ok(shown, 'the card chosen is in view');
});
