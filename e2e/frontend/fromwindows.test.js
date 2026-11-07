'use strict';

// A new tab of windows that are open: the New tab dialog's Start with,
// its list of windows grouped by app, the layout it suggests, Make tab and
// its Undo, the ways in (Tab ▸ New Tab from Open Windows…, Jump to…, and
// the welcome guide's set-up), and the add dialog's ticks, which add
// several windows at once.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench } = require('./helpers/harness.js');
const { defaultWorld } = require('./helpers/fixture.js');

const focusedKey = (page) => page.evaluate(() => document.activeElement && document.activeElement.dataset.key);

// listed returns the dialog's list of windows: each app's name, then each
// window's title and hint, with + if it's ticked and ~ if it's partly
// ticked.
const listed = (page, within = '.win-list > .win-groups') => page.$$eval(within + ' > .win-app', (apps) => apps.map((app) => Array.from(app.querySelectorAll('.win-row'), (row) => {
  const box = row.querySelector('input');
  const mark = box.indeterminate ? '~' : box.checked ? '+' : '-';
  const text = Array.from(row.querySelectorAll('.win-name, .win-title, .win-count, .win-hint'), (el) => el.textContent).join(' | ');
  return mark + ' ' + text;
})));

// pressed returns the data-key of the pressed button among selector's.
const pressed = (page, selector) => page.$eval(selector + '[aria-pressed="true"]', (el) => el.dataset.key);

// chips returns the preview's panes, each with the windows it shows.
const chips = (page) => page.$$eval('.mini .mini-pane', (els) => els.map((el) => Array.from(el.querySelectorAll('.chips span'), (c) => c.textContent)));

// openFromWindows opens the New tab dialog from the title bar's + and
// chooses Windows that are open.
async function openFromWindows(page) {
  await page.click('[data-action="new-tab"]');
  await page.waitForSelector('.editor-sheet');
  await page.click('[data-key="start-windows"]');
  await page.waitForSelector('.win-list');
}

test('New tab starts with an empty layout, or with the windows that are open, ticked and grouped by app', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('[data-action="new-tab"]');
  await page.waitForSelector('.editor-sheet');
  assert.equal(await pressed(page, '[data-action="draft-start"]'), 'start-empty');
  assert.equal(await page.inputValue('#tab-name'), 'New tab');
  assert.equal(await page.locator('.win-list').count(), 0);
  assert.equal(await page.textContent('[data-key="editor-save"]'), 'Save');

  await page.click('[data-key="start-windows"]');
  await page.waitForSelector('.win-list');
  assert.deepEqual(await listed(page), [['+ Brave | Release notes'], ['+ Finder | Downloads']]);
  // The name is the first app's, and the layout has a pane for each app.
  assert.equal(await page.inputValue('#tab-name'), 'Brave');
  assert.equal(await pressed(page, '.preset'), 'preset-side');
  assert.match(await page.textContent('.editor-sheet'), /Bench suggests Side by side for 2 apps\. Apps after the last pane go in it too\./);
  await page.waitForFunction(() => document.querySelectorAll('.mini .mini-pane').length === 2);
  assert.deepEqual(await chips(page), [['Brave: Release notes'], ['Finder: Downloads']]);
  assert.equal(await page.textContent('[data-key="editor-save"]'), 'Make tab');
  assert.deepEqual((await fake.waitForCall('PreviewTabFromWindows')).args, ['', ['f101', 'f102']]);

  // Unticking Brave leaves Finder: one app, one pane, named after it.
  await page.click('.win-app[data-app-name="Brave"] input');
  assert.equal(await page.inputValue('#tab-name'), 'Finder');
  assert.equal(await pressed(page, '.preset'), 'preset-single');
  await page.waitForFunction(() => document.querySelectorAll('.mini .mini-pane').length === 1);
  assert.match(await page.textContent('#win-note'), /^1 window ticked\./);
  // With nothing ticked, there's no tab to make.
  await page.click('.win-app[data-app-name="Finder"] input');
  assert.equal(await page.isDisabled('[data-key="editor-save"]'), true);
  assert.equal(await page.textContent('#win-note'), 'Tick the windows for the tab.');

  // A layout or a name the user chooses stays, whatever is ticked.
  await page.click('[data-key="preset-grid"]');
  await page.fill('#tab-name', 'Reading');
  await page.click('.win-app[data-app-name="Brave"] input');
  await page.click('.win-app[data-app-name="Finder"] input');
  assert.equal(await pressed(page, '.preset'), 'preset-grid');
  assert.equal(await page.inputValue('#tab-name'), 'Reading');
  await page.waitForFunction(() => document.querySelectorAll('.mini .mini-pane').length === 4);

  // Back to an empty layout, the name and layout stay as chosen.
  await page.click('[data-key="start-empty"]');
  assert.equal(await page.locator('.win-list').count(), 0);
  assert.equal(await page.inputValue('#tab-name'), 'Reading');
  assert.equal(await page.textContent('[data-key="editor-save"]'), 'Save');
});

