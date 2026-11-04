'use strict';

// Room for apps, part two: the compact density's smaller chrome, the "tabs"
// view's sub-tabs in the title bar in place of its pane's header, and panes
// that go without their header under pane_headers = "auto".

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, openSettings, settle } = require('./helpers/harness.js');
const { defaultWorld } = require('./helpers/fixture.js');

// world returns the default world with some settings changed.
function world(settings = {}, opts = {}) {
  const w = defaultWorld(opts);
  Object.assign(w.settings, settings);
  return w;
}

// tabsView returns a world on a laptop's screen, the compact size class,
// showing Comms, whose small-screen setting is "tabs": one pane, with every
// window as a sub-tab.
function tabsView(settings = {}) {
  return world(settings, { screenWidth: 1512, current: 0 });
}

// sizes returns the heights of the title bar and its tabs and +, and of the
// first pane header and its sub-tabs and buttons, rounded to 0.1 pt.
function sizes(page) {
  return page.evaluate(() => {
    const h = (sel) => {
      const el = document.querySelector(sel);
      return el ? Math.round(el.getBoundingClientRect().height * 10) / 10 : null;
    };
    return {
      title: h('#titlebar'),
      tab: h('#titlebar .tab'),
      plus: h('#titlebar .new-tab'),
      header: h('.pane-header'),
      subtab: h('.pane-header .subtab'),
      add: h('.pane-header .add-btn'),
      more: h('.pane-header [data-action="menu"]'),
    };
  });
}

// bodies returns, for each pane Go sent, whether its body is drawn where Go
// docks its window: below the pane's header, as tall as Go says.
function bodies(page) {
  return page.evaluate(() => {
    const v = window.bench.store.view;
    const area = v.display.area;
    const panes = document.getElementById('panes').getBoundingClientRect();
    return v.display.panes.map((p) => {
      const b = document.querySelector('.pane[data-pane-id="' + p.id + '"] .pane-body').getBoundingClientRect();
      return [p.id, Math.round(b.top - panes.top) - (p.body.y - area.y), Math.round(b.height) - p.body.h];
    });
  });
}

// redraw sends the world as it is now, and waits for the redraw and the
// pane area that follows a change to the title bar's height.
async function redraw(page, fake) {
  await fake.emitState();
  await settle(page);
  await page.waitForFunction(() => {
    const v = window.bench.store.view;
    return Math.round(document.getElementById('panes').getBoundingClientRect().height) === v.display.area.h;
  });
  await settle(page);
}

const STANDARD = { title: 53, tab: 36, plus: 32, header: 41, subtab: 30, add: 28, more: 28 };
const COMPACT = { title: 39, tab: 30, plus: 28, header: 33, subtab: 26, add: 24, more: 24 };

test('the compact density makes the chrome smaller, and each pane header is as tall as Go says', async (t) => {
  const { page, fake, close } = await openBench({ world: world({ density: 'compact' }) });
  t.after(close);
  await redraw(page, fake);
  assert.deepEqual(await sizes(page), COMPACT);
  assert.equal(await page.getAttribute('body', 'data-density'), 'compact');
  assert.deepEqual(await bodies(page), [['p3', 0, 0], ['p4', 0, 0]]);
  assert.deepEqual(await page.$$eval('.pane', (els) => els.map((el) => el.style.getPropertyValue('--pane-header'))), ['32px', '32px']);

  fake.world.settings.density = 'standard';
  await redraw(page, fake);
  assert.deepEqual(await sizes(page), STANDARD);
  assert.deepEqual(await bodies(page), [['p3', 0, 0], ['p4', 0, 0]]);

  // Automatic is standard on a wide screen, and compact on a laptop's.
  fake.world.settings.density = 'auto';
  await redraw(page, fake);
  assert.deepEqual(await sizes(page), STANDARD);
  fake.world.screenWidth = 1512;
  await redraw(page, fake);
  assert.equal(fake.view().sizeClass, 'compact');
  assert.deepEqual(await sizes(page), COMPACT);
  assert.deepEqual(await bodies(page), [['p3', 0, 0], ['p4', 0, 0]]);

  // In the app, the title bar keeps the density it opened with, to match
  // the native title bar, while the panes follow the setting.
  fake.world.titleBar = 'standard';
  await redraw(page, fake);
  const both = await sizes(page);
  assert.deepEqual([both.title, both.tab, both.header, both.subtab], [53, 36, 33, 26]);
  assert.deepEqual(await bodies(page), [['p3', 0, 0], ['p4', 0, 0]]);
});

