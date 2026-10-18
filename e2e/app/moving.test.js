'use strict';

// Moving windows: dragging sub-tabs, the split button, the ⋯ menu, the
// keyboard, popping out, and dragging dividers.

const assert = require('node:assert/strict');
const test = require('node:test');
const {
  itemID, itemsOf, nextState, openPage, paneOf, panesOf, setScreen, startPreview, stateCount, switchTo,
  tabIndex, view, waitForToast, waitView,
} = require('./helpers/preview.js');

// openCode starts a preview showing the Code tab on a simulated monitor.
async function openCode(t) {
  const preview = await startPreview({ t });
  const { page, close } = await openPage(preview);
  t.after(close);
  await setScreen(page, 'monitor');
  const v = await switchTo(page, 'Code');
  return { preview, page, v };
}

// box returns an element's bounding box, failing if it isn't shown.
async function box(page, selector) {
  const b = await page.locator(selector).boundingBox();
  assert.ok(b, selector + ' is on the page');
  return b;
}

test('dragging a sub-tab to another pane\'s right edge splits that pane', async (t) => {
  const { page, v } = await openCode(t);
  const vscode = itemID(v, 'index.html — bench-ui');
  const [, terminalPane] = panesOf(v, 'Code');
  const target = '.pane[data-pane-id="' + terminalPane.id + '"]';
  const b = await box(page, target);
  const n = await stateCount(page);
  await page.dragAndDrop('.subtab[data-item="' + vscode + '"]', target, {
    targetPosition: { x: b.width - 12, y: b.height / 2 },
  });
  const s = await nextState(page, n, (w) => w.display.panes.length === 3);
  const panes = s.display.panes;
  assert.deepEqual(panes.map((p) => p.items), [
    [itemID(v, 'bench — switch.go')],
    [itemID(v, 'zsh — ~/code')],
    [vscode],
  ]);
  // The new pane is at the right of the terminal's, and is focused.
  assert.ok(panes[2].rect.x > panes[1].rect.x);
  assert.equal(s.display.focus, panes[2].id);
  assert.equal(s.tabs[s.current].preset, 'custom');
  await page.waitForSelector('.pane[data-pane-id="' + panes[2].id + '"] .subtab[data-item="' + vscode + '"]');
});

test('dropping a sub-tab on a tab moves the window to that tab', async (t) => {
  const { page, v } = await openCode(t);
  const zed = itemID(v, 'bench — switch.go');
  const research = tabIndex(v, 'Research');
  await page.dragAndDrop('.subtab[data-item="' + zed + '"]', '.tab[data-index="' + research + '"]');
  const w = await waitView(page, (x, a) => x.items[a.zed].tab === a.research, { zed, research });
  // It joins Research's focused pane and is shown there, and Code stays
  // the current tab.
  assert.equal(w.current, tabIndex(v, 'Code'));
  const focused = panesOf(w, 'Research').find((p) => p.id === w.tabs[research].focus);
  assert.ok(focused.items.includes(zed));
  assert.equal(focused.active, zed);
  assert.deepEqual(itemsOf(w, 'Code')[0], [{ app: 'VS Code', title: 'index.html — bench-ui' }]);
});

test('the split button on a pane\'s only window shows Go\'s reason', async (t) => {
  const { page, v } = await openCode(t);
  const [, terminalPane] = panesOf(v, 'Code');
  await page.click('[data-action="split-right"][data-pane="' + terminalPane.id + '"]');
  await waitForToast(page,
    'This pane has only one window, so there\'s nothing to show beside it. Add another window first, or drag one here.');
  assert.equal((await view(page)).display.panes.length, 2);
});

test('the ⋯ menu moves a window to another tab', async (t) => {
  const { page, v } = await openCode(t);
  const [mainPane] = panesOf(v, 'Code');
  const zed = itemID(v, 'bench — switch.go');
  await page.click('[data-action="menu"][data-pane="' + mainPane.id + '"]');
  await page.locator('#window-menu .menu-item', { hasText: 'Research' }).click();
  const w = await waitView(page, (x, a) => x.items[a].tab === 2, zed);
  assert.equal(paneOf(w, zed), w.tabs[2].focus);
  assert.equal(await page.locator('#window-menu').count(), 0);
});

