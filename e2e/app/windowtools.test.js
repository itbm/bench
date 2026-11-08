'use strict';

// What fights Bench (decision 69), with the real engine. The demo bar's
// Simulate ▸ Rectangle runs Rectangle, a window tool, on the demo platform:
// the status bar says what it can do to Bench, with the keys its shortcuts
// share with Bench's, until the user dismisses it, and Settings ▸ Keyboard
// says which of Bench's shortcuts Rectangle uses too. Simulate ▸ Keep moving
// window has something move the focused pane's window out of the pane again
// and again, until Bench stops putting it back: its pane says so, with
// Dock again.

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const {
  TIMEOUT, itemID, openPage, paneOf, setScreen, startPreview, switchTo, waitUntil, waitView,
} = require('./helpers/preview.js');

const NOTICE = /^Rectangle is running\. It can move docked windows out of their panes\. It uses the same keys as Bench for Focus a pane \(⌃⌥←, ⌃⌥→, ⌃⌥↑, ⌃⌥↓\), .*Zoom pane \(⌃⌥↩\)\. macOS doesn't warn about this, and both apps may act on these keys\. Change the keys in Rectangle or in Keyboard settings\.$/;

// readState returns the preview's state.json.
function readState(preview) {
  return JSON.parse(fs.readFileSync(path.join(preview.home, 'state.json'), 'utf8'));
}

// rectangle presses the demo bar's Simulate ▸ Rectangle, and waits until Go
// says that Rectangle is running, or not.
async function rectangle(page, on) {
  await page.click('#demo-simulate');
  await page.click('#demo-window-tool');
  await page.waitForSelector('#demo-window-tool[aria-pressed="' + on + '"]', { state: 'attached' });
  return waitView(page, (x, a) => (x.windowTools || []).includes('Rectangle') === a, on);
}

// toolNotice returns the View's notice about Rectangle, if there is one.
function toolNotice(v) {
  return (v.notices || []).find((n) => n.text.startsWith('Rectangle is running')) || null;
}

test('Rectangle\'s notice names the keys it shares with Bench, until Dismiss, and Settings ▸ Keyboard says so too', async (t) => {
  const first = await startPreview({ t });
  const one = await openPage(first);
  let v = await rectangle(one.page, true);
  if (!toolNotice(v)) v = await waitView(one.page, (x) => (x.notices || []).some((n) => n.text.startsWith('Rectangle is running')));
  assert.match(toolNotice(v).text, NOTICE);
  assert.equal(toolNotice(v).key, 'window-tool:com.knollsoft.rectangle');
  assert.deepEqual(toolNotice(v).actions.map((a) => a.label), ['Keyboard settings', 'Dismiss']);
  assert.match(await one.page.textContent('#statusbar .status-notice'), /Rectangle is running/);

  // Keyboard settings opens Settings ▸ Keyboard, where the shortcuts that
  // Rectangle uses too don't say "Works.", and Bench's suggestion is one
  // that Rectangle doesn't use.
  await one.page.click('#statusbar [data-action="notices"]');
  await one.page.waitForSelector('#notices-menu');
  await one.page.click('#notices-menu [data-action="notice-run"]:has-text("Keyboard settings")');
  await one.page.waitForSelector('.settings-sheet');
  await one.page.waitForFunction(() => window.bench.store.ui.settings && window.bench.store.ui.settings.section === 'keyboard');
  await one.page.waitForSelector('#key-focus-pane-modifiers-state');
  assert.equal(await one.page.textContent('#key-focus-pane-modifiers-state'),
    'Rectangle uses ⌃⌥←, ⌃⌥→, ⌃⌥↑, and ⌃⌥↓ too. Both apps may act on these keys. Use ⌃⌥⇧⌘');
  assert.equal(await one.page.textContent('#key-zoom-pane-state'), 'Rectangle uses ⌃⌥↩ too. Both apps may act on this key. Use ⌃⌥⌘↩');
  assert.equal(await one.page.textContent('#key-jump-state'), 'Works.');
  // Changing Zoom pane to a key that Rectangle doesn't use works, and the
  // notice no longer names it, with no copy of its old text in the recent
  // messages.
  await one.page.click('#key-zoom-pane-state [data-action="key-use"]');
  await one.page.waitForFunction(() => document.querySelector('#key-zoom-pane-state').textContent === 'Works.');
  await one.page.click('.settings-sheet [data-action="save-settings"]');
  await one.page.waitForSelector('.settings-sheet', { state: 'detached' });
  v = await waitView(one.page, (x) => (x.notices || []).some((n) => n.text.startsWith('Rectangle is running') && !n.text.includes('Zoom pane')));
  await one.page.click('#statusbar [data-action="notices"]');
  await one.page.waitForSelector('#notices-menu');
  const listed = await one.page.textContent('#notices-menu');
  assert.equal(listed.split('Rectangle is running').length - 1, 1, 'Rectangle\'s notice is listed once: ' + listed);
  assert.doesNotMatch(listed, /Zoom pane/);
  await one.page.keyboard.press('Escape');

  // The notice goes when Rectangle quits, and comes back when it starts
  // again, until the user dismisses it.
  v = await rectangle(one.page, false);
  assert.equal(toolNotice(v), null);
  v = await rectangle(one.page, true);
  if (!toolNotice(v)) v = await waitView(one.page, (x) => (x.notices || []).some((n) => n.text.startsWith('Rectangle is running')));
  assert.notEqual(toolNotice(v), null);
  assert.equal(readState(first).windowTools, undefined);
  await one.page.click('#statusbar [data-action="notices"]');
  await one.page.waitForSelector('#notices-menu');
  await one.page.click('#notices-menu [data-action="notice-run"]:has-text("Dismiss")');
  await waitView(one.page, (x) => !(x.notices || []).some((n) => n.text.startsWith('Rectangle is running')));
  // Bench remembers it, in state.json.
  await waitUntil(() => {
    try {
      return (readState(first).windowTools || []).includes('com.knollsoft.rectangle');
    } catch {
      return false;
    }
  }, 'state.json to remember Rectangle');
  v = await rectangle(one.page, false);
  v = await rectangle(one.page, true);
  assert.equal(toolNotice(v), null);
  await one.close();
  assert.equal(await first.stop(), 0);

  // Nor after a restart, with Rectangle still running.
  assert.deepEqual(readState(first).windowTools, ['com.knollsoft.rectangle']);
  const second = await startPreview({ home: first.home, t });
  const two = await openPage(second);
  t.after(two.close);
  v = await rectangle(two.page, true);
  assert.equal(toolNotice(v), null);
});

