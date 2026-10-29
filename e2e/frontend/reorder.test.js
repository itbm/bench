'use strict';

// Putting sub-tabs and tabs in order: a strip of sub-tabs is a drop zone of
// its own, where a drop goes between two sub-tabs and never splits the
// pane, tabs are dragged along the title bar, and Move left and Move right
// do both from the keyboard.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, openTabProperties, settle } = require('./helpers/harness.js');
const { defaultWorld, manyTabs } = require('./helpers/fixture.js');

async function box(page, selector) {
  const b = await page.locator(selector).boundingBox();
  assert.ok(b, selector + ' is on the page');
  return b;
}

// at returns a point in selector's box, at fractions fx and fy of its width
// and height.
async function at(page, selector, fx = 0.5, fy = 0.5) {
  const b = await box(page, selector);
  return { x: b.x + b.width * fx, y: b.y + b.height * fy };
}

// press puts the button down at p.
async function press(page, p) {
  await page.mouse.move(p.x, p.y);
  await page.mouse.down();
}

async function moveTo(page, p) {
  await page.mouse.move(p.x, p.y, { steps: 6 });
}

// order returns the items of a strip's sub-tabs, in order.
function order(page, strip) {
  return page.$$eval(strip + ' .subtab', (els) => els.map((el) => el.dataset.item));
}

function tabNames(page) {
  return page.$$eval('.tab .tab-name', (els) => els.map((el) => el.textContent));
}

const CARET = '.drop-caret:not([hidden])';

test('dragging a sub-tab along its strip shows a caret between sub-tabs, and puts it there', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await settle(page);
  fake.clearCalls();
  assert.deepEqual(await order(page, '.pane[data-pane-id="p4"]'), ['w5', 'w6']);
  await press(page, await at(page, '.subtab[data-item="w5"]'));
  // Past the middle of the next sub-tab: after it.
  await moveTo(page, await at(page, '.subtab[data-item="w6"]', 0.75));
  await page.waitForSelector(CARET + '[data-kind="subtab"]');
  assert.equal(await page.$('.drop-indicator:not([hidden])'), null, 'no split is offered');
  const caret = await box(page, '.drop-caret');
  const w6 = await box(page, '.subtab[data-item="w6"]');
  assert.ok(Math.abs(caret.x + caret.width / 2 - (w6.x + w6.width)) <= 3, 'the caret is after w6: ' + JSON.stringify({ caret, w6 }));
  // Right before or after itself, the drop would change nothing: no caret.
  await moveTo(page, await at(page, '.subtab[data-item="w6"]', 0.25));
  await page.waitForSelector('.drop-caret[hidden]', { state: 'attached' });
  await moveTo(page, await at(page, '.subtab[data-item="w6"]', 0.75));
  await page.waitForSelector(CARET);
  await page.mouse.up();
  await fake.waitForLowered();
  assert.deepEqual(fake.callsTo('MoveItemAt').map((c) => c.args), [['w5', 1, 'p4', 2]]);
  assert.equal(fake.callsTo('MoveItem').length, 0);
  assert.equal(fake.callsTo('ShowItem').length, 0, 'the drop isn\'t a click');
  await page.waitForFunction(() => [...document.querySelectorAll('.pane[data-pane-id="p4"] .subtab')].map((el) => el.dataset.item).join() === 'w6,w5');
  assert.equal(await page.$('.drop-caret'), null);
  // The toast can undo it.
  await page.waitForFunction(() => document.getElementById('toast').textContent.startsWith('Moved Terminal — zsh — ~/code/bench.'));
  await page.click('#toast .toast-action');
  await fake.waitForCall('Undo');
  await page.waitForFunction(() => [...document.querySelectorAll('.pane[data-pane-id="p4"] .subtab')].map((el) => el.dataset.item).join() === 'w5,w6');
});

