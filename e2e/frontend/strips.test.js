'use strict';

// Strips of tabs and sub-tabs that don't fit: a vertical wheel scrolls
// them sideways, the current tab and the shown sub-tabs stay in view, the
// ends with more to show fade, sub-tabs keep a readable width, a drag near
// either end scrolls them, and the Show scroll bars setting.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, openSettings, settle } = require('./helpers/harness.js');
const { defaultWorld, find, manyTabs } = require('./helpers/fixture.js');

// crowded returns a world of n tabs whose current tab, the last, has one
// pane holding a Brave window and terminals more terminals.
function crowded(n = 12, more = 15, opts = {}) {
  const world = manyTabs(defaultWorld(opts), n);
  world.current = n - 1;
  const p = world.tabs[n - 1].root;
  for (let i = 0; i < more; i++) {
    const id = 'w' + (400 + i);
    world.items[id] = { id, kind: 'builtin', app: 'Terminal', mono: '>_', title: 'zsh ' + i + ' — ~/code/project-' + i, status: 'ready', folder: '~' };
    p.items.push(id);
  }
  return world;
}

// strip returns a strip's scroll position and size, and which of its ends
// fade (data-more).
function strip(page, selector) {
  return page.evaluate((sel) => {
    const el = document.querySelector(sel);
    return {
      left: el.scrollLeft,
      max: el.scrollWidth - el.clientWidth,
      more: el.dataset.more || '',
      mask: getComputedStyle(el).maskImage || getComputedStyle(el).webkitMaskImage,
    };
  }, selector);
}

// inView reports whether an element is inside its strip's visible part,
// clear of the fades.
function inView(page, selector) {
  return page.evaluate((sel) => {
    const el = document.querySelector(sel);
    const s = el.closest('.tabs, .subtabs').getBoundingClientRect();
    const r = el.getBoundingClientRect();
    return r.left >= s.left - 0.5 && r.right <= s.right + 0.5;
  }, selector);
}

// setScroll scrolls a strip to left, as a script does, which isn't the
// user's scrolling.
async function setScroll(page, selector, left) {
  await page.evaluate(([sel, x]) => { document.querySelector(sel).scrollLeft = x; }, [selector, left]);
  await settle(page, 60);
}

// wheelOver turns the wheel with the pointer over an element.
async function wheelOver(page, selector, dx, dy) {
  const b = await page.locator(selector).boundingBox();
  await page.mouse.move(b.x + 40, b.y + b.height / 2);
  await page.mouse.wheel(dx, dy);
  await settle(page, 60);
}

const TABS = '#titlebar .tabs';
const SUBTABS = '.pane[data-pane-id="p211"] .subtabs';

test('a vertical wheel scrolls the tabs and sub-tabs sideways, and a sideways one as before', async (t) => {
  const { page, fake, close } = await openBench({ world: crowded(), viewport: { width: 760, height: 600 } });
  t.after(close);
  await settle(page);
  await setScroll(page, TABS, 0);
  await wheelOver(page, TABS, 0, 120);
  assert.equal((await strip(page, TABS)).left, 120, 'the tab strip scrolled by the vertical wheel');
  await wheelOver(page, TABS, 0, -50);
  assert.equal((await strip(page, TABS)).left, 70, 'and back');
  await wheelOver(page, TABS, 60, 0);
  assert.equal((await strip(page, TABS)).left, 130, 'a sideways swipe scrolls it as before');

  await wheelOver(page, SUBTABS, 0, 150);
  assert.equal((await strip(page, SUBTABS)).left, 150, 'the sub-tab strip scrolled');
  await wheelOver(page, SUBTABS, 90, 10);
  assert.equal((await strip(page, SUBTABS)).left, 240, 'a mainly sideways swipe scrolls as it would');

  // A wheel that counts in lines moves 40 points a line.
  const lines = await page.evaluate((sel) => {
    const el = document.querySelector(sel);
    el.scrollLeft = 0;
    const e = new WheelEvent('wheel', { deltaY: 3, deltaMode: WheelEvent.DOM_DELTA_LINE, bubbles: true, cancelable: true });
    el.firstElementChild.dispatchEvent(e);
    return { left: el.scrollLeft, prevented: e.defaultPrevented };
  }, SUBTABS);
  assert.deepEqual(lines, { left: 120, prevented: true });

  // Redraws leave a strip that the user has scrolled where it is, until it
  // shows another sub-tab.
  await setScroll(page, SUBTABS, 0);
  await wheelOver(page, SUBTABS, 0, 600);
  const scrolled = (await strip(page, SUBTABS)).left;
  assert.ok(scrolled >= 600, 'scrolled to ' + scrolled);
  fake.world.items.w405.title = 'vim — ~/code/project-5';
  await fake.emitState();
  await settle(page);
  assert.equal((await strip(page, SUBTABS)).left, scrolled, 'a new title doesn\'t move the strip');
  assert.equal(await inView(page, '.subtab[aria-selected="true"]'), false);
  await page.click('.subtab[data-item="w405"]');
  await settle(page);
  assert.equal(fake.world.tabs[11].root.active, 'w405');
  assert.equal(await inView(page, '.subtab[data-item="w405"]'), true);
});

