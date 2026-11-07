'use strict';

// Unread counts: a grey pill on the sub-tab of an app with a badge on its
// Dock icon, or of a page whose title starts with a count, a grey dot for
// a badge that isn't a number, and the total after each tab's name. Go
// works the counts out (ItemView's and TabView's unread); the frontend
// draws them, says them to a screen reader, and keeps them clear of the
// ×.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, openSettings, settle } = require('./helpers/harness.js');
const { defaultWorld, manyTabs } = require('./helpers/fixture.js');

// unreadWorld is the default world on the Comms tab, with Slack's page at
// 3 unread and Outlook's at 12, VS Code with a badge that isn't a number,
// and Brave with a badge of 4 on the first of its three windows.
function unreadWorld(opts = {}) {
  const world = defaultWorld({ current: 0, ...opts });
  world.items.w1.unread = { count: 3 };
  world.items.w2.unread = { count: 12 };
  world.items.w4.unread = { dot: true };
  world.items.w7.unread = { count: 4, wholeApp: true };
  return world;
}

const tab = (i) => '.tab[data-index="' + i + '"]';
const sub = (id) => '.subtab[data-item="' + id + '"]';

test('a tab shows its unread total after its name, which its tooltip and a screen reader say', async (t) => {
  const { page, close } = await openBench({ world: unreadWorld() });
  t.after(close);
  await page.waitForSelector(tab(0) + ' .tab-unread');
  // The current tab has its total too.
  assert.equal(await page.textContent(tab(0) + ' .tab-unread'), '15');
  assert.equal(await page.getAttribute(tab(0) + ' .tab-unread', 'aria-hidden'), 'true');
  assert.equal(await page.getAttribute(tab(0) + ' .tab-unread', 'title'), '15 unread');
  assert.ok((await page.getAttribute(tab(0), 'title')).split('\n').includes('15 unread'));
  // A screen reader reads the count after the name, and not the pill: the
  // tab's name starts with what it shows (decision 64). (The current tab
  // shows no shortcut, and Chromium puts a space between the tab's parts.)
  assert.equal(await page.getByRole('tab', { name: /^Comms ?, 15 unread$/, selected: true }).count(), 1);
  // Code's only unread thing is a badge that isn't a number: a dot.
  assert.equal(await page.$eval(tab(1) + ' .tab-unread', (el) => el.classList.contains('unread-dot') && el.textContent), '');
  assert.equal(await page.getByRole('tab', { name: /^Code ?, unread ⌃⌥2$/, selected: false }).count(), 1);
  assert.equal(await page.textContent(tab(2) + ' .tab-unread'), '4');

  // The pill comes right after the name, and the shortcut's place, where
  // the × shows, stays clear of it, on the current tab and under the
  // pointer.
  for (const i of [0, 2]) {
    await page.hover(tab(i) + ' .tab-name');
    await page.waitForFunction((sel) => getComputedStyle(document.querySelector(sel + ' .close-cross')).visibility === 'visible', tab(i));
    const r = await page.$eval(tab(i), (el) => {
      const box = (n) => n.getBoundingClientRect();
      const pill = box(el.querySelector('.tab-unread'));
      const cross = box(el.querySelector('.close-cross'));
      return { afterName: pill.left >= box(el.querySelector('.tab-name')).right, clear: pill.right <= cross.left };
    });
    assert.deepEqual(r, { afterName: true, clear: true }, 'tab ' + i);
  }
});

