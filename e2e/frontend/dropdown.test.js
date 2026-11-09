'use strict';

// The drop-down terminal (decision 72): Use as Drop-down in a terminal's ⋯
// menu, Move into This Pane in a pane's menus, the dropdown shortcut and
// the jump palette's Show Drop-down, Settings ▸ Terminals' Drop-down height,
// the browser preview's drawing of the drop-down over the top of the
// window, and the drop-down panel's own page, with Hide.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, openSettings, openTerminalWindow, settle } = require('./helpers/harness.js');
const { defaultWorld, find } = require('./helpers/fixture.js');

// withDropdown returns the default world with w5, Code's terminal, as the
// drop-down, shown or not, out of its pane, as Go has it after Use as
// Drop-down. opts go to defaultWorld.
function withDropdown(shown, opts = {}) {
  const world = defaultWorld(opts);
  const p4 = find(world.tabs[1].root, 'p4');
  p4.items = p4.items.filter((id) => id !== 'w5');
  p4.active = 'w6';
  world.floating.push({ ...world.items.w5, tab: -1 });
  delete world.items.w5;
  world.dropdown = { id: 'w5', shown, inPage: true };
  return world;
}

// menuLabels returns the open menu's entries and headings, in order.
const menuLabels = (page, menu) => page.$$eval(menu + ' .menu-item, ' + menu + ' .menu-heading', (els) => els.map((el) => {
  const keys = el.querySelector('kbd');
  return el.textContent.replace(keys ? keys.textContent : '', '').trim();
}));

// sheetBox returns the in-page drop-down's place and size, and the window's.
const sheetBox = (page) => page.evaluate(() => {
  const sheet = document.getElementById('dropdown').getBoundingClientRect();
  const win = document.getElementById('app-window').getBoundingClientRect();
  return { top: sheet.top - win.top, left: sheet.left - win.left, width: sheet.width, height: sheet.height, winWidth: win.width, winHeight: win.height };
});

test('a terminal\'s ⋯ menu has Use as Drop-down, which draws the drop-down over the top of the window', async (t) => {
  // Without the slide, so that it's measured in place.
  const { page, fake, close } = await openBench({ contextOptions: { reducedMotion: 'reduce' } });
  t.after(close);
  await page.waitForSelector('.pane[data-pane-id="p4"] .xterm');
  await page.$eval('.pane[data-pane-id="p4"] .xterm', (el) => { el.dataset.marker = 'kept'; });
  // An app's window can't be the drop-down.
  await page.click('[data-action="menu"][data-pane="p3"]');
  await page.waitForSelector('#window-menu');
  assert.equal((await menuLabels(page, '#window-menu')).includes('Use as Drop-down'), false);
  await page.keyboard.press('Escape');
  await page.waitForSelector('#window-menu', { state: 'detached' });

  await page.click('[data-action="menu"][data-pane="p4"]');
  await page.waitForSelector('#window-menu');
  const labels = await menuLabels(page, '#window-menu');
  // It's in the last group, before Pop Out of Bench.
  assert.deepEqual(labels.slice(-3), ['New Terminal in ~/code/bench', 'Use as Drop-down', 'Pop Out of Bench']);
  await page.click('#window-menu .menu-item:has-text("Use as Drop-down")');
  assert.deepEqual((await fake.waitForCall('UseAsDropdown')).args, ['w5']);
  await page.waitForFunction(() => {
    const d = window.bench.store.view.dropdown;
    return d && d.shown && document.querySelector('#dropdown [data-term-host="w5"] .xterm');
  });
  await page.waitForFunction(() => document.getElementById('toast').textContent.includes('is the drop-down now'));
  assert.equal(await page.textContent('#toast'),
    'Terminal — zsh — ~/code/bench is the drop-down now. Show it again with View ▸ Show Drop-down, or set its shortcut in Settings ▸ Keyboard.');
  // It's the same terminal, moved, not a new one.
  assert.equal(await page.$eval('#dropdown .xterm', (el) => el.dataset.marker), 'kept');
  assert.equal(fake.callsTo('TermAttach').filter((c) => c.args[0] === 'w5').length, 1);
  assert.equal(await page.$('#panes [data-term-host="w5"]'), null);
  // It covers the top of the window, the whole width, at 40% of its height.
  const box = await sheetBox(page);
  assert.deepEqual([box.top, box.left, box.width], [0, 0, box.winWidth]);
  assert.ok(Math.abs(box.height - box.winHeight * 0.4) < 1, 'height ' + box.height + ' of ' + box.winHeight);
  // Its bar names it, and has Back to Bench and Hide; it's a region named
  // for VoiceOver; and its terminal has the keyboard.
  assert.equal(await page.textContent('#dropdown .term-window-note'), 'Drop-down: Terminal — zsh — ~/code/bench');
  assert.deepEqual(await page.$$eval('#dropdown-bar button', (els) => els.map((el) => [el.textContent, el.title])), [
    ['Back to Bench', 'Put this terminal back in Bench, in the focused pane'],
    ['Hide', 'Hide the drop-down (⌘⎋)'],
  ]);
  assert.equal(await page.getAttribute('#dropdown', 'aria-label'), 'Drop-down terminal');
  await page.waitForFunction(() => document.activeElement && document.activeElement.closest('#dropdown .term-host'));
  await page.keyboard.type('ls');
  await fake.waitForCall('TermInput', (a) => a[0] === 'w5' && a[1] === 's');
  // Escape goes to Go, which hides the drop-down only at the shell's prompt.
  await page.keyboard.press('Escape');
  await fake.waitForCall('TermInput', (a) => a[0] === 'w5' && a[1] === '\x1b');
  assert.equal(fake.callsTo('HideDropdown').length, 0);
});

