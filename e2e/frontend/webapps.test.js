'use strict';

// A web page's menu, which puts a web app of its site in the page's place:
// the web app the user has (UseWebApp), or one that the default browser
// makes while the add dialog's make step waits (MakeWebApp and
// UseWebAppWhenReady).

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, settle } = require('./helpers/harness.js');
const { defaultWorld, pane, split } = require('./helpers/fixture.js');

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const toastIs = (page, text) => page.waitForFunction((t) => document.getElementById('toast').textContent === t, text);

// openComms opens Bench on the Comms tab: Slack's page in p1, Outlook's in
// p2.
function openComms(opts = {}) {
  return openBench({ world: defaultWorld({ current: 0 }), ...opts });
}

// openMenu opens the ⋯ menu of a pane.
async function openMenu(page, paneId) {
  await page.click('.pane[data-pane-id="' + paneId + '"] [data-action="menu"]');
  await page.waitForSelector('#window-menu');
}

// entries returns the open menu's entries, as the menu interact.test.js
// checks them.
const entries = (page) => page.$$eval('#window-menu > *', (els) => els.map((el) => el.className + ':' + el.textContent));

// webAppEntry waits for the open menu's web app entry with a label.
const webAppEntry = (page, label) => page.waitForSelector('#window-menu .menu-item:has-text("' + label + '")');

test('a web page\'s menu opens at once, and offers the web app when Go has answered', async (t) => {
  const { page, fake, close } = await openComms();
  t.after(close);
  let answer = null;
  fake.on('WebAppChoice', () => new Promise((resolve) => { answer = resolve; }));
  await openMenu(page, 'p1');
  assert.deepEqual((await fake.waitForCall('WebAppChoice')).args, ['w1']);
  const before = await entries(page);
  assert.equal(before[before.length - 1], 'menu-item:Pop out of Bench');
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'menu-item-0');
  answer({ offer: 'use', app: 'Slack', browser: 'Brave' });
  await webAppEntry(page, 'Use the Slack web app from Brave');
  // The entry joins at the end, so nothing above it moves, and the focus
  // stays where it was.
  assert.deepEqual(await entries(page), [...before, 'menu-sep:', 'menu-item:Use the Slack web app from Brave']);
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'menu-item-0');
  await page.keyboard.press('ArrowUp');
  assert.equal(await page.evaluate(() => document.activeElement.textContent), 'Use the Slack web app from Brave');
  await page.keyboard.press('Escape');
  await page.waitForSelector('#window-menu', { state: 'detached' });
  assert.equal(fake.callsTo('UseWebApp').length, 0);
});

// boxes returns the boxes of the open menu's entries and their shortcuts,
// and the menu's left, top, and width.
const boxes = (page) => page.evaluate(() => {
  const box = (el) => { const r = el.getBoundingClientRect(); return [r.x, r.y, r.width, r.height].map(Math.round); };
  const menu = document.getElementById('window-menu');
  return { menu: box(menu).slice(0, 3), entries: Array.from(menu.querySelectorAll(':scope > *, kbd'), box) };
});

test('nothing in a web page\'s menu moves when the web app entry joins it', async (t) => {
  // Outlook's pane is below Slack's, so its menu reaches the bottom of the
  // window; and the names in the entry can be long.
  const lower = defaultWorld({ current: 0 });
  lower.tabs[0].root = split('s1', 'column', [50, 50], [pane('p1', ['w1']), pane('p2', ['w2'])]);
  const cases = [
    { world: lower, paneId: 'p2', choice: { offer: 'make', browser: 'Safari' }, label: 'Make a web app in Safari…' },
    { world: lower, paneId: 'p2', choice: { offer: 'use', app: 'Outlook', browser: 'Microsoft Edge' }, label: 'Use the Outlook web app from Microsoft Edge' },
    { paneId: 'p1', choice: { offer: 'use', app: 'Google Calendar', browser: 'Chrome' }, label: 'Use the Google Calendar web app from Chrome' },
    {
      paneId: 'p1', choice: { offer: 'use', app: 'Microsoft Teams (work or school)', browser: 'Microsoft Edge' },
      label: 'Use the Microsoft Teams (work or school) web app from Microsoft Edge',
    },
  ];
  for (const c of cases) {
    const { page, fake, close } = await openBench({ world: c.world || defaultWorld({ current: 0 }) });
    try {
      let answer = null;
      fake.on('WebAppChoice', () => new Promise((resolve) => { answer = resolve; }));
      await openMenu(page, c.paneId);
      await fake.waitForCall('WebAppChoice');
      const before = await boxes(page);
      answer(c.choice);
      const entry = await webAppEntry(page, c.label);
      const after = await boxes(page);
      assert.deepEqual(after.menu, before.menu, c.label + ': the menu moved');
      assert.deepEqual(after.entries.slice(0, before.entries.length), before.entries, c.label + ': an entry moved');
      // The whole entry shows, without scrolling the menu.
      assert.deepEqual(await entry.evaluate((el) => {
        const label = el.querySelector('.menu-label');
        const r = el.getBoundingClientRect();
        const m = el.parentElement.getBoundingClientRect();
        return [label.textContent, label.scrollWidth <= label.clientWidth, r.bottom <= m.bottom, r.bottom <= window.innerHeight];
      }), [c.label, true, true, true], c.label);
      assert.equal(await page.evaluate(() => document.getElementById('window-menu').scrollTop), 0, c.label);
    } finally {
      await close();
    }
  }
});

