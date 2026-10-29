'use strict';

// Recent messages, and the app menu's commands: the toasts and notices of
// late stay in the list of notices, with when each came, its whole text,
// and Copy; the status bar has a button for them while there are no
// notices; and the Tab, Pane, and Help menus' items that need the
// frontend send it bench:commands.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, settle } = require('./helpers/harness.js');
const { defaultWorld } = require('./helpers/fixture.js');

// messages returns the recent messages as listed: each one's time, its
// notes, such as Notice, and its text.
const messages = (page) => page.$$eval('#notices-menu .message', (els) => els.map((el) => ({
  time: el.querySelector('time').textContent,
  notes: Array.from(el.querySelectorAll('.message-head span'), (s) => s.textContent),
  text: el.querySelector('.message-text').textContent,
})));

test('a toast stays in the recent messages, with its time, its whole text, and Copy', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  // There's nothing to show yet.
  assert.equal(await page.locator('.status-messages').count(), 0);

  const long = 'Bench couldn\'t save config.toml: ' + 'the disk is full, '.repeat(12) + 'so nothing was saved.';
  await fake.emit('bench:toast', long);
  await fake.emit('bench:toast', 'Moved Zed to Comms.');
  await fake.emit('bench:toast', 'Moved Zed to Comms.');
  await page.waitForSelector('.status-messages');
  assert.equal(await page.getAttribute('.status-messages', 'aria-label'), 'Recent messages');
  await page.click('.status-messages');
  await page.waitForSelector('#notices-menu');
  await fake.waitForCall('SetOverlay', (a) => a[0] === true);
  const listed = await messages(page);
  assert.equal(listed.length, 2);
  // The latest first, and the same message again counts again.
  assert.deepEqual(listed.map((m) => [m.notes, m.text]), [[['2 times'], 'Moved Zed to Comms.'], [[], long]]);
  for (const m of listed) assert.match(m.time, /^\d\d:\d\d$/);
  // The whole text shows, on as many lines as it takes, and can be
  // selected.
  const text = page.locator('#notices-menu .message-text').nth(1);
  assert.equal(await text.evaluate((el) => getComputedStyle(el).userSelect), 'text');
  assert.ok((await text.boundingBox()).height > 30, 'a long message wraps');

  // Copy puts the message on the clipboard, and says so.
  await page.click('#notices-menu .message:nth-child(2) [data-action="message-copy"]');
  await page.waitForFunction(() => document.querySelector('#notices-menu .message:nth-child(2) [data-action="message-copy"]').textContent === 'Copied');
  assert.equal(fake.clipboard, long);

  // The keyboard moves between the Copy buttons, and Escape closes the
  // list, giving the focus back to its button.
  const focused = () => page.evaluate(() => document.activeElement.dataset.key);
  await page.focus('[data-key="notices-list"]');
  await page.keyboard.press('Tab');
  assert.equal(await focused(), 'message-0');
  await page.keyboard.press('ArrowDown');
  assert.equal(await focused(), 'message-1');
  await page.keyboard.press('ArrowDown');
  assert.equal(await focused(), 'message-0');
  await page.keyboard.press('Escape');
  await page.waitForSelector('#notices-menu', { state: 'detached' });
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'notices');
  assert.deepEqual(fake.errors, []);
});

test('a notice is listed once while it lasts, then stays in the recent messages', async (t) => {
  const notice = 'Couldn\'t register ⌃⌥Space: another app uses it.';
  const { page, fake, close } = await openBench({ world: defaultWorld({ notices: [notice] }) });
  t.after(close);
  await fake.emit('bench:toast', 'Saved settings.');
  await page.click('[data-action="notices"]');
  await page.waitForSelector('#notices-menu');
  // While it lasts, it's in the notices, and not the recent messages too.
  assert.deepEqual(await page.$$eval('#notices-menu .menu-text', (els) => els.map((el) => el.textContent)), [notice]);
  assert.deepEqual((await messages(page)).map((m) => m.text), ['Saved settings.']);

  // Once it's gone, the list stays open, and the notice is a recent
  // message.
  fake.world.notices = [];
  await fake.emitState();
  await page.waitForFunction(() => !document.querySelector('#notices-menu .menu-text'));
  assert.deepEqual((await messages(page)).map((m) => [m.notes, m.text]), [[[], 'Saved settings.'], [['Notice'], notice]]);
  await page.keyboard.press('Escape');
  await page.waitForSelector('#notices-menu', { state: 'detached' });
  // The status bar has the button for the recent messages in the notice's
  // place.
  assert.equal(await page.locator('.status-notice').count(), 0);
  assert.equal(await page.locator('.status-messages').count(), 1);
});

