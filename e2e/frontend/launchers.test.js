'use strict';

// The add dialog's New terminal… form and its Run "x" offer, Save as
// launcher, Choose…, Browse…, launchers in the add dialog and Jump to…,
// and Settings ▸ Launchers.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, openSettings, settle } = require('./helpers/harness.js');
const { defaultWorld } = require('./helpers/fixture.js');

const rows = (page) => page.$$eval('#overlay-root .palette-list > :not(.palette-note)', (els) => els.map((el) => (el.classList.contains('palette-group')
  ? '# ' + el.textContent
  : el.querySelector('.palette-label').textContent + ' | ' + el.querySelector('.palette-hint').textContent)));
const focused = (page) => page.evaluate(() => document.activeElement && (document.activeElement.dataset.key || document.activeElement.id));
const toastIs = (page, text) => page.waitForFunction((t) => document.getElementById('toast').textContent === t, text);

// LAUNCHERS are the launchers of the worlds that have some: two terminals
// and Zed on a folder.
const LAUNCHERS = [
  { name: 'Claude', command: 'claude --continue', folder: '~/code/bench' },
  { name: 'Deploy', command: 'make deploy', folder: '~/code/site', startAgain: false },
  { name: 'Bench in Zed', bundleId: 'dev.zed.Zed', app: 'Zed', folder: '~/code/bench' },
];

// openAdd opens the add dialog for p4, in the Code tab.
async function openAdd(page) {
  await page.click('.pane[data-pane-id="p4"] [data-action="add"]');
  await page.waitForSelector('#palette-query');
}

// openForm opens the add dialog's New terminal… form for p4.
async function openForm(page) {
  await openAdd(page);
  await page.click('.palette-item:has-text("New terminal…")');
  await page.waitForSelector('#terminal-command');
}

test('the add dialog says how to add any app, and offers to run text that matches nothing in a terminal', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await openAdd(page);
  assert.equal(await page.textContent('.palette-foot span:last-child'), 'Open any app, and its windows show under Windows not in a tab');
  await page.keyboard.type('claude --continue');
  await page.waitForSelector('#palette-nothing');
  assert.equal(await page.textContent('#palette-nothing'),
    'Nothing matches “claude --continue”. To add the window of an app that isn\'t listed, open the app: its windows show here, under Windows not in a tab.');
  assert.deepEqual(await rows(page), ['# Terminal', 'Run “claude --continue” in a new terminal… | Types it into your shell']);
  assert.equal(await page.getAttribute('.palette-item', 'aria-selected'), 'true');

  // Return opens the form with the command typed, and the name to fill in.
  await page.keyboard.press('Enter');
  await page.waitForSelector('#terminal-command');
  assert.equal(await page.getAttribute('.palette', 'aria-labelledby'), 'terminal-title');
  assert.equal(await page.textContent('#terminal-title'), 'New terminal');
  assert.equal(await page.inputValue('#terminal-command'), 'claude --continue');
  assert.equal(await focused(page), 'terminal-name');
  assert.equal(await page.isChecked('#terminal-start-again'), true);
  assert.equal(await page.isChecked('#terminal-save'), false);
  assert.equal(fake.callsTo('AddToPane').length, 0);

  // A new View doesn't lose what's typed.
  await page.keyboard.type('Claude');
  await page.fill('#terminal-folder', '~/code/bench');
  await fake.emitState();
  await settle(page);
  assert.equal(await page.inputValue('#terminal-name'), 'Claude');
  assert.equal(await page.inputValue('#terminal-folder'), '~/code/bench');

  // Return in a field adds the terminal and closes the dialog.
  await page.press('#terminal-folder', 'Enter');
  const add = await fake.waitForCall('AddToPane');
  assert.deepEqual(add.args, ['p4', { type: 'terminal', folder: '~/code/bench', command: 'claude --continue', name: 'Claude', startAgain: true }]);
  await page.waitForSelector('.palette', { state: 'detached' });
  assert.equal(fake.callsTo('SaveLauncher').length, 0);
});

