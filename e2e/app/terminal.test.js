'use strict';

// Terminals: typing into the preview's pretend terminal (term.Echo), and a
// popped-out terminal's own window.

const assert = require('node:assert/strict');
const test = require('node:test');
const {
  eventsOpen, itemID, openPage, panesOf, setScreen, startPreview, switchTo, waitView,
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
  await page.locator('#window-menu .menu-item', { hasText: 'Pop out of Bench' }).click();
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
  const host = await win.locator('.terminal-window .term-host').boundingBox();
  assert.deepEqual([host.width, host.height], [760, 480]);
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
