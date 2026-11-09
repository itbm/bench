'use strict';

// Drops on Bench's window (drops.js, decision 73): the elements that take
// files from Finder and say what they are, the highlight while files are
// over one, the paths typed into a terminal, and the guard that keeps a
// link dropped on the page from loading in place of Bench.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, openTerminalWindow, settle } = require('./helpers/harness.js');
const { defaultWorld } = require('./helpers/fixture.js');

const NOT_FILES = 'Bench takes files, folders, and apps from the Finder.';
const NOT_FINDER = 'Save it as a file first, then drag the file from the Finder.';

// appWails runs before the page, and stands in for what Wails sets up in
// the app's windows that take drops: the flag enableFileDrop; its native
// handlers, of which handlePlatformFileDrop records what it's given in
// window.__native; and its runtime's listener for HTML drags of files on
// the document's element, which lights up what they're over and cancels
// them (setupDropTargetListeners in Wails' window.ts).
function appWails() {
  window.__native = [];
  window._wails = {
    flags: { enableFileDrop: true },
    handlePlatformFileDrop: (paths, x, y) => window.__native.push([paths, x, y]),
    handleDragEnter() {},
    handleDragOver() {},
    handleDragLeave() {},
  };
  const light = (e) => {
    if (!e.dataTransfer || !Array.from(e.dataTransfer.types).includes('Files')) return;
    e.preventDefault();
    const el = document.elementFromPoint(e.clientX, e.clientY);
    const target = el && el.closest('[data-file-drop-target]');
    document.querySelectorAll('.file-drop-target-active').forEach((x) => x.classList.remove('file-drop-target-active'));
    if (target && e.type !== 'drop') target.classList.add('file-drop-target-active');
    e.dataTransfer.dropEffect = target ? 'copy' : 'none';
  };
  const listen = () => {
    for (const type of ['dragenter', 'dragover', 'drop']) document.documentElement.addEventListener(type, light);
  };
  if (document.documentElement) listen();
  else document.addEventListener('DOMContentLoaded', listen);
}

// ready waits until the page handles drops as Wails' native code makes
// them.
function ready(page) {
  return page.waitForFunction(() => window._wails && window._wails.handlePlatformFileDrop
    && window._wails.handlePlatformFileDrop.name === 'benchDrop');
}

// centre returns the middle of the first element that matches selector.
async function centre(page, selector) {
  const b = await page.locator(selector).first().boundingBox();
  return { x: Math.round(b.x + b.width / 2), y: Math.round(b.y + b.height / 2) };
}

// drop drops paths on the middle of an element, as Wails' native code does.
async function drop(page, paths, selector) {
  const at = await centre(page, selector);
  await page.evaluate(([p, x, y]) => window._wails.handlePlatformFileDrop(p, x, y), [paths, at.x, at.y]);
}

// dropped returns what the latest DemoDropFiles call said the drop was on.
async function dropped(fake, n) {
  await fake.waitForCall('DemoDropFiles', () => fake.callsTo('DemoDropFiles').length >= n);
  const [paths, attrs] = fake.callsTo('DemoDropFiles')[n - 1].args;
  return { paths, kind: attrs['data-file-drop-target'], item: attrs['data-drop-item'], pane: attrs['data-drop-pane'] };
}

// inputTo returns everything typed into a terminal, in order.
function inputTo(fake, id) {
  return fake.callsTo('TermInput').filter((c) => c.args[0] === id).map((c) => c.args[1]).join('');
}

// dragText sends HTML drag events with a link at an element, as WebKit
// does for a link dragged from a browser, or, with inside, for a drag that
// started in the page, and returns whether the page cancelled the drop.
// With file, the drag holds a file instead, as WebKit gives one with no
// path, such as an image dragged from a browser, and lit says whether
// anything was lit up while it was over the element.
function dragText(page, selector, { inside = false, file = false } = {}) {
  return page.evaluate(([sel, from, withFile]) => {
    const target = document.querySelector(sel);
    const r = target.getBoundingClientRect();
    const at = { clientX: r.left + r.width / 2, clientY: r.top + r.height / 2 };
    const dt = new DataTransfer();
    if (withFile) {
      dt.items.add(new File(['png'], 'photo.png', { type: 'image/png' }));
    } else {
      dt.setData('text/uri-list', 'https://example.com/');
      dt.setData('text/plain', 'https://example.com/');
    }
    // Chromium keeps the dropEffect of a DataTransfer made by a page as
    // 'none', so this one records what the page sets instead.
    let effect = 'none';
    Object.defineProperty(dt, 'dropEffect', { get: () => effect, set: (v) => { effect = v; }, configurable: true });
    if (from) target.dispatchEvent(new DragEvent('dragstart', { dataTransfer: dt, bubbles: true, cancelable: true, ...at }));
    const fire = (type) => target.dispatchEvent(new DragEvent(type, { dataTransfer: dt, bubbles: true, cancelable: true, ...at }));
    const over = !fire('dragenter') & !fire('dragover');
    const lit = document.querySelectorAll('.file-drop-target-active').length > 0;
    const dropped = !fire('drop');
    if (from) target.dispatchEvent(new DragEvent('dragend', { dataTransfer: dt, bubbles: true, ...at }));
    return withFile ? { over: Boolean(over), dropped, effect, lit } : { over: Boolean(over), dropped, effect };
  }, [selector, inside, file]);
}

