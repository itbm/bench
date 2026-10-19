'use strict';

// Switching the default config's web pages to web apps from their menus:
// Slack to the demo's Brave web app of it, which stays across a restart,
// and Outlook to the web app that the demo's Safari makes.

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const {
  itemsOf, openPage, panesOf, setScreen, startPreview, view, waitForToast, waitView,
} = require('./helpers/preview.js');

// The demo's Slack web app, which Brave made.
const SLACK = 'com.brave.Browser.app.jneocipojkkahfcibhjaiilegofacenn';

// webAppIn waits for a pane to show an item of the app with bundle ID id,
// ready, and returns the View and the item.
async function webAppIn(page, paneId, id) {
  const v = await waitView(page, (x, a) => {
    const p = x.display.panes.find((q) => q.id === a.pane);
    const it = p && x.items[p.active];
    return Boolean(it && it.bundleId === a.id && it.status === 'ready');
  }, { pane: paneId, id });
  const p = v.display.panes.find((q) => q.id === paneId);
  return { v, item: v.items[p.active] };
}

// chooseFromMenu opens a pane's ⋯ menu and chooses the web app entry with
// a label, once Go has offered it.
async function chooseFromMenu(page, paneId, label) {
  await page.click('[data-action="menu"][data-pane="' + paneId + '"]');
  await page.locator('#window-menu .menu-item', { hasText: label }).click();
}

test('a web page\'s menu puts the web app the user has in its place, for good', async (t) => {
  const first = await startPreview({ t });
  const one = await openPage(first);
  let v = await setScreen(one.page, 'monitor');
  const [slackPane] = panesOf(v, 'Comms');
  await chooseFromMenu(one.page, slackPane.id, 'Use the Slack web app from Brave');
  await waitForToast(one.page, 'Replaced Slack with the Slack web app.');
  const { item } = await webAppIn(one.page, slackPane.id, SLACK);
  assert.equal(item.kind, 'app');
  assert.equal(item.app, 'Slack');
  v = await view(one.page);
  assert.deepEqual(itemsOf(v, 'Comms'), [
    [{ app: 'Slack', title: 'Slack window' }],
    [{ app: 'Outlook', title: 'Outlook' }],
  ]);
  assert.equal(await one.page.textContent('.pane[data-pane-id="' + slackPane.id + '"] .badge'), 'App');

  // config.toml lists the web app in the page's place, and keeps the file
  // from before.
  const config = fs.readFileSync(path.join(first.home, 'config.toml'), 'utf8');
  assert.ok(config.includes('app = "' + SLACK + '"'), config);
  assert.ok(!config.includes('app.slack.com'), config);
  assert.match(fs.readFileSync(path.join(first.home, 'config.toml.bak'), 'utf8'), /app\.slack\.com/);
  await one.close();
  assert.equal(await first.stop(), 0);

  // The web app is there when Bench opens again, and the page isn't.
  const second = await startPreview({ home: first.home, t });
  const two = await openPage(second);
  t.after(two.close);
  v = await setScreen(two.page, 'monitor');
  const again = await webAppIn(two.page, panesOf(v, 'Comms')[0].id, SLACK);
  assert.equal(again.item.id, item.id);
  v = await view(two.page);
  assert.deepEqual(itemsOf(v, 'Comms'), [
    [{ app: 'Slack', title: 'Slack window' }],
    [{ app: 'Outlook', title: 'Outlook' }],
  ]);
  assert.ok(!Object.values(v.items).some((it) => it.kind === 'web' && it.app === 'Slack'), 'Slack\'s page is gone');
});

test('a web page\'s menu has Safari make a web app, and puts it in the page\'s place', async (t) => {
  const preview = await startPreview({ t });
  const { page, close } = await openPage(preview);
  t.after(close);
  const v = await setScreen(page, 'monitor');
  const [, outlookPane] = panesOf(v, 'Comms');
  await chooseFromMenu(page, outlookPane.id, 'Make a web app in Safari…');
  await page.waitForSelector('#palette-make-title');
  assert.equal(await page.textContent('#palette-make-title'), 'Make a web app in Safari');
  assert.equal(await page.textContent('#palette-make-steps'),
    'Bench has opened Outlook in Safari. Choose File ▸ Add to Dock, then click Add.');
  // The demo's Safari makes the web app a few seconds later, as the user
  // would, and the dialog puts it in the page's place.
  await page.waitForSelector('#overlay-root .dialog', { state: 'detached', timeout: 15000 });
  await waitForToast(page, 'Replaced Outlook with its web app.');
  const { v: w, item } = await webAppIn(page, outlookPane.id, 'com.apple.Safari.WebApp.1');
  assert.equal(item.app, 'Outlook');
  assert.equal(item.title, 'Outlook');
  assert.deepEqual(itemsOf(w, 'Comms'), [
    [{ app: 'Slack', title: 'Slack' }],
    [{ app: 'Outlook', title: 'Outlook' }],
  ]);
  assert.ok(!Object.values(w.items).some((it) => it.kind === 'web' && it.app === 'Outlook'), 'Outlook\'s page is gone');
  // Safari's own window, which it opened for the site, stays out of the tab.
  assert.ok(!Object.values(w.items).some((it) => it.bundleId === 'com.apple.Safari'), 'Safari\'s window isn\'t an item');
});
