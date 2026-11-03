'use strict';

// More room for apps: hiding the status bar, whose notes and toasts then
// show in the title bar; a slim title bar on a narrow window or a small
// screen, where the layout control is one button with a menu, and Jump to…
// its icon; and when tabs show their shortcuts.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, openSettings, openTabProperties, settle } = require('./helpers/harness.js');
const { defaultWorld } = require('./helpers/fixture.js');

const CHANGED = 'config.toml was changed outside Bench, so Bench didn\'t save its own changes over it.';
const NOTICES = [
  { text: CHANGED, actions: [{ label: 'Reload', method: 'ReloadConfig' }, { label: 'Overwrite', method: 'OverwriteConfig' }] },
  'Couldn\'t register ⌃⌥Space.',
];

// hidden returns a world whose status bar is hidden.
function hidden(opts) {
  const world = defaultWorld(opts);
  world.settings.showStatusBar = false;
  return world;
}

// boxes returns the bounding boxes of the title bar and of what's in it, in
// the page's coordinates.
function boxes(page) {
  return page.evaluate(() => {
    const box = (sel) => {
      const el = document.querySelector(sel);
      if (!el) return null;
      const r = el.getBoundingClientRect();
      return { left: r.left, right: r.right, top: r.top, bottom: r.bottom, width: r.width };
    };
    const title = document.getElementById('titlebar');
    return {
      title: box('#titlebar'),
      padding: parseFloat(getComputedStyle(title).paddingLeft),
      traffic: box('#titlebar .traffic'),
      plus: box('#titlebar .new-tab'),
      end: box('#titlebar .titlebar-end'),
      toast: box('#toast'),
    };
  });
}

// toastShown waits for the toast to show text.
async function toastShown(page, fake, text) {
  await fake.emit('bench:toast', text);
  await page.waitForFunction((t) => {
    const el = document.getElementById('toast');
    return el.classList.contains('show') && el.textContent === t;
  }, text);
}

test('hiding the status bar gives its height to the panes, and Settings shows it again', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  const before = fake.world.area;
  const bar = await page.locator('#statusbar').boundingBox();
  assert.ok(bar.height >= 30, 'the status bar is ' + bar.height + ' high');

  // Settings' Appearance group hides it, as Go is told.
  await openSettings(page);
  const box = page.locator('[data-setting="showStatusBar"]');
  assert.equal(await box.isChecked(), true);
  assert.equal(await box.getAttribute('aria-describedby'), 'status-bar-help');
  assert.match(await page.textContent('#status-bar-help'), /panes are taller/);
  await box.uncheck();
  await page.click('[data-action="save-settings"]');
  assert.equal((await fake.waitForCall('SaveSettings')).args[0].showStatusBar, false);
  await page.waitForSelector('.settings-sheet', { state: 'detached' });

  // The status bar goes, and the pane area Go is told of grows by its
  // height, so that docked windows grow too.
  await page.waitForSelector('#statusbar', { state: 'hidden' });
  const grown = await fake.waitForCall('SetPaneArea', ([a]) => a.h > before.h);
  const area = grown.args[0];
  assert.equal(area.h - before.h, Math.round(bar.height));
  assert.deepEqual([area.x, area.y, area.w], [before.x, before.y, before.w]);
  // The panes are drawn to the window's foot.
  const foot = await page.evaluate(() => {
    const win = document.getElementById('app-window').getBoundingClientRect();
    const panes = Array.from(document.querySelectorAll('.pane'), (el) => el.getBoundingClientRect().bottom);
    return { win: win.bottom, panes: Math.max(...panes) };
  });
  assert.ok(foot.win - foot.panes <= 8, 'the panes end ' + (foot.win - foot.panes) + ' pt above the window\'s foot');

  // Ticking it again brings the status bar back, and the panes shrink.
  await openSettings(page);
  assert.equal(await box.isChecked(), false);
  await box.check();
  await page.click('[data-action="save-settings"]');
  assert.equal((await fake.waitForCall('SaveSettings', ([s]) => s.showStatusBar === true)).args[0].showStatusBar, true);
  await page.waitForSelector('#statusbar .status-count');
  await fake.waitForCall('SetPaneArea', ([a]) => a.h === before.h);
});

