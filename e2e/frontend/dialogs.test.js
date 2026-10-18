'use strict';

// The jump palette, the add dialog, and the layout editor.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, settle } = require('./helpers/harness.js');
const { defaultWorld } = require('./helpers/fixture.js');

const rows = (page) => page.$$eval('#overlay-root .palette-list > *', (els) => els.map((el) => (el.classList.contains('palette-group')
  ? '# ' + el.textContent
  : el.querySelector('.palette-label').textContent + ' | ' + el.querySelector('.palette-hint').textContent)));

test('the jump palette lists tabs, windows, floating windows, and actions', async (t) => {
  const { page, close } = await openBench();
  t.after(close);
  await page.click('[data-action="open-palette"]');
  await page.waitForSelector('.palette');
  assert.equal(await page.evaluate(() => document.activeElement.id), 'palette-query');
  assert.deepEqual(await rows(page), [
    '# Tabs',
    'Comms | Slack, Outlook',
    'Code | Zed, VS Code, Terminal · current',
    'Research | Brave, Obsidian',
    '# Windows',
    '# general | Comms · Slack',
    'Inbox | Comms · Outlook',
    'bench — switch.go | Code · Zed',
    'index.html — bench-ui | Code · VS Code',
    'zsh — ~/code/bench | Code · Terminal',
    'go test — ~/code/bench | Code · Terminal',
    'Wails v3 docs | Research · Brave',
    'Pull request #42 · bench | Research · Brave',
    'Go packages | Research · Brave',
    'Daily note | Research · Obsidian',
    '# Not in a tab',
    'Release notes | Brave · add to this tab',
    'Downloads | Finder · add to this tab',
    '# Actions',
    'Rearrange this tab | ',
    'Edit the layout of Code… | ',
    'New tab… | ',
    'Use the compact layout | ',
    'Use the wide layout | ',
  ]);
  const keys = await page.$$eval('.palette-item .palette-keys', (els) => els.map((el) => el.textContent));
  assert.equal(keys[0], '⌃⌥1  ↵');
  assert.equal(keys[2], '⌃⌥3');
});

test('choosing a window in another tab switches tabs, then shows it', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('[data-action="open-palette"]');
  await page.keyboard.type('packages');
  assert.deepEqual(await rows(page), ['# Windows', 'Go packages | Research · Brave']);
  await page.keyboard.press('Enter');
  await fake.waitForCall('ShowItem');
  assert.deepEqual(fake.calls.filter((c) => ['SwitchTab', 'ShowItem'].includes(c.method)).map((c) => [c.method, c.args]), [
    ['SwitchTab', [2]],
    ['ShowItem', ['p5', 'w9']],
  ]);
  await page.waitForSelector('.palette', { state: 'detached' });
  // Focus goes back to where it was.
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'open-palette');
});

test('the palette moves with the arrows, and Escape or the backdrop closes it', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('[data-action="open-palette"]');
  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('ArrowDown');
  assert.equal(await page.getAttribute('#palette-query', 'aria-activedescendant'), 'palette-item-2');
  assert.equal(await page.getAttribute('#palette-item-2', 'aria-selected'), 'true');
  await page.keyboard.press('ArrowUp');
  await page.keyboard.press('Enter');
  const sw = await fake.waitForCall('SwitchTab');
  assert.deepEqual(sw.args, [1]);

  await page.click('[data-action="open-palette"]');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.palette', { state: 'detached' });
  await page.click('[data-action="open-palette"]');
  await page.mouse.click(20, 880);
  await page.waitForSelector('.palette', { state: 'detached' });

  // Nothing matches.
  await page.click('[data-action="open-palette"]');
  await page.keyboard.type('zzz');
  assert.equal(await page.textContent('.palette-empty'), 'Nothing matches “zzz”.');
});

test('palette actions: floating windows, modes, rearrange, and the editor', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  const choose = async (query) => {
    await page.click('[data-action="open-palette"]');
    await page.keyboard.type(query);
    await page.keyboard.press('Enter');
  };
  await choose('Release notes');
  const add = await fake.waitForCall('AddToPane');
  assert.deepEqual(add.args, ['p3', { type: 'floating', id: 'f101' }]);
  await choose('compact layout');
  assert.deepEqual((await fake.waitForCall('SetMode')).args, ['compact']);
  await choose('Rearrange');
  await fake.waitForCall('Rearrange');
  await choose('Edit the layout');
  await page.waitForSelector('.editor-sheet');
  assert.equal(await page.textContent('#editor-title'), 'Edit tab');
});

