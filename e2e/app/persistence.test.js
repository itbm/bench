'use strict';

// Remembering the arrangement: state.json keeps a split across a restart
// of the preview on the same support folder, and across tabs added,
// reordered, and renamed in config.toml meanwhile, or while Bench runs.

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const {
  itemID, itemsOf, openPage, openTabProperties, panesOf, setScreen, startPreview, switchTo, waitForToast, waitUntil,
  waitView,
} = require('./helpers/preview.js');

// splitCode drags VS Code to the right of the Code tab's terminal, so that
// Code has three panes, and returns the view.
async function splitCode(page) {
  let v = await setScreen(page, 'monitor');
  v = await switchTo(page, 'Code');
  const vscode = itemID(v, 'index.html — bench-ui');
  const [, terminalPane] = panesOf(v, 'Code');
  const target = '.pane[data-pane-id="' + terminalPane.id + '"]';
  const b = await page.locator(target).boundingBox();
  await page.dragAndDrop('.subtab[data-item="' + vscode + '"]', target, {
    targetPosition: { x: b.width - 12, y: b.height / 2 },
  });
  return waitView(page, (x) => x.tabs.find((tab) => tab.name === 'Code').panes === 3);
}

// editTabs rewrites the [[tab]] blocks of config.toml.
function editTabs(home, change) {
  const file = path.join(home, 'config.toml');
  const text = fs.readFileSync(file, 'utf8');
  const start = text.indexOf('[[tab]]');
  const end = text.includes('[[rule]]') ? text.indexOf('[[rule]]') : text.length;
  const blocks = text.slice(start, end).split(/(?=\[\[tab\]\]\n)/);
  fs.writeFileSync(file, text.slice(0, start) + change(blocks).join('') + text.slice(end));
}

// reloadConfig chooses Reload config.toml in the jump palette.
async function reloadConfig(page) {
  await page.click('[data-action="open-palette"]');
  await page.waitForSelector('#palette-query');
  await page.fill('#palette-query', 'Reload config');
  await page.press('#palette-query', 'Enter');
  await waitForToast(page, 'Reloaded config.toml');
}

