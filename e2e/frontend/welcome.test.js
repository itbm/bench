'use strict';

// Learning Bench in the app, and keeping its permissions: the welcome
// guide's pages (the permissions, which open System Settings, with Open at
// login; the starting set-up, the first time; then four tips), the
// Keyboard Shortcuts sheet, the hint in an empty pane, Quit and Reopen
// Bench for Screen Recording, the banner after an update, and the notice of
// a newer version.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, openSettings, settle } = require('./helpers/harness.js');
const { defaultWorld } = require('./helpers/fixture.js');

const focusedKey = (page) => page.evaluate(() => document.activeElement && document.activeElement.dataset.key);

// guidePage waits for the welcome guide to show a page, and returns its
// title and its step count.
async function guidePage(page, name) {
  await page.waitForSelector('.guide-sheet[data-page="' + name + '"]');
  return [await page.textContent('#guide-title'), await page.textContent('.guide-count')];
}

// firstRun is a world on its first run, without Accessibility permission,
// whose one tab is the browser's, as Bench's first config has it.
function firstRun(opts = {}) {
  const world = defaultWorld({ firstRun: true, accessibility: false, current: 2, ...opts });
  world.tabs = [world.tabs[2]];
  world.current = 0;
  return world;
}

test('the welcome guide goes from the permissions to a starting set-up, then four tips', async (t) => {
  const { page, fake, close } = await openBench({ world: firstRun() });
  t.after(close);
  assert.deepEqual(await guidePage(page, 'permissions'), ['Welcome to Bench', 'Step 1 of 3']);
  const text = await page.textContent('.guide-sheet');
  assert.match(text, /opens System Settings at Privacy & Security ▸ Accessibility/);
  assert.equal(await focusedKey(page), 'guide-allow');
  await page.click('[data-key="guide-allow"]');
  await fake.waitForCall('RequestAccessibility');

  // Screen Recording is optional; once Bench has asked, it offers to quit
  // and reopen, for macOS to apply it.
  await page.click('[data-key="guide-recording"]');
  await fake.waitForCall('RequestScreenRecording');
  await page.waitForSelector('[data-key="guide-reopen"]');
  assert.match(await page.textContent('.guide-sheet'), /macOS applies Screen Recording when Bench opens again/);
  await page.click('[data-key="guide-reopen"]');
  assert.deepEqual((await fake.waitForCall('QuitAndReopen')).args, [false, false]);

  // Open at login is a tick box, off until the user ticks it.
  assert.equal(await page.isChecked('[data-guide="open-at-login"]'), false);
  await page.click('[data-guide="open-at-login"]');
  assert.deepEqual((await fake.waitForCall('SetOpenAtLogin')).args, [true]);
  await page.waitForFunction(() => window.bench.store.view.openAtLogin);
  assert.equal(await page.isChecked('[data-guide="open-at-login"]'), true);

  // The set-up suggests the tabs Go suggests, which the user can remove.
  await page.click('[data-key="guide-continue"]');
  assert.deepEqual(await guidePage(page, 'setup'), ['Choose a starting set-up', 'Step 2 of 3']);
  await page.waitForSelector('.starter');
  const starters = await page.$$eval('.starter', (els) => els.map((el) => [
    el.querySelector('.starter-name').textContent, ...Array.from(el.querySelectorAll('.starter-app'), (a) => a.textContent),
  ]));
  assert.deepEqual(starters, [
    ['Comms', 'SlSlack', 'MaMail'],
    ['Code', 'VSVisual Studio Code', '>_Terminal'],
    ['Plan', 'ObObsidian'],
  ]);
  assert.equal(await focusedKey(page), 'guide-add-tabs');
  assert.equal(await page.textContent('[data-key="guide-add-tabs"]'), 'Add 3 tabs');
  await page.click('[data-key="starter-remove-Code"]');
  assert.equal(await page.textContent('[data-key="guide-add-tabs"]'), 'Add 2 tabs');
  await page.click('[data-key="guide-add-tabs"]');
  assert.deepEqual((await fake.waitForCall('UseStarterTabs')).args, [['Comms', 'Plan']]);
  await page.waitForFunction(() => window.bench.store.view.tabs.map((x) => x.name).join() === 'Research,Comms,Plan');

  // Then four tips, with the shortcuts as configured. The set-up still
  // counts, but Back and Continue skip it, as it's done.
  assert.deepEqual(await guidePage(page, 'tips'), ['A few tips', 'Step 3 of 3']);
  assert.deepEqual(await page.$$eval('.guide-step h3', (els) => els.map((el) => el.textContent)),
    ['Tabs', 'Sub-tabs and dragging', 'Jump to…', 'Right-click menus']);
  const tips = await page.textContent('.guide-sheet .dialog-body');
  assert.match(tips, /press ⌃⌥1 to ⌃⌥9 in any app/);
  assert.match(tips, /Hold Option as you drop it on a tab to show the window in both tabs/);
  assert.match(tips, /Press ⌃⌥Space in any app to jump/);
  assert.match(tips, /Right-click a tab, a sub-tab, or a pane's header/);
  assert.equal(await focusedKey(page), 'guide-done');
  await page.click('[data-key="guide-back"]');
  assert.deepEqual(await guidePage(page, 'permissions'), ['Welcome to Bench', 'Step 1 of 3']);
  await page.click('[data-key="guide-continue"]');
  assert.deepEqual(await guidePage(page, 'tips'), ['A few tips', 'Step 3 of 3']);
  assert.equal(fake.callsTo('DismissFirstRun').length, 0);
  await page.click('[data-key="guide-done"]');
  await fake.waitForCall('DismissFirstRun');
  await page.waitForSelector('.guide-sheet', { state: 'detached' });
  assert.equal(fake.callsTo('UseStarterTabs').length, 1);
});

test('Start with one tab adds none, as Don\'t add tabs does with more tabs, and the guide leaves out a set-up with nothing to suggest', async (t) => {
  const { page, fake, close } = await openBench({ world: firstRun() });
  t.after(close);
  await guidePage(page, 'permissions');
  await page.click('[data-key="guide-continue"]');
  await page.waitForSelector('.starter');
  assert.match(await page.textContent('#guide-intro'), /or start with one tab and make your own\.$/);
  // Removing them all leaves nothing to add, until they're put back.
  for (const name of ['Comms', 'Code', 'Plan']) await page.click('[data-key="starter-remove-' + name + '"]');
  assert.equal(await page.isDisabled('[data-key="guide-add-tabs"]'), true);
  await page.click('[data-key="guide-restore"]');
  assert.equal(await page.locator('.starter').count(), 3);
  assert.equal(await page.textContent('[data-key="guide-one-tab"]'), 'Start with one tab');
  await page.click('[data-key="guide-one-tab"]');
  assert.deepEqual(await guidePage(page, 'tips'), ['A few tips', 'Step 3 of 3']);
  assert.equal(fake.callsTo('UseStarterTabs').length, 0);
  await page.keyboard.press('Escape');
  await page.waitForSelector('.guide-sheet', { state: 'detached' });

  // With more than one tab already, as in a config.toml copied from
  // another Mac, the set-up doesn't offer to start with one tab, as it
  // removes none.
  const world = firstRun();
  world.tabs = defaultWorld().tabs;
  const more = await openBench({ world });
  t.after(more.close);
  await guidePage(more.page, 'permissions');
  await more.page.click('[data-key="guide-continue"]');
  await more.page.waitForSelector('.starter');
  assert.match(await more.page.textContent('#guide-intro'), /after your tabs\. Remove any you don't want, or don't add any\.$/);
  assert.equal(await more.page.textContent('[data-key="guide-one-tab"]'), 'Don\'t add tabs');
  for (const name of ['Comms', 'Code', 'Plan']) {
    if (await more.page.locator('[data-key="starter-remove-' + name + '"]').count()) await more.page.click('[data-key="starter-remove-' + name + '"]');
  }
  assert.match(await more.page.textContent('.guide-loading'), /^No tabs to add\. Go on with the tabs you have/);
  await more.page.click('[data-key="guide-one-tab"]');
  assert.deepEqual(await guidePage(more.page, 'tips'), ['A few tips', 'Step 3 of 3']);
  assert.equal(more.fake.callsTo('UseStarterTabs').length, 0);

  // With no suggestions, the guide has two pages, once Go has said.
  const none = await openBench({ world: firstRun({ starterTabs: [] }) });
  t.after(none.close);
  await guidePage(none.page, 'permissions');
  await none.fake.waitForCall('StarterTabs');
  await none.page.waitForFunction(() => document.querySelector('.guide-count').textContent === 'Step 1 of 2');
  await none.page.click('[data-key="guide-continue"]');
  assert.deepEqual(await guidePage(none.page, 'tips'), ['A few tips', 'Step 2 of 2']);
});

test('Help ▸ Show Welcome Guide and Jump to… open the guide again, without the set-up', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await fake.emit('bench:command', { name: 'first-run' });
  assert.deepEqual(await guidePage(page, 'permissions'), ['Welcome to Bench', 'Step 1 of 2']);
  assert.match(await page.textContent('.guide-sheet'), /Bench has Accessibility permission/);
  assert.equal(await focusedKey(page), 'guide-continue');
  await page.click('[data-key="guide-continue"]');
  await guidePage(page, 'tips');
  await page.click('[data-key="guide-done"]');
  await page.waitForSelector('.guide-sheet', { state: 'detached' });
  assert.equal(fake.callsTo('StarterTabs').length, 0);

  await fake.emit('bench:command', { name: 'open-palette' });
  await page.fill('#palette-query', 'help');
  assert.deepEqual(await page.$$eval('.palette-item .palette-label', (els) => els.map((el) => el.textContent)),
    ['Keyboard shortcuts', 'Show the welcome guide']);
  await page.fill('#palette-query', 'welcome');
  await page.keyboard.press('Enter');
  await guidePage(page, 'permissions');
});

