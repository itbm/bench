'use strict';

// Renaming in place, and new tabs for a window: double-clicking a tab, or
// Rename… in its menu, turns its name into a field; Rename… in a
// terminal's or web page's menu gives it a name of its own; and Move to
// new tab, Also show in new tab, and a drop on the tabs' + make a tab for a
// window.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, settle } = require('./helpers/harness.js');
const { defaultWorld } = require('./helpers/fixture.js');

async function box(page, selector) {
  const b = await page.locator(selector).boundingBox();
  assert.ok(b, selector + ' is on the page');
  return b;
}

// renamesTabs makes SaveTab rename the tab, numbering a name that another
// tab has, ignoring case, as Go does. FakeGo's SaveTab changes nothing.
function renamesTabs(fake) {
  fake.on('SaveTab', ([index, draft], f) => {
    const taken = (n) => f.world.tabs.some((t, i) => i !== index && t.name.toLowerCase() === n.toLowerCase());
    let name = draft.name;
    for (let n = 1; taken(name); n++) name = draft.name + ' (' + n + ')';
    f.world.tabs[index].name = name;
    f.changed();
    return null;
  });
}

// tabNames returns the tabs' names, as drawn.
function tabNames(page) {
  return page.$$eval('#titlebar .tab .tab-name', (els) => els.map((el) => el.textContent));
}

// active returns the ID, or else the data-key, of the focused element.
function active(page) {
  return page.evaluate(() => [document.activeElement.id || document.activeElement.dataset.key]);
}

test('double-clicking a tab renames it in place, and Go numbers a name another tab has', async (t) => {
  const { page, fake, close } = await openBench({ setup: renamesTabs });
  t.after(close);
  await settle(page);
  fake.clearCalls();
  await page.dblclick('.tab[data-index="2"] .tab-name');
  await page.waitForSelector('.tab.renaming[data-index="2"] #rename-input');
  // The first click switches to the tab; the second doesn't again.
  assert.deepEqual(fake.callsTo('SwitchTab').map((c) => c.args), [[2]]);
  assert.deepEqual(await active(page), ['rename-input']);
  assert.deepEqual(await page.evaluate(() => {
    const el = document.activeElement;
    return [el.value, el.selectionStart, el.selectionEnd];
  }), ['Research', 0, 'Research'.length]);
  // The tab, the field, and the other tabs don't move the window, so a
  // double-click on them doesn't zoom it as one on the title bar does.
  assert.deepEqual(await page.evaluate(() => {
    const drag = (sel) => getComputedStyle(document.querySelector(sel)).getPropertyValue('--wails-draggable').trim();
    return [drag('#titlebar'), drag('.tab.renaming'), drag('#rename-input'), drag('.rename-note'), drag('.tab[data-index="0"] .tab-name')];
  }), ['drag', 'no-drag', 'no-drag', 'no-drag', 'no-drag']);
  // Bench isn't raised for it: no menu or dialog is open.
  assert.equal(fake.callsTo('SetOverlay').filter((c) => c.args[0]).length, 0);

  // A name another tab has will be numbered, and the field says so.
  await page.keyboard.type('code');
  await page.waitForFunction(() => document.getElementById('rename-note').textContent.startsWith('→ code (1)'));
  assert.equal(await page.textContent('#rename-note .visually-hidden'), 'There\'s already a tab called Code, so this one will be called code (1).');
  assert.equal(await page.getAttribute('#rename-input', 'aria-describedby'), 'rename-help rename-note');
  await page.keyboard.press('Backspace');
  await page.waitForFunction(() => document.getElementById('rename-note').textContent === '');

  // Return saves through SaveTab, keeping the tab's layout and its
  // small-screen setting, and the name Go gives it shows.
  await page.keyboard.type('e');
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('SaveTab')).args, [2, { name: 'code', preset: 'custom', smallScreens: 'tabs' }]);
  await page.waitForSelector('#rename-input', { state: 'detached' });
  await page.waitForFunction(() => document.querySelector('.tab[data-index="2"] .tab-name').textContent === 'code (1)');
  assert.deepEqual(await tabNames(page), ['Comms', 'Code', 'code (1)']);
  // The keyboard goes back to the tab.
  assert.deepEqual(await active(page), ['tab-2']);
  assert.equal(await page.$('.editor-sheet'), null);
});