test('a sub-tab shows its unread count before its title, where its × never covers it, and says it', async (t) => {
  const { page, fake, close } = await openBench({ world: unreadWorld() });
  t.after(close);
  await page.waitForSelector(sub('w1') + ' .subtab-unread');
  assert.equal(await page.textContent(sub('w1') + ' .subtab-unread'), '3');
  assert.equal(await page.getAttribute(sub('w1'), 'aria-label'), '# general, Slack (3 unread)');
  // It's a tab in its pane's strip, whose name starts with the title it
  // shows, then its app, then the count (decision 64).
  assert.equal(await page.getByRole('tab', { name: '# general, Slack (3 unread)', exact: true, selected: true }).count(), 1);
  assert.ok((await page.getAttribute(sub('w1'), 'title')).split('\n').includes('3 unread'));
  for (const id of ['w1', 'w2']) {
    await page.hover(sub(id) + ' .subtab-title');
    await page.waitForFunction((sel) => getComputedStyle(document.querySelector(sel + ' .close-cross')).visibility === 'visible', sub(id));
    const r = await page.$eval(sub(id), (el) => {
      const box = (n) => n.getBoundingClientRect();
      const pill = box(el.querySelector('.subtab-unread'));
      const hit = document.elementFromPoint(pill.x + pill.width / 2, pill.y + pill.height / 2);
      return { before: pill.right <= box(el.querySelector('.subtab-title')).left, hit: Boolean(hit && hit.closest('.subtab-unread')) };
    });
    assert.deepEqual(r, { before: true, hit: true }, id);
  }

  // An app's Dock badge is the whole app's: it shows on the first of its
  // windows, which says so.
  fake.world.current = 2;
  await fake.emitState();
  await page.waitForSelector(sub('w7') + ' .subtab-unread');
  assert.equal(await page.getAttribute(sub('w7') + ' .subtab-unread', 'title'), '4 unread (whole app)');
  assert.ok((await page.getAttribute(sub('w7'), 'title')).split('\n').includes('4 unread (whole app)'));
  assert.match(await page.getAttribute(sub('w7'), 'aria-label'), /\(4 unread in the whole app\)$/);
  assert.equal(await page.$(sub('w8') + ' .unread-mark'), null);

  // A badge that isn't a number is a dot.
  fake.world.current = 1;
  await fake.emitState();
  await page.waitForSelector(sub('w4') + ' .unread-dot');
  assert.equal(await page.getAttribute(sub('w4') + ' .unread-dot', 'title'), 'Unread');
  assert.match(await page.getAttribute(sub('w4'), 'aria-label'), /\(unread\)$/);
});

test('long counts are cut short in the pill, and said whole', async (t) => {
  const world = unreadWorld();
  world.items.w1.unread = { count: 1500 };
  world.items.w2.unread = { count: 99, more: true };
  const { page, close } = await openBench({ world });
  t.after(close);
  await page.waitForSelector(sub('w1') + ' .subtab-unread');
  assert.equal(await page.textContent(sub('w1') + ' .subtab-unread'), '999+');
  assert.equal(await page.getAttribute(sub('w1') + ' .subtab-unread', 'title'), '1,500 unread');
  assert.equal(await page.textContent(sub('w2') + ' .subtab-unread'), '99+');
  assert.equal(await page.textContent(tab(0) + ' .tab-unread'), '999+');
  assert.equal(await page.getAttribute(tab(0) + ' .tab-unread', 'title'), '1,599+ unread');
  // A pill for one digit is round; longer ones are as wide as their text.
  const sizes = await page.evaluate(() => [...document.querySelectorAll('.subtab-unread')].map((el) => {
    const r = el.getBoundingClientRect();
    return { w: Math.round(r.width), h: Math.round(r.height) };
  }));
  assert.ok(sizes.every((s) => s.h === 16 && s.w >= 16), JSON.stringify(sizes));
});

test('the pills are grey, apart from the blue bell and the amber ⚠, and readable in each appearance', async (t) => {
  for (const colorScheme of ['dark', 'light']) {
    for (const contrast of ['no-preference', 'more']) {
      const world = unreadWorld();
      world.items.w2.attention = 'Notification: New mail';
      const { page, close } = await openBench({ world, contextOptions: { colorScheme, contrast } });
      await page.waitForSelector(sub('w2') + ' .subtab-bell');
      const c = await page.evaluate((sel) => {
        const style = (q) => getComputedStyle(document.querySelector(q));
        const rgb = (s) => s.match(/\d+/g).slice(0, 3).map(Number);
        return {
          pill: rgb(style(sel + ' .subtab-unread').backgroundColor),
          text: rgb(style(sel + ' .subtab-unread').color),
          bell: rgb(style(sel + ' .subtab-bell').color),
        };
      }, sub('w2'));
      const lum = (v) => {
        const [r, g, b] = v.map((x) => x / 255).map((x) => (x <= 0.03928 ? x / 12.92 : ((x + 0.055) / 1.055) ** 2.4));
        return 0.2126 * r + 0.7152 * g + 0.0722 * b;
      };
      const ratio = (a, b) => {
        const [hi, lo] = [lum(a), lum(b)].sort((x, y) => y - x);
        return (hi + 0.05) / (lo + 0.05);
      };
      const grey = (v) => Math.max(...v) - Math.min(...v) <= 30;
      const label = colorScheme + ', contrast ' + contrast;
      assert.ok(grey(c.pill), label + ': the pill is ' + c.pill);
      assert.ok(!grey(c.bell), label + ': the bell is ' + c.bell);
      const least = contrast === 'more' ? 7 : 4.5;
      assert.ok(ratio(c.text, c.pill) >= least, label + ': the count is at ' + ratio(c.text, c.pill).toFixed(2) + ':1');
      await close();
    }
  }
});