test('with the status bar hidden, its notes are buttons in the title bar beside Jump to…', async (t) => {
  const world = hidden({ notices: NOTICES });
  world.items.w4.status = 'undocked';
  world.items.w4.statusText = 'Moved out of its pane';
  // On a dark Mac, as the amber below is the dark set's.
  const { page, fake, close } = await openBench({ world, contextOptions: { colorScheme: 'dark' } });
  t.after(close);
  assert.equal(await page.isVisible('#statusbar'), false);
  assert.equal(await page.$('#statusbar [data-action]'), null);

  // The windows that need attention, then the notices, then Jump to….
  const order = await page.$$eval('#titlebar .titlebar-end > *', (els) => els.filter((el) => el.offsetParent).map((el) => el.dataset.action));
  assert.deepEqual(order.slice(-4), ['attention', 'notices', 'open-palette', 'open-settings']);
  const attention = page.locator('#titlebar [data-action="attention"]');
  assert.equal(await attention.textContent(), '1 needs attention');
  assert.equal(await attention.getAttribute('aria-label'), '1 window needs attention');
  const notices = page.locator('#titlebar [data-action="notices"]');
  assert.equal(await notices.textContent(), '2 notices');
  assert.equal(await notices.getAttribute('title'), CHANGED + '\nCouldn\'t register ⌃⌥Space.');
  // Both are amber, as warnings are.
  const colours = await page.$$eval('#titlebar .title-note', (els) => els.map((el) => getComputedStyle(el).color));
  assert.deepEqual(colours, ['rgb(242, 201, 125)', 'rgb(242, 201, 125)']);

  // Each opens the same list as in the status bar, below it.
  await notices.click();
  await page.waitForSelector('#notices-menu');
  assert.equal(await notices.getAttribute('aria-expanded'), 'true');
  const under = async (sel, button) => {
    const b = await button.boundingBox();
    const m = await page.locator(sel).boundingBox();
    assert.ok(m.y >= b.y + b.height, sel + ' starts below its button');
    assert.ok(m.y + m.height <= 900, sel + ' fits in the window');
  };
  await under('#notices-menu', notices);
  await page.click('#notices-menu button:has-text("Reload")');
  await fake.waitForCall('ReloadConfig');
  await page.waitForSelector('#notices-menu', { state: 'detached' });
  // Reloading took away the notice with buttons.
  await page.waitForSelector('#titlebar [data-action="notices"]:has-text("1 notice")');

  await attention.click();
  await page.waitForSelector('#attention-menu');
  await under('#attention-menu', attention);
  await page.keyboard.press('Escape');
  await page.waitForSelector('#attention-menu', { state: 'detached' });
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'attention');

  // With nothing to say, neither is drawn.
  fake.world.notices = [];
  fake.world.items.w4.status = 'ready';
  await fake.emitState();
  await page.waitForSelector('#titlebar .title-note', { state: 'detached' });
});

test('with the status bar hidden, Settings and Tab properties say the title bar lists config.toml\'s problems', async (t) => {
  const { page, fake, close } = await openBench({ world: hidden({ notices: NOTICES, configWritable: false }) });
  t.after(close);
  const lead = 'config.toml has problems, so changes apply but aren\'t saved to it. ';
  await openSettings(page);
  assert.equal(await page.textContent('.settings-sheet .editor-note'), lead + 'The notices in the title bar list the problems.');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
  await openTabProperties(page);
  assert.equal(await page.textContent('.editor-sheet .editor-note'), lead + 'The notices in the title bar list the problems.');

  // With the status bar back, they say it lists them.
  fake.world.settings.showStatusBar = true;
  await fake.emitState();
  await page.waitForFunction((text) => document.querySelector('.editor-sheet .editor-note').textContent === text,
    lead + 'The status bar lists the problems.');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.editor-sheet', { state: 'detached' });
  await openSettings(page);
  assert.equal(await page.textContent('.settings-sheet .editor-note'), lead + 'The status bar lists the problems.');
});