test('the field takes the keyboard after the switch that a double-click\'s first click sends', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await settle(page);
  // Go is slow to switch, and focuses the tab's window as it does.
  const order = [];
  fake.on('SwitchTab', async ([index], f) => {
    order.push('SwitchTab ' + index);
    await new Promise((r) => setTimeout(r, 200));
    order.push('switched');
    return f.mSwitchTab(index);
  });
  fake.on('FocusUI', () => { order.push('FocusUI'); return null; });

  // On another tab: the switch first, then the field takes the keyboard.
  fake.clearCalls();
  await page.dblclick('.tab[data-index="2"] .tab-name');
  await page.waitForSelector('.tab.renaming[data-index="2"] #rename-input');
  await fake.waitForCall('FocusUI');
  assert.deepEqual(order, ['SwitchTab 2', 'switched', 'FocusUI']);
  await page.keyboard.press('Escape');
  await page.waitForSelector('#rename-input', { state: 'detached' });

  // On the current tab, the first click doesn't switch at all: it would
  // only give the tab's window the keyboard.
  order.length = 0;
  await page.dblclick('.tab[data-index="2"] .tab-name');
  await page.waitForSelector('.tab.renaming[data-index="2"] #rename-input');
  await fake.waitForCall('FocusUI', () => order.length > 0);
  await new Promise((r) => setTimeout(r, 500));
  assert.deepEqual(order, ['FocusUI']);
  assert.deepEqual(await active(page), ['rename-input']);
  await page.keyboard.press('Escape');
  await page.waitForSelector('#rename-input', { state: 'detached' });

  // A single click on the current tab still gives its window the keyboard,
  // after a moment.
  order.length = 0;
  await page.click('.tab[data-index="2"] .tab-name');
  await fake.waitForCall('SwitchTab', () => order.length > 0);
  assert.deepEqual(order.slice(0, 1), ['SwitchTab 2']);
});

test('Escape, or clicking elsewhere, cancels a rename, and an unchanged or empty name is kept', async (t) => {
  const { page, fake, close } = await openBench({ setup: renamesTabs });
  t.after(close);
  await settle(page);
  fake.clearCalls();

  // Escape, from the field that Rename… in the tab's menu opened.
  await page.click('.tab[data-index="0"]', { button: 'right' });
  await page.click('#tab-menu .menu-item:has-text("Rename…")');
  await page.waitForSelector('.tab.renaming[data-index="0"] #rename-input');
  assert.deepEqual(await active(page), ['rename-input']);
  await page.keyboard.type('Chat');
  await page.keyboard.press('Escape');
  await page.waitForSelector('#rename-input', { state: 'detached' });
  assert.deepEqual(await active(page), ['tab-0']);

  // Clicking elsewhere, here in a pane.
  await page.dblclick('.tab[data-index="1"] .tab-name');
  await page.waitForSelector('.tab.renaming[data-index="1"] #rename-input');
  await page.keyboard.type('Work');
  await page.click('.pane[data-pane-id="p4"] .pane-body');
  await page.waitForSelector('#rename-input', { state: 'detached' });

  // Another app taking the keyboard.
  await page.dblclick('.tab[data-index="1"] .tab-name');
  await page.waitForSelector('#rename-input');
  await page.evaluate(() => document.getElementById('rename-input').blur());
  await page.waitForSelector('#rename-input', { state: 'detached' });

  // The same name, or no name, keeps the tab's.
  await page.dblclick('.tab[data-index="1"] .tab-name');
  await page.waitForSelector('#rename-input');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#rename-input', { state: 'detached' });
  await page.dblclick('.tab[data-index="1"] .tab-name');
  await page.waitForSelector('#rename-input');
  await page.keyboard.press('Backspace');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#rename-input', { state: 'detached' });

  await settle(page);
  assert.equal(fake.callsTo('SaveTab').length, 0);
  assert.deepEqual(await tabNames(page), ['Comms', 'Code', 'Research']);
  // A double-click on the current tab doesn't switch to it, which would
  // give its window the keyboard: neither of its clicks does.
  assert.deepEqual(fake.callsTo('SwitchTab').map((c) => c.args), []);
});

