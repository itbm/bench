'use strict';

// Tabs and screens: switching tabs, showing sub-tabs, and the simulated
// laptop and monitor.

const assert = require('node:assert/strict');
const test = require('node:test');
const {
  itemID, openPage, panesOf, setScreen, startPreview, switchTo, tabIndex, view, waitForToast, waitView,
} = require('./helpers/preview.js');

async function open(t) {
  const preview = await startPreview({ t });
  const { page, close } = await openPage(preview);
  t.after(close);
  return { preview, page };
}

test('clicking a tab and Control+Option+number switch tabs', async (t) => {
  const { page } = await open(t);
  let v = await switchTo(page, 'Research');
  assert.equal(await page.getAttribute('.tab[data-index="2"]', 'aria-pressed'), 'true');
  assert.equal(await page.getAttribute('#panes', 'aria-label'), 'Research tab');

  await page.keyboard.press('Control+Alt+Digit2');
  v = await waitView(page, (x) => x.current === 1);
  assert.equal(await page.getAttribute('.tab[data-index="1"]', 'aria-pressed'), 'true');
  assert.match(await page.textContent('#statusbar .status-tab'), /Code/);

  // Next and previous tab wrap around.
  await page.keyboard.press('Control+Alt+BracketRight');
  await waitView(page, (x) => x.current === 2);
  await page.keyboard.press('Control+Alt+BracketRight');
  v = await waitView(page, (x) => x.current === 0);
  assert.equal(v.tabs[v.current].name, 'Comms');
});

test('clicking a sub-tab shows its window', async (t) => {
  const { page } = await open(t);
  await setScreen(page, 'monitor');
  const v = await switchTo(page, 'Research');
  const [bravePane] = panesOf(v, 'Research');
  const pr = itemID(v, 'Pull request #42 · bench');
  assert.notEqual(bravePane.active, pr);
  await page.click('.subtab[data-item="' + pr + '"]');
  const w = await waitView(page, (x, a) => x.display.panes.some((p) => p.id === a.pane && p.active === a.pr), { pane: bravePane.id, pr });
  assert.equal(w.display.focus, bravePane.id);
  assert.equal(await page.getAttribute('.subtab[data-item="' + pr + '"]', 'aria-pressed'), 'true');
  assert.equal(await page.getAttribute('.pane[data-pane-id="' + bravePane.id + '"]', 'aria-label'), 'Brave — Pull request #42 · bench');
});

test('Laptop makes Auto compact and Monitor makes it wide again', async (t) => {
  const { page } = await open(t);
  await setScreen(page, 'monitor');
  let v = await switchTo(page, 'Research');
  assert.equal(v.display.panes.length, 2);

  v = await setScreen(page, 'laptop');
  assert.equal(v.mode, 'auto');
  assert.equal(v.sizeClass, 'compact');
  assert.equal(v.screenWidth, 1512);
  // Research shows one pane with every window as a sub-tab.
  assert.equal(v.display.compact, true);
  assert.equal(v.display.panes.length, 1);
  const titles = v.display.panes[0].items.map((id) => v.items[id].title);
  assert.deepEqual(titles, ['Wails v3 docs', 'Pull request #42 · bench', 'Go packages', 'Daily note']);
  assert.equal(await page.locator('.pane').count(), 1);
  assert.equal(await page.locator('.pane .subtab').count(), 4);
  // Splits are disabled, with the reason shown.
  const split = page.locator('[data-action="split-right"]');
  assert.equal(await split.getAttribute('aria-disabled'), 'true');
  assert.equal(await split.getAttribute('title'), v.display.splitsDisabled);
  assert.match(await page.textContent('#statusbar'), /Compact layout.*1,512-pt screen/);
  await split.click({ force: true });
  await waitForToast(page, 'Splits are hidden on small screens.');
  await page.click('[data-action="menu"]');
  assert.equal(await page.locator('#window-menu .menu-item[disabled]').count(), 2);
  assert.equal(await page.textContent('#window-menu .menu-note'), v.display.splitsDisabled);
  await page.keyboard.press('Escape');

  // Code stacks its panes instead.
  v = await switchTo(page, 'Code');
  assert.equal(v.display.compact, false);
  assert.equal(v.display.panes.length, 2);
  assert.equal(v.display.panes[0].rect.x, v.display.panes[1].rect.x);
  assert.ok(v.display.panes[1].rect.y > v.display.panes[0].rect.y);

  v = await setScreen(page, 'monitor');
  assert.equal(v.sizeClass, 'wide');
  assert.ok(v.display.panes[1].rect.x > v.display.panes[0].rect.x);
  v = await switchTo(page, 'Research');
  assert.equal(v.display.compact, false);
  assert.equal(v.display.panes.length, 2);
  assert.equal(await page.locator('[data-action="split-right"][aria-disabled]').count(), 0);
});

test('Wide and Compact override Auto until Auto is chosen again', async (t) => {
  const { page } = await open(t);
  await setScreen(page, 'monitor');
  await switchTo(page, 'Research');
  await page.click('[data-action="mode"][data-mode="compact"]');
  let v = await waitView(page, (x) => x.mode === 'compact' && x.display.compact);
  assert.equal(v.sizeClass, 'compact');
  await page.click('[data-action="mode"][data-mode="auto"]');
  v = await waitView(page, (x) => x.mode === 'auto' && !x.display.compact);
  assert.equal(v.sizeClass, 'wide');
  assert.equal(tabIndex(await view(page), 'Research'), v.current);
});

test('the jump palette works by keyboard alone', async (t) => {
  const { page } = await open(t);
  await setScreen(page, 'monitor');
  // A tab.
  await page.keyboard.press('Control+Alt+Space');
  await page.waitForSelector('#palette-query:focus');
  await page.keyboard.type('research');
  await page.keyboard.press('Enter');
  let v = await waitView(page, (x) => x.current === 2);
  assert.equal(await page.locator('.palette').count(), 0);

  // A window in another tab: its tab, pane, and sub-tab.
  await page.keyboard.press('Control+Alt+Space');
  await page.keyboard.type('switch.go');
  await page.keyboard.press('Enter');
  const zed = itemID(v, 'bench — switch.go');
  v = await waitView(page, (x, a) => x.current === 1 && x.display.panes.some((p) => p.id === x.display.focus && p.active === a), zed);

  // A window that isn't in a tab joins the focused pane.
  await page.keyboard.press('Control+Alt+Space');
  await page.keyboard.type('downloads');
  await page.keyboard.press('Enter');
  v = await waitView(page, (x) => Object.values(x.items).some((it) => it.title === 'Downloads' && it.tab === 1));
  assert.deepEqual((v.floating || []).filter((f) => f.title === 'Downloads'), []);

  // An action, chosen with the arrow keys.
  await page.keyboard.press('Control+Alt+Space');
  await page.keyboard.type('layout');
  const labels = await page.locator('.palette-item .palette-label').allTextContents();
  const target = labels.indexOf('Use the compact layout');
  assert.ok(target > 0, 'the palette offers the compact layout: ' + labels);
  for (let i = 0; i < target; i++) await page.keyboard.press('ArrowDown');
  assert.equal(await page.textContent('.palette-item[aria-selected="true"] .palette-label'), 'Use the compact layout');
  await page.keyboard.press('Enter');
  await waitView(page, (x) => x.mode === 'compact');

  // Escape closes it without doing anything.
  await page.keyboard.press('Control+Alt+Space');
  await page.waitForSelector('.palette');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.palette', { state: 'detached' });
});
