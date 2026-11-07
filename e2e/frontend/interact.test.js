'use strict';

// Clicking: tabs, sub-tabs, panes, the split button, the ⋯ menu, a tab's
// menu, the Settings button, and toasts for refused calls.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, openSettings, openTabProperties, settle } = require('./helpers/harness.js');
const { defaultWorld } = require('./helpers/fixture.js');

test('clicking a tab and a sub-tab calls SwitchTab and ShowItem', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('.tab:has-text("Research")');
  const sw = await fake.waitForCall('SwitchTab');
  assert.deepEqual(sw.args, [2]);
  await page.waitForSelector('.tab[data-index="2"][aria-selected="true"]');
  await page.click('.subtab:has-text("Go packages")');
  const show = await fake.waitForCall('ShowItem');
  assert.deepEqual(show.args, ['p5', 'w9']);
  await page.waitForSelector('.subtab[data-item="w9"][aria-selected="true"]');
  // Showing an item focuses its pane, so there's no separate FocusPane.
  assert.equal(fake.callsTo('FocusPane').length, 0);
  // Focus stays on the sub-tab after the redraw.
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'sub-w9');
});

test('clicking a pane focuses it', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('.pane[data-pane-id="p4"] .pane-body');
  const call = await fake.waitForCall('FocusPane');
  assert.deepEqual(call.args, ['p4']);
  await page.waitForSelector('.pane[data-pane-id="p4"][data-focused="true"]');
  assert.equal(await page.getAttribute('.pane[data-pane-id="p3"]', 'data-focused'), 'false');
  // Clicking the focused pane again doesn't call it again.
  await page.click('.pane[data-pane-id="p4"] .pane-body');
  await settle(page);
  assert.equal(fake.callsTo('FocusPane').length, 1);
});

test('the mode buttons call SetMode', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('[data-action="mode"][data-mode="compact"]');
  const call = await fake.waitForCall('SetMode');
  assert.deepEqual(call.args, ['compact']);
  await page.waitForSelector('[data-mode="compact"][aria-checked="true"]', { state: 'attached' });
  // Code stacks on small screens.
  await page.waitForFunction(() => {
    const [a, b] = document.querySelectorAll('.pane');
    return a && b && a.getBoundingClientRect().left === b.getBoundingClientRect().left;
  });
  // In the compact layout, the title bar slims: the layout control is one
  // button that names the layout in use, and opens a menu of the layouts.
  await page.waitForSelector('#titlebar[data-slim="true"]');
  assert.equal(await page.isVisible('#titlebar .layout-seg'), false);
  assert.equal(await page.textContent('[data-action="layout-menu"]'), 'Compact');
  await page.click('[data-action="layout-menu"]');
  await page.waitForSelector('#layout-menu');
  await page.click('#layout-menu [role="menuitemradio"]:has-text("Auto")');
  assert.deepEqual((await fake.waitForCall('SetMode', (a) => a[0] === 'auto')).args, ['auto']);
  await page.waitForSelector('#titlebar[data-slim="false"]');
});

test('the ⋯ menu shows names as they are, even with @NOTE@ in them', async (t) => {
  const world = defaultWorld();
  world.tabs[0].name = 'Docs @NOTE@ x';
  const { page, close } = await openBench({ world });
  t.after(close);
  await page.click('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.waitForSelector('#window-menu');
  await page.click('#window-menu .menu-item:has-text("Move to Tab")');
  await page.waitForSelector('#menu-sub');
  const labels = await page.$$eval('#menu-sub .menu-label', (els) => els.map((el) => el.textContent));
  assert.ok(labels.includes('Docs @NOTE@ x'), 'the tab\'s name is in ' + JSON.stringify(labels));
  // Its disabled entries, Move Left and the current tab, have no note, so
  // none is described by one.
  assert.equal(await page.$('#menu-root [aria-describedby]'), null);
});

test('the split button calls MoveItem with zone right', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('.pane[data-pane-id="p3"] [data-action="split-right"]');
  const call = await fake.waitForCall('MoveItem');
  assert.deepEqual(call.args, ['w3', 1, 'p3', 'right']);
});

test('a refused call shows Go\'s message as a toast', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  fake.on('MoveItem', () => { throw new Error('A pane can\'t be split by its only window.'); });
  await page.click('.pane[data-pane-id="p4"] [data-action="split-right"]');
  await fake.waitForCall('MoveItem');
  await page.waitForFunction(() => document.getElementById('toast').classList.contains('show'));
  assert.equal(await page.textContent('#toast'), 'A pane can\'t be split by its only window.');
  assert.equal(await page.getAttribute('#toast', 'role'), 'status');
  // Go can send a toast too.
  await fake.emit('bench:toast', 'Rearranged Code');
  await page.waitForFunction(() => document.getElementById('toast').textContent === 'Rearranged Code');
});