test('after an update, the guide and the banner say how to allow Bench again', async (t) => {
  const world = firstRun();
  world.permissions.accessibilityUpdated = true;
  const { page, close } = await openBench({ world });
  t.after(close);
  await guidePage(page, 'permissions');
  const updated = 'Bench was updated, so macOS needs you to allow it again: select Bench, click −, then turn it on again.';
  assert.equal(await page.textContent('.guide-sheet .guide-warn'), updated);
  assert.doesNotMatch(await page.textContent('.guide-sheet'), /opens System Settings at Privacy/);
});

test('when macOS needs the user to allow the login item, the guide and Settings say so, with Open Login Items', async (t) => {
  const { page, fake, close } = await openBench({ world: firstRun({ needsApproval: true }) });
  t.after(close);
  await guidePage(page, 'permissions');
  await page.click('[data-guide="open-at-login"]');
  await fake.waitForCall('SetOpenAtLogin');
  await page.waitForSelector('#guide-login-note');
  assert.equal(await page.isChecked('[data-guide="open-at-login"]'), false);
  assert.equal(await page.getAttribute('[data-guide="open-at-login"]', 'aria-describedby'), 'guide-login-note');
  await page.click('[data-key="guide-login-items"]');
  await fake.waitForCall('OpenLoginItems');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.guide-sheet', { state: 'detached' });

  await openSettings(page);
  assert.match(await page.textContent('#login-approval-note'), /System Settings ▸ General ▸ Login Items/);
  assert.equal(await page.getAttribute('[data-setting="openAtLogin"]', 'aria-describedby'), 'login-approval-note');
  fake.clearCalls();
  await page.click('[data-key="settings-login-items"]');
  await fake.waitForCall('OpenLoginItems');
});