test('the "tabs" view puts its pane\'s sub-tabs and controls in the title bar, after the tabs', async (t) => {
  const w = tabsView();
  w.items.w2.status = 'missing';
  w.items.w2.statusText = 'Outlook isn\'t open.';
  const { page, fake, close } = await openBench({ world: w, viewport: { width: 1280, height: 800 } });
  t.after(close);
  await redraw(page, fake);
  const v = fake.view();
  assert.equal(v.display.compact, true);
  assert.equal(v.density, 'compact');
  // The pane has no header: it reaches up to the title bar.
  assert.equal(await page.locator('.pane-header').count(), 0);
  assert.equal(await page.getAttribute('.pane[data-pane-id="compact"]', 'data-header'), 'title-bar');
  const [p] = v.display.panes;
  assert.equal(p.header, 0);
  assert.equal(p.rect.y, v.display.area.y);
  assert.equal(p.body.y, v.display.area.y + 1);
  assert.deepEqual(await bodies(page), [['compact', 0, 0]]);
  const tops = await page.evaluate(() => [
    document.getElementById('panes').getBoundingClientRect().top,
    document.querySelector('.pane').getBoundingClientRect().top,
  ]);
  assert.equal(tops[1], tops[0]);

  // The title bar has the tabs, +, the pane's sub-tabs and controls, then
  // its own controls, in one row of the compact density's height.
  const row = await page.$$eval('#titlebar > *', (els) => els.map((el) => el.className.split(' ')[0]));
  assert.deepEqual(row, ['tabs', 'icon-btn', 'title-pane', 'titlebar-end']);
  assert.equal((await sizes(page)).title, 39);
  const pane = await page.$$eval('.title-pane > *, .title-pane .pane-actions > *', (els) => els.map((el) => el.dataset.action || el.className.split(' ')[0]));
  assert.deepEqual(pane, ['subtabs', 'add', 'pane-actions', 'host-chip', 'split-right', 'menu']);
  assert.deepEqual(await page.$$eval('.title-pane .subtab', (els) => els.map((el) => el.dataset.item)), ['w1', 'w2']);
  // A sub-tab still shows its window's status.
  assert.equal(await page.locator('.title-pane .subtab[data-item="w2"] .status-mark').count(), 1);
  assert.equal(await page.getAttribute('.title-pane [data-action="split-right"]', 'aria-disabled'), null);
  assert.equal(await page.getAttribute('.title-pane .subtabs', 'data-scroll'), 'subtabs-compact');

  // A click shows a window, a right-click opens its menu, and so does a
  // right-click on the pane's part of the title bar.
  fake.clearCalls();
  await page.click('.title-pane .subtab[data-item="w2"]');
  await fake.waitForCall('ShowItem', (a) => a[0] === 'compact' && a[1] === 'w2');
  await settle(page);
  await page.click('.title-pane .subtab[data-item="w1"]', { button: 'right' });
  await page.waitForSelector('#window-menu');
  assert.equal(await page.evaluate(() => window.bench.store.ui.menu.itemId), 'w1');
  await page.keyboard.press('Escape');
  await page.waitForSelector('#window-menu', { state: 'detached' });
  await page.click('.title-pane', { button: 'right', position: { x: 4, y: 12 } });
  await page.waitForSelector('#window-menu');
  await page.keyboard.press('Escape');
  await page.waitForSelector('#window-menu', { state: 'detached' });

  // + adds to the tab's focused pane.
  fake.clearCalls();
  await page.click('.title-pane .add-btn');
  await fake.waitForCall('AddOptions', (a) => a[0] === 'p1');
  await page.waitForSelector('.palette');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.palette', { state: 'detached' });

  // A sub-tab in the title bar is dragged as in a pane's header.
  fake.clearCalls();
  await page.dragAndDrop('.title-pane .subtab[data-item="w1"]', '.tab[data-index="2"]');
  await fake.waitForLowered();
  assert.deepEqual(fake.callsTo('MoveItemToTab').map((c) => c.args), [['w1', 2]]);
});