test('the ⋯ menu entries call the right methods', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  const open = async () => {
    await page.click('.pane[data-pane-id="p3"] [data-action="menu"]');
    await page.waitForSelector('#window-menu');
  };

  await open();
  await fake.waitForCall('SetOverlay', (a) => a[0] === true);
  assert.equal(await page.getAttribute('.pane[data-pane-id="p3"] [data-action="menu"]', 'aria-expanded'), 'true');
  const entries = await page.$$eval('#window-menu > *', (els) => els.map((el) => el.className + ':' + el.textContent));
  assert.deepEqual(entries, [
    // The common actions come first: for an app's window, Close Window.
    'menu-item:Close Window',
    'menu-sep:',
    'menu-item:Split Right⌃⌥⇧→',
    'menu-item:Split Down⌃⌥⇧↓',
    'menu-item:Split Right with an Empty Pane',
    'menu-item:Split Down with an Empty Pane',
    'menu-item:Zoom Pane⌃⌥↩',
    'menu-sep:',
    'menu-item:Move Left',
    'menu-item:Move Right',
    'menu-sep:',
    'menu-item has-sub:Move to Pane',
    'menu-item has-sub:Move to Tab',
    'menu-item has-sub:Also Show In',
    'menu-sep:',
    'menu-item:Pop Out of Bench',
  ]);
  // The menu itself has the focus, not Close Window, so that a second
  // Return can't close the window; arrows move it.
  assert.equal(await page.evaluate(() => document.activeElement.id), 'window-menu');
  assert.equal(await page.getAttribute('#window-menu', 'tabindex'), '-1');
  await page.keyboard.press('Enter');
  await settle(page);
  assert.equal(fake.callsTo('CloseItem').length, 0);
  assert.equal(await page.isVisible('#window-menu'), true);
  await page.keyboard.press('ArrowDown');
  assert.equal(await page.evaluate(() => document.activeElement.textContent), 'Close Window');
  await page.keyboard.press('ArrowDown');
  assert.equal(await page.evaluate(() => document.activeElement.textContent), 'Split Right⌃⌥⇧→');
  await page.keyboard.press('ArrowUp');
  await page.keyboard.press('ArrowUp');
  assert.equal(await page.evaluate(() => document.activeElement.textContent), 'Pop Out of Bench');
  // Escape closes it and puts focus back on the button.
  await page.keyboard.press('Escape');
  await page.waitForSelector('#window-menu', { state: 'detached' });
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'menu-p3');
  await fake.waitForLowered();

  // The tabs stay as they are, for the next entry.
  fake.on('MoveItemToNewTab', () => null);
  // Move to Pane lists the tab's other panes, each named by the window it
  // shows. Move to Tab and Also Show In list every tab, with the tab it's
  // in already off, and New Tab.
  const submenu = async (label) => {
    await page.click('#window-menu .menu-item:has-text("' + label + '")');
    await page.waitForSelector('#menu-sub');
    return page.$$eval('#menu-sub > *', (els) => els.map((el) => el.className + ':' + el.textContent + (el.disabled ? ' (off)' : '')));
  };
  await open();
  assert.deepEqual(await submenu('Move to Pane'), ['menu-item:Pane with Terminal — zsh — ~/code/bench']);
  assert.deepEqual(await submenu('Move to Tab'), [
    // Each with the send modifiers and its number.
    'menu-item:Comms⌃⌥⇧1', 'menu-item:Code⌃⌥⇧2 (off)', 'menu-item:Research⌃⌥⇧3', 'menu-sep:', 'menu-item:New Tab',
  ]);
  assert.deepEqual(await submenu('Also Show In'), [
    'menu-item:Comms', 'menu-item:Code (off)', 'menu-item:Research', 'menu-sep:', 'menu-item:New Tab',
  ]);
  await page.keyboard.press('Escape');
  await page.keyboard.press('Escape');
  await page.waitForSelector('#window-menu', { state: 'detached' });

  // The tabs stay as they are, for the next entry.
  fake.on('MoveItemToNewTab', () => null);
  const cases = [
    ['Split Right', 'MoveItem', ['w3', 1, 'p3', 'right']],
    ['Split Down⌃⌥⇧↓', 'MoveItem', ['w3', 1, 'p3', 'bottom']],
    ['Zoom Pane', 'ZoomPane', ['p3']],
    ['Move to Pane ▸ Pane with Terminal — zsh — ~/code/bench', 'MoveItem', ['w3', 1, 'p4', 'center']],
    ['Move to Tab ▸ Comms', 'MoveItemToTab', ['w3', 0]],
    ['Move to Tab ▸ Research', 'MoveItemToTab', ['w3', 2]],
    ['Also Show In ▸ Comms', 'AddItemToTab', ['w3', 0]],
    ['Also Show In ▸ Research', 'AddItemToTab', ['w3', 2]],
    ['Move to Tab ▸ New Tab', 'MoveItemToNewTab', ['w3', false]],
    ['Also Show In ▸ New Tab', 'MoveItemToNewTab', ['w3', true]],
    ['Pop Out of Bench', 'PopOut', ['w3']],
    ['Close Window', 'CloseItem', ['w3', false, false]],
  ];
  for (const [label, method, args] of cases) {
    fake.clearCalls();
    await open();
    const [parent, entry] = label.split(' ▸ ');
    if (entry) {
      await page.click('#window-menu .menu-item:has-text("' + parent + '")');
      await page.click('#menu-sub .menu-item:has-text("' + entry + '")');
    } else {
      await page.click('#window-menu .menu-item:has-text("' + label + '")');
    }
    const call = await fake.waitForCall(method);
    assert.deepEqual(call.args, args, label);
    await page.waitForSelector('#window-menu', { state: 'detached' });
  }

  // Clicking outside closes it.
  await open();
  await page.click('.statusbar');
  await page.waitForSelector('#window-menu', { state: 'detached' });
});

// subFocus returns the text of the focused entry, and whether it's in the
// submenu.
const subFocus = (page) => page.evaluate(() => [document.activeElement.textContent, Boolean(document.activeElement.closest('#menu-sub'))]);

test('Move to Tab is a submenu: Right Arrow or Return opens it, Left Arrow or Escape closes it', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.focus('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#window-menu');
  const parent = '#window-menu [data-action="menu-sub"]:has-text("Move to Tab")';
  assert.equal(await page.getAttribute(parent, 'role'), 'menuitem');
  assert.equal(await page.getAttribute(parent, 'aria-haspopup'), 'menu');
  assert.equal(await page.getAttribute(parent, 'aria-expanded'), 'false');
  // End, then Up past Pop Out of Bench and Also Show In.
  await page.keyboard.press('End');
  await page.keyboard.press('ArrowUp');
  await page.keyboard.press('ArrowUp');
  assert.deepEqual(await subFocus(page), ['Move to Tab', false]);

  // Right Arrow opens it, with the focus on its first entry that's on.
  await page.keyboard.press('ArrowRight');
  await page.waitForSelector('#menu-sub');
  assert.equal(await page.getAttribute('#menu-sub', 'role'), 'menu');
  assert.equal(await page.getAttribute('#menu-sub', 'aria-labelledby'), 'menu-parent');
  assert.equal(await page.getAttribute(parent, 'id'), 'menu-parent');
  assert.equal(await page.getAttribute(parent, 'aria-expanded'), 'true');
  assert.equal(await page.getAttribute(parent, 'aria-controls'), 'menu-sub');
  assert.deepEqual(await subFocus(page), ['Comms⌃⌥⇧1', true]);
  // The arrows move in the submenu, past the current tab, which is off,
  // and wrap.
  await page.keyboard.press('ArrowDown');
  assert.deepEqual(await subFocus(page), ['Research⌃⌥⇧3', true]);
  await page.keyboard.press('ArrowDown');
  assert.deepEqual(await subFocus(page), ['New Tab', true]);
  await page.keyboard.press('ArrowDown');
  assert.deepEqual(await subFocus(page), ['Comms⌃⌥⇧1', true]);
  // Left Arrow closes it, and gives the focus back to its entry.
  await page.keyboard.press('ArrowLeft');
  await page.waitForSelector('#menu-sub', { state: 'detached' });
  assert.deepEqual(await subFocus(page), ['Move to Tab', false]);
  assert.equal(await page.getAttribute(parent, 'aria-expanded'), 'false');

  // Return opens it too; Escape closes the submenu, then the menu.
  await page.keyboard.press('Enter');
  await page.waitForSelector('#menu-sub');
  assert.deepEqual(await subFocus(page), ['Comms⌃⌥⇧1', true]);
  await page.keyboard.press('Escape');
  await page.waitForSelector('#menu-sub', { state: 'detached' });
  assert.deepEqual(await subFocus(page), ['Move to Tab', false]);
  assert.equal(await page.isVisible('#window-menu'), true);

  // A click opens it without moving the focus into it. Down Arrow then
  // moves on from its entry, and closes it.
  await page.click(parent);
  await page.waitForSelector('#menu-sub');
  assert.deepEqual(await subFocus(page), ['Move to Tab', false]);
  await page.keyboard.press('ArrowDown');
  await page.waitForSelector('#menu-sub', { state: 'detached' });
  assert.deepEqual(await subFocus(page), ['Also Show In', false]);

  // Return on a submenu's entry runs it, and closes the menu.
  fake.clearCalls();
  await page.keyboard.press('ArrowRight');
  await page.waitForSelector('#menu-sub');
  await page.keyboard.press('ArrowDown');
  assert.deepEqual(await subFocus(page), ['Research', true]);
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('AddItemToTab')).args, ['w3', 2]);
  await page.waitForSelector('#window-menu', { state: 'detached' });
  await page.keyboard.press('Escape');
});

