'use strict';

// Pages kept running in the background, end to end (decision 76): a page's
// first notification, from the demo bar's Simulate ▸ Post notification,
// turns Keep Running in the Background on, which config.toml keeps, and
// Undo, on the toast or in the recent messages, turns it off. A page kept
// running loads, hidden, soon after Bench starts, so its count arrives in
// a tab that hasn't been shown; one that isn't says so on its sub-tab, and
// in the overview of every tab, until it's shown.

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const {
  REPO, itemID, newHome, openPage, panesOf, setScreen, startPreview, tabIndex, view, waitForToast, waitUntil, waitView,
} = require('./helpers/preview.js');

const NOT_LOADED = 'Not loaded yet: no notifications or counts until you show it';

// configText returns config.toml in a support folder.
const configText = (preview) => fs.readFileSync(path.join(preview.home, 'config.toml'), 'utf8');

// lineOf returns the line of config.toml with a page's address.
const lineOf = (preview, address) => configText(preview).split('\n').find((l) => l.includes('web = "' + address + '"')) || '';

test('a page\'s first notification keeps it running in the background, and Undo turns that off', async (t) => {
  const preview = await startPreview({ t });
  const { page, close } = await openPage(preview);
  t.after(close);
  let v = await setScreen(page, 'monitor');
  const slack = itemID(v, 'Slack');
  const [slackPane] = panesOf(v, 'Comms');
  await page.click('.subtab[data-item="' + slack + '"]');
  await waitView(page, (x, p) => x.display.focus === p, slackPane.id);

  const text = 'Slack will keep running in the background, so it can notify you.';
  await page.click('#demo-simulate');
  await page.click('#demo-bar [data-action="demo-notify"]');
  await waitForToast(page, text);
  assert.equal(await page.textContent('#toast .toast-action'), 'Undo');
  v = await waitView(page, (x, id) => x.items[id].keepRunning, slack);
  await waitUntil(() => lineOf(preview, 'https://app.slack.com/client').includes('keep_running = true'), 'keep_running = true in config.toml');
  // It's a change that Undo takes back away from its toast, as Tab ▸ Undo
  // names it.
  assert.deepEqual([v.undo.name, v.undo.did], ['Keep Running in the Background', 'kept Slack running in the background']);
  // Its ⋯ menu ticks it.
  await page.click('[data-action="menu"][data-pane="' + slackPane.id + '"]');
  const keep = '#window-menu .menu-item:has(.menu-label:text-is("Keep Running in the Background"))';
  assert.equal(await page.getAttribute(keep, 'aria-checked'), 'true');
  await page.keyboard.press('Escape');

  // The recent messages keep it, with Undo, which turns it off, as the
  // user's choice, which a later notification leaves alone.
  await page.click('[data-action="notices"]');
  const first = '#notices-menu .message:first-child';
  await page.waitForSelector(first);
  assert.equal(await page.textContent(first + ' .message-text'), text);
  assert.equal(await page.getAttribute(first + ' [data-action="message-undo"]', 'aria-label'), 'Undo Keep Running in the Background');
  await page.click(first + ' [data-action="message-undo"]');
  await page.waitForFunction((s) => document.querySelector(s).textContent
    === 'Slack will load only when you show it, from the next time Bench opens.', first + ' .message-text');
  await page.keyboard.press('Escape');
  await waitView(page, (x, id) => !x.items[id].keepRunning, slack);
  await waitUntil(() => lineOf(preview, 'https://app.slack.com/client').includes('keep_running = false'), 'keep_running = false in config.toml');
  await page.click('#demo-simulate');
  await page.click('#demo-bar [data-action="demo-notify"]');
  await page.waitForTimeout(500);
  assert.ok(!(await view(page)).items[slack].keepRunning);
  assert.match(lineOf(preview, 'https://app.slack.com/client'), /keep_running = false/);
});

