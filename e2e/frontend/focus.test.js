'use strict';

// Keyboard focus and terminals: Go's focus-terminal command, a terminal
// letting go of the focus when its pane does, and dialogs giving the focus
// back to the terminal that had it, or to the tab whose menu opened them.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, openTabProperties, settle } = require('./helpers/harness.js');
const { defaultWorld, manyTabs } = require('./helpers/fixture.js');

// focusedTerminal returns the item ID of the terminal with the keyboard
// focus, or the focused element's tag.
const focusedTerminal = (page) => page.evaluate(() => {
  const el = document.activeElement;
  const host = el && el.closest('.term-host[data-term-host]');
  return host ? host.dataset.termHost : el.tagName;
});

test('focus-terminal gives a terminal the keyboard focus, waiting for it to be drawn', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.waitForSelector('.pane[data-pane-id="p4"] .xterm');
  // Go focuses p4, which shows w5: the command can come before the View.
  await fake.emit('bench:command', { name: 'focus-terminal', itemId: 'w5' });
  await settle(page);
  assert.equal(await focusedTerminal(page), 'BODY', 'p4 isn\'t the focused pane yet');
  fake.world.tabs[1].focus = 'p4';
  await fake.emitState();
  await page.waitForFunction(() => document.activeElement.closest('.term-host[data-term-host="w5"]'));
  // Typing goes to the terminal.
  await page.keyboard.type('ls');
  await fake.waitForCall('TermInput', (a) => a[0] === 'w5' && a[1].endsWith('s'));

  // Go cycles the sub-tab to w6, which hasn't been drawn yet.
  await fake.emit('bench:command', { name: 'focus-terminal', itemId: 'w6' });
  fake.world.tabs[1].root.children[1].active = 'w6';
  await fake.emitState();
  await page.waitForFunction(() => document.activeElement.closest('.term-host[data-term-host="w6"]'));
});

test('a terminal lets go of the focus when the focus moves to another pane or sub-tab', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('.pane[data-pane-id="p4"] .xterm');
  await fake.waitForCall('FocusPane');
  await page.waitForSelector('.pane[data-pane-id="p4"][data-focused="true"]');
  assert.equal(await focusedTerminal(page), 'w5');
  // A redraw that doesn't move the focus keeps it.
  fake.world.items.w3.title = 'bench — main.go';
  await fake.emitState();
  await page.waitForSelector('.subtab:has-text("bench — main.go")');
  assert.equal(await focusedTerminal(page), 'w5');

  // Control+Option+Left focuses Zed's pane.
  fake.world.tabs[1].focus = 'p3';
  await fake.emitState();
  await page.waitForSelector('.pane[data-pane-id="p3"][data-focused="true"]');
  assert.equal(await focusedTerminal(page), 'BODY');
  fake.clearCalls();
  await page.keyboard.type('x');
  await settle(page);
  assert.equal(fake.callsTo('TermInput').length, 0, 'typing doesn\'t reach the terminal');

  // Control+Option+Tab in p4 shows w6 instead of w5, which had the focus.
  await page.click('.pane[data-pane-id="p4"] .xterm');
  await page.waitForSelector('.pane[data-pane-id="p4"][data-focused="true"]');
  assert.equal(await focusedTerminal(page), 'w5');
  fake.world.tabs[1].root.children[1].active = 'w6';
  await fake.emitState();
  await page.waitForFunction(() => document.querySelector('.pane[data-pane-id="p4"] .term-host').dataset.termHost === 'w6');
  assert.equal(await focusedTerminal(page), 'BODY');
});

test('focus-terminal waits while a dialog has the focus', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  fake.world.tabs[1].focus = 'p4';
  await fake.emitState();
  await page.waitForSelector('.pane[data-pane-id="p4"][data-focused="true"] .xterm');
  await page.click('[data-action="open-palette"]');
  await page.waitForSelector('.palette');
  await fake.emit('bench:command', { name: 'focus-terminal', itemId: 'w5' });
  await settle(page);
  assert.equal(await page.evaluate(() => document.activeElement.id), 'palette-query');
});

