'use strict';

// Undo on a toast, and Reopen Closed Tab: a toast that Go sends with a
// button, such as Undo after Delete tab, shows it, longer, unless the toast
// covers tabs, and the button calls the method Go named; Reopen Closed Tab
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
  await page.click('#tab-menu .menu-item:has-text("Delete Tab (Keep Windows)")');
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

// MOVED is the toast after a move to another tab, with Undo and Go there.
const MOVED = {
  text: 'Moved Zed — bench — switch.go to Research.',
  action: { label: 'Undo', method: 'Undo', arg: 3 },
  second: { label: 'Go there', method: 'SwitchTab', arg: 2 },
};

// buttons returns the toast's buttons' labels.
const buttons = (page) => page.$$eval('#toast .toast-action', (els) => els.map((el) => el.textContent));

test('a toast with Undo and Go there shows both, and Go there calls SwitchTab once, taking the toast away', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await fake.emit('bench:toast', MOVED);
  await toastIs(page, MOVED.text);
  assert.deepEqual(await buttons(page), ['Undo', 'Go there']);
  // Over the status bar, both take clicks, in the toast.
  const b = await page.evaluate(() => {
    const box = (el) => el.getBoundingClientRect().toJSON();
    return { toast: box(document.getElementById('toast')), buttons: [...document.querySelectorAll('#toast .toast-action')].map(box) };
  });
  for (const r of b.buttons) assert.ok(r.left >= b.toast.left && r.right <= b.toast.right, 'the button is in the toast: ' + JSON.stringify(b));
  assert.ok(b.buttons[0].right <= b.buttons[1].left, 'Undo comes before Go there: ' + JSON.stringify(b));
  assert.equal(await hit(page, '#toast .toast-action[data-index="0"]'), true);
  assert.equal(await hit(page, '#toast .toast-action[data-index="1"]'), true);

  await page.evaluate(() => {
    const go = document.querySelector('#toast .toast-action[data-index="1"]');
    go.click();
    go.click();
  });
  assert.deepEqual((await fake.waitForCall('SwitchTab')).args, [2]);
  await page.waitForFunction(() => !document.getElementById('toast').classList.contains('show'));
  await page.waitForFunction(() => window.bench.store.view.current === 2);
  await settle(page, 300);
  assert.equal(fake.callsTo('SwitchTab').length, 1);
  // Undo went with the toast.
  await page.evaluate(() => document.querySelector('#toast .toast-action[data-index="0"]').click());
  await settle(page, 300);
  assert.equal(fake.callsTo('Undo').length, 0);

  // A second button that isn't a method's name is left out; Undo stays,
  // and calls Undo.
  await fake.emit('bench:toast', { ...MOVED, second: { label: 'Go there', method: 'go()', arg: 2 } });
  await page.waitForFunction(() => document.querySelector('#toast.show .toast-action'));
  assert.deepEqual(await buttons(page), ['Undo']);
  await page.click('#toast .toast-action');
  assert.deepEqual((await fake.waitForCall('Undo')).args, [3]);
});

test('with the status bar hidden, the title bar\'s toast has Undo and Go there whole, and they take clicks there', async (t) => {
  const { page, fake, close } = await openBench({ world: hidden() });
  t.after(close);
  await fake.emit('bench:toast', MOVED);
  await toastIs(page, MOVED.text);
  assert.equal(await page.$eval('#toast', (el) => el.classList.contains('in-titlebar')), true);
  const b = await page.evaluate(() => {
    const box = (el) => el.getBoundingClientRect().toJSON();
    return {
      title: box(document.getElementById('titlebar')), toast: box(document.getElementById('toast')),
      buttons: [...document.querySelectorAll('#toast .toast-action')].map(box),
    };
  });
  assert.ok(b.toast.top >= b.title.top && b.toast.bottom <= b.title.bottom, 'the toast is in the title bar: ' + JSON.stringify(b));
  for (const r of b.buttons) assert.ok(r.left >= b.toast.left && r.right <= b.toast.right, 'the button is whole: ' + JSON.stringify(b));
  assert.equal(await hit(page, '#toast .toast-action[data-index="1"]'), true);
  await page.click('#toast .toast-action[data-index="1"]');
  assert.deepEqual((await fake.waitForCall('SwitchTab')).args, [2]);
});