test('an Escape typed while other keys are on their way to Go goes alone, so that Go can tell it from Option keys', async (t) => {
  const { page, fake, close } = await openBench({ world: withDropdown(true) });
  t.after(close);
  await page.waitForSelector('#dropdown .xterm');
  await page.waitForFunction(() => document.activeElement && document.activeElement.closest('#dropdown .term-host'));
  // Go holds the first call until every key is typed, so that what's
  // typed meanwhile waits in the page.
  let release;
  const held = new Promise((r) => { release = r; });
  fake.on('TermInput', () => held.then(() => null));
  await page.keyboard.type('ab');
  await page.keyboard.press('Escape');
  await page.keyboard.type('cd');
  await page.keyboard.press('Escape');
  await page.keyboard.press('Escape');
  release();
  const sent = () => fake.callsTo('TermInput').filter((c) => c.args[0] === 'w5').map((c) => c.args[1]);
  for (let i = 0; i < 80 && sent().length < 6; i += 1) await new Promise((r) => setTimeout(r, 25));
  // In order, with what waited together sent together, but each Escape on
  // its own.
  assert.deepEqual(sent(), ['a', 'b', '\x1b', 'cd', '\x1b', '\x1b']);
});

test('the dropdown shortcut and Hide hide and show the drop-down, which keeps its terminal', async (t) => {
  // The browser preview runs Bench's shortcuts in the page.
  const { page, fake, close } = await openBench({ world: withDropdown(true, { demo: true, keys: { dropdown: '⌃⌥D' } }) });
  t.after(close);
  await page.waitForSelector('#dropdown .xterm');
  await page.$eval('#dropdown .xterm', (el) => { el.dataset.marker = 'kept'; });
  // Hide's tooltip has the key that hides it from its terminal.
  assert.equal(await page.getAttribute('[data-action="hide-dropdown"]', 'title'), 'Hide the drop-down (⌘⎋)');
  // Hide gives the keyboard back.
  await page.click('[data-action="hide-dropdown"]');
  await fake.waitForCall('HideDropdown');
  await page.waitForSelector('#dropdown', { state: 'hidden' });
  assert.equal(await page.evaluate(() => Boolean(document.activeElement && document.activeElement.closest('#dropdown'))), false);
  // Out of sight, its terminal goes on.
  assert.ok(await page.$('#dropdown [data-term-host="w5"]') || await page.$('#term-park [data-term-host="w5"]'));

  await page.keyboard.press('Control+Alt+KeyD');
  await fake.waitForCall('ToggleDropdown');
  await page.waitForSelector('#dropdown .xterm');
  assert.equal(await page.$eval('#dropdown .xterm', (el) => el.dataset.marker), 'kept');
  assert.equal(fake.callsTo('TermAttach').filter((c) => c.args[0] === 'w5').length, 1);
  await page.waitForFunction(() => document.activeElement && document.activeElement.closest('#dropdown .term-host'));
  // The shortcut works from the drop-down's terminal too.
  fake.clearCalls();
  await page.keyboard.press('Control+Alt+KeyD');
  await fake.waitForCall('ToggleDropdown');
  await page.waitForSelector('#dropdown', { state: 'hidden' });
  assert.deepEqual(fake.callsTo('TermInput').filter((c) => c.args[0] === 'w5'), []);

  // A redraw of the panes while it shows leaves it, and its focus, alone.
  await page.keyboard.press('Control+Alt+KeyD');
  await page.waitForSelector('#dropdown .xterm');
  await page.waitForFunction(() => document.activeElement && document.activeElement.closest('#dropdown .term-host'));
  fake.world.items.w3.title = 'bench — layout.go';
  await fake.emitState();
  await page.waitForSelector('.subtab:has-text("bench — layout.go")');
  await settle(page);
  assert.equal(await page.$eval('#dropdown .xterm', (el) => el.dataset.marker), 'kept');
  assert.equal(await page.evaluate(() => Boolean(document.activeElement.closest('#dropdown .term-host'))), true);
});

