'use strict';

// Screenshots of the prototype and of the UI with an equivalent fixture, at
// 1440 by 900, for comparing by eye, and of the Acknowledgements window, at
// 640 by 720, and of the UI in light colours (ui-light-*). They're saved in
// screenshots/. The prototype is dark, so the UI matches macOS on a dark
// Mac for those. The prototype's editor is the UI's Tab properties
// (ui-editor); a tab's menu, Settings, and the light colours have no
// prototype to compare with.

// DARK_MAC is a browser that prefers dark, as a Mac set to dark does.
const DARK_MAC = { colorScheme: 'dark' };

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const {
  chooseMode, openBench, openPage, openPrototype, openSettings, openTabProperties, openTerminalWindow, settle, SCREENSHOTS,
} = require('./helpers/harness.js');
const { defaultWorld } = require('./helpers/fixture.js');

fs.mkdirSync(SCREENSHOTS, { recursive: true });
const shot = (name) => path.join(SCREENSHOTS, name + '.png');

test('screenshots of the prototype', async (t) => {
  const { page, close } = await openPrototype();
  t.after(close);
  await page.screenshot({ path: shot('prototype-fit') });
  await page.click('[data-action="mode"][data-mode="wide"]');
  await page.screenshot({ path: shot('prototype-wide') });
  await page.click('[data-action="tab"][data-index="2"]');
  await page.click('[data-action="mode"][data-mode="compact"]');
  await page.screenshot({ path: shot('prototype-research-compact') });
  await page.click('[data-action="mode"][data-mode="wide"]');
  await page.click('[data-action="tab"][data-index="1"]');
  await page.click('[data-action="open-palette"]');
  await page.screenshot({ path: shot('prototype-palette') });
  await page.keyboard.press('Escape');
  await page.click('[data-action="edit-tab"]');
  await page.screenshot({ path: shot('prototype-editor') });
  await page.keyboard.press('Escape');
  await page.click('.pane [data-action="menu"]');
  await page.screenshot({ path: shot('prototype-menu') });
});

test('screenshots of the UI in the browser preview', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ demo: true }), contextOptions: DARK_MAC });
  t.after(close);
  await settle(page, 400);
  await page.screenshot({ path: shot('ui-fit') });
  await chooseMode(page, 'wide');
  await fake.waitForCall('SetMode');
  await settle(page, 400);
  await page.screenshot({ path: shot('ui-wide') });
  await page.click('[data-action="tab"][data-index="2"]');
  await chooseMode(page, 'compact');
  await page.waitForSelector('.pane[data-pane-id="compact"]');
  await settle(page);
  await page.screenshot({ path: shot('ui-research-compact') });
  await chooseMode(page, 'wide');
  await page.click('[data-action="tab"][data-index="1"]');
  await page.waitForSelector('.tab[data-index="1"][aria-pressed="true"]');
  await settle(page, 300);
  await page.click('[data-action="open-palette"]');
  await settle(page);
  await page.screenshot({ path: shot('ui-palette') });
  await page.keyboard.press('Escape');
  await page.click('.tab[data-index="1"]', { button: 'right' });
  await page.waitForSelector('#tab-menu');
  await settle(page);
  await page.screenshot({ path: shot('ui-tab-menu') });
  await page.keyboard.press('Escape');
  await openTabProperties(page);
  await fake.waitForCall('PreviewTree');
  await settle(page);
  await page.screenshot({ path: shot('ui-editor') });
  await page.keyboard.press('Escape');
  await openSettings(page);
  await settle(page);
  await page.screenshot({ path: shot('ui-settings') });
  await page.keyboard.press('Escape');
  await page.click('.pane [data-action="menu"]');
  await settle(page);
  await page.screenshot({ path: shot('ui-menu') });
  assert.deepEqual(fake.errors, []);
});

test('screenshot of the UI in the app', async (t) => {
  const { page, fake, close } = await openBench({
    world: defaultWorld({ accessibility: false, notices: ['Couldn\'t register ⌃⌥Space: another app uses it. Try ⌃⌥J.', 'config.toml: tab "Code": unknown preset "mainside".'] }),
    contextOptions: DARK_MAC,
  });
  t.after(close);
  await settle(page, 400);
  await page.screenshot({ path: shot('ui-app') });
  assert.deepEqual(fake.errors, []);
});

test('screenshot of the Acknowledgements window', async (t) => {
  const { page, close } = await openPage('/acknowledgements.html', { viewport: { width: 640, height: 720 } });
  t.after(close);
  // The page follows its window, which is dark on a dark Mac.
  await page.emulateMedia({ colorScheme: 'dark' });
  await page.screenshot({ path: shot('ui-acknowledgements') });
});

// lightWorld returns the default world with the appearance setting light.
function lightWorld(opts) {
  const w = defaultWorld(opts);
  w.settings.appearance = 'light';
  return w;
}

test('screenshots of the UI in light colours', async (t) => {
  const { page, fake, close } = await openBench({ world: lightWorld({ notices: ['Stage Manager is on, so Bench can\'t keep windows in their panes. Turn it off in Control Centre.'] }) });
  t.after(close);
  await settle(page, 400);
  await page.screenshot({ path: shot('ui-light-fit') });
  await page.click('[data-action="tab"][data-index="1"]');
  await page.waitForSelector('.tab[data-index="1"][aria-pressed="true"]');
  await settle(page, 300);
  await page.screenshot({ path: shot('ui-light-code') });
  await page.click('[data-action="open-palette"]');
  await settle(page);
  await page.screenshot({ path: shot('ui-light-palette') });
  await page.keyboard.press('Escape');
  await page.click('.tab[data-index="1"]', { button: 'right' });
  await page.waitForSelector('#tab-menu');
  await settle(page);
  await page.screenshot({ path: shot('ui-light-tab-menu') });
  await page.keyboard.press('Escape');
  await openTabProperties(page);
  await fake.waitForCall('PreviewTree');
  await settle(page);
  await page.screenshot({ path: shot('ui-light-editor') });
  await page.keyboard.press('Escape');
  await openSettings(page);
  await settle(page);
  await page.screenshot({ path: shot('ui-light-settings') });
  await page.keyboard.press('Escape');
  assert.deepEqual(fake.errors, []);
});

test('screenshot of a popped-out terminal in light colours', async (t) => {
  const { page, fake, close } = await openTerminalWindow({ id: 'w5', world: lightWorld() });
  t.after(close);
  await settle(page, 300);
  await page.screenshot({ path: shot('ui-light-terminal') });
  assert.deepEqual(fake.errors, []);
});

test('screenshot of the Acknowledgements window in light colours', async (t) => {
  const { page, close } = await openPage('/acknowledgements.html', { viewport: { width: 640, height: 720 } });
  t.after(close);
  await page.emulateMedia({ colorScheme: 'light' });
  await page.screenshot({ path: shot('ui-light-acknowledgements') });
});
