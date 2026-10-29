'use strict';

// Moving and resizing dialogs: Tab properties, Settings, the add dialog,
// Jump to…, and the welcome guide move by their header and resize from
// their corner, stay inside the window, and remember their size. Moving
// one doesn't size it: with no size of its own, it's as tall as its
// contents, down to 8 points above the window's bottom edge.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, openSettings, openTabProperties, settle } = require('./helpers/harness.js');
const { defaultWorld } = require('./helpers/fixture.js');

const DIALOG = '#overlay-root .dialog';

// frameOf returns the open dialog's frame in the window, in whole points.
const frameOf = (page) => page.$eval(DIALOG, (el) => {
  const r = el.getBoundingClientRect();
  const o = document.getElementById('app-window').getBoundingClientRect();
  return { x: Math.round(r.left - o.left), y: Math.round(r.top - o.top), w: Math.round(r.width), h: Math.round(r.height) };
});

// pointOf returns the middle of the first element that sel matches, or a
// point inside it from its top-left with at.
const pointOf = (page, sel, at = null) => page.$eval(sel, (el, a) => {
  const r = el.getBoundingClientRect();
  return a ? { x: r.left + a.x, y: r.top + a.y } : { x: r.left + r.width / 2, y: r.top + r.height / 2 };
}, at);

// drag presses at a point, moves the pointer by (dx, dy) in steps, and
// lets go.
async function drag(page, from, dx, dy) {
  await page.mouse.move(from.x, from.y);
  await page.mouse.down();
  await page.mouse.move(from.x + dx / 2, from.y + dy / 2, { steps: 3 });
  await page.mouse.move(from.x + dx, from.y + dy, { steps: 3 });
  await page.mouse.up();
  await settle(page, 60);
}

// grip returns the middle of the open dialog's resize grip.
const grip = (page) => pointOf(page, DIALOG + ' .dialog-grip');

// reaching returns a frame at x and y, w wide, and as tall as a 900-point
// window lets a dialog with more contents than it shows be: down to 8
// points above its bottom edge.
const reaching = (x, y, w) => ({ x, y, w, h: 900 - 8 - y });

// inView reports whether the element that sel matches lies wholly inside
// the open dialog, and inside each part of it that scrolls or clips, where
// it can be seen without scrolling.
const inView = (page, sel) => page.$eval(sel, (el) => {
  const b = el.getBoundingClientRect();
  if (!(b.width > 0)) return false;
  for (let p = el.parentElement; p; p = p.parentElement) {
    if (getComputedStyle(p).overflow !== 'visible') {
      const r = p.getBoundingClientRect();
      if (b.top < r.top || b.bottom > r.bottom || b.left < r.left || b.right > r.right) return false;
    }
    if (p.classList.contains('dialog')) return true;
  }
  return false;
});

test('dragging a dialog\'s header moves it, and redraws keep it there', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await openSettings(page);
  const before = await frameOf(page);
  assert.deepEqual({ x: before.x, y: before.y, w: before.w }, { x: 440, y: 72, w: 560 }, 'Settings opens in the middle');
  // The header shows that it moves the dialog, and the grip that it
  // resizes it.
  const cursors = await page.evaluate(() => ({
    head: getComputedStyle(document.querySelector('.settings-head')).cursor,
    title: getComputedStyle(document.querySelector('#settings-title')).cursor,
    grip: getComputedStyle(document.querySelector('.dialog-grip')).cursor,
  }));
  assert.deepEqual(cursors, { head: 'move', title: 'move', grip: 'nwse-resize' });

  fake.clearCalls();
  // A press that wobbles less than 4 points is a click, which doesn't move
  // it.
  await drag(page, await pointOf(page, '#settings-title'), 3, 2);
  assert.deepEqual(await frameOf(page), before);
  assert.equal(await page.$eval(DIALOG, (el) => el.classList.contains('placed')), false);
  // Settings is as tall as the window lets it be, so there's room above it
  // but not below. Moved up, it has more room, and reaches down to the
  // window's edge.
  await drag(page, await pointOf(page, '#settings-title'), 160, -40);
  const moved = reaching(before.x + 160, before.y - 40, before.w);
  assert.deepEqual(await frameOf(page), moved);
  // The dialog is still open, Bench is still raised, and the focus is
  // where it was.
  assert.ok(await page.$('.settings-sheet'));
  assert.deepEqual(fake.callsTo('SetOverlay').map((c) => c.args[0]).filter((open) => !open), []);
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'setting-openAtLogin');
  // Moving it isn't resizing it: Go isn't told a size.
  assert.equal(fake.callsTo('SetDialogSize').length, 0);

  // A new View, and a change that redraws the dialog, keep it where it is.
  fake.world.notices = ['Couldn\'t register ⌃⌥Space'];
  await fake.emitState();
  await page.click('[data-key="setting-livePreviews"]');
  await settle(page);
  assert.deepEqual(await frameOf(page), moved);

  // Escape still closes it, and the focus goes back to the gear button.
  await page.keyboard.press('Escape');
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
  assert.equal(await page.evaluate(() => document.activeElement.dataset.action), 'open-settings');
  // It opens where the user left it, while Bench runs.
  await openSettings(page);
  assert.deepEqual(await frameOf(page), moved);
});