test('a submenu opens when the pointer rests on its entry, and stays open on a diagonal move to it', async (t) => {
  const { page, close } = await openBench();
  t.after(close);
  await page.click('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.waitForSelector('#window-menu');
  const centre = async (sel) => {
    const b = await page.locator(sel).boundingBox();
    return { x: b.x + b.width / 2, y: b.y + b.height / 2, box: b };
  };
  const move = await centre('#window-menu [data-action="menu-sub"]:has-text("Move to Tab")');
  const also = await centre('#window-menu [data-action="menu-sub"]:has-text("Also Show In")');
  // Resting on Move to Tab opens it, after a moment, without taking the
  // focus from the menu.
  await page.mouse.move(move.x, move.y, { steps: 2 });
  await page.waitForSelector('#menu-sub');
  assert.equal(await page.getAttribute('#menu-parent', 'aria-expanded'), 'true');
  assert.equal(await page.evaluate(() => Boolean(document.activeElement.closest('#menu-sub'))), false);
  assert.ok((await page.textContent('#menu-sub')).includes('Comms'));

  // Moving straight down to Also Show In switches, after a moment.
  await page.mouse.move(also.x - 60, also.y, { steps: 2 });
  await page.waitForFunction(() => (document.getElementById('menu-parent') || {}).textContent === 'Also Show In');
  await page.mouse.move(move.x - 60, move.y, { steps: 2 });
  await page.waitForFunction(() => (document.getElementById('menu-parent') || {}).textContent === 'Move to Tab');

  // A diagonal move towards the submenu, across Also Show In, keeps
  // Move to Tab's open, and the pointer reaches it.
  const sub = await page.locator('#menu-sub').boundingBox();
  const target = { x: sub.x + 30, y: sub.y + sub.height - 12 };
  await page.mouse.move(target.x, target.y, { steps: 12 });
  await page.waitForTimeout(500);
  assert.equal(await page.textContent('#menu-parent'), 'Move to Tab');
  // The pointer is over the submenu now, which stays open.
  assert.ok(await page.evaluate(({ x, y }) => Boolean(document.elementFromPoint(x, y).closest('#menu-sub')), target));
  await page.mouse.move(target.x, target.y);
  await settle(page, 300);
  assert.equal(await page.textContent('#menu-parent'), 'Move to Tab');

  // Resting on another entry closes it.
  const pop = await centre('#window-menu .menu-item:has-text("Pop Out of Bench")');
  await page.mouse.move(pop.x, pop.y, { steps: 4 });
  await page.waitForSelector('#menu-sub', { state: 'detached' });
  assert.equal(await page.$('#menu-parent'), null);
});

test('a submenu that the pointer only passes over doesn\'t open, and Escape in the menu closes the menu with its submenu', async (t) => {
  const { page, close } = await openBench();
  t.after(close);
  await page.focus('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#window-menu');
  const parent = await page.locator('#window-menu [data-action="menu-sub"]:has-text("Move to Tab")').boundingBox();
  const menu = await page.locator('#window-menu').boundingBox();
  const y = parent.y + parent.height / 2;
  // Resting on Move to Tab for less than the delay, then leaving the menu
  // to the left in one move, doesn't open its submenu.
  await page.mouse.move(parent.x + 40, y, { steps: 2 });
  await page.waitForTimeout(60);
  await page.mouse.move(menu.x - 40, y);
  await page.waitForTimeout(400);
  assert.equal(await page.$('#menu-sub'), null);

  // Resting on it opens it, and the focus stays on the menu. Escape there
  // closes the menu, not only the submenu.
  await page.mouse.move(parent.x + 40, y, { steps: 2 });
  await page.waitForSelector('#menu-sub');
  assert.equal(await page.evaluate(() => document.activeElement.id), 'window-menu');
  await page.keyboard.press('Escape');
  await page.waitForSelector('#window-menu', { state: 'detached' });
  assert.equal(await page.$('#menu-sub'), null);
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'menu-p3');
});

test('a submenu opens beside its entry, on the left where there\'s no room on the right, and inside the window', async (t) => {
  const { page, close } = await openBench();
  t.after(close);
  const placed = async (button) => {
    await page.click(button);
    await page.waitForSelector('#window-menu');
    await page.click('#window-menu [data-action="menu-sub"]:has-text("Move to Tab")');
    await page.waitForSelector('#menu-sub');
    await page.waitForFunction(() => getComputedStyle(document.getElementById('menu-sub')).visibility === 'visible');
    const r = await page.evaluate(() => {
      const box = (el) => { const b = el.getBoundingClientRect(); return { left: b.left, right: b.right, top: b.top, bottom: b.bottom }; };
      const first = document.querySelector('#menu-sub .menu-item');
      return {
        menu: box(document.getElementById('window-menu')), sub: box(document.getElementById('menu-sub')),
        parent: box(document.getElementById('menu-parent')), first: box(first), width: window.innerWidth, height: window.innerHeight,
      };
    });
    await page.keyboard.press('Escape');
    await page.keyboard.press('Escape');
    await page.waitForSelector('#window-menu', { state: 'detached' });
    return r;
  };
  // Code's left pane's ⋯ menu has room on its right.
  const left = await placed('.pane[data-pane-id="p3"] [data-action="menu"]');
  assert.ok(Math.abs(left.sub.left - (left.menu.right - 4)) <= 1, JSON.stringify(left));
  assert.ok(Math.abs(left.first.top - left.parent.top) <= 1, JSON.stringify(left));
  // The right pane's opens at the window's right edge: its submenu goes
  // to the left of the menu.
  const right = await placed('.pane[data-pane-id="p4"] [data-action="menu"]');
  assert.ok(right.sub.right <= right.menu.left + 5 && right.sub.left >= 8, JSON.stringify(right));
  assert.ok(right.sub.bottom <= right.height - 8, JSON.stringify(right));
});

test('right-clicking a sub-tab opens its menu, and shows it only when the menu closes', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  // Showing an App item first would activate its app, which would then
  // get the menu's keys, so it waits.
  await page.click('.subtab:has-text("index.html")', { button: 'right' });
  await page.waitForSelector('#window-menu');
  await fake.waitForCall('SetOverlay', (a) => a[0] === true);
  await settle(page);
  assert.equal(await page.$('#tab-menu'), null, 'a sub-tab isn\'t a tab');
  assert.equal(fake.callsTo('ShowItem').length, 0);
  assert.equal(fake.callsTo('FocusPane').length, 0);
  // Running an entry doesn't show it first: the entry moves it.
  await page.click('#window-menu .menu-item:has-text("Split Down")');
  const move = await fake.waitForCall('MoveItem');
  assert.deepEqual(move.args, ['w4', 1, 'p3', 'bottom']);
  await settle(page);
  assert.equal(fake.callsTo('ShowItem').length, 0);

  // Closing the menu without choosing shows it, as the right-click asked.
  fake.clearCalls();
  await page.click('.subtab:has-text("index.html")', { button: 'right' });
  await page.waitForSelector('#window-menu');
  await page.keyboard.press('Escape');
  await page.waitForSelector('#window-menu', { state: 'detached' });
  const show = await fake.waitForCall('ShowItem');
  assert.deepEqual(show.args, ['p3', 'w4']);
  assert.deepEqual(fake.calls.map((c) => c.method).filter((m) => m === 'SetOverlay' || m === 'ShowItem'), ['SetOverlay', 'SetOverlay', 'ShowItem']);
});

test('closing the menu of a minimised window\'s sub-tab leaves it in the Dock', async (t) => {
  const world = defaultWorld();
  world.items.w3.status = 'minimised';
  world.items.w3.statusText = 'Minimised';
  world.tabs[1].root.children[0].active = 'w4';
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await page.click('.subtab[data-item="w3"]', { button: 'right' });
  await page.waitForSelector('#window-menu');
  await page.keyboard.press('Escape');
  await page.waitForSelector('#window-menu', { state: 'detached' });
  await settle(page);
  // Showing it would bring it back from the Dock: the menu's Show again
  // does that.
  assert.equal(fake.callsTo('ShowItem').length, 0);
  await page.click('.subtab[data-item="w3"]', { button: 'right' });
  await page.click('#window-menu .menu-item:has-text("Show again")');
  assert.deepEqual((await fake.waitForCall('ShowItem')).args, ['p3', 'w3']);
});

