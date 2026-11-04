'use strict';

// A web page's controls: the Page group of its ⋯ and sub-tab menus, the
// jump palette's rows for it, Pane ▸ Page's commands, the find bar, Copy
// page link, and the marks of a page that uses the camera or the
// microphone.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench } = require('./helpers/harness.js');
const { defaultWorld } = require('./helpers/fixture.js');

// toastIs waits for the toast to show text.
const toastIs = (page, text) => page.waitForFunction((t) => {
  const el = document.getElementById('toast');
  return el.classList.contains('show') && (el.querySelector('.toast-text') || el).textContent === t;
}, text);

// pageGroup lists the Page group of the open window menu: the buttons of
// its row, by their labels, then its entries, up to the separator after
// them, each with " (off)" if it's disabled.
const pageGroup = (page) => page.evaluate(() => {
  const row = document.querySelector('#window-menu .menu-row');
  if (!row) return [];
  const name = (el) => {
    if (el.classList.contains('menu-icon')) return el.getAttribute('aria-label');
    const main = el.querySelector('.menu-main') || el.querySelector('.menu-label');
    const detail = el.querySelector('.menu-detail');
    return main.textContent + (detail ? ' — ' + detail.textContent : '');
  };
  const out = Array.from(row.querySelectorAll('.menu-item'), (el) => name(el) + (el.disabled ? ' (off)' : ''));
  for (let el = row.nextElementSibling; el && el.classList.contains('menu-item'); el = el.nextElementSibling) {
    out.push(name(el) + (el.disabled ? ' (off)' : ''));
  }
  return out;
});

// openMenu opens the ⋯ menu of a pane's shown window.
async function openMenu(page, paneId) {
  await page.click('.pane[data-pane-id="' + paneId + '"] [data-action="menu"]');
  await page.waitForSelector('#window-menu');
}

// runEntry chooses an entry of the open window menu by its label, or a
// button of its Page row by its tooltip.
async function runEntry(page, label) {
  await page.click('#window-menu .menu-item:has(.menu-label:text-is("' + label + '")), #window-menu .menu-icon[title="' + label + '"]');
}

test('a web page\'s menu has a Page group, with Back and Forward off while the page can\'t go there', async (t) => {
  const { page, fake, close } = await openBench({ world: defaultWorld({ current: 0 }) });
  t.after(close);
  await openMenu(page, 'p1');
  assert.deepEqual(await pageGroup(page), [
    'Back (off)', 'Forward (off)', 'Zoom Out', 'Actual Size (now 100%) (off)', 'Zoom In', 'Find in Page…', 'Copy Page Link',
    'Stop Sound and Video',
  ]);
  assert.equal(await page.textContent('#window-menu .menu-row-label'), 'Page');
  assert.equal(await page.getAttribute('#window-menu .menu-row', 'aria-label'), 'Page');
  // Reload Page is the first of the menu's common actions, at its top.
  assert.equal(await page.textContent('#window-menu .menu-item .menu-label'), 'Reload Page');
  await runEntry(page, 'Reload Page');
  assert.deepEqual((await fake.waitForCall('ReloadPage')).args, ['w1']);

  // The page has been somewhere, and is zoomed in.
  fake.world.items.w1.canGoBack = true;
  fake.world.items.w1.zoom = 125;
  await fake.emitState();
  await openMenu(page, 'p1');
  assert.deepEqual(await pageGroup(page), [
    'Back', 'Forward (off)', 'Zoom Out', 'Actual Size (now 125%)', 'Zoom In', 'Find in Page…', 'Copy Page Link',
    'Stop Sound and Video',
  ]);
  // The zoom shows on Actual Size.
  assert.equal(await page.textContent('#window-menu .menu-icon[title="Actual Size"]'), '125%');
  await runEntry(page, 'Back');
  assert.deepEqual((await fake.waitForCall('GoBack')).args, ['w1']);
  await openMenu(page, 'p1');
  await runEntry(page, 'Zoom In');
  assert.deepEqual((await fake.waitForCall('ZoomPage')).args, ['w1', 1]);
  await toastIs(page, 'Zoomed Slack to 150%.');

  // All the way in, Zoom In is off.
  fake.world.items.w1.zoom = 300;
  await fake.emitState();
  await openMenu(page, 'p1');
  const group = await pageGroup(page);
  assert.ok(group.includes('Zoom In (off)') && group.includes('Actual Size (now 300%)'), JSON.stringify(group));
  await runEntry(page, 'Actual Size');
  assert.deepEqual(fake.callsTo('ZoomPage').at(-1).args, ['w1', 0]);
  await page.keyboard.press('Escape');

  // The arrow keys move along the row.
  await openMenu(page, 'p1');
  await page.focus('#window-menu .menu-icon[title="Zoom Out"]');
  await page.keyboard.press('ArrowLeft');
  assert.equal(await page.evaluate(() => document.activeElement.title), 'Back');
  await page.keyboard.press('ArrowRight');
  assert.equal(await page.evaluate(() => document.activeElement.title), 'Zoom Out');
  await page.keyboard.press('Escape');

  // An app's window has no Page group.
  fake.world.current = 1;
  await fake.emitState();
  await page.waitForSelector('.pane[data-pane-id="p3"]');
  await openMenu(page, 'p3');
  assert.equal(await page.$('#window-menu .menu-row'), null);
});

