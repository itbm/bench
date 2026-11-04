'use strict';

// Keyboard and VoiceOver (decision 64): the roles and names that a screen
// reader reads, the keys of the strips of tabs and sub-tabs and of the
// radio groups, the Command keys of Bench's window, and the terminals while
// VoiceOver is on.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, openTerminalWindow, settle } = require('./helpers/harness.js');
const { defaultWorld } = require('./helpers/fixture.js');

// focused returns the data-key of the element with the focus.
const focused = (page) => page.evaluate(() => document.activeElement && document.activeElement.dataset.key);

// stops returns the members of a group that the Tab key stops at.
const stops = (page, selector) => page.$$eval(selector, (els) => els.filter((el) => el.tabIndex === 0).map((el) => el.dataset.key));

// toastSays waits until the toast shows text.
const toastSays = (page, text) => page.waitForFunction(
  (want) => document.querySelector('#toast.show') && document.querySelector('#toast').textContent === want, text,
);

// recordToast notes each text that the toast shows from now on, in order,
// for toastTexts.
const recordToast = (page) => page.evaluate(() => {
  const el = document.getElementById('toast');
  window.toastTexts = [];
  new MutationObserver(() => window.toastTexts.push(el.textContent)).observe(el, { childList: true, subtree: true, characterData: true });
});
const toastTexts = (page) => page.evaluate(() => window.toastTexts);

// later moves the page's clock, as Date.now() tells it, ms milliseconds on.
const later = (page, ms) => page.evaluate((by) => {
  const now = Date.now;
  Date.now = () => now() + by;
}, ms);

// ring has terminal w5 write mark and ring the bell, and waits until it has
// drawn the mark, by when xterm.js has rung it.
async function ring(page, fake, mark) {
  await fake.termOut('w5', mark + '\x07');
  await page.waitForFunction((m) => document.querySelector('.pane[data-pane-id="p4"] .xterm-rows').textContent.includes(m), mark);
}

test('the tabs and sub-tabs are tab lists, the choices radio groups, and names start with what shows', async (t) => {
  const { page, close } = await openBench();
  t.after(close);

  // The title bar's tabs: the current one, Code, is selected, the one stop
  // for the Tab key, and controls the panes, which are named after it.
  assert.equal(await page.getAttribute('#titlebar .tabs', 'role'), 'tablist');
  const tabs = await page.$$eval('#titlebar .tabs > *', (els) => els.map((el) => [
    el.getAttribute('role'), el.getAttribute('aria-selected'), el.getAttribute('tabindex'), el.getAttribute('aria-controls'),
  ]));
  assert.deepEqual(tabs, [['tab', 'false', '-1', null], ['tab', 'true', '0', 'panes'], ['tab', 'false', '-1', null]]);
  assert.equal(await page.getAttribute('#panes', 'aria-label'), 'Code tab');

  // A pane's sub-tabs: the shown one controls the pane's body, its tab
  // panel, which it labels. A sub-tab's name starts with its title.
  assert.equal(await page.getAttribute('.pane[data-pane-id="p3"] .subtabs', 'role'), 'tablist');
  const subtabs = await page.$$eval('.pane[data-pane-id="p3"] .subtab', (els) => els.map((el) => [
    el.dataset.item, el.getAttribute('role'), el.getAttribute('aria-selected'), el.getAttribute('tabindex'), el.getAttribute('aria-controls'),
    el.getAttribute('aria-label'),
  ]));
  assert.deepEqual(subtabs, [
    ['w3', 'tab', 'true', '0', 'pane-body-p3', 'bench — switch.go, Zed'],
    ['w4', 'tab', 'false', '-1', null, 'index.html — bench-ui, VS Code'],
  ]);
  const panel = await page.$eval('#pane-body-p3', (el) => [el.className, el.getAttribute('role'), el.getAttribute('aria-labelledby')]);
  assert.deepEqual(panel, ['pane-body', 'tabpanel', 'subtab-p3-w3']);
  assert.equal(await page.getAttribute('#subtab-p3-w3', 'data-item'), 'w3');

  // The layout control is a radio group, with the chosen layout its stop.
  assert.equal(await page.getAttribute('.layout-seg', 'role'), 'radiogroup');
  const modes = await page.$$eval('.layout-seg button', (els) => els.map((el) => [
    el.dataset.mode, el.getAttribute('role'), el.getAttribute('aria-checked'), el.getAttribute('tabindex'),
  ]));
  assert.deepEqual(modes, [['auto', 'radio', 'true', '0'], ['wide', 'radio', 'false', '-1'], ['compact', 'radio', 'false', '-1']]);

  // The status bar's key hints are named after what they open, and read as
  // its description.
  const keys = await page.$eval('#statusbar .status-keys', (el) => [
    el.getAttribute('aria-label'), document.getElementById(el.getAttribute('aria-describedby')).textContent, el.title,
  ]);
  assert.equal(keys[0], 'Keyboard shortcuts');
  assert.match(keys[1], /tabs · .* jump/);
  assert.equal(keys[2], 'Keyboard Shortcuts');

  // Jump to…'s groups are groups, named by their headings.
  await page.click('[data-action="open-palette"]');
  await page.waitForSelector('.palette');
  const groups = await page.$$eval('#palette-list .palette-set', (els) => els.map((el) => [
    el.getAttribute('role'), document.getElementById(el.getAttribute('aria-labelledby')).textContent, el.querySelectorAll('[role="option"]').length > 0,
  ]));
  assert.ok(groups.length >= 2, 'groups: ' + JSON.stringify(groups));
  for (const [role, name, rows] of groups) {
    assert.equal(role, 'group');
    assert.ok(name.length > 0 && rows, name);
  }
  assert.equal(await page.getAttribute('[data-key="palette-close"]', 'aria-label'), 'Esc to close');
  await page.keyboard.press('Escape');

  // Settings' choices are radio groups too.
  await page.click('[data-action="open-settings"]');
  await page.waitForSelector('.settings-sheet');
  const appearance = await page.$eval('[data-choice="appearance"]', (el) => el.parentElement.getAttribute('role'));
  assert.equal(appearance, 'radiogroup');
  assert.deepEqual(await stops(page, '[data-choice="appearance"]'), ['appearance-system']);
});