test('New terminal… asks for a command, a name, and a folder, and Escape goes back to the list', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await openForm(page);
  assert.equal(await focused(page), 'terminal-command');
  assert.equal(await page.inputValue('#terminal-command'), '');
  assert.deepEqual(await page.$$eval('.terminal-form .field-label', (els) => els.map((el) => el.textContent)), ['Command', 'Name', 'Folder']);
  assert.deepEqual(await page.$$eval('.terminal-form .toggle span', (els) => els.map((el) => el.textContent)),
    ['Start again when Bench opens', 'Save as launcher']);
  assert.equal(await page.textContent('#terminal-start-again-help'),
    'After Bench restarts, the terminal runs the command again. Untick it for a command to run once, such as a deploy.');

  await page.keyboard.press('Escape');
  await page.waitForSelector('#palette-query');
  assert.equal(await page.isVisible('.palette'), true);

  // A command to run once doesn't start again when Bench opens.
  await page.click('.palette-item:has-text("New terminal…")');
  await page.waitForSelector('#terminal-command');
  await page.keyboard.type('make deploy');
  await page.click('#terminal-start-again');
  assert.equal(await page.isChecked('#terminal-start-again'), false);
  await page.click('[data-action="add-terminal"]');
  const add = await fake.waitForCall('AddToPane');
  assert.deepEqual(add.args, ['p4', { type: 'terminal', folder: '', command: 'make deploy', name: '', startAgain: false }]);
  await page.waitForSelector('.palette', { state: 'detached' });
});

test('Save as launcher saves the terminal first, and the add dialog then lists the launcher first', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await openForm(page);
  await page.keyboard.type('codex resume --last');
  await page.click('#terminal-save');
  await page.press('#terminal-command', 'Enter');
  const save = await fake.waitForCall('SaveLauncher');
  assert.deepEqual(save.args, [{ kind: 'terminal', name: '', command: 'codex resume --last', folder: '', startAgain: true }]);
  // Go named the launcher, and the terminal takes its name.
  const add = await fake.waitForCall('AddToPane');
  assert.deepEqual(add.args, ['p4', { type: 'terminal', folder: '', command: 'codex resume --last', name: 'codex', startAgain: true }]);
  assert.ok(fake.calls.indexOf(save) < fake.calls.indexOf(add));
  await toastIs(page, 'Saved launcher codex');
  await page.waitForSelector('.palette', { state: 'detached' });

  await openAdd(page);
  assert.deepEqual((await rows(page)).slice(0, 3), ['# Launchers', 'codex | codex resume --last · ~', '# Windows not in a tab']);
  // Typing its name chooses it, before the rest.
  await page.keyboard.type('codex');
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('AddToPane', (a) => a[1].type === 'launcher')).args, ['p4', { type: 'launcher', id: 'codex' }]);
});

test('a refused launcher keeps the form open, and one that was saved isn\'t saved again', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ launchers: LAUNCHERS }) });
  t.after(close);
  await openForm(page);
  await page.fill('#terminal-command', 'claude');
  await page.fill('#terminal-name', 'claude');
  await page.click('#terminal-save');
  await page.click('[data-action="add-terminal"]');
  // It names the launcher that has the name.
  await toastIs(page, 'There\'s a launcher called Claude already. Give this one another name.');
  assert.equal(await page.isVisible('#terminal-command'), true);
  assert.equal(fake.callsTo('AddToPane').length, 0);

  // Saved, but not added: Add again adds it without saving it twice.
  let refuse = true;
  fake.on('AddToPane', () => {
    if (refuse) {
      refuse = false;
      throw new Error('That pane has gone.');
    }
    return null;
  });
  await page.fill('#terminal-name', 'Claude 2');
  await page.click('[data-action="add-terminal"]');
  await fake.waitForCall('AddToPane');
  assert.equal(fake.callsTo('SaveLauncher').length, 2);
  await page.waitForFunction(() => !document.getElementById('terminal-save').checked);
  assert.equal(await page.isVisible('#terminal-command'), true);
  await page.click('[data-action="add-terminal"]');
  await page.waitForSelector('.palette', { state: 'detached' });
  assert.equal(fake.callsTo('SaveLauncher').length, 2);
  assert.deepEqual(fake.callsTo('AddToPane').pop().args,
    ['p4', { type: 'terminal', folder: '', command: 'claude', name: 'Claude 2', startAgain: true }]);
});