test('with the status bar hidden, in Bench\'s smallest window, the toast with Undo and Go there sits in the one-row title bar, clear of its notes', async (t) => {
  // Comms in the "tabs" view, on a laptop's screen, with a window that
  // needs attention and a notice in the title bar.
  const world = hidden({ screenWidth: 1512, current: 0, notices: ['Couldn\'t register ⌃⌥Space.'] });
  world.items.w1.status = 'undocked';
  world.items.w1.statusText = 'Moved out of its pane';
  const { page, fake, close } = await openBench({ world, viewport: { width: 640, height: 480 } });
  t.after(close);
  await fake.emit('bench:toast', MOVED);
  await toastIs(page, MOVED.text);
  assert.deepEqual(await buttons(page), ['Undo', 'Go there']);
  const b = await page.evaluate(() => {
    const box = (sel) => document.querySelector(sel).getBoundingClientRect().toJSON();
    const title = document.getElementById('titlebar');
    return {
      title: box('#titlebar'), toast: box('#toast'), end: box('#titlebar .titlebar-end'),
      start: title.getBoundingClientRect().left + parseFloat(getComputedStyle(title).paddingLeft),
      notes: document.querySelectorAll('#titlebar .title-note').length,
      buttons: [...document.querySelectorAll('#toast .toast-action')].map((el) => el.getBoundingClientRect().toJSON()),
    };
  });
  assert.equal(b.notes, 2);
  assert.equal(Math.round(b.title.height), 39, 'the title bar is one row: ' + JSON.stringify(b));
  assert.ok(b.toast.top >= b.title.top && b.toast.bottom <= b.title.bottom, 'the toast is in the title bar: ' + JSON.stringify(b));
  // It goes over the tabs, clear of the traffic lights, and ends before
  // the notes and the controls.
  assert.ok(b.toast.left >= b.start && b.toast.right <= b.end.left, 'the toast is between the traffic lights and the notes: ' + JSON.stringify(b));
  for (const r of b.buttons) assert.ok(r.left >= b.toast.left && r.right <= b.toast.right, 'the button is whole: ' + JSON.stringify(b));
  assert.equal(await hit(page, '#toast .toast-action[data-index="0"]'), true);
  assert.equal(await hit(page, '#toast .toast-action[data-index="1"]'), true);
  await page.click('#toast .toast-action[data-index="1"]');
  assert.deepEqual((await fake.waitForCall('SwitchTab')).args, [2]);
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
  await page.click('#tab-menu .menu-item:has-text("Reset Layout")');
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
  assert.ok(labels.includes('Reopen Closed Tab'), labels.join(', '));
  assert.equal(await page.textContent('#tab-menu .menu-item:has-text("Reopen Closed Tab") .menu-detail'), 'Mail');
  await page.click('#tab-menu .menu-item:has-text("Reopen Closed Tab")');
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
  assert.deepEqual(rows.filter(([label]) => label === 'Reopen Closed Tab'), [['Reopen Closed Tab', 'Notes']]);
  await page.fill('#palette-query', 'reopen');
  assert.deepEqual(await page.$$eval('.palette-item .palette-label', (els) => els.map((el) => el.textContent)), ['Reopen Closed Tab']);
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('ReopenTab')).args, ['Notes']);
  await page.waitForFunction(() => !window.bench.store.view.closedTabs);

  // With no closed tabs, neither offers it.
  await page.click('.tab[data-index="0"]', { button: 'right' });
  await page.waitForSelector('#tab-menu');
  assert.equal(await page.$('#tab-menu .menu-item:has-text("Reopen Closed Tab")'), null);
  await page.keyboard.press('Escape');
  await page.click('[data-action="open-palette"]');
  await page.fill('#palette-query', 'reopen');
  assert.equal(await page.$('.palette-item'), null);
});

test('a deleted tab is offered for reopening, and Reopen Closed Tab brings back its windows', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await deleteCode(page);
  await toastIs(page, DELETED);
  await page.waitForFunction(() => (window.bench.store.view.closedTabs || []).includes('Code'));
  await page.click('.tab[data-index="0"]', { button: 'right' });
  await page.waitForSelector('#tab-menu');
  await page.click('#tab-menu .menu-item:has-text("Reopen Closed Tab")');
  assert.deepEqual((await fake.waitForCall('ReopenTab')).args, ['Code']);
  await page.waitForFunction(() => window.bench.store.view.tabs.length === 3);
  const v = await page.evaluate(() => window.bench.store.view);
  assert.equal(v.tabs[v.current].name, 'Code');
  assert.equal(v.tabs[v.current].windows, 4);
  assert.equal(v.floating.length, 2);
});

