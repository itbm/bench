'use strict';

// Jump to…: words in any order, across a row's label and hint; how rows
// rank; the focused pane's window's ⋯ menu as the This window group; the
// actions for the pane, the tabs, and Bench; and the tab and windows used
// last, first.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, settle } = require('./helpers/harness.js');
const { defaultWorld } = require('./helpers/fixture.js');

const rows = (page) => page.$$eval('#overlay-root .palette-list > *', (els) => els.map((el) => (el.classList.contains('palette-group')
  ? '# ' + el.textContent
  : el.querySelector('.palette-label').textContent + ' | ' + el.querySelector('.palette-hint').textContent)));

// labels lists the rows' labels, without the groups.
const labels = (page) => page.$$eval('#overlay-root .palette-item .palette-label', (els) => els.map((el) => el.textContent));

// groupOf returns a group's rows, without its heading. A heading has no
// hint, unlike a row such as "# general | Comms · Slack".
async function groupOf(page, name) {
  const all = await rows(page);
  const at = all.indexOf('# ' + name);
  if (at < 0) return [];
  const end = all.findIndex((r, i) => i > at && r.startsWith('# ') && !r.includes(' | '));
  return all.slice(at + 1, end < 0 ? undefined : end);
}

// jump opens Jump to… and types query.
async function jump(page, query = '') {
  await page.click('[data-action="open-palette"]');
  await page.waitForSelector('#palette-query');
  if (query) await page.fill('#palette-query', query);
}

// selected returns the selected row's label.
const selected = (page) => page.textContent('.palette-item[aria-selected="true"] .palette-label');

test('Jump to… finds each typed word in any order, in a row\'s label and hint together', async (t) => {
  const { page, close } = await openBench();
  t.after(close);
  // The words are in the title, apart and in either order.
  await jump(page, 'pull 42');
  assert.deepEqual(await rows(page), ['# Windows', 'Pull request #42 · bench | Research · Brave']);
  await page.fill('#palette-query', '42 PULL');
  assert.deepEqual(await rows(page), ['# Windows', 'Pull request #42 · bench | Research · Brave']);
  // One in the title, one in the hint. Moving Zed's window to the
  // terminal's pane has both too, further down.
  await page.fill('#palette-query', 'code zsh');
  assert.deepEqual((await rows(page)).slice(0, 2), ['# Windows', 'zsh — ~/code/bench | Code · Terminal']);
  await page.fill('#palette-query', 'brave docs');
  assert.deepEqual(await rows(page), ['# Windows', 'Wails v3 docs | Research · Brave']);
  // Every word has to be there.
  await page.fill('#palette-query', 'pull 43');
  assert.equal(await page.textContent('.palette-empty'), 'Nothing matches “pull 43”.');
  // A word with no letter or digit is left out, as it's found almost
  // anywhere.
  await page.fill('#palette-query', 'pull . 42');
  assert.deepEqual(await rows(page), ['# Windows', 'Pull request #42 · bench | Research · Brave']);
});

test('Jump to… finds a word with letters left out once nothing has it as typed', async (t) => {
  const world = defaultWorld();
  world.items.w9.title = 'Speech notes';
  world.items.w10.title = 'Spec draft';
  const { page, close } = await openBench({ world });
  t.after(close);
  // "termnal" is "terminal" without its i: the terminals come first, as
  // their app is part of their whole title.
  await jump(page, 'termnal');
  const found = await rows(page);
  assert.deepEqual(found.slice(0, 3), ['# Windows', 'zsh — ~/code/bench | Code · Terminal', 'go test — ~/code/bench | Code · Terminal']);
  // The letters have to be in a word that starts with the same letter, so
  // that a few letters scattered across words don't find everything.
  await page.fill('#palette-query', 'rmnl');
  assert.equal(await page.textContent('.palette-empty'), 'Nothing matches “rmnl”.');
  await page.fill('#palette-query', 'fill');
  assert.deepEqual(await labels(page), ['Fill the screen']);
  // While a row has the word as typed, no row is found by its letters
  // alone: "spec" finds the draft, not the speech.
  await page.fill('#palette-query', 'spec');
  assert.deepEqual(await rows(page), ['# Windows', 'Spec draft | Research · Obsidian']);
  await page.fill('#palette-query', 'spch');
  assert.deepEqual(await rows(page), ['# Windows', 'Speech notes | Research · Brave']);
});