test('with the status bar hidden, a toast shows briefly in the title bar\'s empty space', async (t) => {
  const { page, fake, close } = await openBench({ world: hidden() });
  t.after(close);
  await toastShown(page, fake, 'Rearranged Code');
  assert.equal(await page.$eval('#toast', (el) => el.classList.contains('in-titlebar')), true);
  const b = await boxes(page);
  // In the title bar, between + and the controls at its right, and in the
  // middle of that space.
  assert.ok(b.toast.top >= b.title.top && b.toast.bottom <= b.title.bottom, 'the toast is within the title bar');
  assert.ok(b.toast.left >= b.plus.right && b.toast.right <= b.end.left, 'the toast is between + and the controls');
  const middle = (b.plus.right + b.end.left) / 2;
  assert.ok(Math.abs((b.toast.left + b.toast.right) / 2 - middle) <= 1, 'the toast is centred in the empty space');
  // Clicks go through it to the title bar, which moves the window.
  assert.equal(await page.$eval('#toast', (el) => getComputedStyle(el).pointerEvents), 'none');

  // A long message is cut short to one line, and goes after 4 seconds at
  // most, rather than the 6 it would show for at the window's foot.
  const long = 'Couldn\'t open Obsidian: it didn\'t start within 15 seconds. Open it yourself, then choose Rearrange from the View menu.';
  await toastShown(page, fake, long);
  const shown = Date.now();
  assert.equal(await page.$eval('#toast', (el) => getComputedStyle(el).whiteSpace), 'nowrap');
  const after = await boxes(page);
  assert.ok(after.toast.right <= after.end.left && after.toast.left >= after.plus.right, 'the long toast stays in the empty space');
  await page.waitForFunction(() => !document.getElementById('toast').classList.contains('show'), null, { timeout: 5000 });
  assert.ok(Date.now() - shown <= 4600, 'the toast went after ' + (Date.now() - shown) + ' ms');

  // With the status bar back, toasts sit at the window's foot again.
  fake.world.settings.showStatusBar = true;
  await fake.emitState();
  await page.waitForSelector('#statusbar .status-count');
  await toastShown(page, fake, 'Rearranged Code');
  assert.equal(await page.$eval('#toast', (el) => el.classList.contains('in-titlebar') || el.style.left !== ''), false);
  const foot = await boxes(page);
  assert.ok(foot.toast.top > foot.title.bottom, 'the toast is below the title bar');
});

test('in a crowded title bar, a toast goes over the tabs, but never over the traffic lights or the controls', async (t) => {
  // crowded is a world whose title bar's empty space is too narrow for a
  // toast: the tabs show their shortcuts, and there are notes.
  const crowded = (opts) => {
    const world = hidden({ notices: NOTICES, ...opts });
    world.settings.tabShortcutHints = 'always';
    world.items.w4.status = 'undocked';
    return world;
  };
  const message = 'Splits are hidden on small screens.';
  // In the app, the traffic lights are in the title bar's left padding.
  const app = await openBench({ world: crowded(), viewport: { width: 900, height: 600 } });
  t.after(app.close);
  await toastShown(app.page, app.fake, message);
  let b = await boxes(app.page);
  assert.ok(b.end.left - b.plus.right - 16 < 240, 'the empty space is narrow');
  assert.ok(b.toast.left >= b.title.left + b.padding, 'the toast is clear of the traffic lights: ' + b.toast.left + ' < ' + (b.title.left + b.padding));
  assert.ok(b.toast.right <= b.end.left, 'the toast is clear of the controls');
  assert.ok(b.toast.left < b.plus.right, 'the toast is over the tabs');
  assert.ok(b.toast.top >= b.title.top && b.toast.bottom <= b.title.bottom, 'the toast is within the title bar');
  // Clicks go through it to the tabs.
  await app.page.click('.tab[data-index="0"]', { position: { x: 20, y: 18 } });
  await app.fake.waitForCall('SwitchTab', (a) => a[0] === 0);

  // The browser preview draws them at the title bar's start.
  const demo = await openBench({ world: crowded({ demo: true }), viewport: { width: 900, height: 600 } });
  t.after(demo.close);
  await toastShown(demo.page, demo.fake, message);
  b = await boxes(demo.page);
  assert.ok(b.traffic, 'the preview draws traffic lights');
  assert.ok(b.toast.left >= b.traffic.right, 'the toast is clear of the traffic lights');
  assert.ok(b.toast.right <= b.end.left, 'the toast is clear of the controls');
  assert.ok(b.toast.top >= b.title.top && b.toast.bottom <= b.title.bottom, 'the toast is within the title bar');
});