test('a web page\'s menu offers to make a web app, or nothing', async (t) => {
  const { page, fake, close } = await openComms();
  t.after(close);
  await openMenu(page, 'p2');
  assert.deepEqual((await fake.waitForCall('WebAppChoice')).args, ['w2']);
  await webAppEntry(page, 'Make a web app in Safari…');
  const all = await entries(page);
  assert.deepEqual(all.slice(-3), ['menu-item:Pop out of Bench', 'menu-sep:', 'menu-item:Make a web app in Safari…']);
  await page.keyboard.press('Escape');
  await page.waitForSelector('#window-menu', { state: 'detached' });

  // Go offers nothing, or refuses, as for a page that has gone: the menu
  // stays as it is, without a toast.
  for (const answer of [() => ({ offer: '' }), () => { throw new Error('That window has closed.'); }]) {
    fake.clearCalls();
    fake.on('WebAppChoice', answer);
    await openMenu(page, 'p2');
    await fake.waitForCall('WebAppChoice');
    await settle(page);
    assert.deepEqual(await entries(page), all.slice(0, -2));
    assert.equal(await page.evaluate(() => document.getElementById('toast').classList.contains('show')), false);
    await page.keyboard.press('Escape');
    await page.waitForSelector('#window-menu', { state: 'detached' });
  }
});

test('the menu of an App item or a terminal doesn\'t ask', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await openMenu(page, 'p3');
  await page.keyboard.press('Escape');
  await page.click('.subtab:has-text("zsh")', { button: 'right' });
  await page.waitForSelector('#window-menu');
  await settle(page);
  assert.equal(fake.callsTo('WebAppChoice').length, 0);
});

test('an answer for a menu that has closed is dropped', async (t) => {
  const { page, fake, close } = await openComms();
  t.after(close);
  // Slack's answer is slow; Outlook's offers nothing.
  fake.on('WebAppChoice', async ([itemId]) => {
    if (itemId !== 'w1') return { offer: '' };
    await sleep(500);
    return { offer: 'use', app: 'Slack', browser: 'Brave' };
  });
  await openMenu(page, 'p1');
  await page.keyboard.press('Escape');
  await page.waitForSelector('#window-menu', { state: 'detached' });
  // Outlook's menu opens before Slack's answer comes.
  await openMenu(page, 'p2');
  await sleep(700);
  assert.equal(await page.locator('#window-menu .menu-item', { hasText: 'web app' }).count(), 0);
  assert.deepEqual(fake.callsTo('WebAppChoice').map((c) => c.args), [['w1'], ['w2']]);
  await page.keyboard.press('Escape');
  await page.waitForSelector('#window-menu', { state: 'detached' });
  // Slack's menu, opened again, gets its own answer.
  await openMenu(page, 'p1');
  await webAppEntry(page, 'Use the Slack web app from Brave');
});

test('Use the web app calls UseWebApp, and says what it did', async (t) => {
  const { page, fake, close } = await openComms();
  t.after(close);
  await openMenu(page, 'p1');
  await (await webAppEntry(page, 'Use the Slack web app from Brave')).click();
  assert.deepEqual((await fake.waitForCall('UseWebApp')).args, ['w1']);
  await page.waitForSelector('#window-menu', { state: 'detached' });
  await toastIs(page, 'Replaced Slack with the Slack web app.');
  // The web app is in the page's place.
  await page.waitForSelector('.pane[data-pane-id="p1"] .badge:has-text("App")');

  // A refusal says why.
  fake.on('WebAppChoice', () => ({ offer: 'use', app: 'Outlook', browser: 'Chrome' }));
  fake.on('UseWebApp', () => { throw new Error('Bench couldn\'t open Outlook.'); });
  await page.click('.subtab:has-text("Inbox")', { button: 'right' });
  await page.waitForSelector('#window-menu');
  await (await webAppEntry(page, 'Use the Outlook web app from Chrome')).click();
  await toastIs(page, 'Bench couldn\'t open Outlook.');
});