test('Make tab makes the tab of the windows ticked, and its Undo takes it away', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await openFromWindows(page);
  await page.waitForFunction(() => document.querySelectorAll('.mini .mini-pane').length === 2);
  await page.click('[data-key="editor-save"]');
  assert.deepEqual((await fake.waitForCall('NewTabFromWindows')).args, [{ name: 'Brave', preset: 'side', smallScreens: 'stack' }, ['f101', 'f102']]);
  await page.waitForSelector('.editor-sheet', { state: 'detached' });
  await page.waitForFunction(() => window.bench.store.view.tabs.length === 4 && window.bench.store.view.current === 3);
  assert.equal(await page.textContent('.tab[data-index="3"] .tab-name'), 'Brave');
  assert.deepEqual(await page.evaluate(() => window.bench.store.view.floating), []);
  await page.waitForSelector('#toast.show');
  assert.match(await page.textContent('#toast'), /Made a new tab, Brave, with 2 windows\./);
  await page.click('#toast .toast-action');
  await fake.waitForCall('Undo');
  await page.waitForFunction(() => window.bench.store.view.tabs.length === 3);
  assert.deepEqual(await page.evaluate(() => window.bench.store.view.floating.map((f) => f.id)), ['f101', 'f102']);
});

test('the windows of other tabs are in a closed group, unticked, and show in the new tab as well', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await openFromWindows(page);
  const more = '[data-key="win-others"]';
  assert.equal(await page.getAttribute(more, 'aria-expanded'), 'false');
  assert.equal(await page.textContent(more), 'Windows in other tabs (10)');
  assert.equal(await page.locator('.win-others').count(), 0);
  await page.click(more);
  assert.equal(await page.getAttribute(more, 'aria-expanded'), 'true');
  assert.match(await page.textContent('.win-others'), /These stay in their tabs, and show in the new tab as well\./);
  assert.deepEqual(await listed(page, '.win-others > .win-groups'), [
    ['- Brave | 3 windows', '- Wails v3 docs | In Research', '- Pull request #42 · bench | In Research', '- Go packages | In Research'],
    ['- Obsidian | Daily note | In Research'],
    ['- Outlook | Inbox | In Comms'],
    ['- Slack | # general | In Comms'],
    ['- Terminal | 2 windows', '- zsh — ~/code/bench | In Code', '- go test — ~/code/bench | In Code'],
    ['- VS Code | index.html — bench-ui | In Code'],
    ['- Zed | bench — switch.go | In Code'],
  ]);
  // Ticking one of Brave's windows ticks Brave's row partly; Brave is
  // still one app with the one not in a tab.
  await page.click('[data-key="win-w8"]');
  assert.deepEqual((await listed(page, '.win-others > .win-groups'))[0].slice(0, 3),
    ['~ Brave | 3 windows', '- Wails v3 docs | In Research', '+ Pull request #42 · bench | In Research']);
  assert.equal(await page.textContent(more), 'Windows in other tabs (10) · 1 ticked');
  assert.equal(await pressed(page, '.preset'), 'preset-side');
  // An app's row ticks all its windows, and Zed makes three apps.
  await page.click('.win-others .win-app[data-app-name="Brave"] .win-head input');
  await page.click('.win-others .win-app[data-app-name="Zed"] input');
  assert.equal(await pressed(page, '.preset'), 'preset-three');
  await page.click('[data-key="editor-save"]');
  assert.deepEqual((await fake.waitForCall('NewTabFromWindows')).args[1], ['f101', 'f102', 'w7', 'w8', 'w9', 'w3']);
});

test('Tab ▸ New Tab from Open Windows… and Jump to… open the dialog with the windows ticked', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await fake.emit('bench:command', { name: 'new-tab-from-windows' });
  await page.waitForSelector('.win-list');
  assert.equal(await pressed(page, '[data-action="draft-start"]'), 'start-windows');
  assert.equal(await focusedKey(page), 'tab-name');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.editor-sheet', { state: 'detached' });

  await page.click('[data-action="open-palette"]');
  await page.keyboard.type('new tab from');
  assert.equal(await page.textContent('.palette-item[aria-selected="true"] .palette-label'), 'New tab from open windows…');
  await page.keyboard.press('Enter');
  await page.waitForSelector('.win-list');
  assert.deepEqual(await listed(page), [['+ Brave | Release notes'], ['+ Finder | Downloads']]);
  // Return in the name makes the tab.
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('NewTabFromWindows')).args[1], ['f101', 'f102']);
});