test('the arrow keys move along the tabs and sub-tabs, which are one stop each for the Tab key', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);

  // Right Arrow moves along the tabs, round from the end, without
  // switching tabs; Home and End go to the ends.
  await page.focus('.tab[data-index="1"]');
  for (const [key, want] of [['ArrowRight', 'tab-2'], ['ArrowRight', 'tab-0'], ['ArrowLeft', 'tab-2'], ['Home', 'tab-0'], ['End', 'tab-2']]) {
    await page.keyboard.press(key);
    assert.equal(await focused(page), want, key);
  }
  assert.deepEqual(await stops(page, '#titlebar .tab'), ['tab-2']);
  assert.equal(fake.callsTo('SwitchTab').length, 0);
  // Tab moves on from the strip, and Shift+Tab comes back to the current
  // tab.
  await page.keyboard.press('Tab');
  assert.equal(await focused(page), 'new-tab');
  await page.keyboard.press('Shift+Tab');
  assert.equal(await focused(page), 'tab-1');

  // Down Arrow goes to the focused pane's sub-tabs, at the one shown, and
  // Up Arrow back to the tabs.
  await page.keyboard.press('ArrowDown');
  assert.equal(await focused(page), 'sub-w3');
  await page.keyboard.press('ArrowRight');
  assert.equal(await focused(page), 'sub-w4');
  assert.deepEqual(await stops(page, '.pane[data-pane-id="p3"] .subtab'), ['sub-w4']);
  await page.keyboard.press('ArrowUp');
  assert.equal(await focused(page), 'tab-1');
  assert.deepEqual(await stops(page, '.pane[data-pane-id="p3"] .subtab'), ['sub-w3']);
  // Up Arrow goes back to the tab that Down Arrow came from, which needn't
  // be the current one.
  await page.keyboard.press('ArrowRight');
  await page.keyboard.press('ArrowDown');
  assert.equal(await focused(page), 'sub-w3');
  await page.keyboard.press('ArrowRight');
  await page.keyboard.press('ArrowUp');
  assert.equal(await focused(page), 'tab-2');
  assert.equal(fake.callsTo('SwitchTab').length + fake.callsTo('ShowItem').length, 0);

  // Return shows the one with the focus, as a click does.
  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('ArrowRight');
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('ShowItem')).args, ['p3', 'w4']);
  await page.waitForSelector('.subtab[data-item="w4"][aria-selected="true"]');
  assert.equal(await focused(page), 'sub-w4');
  await page.focus('.tab[data-index="1"]');
  await page.keyboard.press('ArrowRight');
  await page.keyboard.press('Space');
  assert.deepEqual((await fake.waitForCall('SwitchTab')).args, [2]);
});