test('the add dialog lists AddOptions and adds the chosen row', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('.pane[data-pane-id="p4"] [data-action="add"]');
  const opts = await fake.waitForCall('AddOptions');
  assert.deepEqual(opts.args, ['p4']);
  await page.waitForSelector('.palette');
  await fake.waitForCall('SetOverlay', (a) => a[0] === true);
  assert.equal(await page.getAttribute('.palette', 'aria-label'), 'Add a window or app to this pane');
  assert.deepEqual(await rows(page), [
    '# Windows not in a tab',
    'Release notes | Brave',
    'Downloads | Finder',
    '# Windows in other tabs',
    '# general | Comms · Slack',
    'Inbox | Comms · Outlook',
    'Wails v3 docs | Research · Brave',
    'Pull request #42 · bench | Research · Brave',
    'Go packages | Research · Brave',
    'Daily note | Research · Obsidian',
    '# New window',
    'New Zed window | ',
    'New VS Code window | ',
    '# Built in',
    'New terminal | ~',
    'Web page… | Opens in a Bench web view',
    '# Apps',
    'Calendar | Open, or use its open windows',
    'Figma | Open, or use its open windows',
    'Linear | Open, or use its open windows',
  ]);
  await page.keyboard.type('fig');
  await page.keyboard.press('Enter');
  const call = await fake.waitForCall('AddToPane');
  assert.deepEqual(call.args, ['p4', { type: 'app', bundleId: 'com.figma.Desktop' }]);
  await page.waitForSelector('.palette', { state: 'detached' });
  await fake.waitForLowered();
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'add-p4');

  // Clicking a row works too.
  fake.clearCalls();
  await page.click('.pane[data-pane-id="p4"] [data-action="add"]');
  await page.click('.palette-item:has-text("New terminal")');
  assert.deepEqual((await fake.waitForCall('AddToPane')).args, ['p4', { type: 'terminal', folder: '' }]);
});

test('Web page… asks for an address, then adds it', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('.pane[data-pane-id="p3"] [data-action="add"]');
  await page.waitForSelector('.palette');
  await page.keyboard.type('web page');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#palette-url');
  assert.equal(await page.evaluate(() => document.activeElement.id), 'palette-url');
  assert.equal(fake.callsTo('AddToPane').length, 0);
  // Escape goes back to the list, and choosing it again asks again.
  await page.keyboard.press('Escape');
  await page.waitForSelector('#palette-query');
  await page.click('.palette-item:has-text("Web page…")');
  await page.waitForSelector('#palette-url');

  // An empty address isn't sent.
  await page.keyboard.press('Enter');
  await page.waitForFunction(() => document.getElementById('toast').textContent === 'Type the address of a web page.');
  assert.equal(fake.callsTo('AddToPane').length, 0);

  // A refused address keeps the dialog open.
  fake.on('AddToPane', () => { throw new Error('Bench only opens https addresses.'); });
  await page.keyboard.type('http://example.com');
  await page.keyboard.press('Enter');
  await page.waitForFunction(() => document.getElementById('toast').textContent === 'Bench only opens https addresses.');
  assert.equal(await page.isVisible('#palette-url'), true);

  fake.on('AddToPane', () => null);
  await page.fill('#palette-url', 'example.com/docs');
  await page.keyboard.press('Enter');
  await page.waitForSelector('.palette', { state: 'detached' });
  const last = fake.callsTo('AddToPane').pop();
  assert.deepEqual(last.args, ['p3', { type: 'web', url: 'example.com/docs' }]);
});