test('Settings offers to open Screen Recording, then to quit and reopen Bench, for live previews', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ livePreviews: true }) });
  t.after(close);
  await openSettings(page);
  assert.match(await page.textContent('#recording-note'), /Turn on Bench in System Settings ▸ Privacy & Security ▸ Screen Recording/);
  assert.equal(await page.locator('[data-key="settings-reopen"]').count(), 0);
  await page.click('[data-key="settings-recording"]');
  await fake.waitForCall('RequestScreenRecording');
  await page.waitForSelector('[data-key="settings-reopen"]');

  // Once Bench has it, previews show after Bench opens again.
  fake.world.permissions.screenRecording = true;
  await fake.emitState();
  await page.waitForFunction(() => /when Bench opens again/.test(document.getElementById('recording-note').textContent));
  assert.equal(await page.locator('[data-key="settings-recording"]').count(), 0);
  await page.click('[data-key="settings-reopen"]');
  assert.deepEqual((await fake.waitForCall('QuitAndReopen')).args, [false, false]);
});

test('Quit and Reopen Bench asks first while a terminal is running something', async (t) => {
  const world = defaultWorld({ screenRecording: true });
  world.permissions.screenRecordingReopen = true;
  world.busyTerminals = [{ id: 'w5', title: 'claude', tab: 'Code', running: 'claude' }];
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await fake.emit('bench:command', { name: 'first-run' });
  await guidePage(page, 'permissions');
  await page.click('[data-key="guide-reopen"]');
  await page.waitForSelector('.question-sheet');
  assert.equal(await page.textContent('#question-title'), 'Quit and Reopen Bench?');
  assert.equal(await page.textContent('#question-text'), 'A terminal is still running: “claude” (Code). Quit and reopen Bench, and end it?');
  assert.deepEqual(await page.$$eval('.question-sheet button', (els) => els.map((el) => el.textContent)), ['Cancel', 'Quit and Reopen']);
  await page.click('[data-question="dont-ask"]');
  await page.click('[data-key="question-run"]');
  await fake.waitForCall('QuitAndReopen', (a) => a[0] === true);
  assert.deepEqual(fake.callsTo('QuitAndReopen').map((c) => c.args), [[false, false], [true, true]]);
  assert.equal(fake.world.reopened, true);
  assert.equal(fake.callsTo('Quit').length, 0);
});

