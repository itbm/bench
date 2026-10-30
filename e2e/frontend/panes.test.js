'use strict';

// Arranging a tab's panes: Zoom pane, from a window's menu, the jump
// palette, and the zoom_pane shortcut, which shows one pane alone, with a
// Zoomed chip that shows every pane again; a double-click on a divider,
// which gives its split's panes equal sizes, with Undo; and Split right or
// down with an empty pane, which opens the add dialog for the new pane and
// takes the pane away again if nothing is added.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, settle } = require('./helpers/harness.js');
const { defaultWorld } = require('./helpers/fixture.js');

// toastIs waits for the toast to show text.
const toastIs = (page, text) => page.waitForFunction((t) => {
  const el = document.getElementById('toast');
  return el.classList.contains('show') && (el.querySelector('.toast-text') || el).textContent === t;
}, text);

// paneIds returns the panes drawn, in order.
const paneIds = (page) => page.$$eval('.pane', (els) => els.map((el) => el.dataset.paneId));

// openMenu opens the ⋯ menu of the window that a pane shows.
async function openMenu(page, paneId) {
  await page.click('[data-action="menu"][data-pane="' + paneId + '"]');
  await page.waitForSelector('#window-menu');
}

// menuEntries returns the open menu's entries, with their shortcuts, and
// whether each is disabled.
const menuEntries = (page) => page.$$eval('#menu-root .menu-item', (els) => els.map((el) => ({
  // A button of a web page's Page row has its label as its name.
  label: el.querySelector('.menu-label') ? el.querySelector('.menu-label').textContent : el.getAttribute('aria-label'),
  keys: el.querySelector('kbd') ? el.querySelector('kbd').textContent : '',
  disabled: el.disabled,
})));

test('Zoom pane, from a window\'s menu, shows the pane alone, with a Zoomed chip that shows every pane again', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await openMenu(page, 'p4');
  const zoom = (await menuEntries(page)).find((e) => e.label === 'Zoom pane');
  assert.deepEqual(zoom, { label: 'Zoom pane', keys: '⌃⌥↩', disabled: false });
  await page.click('#window-menu .menu-item:has-text("Zoom pane")');
  assert.deepEqual((await fake.waitForCall('ZoomPane')).args, ['p4']);

  await page.waitForSelector('.pane[data-pane-id="p4"] .zoom-chip');
  assert.deepEqual(await paneIds(page), ['p4']);
  assert.equal(await page.$$eval('.divider', (els) => els.length), 0);
  // The pane fills the pane area, inside the gap.
  const box = await page.$eval('.pane[data-pane-id="p4"]', (el) => [el.offsetLeft, el.offsetWidth]);
  const area = await page.$eval('#panes', (el) => el.clientWidth);
  assert.deepEqual(box, [4, area - 8]);
  const chip = await page.$eval('.zoom-chip', (el) => [el.textContent, el.title, el.getAttribute('aria-label')]);
  assert.deepEqual(chip, ['Zoomed', 'Show all panes (⌃⌥↩)', 'Zoomed: show all panes']);
  // The status bar says so, and counts the tab's panes, out of sight or not.
  assert.equal(await page.textContent('.status-zoom'), 'One pane zoomed');
  assert.equal(await page.textContent('.status-count'), '4 windows in 2 panes');
  // The window menu says how to show them again.
  await openMenu(page, 'p4');
  assert.ok((await menuEntries(page)).some((e) => e.label === 'Show all panes' && e.keys === '⌃⌥↩'));
  await page.keyboard.press('Escape');
  await page.waitForSelector('#window-menu', { state: 'detached' });

  await page.click('.zoom-chip');
  await fake.waitForCall('ShowAllPanes');
  await page.waitForFunction(() => document.querySelectorAll('.pane').length === 2);
  assert.deepEqual(await paneIds(page), ['p3', 'p4']);
  assert.equal(await page.$$eval('.zoom-chip, .status-zoom', (els) => els.length), 0);
});