test('at 900 by 600, the layout control is one button with a menu, and Jump to… its icon', async (t) => {
  const { page, fake, close } = await openBench({ viewport: { width: 900, height: 600 } });
  t.after(close);
  // The screen is wide, but the window is narrow.
  assert.equal(await page.evaluate(() => window.bench.store.view.sizeClass), 'wide');
  await page.waitForSelector('#titlebar[data-slim="true"]');
  assert.equal(await page.isVisible('#titlebar .layout-seg'), false);
  const button = page.locator('#titlebar [data-action="layout-menu"]');
  assert.equal(await button.isVisible(), true);
  assert.equal(await button.textContent(), 'Auto');
  assert.equal(await page.isVisible('#titlebar [data-action="layout-menu"] svg'), true);
  assert.equal(await button.getAttribute('aria-label'), 'Layout: Auto');
  assert.equal(await button.getAttribute('aria-haspopup'), 'menu');
  assert.equal(await button.getAttribute('title'), 'Wide layout, chosen for a 2,560-pt screen');

  // Jump to… is its search icon, with the shortcut in its name and tooltip.
  const jump = page.locator('#titlebar [data-action="open-palette"]');
  assert.equal(await jump.getAttribute('aria-label'), 'Jump to… (⌃⌥Space)');
  assert.equal(await jump.getAttribute('title'), 'Jump to… (⌃⌥Space)');
  const size = await jump.boundingBox();
  assert.ok(size.width <= 34, 'Jump to… is ' + size.width + ' wide');
  assert.equal(await page.isVisible('#titlebar [data-action="open-palette"] span'), false);
  assert.equal(await page.isVisible('#titlebar [data-action="open-palette"] kbd'), false);
  await jump.click();
  await page.waitForSelector('.palette');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.palette', { state: 'detached' });

  // The tabs have more room: all three fit without scrolling.
  const tabs = await page.$eval('#titlebar .tabs', (el) => el.scrollWidth <= el.clientWidth);
  assert.equal(tabs, true);

  // The menu lists the layouts, with the one in use ticked and focused, and
  // says why the layout is as it is.
  await button.click();
  await page.waitForSelector('#layout-menu');
  assert.equal(await button.getAttribute('aria-expanded'), 'true');
  const items = await page.$$eval('#layout-menu [role="menuitemradio"]', (els) => els.map((el) => [el.textContent, el.getAttribute('aria-checked')]));
  assert.deepEqual(items, [['Auto', 'true'], ['Wide', 'false'], ['Compact', 'false']]);
  assert.equal(await page.textContent('#layout-menu .menu-note'), 'Wide layout, chosen for a 2,560-pt screen.');
  assert.equal(await page.evaluate(() => document.activeElement.textContent), 'Auto');
  const b = await button.boundingBox();
  const m = await page.locator('#layout-menu').boundingBox();
  assert.ok(m.y >= b.y + b.height, 'the menu opens below its button');
  assert.ok(m.x + m.width <= 900, 'the menu fits in the window');
  await fake.waitForCall('SetOverlay', (a) => a[0] === true);

  // Escape closes it, and gives the focus back to the button.
  await page.keyboard.press('Escape');
  await page.waitForSelector('#layout-menu', { state: 'detached' });
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'layout-menu');

  // The keyboard chooses a layout, and the button names it.
  await page.keyboard.press('Enter');
  await page.waitForSelector('#layout-menu');
  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('SetMode')).args, ['compact']);
  await page.waitForSelector('#layout-menu', { state: 'detached' });
  await page.waitForSelector('#titlebar [data-action="layout-menu"]:has-text("Compact")');
  assert.equal(await button.getAttribute('aria-label'), 'Layout: Compact');

  // In a tab shown as one pane, the split button stays, off, and says why.
  await page.click('.tab[data-index="0"]');
  await page.waitForSelector('.pane[data-pane-id="compact"]');
  const split = page.locator('[data-action="split-right"]');
  assert.equal(await split.getAttribute('aria-disabled'), 'true');
  assert.equal(await split.getAttribute('title'), 'Splits are hidden on small screens.');
});