test('the Keyboard Shortcuts sheet lists the shortcuts as configured, from the status bar, Help, and Jump to…', async (t) => {
  const world = defaultWorld({ keys: { zoomPane: '' } });
  world.keyStates = { jump: { value: 'ctrl+alt+space', problem: 'macOS uses ⌃⌥Space to select the previous input source.' } };
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  // The status bar's key hints are a button that opens it.
  assert.equal(await page.getAttribute('#statusbar .status-keys', 'title'), 'Keyboard Shortcuts');
  await page.click('#statusbar .status-keys');
  await page.waitForSelector('.shortcuts-sheet');
  await fake.waitForCall('SetOverlay', (a) => a[0] === true);
  assert.equal(await focusedKey(page), 'shortcuts-done');
  const rows = await page.$$eval('.shortcuts-sheet .sheet-rows:first-of-type .sheet-row', (els) => els.map((el) => [
    el.querySelector('dt').textContent, el.querySelector('kbd').textContent, (el.querySelector('.key-added') || {}).textContent || '',
    (el.querySelector('.sheet-problem') || {}).textContent || '',
  ]));
  assert.deepEqual(rows, [
    ['Tabs 1 to 9', '⌃⌥', 'with 1 to 9', ''],
    ['Next tab', '⌃⌥]', '', ''],
    ['Previous tab', '⌃⌥[', '', ''],
    ['Last used tab', '⌃⌥`', '', ''],
    ['Jump to…', '⌃⌥Space', '', 'macOS uses ⌃⌥Space to select the previous input source.'],
    ['Rearrange', '⌃⌥0', '', ''],
    ['Focus a pane', '⌃⌥', 'with the arrow keys', ''],
    ['Move a window', '⌃⌥⇧', 'with the arrow keys', ''],
    ['Next window', '⌃⌥Tab', 'and ⇧ for the previous', ''],
    ['Zoom pane', 'Off', '', ''],
    ['Add front window to this pane', 'Off', '', ''],
  ]);
  assert.match(await page.textContent('.shortcuts-sheet'), /In a terminal.*Find⌘F/s);
  // Escape closes it, and the focus goes back to the button.
  await page.keyboard.press('Escape');
  await page.waitForSelector('.shortcuts-sheet', { state: 'detached' });
  assert.equal(await focusedKey(page), 'status-keys');

  // Jump to… finds it as "keyboard".
  await fake.emit('bench:command', { name: 'open-palette' });
  await page.fill('#palette-query', 'keyboard');
  await page.keyboard.press('Enter');
  await page.waitForSelector('.shortcuts-sheet');
  await page.click('[data-key="shortcuts-change"]');
  await page.waitForSelector('.settings-sheet #settings-keyboard');
  await settle(page);
});

