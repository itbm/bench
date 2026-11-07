'use strict';

// The jump palette, the add dialog, Tab properties, and Settings.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, openSettings, openTabProperties, settle } = require('./helpers/harness.js');
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
    // The focused pane's window's ⋯ menu, as a group of its own.
    '# This window',
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
    '# Actions',
    'Rearrange this tab | ',
    'Zoom this pane | Zed — bench — switch.go',
    'Add a window or app to this pane… | A window, app, web page, or terminal',
    'New terminal… | In this pane: a command, a name, and a folder',
    'Add a web page… | In this pane: a website, as a web app or in Bench',
    'Close tab… | Code',
    'Delete tab (keep windows) | Code',
    'Fill the screen | Zoom Bench\'s window',
    'Tab properties of Code… | Name, layout, small screens',
    'Tab properties of Comms… | Name, layout, small screens',
    'Tab properties of Research… | Name, layout, small screens',
    'Move tab left | Code',
    'Move tab right | Code',
    'New tab… | ',
    'New tab from open windows… | Tick the windows for a new tab',
    'Settings… | For every tab',
    'Reload config.toml | Use changes made to the file',
    'Recent messages | Read or copy what Bench said',
    'Keyboard shortcuts | Help: what each shortcut does',
    'Show the welcome guide | Help: permissions and tips',
    'Use the compact layout | ',
    'Use the wide layout | ',
  ]);
  const keys = await page.$$eval('.palette-item .palette-keys', (els) => els.map((el) => el.textContent));
  assert.equal(keys[0], '⌃⌥1  ↵');
  assert.equal(keys[2], '⌃⌥3');
});

test('the browser preview\'s jump palette has no Fill the screen, as it has no window to fill it with', async (t) => {
  const { page, close } = await openBench({ world: defaultWorld({ demo: true }) });
  t.after(close);
  await page.click('[data-action="open-palette"]');
  await page.keyboard.type('Rearrange');
  assert.deepEqual(await rows(page), ['# Actions', 'Rearrange this tab | ']);
  await page.fill('#palette-query', 'fill');
  assert.deepEqual(await page.$$eval('#overlay-root .palette-label', (els) => els.map((el) => el.textContent)), []);
});

test('the jump palette leaves out the sub-tabs of windows that have closed', async (t) => {
  const world = defaultWorld();
  world.items.w10.hidden = true;
  const { page, close } = await openBench({ world });
  t.after(close);
  await page.click('[data-action="open-palette"]');
  await page.keyboard.type('Research');
  assert.deepEqual(await rows(page), [
    '# Tabs',
    'Research | Brave',
    '# This window',
    'Move to tab ▸ Research | Zed — bench — switch.go',
    'Also show in Research | Zed — bench — switch.go',
    '# Actions',
    'Tab properties of Research… | Name, layout, small screens',
    '# Windows',
    'Wails v3 docs | Research · Brave',
    'Pull request #42 · bench | Research · Brave',
    'Go packages | Research · Brave',
  ]);
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

test('palette actions: floating windows, modes, rearrange, Tab properties, and Settings', async (t) => {
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
  await choose('reload config');
  assert.deepEqual((await fake.waitForCall('ReloadConfig')).args, []);
  await choose('Rearrange');
  await fake.waitForCall('Rearrange');
  await choose('Fill the screen');
  assert.deepEqual((await fake.waitForCall('FillScreen')).args, []);
  await choose('Tab properties');
  await page.waitForSelector('.editor-sheet');
  assert.equal(await page.textContent('#editor-title'), 'Tab properties');
  assert.equal(await page.inputValue('#tab-name'), 'Code');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.editor-sheet', { state: 'detached' });

  // The palette finds the actions by their hints too.
  await page.click('[data-action="open-palette"]');
  await page.keyboard.type('every tab');
  assert.deepEqual(await rows(page), ['# Actions', 'Settings… | For every tab']);
  await page.keyboard.press('Escape');
  await choose('Settings');
  await page.waitForSelector('.settings-sheet');
  assert.equal(await page.textContent('#settings-title'), 'Settings');
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'setting-openAtLogin');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
  // Focus goes back to where it was before the palette opened.
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'open-palette');
  assert.equal(fake.callsTo('SaveSettings').length, 0);
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
    'New terminal in ~/code/bench | Where the terminal here is',
    'New terminal | ~',
    'New terminal in… | Choose a folder',
    'New terminal… | With a command, a name, or a folder',
    'Web page… | As a web app, or in Bench',
    'Browse… | An app, or a program to run in a terminal',
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
  await page.click('.palette-item:has(.palette-label:text-is("New terminal"))');
  assert.deepEqual((await fake.waitForCall('AddToPane')).args, ['p4', { type: 'terminal', folder: '' }]);

  // The first Built in row starts one where the terminal p4 shows is.
  fake.clearCalls();
  await page.click('.pane[data-pane-id="p4"] [data-action="add"]');
  await page.click('.palette-item:has-text("New terminal in ~/code/bench")');
  assert.deepEqual((await fake.waitForCall('AddToPane')).args, ['p4', { type: 'terminal', folder: '~/code/bench' }]);
});

// openURLStep opens the add dialog for p3 and chooses Web page….
async function openURLStep(page) {
  await page.click('.pane[data-pane-id="p3"] [data-action="add"]');
  await page.waitForSelector('.palette');
  await page.fill('#palette-query', 'web page');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#palette-url');
}

const toastIs = (page, text) => page.waitForFunction((t) => document.getElementById('toast').textContent === t, text);
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

test('Web page… asks for an address, and Return adds the site\'s web app', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await openURLStep(page);
  assert.equal(await page.evaluate(() => document.activeElement.id), 'palette-url');
  assert.equal(await page.textContent('#palette-url-hint'),
    'Return adds your web app for this site, or opens the site in Safari so that you can make one. '
    + 'Option-Return shows the page in Bench instead. Bench adds https:// if you leave it out.');
  assert.equal(fake.callsTo('AddWebsite').length, 0);
  // Escape goes back to the list, and choosing it again asks again.
  await page.keyboard.press('Escape');
  await page.waitForSelector('#palette-query');
  await page.click('.palette-item:has-text("Web page…")');
  await page.waitForSelector('#palette-url');

  // An empty address isn't sent.
  await page.keyboard.press('Enter');
  await toastIs(page, 'Type the address of a web page.');
  assert.equal(fake.callsTo('AddWebsite').length, 0);

  // A refused address keeps the dialog open.
  fake.on('AddWebsite', () => { throw new Error('That address isn\'t a secure web address (https).'); });
  await page.keyboard.type('http://squoosh.app');
  await page.keyboard.press('Enter');
  await toastIs(page, 'That address isn\'t a secure web address (https).');
  assert.equal(await page.isVisible('#palette-url'), true);

  fake.on('AddWebsite', () => ({ added: 'web-app', app: 'Squoosh', browser: 'Chrome', note: 'Added the Squoosh web app from Chrome.' }));
  await page.fill('#palette-url', 'squoosh.app');
  await page.keyboard.press('Enter');
  await page.waitForSelector('.palette', { state: 'detached' });
  await toastIs(page, 'Added the Squoosh web app from Chrome.');
  assert.deepEqual(fake.callsTo('AddWebsite').pop().args, ['p3', 'squoosh.app']);
  assert.equal(fake.callsTo('AddToPane').length, 0);
  // The Add button does what Return does.
  await openURLStep(page);
  await page.fill('#palette-url', 'squoosh.app/editor');
  await page.click('[data-action="add-website"]');
  await page.waitForSelector('.palette', { state: 'detached' });
  assert.deepEqual(fake.callsTo('AddWebsite').pop().args, ['p3', 'squoosh.app/editor']);
});