test('Jump to… ranks the start of a title first, then a word in it, then the hint, and keeps groups together', async (t) => {
  const { page, close } = await openBench();
  t.after(close);
  // Zed's window has its app in its whole title, so it comes before the
  // tab whose hint names Zed and the This window rows about it.
  await jump(page, 'zed');
  const found = await rows(page);
  assert.equal(found[0], '# Windows');
  assert.equal(found[1], 'bench — switch.go | Code · Zed');
  assert.equal(await selected(page), 'bench — switch.go');
  // A tab's name finds the tab first.
  await page.fill('#palette-query', 'research');
  assert.deepEqual((await rows(page)).slice(0, 2), ['# Tabs', 'Research | Brave, Obsidian']);
  // The best row's group comes first, so Return takes the best: an
  // action's label before a window whose hint has the word.
  await page.fill('#palette-query', 'close');
  const close1 = await rows(page);
  assert.deepEqual(close1.slice(0, 2), ['# This window', 'Close window | Zed — bench — switch.go']);
  assert.ok(close1.indexOf('# Actions') > 0 && close1.includes('Close tab… | Code'), JSON.stringify(close1));
  // Accents don't count.
  await page.fill('#palette-query', 'résearch');
  assert.deepEqual((await rows(page)).slice(0, 2), ['# Tabs', 'Research | Brave, Obsidian']);
});

test('the add dialog finds what\'s typed as it\'s typed, and offers to run what matches nothing', async (t) => {
  const world = defaultWorld();
  world.items.w10.title = 'spec.md';
  const { page, close } = await openBench({ world });
  t.after(close);
  await page.click('.pane[data-pane-id="p4"] [data-action="add"]');
  await page.waitForSelector('#palette-query');
  await page.fill('#palette-query', 'V3 DOCS');
  assert.deepEqual(await rows(page), ['# Windows in other tabs', 'Wails v3 docs | Research · Brave']);
  // Not word by word, as Jump to… finds them: text that matches nothing as
  // typed may be a command, such as "obsidian .", which would otherwise
  // find Obsidian's spec.md, and Return would add that window instead.
  await page.fill('#palette-query', 'obsidian .');
  assert.deepEqual(await labels(page), ['Run “obsidian .” in a new terminal…']);
  await page.keyboard.press('Enter');
  await page.waitForSelector('#terminal-command');
  assert.equal(await page.inputValue('#terminal-command'), 'obsidian .');
  await page.keyboard.press('Escape');
  await page.waitForSelector('#palette-query');
  // It doesn't look for letters left out either.
  await page.fill('#palette-query', 'fgma');
  assert.deepEqual(await labels(page), ['Run “fgma” in a new terminal…']);
});