test('a drop on another pane\'s header goes between its sub-tabs and never splits it', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await settle(page);
  fake.clearCalls();
  // The very top of the header, by the left of w6, where the pane's top
  // edge used to split it: before w6.
  const header = await box(page, '.pane[data-pane-id="p4"] .pane-header');
  const w6 = await box(page, '.subtab[data-item="w6"]');
  await press(page, await at(page, '.subtab[data-item="w3"]'));
  await moveTo(page, { x: w6.x + 4, y: header.y + 2 });
  await page.waitForSelector(CARET + '[data-kind="subtab"]');
  assert.equal(await page.$('.drop-indicator:not([hidden])'), null);
  await page.mouse.up();
  await fake.waitForLowered();
  assert.deepEqual(fake.callsTo('MoveItemAt').map((c) => c.args), [['w3', 1, 'p4', 1]]);
  assert.equal(fake.callsTo('MoveItem').length, 0);

  // The header's buttons, after the strip: after the last sub-tab.
  await page.waitForFunction(() => document.querySelectorAll('.pane[data-pane-id="p4"] .subtab').length === 3);
  await settle(page);
  fake.clearCalls();
  await press(page, await at(page, '.subtab[data-item="w4"]'));
  await moveTo(page, await at(page, '.pane[data-pane-id="p4"] [data-action="menu"]'));
  await page.waitForSelector(CARET);
  await page.mouse.up();
  await fake.waitForLowered();
  assert.deepEqual(fake.callsTo('MoveItemAt').map((c) => c.args), [['w4', 1, 'p4', 3]]);
});

test('the edges of a pane\'s body still split it, and the indicator stays on the body', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await settle(page);
  fake.clearCalls();
  const body = await box(page, '.pane[data-pane-id="p4"] .pane-body');
  await press(page, await at(page, '.subtab[data-item="w3"]'));
  // Just under the header: the body's top edge.
  await moveTo(page, { x: body.x + body.width / 2, y: body.y + 10 });
  await page.waitForSelector('.pane[data-pane-id="p4"] .drop-indicator[data-zone="top"]:not([hidden])');
  assert.equal(await page.$(CARET), null);
  const ind = await box(page, '.pane[data-pane-id="p4"] .drop-indicator');
  assert.ok(ind.y >= body.y, 'the indicator is on the body: ' + JSON.stringify({ ind, body }));
  assert.ok(Math.abs(ind.height - (body.height / 2 - 9)) <= 1, 'it covers the top half of the body');
  await page.mouse.up();
  await fake.waitForLowered();
  assert.deepEqual(fake.callsTo('MoveItem').map((c) => c.args), [['w3', 1, 'p4', 'top']]);
  assert.equal(fake.callsTo('MoveItemAt').length, 0);
});

test('in the "tabs" view, a sub-tab dragged along the title bar\'s strip goes between sub-tabs', async (t) => {
  // Research, on a laptop's screen: one pane, its four windows in the
  // title bar.
  const { page, fake, close } = await openBench({ world: defaultWorld({ screenWidth: 1512, current: 2 }) });
  t.after(close);
  await settle(page);
  fake.clearCalls();
  assert.deepEqual(await order(page, '.title-pane'), ['w7', 'w8', 'w9', 'w10']);
  await press(page, await at(page, '.title-pane .subtab[data-item="w7"]'));
  await moveTo(page, await at(page, '.title-pane .subtab[data-item="w9"]', 0.8));
  await page.waitForSelector(CARET + '[data-kind="subtab"]');
  const caret = await box(page, '.drop-caret');
  const bar = await box(page, '#titlebar');
  assert.ok(caret.y >= bar.y && caret.y + caret.height <= bar.y + bar.height, 'the caret is in the title bar');
  await page.mouse.up();
  await fake.waitForLowered();
  assert.deepEqual(fake.callsTo('MoveItemAt').map((c) => c.args), [['w7', 2, 'compact', 3]]);
  await page.waitForFunction(() => [...document.querySelectorAll('.title-pane .subtab')].map((el) => el.dataset.item).join() === 'w8,w9,w7,w10');
});