// center returns the middle of selector's box.
const center = (page, selector) => page.$eval(selector, (el) => {
  const r = el.getBoundingClientRect();
  return { x: r.x + r.width / 2, y: r.y + r.height / 2 };
});

// shows reports whether the toast is showing.
const shows = (page) => page.$eval('#toast', (el) => el.classList.contains('show'));

test('a toast with Undo waits while the pointer is over it or the keyboard is in it, then shows for 3 seconds more', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await fake.emit('bench:toast', { text: 'Moved Zed.', action: { label: 'Undo', method: 'Undo', arg: 1 } });
  await toastIs(page, 'Moved Zed.');
  // Near the end of its 10 seconds, the pointer over its text, which lets
  // clicks through, holds it, and then the keyboard in it does, after the
  // pointer has gone.
  await page.waitForTimeout(8500);
  const at = await center(page, '#toast .toast-text');
  await page.mouse.move(at.x, at.y);
  await page.waitForTimeout(3000);
  await page.focus('#toast .toast-action');
  await page.mouse.move(at.x, 200);
  await page.waitForTimeout(1500);
  assert.equal(await shows(page), true, 'the toast went after 13 seconds, while held');
  await page.evaluate(() => document.activeElement.blur());
  const let_go = Date.now();
  await page.waitForFunction(() => !document.getElementById('toast').classList.contains('show'), null, { timeout: 6000 });
  const took = Date.now() - let_go;
  assert.ok(took >= 2500 && took <= 4500, 'the toast went ' + took + ' ms after it was let go');
  assert.equal(fake.callsTo('Undo').length, 0);

  // A toast without a button goes in its own time, pointer or not.
  await fake.emit('bench:toast', 'Saved.');
  await toastIs(page, 'Saved.');
  const plain = await center(page, '#toast');
  await page.mouse.move(plain.x, plain.y);
  await page.waitForFunction(() => !document.getElementById('toast').classList.contains('show'), null, { timeout: 4000 });
});

test('a toast doesn\'t cover the open list of messages, which shows it at its top with Undo while that change is the latest', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await deleteCode(page);
  await toastIs(page, DELETED);
  await page.click('[data-action="notices"]');
  await page.waitForSelector('#notices-menu');
  // The toast is out of sight, but still in the live region, and its
  // button takes neither clicks nor the keyboard.
  await page.waitForFunction(() => getComputedStyle(document.getElementById('toast')).opacity === '0');
  assert.equal(await page.$eval('#toast', (el) => el.getAttribute('aria-live')), 'polite');
  assert.equal(await page.$eval('#toast .toast-action', (el) => [getComputedStyle(el).pointerEvents, el.tabIndex].join()), 'none,-1');
  const first = '#notices-menu .message:first-child';
  assert.equal(await page.textContent(first + ' .message-text'), DELETED);
  assert.equal(await page.getAttribute(first + ' [data-action="message-undo"]', 'aria-label'), 'Undo Delete Tab');
  assert.equal(await page.getAttribute(first + ' [data-action="message-undo"]', 'title'), 'Deleted Code.');

  // A change after it takes its Undo, which the new one's message has.
  await fake.emit('bench:toast', 'Saved.');
  await page.waitForFunction(() => document.querySelector('#notices-menu .message:first-child .message-text').textContent === 'Saved.');
  assert.equal(await page.$(first + ' [data-action="message-undo"]'), null);
  assert.equal(await page.$$eval('#notices-menu [data-action="message-undo"]', (els) => els.length), 1);
  await page.click('#notices-menu [data-action="message-undo"]');
  assert.deepEqual((await fake.waitForCall('Undo')).args, [1]);
  // The list stays open, with what Undo said at its top, and no Undo.
  await page.waitForFunction(() => document.querySelector('#notices-menu .message:first-child .message-text').textContent === 'Brought back Code.');
  assert.equal(await page.$('#notices-menu [data-action="message-undo"]'), null);
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'notices-list');

  // Once the list closes, a toast with time left shows again.
  await page.keyboard.press('Escape');
  await page.waitForSelector('#notices-menu', { state: 'detached' });
  assert.equal(await page.$eval('#toast', (el) => el.classList.contains('under-list')), false);
  assert.equal(await toastText(page), 'Brought back Code.');
});