test('a rename that Go refuses keeps the field, with Go\'s message', async (t) => {
  const { page, fake, close } = await openBench({
    setup: (f) => f.on('SaveTab', () => { throw new Error('Bench can\'t save that: the name is too long.'); }),
  });
  t.after(close);
  await page.dblclick('.tab[data-index="1"] .tab-name');
  await page.waitForSelector('#rename-input');
  await page.keyboard.type('Building');
  await page.keyboard.press('Enter');
  await fake.waitForCall('SaveTab');
  await page.waitForFunction(() => document.getElementById('toast').textContent === 'Bench can\'t save that: the name is too long.');
  await settle(page);
  assert.equal(await page.inputValue('#rename-input'), 'Building');
  assert.deepEqual(await active(page), ['rename-input']);
  assert.equal(await page.getAttribute('#rename-input', 'readonly'), null);
});

test('a terminal\'s sub-tab can be renamed, and given its automatic title back', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await settle(page);
  const menuOf = async (item) => {
    await page.click('.subtab[data-item="' + item + '"]', { button: 'right' });
    await page.waitForSelector('#window-menu');
    return page.$$eval('#window-menu .menu-item', (els) => els.map((el) => el.textContent));
  };
  // An app's window takes its title from the app.
  assert.ok(!(await menuOf('w3')).includes('Rename…'));
  await page.keyboard.press('Escape');

  const entries = await menuOf('w5');
  assert.ok(entries.includes('Rename…'), JSON.stringify(entries));
  assert.ok(!entries.some((e) => e.startsWith('Use automatic title')));
  await page.click('#window-menu .menu-item:has-text("Rename…")');
  await page.waitForSelector('.subtab.renaming[data-item="w5"][data-pane="p4"] #rename-input');
  assert.deepEqual(await active(page), ['rename-input']);
  assert.equal(await page.inputValue('#rename-input'), 'zsh — ~/code/bench');
  assert.equal(await page.getAttribute('#rename-input', 'aria-label'), 'Name of this terminal, Terminal — zsh — ~/code/bench');
  await page.keyboard.type('API server');
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('RenameItem')).args, ['w5', 'API server']);
  await page.waitForSelector('#rename-input', { state: 'detached' });
  await page.waitForFunction(() => document.querySelector('.subtab[data-item="w5"] .subtab-title').textContent === 'API server');
  assert.deepEqual(await active(page), ['sub-w5']);
  // Its tooltip has its whole name, its app, and the title its shell gives
  // it.
  assert.equal(await page.getAttribute('.subtab[data-item="w5"]', 'title'), 'Terminal — API server\nAutomatic title: zsh — ~/code/bench');

  // Use automatic title takes the name away.
  const named = await menuOf('w5');
  assert.ok(named.includes('Use automatic titlezsh — ~/code/bench'), JSON.stringify(named));
  await page.click('#window-menu .menu-item:has-text("Use automatic title")');
  assert.deepEqual((await fake.waitForCall('RenameItem', (a) => a[1] === '')).args, ['w5', '']);
  await page.waitForFunction(() => document.querySelector('.subtab[data-item="w5"] .subtab-title').textContent === 'zsh — ~/code/bench');
  assert.equal(await page.getAttribute('.subtab[data-item="w5"]', 'title'), 'Terminal — zsh — ~/code/bench');
});

test('a sub-tab that\'s in other tabs too keeps a tooltip that names them', async (t) => {
  const world = defaultWorld();
  world.tabs[0].root.children[0].items.push('w5');
  world.items.w5 = { ...world.items.w5, title: 'Build', label: 'Build', autoTitle: 'zsh — ~/code/bench' };
  const { page, close } = await openBench({ world });
  t.after(close);
  assert.equal(await page.getAttribute('.subtab[data-item="w5"]', 'title'), 'Terminal — Build\nAutomatic title: zsh — ~/code/bench\nAlso in Comms');
});