test('with unread_counts off, there are no counts, and Settings ▸ General turns it on and off', async (t) => {
  const world = unreadWorld();
  world.settings.unreadCounts = false;
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await page.waitForSelector(sub('w1'));
  assert.equal(await page.$('.unread-mark'), null);
  assert.equal(await page.getAttribute(sub('w1'), 'aria-label'), '# general, Slack');

  await openSettings(page);
  const box = '[data-setting="unreadCounts"]';
  assert.equal(await page.isChecked(box), false);
  assert.match(await page.$eval('#' + await page.getAttribute(box, 'aria-describedby'), (el) => el.textContent), /Dock.*Accessibility.*\(3\)/);
  // It's in General, after bringing back a hidden Bench.
  assert.deepEqual(await page.$$eval('#settings-general ~ .toggles input[data-setting]', (els) => els.slice(0, 4).map((el) => el.dataset.setting)),
    ['openAtLogin', 'fillScreen', 'showOnTabShortcut', 'unreadCounts']);
  await page.check(box);
  await page.click('[data-action="save-settings"]');
  await page.waitForSelector(sub('w1') + ' .subtab-unread');
  assert.equal(fake.world.settings.unreadCounts, true);
  assert.equal(await page.textContent(tab(0) + ' .tab-unread'), '15');
});

test('the overview shows each tab\'s unread total, and the counts of the windows it shows', async (t) => {
  const { page, close } = await openBench({ world: unreadWorld() });
  t.after(close);
  await page.waitForSelector(tab(0) + ' .tab-unread');
  await page.click('[data-action="open-palette"]');
  await page.fill('#palette-query', 'show all tabs');
  await page.keyboard.press('Enter');
  await page.waitForSelector('.overview-card');
  assert.equal(await page.textContent('#overview-card-0 .overview-card-head .overview-unread'), '15');
  assert.match(await page.getAttribute('#overview-card-0', 'aria-label'), /, 15 unread/);
  assert.equal(await page.$eval('#overview-card-1 .overview-card-head .overview-unread', (el) => el.classList.contains('unread-dot')), true);
  assert.equal(await page.textContent('#overview-card-0 .overview-shown .overview-unread'), '3');
});

test('in the smallest window, the sub-tabs\' counts show, and the current tab keeps its × in view', async (t) => {
  // measure gives the current tab's and the strips' sizes, whether the
  // tab's × is in its strip and takes a click there, and how much of the
  // first sub-tab's title shows.
  const measure = (page) => page.evaluate(() => {
    const box = (el) => el.getBoundingClientRect();
    const tab = document.querySelector('.tab[aria-selected="true"]');
    const strip = box(document.querySelector('.tabs'));
    const cross = box(tab.querySelector('.close-cross'));
    const hit = document.elementFromPoint(cross.x + cross.width / 2, cross.y + cross.height / 2);
    const subtabs = box(document.querySelector('.title-pane .subtabs'));
    const title = box(document.querySelector('.title-pane .subtab .subtab-title'));
    return {
      tab: Math.round(box(tab).width),
      tabs: Math.round(strip.width),
      subtabs: Math.round(subtabs.width),
      crossInStrip: cross.left >= strip.left && cross.right <= strip.right + 0.5,
      crossHit: Boolean(hit && hit.closest('.close-cross')),
      titleShown: Math.round(Math.min(title.right, subtabs.right) - title.left),
      overflow: document.documentElement.scrollWidth > document.documentElement.clientWidth,
    };
  });
  const small = { viewport: { width: 640, height: 480 } };
  // Without counts, for comparison.
  const plain = await openBench({ world: defaultWorld({ current: 0, screenWidth: 1512 }), ...small });
  await plain.page.waitForSelector('.title-pane .subtab');
  const before = await measure(plain.page);
  await plain.close();

  const { page, close } = await openBench({ world: unreadWorld({ screenWidth: 1512 }), ...small });
  t.after(close);
  await page.waitForSelector(sub('w1') + ' .subtab-unread');
  assert.equal(await page.textContent(sub('w1') + ' .subtab-unread'), '3');
  // The current tab's sub-tabs are beside it, in the title bar, with their
  // counts: it has no pill of its own, which would push its × out of its
  // strip, but its tooltip and a screen reader still have its total.
  assert.equal(await page.$(tab(0) + ' .tab-unread'), null);
  assert.ok((await page.getAttribute(tab(0), 'title')).split('\n').includes('15 unread'));
  assert.equal(await page.getByRole('tab', { name: /^Comms ?, 15 unread$/, selected: true }).count(), 1);
  const after = await measure(page);
  assert.deepEqual(after, { ...before, titleShown: after.titleShown }, 'the title bar is laid out as without counts');
  assert.ok(after.crossInStrip && after.crossHit, JSON.stringify(after));
  assert.equal(after.overflow, false);
  // The other tabs still show their totals.
  assert.equal(await page.textContent(tab(2) + ' .tab-unread'), '4');
});