test('Move into This Pane in a pane\'s menus brings the drop-down back into that pane', async (t) => {
  const world = withDropdown(false);
  // p4 is empty, for the empty pane's menu.
  const p3 = find(world.tabs[1].root, 'p3');
  const p4 = find(world.tabs[1].root, 'p4');
  p3.items.push('w6');
  p4.items = [];
  p4.active = '';
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  // The empty pane's menu.
  await page.click('.pane[data-pane-id="p4"] .pane-body', { button: 'right' });
  await page.waitForSelector('#pane-menu');
  const paneLabels = await menuLabels(page, '#pane-menu');
  assert.deepEqual(paneLabels.slice(paneLabels.indexOf('Drop-down'), paneLabels.indexOf('Drop-down') + 2), ['Drop-down', 'Move into This Pane']);
  await page.keyboard.press('Escape');
  await page.waitForSelector('#pane-menu', { state: 'detached' });
  // A window's ⋯ menu, which has it as a group of its own.
  await page.click('[data-action="menu"][data-pane="p3"]');
  await page.waitForSelector('#window-menu');
  const labels = await menuLabels(page, '#window-menu');
  const at = labels.indexOf('Drop-down');
  assert.ok(at > 0, labels.join(', '));
  assert.equal(labels[at + 1], 'Move into This Pane');
  await page.click('#window-menu .menu-item:has-text("Move into This Pane")');
  assert.deepEqual((await fake.waitForCall('AddToPane')).args, ['p3', { type: 'floating', id: 'w5' }]);
  await page.waitForSelector('.pane[data-pane-id="p3"] [data-term-host="w5"] .xterm');
  assert.equal(await page.evaluate(() => window.bench.store.view.dropdown), undefined);
  assert.equal(await page.isHidden('#dropdown'), true);
  // With no drop-down, the menus don't offer it.
  await page.click('[data-action="menu"][data-pane="p3"]');
  await page.waitForSelector('#window-menu');
  assert.equal((await menuLabels(page, '#window-menu')).includes('Move into This Pane'), false);
});