test('Choose… puts the folder chosen in the Folder field, and keeps Bench raised while the picker is open', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await openForm(page);
  await page.fill('#terminal-folder', '~/code');
  await page.click('[data-action="choose-folder"]');
  assert.deepEqual((await fake.waitForCall('ChooseFolder')).args, ['~/code']);
  await page.waitForFunction(() => document.getElementById('terminal-folder').value === '~/code/bench');
  assert.equal(await focused(page), 'terminal-folder');
  await page.press('#terminal-folder', 'Enter');
  assert.equal((await fake.waitForCall('AddToPane')).args[1].folder, '~/code/bench');
  await page.waitForSelector('.palette', { state: 'detached' });

  // A tab switch closes the dialog while the picker is open, as a global
  // shortcut can; Bench stays raised until the picker closes, and the
  // folder chosen is dropped.
  let answer;
  fake.on('ChooseFolder', () => new Promise((r) => { answer = r; }));
  await openForm(page);
  fake.clearCalls();
  await page.click('[data-action="choose-folder"]');
  await fake.waitForCall('ChooseFolder');
  fake.world.current = 0;
  await fake.emitState();
  await page.waitForSelector('.palette', { state: 'detached' });
  await settle(page);
  assert.deepEqual(fake.callsTo('SetOverlay').map((c) => c.args[0]).filter((on) => !on), []);
  answer('~/notes');
  await fake.waitForCall('SetOverlay', (a) => a[0] === false);
  assert.equal(fake.callsTo('AddToPane').length, 0);
});

test('where there\'s no folder picker, Choose… says to type the folder', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  fake.on('ChooseFolder', () => { throw new Error('This build of Bench can\'t show a folder picker. Type the folder instead, such as ~/code/bench.'); });
  await openForm(page);
  await page.click('[data-action="choose-folder"]');
  await toastIs(page, 'This build of Bench can\'t show a folder picker. Type the folder instead, such as ~/code/bench.');
  await page.waitForFunction(() => document.activeElement.id === 'terminal-folder');
  assert.equal(await page.isVisible('#terminal-command'), true);
});

test('New terminal in… asks for a folder, starting where the terminal in the pane is, and opens a terminal there', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  let answer;
  fake.on('ChooseFolder', () => new Promise((r) => { answer = r; }));
  await openAdd(page);
  fake.clearCalls();
  await page.click('.palette-item:has-text("New terminal in…")');
  assert.deepEqual((await fake.waitForCall('ChooseFolder')).args, ['~/code/bench']);
  // Bench stays raised while the picker is open, with the list there.
  await settle(page);
  assert.deepEqual(fake.callsTo('SetOverlay').map((c) => c.args[0]).filter((on) => !on), []);
  assert.equal(await page.isVisible('#palette-query'), true);
  answer('~/notes');
  assert.deepEqual((await fake.waitForCall('AddToPane')).args, ['p4', { type: 'terminal', folder: '~/notes' }]);
  await page.waitForSelector('.palette', { state: 'detached' });

  // Cancelled, the dialog stays at its list.
  fake.on('ChooseFolder', () => '');
  await openAdd(page);
  fake.clearCalls();
  await page.click('.palette-item:has-text("New terminal in…")');
  await fake.waitForCall('ChooseFolder');
  await settle(page);
  assert.equal(await page.isVisible('#palette-query'), true);
  assert.equal(fake.callsTo('AddToPane').length, 0);
});

test('in the browser preview, New terminal in… says to type the folder, in the New terminal form', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ demo: true }) });
  t.after(close);
  await openAdd(page);
  await page.click('.palette-item:has-text("New terminal in…")');
  await toastIs(page, 'This build of Bench can\'t show a folder picker. Type the folder instead, such as ~/code/bench.');
  await page.waitForFunction(() => document.activeElement.id === 'terminal-folder');
  assert.equal(await page.inputValue('#terminal-folder'), '~/code/bench');
  assert.equal(await page.inputValue('#terminal-command'), '');
  await page.fill('#terminal-folder', '~/code/site');
  await page.press('#terminal-folder', 'Enter');
  assert.equal((await fake.waitForCall('AddToPane')).args[1].folder, '~/code/site');
});

// BROWSE_PREVIEW is what Go says about Browse… in the browser preview.
const BROWSE_PREVIEW = 'The browser preview can\'t look for apps or programs on this computer. In the Mac app, Browse… finds one on your Mac.';