test('Copy Page Link puts the page\'s address on the clipboard, but VS Code for the Web has none', async (t) => {
  const world = defaultWorld({ current: 0 });
  world.items.w2.folder = '~/code/bench';
  world.items.w2.app = 'VS Code';
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await openMenu(page, 'p1');
  await runEntry(page, 'Copy Page Link');
  assert.deepEqual((await fake.waitForCall('PageLink')).args, ['w1']);
  await toastIs(page, 'Copied the page\'s link.');
  assert.equal(fake.clipboard, 'https://app.slack.com/client');

  await openMenu(page, 'p2');
  const group = await pageGroup(page);
  assert.ok(!group.some((e) => e.startsWith('Copy Page Link')), JSON.stringify(group));
  assert.ok(group.includes('Find in Page…'), JSON.stringify(group));
  assert.equal(await page.textContent('#window-menu .menu-item .menu-label'), 'Reload Page');
});

test('the find bar finds in the page, at the top of its pane, as tall as the room Go leaves', async (t) => {
  const world = defaultWorld({ current: 0 });
  world.items.w1.pageText = 'Huddle in #general at 10:00';
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await openMenu(page, 'p1');
  await runEntry(page, 'Find in Page…');
  assert.deepEqual((await fake.waitForCall('SetFindBar')).args, ['w1', true]);

  // The bar opens with the keyboard in its field.
  const input = '.pane[data-pane-id="p1"] .page-find-input';
  await page.waitForFunction((sel) => document.activeElement === document.querySelector(sel), input);
  const box = await page.$eval('.pane[data-pane-id="p1"] .page-find', (el) => {
    const r = el.getBoundingClientRect();
    const body = el.closest('.pane-body').getBoundingClientRect();
    return { h: r.height, top: r.top - body.top };
  });
  assert.deepEqual(box, { h: 36, top: 0 }, 'layout.FindBarHeight is 36');
  assert.equal(await page.$('.pane[data-pane-id="p2"] .page-find'), null);

  // Return finds the next match, Shift-Return the one before.
  await page.keyboard.type('HUDDLE');
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('FindInPage')).args, ['w1', 'HUDDLE', false]);
  assert.equal(await page.textContent('.page-find-count'), '');
  await page.fill(input, 'standup');
  await page.keyboard.press('Shift+Enter');
  assert.deepEqual((await fake.waitForCall('FindInPage', (a) => a[1] === 'standup')).args, ['w1', 'standup', true]);
  await page.waitForFunction(() => document.querySelector('.page-find-count').textContent === 'No matches');

  // Its arrows find too; typing takes the note away.
  await page.fill(input, 'general');
  await page.waitForFunction(() => document.querySelector('.page-find-count').textContent === '');
  await page.click('.pane[data-pane-id="p1"] [data-action="find-previous"]');
  assert.deepEqual((await fake.waitForCall('FindInPage', (a) => a[1] === 'general')).args, ['w1', 'general', true]);

  // What's typed stays as the view is drawn again.
  await fake.emitState();
  assert.equal(await page.inputValue(input), 'general');

  // Escape closes it.
  await page.focus(input);
  await page.keyboard.press('Escape');
  assert.deepEqual((await fake.waitForCall('SetFindBar', (a) => a[1] === false)).args, ['w1', false]);
  await page.waitForSelector('.page-find', { state: 'detached' });

  // Opened again, it has what it had, selected.
  await openMenu(page, 'p1');
  await runEntry(page, 'Find in Page…');
  await page.waitForFunction((sel) => document.activeElement === document.querySelector(sel), input);
  assert.equal(await page.inputValue(input), 'general');
  assert.equal(await page.evaluate(() => [document.activeElement.selectionStart, document.activeElement.selectionEnd].join()), '0,7');
  await page.click('.pane[data-pane-id="p1"] [data-action="close-find"]');
  await page.waitForSelector('.page-find', { state: 'detached' });
});