// toastShown returns the toast's text while it shows, or ''.
function toastShown(page) {
  return page.evaluate(() => {
    const el = document.getElementById('toast');
    return el.classList.contains('show') ? (el.querySelector('.toast-text') || el).textContent : '';
  });
}

// waitForToast waits until the toast shows text.
function waitForToast(page, text) {
  return page.waitForFunction((want) => {
    const el = document.getElementById('toast');
    return el.classList.contains('show') && el.textContent === want;
  }, text);
}

test('the panes, the terminals, and the tabs\' + say what a drop on them is for', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.waitForSelector('.pane[data-pane-id="p4"] .xterm');
  await ready(page);
  const attrs = (sel) => page.$eval(sel, (el) => [el.dataset.fileDropTarget, el.dataset.dropItem || el.dataset.dropPane || '']);
  assert.deepEqual(await attrs('.pane[data-pane-id="p3"] .pane-header'), ['pane', 'p3']);
  assert.deepEqual(await attrs('.pane[data-pane-id="p4"] .pane-header'), ['pane', 'p4']);
  assert.deepEqual(await attrs('.term-host[data-term-host="w5"]'), ['terminal', 'w5']);
  assert.deepEqual(await attrs('.pane[data-pane-id="p4"] .pane-body'), ['terminal', 'w5']);
  assert.deepEqual(await attrs('.new-tab'), ['new-tab', '']);
  assert.deepEqual(await attrs('body'), ['window', '']);
  // A docked app's body takes no drop of its own: its window is the app's.
  assert.equal(await page.$eval('.pane[data-pane-id="p3"] .pane-body', (el) => el.dataset.fileDropTarget || ''), '');

  await drop(page, ['/Users/me/code/site'], '.pane[data-pane-id="p3"] .pane-header');
  assert.deepEqual(await dropped(fake, 1), { paths: ['/Users/me/code/site'], kind: 'pane', item: undefined, pane: 'p3' });
  await drop(page, ['/Users/me/a b.txt', '/Users/me/c.png'], '.pane[data-pane-id="p4"] .xterm');
  assert.deepEqual(await dropped(fake, 2), { paths: ['/Users/me/a b.txt', '/Users/me/c.png'], kind: 'terminal', item: 'w5', pane: undefined });
  await drop(page, ['/Applications/Zed.app'], '.new-tab');
  assert.equal((await dropped(fake, 3)).kind, 'new-tab');
  await drop(page, ['/Users/me/a.txt'], '.pane[data-pane-id="p3"] .pane-body');
  assert.equal((await dropped(fake, 4)).kind, 'window');
  await drop(page, ['/Users/me/a.txt'], '#statusbar');
  assert.equal((await dropped(fake, 5)).kind, 'window');
});

test('an empty pane, and the "tabs" view\'s pane in the title bar, take folders and apps', async (t) => {
  const world = defaultWorld();
  world.tabs[1].root.children[1] = { type: 'pane', id: 'p4', items: [], active: '' };
  const { page, close } = await openBench({ world });
  t.after(close);
  await page.waitForSelector('.pane[data-pane-id="p4"] .empty');
  assert.deepEqual(await page.$eval('.pane[data-pane-id="p4"] .pane-body', (el) => [el.dataset.fileDropTarget, el.dataset.dropPane]),
    ['pane', 'p4']);

  const tabs = await openBench({ world: defaultWorld({ screenWidth: 1512, current: 0 }), viewport: { width: 1280, height: 800 } });
  t.after(tabs.close);
  await tabs.page.waitForSelector('.title-pane');
  assert.deepEqual(await tabs.page.$eval('.title-pane', (el) => [el.dataset.fileDropTarget, el.dataset.dropPane]), ['pane', 'compact']);
});