test('Delete closes the tab or sub-tab with the focus, as its × does, and the focus stays in the strip', async (t) => {
  const world = defaultWorld();
  world.busy = { w6: 'go test ./...' };
  const { page, fake, close } = await openBench({ world });
  t.after(close);

  // A busy terminal asks first, as its × does, and Close closes it. The
  // focus goes to the sub-tab in its place.
  await page.focus('.subtab[data-item="w5"]');
  await page.keyboard.press('ArrowRight');
  assert.equal(await focused(page), 'sub-w6');
  await page.keyboard.press('Backspace');
  assert.deepEqual((await fake.waitForCall('CloseItem')).args, ['w6', false, false]);
  await page.waitForSelector('.question-sheet');
  await page.click('[data-key="question-run"]');
  await page.waitForSelector('.subtab[data-item="w6"]', { state: 'detached' });
  await page.waitForFunction(() => document.activeElement && document.activeElement.dataset.key === 'sub-w5');

  // With pane_headers = "auto", a pane left with one app's window has no
  // sub-tabs: the focus goes to the current tab.
  fake.world.settings.paneHeaders = 'auto';
  await fake.emitState();
  await page.focus('.subtab[data-item="w3"]');
  await page.keyboard.press('ArrowRight');
  await page.keyboard.press('Delete');
  await fake.waitForCall('CloseItem', (a) => a[0] === 'w4');
  await page.waitForSelector('.pane[data-pane-id="p3"] .subtabs', { state: 'detached' });
  await page.waitForFunction(() => document.activeElement && document.activeElement.dataset.key === 'tab-1');

  // A tab asks first, as its × does.
  await page.focus('.tab[data-index="2"]');
  await page.keyboard.press('Delete');
  assert.deepEqual((await fake.waitForCall('CloseTab')).args, [2, false, false]);
  await page.waitForSelector('.question-sheet');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.question-sheet', { state: 'detached' });
  assert.equal(await focused(page), 'tab-2');
});

test('Backspace closes a tab, and a tab closed without asking gives the focus to the current tab', async (t) => {
  const world = defaultWorld();
  world.settings.confirmClose = false;
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await page.focus('.tab[data-index="1"]');
  await page.keyboard.press('ArrowRight');
  assert.equal(await focused(page), 'tab-2');
  await page.keyboard.press('Backspace');
  assert.deepEqual((await fake.waitForCall('CloseTab')).args, [2, false, false]);
  await page.waitForFunction(() => document.querySelectorAll('#titlebar .tab').length === 2);
  assert.equal(await page.$('.question-sheet'), null);
  await page.waitForFunction(() => document.activeElement && document.activeElement.dataset.key === 'tab-1');
  assert.equal(await page.textContent('.tab[data-index="1"] .tab-name'), 'Code');
  // A tab before the current one: the current tab takes its place, and
  // the focus.
  await page.focus('.tab[data-index="0"]');
  await page.keyboard.press('Backspace');
  await fake.waitForCall('CloseTab', (a) => a[0] === 0);
  await page.waitForFunction(() => document.querySelectorAll('#titlebar .tab').length === 1);
  await page.waitForFunction(() => document.activeElement && document.activeElement.dataset.key === 'tab-0');
  assert.equal(await page.textContent('.tab[data-index="0"] .tab-name'), 'Code');
});

test('Shift+F10, Control+Return, and the menu key open the menu of a tab or sub-tab beside it', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.focus('.tab[data-index="2"]');
  await page.keyboard.press('Shift+F10');
  await page.waitForSelector('#tab-menu');
  assert.equal(await focused(page), 'menu-item-0');
  await page.keyboard.press('Escape');
  await page.waitForSelector('#tab-menu', { state: 'detached' });
  assert.equal(await focused(page), 'tab-2');

  // A sub-tab's menu is its window's, beside it rather than at its pane's
  // ⋯ button, and gives the focus back to it.
  await page.focus('.subtab[data-item="w4"]');
  await page.keyboard.press('Control+Enter');
  await page.waitForSelector('#window-menu');
  assert.equal(await focused(page), 'window-menu');
  const sub = await page.locator('.subtab[data-item="w4"]').boundingBox();
  const menu = await page.locator('#window-menu').boundingBox();
  assert.ok(Math.abs(menu.x - sub.x) <= 1 && menu.y >= sub.y + sub.height, 'the menu is at ' + menu.x + ', ' + menu.y);
  assert.match(await page.textContent('#window-menu'), /Close Window/);
  await page.keyboard.press('Escape');
  await page.waitForSelector('#window-menu', { state: 'detached' });
  assert.equal(await focused(page), 'sub-w4');
  await page.keyboard.press('ContextMenu');
  await page.waitForSelector('#window-menu');
  await page.keyboard.press('Escape');
  await page.waitForSelector('#window-menu', { state: 'detached' });
  // None of it showed or switched anything.
  assert.equal(fake.callsTo('ShowItem').length + fake.callsTo('SwitchTab').length, 0);
});

