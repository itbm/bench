'use strict';

// Tabs and screens: switching tabs, showing sub-tabs, and the simulated
// laptop and monitor.

const assert = require('node:assert/strict');
const test = require('node:test');
const {
  chooseMode, itemID, openPage, openSettings, panesOf, setScreen, startPreview, switchTo, tabIndex, view, waitForToast, waitView,
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
  assert.equal(await page.getAttribute('#panes', 'aria-label'), 'Code tab');

  // Next and previous tab wrap around.
  await page.keyboard.press('Control+Alt+BracketRight');
  await waitView(page, (x) => x.current === 2);
  await page.keyboard.press('Control+Alt+BracketRight');
  v = await waitView(page, (x) => x.current === 0);
  assert.equal(v.tabs[v.current].name, 'Comms');
});

test('Control+Option+` goes back to the tab used before, which the jump palette lists first', async (t) => {
  const { page } = await open(t);
  let v = await switchTo(page, 'Research');
  await page.keyboard.press('Control+Alt+Digit2');
  v = await waitView(page, (x) => x.current === 1 && x.lastTab === 2);

  // Return with nothing typed goes back too.
  await page.keyboard.press('Control+Alt+Space');
  await page.waitForSelector('#palette-query:focus');
  assert.equal(await page.textContent('.palette-item[aria-selected="true"] .palette-label'), 'Research');
  await page.keyboard.press('Enter');
  v = await waitView(page, (x) => x.current === 2 && x.lastTab === 1);

  // The shortcut goes back and forth between the two.
  await page.keyboard.press('Control+Alt+Backquote');
  v = await waitView(page, (x) => x.current === 1 && x.lastTab === 2);
  await page.keyboard.press('Control+Alt+Backquote');
  v = await waitView(page, (x) => x.current === 2 && x.lastTab === 1);
  assert.equal(v.tabs[v.current].name, 'Research');
});

