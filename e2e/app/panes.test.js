'use strict';

// Arranging a tab's panes with the real engine: Zoom pane and Show all
// panes, a double-click on a divider, Split right with an empty pane,
// whose pane goes again if the add dialog closes without adding anything,
// and the keys that arrange the panes: Grow pane, Shrink pane, Make panes
// equal, and Next and Previous layout.

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const {
  chooseMode, openPage, panesOf, setScreen, startPreview, switchTo, waitForToast, waitView,
} = require('./helpers/preview.js');

// openCode starts a preview showing the Code tab on a simulated monitor.
async function openCode(t) {
  const preview = await startPreview({ t });
  const { page, close } = await openPage(preview);
  t.after(close);
  await setScreen(page, 'monitor');
  const v = await switchTo(page, 'Code');
  return { page, v };
}

test('Zoom pane shows one pane until its chip is clicked, or the tab changes', async (t) => {
  const { page, v } = await openCode(t);
  const [, terminal] = panesOf(v, 'Code');
  await page.click('[data-action="menu"][data-pane="' + terminal.id + '"]');
  await page.click('#window-menu .menu-item:has-text("Zoom pane")');
  let w = await waitView(page, (x, id) => x.display.zoomed === id, terminal.id);
  assert.deepEqual(w.display.panes.map((p) => p.id), [terminal.id]);
  assert.equal(w.display.focus, terminal.id);
  assert.equal(w.display.dividers.length, 0);
  // The tab keeps its panes.
  assert.equal(panesOf(w, 'Code').length, 2);
  await page.waitForSelector('.pane[data-pane-id="' + terminal.id + '"] .zoom-chip');
  assert.equal(await page.textContent('#statusbar .status-zoom'), 'One pane zoomed');

  await page.click('.zoom-chip');
  w = await waitView(page, (x) => !x.display.zoomed && x.display.panes.length === 2);
  assert.equal(w.display.focus, terminal.id);

  // From the jump palette; then another tab and back shows every pane.
  await page.click('[data-action="open-palette"]');
  await page.fill('#palette-query', 'zoom this pane');
  await page.keyboard.press('Enter');
  await waitView(page, (x) => Boolean(x.display.zoomed));
  await switchTo(page, 'Research');
  await switchTo(page, 'Code');
  await waitView(page, (x) => !x.display.zoomed && x.display.panes.length === 2);
  assert.equal(await page.locator('.zoom-chip').count(), 0);
});

test('a double-click on a divider gives the panes equal sizes, and Undo puts them back', async (t) => {
  const { page, v } = await openCode(t);
  const root = v.tabs[v.current].root;
  assert.deepEqual(root.sizes.map(Math.round), [65, 35]);
  await page.dblclick('.divider[data-split="' + root.id + '"]');
  await waitForToast(page, 'Made the panes the same size.');
  await waitView(page, (x) => x.tabs[x.current].root.sizes.map(Math.round).join() === '50,50');
  await page.click('#toast .toast-action');
  await waitForToast(page, 'Put back the sizes of the panes.');
  await waitView(page, (x) => x.tabs[x.current].root.sizes.map(Math.round).join() === '65,35');
});

test('Split right with an empty pane opens the add dialog for it, and the pane goes if nothing is added', async (t) => {
  const { page, v } = await openCode(t);
  const [editors] = panesOf(v, 'Code');
  const sizes = v.tabs[v.current].root.sizes;
  const split = async () => {
    await page.click('[data-action="menu"][data-pane="' + editors.id + '"]');
    await page.click('#window-menu .menu-item:has-text("Split right with an empty pane")');
    const w = await waitView(page, (x) => x.display.panes.length === 3);
    await page.waitForSelector('.palette');
    return w;
  };
  let w = await split();
  const fresh = w.display.focus;
  assert.deepEqual(w.display.panes.map((p) => p.id).slice(0, 2), [editors.id, fresh]);
  assert.deepEqual(w.display.panes[1].items, []);
  assert.equal(await page.evaluate(() => window.bench.store.ui.add.paneId), fresh);

  // Cancelled: the tab is as it was.
  await page.keyboard.press('Escape');
  w = await waitView(page, (x) => x.display.panes.length === 2);
  assert.deepEqual(w.tabs[w.current].root.sizes, sizes);
  assert.equal(w.display.focus, v.display.focus);

  // A terminal added: the pane stays.
  w = await split();
  const kept = w.display.focus;
  await page.fill('#palette-query', 'New terminal');
  await page.keyboard.press('Enter');
  await waitView(page, (x, id) => (x.display.panes.find((p) => p.id === id) || { items: [] }).items.length === 1, kept);
  // Closing the dialog asked for the pane to go, after the terminal was
  // added: it stays.
  await page.waitForTimeout(500);
  w = await waitView(page, (x) => x.display.panes.length === 3);
  assert.ok(w.display.panes.some((p) => p.id === kept));
});