test('the jump palette has Show Drop-down, with its shortcut, and Hide Drop-down while it shows', async (t) => {
  const { page, fake, close } = await openBench({ world: withDropdown(false, { keys: { dropdown: '⌃⌥D' } }) });
  t.after(close);
  await page.click('[data-action="open-palette"]');
  await page.waitForSelector('#palette-query');
  await page.fill('#palette-query', 'drop-down');
  const row = page.locator('#palette-list [role="option"]:has-text("Show Drop-down")');
  await row.waitFor();
  assert.match(await row.textContent(), /Show Drop-down.*Terminal — zsh — ~\/code\/bench.*⌃⌥D/);
  await row.click();
  await fake.waitForCall('ToggleDropdown');
  await page.waitForSelector('#dropdown .xterm');
  // The browser preview's drop-down covers the title bar's Jump to…, as
  // the panel covers the top of the screen.
  await page.$eval('[data-action="open-palette"]', (el) => el.click());
  await page.waitForSelector('#palette-query');
  await page.fill('#palette-query', 'drop-down');
  await page.locator('#palette-list [role="option"]:has-text("Hide Drop-down")').waitFor();
});

test('Settings ▸ Terminals has Drop-down height, from 20 to 90 percent, which Save sends', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await openSettings(page, 'terminals');
  assert.equal(await page.textContent('label[for="dropdown-height"]'), 'Drop-down height');
  assert.equal(await page.inputValue('#dropdown-height'), '40');
  assert.deepEqual(await page.$eval('#dropdown-height', (el) => [el.type, el.min, el.max, el.getAttribute('aria-describedby')]),
    ['number', '20', '90', 'dropdown-height-help']);
  assert.match(await page.textContent('#dropdown-height-help'), /^From 20 to 90\. The drop-down terminal slides down this far from the top of the screen/);
  await page.fill('#dropdown-height', '55');
  // Terminals is marked as changed.
  await page.waitForSelector('#settings-tab-terminals .settings-dot');
  await page.click('[data-key="settings-save"]');
  const saved = await fake.waitForCall('SaveSettings');
  assert.equal(saved.args[0].dropdownHeight, 55);
  assert.equal(saved.args[0].terminalFontSize, 12.5);
});

test('the preview\'s drop-down slides down, except with Reduce Motion, and fits a 640 × 480 window', async (t) => {
  {
    const { page, close } = await openBench({ world: withDropdown(true) });
    t.after(close);
    await page.waitForSelector('#dropdown .xterm');
    assert.equal(await page.$eval('#dropdown', (el) => getComputedStyle(el).animationName), 'dropdown-in');
  }
  const { page, close } = await openBench({ world: withDropdown(true), viewport: { width: 640, height: 480 }, contextOptions: { reducedMotion: 'reduce', colorScheme: 'light' } });
  t.after(close);
  await page.waitForSelector('#dropdown .xterm');
  assert.equal(await page.$eval('#dropdown', (el) => getComputedStyle(el).animationName), 'none');
  const box = await sheetBox(page);
  // 40% of 480 is under the drop-down's least height.
  assert.equal(box.height, 192);
  // Its bar's buttons are inside it, and its terminal has room below.
  const inside = await page.evaluate(() => {
    const sheet = document.getElementById('dropdown').getBoundingClientRect();
    return Array.from(document.querySelectorAll('#dropdown-bar button, #dropdown .term-window-note')).every((el) => {
      const r = el.getBoundingClientRect();
      return r.left >= sheet.left && r.right <= sheet.right && r.top >= sheet.top && r.bottom <= sheet.bottom && r.width > 0;
    });
  });
  assert.equal(inside, true);
  const term = await page.locator('#dropdown .term-host').boundingBox();
  assert.ok(term.height > 100, 'terminal height ' + term.height);
});

