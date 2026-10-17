'use strict';

// Live previews on hovering an inactive sub-tab.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, settle } = require('./helpers/harness.js');
const { defaultWorld } = require('./helpers/fixture.js');

test('in the browser preview, hovering an inactive sub-tab shows a preview after 450 ms', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ demo: true, livePreviews: true }) });
  t.after(close);
  await page.hover('.subtab[data-item="w4"]');
  await page.waitForTimeout(200);
  assert.equal(await page.isVisible('#preview'), false, 'not before 450 ms');
  await page.waitForSelector('#preview:not([hidden])', { timeout: 1000 });
  assert.equal(await page.textContent('.preview-caption strong'), 'index.html — bench-ui');
  assert.equal(await page.textContent('.preview-caption span'), 'Live');
  assert.ok(await page.$('#preview .preview-content .code'), 'placeholder content');
  // It sits under the sub-tab.
  const sub = await page.locator('.subtab[data-item="w4"]').boundingBox();
  const prev = await page.locator('#preview').boundingBox();
  assert.ok(prev.y > sub.y + sub.height - 1 && prev.y < sub.y + sub.height + 12);
  // Leaving hides it; there's no native preview in the browser preview.
  await page.mouse.move(700, 600);
  await page.waitForSelector('#preview', { state: 'hidden' });
  assert.equal(fake.callsTo('ShowPreview').length, 0);

  // The shown sub-tab has no preview.
  await page.hover('.subtab[data-item="w3"]');
  await page.waitForTimeout(600);
  assert.equal(await page.isVisible('#preview'), false);
});

test('previews are off unless live previews are turned on', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ demo: true }) });
  t.after(close);
  await page.hover('.subtab[data-item="w4"]');
  await page.waitForTimeout(650);
  assert.equal(await page.isVisible('#preview'), false);
  assert.equal(fake.callsTo('ShowPreview').length, 0);
});

test('in the app, hovering calls ShowPreview with the sub-tab\'s rectangle, and leaving calls HidePreview', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ livePreviews: true, screenRecording: true }) });
  t.after(close);
  const start = Date.now();
  await page.hover('.subtab[data-item="w4"]');
  const call = await fake.waitForCall('ShowPreview');
  assert.ok(call.at - start >= 400, 'after hovering for 450 ms');
  const rect = await page.$eval('.subtab[data-item="w4"]', (el) => {
    const r = el.getBoundingClientRect();
    return { x: Math.round(r.left), y: Math.round(r.top), w: Math.round(r.width), h: Math.round(r.height) };
  });
  assert.deepEqual(call.args, ['w4', rect]);
  assert.equal(await page.isVisible('#preview'), false, 'the app draws nothing itself');
  await page.mouse.move(700, 600);
  await fake.waitForCall('HidePreview');
  await settle(page);
  assert.equal(fake.callsTo('HidePreview').length, 1);
});

test('in the app, previews need Screen Recording, and the editor says how to grant it', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ livePreviews: true, screenRecording: false }) });
  t.after(close);
  await page.hover('.subtab[data-item="w4"]');
  await page.waitForTimeout(650);
  assert.equal(fake.callsTo('ShowPreview').length, 0, 'no preview without permission');
  assert.equal(await page.textContent('#toast'), '', 'and no toast on hover');

  await page.click('[data-action="edit-tab"]');
  await page.waitForSelector('.editor-sheet');
  const note = page.locator('#recording-note');
  assert.match(await note.textContent(), /doesn't have Screen Recording permission yet/);
  assert.equal(await page.getAttribute('[data-setting="livePreviews"]', 'aria-describedby'), 'recording-note');
  // Turning previews off removes the note.
  await page.click('[data-setting="livePreviews"]');
  await page.waitForSelector('#recording-note', { state: 'detached' });

  // Once Go reports the permission, hovering shows previews.
  await page.keyboard.press('Escape');
  fake.world.permissions.screenRecording = true;
  await fake.emitState();
  await settle(page);
  await page.hover('.subtab[data-item="w3"]');
  await page.hover('.subtab[data-item="w4"]');
  await fake.waitForCall('ShowPreview');
});

test('a pending preview doesn\'t show if previews are turned off meanwhile', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ demo: true, livePreviews: true }) });
  t.after(close);
  await page.hover('.subtab[data-item="w4"]');
  fake.world.settings.livePreviews = false;
  await fake.emitState();
  await page.waitForTimeout(650);
  assert.equal(await page.isVisible('#preview'), false);
});

test('a preview goes when a redraw removes its sub-tab', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ livePreviews: true, screenRecording: true }) });
  t.after(close);
  await page.hover('.subtab[data-item="w4"]');
  await fake.waitForCall('ShowPreview');
  // A shortcut switches tabs while the pointer stays where it is.
  fake.world.current = 2;
  await fake.emitState();
  await fake.waitForCall('HidePreview');
});
