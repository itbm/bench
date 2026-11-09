'use strict';

// Drops from Finder (decision 73), in the browser preview: its browser
// can't give the paths of files dropped on it, so each test calls the
// entry point that Wails' native code calls on the Mac,
// window._wails.handlePlatformFileDrop, with paths and a point, and the
// preview's frontend sends them to Go's DemoDropFiles, which reads the
// paths on this computer as the app reads them on the Mac.

const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const test = require('node:test');
const {
  itemID, openPage, panesOf, setScreen, startPreview, switchTo, tabIndex, view, waitForToast, waitView,
} = require('./helpers/preview.js');

// ready waits until the page handles drops as Wails' native code makes
// them.
function ready(page) {
  return page.waitForFunction(() => window._wails && window._wails.handlePlatformFileDrop
    && window._wails.handlePlatformFileDrop.name === 'benchDrop');
}

// drop drops paths on the middle of the first element that matches
// selector, as Wails' native code does.
async function drop(page, paths, selector) {
  const b = await page.locator(selector).first().boundingBox();
  const at = [Math.round(b.x + b.width / 2), Math.round(b.y + b.height / 2)];
  await page.evaluate(([p, x, y]) => window._wails.handlePlatformFileDrop(p, x, y), [paths, ...at]);
}

// waitForText waits until the terminal in an element shows text.
function waitForText(page, selector, text) {
  return page.waitForFunction(([sel, t]) => {
    const rows = document.querySelector(sel + ' .xterm-rows');
    return Boolean(rows && rows.textContent.includes(t));
  }, [selector, text]);
}

// short returns a folder as Bench shows it: in the home folder, from ~.
function short(folder) {
  const home = os.homedir();
  return folder.startsWith(home + path.sep) ? '~' + folder.slice(home.length) : folder;
}

// things makes, in the preview's support folder, what the tests drop: a
// folder, bench-site; a file, notes.txt; and an app, Figma.app, with the
// bundle ID of the demo's Figma, which is installed but not running.
function things(preview) {
  const dir = path.join(preview.home, 'dropped');
  const site = path.join(dir, 'bench-site');
  fs.mkdirSync(site, { recursive: true });
  const notes = path.join(dir, 'notes.txt');
  fs.writeFileSync(notes, 'notes\n');
  const figma = path.join(dir, 'Figma.app');
  fs.mkdirSync(path.join(figma, 'Contents'), { recursive: true });
  fs.writeFileSync(path.join(figma, 'Contents', 'Info.plist'), '<?xml version="1.0" encoding="UTF-8"?>\n'
    + '<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">\n'
    + '<plist version="1.0"><dict>'
    + '<key>CFBundleIdentifier</key><string>com.figma.Desktop</string>'
    + '<key>CFBundleName</key><string>Figma</string>'
    + '</dict></plist>\n');
  return { site, notes, figma };
}

// open starts a preview on the Code tab, with what the tests drop.
async function open(t) {
  const preview = await startPreview({ t });
  const { page, close } = await openPage(preview);
  t.after(close);
  await setScreen(page, 'monitor');
  const v = await switchTo(page, 'Code');
  await ready(page);
  return { preview, page, v, ...things(preview) };
}

test('files dropped on a terminal type their paths, quoted for the shell', async (t) => {
  const { page, v } = await open(t);
  const term = itemID(v, 'zsh — ~/code');
  const body = '[data-term="' + term + '"]';
  await page.waitForSelector(body + ' .xterm');
  await drop(page, ['/Users/demo/Projects/Q4 report (final).pdf', '/Users/demo/Pictures/café.jpg'], body);
  await waitForText(page, body, 'Q4\\ report\\ \\(final\\).pdf /Users/demo/Pictures/café.jpg');
  // Nothing else changes: the terminal is still the pane's.
  const after = await view(page);
  assert.equal(after.items[term].pane, v.items[term].pane);
});

test('a folder dropped on a pane\'s header opens a terminal in it there, and an app joins the pane', async (t) => {
  const { page, v, site, figma } = await open(t);
  const pane = panesOf(v, 'Code')[0];
  await drop(page, [site, figma], '.pane[data-pane-id="' + pane.id + '"] .pane-header');
  const after = await waitView(page, (x, a) => {
    const p = x.display.panes.find((q) => q.id === a.pane);
    const its = p ? p.items.map((i) => x.items[i]) : [];
    return its.some((it) => it.kind === 'builtin' && it.folder === a.folder)
      && its.some((it) => it.bundleId === 'com.figma.Desktop' && it.status === 'ready');
  }, { pane: pane.id, folder: short(site) });
  const p = after.display.panes.find((q) => q.id === pane.id);
  const terminal = p.items.map((i) => after.items[i]).find((it) => it.kind === 'builtin' && it.folder === short(site));
  await page.waitForSelector('.pane[data-pane-id="' + pane.id + '"] .subtab[data-item="' + terminal.id + '"]');
  // Each other pane keeps what it had.
  assert.deepEqual(after.display.panes.filter((q) => q.id !== pane.id).map((q) => q.items),
    v.display.panes.filter((q) => q.id !== pane.id).map((q) => q.items));
});