// clearOfCross gives, for a sub-tab with the pointer over it, so that its
// × shows: whether its marks end before the ×'s fade (14 pt before it)
// begins, whether a click at the right end of its pill lands on the pill,
// not the ×, and how wide its title is.
const clearOfCross = (page, id) => page.$eval(sub(id), (el) => {
  const box = (n) => n.getBoundingClientRect();
  const cross = box(el.querySelector('.close-cross'));
  const marks = [...el.querySelectorAll('.capture-mark, .subtab-bell, .subtab-unread')].map(box);
  const pill = box(el.querySelector('.subtab-unread'));
  const hit = document.elementFromPoint(pill.right - 1, pill.y + pill.height / 2);
  return {
    clear: marks.every((m) => m.right <= cross.left - 14 + 0.5),
    pillHit: Boolean(hit && hit.closest('.subtab-unread')),
    title: Math.round(box(el.querySelector('.subtab-title')).width),
  };
});

test('in a full strip, a sub-tab with marks is wider by them, so that its × covers neither them nor all of its title', async (t) => {
  // A tab with one pane of 16 sub-tabs, each with a count of 1,234 and a
  // bell, or other marks as well.
  const world = manyTabs(defaultWorld({}), 4);
  world.current = 3;
  const p = world.tabs[3].root;
  const marks = {
    w400: { attention: 'Rang the bell', unread: { count: 1234 } },
    w401: { unread: { count: 1234 } },
    w402: { camera: 'active', microphone: 'muted', attention: 'Rang the bell', unread: { count: 12 } },
    w403: { unread: { dot: true } },
    w404: {},
  };
  for (let i = 0; i < 15; i++) {
    const id = 'w' + (400 + i);
    world.items[id] = {
      id, kind: 'builtin', app: 'Terminal', mono: '>_', title: 'zsh ' + i + ' — ~/code/project-' + i, status: 'ready', folder: '~',
      ...(marks[id] || marks.w400),
    };
    p.items.push(id);
  }
  const { page, close } = await openBench({ world, viewport: { width: 1440, height: 900 } });
  t.after(close);
  await page.waitForSelector(sub('w403'));
  await settle(page);
  assert.equal(await page.getAttribute('.pane[data-pane-id="p203"] .subtabs', 'data-more'), 'end');
  // Without marks, a sub-tab is as narrow as it goes, and its title keeps
  // 54 pt, the end of which goes under the ×.
  const plain = await page.$eval(sub('w404'), (el) => [el.getBoundingClientRect().width, el.querySelector('.subtab-title').getBoundingClientRect().width].map(Math.round));
  assert.deepEqual(plain, [96, 54]);
  for (const id of ['w400', 'w401', 'w402', 'w403']) {
    await page.hover(sub(id) + ' .mono-tile');
    await page.waitForFunction((sel) => getComputedStyle(document.querySelector(sel + ' .close-cross')).visibility === 'visible', sub(id));
    assert.deepEqual(await clearOfCross(page, id), { clear: true, pillHit: true, title: 54 }, id);
  }

  // So are the title bar's, in the smallest window, whose strip of
  // sub-tabs is narrower than one of them, and scrolls.
  const small = unreadWorld({ screenWidth: 1512 });
  small.items.w1.unread = { count: 1500 };
  small.items.w1.attention = 'Notification: New message';
  const two = await openBench({ world: small, viewport: { width: 640, height: 480 } });
  t.after(two.close);
  await two.page.waitForSelector('.title-pane ' + sub('w1') + ' .subtab-unread');
  await two.page.hover('.title-pane ' + sub('w1') + ' .mono-tile');
  const r = await clearOfCross(two.page, 'w1');
  assert.ok(r.clear && r.title === 54, JSON.stringify(r));
});