test('in the "tabs" view, the shown sub-tab in the title bar has room for its × too', async (t) => {
  const w = tabsView();
  const long = 'A very long window title that goes on and on, well past any sensible width for a sub-tab';
  w.items.w1.title = long;
  w.items.w2.title = long;
  const { page, fake, close } = await openBench({ world: w, viewport: { width: 1440, height: 800 } });
  t.after(close);
  await redraw(page, fake);
  // Cut short, a sub-tab there is 220 pt wide, and the shown one 22 pt
  // more, with its title ending clear of its ×.
  const got = await page.$$eval('.title-pane .subtab', (els) => els.map((el) => {
    const title = el.querySelector('.subtab-title');
    return [el.dataset.item, el.getAttribute('aria-pressed'), Math.round(el.getBoundingClientRect().width),
      title.getBoundingClientRect().right <= el.querySelector('.close-cross').getBoundingClientRect().left];
  }));
  assert.deepEqual(got, [['w1', 'true', 242, true], ['w2', 'false', 220, false]]);
});

test('the "tabs" view\'s title bar stays one row from 640 by 480 to 1470 by 919, with each strip scrolling on its own', async (t) => {
  const w = tabsView();
  // Research has four windows.
  w.current = 2;
  const { page, fake, close } = await openBench({ world: w, viewport: { width: 900, height: 600 } });
  t.after(close);
  // 640 by 480 is Bench's smallest window, and 735 by 925 half a 13-inch
  // MacBook's screen.
  for (const [width, height] of [[640, 480], [735, 925], [760, 600], [761, 600], [900, 600], [1280, 800], [1470, 919]]) {
    await page.setViewportSize({ width, height });
    await redraw(page, fake);
    const got = await page.evaluate(() => {
      const box = (sel) => document.querySelector(sel).getBoundingClientRect();
      const strip = document.querySelector('.title-pane .subtabs');
      return {
        title: Math.round(box('#titlebar').height),
        tabs: box('#titlebar .tabs').width,
        strip: strip.getBoundingClientRect().width,
        overflows: strip.scrollWidth > strip.clientWidth,
        clear: box('.title-pane').right <= box('.titlebar-end').left,
        after: box('.title-pane').left >= box('#titlebar .new-tab').right,
      };
    });
    const at = width + ' by ' + height;
    assert.equal(got.title, 39, at + ': one row');
    assert.ok(got.tabs > 100 && got.strip > 100, at + ': both strips have room: ' + JSON.stringify(got));
    assert.ok(got.clear && got.after, at + ': the pane\'s part sits between + and the controls');
    if (width <= 900) assert.equal(got.overflows, true, 'the sub-tabs scroll at ' + width + ' points');
  }
  // The shown sub-tab is kept in view in the title bar too.
  await page.setViewportSize({ width: 900, height: 600 });
  await redraw(page, fake);
  fake.world.compactShown[2] = 'w10';
  await redraw(page, fake);
  const shown = await page.evaluate(() => {
    const s = document.querySelector('.title-pane .subtabs').getBoundingClientRect();
    const r = document.querySelector('.title-pane .subtab[aria-pressed="true"]').getBoundingClientRect();
    return [document.querySelector('.title-pane .subtab[aria-pressed="true"]').dataset.item, r.left >= s.left - 0.5 && r.right <= s.right + 0.5];
  });
  assert.deepEqual(shown, ['w10', true]);
});

