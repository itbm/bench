'use strict';

// Settings' sections, which a list down its side, or a pop-up menu in a
// narrow dialog, chooses one at a time, and its preview of the colours,
// density, and space between panes while it's open (decision 66).

const assert = require('node:assert/strict');
const test = require('node:test');
const {
  openBench, openSettings, settle, showSection,
} = require('./helpers/harness.js');
const { defaultWorld, frames } = require('./helpers/fixture.js');

const focused = (page) => page.evaluate(() => document.activeElement.dataset.key || document.activeElement.id);
const shownSection = (page) => page.$eval('#settings-panel', (el) => el.getAttribute('aria-labelledby').replace('settings-tab-', ''));

test('Settings shows one section at a time, from its list of sections, by click or keyboard', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await openSettings(page);
  // The list is a column of tabs, one stop for the Tab key, which screen
  // readers find with the section each shows.
  const list = await page.$eval('.settings-nav', (el) => ({
    role: el.getAttribute('role'), orientation: el.getAttribute('aria-orientation'), label: el.getAttribute('aria-label'),
  }));
  assert.deepEqual(list, { role: 'tablist', orientation: 'vertical', label: 'Sections' });
  assert.equal(await page.getAttribute('#settings-panel', 'role'), 'tabpanel');
  assert.equal(await page.getAttribute('[data-key="settings-section-general"]', 'aria-controls'), 'settings-panel');
  assert.equal(await focused(page), 'settings-section-general');
  assert.equal(await shownSection(page), 'general');

  // Down Arrow and Up Arrow move along it, wrapping, and Home and End to
  // its ends, each showing its section as the focus moves.
  await page.keyboard.press('ArrowDown');
  assert.equal(await focused(page), 'settings-section-appearance');
  assert.equal(await shownSection(page), 'appearance');
  assert.equal(await page.textContent('#settings-panel h3'), 'Appearance');
  await page.keyboard.press('End');
  assert.equal(await shownSection(page), 'advanced');
  await page.keyboard.press('ArrowDown');
  assert.equal(await shownSection(page), 'general');
  await page.keyboard.press('ArrowUp');
  assert.equal(await shownSection(page), 'advanced');
  await page.keyboard.press('Home');
  assert.equal(await focused(page), 'settings-section-general');
  // Left and Right Arrow, Delete, and the menu keys do nothing here: they
  // are for the tabs in the title bar.
  for (const key of ['ArrowRight', 'ArrowLeft', 'Delete', 'Backspace', 'Shift+F10']) await page.keyboard.press(key);
  assert.equal(await focused(page), 'settings-section-general');
  assert.equal(await page.isVisible('#tab-menu'), false);
  assert.equal(fake.callsTo('CloseTab').length, 0);
  // Tab goes on into the section, and Shift-Tab comes back to the list.
  await page.keyboard.press('End');
  await page.keyboard.press('Tab');
  assert.equal(await focused(page), 'launch-wait');
  await page.keyboard.press('Shift+Tab');
  assert.equal(await focused(page), 'settings-section-advanced');
  // A click shows a section too.
  await page.click('[data-key="settings-section-terminals"]');
  assert.equal(await shownSection(page), 'terminals');
  assert.equal(await page.getAttribute('[data-key="settings-section-terminals"]', 'aria-selected'), 'true');
  assert.equal(await page.getAttribute('[data-key="settings-section-advanced"]', 'aria-selected'), 'false');

  // A section with changes that aren't saved has a dot, which screen
  // readers hear as "changed".
  await page.click('[data-setting="notifyTerminals"]');
  assert.equal(await page.isVisible('[data-key="settings-section-terminals"] .settings-dot'), true);
  assert.equal(await page.textContent('[data-key="settings-section-terminals"]'), 'Terminals, changed');
  assert.equal(await page.$('[data-key="settings-section-general"] .settings-dot'), null);
  await page.click('[data-setting="notifyTerminals"]');
  assert.equal(await page.$('[data-key="settings-section-terminals"] .settings-dot'), null);
  assert.deepEqual(fake.errors, []);
});

test('Settings opens at the section it last showed, after the page loads again too', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await openSettings(page, 'keyboard');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
  await openSettings(page);
  assert.equal(await shownSection(page), 'keyboard');
  assert.equal(await focused(page), 'settings-section-keyboard');
  await page.keyboard.press('Escape');

  await page.reload();
  await page.waitForSelector('.pane');
  await openSettings(page);
  assert.equal(await shownSection(page), 'keyboard');
  await page.keyboard.press('Escape');

  // A page that can't use its storage, as in a private window, remembers
  // the section until it loads again.
  await page.addInitScript(() => {
    Storage.prototype.getItem = () => { throw new Error('denied'); };
    Storage.prototype.setItem = () => { throw new Error('denied'); };
  });
  await page.reload();
  await page.waitForSelector('.pane');
  await openSettings(page);
  assert.equal(await shownSection(page), 'general');
  await showSection(page, 'windows');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
  await openSettings(page);
  assert.equal(await shownSection(page), 'windows');
  assert.deepEqual(fake.errors, []);
});

