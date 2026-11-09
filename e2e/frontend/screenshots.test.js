'use strict';

// Screenshots of the prototype and of the UI with an equivalent fixture, at
// 1440 by 900, for comparing by eye, and of the Acknowledgements window, at
// 640 by 720, and of the UI in light colours (ui-light-*), with agents'
// marks too (ui-agents* and ui-light-agents*). They're saved in
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
  await page.waitForSelector('.tab[data-index="1"][aria-selected="true"]');
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
  await page.waitForSelector('.tab[data-index="1"][aria-selected="true"]');
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

// agentWorld returns the default world with agents in terminals in each
// state (decision 78), as Go sends them: in the Code tab, which is shown,
// one at work and one that needs you in one pane, and one that has
// finished and one that stopped with an error in the other, which Go has
// marked as they weren't shown; and in Research, one at work.
function agentWorld(w) {
  const terminal = (id, title, agent) => {
    w.items[id] = { ...w.items.w5, id, title, ...agent };
    return id;
  };
  Object.assign(w.items.w5, { agent: 'working', agentText: 'Running the tests' });
  Object.assign(w.items.w6, {
    agent: 'waiting', agentKind: 'permission', agentText: 'approve Bash: npm test', attention: 'Needs your permission: approve Bash: npm test',
  });
  w.tabs[1].root.children[0].items.push(
    terminal('w11', 'claude — ~/code/web', { agent: 'done', agentText: 'All 12 tests pass', attention: 'Finished: All 12 tests pass' }),
    terminal('w12', 'codex — ~/code/api', { agent: 'error', attention: 'Stopped with an error' }),
  );
  w.tabs[2].root.children[1].items.push(terminal('w13', 'claude — ~/notes', { agent: 'working' }));
  return w;
}

// agentShots saves the window with agents' marks, and, at twice the size,
// the title bar and the headers of the panes, as name, name-tabs, and
// name-subtabs-p3 and -p4. The dots of an agent at work pulse, so
// animations are stopped, which shows them in full.
async function agentShots(page, name) {
  await page.waitForSelector('.subtab[data-item="w12"] .agent-mark');
  await settle(page, 400);
  const still = { animations: 'disabled' };
  await page.screenshot({ path: shot(name), scale: 'css', ...still });
  await page.locator('#titlebar').screenshot({ path: shot(name + '-tabs'), ...still });
  for (const p of ['p3', 'p4']) {
    await page.locator('.pane[data-pane-id="' + p + '"] .pane-header').screenshot({ path: shot(name + '-subtabs-' + p), ...still });
  }
}

test('screenshots of agents\' marks, in dark and light colours', async (t) => {
  const dark = await openBench({ world: agentWorld(defaultWorld()), contextOptions: { ...DARK_MAC, deviceScaleFactor: 2 } });
  t.after(dark.close);
  await agentShots(dark.page, 'ui-agents');
  assert.deepEqual(dark.fake.errors, []);
  const light = await openBench({ world: agentWorld(lightWorld()), contextOptions: { deviceScaleFactor: 2 } });
  t.after(light.close);
  await agentShots(light.page, 'ui-light-agents');
  assert.deepEqual(light.fake.errors, []);
});

test('screenshot of the Acknowledgements window in light colours', async (t) => {
  const { page, close } = await openPage('/acknowledgements.html', { viewport: { width: 640, height: 720 } });
  t.after(close);
  await page.emulateMedia({ colorScheme: 'light' });
  await page.screenshot({ path: shot('ui-light-acknowledgements') });
});