test('a field or a button in a dialog\'s header doesn\'t move it', async (t) => {
  const { page, close } = await openBench();
  t.after(close);
  await openTabProperties(page);
  const before = await frameOf(page);
  // Dragging across the tab's name selects some of it instead.
  await drag(page, await pointOf(page, '#tab-name', { x: 4, y: 17 }), 60, 0);
  assert.deepEqual(await frameOf(page), before);
  assert.equal(await page.evaluate(() => document.activeElement.id), 'tab-name');
  await drag(page, await pointOf(page, '[data-key="tab-place"]'), 60, 40);
  assert.deepEqual(await frameOf(page), before);
  // But the space around them moves it.
  await drag(page, await pointOf(page, '.editor-head', { x: 8, y: 8 }), -50, 30);
  assert.deepEqual(await frameOf(page), { ...before, x: before.x - 50, y: before.y + 30 });
  await page.keyboard.press('Escape');

  // Jump to… moves by its search row, but not from its search field or
  // its esc button, and by its row of hints at the foot. It lists more
  // than the window shows, so it reaches down to the window's edge.
  await page.click('[data-action="open-palette"]');
  await page.waitForSelector('.palette');
  const palette = await frameOf(page);
  await drag(page, await pointOf(page, '#palette-query'), 80, 80);
  assert.deepEqual(await frameOf(page), palette);
  await drag(page, await pointOf(page, '.palette-search .esc'), 80, 80);
  assert.ok(await page.$('.palette'), 'the esc button didn\'t close the palette, as the press didn\'t end on it');
  assert.deepEqual(await frameOf(page), palette);
  await drag(page, await pointOf(page, '.palette-search', { x: 6, y: 6 }), 80, -40);
  assert.deepEqual(await frameOf(page), reaching(palette.x + 80, palette.y - 40, palette.w));
  await drag(page, await pointOf(page, '.palette-foot', { x: 4, y: 4 }), -30, -20);
  assert.deepEqual(await frameOf(page), reaching(palette.x + 50, palette.y - 60, palette.w));
  // The search field still filters the rows.
  await page.fill('#palette-query', 'Settings');
  await page.waitForFunction(() => window.bench.store.ui.listItems.length === 1);
});

test('resizing a dialog from its corner changes its size, within limits, and Bench remembers it', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await openTabProperties(page);
  const before = await frameOf(page);
  const mini = () => page.$eval('.mini', (el) => Math.round(el.getBoundingClientRect().height));
  assert.equal(await mini(), 230);

  // Larger: the layout's preview takes the room.
  await drag(page, await grip(page), 120, 90);
  assert.deepEqual(await frameOf(page), { ...before, w: before.w + 120, h: before.h + 90 });
  assert.equal(await mini(), 230 + 90);
  await fake.waitForCall('SetDialogSize', (a) => a[0] === 'editor' && a[1] === before.w + 120 && a[2] === before.h + 90);
  await page.waitForFunction(() => window.bench.store.view.dialogs && window.bench.store.view.dialogs.editor);

  // No smaller than 420 by 300 points: the middle scrolls.
  await drag(page, await grip(page), -2000, -2000);
  assert.deepEqual(await frameOf(page), { ...before, w: 420, h: 300 });
  assert.ok(await page.$eval('.editor-sheet .dialog-body', (el) => el.scrollHeight > el.clientHeight), 'the middle scrolls');
  assert.ok(await page.$eval('[data-key="editor-save"]', (el) => {
    const b = el.getBoundingClientRect();
    const d = el.closest('.dialog').getBoundingClientRect();
    return b.bottom <= d.bottom && b.right <= d.right;
  }), 'Save is in view');

  // No larger than the window: the corner stops 8 points from its edges.
  await drag(page, await grip(page), 3000, 3000);
  assert.deepEqual(await frameOf(page), { ...before, w: 1440 - 8 - before.x, h: 900 - 8 - before.y });
  await fake.waitForCall('SetDialogSize', (a) => a[0] === 'editor' && a[1] === 1440 - 8 - before.x);

  // A click on the grip, without a move, changes nothing.
  fake.clearCalls();
  await drag(page, await grip(page), 0, 0);
  assert.equal(fake.callsTo('SetDialogSize').length, 0);
  assert.ok(await page.$('.editor-sheet'));
});