test('a notice\'s Keyboard settings shows Settings ▸ Keyboard, with the shortcut that doesn\'t work focused', async (t) => {
  const world = defaultWorld();
  world.keyStates = { zoom_pane: { value: 'ctrl+alt+return', problem: 'Another app has this shortcut.' } };
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await openSettings(page, 'appearance');
  await page.click('[data-key="appearance-dark"]');
  await fake.emit('bench:command', { name: 'keyboard-settings' });
  await page.waitForSelector('#settings-panel[aria-labelledby="settings-tab-keyboard"]');
  assert.equal(await focused(page), 'key-zoom_pane');
  // The dialog keeps what it had.
  assert.equal(await page.isVisible('[data-key="settings-section-appearance"] .settings-dot'), true);
});

test('a narrow Settings has a pop-up menu of its sections beside its title, in place of the list', async (t) => {
  // Bench's smallest window, where Settings keeps clear of the traffic
  // lights on both sides.
  const { page, close } = await openBench({ viewport: { width: 640, height: 480 } });
  t.after(close);
  await openSettings(page);
  assert.ok((await page.locator('.settings-sheet').boundingBox()).width < 560);
  assert.equal(await page.isVisible('.settings-nav'), false);
  assert.equal(await page.isVisible('#settings-section-menu'), true);
  assert.equal(await page.getAttribute('#settings-section-menu', 'aria-label'), 'Section');
  assert.equal(await page.inputValue('#settings-section-menu'), 'general');
  await page.selectOption('#settings-section-menu', 'appearance');
  assert.equal(await shownSection(page), 'appearance');
  // It keeps the keyboard, and says which sections have changes.
  assert.equal(await focused(page), 'settings-section-menu');
  await page.click('[data-key="density-compact"]');
  assert.deepEqual(await page.$$eval('#settings-section-menu option', (els) => els.map((el) => el.textContent)), [
    'General', 'Appearance (changed)', 'Windows', 'Keyboard', 'Small screens', 'Terminals', 'Launchers', 'Advanced',
  ]);
  // Tab goes from the menu into the section, and the hidden list isn't a
  // stop.
  await page.focus('#settings-section-menu');
  await page.keyboard.press('Shift+Tab');
  assert.equal(await focused(page), 'settings-save');
  await page.keyboard.press('Tab');
  assert.equal(await focused(page), 'settings-section-menu');
});

test('Settings keeps its list of sections when resized, and for a size from before it had sections', async (t) => {
  // A size remembered from the long Settings, which opened 560 points wide.
  const world = defaultWorld();
  world.dialogs = { settings: { w: 540, h: 820 } };
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await openSettings(page);
  let box = await page.locator('.settings-sheet').boundingBox();
  assert.equal(Math.round(box.width), 560);
  assert.equal(await page.isVisible('.settings-nav'), true);
  assert.equal(await page.isVisible('#settings-section-menu'), false);
  // It can't be made narrower than that, from its corner.
  await page.mouse.move(box.x + box.width - 4, box.y + box.height - 4);
  await page.mouse.down();
  await page.mouse.move(box.x + 300, box.y + 500, { steps: 8 });
  await page.mouse.up();
  await settle(page);
  box = await page.locator('.settings-sheet').boundingBox();
  assert.equal(Math.round(box.width), 560);
  assert.equal(await page.isVisible('.settings-nav'), true);
  assert.deepEqual(fake.callsTo('SetDialogSize').map((c) => c.args), [['settings', 560, Math.round(box.height)]]);
  assert.deepEqual(fake.errors, []);
});

test('Settings is as tall as the window has room for, and a section with more to show has a shadow at that edge', async (t) => {
  const { page, close } = await openBench();
  t.after(close);
  await openSettings(page, 'appearance');
  const box = await page.locator('.settings-sheet').boundingBox();
  assert.ok(box.height > 640 && box.height <= 760, 'Settings is ' + box.height + ' points tall');
  const panel = await page.$eval('#settings-panel', (el) => ({
    scrolls: el.scrollHeight > el.clientHeight, attachment: getComputedStyle(el).backgroundAttachment,
  }));
  assert.ok(panel.scrolls, 'Appearance has more than fits');
  assert.equal(panel.attachment, 'local, local, scroll, scroll');
  // Every section has the same size.
  await showSection(page, 'advanced');
  assert.deepEqual(await page.locator('.settings-sheet').boundingBox(), box);
});

