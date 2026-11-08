'use strict';

// What fights Bench (decision 69): a window that its app, or a window tool
// such as Rectangle, keeps moving out of its pane, which its pane and
// sub-tab say, with Dock again; the notice about a window tool, with
// Keyboard settings and Dismiss; and the demo bar's Rectangle and Keep
// moving window.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench } = require('./helpers/harness.js');
const { defaultWorld } = require('./helpers/fixture.js');

const MOVING = 'Brave or Rectangle keeps moving this window';

test('a window that keeps moving out of its pane says so, with a warning mark and Dock again', async (t) => {
  const world = defaultWorld({ current: 2 });
  world.items.w7.status = 'moving';
  world.items.w7.statusText = MOVING;
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  const body = page.locator('.pane[data-pane-id="p5"] .pane-status');
  assert.equal(await body.getAttribute('data-status'), 'moving');
  assert.equal(await body.locator('.status-text').textContent(), MOVING);
  assert.deepEqual(await body.locator('button').allTextContents(), ['Dock again']);

  // Its sub-tab has the warning mark, says why in its tooltip, and names
  // it for a screen reader.
  const sub = page.locator('.subtab[data-item="w7"]');
  assert.equal(await sub.getAttribute('data-marker'), 'problem');
  assert.equal(await sub.locator('.status-mark').getAttribute('data-marker'), 'problem');
  assert.match(await sub.getAttribute('title'), new RegExp(MOVING + '\\.'));
  assert.equal(await sub.getAttribute('aria-label'), 'Wails v3 docs, Brave (keeps moving out of the pane)');
  // It needs attention, as a window moved out of its pane does.
  assert.match(await page.textContent('#statusbar'), /1 window needs attention/);

  await body.locator('[data-action="redock"]').click();
  assert.deepEqual((await fake.waitForCall('Redock')).args, ['w7']);
  // The menu offers Dock Again too.
  await page.click('.pane[data-pane-id="p5"] [data-action="menu"]');
  await page.click('.menu-item:has-text("Dock Again")');
  assert.equal(fake.callsTo('Redock').length, 2);
});

test('a window that keeps moving says so even when Go gives no text', async (t) => {
  const world = defaultWorld({ current: 2 });
  world.items.w7.status = 'moving';
  const { page, close } = await openBench({ world });
  t.after(close);
  assert.equal(await page.textContent('.pane[data-pane-id="p5"] .status-text'), 'Brave or another tool keeps moving this window.');
});

// toolNotice is Rectangle's notice, as Go gives it, naming the fields
// whose keys Rectangle uses too.
function toolNotice(fields) {
  return {
    key: 'window-tool:com.knollsoft.rectangle',
    text: 'Rectangle is running. It can move docked windows out of their panes. It uses the same keys as Bench for '
      + fields + '. macOS doesn\'t warn about this, and both apps may act on these keys. '
      + 'Change the keys in Rectangle or in Keyboard settings.',
    actions: [
      { label: 'Keyboard settings', command: 'keyboard-settings' },
      { label: 'Dismiss', method: 'DismissWindowTool', arg: 'com.knollsoft.Rectangle' },
    ],
  };
}

const ALL = 'Focus a pane (⌃⌥←, ⌃⌥→, ⌃⌥↑, ⌃⌥↓), Move a window (⌃⌥⇧↑), and Zoom pane (⌃⌥↩)';

test('the notice about a window tool opens Settings ▸ Keyboard, or goes when dismissed', async (t) => {
  const notice = toolNotice(ALL);
  const { page, fake, close } = await openBench({ world: defaultWorld({ notices: [notice] }) });
  t.after(close);
  // The status bar starts with it.
  assert.match(await page.textContent('#statusbar .status-notice'), /^Rectangle is running\./);

  await page.click('#statusbar [data-action="notices"]');
  await page.waitForSelector('#notices-menu');
  assert.match(await page.textContent('#notices-menu'), /Focus a pane \(⌃⌥←, ⌃⌥→, ⌃⌥↑, ⌃⌥↓\)/);
  const buttons = page.locator('#notices-menu [data-action="notice-run"]');
  assert.deepEqual(await buttons.allTextContents(), ['Keyboard settings', 'Dismiss']);
  await buttons.first().click();
  await page.waitForSelector('.settings-sheet');
  await page.waitForFunction(() => window.bench.store.ui.settings && window.bench.store.ui.settings.section === 'keyboard');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.settings-sheet', { state: 'detached' });

  // Dismiss, from the keyboard.
  await page.click('#statusbar [data-action="notices"]');
  await page.waitForSelector('#notices-menu');
  await page.focus('#notices-menu [data-action="notice-run"][data-index="1"]');
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('DismissWindowTool')).args, ['com.knollsoft.Rectangle']);
  await page.waitForFunction(() => !(window.bench.store.view.notices || []).length);
});

