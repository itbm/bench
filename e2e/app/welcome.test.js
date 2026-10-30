'use strict';

// The welcome guide in the browser preview: the first time Bench opens, its
// starting set-up suggests tabs made only of the demo's installed apps,
// and adds the ones kept to config.toml; then its tips; and Jump to… shows
// it again, without the set-up.

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const { openPage, startPreview, waitForToast, waitUntil, waitView } = require('./helpers/preview.js');

test('the welcome guide\'s starting set-up adds tabs of installed apps to config.toml', async (t) => {
  const preview = await startPreview({ t });
  const { page, close } = await openPage(preview, { keepGuide: true });
  t.after(close);
  await page.waitForSelector('.guide-sheet[data-page="permissions"]');
  await page.click('[data-key="guide-continue"]');
  await page.waitForSelector('.guide-sheet[data-page="setup"] .starter');

  // The example config has Comms and Code, and Research has Obsidian, so
  // the set-up suggests only Plan: Notes and Calendar, which the demo has.
  const starters = await page.$$eval('.starter', (els) => els.map((el) => [
    el.querySelector('.starter-name').textContent, ...Array.from(el.querySelectorAll('.starter-app'), (a) => a.textContent.slice(2)),
  ]));
  assert.deepEqual(starters, [['Plan', 'Notes', 'Calendar']]);
  await page.click('[data-key="guide-add-tabs"]');
  await waitForToast(page, 'Added Plan.');
  const v = await waitView(page, (x) => x.tabs.length === 4);
  assert.deepEqual(v.tabs.map((x) => x.name), ['Comms', 'Code', 'Research', 'Plan']);
  const config = await waitUntil(() => {
    const text = fs.readFileSync(path.join(preview.home, 'config.toml'), 'utf8');
    return text.includes('name = "Plan"') && text;
  }, 'config.toml to have the Plan tab');
  const plan = config.slice(config.indexOf('name = "Plan"'));
  assert.match(plan, /app = "com\.apple\.Notes"/);
  assert.match(plan, /app = "com\.apple\.iCal"/);

  await page.waitForSelector('.guide-sheet[data-page="tips"]');
  await page.click('[data-key="guide-done"]');
  await waitView(page, (x) => !x.firstRun);
  assert.ok(fs.existsSync(path.join(preview.home, 'first-run-done')), 'Done dismisses the guide for good');

  // Jump to… shows it again, without the set-up.
  await page.click('[data-action="open-palette"]');
  await page.fill('#palette-query', 'welcome');
  await page.press('#palette-query', 'Enter');
  await page.waitForSelector('.guide-sheet[data-page="permissions"]');
  assert.equal(await page.textContent('.guide-count'), 'Step 1 of 2');
});