test('a folder dropped on the + after the tabs makes a new tab of it, which Undo takes away', async (t) => {
  const { page, v, site } = await open(t);
  await drop(page, [site], '.new-tab');
  await waitForToast(page, 'Made a new tab, bench-site.');
  let after = await waitView(page, (x) => x.tabs.some((tab) => tab.name === 'bench-site'));
  const ti = tabIndex(after, 'bench-site');
  assert.equal(ti, v.tabs.length);
  assert.equal(after.current, ti);
  const its = panesOf(after, 'bench-site').flatMap((p) => p.items).map((i) => after.items[i]);
  assert.deepEqual(its.map((it) => [it.kind, it.folder]), [['builtin', short(site)]]);
  await page.waitForSelector('.tab[data-index="' + ti + '"][aria-selected="true"]');

  assert.equal(await page.textContent('#toast .toast-action'), 'Undo');
  await page.click('#toast .toast-action');
  after = await waitView(page, (x) => !x.tabs.some((tab) => tab.name === 'bench-site'));
  assert.deepEqual(after.tabs.map((tab) => tab.name), v.tabs.map((tab) => tab.name));
  await waitForToast(page, 'Deleted the new tab, bench-site.');
});

test('a file dropped on a pane, or anything dropped away from a pane or terminal, says where to drop it', async (t) => {
  const { page, v, notes } = await open(t);
  const pane = panesOf(v, 'Code')[0];
  await drop(page, [notes], '.pane[data-pane-id="' + pane.id + '"] .pane-header');
  await waitForToast(page, 'Drop folders and apps here. Drop files on a terminal.');
  await drop(page, [notes], '#statusbar');
  await waitForToast(page, 'Drop on a terminal, a pane\'s header, or the + after the tabs.');
  const after = await view(page);
  assert.deepEqual(after.display.panes.map((q) => q.items), v.display.panes.map((q) => q.items));
  assert.equal(after.tabs.length, v.tabs.length);
});

test('an app that can\'t open, or an Xcode project, dropped on the + makes no tab, and says why', async (t) => {
  const { preview, page, v } = await open(t);
  const dir = path.join(preview.home, 'dropped');
  const ghost = path.join(dir, 'Ghost.app');
  fs.mkdirSync(path.join(ghost, 'Contents'), { recursive: true });
  fs.writeFileSync(path.join(ghost, 'Contents', 'Info.plist'), '<?xml version="1.0" encoding="UTF-8"?>\n'
    + '<plist version="1.0"><dict>'
    + '<key>CFBundleIdentifier</key><string>com.example.ghost</string>'
    + '<key>CFBundleName</key><string>Ghost</string>'
    + '</dict></plist>\n');
  const project = path.join(dir, 'MyApp.xcodeproj');
  fs.mkdirSync(project, { recursive: true });
  await drop(page, [ghost], '.new-tab');
  await waitForToast(page, 'Bench couldn\'t open Ghost.');
  await drop(page, [project], '.new-tab');
  await waitForToast(page, 'Drop folders and apps here. Drop files on a terminal.');
  const after = await view(page);
  assert.deepEqual(after.tabs.map((tab) => tab.name), v.tabs.map((tab) => tab.name));
  assert.equal(after.current, v.current);
  assert.equal(after.undo, v.undo);
});

test('a link dropped on the window says what Bench takes, and Bench\'s page stays', async (t) => {
  const { page } = await open(t);
  const url = page.url();
  await page.evaluate(() => {
    const bar = document.getElementById('titlebar');
    const r = bar.getBoundingClientRect();
    const at = { clientX: r.left + r.width * 0.55, clientY: r.top + r.height / 2 };
    const target = document.elementFromPoint(at.clientX, at.clientY) || bar;
    const dt = new DataTransfer();
    dt.setData('text/uri-list', 'https://example.com/');
    dt.setData('text/plain', 'https://example.com/');
    for (const type of ['dragenter', 'dragover', 'drop']) {
      target.dispatchEvent(new DragEvent(type, { dataTransfer: dt, bubbles: true, cancelable: true, ...at }));
    }
  });
  await waitForToast(page, 'Bench takes files, folders, and apps from the Finder.');
  assert.equal(page.url(), url);
});

test('files dropped on a popped-out terminal\'s window type their paths into it', async (t) => {
  const { preview, page, v } = await open(t);
  const term = itemID(v, 'zsh — ~/code');
  const terminalPane = panesOf(v, 'Code')[1];
  await page.click('[data-action="menu"][data-pane="' + terminalPane.id + '"]');
  await page.locator('#window-menu .menu-item', { hasText: 'Pop Out of Bench' }).click();
  await waitView(page, (x, a) => (x.floating || []).some((f) => f.id === a), term);

  const popped = await openPage(preview, { path: '/?terminal=' + encodeURIComponent(term), viewport: { width: 760, height: 480 } });
  t.after(popped.close);
  const win = popped.page;
  await win.waitForSelector('.terminal-window .xterm');
  await ready(win);
  // Anywhere in the window, even on its bar.
  await drop(win, ['/Users/demo/Ünïcödé café.jpg'], '.term-window-bar');
  await waitForText(win, '.terminal-window', '/Users/demo/Ünïcödé\\ café.jpg');
  // It's typed once, by the window that shows the terminal.
  await new Promise((r) => setTimeout(r, 500));
  const text = await win.$eval('.terminal-window .xterm-rows', (el) => el.textContent);
  assert.equal(text.split('café.jpg').length - 1, 1);
});
