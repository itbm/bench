'use strict';

// Undo on a toast, and Reopen closed tab: a toast that Go sends with a
// button, such as Undo after Delete tab, shows it, longer, unless the toast
// covers tabs, and the button calls the method Go named; Reopen closed tab
// is in a tab's menu and the jump palette's Actions.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, settle } = require('./helpers/harness.js');
const { defaultWorld, manyTabs } = require('./helpers/fixture.js');

const DELETED = 'Deleted Code. Its windows are in Jump to… under “Not in a tab”.';

// toastText returns the toast's message, without its button.
const toastText = (page) => page.evaluate(() => {
  const el = document.getElementById('toast');
  return (el.querySelector('.toast-text') || el).textContent;
});

// toastIs waits for the toast to show text.
const toastIs = (page, text) => page.waitForFunction((t) => {
  const el = document.getElementById('toast');
  return el.classList.contains('show') && (el.querySelector('.toast-text') || el).textContent === t;
}, text);

// hit reports whether a click at the middle of selector reaches it, rather
// than something over or under it.
const hit = (page, selector) => page.$eval(selector, (el) => {
  const r = el.getBoundingClientRect();
  return document.elementFromPoint(r.x + r.width / 2, r.y + r.height / 2) === el;
});

// hidden returns a world whose status bar is hidden, so that toasts show in
// the title bar.
function hidden(opts) {
  const world = defaultWorld(opts);
  world.settings.showStatusBar = false;
  return world;
}

// deleteCode deletes Code from its tab menu.
async function deleteCode(page) {
  await page.click('.tab[data-index="1"]', { button: 'right' });
  await page.waitForSelector('#tab-menu');
  await page.click('#tab-menu .menu-item:has-text("Delete tab (keep windows)")');
}

test('Delete tab\'s toast has Undo, which calls Undo with Go\'s step and puts the tab back', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await deleteCode(page);
  assert.deepEqual((await fake.waitForCall('DeleteTab')).args, [1]);
  await toastIs(page, DELETED);
  assert.equal(await page.textContent('#toast .toast-action'), 'Undo');
  assert.equal(await page.$eval('#toast', (el) => el.classList.contains('with-action')), true);
  // Over the status bar, the button takes the click, though the toast
  // lets clicks through.
  assert.equal(await page.$eval('#toast', (el) => getComputedStyle(el).pointerEvents), 'none');
  assert.equal(await page.$eval('#toast .toast-action', (el) => getComputedStyle(el).pointerEvents), 'auto');
  assert.equal(await hit(page, '#toast .toast-action'), true);
  await page.waitForFunction(() => window.bench.store.view.tabs.length === 2);

  await page.click('#toast .toast-action');
  assert.deepEqual((await fake.waitForCall('Undo')).args, [1]);
  await toastIs(page, 'Brought back Code.');
  // Go's answer is a toast of its own, without a button.
  assert.equal(await page.$('#toast .toast-action'), null);
  await page.waitForFunction(() => window.bench.store.view.tabs.length === 3);
  assert.deepEqual(await page.evaluate(() => window.bench.store.view.tabs.map((x) => x.name)), ['Comms', 'Code', 'Research']);
  assert.deepEqual(fake.callsTo('Undo').length, 1);
});

test('a toast\'s button runs once, and a hidden toast\'s button takes no clicks', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  // A toast that Go sends with a button that isn't a method's name has no
  // button.
  await fake.emit('bench:toast', { text: 'Moved Zed.', action: { label: 'Undo', method: 'alert(1)', arg: 1 } });
  await toastIs(page, 'Moved Zed.');
  assert.equal(await page.$('#toast .toast-action'), null);

  // Go answers a little later. Two clicks before then call Undo once.
  fake.on('Undo', async () => {
    await new Promise((r) => setTimeout(r, 300));
    return null;
  });
  await fake.emit('bench:toast', { text: 'Moved Zed.', action: { label: 'Undo', method: 'Undo', arg: 7 } });
  await page.waitForSelector('#toast .toast-action');
  await page.evaluate(() => {
    const b = document.querySelector('#toast .toast-action');
    b.click();
    b.click();
  });
  // The toast goes at once. Its button stays until the next toast, hidden,
  // and clicks there reach what's under it.
  await page.waitForFunction(() => !document.getElementById('toast').classList.contains('show'));
  await settle(page, 400);
  assert.equal(fake.callsTo('Undo').length, 1);
  assert.deepEqual(fake.callsTo('Undo')[0].args, [7]);
  assert.equal(await page.$eval('#toast .toast-action', (el) => getComputedStyle(el).visibility), 'hidden');
  assert.equal(await hit(page, '#toast .toast-action'), false);

  // A step that Go no longer has: its refusal shows as a toast.
  fake.overrides.delete('Undo');
  await fake.emit('bench:toast', { text: 'Moved Zed.', action: { label: 'Undo', method: 'Undo', arg: 8 } });
  await page.waitForFunction(() => document.querySelector('#toast.show .toast-action'));
  await page.click('#toast .toast-action');
  await toastIs(page, 'Bench can\'t undo that any more.');
  assert.equal(await page.$('#toast .toast-action'), null);
});