test('a wide window\'s title bar shows the whole layout control and Jump to…', async (t) => {
  const { page, close } = await openBench();
  t.after(close);
  assert.equal(await page.getAttribute('#titlebar', 'data-slim'), 'false');
  assert.equal(await page.isVisible('#titlebar .layout-seg'), true);
  assert.equal(await page.isVisible('#titlebar [data-action="layout-menu"]'), false);
  assert.equal(await page.isVisible('#titlebar [data-action="open-palette"] span'), true);
  assert.equal(await page.textContent('#titlebar [data-action="open-palette"] kbd'), '⌃⌥Space');

  // Narrowing the window slims it, and widening it again doesn't.
  await page.setViewportSize({ width: 1000, height: 900 });
  await page.waitForSelector('#titlebar[data-slim="true"]');
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.waitForSelector('#titlebar[data-slim="false"]');
});

test('a small screen slims the title bar however wide the window is', async (t) => {
  const { page, close } = await openBench({ world: defaultWorld({ screenWidth: 1512 }) });
  t.after(close);
  assert.equal(await page.evaluate(() => window.bench.store.view.sizeClass), 'compact');
  await page.waitForSelector('#titlebar[data-slim="true"]');
  assert.equal(await page.textContent('#titlebar [data-action="layout-menu"]'), 'Auto');
});

test('choosing a layout that changes the title bar keeps the focus on the layout control', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ mode: 'compact' }) });
  t.after(close);
  const focused = () => page.evaluate(() => document.activeElement.dataset.key);
  await page.waitForSelector('#titlebar[data-slim="true"]');
  await page.focus('[data-action="layout-menu"]');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#layout-menu');
  // The focus starts on Compact, the layout in use.
  assert.equal(await page.evaluate(() => document.activeElement.textContent), 'Compact');
  await page.keyboard.press('ArrowUp');
  await page.keyboard.press('ArrowUp');
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('SetMode')).args, ['auto']);
  // The screen is wide, so the whole control comes back, and its Auto
  // takes the focus.
  await page.waitForSelector('#titlebar[data-slim="false"]');
  assert.equal(await focused(), 'mode-auto');

  // Choosing Compact with the keyboard slims the title bar again, and the
  // layout button takes the focus.
  await page.focus('[data-key="mode-compact"]');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#titlebar[data-slim="true"]');
  assert.equal(await focused(), 'layout-menu');
});