test('dragging a tab along the title bar moves it, with a caret of its own', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await settle(page);
  fake.clearCalls();
  assert.deepEqual(await tabNames(page), ['Comms', 'Code', 'Research']);
  await press(page, await at(page, '.tab[data-index="0"]'));
  // Less than the drag threshold: still a press.
  const start = await at(page, '.tab[data-index="0"]');
  await page.mouse.move(start.x + 2, start.y);
  assert.equal(await page.$('.drag-ghost'), null);
  // Over the next tab's first half, the drop would change nothing.
  await moveTo(page, await at(page, '.tab[data-index="1"]', 0.3));
  await page.waitForSelector('.drag-ghost:has-text("Comms")');
  assert.equal(await page.evaluate(() => document.body.classList.contains('dragging-tab')), true);
  // No × shows while a tab is dragged, not even the current tab's or the
  // shown sub-tab's, and the current tab shows its shortcut instead.
  const shows = await page.evaluate(() => {
    const vis = (sel) => getComputedStyle(document.querySelector(sel)).visibility;
    return [vis('.tab[aria-pressed="true"] .close-cross'), vis('.subtab[aria-pressed="true"] .close-cross'), vis('.tab[aria-pressed="true"] kbd')];
  });
  assert.deepEqual(shows, ['hidden', 'hidden', 'visible']);
  assert.equal(await page.$(CARET), null);
  // Past the middle of the last tab: after it.
  await moveTo(page, await at(page, '.tab[data-index="2"]', 0.8));
  await page.waitForSelector(CARET + '[data-kind="tab"]');
  // It isn't a drop on the tab, which a sub-tab's would be.
  assert.equal(await page.$('.tab.drop-target'), null);
  const caret = await box(page, '.drop-caret');
  const last = await box(page, '.tab[data-index="2"]');
  assert.ok(Math.abs(caret.x + caret.width / 2 - (last.x + last.width)) <= 3, 'the caret is after the last tab');
  await page.mouse.up();
  await fake.waitForLowered();
  assert.deepEqual(fake.callsTo('MoveTab').map((c) => c.args), [[0, 2]]);
  // The drop isn't a click on the tab.
  await settle(page);
  assert.equal(fake.callsTo('SwitchTab').length, 0);
  await page.waitForFunction(() => [...document.querySelectorAll('.tab .tab-name')].map((el) => el.textContent).join() === 'Code,Research,Comms');
  // Code is still the current tab, and the shortcuts follow the order.
  assert.equal(await page.getAttribute('.tab[data-index="0"]', 'aria-pressed'), 'true');
  assert.equal(await page.getAttribute('.tab[data-index="2"]', 'title'), 'Comms (⌃⌥3)');
  await page.waitForFunction(() => document.getElementById('toast').textContent.startsWith('Moved Comms to 3rd place (⌃⌥3).'));
  await page.click('#toast .toast-action');
  await fake.waitForCall('Undo');
  await page.waitForFunction(() => [...document.querySelectorAll('.tab .tab-name')].map((el) => el.textContent).join() === 'Comms,Code,Research');

  // A drop outside the title bar leaves the tab where it is.
  fake.clearCalls();
  await press(page, await at(page, '.tab[data-index="2"]'));
  await moveTo(page, await at(page, '.tab[data-index="0"]', 0.2));
  await page.waitForSelector(CARET);
  await moveTo(page, await at(page, '.pane[data-pane-id="p3"]'));
  await page.waitForSelector('.drop-caret[hidden]', { state: 'attached' });
  await page.mouse.up();
  await fake.waitForLowered();
  assert.equal(fake.callsTo('MoveTab').length, 0);
});

test('a tab drag starts only from a press on a tab, not its × or the title bar around it', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await settle(page);
  fake.clearCalls();
  const target = await at(page, '.tab[data-index="2"]', 0.8);
  // The current tab's × shows.
  await press(page, await at(page, '.tab[data-index="1"] .close-cross'));
  await moveTo(page, target);
  assert.equal(await page.$('.drag-ghost'), null);
  assert.equal(await page.$('.drop-caret'), null);
  await page.mouse.up();
  // The title bar's own space, which moves the window in the app.
  const plus = await box(page, '.new-tab');
  await press(page, { x: plus.x + plus.width + 40, y: plus.y + plus.height / 2 });
  await moveTo(page, target);
  assert.equal(await page.$('.drag-ghost'), null);
  await page.mouse.up();
  await settle(page);
  for (const method of ['MoveTab', 'CloseTab', 'SetOverlay']) assert.equal(fake.callsTo(method).length, 0, method);

  // With one tab, there's nothing to put it among.
  const world = defaultWorld({ current: 0 });
  world.tabs.splice(1);
  const one = await openBench({ world });
  t.after(one.close);
  await settle(one.page);
  await press(one.page, await at(one.page, '.tab[data-index="0"]'));
  await moveTo(one.page, await at(one.page, '.new-tab'));
  assert.equal(await one.page.$('.drag-ghost'), null);
  await one.page.mouse.up();
});

