'use strict';

// The host chip in a web page's pane header, and its warnings, and Open in
// browser in the ⋯ menu.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench } = require('./helpers/harness.js');
const { defaultWorld } = require('./helpers/fixture.js');

// comms returns the default world on the Comms tab, with Slack's item (w1)
// changed by page.
function comms(page = {}) {
  const world = defaultWorld({ current: 0 });
  world.items.w1 = { ...world.items.w1, ...page };
  return world;
}

// chipOf describes the chip in a pane's header, or null.
function chipOf(page, paneId) {
  return page.$eval('.pane[data-pane-id="' + paneId + '"] .pane-actions', (el) => {
    const chip = el.querySelector('.host-chip');
    const badge = el.querySelector('.badge');
    if (!chip) return { badge: badge ? badge.textContent : null };
    const part = (sel) => (chip.querySelector(sel) ? chip.querySelector(sel).textContent : null);
    return {
      badge: badge ? badge.textContent : null,
      text: chip.textContent,
      sub: part('.host-sub'),
      site: part('.host-site'),
      warning: chip.dataset.warning || null,
      role: chip.getAttribute('role'),
      label: chip.getAttribute('aria-label'),
      title: chip.getAttribute('title'),
    };
  });
}

test('a web page\'s pane header shows its host, with its site emphasised', async (t) => {
  const { page, close } = await openBench({ world: comms() });
  t.after(close);
  assert.deepEqual(await chipOf(page, 'p1'), {
    badge: null, text: 'app.slack.com', sub: 'app.', site: 'slack.com', warning: null,
    role: 'note', label: 'Page on app.slack.com', title: 'app.slack.com',
  });
  assert.deepEqual(await chipOf(page, 'p2'), {
    badge: null, text: 'outlook.office.com', sub: 'outlook.', site: 'office.com', warning: null,
    role: 'note', label: 'Page on outlook.office.com', title: 'outlook.office.com',
  });
  // The site is stronger than the rest of the host.
  const looks = await page.$eval('.pane[data-pane-id="p1"] .host-chip', (chip) => {
    const style = (sel) => getComputedStyle(chip.querySelector(sel));
    return {
      sub: [style('.host-sub').color, style('.host-sub').fontWeight],
      site: [style('.host-site').color, style('.host-site').fontWeight],
      dir: chip.querySelector('.host-sub').getAttribute('dir'),
    };
  });
  assert.notEqual(looks.sub[0], looks.site[0]);
  assert.ok(Number(looks.site[1]) > Number(looks.sub[1]), JSON.stringify(looks));
  assert.equal(looks.dir, 'rtl');
});

test('a page on another site, or that isn\'t secure, has a warning', async (t) => {
  const cases = [
    {
      page: { url: 'https://evil.example/slack', host: 'evil.example', site: 'evil.example', offSite: true },
      want: {
        text: 'Other site: evil.example', sub: null, site: 'evil.example', warning: 'off-site',
        label: 'Other site: evil.example',
        title: 'Other site: evil.example. The page has moved to another site. Check that it\'s the one you expect before you type a password.',
      },
    },
    {
      page: { url: 'https://evil.okta.com/sso', host: 'evil.okta.com', site: 'okta.com', offSite: true },
      want: {
        text: 'Other site: evil.okta.com', sub: 'evil.', site: 'okta.com', warning: 'off-site',
        label: 'Other site: evil.okta.com',
        title: 'Other site: evil.okta.com. The page has moved to another site. Check that it\'s the one you expect before you type a password.',
      },
    },
    {
      page: { url: 'http://slack.com/', host: 'slack.com', site: 'slack.com', notSecure: true },
      want: {
        text: 'Not secure: slack.com', sub: null, site: 'slack.com', warning: 'not-secure',
        label: 'Not secure: page on slack.com',
        title: 'Not secure: slack.com. Don\'t enter passwords or other private details on this page.',
      },
    },
    {
      // Not secure wins.
      page: { url: 'http://www.example.com/', host: 'www.example.com', site: 'example.com', offSite: true, notSecure: true },
      want: {
        text: 'Not secure: www.example.com', sub: 'www.', site: 'example.com', warning: 'not-secure',
        label: 'Not secure: page on www.example.com',
        title: 'Not secure: www.example.com. Don\'t enter passwords or other private details on this page.',
      },
    },
  ];
  for (const c of cases) {
    const { page, close } = await openBench({ world: comms(c.page) });
    try {
      assert.deepEqual(await chipOf(page, 'p1'), { badge: null, role: 'note', ...c.want }, c.page.host);
      // Styled as a pane note.
      const bg = await page.$eval('.pane[data-pane-id="p1"] .host-chip', (el) => getComputedStyle(el).backgroundColor);
      assert.equal(bg, 'rgba(242, 201, 125, 0.1)', c.page.host);
    } finally {
      await close();
    }
  }
});

