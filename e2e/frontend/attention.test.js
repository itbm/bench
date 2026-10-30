'use strict';

// The bell on the sub-tab of a window that needs you, as a terminal that
// rang its bell or a page that posted a notification while you couldn't
// see it, and on its tab; and a terminal's New terminal in rows.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench } = require('./helpers/harness.js');
const { defaultWorld } = require('./helpers/fixture.js');

// ariaLabel returns an element's aria-label, or null if it has none.
const ariaLabel = (page, sel) => page.getAttribute(sel, 'aria-label');

test('a window that needs you has a bell on its sub-tab, and its tab one that counts them, which say why', async (t) => {
  const world = defaultWorld({ current: 0 });
  world.items.w5.attention = 'Rang the bell';
  world.items.w6.attention = 'Notification: Claude Code: Claude needs your permission';
  const { page, fake, close } = await openBench({ world });
  t.after(close);

  // The Code tab, which isn't shown, has a bell with the count, which its
  // tooltip says, and a screen reader reads with its name.
  const tab = '.tab[data-index="1"]';
  await page.waitForSelector(tab + ' .tab-bell');
  assert.equal(await page.getAttribute(tab + ' .tab-bell', 'title'), '2 windows need you');
  assert.equal(await page.getAttribute(tab + ' .tab-bell', 'aria-hidden'), 'true');
  assert.equal(await page.textContent(tab + ' .bell-count'), '2');
  assert.ok((await page.getAttribute(tab, 'title')).endsWith('\n2 windows need you'));
  assert.equal(await page.textContent(tab + ' .visually-hidden'), ', 2 windows need you');
  assert.equal(await page.$('.tab[data-index="0"] .tab-bell'), null);
  assert.equal(await page.$('.tab[data-index="2"] .tab-bell'), null);

  // One window has no count.
  delete fake.world.items.w5.attention;
  await fake.emitState();
  await page.waitForFunction((sel) => !document.querySelector(sel + ' .bell-count'), tab);
  assert.equal(await page.getAttribute(tab + ' .tab-bell', 'title'), '1 window needs you');
  assert.equal(await page.textContent(tab + ' .visually-hidden'), ', 1 window needs you');

  // In the tab, the sub-tab has the bell, apart from the marker of a window
  // that isn't ready, and says why in its tooltip and its label.
  fake.world.current = 1;
  await fake.emitState();
  const sub = '.subtab[data-item="w6"]';
  await page.waitForSelector(sub + ' .subtab-bell');
  const why = 'Notification: Claude Code: Claude needs your permission.';
  assert.equal(await page.getAttribute(sub + ' .subtab-bell', 'title'), why);
  assert.equal(await page.$(sub + ' .status-mark'), null);
  assert.ok((await page.getAttribute(sub, 'title')).split('\n').includes(why));
  assert.equal(await ariaLabel(page, sub), 'Terminal — go test — ~/code/bench (needs you)');
  assert.equal(await page.$('.subtab[data-item="w5"] .subtab-bell'), null);

  // Shown, it loses its mark, as Go takes it off.
  delete fake.world.items.w6.attention;
  await fake.emitState();
  await page.waitForFunction(() => !document.querySelector('.bell-mark'));
  assert.equal(await ariaLabel(page, sub), 'Terminal — go test — ~/code/bench');
});

test('a sub-tab\'s bell is before its title, where its × never covers it, shown or hovered', async (t) => {
  const world = defaultWorld();
  world.items.w5.attention = 'Its command has finished';
  world.items.w6.attention = 'Rang the bell';
  const { page, close } = await openBench({ world });
  t.after(close);
  // w5 is the sub-tab p4 shows, whose × always shows; w6's shows on hover.
  for (const id of ['w5', 'w6']) {
    const sub = '.subtab[data-item="' + id + '"]';
    await page.hover(sub + ' .subtab-title');
    await page.waitForFunction((sel) => getComputedStyle(document.querySelector(sel + ' .close-cross')).visibility === 'visible', sub);
    const r = await page.$eval(sub, (el) => {
      const box = (n) => n.getBoundingClientRect();
      const bell = box(el.querySelector('.subtab-bell'));
      const hit = document.elementFromPoint(bell.x + bell.width / 2, bell.y + bell.height / 2);
      return { before: bell.right <= box(el.querySelector('.subtab-title')).left, hit: Boolean(hit && hit.closest('.subtab-bell')) };
    });
    assert.deepEqual(r, { before: true, hit: true }, id);
  }
});

test('a terminal\'s menu opens a new terminal in the folder its shell is in', async (t) => {
  const world = defaultWorld();
  world.items.w5.here = '~/code/bench/internal';
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await page.click('.pane[data-pane-id="p4"] [data-action="menu"]');
  await page.waitForSelector('#window-menu');
  await page.click('#window-menu .menu-item:has-text("New terminal in ~/code/bench/internal")');
  assert.deepEqual((await fake.waitForCall('AddToPane')).args, ['p4', { type: 'terminal', folder: '~/code/bench/internal' }]);

  // A window that isn't a terminal has no such entry.
  await page.click('.pane[data-pane-id="p3"] [data-action="menu"]');
  await page.waitForSelector('#window-menu');
  assert.equal(await page.$('#window-menu .menu-item:has-text("New terminal in")'), null);
});