test('Move to new tab and Also show in new tab, from a window\'s menu and the jump palette', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await settle(page);
  fake.clearCalls();
  await page.click('.subtab[data-item="w6"]', { button: 'right' });
  await page.click('#window-menu .menu-item:has-text("Move to new tab")');
  assert.deepEqual((await fake.waitForCall('MoveItemToNewTab')).args, ['w6', false]);
  // Go names the tab after the window's app, and switches to it.
  await page.waitForFunction(() => document.querySelectorAll('#titlebar .tab').length === 4);
  await page.waitForSelector('.tab[data-index="3"][aria-pressed="true"]');
  assert.deepEqual(await tabNames(page), ['Comms', 'Code', 'Research', 'Terminal']);
  await page.waitForFunction(() => document.getElementById('toast').textContent.startsWith('Moved Terminal — go test — ~/code/bench to a new tab, Terminal.'));
  // Its Undo takes the tab away again.
  await page.click('#toast .toast-action');
  await fake.waitForCall('Undo');
  await page.waitForFunction(() => document.querySelectorAll('#titlebar .tab').length === 3);
  await page.waitForSelector('.subtab[data-item="w6"]');
  await page.click('.subtab[data-item="w6"]');

  // The palette's window actions are for the focused pane's window.
  await page.click('[data-action="open-palette"]');
  await page.waitForSelector('.palette');
  await page.keyboard.type('new tab');
  const rows = await page.$$eval('#palette-list .palette-item', (els) => els.map((el) =>
    el.querySelector('.palette-label').textContent + ' | ' + el.querySelector('.palette-hint').textContent));
  assert.ok(rows.includes('Move to new tab | Terminal — go test — ~/code/bench'), JSON.stringify(rows));
  assert.ok(rows.includes('Also show in new tab | Terminal — go test — ~/code/bench'), JSON.stringify(rows));
  await page.click('.palette-item:has-text("Also show in new tab")');
  assert.deepEqual((await fake.waitForCall('MoveItemToNewTab', (a) => a[1])).args, ['w6', true]);
  // Not numbered, as Undo took the first Terminal away.
  await page.waitForFunction(() => document.querySelectorAll('#titlebar .tab').length === 4);
  assert.deepEqual(await tabNames(page), ['Comms', 'Code', 'Research', 'Terminal']);
  // The window is in both.
  await page.waitForSelector('.subtab[data-item="w6"] .subtab-shared');
});

test('a window\'s menu offers a new tab when there\'s only one tab', async (t) => {
  const world = defaultWorld({ current: 0 });
  world.tabs = [world.tabs[0]];
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await page.click('.subtab[data-item="w1"]', { button: 'right' });
  await page.waitForSelector('#window-menu');
  const entries = await page.$$eval('#window-menu > *', (els) => els.map((el) => el.className + ':' + el.textContent));
  const at = entries.indexOf('menu-item:Move to new tab');
  assert.ok(at > 0, JSON.stringify(entries));
  assert.deepEqual(entries.slice(at - 1, at + 3), ['menu-sep:', 'menu-item:Move to new tab', 'menu-sep:', 'menu-item:Also show in new tab']);
  assert.ok(!entries.includes('menu-heading:Move to tab'));
  // A web page can be renamed too.
  assert.ok(entries.includes('menu-item:Rename…'));
  await page.click('#window-menu .menu-item:has-text("Move to new tab")');
  assert.deepEqual((await fake.waitForCall('MoveItemToNewTab')).args, ['w1', false]);
  await page.waitForFunction(() => document.querySelectorAll('#titlebar .tab').length === 2);
  assert.deepEqual(await tabNames(page), ['Comms', 'Slack']);
});