test('a wheel over a strip that fits does nothing, and hides a live preview', async (t) => {
  const { page, close } = await openBench({ world: defaultWorld({ demo: true, livePreviews: true }) });
  t.after(close);
  await page.hover('.subtab[data-item="w4"]');
  await page.waitForSelector('#preview:not([hidden])', { timeout: 1000 });
  const prevented = await page.evaluate(() => {
    const el = document.querySelector('.subtab[data-item="w4"]');
    const e = new WheelEvent('wheel', { deltaY: 100, bubbles: true, cancelable: true });
    el.dispatchEvent(e);
    return e.defaultPrevented;
  });
  assert.equal(prevented, false, 'a strip that fits leaves the wheel alone');
  await page.waitForSelector('#preview', { state: 'hidden' });
  assert.equal((await strip(page, '.pane[data-pane-id="p3"] .subtabs')).left, 0);
});

test('the current tab and the shown sub-tabs stay in view as the window narrows, and a new sub-tab shows', async (t) => {
  const world = crowded();
  find(world.tabs[11].root, 'p211').active = 'w407';
  const { page, fake, close } = await openBench({ world, viewport: { width: 1440, height: 800 } });
  t.after(close);
  await settle(page);
  assert.equal(await inView(page, '.tab[aria-selected="true"]'), true);
  assert.equal(await inView(page, '.subtab[data-item="w407"]'), true);

  // The user scrolls both strips away from them; narrowing the window
  // brings them back.
  await setScroll(page, SUBTABS, 0);
  await wheelOver(page, TABS, 0, -2000);
  await wheelOver(page, SUBTABS, 0, 2000);
  await page.setViewportSize({ width: 700, height: 800 });
  await page.waitForFunction(() => window.bench.store.view.display.area.w === 700);
  await settle(page);
  assert.equal(await inView(page, '.tab[aria-selected="true"]'), true, 'the current tab is in view');
  assert.equal(await inView(page, '.subtab[data-item="w407"]'), true, 'the shown sub-tab is in view');
  assert.ok((await strip(page, SUBTABS)).left > 0);

  // A new sub-tab at the end, shown, comes into view, even after the user
  // has scrolled away.
  await wheelOver(page, SUBTABS, 0, -2000);
  fake.world.items.w500 = { id: 'w500', kind: 'builtin', app: 'Terminal', mono: '>_', title: 'new — ~', status: 'ready', folder: '~' };
  const p = find(fake.world.tabs[11].root, 'p211');
  p.items.push('w500');
  p.active = 'w500';
  await fake.emitState();
  await settle(page);
  assert.equal(await inView(page, '.subtab[data-item="w500"]'), true);
  const s = await strip(page, SUBTABS);
  assert.ok(Math.abs(s.left - s.max) <= 1, 'scrolled to the end: ' + JSON.stringify(s));

  // Switching to a tab out of view brings it into view.
  await setScroll(page, TABS, 10000);
  await page.evaluate(() => document.querySelector('.tab[data-index="0"]').click());
  await settle(page);
  assert.equal(fake.world.current, 0);
  assert.equal(await inView(page, '.tab[data-index="0"]'), true);
});