test('Option-Return and Show in Bench add a web page that Bench shows itself', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await openURLStep(page);
  await page.keyboard.type('example.com/docs');
  await page.keyboard.press('Alt+Enter');
  await page.waitForSelector('.palette', { state: 'detached' });
  assert.deepEqual(fake.callsTo('AddToPane').pop().args, ['p3', { type: 'web', url: 'example.com/docs' }]);

  await openURLStep(page);
  await page.keyboard.type('example.com/blog');
  await page.click('[data-action="add-in-bench"]');
  await page.waitForSelector('.palette', { state: 'detached' });
  assert.deepEqual(fake.callsTo('AddToPane').pop().args, ['p3', { type: 'web', url: 'example.com/blog' }]);
  assert.equal(fake.callsTo('AddWebsite').length, 0);

  // A refused address keeps the dialog open.
  fake.on('AddToPane', () => { throw new Error('That address isn\'t a secure web address (https).'); });
  await openURLStep(page);
  await page.keyboard.type('http://example.com');
  await page.keyboard.press('Alt+Enter');
  await toastIs(page, 'That address isn\'t a secure web address (https).');
  assert.equal(await page.isVisible('#palette-url'), true);
});

test('when Bench shows the page itself, the dialog says why', async (t) => {
  const { page, fake, close } = await openBench({
    setup: (f) => f.on('AddOptions', (args, g) => g.mAddOptions().map((o) => (o.asksForUrl
      ? { ...o, hint: 'Opens in a Bench web view', webAppBrowser: '' } : o))),
  });
  t.after(close);
  await openURLStep(page);
  assert.equal(await page.textContent('#palette-url-hint'),
    'Return adds your web app for this site if you have one, or else shows the page in Bench, as Option-Return always does. '
    + 'Bench adds https:// if you leave it out.');
  fake.on('AddWebsite', () => ({ added: 'web', note: 'Firefox can\'t make web apps, so Bench shows the page itself.' }));
  await page.keyboard.type('squoosh.app');
  await page.keyboard.press('Enter');
  await page.waitForSelector('.palette', { state: 'detached' });
  await toastIs(page, 'Firefox can\'t make web apps, so Bench shows the page itself.');
  assert.deepEqual(fake.callsTo('AddWebsite').pop().args, ['p3', 'squoosh.app']);
});

test('while a browser makes the web app, the dialog waits for it, and adds it when it\'s ready', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  let polls = 0;
  let busy = false;
  let overlapped = false;
  fake.on('AddWebAppWhenReady', async () => {
    overlapped = overlapped || busy;
    busy = true;
    polls += 1;
    await sleep(300);
    busy = false;
    return polls >= 3;
  });
  await openURLStep(page);
  await page.keyboard.type('squoosh.app');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#palette-make-title');
  assert.equal(await page.textContent('#palette-make-title'), 'Make a web app in Safari');
  assert.equal(await page.textContent('#palette-make-steps'),
    'Bench has opened squoosh.app in Safari. Choose File ▸ Add to Dock, then click Add.');
  assert.match(await page.textContent('.palette'), /Bench adds the web app to this pane as soon as it's ready\./);
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'make-cancel');
  assert.equal(fake.callsTo('AddToPane').length, 0);

  // The user goes to the browser: the dialog stays open, and keeps asking.
  await page.evaluate(() => window.dispatchEvent(new Event('blur')));
  await fake.waitForCall('AddWebAppWhenReady', () => true, 4000);
  assert.ok(await page.$('#palette-make-title'), 'the dialog is still open');
  await page.waitForSelector('.palette', { state: 'detached', timeout: 10000 });
  await toastIs(page, 'Added the web app.');
  const asks = fake.callsTo('AddWebAppWhenReady');
  assert.equal(asks.length, 3);
  assert.deepEqual(asks[0].args, ['p3', 'squoosh.app']);
  assert.equal(overlapped, false, 'one call at a time');
  for (let i = 1; i < asks.length; i++) {
    assert.ok(asks[i].at - asks[i - 1].at >= 1400, 'about 1.5 s apart: ' + (asks[i].at - asks[i - 1].at) + ' ms');
  }
  await fake.waitForLowered();
  // It stops asking once it's added.
  await sleep(2000);
  assert.equal(fake.callsTo('AddWebAppWhenReady').length, 3);
});

test('Escape, Cancel, and Show in Bench instead stop waiting for the web app', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  const toMake = async () => {
    await openURLStep(page);
    await page.keyboard.type('squoosh.app');
    await page.keyboard.press('Enter');
    await page.waitForSelector('#palette-make-title');
  };
  // stopped checks that no call to AddWebAppWhenReady starts more than a
  // moment after the step was left.
  const stopped = async () => {
    const n = fake.callsTo('AddWebAppWhenReady').length;
    await sleep(1800);
    assert.ok(fake.callsTo('AddWebAppWhenReady').length <= n, 'it stopped asking');
  };

  await toMake();
  await fake.waitForCall('AddWebAppWhenReady');
  await page.keyboard.press('Escape');
  await page.waitForSelector('#palette-query');
  await stopped();

  fake.clearCalls();
  await page.click('.palette-item:has-text("Web page…")');
  await page.waitForSelector('#palette-url');
  await page.keyboard.type('squoosh.app');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#palette-make-title');
  await page.click('[data-key="make-cancel"]');
  await page.waitForSelector('#palette-query');
  await stopped();
  await page.keyboard.press('Escape');
  await page.waitForSelector('.palette', { state: 'detached' });

  fake.clearCalls();
  await toMake();
  await page.click('[data-action="add-in-bench"]');
  await page.waitForSelector('.palette', { state: 'detached' });
  assert.deepEqual(fake.callsTo('AddToPane').pop().args, ['p3', { type: 'web', url: 'squoosh.app' }]);
  await stopped();
});

test('a refusal while waiting for the web app shows once', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  fake.on('AddWebAppWhenReady', () => { throw new Error('Bench couldn\'t open Squoosh.'); });
  await openURLStep(page);
  await page.keyboard.type('squoosh.app');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#palette-make-title');
  await page.evaluate(() => {
    window.toastsShown = 0;
    new MutationObserver(() => { window.toastsShown += 1; }).observe(document.getElementById('toast'), { childList: true });
  });
  await fake.waitForCall('AddWebAppWhenReady');
  await toastIs(page, 'Bench couldn\'t open Squoosh.');
  while (fake.callsTo('AddWebAppWhenReady').length < 3) await sleep(100);
  await settle(page);
  assert.equal(await page.evaluate(() => window.toastsShown), 1);
  assert.ok(await page.$('#palette-make-title'), 'the dialog is still open');
});