test('right-clicking a sub-tab, or the empty part of a pane\'s header, opens the window\'s menu at the pointer', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  const box = (sel) => page.locator(sel).boundingBox();

  // A sub-tab's menu opens where it was clicked, not by the pane's ⋯ button.
  await page.click('.subtab:has-text("index.html")', { button: 'right', position: { x: 12, y: 8 } });
  await page.waitForSelector('#window-menu');
  const sub = await box('.subtab:has-text("index.html")');
  let menu = await box('#window-menu');
  // The pointer's position is in whole pixels.
  assert.ok(Math.abs(menu.x - (sub.x + 12)) <= 1 && Math.abs(menu.y - (sub.y + 8)) <= 1, JSON.stringify({ sub, menu }));
  await page.keyboard.press('Escape');
  await page.waitForSelector('#window-menu', { state: 'detached' });

  // The empty part of a header: the menu of the window the pane shows, at
  // the pointer. Code's first pane, p3, shows VS Code now, as the sub-tab's
  // menu closed without choosing; focus the other.
  fake.world.tabs[1].focus = 'p4';
  await fake.emitState();
  await page.waitForSelector('.pane[data-pane-id="p4"][data-focused="true"]');
  fake.clearCalls();
  const add = await box('.pane[data-pane-id="p3"] [data-action="add"]');
  const actions = await box('.pane[data-pane-id="p3"] .pane-actions');
  const x = Math.round((add.x + add.width + actions.x) / 2);
  const y = Math.round(add.y + add.height / 2);
  assert.ok(actions.x - (add.x + add.width) > 40, 'room between + and the actions');
  await page.mouse.click(x, y, { button: 'right' });
  await page.waitForSelector('#window-menu');
  menu = await box('#window-menu');
  assert.deepEqual([Math.round(menu.x), Math.round(menu.y)], [x, y]);
  assert.equal(await page.getAttribute('.pane[data-pane-id="p3"] [data-action="menu"]', 'aria-expanded'), 'true');
  // It's the shown window's.
  await page.click('#window-menu .menu-item:has-text("Split Down")');
  assert.deepEqual((await fake.waitForCall('MoveItem')).args, ['w4', 1, 'p3', 'bottom']);
  await page.waitForSelector('#window-menu', { state: 'detached' });

  // Closing it without choosing focuses the pane, as a click in it would.
  fake.clearCalls();
  await page.mouse.click(x, y, { button: 'right' });
  await page.waitForSelector('#window-menu');
  await settle(page);
  assert.equal(fake.callsTo('FocusPane').length, 0);
  await page.keyboard.press('Escape');
  await page.waitForSelector('#window-menu', { state: 'detached' });
  assert.deepEqual((await fake.waitForCall('FocusPane')).args, ['p3']);

  // Near the right edge of the window, on the badge of the last pane, it
  // stays inside the window.
  const win = await box('#app-window');
  const badge = await box('.pane[data-pane-id="p4"] .badge');
  await page.mouse.click(badge.x + badge.width / 2, badge.y + badge.height / 2, { button: 'right' });
  await page.waitForSelector('#window-menu');
  menu = await box('#window-menu');
  assert.ok(menu.x + menu.width <= win.x + win.width - 8, 'inside the window: ' + JSON.stringify({ menu, win }));
  await page.keyboard.press('Escape');
  await page.waitForSelector('#window-menu', { state: 'detached' });

  // Right-clicking a button in the header opens nothing.
  await page.click('.pane[data-pane-id="p4"] [data-action="add"]', { button: 'right' });
  await settle(page);
  assert.equal(await page.$('#menu-root .menu'), null);
});

test('right-clicking an empty pane opens its menu, to add a window or close the pane', async (t) => {
  const world = defaultWorld();
  world.tabs[1].root.children[1].items = [];
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  const box = (sel) => page.locator(sel).boundingBox();
  const entries = () => page.$$eval('#pane-menu .menu-item', (els) => els.map((el) => el.textContent));

  // Its header, at the pointer: what the buttons in its body do.
  const add = await box('.pane[data-pane-id="p4"] .pane-header [data-action="add"]');
  const x = Math.round(add.x + add.width + 40);
  const y = Math.round(add.y + add.height / 2);
  await page.mouse.click(x, y, { button: 'right' });
  await page.waitForSelector('#pane-menu');
  const menu = await box('#pane-menu');
  assert.deepEqual([Math.round(menu.x), Math.round(menu.y)], [x, y]);
  assert.deepEqual(await entries(), ['Add to Pane…', 'Close Pane', 'Zoom Pane⌃⌥↩']);
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'menu-item-0');
  // Closing it without choosing focuses the pane, as a click in it would.
  fake.clearCalls();
  await page.keyboard.press('Escape');
  await page.waitForSelector('#pane-menu', { state: 'detached' });
  assert.deepEqual((await fake.waitForCall('FocusPane')).args, ['p4']);

  // Its body too. Add to Pane… opens the add dialog for the pane.
  const body = await box('.pane[data-pane-id="p4"] .pane-body');
  await page.mouse.click(body.x + 20, body.y + 20, { button: 'right' });
  await page.waitForSelector('#pane-menu');
  await page.click('#pane-menu .menu-item:has-text("Add to Pane")');
  assert.deepEqual((await fake.waitForCall('AddOptions')).args, ['p4']);
  await page.waitForSelector('.palette');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.palette', { state: 'detached' });

  // Close Pane closes it.
  await page.mouse.click(x, y, { button: 'right' });
  await page.waitForSelector('#pane-menu');
  await page.click('#pane-menu .menu-item:has-text("Close Pane")');
  assert.deepEqual((await fake.waitForCall('ClosePane')).args, ['p4']);

  // Right-clicking a button in it opens nothing.
  await page.click('.pane[data-pane-id="p4"] .pane-body [data-action="add"]', { button: 'right' });
  await settle(page);
  assert.equal(await page.$('#menu-root .menu'), null);

  // A tab's only pane can't close.
  fake.world.tabs[1].root = fake.world.tabs[1].root.children[1];
  fake.world.tabs[1].focus = 'p4';
  await fake.emitState();
  await page.waitForSelector('.pane[data-pane-id="p3"]', { state: 'detached' });
  await page.mouse.click(x, y, { button: 'right' });
  await page.waitForSelector('#pane-menu');
  assert.deepEqual(await entries(), ['Add to Pane…']);

  // The menu goes when the pane gets a window.
  fake.world.tabs[1].root.items = ['w5'];
  fake.world.tabs[1].root.active = 'w5';
  await fake.emitState();
  await page.waitForSelector('#pane-menu', { state: 'detached' });
  assert.deepEqual(fake.errors, []);
});