test('closing the palette, Tab properties, or Settings gives the focus back to the terminal that had it', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('.pane[data-pane-id="p4"] .xterm');
  await page.waitForSelector('.pane[data-pane-id="p4"][data-focused="true"]');
  assert.equal(await focusedTerminal(page), 'w5');
  // The jump shortcut, in the app.
  await fake.emit('bench:command', { name: 'toggle-palette' });
  await page.waitForSelector('.palette');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.palette', { state: 'detached' });
  assert.equal(await focusedTerminal(page), 'w5');
  fake.clearCalls();
  await page.keyboard.type('pwd');
  await fake.waitForCall('TermInput', (a) => a[0] === 'w5' && a[1].endsWith('d'));

  // Tab properties (View ▸ Tab Properties…) and Settings (Bench ▸
  // Settings…) too.
  for (const [name, sheet, first] of [['edit-tab', '.editor-sheet', 'tab-name'], ['settings', '.settings-sheet', 'setting-openAtLogin']]) {
    await fake.emit('bench:command', { name });
    await page.waitForSelector(sheet);
    assert.equal(await page.evaluate(() => document.activeElement.dataset.key), first, name);
    await page.keyboard.press('Escape');
    await page.waitForSelector(sheet, { state: 'detached' });
    assert.equal(await focusedTerminal(page), 'w5', name);
  }
});

test('a terminal that Go asks to focus while a dialog is open gets the focus when it closes', async (t) => {
  const world = defaultWorld({ current: 0 });
  world.tabs[1].focus = 'p4';
  const { page, fake, close } = await openBench({ world, setup: (f) => {
    // Deleting the current tab switches to the next one, with the focus,
    // and Go focuses the terminal in its focused pane while Tab properties
    // are still open.
    f.on('DeleteTab', async ([i]) => {
      f.world.tabs.splice(i, 1);
      f.world.current = 0;
      await f.emit('bench:command', { name: 'focus-terminal', itemId: 'w5' });
      await f.emitState();
      return null;
    });
  } });
  t.after(close);
  await openTabProperties(page);
  await page.click('[data-action="delete-tab"]');
  await page.waitForSelector('.editor-sheet', { state: 'detached' });
  await page.waitForFunction(() => document.activeElement.closest('.term-host[data-term-host="w5"]'), null, { timeout: 2000 });
  fake.clearCalls();
  await page.keyboard.type('ls');
  await fake.waitForCall('TermInput', (a) => a[0] === 'w5' && a[1].endsWith('s'));

  // The jump palette, after the focus has moved to another terminal.
  await fake.emit('bench:command', { name: 'toggle-palette' });
  await page.waitForSelector('.palette');
  fake.world.tabs[0].root.children[1].active = 'w6';
  await fake.emit('bench:command', { name: 'focus-terminal', itemId: 'w6' });
  await fake.emitState();
  await settle(page);
  assert.equal(await page.evaluate(() => document.activeElement.id), 'palette-query');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.palette', { state: 'detached' });
  await page.waitForFunction(() => document.activeElement.closest('.term-host[data-term-host="w6"]'), null, { timeout: 2000 });
});

test('closing a dialog doesn\'t give the focus back to a terminal outside the focused pane', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('.pane[data-pane-id="p4"] .xterm');
  await page.waitForSelector('.pane[data-pane-id="p4"][data-focused="true"]');
  assert.equal(await focusedTerminal(page), 'w5');
  await fake.emit('bench:command', { name: 'toggle-palette' });
  await page.waitForSelector('.palette');
  // Control+Option+Left focuses Zed's pane while the palette is open.
  fake.world.tabs[1].focus = 'p3';
  await fake.emitState();
  await page.waitForSelector('.pane[data-pane-id="p3"][data-focused="true"]');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.palette', { state: 'detached' });
  assert.equal(await focusedTerminal(page), 'BODY');
  fake.clearCalls();
  await page.keyboard.type('x');
  await settle(page);
  assert.equal(fake.callsTo('TermInput').length, 0, 'typing doesn\'t reach the terminal');
});

test('a terminal that Go asks to focus during a dialog, but isn\'t shown yet, takes the focus from the opener when it is', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('[data-action="open-palette"]');
  await page.waitForSelector('.palette');
  await fake.emit('bench:command', { name: 'focus-terminal', itemId: 'w6' });
  await page.keyboard.press('Escape');
  await page.waitForSelector('.palette', { state: 'detached' });
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'open-palette');
  fake.world.tabs[1].focus = 'p4';
  fake.world.tabs[1].root.children[1].active = 'w6';
  await fake.emitState();
  await page.waitForFunction(() => document.activeElement.closest('.term-host[data-term-host="w6"]'), null, { timeout: 2000 });
});