test('stopping the wait as Go asks for the web app waits for its answer', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  // Each call takes a while, as Go's scan of the installed apps does.
  let found = true;
  fake.on('AddWebAppWhenReady', async () => {
    await sleep(800);
    return found;
  });
  const toMake = async () => {
    fake.clearCalls();
    await openURLStep(page);
    await page.keyboard.type('squoosh.app');
    await page.keyboard.press('Enter');
    await page.waitForSelector('#palette-make-title');
    await fake.waitForCall('AddWebAppWhenReady');
  };

  // The call under way adds the web app: that's all that's added, and the
  // dialog closes, whether the user chose Show in Bench instead or Cancel.
  await toMake();
  await page.click('[data-action="add-in-bench"]');
  await page.waitForSelector('.palette', { state: 'detached' });
  await toastIs(page, 'Added the web app.');
  assert.equal(fake.callsTo('AddToPane').length, 0);
  await toMake();
  await page.click('[data-key="make-cancel"]');
  await page.waitForSelector('.palette', { state: 'detached' });
  assert.equal(fake.callsTo('AddToPane').length, 0);

  // It finds none: the choice goes ahead once it has answered, and the
  // dialog asks no more.
  found = false;
  await toMake();
  await page.click('[data-action="add-in-bench"]');
  await page.waitForSelector('.palette', { state: 'detached' });
  assert.deepEqual(fake.callsTo('AddToPane').map((c) => c.args), [['p3', { type: 'web', url: 'squoosh.app' }]]);
  await toMake();
  await page.click('[data-key="make-cancel"]');
  await page.waitForSelector('#palette-query');
  await sleep(1800);
  assert.equal(fake.callsTo('AddWebAppWhenReady').length, 1);
});

test('Tab properties previews presets and saves the tab\'s name, layout, and small-screen setting', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await openTabProperties(page);
  await fake.waitForCall('SetOverlay', (a) => a[0] === true);
  assert.deepEqual((await fake.waitForCall('PreviewTree')).args, [1, 'main-side']);
  assert.equal(await page.textContent('#editor-title'), 'Tab properties');
  assert.equal(await page.evaluate(() => document.activeElement.id), 'tab-name');
  assert.equal(await page.inputValue('#tab-name'), 'Code');
  assert.equal(await page.$eval('#tab-place', (el) => el.selectedOptions[0].textContent), '2nd (⌃⌥2)');
  assert.equal(await page.getAttribute('[data-preset="main-side"]', 'aria-pressed'), 'true');
  assert.equal(await page.getAttribute('[data-small="stack"]', 'aria-pressed'), 'true');
  const chips = await page.$$eval('.mini-pane', (els) => els.map((el) => Array.from(el.querySelectorAll('.chips span'), (s) => s.textContent)));
  assert.deepEqual(chips, [['Zed: bench — switch.go', 'VS Code: index.html — bench-ui'], ['Terminal: zsh — ~/code/bench', 'Terminal: go test — ~/code/bench']]);
  // The threshold for small screens applies to every tab, so it's in
  // Settings, and so are the other settings.
  assert.equal(await page.textContent('#small-screens-text'), 'When Bench is narrower than 1,800 points, as set in Settings.');
  assert.equal(await page.$('#compact-below'), null);
  assert.equal(await page.$('[data-setting]'), null);

  // Choosing a preset asks Go for its preview.
  await page.click('[data-preset="three"]');
  assert.deepEqual((await fake.waitForCall('PreviewTree', (a) => a[1] === 'three')).args, [1, 'three']);
  await page.waitForFunction(() => document.querySelectorAll('.mini-pane').length === 3);
  const three = await page.$$eval('.mini-pane', (els) => els.map((el) => el.textContent));
  assert.equal(three[2], 'Empty pane');

  // The shortcut follows tab order: each place names its own.
  assert.deepEqual(await page.$$eval('#tab-place option', (els) => els.map((el) => el.textContent)), ['1st (⌃⌥1)', '2nd (⌃⌥2)', '3rd (⌃⌥3)']);

  await page.fill('#tab-name', '  Coding  ');
  await page.click('[data-small="tabs"]');
  await page.click('[data-action="save-tab"]');
  const save = await fake.waitForCall('SaveTab');
  assert.deepEqual(save.args, [1, { name: 'Coding', preset: 'three', smallScreens: 'tabs' }]);
  await page.waitForSelector('.editor-sheet', { state: 'detached' });
  await fake.waitForLowered();
  // Focus goes back to the tab that was right-clicked.
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'tab-1');
  assert.equal(fake.callsTo('SaveSettings').length, 0);
  assert.equal(fake.callsTo('SetOpenAtLogin').length, 0);
});

test('a tab\'s menu opens Tab properties for that tab, whichever is current', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('.tab[data-index="0"]', { button: 'right' });
  await page.waitForSelector('#tab-menu');
  assert.equal(await page.getAttribute('#tab-menu', 'aria-label'), 'Options for the Comms tab');
  assert.deepEqual(await page.$$eval('#tab-menu .menu-item', (els) => els.map((el) => el.textContent)),
    ['Tab properties…', 'Rename…', 'Move left', 'Move right', 'Rearrange', 'Reset layout', 'Close tab…', 'Delete tab (keep windows)', 'New tab…']);
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'menu-item-0');
  await page.keyboard.press('Enter');
  await page.waitForSelector('.editor-sheet');
  assert.deepEqual((await fake.waitForCall('PreviewTree')).args, [0, 'side']);
  assert.equal(await page.textContent('#editor-title'), 'Tab properties');
  assert.equal(await page.inputValue('#tab-name'), 'Comms');
  assert.equal(await page.$eval('#tab-place', (el) => el.selectedOptions[0].textContent), '1st (⌃⌥1)');
  assert.equal(await page.getAttribute('[data-small="tabs"]', 'aria-pressed'), 'true');
  await page.click('[data-small="same"]');
  await page.click('[data-action="save-tab"]');
  assert.deepEqual((await fake.waitForCall('SaveTab')).args, [0, { name: 'Comms', preset: 'side', smallScreens: 'same' }]);
  await page.waitForSelector('.editor-sheet', { state: 'detached' });
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'tab-0');

  // New tab… in a tab's menu adds a tab, as + does.
  await page.click('.tab[data-index="2"]', { button: 'right' });
  await page.click('#tab-menu .menu-item:has-text("New tab…")');
  await page.waitForSelector('.editor-sheet');
  assert.equal(await page.textContent('#editor-title'), 'New tab');
  assert.deepEqual((await fake.waitForCall('PreviewTree', (a) => a[0] === -1)).args, [-1, 'single']);
  await page.click('[data-key="editor-cancel"]');
  await page.waitForSelector('.editor-sheet', { state: 'detached' });
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'tab-2');
  // The menu is about the tab it's for: it doesn't switch to it.
  assert.equal(fake.callsTo('SwitchTab').length, 0);
  assert.equal(fake.callsTo('SaveTab').length, 1);
});