test('typing in a number field marks its section as changed, without a redraw', async (t) => {
  const { page, close } = await openBench();
  t.after(close);
  await openSettings(page, 'screens');
  const field = await page.$('#compact-below');
  await page.fill('#compact-below', '1500');
  assert.equal(await page.isVisible('[data-key="settings-section-screens"] .settings-dot'), true);
  assert.equal(await page.textContent('[data-key="settings-section-screens"]'), 'Small screens, changed');
  assert.equal(await page.$eval('#settings-section-menu option[value="screens"]', (el) => el.textContent), 'Small screens (changed)');
  // The field is the one typed in, with the keyboard still.
  assert.equal(await field.evaluate((el) => el.isConnected && document.activeElement === el), true);
  await page.fill('#compact-below', '');
  await page.keyboard.type(String(await page.evaluate(() => window.bench.store.view.settings.compactBelow)));
  assert.equal(await page.$('[data-key="settings-section-screens"] .settings-dot'), null);
  assert.equal(await page.$eval('#settings-section-menu option[value="screens"]', (el) => el.textContent), 'Small screens');
  // Text size and Wait for apps to open do the same.
  await showSection(page, 'terminals');
  await page.fill('#terminal-font-size', '14');
  await showSection(page, 'advanced');
  await page.fill('#launch-wait', '30');
  assert.deepEqual(await page.$$eval('.settings-dot', (els) => els.map((el) => el.closest('[data-section]').dataset.section)), ['terminals', 'advanced']);
  await page.keyboard.press('Escape');
  await page.waitForSelector('.discard-sheet');
  assert.equal(await page.textContent('#discard-text'), 'Your changes to Terminals and Advanced aren\'t saved.');
});

test('Settings previews the colours, density, and space between panes while it\'s open, and puts them back when its changes are discarded', async (t) => {
  const world = defaultWorld();
  const { page, fake, close } = await openBench({ world, contextOptions: { colorScheme: 'dark' } });
  t.after(close);
  const view = () => page.evaluate(() => window.bench.store.view);
  const theme = () => page.evaluate(() => document.documentElement.dataset.theme);
  const density = () => page.evaluate(() => document.body.dataset.density);
  const before = await view();
  assert.equal(await theme(), 'dark');
  assert.equal(await density(), 'standard');
  await openSettings(page, 'appearance');
  assert.match(await page.textContent('.settings-lead'), /^Bench shows the colours, density, and space between panes that you choose here immediately\./);

  await page.click('[data-key="appearance-light"]');
  await page.waitForFunction(() => document.documentElement.dataset.theme === 'light');
  await page.click('[data-key="density-compact"]');
  await page.waitForFunction(() => document.body.dataset.density === 'compact');
  await page.click('[data-key="gap-8"]');
  await fake.waitForCall('PreviewSettings', (a) => a[0].gap === 8);
  assert.deepEqual(fake.callsTo('PreviewSettings').map((c) => c.args[0]), [
    { appearance: 'light', density: '', gap: -1 },
    { appearance: 'light', density: 'compact', gap: -1 },
    { appearance: 'light', density: 'compact', gap: 8 },
  ]);
  // The panes are drawn as the gap and density would place them, with
  // compact headers; the settings themselves are as they were.
  await page.waitForFunction(() => window.bench.store.view.display.panes[0].header === 32);
  const v = await view();
  assert.deepEqual(v.display.panes.map((p) => p.rect), frames(v.display.root, v.display.area, 8).map((f) => f.rect));
  assert.equal(v.appearance, 'light');
  assert.equal(v.settings.appearance, 'system');
  assert.equal(v.settings.gap, 4);
  const drawn = await page.$eval('.pane', (el) => Math.round(el.getBoundingClientRect().left));
  assert.equal(drawn - Math.round(v.display.area.x), v.display.panes[0].rect.x);

  // The question before discarding keeps the preview, and Discard puts the
  // View back as it was.
  await page.keyboard.press('Escape');
  await page.waitForSelector('.discard-sheet');
  assert.equal(await page.textContent('#discard-text'), 'Your changes to Appearance aren\'t saved.');
  assert.equal(await theme(), 'light');
  await page.click('[data-key="discard-discard"]');
  await page.waitForFunction(() => document.documentElement.dataset.theme === 'dark');
  await page.waitForFunction(() => document.body.dataset.density === 'standard');
  const after = await view();
  assert.deepEqual(after.display, before.display);
  assert.equal(after.appearance, 'system');
  assert.equal(fake.callsTo('SaveSettings').length, 0);

  // Choosing the setting there was previews nothing; Save keeps what's
  // chosen, and ends the preview.
  fake.clearCalls();
  await openSettings(page);
  await page.click('[data-key="gap-4"]');
  await page.click('[data-key="appearance-light"]');
  await page.click('[data-key="appearance-system"]');
  await settle(page);
  assert.deepEqual(fake.callsTo('PreviewSettings').map((c) => c.args[0]), [
    { appearance: 'light', density: '', gap: -1 },
    { appearance: '', density: '', gap: -1 },
  ]);
  await page.click('[data-key="appearance-light"]');
  await page.click('[data-action="save-settings"]');
  assert.equal((await fake.waitForCall('SaveSettings')).args[0].appearance, 'light');
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
  assert.equal(fake.world.preview, null);
  assert.equal(await theme(), 'light');
  assert.deepEqual(fake.errors, []);
});