test('a tab\'s menu, and Tab properties, give the focus back to their tab after following it', async (t) => {
  const { page, fake, close } = await openBench({ world: manyTabs(defaultWorld(), 4) });
  t.after(close);
  const focused = () => page.evaluate(() => document.activeElement.dataset.key);
  // Comms is deleted elsewhere while Research's menu is open, so Research
  // is the second tab now, and Tab 4 is where Research was.
  await page.click('.tab[data-index="2"]', { button: 'right' });
  await page.waitForSelector('#tab-menu');
  fake.world.tabs.splice(0, 1);
  fake.world.current = 0;
  await fake.emitState();
  await page.waitForSelector('.tab[data-index="1"]:has-text("Research")');
  await page.keyboard.press('Escape');
  await page.waitForSelector('#tab-menu', { state: 'detached' });
  assert.equal(await focused(), 'tab-1');

  // Tab properties of Tab 4, while Research is deleted elsewhere.
  await openTabProperties(page, 2);
  fake.world.tabs.splice(1, 1);
  await fake.emitState();
  await page.waitForSelector('.tab[data-index="1"]:has-text("Tab 4")');
  assert.equal(await page.inputValue('#tab-name'), 'Tab 4');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.editor-sheet', { state: 'detached' });
  assert.equal(await focused(), 'tab-1');
});

test('a tab\'s menu, and a tab\'s button, give the focus to their tab wherever it moves, or the current tab if it\'s deleted', async (t) => {
  const { page, fake, close } = await openBench({ world: manyTabs(defaultWorld({ current: 2 }), 5) });
  t.after(close);
  const focused = () => page.evaluate(() => document.activeElement.dataset.key);
  const remove = async (i, current) => {
    const n = fake.world.tabs.length - 1;
    fake.world.tabs.splice(i, 1);
    fake.world.current = current;
    await fake.emitState();
    await page.waitForFunction((count) => document.querySelectorAll('.tab').length === count, n);
    await settle(page);
  };
  // Comms's menu is open when Comms is deleted elsewhere, such as in
  // config.toml: the focus goes to Research, the current tab, rather than
  // to the first tab's button, which is Code's now.
  await page.click('.tab[data-index="0"]', { button: 'right' });
  await page.waitForSelector('#tab-menu');
  await remove(0, 1);
  assert.equal(await page.$('#tab-menu'), null);
  assert.equal(await focused(), 'tab-1');

  // Tab 5's button has the focus when Code is deleted: it keeps it as Tab 5
  // moves back one place.
  await page.focus('.tab[data-index="3"]');
  await remove(0, 0);
  assert.equal(await page.textContent('.tab[data-index="2"] span'), 'Tab 5');
  assert.equal(await focused(), 'tab-2');

  // Tab 4's button has the focus when Tab 4 is deleted: it goes to the
  // current tab, Research.
  await page.focus('.tab[data-index="1"]');
  await remove(1, 0);
  assert.equal(await focused(), 'tab-0');
});

// deletingWorld returns a world of n tabs, and a setup that deletes a tab
// as Go does: a tab before the current one moves it back one place, and
// deleting the last tab, if current, makes the one before it current. If
// hold() has been called, the View without the tab waits until release()
// is, after DeleteTab's answer.
function deletingWorld(n) {
  let held = null;
  let release = null;
  return {
    world: manyTabs(defaultWorld(), n),
    setup: (f) => {
      f.on('DeleteTab', async ([i]) => {
        const send = () => {
          f.world.tabs.splice(i, 1);
          if (i < f.world.current || f.world.current >= f.world.tabs.length) f.world.current--;
          return f.emitState();
        };
        if (held) held.then(send);
        else await send();
        return null;
      });
    },
    hold: () => { held = new Promise((r) => { release = r; }); },
    release: () => {
      held = null;
      release();
    },
  };
}