test('in the layout control the arrow keys move and Return or Space chooses; in Settings they choose', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  // The layout control rearranges every window, so the arrow keys, Home,
  // and End only move the focus, round from either end, and the focus
  // stays in the group.
  await page.focus('.layout-seg [data-mode="auto"]');
  for (const [key, want] of [
    ['ArrowLeft', 'mode-compact'], ['Home', 'mode-auto'], ['ArrowRight', 'mode-wide'], ['End', 'mode-compact'],
    ['ArrowUp', 'mode-wide'], ['ArrowDown', 'mode-compact'], ['ArrowRight', 'mode-auto'], ['ArrowRight', 'mode-wide'],
  ]) {
    await page.keyboard.press(key);
    assert.equal(await focused(page), want, key);
  }
  assert.deepEqual(await stops(page, '.layout-seg button'), ['mode-wide']);
  await settle(page);
  assert.equal(fake.callsTo('SetMode').length, 0);
  assert.equal(await page.getAttribute('[data-mode="auto"]', 'aria-checked'), 'true');
  // Space chooses, as Return does.
  await page.keyboard.press('Space');
  assert.deepEqual((await fake.waitForCall('SetMode')).args, ['wide']);
  await page.waitForSelector('[data-mode="wide"][aria-checked="true"]');
  assert.equal(await focused(page), 'mode-wide');
  await page.keyboard.press('ArrowLeft');
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('SetMode', (a) => a[0] === 'auto')).args, ['auto']);
  // Moving on without choosing leaves the chosen one the stop.
  await page.waitForSelector('[data-mode="auto"][aria-checked="true"]');
  await page.keyboard.press('ArrowRight');
  await page.focus('.tab[data-index="1"]');
  assert.deepEqual(await stops(page, '.layout-seg button'), ['mode-auto']);

  // In Settings, a choice changes the draft, so the arrow keys, Home, and
  // End choose as they move, round from either end, and Tab moves on to
  // the next group.
  await page.click('[data-action="open-settings"]');
  await page.waitForSelector('.settings-sheet');
  const chosen = () => page.$$eval('[data-choice="appearance"]', (els) => els.filter((el) => el.getAttribute('aria-checked') === 'true').map((el) => el.dataset.key));
  await page.focus('[data-key="appearance-system"]');
  for (const [key, want] of [
    ['ArrowRight', 'appearance-light'], ['End', 'appearance-dark'], ['Home', 'appearance-system'], ['ArrowDown', 'appearance-light'],
    ['ArrowUp', 'appearance-system'], ['ArrowLeft', 'appearance-dark'], ['ArrowRight', 'appearance-system'],
  ]) {
    await page.keyboard.press(key);
    assert.equal(await focused(page), want, key);
    assert.deepEqual(await chosen(), [want], key);
  }
  await page.keyboard.press('Tab');
  assert.equal(await page.evaluate(() => document.activeElement.dataset.choice === 'appearance'), false);
});