test('a pane whose window keeps moving takes folders and apps, as its header does, and docks nothing', async (t) => {
  // Brave's window keeps moving out of p5 (decision 69): the pane says so,
  // with Dock again, and a drop from the Finder there adds to the pane.
  // Only Dock again, or the window dragged onto a pane, docks it again.
  const world = defaultWorld({ current: 2 });
  world.items.w7.status = 'moving';
  world.items.w7.statusText = 'Brave or Rectangle keeps moving this window';
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await page.waitForSelector('.pane[data-pane-id="p5"] .pane-status[data-status="moving"]');
  await ready(page);
  assert.deepEqual(await page.$eval('.pane[data-pane-id="p5"] .pane-body', (el) => [el.dataset.fileDropTarget, el.dataset.dropPane]),
    ['pane', 'p5']);
  await drop(page, ['/Users/me/code/site'], '.pane[data-pane-id="p5"] .status-text');
  assert.deepEqual(await dropped(fake, 1), { paths: ['/Users/me/code/site'], kind: 'pane', item: undefined, pane: 'p5' });
  await drop(page, ['/Applications/Zed.app'], '.pane[data-pane-id="p5"] [data-action="redock"]');
  assert.deepEqual(await dropped(fake, 2), { paths: ['/Applications/Zed.app'], kind: 'pane', item: undefined, pane: 'p5' });
  await settle(page);
  assert.equal(fake.callsTo('Redock').length, 0);
});

test('files over a header, a terminal, or the tabs\' + light it up until they leave', async (t) => {
  for (const scheme of ['dark', 'light']) {
    const { page, close } = await openBench({ contextOptions: { colorScheme: scheme, forcedColors: 'none' } });
    t.after(close);
    await page.waitForSelector('.pane[data-pane-id="p4"] .xterm');
    await ready(page);
    const over = async (sel) => {
      const at = await centre(page, sel);
      await page.evaluate(([x, y]) => { window._wails.handleDragEnter(); window._wails.handleDragOver(x, y); }, [at.x, at.y]);
    };
    const lit = () => page.$$eval('.file-drop-target-active', (els) => els.map((el) => el.dataset.fileDropTarget));
    const header = '.pane[data-pane-id="p3"] .pane-header';
    const before = await page.$eval(header, (el) => getComputedStyle(el).backgroundColor);
    await over(header);
    assert.deepEqual(await lit(), ['pane']);
    assert.notEqual(await page.$eval(header, (el) => getComputedStyle(el).backgroundColor), before, scheme + ': the header is washed');
    await over('.pane[data-pane-id="p4"] .xterm');
    assert.deepEqual(await lit(), ['terminal'], 'one thing at a time');
    // The terminal's frame sits over what it shows.
    const frame = await page.$eval('.term-host.file-drop-target-active', (el) => {
      const s = getComputedStyle(el, '::after');
      return [s.content, s.borderTopWidth, s.zIndex];
    });
    assert.deepEqual(frame, ['""', '2px', '13']);
    await over('.new-tab');
    assert.deepEqual(await lit(), ['new-tab']);
    await page.evaluate(() => window._wails.handleDragLeave());
    assert.deepEqual(await lit(), []);
  }
});

test('with Increase contrast, what files are over has a thicker frame', async (t) => {
  const { page, close } = await openBench({ contextOptions: { contrast: 'more' } });
  t.after(close);
  await page.waitForSelector('.pane[data-pane-id="p4"] .xterm');
  await ready(page);
  const at = await centre(page, '.pane[data-pane-id="p4"] .xterm');
  await page.evaluate(([x, y]) => window._wails.handleDragOver(x, y), [at.x, at.y]);
  assert.equal(await page.$eval('.term-host.file-drop-target-active', (el) => getComputedStyle(el, '::after').borderTopWidth), '3px');
});

test('Go\'s paste-terminal types the paths into the terminal that it names, as a paste', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.waitForSelector('.pane[data-pane-id="p4"] .xterm');
  await fake.waitForCall('TermResize');
  await settle(page);
  await fake.emit('bench:command', { name: 'paste-terminal', itemId: 'w5', text: '/Users/me/Q4\\ report.pdf ' });
  await fake.waitForCall('TermInput', (a) => a[0] === 'w5');
  assert.equal(inputTo(fake, 'w5'), '/Users/me/Q4\\ report.pdf ');

  // A program that asks for pastes to be marked gets them marked.
  await fake.termOut('w5', '\x1b[?2004h');
  await settle(page);
  await fake.emit('bench:command', { name: 'paste-terminal', itemId: 'w5', text: '/tmp/a ' });
  await fake.waitForCall('TermInput', (a) => a[0] === 'w5' && a[1].includes('/tmp/a'));
  assert.equal(inputTo(fake, 'w5'), '/Users/me/Q4\\ report.pdf \x1b[200~/tmp/a \x1b[201~');

  // A terminal that this window doesn't show isn't typed into here.
  await fake.emit('bench:command', { name: 'paste-terminal', itemId: 'w6', text: '/tmp/b ' });
  await settle(page);
  assert.equal(fake.callsTo('TermInput').filter((c) => c.args[0] !== 'w5').length, 0);
});