test('a recent message keeps its toast\'s Keep over Bench while Go can still keep that app over Bench, and the keyboard reaches it', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  const HID = 'Hid Finder because it covered Bench.';
  fake.world.keepOver = ['com.apple.finder'];
  fake.on('KeepOverBench', ([bundleID], f) => {
    f.world.keepOver = f.world.keepOver.filter((b) => b !== bundleID);
    f.changed();
    setTimeout(() => f.emit('bench:toast', 'Added a rule to config.toml: Finder windows now stay over Bench, and new ones stay out of your tabs.'), 0);
    return null;
  });
  await fake.emitState();
  await fake.emit('bench:toast', { text: HID, action: { label: 'Keep over Bench', method: 'KeepOverBench', item: 'com.apple.finder' } });
  await toastIs(page, HID);
  // Another toast takes the toast's place: the list still offers it.
  await fake.emit('bench:toast', 'Saved.');
  await toastIs(page, 'Saved.');
  await page.click('[data-action="notices"]');
  const keep = '#notices-menu .message:nth-child(2) [data-action="message-keep"]';
  await page.waitForSelector(keep);
  assert.equal(await page.textContent(keep), 'Keep over Bench');
  assert.match(await page.getAttribute(keep, 'aria-label'), /^Keep over Bench, for the message from \d\d:\d\d$/);
  assert.equal(await page.$('#notices-menu .message:first-child [data-action="message-keep"]'), null);
  // The keyboard reaches it, before the message's Copy.
  await page.focus('[data-key="notices-list"]');
  await page.keyboard.press('Tab');
  await page.keyboard.press('Tab');
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'message-keep-1');
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('KeepOverBench')).args, ['com.apple.finder']);
  // Go can't any more, so the button goes, and the list stays open.
  await page.waitForFunction(() => !document.querySelector('#notices-menu [data-action="message-keep"]'));
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'notices-list');
});

test('the keyboard reaches a recent message\'s Undo, in the order the buttons show, and Return takes the change back', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await deleteCode(page);
  await toastIs(page, DELETED);
  await fake.emit('bench:toast', 'Saved.');
  await page.click('[data-action="notices"]');
  await page.waitForSelector('#notices-menu [data-action="message-undo"]');
  const focused = () => page.evaluate(() => document.activeElement.dataset.key);
  // The messages are Saved., then Deleted Code. with Undo before its Copy.
  await page.focus('[data-key="notices-list"]');
  await page.keyboard.press('Tab');
  assert.equal(await focused(), 'message-0');
  await page.keyboard.press('Tab');
  assert.equal(await focused(), 'message-undo');
  await page.keyboard.press('ArrowDown');
  assert.equal(await focused(), 'message-1');
  await page.keyboard.press('ArrowUp');
  assert.equal(await focused(), 'message-undo');
  await page.keyboard.press('Shift+Tab');
  assert.equal(await focused(), 'message-0');
  await page.keyboard.press('ArrowLeft');
  assert.equal(await focused(), 'message-1');
  await page.keyboard.press('ArrowLeft');
  assert.equal(await focused(), 'message-undo');

  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('Undo')).args, [1]);
  await page.waitForFunction(() => document.querySelector('#notices-menu .message:first-child .message-text').textContent === 'Brought back Code.');
  assert.equal(await page.$('#notices-menu [data-action="message-undo"]'), null);
  // The list stays open, with the keyboard in it.
  assert.equal(await focused(), 'notices-list');
  await page.keyboard.press('Tab');
  assert.equal(await focused(), 'message-0');
});

test('Jump to… lists Undo first, with the undo shortcut, which Return with nothing typed passes by', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ lastTab: 0, keys: { undo: '⌃⌥U' } }) });
  t.after(close);
  await page.click('.tab[data-index="2"]', { button: 'right' });
  await page.waitForSelector('#tab-menu');
  await page.click('#tab-menu .menu-item:has-text("Reset Layout")');
  await page.waitForFunction(() => window.bench.store.view.undo);
  await page.click('[data-action="open-palette"]');
  await page.waitForSelector('.palette-item');
  const row = (i) => page.$eval('#palette-item-' + i, (el) => [el.querySelector('.palette-label').textContent, el.querySelector('.palette-hint').textContent,
    el.querySelector('.palette-keys').textContent, el.getAttribute('aria-selected')]);
  assert.deepEqual(await row(0), ['Undo Reset Layout', 'Reset the layout of Research', '⌃⌥U', 'false']);
  assert.equal(await page.textContent('.palette-group'), 'Last change');
  // The tab used last is selected, as it always is with nothing typed.
  assert.deepEqual((await row(1)).slice(0, 1).concat((await row(1))[3]), ['Comms', 'true']);
  await page.keyboard.press('ArrowUp');
  assert.equal((await row(0))[3], 'true');
  await page.keyboard.press('Enter');
  await fake.waitForCall('UndoLatest');
  await toastIs(page, 'Undid: reset the layout of Research.');
  await page.waitForFunction(() => !window.bench.store.view.undo);

  // With nothing to undo, there's no row for it.
  await page.click('[data-action="open-palette"]');
  await page.waitForSelector('.palette-item');
  assert.equal(await page.$('.palette-item:has-text("Undo")'), null);
  assert.deepEqual((await row(0)).slice(0, 1).concat((await row(0))[3]), ['Comms', 'true']);
});