// browse clicks the add dialog's Browse… for p4, with the open panel's
// answer held back until the returned function gives it.
async function browse(page, fake) {
  let answer;
  fake.on('BrowseToAdd', (args, f) => new Promise((r) => { answer = () => r(f.mBrowseToAdd(...args)); }));
  await openAdd(page);
  fake.clearCalls();
  await page.click('.palette-item:has-text("Browse…")');
  assert.deepEqual((await fake.waitForCall('BrowseToAdd')).args, ['p4']);
  return () => answer();
}

test('Browse… closes the add dialog first, lowering Bench, and fills in the New terminal form with the program chosen', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await openAdd(page);
  assert.ok((await rows(page)).includes('Browse… | An app, or a program to run in a terminal'));
  await page.keyboard.press('Escape');
  await page.waitForSelector('.palette', { state: 'detached' });

  fake.browsed = { command: "~/'My Tools/run'", name: 'run', folder: '~/My Tools' };
  const answer = await browse(page, fake);
  // The panel is a window of its own, which stays in front of Bench only
  // while Bench is lowered.
  await page.waitForSelector('.palette', { state: 'detached' });
  const lowered = await fake.waitForCall('SetOverlay', (a) => a[0] === false);
  assert.ok(fake.calls.indexOf(lowered) < fake.calls.indexOf(fake.callsTo('BrowseToAdd')[0]), 'Bench should be lowered first');
  assert.equal(fake.callsTo('AddOptions').length, 0);

  answer();
  await page.waitForSelector('#terminal-command');
  assert.deepEqual(fake.callsTo('AddOptions').map((c) => c.args), [['p4']]);
  assert.equal(await page.inputValue('#terminal-command'), "~/'My Tools/run'");
  assert.equal(await page.inputValue('#terminal-folder'), '~/My Tools');
  assert.equal(await page.inputValue('#terminal-name'), 'run');
  assert.equal(await focused(page), 'terminal-name');
  await page.click('#terminal-command');
  await page.keyboard.press('End');
  await page.keyboard.type(' --fast');
  await page.press('#terminal-command', 'Enter');
  assert.deepEqual((await fake.waitForCall('AddToPane')).args,
    ['p4', { type: 'terminal', folder: '~/My Tools', command: "~/'My Tools/run' --fast", name: 'run', startAgain: true }]);
  await page.waitForSelector('.palette', { state: 'detached' });
});

test('Browse… adds the app chosen, opens the add dialog again if the user cancels, and not if the tab changed', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  // Go adds the app: the dialog stays closed.
  fake.browsed = { added: 'Linear' };
  let answer = await browse(page, fake);
  answer();
  await settle(page);
  assert.equal(await page.isVisible('.palette'), false);
  assert.equal(fake.callsTo('AddOptions').length, 0);
  assert.equal(fake.callsTo('AddToPane').length, 0);

  // Cancelled, the dialog opens again at its list.
  fake.browsed = null;
  answer = await browse(page, fake);
  answer();
  await page.waitForSelector('#palette-query');
  assert.equal(await focused(page), 'palette-query');
  assert.deepEqual(fake.callsTo('AddOptions').map((c) => c.args), [['p4']]);
  await page.keyboard.press('Escape');
  await page.waitForSelector('.palette', { state: 'detached' });

  // The tab changed while the panel was open: the dialog doesn't open, and
  // a toast says why the form didn't.
  fake.browsed = { command: 'tool', folder: '' };
  answer = await browse(page, fake);
  fake.world.current = 0;
  await fake.emitState();
  await settle(page);
  answer();
  await toastIs(page, 'Bench didn\'t open the New terminal form, as you switched tabs or opened something else meanwhile. Use Browse… again.');
  assert.equal(await page.isVisible('.palette'), false);
  assert.equal(fake.callsTo('AddOptions').length, 0);
});

test('Browse… in a split\'s empty pane keeps the pane while the panel is open, for the dialog that opens again', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  fake.browsed = { command: 'tool', folder: '' };
  let answer;
  fake.on('BrowseToAdd', (args, f) => new Promise((r) => { answer = () => r(f.mBrowseToAdd(...args)); }));
  await page.click('.pane[data-pane-id="p4"] [data-action="menu"]');
  await page.click('#window-menu .menu-item:has-text("Split right with an empty pane")');
  const id = (await fake.waitForCall('AddOptions')).args[0];
  await page.waitForSelector('#palette-query');
  await page.click('.palette-item:has-text("Browse…")');
  await fake.waitForCall('BrowseToAdd');
  await page.waitForSelector('.palette', { state: 'detached' });
  await settle(page);
  assert.equal(fake.callsTo('CancelSplit').length, 0);
  answer();
  await page.waitForSelector('#terminal-command');
  assert.equal(await page.evaluate(() => window.bench.store.ui.add.paneId), id);
  assert.equal(fake.callsTo('CancelSplit').length, 0);

  // The form's dialog has the pane now: Escape twice closes it, and the
  // pane goes.
  await page.keyboard.press('Escape');
  await page.waitForSelector('#palette-query');
  await page.keyboard.press('Escape');
  assert.deepEqual((await fake.waitForCall('CancelSplit')).args, [id]);
});