// A notice whose text changes while it shows, as Rectangle's does as the
// user changes Bench's shortcuts, keeps one place in the recent messages,
// with its new text: no copy of the old text, which names a clash that has
// gone, stays among them.
test('a window tool\'s notice that changes leaves no stale copy in the recent messages', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ notices: [toolNotice(ALL)] }) });
  t.after(close);
  fake.world.notices = [toolNotice('Focus a pane (⌃⌥←, ⌃⌥→, ⌃⌥↑, ⌃⌥↓) and Move a window (⌃⌥⇧↑)')];
  await fake.emitState();
  await page.waitForFunction(() => !window.bench.store.view.notices[0].text.includes('Zoom pane'));
  // Another notice comes and goes, so that the list has recent messages.
  fake.world.notices = [...fake.world.notices, 'Couldn\'t register ⌃⌥J.'];
  await fake.emitState();
  await page.waitForFunction(() => window.bench.store.view.notices.length === 2);
  fake.world.notices = fake.world.notices.slice(0, 1);
  await fake.emitState();
  await page.waitForFunction(() => window.bench.store.view.notices.length === 1);
  await page.click('#statusbar [data-action="notices"]');
  await page.waitForSelector('#notices-menu');
  const text = await page.textContent('#notices-menu');
  assert.match(text, /Couldn't register ⌃⌥J\./);
  assert.equal(text.split('Rectangle is running').length - 1, 1, 'Rectangle\'s notice shows once: ' + text);
  assert.doesNotMatch(text, /Zoom pane/);

  // Once it has gone, the recent messages keep its latest text, once.
  await page.keyboard.press('Escape');
  fake.world.notices = [];
  await fake.emitState();
  await page.waitForFunction(() => !window.bench.store.view.notices.length);
  await page.click('#statusbar [data-action="notices"]');
  await page.waitForSelector('#notices-menu');
  const recent = await page.textContent('#notices-menu');
  assert.equal(recent.split('Rectangle is running').length - 1, 1, 'the recent messages keep it once: ' + recent);
  assert.doesNotMatch(recent, /Zoom pane/);
});

test('the demo bar starts Rectangle, with its button pressed, and has the shown window keep moving', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ demo: true }) });
  t.after(close);
  const tool = page.locator('#demo-window-tool');
  assert.equal(await tool.textContent(), 'Rectangle');
  assert.equal(await tool.getAttribute('aria-pressed'), 'false');
  // It's in the list that Simulate shows.
  await page.click('#demo-simulate');
  await tool.click();
  assert.deepEqual((await fake.waitForCall('SetDemoWindowTool')).args, [true]);
  await page.waitForSelector('#demo-window-tool[aria-pressed="true"]', { state: 'attached' });
  await page.click('#demo-simulate');
  await tool.click();
  await fake.waitForCall('SetDemoWindowTool', (a) => a[0] === false);
  await page.waitForSelector('#demo-window-tool[aria-pressed="false"]', { state: 'attached' });

  const moves = page.locator('#demo-bar [data-action="demo-tool-moves"]');
  assert.equal(await moves.textContent(), 'Keep moving window');
  await page.click('#demo-simulate');
  await moves.click();
  assert.deepEqual((await fake.waitForCall('DemoToolMoves')).args, []);
  // The bar stays one line at this width.
  const tops = await page.$$eval('#demo-bar > *', (els) => els.filter((el) => !el.hidden)
    .map((el) => Math.round(el.getBoundingClientRect().top + el.getBoundingClientRect().height / 2)));
  assert.equal(new Set(tops).size, 1, 'the demo bar is one line: ' + tops);
});