test('the layout editor previews presets and saves the draft', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('[data-action="edit-tab"]');
  await page.waitForSelector('.editor-sheet');
  await fake.waitForCall('SetOverlay', (a) => a[0] === true);
  assert.deepEqual((await fake.waitForCall('PreviewTree')).args, [1, 'main-side']);
  assert.equal(await page.evaluate(() => document.activeElement.id), 'tab-name');
  assert.equal(await page.inputValue('#tab-name'), 'Code');
  assert.equal(await page.textContent('#shortcut-value'), '⌃⌥2');
  assert.equal(await page.getAttribute('[data-preset="main-side"]', 'aria-pressed'), 'true');
  assert.equal(await page.getAttribute('[data-small="stack"]', 'aria-pressed'), 'true');
  const chips = await page.$$eval('.mini-pane', (els) => els.map((el) => Array.from(el.querySelectorAll('.chips span'), (s) => s.textContent)));
  assert.deepEqual(chips, [['Zed: bench — switch.go', 'VS Code: index.html — bench-ui'], ['Terminal: zsh — ~/code/bench', 'Terminal: go test — ~/code/bench']]);

  // Choosing a preset asks Go for its preview.
  await page.click('[data-preset="three"]');
  assert.deepEqual((await fake.waitForCall('PreviewTree', (a) => a[1] === 'three')).args, [1, 'three']);
  await page.waitForFunction(() => document.querySelectorAll('.mini-pane').length === 3);
  const three = await page.$$eval('.mini-pane', (els) => els.map((el) => el.textContent));
  assert.equal(three[2], 'Empty pane');

  // The shortcut follows tab order.
  await page.click('[data-action="shortcut"]');
  await page.waitForFunction(() => document.getElementById('toast').textContent === 'Shortcuts follow tab order: the first nine tabs get ⌃⌥1 to ⌃⌥9.');

  await page.fill('#tab-name', '  Coding  ');
  await page.click('[data-small="tabs"]');
  await page.fill('#compact-below', '1600');
  assert.match(await page.textContent('.editor-row:has(h3:text("On small screens")) p'), /Under 1,600 points wide/);
  await page.click('[data-setting="hideOtherTabs"]');
  await page.click('[data-setting="followFocus"]');
  await page.click('[data-setting="livePreviews"]');
  await fake.waitForCall('RequestScreenRecording');
  await page.click('[data-setting="openAtLogin"]');
  await page.click('[data-action="save-tab"]');
  const save = await fake.waitForCall('SaveTab');
  assert.deepEqual(save.args, [1, {
    name: 'Coding',
    preset: 'three',
    smallScreens: 'tabs',
    settings: {
      compactBelow: 1600,
      gap: 8,
      openMissingApps: true,
      hideOtherTabs: false,
      hideOtherApps: false,
      newWindows: 'app-pane',
      followFocus: 'off',
      livePreviews: true,
      launchTimeout: '15s',
    },
  }]);
  assert.deepEqual((await fake.waitForCall('SetOpenAtLogin')).args, [true]);
  await page.waitForSelector('.editor-sheet', { state: 'detached' });
  await fake.waitForLowered();
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'edit-tab');
});

test('typing the threshold for small screens keeps the digits in order', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  const explained = () => page.textContent('.editor-row:has(h3:text("On small screens")) p');
  await page.click('[data-action="edit-tab"]');
  await page.waitForSelector('.editor-sheet');
  assert.equal(await page.inputValue('#compact-below'), '1800');
  assert.equal(await explained(), 'Under 1,800 points wide');
  await page.fill('#compact-below', '');
  await page.keyboard.type('1650');
  assert.equal(await page.inputValue('#compact-below'), '1650');
  assert.equal(await explained(), 'Under 1,650 points wide');
  // A window's new title in the preview, halfway through typing, leaves
  // the field alone.
  await page.fill('#compact-below', '');
  await page.keyboard.type('16');
  fake.world.items.w3.title = 'bench — main.go';
  await fake.emitState();
  await page.waitForSelector('.mini-pane:has-text("Zed: bench — main.go")');
  await page.keyboard.type('50');
  assert.equal(await page.inputValue('#compact-below'), '1650');
  assert.equal(await explained(), 'Under 1,650 points wide');
  // A redraw of the editor, for another preset, keeps what was typed.
  await page.click('[data-preset="three"]');
  await page.waitForFunction(() => document.querySelectorAll('.mini-pane').length === 3);
  assert.equal(await page.inputValue('#compact-below'), '1650');
  assert.equal(await explained(), 'Under 1,650 points wide');
  await page.click('[data-action="save-tab"]');
  const save = await fake.waitForCall('SaveTab');
  assert.equal(save.args[1].settings.compactBelow, 1650);
});