test('in Jump to…, the name of the last change finds the command again before its Undo, and "undo" finds Undo', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld() });
  t.after(close);
  const run = async (query) => {
    await page.click('[data-action="open-palette"]');
    await page.waitForSelector('#palette-query');
    await page.fill('#palette-query', query);
    await page.waitForFunction((q) => window.bench.store.ui.palette.query === q && document.querySelector('#palette-item-0'), query);
    const first = await page.textContent('#palette-item-0 .palette-label');
    await page.press('#palette-query', 'Enter');
    return first;
  };
  assert.equal(await run('Also Show In New Tab'), 'Also Show In ▸ New Tab');
  await fake.waitForCall('MoveItemToNewTab', (a) => a[1] === true);
  await page.waitForFunction(() => (window.bench.store.view.undo || {}).name === 'Also Show In New Tab');
  // Again: the command, not Undo Also Show In New Tab, which comes after it.
  assert.equal(await run('Also Show In New Tab'), 'Also Show In ▸ New Tab');
  await page.waitForFunction(() => window.bench.store.view.tabs.length === 5);
  assert.equal(fake.callsTo('UndoLatest').length, 0);
  assert.equal(await run('undo'), 'Undo Also Show In New Tab');
  await fake.waitForCall('UndoLatest');
});

test('closing a page has Undo, which puts it back, and the tab menu and Jump to… offer to reopen what was closed', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ current: 0 }) });
  t.after(close);
  await page.hover('.subtab[data-item="w1"]');
  await page.click('.close-cross[data-action="close-item"][data-item="w1"]');
  assert.deepEqual((await fake.waitForCall('CloseItem')).args.slice(0, 1), ['w1']);
  await toastIs(page, 'Closed Slack — # general.');
  assert.equal(await page.textContent('#toast .toast-action'), 'Undo');
  await page.waitForFunction(() => !window.bench.store.view.items.w1);
  await page.click('#toast .toast-action');
  await toastIs(page, 'Reopened Slack — # general.');
  await page.waitForFunction(() => window.bench.store.view.items.w1 && !window.bench.store.view.closedItems);

  // Closed again, the tab's menu offers to reopen it, by the name the
  // toast gave it, with its app.
  fake.clearCalls();
  await page.hover('.subtab[data-item="w1"]');
  await page.click('.close-cross[data-action="close-item"][data-item="w1"]');
  await page.waitForFunction(() => (window.bench.store.view.closedItems || []).length === 1);
  await page.click('.tab[data-index="0"]', { button: 'right' });
  await page.waitForSelector('#tab-menu');
  assert.equal(await page.textContent('#tab-menu .menu-item:has-text("Reopen Closed Page") .menu-detail'), 'Slack — # general');
  await page.keyboard.press('Escape');

  // Jump to… lists it under Actions, with its tab, and finds it.
  await page.click('[data-action="open-palette"]');
  await page.fill('#palette-query', 'reopen');
  const rows = await page.$$eval('.palette-item', (els) => els.map((el) => [el.querySelector('.palette-label').textContent, el.querySelector('.palette-hint').textContent]));
  assert.deepEqual(rows, [['Reopen Closed Page', 'Slack — # general · Comms']]);
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('ReopenItem')).args, ['w1']);
  await toastIs(page, 'Reopened Slack — # general.');
  await page.waitForFunction(() => window.bench.store.view.items.w1);
  const v = await page.evaluate(() => window.bench.store.view);
  assert.deepEqual(v.tabs[0].root.children[0].items, ['w1']);
});
