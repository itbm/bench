'use strict';

// Remembering the arrangement: state.json keeps a split across a restart
// of the preview on the same support folder.

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const {
  itemID, itemsOf, openPage, panesOf, setScreen, startPreview, switchTo, waitView,
} = require('./helpers/preview.js');

test('a split survives stopping and starting the preview again', async (t) => {
  const first = await startPreview({ t });
  const one = await openPage(first);
  let v = await setScreen(one.page, 'monitor');
  v = await switchTo(one.page, 'Code');
  const vscode = itemID(v, 'index.html — bench-ui');
  const [, terminalPane] = panesOf(v, 'Code');
  const target = '.pane[data-pane-id="' + terminalPane.id + '"]';
  const b = await one.page.locator(target).boundingBox();
  await one.page.dragAndDrop('.subtab[data-item="' + vscode + '"]', target, {
    targetPosition: { x: b.width - 12, y: b.height / 2 },
  });
  v = await waitView(one.page, (x) => x.tabs[1].panes === 3);
  const arranged = itemsOf(v, 'Code');
  const sizes = v.tabs[1].root.sizes;
  await one.close();

  assert.equal(await first.stop(), 0);
  assert.equal(fs.existsSync(path.join(first.home, 'parked.json')), false, 'parked.json is removed on quit');
  const state = JSON.parse(fs.readFileSync(path.join(first.home, 'state.json'), 'utf8'));
  assert.ok(state, 'state.json is saved');

  const second = await startPreview({ home: first.home, t });
  const two = await openPage(second);
  t.after(two.close);
  // The permissions guide was dismissed in the first run.
  assert.equal(await two.page.locator('.guide-sheet').count(), 0);
  v = await waitView(two.page, (x) => x.tabs.length === 3 && x.current === 1);
  assert.deepEqual(itemsOf(v, 'Code'), arranged);
  assert.deepEqual(itemsOf(v, 'Code'), [
    [{ app: 'Zed', title: 'bench — switch.go' }],
    [{ app: 'Terminal', title: 'zsh — ~/code' }],
    [{ app: 'VS Code', title: 'index.html — bench-ui' }],
  ]);
  assert.deepEqual(v.tabs[1].root.sizes, sizes);
  assert.equal(v.tabs[1].preset, 'custom');
  // Every window was matched back to its item.
  assert.ok(Object.values(v.items).every((it) => it.status === 'ready'), 'every item is ready');
  v = await setScreen(two.page, 'monitor');
  assert.equal(v.display.panes.length, 3);
  await two.page.waitForSelector('.pane[data-pane-id="' + v.display.panes[2].id + '"] .subtab:has-text("index.html — bench-ui")');
});
