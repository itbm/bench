'use strict';

// Settings ▸ Keyboard: Bench's shortcuts, whether each works, recording new
// keys with Bench's own shortcuts released, Turn off, Restore defaults, the
// suggestions, and the notices' buttons that lead there or use a
// suggestion at once.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, openSettings, settle } = require('./helpers/harness.js');
const { defaultWorld } = require('./helpers/fixture.js');

// keyRows returns Settings ▸ Keyboard's rows: each one's name, keys, and
// the line that says whether it works.
const keyRows = (page) => page.$$eval('.key-row', (els) => els.map((el) => [
  el.querySelector('.key-name').textContent,
  el.querySelector('.key-field').textContent + (el.querySelector('.key-added') ? ' ' + el.querySelector('.key-added').textContent : ''),
  el.querySelector('.key-state').textContent,
]));

// stateOf returns the line under a field's row.
const stateOf = (page, field) => page.textContent('#key-' + field.replace(/_/g, '-') + '-state');

// keyOf returns what a field's row shows as its keys.
const keyOf = (page, field) => page.textContent('[data-key="key-' + field + '"]');

const toastIs = (page, text) => page.waitForFunction((t) => document.getElementById('toast').textContent === t, text);

test('Settings ▸ Keyboard lists every shortcut, whether it works, and saves the changes', async (t) => {
  const world = defaultWorld();
  world.keyStates = {
    jump: {
      value: 'ctrl+alt+space', problem: 'macOS uses ⌃⌥Space to switch input sources, so Bench won\'t see it.', suggestion: 'ctrl+alt+j',
    },
  };
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await openSettings(page);
  assert.equal(await page.textContent('#settings-keyboard'), 'Keyboard');
  assert.deepEqual(await keyRows(page), [
    ['Tabs 1 to 9', '⌃⌥ with 1 to 9', 'Works.'],
    ['Next tab', '⌃⌥]', 'Works.'],
    ['Previous tab', '⌃⌥[', 'Works.'],
    ['Last used tab', '⌃⌥` hold ⌃⌥ to choose a tab', 'Works.'],
    ['Jump to…', '⌃⌥Space', 'macOS uses ⌃⌥Space to switch input sources, so Bench won\'t see it. Use ⌃⌥J'],
    // Off unless the user gives it a shortcut, as are the last seven.
    ['Show all tabs', 'Off', 'Off. Bench has no shortcut for this.'],
    ['Rearrange', '⌃⌥0', 'Works.'],
    ['Focus a pane', '⌃⌥ with the arrow keys', 'Works.'],
    ['Move a window', '⌃⌥⇧ with the arrow keys', 'Works.'],
    ['Send a window to a tab', '⌃⌥⇧ with 1 to 9, and ` for the last used tab', 'Works.'],
    ['Next window', '⌃⌥Tab and ⇧ for the previous', 'Works.'],
    ['Zoom pane', '⌃⌥↩', 'Works.'],
    // Off unless the user gives them a shortcut.
    ['Grow pane', 'Off', 'Off. Bench has no shortcut for this.'],
    ['Shrink pane', 'Off', 'Off. Bench has no shortcut for this.'],
    ['Make panes equal', 'Off', 'Off. Bench has no shortcut for this.'],
    ['Next layout', 'Off', 'Off. Bench has no shortcut for this.'],
    ['Previous layout', 'Off', 'Off. Bench has no shortcut for this.'],
    ['Add front window to this pane', 'Off', 'Off. Bench has no shortcut for this.'],
    ['Undo the latest change', 'Off', 'Off. Bench has no shortcut for this.'],
  ]);
  // Every default is in place, so there's nothing to restore.
  assert.equal(await page.isDisabled('[data-action="keys-defaults"]'), true);
  // The field names its row and its keys, and is described by the line.
  assert.equal(await page.getAttribute('[data-key="key-jump"]', 'aria-labelledby'), 'key-jump-label key-jump-value');
  assert.equal(await page.getAttribute('[data-key="key-jump"]', 'aria-describedby'), 'key-jump-state');

  // Use ⌃⌥J takes the suggestion, which Go checks.
  await page.click('[data-key="key-jump-use"]');
  assert.equal(await keyOf(page, 'jump'), '⌃⌥J');
  assert.deepEqual((await fake.waitForCall('CheckShortcuts')).args, [{ jump: 'ctrl+alt+j' }]);
  await page.waitForFunction(() => document.getElementById('key-jump-state').textContent === 'Works.');
  assert.equal(await page.isDisabled('[data-action="keys-defaults"]'), false);

  // Turn off empties a field, and its Turn off is then unavailable.
  await page.click('[data-key="key-zoom_pane-off"]');
  assert.equal(await keyOf(page, 'zoom_pane'), 'Off');
  assert.equal(await stateOf(page, 'zoom_pane'), 'Off. Bench has no shortcut for this.');
  assert.equal(await page.isDisabled('[data-key="key-zoom_pane-off"]'), true);

  await page.click('[data-action="save-settings"]');
  const save = (await fake.waitForCall('SaveSettings')).args[0];
  assert.deepEqual(save.shortcuts, { jump: 'ctrl+alt+j', zoom_pane: '' });
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
  // The new keys show at once, as Go has registered them.
  await page.waitForFunction(() => window.bench.store.view.keys.jump === '⌃⌥J' && window.bench.store.view.keys.zoomPane === '');

  // Restore defaults puts them back.
  await openSettings(page);
  await page.click('[data-action="keys-defaults"]');
  assert.equal(await keyOf(page, 'jump'), '⌃⌥Space');
  assert.equal(await keyOf(page, 'zoom_pane'), '⌃⌥↩');
  await page.click('[data-action="save-settings"]');
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
  assert.deepEqual(fake.callsTo('SaveSettings').pop().args[0].shortcuts, { jump: 'ctrl+alt+space', zoom_pane: 'ctrl+alt+return' });
  assert.deepEqual(fake.errors, []);
});