test('with the status bar hidden, a slim title bar stays one row with its notes, in the "tabs" view and the stack view', async (t) => {
  const w = tabsView({ showStatusBar: false });
  const { page, fake, close } = await openBench({ world: w, viewport: { width: 640, height: 480 } });
  t.after(close);
  // Comms and Research show one pane, whose sub-tabs and controls are in
  // the title bar; Code stacks its panes. Slack's pane has a host chip too.
  for (const current of [0, 2, 1]) {
    for (const [width, height] of [[640, 480], [735, 925]]) {
      for (const notes of [['attention'], ['notices'], ['attention', 'notices']]) {
        fake.world.current = current;
        // One window of each tab needs attention, and Bench has a notice.
        for (const id of ['w1', 'w4', 'w7']) {
          Object.assign(fake.world.items[id], notes.includes('attention')
            ? { status: 'undocked', statusText: 'Moved out of its pane' }
            : { status: 'ready', statusText: '' });
        }
        fake.world.notices = notes.includes('notices') ? ['Couldn\'t register ⌃⌥Space.'] : [];
        await page.setViewportSize({ width, height });
        await redraw(page, fake);
        const got = await page.evaluate(() => {
          const box = (sel) => document.querySelector(sel).getBoundingClientRect();
          const pane = document.querySelector('.title-pane');
          const last = pane ? pane.lastElementChild : document.querySelector('#titlebar .new-tab');
          return {
            title: Math.round(box('#titlebar').height),
            notes: Array.from(document.querySelectorAll('#titlebar .title-note'), (el) => [el.dataset.action, el.innerText, el.getAttribute('aria-label')]),
            clear: last.getBoundingClientRect().right <= box('.titlebar-end').left,
            panes: Math.round(box('#panes').top) === Math.round(box('#titlebar').bottom),
          };
        });
        const at = 'tab ' + current + ' at ' + width + ' by ' + height + ' with ' + notes.join(' and ');
        assert.equal(got.title, 39, at + ': one row');
        assert.ok(got.clear, at + ': what comes before the controls ends clear of them');
        assert.ok(got.panes, at + ': the pane area starts below the title bar');
        // A note shows its number, and its name has its words.
        assert.deepEqual(got.notes, [
          ...(notes.includes('attention') ? [['attention', '1', '1 window needs attention']] : []),
          ...(notes.includes('notices') ? [['notices', '1', '1 notice']] : []),
        ], at);
      }
    }
  }
});

test('with the status bar hidden, a toast in the "tabs" view sits after the pane\'s part of the title bar', async (t) => {
  const w = tabsView({ showStatusBar: false });
  const { page, fake, close } = await openBench({ world: w, viewport: { width: 1470, height: 919 } });
  t.after(close);
  await redraw(page, fake);
  await fake.emit('bench:toast', 'Saved settings');
  await page.waitForFunction(() => document.getElementById('toast').classList.contains('show'));
  const got = await page.evaluate(() => {
    const box = (sel) => document.querySelector(sel).getBoundingClientRect();
    const toast = box('#toast');
    const title = box('#titlebar');
    return {
      after: toast.left >= box('.title-pane').right,
      before: toast.right <= box('.titlebar-end').left,
      centred: Math.abs((toast.top + toast.bottom) / 2 - (title.top + (title.height - 1) / 2)) <= 1,
      height: toast.height,
    };
  });
  assert.deepEqual(got, { after: true, before: true, centred: true, height: 28 });
});

