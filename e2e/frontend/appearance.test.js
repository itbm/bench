'use strict';

// Bench's colours, which the appearance setting chooses: dark, the
// default, light, or as macOS is (system), which in the app is the
// colour scheme of the page's window. The main window, a popped-out
// terminal's window, and its terminals follow the setting, and the static
// Acknowledgements page its window's colour scheme. Both sets of tokens
// keep text readable.

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, openPage, openSettings, openTerminalWindow, settle } = require('./helpers/harness.js');
const { defaultWorld } = require('./helpers/fixture.js');

const DARK_PANE = '#0E1014';
const LIGHT_PANE = '#FFFFFF';

// world returns the default world with an appearance setting.
function world(appearance, opts) {
  const w = defaultWorld(opts);
  w.settings.appearance = appearance;
  return w;
}

// look returns the colours in use: the page's data-theme, its --bg-pane
// token, and the light tokens' media query.
function look(page) {
  return page.evaluate(() => ({
    theme: document.documentElement.dataset.theme,
    pane: getComputedStyle(document.documentElement).getPropertyValue('--bg-pane').trim(),
    media: document.getElementById('light-tokens').sheet.media.mediaText,
  }));
}

// colours returns the background and text of item id's terminal.
const colours = (page, id = 'w5') => page.evaluate((i) => window.bench.terminalColours(i), id);

// waitForTerminal waits until w5's terminal has drawn.
async function waitForTerminal(page, root = '.term-host[data-term-host="w5"]') {
  await page.waitForFunction((sel) => Boolean(document.querySelector(sel + ' .xterm-rows')), root);
}

test('Bench is dark by default, even in a browser that prefers light', async (t) => {
  const { page, close } = await openBench({ contextOptions: { colorScheme: 'light' } });
  t.after(close);
  await waitForTerminal(page);
  assert.deepEqual(await look(page), { theme: 'dark', pane: DARK_PANE, media: 'not all' });
  assert.deepEqual(await colours(page), { background: DARK_PANE, foreground: '#E6E8EC' });
});

test('Settings ▸ Appearance ▸ Colours saves the appearance, and Bench takes its colours at once', async (t) => {
  const { page, fake, close } = await openBench({ contextOptions: { colorScheme: 'dark' } });
  t.after(close);
  await waitForTerminal(page);
  await openSettings(page);
  const choices = () => page.$$eval('[data-choice="appearance"]', (els) => els.map((el) => [el.textContent, el.getAttribute('aria-pressed')]));
  assert.equal(await page.textContent('#appearance-label'), 'Colours');
  assert.deepEqual(await choices(), [['Match macOS', 'false'], ['Light', 'false'], ['Dark', 'true']]);
  assert.equal(await page.textContent('#appearance-help'),
    'Bench is dark, even when macOS is light. Web pages and the apps in your panes follow macOS either way.');
  await page.click('[data-key="appearance-system"]');
  assert.equal(await page.textContent('#appearance-help'),
    'Bench is light or dark as macOS is (System Settings, Appearance), and changes when it does. Web pages and the apps in your panes follow macOS either way.');
  await page.click('[data-key="appearance-light"]');
  assert.deepEqual(await choices(), [['Match macOS', 'false'], ['Light', 'true'], ['Dark', 'false']]);
  assert.equal(await page.textContent('#appearance-help'),
    'Bench is light, even when macOS is dark. Web pages and the apps in your panes follow macOS either way.');
  // Choosing isn't saving.
  assert.equal((await look(page)).theme, 'dark');

  await page.click('[data-action="save-settings"]');
  const save = await fake.waitForCall('SaveSettings');
  assert.equal(save.args[0].appearance, 'light');
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
  await settle(page);
  assert.deepEqual(await look(page), { theme: 'light', pane: LIGHT_PANE, media: 'all' });
  assert.deepEqual(await colours(page), { background: LIGHT_PANE, foreground: '#1C2028' });

  // And back to dark.
  await openSettings(page);
  await page.click('[data-key="appearance-dark"]');
  await page.click('[data-action="save-settings"]');
  await fake.waitForCall('SaveSettings', (a) => a[0].appearance === 'dark');
  await page.waitForFunction(() => document.documentElement.dataset.theme === 'dark');
  assert.deepEqual(await look(page), { theme: 'dark', pane: DARK_PANE, media: 'not all' });
  assert.deepEqual(await colours(page), { background: DARK_PANE, foreground: '#E6E8EC' });
});

