'use strict';

// The add dialog: a new terminal, a web page, a new window of an app in the
// tab, and an installed app that isn't running yet.

const assert = require('node:assert/strict');
const test = require('node:test');
const {
  openPage, panesOf, setScreen, startPreview, switchTo, waitForToast, waitView,
} = require('./helpers/preview.js');

// openAdd starts a preview on the Code tab and opens the add dialog for
// one of its panes (0 is the main pane, 1 the terminal's).
async function openAdd(t, paneIndex = 1) {
  const preview = await startPreview({ t });
  const { page, close } = await openPage(preview);
  t.after(close);
  await setScreen(page, 'monitor');
  const v = await switchTo(page, 'Code');
  const pane = panesOf(v, 'Code')[paneIndex];
  await page.click('[data-action="add"][data-pane="' + pane.id + '"]');
  await page.waitForSelector('#palette-query');
  return { page, v, pane };
}

// newItemIn waits for the pane to have an item that isn't in before, and
// returns it and the View.
async function newItemIn(page, pane, before) {
  const v = await waitView(page, (x, a) => {
    const p = x.display.panes.find((q) => q.id === a.pane);
    return Boolean(p && p.items.some((id) => !a.before.includes(id)));
  }, { pane: pane.id, before });
  const p = v.display.panes.find((q) => q.id === pane.id);
  const id = p.items.find((i) => !before.includes(i));
  return { v, item: v.items[id], shown: p.active === id };
}

test('the add dialog lists its groups in order', async (t) => {
  const { page } = await openAdd(t, 0);
  const groups = await page.locator('.palette-group').allTextContents();
  assert.deepEqual(groups, ['Windows not in a tab', 'Windows in other tabs', 'New window', 'Built in', 'Apps']);
  const labels = await page.locator('.palette-item .palette-label').allTextContents();
  assert.deepEqual(labels.slice(0, 3), ['Downloads', 'Slack', 'Outlook']);
  const firstNew = labels.indexOf('New Zed window');
  assert.deepEqual(labels.slice(firstNew, firstNew + 4), ['New Zed window', 'New VS Code window', 'New terminal', 'Web page…']);
  assert.ok(labels.includes('Calendar'));
  assert.ok(labels.includes('Activity Monitor'));
});

test('New terminal adds a terminal to the pane', async (t) => {
  const { page, pane } = await openAdd(t);
  await page.fill('#palette-query', 'terminal');
  await page.locator('.palette-item', { hasText: 'New terminal' }).click();
  const { item, shown } = await newItemIn(page, pane, pane.items);
  assert.equal(item.kind, 'builtin');
  assert.equal(item.app, 'Terminal');
  assert.equal(item.folder, '~');
  assert.ok(shown, 'the new terminal is shown');
  await page.waitForSelector('.pane[data-pane-id="' + pane.id + '"] [data-term="' + item.id + '"] .xterm');
  assert.equal(await page.locator('#overlay-root .dialog').count(), 0);
});

test('Web page… adds a web page, with https:// added', async (t) => {
  const { page, pane } = await openAdd(t);
  await page.locator('.palette-item', { hasText: 'Web page…' }).click();
  await page.waitForSelector('#palette-url');
  // An address that isn't https is refused, and the dialog stays open.
  await page.fill('#palette-url', 'http://example.com');
  await page.keyboard.press('Enter');
  await waitForToast(page, 'That address isn\'t a secure web address (https).');
  assert.equal(await page.locator('#palette-url').count(), 1);

  await page.fill('#palette-url', 'example.com/docs');
  await page.keyboard.press('Enter');
  const { item, shown } = await newItemIn(page, pane, pane.items);
  assert.equal(item.kind, 'web');
  assert.equal(item.url, 'https://example.com/docs');
  assert.ok(shown, 'the web page is shown');
  assert.equal(await page.locator('#overlay-root .dialog').count(), 0);
  // Once the page is in view (the demo reports the address it was given),
  // the pane's header shows its host in place of the Web badge.
  const v = await waitView(page, (x, id) => x.items[id].host === 'example.com', item.id);
  assert.equal(v.items[item.id].site, 'example.com');
  const chip = '.pane[data-pane-id="' + pane.id + '"] .host-chip';
  await page.waitForSelector(chip);
  assert.equal(await page.textContent(chip), 'example.com');
  assert.equal(await page.getAttribute(chip, 'aria-label'), 'Page on example.com');
  assert.equal(await page.locator('.pane[data-pane-id="' + pane.id + '"] .badge').count(), 0);
});

test('New window opens another window of an app in the tab', async (t) => {
  const { page, pane } = await openAdd(t, 0);
  await page.locator('.palette-item', { hasText: 'New Zed window' }).click();
  const { item, shown } = await newItemIn(page, pane, pane.items);
  assert.equal(item.app, 'Zed');
  assert.equal(item.kind, 'app');
  assert.equal(item.status, 'ready');
  assert.ok(shown, 'the new window is shown');
});

test('an installed app opens after a moment and its window joins the pane', async (t) => {
  const { page, pane } = await openAdd(t);
  await page.fill('#palette-query', 'calendar');
  await page.keyboard.press('Enter');
  const { item, shown } = await newItemIn(page, pane, pane.items);
  assert.equal(item.app, 'Calendar');
  assert.equal(item.bundleId, 'com.apple.iCal');
  assert.equal(item.title, 'Calendar window');
  assert.equal(item.status, 'ready');
  assert.ok(shown, 'the app\'s window is shown');
  await page.waitForSelector('.pane[data-pane-id="' + pane.id + '"] .subtab[data-item="' + item.id + '"][aria-pressed="true"]');
});