test('a text selection that ends on the backdrop doesn\'t close the dialog', async (t) => {
  const { page, close } = await openBench();
  t.after(close);
  await page.click('[data-action="edit-tab"]');
  await page.waitForSelector('.editor-sheet');
  await page.fill('#tab-name', 'Coding');
  await page.click('[data-preset="three"]');
  const field = await page.locator('#tab-name').boundingBox();
  await page.mouse.move(field.x + field.width - 4, field.y + field.height / 2);
  await page.mouse.down();
  await page.mouse.move(20, 880, { steps: 6 });
  await page.mouse.up();
  await settle(page);
  assert.notEqual(await page.$('.editor-sheet'), null, 'the editor is still open');
  assert.equal(await page.inputValue('#tab-name'), 'Coding');
  assert.equal(await page.getAttribute('[data-preset="three"]', 'aria-pressed'), 'true');
  // A press and release on the backdrop still closes it.
  await page.mouse.click(20, 880);
  await page.waitForSelector('.editor-sheet', { state: 'detached' });
});

test('the layout editor for a new tab, a custom tab, and deleting or resetting', async (t) => {
  const world = defaultWorld();
  world.tabs[1].preset = 'custom';
  const { page, fake, close } = await openBench({ world });
  t.after(close);

  // A new tab previews with index -1 and saves with it.
  await page.click('[data-action="new-tab"]');
  assert.deepEqual((await fake.waitForCall('PreviewTree')).args, [-1, 'single']);
  assert.equal(await page.textContent('#editor-title'), 'New tab');
  assert.equal(await page.textContent('#shortcut-value'), '⌃⌥4');
  assert.equal(await page.$('[data-action="delete-tab"]'), null);
  assert.equal(await page.$('[data-action="reset-layout"]'), null);
  await page.fill('#tab-name', 'Design');
  await page.keyboard.press('Enter');
  const save = await fake.waitForCall('SaveTab');
  assert.equal(save.args[0], -1);
  assert.equal(save.args[1].name, 'Design');
  assert.equal(save.args[1].preset, 'single');
  assert.equal(save.args[1].smallScreens, 'stack');
  await page.waitForSelector('.editor-sheet', { state: 'detached' });

  // A custom tab shows its own tree without asking for a preview.
  fake.clearCalls();
  await page.click('[data-action="edit-tab"]');
  await page.waitForSelector('.editor-sheet');
  assert.match(await page.textContent('.editor-row p'), /Custom, from dragging windows/);
  assert.equal(await page.$('.preset[aria-pressed="true"]'), null);
  assert.equal(await page.$$eval('.mini-pane', (els) => els.length), 2);
  await settle(page);
  assert.equal(fake.callsTo('PreviewTree').length, 0);
  // Saving keeps it custom.
  await page.click('[data-action="save-tab"]');
  assert.equal((await fake.waitForCall('SaveTab')).args[1].preset, 'custom');

  // Reset layout and Delete tab.
  await page.click('[data-action="edit-tab"]');
  await page.click('[data-action="reset-layout"]');
  assert.deepEqual((await fake.waitForCall('ResetLayout')).args, [1]);
  await page.waitForSelector('.editor-sheet', { state: 'detached' });
  await page.click('[data-action="edit-tab"]');
  await page.click('[data-action="delete-tab"]');
  assert.deepEqual((await fake.waitForCall('DeleteTab')).args, [1]);
  await page.waitForSelector('.editor-sheet', { state: 'detached' });

  // A refused save keeps the editor open.
  fake.on('SaveTab', () => { throw new Error('There\'s already a tab called Comms.'); });
  await page.click('[data-action="edit-tab"]');
  await page.fill('#tab-name', 'Comms');
  await page.click('[data-action="save-tab"]');
  await page.waitForFunction(() => document.getElementById('toast').textContent === 'There\'s already a tab called Comms.');
  assert.equal(await page.isVisible('.editor-sheet'), true);
  // Cancel closes it without saving.
  await page.click('[data-key="editor-cancel"]');
  await page.waitForSelector('.editor-sheet', { state: 'detached' });
});