test('in the browser preview, Browse… says that it can\'t look for apps or programs', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ demo: true }) });
  t.after(close);
  await openAdd(page);
  await page.click('.palette-item:has-text("Browse…")');
  await toastIs(page, BROWSE_PREVIEW);
  assert.equal(await page.isVisible('.palette'), false);
  assert.equal(fake.callsTo('AddOptions').length, 1);
});

test('Jump to… lists the launchers after the tabs and windows, finds them by name only, and adds one to the focused pane', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ launchers: LAUNCHERS }) });
  t.after(close);
  const openJump = async (query) => {
    await page.click('[data-action="open-palette"]');
    await page.waitForSelector('#palette-query');
    if (query) await page.fill('#palette-query', query);
  };
  await openJump('');
  const all = await rows(page);
  assert.deepEqual(all.slice(0, 2), ['# Tabs', 'Comms | Slack, Outlook']);
  const at = all.indexOf('# Launchers');
  assert.ok(at > all.indexOf('# Windows'), JSON.stringify(all));
  assert.deepEqual(all.slice(at, at + 5), [
    '# Launchers',
    'Claude | claude --continue · ~/code/bench',
    'Deploy | make deploy · ~/code/site',
    'Bench in Zed | Zed · ~/code/bench',
    '# This window',
  ]);
  // Return with nothing typed goes to the first tab, as no tab was used
  // before this one, and starts nothing.
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('SwitchTab')).args, [0]);
  await page.waitForSelector('.palette', { state: 'detached' });

  // A word in a launcher's command or folder doesn't find it, so a tab's
  // name finds the tab first.
  await openJump('code');
  assert.deepEqual((await rows(page)).slice(0, 2), ['# Tabs', 'Code | Zed, VS Code, Terminal · last used']);
  assert.ok(!(await rows(page)).includes('# Launchers'));
  await page.keyboard.press('Enter');
  await fake.waitForCall('SwitchTab', (a) => a[0] === 1);
  await page.waitForSelector('.palette', { state: 'detached' });
  assert.equal(fake.callsTo('AddToPane').length, 0);

  // Its name does, and choosing it adds it to the focused pane.
  await openJump('claude');
  assert.deepEqual(await rows(page), ['# Launchers', 'Claude | claude --continue · ~/code/bench']);
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('AddToPane')).args, ['p3', { type: 'launcher', id: 'Claude' }]);
  await page.waitForSelector('.palette', { state: 'detached' });

  // The add dialog lists them first, and finds them by name only too.
  await openAdd(page);
  assert.deepEqual((await rows(page)).slice(0, 5), [
    '# Launchers', 'Claude | claude --continue · ~/code/bench', 'Deploy | make deploy · ~/code/site', 'Bench in Zed | Zed · ~/code/bench',
    '# Windows not in a tab',
  ]);
  await page.fill('#palette-query', 'site');
  assert.ok(!(await rows(page)).includes('# Launchers'));
  await page.fill('#palette-query', 'deploy');
  assert.deepEqual((await rows(page)).slice(0, 2), ['# Launchers', 'Deploy | make deploy · ~/code/site']);
});