test('with no windows outside the tabs, the dialog says so, and offers those of the tabs', async (t) => {
  const world = defaultWorld();
  world.floating = [];
  const { page, close } = await openBench({ world });
  t.after(close);
  await openFromWindows(page);
  assert.match(await page.textContent('.win-empty'), /^There are no windows outside your tabs\./);
  assert.equal(await page.isDisabled('[data-key="editor-save"]'), true);
  assert.equal(await page.inputValue('#tab-name'), 'New tab');
  await page.click('[data-key="win-others"]');
  await page.click('[data-key="win-w10"]');
  assert.equal(await page.inputValue('#tab-name'), 'Obsidian');
  assert.equal(await page.isDisabled('[data-key="editor-save"]'), false);
});

test('the welcome guide offers a tab of the open windows first, and comes back once it\'s made', async (t) => {
  const world = defaultWorld({ firstRun: true, current: 2 });
  world.tabs = [world.tabs[2]];
  world.current = 0;
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await page.waitForSelector('.guide-sheet[data-page="permissions"]');
  await page.click('[data-key="guide-continue"]');
  await page.waitForSelector('.windows-starter');
  assert.equal(await page.$eval('.guide-sheet .dialog-body > :first-child .starter', (el) => el.classList.contains('windows-starter')), true);
  assert.deepEqual(await page.$$eval('.windows-starter .starter-app', (els) => els.map((el) => el.textContent)), ['BrBrave', 'FnFinder']);
  assert.equal(await page.textContent('.guide-sub'), 'Suggested tabs');

  // Cancel goes back to the guide.
  await page.click('[data-key="guide-windows"]');
  await page.waitForSelector('.editor-sheet');
  assert.equal(await pressed(page, '[data-action="draft-start"]'), 'start-windows');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.guide-sheet[data-page="setup"]');
  assert.equal(await focusedKey(page), 'guide-windows');

  // So does Make tab, and the set-up then says what it made.
  const asked = fake.callsTo('StarterTabs').length;
  await page.click('[data-key="guide-windows"]');
  await page.waitForSelector('.win-list');
  await page.click('[data-key="editor-save"]');
  await fake.waitForCall('NewTabFromWindows');
  await page.waitForSelector('.guide-sheet[data-page="setup"]');
  await page.waitForFunction(() => document.querySelector('.windows-starter').textContent === 'Made the tab Brave.');
  assert.ok(fake.callsTo('StarterTabs').length > asked);
  assert.equal(await page.textContent('[data-key="guide-one-tab"]'), 'Don\'t add these');
  assert.deepEqual(await page.evaluate(() => window.bench.store.view.tabs.map((x) => x.name)), ['Research', 'Brave']);
});

test('the welcome guide forgets a tab of open windows that Undo takes away', async (t) => {
  const world = defaultWorld({ firstRun: true, current: 2 });
  world.tabs = [world.tabs[2]];
  world.current = 0;
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await page.waitForSelector('.guide-sheet[data-page="permissions"]');
  await page.click('[data-key="guide-continue"]');
  await page.waitForSelector('.windows-starter');
  const intro = await page.textContent('#guide-intro');
  assert.equal(await page.textContent('[data-key="guide-one-tab"]'), 'Start with one tab');

  await page.click('[data-key="guide-windows"]');
  await page.waitForSelector('.win-list');
  await page.click('[data-key="editor-save"]');
  await fake.waitForCall('NewTabFromWindows');
  await page.waitForFunction(() => document.querySelector('.windows-starter').textContent === 'Made the tab Brave.');
  assert.equal(await page.textContent('[data-key="guide-one-tab"]'), 'Don\'t add these');
  assert.notEqual(await page.textContent('#guide-intro'), intro);

  // The toast's Undo, over the guide, deletes the tab: the set-up no longer
  // names it, offers the windows again, and asks Go for its suggestions
  // again.
  await page.waitForSelector('#toast.show');
  fake.clearCalls();
  await page.click('#toast .toast-action');
  await fake.waitForCall('Undo');
  await page.waitForFunction(() => window.bench.store.view.tabs.length === 1);
  await page.waitForFunction(() => !/Made/.test(document.querySelector('.guide-sheet .dialog-body').textContent));
  assert.equal(await page.textContent('#windows-starter-name'), 'A tab of your open windows');
  assert.deepEqual(await page.$$eval('.windows-starter .starter-app', (els) => els.map((el) => el.textContent)), ['BrBrave', 'FnFinder']);
  assert.equal(await page.textContent('[data-key="guide-one-tab"]'), 'Start with one tab');
  assert.equal(await page.textContent('#guide-intro'), intro);
  await fake.waitForCall('StarterTabs');
});

