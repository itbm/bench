'use strict';

// Tiles: an app's or a page's icon in place of its monogram, in sub-tabs,
// Jump to…, and the add dialog; the monogram where there's none, or it
// doesn't load; and a page's new icon as Go sends it.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, settle } = require('./helpers/harness.js');
const { defaultWorld } = require('./helpers/fixture.js');

// Two icons, red and blue, as Go would serve them, and one that doesn't
// load. The app serves icons from its own /icons/ addresses; data:
// addresses stand in for them here, as the tests' server has none.
const RED = 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAQAAAAECAYAAACp8Z5+AAAAEklEQVR4nGN4ZmPzHxkzkC4AAAPXJdHEY5f+AAAAAElFTkSuQmCC';
const BLUE = 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAQAAAAECAYAAACp8Z5+AAAAEklEQVR4nGOwiXr2HxkzkC4AAC9HJ7EV7nQMAAAAAElFTkSuQmCC';
const BROKEN = 'data:image/png;base64,AAAA';

// tileOf describes the tile in the element that a locator, or a selector,
// finds: whether it shows an icon, and which, the monogram's text, and
// whether that's shown, and the tile's size.
function tileOf(page, where) {
  return locate(page, where).evaluate((el) => {
    const t = el.querySelector('.mono-tile');
    const img = t.querySelector('img.tile-icon');
    const mono = t.querySelector('.tile-mono');
    const r = t.getBoundingClientRect();
    return {
      icon: Boolean(img) && t.classList.contains('has-icon') && getComputedStyle(img).opacity === '1' && img.naturalWidth > 0,
      src: img ? img.getAttribute('src') : '',
      mono: mono.textContent,
      monoShown: getComputedStyle(mono).visibility !== 'hidden',
      size: [Math.round(r.width), Math.round(r.height)],
    };
  });
}

// iconShown waits until the tile in the element that a locator, or a
// selector, finds shows its icon.
function iconShown(page, where) {
  return locate(page, where).locator('.mono-tile.has-icon').waitFor();
}

function locate(page, where) {
  return typeof where === 'string' ? page.locator(where).first() : where;
}