test('Settings shows the settings for every tab, and saves them with Open at login', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await openSettings(page);
  await fake.waitForCall('SetOverlay', (a) => a[0] === true);
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'setting-openAtLogin');
  assert.equal(await page.textContent('#settings-title'), 'Settings');
  assert.equal(await page.textContent('#settings-intro'),
    'These settings apply to every tab. To change a tab\'s name, layout, or small-screen setting, right-click the tab and choose Tab properties.');
  const groups = await page.$$eval('.settings-group', (els) => els.map((el) => [
    el.querySelector('h3').textContent, ...Array.from(el.querySelectorAll('.toggle span'), (s) => s.textContent)]));
  assert.deepEqual(groups, [
    ['General', 'Open Bench at login', 'Open Bench filling the screen', 'Bring back a hidden Bench with tab shortcuts',
      'Check for a new version once a day', 'Ask before quitting while a terminal is running something', 'Ask before closing a tab\'s windows'],
    ['Appearance', 'Show status bar', 'Hide the sub-tab row when a pane holds one app window'],
    ['Windows', 'Open apps that aren\'t running', 'Hide windows that belong to other tabs',
      'Also hide apps that aren\'t in any tab', 'Show live previews of windows (needs Screen Recording)', 'Drag windows onto Bench to add them'],
    ['Keyboard'],
    ['Small screens'],
    ['Terminals', 'Use Option as Meta key', 'Notify me when a terminal in another tab needs attention', 'Keep terminal output between launches'],
    ['Launchers'],
    ['Advanced'],
  ]);
  // The boxes, the threshold, and the terminals' text size show the
  // settings.
  const boxes = await page.$$eval('[data-setting]', (els) => els.map((el) => [el.dataset.setting, el.checked]));
  assert.deepEqual(boxes, [['openAtLogin', false], ['fillScreen', true], ['showOnTabShortcut', true], ['checkForUpdates', false],
    ['confirmQuitBusyTerminals', true], ['confirmClose', true], ['showStatusBar', true], ['paneHeaders', false], ['openMissingApps', true],
    ['hideOtherTabs', true], ['hideOtherApps', false], ['livePreviews', false], ['dragToAdd', true], ['terminalOptionAsMeta', false],
    ['notifyTerminals', true], ['keepTerminalOutput', true]]);
  assert.equal(await page.inputValue('#compact-below'), '1800');
  assert.equal(await page.inputValue('#terminal-font-size'), '12.5');
  assert.equal(await page.inputValue('#launch-wait'), '15');
  // A tab's own properties aren't here.
  assert.equal(await page.$('#tab-name'), null);
  assert.equal(await page.$('[data-preset]'), null);

  await page.fill('#compact-below', '1600');
  await page.click('[data-setting="hideOtherTabs"]');
  await page.click('[data-key="follow-focus-off"]');
  // Bench doesn't have Screen Recording permission, which previews need, so
  // turning them on asks for it at once, and the dialog says so.
  assert.equal(await page.$('#recording-note'), null);
  await page.click('[data-setting="livePreviews"]');
  await fake.waitForCall('RequestScreenRecording');
  await page.waitForSelector('#recording-note');
  assert.equal(await page.getAttribute('[data-setting="livePreviews"]', 'aria-describedby'), 'recording-note');
  // Dragging windows onto panes is on by default, and says how it works.
  assert.equal(await page.getAttribute('[data-setting="dragToAdd"]', 'aria-describedby'), 'drag-to-add-help');
  assert.match(await page.textContent('#drag-to-add-help'), /Drop it in the middle to add it to the pane, or at an edge/);
  await page.click('[data-setting="dragToAdd"]');
  await page.click('[data-setting="openAtLogin"]');
  await page.click('[data-setting="fillScreen"]');
  await page.click('[data-setting="showOnTabShortcut"]');
  // Bench looks for a new version only once this is on.
  await page.click('[data-setting="checkForUpdates"]');
  await page.click('[data-setting="confirmQuitBusyTerminals"]');
  await page.click('[data-setting="confirmClose"]');
  await page.fill('#terminal-font-size', '15');
  await page.click('[data-setting="terminalOptionAsMeta"]');
  await page.click('[data-setting="notifyTerminals"]');
  await page.click('[data-setting="keepTerminalOutput"]');
  await page.click('[data-action="save-settings"]');
  const save = await fake.waitForCall('SaveSettings');
  assert.deepEqual(save.args, [{
    fillScreen: false,
    menuBar: 'name',
    showOnTabShortcut: false,
    checkForUpdates: true,
    confirmQuitBusyTerminals: false,
    confirmClose: false,
    compactBelow: 1600,
    gap: 4,
    showStatusBar: true,
    tabShortcutHints: 'auto',
    scrollBars: 'auto',
    appearance: 'system',
    density: 'auto',
    paneHeaders: 'always',
    openMissingApps: true,
    hideOtherTabs: false,
    hideOtherApps: false,
    newWindows: 'app-pane',
    followFocus: 'off',
    livePreviews: true,
    dragToAdd: false,
    terminalFontSize: 15,
    terminalOptionAsMeta: true,
    notifyTerminals: false,
    keepTerminalOutput: false,
    launchTimeout: '15s',
  }]);
  assert.deepEqual((await fake.waitForCall('SetOpenAtLogin')).args, [true]);
  // Open at login waits until Go has accepted the settings.
  assert.deepEqual(fake.calls.filter((c) => ['SaveSettings', 'SetOpenAtLogin'].includes(c.method)).map((c) => c.method), ['SaveSettings', 'SetOpenAtLogin']);
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
  await fake.waitForLowered();
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'open-settings');
  assert.equal(fake.callsTo('SaveTab').length, 0);
});

test('Settings chooses what the menu bar item shows, and says how to bring Bench back without it', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await openSettings(page);
  const choices = () => page.$$eval('[data-choice="menuBar"]', (els) => els.map((el) => [el.textContent, el.getAttribute('aria-pressed')]));
  assert.equal(await page.textContent('#menu-bar-label'), 'Menu bar item');
  assert.deepEqual(await choices(), [['Icon and tab name', 'true'], ['Icon only', 'false'], ['Hidden', 'false']]);
  assert.equal(await page.getAttribute('.settings-choice .seg', 'aria-labelledby'), 'menu-bar-label');
  assert.equal(await page.getAttribute('.settings-choice .seg', 'aria-describedby'), 'menu-bar-help');
  assert.equal(await page.textContent('#menu-bar-help'),
    'Bench\'s icon, with up to 16 characters of the current tab\'s name. Click it for your tabs, Jump to…, and Settings.');

  await page.click('[data-key="menu-bar-icon"]');
  assert.deepEqual(await choices(), [['Icon and tab name', 'false'], ['Icon only', 'true'], ['Hidden', 'false']]);
  assert.equal(await page.textContent('#menu-bar-help'),
    'Bench\'s icon alone, to save room in the menu bar. Click it for your tabs, Jump to…, and Settings.');
  // The button keeps the keyboard as the dialog is redrawn.
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'menu-bar-icon');

  // Hidden, the menu bar item can't bring Bench back, so Settings says what
  // does.
  await page.click('[data-key="menu-bar-hidden"]');
  assert.equal(await page.textContent('#menu-bar-help'),
    'No menu bar item. While Bench is hidden, press ⌃⌥Space or click Bench\'s icon in the Dock to bring it back.');
  await page.click('[data-action="save-settings"]');
  const save = await fake.waitForCall('SaveSettings');
  assert.equal(save.args[0].menuBar, 'hidden');
  assert.equal(save.args[0].showOnTabShortcut, true);
  await page.waitForSelector('.settings-sheet', { state: 'detached' });

  // Opened again, Settings shows the saved choice.
  await openSettings(page);
  assert.deepEqual(await choices(), [['Icon and tab name', 'false'], ['Icon only', 'false'], ['Hidden', 'true']]);
});