test('the status bar\'s note of a zoom shows every pane again, and the jump palette zooms and shows them', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('[data-action="open-palette"]');
  await page.fill('#palette-query', 'zoom');
  // Fill the screen zooms Bench's window, and says so.
  const rows = await page.$$eval('.palette-item', (els) => els.map((el) => el.querySelector('.palette-label').textContent));
  assert.deepEqual(rows, ['Zoom this pane', 'Fill the screen']);
  assert.equal(await page.$eval('.palette-item .palette-keys', (el) => el.textContent), '⌃⌥↩  ↵');
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('ZoomPane')).args, ['']);
  await page.waitForSelector('.status-zoom');
  assert.deepEqual(await paneIds(page), ['p3']);

  await page.click('[data-action="open-palette"]');
  await page.fill('#palette-query', 'all panes');
  assert.deepEqual(await page.$$eval('.palette-item .palette-label', (els) => els.map((el) => el.textContent)), ['Show all panes']);
  await page.keyboard.press('Escape');
  await page.click('.status-zoom');
  await fake.waitForCall('ShowAllPanes');
  await page.waitForFunction(() => document.querySelectorAll('.pane').length === 2);
});

test('the zoom_pane shortcut zooms the focused pane, and shows every pane again, in the browser preview', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ demo: true }) });
  t.after(close);
  await page.click('.statusbar');
  await page.keyboard.press('Control+Alt+Enter');
  assert.deepEqual((await fake.waitForCall('ZoomPane')).args, ['']);
  await page.waitForSelector('.zoom-chip');
  await page.keyboard.press('Control+Alt+Enter');
  await fake.waitForCall('ShowAllPanes');
  await page.waitForFunction(() => document.querySelectorAll('.pane').length === 2);
  // Switching tabs ends a zoom.
  await page.keyboard.press('Control+Alt+Enter');
  await page.waitForSelector('.zoom-chip');
  await page.click('.tab[data-index="2"]');
  await page.waitForFunction(() => window.bench.store.view.current === 2);
  await page.click('.tab[data-index="1"]');
  await page.waitForFunction(() => window.bench.store.view.current === 1 && document.querySelectorAll('.pane').length === 2);
  assert.equal(await page.$$eval('.zoom-chip', (els) => els.length), 0);
});

test('the "tabs" view has no Zoom pane, and splits with an empty pane are off there', async (t) => {
  const { page, close } = await openBench({ world: defaultWorld({ current: 0, mode: 'compact' }) });
  t.after(close);
  await page.click('.title-pane [data-action="menu"]');
  await page.waitForSelector('#window-menu');
  const entries = await menuEntries(page);
  // A web page's Zoom in and Zoom out zoom the page, not the pane.
  assert.equal(entries.some((e) => /zoom (this )?pane|all panes/i.test(e.label)), false);
  for (const label of ['Split right with an empty pane', 'Split down with an empty pane']) {
    assert.equal(entries.find((e) => e.label === label).disabled, true, label);
  }
  await page.keyboard.press('Escape');
  await page.click('[data-action="open-palette"]');
  await page.fill('#palette-query', 'pane');
  const rows = await page.$$eval('.palette-item .palette-label', (els) => els.map((el) => el.textContent));
  assert.equal(rows.some((r) => /zoom (this )?pane|all panes/i.test(r)), false, rows.join(', '));
});

test('a double-click on a divider gives its split\'s panes equal sizes, with Undo, and so does Return', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  const sizes = () => page.evaluate(() => window.bench.store.view.tabs[1].root.sizes.map(Math.round));
  await page.dblclick('.divider[data-split="s2"]');
  assert.deepEqual((await fake.waitForCall('EvenSizes')).args, ['s2']);
  await toastIs(page, 'Made the panes the same size.');
  assert.equal(await page.textContent('#toast .toast-action'), 'Undo');
  await page.waitForFunction(() => Math.round(window.bench.store.view.tabs[1].root.sizes[0]) === 50);
  // The clicks don't drag the divider.
  assert.equal(fake.callsTo('SetSizes').length, 0);

  await page.click('#toast .toast-action');
  await fake.waitForCall('Undo');
  await page.waitForFunction(() => Math.round(window.bench.store.view.tabs[1].root.sizes[0]) === 65);
  assert.deepEqual(await sizes(), [65, 35]);

  fake.clearCalls();
  await page.focus('.divider[data-split="s2"]');
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('EvenSizes')).args, ['s2']);
  await page.waitForFunction(() => Math.round(window.bench.store.view.tabs[1].root.sizes[0]) === 50);
});