test('a tab or sub-tab that has the keyboard stays in view as Bench draws again', async (t) => {
  const { page, fake, close } = await openBench({ world: crowded(), viewport: { width: 900, height: 700 } });
  t.after(close);
  await settle(page);
  assert.equal(await inView(page, '.tab[data-index="0"]'), false);
  assert.equal(await inView(page, '.subtab[data-item="w414"]'), false);

  // Tab moves the keyboard to a tab out of view, which scrolls it into
  // view; a new title elsewhere doesn't scroll it away again.
  await page.focus('.tab[data-index="0"]');
  assert.equal(await inView(page, '.tab[data-index="0"]'), true);
  fake.world.items.w405.title = 'vim — ~/code/project-5';
  await fake.emitState();
  await settle(page);
  assert.equal(await page.evaluate(() => document.activeElement.dataset.index), '0');
  assert.equal(await inView(page, '.tab[data-index="0"]'), true, 'the focused tab is still in view');

  // The same goes for a sub-tab.
  await page.focus('.subtab[data-item="w414"]');
  assert.equal(await inView(page, '.subtab[data-item="w414"]'), true);
  fake.world.items.w406.title = 'less — ~/code/project-6';
  await fake.emitState();
  await settle(page);
  assert.equal(await inView(page, '.subtab[data-item="w414"]'), true, 'the focused sub-tab is still in view');

  // Another current tab still comes into view.
  await page.focus('.tab[data-index="0"]');
  fake.world.current = 10;
  await fake.emitState();
  await settle(page);
  assert.equal(await inView(page, '.tab[data-index="10"]'), true, 'the new current tab is in view');
});

test('an end of a strip with more to show fades', async (t) => {
  const { page, close } = await openBench({ world: crowded(), viewport: { width: 760, height: 600 } });
  t.after(close);
  await settle(page);
  await setScroll(page, SUBTABS, 0);
  let s = await strip(page, SUBTABS);
  assert.equal(s.more, 'end');
  assert.match(s.mask, /gradient/);
  await setScroll(page, SUBTABS, 200);
  assert.equal((await strip(page, SUBTABS)).more, 'start end');
  await setScroll(page, SUBTABS, 100000);
  assert.equal((await strip(page, SUBTABS)).more, 'start');

  // The tab strip fades too, and a strip that fits doesn't.
  await setScroll(page, TABS, 0);
  assert.equal((await strip(page, TABS)).more, 'end');
  await page.setViewportSize({ width: 1800, height: 600 });
  await page.waitForFunction(() => window.bench.store.view.display.area.w === 1800);
  await settle(page);
  s = await strip(page, TABS);
  assert.equal(s.max, 0, 'the tabs fit');
  assert.equal(s.more, '');
  assert.equal(s.mask, 'none');
});

test('sub-tabs keep a readable width and scroll, and tabs cut long names short', async (t) => {
  const world = crowded(12, 15);
  world.tabs[8].name = 'A tab with a name much too long to show whole in the tab strip';
  world.items.w400.title = 'A very long window title that goes on and on, well past any sensible width';
  const { page, close } = await openBench({ world, viewport: { width: 760, height: 600 } });
  t.after(close);
  await settle(page);
  const sizes = await page.evaluate((sel) => {
    const el = document.querySelector(sel);
    const pane = el.closest('.pane').getBoundingClientRect();
    const right = (s) => el.closest('.pane').querySelector(s).getBoundingClientRect().right;
    return {
      widths: Array.from(el.children, (c) => c.getBoundingClientRect().width),
      scrolls: el.scrollWidth > el.clientWidth,
      pane: pane.right,
      add: right('.add-btn'),
      menu: right('[data-action="menu"]'),
    };
  }, SUBTABS);
  assert.equal(sizes.widths.length, 16);
  assert.ok(sizes.widths.every((w) => w >= 95.5), 'no sub-tab is narrower than 96: ' + sizes.widths);
  assert.ok(sizes.scrolls, 'the strip scrolls');
  assert.ok(sizes.menu <= sizes.pane && sizes.add < sizes.menu, 'the pane\'s buttons stay inside it');

  // A long tab name is cut short, keeping its shortcut, and its tooltip
  // has it whole.
  const tab = await page.evaluate(() => {
    const el = document.querySelector('.tab[data-index="8"]');
    const name = el.querySelector('.tab-name');
    const kbd = el.querySelector('kbd');
    const r = el.getBoundingClientRect();
    return {
      width: r.width,
      cut: name.scrollWidth > name.clientWidth,
      overflow: getComputedStyle(name).textOverflow,
      kbd: kbd ? kbd.getBoundingClientRect().right <= r.right : false,
      title: el.title,
    };
  });
  assert.ok(tab.width <= 200, 'tab width ' + tab.width);
  assert.ok(tab.cut && tab.overflow === 'ellipsis');
  assert.ok(tab.kbd, 'the shortcut stays in the tab');
  assert.equal(tab.title, 'A tab with a name much too long to show whole in the tab strip (⌃⌥9)');
});