test('a split survives stopping and starting the preview again', async (t) => {
  const first = await startPreview({ t });
  const one = await openPage(first);
  let v = await splitCode(one.page);
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

test('tabs added, reordered, and renamed in config.toml keep or give back their arrangement', async (t) => {
  const first = await startPreview({ t });
  const one = await openPage(first);
  const arranged = itemsOf(await splitCode(one.page), 'Code');
  await one.close();
  assert.equal(await first.stop(), 0);

  // Code moves first, and Notes is added: Code keeps its split.
  editTabs(first.home, (blocks) => {
    const code = blocks.find((b) => b.includes('name = "Code"'));
    return [code, ...blocks.filter((b) => b !== code), '[[tab]]\nname = "Notes"\nlayout = "single"\n\n'];
  });
  const second = await startPreview({ home: first.home, t });
  const two = await openPage(second);
  let v = await waitView(two.page, (x) => x.tabs.length === 4);
  assert.deepEqual(v.tabs.map((x) => x.name), ['Code', 'Comms', 'Research', 'Notes']);
  assert.equal(v.current, 0, 'Code is still the current tab');
  assert.deepEqual(itemsOf(v, 'Code'), arranged);
  assert.deepEqual(v.notices || [], []);
  assert.equal(fs.existsSync(path.join(first.home, 'state.json.bad')), false);
  await two.close();
  assert.equal(await second.stop(), 0);

  // Code is renamed Dev, and Notes removed: Dev can't be told from a new
  // tab, so it starts from the file, and the notice says so.
  editTabs(first.home, (blocks) => blocks.filter((b) => !b.includes('name = "Notes"'))
    .map((b) => b.replace('name = "Code"', 'name = "Dev"')));
  const third = await startPreview({ home: first.home, t });
  const { page, close } = await openPage(third);
  t.after(close);
  v = await waitView(page, (x) => x.tabs.length === 3 && (x.notices || []).length > 0);
  assert.equal(v.tabs.find((x) => x.name === 'Dev').panes, 2);
  const notice = v.notices.find((n) => n.text.startsWith('When Bench started, config.toml had no tabs called Code and Notes,'));
  assert.ok(notice, 'a notice names Code and Notes');
  assert.deepEqual(notice.actions.map((a) => a.label), ['Dismiss']);
  assert.ok(fs.existsSync(path.join(first.home, 'state.json.bad')), 'a copy of state.json is set aside');

  // Dev is called Code again in Tab properties, and its split comes back.
  await openTabProperties(page, 'Dev');
  await page.fill('#tab-name', 'Code');
  await page.press('#tab-name', 'Enter');
  await waitView(page, (x) => x.tabs[0].name === 'Code');
  await page.click('[data-action="notices"]');
  await page.click('#notices-menu button:has-text("Restore previous arrangement")');
  await waitForToast(page, 'Restored the arrangement of Code');
  v = await waitView(page, (x) => x.tabs[0].panes === 3);
  assert.deepEqual(itemsOf(v, 'Code'), arranged);
  // Notes is left, until dismissed.
  assert.match(v.notices[0].text, /had no tab called Notes,/);
  await page.click('[data-action="notices"]');
  await page.click('#notices-menu button:has-text("Dismiss")');
  await waitView(page, (x) => (x.notices || []).length === 0);
});

test('a reload that renames a split tab, and adds another, says so, and Restore gives the split back', async (t) => {
  const preview = await startPreview({ t });
  const { page, close } = await openPage(preview);
  t.after(close);
  let v = await splitCode(page);
  const arranged = itemsOf(v, 'Code');
  await waitUntil(() => fs.existsSync(path.join(preview.home, 'state.json')), 'state.json to be saved');

  // Code is renamed Dev, and Notes added: Dev can't be told from a new tab,
  // so it takes the file's layout, with Code's windows and terminal.
  editTabs(preview.home, (blocks) => [
    ...blocks.map((b) => b.replace('name = "Code"', 'name = "Dev"')), '[[tab]]\nname = "Notes"\nlayout = "single"\n\n',
  ]);
  await reloadConfig(page);
  v = await waitView(page, (x) => x.tabs.length === 4 && (x.notices || []).length > 0);
  assert.equal(v.tabs.find((x) => x.name === 'Dev').panes, 2);
  const titles = (panes) => panes.flat().map((it) => it.title).sort();
  assert.deepEqual(titles(itemsOf(v, 'Dev')), titles(arranged));
  assert.ok(panesOf(v, 'Dev').flatMap((p) => p.items).every((id) => v.items[id].status === 'ready'), 'Dev\'s windows are docked');
  const notice = v.notices.find((n) => n.text.startsWith('The config.toml that Bench reloaded had no tab called Code,'));
  assert.ok(notice, 'a notice names Code');
  assert.deepEqual(notice.actions.map((a) => a.label), ['Dismiss']);
  assert.ok(fs.existsSync(path.join(preview.home, 'state.json.bad')), 'a copy of state.json is set aside');

  // Dev is called Code again, as the only change to the tabs, and its
  // split comes back.
  editTabs(preview.home, (blocks) => blocks.map((b) => b.replace('name = "Dev"', 'name = "Code"')));
  await reloadConfig(page);
  await waitView(page, (x) => x.tabs[1].name === 'Code');
  await page.click('[data-action="notices"]');
  await page.click('#notices-menu button:has-text("Restore previous arrangement")');
  await waitForToast(page, 'Restored the arrangement of Code');
  v = await waitView(page, (x) => x.tabs[1].panes === 3);
  assert.deepEqual(itemsOf(v, 'Code'), arranged);
  assert.deepEqual(v.notices || [], []);
});