test('Escape cancels a tab drag', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await settle(page);
  fake.clearCalls();
  await press(page, await at(page, '.tab[data-index="0"]'));
  await moveTo(page, await at(page, '.tab[data-index="2"]', 0.8));
  await page.waitForSelector(CARET);
  await page.keyboard.press('Escape');
  await fake.waitForLowered();
  assert.equal(await page.$('.drop-caret'), null);
  assert.equal(await page.$('.drag-ghost'), null);
  assert.equal(await page.$('.tab.dragging'), null);
  await page.mouse.up();
  await settle(page);
  assert.equal(fake.callsTo('MoveTab').length, 0);
  assert.equal(fake.callsTo('SwitchTab').length, 0, 'the release isn\'t a click');
});

test('a drag near the end of the tab strip scrolls it, to put a tab among those out of view', async (t) => {
  const { page, fake, close } = await openBench({ world: manyTabs(defaultWorld({ current: 0 }), 20), viewport: { width: 1000, height: 700 } });
  t.after(close);
  await settle(page);
  fake.clearCalls();
  const strip = await box(page, '#titlebar .tabs');
  await press(page, await at(page, '.tab[data-index="0"]'));
  await moveTo(page, { x: strip.x + strip.width - 4, y: strip.y + strip.height / 2 });
  await page.waitForFunction(() => document.querySelector('#titlebar .tabs').scrollLeft > 100);
  await page.waitForSelector(CARET + '[data-kind="tab"]');
  await page.mouse.up();
  await fake.waitForLowered();
  const [call] = fake.callsTo('MoveTab');
  assert.ok(call && call.args[0] === 0 && call.args[1] > 3, 'moved the first tab far along: ' + JSON.stringify(call && call.args));
});

test('Move left and Move right in the tab menu and the window menu work from the keyboard', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await settle(page);

  // The first tab can only move right.
  await page.click('.tab[data-index="0"]', { button: 'right' });
  await page.waitForSelector('#tab-menu');
  const entries = await page.$$eval('#tab-menu > *', (els) => els.map((el) => el.className + ':' + el.textContent + (el.disabled ? ' (off)' : '')));
  assert.deepEqual(entries.slice(0, 6), [
    'menu-item:Tab properties…', 'menu-item:Rename…', 'menu-sep:',
    'menu-item:Move left (off)', 'menu-item:Move right', 'menu-sep:',
  ]);
  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('ArrowDown');
  assert.equal(await page.evaluate(() => document.activeElement.textContent), 'Move right');
  fake.clearCalls();
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('MoveTab')).args, [0, 1]);
  await page.waitForSelector('#tab-menu', { state: 'detached' });
  await page.waitForFunction(() => document.querySelector('.tab[data-index="0"] .tab-name').textContent === 'Code');
  // The keyboard goes back to the tab, in its new place.
  await settle(page);
  assert.equal(await page.evaluate(() => document.activeElement.dataset.key), 'tab-1');
  assert.equal(await page.textContent('.tab[data-index="1"] .tab-name'), 'Comms');

  // The last tab can only move left.
  await page.click('.tab[data-index="2"]', { button: 'right' });
  await page.waitForSelector('#tab-menu');
  assert.equal(await page.$eval('#tab-menu .menu-item:has-text("Move right")', (el) => el.disabled), true);
  await page.click('#tab-menu .menu-item:has-text("Move left")');
  assert.deepEqual((await fake.waitForCall('MoveTab', (a) => a[0] === 2)).args, [2, 1]);

  // A window's menu moves its sub-tab along its strip.
  await page.waitForSelector('#tab-menu', { state: 'detached' });
  await settle(page);
  fake.clearCalls();
  const code = await page.evaluate(() => window.bench.store.view.current);
  await page.click('.pane[data-pane-id="p4"] [data-action="menu"]');
  await page.waitForSelector('#window-menu');
  const window = await page.$$eval('#window-menu .menu-item', (els) => els.map((el) => el.textContent + (el.disabled ? ' (off)' : '')));
  assert.deepEqual(window.slice(5, 7), ['Move left (off)', 'Move right']);
  // A disabled move isn't explained by a note meant for other entries.
  assert.equal(await page.$('#window-menu .menu-item:has-text("Move left")[aria-describedby]'), null);
  await page.click('#window-menu .menu-item:has-text("Move right")');
  assert.deepEqual((await fake.waitForCall('MoveItemAt')).args, ['w5', code, 'p4', 2]);
  await page.waitForFunction(() => [...document.querySelectorAll('.pane[data-pane-id="p4"] .subtab')].map((el) => el.dataset.item).join() === 'w6,w5');
  // w5 is now last, and shown.
  await page.click('.pane[data-pane-id="p4"] [data-action="menu"]');
  await page.waitForSelector('#window-menu');
  assert.equal(await page.$eval('#window-menu .menu-item:has-text("Move right")', (el) => el.disabled), true);
  await page.click('#window-menu .menu-item:has-text("Move left")');
  assert.deepEqual((await fake.waitForCall('MoveItemAt', (a) => a[3] === 0)).args, ['w5', code, 'p4', 0]);
});

