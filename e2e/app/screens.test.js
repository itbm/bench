'use strict';

// Going back to the display that the user chose (decision 70), with the
// real engine. The demo bar's Simulate ▸ Unplug monitor takes the monitor
// away, as a cable does on a Mac: Bench goes to the laptop's screen. Plug
// in monitor puts it back, and once the screens settle, Bench goes back to
// it and says so, with Undo, which takes it back to the laptop. Choosing
// the laptop in the screen picker is choosing it: then Bench stays.

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const { openPage, setScreen, startPreview, waitForToast, waitView } = require('./helpers/preview.js');

// monitorButton presses the demo bar's Simulate ▸ Unplug monitor, or Plug
// in monitor, whichever it shows, and checks which.
async function monitorButton(page, label) {
  await page.click('#demo-simulate');
  assert.equal(await page.textContent('#demo-monitor'), label);
  await page.click('#demo-monitor');
}

test('Bench goes back to the monitor when it\'s plugged in again, with Undo', async (t) => {
  const preview = await startPreview({ t });
  const { page } = await openPage(preview);
  await setScreen(page, 'monitor');

  await monitorButton(page, 'Unplug monitor');
  let v = await waitView(page, (x) => x.demo.screen === 'laptop' && x.demo.unplugged);
  await page.waitForSelector('#app-window[data-screen="laptop"]');
  assert.equal(await page.getAttribute('#screen-picker [data-screen="laptop"]', 'aria-checked'), 'true');

  // Plugged in again, the monitor is back to the laptop's right; once the
  // screens settle, Bench goes back to it, and says so.
  await monitorButton(page, 'Plug in monitor');
  v = await waitView(page, (x) => !x.demo.unplugged);
  assert.equal(v.demo.screen, 'laptop');
  assert.equal(await waitForToast(page, 'Back on Monitor.'), 'Back on Monitor.');
  v = await waitView(page, (x) => x.demo.screen === 'monitor');
  await page.waitForSelector('#app-window[data-screen="monitor"]');
  // Tab ▸ Undo doesn't take the move back: only its toast's Undo does,
  // and the recent messages'.
  assert.equal(v.wentBack && v.wentBack.name, 'Move to Display');
  assert.equal(v.undo, undefined);
  assert.equal(await page.textContent('#demo-monitor'), 'Unplug monitor');

  // Undo takes Bench back to the laptop, which the user has then chosen.
  await page.click('#toast [data-action="toast-run"]');
  await waitForToast(page, 'Back on Laptop.');
  v = await waitView(page, (x) => x.demo.screen === 'laptop');
  assert.equal(v.wentBack, undefined);
  await page.waitForSelector('#app-window[data-screen="laptop"]');
  await monitorButton(page, 'Unplug monitor');
  await waitView(page, (x) => x.demo.unplugged);
  await monitorButton(page, 'Plug in monitor');
  await waitView(page, (x) => !x.demo.unplugged);
  // Longer than the screens take to settle: Bench stays on the laptop.
  await page.waitForTimeout(2500);
  v = await waitView(page, (x) => !x.demo.unplugged);
  assert.equal(v.demo.screen, 'laptop');

  // state.json keeps the display chosen apart from the one Bench is on.
  await preview.stop();
  const state = JSON.parse(fs.readFileSync(path.join(preview.home, 'state.json'), 'utf8'));
  assert.equal(state.preferredScreen && state.preferredScreen.id, 'laptop');
});

test('Unplug monitor needs the monitor', async (t) => {
  const preview = await startPreview({ t });
  const { page } = await openPage(preview);
  await setScreen(page, 'laptop');
  await monitorButton(page, 'Unplug monitor');
  assert.equal(await waitForToast(page, 'Choose Monitor in the demo bar first, then unplug it.'),
    'Choose Monitor in the demo bar first, then unplug it.');
  const v = await waitView(page, (x) => x.demo.screen === 'laptop');
  assert.equal(Boolean(v.demo.unplugged), false);
});
