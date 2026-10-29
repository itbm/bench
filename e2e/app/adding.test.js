'use strict';

// The add dialog: a new terminal, one that runs a command, and a launcher
// saved from it, a website (as its web app, one that the demo's Safari
// makes, or a web page in Bench), a new window of an app in the tab, and an
// installed app that isn't running yet.

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const {
  openPage, panesOf, setScreen, startPreview, switchTo, view, waitForToast, waitUntil, waitView,
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
  return { preview, page, v, pane };
}

// row returns the add dialog's row whose label is label.
function row(page, label) {
  return page.locator('.palette-item').filter({ has: page.locator('.palette-label', { hasText: new RegExp('^' + label + '$') }) });
}

// rowsText returns the text of a terminal's rows.
function rowsText(page, itemId) {
  return page.$eval('[data-term="' + itemId + '"] .xterm-rows', (el) => el.textContent);
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

// appIn waits for the pane to have an item of the app with bundle ID id,
// and returns it and whether it's shown.
async function appIn(page, pane, id) {
  const v = await waitView(page, (x, a) => {
    const p = x.display.panes.find((q) => q.id === a.pane);
    return Boolean(p && p.items.some((i) => x.items[i] && x.items[i].bundleId === a.id && x.items[i].status === 'ready'));
  }, { pane: pane.id, id });
  const p = v.display.panes.find((q) => q.id === pane.id);
  const item = p.items.map((i) => v.items[i]).find((it) => it.bundleId === id);
  return { item, shown: p.active === item.id };
}

// chooseWebPage chooses Web page… and types an address.
async function chooseWebPage(page, address) {
  await page.locator('.palette-item', { hasText: 'Web page…' }).click();
  await page.waitForSelector('#palette-url');
  await page.fill('#palette-url', address);
}

// The demo's Gmail web app, which Brave made.
const GMAIL = 'com.brave.Browser.app.fmgjjmmmlfnkbppncabfkddbjimcfncm';

test('the add dialog lists its groups in order', async (t) => {
  const { page } = await openAdd(t, 0);
  const groups = await page.locator('.palette-group').allTextContents();
  assert.deepEqual(groups, ['Windows not in a tab', 'Windows in other tabs', 'New window', 'Built in', 'Apps', 'Web apps']);
  const labels = await page.locator('.palette-item .palette-label').allTextContents();
  assert.deepEqual(labels.slice(0, 3), ['Downloads', 'Slack', 'Outlook']);
  const firstNew = labels.indexOf('New Zed window');
  assert.deepEqual(labels.slice(firstNew, firstNew + 7),
    ['New Zed window', 'New VS Code window', 'New terminal', 'New terminal in…', 'New terminal…', 'Web page…', 'Browse…']);
  assert.ok(labels.includes('Calendar'));
  assert.ok(labels.includes('Activity Monitor'));
  const gmail = page.locator('.palette-item', { hasText: 'Gmail' });
  assert.equal(await gmail.locator('.palette-hint').textContent(), 'Web app from Brave');
  const web = page.locator('.palette-item', { hasText: 'Web page…' });
  assert.equal(await web.locator('.palette-hint').textContent(), 'As a web app, or in Bench');
});

test('New terminal adds a terminal to the pane', async (t) => {
  const { page, pane } = await openAdd(t);
  await page.fill('#palette-query', 'terminal');
  await row(page, 'New terminal').click();
  const { item, shown } = await newItemIn(page, pane, pane.items);
  assert.equal(item.kind, 'builtin');
  assert.equal(item.app, 'Terminal');
  assert.equal(item.folder, '~');
  assert.ok(shown, 'the new terminal is shown');
  await page.waitForSelector('.pane[data-pane-id="' + pane.id + '"] [data-term="' + item.id + '"] .xterm');
  // The dialog closes once Go's answer arrives, which can be after the
  // view that Go sent first.
  await page.waitForSelector('#overlay-root .dialog', { state: 'detached' });
});

test('in a terminal\'s pane, the first Built in row opens a new terminal in the folder its shell is in', async (t) => {
  const { page, pane } = await openAdd(t);
  const labels = await page.locator('.palette-item .palette-label').allTextContents();
  const first = labels.indexOf('New terminal in ~/code');
  assert.deepEqual(labels.slice(first, first + 3), ['New terminal in ~/code', 'New terminal', 'New terminal in…']);
  assert.equal(await row(page, 'New terminal in ~/code').locator('.palette-hint').textContent(), 'Where the terminal here is');
  await row(page, 'New terminal in ~/code').click();
  const { item } = await newItemIn(page, pane, pane.items);
  assert.equal(item.kind, 'builtin');
  assert.equal(item.folder, '~/code');
});

test('a command that matches nothing runs in a new terminal, which Save as launcher keeps in config.toml', async (t) => {
  const { preview, page, pane } = await openAdd(t);
  await page.fill('#palette-query', 'make test');
  await page.waitForSelector('#palette-nothing');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#terminal-command');
  assert.equal(await page.inputValue('#terminal-command'), 'make test');
  await page.fill('#terminal-name', 'Tests');
  await page.fill('#terminal-folder', '~/code/bench');
  await page.click('#terminal-save');
  await page.press('#terminal-name', 'Enter');
  await waitForToast(page, 'Saved launcher Tests');
  await page.waitForSelector('#overlay-root .dialog', { state: 'detached' });

  // The terminal is named, starts in the folder, and the pretend shell
  // shows the command typed at its prompt.
  const { item, shown } = await newItemIn(page, pane, pane.items);
  assert.equal(item.kind, 'builtin');
  assert.equal(item.title, 'Tests');
  assert.equal(item.folder, '~/code/bench');
  assert.ok(shown, 'the new terminal is shown');
  await page.waitForFunction((id) => {
    const rows = document.querySelector('[data-term="' + id + '"] .xterm-rows');
    return Boolean(rows && rows.textContent.includes('make test'));
  }, item.id);
  assert.match(await rowsText(page, item.id), /~\/code\/bench/);

  // config.toml has the launcher, and the add dialog lists it first, and
  // Jump to… finds it by its name.
  const toml = await waitUntil(() => {
    const text = fs.readFileSync(path.join(preview.home, 'config.toml'), 'utf8');
    return text.includes('[[launcher]]') ? text : null;
  }, 'config.toml to have the launcher');
  assert.match(toml, /\[\[launcher\]\]\nname = "Tests"\nfolder = "~\/code\/bench"\nrun = "make test"\n/);
  await page.click('[data-action="add"][data-pane="' + pane.id + '"]');
  await page.waitForSelector('#palette-query');
  assert.deepEqual((await page.locator('.palette-group').allTextContents())[0], 'Launchers');
  assert.equal(await page.locator('.palette-item .palette-label').first().textContent(), 'Tests');
  await page.keyboard.press('Escape');
  await page.waitForSelector('#overlay-root .dialog', { state: 'detached' });
  await page.click('[data-action="open-palette"]');
  await page.waitForSelector('#palette-query');
  await page.fill('#palette-query', 'tests');
  // The terminal, which has the launcher's name, comes first, among the
  // windows, then the launcher, and what can be done with the terminal,
  // which the focused pane shows, after.
  const found = await page.$$eval('#overlay-root .palette-list > :not(.palette-note)', (els) => els.map((el) => (el.classList.contains('palette-group')
    ? '# ' + el.textContent
    : el.querySelector('.palette-label').textContent)));
  assert.deepEqual(found.slice(0, 5), ['# Windows', 'Tests', '# Launchers', 'Tests', '# This window']);
});

test('Web page… adds the site\'s web app, if there\'s one', async (t) => {
  const { page, pane } = await openAdd(t);
  await chooseWebPage(page, 'mail.google.com');
  assert.match(await page.textContent('#palette-url-hint'), /or opens the site in Safari so that you can make one/);
  await page.keyboard.press('Enter');
  await waitForToast(page, 'Added the Gmail web app from Brave.');
  // The dialog closes once Go's answer arrives, which can be after the
  // view that Go sent first.
  await page.waitForSelector('#overlay-root .dialog', { state: 'detached' });
  const { item, shown } = await appIn(page, pane, GMAIL);
  assert.equal(item.kind, 'app');
  assert.equal(item.app, 'Gmail');
  assert.equal(item.title, 'Gmail window');
  assert.ok(shown, 'the web app is shown');
});

test('Web page… opens a site in Safari, and adds the web app that Safari makes', async (t) => {
  const { page, pane } = await openAdd(t);
  await chooseWebPage(page, 'squoosh.app');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#palette-make-title');
  assert.equal(await page.textContent('#palette-make-title'), 'Make a web app in Safari');
  assert.equal(await page.textContent('#palette-make-steps'),
    'Bench has opened squoosh.app in Safari. Choose File ▸ Add to Dock, then click Add.');
  // The demo's Safari makes the web app a few seconds later, as the user
  // would, and the dialog adds it.
  await page.waitForSelector('#overlay-root .dialog', { state: 'detached', timeout: 15000 });
  await waitForToast(page, 'Added the web app.');
  const { item, shown } = await appIn(page, pane, 'com.apple.Safari.WebApp.1');
  assert.equal(item.app, 'Squoosh');
  assert.equal(item.title, 'Squoosh');
  assert.ok(shown, 'the web app is shown');
  // Safari's own window, which it opened for the site, stays out of the tab.
  const v = await view(page);
  assert.ok(!Object.values(v.items).some((it) => it.bundleId === 'com.apple.Safari'), 'Safari\'s window isn\'t an item');
  assert.ok((v.floating || []).some((it) => it.bundleId === 'com.apple.Safari'), 'Safari\'s window is under Not in a tab');
});

test('Option-Return and Show in Bench instead add a web page, with https:// added', async (t) => {
  const { page, pane } = await openAdd(t);
  // An address that isn't https is refused, and the dialog stays open.
  await chooseWebPage(page, 'http://example.com');
  await page.keyboard.press('Alt+Enter');
  await waitForToast(page, 'That address isn\'t a secure web address (https).');
  assert.equal(await page.locator('#palette-url').count(), 1);

  await page.fill('#palette-url', 'example.com/docs');
  await page.keyboard.press('Alt+Enter');
  const { item, shown } = await newItemIn(page, pane, pane.items);
  assert.equal(item.kind, 'web');
  assert.equal(item.url, 'https://example.com/docs');
  assert.ok(shown, 'the web page is shown');
  // The dialog closes once Go's answer arrives, which can be after the
  // view that Go sent first.
  await page.waitForSelector('#overlay-root .dialog', { state: 'detached' });
  // Once the page is in view (the demo reports the address it was given),
  // the pane's header shows its host in place of the Web badge.
  const v = await waitView(page, (x, id) => x.items[id].host === 'example.com', item.id);
  assert.equal(v.items[item.id].site, 'example.com');
  const chip = '.pane[data-pane-id="' + pane.id + '"] .host-chip';
  await page.waitForSelector(chip);
  assert.equal(await page.textContent(chip), 'example.com');
  assert.equal(await page.getAttribute(chip, 'aria-label'), 'Page on example.com');
  assert.equal(await page.locator('.pane[data-pane-id="' + pane.id + '"] .badge').count(), 0);

  // From the make step too.
  await page.click('[data-action="add"][data-pane="' + pane.id + '"]');
  await page.waitForSelector('#palette-query');
  await chooseWebPage(page, 'example.org');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#palette-make-title');
  const before = (await view(page)).display.panes.find((p) => p.id === pane.id).items;
  await page.click('[data-action="add-in-bench"]');
  const added = await newItemIn(page, pane, before);
  assert.equal(added.item.kind, 'web');
  assert.equal(added.item.url, 'https://example.org');
  // The dialog closes once Go's answer arrives, which can be after the
  // view that Go sent first.
  await page.waitForSelector('#overlay-root .dialog', { state: 'detached' });
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

test('in the preview, Browse… closes the dialog and says that it can\'t look for apps or programs', async (t) => {
  const { page } = await openAdd(t);
  await row(page, 'Browse…').click();
  await waitForToast(page, 'The browser preview can\'t look for apps or programs on this computer. In the Mac app, Browse… finds one on your Mac.');
  assert.equal(await page.isVisible('.palette'), false);
});