test('+ and ⋯ in an unfocused pane don\'t focus it until their dialog or menu closes', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  // Code: p3 (Zed) is focused, so focus p4 first.
  fake.world.tabs[1].focus = 'p4';
  await fake.emitState();
  await page.waitForSelector('.pane[data-pane-id="p4"][data-focused="true"]');
  fake.clearCalls();

  // + opens the add dialog for p3 without focusing p3, which would
  // activate Zed and send it the keys typed into the search field.
  await page.click('.pane[data-pane-id="p3"] [data-action="add"]');
  await page.waitForSelector('.palette');
  assert.deepEqual((await fake.waitForCall('AddOptions')).args, ['p3']);
  await page.keyboard.type('term');
  await settle(page);
  assert.equal(fake.callsTo('FocusPane').length, 0);
  // Adding focuses the pane by itself (in Go), so nothing else does. The
  // first row starts the terminal where the focused pane's terminal is.
  await page.keyboard.press('Enter');
  const add = await fake.waitForCall('AddToPane');
  assert.deepEqual(add.args, ['p3', { type: 'terminal', folder: '~/code/bench' }]);
  await settle(page);
  assert.equal(fake.callsTo('FocusPane').length, 0);

  // Cancelling focuses the pane, as the click would have.
  fake.clearCalls();
  await page.click('.pane[data-pane-id="p3"] [data-action="add"]');
  await page.waitForSelector('.palette');
  await page.keyboard.press('Escape');
  assert.deepEqual((await fake.waitForCall('FocusPane')).args, ['p3']);
  assert.deepEqual(fake.calls.map((c) => c.method).filter((m) => m === 'AddOptions' || m === 'FocusPane'), ['AddOptions', 'FocusPane']);
  await page.waitForSelector('.pane[data-pane-id="p3"][data-focused="true"]');

  // The same for the ⋯ menu, in p4 now.
  fake.clearCalls();
  await page.click('.pane[data-pane-id="p4"] [data-action="menu"]');
  await page.waitForSelector('#window-menu');
  await page.keyboard.press('ArrowDown');
  await settle(page);
  assert.equal(fake.callsTo('FocusPane').length, 0);
  await page.keyboard.press('Escape');
  assert.deepEqual((await fake.waitForCall('FocusPane')).args, ['p4']);
  await page.waitForSelector('.pane[data-pane-id="p4"][data-focused="true"]');

  // Choosing a menu entry leaves the focusing to Go.
  fake.clearCalls();
  await page.click('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.click('#window-menu .menu-item:has-text("Pop Out of Bench")');
  await fake.waitForCall('PopOut');
  await settle(page);
  assert.equal(fake.callsTo('FocusPane').length, 0);

  // A click in another pane while the menu is open does what that click
  // does: p4 is focused already, so nothing is focused.
  fake.clearCalls();
  await page.click('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.waitForSelector('#window-menu');
  await page.click('.pane[data-pane-id="p4"] .pane-body');
  await page.waitForSelector('#window-menu', { state: 'detached' });
  await settle(page);
  assert.deepEqual(fake.callsTo('FocusPane').map((c) => c.args), []);

  // Showing the window side by side focuses its new pane, in Go, so the
  // click doesn't focus the pane it was in as well: Go could handle that
  // after the move, and focus the pane left behind.
  fake.clearCalls();
  await page.click('.pane[data-pane-id="p3"] [data-action="split-right"]');
  await fake.waitForCall('MoveItem');
  await settle(page);
  assert.deepEqual(fake.calls.map((c) => c.method).filter((m) => m === 'FocusPane' || m === 'MoveItem'), ['MoveItem']);

  // The rest of the pane still focuses it.
  fake.clearCalls();
  await page.click('.pane[data-pane-id="p3"] .pane-body');
  assert.deepEqual((await fake.waitForCall('FocusPane')).args, ['p3']);
});

test('a split that Go refuses focuses the pane clicked in', async (t) => {
  const { page, fake, close } = await openBench({ setup: (f) => {
    f.on('MoveItem', () => { throw new Error('That\'s the only window in its pane.'); });
  } });
  t.after(close);
  fake.world.tabs[1].root.children[1].items = ['w5'];
  fake.world.tabs[1].root.children[1].active = 'w5';
  await fake.emitState();
  await page.waitForSelector('.pane[data-pane-id="p4"] .subtab:only-child');
  fake.clearCalls();
  await page.click('.pane[data-pane-id="p4"] [data-action="split-right"]');
  assert.deepEqual((await fake.waitForCall('FocusPane')).args, ['p4']);
  assert.deepEqual(fake.calls.map((c) => c.method).filter((m) => m === 'FocusPane' || m === 'MoveItem'), ['MoveItem', 'FocusPane']);
  await page.waitForFunction(() => document.getElementById('toast').textContent === 'That\'s the only window in its pane.');
});

test('opening + or ⋯ from the keyboard leaves the pane unfocused', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  fake.world.tabs[1].focus = 'p4';
  await fake.emitState();
  await page.waitForSelector('.pane[data-pane-id="p4"][data-focused="true"]');
  fake.clearCalls();
  await page.focus('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#window-menu');
  await page.keyboard.press('Escape');
  await page.waitForSelector('#window-menu', { state: 'detached' });
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'menu-p3');
  await settle(page);
  assert.equal(fake.callsTo('FocusPane').length, 0);
});

test('when another app takes the keyboard, an open menu closes, but a dialog stays', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  fake.world.tabs[1].focus = 'p4';
  await fake.emitState();
  await page.waitForSelector('.pane[data-pane-id="p4"][data-focused="true"]');
  await page.click('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.waitForSelector('#window-menu');
  await fake.waitForCall('SetOverlay', (a) => a[0] === true);
  fake.clearCalls();
  await page.evaluate(() => window.dispatchEvent(new FocusEvent('blur')));
  await page.waitForSelector('#window-menu', { state: 'detached' });
  await fake.waitForCall('SetOverlay', (a) => a[0] === false);
  // Closing it this way doesn't focus the pane, which would bring Bench
  // and Zed back in front of the other app.
  await settle(page);
  assert.equal(fake.callsTo('FocusPane').length, 0);

  await page.click('[data-action="open-palette"]');
  await page.waitForSelector('.palette');
  await page.evaluate(() => window.dispatchEvent(new FocusEvent('blur')));
  await settle(page);
  assert.ok(await page.$('.palette'), 'the palette stays open');
});

test('Tab in a menu closes it and moves on from its button', async (t) => {
  const { page, close } = await openBench();
  t.after(close);
  await page.focus('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#window-menu');
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'window-menu');
  await page.keyboard.press('Tab');
  await page.waitForSelector('#window-menu', { state: 'detached' });
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'sub-w5');
  await page.focus('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#window-menu');
  await page.keyboard.press('Shift+Tab');
  await page.waitForSelector('#window-menu', { state: 'detached' });
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'split-p3');
});

test('a menu or the add dialog closes when Go switches tabs', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.waitForSelector('#window-menu');
  await fake.waitForCall('SetOverlay', (a) => a[0] === true);
  // A global shortcut switches to Research.
  fake.world.current = 2;
  await fake.emitState();
  await page.waitForSelector('#window-menu', { state: 'detached' });
  await fake.waitForLowered();

  fake.world.current = 1;
  await fake.emitState();
  await page.waitForSelector('.tab[data-index="1"][aria-selected="true"]');
  fake.clearCalls();
  await page.click('.pane[data-pane-id="p4"] [data-action="add"]');
  await page.waitForSelector('.palette');
  await fake.waitForCall('SetOverlay', (a) => a[0] === true);
  // Focus following switches to Comms.
  fake.world.current = 0;
  await fake.emitState();
  await page.waitForSelector('.palette', { state: 'detached' });
  await fake.waitForLowered();
  await settle(page);
  assert.equal(fake.callsTo('AddToPane').length, 0);
  assert.equal(fake.callsTo('FocusPane').length, 0);
});

test('a menu closes when its window leaves the pane', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.waitForSelector('#window-menu');
  const [p3, p4] = fake.world.tabs[1].root.children;
  p3.items = ['w4'];
  p3.active = 'w4';
  p4.items.push('w3');
  await fake.emitState();
  await page.waitForSelector('#window-menu', { state: 'detached' });
});