test('dropping a sub-tab on the tabs\' + moves its window to a new tab, or with Option shows it there too', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await settle(page);
  fake.clearCalls();
  const drag = async (item, to, alt = false) => {
    const src = await box(page, '.subtab[data-item="' + item + '"]');
    await page.mouse.move(src.x + src.width / 2, src.y + src.height / 2);
    await page.mouse.down();
    await page.mouse.move(src.x + src.width / 2 + 20, src.y + src.height / 2 - 10, { steps: 4 });
    if (alt) await page.keyboard.down('Alt');
    await page.mouse.move(to.x, to.y, { steps: 6 });
  };

  // On the +: it's marked as a tab is, and the drop moves the window.
  const plus = await box(page, '.new-tab');
  await drag('w5', { x: plus.x + plus.width / 2, y: plus.y + plus.height / 2 });
  await page.waitForSelector('.new-tab.drop-target:not(.drop-add)');
  assert.equal(await page.$('.drag-ghost.adding'), null);
  assert.equal(await page.$('.drop-caret:not([hidden])'), null);
  await page.mouse.up();
  assert.deepEqual((await fake.waitForCall('MoveItemToNewTab')).args, ['w5', false]);
  await fake.waitForLowered();
  assert.equal(await page.$('.new-tab.drop-target'), null);
  for (const method of ['MoveItemToTab', 'AddItemToTab', 'MoveItem', 'MoveItemAt', 'ShowItem', 'SaveTab']) assert.equal(fake.callsTo(method).length, 0, method);
  await page.waitForSelector('.tab[data-index="3"][aria-pressed="true"]');
  assert.deepEqual(await tabNames(page), ['Comms', 'Code', 'Research', 'Terminal']);

  // Back in Code, on the title bar just after the +, with Option held:
  // shown there as well.
  await page.click('.tab[data-index="1"]');
  await page.waitForSelector('.tab[data-index="1"][aria-pressed="true"]');
  await settle(page);
  fake.clearCalls();
  const after = await box(page, '.new-tab');
  await drag('w6', { x: after.x + after.width * 1.5, y: after.y + after.height / 2 }, true);
  await page.waitForSelector('.new-tab.drop-target.drop-add');
  await page.waitForSelector('.drag-ghost.adding');
  await page.mouse.up();
  await page.keyboard.up('Alt');
  assert.deepEqual((await fake.waitForCall('MoveItemToNewTab')).args, ['w6', true]);
  await page.waitForFunction(() => document.querySelectorAll('#titlebar .tab').length === 5);
  assert.deepEqual(await tabNames(page), ['Comms', 'Code', 'Research', 'Terminal', 'Terminal (1)']);
});

test('a sub-tab dropped on the title bar well past the + makes no tab', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await settle(page);
  fake.clearCalls();
  const src = await box(page, '.subtab[data-item="w5"]');
  const end = await box(page, '.titlebar-end');
  const plus = await box(page, '.new-tab');
  assert.ok(end.x - plus.x > 4 * plus.width, 'the title bar has room past the +');
  await page.mouse.move(src.x + src.width / 2, src.y + src.height / 2);
  await page.mouse.down();
  await page.mouse.move(src.x + src.width / 2 + 20, src.y + src.height / 2 - 10, { steps: 4 });
  // Next to the controls at the title bar's end: the window's space.
  await page.mouse.move(end.x - 12, plus.y + plus.height / 2, { steps: 6 });
  await page.waitForSelector('body.dragging-subtab');
  assert.equal(await page.$('.new-tab.drop-target'), null);
  await page.mouse.up();
  await fake.waitForLowered();
  for (const method of ['MoveItemToNewTab', 'MoveItemToTab', 'MoveItem', 'MoveItemAt']) assert.equal(fake.callsTo(method).length, 0, method);
});

test('a tab dragged over the + goes to the end of the tabs, as before', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await settle(page);
  fake.clearCalls();
  const src = await box(page, '.tab[data-index="0"]');
  const plus = await box(page, '.new-tab');
  await page.mouse.move(src.x + src.width / 2, src.y + src.height / 2);
  await page.mouse.down();
  await page.mouse.move(plus.x + plus.width / 2, plus.y + plus.height / 2, { steps: 8 });
  await page.waitForSelector('.drop-caret:not([hidden])[data-kind="tab"]');
  assert.equal(await page.$('.new-tab.drop-target'), null);
  await page.mouse.up();
  assert.deepEqual((await fake.waitForCall('MoveTab')).args, [0, 2]);
  assert.equal(fake.callsTo('MoveItemToNewTab').length, 0);
});