test('a link dropped on the window says what Bench takes, and the page stays', async (t) => {
  const { page, close } = await openBench();
  t.after(close);
  await ready(page);
  const url = page.url();
  const r = await dragText(page, '#titlebar');
  assert.deepEqual(r, { over: true, dropped: true, effect: 'copy' });
  await page.waitForFunction((want) => document.getElementById('toast').textContent.includes(want), NOT_FILES);
  assert.equal(await toastShown(page), NOT_FILES);
  assert.equal(page.url(), url);

  // A drag that started in the page lands nowhere, and says nothing.
  await page.evaluate(() => { document.getElementById('toast').classList.remove('show'); });
  const inside = await dragText(page, '.pane[data-pane-id="p3"] .pane-header', { inside: true });
  assert.equal(inside.dropped, true);
  assert.equal(inside.effect, 'none');
  await settle(page);
  assert.equal(await toastShown(page), '');

  // A drag of a tab's text, which the page stops as it starts, never
  // starts, so a link from another app after it still gets the message.
  const stopped = await page.$eval('.tab', (el) => !el.dispatchEvent(new DragEvent('dragstart', {
    dataTransfer: new DataTransfer(), bubbles: true, cancelable: true,
  })));
  assert.equal(stopped, true);
  assert.equal((await dragText(page, '#statusbar')).effect, 'copy');
  await page.waitForFunction((want) => {
    const el = document.getElementById('toast');
    return el.classList.contains('show') && el.textContent.includes(want);
  }, NOT_FILES);

  // A field still takes text dropped in it.
  await page.evaluate(() => { document.getElementById('toast').classList.remove('show'); });
  await page.click('[data-action="open-palette"]');
  await page.waitForSelector('#palette-query');
  const field = await dragText(page, '#palette-query');
  assert.deepEqual([field.over, field.dropped], [false, false]);
});

test('a popped-out terminal\'s window takes files anywhere in it, and types their paths', async (t) => {
  const { page, fake, close } = await openTerminalWindow({ id: 'w5' });
  t.after(close);
  await page.waitForSelector('.terminal-window .xterm');
  await ready(page);
  assert.deepEqual(await page.$eval('body', (el) => [el.dataset.fileDropTarget, el.dataset.dropItem]), ['terminal', 'w5']);
  await drop(page, ['/Users/me/notes.txt'], '.term-window-bar .term-window-note');
  assert.deepEqual(await dropped(fake, 1), { paths: ['/Users/me/notes.txt'], kind: 'terminal', item: 'w5', pane: undefined });
  await drop(page, ['/Users/me/notes.txt'], '.terminal-window .xterm');
  assert.equal((await dropped(fake, 2)).item, 'w5');

  await fake.emit('bench:command', { name: 'paste-terminal', itemId: 'w5', text: '/Users/me/notes.txt ' });
  await fake.waitForCall('TermInput', (a) => a[0] === 'w5');
  assert.equal(inputTo(fake, 'w5'), '/Users/me/notes.txt ');
});

test('in the app, a drop goes to Wails\' runtime, which sends it to Go, and not to the preview\'s DemoDropFiles', async (t) => {
  const { page, fake, close } = await openBench({ init: appWails });
  t.after(close);
  await page.waitForSelector('.pane[data-pane-id="p4"] .xterm');
  await ready(page);
  const at = await centre(page, '.pane[data-pane-id="p4"] .xterm');
  await page.evaluate(([x, y]) => window._wails.handlePlatformFileDrop(['/Users/me/a b.txt'], x, y), [at.x, at.y]);
  assert.deepEqual(await page.evaluate(() => window.__native), [[['/Users/me/a b.txt'], at.x, at.y]]);
  await settle(page);
  assert.equal(fake.callsTo('DemoDropFiles').length, 0);
});