test('under pane_headers = "auto", a pane with one app window has no header, and Jump to… adds to it, opens its window\'s menu, and splits it', async (t) => {
  const w = world({ paneHeaders: 'auto' }, { current: 2 });
  w.tabs[2].focus = 'p6';
  const { page, fake, close } = await openBench({ world: w });
  t.after(close);
  await redraw(page, fake);
  // Obsidian's pane has no header; Brave's, with three windows, has one.
  assert.equal(await page.getAttribute('.pane[data-pane-id="p6"]', 'data-header'), 'none');
  assert.equal(await page.locator('.pane[data-pane-id="p6"] .pane-header').count(), 0);
  assert.equal(await page.locator('.pane[data-pane-id="p5"] .pane-header').count(), 1);
  assert.deepEqual(fake.view().display.panes.map((p) => [p.id, p.header]), [['p5', 40], ['p6', 0]]);
  assert.deepEqual(await bodies(page), [['p5', 0, 0], ['p6', 0, 0]]);

  // Jump to… has what the missing header's + and split button do.
  fake.clearCalls();
  await page.click('[data-action="open-palette"]');
  await page.keyboard.type('add to this pane');
  assert.equal(await page.textContent('.palette-item[aria-selected="true"] .palette-label'), 'Add a window or app to this pane…');
  await page.keyboard.press('Enter');
  await fake.waitForCall('AddOptions', (a) => a[0] === 'p6');
  await page.waitForFunction(() => window.bench.store.ui.overlay === 'add');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.palette', { state: 'detached' });

  // Window options… opens what the header's ⋯ button would, inside the
  // pane's top right corner, in the palette's place, so Bench stays raised.
  fake.clearCalls();
  await page.click('[data-action="open-palette"]');
  await page.keyboard.type('window options');
  assert.equal(await page.textContent('.palette-item[aria-selected="true"] .palette-label'), 'Window options…');
  assert.equal(await page.textContent('.palette-item[aria-selected="true"] .palette-hint'), 'Obsidian — Daily note');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#window-menu');
  assert.equal(await page.locator('.palette').count(), 0);
  const menu = await page.evaluate(() => {
    const m = window.bench.store.ui.menu;
    const r = document.getElementById('window-menu').getBoundingClientRect();
    const p = document.querySelector('.pane[data-pane-id="p6"]').getBoundingClientRect();
    return {
      item: m.itemId,
      pane: m.paneId,
      inside: r.left >= p.left && r.right <= p.right + 0.5 && r.top >= p.top && r.top <= p.top + 8,
      focused: document.activeElement.dataset.key,
      entries: Array.from(document.querySelectorAll('#window-menu .menu-item'), (el) => el.textContent),
    };
  });
  assert.deepEqual([menu.item, menu.pane, menu.inside, menu.focused], ['w10', 'p6', true, 'menu-item-0'], JSON.stringify(menu));
  assert.ok(menu.entries.includes('Pop out of Bench'), menu.entries.join(', '));
  assert.deepEqual(fake.callsTo('SetOverlay').map((c) => c.args[0]), [true], 'Bench wasn\'t lowered in between');
  // Escape gives the focus back to where it was before the palette opened.
  await page.keyboard.press('Escape');
  await page.waitForSelector('#window-menu', { state: 'detached' });
  await fake.waitForCall('SetOverlay', (a) => a[0] === false);
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'open-palette');

  await page.click('[data-action="open-palette"]');
  await page.keyboard.type('side by side');
  assert.equal(await page.textContent('.palette-item[aria-selected="true"] .palette-label'), 'Show side by side (split right)');
  assert.equal(await page.textContent('.palette-item[aria-selected="true"] .palette-hint'), 'Obsidian — Daily note');
  assert.equal(await page.textContent('.palette-item[aria-selected="true"] .palette-keys'), '⌃⌥⇧→  ↵');
  await page.keyboard.press('Enter');
  await fake.waitForCall('MoveItem', (a) => JSON.stringify(a) === JSON.stringify(['w10', 2, 'p6', 'right']));

  // A window with something to report gets its header back, which says it.
  fake.world.items.w10.status = 'needs-room';
  fake.world.items.w10.statusText = 'Obsidian needs more room than this pane has.';
  await redraw(page, fake);
  assert.equal(await page.getAttribute('.pane[data-pane-id="p6"]', 'data-header'), null);
  assert.equal(await page.locator('.pane[data-pane-id="p6"] .pane-header .pane-note').count(), 1);
  assert.deepEqual(await bodies(page), [['p5', 0, 0], ['p6', 0, 0]]);
});