test('a shortcut field records the keys pressed, with Bench\'s own shortcuts released meanwhile', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ demo: true }) });
  t.after(close);
  await openSettings(page);
  await page.click('[data-key="key-jump"]');
  assert.deepEqual((await fake.waitForCall('PauseShortcuts')).args, [true]);
  assert.equal(await keyOf(page, 'jump'), 'Press keys…');
  assert.equal(await page.getAttribute('[data-key="key-jump"]', 'aria-pressed'), 'true');
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'key-jump');
  assert.equal(await stateOf(page, 'jump'), 'Press the keys, such as ⌃⌥J. Escape stops; Delete turns it off.');

  // A key without ⌃, ⌥, or ⌘ would take over typing.
  await page.keyboard.press('Shift+KeyK');
  assert.equal(await stateOf(page, 'jump'), 'Add ⌃, ⌥, or ⌘, so that the shortcut doesn\'t take over typing.');
  // Bench's own shortcuts don't run while it records: in the preview, ⌃⌥1
  // would switch tabs.
  fake.clearCalls();
  await page.keyboard.press('Control+Alt+Digit1');
  assert.equal(await keyOf(page, 'jump'), '⌃⌥1');
  await settle(page);
  assert.equal(fake.callsTo('SwitchTab').length, 0);
  assert.deepEqual(fake.callsTo('PauseShortcuts').map((c) => c.args), [[false]]);
  assert.deepEqual((await fake.waitForCall('CheckShortcuts')).args, [{ jump: 'ctrl+alt+1' }]);
  // ⌃⌥1 is a tab's shortcut, so the row says so, without waiting for Go.
  assert.equal(await stateOf(page, 'jump'), '⌃⌥1 is already the shortcut for Tabs 1 to 9.');

  // A field of modifiers takes them as they're let go.
  await page.click('[data-key="key-focus_pane_modifiers"]');
  assert.equal(await keyOf(page, 'focus_pane_modifiers'), 'Hold modifiers…');
  await page.keyboard.down('Control');
  await page.keyboard.down('Meta');
  assert.equal(await keyOf(page, 'focus_pane_modifiers'), '⌃⌘…');
  await page.keyboard.up('Meta');
  await page.keyboard.up('Control');
  assert.equal(await keyOf(page, 'focus_pane_modifiers'), '⌃⌘');

  // Escape stops recording and leaves the dialog open; clicking elsewhere
  // stops it too.
  await page.click('[data-key="key-rearrange"]');
  await page.keyboard.press('Escape');
  assert.equal(await keyOf(page, 'rearrange'), '⌃⌥0');
  assert.equal(await page.isVisible('.settings-sheet'), true);
  await page.click('[data-key="key-rearrange"]');
  await page.click('#settings-title');
  assert.equal(await keyOf(page, 'rearrange'), '⌃⌥0');
  // Delete turns the shortcut off.
  await page.click('[data-key="key-next_window"]');
  await page.keyboard.press('Backspace');
  assert.equal(await keyOf(page, 'next_window'), 'Off');

  // Closing Settings while a field records gives Bench its shortcuts back.
  fake.clearCalls();
  await page.click('[data-key="key-rearrange"]');
  await fake.waitForCall('PauseShortcuts', (a) => a[0] === true);
  await page.click('[data-key="settings-cancel"]');
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
  await fake.waitForCall('PauseShortcuts', (a) => a[0] === false);
  assert.equal(fake.callsTo('SaveSettings').length, 0);
  assert.deepEqual(fake.errors, []);
});