test('This window lists the focused pane\'s window\'s ⋯ menu, with its shortcuts, and does what it does', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await jump(page);
  assert.deepEqual(await groupOf(page, 'This window'), [
    'Show side by side (split right) | Zed — bench — switch.go',
    'Split down | Zed — bench — switch.go',
    'Split right with an empty pane | Zed — bench — switch.go',
    'Split down with an empty pane | Zed — bench — switch.go',
    'Move right | Zed — bench — switch.go',
    'Move to pane ▸ With Terminal — zsh — ~/code/bench | Zed — bench — switch.go',
    'Move to tab ▸ Comms | Zed — bench — switch.go',
    'Move to tab ▸ Research | Zed — bench — switch.go',
    'Move to new tab | Zed — bench — switch.go',
    'Also show in Comms | Zed — bench — switch.go',
    'Also show in Research | Zed — bench — switch.go',
    'Also show in new tab | Zed — bench — switch.go',
    'Pop out of Bench | Zed — bench — switch.go',
    'Close window | Zed — bench — switch.go',
    'Window options… | Zed — bench — switch.go',
  ]);
  // The splits have the move shortcut's arrows.
  await page.fill('#palette-query', 'split down');
  assert.equal(await selected(page), 'Split down');
  assert.equal(await page.textContent('.palette-item[aria-selected="true"] .palette-keys'), '⌃⌥⇧↓  ↵');

  // Choosing a row does what the menu's entry does.
  await page.fill('#palette-query', 'move to tab research');
  assert.deepEqual(await labels(page), ['Move to tab ▸ Research']);
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('MoveItemToTab')).args, ['w3', 2]);
  await page.waitForSelector('.palette', { state: 'detached' });
  await jump(page, 'pop out');
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('PopOut')).args, ['w3']);
});

test('This window has a terminal\'s rows, and Close asks first about a terminal that\'s running something, as elsewhere', async (t) => {
  const world = defaultWorld();
  world.tabs[1].focus = 'p4';
  world.busy = { w5: 'npm run dev' };
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await jump(page);
  const group = await groupOf(page, 'This window');
  for (const row of ['Rename… | Terminal — zsh — ~/code/bench', 'New terminal in ~/code/bench | Terminal — zsh — ~/code/bench',
    'Move right | Terminal — zsh — ~/code/bench', 'Close terminal | Terminal — zsh — ~/code/bench']) {
    assert.ok(group.includes(row), row + ' in ' + JSON.stringify(group));
  }
  // Move left is off for the first sub-tab, so it's left out.
  assert.ok(!group.some((r) => r.startsWith('Move left')));
  await page.fill('#palette-query', 'close terminal');
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('CloseItem')).args, ['w5', false, false]);
  await page.waitForSelector('.question-sheet');
  assert.equal(await page.textContent('#question-title'), 'Close terminal?');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.question-sheet', { state: 'detached' });

  // Rename… opens the field on the terminal's sub-tab.
  await jump(page, 'rename');
  assert.equal(await selected(page), 'Rename…');
  await page.keyboard.press('Enter');
  await page.waitForSelector('.palette', { state: 'detached' });
  await page.waitForFunction(() => document.activeElement && document.activeElement.dataset.key === 'rename-input');
  assert.equal(await page.inputValue('[data-key="rename-input"]'), 'zsh — ~/code/bench');
});

test('This window is left out for an empty pane, whose Actions have Close this pane', async (t) => {
  const world = defaultWorld();
  world.tabs[1].root.children[1].items = [];
  world.tabs[1].root.children[1].active = '';
  world.tabs[1].focus = 'p4';
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await jump(page);
  const all = await rows(page);
  assert.ok(!all.includes('# This window'), JSON.stringify(all));
  assert.ok(all.includes('Close this pane | Empty pane'), JSON.stringify(all));
  assert.ok(all.includes('Zoom this pane | Empty pane'), JSON.stringify(all));
  await page.fill('#palette-query', 'close pane');
  assert.equal(await selected(page), 'Close this pane');
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('ClosePane')).args, ['p4']);
});

test('a pane with a window has no Close this pane, and one pane alone has none either', async (t) => {
  const world = defaultWorld();
  world.tabs[1].root = world.tabs[1].root.children[0];
  const { page, close } = await openBench({ world });
  t.after(close);
  await jump(page, 'close pane');
  assert.ok(!(await labels(page)).includes('Close this pane'));
});

