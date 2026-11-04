'use strict';

// Terminals: typing into the preview's pretend terminal (term.Echo), and a
// popped-out terminal's own window, the bell that marks a terminal that
// needs you, and a terminal's folder and output kept between launches.

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const {
  eventsOpen, itemID, openPage, panesOf, setScreen, startPreview, switchTo, tabIndex, waitForToast, waitView,
} = require('./helpers/preview.js');

// rowsText returns the text of the terminal in an element.
function rowsText(page, selector) {
  return page.$eval(selector + ' .xterm-rows', (el) => el.textContent);
}

// waitForText waits until the terminal in an element shows text.
function waitForText(page, selector, text) {
  return page.waitForFunction(([sel, t]) => {
    const rows = document.querySelector(sel + ' .xterm-rows');
    return Boolean(rows && rows.textContent.includes(t));
  }, [selector, text]);
}

test('typing into a terminal shows its output, which survives switching tabs', async (t) => {
  const preview = await startPreview({ t });
  const { page, close } = await openPage(preview);
  t.after(close);
  await setScreen(page, 'monitor');
  const v = await switchTo(page, 'Code');
  const term = itemID(v, 'zsh — ~/code');
  const body = '[data-term="' + term + '"]';
  await page.waitForSelector(body + ' .xterm');
  await page.click(body + ' .xterm');
  await page.keyboard.type('echo hello from bench');
  await page.keyboard.press('Enter');
  // What's typed reaches the session in order, and its output comes back.
  await page.waitForFunction((sel) => {
    const text = document.querySelector(sel + ' .xterm-rows').textContent;
    return text.split('hello from bench').length - 1 >= 2;
  }, body);

  await switchTo(page, 'Comms');
  assert.equal(await page.$(body), null);
  await switchTo(page, 'Code');
  await waitForText(page, body, 'echo hello from bench');
  assert.equal(await page.evaluate(() => window.bench.terminalCount()), 1);
});

test('a popped-out terminal gets a window of its own showing just the terminal', async (t) => {
  const preview = await startPreview({ t });
  const { page, close } = await openPage(preview);
  t.after(close);
  await setScreen(page, 'monitor');
  let v = await switchTo(page, 'Code');
  const term = itemID(v, 'zsh — ~/code');
  const [mainPane, terminalPane] = panesOf(v, 'Code');
  await page.click('[data-term="' + term + '"] .xterm');
  await page.keyboard.type('echo before');
  await page.keyboard.press('Enter');

  await page.click('[data-action="menu"][data-pane="' + terminalPane.id + '"]');
  await page.locator('#window-menu .menu-item', { hasText: 'Pop Out of Bench' }).click();
  v = await waitView(page, (x, a) => (x.floating || []).some((f) => f.id === a), term);
  assert.equal(v.display.panes.length, 1);

  // The shell opens the terminal's window at /?terminal=<id>. The browser
  // preview has no windows of its own, so open the page here.
  const popped = await openPage(preview, { path: '/?terminal=' + encodeURIComponent(term), viewport: { width: 760, height: 480 } });
  t.after(popped.close);
  const win = popped.page;
  await waitForText(win, '.terminal-window', 'echo before');
  assert.equal(await win.$('#titlebar'), null);
  assert.equal(await win.isVisible('#demo-bar'), false);
  // Below the bar with Back to Bench.
  const host = await win.locator('.terminal-window .term-host').boundingBox();
  assert.deepEqual([host.width, host.height], [760, 444]);
  await win.keyboard.type('echo in its own window');
  await win.keyboard.press('Enter');
  await win.waitForFunction(() => document.querySelector('.terminal-window .xterm-rows').textContent
    .split('in its own window').length - 1 >= 2);
  // Its title follows the terminal's, from the next bench:state.
  await eventsOpen(win);
  await switchTo(page, 'Research');
  await win.waitForFunction(() => /^Terminal — /.test(document.title));
  await switchTo(page, 'Code');

  // Adding it back to a pane shows the same session there.
  await page.click('[data-action="add"][data-pane="' + mainPane.id + '"]');
  await page.locator('.palette-item', { hasText: 'zsh — ~/code' }).click();
  v = await waitView(page, (x, a) => x.items[a] && x.items[a].tab === 1, term);
  await waitForText(page, '[data-term="' + term + '"]', 'echo in its own window');
  assert.ok((await rowsText(page, '[data-term="' + term + '"]')).includes('echo before'));
});

test('a popped-out terminal\'s Back to Bench puts it back in the focused pane, with Pop out to take it out again', async (t) => {
  const preview = await startPreview({ t });
  const { page, close } = await openPage(preview);
  t.after(close);
  await setScreen(page, 'monitor');
  let v = await switchTo(page, 'Code');
  const term = itemID(v, 'zsh — ~/code');
  const [mainPane, terminalPane] = panesOf(v, 'Code');
  await page.click('[data-action="menu"][data-pane="' + terminalPane.id + '"]');
  await page.locator('#window-menu .menu-item', { hasText: 'Pop Out of Bench' }).click();
  await waitView(page, (x, a) => (x.floating || []).some((f) => f.id === a), term);

  const popped = await openPage(preview, { path: '/?terminal=' + encodeURIComponent(term), viewport: { width: 760, height: 480 } });
  t.after(popped.close);
  const win = popped.page;
  await win.waitForSelector('.terminal-window .xterm');
  await win.click('[data-key="back-to-bench"]');
  // It joins the focused pane, the one left, and the toast's Pop out takes
  // it out again.
  const code = tabIndex(v, 'Code');
  v = await waitView(page, (x, [a, ti]) => x.items[a] && x.items[a].tab === ti, [term, code]);
  assert.equal(v.items[term].pane, mainPane.id);
  await waitForToast(page, /^Added .* to this pane\.$/);
  assert.equal(await page.textContent('#toast .toast-action'), 'Pop out');
  await page.click('#toast .toast-action');
  await waitView(page, (x, a) => (x.floating || []).some((f) => f.id === a), term);
});

