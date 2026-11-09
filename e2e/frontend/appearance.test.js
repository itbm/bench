'use strict';

// Bench's colours, which the appearance setting chooses: as macOS is
// (system), the default, which in the app is the colour scheme of the
// page's window, light, or dark. The main window, a popped-out terminal's
// window, and its terminals follow the setting, and the static
// Acknowledgements page its window's colour scheme. Both sets of tokens
// keep text readable, and more so with macOS's Increase contrast
// (prefers-contrast: more), as terminals do.

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

test('Bench matches macOS by default: light in a browser that prefers light, dark in one that prefers dark', async (t) => {
  const { page, close } = await openBench({ contextOptions: { colorScheme: 'light' } });
  t.after(close);
  await waitForTerminal(page);
  assert.deepEqual(await look(page), { theme: 'light', pane: LIGHT_PANE, media: '(prefers-color-scheme: light)' });
  assert.deepEqual(await colours(page), { background: LIGHT_PANE, foreground: '#1C2028' });
  await page.emulateMedia({ colorScheme: 'dark' });
  await page.waitForFunction(() => document.documentElement.dataset.theme === 'dark');
  assert.deepEqual(await colours(page), { background: DARK_PANE, foreground: '#E6E8EC' });
});

test('a View without an appearance matches macOS, as Go\'s default does', async (t) => {
  const w = defaultWorld();
  delete w.settings.appearance;
  const { page, close } = await openBench({ world: w, contextOptions: { colorScheme: 'light' } });
  t.after(close);
  assert.equal((await look(page)).theme, 'light');
  await openSettings(page, 'appearance');
  assert.equal(await page.getAttribute('[data-key="appearance-system"]', 'aria-checked'), 'true');
});