test('a dialog stays inside the window, clear of the traffic lights, and moves back in when the window shrinks', async (t) => {
  const { page, close } = await openBench();
  t.after(close);
  await openSettings(page);
  const inside = async (width, height) => {
    const f = await frameOf(page);
    assert.ok(f.x >= 8 && f.y >= 8 && f.x + f.w <= width - 8 && f.y + f.h <= height - 8, 'inside the window: ' + JSON.stringify(f));
    assert.ok(f.x >= 78 || f.y >= 52, 'clear of the traffic lights: ' + JSON.stringify(f));
    return f;
  };
  // Dragged past the top-left corner, it stops below the traffic lights,
  // the smaller move.
  await drag(page, await pointOf(page, '#settings-title'), -1200, -600);
  const corner = await inside(1440, 900);
  assert.deepEqual({ x: corner.x, y: corner.y }, { x: 8, y: 52 });
  // Dragged along the top, it stops beside them.
  await drag(page, await pointOf(page, '#settings-title'), 40, -200);
  assert.deepEqual(await frameOf(page), reaching(78, 8, corner.w));
  // Past the bottom-right corner, it stops at the edges, and is as short
  // as a dialog can be made, 300 points: its middle scrolls.
  await drag(page, await pointOf(page, '#settings-title'), 2000, 2000);
  const far = await inside(1440, 900);
  assert.deepEqual({ x: far.x + far.w, y: far.y + far.h, h: far.h }, { x: 1432, y: 892, h: 300 });

  // The window shrinks to Bench's smallest: the dialog moves back inside,
  // and is no taller than the window.
  await page.setViewportSize({ width: 900, height: 600 });
  await page.waitForFunction(() => {
    const r = document.querySelector('#overlay-root .dialog').getBoundingClientRect();
    return r.right <= 892 && r.bottom <= 592;
  });
  const shrunk = await inside(900, 600);
  assert.equal(shrunk.w, far.w);
  assert.ok(shrunk.h <= 600 - 16);
  // Its buttons are still in view.
  assert.ok(await page.$eval('[data-key="settings-save"]', (el) => el.getBoundingClientRect().bottom <= 592));

  // The window grows again: the dialog goes back where the user left it.
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.waitForFunction((x) => Math.round(document.querySelector('#overlay-root .dialog').getBoundingClientRect().left) === x, far.x);
  assert.deepEqual(await frameOf(page), far);
});

test('a dialog that a smaller window cuts short gets its size back when the window grows', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await openTabProperties(page);
  const before = await frameOf(page);
  await drag(page, await grip(page), 100, 100);
  const sized = { ...before, w: before.w + 100, h: before.h + 100 };
  assert.deepEqual(await frameOf(page), sized);
  await fake.waitForCall('SetDialogSize', (a) => a[0] === 'editor' && a[1] === sized.w && a[2] === sized.h);

  // At Bench's smallest, it's cut to fit the window, below the traffic
  // lights.
  await page.setViewportSize({ width: 900, height: 600 });
  await page.waitForFunction(() => document.querySelector('#overlay-root .dialog').getBoundingClientRect().bottom <= 592);
  assert.deepEqual(await frameOf(page), { x: 8, y: 52, w: 900 - 16, h: 600 - 8 - 52 });
  // Larger again: it's the size the user gave it, where they left it.
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.waitForFunction((w) => Math.round(document.querySelector('#overlay-root .dialog').getBoundingClientRect().width) === w, sized.w);
  assert.deepEqual(await frameOf(page), sized);
  // And so it opens next time.
  await page.keyboard.press('Escape');
  await page.waitForSelector('.editor-sheet', { state: 'detached' });
  await openTabProperties(page);
  assert.deepEqual(await frameOf(page), sized);
  assert.deepEqual(fake.world.dialogs, { editor: { w: sized.w, h: sized.h } });
});