test('Actions add to the focused pane, zoom it, close and delete the tab, reopen one, and open any tab\'s properties', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ closedTabs: ['Notes'] }) });
  t.after(close);
  await jump(page);
  const actions = await groupOf(page, 'Actions');
  for (const row of [
    'Zoom this pane | Zed — bench — switch.go',
    'Add a window or app to this pane… | A window, app, web page, or terminal',
    'Close tab… | Code',
    'Delete tab (keep windows) | Code',
    'Tab properties of Code… | Name, layout, small screens',
    'Tab properties of Comms… | Name, layout, small screens',
    'Tab properties of Research… | Name, layout, small screens',
    'Reopen closed tab | Notes',
    'Keyboard shortcuts | Help: what each shortcut does',
  ]) {
    assert.ok(actions.includes(row), row + ' in ' + JSON.stringify(actions));
  }
  // The zoom shortcut is on its row.
  await page.fill('#palette-query', 'zoom');
  assert.equal(await page.textContent('.palette-item[aria-selected="true"] .palette-keys'), '⌃⌥↩  ↵');

  // Add a window or app to this pane… opens the add dialog for it.
  await page.fill('#palette-query', 'add window pane');
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('AddOptions')).args, ['p3']);
  await page.waitForFunction(() => window.bench.store.ui.overlay === 'add');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.palette', { state: 'detached' });

  // New terminal… opens the add dialog for it at its New terminal… form,
  // and Add a web page… at Web page…'s address.
  fake.clearCalls();
  await jump(page, 'new terminal');
  assert.equal(await selected(page), 'New terminal…');
  assert.equal(await page.textContent('.palette-item[aria-selected="true"] .palette-hint'), 'In this pane: a command, a name, and a folder');
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('AddOptions')).args, ['p3']);
  await page.waitForFunction(() => document.activeElement && document.activeElement.id === 'terminal-command');
  assert.equal(await page.textContent('#terminal-title'), 'New terminal');
  await page.keyboard.press('Escape');
  await page.waitForSelector('#palette-query');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.palette', { state: 'detached' });
  await jump(page, 'web page');
  assert.equal(await selected(page), 'Add a web page…');
  await page.keyboard.press('Enter');
  await page.waitForFunction(() => document.activeElement && document.activeElement.dataset.key === 'palette-url');
  assert.equal(await page.evaluate(() => window.bench.store.ui.add.paneId), 'p3');
  await page.keyboard.press('Escape');
  await page.waitForSelector('#palette-query');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.palette', { state: 'detached' });

  // Another tab's properties open for that tab.
  await jump(page, 'properties research');
  await page.keyboard.press('Enter');
  await page.waitForSelector('.editor-sheet');
  assert.equal(await page.inputValue('#tab-name'), 'Research');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.editor-sheet', { state: 'detached' });

  // Close tab… asks first, as it does from the tab's menu.
  fake.clearCalls();
  await jump(page, 'close tab');
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('CloseTab')).args, [1, false, false]);
  await page.waitForSelector('.question-sheet');
  assert.equal(await page.textContent('#question-title'), 'Close Code?');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.question-sheet', { state: 'detached' });

  // Delete tab (keep windows) keeps them, with Undo.
  await jump(page, 'delete tab');
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('DeleteTab')).args, [1]);
  await page.waitForSelector('.palette', { state: 'detached' });

  // Reopen closed tab names the tab it brings back.
  await jump(page, 'reopen notes');
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('ReopenTab')).args, ['Notes']);
});