test('a page kept running loads in a tab that hasn\'t been shown, with its count, and one that isn\'t says so', async (t) => {
  // The example config, with Gmail behind Slack in Comms, and Outlook in a
  // tab of its own, kept running.
  const home = newHome();
  const example = fs.readFileSync(path.join(REPO, 'internal', 'config', 'example.toml'), 'utf8');
  const start = example.indexOf('[[tab]]\nname = "Comms"');
  const end = example.indexOf('[[tab]]\nname = "Code"');
  assert.ok(start > 0 && end > start, 'the example config has Comms, then Code');
  const tabs = '[[tab]]\nname = "Comms"\nlayout = "single"\n\n'
    + '  [[tab.pane]]\n  items = [\n'
    + '    { web = "https://app.slack.com/client", profile = "slack", name = "Slack" },\n'
    + '    { web = "https://mail.google.com/mail/", name = "Gmail" },\n  ]\n\n'
    + '[[tab]]\nname = "Mail"\nlayout = "single"\n\n'
    + '  [[tab.pane]]\n  items = [{ web = "https://outlook.office.com/mail/", profile = "outlook", name = "Outlook", keep_running = true }]\n\n';
  fs.writeFileSync(path.join(home, 'config.toml'), example.slice(0, start) + tabs + example.slice(end));

  const preview = await startPreview({ home, t });
  const { page, close } = await openPage(preview);
  t.after(close);
  let v = await setScreen(page, 'monitor');
  const gmail = itemID(v, 'Gmail');
  const outlook = itemID(v, 'Outlook');
  assert.equal(v.current, tabIndex(v, 'Comms'));

  // Outlook loads, hidden, in the Mail tab, which hasn't been shown, so
  // its count arrives.
  v = await waitView(page, (x, id) => x.items[id].unread && x.items[id].unread.count === 12, outlook);
  assert.ok(!v.items[outlook].notLoaded);
  assert.equal(v.current, tabIndex(v, 'Comms'));
  await page.waitForSelector('.tab[data-index="' + tabIndex(v, 'Mail') + '"] .tab-unread');

  // Gmail, behind Slack, isn't loaded, and says so.
  const sub = '.subtab[data-item="' + gmail + '"]';
  assert.ok(v.items[gmail].notLoaded);
  assert.ok((await page.getAttribute(sub, 'title')).split('\n').includes(NOT_LOADED));
  assert.equal(await page.getAttribute(sub, 'aria-label'), 'Gmail (' + NOT_LOADED.toLowerCase() + ')');

  // Kept running from its sub-tab's menu, it loads soon, and says nothing
  // more of it; config.toml keeps it.
  await page.click(sub, { button: 'right' });
  await page.click('#window-menu .menu-item:has(.menu-label:text-is("Keep Running in the Background"))');
  await waitForToast(page, 'Gmail will keep running in the background.');
  assert.match(lineOf(preview, 'https://mail.google.com/mail/'), /keep_running = true/);
  await waitView(page, (x, id) => !x.items[id].notLoaded, gmail);
  await page.waitForFunction((s) => !document.querySelector(s).title.includes('Not loaded'), sub);
  assert.equal(await page.getAttribute(sub, 'aria-label'), 'Gmail');
});

test('the overview of every tab says which pages aren\'t loaded yet', async (t) => {
  // The example config, with Outlook alone in a Mail tab, not kept
  // running, so that it isn't loaded until that tab is shown.
  const home = newHome();
  const example = fs.readFileSync(path.join(REPO, 'internal', 'config', 'example.toml'), 'utf8');
  const start = example.indexOf('[[tab]]\nname = "Comms"');
  const end = example.indexOf('[[tab]]\nname = "Code"');
  const tabs = '[[tab]]\nname = "Comms"\nlayout = "single"\n\n'
    + '  [[tab.pane]]\n  items = [{ web = "https://app.slack.com/client", profile = "slack", name = "Slack" }]\n\n'
    + '[[tab]]\nname = "Mail"\nlayout = "single"\n\n'
    + '  [[tab.pane]]\n  items = [{ web = "https://outlook.office.com/mail/", profile = "outlook", name = "Outlook" }]\n\n';
  fs.writeFileSync(path.join(home, 'config.toml'), example.slice(0, start) + tabs + example.slice(end));

  const preview = await startPreview({ home, t });
  const { page, close } = await openPage(preview);
  t.after(close);
  const v = await setScreen(page, 'monitor');
  const mail = tabIndex(v, 'Mail');
  assert.ok(v.items[itemID(v, 'Outlook')].notLoaded);
  await page.click('[data-action="open-palette"]');
  await page.fill('#palette-query', 'show all tabs');
  await page.keyboard.press('Enter');
  const card = '#overview-card-' + mail;
  await page.waitForSelector(card);
  assert.match(await page.getAttribute(card, 'aria-label'), /^Mail .*, showing Outlook, 1 page not loaded yet$/);
  assert.equal(await page.getAttribute(card + ' .overview-shown', 'title'), 'Outlook\n' + NOT_LOADED);
  // Comms, shown, has none.
  assert.doesNotMatch(await page.getAttribute('#overview-card-' + tabIndex(v, 'Comms'), 'aria-label'), /not loaded/);
});