test('an empty pane says how to bring a window into it, with the shortcut as configured', async (t) => {
  const world = defaultWorld({ current: 0, keys: { moveWindow: '⌥⇧' } });
  world.tabs[0].root.children[1].items = [];
  world.tabs[0].root.children[1].active = '';
  delete world.items.w2;
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  const hint = page.locator('.pane[data-pane-id="p2"] .empty-hint');
  assert.equal(await hint.textContent(), 'Drag a sub-tab here, or press ⌥⇧ and an arrow to move the shown window here.');

  fake.world.keys.moveWindow = '';
  await fake.emitState();
  await page.waitForFunction(() => document.querySelector('.pane[data-pane-id="p2"] .empty-hint').textContent === 'Drag a sub-tab here.');
});

test('the notice of a newer version has Download, which opens its page, and Dismiss', async (t) => {
  const notice = {
    text: 'Bench 0.3.0 is out. To update, download it and replace Bench in Applications.',
    actions: [
      { label: 'Download', method: 'OpenLink', arg: 'https://github.com/itbm/bench/releases/tag/v0.3.0' },
      { label: 'Dismiss', method: 'DismissUpdate' },
    ],
  };
  const { page, fake, close } = await openBench({ world: defaultWorld({ notices: [notice] }) });
  t.after(close);
  await page.click('[data-action="notices"]');
  await page.click('#notices-menu button:has-text("Download")');
  assert.deepEqual((await fake.waitForCall('OpenLink')).args, ['https://github.com/itbm/bench/releases/tag/v0.3.0']);
  await page.click('[data-action="notices"]');
  await page.click('#notices-menu button:has-text("Dismiss")');
  await fake.waitForCall('DismissUpdate');
  await page.waitForFunction(() => !(window.bench.store.view.notices || []).length);
});

test('with Sparkle, the notice of a newer version has Install…, and Settings says Bench installs it', async (t) => {
  const notice = {
    text: 'Bench 0.3.0 is out.',
    actions: [
      { label: 'Install…', method: 'InstallUpdate' },
      { label: 'Dismiss', method: 'DismissUpdate' },
    ],
  };
  const { page, fake, close } = await openBench({ world: defaultWorld({ notices: [notice], installsUpdates: true }) });
  t.after(close);
  await page.click('[data-action="notices"]');
  await page.click('#notices-menu button:has-text("Install…")');
  await fake.waitForCall('InstallUpdate');

  await openSettings(page);
  assert.equal(await page.textContent('#updates-help'),
    'Bench looks in the latest release on GitHub, and says when there\'s a newer version, with Install… to download and install it. It sends only Bench\'s version. Bench ▸ Check for Updates… looks at once.');
});

test('the banner says what to do after an update', async (t) => {
  const world = defaultWorld({ accessibility: false });
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  assert.match(await page.textContent('#banner'), /until it has Accessibility permission/);
  fake.world.permissions.accessibilityUpdated = true;
  await fake.emitState();
  await page.waitForFunction(() => /Bench was updated, so macOS needs you to allow it again/.test(document.getElementById('banner').textContent));
  await page.click('[data-key="banner-allow"]');
  await fake.waitForCall('RequestAccessibility');
});