test('with the jump shortcut off, Settings names only the Dock to bring Bench back', async (t) => {
  const world = defaultWorld();
  world.settings.menuBar = 'hidden';
  world.keys.jump = '';
  const { page, close } = await openBench({ world });
  t.after(close);
  await openSettings(page);
  assert.equal(await page.textContent('#menu-bar-help'),
    'No menu bar item. While Bench is hidden, click Bench\'s icon in the Dock to bring it back.');
});

test('typing the threshold for small screens keeps the digits in order', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await openSettings(page);
  // marker tells a field that's been redrawn from the one that was there.
  const mark = () => page.$eval('#compact-below', (el) => { el.dataset.marker = 'kept'; });
  const marker = () => page.$eval('#compact-below', (el) => el.dataset.marker || '');
  assert.equal(await page.inputValue('#compact-below'), '1800');
  await mark();
  await page.fill('#compact-below', '');
  await page.keyboard.type('1650');
  assert.equal(await page.inputValue('#compact-below'), '1650');
  // Typing doesn't redraw the dialog.
  assert.equal(await marker(), 'kept');
  // A new View halfway through typing, which redraws the dialog to say that
  // config.toml has problems, leaves what's typed alone.
  await page.fill('#compact-below', '');
  await page.keyboard.type('16');
  fake.world.configWritable = false;
  await fake.emitState();
  await page.waitForSelector('.settings-sheet .editor-note');
  assert.equal(await marker(), '');
  assert.equal(await page.evaluate(() => document.activeElement.id), 'compact-below');
  await page.keyboard.type('50');
  assert.equal(await page.inputValue('#compact-below'), '1650');
  // A redraw of the dialog, for a ticked box, keeps what was typed.
  await mark();
  await page.click('[data-setting="openMissingApps"]');
  await page.waitForFunction(() => !document.getElementById('compact-below').dataset.marker);
  assert.equal(await page.inputValue('#compact-below'), '1650');
  await page.click('[data-action="save-settings"]');
  const save = await fake.waitForCall('SaveSettings');
  assert.equal(save.args[0].compactBelow, 1650);
  assert.equal(save.args[0].openMissingApps, false);
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
  // Tab properties says which screens are small by the new threshold.
  await openTabProperties(page);
  await page.waitForFunction(() => document.getElementById('small-screens-text').textContent
    === 'When Bench is narrower than 1,650 points, as set in Settings.');
});

test('Return in the threshold field saves Settings, and a threshold that isn\'t a number keeps the one there was', async (t) => {
  const world = defaultWorld();
  world.settings.compactBelow = 1700;
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await openSettings(page);
  assert.equal(await page.inputValue('#compact-below'), '1700');
  await page.fill('#compact-below', '');
  await page.keyboard.press('Enter');
  assert.equal((await fake.waitForCall('SaveSettings')).args[0].compactBelow, 1700);
  await page.waitForSelector('.settings-sheet', { state: 'detached' });

  fake.clearCalls();
  await openSettings(page);
  await page.fill('#compact-below', '1500');
  await page.keyboard.press('Enter');
  assert.equal((await fake.waitForCall('SaveSettings')).args[0].compactBelow, 1500);
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
  // Open at login didn't change, so it isn't set.
  assert.equal(fake.callsTo('SetOpenAtLogin').length, 0);
});

test('choosing Settings… or Tab Properties… again keeps what the open dialog has', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await openSettings(page);
  await page.fill('#compact-below', '1500');
  await fake.emit('bench:command', { name: 'settings' });
  await settle(page);
  assert.equal(await page.inputValue('#compact-below'), '1500');
  await page.click('[data-action="save-settings"]');
  assert.equal((await fake.waitForCall('SaveSettings')).args[0].compactBelow, 1500);
  await page.waitForSelector('.settings-sheet', { state: 'detached' });

  await fake.emit('bench:command', { name: 'edit-tab' });
  await page.waitForSelector('.editor-sheet');
  await page.fill('#tab-name', 'Coding');
  await fake.emit('bench:command', { name: 'edit-tab' });
  await settle(page);
  assert.equal(await page.inputValue('#tab-name'), 'Coding');
  await page.click('[data-action="save-tab"]');
  assert.equal((await fake.waitForCall('SaveTab')).args[1].name, 'Coding');
  await page.waitForSelector('.editor-sheet', { state: 'detached' });
  assert.deepEqual(fake.errors, []);
});

test('Save sends only what Settings changed, keeping what Reload config.toml brought in while it was open', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await openSettings(page);
  await page.click('[data-setting="confirmClose"]');
  await page.fill('#compact-below', '1700');
  await page.click('[data-key="key-zoom_pane-off"]');
  // config.toml is changed outside Bench, and Bench ▸ Reload config.toml
  // applies it while the dialog is open.
  fake.world.settings.hideOtherApps = true;
  fake.world.settings.launchTimeout = '30s';
  fake.world.settings.gap = 8;
  fake.world.keys.jump = '⌃⌥K';
  fake.mReloadConfig();
  await toastIs(page, 'Reloaded config.toml');
  await page.waitForFunction(() => window.bench.store.view.settings.hideOtherApps === true);
  assert.equal(await page.isVisible('.settings-sheet'), true);

  await page.click('[data-action="save-settings"]');
  const save = (await fake.waitForCall('SaveSettings')).args[0];
  assert.equal(save.confirmClose, false);
  assert.equal(save.compactBelow, 1700);
  assert.deepEqual(save.shortcuts, { zoom_pane: '' });
  // What the reload brought in is sent as it is now.
  assert.equal(save.hideOtherApps, true);
  assert.equal(save.launchTimeout, '30s');
  assert.equal(save.gap, 8);
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
  assert.equal(fake.world.settings.hideOtherApps, true);
  assert.equal(fake.world.keys.jump, '⌃⌥K');
  assert.equal(fake.world.keys.zoomPane, '');
  assert.deepEqual(fake.errors, []);
});

test('Cancel, Escape, and the backdrop close Settings without saving', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await openSettings(page);
  await page.click('[data-setting="hideOtherTabs"]');
  await page.click('[data-setting="openAtLogin"]');
  await page.fill('#compact-below', '1500');
  await page.click('[data-key="settings-cancel"]');
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
  await fake.waitForLowered();
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'open-settings');

  // It opens again with the settings, not what was changed.
  await openSettings(page);
  assert.equal(await page.isChecked('[data-setting="hideOtherTabs"]'), true);
  assert.equal(await page.isChecked('[data-setting="openAtLogin"]'), false);
  assert.equal(await page.inputValue('#compact-below'), '1800');
  await page.click('[data-key="follow-focus-bring-here"]');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'open-settings');

  await openSettings(page);
  await page.click('[data-setting="openMissingApps"]');
  await page.mouse.click(20, 880);
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
  assert.equal(fake.callsTo('SaveSettings').length, 0);
  assert.equal(fake.callsTo('SetOpenAtLogin').length, 0);
});