test('Send a window to a tab records modifiers, and its keys, 1 to 9 and `, clash with the other shortcuts', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ demo: true }) });
  t.after(close);
  await openSettings(page);
  assert.equal(await keyOf(page, 'send_to_tab_modifiers'), '⌃⌥⇧');
  assert.equal(await page.textContent('#key-send-to-tab-modifiers-label'), 'Send a window to a tab');

  // It takes modifiers alone, as they're let go: Bench adds the keys.
  await page.click('[data-key="key-send_to_tab_modifiers"]');
  assert.equal(await keyOf(page, 'send_to_tab_modifiers'), 'Hold modifiers…');
  assert.equal(await stateOf(page, 'send_to_tab_modifiers'), 'Hold the modifiers, such as ⌃⌥, and let go. Escape stops; Delete turns it off.');
  fake.clearCalls();
  await page.keyboard.down('Control');
  await page.keyboard.down('Alt');
  await page.keyboard.up('Alt');
  await page.keyboard.up('Control');
  assert.equal(await keyOf(page, 'send_to_tab_modifiers'), '⌃⌥');
  assert.deepEqual((await fake.waitForCall('CheckShortcuts')).args, [{ send_to_tab_modifiers: 'ctrl+alt' }]);
  // ⌃⌥ with 1 to 9 are the tabs' shortcuts.
  assert.equal(await stateOf(page, 'send_to_tab_modifiers'), '⌃⌥1 is already the shortcut for Tabs 1 to 9.');
  await page.click('[data-action="save-settings"]');
  await toastIs(page, 'Bench can\'t save that shortcut: ⌃⌥1 is already the shortcut for Tabs 1 to 9.');
  assert.equal(fake.callsTo('SaveSettings').length, 0);

  await page.click('[data-key="key-send_to_tab_modifiers"]');
  await page.keyboard.down('Control');
  await page.keyboard.down('Meta');
  await page.keyboard.up('Meta');
  await page.keyboard.up('Control');
  assert.equal(await keyOf(page, 'send_to_tab_modifiers'), '⌃⌘');
  await page.waitForFunction(() => document.getElementById('key-send-to-tab-modifiers-state').textContent === 'Works.');
  // ` is one of its keys too: another shortcut can't have ⌃⌘`.
  await page.click('[data-key="key-jump"]');
  await page.keyboard.press('Control+Meta+Backquote');
  assert.equal(await stateOf(page, 'jump'), '⌃⌘` is already the shortcut for Send a window to a tab.');
  await page.click('[data-key="key-jump-off"]');

  await page.click('[data-action="save-settings"]');
  const save = (await fake.waitForCall('SaveSettings')).args[0];
  assert.deepEqual(save.shortcuts, { jump: '', send_to_tab_modifiers: 'ctrl+cmd' });
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
  await page.waitForFunction(() => window.bench.store.view.keys.sendToTab === '⌃⌘');
  // The new keys work at once in the preview.
  fake.clearCalls();
  await page.click('.statusbar');
  await page.keyboard.press('Control+Meta+Digit1');
  assert.deepEqual((await fake.waitForCall('SendToTab')).args, [0]);
  await page.keyboard.press('Control+Meta+Backquote');
  await fake.waitForCall('SendToLastTab');
  assert.deepEqual(fake.errors, []);
});