test('while splits are hidden, Jump to…\'s "Show side by side" changes to the wide layout', async (t) => {
  const { page, fake, close } = await openBench({ world: tabsView() });
  t.after(close);
  const shown = await page.getAttribute('#titlebar [data-action="menu"]', 'data-item');
  await page.click('[data-action="open-palette"]');
  await page.keyboard.type('side by side');
  assert.equal(await page.locator('.palette-item').count(), 1);
  assert.equal(await page.textContent('.palette-item[aria-selected="true"] .palette-label'), 'Show side by side (wide layout)');
  assert.equal(await page.textContent('.palette-item[aria-selected="true"] .palette-keys'), '⌃⌥⇧→  ↵');
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('ShowSideBySide')).args, [shown]);
  await page.waitForFunction(() => document.querySelectorAll('.pane').length === 2);
  assert.equal(fake.world.mode, 'wide');
});

test('Window options… in Jump to… opens below the ⋯ button of a pane with one, as in the "tabs" view', async (t) => {
  const { page, close } = await openBench({ world: tabsView() });
  t.after(close);
  await page.click('[data-action="open-palette"]');
  await page.keyboard.type('window options');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#window-menu');
  const m = await page.evaluate(() => window.bench.store.ui.menu);
  const shown = await page.getAttribute('#titlebar [data-action="menu"]', 'data-item');
  assert.deepEqual([m.itemId, m.shownIn, m.opener], [shown, 'compact', 'menu-compact']);
  const button = await page.locator('#titlebar [data-action="menu"]').boundingBox();
  const menu = await page.locator('#window-menu').boundingBox();
  assert.ok(Math.abs(menu.y - (button.y + button.height + 4)) <= 1, 'the menu is below the button');
  // Escape puts the focus on the button, as for a menu it opened.
  await page.keyboard.press('Escape');
  await page.waitForSelector('#window-menu', { state: 'detached' });
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'menu-compact');
});

test('Settings chooses the density, and hiding the sub-tab row over one app window', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await openSettings(page);
  const choices = () => page.$$eval('[data-choice="density"]', (els) => els.map((el) => [el.textContent, el.getAttribute('aria-pressed')]));
  assert.equal(await page.textContent('#density-label'), 'Density');
  assert.deepEqual(await choices(), [['Automatic', 'true'], ['Standard', 'false'], ['Compact', 'false']]);
  // On a wide screen, Automatic is the standard density the title bar
  // opened with, so nothing waits for Bench to open again.
  assert.equal(await page.textContent('#density-help'),
    'Compact while Bench uses the compact layout, as on a small screen, and standard otherwise.');
  await page.click('[data-key="density-compact"]');
  assert.deepEqual(await choices(), [['Automatic', 'false'], ['Standard', 'false'], ['Compact', 'true']]);
  assert.equal(await page.textContent('#density-help'),
    'Smaller tabs, sub-tabs, and buttons, with pane headers 32 points tall, to leave your apps more room on a laptop. The title bar changes height when Bench next opens.');

  // The tick box is off by default, and described by its help.
  const box = '[data-setting="paneHeaders"]';
  assert.equal(await page.isChecked(box), false);
  assert.equal(await page.textContent('.toggle:has(' + box + ') span'), 'Hide the sub-tab row when a pane holds one app window');
  assert.equal(await page.getAttribute(box, 'aria-describedby'), 'pane-headers-help');
  assert.match(await page.textContent('#pane-headers-help'), /^The app's own title bar takes the row's place\./);
  await page.click(box);
  await page.click('[data-action="save-settings"]');
  const save = await fake.waitForCall('SaveSettings');
  assert.equal(save.args[0].density, 'compact');
  assert.equal(save.args[0].paneHeaders, 'auto');
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
  await settle(page);
  assert.equal(await page.getAttribute('body', 'data-density'), 'compact');

  // Opened again, Settings shows them; unticking the box asks for headers
  // always.
  await openSettings(page);
  assert.equal(await page.isChecked(box), true);
  assert.equal(await page.getAttribute('[data-key="density-compact"]', 'aria-pressed'), 'true');
  await page.click(box);
  await page.click('[data-action="save-settings"]');
  await fake.waitForCall('SaveSettings', (a) => a[0].paneHeaders === 'always');
});