test('Tab stays inside an open dialog', async (t) => {
  const { page, close } = await openBench();
  t.after(close);
  await page.click('[data-action="edit-tab"]');
  await page.waitForSelector('.editor-sheet');
  await page.focus('[data-key="editor-save"]');
  await page.keyboard.press('Tab');
  assert.equal(await page.evaluate(() => document.activeElement.id), 'tab-name');
  await page.keyboard.press('Shift+Tab');
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'editor-save');
  await page.keyboard.press('Escape');

  // From the palette's search field too.
  await page.click('[data-action="open-palette"]');
  await page.keyboard.press('Shift+Tab');
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'palette-close');
  await page.keyboard.press('Tab');
  assert.equal(await page.evaluate(() => document.activeElement.id), 'palette-query');
});

test('the editor closes if its tab is deleted elsewhere', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ current: 2 }) });
  t.after(close);
  await page.click('[data-action="edit-tab"]');
  await page.waitForSelector('.editor-sheet');
  fake.world.tabs.pop();
  fake.world.current = 1;
  await fake.emitState();
  await page.waitForSelector('.editor-sheet', { state: 'detached' });
  await fake.waitForLowered();
  // Focus was in the editor, so it goes back to the button that opened it.
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'edit-tab');
  assert.deepEqual(fake.errors, []);
});

test('the editor follows its tab when another tab is deleted, and Save works once', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ current: 2 }) });
  t.after(close);
  await page.click('[data-action="edit-tab"]');
  await page.waitForSelector('.editor-sheet');
  assert.equal(await page.inputValue('#tab-name'), 'Research');
  // Comms is deleted elsewhere: Research is now the second tab.
  fake.world.tabs.shift();
  fake.world.current = 1;
  await fake.emitState();
  await settle(page);
  assert.equal(await page.isVisible('.editor-sheet'), true);
  assert.equal(await page.textContent('#shortcut-value'), '⌃⌥2');

  // A double click saves once.
  fake.on('SaveTab', async () => { await new Promise((r) => setTimeout(r, 150)); return null; });
  await page.dblclick('[data-action="save-tab"]');
  await page.waitForSelector('.editor-sheet', { state: 'detached' });
  const saves = fake.callsTo('SaveTab');
  assert.equal(saves.length, 1);
  assert.equal(saves[0].args[0], 1);
  assert.equal(saves[0].args[1].name, 'Research');
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'edit-tab');
  assert.deepEqual(fake.errors, []);
});

test('a new View during Delete tab doesn\'t lose where focus goes back to', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ current: 2 }) });
  t.after(close);
  fake.on('DeleteTab', async ([i], f) => {
    f.world.tabs.splice(i, 1);
    f.world.current = 1;
    await f.emitState();
    await new Promise((r) => setTimeout(r, 100));
    return null;
  });
  await page.click('[data-action="edit-tab"]');
  await page.waitForSelector('.editor-sheet');
  await page.click('[data-action="delete-tab"]');
  await page.waitForSelector('.editor-sheet', { state: 'detached' });
  await settle(page);
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'edit-tab');
  assert.equal(await page.evaluate(() => window.bench.store.ui.opener), null);
  assert.deepEqual(fake.errors, []);
});

