'use strict';

// Events: the browser preview gets Go's events over a WebSocket that Wails
// opens a moment after the page loads, and reopens if it drops. Events sent
// while it isn't open never arrive, so the page catches up when it opens.

const assert = require('node:assert/strict');
const test = require('node:test');
const { browser, startPreview, TIMEOUT } = require('./helpers/preview.js');

const SERVICE = 'github.com/itbm/bench/internal/ui.Service.';

// callGo calls a bound method over Wails' HTTP transport, as the page does.
async function callGo(preview, method, ...args) {
  const res = await fetch(preview.url + '/wails/runtime', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ object: 0, method: 0, args: { 'call-id': 'e2e-' + method, methodName: SERVICE + method, args } }),
  });
  assert.equal(res.status, 200, method + ' answered ' + res.status + ': ' + await res.text());
}

// isCall reports whether a request calls a bound method.
function isCall(request, method) {
  return request.method() === 'POST' && (request.postData() || '').includes('"' + SERVICE + method + '"');
}

test('the page catches up on changes made before its event socket opened', async (t) => {
  const preview = await startPreview({ t });
  const context = await (await browser()).newContext({ viewport: { width: 1600, height: 1000 } });
  preview.contexts.push(context);
  const page = await context.newPage();
  const errors = [];
  page.on('pageerror', (err) => errors.push(String(err)));

  // Hold back the script that opens the event socket.
  let release;
  const held = new Promise((r) => { release = r; });
  await page.route('**/wails/custom.js', async (route) => {
    await held;
    await route.continue();
  });
  // The page draws Ready's View and reports its pane area, but hears
  // nothing back: Go's answer to that, and to a change made elsewhere, is
  // lost.
  const measured = page.waitForResponse((res) => isCall(res.request(), 'SetPaneArea'), { timeout: TIMEOUT });
  await page.goto(preview.url + '/');
  await measured;
  assert.deepEqual(await page.evaluate(() => window.bench.store.view.display.area), { x: 0, y: 0, w: 0, h: 0 });
  await callGo(preview, 'SetMode', 'wide');

  // Once the socket opens, the page asks for the View again.
  const again = page.waitForRequest((req) => isCall(req, 'Ready'), { timeout: TIMEOUT });
  release();
  await again;
  await page.waitForFunction(() => {
    const v = window.bench.store.view;
    return v.mode === 'wide' && v.display.area.h > 0;
  }, null, { timeout: TIMEOUT });
  assert.equal(await page.getAttribute('[data-action="mode"][data-mode="wide"]', 'aria-checked'), 'true');
  const pane = await page.locator('.pane').first().boundingBox();
  assert.ok(pane.width > 100 && pane.height > 100, 'the panes are drawn in the pane area');
  assert.deepEqual(errors, []);
});