test('a shortcut that another app has shows Go\'s suggestion, and a clash is refused on Save', async (t) => {
  const world = defaultWorld();
  world.takenShortcuts = ['ctrl+alt+k'];
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await openSettings(page);
  await page.click('[data-key="key-jump"]');
  await page.keyboard.press('Control+Alt+KeyK');
  await page.waitForFunction(() => document.getElementById('key-jump-state').textContent.startsWith('Bench couldn'));
  assert.equal(await stateOf(page, 'jump'), 'Bench couldn\'t register ⌃⌥K: another app might be using it. Use ⌃⌥⌘K');
  await page.click('[data-key="key-jump-use"]');
  assert.equal(await keyOf(page, 'jump'), '⌃⌥⌘K');
  await page.waitForFunction(() => document.getElementById('key-jump-state').textContent === 'Works.');

  // Two fields with the same shortcut can't be saved: Settings says why,
  // as the row does, and stays open, with the keyboard on that row.
  await page.click('[data-key="key-rearrange"]');
  await page.keyboard.press('Control+Alt+BracketRight');
  assert.equal(await stateOf(page, 'rearrange'), '⌃⌥] is already the shortcut for Next tab.');
  await page.click('[data-action="save-settings"]');
  await toastIs(page, 'Bench can\'t save that shortcut: ⌃⌥] is already the shortcut for Next tab.');
  assert.equal(await page.isVisible('.settings-sheet'), true);
  await page.waitForFunction(() => document.activeElement && document.activeElement.dataset.key === 'key-rearrange');

  // It names the field changed, even one that comes before the other in
  // config.toml, which Go would blame instead.
  await page.click('[data-key="key-rearrange-off"]');
  await page.click('[data-key="key-next_tab"]');
  await page.keyboard.press('Control+Alt+BracketLeft');
  assert.equal(await stateOf(page, 'next_tab'), '⌃⌥[ is already the shortcut for Previous tab.');
  await page.click('[data-action="save-settings"]');
  await toastIs(page, 'Bench can\'t save that shortcut: ⌃⌥[ is already the shortcut for Previous tab.');
  await page.waitForFunction(() => document.activeElement && document.activeElement.dataset.key === 'key-next_tab');
  assert.equal(await page.isVisible('.settings-sheet'), true);
  assert.equal(fake.callsTo('SaveSettings').length, 0);
  assert.deepEqual(fake.errors, []);
});

test('a notice about a shortcut uses its suggestion, or opens Settings ▸ Keyboard', async (t) => {
  const world = defaultWorld({
    notices: [{
      text: 'macOS uses ⌃⌥Space to switch input sources, so Bench won\'t see it. Try ⌃⌥J instead.',
      actions: [
        { label: 'Use this instead', method: 'UseSuggestedShortcuts', arg: 'jump' },
        { label: 'Keyboard settings', command: 'keyboard-settings' },
      ],
    }],
  });
  world.keyStates = {
    jump: { value: 'ctrl+alt+space', problem: 'macOS uses ⌃⌥Space to switch input sources, so Bench won\'t see it.', suggestion: 'ctrl+alt+j' },
  };
  const { page, fake, close } = await openBench({ world, viewport: { width: 900, height: 600 } });
  t.after(close);

  // Keyboard settings opens Settings at its Keyboard section, with the
  // shortcut that doesn't work focused.
  await page.click('[data-action="notices"]');
  await page.click('[data-action="notice-run"][data-index="1"]');
  await page.waitForSelector('.settings-sheet');
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'key-jump');
  const shown = await page.evaluate(() => {
    const body = document.querySelector('.settings-sheet .dialog-body').getBoundingClientRect();
    const heading = document.getElementById('settings-keyboard').getBoundingClientRect();
    return heading.top >= body.top - 1 && heading.top < body.bottom;
  });
  assert.equal(shown, true, 'the Keyboard heading is in view');
  // At 640 by 480, Bench's smallest, the rows fit the dialog without
  // scrolling sideways, and Save stays in view.
  await page.setViewportSize({ width: 640, height: 480 });
  await page.waitForFunction(() => document.querySelector('#overlay-root .dialog').getBoundingClientRect().right <= 632);
  const fits = await page.evaluate(() => {
    const body = document.querySelector('.settings-sheet .dialog-body');
    const save = document.querySelector('[data-action="save-settings"]').getBoundingClientRect();
    return body.scrollWidth <= body.clientWidth && save.bottom <= window.innerHeight;
  });
  assert.equal(fits, true);
  await page.keyboard.press('Escape');
  await page.waitForSelector('.settings-sheet', { state: 'detached' });

  // Use this instead has Go take the suggestion.
  await page.click('[data-action="notices"]');
  await page.click('[data-action="notice-run"][data-index="0"]');
  assert.deepEqual((await fake.waitForCall('UseSuggestedShortcuts')).args, ['jump']);
  await toastIs(page, 'Changed Jump to… to ⌃⌥J');
  await page.waitForFunction(() => window.bench.store.view.keys.jump === '⌃⌥J' && !window.bench.store.view.notices.length);
  assert.deepEqual(fake.errors, []);
});

test('Tab properties says that a tab\'s shortcut follows its position, and where to change the keys', async (t) => {
  const { page, close } = await openBench();
  t.after(close);
  await page.click('.tab[data-index="1"]', { button: 'right' });
  await page.click('#tab-menu .menu-item:has-text("Tab properties")');
  await page.waitForSelector('.editor-sheet');
  assert.equal(await page.textContent('#tab-place-note'), '');
  assert.equal(await page.textContent('#tab-shortcut-text'), '⌃⌥2, which follows the tab\'s position. Settings ▸ Keyboard changes the keys.');
});