test('a page that uses the camera or the microphone has marks on its sub-tab and tab, and its menu turns them off', async (t) => {
  const world = defaultWorld({ current: 1 });
  world.items.w1.camera = 'active';
  world.items.w1.microphone = 'muted';
  const { page, fake, close } = await openBench({ world });
  t.after(close);

  // Comms isn't shown: its tab has one mark, which says what's on.
  const tab = '.tab[data-index="0"]';
  await page.waitForSelector(tab + ' .tab-capture');
  const text = 'A page here is using the camera, with the microphone turned off';
  assert.equal(await page.getAttribute(tab + ' .tab-capture', 'title'), text);
  assert.equal(await page.getAttribute(tab + ' .tab-capture', 'data-capture'), 'live');
  assert.equal(await page.getAttribute(tab + ' .tab-capture', 'data-device'), 'camera');
  assert.ok((await page.getAttribute(tab, 'title')).split('\n').includes(text));
  assert.equal(await page.textContent(tab + ' .visually-hidden'), ', ' + text);
  assert.equal(await page.$('.tab[data-index="1"] .tab-capture'), null);

  // In the tab, the sub-tab has a mark of each, before its title.
  fake.world.current = 0;
  await fake.emitState();
  const sub = '.subtab[data-item="w1"]';
  await page.waitForSelector(sub + ' .capture-mark');
  const marks = await page.$$eval(sub + ' .capture-mark', (els) => els.map((el) => el.dataset.capture + ' ' + el.title));
  assert.deepEqual(marks, ['live Using the camera', 'muted Microphone muted']);
  assert.ok(await page.$eval(sub + ' .capture-mark', (el) => el.nextElementSibling.classList.contains('capture-mark')
    && el.previousElementSibling.classList.contains('mono-tile')));
  assert.equal(await page.getAttribute(sub, 'aria-label'), 'Slack — # general (using the camera, microphone muted)');
  assert.ok((await page.getAttribute(sub, 'title')).split('\n').includes('Using the camera'));

  // The menu turns the camera off and the microphone back on, and stops
  // the page's sound and video.
  await openMenu(page, 'p1');
  const group = await pageGroup(page);
  assert.deepEqual(group.slice(-3), ['Unmute Microphone', 'Turn Off Camera', 'Stop Sound and Video']);
  await runEntry(page, 'Turn Off Camera');
  assert.deepEqual((await fake.waitForCall('MuteCapture')).args, ['w1', 'camera', true]);
  await toastIs(page, 'Turned off Slack\'s camera.');
  await page.waitForFunction((sel) => document.querySelector(sel + ' .capture-mark').dataset.capture === 'muted', sub);
  // All off, the tab's mark is grey.
  fake.world.current = 1;
  await fake.emitState();
  await page.waitForFunction((sel) => {
    const el = document.querySelector(sel + ' .tab-capture');
    return el && el.dataset.capture === 'muted';
  }, tab);
  assert.equal(await page.getAttribute(tab + ' .tab-capture', 'title'), 'A page here has the camera and the microphone turned off');
  fake.world.current = 0;
  await fake.emitState();
  await openMenu(page, 'p1');
  await runEntry(page, 'Unmute Microphone');
  assert.deepEqual(fake.callsTo('MuteCapture').at(-1).args, ['w1', 'microphone', false]);
  await openMenu(page, 'p1');
  await runEntry(page, 'Stop Sound and Video');
  assert.deepEqual((await fake.waitForCall('StopMedia')).args, ['w1']);
  await toastIs(page, 'Paused Slack\'s sound and video.');
});