test('a sub-tab\'s widest grows with its pane, the shown one\'s by the room for its ×, and its tooltip has its whole title', async (t) => {
  const world = defaultWorld();
  world.items.w3.title = 'A very long window title that goes on and on, well past any sensible width for a sub-tab';
  const { page, fake, close } = await openBench({ world, viewport: { width: 2400, height: 800 } });
  t.after(close);
  await settle(page);
  const width = () => page.evaluate(() => document.querySelector('.subtab[data-item="w3"]').getBoundingClientRect().width);
  // show shows w3, or the other window in its pane.
  const show = async (id) => {
    find(fake.world.tabs[1].root, 'p3').active = id;
    await fake.emitState();
    await page.waitForSelector('.subtab[data-item="' + id + '"][aria-selected="true"]');
    await settle(page);
  };
  assert.equal(Math.round(await width()), 382, 'a wide pane lets the shown sub-tab grow to 360, and 22 for its ×');
  assert.match(await page.getAttribute('.subtab[data-item="w3"]', 'title'), /^Zed — A very long window title .* for a sub-tab$/m);
  await show('w4');
  assert.equal(Math.round(await width()), 360, 'a wide pane lets the others grow to 360');
  await page.setViewportSize({ width: 900, height: 800 });
  await page.waitForFunction(() => window.bench.store.view.display.area.w === 900);
  await settle(page);
  assert.equal(Math.round(await width()), 220, 'a narrow pane keeps it to 220');
  await show('w3');
  assert.equal(Math.round(await width()), 242, 'a narrow pane keeps the shown one to 242');
  // Cut short, the shown sub-tab's title ends with an ellipsis, clear of
  // its ×.
  assert.deepEqual(await page.$eval('.subtab[data-item="w3"]', (el) => {
    const title = el.querySelector('.subtab-title');
    return [title.scrollWidth > title.clientWidth, getComputedStyle(title).textOverflow,
      title.getBoundingClientRect().right <= el.querySelector('.close-cross').getBoundingClientRect().left];
  }), [true, 'ellipsis', true]);
});

test('in a full strip, the shown sub-tab is as narrow as the others, with its × over the end of its title', async (t) => {
  const world = crowded(12, 15);
  find(world.tabs[11].root, 'p211').active = 'w403';
  const { page, fake, close } = await openBench({ world, viewport: { width: 760, height: 600 } });
  t.after(close);
  await settle(page);
  // shown describes the strip and its shown sub-tab: whether the strip is
  // full, the sub-tabs' widths, whether the sub-tab's slot for its × shows,
  // and whether the × is over the end of its title, which is cut off and
  // fades into it.
  const shown = () => page.$eval(SUBTABS, (strip) => {
    const el = strip.querySelector('.subtab[aria-selected="true"]');
    const title = el.querySelector('.subtab-title');
    const cross = el.querySelector('.close-cross');
    return {
      full: Boolean(strip.dataset.more),
      width: Math.round(el.getBoundingClientRect().width),
      widest: Math.max(...Array.from(strip.children, (c) => Math.round(c.getBoundingClientRect().width))),
      slot: getComputedStyle(el.querySelector('.cross-slot')).display !== 'none',
      over: title.getBoundingClientRect().right > cross.getBoundingClientRect().left,
      overflow: getComputedStyle(title).textOverflow,
      fade: getComputedStyle(cross, '::before').display !== 'none',
    };
  });
  assert.deepEqual(await shown(), { full: true, width: 96, widest: 96, slot: false, over: true, overflow: 'clip', fade: true });
  // Scrolled to either end, it's still full.
  await setScroll(page, SUBTABS, 100000);
  assert.deepEqual(await shown(), { full: true, width: 96, widest: 96, slot: false, over: true, overflow: 'clip', fade: true });

  // With fewer sub-tabs, the strip has room again, and so does the ×.
  const p = find(fake.world.tabs[11].root, 'p211');
  p.items = ['w211', 'w402', 'w403'];
  await fake.emitState();
  await page.waitForFunction((sel) => document.querySelector(sel).children.length === 3, SUBTABS);
  await settle(page);
  const { width, widest, ...rest } = await shown();
  assert.deepEqual(rest, { full: false, slot: true, over: false, overflow: 'ellipsis', fade: false });
  assert.ok(width > 96 && width === widest, 'the shown sub-tab is the widest: ' + width + ', ' + widest);
});