test('a plain toast after one with Undo has no button', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await deleteCode(page);
  await toastIs(page, DELETED);
  await fake.emit('bench:toast', 'Saved settings');
  await page.waitForFunction(() => document.getElementById('toast').textContent === 'Saved settings');
  assert.equal(await page.$eval('#toast', (el) => el.classList.contains('with-action')), false);
  assert.equal(await page.$('#toast .toast-action'), null);
});

test('with the status bar hidden, Undo shows in the title bar\'s toast, takes clicks there, and stays for 10 seconds', async (t) => {
  const { page, fake, close } = await openBench({ world: hidden() });
  t.after(close);
  await deleteCode(page);
  await toastIs(page, DELETED);
  const shown = Date.now();
  assert.equal(await page.$eval('#toast', (el) => el.classList.contains('in-titlebar')), true);
  const b = await page.evaluate(() => {
    const box = (sel) => document.querySelector(sel).getBoundingClientRect().toJSON();
    return { title: box('#titlebar'), toast: box('#toast'), button: box('#toast .toast-action') };
  });
  assert.ok(b.toast.top >= b.title.top && b.toast.bottom <= b.title.bottom, 'the toast is in the title bar: ' + JSON.stringify(b));
  // The button is whole, inside the toast, whose text is cut short instead.
  assert.ok(b.button.left >= b.toast.left && b.button.right <= b.toast.right, 'the button is in the toast: ' + JSON.stringify(b));
  assert.equal(await page.$eval('#toast', (el) => getComputedStyle(el).pointerEvents), 'none');
  assert.equal(await hit(page, '#toast .toast-action'), true);

  // It outlasts the 4 seconds a title-bar toast shows for, and goes after
  // 10.
  await page.waitForTimeout(5000);
  assert.equal(await page.$eval('#toast', (el) => el.classList.contains('show')), true);
  await page.waitForFunction(() => !document.getElementById('toast').classList.contains('show'), null, { timeout: 8000 });
  const took = Date.now() - shown;
  assert.ok(took >= 9500 && took <= 11000, 'the toast went after ' + took + ' ms');
  assert.equal(fake.callsTo('Undo').length, 0);
});

test('with the status bar hidden, a toast with Undo that has to go over the tabs shows for 4 seconds, as other toasts there do', async (t) => {
  // Six tabs leave too little room after + for the toast, which goes over
  // the tabs, where its button would take a click meant for a tab.
  const { page, fake, close } = await openBench({ world: manyTabs(hidden(), 6), viewport: { width: 1100, height: 700 } });
  t.after(close);
  await fake.emit('bench:toast', { text: 'Moved Brave.', action: { label: 'Undo', method: 'Undo', arg: 1 } });
  await toastIs(page, 'Moved Brave.');
  const shown = Date.now();
  const b = await page.evaluate(() => {
    const box = (sel) => document.querySelector(sel).getBoundingClientRect().toJSON();
    return { toast: box('#toast'), plus: box('#titlebar .new-tab') };
  });
  assert.ok(b.toast.left < b.plus.left, 'the toast is over the tabs: ' + JSON.stringify(b));
  await page.waitForFunction(() => !document.getElementById('toast').classList.contains('show'), null, { timeout: 6000 });
  const took = Date.now() - shown;
  assert.ok(took >= 3500 && took <= 5000, 'the toast went after ' + took + ' ms');
  assert.equal(fake.callsTo('Undo').length, 0);
});

