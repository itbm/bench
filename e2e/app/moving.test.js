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

test('dragging a sub-tab along its strip puts it there, without splitting the pane', async (t) => {
  const { page, v } = await openCode(t);
  const zed = itemID(v, 'bench — switch.go');
  const vscode = itemID(v, 'index.html — bench-ui');
  const [editors] = panesOf(v, 'Code');
  assert.deepEqual(editors.items, [zed, vscode]);
  // The top of the header, before Zed: where the pane's top edge used to
  // split it.
  const header = await box(page, '.pane[data-pane-id="' + editors.id + '"] .pane-header');
  const first = await box(page, '.subtab[data-item="' + zed + '"]');
  const src = await box(page, '.subtab[data-item="' + vscode + '"]');
  await page.mouse.move(src.x + src.width / 2, src.y + src.height / 2);
  await page.mouse.down();
  await page.mouse.move(first.x + 4, header.y + 2, { steps: 8 });
  await page.waitForSelector('.drop-caret:not([hidden])');
  assert.equal(await page.locator('.drop-indicator:not([hidden])').count(), 0);
  await page.mouse.up();
  const w = await waitView(page, (x, a) => x.tabs[x.current].root.children[0].items[0] === a, vscode);
  assert.deepEqual(panesOf(w, 'Code').map((p) => p.items), [[vscode, zed], panesOf(v, 'Code')[1].items]);
  // Its layout is still the preset's: only the order in the pane changed.
  assert.equal(w.tabs[w.current].preset, v.tabs[v.current].preset);
  await waitForToast(page, 'Moved VS Code — index.html — bench-ui.');
  assert.deepEqual(await page.$$eval('.pane[data-pane-id="' + editors.id + '"] .subtab', (els) => els.map((el) => el.dataset.item)), [vscode, zed]);
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
  // The Move to Tab submenu lists the tabs. Its entry shows the
  // send_to_tab_modifiers shortcut, which does the same.
  await page.getByRole('menuitem', { name: 'Move to Tab' }).click();
  const entry = page.locator('#menu-sub').getByRole('menuitem', { name: /^Research/ });
  assert.equal(await entry.locator('kbd').textContent(), '⌃⌥⇧3');
  await entry.click();
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
  // The demo's Brave web app of Slack joins the menu once Go has found it.
  await page.waitForSelector('#window-menu .menu-item:has-text("Use the Slack Web App from Brave")');
  const entries = await page.locator('#window-menu').evaluate((m) => Array.from(m.children, (c) => c.textContent).filter(Boolean));
  assert.deepEqual(entries, [
    'Reload Page', 'Rename…', 'Close Page',
    'Split Right⌃⌥⇧→', 'Split Down⌃⌥⇧↓',
    'Split Right with an Empty Pane', 'Split Down with an Empty Pane', 'Zoom Pane⌃⌥↩',
    'Move to Pane', 'Pane with Outlook',
    'Move to Tab', 'Also Show In',
    'Page100%', 'Find in Page…', 'Copy Page Link', 'Stop Sound and Video',
    'Open in Browser', 'Pop Out of Bench',
    'Use the Slack Web App from Brave',
  ]);
  await page.locator('#window-menu .menu-item', { hasText: 'Pane with Outlook' }).click();
  v = await waitView(page, (x) => x.display.panes.length === 1);
  assert.deepEqual(itemsOf(v, 'Comms'), [[{ app: 'Outlook', title: 'Outlook' }, { app: 'Slack', title: 'Slack' }]]);
  assert.equal(v.display.panes[0].active, itemID(v, 'Slack'));
});

test('Pop Out of Bench puts a window under Not in a tab, and the add dialog brings it back', async (t) => {
  const { page, v } = await openCode(t);
  const [mainPane, terminalPane] = panesOf(v, 'Code');
  const zed = itemID(v, 'bench — switch.go');
  await page.click('[data-action="menu"][data-pane="' + mainPane.id + '"]');
  await page.locator('#window-menu .menu-item', { hasText: 'Pop Out of Bench' }).click();
  let w = await waitView(page, (x, a) => (x.floating || []).some((f) => f.id === a), zed);
  assert.equal(w.items[zed].tab, -1);
  assert.equal(paneOf(w, zed), '');
  assert.deepEqual(itemsOf(w, 'Code')[0], [{ app: 'VS Code', title: 'index.html — bench-ui' }]);

  // The palette lists it under Not in a tab.
  await page.click('[data-action="open-palette"]');
  await page.fill('#palette-query', 'switch.go');
  const rows = await page.$$eval('#palette-list > :not(.palette-set), #palette-list .palette-set > *', (els) => els.map((el) => el.textContent));
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

test('Control+Option+Shift+3 sends the shown window to Research, and the toast\'s Go there follows it', async (t) => {
  const { page, v } = await openCode(t);
  const [mainPane] = panesOf(v, 'Code');
  const zed = itemID(v, 'bench — switch.go');
  await page.click('.pane[data-pane-id="' + mainPane.id + '"] .pane-header', { position: { x: 400, y: 20 } });
  await waitView(page, (x, a) => x.display.focus === a, mainPane.id);
  await page.keyboard.press('Control+Alt+Shift+Digit3');
  let w = await waitView(page, (x, a) => x.items[a].tab === 2, zed);
  // Bench stays on Code, and the window is in Research's focused pane.
  assert.equal(w.current, 1);
  assert.equal(paneOf(w, zed), w.tabs[2].focus);
  assert.deepEqual(itemsOf(w, 'Code')[0], [{ app: 'VS Code', title: 'index.html — bench-ui' }]);
  await waitForToast(page, 'Moved Zed — bench — switch.go to Research.');
  assert.deepEqual(await page.$$eval('#toast .toast-action', (els) => els.map((el) => el.textContent)), ['Undo', 'Go there']);
  await page.click('#toast .toast-action:has-text("Go there")');
  w = await waitView(page, (x) => x.current === 2);
  assert.equal(w.display.focus, w.tabs[2].focus);
  assert.equal(w.display.panes.find((p) => p.id === w.display.focus).active, zed);

  // Control+Option+Shift+` sends it back to Code, the tab used last.
  await page.keyboard.press('Control+Alt+Shift+Backquote');
  w = await waitView(page, (x, a) => x.items[a].tab === 1, zed);
  assert.equal(w.current, 2);
  await waitForToast(page, 'Moved Zed — bench — switch.go to Code.');
  // To the tab it's on, it says so.
  await page.keyboard.press('Control+Alt+Shift+Digit3');
  await waitForToast(page, 'That window is already in Research.');
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
  // They're the gap apart, 4 pt by default.
  assert.equal(w.settings.gap, 4);
  assert.equal(w.display.panes[1].rect.x, w.display.panes[0].rect.x + w.display.panes[0].rect.w + w.settings.gap);
});