test('menus and dialogs raise Bench while any is open', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  // The first render says Bench isn't raised, in case a page before this
  // one left it raised.
  assert.deepEqual(fake.callsTo('SetOverlay').map((c) => c.args), [[false]]);
  fake.clearCalls();
  const overlays = () => fake.callsTo('SetOverlay').map((c) => c.args[0]);
  await page.click('[data-action="open-palette"]');
  await page.waitForSelector('.palette');
  await page.keyboard.press('Escape');
  // A tab's menu hands Bench on to the Tab properties it opens without
  // lowering it in between, which would rearrange the docked windows in
  // front of it.
  await openTabProperties(page);
  await page.click('[data-key="editor-cancel"]');
  await openSettings(page);
  await page.click('[data-key="settings-cancel"]');
  await page.click('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.click('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.waitForSelector('#window-menu', { state: 'detached' });
  await settle(page);
  assert.deepEqual(overlays(), [true, false, true, false, true, false, true, false]);
  // Escape restored focus to the palette's opener.
  await page.click('[data-action="open-palette"]');
  await page.keyboard.press('Escape');
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'open-palette');
});

test('a menu whose window closes goes away, and Bench is lowered again', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.waitForSelector('#window-menu');
  await fake.waitForCall('SetOverlay', (a) => a[0] === true);
  const p3 = fake.world.tabs[1].root.children[0];
  p3.items = ['w4'];
  p3.active = 'w4';
  delete fake.world.items.w3;
  await fake.emitState();
  await page.waitForSelector('#window-menu', { state: 'detached' });
  await fake.waitForLowered();
  // Focus was in the menu, so it goes back to the menu button.
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'menu-p3');
});

test('another control that closes the ⋯ menu stops it focusing its pane later', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ notices: ['⌃⌥Space is taken by another app.'] }) });
  t.after(close);
  fake.world.tabs[1].focus = 'p4';
  await fake.emitState();
  await page.waitForSelector('.pane[data-pane-id="p4"][data-focused="true"]');
  fake.clearCalls();
  // The notices replace Zed's menu: closing them doesn't focus Zed's pane.
  await page.click('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.waitForSelector('#window-menu');
  await page.click('[data-action="notices"]');
  await page.waitForSelector('#notices-menu');
  await page.keyboard.press('Escape');
  await page.waitForSelector('#notices-menu', { state: 'detached' });
  await settle(page);
  assert.equal(fake.callsTo('FocusPane').length, 0);
});

test('in the browser preview, a shortcut that closes the ⋯ menu doesn\'t focus its pane first', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ demo: true }) });
  t.after(close);
  fake.world.tabs[1].focus = 'p4';
  await fake.emitState();
  await page.waitForSelector('.pane[data-pane-id="p4"][data-focused="true"]');
  fake.clearCalls();
  await page.click('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.waitForSelector('#window-menu');
  // Focusing p3 as well would race the shortcut's own call in Go.
  await page.keyboard.press('Control+Alt+0');
  await fake.waitForCall('Rearrange');
  await page.waitForSelector('#window-menu', { state: 'detached' });
  await settle(page);
  assert.equal(fake.callsTo('FocusPane').length, 0);
});

test('right-clicking a tab opens its menu at the pointer, without switching tabs', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  fake.clearCalls();
  // Code is the current tab; the menu is for Research.
  await page.click('.tab[data-index="2"]', { button: 'right', position: { x: 30, y: 12 } });
  await page.waitForSelector('#tab-menu');
  await fake.waitForCall('SetOverlay', (a) => a[0] === true);
  assert.equal(await page.getAttribute('#tab-menu', 'role'), 'menu');
  assert.equal(await page.getAttribute('#tab-menu', 'aria-label'), 'Options for the Research tab');
  const entries = await page.$$eval('#tab-menu > *', (els) => els.map((el) => el.className + ':' + el.textContent));
  assert.deepEqual(entries, [
    'menu-item:Tab Properties…', 'menu-item:Rename…', 'menu-sep:',
    'menu-item:Move Tab Left', 'menu-item:Move Tab Right', 'menu-sep:',
    'menu-item:Rearrange', 'menu-item:Reset Layout', 'menu-sep:',
    'menu-item:Close Tab…', 'menu-item:Delete Tab (Keep Windows)', 'menu-sep:',
    'menu-item:New Tab…',
  ]);
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'menu-item-0');
  // Its top-left corner is at the pointer.
  const tab = await page.locator('.tab[data-index="2"]').boundingBox();
  const menu = await page.locator('#tab-menu').boundingBox();
  // The pointer's position is in whole pixels.
  assert.ok(Math.abs(menu.x - (tab.x + 30)) <= 1 && Math.abs(menu.y - (tab.y + 12)) <= 1, JSON.stringify({ tab, menu }));
  await settle(page);
  assert.equal(fake.callsTo('SwitchTab').length, 0);
  assert.equal(await page.getAttribute('.tab[data-index="1"]', 'aria-selected'), 'true');

  // Clicking outside closes it without running anything, and lowers Bench.
  await page.click('.statusbar');
  await page.waitForSelector('#tab-menu', { state: 'detached' });
  await fake.waitForLowered();
  await settle(page);
  assert.equal(await page.$('.overlay'), null);
  assert.deepEqual(fake.calls.map((c) => c.method).filter((m) => m !== 'SetOverlay'), []);
});

test('a tab\'s menu renames, rearranges, and resets its tab', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  const choose = async (index, label) => {
    await page.click('.tab[data-index="' + index + '"]', { button: 'right' });
    await page.waitForSelector('#tab-menu');
    await page.click('#tab-menu .menu-item:has-text("' + label + '")');
  };

  // Rename… turns the tab's name into a field, in place, with the name
  // selected to type over. Return saves it, keeping the tab's layout as it
  // is and its small-screen setting.
  await choose(2, 'Rename…');
  await page.waitForSelector('.tab.renaming[data-index="2"] #rename-input');
  assert.equal(await page.$('#tab-menu'), null);
  assert.equal(await page.$('.editor-sheet'), null);
  assert.equal(await page.evaluate(() => document.activeElement.id), 'rename-input');
  assert.deepEqual(await page.evaluate(() => {
    const el = document.activeElement;
    return [el.value, el.selectionStart, el.selectionEnd];
  }), ['Research', 0, 'Research'.length]);
  await page.keyboard.type('Reading');
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('SaveTab')).args, [2, { name: 'Reading', preset: 'custom', smallScreens: 'tabs' }]);
  await page.waitForSelector('#rename-input', { state: 'detached' });

  // The current tab's Rearrange has its shortcut, and rearranges it.
  fake.clearCalls();
  await page.click('.tab[data-index="1"]', { button: 'right' });
  await page.waitForSelector('#tab-menu');
  assert.equal(await page.textContent('#tab-menu .menu-item:has-text("Rearrange")'), 'Rearrange⌃⌥0');
  await page.click('#tab-menu .menu-item:has-text("Rearrange")');
  await fake.waitForCall('Rearrange');
  await settle(page);
  assert.equal(fake.callsTo('SwitchTab').length, 0);

  // Another tab's switches to it first, as only the current tab is arranged.
  fake.clearCalls();
  await choose(0, 'Rearrange');
  await fake.waitForCall('Rearrange');
  assert.deepEqual(fake.calls.map((c) => c.method).filter((m) => m === 'SwitchTab' || m === 'Rearrange'), ['SwitchTab', 'Rearrange']);
  assert.deepEqual(fake.callsTo('SwitchTab')[0].args, [0]);

  // Reset Layout resets that tab, without asking first or switching.
  fake.clearCalls();
  await choose(2, 'Reset Layout');
  assert.deepEqual((await fake.waitForCall('ResetLayout')).args, [2]);
  await page.waitForSelector('#tab-menu', { state: 'detached' });
  await settle(page);
  assert.equal(await page.$('.overlay'), null);
  assert.equal(fake.callsTo('SwitchTab').length, 0);
});