// jumpTo runs the jump palette's first row for query.
async function jumpTo(page, query) {
  await page.click('[data-action="open-palette"]');
  await page.fill('#palette-query', query);
  await page.keyboard.press('Enter');
  await page.waitForSelector('.palette', { state: 'detached' });
}

// paletteLabels returns the jump palette's rows for query, and closes it.
async function paletteLabels(page, query) {
  await page.click('[data-action="open-palette"]');
  await page.fill('#palette-query', query);
  const labels = await page.$$eval('.palette-item .palette-label', (els) => els.map((el) => el.textContent));
  await page.keyboard.press('Escape');
  await page.waitForSelector('.palette', { state: 'detached' });
  return labels;
}

test('the keys that arrange the panes say what they did, with Undo, from the jump palette and their shortcuts', async (t) => {
  const preview = await startPreview({ t });
  const { page, close } = await openPage(preview);
  t.after(close);
  // The shortcuts are off until the user sets them.
  const file = path.join(preview.home, 'config.toml');
  const text = fs.readFileSync(file, 'utf8');
  assert.match(text, /^grow_pane = ""/m);
  fs.writeFileSync(file, text.replace(/^grow_pane = ""/m, 'grow_pane = "ctrl+alt+="').replace(/^next_layout = ""/m, 'next_layout = "ctrl+alt+l"'));
  await jumpTo(page, 'Reload config');
  await waitForToast(page, 'Reloaded config.toml');
  await waitView(page, (x) => x.keys.growPane === '⌃⌥=' && x.keys.nextLayout === '⌃⌥L');
  await setScreen(page, 'monitor');
  let v = await switchTo(page, 'Code');
  const [editors] = panesOf(v, 'Code');
  await page.click('.pane[data-pane-id="' + editors.id + '"] .pane-header', { position: { x: 300, y: 10 } });
  await waitView(page, (x, id) => x.display.focus === id, editors.id);

  // Grow this pane, then its shortcut: one run, which one Undo takes back.
  await jumpTo(page, 'grow this pane');
  const grown = await waitForToast(page, /^\S.* 70% · \S.* 30%$/);
  await waitView(page, (x) => x.tabs[x.current].root.sizes.map(Math.round).join() === '70,30');
  await page.keyboard.press('Control+Alt+Equal');
  await waitForToast(page, grown.replace('70%', '75%').replace('30%', '25%'));
  await page.click('#toast .toast-action');
  await waitForToast(page, 'Put back the sizes of the panes.');
  await waitView(page, (x) => x.tabs[x.current].root.sizes.map(Math.round).join() === '65,35');

  // Next layout, by its shortcut, then Previous layout.
  await page.keyboard.press('Control+Alt+KeyL');
  await waitForToast(page, 'Layout: Three columns');
  v = await waitView(page, (x) => x.tabs[x.current].preset === 'three' && x.display.panes.length === 3);
  assert.deepEqual(v.display.panes.map((p) => p.items.length > 0), [true, true, false]);
  assert.ok((await paletteLabels(page, 'layout')).includes('Next layout'));
  await jumpTo(page, 'previous layout');
  await waitForToast(page, 'Layout: Main and side');
  await jumpTo(page, 'make panes equal');
  await waitForToast(page, 'Made the panes the same size.');
  await waitView(page, (x) => x.tabs[x.current].root.sizes.map(Math.round).join() === '50,50');
  await page.click('#toast .toast-action');
  await waitForToast(page, 'Put back the layout.');
  v = await waitView(page, (x) => x.tabs[x.current].preset === 'main-side' && x.tabs[x.current].root.sizes.map(Math.round).join() === '65,35');
  assert.equal(v.display.panes.length, 2);

  // The "tabs" view shows one pane: the palette has no rows for them, and
  // the shortcuts say why.
  await switchTo(page, 'Comms');
  await chooseMode(page, 'compact');
  await waitView(page, (x) => x.display.compact);
  const rows = await paletteLabels(page, 'pane');
  assert.equal(rows.some((r) => /^(Grow|Shrink) this pane|^Make panes equal/.test(r)), false, rows.join(', '));
  assert.equal((await paletteLabels(page, 'layout')).some((r) => /^(Next|Previous) layout/.test(r)), false);
  await page.keyboard.press('Control+Alt+Equal');
  await waitForToast(page, 'In the compact layout, this tab shows its windows as one pane. Choose Wide to arrange its panes.');
});