test('Undo in the title bar\'s toast calls Undo', async (t) => {
  const { page, fake, close } = await openBench({ world: hidden() });
  t.after(close);
  await deleteCode(page);
  await toastIs(page, DELETED);
  await page.click('#toast .toast-action');
  assert.deepEqual((await fake.waitForCall('Undo')).args, [1]);
  await toastIs(page, 'Brought back Code.');
  assert.equal(await toastText(page), 'Brought back Code.');
});

test('Reset layout\'s toast has Undo', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('.tab[data-index="2"]', { button: 'right' });
  await page.waitForSelector('#tab-menu');
  await page.click('#tab-menu .menu-item:has-text("Reset layout")');
  await toastIs(page, 'Reset the layout of Research');
  await page.click('#toast .toast-action');
  assert.deepEqual((await fake.waitForCall('Undo')).args, [1]);
  await toastIs(page, 'Put back the layout of Research.');
});

test('a tab\'s menu offers to reopen the tab closed last, and the jump palette each closed tab', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ closedTabs: ['Mail', 'Notes'] }) });
  t.after(close);
  await page.click('.tab[data-index="0"]', { button: 'right' });
  await page.waitForSelector('#tab-menu');
  const labels = await page.locator('#tab-menu .menu-item .menu-main').allTextContents();
  assert.ok(labels.includes('Reopen closed tab'), labels.join(', '));
  assert.equal(await page.textContent('#tab-menu .menu-item:has-text("Reopen closed tab") .menu-detail'), 'Mail');
  await page.click('#tab-menu .menu-item:has-text("Reopen closed tab")');
  assert.deepEqual((await fake.waitForCall('ReopenTab')).args, ['Mail']);
  await toastIs(page, 'Reopened Mail.');
  await page.waitForFunction(() => window.bench.store.view.tabs.length === 4);
  const v = await page.evaluate(() => window.bench.store.view);
  assert.equal(v.tabs[v.current].name, 'Mail');
  assert.deepEqual(v.closedTabs, ['Notes']);

  // The jump palette lists the rest under Actions, and finds them by name.
  fake.clearCalls();
  await page.click('[data-action="open-palette"]');
  await page.fill('#palette-query', 'notes');
  const rows = await page.$$eval('.palette-item', (els) => els.map((el) => [el.querySelector('.palette-label').textContent, el.querySelector('.palette-hint').textContent]));
  assert.deepEqual(rows.filter(([label]) => label === 'Reopen closed tab'), [['Reopen closed tab', 'Notes']]);
  await page.fill('#palette-query', 'reopen');
  assert.deepEqual(await page.$$eval('.palette-item .palette-label', (els) => els.map((el) => el.textContent)), ['Reopen closed tab']);
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('ReopenTab')).args, ['Notes']);
  await page.waitForFunction(() => !window.bench.store.view.closedTabs);

  // With no closed tabs, neither offers it.
  await page.click('.tab[data-index="0"]', { button: 'right' });
  await page.waitForSelector('#tab-menu');
  assert.equal(await page.$('#tab-menu .menu-item:has-text("Reopen closed tab")'), null);
  await page.keyboard.press('Escape');
  await page.click('[data-action="open-palette"]');
  await page.fill('#palette-query', 'reopen');
  assert.equal(await page.$('.palette-item'), null);
});

test('a deleted tab is offered for reopening, and Reopen closed tab brings back its windows', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await deleteCode(page);
  await toastIs(page, DELETED);
  await page.waitForFunction(() => (window.bench.store.view.closedTabs || []).includes('Code'));
  await page.click('.tab[data-index="0"]', { button: 'right' });
  await page.waitForSelector('#tab-menu');
  await page.click('#tab-menu .menu-item:has-text("Reopen closed tab")');
  assert.deepEqual((await fake.waitForCall('ReopenTab')).args, ['Code']);
  await page.waitForFunction(() => window.bench.store.view.tabs.length === 3);
  const v = await page.evaluate(() => window.bench.store.view);
  assert.equal(v.tabs[v.current].name, 'Code');
  assert.equal(v.tabs[v.current].windows, 4);
  assert.equal(v.floating.length, 2);
});