test('a tab\'s menu asked for from the keyboard opens below the tab', async (t) => {
  const { page, close } = await openBench();
  t.after(close);
  await page.dispatchEvent('.tab[data-index="2"]', 'contextmenu', { clientX: 0, clientY: 0 });
  await page.waitForSelector('#tab-menu');
  const tab = await page.locator('.tab[data-index="2"]').boundingBox();
  const menu = await page.locator('#tab-menu').boundingBox();
  assert.equal(Math.round(menu.x), Math.round(tab.x));
  assert.ok(menu.y >= tab.y + tab.height && menu.y <= tab.y + tab.height + 8, 'below the tab: ' + JSON.stringify({ tab, menu }));
});

test('clicking Tab Properties… in a tab\'s menu opens that tab\'s properties', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  const focused = () => page.evaluate(() => document.activeElement.dataset.key);
  // Research isn't the current tab.
  await page.click('.tab[data-index="2"]', { button: 'right' });
  await page.click('#tab-menu .menu-item:has-text("Tab Properties…")');
  await page.waitForSelector('.editor-sheet');
  assert.equal(await page.$('#tab-menu'), null);
  assert.equal(await page.textContent('#editor-title'), 'Tab properties');
  assert.equal(await page.inputValue('#tab-name'), 'Research');
  assert.deepEqual((await fake.waitForCall('PreviewTree')).args, [2, 'side']);
  // Cancel gives the focus back to the tab.
  await page.click('[data-key="editor-cancel"]');
  await page.waitForSelector('.editor-sheet', { state: 'detached' });
  assert.equal(await focused(), 'tab-2');

  // Save sends that tab's index, and none of the settings for every tab.
  await openTabProperties(page, 2);
  await page.fill('#tab-name', 'Reading');
  await page.click('[data-action="save-tab"]');
  assert.deepEqual((await fake.waitForCall('SaveTab')).args, [2, { name: 'Reading', preset: 'side', smallScreens: 'tabs' }]);
  await page.waitForSelector('.editor-sheet', { state: 'detached' });
  assert.equal(await focused(), 'tab-2');
  await settle(page);
  assert.equal(fake.callsTo('SwitchTab').length, 0);
  assert.equal(fake.callsTo('SetOpenAtLogin').length, 0);
});

test('right-clicking another tab while a tab\'s menu is open opens that tab\'s menu instead', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('.tab[data-index="2"]', { button: 'right' });
  await page.waitForSelector('#tab-menu[aria-label="Options for the Research tab"]');
  await page.click('.tab[data-index="0"]', { button: 'right', position: { x: 20, y: 10 } });
  await page.waitForSelector('#tab-menu[aria-label="Options for the Comms tab"]');
  assert.equal(await page.locator('#menu-root .menu').count(), 1);
  const tab = await page.locator('.tab[data-index="0"]').boundingBox();
  const menu = await page.locator('#tab-menu').boundingBox();
  assert.ok(Math.abs(menu.x - (tab.x + 20)) <= 1, JSON.stringify({ tab, menu }));
  await page.click('#tab-menu .menu-item:has-text("Tab Properties…")');
  await page.waitForSelector('.editor-sheet');
  assert.equal(await page.inputValue('#tab-name'), 'Comms');
  assert.deepEqual(fake.callsTo('PreviewTree').map((c) => c.args), [[0, 'side']]);
});

test('a tab\'s menu that replaces a window\'s menu stops that one focusing its pane or showing its window later', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  fake.world.tabs[1].focus = 'p4';
  await fake.emitState();
  await page.waitForSelector('.pane[data-pane-id="p4"][data-focused="true"]');
  const overlays = () => fake.callsTo('SetOverlay').map((c) => c.args[0]);
  fake.clearCalls();
  // Zed's ⋯ menu, in the unfocused pane.
  await page.click('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.waitForSelector('#window-menu');
  await page.click('.tab[data-index="2"]', { button: 'right' });
  await page.waitForSelector('#tab-menu');
  assert.equal(await page.$('#window-menu'), null);
  assert.equal(await page.getAttribute('.pane[data-pane-id="p3"] [data-action="menu"]', 'aria-expanded'), 'false');
  await page.keyboard.press('Escape');
  await page.waitForSelector('#tab-menu', { state: 'detached' });
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'tab-2');
  await settle(page);
  assert.equal(fake.callsTo('FocusPane').length, 0);
  // Bench stayed raised from one menu to the other.
  assert.deepEqual(overlays(), [true, false]);

  // A right-clicked sub-tab's menu, whose window shows when it closes.
  fake.clearCalls();
  await page.click('.subtab:has-text("index.html")', { button: 'right' });
  await page.waitForSelector('#window-menu');
  await page.click('.tab[data-index="0"]', { button: 'right' });
  await page.waitForSelector('#tab-menu');
  await page.keyboard.press('Escape');
  await page.waitForSelector('#tab-menu', { state: 'detached' });
  await settle(page);
  assert.deepEqual(fake.calls.map((c) => c.method).filter((m) => m === 'ShowItem' || m === 'FocusPane'), []);
});

test('a tab\'s menu follows its tab when a tab before it is deleted, and closes when its tab is', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('.tab[data-index="2"]', { button: 'right' });
  await page.waitForSelector('#tab-menu');
  // Comms is deleted elsewhere, such as in config.toml: Research is the
  // second tab now.
  fake.world.tabs.splice(0, 1);
  fake.world.current = 0;
  await fake.emitState();
  await page.waitForSelector('.tab[data-index="1"]:has-text("Research")');
  assert.equal(await page.getAttribute('#tab-menu', 'aria-label'), 'Options for the Research tab');
  await page.click('#tab-menu .menu-item:has-text("Tab Properties…")');
  await page.waitForSelector('.editor-sheet');
  assert.equal(await page.inputValue('#tab-name'), 'Research');
  assert.deepEqual((await fake.waitForCall('PreviewTree')).args, [1, 'side']);
  await page.keyboard.press('Escape');
  await page.waitForSelector('.editor-sheet', { state: 'detached' });

  // Deleting Research closes its menu, and lowers Bench.
  await page.click('.tab[data-index="1"]', { button: 'right' });
  await page.waitForSelector('#tab-menu');
  await fake.waitForCall('SetOverlay', (a) => a[0] === true);
  fake.clearCalls();
  fake.world.tabs.splice(1, 1);
  await fake.emitState();
  await page.waitForSelector('#tab-menu', { state: 'detached' });
  await fake.waitForCall('SetOverlay', (a) => a[0] === false);
  await settle(page);
  assert.equal(await page.$('.overlay'), null);
  // Focus was in the menu, and its tab's button has gone, so it goes to
  // the current tab.
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'tab-0');
});

test('the title bar\'s gear opens Settings', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  // The button that edited the current tab's layout has gone: each tab's
  // properties are in its menu.
  assert.equal(await page.$('[data-action="edit-tab"]'), null);
  const gear = page.locator('#titlebar [data-action="open-settings"]');
  assert.equal(await gear.getAttribute('aria-label'), 'Settings');
  assert.equal(await gear.getAttribute('title'), 'Settings');
  fake.clearCalls();
  await gear.click();
  await page.waitForSelector('.settings-sheet');
  await fake.waitForCall('SetOverlay', (a) => a[0] === true);
  assert.equal(await page.textContent('#settings-title'), 'Settings');
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'settings-section-general');
  // Escape closes it without saving, and gives the focus back to the gear.
  await page.keyboard.press('Escape');
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
  await fake.waitForLowered();
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'open-settings');
  assert.equal(fake.callsTo('SaveSettings').length, 0);
});

