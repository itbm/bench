'use strict';

// config.toml edited outside Bench while it runs: a save doesn't write over
// the edit, the notice offers Reload and Overwrite, and Reload config.toml
// applies the edit. And a config.toml with problems: Bench runs with the
// last config that worked, and says it doesn't save.

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const {
  openPage, setScreen, startPreview, view, waitForToast, waitUntil, waitView,
} = require('./helpers/preview.js');

async function open(t) {
  const preview = await startPreview({ t });
  const { page, close } = await openPage(preview);
  t.after(close);
  await setScreen(page, 'monitor');
  return { preview, page };
}

// configPath returns the path of config.toml, or of another file beside it.
function configPath(preview, name = 'config.toml') {
  return path.join(preview.home, name);
}

// saveSettings saves the Settings dialog with a new threshold for small
// screens.
async function saveSettings(page, compactBelow) {
  await page.click('[data-action="open-settings"]');
  await page.waitForSelector('#settings-title:text("Settings")');
  await page.fill('#compact-below', String(compactBelow));
  await page.click('[data-action="save-settings"]');
}

// noticeButtons returns the buttons of the notice whose text starts with
// prefix, from the list of notices.
async function noticeButtons(page, prefix) {
  await page.click('[data-action="notices"]');
  await page.waitForSelector('#notices-menu');
  return page.$$eval('#notices-menu .menu-text', (els, p) => {
    const el = els.find((e) => e.firstChild.textContent.startsWith(p));
    return el ? Array.from(el.querySelectorAll('button'), (b) => b.textContent) : null;
  }, prefix);
}

test('a save keeps config.toml edited outside Bench, and Reload uses the edit', async (t) => {
  const { preview, page } = await open(t);
  const file = configPath(preview);
  await waitUntil(() => fs.existsSync(configPath(preview, 'config.toml.good')), 'the last good config to be kept');
  const edited = fs.readFileSync(file, 'utf8').replace(/^gap = 8\b/m, 'gap = 2');
  assert.match(edited, /^gap = 2/m);
  fs.writeFileSync(file, edited);

  await saveSettings(page, 1600);
  await waitForToast(page, 'Applied for now, not saved: config.toml was changed outside Bench');
  await waitView(page, (v) => v.settings.compactBelow === 1600);
  assert.equal(fs.readFileSync(file, 'utf8'), edited);
  assert.deepEqual(await noticeButtons(page, 'config.toml was changed outside Bench'), ['Reload', 'Overwrite']);

  await page.click('#notices-menu button:has-text("Reload")');
  await waitForToast(page, 'Reloaded config.toml');
  const v = await waitView(page, (x) => x.settings.gap === 2 && (x.notices || []).length === 0);
  assert.equal(v.settings.compactBelow, 1800, 'the file\'s threshold, not the unsaved one');
  assert.equal(fs.readFileSync(file, 'utf8'), edited);

  // Saves work again.
  await saveSettings(page, 1700);
  await waitForToast(page, 'Saved settings');
  await waitUntil(() => /^compact_below = 1700\b/m.test(fs.readFileSync(file, 'utf8')), 'config.toml to be saved');
  assert.match(fs.readFileSync(file, 'utf8'), /^gap = 2\b/m);
});

test('Overwrite writes Bench\'s changes over config.toml edited outside Bench', async (t) => {
  const { preview, page } = await open(t);
  const file = configPath(preview);
  const edited = fs.readFileSync(file, 'utf8') + '\n# My note.\n';
  fs.writeFileSync(file, edited);

  await saveSettings(page, 1600);
  await waitForToast(page, /^Applied for now, not saved/);
  await noticeButtons(page, 'config.toml was changed outside Bench');
  await page.click('#notices-menu button:has-text("Overwrite")');
  await waitForToast(page, /^Saved Bench's changes to config.toml/);
  await waitUntil(() => /^compact_below = 1600\b/m.test(fs.readFileSync(file, 'utf8')), 'config.toml to be overwritten');
  assert.equal(fs.readFileSync(configPath(preview, 'config.toml.bak'), 'utf8'), edited);
  await waitView(page, (v) => (v.notices || []).length === 0);
});

test('with problems in config.toml, Bench runs with the last config that worked and says it doesn\'t save', async (t) => {
  // The first run keeps the example config as the last good one.
  const first = await startPreview({ t });
  await waitUntil(() => fs.existsSync(configPath(first, 'config.toml.good')), 'the last good config to be kept');
  await first.stop();
  const file = configPath(first);
  const broken = fs.readFileSync(file, 'utf8').replace(/^gap = 8\b/m, 'gapp = 8');
  fs.writeFileSync(file, broken);

  const preview = await startPreview({ t, home: first.home });
  const { page, close } = await openPage(preview);
  t.after(close);
  const v = await view(page);
  assert.deepEqual(v.tabs.map((x) => x.name), ['Comms', 'Code', 'Research']);
  assert.equal(v.configWritable, false);
  assert.match(v.notices[0].text, /^config\.toml has problems, so Bench is using the last config that worked\./);
  assert.deepEqual(v.notices[0].actions.map((a) => a.label), ['Open config.toml', 'Show in Finder', 'Reload']);
  assert.match(v.notices[1].text, /gapp/);

  // The preview opens nothing on the computer it runs on.
  await noticeButtons(page, 'config.toml has problems');
  await page.click('#notices-menu button:has-text("Open config.toml")');
  await waitForToast(page, 'This build of Bench can\'t open files.');

  await saveSettings(page, 1600);
  await waitForToast(page, 'Applied for now, not saved: config.toml has problems');
  assert.equal(fs.readFileSync(file, 'utf8'), broken);

  // Fixing the file and reloading it puts things right.
  fs.writeFileSync(file, broken.replace(/^gapp = 8\b/m, 'gap = 8'));
  await noticeButtons(page, 'config.toml has problems');
  await page.click('#notices-menu button:has-text("Reload")');
  await waitForToast(page, 'Reloaded config.toml');
  await waitView(page, (x) => x.configWritable && (x.notices || []).length === 0);
});