test('the drop-down\'s panel page has Hide, with the shortcut, and Escape on its bar hides it', async (t) => {
  const { page, fake, close } = await openTerminalWindow({ id: 'w5', dropdown: true, world: withDropdown(true, { keys: { dropdown: '⌃⌥D' } }) });
  t.after(close);
  await page.waitForFunction(() => document.querySelector('.terminal-window .xterm-rows').textContent.includes('go run ./cmd/bench'));
  assert.equal(await page.evaluate(() => document.body.classList.contains('is-dropdown')), true);
  assert.equal(await page.getAttribute('.terminal-window', 'aria-label'), 'Drop-down terminal');
  // It's named once Go's View comes, as Go sends it on every change.
  assert.equal(await page.textContent('.term-window-note'), 'Drop-down');
  await fake.emitState();
  await page.waitForFunction(() => document.querySelector('.term-window-note').textContent === 'Drop-down: Terminal — zsh — ~/code/bench');
  assert.deepEqual(await page.$$eval('.term-window-bar button', (els) => els.map((el) => [el.textContent, el.title])), [
    ['Back to Bench', 'Put this terminal back in Bench, in the focused pane'],
    ['Hide', 'Hide the drop-down (⌘⎋)'],
  ]);
  // The terminal has the keyboard; Escape there goes to Go, as input.
  await page.waitForFunction(() => document.activeElement && document.activeElement.closest('.term-host'));
  await page.keyboard.press('Escape');
  await fake.waitForCall('TermInput', (a) => a[0] === 'w5' && a[1] === '\x1b');
  assert.equal(fake.callsTo('HideDropdown').length, 0);
  // Escape on the bar hides it.
  await page.focus('[data-key="back-to-bench"]');
  await page.keyboard.press('Escape');
  await fake.waitForCall('HideDropdown');
  fake.clearCalls();
  await page.click('[data-action="hide-dropdown"]');
  await fake.waitForCall('HideDropdown');
  // The terminal's menu has Back to Bench and Hide Drop-down, with ⌘⎋.
  await page.click('.terminal-window .term-host', { button: 'right' });
  await page.waitForSelector('#menu-root .menu');
  const labels = await menuLabels(page, '#menu-root');
  assert.deepEqual(labels.slice(-2), ['Back to Bench', 'Hide Drop-down']);
  assert.equal(await page.textContent('#menu-root .menu-item:has-text("Hide Drop-down") kbd'), '⌘⎋');
  await page.click('#menu-root .menu-item:has-text("Back to Bench")');
  assert.deepEqual((await fake.waitForCall('BackToBench')).args, ['w5']);
});

test('⌘⎋ in the drop-down\'s panel hides it, even while a program has Escape, and so does a click in another app', async (t) => {
  const { page, fake, close } = await openTerminalWindow({ id: 'w5', dropdown: true, world: withDropdown(true) });
  t.after(close);
  await page.waitForFunction(() => document.activeElement && document.activeElement.closest('.term-host'));
  // ⌘⎋ with the terminal's keyboard isn't typed: it hides the drop-down.
  await page.keyboard.press('Meta+Escape');
  await fake.waitForCall('HideDropdown');
  assert.deepEqual(fake.callsTo('TermInput').filter((c) => c.args[0] === 'w5'), []);
  // The panel losing the keyboard, as a click in another app takes it,
  // hides it too.
  fake.clearCalls();
  await page.evaluate(() => window.dispatchEvent(new FocusEvent('blur')));
  await fake.waitForCall('HideDropdown');
  // But not while the close question is open.
  fake.clearCalls();
  await fake.emit('bench:command', { name: 'confirm-close-terminal', itemId: 'w5', terminals: [{ id: 'w5', title: 'zsh', running: 'sleep' }] });
  await page.waitForSelector('.question-sheet');
  await page.evaluate(() => window.dispatchEvent(new FocusEvent('blur')));
  await settle(page);
  assert.equal(fake.callsTo('HideDropdown').length, 0);
});