test('Match macOS follows the window\'s colour scheme, and changes when it does', async (t) => {
  const { page, close } = await openBench({ world: world('system'), contextOptions: { colorScheme: 'dark' } });
  t.after(close);
  await waitForTerminal(page);
  assert.deepEqual(await look(page), { theme: 'dark', pane: DARK_PANE, media: '(prefers-color-scheme: light)' });
  const themes = [];
  await page.exposeFunction('noteTheme', (theme) => themes.push(theme));
  await page.evaluate(() => document.addEventListener('bench:theme', (e) => window.noteTheme(e.detail.theme)));

  await page.emulateMedia({ colorScheme: 'light' });
  await page.waitForFunction(() => document.documentElement.dataset.theme === 'light');
  assert.equal((await look(page)).pane, LIGHT_PANE);
  assert.deepEqual(await colours(page), { background: LIGHT_PANE, foreground: '#1C2028' });

  await page.emulateMedia({ colorScheme: 'dark' });
  await page.waitForFunction(() => document.documentElement.dataset.theme === 'dark');
  assert.deepEqual(await colours(page), { background: DARK_PANE, foreground: '#E6E8EC' });
  await settle(page);
  assert.deepEqual(themes, ['light', 'dark']);
});

test('a fixed appearance doesn\'t follow the window\'s colour scheme', async (t) => {
  const { page, close } = await openBench({ world: world('light'), contextOptions: { colorScheme: 'dark' } });
  t.after(close);
  assert.deepEqual(await look(page), { theme: 'light', pane: LIGHT_PANE, media: 'all' });
  await page.emulateMedia({ colorScheme: 'light' });
  await page.emulateMedia({ colorScheme: 'dark' });
  await settle(page);
  assert.equal((await look(page)).theme, 'light');
});

test('a popped-out terminal\'s window takes the appearance setting', async (t) => {
  const { page, close } = await openTerminalWindow({ id: 'w5', world: world('light') });
  t.after(close);
  await page.waitForFunction(() => document.documentElement.dataset.theme === 'light');
  assert.deepEqual(await colours(page), { background: LIGHT_PANE, foreground: '#1C2028' });
  const bg = await page.evaluate(() => getComputedStyle(document.querySelector('.terminal-window')).backgroundColor);
  assert.notEqual(bg, 'rgb(14, 16, 20)');
});

test('the Acknowledgements page follows its window\'s colour scheme, without a script', async (t) => {
  const { page, close } = await openPage('/acknowledgements.html', { viewport: { width: 640, height: 720 } });
  t.after(close);
  const pane = () => page.evaluate(() => getComputedStyle(document.documentElement).getPropertyValue('--bg-pane').trim());
  await page.emulateMedia({ colorScheme: 'dark' });
  assert.equal(await pane(), DARK_PANE);
  await page.emulateMedia({ colorScheme: 'light' });
  assert.equal(await pane(), LIGHT_PANE);
});

// contrast returns the contrast ratio of two #RRGGBB colours, as WCAG
// defines it.
function contrast(a, b) {
  const lum = (hex) => {
    const [r, g, bl] = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255)
      .map((c) => (c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4));
    return 0.2126 * r + 0.7152 * g + 0.0722 * bl;
  };
  const [hi, lo] = [lum(a), lum(b)].sort((x, y) => y - x);
  return (hi + 0.05) / (lo + 0.05);
}

// READABLE are text tokens and the surfaces each must be readable on, at
// 4.5:1 at least, in both sets of colours.
const READABLE = [
  [['--text', '--text-strong', '--text-muted', '--text-control'], ['--bg-pane', '--bg-window', '--bg-chrome', '--bg-raised', '--bg-pane-header', '--bg-hover']],
  [['--text-faint'], ['--bg-pane', '--bg-window']],
  [['--accent', '--danger', '--warning', '--ok', '--bell', '--syn-keyword', '--syn-func', '--syn-type', '--syn-comment'], ['--bg-pane', '--bg-raised']],
  [['--on-accent'], ['--accent']],
];

test('both sets of colours keep text readable', async (t) => {
  const { page, close } = await openPage('/acknowledgements.html');
  t.after(close);
  for (const scheme of ['dark', 'light']) {
    await page.emulateMedia({ colorScheme: scheme });
    const names = [...new Set(READABLE.flat(2))];
    const tokens = await page.evaluate((list) => {
      const style = getComputedStyle(document.documentElement);
      return Object.fromEntries(list.map((n) => [n, style.getPropertyValue(n).trim()]));
    }, names);
    for (const [fgs, bgs] of READABLE) {
      for (const fg of fgs) {
        for (const bg of bgs) {
          const ratio = contrast(tokens[fg], tokens[bg]);
          assert.ok(ratio >= 4.5, scheme + ': ' + fg + ' ' + tokens[fg] + ' on ' + bg + ' ' + tokens[bg] + ' is ' + ratio.toFixed(2) + ':1');
        }
      }
    }
  }
});