test('a double-click on a divider reaches Go after the sizes its first click sent', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  const order = [];
  fake.on('SetSizes', async (args, f) => {
    order.push('SetSizes');
    await new Promise((r) => setTimeout(r, 300));
    order.push('SetSizes done');
    return f.mSetSizes(...args);
  });
  fake.on('EvenSizes', (args, f) => {
    order.push('EvenSizes');
    return f.mEvenSizes(...args);
  });
  // The first click moves the divider by a pixel, so its release sends the
  // sizes; Go is slow to take them.
  const d = await page.locator('.divider[data-split="s2"]').boundingBox();
  const x = d.x + d.width / 2;
  const y = d.y + d.height / 2;
  await page.mouse.move(x, y);
  await page.mouse.down();
  await page.mouse.move(x + 1, y);
  await page.mouse.up();
  await page.dblclick('.divider[data-split="s2"]');
  await fake.waitForCall('EvenSizes');
  assert.deepEqual(order, ['SetSizes', 'SetSizes done', 'EvenSizes']);
  await page.waitForFunction(() => Math.round(window.bench.store.view.tabs[1].root.sizes[0]) === 50);
});

test('Split down with an empty pane opens the add dialog for the new pane, and Cancel takes the pane away', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await openMenu(page, 'p4');
  await page.click('#window-menu .menu-item:has-text("Split down with an empty pane")');
  const split = await fake.waitForCall('SplitPane');
  assert.deepEqual(split.args, ['p4', 'bottom']);
  const id = (await fake.waitForCall('AddOptions')).args[0];
  assert.notEqual(id, 'p4');
  await page.waitForSelector('.palette');
  await page.waitForSelector('.pane[data-pane-id="' + id + '"][data-focused="true"]');
  assert.equal(await page.evaluate(() => window.bench.store.ui.add.paneId), id);
  assert.deepEqual(await paneIds(page), ['p3', 'p4', id]);

  await page.keyboard.press('Escape');
  assert.deepEqual((await fake.waitForCall('CancelSplit')).args, [id]);
  await page.waitForFunction(() => document.querySelectorAll('.pane').length === 2);
  assert.deepEqual(await paneIds(page), ['p3', 'p4']);
  assert.equal(fake.callsTo('CancelSplit').length, 1);
});

test('Split right with an empty pane waits for what the add dialog adds before asking to take the pane away', async (t) => {
  const { page, fake, close } = await openBench({
    setup: (f) => f.on('AddToPane', () => new Promise((resolve) => setTimeout(() => resolve(null), 300))),
  });
  t.after(close);
  await openMenu(page, 'p3');
  await page.click('#window-menu .menu-item:has-text("Split right with an empty pane")');
  assert.deepEqual((await fake.waitForCall('SplitPane')).args, ['p3', 'right']);
  const id = (await fake.waitForCall('AddOptions')).args[0];
  await page.waitForSelector('.palette');
  await page.fill('#palette-query', 'New terminal');
  await page.keyboard.press('Enter');
  const added = await fake.waitForCall('AddToPane');
  assert.deepEqual(added.args, [id, { type: 'terminal', folder: '' }]);
  const cancel = await fake.waitForCall('CancelSplit');
  assert.deepEqual(cancel.args, [id]);
  // Go keeps a pane that something was added to: CancelSplit comes after
  // AddToPane has answered.
  assert.ok(cancel.at - added.at >= 280, 'CancelSplit came ' + (cancel.at - added.at) + ' ms after AddToPane');
});

test('a split with an empty pane goes again if the add dialog can\'t open', async (t) => {
  const { page, fake, close } = await openBench({
    setup: (f) => f.on('AddOptions', (args) => {
      if (args[0] !== 'p3' && args[0] !== 'p4') throw new Error('Bench is busy.');
      return [];
    }),
  });
  t.after(close);
  await openMenu(page, 'p4');
  await page.click('#window-menu .menu-item:has-text("Split right with an empty pane")');
  await fake.waitForCall('SplitPane');
  const id = (await fake.waitForCall('AddOptions')).args[0];
  assert.deepEqual((await fake.waitForCall('CancelSplit')).args, [id]);
  await settle(page);
  assert.equal(await page.$$eval('.palette', (els) => els.length), 0);
  await page.waitForFunction(() => document.querySelectorAll('.pane').length === 2);
});