test('in the app, files from another app, which have no paths, light nothing up, and a drop says how to drop them', async (t) => {
  const { page, close } = await openBench({ init: appWails });
  t.after(close);
  await page.waitForSelector('.pane[data-pane-id="p4"] .xterm');
  await ready(page);
  const url = page.url();
  for (const sel of ['.pane[data-pane-id="p4"] .term-host', '.pane[data-pane-id="p3"] .pane-header', '.new-tab']) {
    await page.evaluate(() => { document.getElementById('toast').classList.remove('show'); });
    assert.deepEqual(await dragText(page, sel, { file: true }), { over: true, dropped: true, effect: 'copy', lit: false }, sel);
    await waitForToast(page, NOT_FINDER);
  }
  assert.equal(page.url(), url);
  // A link still says what Bench takes.
  assert.equal((await dragText(page, '#titlebar')).effect, 'copy');
  await waitForToast(page, NOT_FILES);
  // A drag that started in the page still lands nowhere.
  await page.evaluate(() => { document.getElementById('toast').classList.remove('show'); });
  const inside = await dragText(page, '.pane[data-pane-id="p3"] .pane-header', { file: true, inside: true });
  assert.deepEqual([inside.effect, inside.lit], ['none', false]);
  await settle(page);
  assert.equal(await toastShown(page), '');
});

test('a drag whose source a new view took out of the page doesn\'t stop a later drop saying what Bench takes', async (t) => {
  const { page, close } = await openBench();
  t.after(close);
  await ready(page);
  // Text dragged from the status bar, which the next view draws again:
  // WebKit sends the drag's dragend to its source, out of the page.
  await page.evaluate(() => {
    const el = document.createElement('span');
    el.textContent = 'Selected text';
    document.getElementById('statusbar').append(el);
    const dt = new DataTransfer();
    dt.setData('text/plain', 'Selected text');
    el.dispatchEvent(new DragEvent('dragstart', { dataTransfer: dt, bubbles: true, cancelable: true }));
    el.remove();
    el.dispatchEvent(new DragEvent('dragend', { dataTransfer: dt, bubbles: true }));
  });
  assert.equal((await dragText(page, '#titlebar')).effect, 'copy');
  await waitForToast(page, NOT_FILES);

  // A press of the pointer ends a drag whose end went unheard.
  await page.evaluate(() => {
    document.getElementById('toast').classList.remove('show');
    const dt = new DataTransfer();
    dt.setData('text/plain', 'Selected text');
    document.getElementById('statusbar').dispatchEvent(new DragEvent('dragstart', { dataTransfer: dt, bubbles: true, cancelable: true }));
  });
  assert.equal((await dragText(page, '#titlebar')).effect, 'none');
  await page.evaluate(() => document.body.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true })));
  assert.equal((await dragText(page, '#titlebar')).effect, 'copy');
});

test('the messages after a drop fit the app\'s toast, on one line, in the smallest window', async (t) => {
  // In the title bar too, while the status bar is hidden.
  for (const showStatusBar of [true, false]) {
    const world = defaultWorld();
    world.settings.showStatusBar = showStatusBar;
    const { page, fake, close } = await openBench({ world, viewport: { width: 640, height: 480 } });
    t.after(close);
    // The longest of Go's, which its tests keep to 64 characters at most.
    for (const text of [NOT_FILES, NOT_FINDER, 'The command has finished. Press Return, then drop again.',
      'Drop at most 8 folders and apps at once. Bench opened none.',
      'Drop on a terminal, a pane\'s header, or the + after the tabs.']) {
      await fake.emit('bench:toast', text);
      await waitForToast(page, text);
      const fit = await page.$eval('#toast', (el) => [el.scrollWidth <= el.clientWidth, getComputedStyle(el).whiteSpace,
        el.classList.contains('in-titlebar')]);
      assert.deepEqual(fit, [true, 'nowrap', !showStatusBar], text);
    }
  }
});

test('a popped-out terminal\'s window shows why Go refused a drop on it, and only its own', async (t) => {
  const { page, fake, close } = await openTerminalWindow({ id: 'w5' });
  t.after(close);
  await page.waitForSelector('.terminal-window .xterm');
  await fake.emit('bench:command', { name: 'drop-refused', itemId: 'w6', text: 'Not this one.' });
  await settle(page);
  assert.equal(await toastShown(page), '');
  const text = 'The command has finished. Press Return, then drop again.';
  await fake.emit('bench:command', { name: 'drop-refused', itemId: 'w5', text });
  await waitForToast(page, text);

  // Bench's window leaves it to the terminal's.
  const main = await openBench();
  t.after(main.close);
  await main.fake.emit('bench:command', { name: 'drop-refused', itemId: 'w5', text });
  await settle(main.page);
  assert.equal(await toastShown(main.page), '');
});