test('a window\'s menu starts with why it isn\'t ready, as it was when the menu opened', async (t) => {
  const world = defaultWorld();
  world.items.w3.status = 'needs-room';
  world.items.w3.statusText = 'Zed\'s window can\'t shrink to fit this pane. Try the compact layout';
  world.items.w4.status = 'full-screen';
  world.items.w4.statusText = 'In full screen';
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  const firstLine = () => page.$eval('#window-menu', (el) => {
    const first = el.firstElementChild;
    const mark = first.querySelector('.status-mark');
    return [first.className, first.textContent, mark ? mark.dataset.marker : '', el.getAttribute('aria-describedby')];
  });

  // The ⋯ menu of the window shown.
  await page.click('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.waitForSelector('#window-menu');
  assert.deepEqual(await firstLine(), ['menu-status', 'Zed\'s window can\'t shrink to fit this pane. Try the compact layout.', 'problem', 'menu-status']);
  // It isn't an entry: the menu, which it describes, has the focus, and
  // Down Arrow goes to the first entry.
  assert.equal(await page.evaluate(() => document.activeElement.id), 'window-menu');
  await page.keyboard.press('ArrowDown');
  assert.equal(await page.evaluate(() => document.activeElement.textContent), 'Close Window');
  // If the window becomes ready, the line stays, so the entries don't move.
  fake.world.items.w3.status = 'ready';
  await fake.emitState();
  await page.waitForFunction(() => !document.querySelector('.subtab[data-item="w3"]').dataset.status);
  assert.equal((await firstLine())[0], 'menu-status');
  await page.keyboard.press('Escape');
  await page.waitForSelector('#window-menu', { state: 'detached' });

  // A sub-tab's right-click menu, for a window that isn't shown.
  await page.click('.subtab[data-item="w4"]', { button: 'right' });
  await page.waitForSelector('#window-menu');
  assert.deepEqual(await firstLine(), ['menu-status', 'In full screen.', 'info', 'menu-status']);
  await page.keyboard.press('Escape');
  await page.waitForSelector('#window-menu', { state: 'detached' });

  // A window that's ready has none.
  await page.click('.pane[data-pane-id="p4"] [data-action="menu"]');
  await page.waitForSelector('#window-menu');
  assert.equal(await page.$('#window-menu .menu-status'), null);
  assert.equal(await page.getAttribute('#window-menu', 'aria-describedby'), null);
});

test('the note of the windows that need attention lists them, and choosing one shows it', async (t) => {
  const world = defaultWorld();
  world.items.w4.status = 'undocked';
  world.items.w4.statusText = 'Moved out of its pane';
  world.items.w6.status = 'needs-room';
  world.items.w6.statusText = 'Terminal\'s window can\'t shrink to fit this pane';
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  const note = page.locator('#statusbar [data-action="attention"]');
  await note.click();
  await page.waitForSelector('#attention-menu');
  await fake.waitForCall('SetOverlay', (a) => a[0] === true);
  assert.equal(await note.getAttribute('aria-expanded'), 'true');
  const rows = await page.$$eval('#attention-menu .menu-item', (els) => els.map((el) => [
    el.querySelector('.menu-main').textContent, el.querySelector('.menu-detail').textContent, el.querySelector('.status-mark').dataset.marker,
  ]));
  assert.deepEqual(rows, [
    ['VS Code — index.html — bench-ui', 'Moved out of its pane.', 'problem'],
    ['Terminal — go test — ~/code/bench', 'Terminal\'s window can\'t shrink to fit this pane.', 'problem'],
  ]);
  assert.equal(await page.getAttribute('#attention-menu .menu-item >> nth=0', 'aria-label'), 'VS Code — index.html — bench-ui: Moved out of its pane.');
  // The keyboard moves through the list, and Return shows the window.
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'menu-item-0');
  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#attention-menu', { state: 'detached' });
  assert.deepEqual((await fake.waitForCall('ShowItem')).args, ['p4', 'w6']);

  // Escape closes it, and gives the focus back to the note.
  await note.click();
  await page.waitForSelector('#attention-menu');
  await page.keyboard.press('Escape');
  await page.waitForSelector('#attention-menu', { state: 'detached' });
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'attention');

  // The list closes when the windows that need attention change, so that
  // its entries don't move under the pointer.
  await note.click();
  await page.waitForSelector('#attention-menu');
  fake.world.items.w4.status = 'ready';
  await fake.emitState();
  await page.waitForSelector('#attention-menu', { state: 'detached' });
  assert.equal(await note.textContent(), '1 window needs attention');
});

test('controls react to the pointer, the chosen ones stand out, and icon buttons have tooltips', async (t) => {
  const world = defaultWorld();
  world.tabs[2].root.children[1].items.push('w3');
  const { page, close } = await openBench({ world });
  t.after(close);
  const titles = await page.evaluate(() => {
    const title = (sel) => document.querySelector(sel).getAttribute('title');
    return [
      title('#titlebar [data-action="new-tab"]'), title('#titlebar [data-action="open-settings"]'),
      title('[data-action="add"][data-pane="p3"]'), title('[data-action="split-right"][data-pane="p3"]'),
      title('[data-action="menu"][data-pane="p3"]'), title('.tab[data-index="1"]'), title('.subtab[data-item="w3"]'),
    ];
  });
  assert.deepEqual(titles, [
    'New tab', 'Settings', 'Add a window or app to this pane', 'Show this window side by side (⌃⌥⇧→)', 'Window options',
    'Code (⌃⌥2)', 'Zed — bench — switch.go\nAlso in Research',
  ]);
  // Every icon-only button has one.
  assert.deepEqual(await page.$$eval('button.icon-btn', (els) => els.filter((el) => !el.title).length), 0);

  // A control under the pointer changes its background, and so does one
  // being pressed.
  const background = (sel) => page.$eval(sel, (el) => getComputedStyle(el).backgroundColor);
  for (const sel of ['.tab[data-index="0"]', '.subtab[data-item="w4"]', '#titlebar [data-action="new-tab"]', '[data-action="menu"][data-pane="p4"]',
    '[data-action="mode"][data-mode="wide"]', '[data-action="open-palette"]']) {
    await page.mouse.move(0, 400);
    const before = await background(sel);
    await page.hover(sel);
    const hovered = await background(sel);
    assert.notEqual(hovered, before, sel + ' changes under the pointer');
    await page.mouse.down();
    assert.notEqual(await background(sel), hovered, sel + ' changes when pressed');
    await page.mouse.move(0, 400);
    await page.mouse.up();
  }

  // The chosen layout and the window shown in a pane that isn't focused
  // have an outline as well as a background.
  const outline = (sel) => page.$eval(sel, (el) => getComputedStyle(el).boxShadow);
  assert.notEqual(await outline('[data-action="mode"][aria-checked="true"]'), 'none');
  assert.equal(await page.getAttribute('.pane[data-pane-id="p4"]', 'data-focused'), 'false');
  assert.notEqual(await outline('.pane[data-pane-id="p4"] .subtab[aria-selected="true"]'), 'none');
  assert.equal(await outline('.pane[data-pane-id="p4"] .subtab[aria-selected="false"]'), 'none');
});