test('the add dialog\'s windows can be ticked, to add several at once', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('.pane[data-pane-id="p4"] [data-action="add"]');
  await page.waitForSelector('.palette-item');
  // Only the windows' rows have tick boxes.
  assert.equal(await page.locator('.row-tick').count(), 8);
  assert.equal(await page.locator('[data-key="add-ticked"]').count(), 0);
  await page.click('.palette-item:has(.palette-label:text-is("Downloads")) .row-tick');
  await page.click('.palette-item:has(.palette-label:text-is("Daily note")) .row-tick');
  assert.equal(await page.getAttribute('.palette-item:has(.palette-label:text-is("Downloads"))', 'aria-checked'), 'true');
  assert.equal(await page.getAttribute('.palette-item:has(.palette-label:text-is("Release notes"))', 'aria-checked'), 'false');
  assert.equal(await page.textContent('[data-key="add-ticked"]'), 'Add 2 windows');
  assert.equal(await focusedKey(page), 'palette-query');
  assert.equal(fake.callsTo('AddToPane').length, 0);

  // Shift-Return ticks the row chosen, and Return adds the windows
  // ticked, in the dialog's order.
  await page.keyboard.type('release');
  await page.keyboard.press('Shift+Enter');
  assert.equal(await page.textContent('[data-key="add-ticked"]'), 'Add 3 windows');
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('AddWindowsToPane')).args, ['p4', ['f101', 'f102', 'w10']]);
  await page.waitForSelector('.palette', { state: 'detached' });
  assert.equal(fake.callsTo('AddToPane').length, 0);
  await page.waitForFunction(() => window.bench.store.view.tabs[1].root.children[1].items.length === 5);

  // A click on a row, rather than its tick box, still adds just that row.
  fake.clearCalls();
  await page.click('.pane[data-pane-id="p4"] [data-action="add"]');
  await page.click('.palette-item:has(.palette-label:text-is("Inbox"))');
  assert.deepEqual((await fake.waitForCall('AddToPane')).args, ['p4', { type: 'item', id: 'w2' }]);
});

// row returns the selector of the add dialog's row with a label.
const row = (label) => '.palette-item:has(.palette-label:text-is("' + label + '"))';

test('while windows are ticked in the add dialog, a click on a window\'s row ticks it, rather than adding it alone', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('.pane[data-pane-id="p4"] [data-action="add"]');
  await page.waitForSelector('.palette-item');
  // Every row's tile is in the same column, with a tick box or without, and
  // the tick box takes clicks over more than 20 by 20 points.
  const lefts = await page.$$eval('.palette-item', (els) => els.map((el) => Math.round(el.querySelector('.palette-label').previousElementSibling.getBoundingClientRect().left)));
  assert.equal(new Set(lefts).size, 1, 'the tiles start at ' + lefts.join(', '));
  const box = await page.locator('.row-tick').first().boundingBox();
  assert.ok(box.width >= 20 && box.height >= 20, 'the tick box takes clicks over ' + box.width + ' by ' + box.height);

  await page.click(row('Downloads') + ' .row-tick');
  await page.click(row('Daily note') + ' .row-tick');
  await page.click(row('Inbox') + ' .palette-label');
  assert.equal(await page.getAttribute(row('Inbox'), 'aria-checked'), 'true');
  assert.equal(await page.textContent('[data-key="add-ticked"]'), 'Add 3 windows');
  assert.equal(await page.locator('.palette').count(), 1);
  // A click on a ticked row unticks it.
  await page.click(row('Daily note') + ' .palette-label');
  assert.equal(await page.getAttribute(row('Daily note'), 'aria-checked'), 'false');
  assert.equal(await page.textContent('[data-key="add-ticked"]'), 'Add 2 windows');
  assert.equal(fake.callsTo('AddToPane').length, 0);
  await page.click('[data-key="add-ticked"]');
  assert.deepEqual((await fake.waitForCall('AddWindowsToPane')).args, ['p4', ['f102', 'w2']]);
  await page.waitForSelector('.palette', { state: 'detached' });

  // On a row with no tick box, Shift-Return does what Return does.
  await page.click('.pane[data-pane-id="p4"] [data-action="add"]');
  await page.waitForSelector('.palette-item');
  await page.keyboard.type('web page');
  assert.equal(await page.textContent('.palette-item[aria-selected="true"] .palette-label'), 'Web page…');
  await page.keyboard.press('Shift+Enter');
  await page.waitForSelector('#palette-url');
});