test('a refused save keeps Settings open, and leaves Open at login alone', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  fake.on('SaveSettings', () => { throw new Error('Bench couldn\'t save config.toml: permission denied'); });
  await openSettings(page);
  await page.click('[data-setting="hideOtherTabs"]');
  await page.click('[data-setting="openAtLogin"]');
  await page.click('[data-action="save-settings"]');
  await toastIs(page, 'Bench couldn\'t save config.toml: permission denied');
  assert.equal(await page.isVisible('.settings-sheet'), true);
  assert.equal(fake.callsTo('SetOpenAtLogin').length, 0);
  // The changes are still there, to save again.
  assert.equal(await page.isChecked('[data-setting="hideOtherTabs"]'), false);
  assert.equal(await page.isChecked('[data-setting="openAtLogin"]'), true);

  fake.on('SaveSettings', ([settings], f) => f.mSaveSettings(settings));
  await page.click('[data-action="save-settings"]');
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
  const saves = fake.callsTo('SaveSettings');
  assert.equal(saves.length, 2);
  assert.equal(saves[1].args[0].hideOtherTabs, false);
  assert.deepEqual(fake.callsTo('SetOpenAtLogin').map((c) => c.args), [[true]]);
});

test('a double click on Save in Settings saves once', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  fake.on('SaveSettings', async ([settings], f) => {
    await sleep(150);
    return f.mSaveSettings(settings);
  });
  await openSettings(page);
  await page.click('[data-setting="openAtLogin"]');
  await page.dblclick('[data-action="save-settings"]');
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
  assert.equal(fake.callsTo('SaveSettings').length, 1);
  assert.equal(fake.callsTo('SetOpenAtLogin').length, 1);
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'open-settings');
  assert.deepEqual(fake.errors, []);
});

test('Settings chooses where new windows go, what another tab\'s window does, hiding other apps, and how long to wait', async (t) => {
  const world = defaultWorld();
  world.settings.followFocus = 'bring-here';
  world.settings.launchTimeout = '1m30s';
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  const choices = (name) => page.$$eval('[data-choice="' + name + '"]', (els) => els.map((el) => [el.textContent, el.getAttribute('aria-pressed')]));
  await openSettings(page);
  assert.equal(await page.textContent('#new-windows-label'), 'New windows go to');
  assert.deepEqual(await choices('newWindows'), [['The app\'s pane', 'true'], ['The focused pane', 'false'], ['Leave floating', 'false']]);
  assert.equal(await page.textContent('#follow-focus-label'), 'When another tab\'s window comes forward');
  // Bring it here, which the old tick box couldn't show, is chosen.
  assert.deepEqual(await choices('followFocus'), [['Switch to its tab', 'false'], ['Bring it here', 'true'], ['Do nothing', 'false']]);
  assert.match(await page.textContent('#follow-focus-help'), /moves it into this tab's focused pane\.$/);
  assert.equal(await page.isChecked('[data-setting="hideOtherApps"]'), false);
  assert.equal(await page.getAttribute('[data-setting="hideOtherApps"]', 'aria-describedby'), 'hide-other-apps-help');
  assert.equal(await page.textContent('label[for="launch-wait"]'), 'Wait for apps to open');
  assert.equal(await page.inputValue('#launch-wait'), '90');

  await page.click('[data-key="new-windows-focused-pane"]');
  assert.equal(await page.textContent('#new-windows-help'), 'A new window joins this tab\'s focused pane.');
  await page.click('[data-key="follow-focus-off"]');
  assert.match(await page.textContent('#follow-focus-help'), /Bench stays on this tab/);
  await page.click('[data-setting="hideOtherApps"]');
  // Typing the wait doesn't redraw the field, and Return saves.
  await page.fill('#launch-wait', '30');
  await page.focus('#launch-wait');
  await page.keyboard.press('Enter');
  const save = (await fake.waitForCall('SaveSettings')).args[0];
  assert.deepEqual([save.newWindows, save.followFocus, save.hideOtherApps, save.launchTimeout], ['focused-pane', 'off', true, '30s']);
  assert.equal(save.shortcuts, undefined);
  await page.waitForSelector('.settings-sheet', { state: 'detached' });

  // Opened again, it shows what was saved. A wait that isn't a number
  // keeps the one there is.
  await openSettings(page);
  assert.deepEqual(await choices('newWindows'), [['The app\'s pane', 'false'], ['The focused pane', 'true'], ['Leave floating', 'false']]);
  assert.deepEqual(await choices('followFocus'), [['Switch to its tab', 'false'], ['Bring it here', 'false'], ['Do nothing', 'true']]);
  assert.equal(await page.isChecked('[data-setting="hideOtherApps"]'), true);
  assert.equal(await page.inputValue('#launch-wait'), '30');
  await page.click('[data-key="new-windows-float"]');
  await page.click('[data-key="follow-focus-switch-tab"]');
  await page.fill('#launch-wait', '');
  await page.click('[data-action="save-settings"]');
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
  const again = fake.callsTo('SaveSettings').pop().args[0];
  assert.deepEqual([again.newWindows, again.followFocus, again.launchTimeout], ['float', 'switch-tab', '30s']);
});

test('a text selection that ends on the backdrop doesn\'t close the dialog', async (t) => {
  const { page, close } = await openBench();
  t.after(close);
  await openTabProperties(page);
  await page.fill('#tab-name', 'Coding');
  await page.click('[data-preset="three"]');
  const field = await page.locator('#tab-name').boundingBox();
  await page.mouse.move(field.x + field.width - 4, field.y + field.height / 2);
  await page.mouse.down();
  await page.mouse.move(20, 880, { steps: 6 });
  await page.mouse.up();
  await settle(page);
  assert.notEqual(await page.$('.editor-sheet'), null, 'Tab properties is still open');
  assert.equal(await page.inputValue('#tab-name'), 'Coding');
  assert.equal(await page.getAttribute('[data-preset="three"]', 'aria-pressed'), 'true');
  // A press and release on the backdrop still closes it.
  await page.mouse.click(20, 880);
  await page.waitForSelector('.editor-sheet', { state: 'detached' });
});

test('Tab properties for a new tab, a custom tab, and deleting or resetting', async (t) => {
  const world = defaultWorld();
  world.tabs[1].preset = 'custom';
  const { page, fake, close } = await openBench({ world });
  t.after(close);

  // A new tab previews with index -1 and saves with it.
  await page.click('[data-action="new-tab"]');
  assert.deepEqual((await fake.waitForCall('PreviewTree')).args, [-1, 'single']);
  assert.equal(await page.textContent('#editor-title'), 'New tab');
  assert.equal(await page.$eval('#tab-place', (el) => el.selectedOptions[0].textContent), '4th (⌃⌥4)');
  assert.equal(await page.$('[data-action="delete-tab"]'), null);
  assert.equal(await page.$('[data-action="reset-layout"]'), null);
  await page.fill('#tab-name', 'Design');
  await page.keyboard.press('Enter');
  const save = await fake.waitForCall('SaveTab');
  assert.deepEqual(save.args, [-1, { name: 'Design', preset: 'single', smallScreens: 'stack' }]);
  await page.waitForSelector('.editor-sheet', { state: 'detached' });

  // A custom tab shows its own tree without asking for a preview.
  fake.clearCalls();
  await openTabProperties(page);
  assert.match(await page.textContent('.editor-row p'), /Custom, from dragging windows/);
  assert.equal(await page.$('.preset[aria-pressed="true"]'), null);
  assert.equal(await page.$$eval('.mini-pane', (els) => els.length), 2);
  await settle(page);
  assert.equal(fake.callsTo('PreviewTree').length, 0);
  // Saving keeps it custom.
  await page.click('[data-action="save-tab"]');
  assert.equal((await fake.waitForCall('SaveTab')).args[1].preset, 'custom');
  await page.waitForSelector('.editor-sheet', { state: 'detached' });

  // Reset layout and Delete tab.
  await openTabProperties(page);
  await page.click('[data-action="reset-layout"]');
  assert.deepEqual((await fake.waitForCall('ResetLayout')).args, [1]);
  await page.waitForSelector('.editor-sheet', { state: 'detached' });
  await openTabProperties(page);
  await page.click('[data-action="delete-tab"]');
  assert.deepEqual((await fake.waitForCall('DeleteTab')).args, [1]);
  await page.waitForSelector('.editor-sheet', { state: 'detached' });

  // A refused save keeps Tab properties open.
  fake.on('SaveTab', () => { throw new Error('There\'s no tab 2.'); });
  await openTabProperties(page);
  await page.fill('#tab-name', 'Reading');
  await page.click('[data-action="save-tab"]');
  await toastIs(page, 'There\'s no tab 2.');
  assert.equal(await page.isVisible('.editor-sheet'), true);
  // Cancel closes it without saving.
  await page.click('[data-key="editor-cancel"]');
  await page.waitForSelector('.editor-sheet', { state: 'detached' });
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'tab-1');
});