test('Settings ▸ Appearance ▸ Colours saves the appearance, and Bench takes its colours at once', async (t) => {
  const { page, fake, close } = await openBench({ contextOptions: { colorScheme: 'dark' } });
  t.after(close);
  await waitForTerminal(page);
  await openSettings(page, 'appearance');
  const choices = () => page.$$eval('[data-choice="appearance"]', (els) => els.map((el) => [el.textContent, el.getAttribute('aria-checked')]));
  assert.equal(await page.textContent('#appearance-label'), 'Colours');
  assert.deepEqual(await choices(), [['Match macOS', 'true'], ['Light', 'false'], ['Dark', 'false']]);
  assert.equal(await page.textContent('#appearance-help'),
    'Bench is light or dark as macOS is (System Settings, Appearance), and changes when it does. Web pages and the apps in your panes follow macOS either way.');
  await page.click('[data-key="appearance-dark"]');
  assert.equal(await page.textContent('#appearance-help'),
    'Bench is dark, even when macOS is light. Web pages and the apps in your panes follow macOS either way.');
  await page.click('[data-key="appearance-light"]');
  assert.deepEqual(await choices(), [['Match macOS', 'false'], ['Light', 'true'], ['Dark', 'false']]);
  assert.equal(await page.textContent('#appearance-help'),
    'Bench is light, even when macOS is dark. Web pages and the apps in your panes follow macOS either way.');
  // Choosing shows the colours at once, the terminals' too, without
  // saving them (PreviewSettings).
  await page.waitForFunction(() => document.documentElement.dataset.theme === 'light');
  await settle(page);
  assert.deepEqual(await colours(page), { background: LIGHT_PANE, foreground: '#1C2028' });
  assert.equal(fake.world.settings.appearance, 'system');
  assert.equal(fake.callsTo('SaveSettings').length, 0);

  await page.click('[data-action="save-settings"]');
  const save = await fake.waitForCall('SaveSettings');
  assert.equal(save.args[0].appearance, 'light');
  await page.waitForSelector('.settings-sheet', { state: 'detached' });
  await settle(page);
  assert.deepEqual(await look(page), { theme: 'light', pane: LIGHT_PANE, media: 'all' });
  assert.deepEqual(await colours(page), { background: LIGHT_PANE, foreground: '#1C2028' });

  // And back to dark.
  await openSettings(page, 'appearance');
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

const SURFACES = ['--bg-pane', '--bg-window', '--bg-chrome', '--bg-raised', '--bg-pane-header', '--bg-hover'];

// TAB_COLOURS are the tokens of the colours a tab can have.
const TAB_COLOURS = ['red', 'yellow', 'green', 'cyan', 'blue', 'purple', 'pink', 'grey'].map((c) => '--tab-' + c);

// READABLE are text tokens, the surfaces each must be readable on, and how
// readable, in both sets of colours: text at 4.5:1 at least, and faint text
// in terminals too.
const READABLE = [
  [['--text', '--text-strong', '--text-muted', '--text-control'], SURFACES, 4.5],
  [['--text-faint'], ['--bg-pane', '--bg-window'], 4.5],
  [['--accent', '--danger', '--warning', '--ok', '--bell', '--syn-keyword', '--syn-func', '--syn-type', '--syn-comment'], ['--bg-pane', '--bg-raised'], 4.5],
  [['--on-accent'], ['--accent'], 4.5],
  [['--term-dim'], ['--bg-pane'], 4.5],
  [['--unread-text'], ['--unread-bg'], 4.5],
  // The dot of a badge that isn't a number is a mark to see, at 3:1, on
  // the tabs and sub-tabs it sits on, at rest, shown, and under the
  // pointer.
  [['--unread-dot'], ['--bg-chrome', '--bg-tab-active', '--bg-pane-header', '--bg-control-on', '--bg-hover'], 3],
  // A tab's colour (decision 74) is a mark to see, at 3:1: its underline
  // on the title bar and on the current tab, at rest, under the pointer,
  // and pressed, the band along its panes' headers, and the dot beside its
  // name in the palette's rows, chosen or not, the overview's cards, and
  // Tab properties' swatches. A tab that a sub-tab is dragged over is
  // checked below, as its wash isn't a token of its own.
  [TAB_COLOURS, ['--bg-chrome', '--bg-tab-active', '--bg-hover', '--bg-pressed', '--bg-pane-header', '--bg-raised', '--bg-window', '--bg-control-on'], 3],
];

// MORE_READABLE is the same with macOS's Increase contrast on: quiet text
// at 7:1 on every surface, faint text in terminals at 7:1, and the lines
// around panes, dialogs, and chosen controls at 3:1, as WCAG asks of the
// edges of controls.
const MORE_READABLE = [
  ...READABLE,
  [['--text-muted', '--text-faint'], SURFACES, 7],
  [['--term-dim'], ['--bg-pane'], 7],
  [['--unread-text'], ['--unread-bg'], 7],
  [['--line-pane', '--line-dialog', '--line-control'], ['--bg-window', '--bg-chrome'], 3],
];

// APART is how far, as CIEDE2000, each tab colour is from the accent at
// least (decision 74), so that a tab's colour can't be taken for the
// focused pane's border or the current tab's own underline, and from the
// amber of a problem, so that a yellow tab's underline and band can't be
// taken for the warning sign of a window that keeps moving (decision 69).
const APART = 20;

// lab returns a #RRGGBB colour in CIELAB, with D65 white.
function lab(hex) {
  const [r, g, b] = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255)
    .map((c) => (c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4));
  const f = (t) => (t > 216 / 24389 ? Math.cbrt(t) : (24389 / 27 * t + 16) / 116);
  const [x, y, z] = [
    (0.4124 * r + 0.3576 * g + 0.1805 * b) / 0.95047,
    0.2126 * r + 0.7152 * g + 0.0722 * b,
    (0.0193 * r + 0.1192 * g + 0.9505 * b) / 1.08883,
  ].map(f);
  return [116 * y - 16, 500 * (x - y), 200 * (y - z)];
}

// deltaE returns how far apart two CIELAB colours look, as CIEDE2000, by
// Sharma, Wu, and Dalal's formulas.
function deltaE([L1, a1, b1], [L2, a2, b2]) {
  const rad = Math.PI / 180;
  const cBar = (Math.hypot(a1, b1) + Math.hypot(a2, b2)) / 2;
  const g = 0.5 * (1 - Math.sqrt(cBar ** 7 / (cBar ** 7 + 25 ** 7)));
  const [p1, p2] = [a1, a2].map((a) => (1 + g) * a);
  const [c1, c2] = [Math.hypot(p1, b1), Math.hypot(p2, b2)];
  const hue = (b, a) => (b === 0 && a === 0 ? 0 : (Math.atan2(b, a) / rad + 360) % 360);
  const [h1, h2] = [hue(b1, p1), hue(b2, p2)];
  let dh = 0;
  if (c1 * c2 !== 0) dh = h2 - h1 > 180 ? h2 - h1 - 360 : h2 - h1 < -180 ? h2 - h1 + 360 : h2 - h1;
  const dH = 2 * Math.sqrt(c1 * c2) * Math.sin(dh * rad / 2);
  const lBar = (L1 + L2) / 2;
  const cpBar = (c1 + c2) / 2;
  let hBar = h1 + h2;
  if (c1 * c2 !== 0) hBar = (Math.abs(h1 - h2) > 180 ? hBar + (hBar < 360 ? 360 : -360) : hBar) / 2;
  const t = 1 - 0.17 * Math.cos((hBar - 30) * rad) + 0.24 * Math.cos(2 * hBar * rad)
    + 0.32 * Math.cos((3 * hBar + 6) * rad) - 0.2 * Math.cos((4 * hBar - 63) * rad);
  const rc = 2 * Math.sqrt(cpBar ** 7 / (cpBar ** 7 + 25 ** 7));
  const rt = -Math.sin(60 * Math.exp(-(((hBar - 275) / 25) ** 2)) * rad) * rc;
  const sl = 1 + 0.015 * (lBar - 50) ** 2 / Math.sqrt(20 + (lBar - 50) ** 2);
  const sc = 1 + 0.045 * cpBar;
  const sh = 1 + 0.015 * cpBar * t;
  const [l, c, h] = [(L2 - L1) / sl, (c2 - c1) / sc, dH / sh];
  return Math.sqrt(l * l + c * c + h * h + rt * c * h);
}

// apart returns how far apart two #RRGGBB colours look, as CIEDE2000.
const apart = (one, two) => deltaE(lab(one), lab(two));

test('deltaE gives CIEDE2000 as Sharma, Wu, and Dalal\'s test data does', () => {
  // Pairs 1, 7, 17, 21, 25, and 33 of their table.
  const pairs = [
    [[50, 2.6772, -79.7751], [50, 0, -82.7485], 2.0425],
    [[50, 0, 0], [50, -1, 2], 2.3669],
    [[50, 2.5, 0], [73, 25, -18], 27.1492],
    [[50, 2.5, 0], [50, 3.1736, 0.5854], 1],
    [[60.2574, -34.0099, 36.2677], [60.4626, -34.1751, 39.4387], 1.2644],
    [[2.0776, 0.0795, -1.135], [0.9033, -0.0636, -0.5514], 0.9082],
  ];
  for (const [one, two, want] of pairs) {
    assert.ok(Math.abs(deltaE(one, two) - want) < 0.0001, one + ' and ' + two + ': ' + deltaE(one, two) + ', want ' + want);
  }
  assert.ok(Math.abs(apart('#000000', '#FFFFFF') - 100) < 0.01);
});

// tokensOf returns the values of the tokens in rules on page.
function tokensOf(page, rules) {
  const names = [...new Set(rules.flatMap(([fgs, bgs]) => [...fgs, ...bgs]))];
  return page.evaluate((list) => {
    const style = getComputedStyle(document.documentElement);
    return Object.fromEntries(list.map((n) => [n, style.getPropertyValue(n).trim()]));
  }, names);
}

test('both sets of colours keep text readable, and more so with Increase contrast', async (t) => {
  const { page, close } = await openPage('/acknowledgements.html');
  t.after(close);
  for (const scheme of ['dark', 'light']) {
    for (const [more, rules] of [['no-preference', READABLE], ['more', MORE_READABLE]]) {
      await page.emulateMedia({ colorScheme: scheme, contrast: more });
      const tokens = await tokensOf(page, [...rules, [TAB_COLOURS, ['--accent', '--accent-wash', '--warning', '--bg-pane']]]);
      for (const [fgs, bgs, least] of rules) {
        for (const fg of fgs) {
          for (const bg of bgs) {
            const ratio = contrast(tokens[fg], tokens[bg]);
            assert.ok(ratio >= least, scheme + ', contrast ' + more + ': ' + fg + ' ' + tokens[fg] + ' on ' + bg + ' ' + tokens[bg]
              + ' is ' + ratio.toFixed(2) + ':1, want ' + least + ':1');
          }
        }
      }
      // A tab's colour on a tab that a sub-tab is dragged over, which has
      // the accent's wash over the title bar, and on a pane header that
      // files from the Finder are over (decision 73), which has it over
      // the pane; and far enough from the accent and the amber of a
      // problem.
      const washes = ['--bg-chrome', '--bg-pane'].map((bg) => hex(tokens['--accent-wash'], tokens[bg]));
      for (const fg of TAB_COLOURS) {
        for (const wash of washes) {
          const ratio = contrast(tokens[fg], wash);
          assert.ok(ratio >= 3, scheme + ', contrast ' + more + ': ' + fg + ' ' + tokens[fg] + ' on the drop wash ' + wash
            + ' is ' + ratio.toFixed(2) + ':1, want 3:1');
        }
        for (const other of ['--accent', '--warning']) {
          const far = apart(tokens[fg], tokens[other]);
          assert.ok(far >= APART, scheme + ', contrast ' + more + ': ' + fg + ' ' + tokens[fg] + ' is ' + far.toFixed(1)
            + ' from ' + other + ' ' + tokens[other] + ', want ' + APART);
        }
      }
    }
  }
  // Increase contrast makes the lines stronger in both sets.
  for (const scheme of ['dark', 'light']) {
    await page.emulateMedia({ colorScheme: scheme, contrast: 'no-preference' });
    const before = await tokensOf(page, [[['--line', '--line-pane'], ['--bg-window']]]);
    await page.emulateMedia({ colorScheme: scheme, contrast: 'more' });
    const after = await tokensOf(page, [[['--line', '--line-pane'], ['--bg-window']]]);
    for (const line of ['--line', '--line-pane']) {
      assert.ok(contrast(after[line], after['--bg-window']) > contrast(before[line], before['--bg-window']) + 1,
        scheme + ': ' + line + ' ' + before[line] + ' became ' + after[line]);
    }
  }
});

// hex returns a CSS colour, rgb() or rgba(), as #RRGGBB, over bg.
function hex(css, bg) {
  const [r, g, b, a = 1] = css.match(/[\d.]+/g).map(Number);
  const under = [1, 3, 5].map((i) => parseInt(bg.slice(i, i + 2), 16));
  return '#' + [r, g, b].map((c, i) => Math.round(c * a + under[i] * (1 - a)).toString(16).padStart(2, '0')).join('').toUpperCase();
}

// drawn returns the colour that item id's terminal draws text in, as
// #RRGGBB over its background, bg.
async function drawn(page, text, bg, id = 'w5') {
  const css = await page.evaluate(([i, t]) => {
    const rows = document.querySelector('.term-host[data-term-host="' + i + '"] .xterm-rows');
    const span = [...rows.querySelectorAll('span')].find((s) => s.textContent.includes(t));
    return span ? getComputedStyle(span).color : null;
  }, [id, text]);
  assert.ok(css, 'no text "' + text + '" in the terminal');
  return hex(css, bg);
}

// PALE is output with plain text, faint text, Bench's own grey, and text
// in colours that are hard to read on one background or the other.
const PALE = '\r\nplainA \x1b[2mfaintA\x1b[0m \x1b[90mgreyA\x1b[0m \x1b[38;2;128;128;128mmidA\x1b[0m'
  + ' \x1b[38;2;235;235;235mpaleA\x1b[0m \x1b[38;2;30;30;30mdeepA\x1b[0m\r\n';

// stillFaint checks that the terminal's faint text is still fainter than
// its plain text: half as readable at most.
async function stillFaint(page, bg, what) {
  const faint = contrast(await drawn(page, 'faintA', bg), bg);
  const plain = contrast(await drawn(page, 'plainA', bg), bg);
  assert.ok(faint <= plain / 2, what + ': faint text is ' + faint.toFixed(2) + ':1, plain text ' + plain.toFixed(2) + ':1');
}

test('terminals keep text readable: faint text, grey text, and text in pale colours, at 4.5:1 in both sets of colours', async (t) => {
  for (const [appearance, bg] of [['light', LIGHT_PANE], ['dark', DARK_PANE]]) {
    const { page, fake, close } = await openBench({ world: world(appearance) });
    await waitForTerminal(page);
    assert.equal(await page.evaluate(() => window.bench.terminalContrast('w5')), 4.5);
    await fake.termOut('w5', PALE);
    await page.waitForFunction(() => document.querySelector('.xterm-rows').textContent.includes('deepA'));
    for (const text of ['faintA', 'greyA', 'midA', 'paleA', 'deepA']) {
      const fg = await drawn(page, text, bg);
      const ratio = contrast(fg, bg);
      assert.ok(ratio >= 4.5, appearance + ': ' + text + ' is ' + fg + ' on ' + bg + ', ' + ratio.toFixed(2) + ':1');
    }
    await stillFaint(page, bg, appearance);
    await close();
  }
});

test('with Increase contrast on, terminals keep text at 7:1, a focused pane has a thicker border, and both follow macOS', async (t) => {
  for (const [appearance, bg] of [['light', LIGHT_PANE], ['dark', DARK_PANE]]) {
    const { page, fake, close } = await openBench({ world: world(appearance), contextOptions: { contrast: 'more' } });
    await waitForTerminal(page);
    assert.equal(await page.evaluate(() => window.bench.terminalContrast('w5')), 7);
    await fake.termOut('w5', PALE);
    await page.waitForFunction(() => document.querySelector('.xterm-rows').textContent.includes('deepA'));
    for (const text of ['faintA', 'greyA', 'midA', 'paleA', 'deepA']) {
      const fg = await drawn(page, text, bg);
      const ratio = contrast(fg, bg);
      assert.ok(ratio >= 7, appearance + ': ' + text + ' is ' + fg + ', ' + ratio.toFixed(2) + ':1');
    }
    await stillFaint(page, bg, appearance + ', contrast more');
    // The focused pane's ring sits above the other panes, so that none
    // covers it when there's no gap.
    const ring = (sel) => page.$eval(sel, (el) => [getComputedStyle(el).boxShadow, getComputedStyle(el).zIndex]);
    const accent = appearance === 'light' ? 'rgb(156, 85, 4)' : 'rgb(242, 163, 58)';
    assert.deepEqual(await ring('.pane[data-focused="true"]'), [accent + ' 0px 0px 0px 1px', '1']);
    assert.deepEqual(await ring('.pane[data-focused="false"]'), ['none', 'auto']);

    // Turning Increase contrast off goes back to 4.5:1 and a 1 pt border.
    await page.emulateMedia({ contrast: 'no-preference' });
    await page.waitForFunction(() => window.bench.terminalContrast('w5') === 4.5);
    assert.deepEqual(await ring('.pane[data-focused="true"]'), ['none', 'auto']);
    const fg = await drawn(page, 'faintA', bg);
    assert.ok(contrast(fg, bg) < 7, appearance + ': faint text is ' + fg);
    await close();
  }
});

// GREYS is output in the terminal's bright black and white, whose colours
// are the --text-faint and --text-muted tokens, which Increase contrast
// changes.
const GREYS = '\r\n\x1b[90mgreyB\x1b[0m \x1b[37mwhiteB\x1b[0m\r\n';

// drawsTokens waits until item w5's terminal draws its bright black and
// white in the tokens' colours now, and fails if it doesn't.
async function drawsTokens(page, bg, what) {
  const want = await tokensOf(page, [[['--text-faint', '--text-muted'], []]]);
  const now = async () => ({ '--text-faint': await drawn(page, 'greyB', bg), '--text-muted': await drawn(page, 'whiteB', bg) });
  for (let i = 0; i < 20; i++) {
    const got = await now();
    if (got['--text-faint'] === want['--text-faint'] && got['--text-muted'] === want['--text-muted']) return;
    await settle(page, 50);
  }
  assert.deepEqual(await now(), want, what);
}

test('a terminal that\'s open takes the new colours when Increase contrast goes on or off, in both sets of colours', async (t) => {
  for (const [appearance, bg] of [['light', LIGHT_PANE], ['dark', DARK_PANE]]) {
    for (const first of ['no-preference', 'more']) {
      const { page, fake, close } = await openBench({ world: world(appearance), contextOptions: { contrast: first } });
      await waitForTerminal(page);
      await fake.termOut('w5', GREYS);
      await page.waitForFunction(() => document.querySelector('.xterm-rows').textContent.includes('whiteB'));
      await drawsTokens(page, bg, appearance + ', opened with contrast ' + first);
      for (const more of first === 'more' ? ['no-preference', 'more'] : ['more', 'no-preference']) {
        await page.emulateMedia({ contrast: more });
        await page.waitForFunction((r) => window.bench.terminalContrast('w5') === r, more === 'more' ? 7 : 4.5);
        await drawsTokens(page, bg, appearance + ', opened with contrast ' + first + ', then ' + more);
      }
      await close();
    }
  }
});