test('Settings ▸ Launchers edits, removes, and adds launchers, which Save sends', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ launchers: LAUNCHERS }) });
  t.after(close);
  await openSettings(page);
  const list = () => page.$$eval('.launcher .launcher-head', (els) => els.map((el) => el.querySelector('.launcher-name').textContent
    + (el.querySelector('.launcher-hint') ? ' | ' + el.querySelector('.launcher-hint').textContent : '')));
  assert.deepEqual(await list(), ['Claude | claude --continue · ~/code/bench', 'Deploy | make deploy · ~/code/site', 'Bench in Zed | Zed · ~/code/bench']);
  assert.equal(await page.getAttribute('[data-key="launcher-edit-0"]', 'aria-label'), 'Edit Claude');

  // A terminal launcher's command, name, folder, and Start again.
  await page.click('[data-key="launcher-edit-0"]');
  await page.waitForSelector('#launcher-command');
  assert.equal(await focused(page), 'launcher-command');
  assert.equal(await page.inputValue('#launcher-command'), 'claude --continue');
  assert.equal(await page.inputValue('#launcher-name'), 'Claude');
  assert.equal(await page.inputValue('#launcher-folder'), '~/code/bench');
  assert.equal(await page.isChecked('#launcher-start-again'), true);
  await page.fill('#launcher-command', 'claude');
  await page.click('#launcher-start-again');
  await page.click('[data-action="launcher-done"]');
  assert.equal(await focused(page), 'launcher-edit-0');
  assert.deepEqual((await list())[0], 'Claude | claude · ~/code/bench');

  // Remove takes one out of the list.
  await page.click('[data-key="launcher-remove-1"]');
  assert.deepEqual((await list()).length, 2);

  // An app launcher's name, folder, link, and window title.
  await page.click('[data-key="launcher-edit-1"]');
  await page.waitForSelector('#launcher-url');
  assert.equal(await page.$('#launcher-command'), null);
  assert.equal(await page.textContent('.launcher.editing .launcher-name'), 'Zed launcher');
  await page.fill('#launcher-title', 'bench');

  // A new terminal launcher, whose folder comes from Choose….
  await page.click('[data-action="launcher-new"]');
  await page.waitForSelector('.launcher:nth-child(3) #launcher-command');
  assert.equal(await page.$('#launcher-url'), null);
  await page.keyboard.type('codex resume --last');
  await page.fill('#launcher-name', 'Codex');
  await page.click('[data-action="choose-folder"]');
  await page.waitForFunction(() => document.getElementById('launcher-folder').value === '~/code/bench');

  await page.click('[data-action="save-settings"]');
  const save = await fake.waitForCall('SaveSettings');
  assert.deepEqual(save.args[0].launchers, [
    { name: 'Claude', kind: 'terminal', folder: '~/code/bench', command: 'claude', startAgain: false, bundleId: '', url: '', title: '' },
    { name: 'Bench in Zed', kind: 'app', folder: '~/code/bench', command: '', startAgain: false, bundleId: 'dev.zed.Zed', url: '', title: 'bench' },
    { name: 'Codex', kind: 'terminal', folder: '~/code/bench', command: 'codex resume --last', startAgain: true, bundleId: '', url: '', title: '' },
  ]);
  await page.waitForSelector('.settings-sheet', { state: 'detached' });

  await openSettings(page);
  assert.deepEqual(await list(), ['Claude | claude · ~/code/bench', 'Bench in Zed | Zed · ~/code/bench', 'Codex | codex resume --last · ~/code/bench']);
});

test('Settings sends the launchers only if they changed, Cancel keeps them, and a refusal keeps the dialog open', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ launchers: LAUNCHERS }) });
  t.after(close);
  await openSettings(page);
  await page.click('[data-key="launcher-remove-0"]');
  await page.click('[data-key="settings-cancel"]');
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
  assert.equal(fake.callsTo('SaveSettings').length, 0);

  // Saved unchanged, they're left out, so that Go keeps its own.
  await openSettings(page);
  assert.equal(await page.$$eval('.launcher', (els) => els.length), 3);
  await page.click('[data-action="save-settings"]');
  const save = await fake.waitForCall('SaveSettings');
  assert.equal('launchers' in save.args[0], false);
  await page.waitForSelector('.settings-sheet', { state: 'detached' });

  // Return in a launcher's field saves, and Go's refusal keeps the dialog
  // open with what was typed.
  fake.on('SaveSettings', () => { throw new Error('There\'s a launcher called Claude already. Give this one another name.'); });
  await openSettings(page);
  await page.click('[data-key="launcher-edit-1"]');
  await page.fill('#launcher-name', 'Claude');
  await page.press('#launcher-name', 'Enter');
  await toastIs(page, 'There\'s a launcher called Claude already. Give this one another name.');
  assert.equal(await page.inputValue('#launcher-name'), 'Claude');
  assert.equal(fake.callsTo('SaveSettings').pop().args[0].launchers[1].name, 'Claude');
});