test('a web page with no host keeps the Web badge, as VS Code for the Web does', async (t) => {
  const world = comms({ host: undefined, site: undefined, url: 'about:blank' });
  // VS Code for the Web has a folder instead of an address. Go sends it no
  // host, and the UI shows none even if it had one.
  world.items.w2 = { ...world.items.w2, app: 'VS Code', url: undefined, folder: '~/code/bench', host: '127.0.0.1', site: '127.0.0.1' };
  const { page, close } = await openBench({ world });
  t.after(close);
  assert.deepEqual(await chipOf(page, 'p1'), { badge: 'Web' });
  assert.deepEqual(await chipOf(page, 'p2'), { badge: 'Web' });
});

// fitOf describes how a pane's header fits its host chip: whether the
// chip and its warning's words are shown, and its site whole, the part of
// the host before it cut short, and whether the sub-tabs and buttons fit.
function fitOf(page, paneId) {
  return page.$eval('.pane[data-pane-id="' + paneId + '"]', (pane) => {
    const rect = (el) => el.getBoundingClientRect();
    const shown = (el) => Boolean(el) && getComputedStyle(el).display !== 'none' && rect(el).width > 1;
    const chip = pane.querySelector('.host-chip');
    const site = chip.querySelector('.host-site');
    const sub = chip.querySelector('.host-sub');
    const words = chip.querySelector('.host-warning-text');
    const c = rect(chip);
    // The width of the site's text, whether or not its box shows all of it.
    const range = document.createRange();
    range.selectNodeContents(site);
    const text = range.getBoundingClientRect().width;
    // Whether a box around the site, up to the pane's own, cuts it short,
    // or puts an ellipsis where it's cut: one at its start would be drawn
    // over the site's start.
    const inside = (a, b) => a.left >= b.left - 0.5 && a.right <= b.right + 0.5;
    let cut = false;
    for (let el = site.parentElement; el && el !== pane.parentElement; el = el.parentElement) {
      const style = getComputedStyle(el);
      if (style.overflowX !== 'visible' && (!inside(rect(site), rect(el)) || (el !== pane && style.textOverflow === 'ellipsis'))) cut = true;
    }
    return {
      pane: rect(pane).width,
      chip: shown(chip),
      words: shown(words),
      icon: shown(chip.querySelector('.host-warning svg')),
      site: shown(site) ? site.textContent : null,
      // The site's box has room for all its text, inside the chip, and
      // after the part of the host that's cut short, whose ellipsis is
      // drawn in its own box.
      siteWhole: shown(site)
        ? !cut && text <= rect(site).width + 0.5 && inside(rect(site), c) && (!shown(sub) || rect(sub).right <= rect(site).left + 0.5)
        : null,
      subCut: shown(sub) ? sub.scrollWidth > sub.clientWidth : null,
      // The chip ends before the pane's buttons, which are inside the pane.
      beforeButtons: !shown(chip) || c.right <= rect(pane.querySelector('[data-action="split-right"]')).left + 0.5,
      menuInside: rect(pane.querySelector('[data-action="menu"]')).right <= rect(pane).right + 0.5,
      subtabs: rect(pane.querySelector('.subtabs')).width,
      label: chip.getAttribute('aria-label'),
      title: chip.getAttribute('title'),
    };
  });
}

test('a long host loses its start, and its site stays whole', async (t) => {
  const host = 'a-very-long-team-name.eu-west-1.service.example.co.uk';
  const { page, close } = await openBench({ world: comms({ host, site: 'example.co.uk', offSite: true }) });
  t.after(close);
  const fit = await fitOf(page, 'p1');
  assert.equal(fit.site, 'example.co.uk');
  assert.ok(fit.siteWhole, 'the site is whole: ' + JSON.stringify(fit));
  assert.ok(fit.subCut, 'the start of the host is cut short: ' + JSON.stringify(fit));
  assert.ok(fit.words && fit.beforeButtons && fit.menuInside, JSON.stringify(fit));
  // The label and tooltip have the whole host.
  assert.equal(fit.label, 'Other site: ' + host);
  assert.ok(fit.title.startsWith('Other site: ' + host + '. '), fit.title);
});