test('focusing a terminal\'s pane from the keyboard gives the terminal the keyboard focus', async (t) => {
  const preview = await startPreview({ t });
  const { page, close } = await openPage(preview);
  t.after(close);
  await setScreen(page, 'monitor');
  const v = await switchTo(page, 'Code');
  const [mainPane, terminalPane] = panesOf(v, 'Code');
  const inTerminal = () => page.evaluate((id) => {
    const el = document.activeElement;
    return Boolean(el && el.closest('.pane[data-pane-id="' + id + '"] .term-host'));
  }, terminalPane.id);
  await page.click('.pane[data-pane-id="' + mainPane.id + '"] .pane-body');
  await waitView(page, (x, id) => x.display.focus === id, mainPane.id);
  assert.equal(await inTerminal(), false);

  // Control+Option+Right focuses the terminal's pane: Go asks the UI to
  // focus the terminal, so typing goes straight to it.
  await page.keyboard.press('Control+Alt+ArrowRight');
  await waitView(page, (x, id) => x.display.focus === id, terminalPane.id);
  await page.waitForFunction((id) => Boolean(document.activeElement.closest('.pane[data-pane-id="' + id + '"] .term-host')), terminalPane.id);
  await page.keyboard.type('echo by keyboard');
  await page.keyboard.press('Enter');
  await waitForText(page, '.pane[data-pane-id="' + terminalPane.id + '"]', 'echo by keyboard');

  // Control+Option+Left focuses the other pane, and the terminal lets go.
  await page.keyboard.press('Control+Alt+ArrowLeft');
  await waitView(page, (x, id) => x.display.focus === id, mainPane.id);
  await page.waitForFunction((id) => !document.activeElement.closest('.pane[data-pane-id="' + id + '"] .term-host'), terminalPane.id);
});

test('a terminal in another tab that asks for a notification has a bell on its sub-tab and tab, until it\'s shown', async (t) => {
  const preview = await startPreview({ t });
  const { page, close } = await openPage(preview);
  t.after(close);
  await setScreen(page, 'monitor');
  let v = await switchTo(page, 'Code');
  const term = itemID(v, 'zsh — ~/code');
  const body = '[data-term="' + term + '"]';
  await page.click(body + ' .xterm');
  await page.keyboard.type('notify 1 Tests passed');
  await page.keyboard.press('Enter');
  await switchTo(page, 'Comms');

  v = await waitView(page, (x, id) => Boolean(x.items[id].attention), term);
  assert.equal(v.items[term].attention, 'Notification: Tests passed');
  assert.equal(v.tabs[tabIndex(v, 'Code')].attention, 1);
  const tab = '.tab[data-index="' + tabIndex(v, 'Code') + '"]';
  await page.waitForSelector(tab + ' .tab-bell');
  assert.equal(await page.getAttribute(tab + ' .tab-bell', 'title'), '1 window needs you');

  // Showing it takes the mark off.
  await switchTo(page, 'Code');
  v = await waitView(page, (x, id) => !x.items[id].attention, term);
  assert.equal(v.tabs[tabIndex(v, 'Code')].attention, undefined);
  await page.waitForFunction(() => !document.querySelector('.bell-mark'));
});

test('a shell starts again where it was last, after its output from before Bench stopped', async (t) => {
  const first = await startPreview({ t });
  const one = await openPage(first);
  await setScreen(one.page, 'monitor');
  let v = await switchTo(one.page, 'Code');
  const term = itemID(v, 'zsh — ~/code');
  const body = '[data-term="' + term + '"]';
  await one.page.click(body + ' .xterm');
  await one.page.keyboard.type('cd bench');
  await one.page.keyboard.press('Enter');
  await one.page.keyboard.type('echo kept for later');
  await one.page.keyboard.press('Enter');
  v = await waitView(one.page, (x, id) => x.items[id].here === '~/code/bench', term);
  assert.equal(v.items[term].folder, '~/code');
  await waitForText(one.page, body, 'kept for later');
  await one.close();
  assert.equal(await first.stop(), 0);

  // The output is kept, readable only by the user, and state.json has the
  // folder.
  const saved = path.join(first.home, 'terms', term + '.out');
  assert.ok(fs.readFileSync(saved, 'utf8').includes('kept for later'));
  assert.equal(fs.statSync(saved).mode & 0o777, 0o600);
  const state = JSON.parse(fs.readFileSync(path.join(first.home, 'state.json'), 'utf8'));
  assert.equal(state.items[term].lastFolder, '~/code/bench');

  const second = await startPreview({ home: first.home, t });
  const two = await openPage(second);
  t.after(two.close);
  await setScreen(two.page, 'monitor');
  v = await switchTo(two.page, 'Code');
  assert.equal(v.items[term].here, '~/code/bench');
  await waitForText(two.page, body, 'The program that wrote it has ended.');
  const text = await rowsText(two.page, body);
  assert.ok(text.includes('kept for later'), text);
  assert.match(text, /Restored output from \d\d:\d\d\. /);
  // The new shell is in the folder it was in last.
  assert.ok(text.lastIndexOf('~/code/bench') > text.indexOf('Restored output'), text);

  // Closing the terminal forgets its output.
  await two.page.hover('.subtab[data-item="' + term + '"]');
  await two.page.click('.subtab[data-item="' + term + '"] .close-cross');
  await waitView(two.page, (x, id) => !x.items[id], term);
  assert.equal(fs.existsSync(saved), false);
});