test('Dismiss takes Rectangle\'s notice away', async (t) => {
  const preview = await startPreview({ t });
  const { page, close } = await openPage(preview);
  t.after(close);
  await rectangle(page, true);
  await waitView(page, (x) => (x.notices || []).some((n) => n.text.startsWith('Rectangle is running')));
  await page.click('#statusbar [data-action="notices"]');
  await page.waitForSelector('#notices-menu');
  await page.click('#notices-menu [data-action="notice-run"]:has-text("Dismiss")');
  await waitView(page, (x) => !(x.notices || []).some((n) => n.text.startsWith('Rectangle is running')));
});

test('a window that keeps moving out of its pane gets a warning mark, and Dock again puts it back', async (t) => {
  const preview = await startPreview({ t });
  const { page, close } = await openPage(preview);
  t.after(close);
  await setScreen(page, 'monitor');
  let v = await switchTo(page, 'Code');
  const zed = itemID(v, 'bench — switch.go');
  const pane = paneOf(v, zed);
  await page.click('.subtab[data-item="' + zed + '"]');
  await waitView(page, (x, a) => x.display.focus === a.pane && x.display.panes.find((p) => p.id === a.pane).active === a.item, { item: zed, pane });
  await rectangle(page, true);

  // Bench puts the window back three times, then stops, and says why.
  await page.click('#demo-simulate');
  await page.click('#demo-bar [data-action="demo-tool-moves"]');
  v = await waitView(page, (x, id) => x.items[id].status === 'moving', zed, TIMEOUT + 5000);
  assert.equal(v.items[zed].statusText, 'Zed or Rectangle keeps moving this window');
  const body = page.locator('.pane[data-pane-id="' + pane + '"] .pane-status');
  await body.waitFor();
  assert.equal(await body.getAttribute('data-status'), 'moving');
  assert.equal(await body.locator('.status-text').textContent(), 'Zed or Rectangle keeps moving this window');
  const sub = page.locator('.subtab[data-item="' + zed + '"]');
  assert.equal(await sub.getAttribute('data-marker'), 'problem');
  assert.match(await sub.getAttribute('aria-label'), /\(keeps moving out of the pane\)$/);
  assert.match(await page.textContent('#statusbar'), /1 window needs attention/);

  // The mark stays across a tab switch.
  await switchTo(page, 'Research');
  v = await switchTo(page, 'Code');
  assert.equal(v.items[zed].status, 'moving');

  // Dock again puts it back in its pane, and Bench puts it back again
  // from now on.
  await body.locator('button:has-text("Dock again")').click();
  v = await waitView(page, (x, id) => x.items[id].status === 'ready', zed);
  assert.equal(v.display.panes.find((p) => p.id === pane).active, zed);
  assert.equal(await page.locator('.pane[data-pane-id="' + pane + '"] .pane-status').count(), 0);
});