test('tabs show their shortcuts as tab_shortcut_hints says', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  const hints = () => page.$$eval('#titlebar .tab', (els) => els.map((el) => {
    const kbd = el.querySelector('kbd');
    return [el.querySelector('.tab-name').textContent, kbd ? kbd.textContent : null];
  }));
  const titles = () => page.$$eval('#titlebar .tab', (els) => els.map((el) => el.title));
  const tooltips = ['Comms (⌃⌥1)', 'Code (⌃⌥2)', 'Research (⌃⌥3)'];

  // Automatic: on a wide layout, they show.
  assert.deepEqual(await hints(), [['Comms', '⌃⌥1'], ['Code', '⌃⌥2'], ['Research', '⌃⌥3']]);
  // On a small screen they make room for the tabs, and are in the tooltips.
  fake.world.mode = 'compact';
  await fake.emitState();
  await page.waitForFunction(() => !document.querySelector('#titlebar .tab kbd'));
  assert.deepEqual(await hints(), [['Comms', null], ['Code', null], ['Research', null]]);
  assert.deepEqual(await titles(), tooltips);

  // Always: they show on a small screen too.
  fake.world.settings.tabShortcutHints = 'always';
  await fake.emitState();
  await page.waitForSelector('#titlebar .tab kbd');
  assert.deepEqual(await hints(), [['Comms', '⌃⌥1'], ['Code', '⌃⌥2'], ['Research', '⌃⌥3']]);

  // Never: not even on a wide layout.
  fake.world.settings.tabShortcutHints = 'never';
  fake.world.mode = 'auto';
  await fake.emitState();
  await page.waitForFunction(() => !document.querySelector('#titlebar .tab kbd') && window.bench.store.view.sizeClass === 'wide');
  assert.deepEqual(await titles(), tooltips);
});

test('Settings offers 8, 4, and 2 pt between panes, and when tabs show their shortcuts', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await openSettings(page);
  const choices = (name) => page.$$eval('[data-choice="' + name + '"]', (els) => els.map((el) => [el.textContent, el.getAttribute('aria-pressed')]));
  assert.deepEqual(await choices('gap'), [['8 pt', 'false'], ['4 pt', 'true'], ['2 pt', 'false']]);
  assert.deepEqual(await choices('tabShortcutHints'), [['Automatic', 'true'], ['Always', 'false'], ['Never', 'false']]);
  assert.equal(await page.getAttribute('.settings-choice:has([data-choice="gap"]) .seg', 'aria-labelledby'), 'gap-label');
  assert.equal(await page.textContent('#gap-label'), 'Space between panes');
  assert.equal(await page.textContent('#tab-shortcut-hints-label'), 'Show shortcuts on tabs');
  assert.match(await page.textContent('#tab-shortcut-hints-help'), /except on small screens/);

  await page.click('[data-choice="gap"][data-value="2"]');
  await page.click('[data-choice="tabShortcutHints"][data-value="never"]');
  assert.deepEqual(await choices('gap'), [['8 pt', 'false'], ['4 pt', 'false'], ['2 pt', 'true']]);
  assert.match(await page.textContent('#tab-shortcut-hints-help'), /only their names/);
  await page.click('[data-action="save-settings"]');
  const saved = (await fake.waitForCall('SaveSettings')).args[0];
  assert.equal(saved.gap, 2);
  assert.equal(saved.tabShortcutHints, 'never');
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
  await page.waitForFunction(() => !document.querySelector('#titlebar .tab kbd'));
  // The panes are 2 pt apart.
  await settle(page);
  const [a, b] = await page.$$eval('.pane', (els) => els.map((el) => el.getBoundingClientRect()));
  assert.equal(Math.round(b.left - a.right), 2);
});

test('Settings keeps a gap from config.toml that it doesn\'t offer', async (t) => {
  const world = defaultWorld();
  world.settings.gap = 6;
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await openSettings(page);
  const gaps = await page.$$eval('[data-choice="gap"]', (els) => els.map((el) => [el.textContent, el.getAttribute('aria-pressed')]));
  assert.deepEqual(gaps, [['8 pt', 'false'], ['4 pt', 'false'], ['2 pt', 'false'], ['6 pt', 'true']]);
  await page.click('[data-action="save-settings"]');
  assert.equal((await fake.waitForCall('SaveSettings')).args[0].gap, 6);
});