test('moving a dialog that a smaller window cuts short keeps the size the user gave it', async (t) => {
  const { page, fake, close } = await openBench({
    world: defaultWorld({ dialogs: { settings: { w: 700, h: 800 } } }), viewport: { width: 900, height: 600 },
  });
  t.after(close);
  await openSettings(page);
  const small = await frameOf(page);
  assert.deepEqual({ w: small.w, h: small.h }, { w: 700, h: 600 - 16 });
  await drag(page, await pointOf(page, '#settings-title'), 10, 0);
  assert.deepEqual(await frameOf(page), { ...small, x: small.x + 10 });
  await page.keyboard.press('Escape');
  await page.waitForSelector('.settings-sheet', { state: 'detached' });

  // In a larger window, it opens where the user moved it, at its full
  // size.
  await page.setViewportSize({ width: 1440, height: 900 });
  await settle(page);
  await openSettings(page);
  assert.deepEqual(await frameOf(page), { x: small.x + 10, y: small.y, w: 700, h: 800 });
  assert.equal(fake.callsTo('SetDialogSize').length, 0);
});

test('a moved dialog is as tall as its contents, as they change', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  // Jump to…, filtered to nothing, is short. Moved, then cleared, it's as
  // tall as the window lets it be again.
  await page.click('[data-action="open-palette"]');
  await page.waitForSelector('.palette');
  const start = await frameOf(page);
  await page.fill('#palette-query', 'nothing matches this');
  await page.waitForFunction(() => window.bench.store.ui.listItems.length === 0);
  const empty = await frameOf(page);
  assert.ok(empty.h < 300, 'the palette is short: ' + JSON.stringify(empty));
  await drag(page, await pointOf(page, '.palette-foot', { x: 4, y: 4 }), 40, 30);
  assert.deepEqual(await frameOf(page), { ...empty, x: empty.x + 40, y: empty.y + 30 });
  await page.fill('#palette-query', '');
  await page.waitForFunction(() => window.bench.store.ui.listItems.length > 0);
  await settle(page);
  assert.deepEqual(await frameOf(page), reaching(start.x + 40, start.y + 30, start.w));
  await page.keyboard.press('Escape');
  await page.waitForSelector('.palette', { state: 'detached' });

  // The add dialog, moved in its Web page… step, is as tall as before back
  // in its list.
  await page.click('.pane[data-pane-id="p3"] [data-action="add"]');
  await page.waitForSelector('.palette');
  await page.fill('#palette-query', 'web page');
  await page.waitForFunction(() => window.bench.store.ui.listItems.length === 1);
  const list = await frameOf(page);
  await page.keyboard.press('Enter');
  await page.waitForSelector('#palette-url');
  const url = await frameOf(page);
  assert.notEqual(url.h, list.h);
  await drag(page, await pointOf(page, '.palette-foot', { x: 4, y: 4 }), -30, 20);
  assert.deepEqual(await frameOf(page), { ...url, x: url.x - 30, y: url.y + 20 });
  await page.keyboard.press('Escape');
  await page.waitForSelector('#palette-query');
  await settle(page);
  assert.deepEqual(await frameOf(page), { ...list, x: list.x - 30, y: list.y + 20 });
  // With the search cleared, it lists every app, as tall as the window
  // lets it be.
  await page.fill('#palette-query', '');
  await page.waitForFunction(() => window.bench.store.ui.listItems.length > 1);
  await settle(page);
  assert.deepEqual(await frameOf(page), reaching(list.x - 30, list.y + 20, list.w));
  // Moving isn't resizing: Go isn't told a size.
  assert.equal(fake.callsTo('SetDialogSize').length, 0);
});

test('the add dialog\'s steps keep their buttons in view at its smallest', async (t) => {
  const { page, close } = await openBench();
  t.after(close);
  await page.click('.pane[data-pane-id="p3"] [data-action="add"]');
  await page.waitForSelector('.palette');
  await drag(page, await grip(page), 0, -2000);
  assert.equal((await frameOf(page)).h, 200);
  await page.fill('#palette-query', 'web page');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#palette-url');
  assert.equal((await frameOf(page)).h, 200);
  assert.ok(await inView(page, '[data-key="palette-add"]'), 'Add is in view');
  assert.ok(await inView(page, '[data-key="palette-in-bench"]'), 'Show in Bench is in view');
  // The note above them scrolls instead.
  assert.ok(await page.$eval('#palette-list', (el) => el.scrollHeight > el.clientHeight), 'the note scrolls');

  // So do the steps for making a web app.
  await page.keyboard.type('squoosh.app');
  await page.keyboard.press('Enter');
  await page.waitForSelector('#palette-make-title');
  assert.equal((await frameOf(page)).h, 200);
  assert.ok(await inView(page, '[data-key="make-cancel"]'), 'Cancel is in view');
  assert.ok(await inView(page, '[data-key="make-in-bench"]'), 'Show in Bench instead is in view');
});