test('whether a strip of sub-tabs is full depends on its width, not on whether it was full before', async (t) => {
  // The "tabs" view's pane, in the title bar, with Slack shown: its host
  // shrinks with the sub-tabs, so the strip is a little narrower without
  // the shown sub-tab's slot for its ×, which a full strip takes away.
  const world = defaultWorld({ screenWidth: 1512, current: 0 });
  world.items.w1.host = 'a-very-long-workspace-name.enterprise.app.slack.com';
  const p1 = find(world.tabs[0].root, 'p1');
  for (let i = 0; i < 3; i++) {
    const id = 'w' + (600 + i);
    world.items[id] = { id, kind: 'builtin', app: 'Terminal', mono: '>_', title: 'zsh ' + i + ' — a fairly long title here', status: 'ready', folder: '~' };
    p1.items.push(id);
  }
  const { page, close } = await openBench({ world, viewport: { width: 1400, height: 800 } });
  t.after(close);
  await settle(page);
  // full resizes the window to width, and reports whether the strip is
  // full then.
  const full = async (width) => {
    await page.setViewportSize({ width, height: 800 });
    await settle(page, 30);
    return page.$eval('.title-pane .subtabs', (el) => 'more' in el.dataset);
  };
  // Narrow the window until the strip is full.
  let width = 1400;
  while (!(await full(width))) {
    width -= 8;
    assert.ok(width > 900, 'the strip is full by 900 points');
  }
  // Then, point by point, narrow it from a little wider and widen it back:
  // each width gives the same answer either way.
  const down = {};
  const up = {};
  for (let x = width + 16; x >= width - 8; x--) down[x] = await full(x);
  for (let x = width - 8; x <= width + 16; x++) up[x] = await full(x);
  // seen writes a scan as one character for each point, from the
  // narrowest: F for full, . for fits.
  const seen = (scan) => Object.keys(scan).map(Number).sort((a, b) => a - b).map((x) => (scan[x] ? 'F' : '.')).join('');
  assert.deepEqual(up, down, 'narrowing: ' + seen(down) + ', widening: ' + seen(up));
  assert.ok(!down[width + 16] && down[width - 8], 'the strip fits at the wider end and is full at the narrower one: from '
    + (width - 8) + ' to ' + (width + 16) + ' points, ' + seen(down));
});

test('a drag near the end of the tab strip scrolls it, to drop on a tab out of view', async (t) => {
  const world = manyTabs(defaultWorld(), 12);
  const { page, fake, close } = await openBench({ world, viewport: { width: 760, height: 600 } });
  t.after(close);
  await settle(page);
  await setScroll(page, TABS, 0);
  assert.equal(await inView(page, '.tab[data-index="11"]'), false);
  fake.clearCalls();
  const src = await page.locator('.subtab[data-item="w4"]').boundingBox();
  const tabs = await page.locator(TABS).boundingBox();
  await page.mouse.move(src.x + src.width / 2, src.y + src.height / 2);
  await page.mouse.down();
  await page.mouse.move(tabs.x + tabs.width / 2, tabs.y + tabs.height / 2, { steps: 4 });
  await page.waitForSelector('.tab.drop-target');
  // Held near the strip's right end, it scrolls to the end, and the tab
  // under the pointer is marked as it changes.
  const x = tabs.x + tabs.width - 6;
  await page.mouse.move(x, tabs.y + tabs.height / 2, { steps: 3 });
  await page.waitForFunction((sel) => {
    const el = document.querySelector(sel);
    return el.scrollLeft >= el.scrollWidth - el.clientWidth - 1;
  }, TABS, { timeout: 4000 });
  await settle(page);
  assert.equal(await page.getAttribute('.tab.drop-target', 'data-index'), '11', 'the last tab is under the pointer');
  const last = await page.locator('.tab[data-index="11"]').boundingBox();
  await page.mouse.move(last.x + last.width / 2, last.y + last.height / 2, { steps: 2 });
  await page.mouse.up();
  await fake.waitForCall('MoveItemToTab');
  assert.deepEqual(fake.callsTo('MoveItemToTab')[0].args, ['w4', 11]);

  // Near the start, it scrolls back.
  await setScroll(page, TABS, 10000);
  const src2 = await page.locator('.subtab[data-item="w3"]').boundingBox();
  await page.mouse.move(src2.x + src2.width / 2, src2.y + src2.height / 2);
  await page.mouse.down();
  await page.mouse.move(tabs.x + tabs.width / 2, tabs.y + tabs.height / 2, { steps: 4 });
  await page.mouse.move(tabs.x + 4, tabs.y + tabs.height / 2, { steps: 3 });
  await page.waitForFunction((sel) => document.querySelector(sel).scrollLeft === 0, TABS, { timeout: 4000 });
  await page.keyboard.press('Escape');
  await page.mouse.up();
});