test('in a narrow pane, the host chip gives way, but never cuts its site short', async (t) => {
  const pages = {
    'login.microsoftonline.com': { host: 'login.microsoftonline.com', site: 'microsoftonline.com', offSite: true },
    'www.example.com': { host: 'www.example.com', site: 'example.com', notSecure: true },
    'app.slack.com': { host: 'app.slack.com', site: 'slack.com' },
  };
  // Pane p1's share of the Comms tab, at 1,440 points: 422, 350, 307, 221
  // and 178 points wide.
  const cases = [
    { size: 30, words: true, site: true },
    { size: 25, words: false, site: true },
    { size: 22, words: false, site: true },
    { size: 16, words: false, site: false },
    { size: 13, words: false, site: false },
  ];
  for (const [host, item] of Object.entries(pages)) {
    for (const c of cases) {
      for (const items of [['w1'], ['w1', 'w3', 'w5']]) {
        const world = comms(item);
        world.tabs[0].root.sizes = [c.size, 100 - c.size];
        world.tabs[0].root.children[0].items = items;
        const { page, close } = await openBench({ world });
        try {
          const fit = await fitOf(page, 'p1');
          const what = host + ' at ' + c.size + '% with ' + items.length + ' sub-tabs: ' + JSON.stringify(fit);
          // The sub-tabs and buttons always fit.
          assert.ok(fit.menuInside && fit.beforeButtons && fit.subtabs >= 30, what);
          if (!item.offSite && !item.notSecure) {
            // A page with no warning shows its host, or nothing but its
            // label, for screen readers.
            assert.equal(fit.chip, c.site, what);
            assert.equal(fit.label, 'Page on ' + host, what);
            if (c.site) assert.ok(fit.siteWhole, what);
            continue;
          }
          // A warning keeps its icon, and its words while there's room.
          assert.ok(fit.chip && fit.icon, what);
          assert.equal(fit.words, c.words, what);
          // The site is whole, or not shown at all.
          assert.equal(fit.site, c.site ? item.site : null, what);
          if (c.site) assert.ok(fit.siteWhole, what);
          assert.ok(fit.label.endsWith(host) && fit.title.includes(host), what);
        } finally {
          await close();
        }
      }
    }
  }
});

test('the ⋯ menu opens a web page in the browser', async (t) => {
  const world = comms();
  world.items.w2 = { ...world.items.w2, app: 'VS Code', url: undefined, folder: '~/code/bench', host: undefined, site: undefined };
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  const labels = async (paneId) => {
    await page.click('.pane[data-pane-id="' + paneId + '"] [data-action="menu"]');
    await page.waitForSelector('#window-menu');
    const out = await page.$$eval('#window-menu .menu-label', (els) => els.map((el) => el.textContent));
    await page.keyboard.press('Escape');
    await page.waitForSelector('#window-menu', { state: 'detached' });
    return out;
  };
  assert.deepEqual((await labels('p1')).slice(-2), ['Open in browser', 'Pop out of Bench']);
  // Not VS Code for the Web, whose address holds its server's token.
  assert.ok(!(await labels('p2')).includes('Open in browser'));

  await page.click('.pane[data-pane-id="p1"] [data-action="menu"]');
  await page.click('#window-menu .menu-item:has-text("Open in browser")');
  const call = await fake.waitForCall('OpenInBrowser');
  assert.deepEqual(call.args, ['w1']);
  await page.waitForSelector('#window-menu', { state: 'detached' });

  // The browser preview can't open the browser: Go says so.
  fake.on('OpenInBrowser', () => { throw new Error('This build of Bench can\'t open links.'); });
  await page.click('.pane[data-pane-id="p1"] [data-action="menu"]');
  await page.click('#window-menu .menu-item:has-text("Open in browser")');
  await page.waitForFunction(() => document.getElementById('toast').textContent === 'This build of Bench can\'t open links.');
});

test('App items and terminals have no Open in browser', async (t) => {
  const { page, close } = await openBench();
  t.after(close);
  for (const paneId of ['p3', 'p4']) {
    await page.click('.pane[data-pane-id="' + paneId + '"] [data-action="menu"]');
    await page.waitForSelector('#window-menu');
    assert.equal(await page.$('#window-menu .menu-item:has-text("Open in browser")'), null, paneId);
    await page.keyboard.press('Escape');
    await page.waitForSelector('#window-menu', { state: 'detached' });
  }
});