// row finds the row of Jump to… or the add dialog with a label.
function row(page, label) {
  return page.locator('.palette-item', { has: page.locator('.palette-label', { hasText: new RegExp('^' + label.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') + '$') }) });
}

test('a sub-tab shows its app\'s icon in place of the monogram, the same size, and keeps the monogram where there\'s none', async (t) => {
  const world = defaultWorld();
  world.items.w3.icon = RED;
  world.items.w4.icon = BROKEN;
  const { page, close } = await openBench({ world });
  t.after(close);
  const zed = '.subtab[data-item="w3"]';
  const code = '.subtab[data-item="w4"]';
  await iconShown(page, zed);
  await page.waitForFunction((sel) => !document.querySelector(sel + ' img.tile-icon'), code);
  assert.deepEqual(await tileOf(page, zed), { icon: true, src: RED, mono: 'Zd', monoShown: false, size: [20, 20] });
  // An icon that doesn't load leaves the monogram, as a terminal has.
  assert.deepEqual(await tileOf(page, code), { icon: false, src: '', mono: 'VS', monoShown: true, size: [20, 20] });
  assert.deepEqual(await tileOf(page, '.subtab[data-item="w5"]'), { icon: false, src: '', mono: '>_', monoShown: true, size: [20, 20] });
  // A monogram is 10 pt, the smallest text that Apple's guidelines allow
  // on the Mac.
  assert.equal(await page.$eval(code + ' .tile-mono', (el) => getComputedStyle(el).fontSize), '10px');
  // The icon makes the sub-tab no taller.
  const heights = await page.$$eval('.pane[data-pane-id="p3"] .subtab, .pane[data-pane-id="p4"] .subtab',
    (els) => els.map((el) => Math.round(el.getBoundingClientRect().height)));
  assert.equal(new Set(heights).size, 1, 'sub-tab heights ' + heights);
  // The monogram is still there for a screen reader to skip, as before:
  // the tile is hidden from it, and the sub-tab's label names the window.
  assert.equal(await page.getAttribute(zed + ' .mono-tile', 'aria-hidden'), 'true');
  assert.equal(await page.getAttribute(zed + ' img.tile-icon', 'alt'), '');
});

test('a tile drawn again shows its icon at once, without the monogram first', async (t) => {
  const world = defaultWorld();
  world.items.w3.icon = RED;
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await iconShown(page, '.subtab[data-item="w3"]');
  fake.world.items.w3.title = 'bench — layout.go';
  const view = fake.view();
  // Checked as the new sub-tab goes in, before any load event can come.
  const hasIcon = await page.evaluate((v) => new Promise((resolve) => {
    const before = document.querySelector('.subtab[data-item="w3"]');
    const mo = new MutationObserver(() => {
      const el = document.querySelector('.subtab[data-item="w3"]');
      if (el && el !== before && el.textContent.includes('layout.go')) {
        mo.disconnect();
        resolve(el.querySelector('.mono-tile').classList.contains('has-icon'));
      }
    });
    mo.observe(document.body, { subtree: true, childList: true });
    window.__benchEmit('bench:state', v);
  }), view);
  assert.equal(hasIcon, true);
});

test('a page\'s sub-tab shows its icon once Go has one, a new one when it changes, and the monogram again without', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ current: 0 }) });
  t.after(close);
  const slack = '.subtab[data-item="w1"]';
  assert.deepEqual(await tileOf(page, slack), { icon: false, src: '', mono: 'Sl', monoShown: true, size: [20, 20] });
  fake.world.items.w1.icon = RED;
  await fake.emitState();
  await iconShown(page, slack);
  assert.equal((await tileOf(page, slack)).src, RED);
  fake.world.items.w1.icon = BLUE;
  await fake.emitState();
  await page.waitForFunction((sel) => {
    const img = document.querySelector(sel + ' img.tile-icon');
    return Boolean(img && img.getAttribute('src').endsWith('MAAAAAElFTkSuQmCC') && img.parentElement.classList.contains('has-icon'));
  }, slack);
  delete fake.world.items.w1.icon;
  await fake.emitState();
  await page.waitForFunction((sel) => !document.querySelector(sel + ' img.tile-icon'), slack);
  assert.deepEqual(await tileOf(page, slack), { icon: false, src: '', mono: 'Sl', monoShown: true, size: [20, 20] });
});

test('Jump to…, the add dialog, and an empty pane\'s list show icons too', async (t) => {
  const world = defaultWorld();
  world.items.w3.icon = RED;
  world.floating[1].icon = BLUE;
  world.items.w10.hidden = true;
  world.remembered = {
    p6: [{ itemId: 'w10', bundleId: 'md.obsidian', app: 'Obsidian', mono: 'Ob', icon: RED, text: 'Obsidian isn\'t running', canOpen: true }],
  };
  const { page, close } = await openBench({ world });
  t.after(close);

  await page.click('[data-action="open-palette"]');
  await page.waitForSelector('#palette-query');
  await page.fill('#palette-query', 'switch.go');
  const zedRow = row(page, 'bench — switch.go');
  await iconShown(page, zedRow);
  assert.equal((await tileOf(page, zedRow)).src, RED);
  await page.fill('#palette-query', 'downloads');
  const finderRow = row(page, 'Downloads');
  await iconShown(page, finderRow);
  assert.equal((await tileOf(page, finderRow)).src, BLUE);
  await page.keyboard.press('Escape');
  await settle(page);

  await page.click('.pane[data-pane-id="p4"] [data-action="add"]');
  await page.waitForSelector('#palette-query');
  const newZed = row(page, 'New Zed window');
  await iconShown(page, newZed);
  const newTerminal = await tileOf(page, row(page, 'New terminal'));
  assert.deepEqual([newTerminal.icon, newTerminal.mono, newTerminal.monoShown], [false, '>_', true]);
  await page.keyboard.press('Escape');
  await settle(page);

  await page.click('[data-action="tab"][data-index="2"]');
  const waiting = '.pane[data-pane-id="p6"] .remembered-app';
  await iconShown(page, waiting);
  assert.equal((await tileOf(page, waiting)).src, RED);
});