test('Tab properties says how a taken name will be numbered, and a new tab starts with a free name', async (t) => {
  const world = defaultWorld();
  world.tabs[2].name = 'New tab';
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  const note = () => page.textContent('#tab-name-note');

  // "New tab" is taken, so a new tab starts as "New tab (1)", with no note.
  await page.click('[data-action="new-tab"]');
  await page.waitForSelector('.editor-sheet');
  assert.equal(await page.inputValue('#tab-name'), 'New tab (1)');
  assert.equal(await note(), '');
  assert.equal(await page.getAttribute('#tab-name', 'aria-describedby'), 'tab-name-note');

  // A taken name, in any case, is numbered: the note says so as it's typed.
  await page.fill('#tab-name', 'code');
  assert.equal(await note(), 'There\'s already a tab called Code, so this one will be called code (1).');
  // A number it ends with is replaced, not added to.
  await page.fill('#tab-name', 'New tab (1)');
  assert.equal(await note(), '');
  await page.fill('#tab-name', 'New tab');
  assert.equal(await note(), 'There\'s already a tab called New tab, so this one will be called New tab (1).');
  await page.fill('#tab-name', '   ');
  assert.equal(await note(), 'With no name, it will be called Untitled.');
  await page.fill('#tab-name', 'Design');
  assert.equal(await note(), '');
  // The field keeps the focus and what's typed.
  assert.equal(await page.evaluate(() => document.activeElement.id), 'tab-name');

  // A tab added meanwhile takes the name: the note follows.
  world.tabs.push({ ...world.tabs[0], name: 'DESIGN' });
  await fake.emitState();
  await page.waitForFunction(() => document.getElementById('tab-name-note').textContent !== '');
  assert.equal(await note(), 'There\'s already a tab called DESIGN, so this one will be called Design (1).');
  assert.equal(await page.inputValue('#tab-name'), 'Design');
  assert.equal(await page.evaluate(() => document.activeElement.id), 'tab-name');
  // Go numbers the name it's given.
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('SaveTab')).args, [-1, { name: 'Design', preset: 'single', smallScreens: 'stack' }]);
  await page.waitForSelector('.editor-sheet', { state: 'detached' });
  world.tabs.pop();
  await fake.emitState();

  // Renaming works the same way, and a tab's own name, in any case, is free.
  await openTabProperties(page, 1);
  await page.fill('#tab-name', 'CODE');
  assert.equal(await note(), '');
  await page.fill('#tab-name', 'comms');
  assert.equal(await note(), 'There\'s already a tab called Comms, so this one will be called comms (1).');
});

test('every dialog fits Bench\'s smallest window, with its buttons in view and only its middle scrolling', async (t) => {
  // 640 by 480 points is the smallest Bench's window gets, which is in
  // the compact layout.
  const world = defaultWorld({ firstRun: true, accessibility: false, screenWidth: 640 });
  const { page, fake, close } = await openBench({ world, viewport: { width: 640, height: 480 } });
  t.after(close);
  // fits measures the open dialog: it must be inside the window, clear of
  // the traffic lights at the top left (78 by 52 points), with the controls
  // that sel names fully in view, and the backdrop mustn't scroll.
  const fits = async (name, sel) => {
    await settle(page);
    const m = await page.evaluate((s) => {
      const d = document.querySelector('#overlay-root .dialog');
      const r = d.getBoundingClientRect();
      const overlay = document.querySelector('#overlay-root .overlay');
      const shown = Array.from(d.querySelectorAll(s)).map((el) => {
        const b = el.getBoundingClientRect();
        return b.top >= r.top && b.bottom <= r.bottom;
      });
      return {
        top: r.top, bottom: r.bottom, left: r.left, right: r.right, shown,
        backdrop: overlay.scrollHeight - overlay.clientHeight,
      };
    }, sel);
    assert.ok(m.top >= 0 && m.bottom <= 480 && m.left >= 0 && m.right <= 640, name + ' is inside the window: ' + JSON.stringify(m));
    assert.ok(m.top >= 52 || m.left >= 78, name + ' is clear of the traffic lights: ' + JSON.stringify(m));
    assert.ok(m.shown.length > 0 && m.shown.every(Boolean), name + '\'s ' + sel + ' are in view: ' + JSON.stringify(m));
    assert.equal(m.backdrop, 0, name + ': the backdrop doesn\'t scroll');
  };

  await page.waitForSelector('.guide-sheet');
  await fits('the welcome guide', '.editor-foot button, .guide-head h2');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.guide-sheet', { state: 'detached' });

  await openTabProperties(page);
  await fits('Tab properties', '.editor-foot button, #tab-name');
  // Its middle scrolls, and stays scrolled when the View changes.
  const body = '.editor-sheet .dialog-body';
  assert.ok(await page.$eval(body, (el) => el.scrollHeight > el.clientHeight), 'the middle scrolls');
  // The preview is smaller on a short window.
  assert.equal(await page.$eval('.mini', (el) => Math.round(el.getBoundingClientRect().height)), Math.round(480 * 0.28));
  await page.$eval(body, (el) => { el.scrollTop = el.scrollHeight; });
  const scrolled = await page.$eval(body, (el) => el.scrollTop);
  assert.ok(scrolled > 0);
  fake.world.items.w3.title = 'bench — layout.go';
  await fake.emitState();
  await page.waitForSelector('.mini .chips span:has-text("bench — layout.go")');
  assert.equal(await page.$eval(body, (el) => el.scrollTop), scrolled);
  // Tabbing to a control in the middle brings it into view.
  await page.$eval(body, (el) => { el.scrollTop = 0; });
  await page.focus('[data-key="small-same"]');
  await fits('Tab properties, scrolled', '[data-key="small-same"]');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.editor-sheet', { state: 'detached' });

  // A new tab, with a note under its name.
  await page.click('[data-action="new-tab"]');
  await page.waitForSelector('.editor-sheet');
  await page.fill('#tab-name', 'comms');
  await fits('New tab', '.editor-foot button, #tab-name, #tab-name-note');
  await page.keyboard.press('Escape');

  await openSettings(page);
  await fits('Settings', '.editor-foot button');
  await page.keyboard.press('Escape');

  await page.click('[data-action="open-palette"]');
  await page.waitForSelector('.palette');
  await fits('the jump palette', '.palette-search, .palette-foot');
  await page.keyboard.press('Escape');

  await page.click('.pane [data-action="add"]');
  await page.waitForSelector('.palette');
  await fits('the add dialog', '.palette-search, .palette-foot');
  await page.fill('#palette-query', 'Web page');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#palette-url');
  await fits('the add dialog\'s Web page… step', '.palette-search, .palette-actions button, .palette-foot');
  await page.fill('#palette-url', 'example.com');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#palette-make-title');
  await fits('the add dialog\'s make step', '.palette-search, .palette-actions button, .palette-foot');
});