test('the recent messages keep the last 20', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  for (let i = 1; i <= 25; i++) await fake.emit('bench:toast', 'Message ' + i);
  await page.click('.status-messages');
  const listed = (await messages(page)).map((m) => m.text);
  assert.equal(listed.length, 20);
  assert.equal(listed[0], 'Message 25');
  assert.equal(listed[19], 'Message 6');
});

test('Help ▸ Recent Messages and the jump palette open the list, even with the status bar hidden', async (t) => {
  const world = defaultWorld();
  world.settings.showStatusBar = false;
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  // With nothing said yet, the list says so.
  await fake.emit('bench:command', { name: 'recent-messages' });
  await page.waitForSelector('#notices-menu');
  assert.equal(await page.textContent('#notices-menu .menu-note'), 'No messages yet.');
  // It's below the title bar, inside the window.
  const placed = await page.evaluate(() => {
    const menu = document.getElementById('notices-menu').getBoundingClientRect();
    const title = document.getElementById('titlebar').getBoundingClientRect();
    return menu.top >= title.bottom && menu.right <= window.innerWidth;
  });
  assert.equal(placed, true);
  await page.keyboard.press('Escape');
  await page.waitForSelector('#notices-menu', { state: 'detached' });

  // A toast that came meanwhile is listed: here, from the palette.
  await fake.emit('bench:toast', 'Bench couldn\'t open Zed.');
  await fake.emit('bench:command', { name: 'open-palette' });
  await page.waitForSelector('#palette-query');
  await page.fill('#palette-query', 'recent messages');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#notices-menu');
  assert.deepEqual((await messages(page)).map((m) => m.text), ['Bench couldn\'t open Zed.']);
});

test('the app menu\'s Rename…, Add to Pane…, and Keyboard Shortcuts', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  const current = await page.evaluate(() => window.bench.store.view.current);

  // Tab ▸ Rename… renames the current tab in place.
  await fake.emit('bench:command', { name: 'rename-tab' });
  await page.waitForSelector('.tab.renaming[data-index="' + current + '"] #rename-input');
  assert.equal(await page.evaluate(() => document.activeElement.id), 'rename-input');
  await page.keyboard.press('Escape');
  await page.waitForSelector('#rename-input', { state: 'detached' });

  // Pane ▸ Add to Pane… opens the add dialog for the focused pane.
  const focus = await page.evaluate(() => {
    const v = window.bench.store.view;
    return v.tabs[v.current].focus;
  });
  await fake.emit('bench:command', { name: 'add-to-pane' });
  assert.deepEqual((await fake.waitForCall('AddOptions')).args, [focus]);
  await page.waitForSelector('#palette-query');
  assert.equal(await page.evaluate(() => window.bench.store.ui.add.paneId), focus);
  await page.keyboard.press('Escape');
  await page.waitForSelector('#palette-query', { state: 'detached' });

  // Help ▸ Keyboard Shortcuts opens the Keyboard Shortcuts sheet, whose
  // Change Shortcuts… opens Settings at its Keyboard section.
  await fake.emit('bench:command', { name: 'keyboard-shortcuts' });
  await page.waitForSelector('.shortcuts-sheet');
  await page.click('.shortcuts-sheet [data-key="shortcuts-change"]');
  await page.waitForSelector('.settings-sheet #settings-keyboard');
  await settle(page);
  assert.deepEqual(fake.errors, []);
});