// openMakeStep opens Outlook's menu and chooses to make a web app.
async function openMakeStep(page, fake) {
  await openMenu(page, 'p2');
  await (await webAppEntry(page, 'Make a web app in Safari…')).click();
  assert.deepEqual((await fake.waitForCall('MakeWebApp')).args, ['w2']);
  await page.waitForSelector('#palette-make-title');
}

test('Make a web app opens the make step, which puts the web app in the page\'s place', async (t) => {
  const { page, fake, close } = await openComms();
  t.after(close);
  let polls = 0;
  fake.on('UseWebAppWhenReady', (args, f) => {
    polls += 1;
    if (polls < 3) return false;
    f.replaceWithApp('w2', { app: 'Outlook', mono: 'Ou', bundleId: 'com.apple.Safari.WebApp.1' });
    return true;
  });
  await openMakeStep(page, fake);
  assert.equal(await page.textContent('#palette-make-title'), 'Make a web app in Safari');
  assert.equal(await page.textContent('#palette-make-steps'),
    'Bench has opened Outlook in Safari. Choose File ▸ Add to Dock, then click Add.');
  assert.match(await page.textContent('.palette'), /Bench puts the web app in place of this page as soon as it's ready\./);
  // Cancel is the only button: the page is in Bench already.
  assert.deepEqual(await page.$$eval('.palette-actions button', (els) => els.map((el) => el.textContent)), ['Cancel']);
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'make-cancel');

  // The user goes to the browser: the dialog stays open, and keeps asking.
  await page.evaluate(() => window.dispatchEvent(new Event('blur')));
  await page.waitForSelector('.palette', { state: 'detached', timeout: 10000 });
  await toastIs(page, 'Replaced Outlook with its web app.');
  const asks = fake.callsTo('UseWebAppWhenReady');
  assert.equal(asks.length, 3);
  assert.deepEqual(asks.map((c) => c.args), [['w2'], ['w2'], ['w2']]);
  for (let i = 1; i < asks.length; i++) {
    assert.ok(asks[i].at - asks[i - 1].at >= 1400, 'about 1.5 s apart: ' + (asks[i].at - asks[i - 1].at) + ' ms');
  }
  assert.equal(fake.callsTo('AddWebAppWhenReady').length, 0);
  await fake.waitForLowered();
  await page.waitForSelector('.pane[data-pane-id="p2"] .badge:has-text("App")');
  await sleep(2000);
  assert.equal(fake.callsTo('UseWebAppWhenReady').length, 3);
});

test('Escape and Cancel close the make step, leaving the page', async (t) => {
  const { page, fake, close } = await openComms();
  t.after(close);
  // stopped checks that no call to UseWebAppWhenReady starts more than a
  // moment after the dialog closed.
  const stopped = async () => {
    const n = fake.callsTo('UseWebAppWhenReady').length;
    await sleep(1800);
    assert.ok(fake.callsTo('UseWebAppWhenReady').length <= n, 'it stopped asking');
  };
  await openMakeStep(page, fake);
  await fake.waitForCall('UseWebAppWhenReady');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.palette', { state: 'detached' });
  await stopped();

  fake.clearCalls();
  await openMakeStep(page, fake);
  await page.click('[data-key="make-cancel"]');
  await page.waitForSelector('.palette', { state: 'detached' });
  await stopped();
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'menu-p2');
  assert.equal(fake.callsTo('UseWebApp').length, 0);
  // The page is still there, showing its host.
  assert.equal(await page.textContent('.pane[data-pane-id="p2"] .host-chip'), 'outlook.office.com');

  // If Go refuses, it says why, and no dialog opens.
  fake.on('MakeWebApp', () => { throw new Error('Bench needs Accessibility permission to dock web apps.'); });
  await openMenu(page, 'p2');
  await (await webAppEntry(page, 'Make a web app in Safari…')).click();
  await toastIs(page, 'Bench needs Accessibility permission to dock web apps.');
  await settle(page);
  assert.equal(await page.locator('.palette').count(), 0);
});

test('the make step closes if the page goes while it waits', async (t) => {
  const { page, fake, close } = await openComms();
  t.after(close);
  await openMakeStep(page, fake);
  await fake.waitForCall('UseWebAppWhenReady');
  // The page closes, as when the user pops it out and closes its window.
  fake.world.tabs[0].root.children[1].items = [];
  fake.world.tabs[0].root.children[1].active = '';
  delete fake.world.items.w2;
  await fake.emitState();
  await page.waitForSelector('.palette', { state: 'detached' });
  const n = fake.callsTo('UseWebAppWhenReady').length;
  await sleep(1800);
  assert.ok(fake.callsTo('UseWebAppWhenReady').length <= n, 'it stopped asking');
});