test('dialogs start lower, and Tab properties\' preview is full size, on a tall window', async (t) => {
  const { page, close } = await openBench({ viewport: { width: 1440, height: 1000 } });
  t.after(close);
  await openTabProperties(page);
  await settle(page);
  const m = await page.evaluate(() => ({
    top: document.querySelector('.editor-sheet').getBoundingClientRect().top,
    mini: document.querySelector('.mini').getBoundingClientRect().height,
    scrolls: (() => { const b = document.querySelector('.editor-sheet .dialog-body'); return b.scrollHeight > b.clientHeight; })(),
  }));
  assert.deepEqual(m, { top: 72, mini: 230, scrolls: false });
});

test('Tab stays inside an open dialog', async (t) => {
  const { page, close } = await openBench();
  t.after(close);
  await openTabProperties(page);
  await page.focus('[data-key="editor-save"]');
  await page.keyboard.press('Tab');
  assert.equal(await page.evaluate(() => document.activeElement.id), 'tab-name');
  await page.keyboard.press('Shift+Tab');
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'editor-save');
  await page.keyboard.press('Escape');

  // In Settings too.
  await openSettings(page);
  await page.focus('[data-key="settings-save"]');
  await page.keyboard.press('Tab');
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'setting-openAtLogin');
  await page.keyboard.press('Shift+Tab');
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'settings-save');
  await page.keyboard.press('Escape');

  // From the palette's search field too.
  await page.click('[data-action="open-palette"]');
  await page.keyboard.press('Shift+Tab');
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'palette-close');
  await page.keyboard.press('Tab');
  assert.equal(await page.evaluate(() => document.activeElement.id), 'palette-query');
});

test('Tab properties closes if its tab is deleted elsewhere', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ current: 2 }) });
  t.after(close);
  await openTabProperties(page);
  fake.world.tabs.pop();
  fake.world.current = 1;
  await fake.emitState();
  await page.waitForSelector('.editor-sheet', { state: 'detached' });
  await fake.waitForLowered();
  // Focus was in Tab properties, and the tab that opened it has gone, so
  // it goes to the current tab.
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'tab-1');
  assert.deepEqual(fake.errors, []);
});

test('Tab properties follows its tab when another tab is deleted, and Save works once', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ current: 2 }) });
  t.after(close);
  await openTabProperties(page);
  assert.equal(await page.inputValue('#tab-name'), 'Research');
  // Comms is deleted elsewhere: Research is now the second tab.
  fake.world.tabs.shift();
  fake.world.current = 1;
  await fake.emitState();
  await settle(page);
  assert.equal(await page.isVisible('.editor-sheet'), true);
  assert.equal(await page.$eval('#tab-place', (el) => el.selectedOptions[0].textContent), '2nd (⌃⌥2)');

  // A double click saves once.
  fake.on('SaveTab', async () => { await sleep(150); return null; });
  await page.dblclick('[data-action="save-tab"]');
  await page.waitForSelector('.editor-sheet', { state: 'detached' });
  const saves = fake.callsTo('SaveTab');
  assert.equal(saves.length, 1);
  assert.equal(saves[0].args[0], 1);
  assert.equal(saves[0].args[1].name, 'Research');
  // Focus goes back to Research's tab, where it is now.
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'tab-1');
  assert.deepEqual(fake.errors, []);
});

test('a new View during Delete tab doesn\'t lose where focus goes back to', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ current: 2 }) });
  t.after(close);
  fake.on('DeleteTab', async ([i], f) => {
    f.world.tabs.splice(i, 1);
    f.world.current = 1;
    await f.emitState();
    await sleep(100);
    return null;
  });
  await openTabProperties(page);
  await page.click('[data-action="delete-tab"]');
  await page.waitForSelector('.editor-sheet', { state: 'detached' });
  await settle(page);
  // The deleted tab's button has gone with it, so focus goes to the
  // current tab.
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'tab-1');
  assert.equal(await page.evaluate(() => window.bench.store.ui.opener), null);
  assert.deepEqual(fake.errors, []);
});

test('a redraw keeps the selection in the tab\'s name and the palette\'s search field', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await openTabProperties(page);
  const selection = (sel) => page.$eval(sel, (el) => [el.selectionStart, el.selectionEnd]);
  // The name starts selected, ready to be replaced.
  assert.deepEqual(await selection('#tab-name'), [0, 4]);
  // A title in the preview changes, which redraws Tab properties.
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

test('Return with keyCode 229, as WebKit sends after a composition, doesn\'t save Tab properties or Settings, or add the address', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  const cdp = await page.context().newCDPSession(page);
  const imeEnter = async () => {
    await cdp.send('Input.dispatchKeyEvent', { type: 'rawKeyDown', key: 'Enter', code: 'Enter', windowsVirtualKeyCode: 229 });
    await cdp.send('Input.dispatchKeyEvent', { type: 'keyUp', key: 'Enter', code: 'Enter', windowsVirtualKeyCode: 229 });
  };
  await openTabProperties(page);
  await imeEnter();
  await settle(page);
  assert.equal(fake.callsTo('SaveTab').length, 0);
  assert.ok(await page.$('.editor-sheet'));
  await page.keyboard.press('Enter');
  await fake.waitForCall('SaveTab');
  await page.waitForSelector('.editor-sheet', { state: 'detached' });

  await openSettings(page);
  await page.focus('#compact-below');
  await imeEnter();
  await settle(page);
  assert.equal(fake.callsTo('SaveSettings').length, 0);
  assert.ok(await page.$('.settings-sheet'));
  await page.keyboard.press('Enter');
  await fake.waitForCall('SaveSettings');
  await page.waitForSelector('.settings-sheet', { state: 'detached' });

  await page.click('.pane[data-pane-id="p3"] [data-action="add"]');
  await page.waitForSelector('.palette');
  await page.keyboard.type('Web page');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#palette-url');
  await page.keyboard.type('example.com');
  await imeEnter();
  await settle(page);
  assert.equal(fake.callsTo('AddToPane').length, 0);
  assert.equal(fake.callsTo('AddWebsite').length, 0);
});