test('a popped-out terminal\'s window doesn\'t hide for a click elsewhere or ⌘⎋', async (t) => {
  const world = defaultWorld();
  const { page, fake, close } = await openTerminalWindow({ id: 'w5', world });
  t.after(close);
  await page.waitForFunction(() => document.activeElement && document.activeElement.closest('.term-host'));
  await page.keyboard.press('Meta+Escape');
  await page.evaluate(() => window.dispatchEvent(new FocusEvent('blur')));
  await settle(page);
  assert.equal(fake.callsTo('HideDropdown').length, 0);
});

test('the preview\'s drop-down hides for ⌘⎋ in it, and for a press outside it, as its panel does', async (t) => {
  const { page, fake, close } = await openBench({ world: withDropdown(true) });
  t.after(close);
  await page.waitForSelector('#dropdown .xterm');
  await page.waitForFunction(() => document.activeElement && document.activeElement.closest('#dropdown .term-host'));
  await page.keyboard.press('Meta+Escape');
  await fake.waitForCall('HideDropdown');
  await page.waitForSelector('#dropdown', { state: 'hidden' });
  assert.deepEqual(fake.callsTo('TermInput').filter((c) => c.args[0] === 'w5'), []);

  // A press in the drop-down leaves it; one outside it hides it.
  fake.clearCalls();
  fake.world.dropdown.shown = true;
  await fake.emitState();
  await page.waitForSelector('#dropdown .xterm');
  await page.click('#dropdown .term-host');
  await settle(page);
  assert.equal(fake.callsTo('HideDropdown').length, 0);
  const box = await page.locator('#dropdown').boundingBox();
  await page.mouse.click(box.x + box.width / 2, box.y + box.height + 40);
  await fake.waitForCall('HideDropdown');
  await page.waitForSelector('#dropdown', { state: 'hidden' });

  // A press in a dialog that opened over it belongs to the dialog.
  fake.clearCalls();
  fake.world.dropdown.shown = true;
  await fake.emitState();
  await page.waitForSelector('#dropdown .xterm');
  await page.click('#dropdown .term-host');
  await page.keyboard.press('Meta+Comma');
  await page.waitForSelector('.settings-sheet');
  await page.click('.settings-sheet h2');
  await settle(page);
  assert.equal(fake.callsTo('HideDropdown').length, 0);
  assert.equal(await page.locator('#dropdown').isVisible(), true);
});

test('New Tab from Open Windows and the jump palette\'s Not in a tab leave out the drop-down', async (t) => {
  const { page, fake, close } = await openBench({ world: withDropdown(false) });
  t.after(close);
  await page.click('[data-action="open-palette"]');
  await page.waitForSelector('#palette-query');
  const groups = await page.$$eval('#overlay-root .palette-list > :not(.palette-set), #overlay-root .palette-set > *', (els) => {
    let group = '';
    return els.flatMap((el) => {
      if (el.classList.contains('palette-group')) {
        group = el.textContent;
        return [];
      }
      return group === 'Not in a tab' ? [el.querySelector('.palette-label').textContent] : [];
    });
  });
  assert.deepEqual(groups, ['Release notes', 'Downloads']);
  await page.keyboard.press('Escape');
  await page.waitForSelector('#palette-query', { state: 'detached' });

  await page.click('[data-action="new-tab"]');
  await page.waitForSelector('.editor-sheet');
  await page.click('[data-key="start-windows"]');
  await page.waitForSelector('.win-list');
  const listed = await page.$$eval('.win-list .win-row', (els) => els.map((el) => el.textContent));
  assert.equal(listed.some((text) => text.includes('zsh')), false, listed.join(', '));
  await page.click('[data-key="editor-save"]');
  assert.deepEqual((await fake.waitForCall('NewTabFromWindows')).args[1], ['f101', 'f102']);
});