test('the ⋯ menu moves a window to another pane, named by its shown window', async (t) => {
  const preview = await startPreview({ t });
  const { page, close } = await openPage(preview);
  t.after(close);
  let v = await setScreen(page, 'monitor');
  const [slackPane] = panesOf(v, 'Comms');
  await page.click('[data-action="menu"][data-pane="' + slackPane.id + '"]');
  const entries = await page.locator('#window-menu').evaluate((m) => Array.from(m.children, (c) => c.textContent).filter(Boolean));
  assert.deepEqual(entries, [
    'Show side by side (split right)⌃⌥⇧→', 'Split down⌃⌥⇧↓',
    'Move to pane', 'With Outlook',
    'Move to tab', 'Code', 'Research',
    'Pop out of Bench',
  ]);
  await page.locator('#window-menu .menu-item', { hasText: 'With Outlook' }).click();
  v = await waitView(page, (x) => x.display.panes.length === 1);
  assert.deepEqual(itemsOf(v, 'Comms'), [[{ app: 'Outlook', title: 'Outlook' }, { app: 'Slack', title: 'Slack' }]]);
  assert.equal(v.display.panes[0].active, itemID(v, 'Slack'));
});

test('Pop out of Bench puts a window under Not in a tab, and the add dialog brings it back', async (t) => {
  const { page, v } = await openCode(t);
  const [mainPane, terminalPane] = panesOf(v, 'Code');
  const zed = itemID(v, 'bench — switch.go');
  await page.click('[data-action="menu"][data-pane="' + mainPane.id + '"]');
  await page.locator('#window-menu .menu-item', { hasText: 'Pop out of Bench' }).click();
  let w = await waitView(page, (x, a) => (x.floating || []).some((f) => f.id === a), zed);
  assert.equal(w.items[zed].tab, -1);
  assert.equal(paneOf(w, zed), '');
  assert.deepEqual(itemsOf(w, 'Code')[0], [{ app: 'VS Code', title: 'index.html — bench-ui' }]);

  // The palette lists it under Not in a tab.
  await page.click('[data-action="open-palette"]');
  await page.fill('#palette-query', 'switch.go');
  const rows = await page.locator('#palette-list').evaluate((list) => Array.from(list.children, (el) => el.textContent));
  assert.deepEqual(rows, ['Not in a tab', 'Zdbench — switch.goZed · add to this tab↵']);
  await page.keyboard.press('Escape');

  // The terminal pane's add dialog adds it back.
  await page.click('[data-action="add"][data-pane="' + terminalPane.id + '"]');
  await page.waitForSelector('#palette-list');
  const group = await page.locator('.palette-group').first().textContent();
  assert.equal(group, 'Windows not in a tab');
  await page.locator('.palette-item', { hasText: 'bench — switch.go' }).click();
  w = await waitView(page, (x, a) => x.items[a.zed] && x.items[a.zed].tab === 1 && !(x.floating || []).some((f) => f.id === a.zed), { zed });
  assert.equal(paneOf(w, zed), terminalPane.id);
  assert.equal(w.display.panes.find((p) => p.id === terminalPane.id).active, zed);
});

test('Control+Option+Shift+arrow moves the shown window to the next pane', async (t) => {
  const { page, v } = await openCode(t);
  const [mainPane, terminalPane] = panesOf(v, 'Code');
  const zed = itemID(v, 'bench — switch.go');
  // Focus the main pane by clicking its header.
  await page.click('.pane[data-pane-id="' + mainPane.id + '"] .pane-header', { position: { x: 400, y: 20 } });
  await waitView(page, (x, a) => x.display.focus === a, mainPane.id);
  await page.keyboard.press('Control+Alt+Shift+ArrowRight');
  const w = await waitView(page, (x, a) => x.display.panes.some((p) => p.id === a.pane && p.items.includes(a.zed)),
    { pane: terminalPane.id, zed });
  assert.equal(w.display.focus, terminalPane.id);
  assert.equal(w.display.panes.find((p) => p.id === terminalPane.id).active, zed);
  assert.deepEqual(itemsOf(w, 'Code')[0], [{ app: 'VS Code', title: 'index.html — bench-ui' }]);
});

test('dragging a divider changes the split\'s sizes', async (t) => {
  const { page, v } = await openCode(t);
  const root = v.tabs[v.current].root;
  assert.deepEqual(root.sizes, [65, 35]);
  const d = await box(page, '.divider[data-split="' + root.id + '"]');
  const x = d.x + d.width / 2;
  const y = d.y + d.height / 2;
  await page.mouse.move(x, y);
  await page.mouse.down();
  await page.mouse.move(x - 150, y, { steps: 6 });
  await page.mouse.move(x - 300, y, { steps: 6 });
  await page.mouse.up();
  const w = await waitView(page, (x2, a) => {
    const r = x2.tabs[x2.current].root;
    return r.id === a && r.sizes[0] < 50;
  }, root.id);
  const sizes = w.tabs[w.current].root.sizes;
  assert.ok(Math.abs(sizes[0] + sizes[1] - 100) < 0.01, 'sizes add up to 100: ' + sizes);
  // The panes follow: the main pane is narrower than before.
  assert.ok(w.display.panes[0].rect.w < v.display.panes[0].rect.w - 200);
  assert.equal(w.display.panes[1].rect.x, w.display.panes[0].rect.x + w.display.panes[0].rect.w + 8);
});