test('Jump to… moves the current tab left and right', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.click('[data-action="open-palette"]');
  await page.fill('#palette-query', 'move tab');
  const rows = await page.$$eval('.palette-item .palette-label', (els) => els.map((el) => el.textContent));
  assert.deepEqual(rows, ['Move tab left', 'Move tab right']);
  await page.click('.palette-item:has-text("Move tab left")');
  assert.deepEqual((await fake.waitForCall('MoveTab')).args, [1, 0]);
  await page.waitForFunction(() => document.querySelector('.tab[aria-pressed="true"] .tab-name').textContent === 'Code'
    && document.querySelector('.tab[data-index="0"] .tab-name').textContent === 'Code');
  // Code is now first, so it can only move right.
  await page.click('[data-action="open-palette"]');
  await page.fill('#palette-query', 'move tab');
  assert.deepEqual(await page.$$eval('.palette-item .palette-label', (els) => els.map((el) => el.textContent)), ['Move tab right']);
});

test('Tab properties\' Position moves the tab, naming each place\'s shortcut', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await openTabProperties(page);
  const options = () => page.$$eval('#tab-place option', (els) => els.map((el) => el.textContent + (el.selected ? ' *' : '')));
  assert.deepEqual(await options(), ['1st (⌃⌥1)', '2nd (⌃⌥2) *', '3rd (⌃⌥3)']);
  assert.equal(await page.textContent('#tab-place-note'), '');
  assert.equal(await page.textContent('label[for="tab-place"]'), 'Position');
  await page.selectOption('#tab-place', '1');
  assert.deepEqual(await options(), ['1st (⌃⌥1) *', '2nd (⌃⌥2)', '3rd (⌃⌥3)']);
  await page.click('[data-action="save-tab"]');
  assert.deepEqual((await fake.waitForCall('SaveTab')).args, [1, { name: 'Code', preset: 'main-side', smallScreens: 'stack', place: 1 }]);
  await page.waitForSelector('.editor-sheet', { state: 'detached' });

  // A tab left where it is saves with no place.
  fake.clearCalls();
  await openTabProperties(page, 2);
  await page.click('[data-action="save-tab"]');
  assert.deepEqual((await fake.waitForCall('SaveTab')).args, [2, { name: 'Research', preset: 'side', smallScreens: 'tabs' }]);
  await page.waitForSelector('.editor-sheet', { state: 'detached' });

  // A new tab goes last, unless another place is chosen.
  await page.click('[data-action="new-tab"]');
  await page.waitForSelector('.editor-sheet');
  assert.deepEqual(await options(), ['1st (⌃⌥1)', '2nd (⌃⌥2)', '3rd (⌃⌥3)', '4th (⌃⌥4) *']);
});

test('Tab properties says how to reach a tab whose place has no shortcut', async (t) => {
  const { page, close } = await openBench({ world: manyTabs(defaultWorld({ current: 0 }), 10) });
  t.after(close);
  await openTabProperties(page, 9);
  assert.equal(await page.$eval('#tab-place', (el) => el.selectedOptions[0].textContent), '10th');
  assert.equal(await page.textContent('#tab-place-note'), 'Only the first nine tabs have shortcuts. Use ⌃⌥[ ] or Jump to… for this one.');
  await page.selectOption('#tab-place', '9');
  assert.equal(await page.textContent('#tab-place-note'), '');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.editor-sheet', { state: 'detached' });

  const off = defaultWorld({ keys: { tabs: '' } });
  const other = await openBench({ world: off });
  t.after(other.close);
  await openTabProperties(other.page);
  assert.deepEqual(await other.page.$$eval('#tab-place option', (els) => els.map((el) => el.textContent)), ['1st', '2nd', '3rd']);
  assert.equal(await other.page.textContent('#tab-place-note'), 'Tabs have no shortcuts, because tab_modifiers is empty in config.toml.');
});