test('a tab whose page has the camera turned off and the microphone live shows the microphone, live', async (t) => {
  const world = defaultWorld({ current: 1 });
  world.items.w1.camera = 'muted';
  world.items.w1.microphone = 'active';
  const { page, close } = await openBench({ world });
  t.after(close);
  const mark = '.tab[data-index="0"] .tab-capture';
  await page.waitForSelector(mark);
  assert.equal(await page.getAttribute(mark, 'data-capture'), 'live');
  assert.equal(await page.getAttribute(mark, 'data-device'), 'microphone');
  const text = 'A page here is using the microphone, with the camera turned off';
  assert.equal(await page.getAttribute(mark, 'title'), text);
  assert.equal(await page.textContent('.tab[data-index="0"] .visually-hidden'), ', ' + text);
});

test('Jump to… lists a page\'s controls under This window, and Pane ▸ Page\'s commands act on the page', async (t) => {
  const world = defaultWorld({ current: 0 });
  world.items.w1.microphone = 'active';
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await page.click('[data-action="open-palette"]');
  await page.waitForSelector('#palette-query');
  await page.fill('#palette-query', 'page');
  const labels = await page.$$eval('#overlay-root .palette-item .palette-label', (els) => els.map((el) => el.textContent));
  for (const label of ['Reload Page', 'Page ▸ Zoom In', 'Page ▸ Find in Page…', 'Page ▸ Copy Page Link', 'Page ▸ Mute Microphone',
    'Page ▸ Stop Sound and Video']) {
    assert.ok(labels.includes(label), label + ' in ' + JSON.stringify(labels));
  }
  // What's off is left out.
  assert.ok(!labels.includes('Page ▸ Back') && !labels.includes('Page ▸ Actual Size'), JSON.stringify(labels));
  await page.fill('#palette-query', 'mute microphone');
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('MuteCapture')).args, ['w1', 'microphone', true]);
  await page.waitForSelector('.palette', { state: 'detached' });

  // Find in Page from the palette gives the bar the keyboard.
  await page.click('[data-action="open-palette"]');
  await page.waitForSelector('#palette-query');
  await page.fill('#palette-query', 'find in page');
  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('SetFindBar')).args, ['w1', true]);
  await page.waitForFunction(() => document.activeElement && document.activeElement.classList.contains('page-find-input'));
  await page.keyboard.press('Escape');
  await page.waitForSelector('.page-find', { state: 'detached' });

  // Pane ▸ Page ▸ Find in Page… and Copy Page Link, for the item Go names.
  await fake.emit('bench:command', { name: 'find-in-page', itemId: 'w2' });
  assert.deepEqual((await fake.waitForCall('SetFindBar', (a) => a[0] === 'w2')).args, ['w2', true]);
  await page.waitForFunction(() => document.activeElement === document.querySelector('.pane[data-pane-id="p2"] .page-find-input'));
  await fake.emit('bench:command', { name: 'copy-page-link', itemId: 'w2' });
  assert.deepEqual((await fake.waitForCall('PageLink', (a) => a[0] === 'w2')).args, ['w2']);
  await toastIs(page, 'Copied the page\'s link.');
  assert.equal(fake.clipboard, 'https://outlook.office.com/mail/');
});