test('holding Control+Option after Control+Option+` shows the tabs, and letting go switches to the one chosen', async (t) => {
  const { page } = await open(t);
  await switchTo(page, 'Research');
  await switchTo(page, 'Code');
  // The hold shows the tabs, most recently used first, with the last used
  // tab chosen; ` again chooses the next one.
  await page.keyboard.down('Control');
  await page.keyboard.down('Alt');
  await page.keyboard.press('Backquote');
  await page.waitForSelector('.overview[data-hold]');
  let v = await view(page);
  assert.deepEqual(v.hold.tabs.map((i) => v.tabs[i].name), ['Code', 'Research', 'Comms']);
  assert.equal(v.tabs[v.hold.selected].name, 'Research');
  assert.equal(v.tabs[v.current].name, 'Code');
  await page.keyboard.press('Backquote');
  v = await waitView(page, (x) => x.hold && x.tabs[x.hold.selected].name === 'Comms');
  assert.equal(await page.textContent('.overview-card[aria-selected="true"] .overview-name'), 'Comms');
  // Shift with it, send_to_last_tab's ⌃⌥⇧`, moves the choice back, and
  // moves no window.
  await page.keyboard.down('Shift');
  await page.keyboard.press('Backquote');
  await page.keyboard.up('Shift');
  v = await waitView(page, (x) => x.hold && x.tabs[x.hold.selected].name === 'Research');
  await page.keyboard.press('Backquote');
  v = await waitView(page, (x) => x.hold && x.tabs[x.hold.selected].name === 'Comms');
  await page.keyboard.up('Alt');
  await page.keyboard.up('Control');
  v = await waitView(page, (x) => !x.hold && x.tabs[x.current].name === 'Comms');
  await page.waitForSelector('.overview', { state: 'detached' });
  assert.equal(v.tabs[v.lastTab].name, 'Code');

  // Escape ends a hold without switching.
  await page.keyboard.down('Control');
  await page.keyboard.down('Alt');
  await page.keyboard.press('Backquote');
  await page.waitForSelector('.overview[data-hold]');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.overview', { state: 'detached' });
  await waitView(page, (x) => !x.hold);
  await page.keyboard.up('Alt');
  await page.keyboard.up('Control');
  await new Promise((r) => setTimeout(r, 300));
  v = await view(page);
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

test('Laptop is wide by default and compact with an earlier compact_below, and Monitor makes it wide again', async (t) => {
  const { page } = await open(t);
  await setScreen(page, 'monitor');
  let v = await switchTo(page, 'Research');
  assert.equal(v.display.panes.length, 2);

  // The laptop's screen is wider than the default compact_below, 1,280
  // points, so Research shows its panes side by side.
  v = await setScreen(page, 'laptop');
  assert.equal(v.mode, 'auto');
  assert.equal(v.sizeClass, 'wide');
  assert.equal(v.display.panes.length, 2);
  // With 1,800 points, as an earlier version of Bench wrote in
  // config.toml, the laptop is compact.
  await openSettings(page);
  await page.fill('#compact-below', '1800');
  await page.press('#compact-below', 'Enter');
  v = await waitView(page, (x) => x.sizeClass === 'compact');
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
  assert.equal(v.mode, 'auto');
  assert.equal(v.screenWidth, 1512);
  // Research shows one pane with every window as a sub-tab.
  assert.equal(v.display.compact, true);
  assert.equal(v.display.panes.length, 1);
  const titles = v.display.panes[0].items.map((id) => v.items[id].title);
  assert.deepEqual(titles, ['Wails v3 docs', 'Pull request #42 · bench', 'Go packages', 'Daily note']);
  assert.equal(await page.locator('.pane').count(), 1);
  // Its sub-tabs are in the title bar, in place of its header, which it
  // doesn't have: it reaches up to the title bar.
  assert.equal(await page.locator('#titlebar .title-pane .subtab').count(), 4);
  assert.equal(await page.locator('.pane-header').count(), 0);
  assert.equal(v.display.panes[0].header, 0);
  assert.equal(v.display.panes[0].rect.y, v.display.area.y);
  // The split button stays on: it shows the window side by side in the
  // wide layout.
  const split = page.locator('[data-action="split-right"]');
  assert.equal(await split.getAttribute('aria-disabled'), null);
  assert.equal(await split.getAttribute('aria-label'), 'Show this window side by side, in the wide layout');
  assert.equal(await split.getAttribute('title'), 'Show this window side by side, in the wide layout (⌃⌥⇧→)');
  // The status bar names the layout in short, with why in its tooltip,
  // and says that the tab's panes are shown as one.
  assert.equal(await page.textContent('#statusbar .status-layout'), 'Auto: compact');
  assert.match(await page.getAttribute('#statusbar .status-layout', 'title'), /^Compact layout.*, chosen for a window 1,512 pt wide$/);
  assert.match(await page.textContent('#statusbar .status-count'), /^\d+ windows, shown as one pane$/);
  // The ⋯ menu has the same in place of the four splits, with no note.
  await page.click('[data-action="menu"]');
  const labels = await page.locator('#window-menu .menu-item').allTextContents();
  assert.equal(labels[0], 'Show side by side (wide layout)⌃⌥⇧→');
  assert.ok(!labels.some((l) => /^Split /.test(l)), labels.join(', '));
  assert.equal(await page.locator('#window-menu .menu-note').count(), 0);
  await page.keyboard.press('Escape');
  await page.waitForSelector('#window-menu', { state: 'detached' });

  // Clicking it changes to the wide layout, which shows Research's two
  // panes, with the shown window in its pane and focused. Research keeps
  // its panes, so the toast says to choose Auto to go back, and Undo goes
  // back to Auto too, which chooses the compact layout again.
  const shown = await split.getAttribute('data-item');
  await split.click();
  v = await waitView(page, (x) => x.mode === 'wide' && !x.display.compact);
  assert.equal(v.display.panes.length, 2);
  const own = v.display.panes.find((p) => p.id === v.display.focus);
  assert.equal(own.active, shown);
  assert.equal(await page.textContent('#statusbar .status-layout'), 'Wide');
  await waitForToast(page, /^Changed to the wide layout, to show .+ side by side\. To go back, choose Auto at the right of the title bar\.$/);
  await page.click('#toast .toast-action');
  v = await waitView(page, (x) => x.mode === 'auto' && x.display.compact);
  assert.equal(v.display.panes.length, 1);

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
  assert.equal(await page.getAttribute('[data-action="split-right"]', 'title'), 'Show this window side by side (⌃⌥⇧→)');
});

test('Wide and Compact override Auto until Auto is chosen again', async (t) => {
  const { page } = await open(t);
  await setScreen(page, 'monitor');
  await switchTo(page, 'Research');
  await chooseMode(page, 'compact');
  let v = await waitView(page, (x) => x.mode === 'compact' && x.display.compact);
  assert.equal(v.sizeClass, 'compact');
  // The compact layout slims the title bar: the layout control is one
  // button, which names the layout and opens a menu of the layouts.
  await page.waitForSelector('#titlebar[data-slim="true"] [data-action="layout-menu"]:has-text("Compact")');
  await chooseMode(page, 'auto');
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

  // An action, chosen with the arrow keys: the compact layout comes first.
  await page.keyboard.press('Control+Alt+Space');
  await page.keyboard.type('layout');
  const labels = await page.locator('.palette-item .palette-label').allTextContents();
  const target = labels.indexOf('Use the wide layout');
  assert.ok(target > 0, 'the palette offers the wide layout: ' + labels);
  for (let i = 0; i < target; i++) await page.keyboard.press('ArrowDown');
  assert.equal(await page.textContent('.palette-item[aria-selected="true"] .palette-label'), 'Use the wide layout');
  await page.keyboard.press('Enter');
  await waitView(page, (x) => x.mode === 'wide');

  // Escape closes it without doing anything.
  await page.keyboard.press('Control+Alt+Space');
  await page.waitForSelector('.palette');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.palette', { state: 'detached' });
});
