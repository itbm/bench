'use strict';

// Windows over Bench, with the real engine. The demo bar's Bring Bench
// forward does what a click on Bench does on a Mac, while another app is in
// front: Bench hides the apps whose windows cover it, here Finder, whose
// Downloads window isn't in a tab, and parked.json records it. The first
// time, a toast says so, with Keep over Bench, which adds a float rule for
// Finder to config.toml, and whose own toast's Undo takes the rule back.
// The recent messages offer Keep over Bench again while it applies
// (decision 68).

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const { openPage, startPreview, waitForToast, waitUntil } = require('./helpers/preview.js');

const RULE = '[[rule]]\napp = "com.apple.finder"\nfloat = true\n';

function readConfig(preview) {
  return fs.readFileSync(path.join(preview.home, 'config.toml'), 'utf8');
}

function readState(preview) {
  return JSON.parse(fs.readFileSync(path.join(preview.home, 'state.json'), 'utf8'));
}

// finderHidden reports whether parked.json records Finder as an app that
// Bench hid, as it does before it hides one.
function finderHidden(preview) {
  try {
    const parked = JSON.parse(fs.readFileSync(path.join(preview.home, 'parked.json'), 'utf8'));
    return (parked.hidden || []).some((a) => a.bundleId === 'com.apple.finder');
  } catch {
    return false;
  }
}

test('Bench hides what covers it as it comes forward, says so once, and Keep over Bench adds a float rule', async (t) => {
  const preview = await startPreview({ t });
  const { page, close } = await openPage(preview);
  t.after(close);
  assert.doesNotMatch(readConfig(preview), /app = "com\.apple\.finder"/);

  await page.click('#demo-bar [data-action="demo-come-forward"]');
  await waitForToast(page, 'Hid Finder because it covered Bench.');
  assert.equal(await page.textContent('#toast .toast-action'), 'Keep over Bench');
  assert.ok(finderHidden(preview), 'parked.json records Finder as hidden');
  // state.json remembers that Bench said so.
  await waitUntil(() => {
    try {
      return readState(preview).covering?.includes('com.apple.finder');
    } catch {
      return false;
    }
  }, 'state.json to list Finder under covering');

  await page.click('#toast .toast-action');
  await waitForToast(page, 'Added a rule to config.toml: Finder windows now stay over Bench, and new ones stay out of your tabs.');
  assert.equal(await page.textContent('#toast .toast-action'), 'Undo');
  assert.ok(readConfig(preview).includes(RULE), 'config.toml has the float rule');
  await waitUntil(() => !finderHidden(preview), 'Keep over Bench to show Finder again');

  // Undo takes the rule back.
  await page.click('#toast .toast-action');
  await waitForToast(page, 'Finder windows no longer stay over Bench.');
  assert.ok(!readConfig(preview).includes(RULE), 'Undo takes the rule back');

  // Bench hides Finder again as it comes forward, without saying so again.
  await page.evaluate(() => {
    window.__toasts = [];
    const el = document.getElementById('toast');
    new MutationObserver(() => window.__toasts.push(el.textContent)).observe(el, { childList: true, characterData: true, subtree: true });
  });
  await page.click('#demo-bar [data-action="demo-come-forward"]');
  await waitUntil(() => finderHidden(preview), 'Bench to hide Finder again');
  await page.waitForTimeout(500);
  assert.deepEqual(await page.evaluate(() => window.__toasts.filter((x) => /covered Bench/.test(x))), []);

  // The recent messages offer Keep over Bench again, by the message that
  // said Bench hid Finder, as Finder no longer floats.
  await page.click('[data-action="notices"]');
  const hid = page.locator('#notices-menu .message', { hasText: 'Hid Finder because it covered Bench.' });
  await hid.locator('[data-action="message-keep"]').click();
  await waitUntil(() => readConfig(preview).includes(RULE), 'config.toml to have the float rule again');
  await page.waitForFunction(() => !document.querySelector('#notices-menu [data-action="message-keep"]'));
});