test('a double-click on a dialog\'s header puts it back in the middle, at its size', async (t) => {
  const { page, close } = await openBench();
  t.after(close);
  await page.click('[data-action="open-palette"]');
  await page.waitForSelector('.palette');
  const start = await frameOf(page);
  assert.deepEqual({ x: start.x, y: start.y, w: start.w }, { x: 400, y: 72, w: 640 });
  const foot = () => pointOf(page, '.palette-foot', { x: 4, y: 4 });

  // Moved, then double-clicked: back where it opened.
  await drag(page, await foot(), 300, -50);
  assert.notDeepEqual(await frameOf(page), start);
  const at = await foot();
  await page.mouse.dblclick(at.x, at.y);
  await settle(page, 60);
  assert.deepEqual(await frameOf(page), start);

  // Resized and moved, then double-clicked: in the middle, at the new
  // size.
  await drag(page, await grip(page), 160, -300);
  await drag(page, await foot(), -300, 150);
  const sized = await frameOf(page);
  assert.deepEqual(sized, { x: start.x - 300, y: start.y + 150, w: start.w + 160, h: start.h - 300 });
  const head = await pointOf(page, '.palette-search', { x: 6, y: 6 });
  await page.mouse.dblclick(head.x, head.y);
  await settle(page, 60);
  assert.deepEqual(await frameOf(page), { x: (1440 - sized.w) / 2, y: 72, w: sized.w, h: sized.h });
  // The search field keeps the keyboard.
  assert.equal(await page.evaluate(() => document.activeElement.id), 'palette-query');
});

test('a dialog opens at the size the user gave it, after a restart too, and its steps keep it', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await openSettings(page);
  const before = await frameOf(page);
  await drag(page, await grip(page), 100, -150);
  const sized = { w: before.w + 100, h: before.h - 150 };
  await fake.waitForCall('SetDialogSize', (a) => a[0] === 'settings' && a[1] === sized.w && a[2] === sized.h);
  await page.keyboard.press('Escape');
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
  // While Bench runs, it opens where it was, at its new size.
  await openSettings(page);
  assert.deepEqual(await frameOf(page), { ...sized, x: before.x, y: before.y });
  await page.keyboard.press('Escape');
  assert.deepEqual(fake.world.dialogs, { settings: sized });

  // Another dialog keeps its own size.
  await openTabProperties(page);
  assert.equal((await frameOf(page)).w, 900);
  await page.keyboard.press('Escape');

  // After a restart, Go's View has the sizes, and dialogs open in the
  // middle.
  const again = await openBench({ world: defaultWorld({ dialogs: { add: { w: 760, h: 420 }, settings: sized } }) });
  t.after(again.close);
  await openSettings(again.page);
  assert.deepEqual(await frameOf(again.page), { ...sized, x: (1440 - sized.w) / 2, y: 72 });
  await again.page.keyboard.press('Escape');
  // The add dialog keeps its size through its steps.
  await again.page.click('.pane [data-action="add"]');
  await again.page.waitForSelector('.palette');
  const add = { x: (1440 - 760) / 2, y: 72, w: 760, h: 420 };
  assert.deepEqual(await frameOf(again.page), add);
  await again.page.fill('#palette-query', 'Web page');
  await again.page.keyboard.press('Enter');
  await again.page.waitForSelector('#palette-url');
  assert.deepEqual(await frameOf(again.page), add);
  // A remembered size larger than the window is cut to fit it.
  await again.page.keyboard.press('Escape');
  await again.page.keyboard.press('Escape');
  await again.page.waitForSelector('.palette', { state: 'detached' });
  await again.page.setViewportSize({ width: 900, height: 600 });
  await settle(again.page);
  await openSettings(again.page);
  const small = await frameOf(again.page);
  assert.ok(small.h <= 600 - 16 && small.y + small.h <= 592, 'Settings fits the smaller window: ' + JSON.stringify(small));
});

test('the welcome guide moves and resizes too', async (t) => {
  const { page, close } = await openBench({ world: defaultWorld({ firstRun: true, accessibility: false }) });
  t.after(close);
  await page.waitForSelector('.guide-sheet');
  const before = await frameOf(page);
  await drag(page, await pointOf(page, '#guide-title'), -100, 50);
  await drag(page, await grip(page), 80, 60);
  assert.deepEqual(await frameOf(page), { x: before.x - 100, y: before.y + 50, w: before.w + 80, h: before.h + 60 });
  // Its buttons still work.
  await page.click('[data-action="dismiss-first-run"]');
  await page.waitForSelector('.guide-sheet', { state: 'detached' });
});
