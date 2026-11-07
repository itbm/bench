'use strict';

// Web items: the demo reports the address each Web item's view is given as
// the page in view, so pane headers show its host, as the app's do once the
// page has loaded. The preview can't open the default browser.

const assert = require('node:assert/strict');
const test = require('node:test');
const {
  openPage, panesOf, setScreen, startPreview, waitForToast, waitView,
} = require('./helpers/preview.js');

test('Web items show their page\'s host, and Open in browser says the preview can\'t', async (t) => {
  const preview = await startPreview({ t });
  const { page, close } = await openPage(preview);
  t.after(close);
  await setScreen(page, 'monitor');
  const v = await waitView(page, (x) => Object.values(x.items).filter((it) => it.host).length === 2);
  const [slackPane, outlookPane] = panesOf(v, 'Comms');
  const slack = v.items[slackPane.items[0]];
  assert.equal(slack.app, 'Slack');
  assert.deepEqual([slack.host, slack.site, Boolean(slack.offSite), Boolean(slack.notSecure)], ['app.slack.com', 'slack.com', false, false]);

  const chipOf = (paneId) => page.locator('.pane[data-pane-id="' + paneId + '"] .host-chip').evaluate((chip) => ({
    sub: chip.querySelector('.host-sub').textContent,
    site: chip.querySelector('.host-site').textContent,
    label: chip.getAttribute('aria-label'),
    warning: chip.dataset.warning || null,
  }));
  assert.deepEqual(await chipOf(slackPane.id), { sub: 'app.', site: 'slack.com', label: 'Page on app.slack.com', warning: null });
  assert.deepEqual(await chipOf(outlookPane.id), {
    sub: 'outlook.', site: 'office.com', label: 'Page on outlook.office.com', warning: null,
  });

  await page.click('[data-action="menu"][data-pane="' + slackPane.id + '"]');
  await page.locator('#window-menu .menu-item', { hasText: 'Open in Browser' }).click();
  await waitForToast(page, 'This build of Bench can\'t open links.');
});

test('a web page\'s Page controls zoom it, which Bench keeps, and its find bar finds in it', async (t) => {
  const preview = await startPreview({ t });
  const { page, close } = await openPage(preview);
  t.after(close);
  await setScreen(page, 'monitor');
  const v = await waitView(page, (x) => Object.values(x.items).filter((it) => it.host).length === 2);
  const [slackPane] = panesOf(v, 'Comms');
  const id = slackPane.items[0];
  const menu = '[data-action="menu"][data-pane="' + slackPane.id + '"]';

  // The preview's pages never go anywhere, so Back and Forward are off, in
  // the menu's Page submenu.
  await page.click(menu);
  await page.locator('#window-menu .menu-item.has-sub', { hasText: /^Page$/ }).click();
  const entry = (label) => page.locator('#menu-sub .menu-item:has(.menu-label:text-is("' + label + '"))');
  assert.equal(await entry('Back').isDisabled(), true);
  assert.equal(await entry('Forward').isDisabled(), true);
  await entry('Zoom In').click();
  await waitForToast(page, 'Zoomed Slack to 110%.');
  await waitView(page, (x, item) => x.items[item].zoom === 110, id);

  // The find bar opens at the top of the pane, with the keyboard in its
  // field. The preview's pages find in their address. Find in Page… is in
  // the menu itself, not in its Page submenu.
  await page.click(menu);
  await page.locator('#window-menu .menu-item:has(.menu-label:text-is("Find in Page…"))').click();
  await waitView(page, (x, item) => x.items[item].finding, id);
  const input = '.pane[data-pane-id="' + slackPane.id + '"] .page-find-input';
  await page.waitForFunction((sel) => document.activeElement === document.querySelector(sel), input);
  await page.keyboard.type('nothing like it');
  await page.keyboard.press('Enter');
  await page.waitForFunction(() => document.querySelector('.page-find-count').textContent === 'No matches');
  await page.fill(input, 'SLACK');
  await page.keyboard.press('Enter');
  await page.waitForFunction(() => document.querySelector('.page-find-count').textContent === '');
  await page.keyboard.press('Escape');
  await waitView(page, (x, item) => !x.items[item].finding, id);
});