test('in Bench\'s window, ⌘, opens Settings, ⌘T New tab…, and in a terminal ⌘T and ⌘W open and close terminals', async (t) => {
  const world = defaultWorld();
  world.busy = { w5: 'npm run dev' };
  const { page, fake, close } = await openBench({ world });
  t.after(close);

  await page.click('.statusbar');
  await page.keyboard.press('Meta+Comma');
  await page.waitForSelector('#settings-title:text("Settings")');
  // With a dialog open, the keys are left alone.
  await page.keyboard.press('Meta+KeyT');
  await settle(page);
  assert.equal(await page.$('.editor-sheet'), null);
  await page.keyboard.press('Escape');
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
  await page.keyboard.press('Meta+KeyT');
  await page.waitForSelector('#editor-title:text("New tab")');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.editor-sheet', { state: 'detached' });
  // ⌘W outside a terminal does nothing.
  await page.click('.statusbar');
  await page.keyboard.press('Meta+KeyW');
  await settle(page);
  assert.equal(fake.callsTo('CloseItem').length + fake.callsTo('CloseTab').length, 0);

  // In a terminal, ⌘T opens another in the same pane, in its folder, and
  // the terminal doesn't get the key.
  await page.click('.pane[data-pane-id="p4"] .xterm');
  fake.clearCalls();
  await page.keyboard.press('Meta+KeyT');
  assert.deepEqual((await fake.waitForCall('AddToPane')).args, ['p4', { type: 'terminal', folder: '~/code/bench' }]);
  assert.equal(await page.$('.editor-sheet'), null);
  // ⌘W closes it, asking first, as it's running something.
  await page.keyboard.press('Meta+KeyW');
  assert.deepEqual((await fake.waitForCall('CloseItem')).args, ['w5', false, false]);
  await page.waitForSelector('.question-sheet');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.question-sheet', { state: 'detached' });
  assert.equal(fake.callsTo('TermInput').length, 0);
  assert.ok(fake.world.items.w5, 'the terminal is still there');
});

test('in the "tabs" view, ⌘T in a terminal opens the new one in the terminal\'s own pane', async (t) => {
  const world = defaultWorld({ mode: 'compact' });
  world.tabs[1].smallScreens = 'tabs';
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await page.click('#titlebar .subtab[data-item="w5"]');
  await fake.waitForCall('ShowItem');
  await page.waitForSelector('.pane[data-pane-id="compact"] .xterm');
  await page.click('.pane[data-pane-id="compact"] .xterm');
  fake.clearCalls();
  await page.keyboard.press('Meta+KeyT');
  assert.deepEqual((await fake.waitForCall('AddToPane')).args, ['p4', { type: 'terminal', folder: '~/code/bench' }]);
});

test('⌘T and ⌘, save the name typed in a tab or sub-tab first, as a Mac\'s text field does', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.dblclick('.tab[data-index="1"] .tab-name');
  await page.waitForSelector('#rename-input');
  await page.keyboard.type('Work');
  await page.keyboard.press('Meta+KeyT');
  assert.deepEqual((await fake.waitForCall('SaveTab')).args, [1, { name: 'Work', preset: 'custom', smallScreens: 'stack' }]);
  await page.waitForSelector('#editor-title:text("New tab")');
  await page.waitForSelector('#rename-input', { state: 'detached' });
  // The dialog keeps the focus, and gives it back to the tab.
  assert.equal(await focused(page), 'tab-name');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.editor-sheet', { state: 'detached' });
  assert.equal(await focused(page), 'tab-1');

  // A sub-tab's name, with ⌘,.
  await page.click('.pane[data-pane-id="p4"] .subtab[data-item="w5"]', { button: 'right' });
  await page.waitForSelector('#window-menu');
  await page.click('#window-menu .menu-item:has-text("Rename…")');
  await page.waitForSelector('#rename-input');
  await page.keyboard.type('Server');
  await page.keyboard.press('Meta+Comma');
  assert.deepEqual((await fake.waitForCall('RenameItem')).args, ['w5', 'Server']);
  await page.waitForSelector('#settings-title:text("Settings")');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
  assert.equal(await focused(page), 'sub-w5');
});