test('deleting a tab gives the focus to the current tab, whenever its View comes', async (t) => {
  const tabs = deletingWorld(6);
  const { page, close } = await openBench(tabs);
  t.after(close);
  const focused = () => page.evaluate(() => document.activeElement.dataset.key);
  const del = async (index) => {
    await openTabProperties(page, index);
    await page.click('[data-action="delete-tab"]');
    await page.waitForSelector('.editor-sheet', { state: 'detached' });
  };
  // Code is current. Deleting Research, the third tab, moves Tab 4 into
  // its place, but the focus goes to Code.
  await del(2);
  await settle(page);
  assert.equal(await focused(), 'tab-1');

  // Tab 5, now the fourth tab, is current, and Comms is deleted, with its
  // View after the answer. The focus goes to Tab 5's button, and stays on
  // it when the View moves Tab 5 back one place, rather than staying with
  // the fourth button, which is Tab 6's then.
  await page.click('.tab[data-index="3"]');
  await page.waitForSelector('.tab[data-index="3"][aria-pressed="true"]');
  tabs.hold();
  await del(0);
  assert.equal(await focused(), 'tab-3');
  tabs.release();
  await page.waitForFunction(() => document.querySelectorAll('.tab').length === 4);
  await settle(page);
  assert.equal(await page.textContent('.tab[data-index="2"] span'), 'Tab 5');
  assert.equal(await focused(), 'tab-2');

  // Tab 6, the last tab, is current, and deleted, with its View after the
  // answer: the focus goes to Tab 5, which Go makes current.
  await page.click('.tab[data-index="3"]');
  await page.waitForSelector('.tab[data-index="3"][aria-pressed="true"]');
  tabs.hold();
  await del(3);
  tabs.release();
  await page.waitForFunction(() => document.querySelectorAll('.tab').length === 3);
  await settle(page);
  assert.equal(await page.textContent('.tab[data-index="2"] span'), 'Tab 5');
  assert.equal(await focused(), 'tab-2');
});

test('deleting a tab from View ▸ Tab Properties… gives the focus back to the tab button that had it', async (t) => {
  const tabs = deletingWorld(4);
  const { page, fake, close } = await openBench(tabs);
  t.after(close);
  const focused = () => page.evaluate(() => document.activeElement.dataset.key);
  const del = async () => {
    await fake.emit('bench:command', { name: 'edit-tab' });
    await page.waitForSelector('.editor-sheet');
    await page.click('[data-action="delete-tab"]');
    await page.waitForSelector('.editor-sheet', { state: 'detached' });
  };
  // Tab 4's button has the focus, and Code, the current tab, is deleted:
  // Tab 4 moves back one place, with the focus.
  await page.focus('.tab[data-index="3"]');
  await del();
  await settle(page);
  assert.equal(await focused(), 'tab-2');

  // Research is current now. Comms's button has the focus, and Research is
  // deleted, with its View after the answer.
  await page.focus('.tab[data-index="0"]');
  tabs.hold();
  await del();
  tabs.release();
  await page.waitForFunction(() => document.querySelectorAll('.tab').length === 2);
  await settle(page);
  assert.equal(await focused(), 'tab-0');
});

test('a dialog that a command opens in place of a tab\'s menu gives the focus back to the tab', async (t) => {
  const { page, fake, close } = await openBench({ world: manyTabs(defaultWorld(), 4) });
  t.after(close);
  const focused = () => page.evaluate(() => document.activeElement.dataset.key);
  // Bench ▸ Settings…, View ▸ Tab Properties…, the jump shortcut, and the
  // first-run guide, while Research's menu is open.
  const dialogs = [['settings', '.settings-sheet'], ['edit-tab', '.editor-sheet'], ['toggle-palette', '.palette'], ['first-run', '.guide-sheet']];
  for (const [name, sheet] of dialogs) {
    await page.click('.tab[data-index="2"]', { button: 'right' });
    await page.waitForSelector('#tab-menu');
    await fake.emit('bench:command', { name });
    await page.waitForSelector(sheet);
    assert.equal(await page.$('#tab-menu'), null, name);
    await page.keyboard.press('Escape');
    await page.waitForSelector(sheet, { state: 'detached' });
    assert.equal(await focused(), 'tab-2', name);
  }

  // Comms is deleted elsewhere while Settings is open: the focus goes back
  // to Research, which is the second tab now.
  await page.click('.tab[data-index="2"]', { button: 'right' });
  await page.waitForSelector('#tab-menu');
  await fake.emit('bench:command', { name: 'settings' });
  await page.waitForSelector('.settings-sheet');
  fake.world.tabs.splice(0, 1);
  fake.world.current = 0;
  await fake.emitState();
  await page.waitForSelector('.tab[data-index="1"]:has-text("Research")');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
  assert.equal(await focused(), 'tab-1');

  // And while New tab… is open from Research's menu, Code is deleted.
  await page.click('.tab[data-index="1"]', { button: 'right' });
  await page.click('#tab-menu .menu-item:has-text("New tab…")');
  await page.waitForSelector('.editor-sheet');
  fake.world.tabs.splice(0, 1);
  await fake.emitState();
  await page.waitForSelector('.tab[data-index="0"]:has-text("Research")');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.editor-sheet', { state: 'detached' });
  assert.equal(await focused(), 'tab-0');
});