test('Add front window to this pane adds the window in front that\'s in no tab, with its shortcut, and Pop out takes it out again', async (t) => {
  const world = defaultWorld({ keys: { addFrontWindow: '⌃⌥A' } });
  // Finder's window was in front before Bench, and is in no tab.
  world.front = 'f102';
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await jump(page, 'front window');
  assert.equal(await selected(page), 'Add front window to this pane');
  assert.equal(await page.textContent('.palette-item[aria-selected="true"] .palette-hint'), 'Finder — Downloads');
  assert.equal(await page.textContent('.palette-item[aria-selected="true"] .palette-keys'), '⌃⌥A  ↵');
  await page.keyboard.press('Enter');
  await fake.waitForCall('AddFrontWindow');
  await page.waitForFunction(() => window.bench.store.view.items.f102 && window.bench.store.view.items.f102.pane === 'p3');
  await page.waitForFunction(() => document.querySelector('#toast .toast-text')
    && document.querySelector('#toast .toast-text').textContent === 'Added Finder — Downloads to this pane.');
  // The toast's Pop out calls PopOut with the window's item.
  assert.equal(await page.textContent('#toast .toast-action'), 'Pop out');
  await page.click('#toast .toast-action');
  assert.deepEqual((await fake.waitForCall('PopOut')).args, ['f102']);

  // With no window in front out of the tabs, there's no such row.
  await jump(page);
  assert.ok(!(await labels(page)).includes('Add front window to this pane'));
});

test('the tab used last comes first, with the last_tab shortcut, and the windows used last before the rest', async (t) => {
  const world = defaultWorld({ lastTab: 2 });
  world.items.w9.used = 7;
  world.items.w2.used = 5;
  world.used = 7;
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await jump(page);
  // Research was used before Code; the rest keep the tab strip's order,
  // which their shortcuts number.
  assert.deepEqual(await groupOf(page, 'Tabs'), [
    'Research | Brave, Obsidian · last used',
    'Comms | Slack, Outlook',
    'Code | Zed, VS Code, Terminal · current',
  ]);
  const keys = await page.$$eval('.palette-item .palette-keys', (els) => els.slice(0, 3).map((el) => el.textContent));
  assert.deepEqual(keys, ['⌃⌥`  ↵', '⌃⌥1', '⌃⌥2']);
  assert.deepEqual((await groupOf(page, 'Windows')).slice(0, 3), [
    'Go packages | Research · Brave',
    'Inbox | Comms · Outlook',
    '# general | Comms · Slack',
  ]);
  // Return with nothing typed goes back to it.
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('SwitchTab')).args, [2]);
  await page.waitForSelector('.palette', { state: 'detached' });
  // Now Code is the tab used last, and what Research's focused pane shows
  // is the window used last.
  await page.waitForFunction(() => window.bench.store.view.current === 2 && window.bench.store.view.lastTab === 1);
  await jump(page);
  assert.equal((await groupOf(page, 'Tabs'))[0], 'Code | Zed, VS Code, Terminal · last used');
  assert.equal((await groupOf(page, 'Windows'))[0], 'Wails v3 docs | Research · Brave');
});

test('with no tab used before this one, the tabs keep their order', async (t) => {
  const { page, close } = await openBench();
  t.after(close);
  await jump(page);
  assert.deepEqual(await groupOf(page, 'Tabs'), [
    'Comms | Slack, Outlook',
    'Code | Zed, VS Code, Terminal · current',
    'Research | Brave, Obsidian',
  ]);
});

test('the browser preview\'s last_tab shortcut goes back and forth between the two tabs used last', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ demo: true }) });
  t.after(close);
  await page.click('.statusbar');
  // Before a switch, there's none to go back to.
  await page.keyboard.press('Control+Alt+Backquote');
  await fake.waitForCall('LastTab');
  await page.waitForFunction(() => document.querySelector('#toast').textContent === 'There\'s no tab to go back to yet. Switch tabs first.');
  await page.keyboard.press('Control+Alt+Digit3');
  await page.waitForFunction(() => window.bench.store.view.current === 2);
  fake.clearCalls();
  await page.keyboard.press('Control+Alt+Backquote');
  await fake.waitForCall('LastTab');
  await page.waitForFunction(() => window.bench.store.view.current === 1);
  await page.keyboard.press('Control+Alt+Backquote');
  await page.waitForFunction(() => window.bench.store.view.current === 2);
  await settle(page);
  assert.equal(fake.callsTo('LastTab').length, 2);
});