test('in the browser preview, the title bar follows the density at once, so Settings doesn\'t say it waits', async (t) => {
  const { page, close } = await openBench({ world: defaultWorld({ demo: true }) });
  t.after(close);
  await openSettings(page);
  assert.equal(await page.textContent('#density-help'),
    'Compact while Bench uses the compact layout, as on a small screen, and standard otherwise.');
  await page.click('[data-key="density-compact"]');
  assert.doesNotMatch(await page.textContent('#density-help'), /next opens/);
});

test('in the app, Settings says the title bar changes height only for a density that would change it', async (t) => {
  // A laptop's screen, where Bench opened in the compact density.
  const { page, close } = await openBench({ world: defaultWorld({ screenWidth: 1512, titleBar: 'compact' }) });
  t.after(close);
  await openSettings(page);
  const AUTO = 'Compact while Bench uses the compact layout, as on a small screen, and standard otherwise.';
  assert.equal(await page.textContent('#density-help'), AUTO);
  await page.click('[data-key="density-compact"]');
  assert.doesNotMatch(await page.textContent('#density-help'), /next opens/);
  await page.click('[data-key="density-standard"]');
  assert.equal(await page.textContent('#density-help'),
    'Roomy tabs, sub-tabs, and buttons, with pane headers 40 points tall. The title bar changes height when Bench next opens.');
});

test('in the compact density, scroll bars fit below the sub-tabs, in the panes and the title bar alike, and move nothing', async (t) => {
  const w = world({ density: 'compact' }, { current: 2 });
  // Brave's pane has more windows than fit.
  for (let i = 0; i < 8; i++) {
    const id = 'w' + (400 + i);
    w.items[id] = { id, kind: 'builtin', app: 'Terminal', mono: '>_', title: 'zsh ' + i + ' — ~/code/project-' + i, status: 'ready', folder: '~' };
    w.tabs[2].root.children[0].items.push(id);
  }
  const { page, fake, close } = await openBench({ world: w, viewport: { width: 900, height: 600 }, scrollBars: true });
  t.after(close);
  const measure = () => page.evaluate(() => {
    const top = (sel) => {
      const el = document.querySelector(sel);
      if (!el) return null;
      const r = el.getBoundingClientRect();
      return [Math.round(r.top * 10) / 10, Math.round(r.height * 10) / 10];
    };
    const bar = (sel) => {
      const el = document.querySelector(sel);
      return el ? el.offsetHeight - el.clientHeight : null;
    };
    const sub = document.querySelector('.subtabs .subtab[aria-pressed="true"]');
    const strip = sub.closest('.subtabs').getBoundingClientRect();
    const r = sub.getBoundingClientRect();
    return {
      title: top('#titlebar'),
      header: top('.pane-header'),
      subtab: top('.subtabs .subtab[aria-pressed="true"]'),
      panes: top('#panes'),
      bar: bar('.subtabs'),
      // The bar is below the sub-tab, inside the strip.
      clear: r.bottom <= strip.bottom - bar('.subtabs'),
    };
  });
  for (const [label, screenWidth] of [['panes', 2560], ['title bar', 1512]]) {
    fake.world.screenWidth = screenWidth;
    fake.world.settings.scrollBars = 'never';
    await redraw(page, fake);
    const before = await measure();
    assert.equal(before.bar, 0);
    fake.world.settings.scrollBars = 'always';
    await redraw(page, fake);
    const now = await measure();
    assert.deepEqual({ ...now, bar: null, clear: null }, { ...before, bar: null, clear: null }, label + ': nothing moved');
    assert.equal(now.bar, 3, label + ': a 3 pt bar');
    assert.equal(now.clear, true, label + ': clear of the sub-tabs');
    if (screenWidth === 1512) assert.equal(now.header, null, 'the "tabs" view has no pane header');
  }
});
