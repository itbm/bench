'use strict';

// Unread counts end to end: the demo's pages of Slack and Outlook have
// counts at the start of their titles, its Brave a badge of 4 on its Dock
// icon, and its Mail a badge of 8. Go finds them and the frontend shows
// them, and Settings ▸ General turns them off in config.toml.

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const {
  itemID, openPage, openSettings, panesOf, setScreen, startPreview, switchTo, view, waitForToast, waitUntil, waitView,
} = require('./helpers/preview.js');

async function open(t) {
  const preview = await startPreview({ t });
  const { page, close } = await openPage(preview);
  t.after(close);
  return { preview, page };
}

const tab = (i) => '.tab[data-index="' + i + '"]';
const sub = (id) => '.subtab[data-item="' + id + '"]';

test('the counts at the start of pages\' titles show on their sub-tabs, and the total on the tab', async (t) => {
  const { page } = await open(t);
  const v = await waitView(page, (x) => x.tabs[0].unread && x.tabs[0].unread.count === 15);
  const slack = itemID(v, 'Slack');
  const outlook = itemID(v, 'Outlook');
  assert.deepEqual(v.items[slack].unread, { count: 3 });
  assert.deepEqual(v.items[outlook].unread, { count: 12 });
  // The titles lose the counts, which the pills show instead.
  assert.equal(v.items[slack].title, 'Slack');
  assert.equal(v.items[outlook].title, 'Outlook');

  await page.waitForSelector(tab(0) + ' .tab-unread');
  assert.equal(await page.textContent(tab(0) + ' .tab-unread'), '15');
  assert.equal(await page.textContent(sub(slack) + ' .subtab-unread'), '3');
  assert.equal(await page.textContent(sub(outlook) + ' .subtab-unread'), '12');
  assert.equal(await page.getAttribute(sub(slack), 'aria-label'), 'Slack (3 unread)');
  // The other tabs have nothing unread.
  assert.equal(await page.locator(tab(1) + ' .unread-mark').count(), 0);
});

test('an app\'s Dock badge shows on the first of its windows in tab order, for the whole app', async (t) => {
  const { page } = await open(t);
  const v = await switchTo(page, 'Research');
  const brave = panesOf(v, 'Research')[0].items;
  assert.equal(brave.length, 3);
  const after = await waitView(page, (x) => x.tabs[2].unread && x.tabs[2].unread.count === 4);
  assert.deepEqual(after.items[brave[0]].unread, { count: 4, wholeApp: true });
  assert.ok(!after.items[brave[1]].unread && !after.items[brave[2]].unread, 'Brave\'s other windows have no count');

  await page.waitForSelector(sub(brave[0]) + ' .subtab-unread');
  assert.equal(await page.textContent(sub(brave[0]) + ' .subtab-unread'), '4');
  assert.equal(await page.getAttribute(sub(brave[0]) + ' .subtab-unread', 'title'), '4 unread (whole app)');
  assert.equal(await page.getAttribute(sub(brave[0]), 'aria-label'), 'Wails v3 docs, Brave (4 unread in the whole app)');
  assert.equal(await page.locator(sub(brave[1]) + ' .unread-mark').count(), 0);
  assert.equal(await page.textContent(tab(2) + ' .tab-unread'), '4');
});

test('an app\'s Dock badge shows on its sub-tab once it runs', async (t) => {
  const { page } = await open(t);
  await setScreen(page, 'monitor');
  const v = await switchTo(page, 'Code');
  const pane = panesOf(v, 'Code')[0];
  await page.click('[data-action="add"][data-pane="' + pane.id + '"]');
  await page.waitForSelector('#palette-query');
  await page.fill('#palette-query', 'mail');
  await page.locator('.palette-item', { has: page.locator('.palette-label', { hasText: /^Mail$/ }) }).click();
  const after = await waitView(page, (x) => Object.values(x.items).some((it) => it.bundleId === 'com.apple.mail' && it.unread && it.unread.count === 8));
  const mail = Object.values(after.items).find((it) => it.bundleId === 'com.apple.mail');
  assert.equal(after.tabs[1].unread.count, 8);
  await page.waitForSelector(sub(mail.id) + ' .subtab-unread');
  assert.equal(await page.textContent(sub(mail.id) + ' .subtab-unread'), '8');
  assert.equal(await page.getAttribute(sub(mail.id) + ' .subtab-unread', 'title'), '8 unread');
  assert.equal(await page.textContent(tab(1) + ' .tab-unread'), '8');
});

test('Settings ▸ General turns the counts off, in config.toml too', async (t) => {
  const { preview, page } = await open(t);
  await waitView(page, (x) => x.tabs[0].unread && x.tabs[0].unread.count === 15);
  const config = () => fs.readFileSync(path.join(preview.home, 'config.toml'), 'utf8');
  assert.match(config(), /^unread_counts = true\s/m);

  await openSettings(page);
  assert.equal(await page.isChecked('[data-setting="unreadCounts"]'), true);
  await page.uncheck('[data-setting="unreadCounts"]');
  await page.click('[data-action="save-settings"]');
  await waitForToast(page, 'Saved settings');
  const v = await waitView(page, (x) => x.settings.unreadCounts === false);
  assert.ok(!v.tabs.some((x) => x.unread), 'no tab has a total');
  assert.ok(!Object.values(v.items).some((it) => it.unread), 'no item has a count');
  // The pages' own titles come back.
  assert.equal(v.items[itemID(v, '(3) Slack')].title, '(3) Slack');
  await page.waitForSelector(tab(0) + ' .tab-unread', { state: 'detached' });
  assert.equal(await page.locator('.unread-mark').count(), 0);
  await waitUntil(() => /^unread_counts = false\s/m.test(config()), 'config.toml to have unread_counts = false');

  // And on again.
  await openSettings(page);
  await page.check('[data-setting="unreadCounts"]');
  await page.click('[data-action="save-settings"]');
  await waitView(page, (x) => x.tabs[0].unread && x.tabs[0].unread.count === 15);
  await page.waitForSelector(tab(0) + ' .tab-unread');
  assert.equal((await view(page)).settings.unreadCounts, true);
});