test('a redraw keeps the selection in the editor\'s name and the palette\'s search field', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('[data-action="edit-tab"]');
  await page.waitForSelector('.editor-sheet');
  const selection = (sel) => page.$eval(sel, (el) => [el.selectionStart, el.selectionEnd]);
  // The name starts selected, ready to be replaced.
  assert.deepEqual(await selection('#tab-name'), [0, 4]);
  // A title in the editor's preview changes, which redraws the editor.
  fake.world.items.w3.title = 'bench — main.go';
  await fake.emitState();
  await page.waitForSelector('.editor-sheet .chips span:has-text("bench — main.go")');
  assert.deepEqual(await selection('#tab-name'), [0, 4]);
  await page.keyboard.type('Work');
  assert.equal(await page.inputValue('#tab-name'), 'Work');
  await page.keyboard.press('Escape');

  await page.click('[data-action="open-palette"]');
  await page.waitForSelector('.palette');
  await page.$eval('#palette-query', (el) => { el.dataset.marker = 'kept'; });
  await page.keyboard.type('bench');
  await page.keyboard.press('Control+A');
  assert.deepEqual(await selection('#palette-query'), [0, 5]);
  fake.world.items.w3.title = 'bench — frame.go';
  await fake.emitState();
  await page.waitForSelector('.palette-item:has-text("bench — frame.go")');
  assert.deepEqual(await selection('#palette-query'), [0, 5]);
  // Neither typing nor a new View replaced the search field.
  assert.equal(await page.$eval('#palette-query', (el) => el.dataset.marker), 'kept');
  assert.equal(await page.getAttribute('#palette-query', 'aria-activedescendant'), 'palette-item-0');
});

test('an input method can compose text in the search field, and its keys don\'t choose rows', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('[data-action="open-palette"]');
  await page.waitForSelector('.palette');
  await page.$eval('#palette-query', (el) => { el.dataset.marker = 'kept'; });
  const cdp = await page.context().newCDPSession(page);
  await cdp.send('Input.imeSetComposition', { text: 'ぶら', selectionStart: 2, selectionEnd: 2 });
  // A View arrives while composing.
  fake.world.items.w3.title = 'bench — compose.go';
  await fake.emitState();
  await settle(page);
  assert.equal(await page.$eval('#palette-query', (el) => el.dataset.marker), 'kept');
  // The input method's keys are its own: here, in WebKit's way, the Return
  // and Escape that end a composition come with keyCode 229.
  for (const key of ['ArrowDown', 'Enter', 'Escape']) {
    await cdp.send('Input.dispatchKeyEvent', { type: 'rawKeyDown', key, code: key, windowsVirtualKeyCode: 229 });
    await cdp.send('Input.dispatchKeyEvent', { type: 'keyUp', key, code: key, windowsVirtualKeyCode: 229 });
  }
  assert.equal(await page.getAttribute('#palette-query', 'aria-activedescendant'), 'palette-item-0');
  await cdp.send('Input.insertText', { text: 'ブラ' });
  await settle(page);
  assert.equal(await page.inputValue('#palette-query'), 'ブラ');
  assert.equal(await page.$eval('#palette-query', (el) => el.dataset.marker), 'kept');
  assert.ok(await page.$('.palette'), 'still open');
  assert.equal(fake.callsTo('SwitchTab').length, 0);
  // The rows follow the committed text, and the new View.
  await page.waitForSelector('.palette-empty:has-text("Nothing matches “ブラ”.")');
  await page.fill('#palette-query', 'compose');
  await page.waitForSelector('.palette-item:has-text("bench — compose.go")');
});

test('Return with keyCode 229, as WebKit sends after a composition, doesn\'t save the editor or add the address', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  const cdp = await page.context().newCDPSession(page);
  const imeEnter = async () => {
    await cdp.send('Input.dispatchKeyEvent', { type: 'rawKeyDown', key: 'Enter', code: 'Enter', windowsVirtualKeyCode: 229 });
    await cdp.send('Input.dispatchKeyEvent', { type: 'keyUp', key: 'Enter', code: 'Enter', windowsVirtualKeyCode: 229 });
  };
  await page.click('[data-action="edit-tab"]');
  await page.waitForSelector('.editor-sheet');
  await imeEnter();
  await settle(page);
  assert.equal(fake.callsTo('SaveTab').length, 0);
  assert.ok(await page.$('.editor-sheet'));
  await page.keyboard.press('Enter');
  await fake.waitForCall('SaveTab');
  await page.waitForSelector('.editor-sheet', { state: 'detached' });

  await page.click('.pane[data-pane-id="p3"] [data-action="add"]');
  await page.waitForSelector('.palette');
  await page.keyboard.type('Web page');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#palette-url');
  await page.keyboard.type('example.com');
  await imeEnter();
  await settle(page);
  assert.equal(fake.callsTo('AddToPane').length, 0);
});