test('Show scroll bars follows macOS by default, and Settings changes it', async (t) => {
  const { page, fake, close } = await openBench({ world: crowded(12, 15, { systemScrollBars: false }), viewport: { width: 1000, height: 700 } });
  t.after(close);
  await settle(page);
  const shown = () => page.evaluate(() => document.body.classList.contains('scroll-bars'));
  assert.equal(await shown(), false, 'macOS shows scroll bars only while scrolling');
  fake.world.systemScrollBars = true;
  await fake.emitState();
  await settle(page);
  assert.equal(await shown(), true, 'macOS shows them all the time');

  await openSettings(page, 'appearance');
  assert.equal(await page.getAttribute('[data-key="scroll-bars-auto"]', 'aria-checked'), 'true');
  assert.match(await page.textContent('#scroll-bars-help'), /all the time now/);
  await page.click('[data-key="scroll-bars-never"]');
  assert.match(await page.textContent('#scroll-bars-help'), /fade out at the edge/);
  await page.click('[data-key="settings-save"]');
  const save = await fake.waitForCall('SaveSettings');
  assert.equal(save.args[0].scrollBars, 'never');
  await page.waitForFunction(() => !document.body.classList.contains('scroll-bars'));

  fake.world.systemScrollBars = false;
  fake.world.settings.scrollBars = 'always';
  await fake.emitState();
  await settle(page);
  assert.equal(await shown(), true, 'always shows them whatever macOS does');
});

test('showing the scroll bars changes no heights, and draws them inside the strips', async (t) => {
  const { page, fake, close } = await openBench({ world: crowded(), viewport: { width: 760, height: 600 }, scrollBars: true });
  t.after(close);
  await settle(page);
  const measure = () => page.evaluate(() => {
    const top = (sel) => {
      const r = document.querySelector(sel).getBoundingClientRect();
      return [Math.round(r.top * 10) / 10, Math.round(r.height * 10) / 10];
    };
    const bar = (sel) => {
      const el = document.querySelector(sel);
      return el.offsetHeight - el.clientHeight;
    };
    return {
      title: top('#titlebar'),
      header: top('.pane-header'),
      tab: top('.tab[aria-selected="true"]'),
      subtab: top('.subtab[aria-selected="true"]'),
      panes: top('#panes'),
      bars: [bar('#titlebar .tabs'), bar('#panes .subtabs')],
    };
  });
  const before = await measure();
  assert.deepEqual(before.bars, [0, 0], 'hidden while macOS shows them only while scrolling');
  const areas = fake.callsTo('SetPaneArea').length;
  for (const choice of ['always', 'never', 'always']) {
    fake.world.settings.scrollBars = choice;
    await fake.emitState();
    await settle(page);
    const now = await measure();
    assert.deepEqual({ ...now, bars: null }, { ...before, bars: null }, choice + ': nothing moved');
    assert.deepEqual(now.bars, choice === 'always' ? [4, 4] : [0, 0], choice + ': the bars');
  }
  assert.equal(fake.callsTo('SetPaneArea').length, areas, 'the pane area stayed the same');
  // A strip that fits has no bar, and its tabs stay where they were.
  await page.evaluate(() => document.querySelector('.tab[data-index="0"]').click());
  await settle(page);
  const fits = await page.evaluate(() => {
    const el = document.querySelector('.pane[data-pane-id="p1"] .subtabs');
    return [el.offsetHeight - el.clientHeight, Math.round(el.querySelector('.subtab').getBoundingClientRect().top)];
  });
  assert.equal(fits[0], 0);
  assert.equal(fits[1], Math.round(before.subtab[0]));
  // In the app, pressing the tab strip's bar scrolls it, where the rest of
  // the title bar moves the window.
  const drags = await page.evaluate(() => {
    document.body.classList.add('is-app');
    const drag = (sel) => getComputedStyle(document.querySelector(sel)).getPropertyValue('--wails-draggable').trim();
    return [drag('#titlebar'), drag('#titlebar .tabs')];
  });
  assert.deepEqual(drags, ['drag', 'no-drag']);
});