test('while VoiceOver is on, the terminals are read out, and bells and finished commands are announced', async (t) => {
  const world = defaultWorld({ demo: true });
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await page.waitForSelector('.pane[data-pane-id="p4"] .xterm');
  assert.equal(await page.evaluate(() => window.bench.terminalReader('w5')), false);
  // While VoiceOver is off, a bell isn't announced.
  await fake.termOut('w5', 'off\x07');
  await page.waitForFunction(() => document.querySelector('.xterm-rows').textContent.includes('off'));
  await settle(page);
  assert.equal(await page.$('#toast.show'), null);

  // The browser preview's VoiceOver pretends that it's on.
  assert.equal(await page.getAttribute('#demo-voiceover', 'aria-pressed'), 'false');
  await page.click('#demo-voiceover');
  assert.deepEqual((await fake.waitForCall('SetDemoVoiceOver')).args, [true]);
  await page.waitForSelector('#demo-voiceover[aria-pressed="true"]');
  await page.waitForFunction(() => window.bench.terminalReader('w5') === true);
  assert.ok(await page.$('.pane[data-pane-id="p4"] .xterm-accessibility-tree'), 'xterm.js has its tree for VoiceOver');

  // A bell in a terminal that's drawn.
  await fake.termOut('w5', '\x07');
  await toastSays(page, 'zsh — ~/code/bench: rang the bell.');
  // A command that ends with an error in a terminal that the user can see,
  // which Go doesn't mark.
  Object.assign(fake.world.items.w5, { ended: true, failed: true });
  await fake.emitState();
  await toastSays(page, 'zsh — ~/code/bench: its command ended with an error.');
  Object.assign(fake.world.items.w5, { ended: false, failed: false });
  await fake.emitState();
  // A finished command, in a terminal that isn't drawn, and one that Go
  // marks for the bell.
  fake.world.items.w6.ended = true;
  await fake.emitState();
  await toastSays(page, 'go test — ~/code/bench: its command has finished.');
  fake.world.items.w6.ended = false;
  fake.world.items.w6.attention = 'Rang the bell';
  await fake.emitState();
  await toastSays(page, 'go test — ~/code/bench: rang the bell.');
  // From another tab, the announcement says which.
  await page.click('.tab[data-index="0"]');
  await fake.waitForCall('SwitchTab');
  await page.waitForSelector('.tab[data-index="0"][aria-selected="true"]');
  fake.world.items.w6.attention = '';
  fake.world.items.w6.ended = true;
  await fake.emitState();
  await toastSays(page, 'go test — ~/code/bench in Code: its command has finished.');

  // Off again, the terminals leave screen reader mode.
  await page.click('#demo-voiceover');
  await page.waitForSelector('#demo-voiceover[aria-pressed="false"]');
  await page.click('.tab[data-index="1"]');
  await page.waitForSelector('.tab[data-index="1"][aria-selected="true"]');
  await page.waitForFunction(() => window.bench.terminalReader('w5') === false);
});

test('an announcement waits until the message showing has been read, and isn\'t said again within 5 seconds', async (t) => {
  const world = defaultWorld();
  world.voiceOver = true;
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await page.waitForFunction(() => window.bench.terminalReader('w5') === true);
  await recordToast(page);
  const bell = 'zsh — ~/code/bench: rang the bell.';

  // A message from Go shows: the bell waits until it has been read, then
  // takes its place.
  const error = 'Bench couldn\'t move that window: it has gone.';
  await fake.emit('bench:toast', error);
  await toastSays(page, error);
  await ring(page, fake, 'one');
  await settle(page, 500);
  assert.equal(await page.textContent('#toast'), error);
  await toastSays(page, bell);
  // The bell again, and Go's mark for it, within 5 seconds, aren't said
  // again.
  await ring(page, fake, 'two');
  fake.world.items.w5.attention = 'Rang the bell';
  await fake.emitState();
  await page.waitForSelector('.subtab[data-item="w5"] .subtab-bell');
  await settle(page);
  assert.deepEqual(await toastTexts(page), [error, bell]);
  // 5 seconds on, it is.
  fake.world.items.w5.attention = '';
  await fake.emitState();
  await later(page, 5100);
  await ring(page, fake, 'three');
  await page.waitForFunction(() => window.toastTexts.length === 3);
  assert.deepEqual(await toastTexts(page), [error, bell, bell]);

  // A message with a button, such as Undo, keeps it, and its text: once
  // the message has been read, the bell follows its text, long before the
  // button's 10 seconds are up.
  await later(page, 5100);
  await fake.emit('bench:toast', { text: 'Closed Research.', action: { label: 'Undo', method: 'Undo', arg: 1 } });
  await page.waitForSelector('#toast.show .toast-action');
  await ring(page, fake, 'four');
  await settle(page, 500);
  assert.equal(await page.$('#toast .toast-said'), null);
  await page.waitForSelector('#toast.show .toast-said', { timeout: 5000 });
  assert.equal(await page.textContent('#toast .toast-text'), 'Closed Research. ' + bell);
  assert.equal(await page.textContent('#toast .toast-action'), 'Undo');
  // Said once, and only the bell is new for the live region.
  assert.equal(await page.$$eval('#toast .toast-said', (els) => els.length), 1);
});

test('a terminal\'s own window follows VoiceOver too', async (t) => {
  const world = defaultWorld();
  world.voiceOver = true;
  const { page, fake, close } = await openTerminalWindow({ id: 'w5', world });
  t.after(close);
  await fake.emitState();
  await page.waitForFunction(() => window.bench.terminalReader('w5') === true);
  await fake.termOut('w5', '\x07');
  await toastSays(page, 'zsh — ~/code/bench: rang the bell.');
});
