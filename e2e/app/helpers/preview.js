'use strict';

// Shared set-up for the end-to-end tests of the whole app: the browser
// preview (the server build of cmd/bench, with the demo platform and
// pretend terminals), started per test with its own support folder and
// port, and a headless Chromium page driving it.
//
// The preview is built once per process with `go build -tags server`,
// unless BENCH_PREVIEW names a binary that's already built.

const { execFile, spawn, execSync } = require('node:child_process');
const fs = require('node:fs');
const net = require('node:net');
const os = require('node:os');
const path = require('node:path');
const { after } = require('node:test');

const REPO = path.resolve(__dirname, '..', '..', '..');

// TIMEOUT is how long a wait lasts before a test fails, in milliseconds.
const TIMEOUT = 15000;

// loadPlaywright finds the playwright package, from e2e/node_modules,
// NODE_PATH, or the global install.
function loadPlaywright() {
  try {
    return require('playwright');
  } catch {
    const root = execSync('npm root -g', { encoding: 'utf8' }).trim();
    return require(path.join(root, 'playwright'));
  }
}

let binaryPromise = null;
let browserPromise = null;
const scratch = [];

// binary returns the preview binary's path, building it the first time.
function binary() {
  if (process.env.BENCH_PREVIEW) return Promise.resolve(path.resolve(process.env.BENCH_PREVIEW));
  if (!binaryPromise) {
    binaryPromise = new Promise((resolve, reject) => {
      const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'bench-e2e-bin-'));
      scratch.push(dir);
      const out = path.join(dir, 'bench-preview');
      // With cgo, Wails' Linux build needs GTK 4, so build without it
      // unless CGO_ENABLED is set.
      const env = { ...process.env, CGO_ENABLED: process.env.CGO_ENABLED || '0' };
      execFile('go', ['build', '-tags', 'server', '-o', out, './cmd/bench'], { cwd: REPO, env }, (err, stdout, stderr) => {
        if (err) reject(new Error('building the preview failed:\n' + stderr));
        else resolve(out);
      });
    });
  }
  return binaryPromise;
}

// browser returns the shared headless Chromium, launching it the first
// time.
function browser() {
  if (!browserPromise) browserPromise = loadPlaywright().chromium.launch();
  return browserPromise;
}

after(async () => {
  if (browserPromise) await (await browserPromise).close();
  browserPromise = null;
  for (const dir of scratch.splice(0)) fs.rmSync(dir, { recursive: true, force: true });
});

// freePort returns a TCP port that's free on localhost right now.
function freePort() {
  return new Promise((resolve, reject) => {
    const srv = net.createServer();
    srv.once('error', reject);
    srv.listen(0, '127.0.0.1', () => {
      const { port } = srv.address();
      srv.close(() => resolve(port));
    });
  });
}

// newHome returns a new, empty support folder, removed after the tests.
function newHome() {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'bench-e2e-home-'));
  scratch.push(dir);
  return dir;
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// waitUntil polls fn until it returns something truthy, and returns that.
async function waitUntil(fn, what, timeout = TIMEOUT) {
  const end = Date.now() + timeout;
  for (;;) {
    const v = await fn();
    if (v) return v;
    if (Date.now() > end) throw new Error('timed out waiting for ' + what);
    await sleep(25);
  }
}

// startOnce runs the preview on port and waits until it serves the UI.
async function startOnce(bin, home, port) {
  // BENCH_HOME keeps anything else off the developer's own support folder.
  const proc = spawn(bin, ['--port', String(port), '--dir', home], {
    env: { ...process.env, BENCH_HOME: home },
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  let output = '';
  proc.stdout.on('data', (d) => { output += d; });
  proc.stderr.on('data', (d) => { output += d; });
  const exited = new Promise((resolve) => {
    proc.once('exit', (code, signal) => resolve({ code, signal }));
    proc.once('error', (err) => resolve({ error: err.message }));
  });
  let gone = null;
  exited.then((r) => { gone = r; });
  const url = 'http://localhost:' + port;
  try {
    await waitUntil(async () => {
      if (gone) throw new Error('the preview exited (' + JSON.stringify(gone) + '):\n' + output);
      try {
        const res = await fetch(url + '/');
        return res.ok;
      } catch {
        return false;
      }
    }, 'the preview to serve ' + url);
  } catch (err) {
    proc.kill('SIGKILL');
    throw err;
  }
  return { proc, exited, url, output: () => output, isGone: () => Boolean(gone) };
}

// startPreview starts the preview with a support folder (a new one unless
// home is given) on a free port. It resolves with { url, home, port,
// stop(), output() }, and stops the preview after the test if t is given.
async function startPreview({ home = newHome(), t = null } = {}) {
  const bin = await binary();
  let lastErr = null;
  for (let attempt = 0; attempt < 3; attempt++) {
    const port = await freePort();
    let run;
    try {
      run = await startOnce(bin, home, port);
    } catch (err) {
      // Another process may have taken the port meanwhile.
      lastErr = err;
      continue;
    }
    let stopped = null;
    const preview = {
      url: run.url,
      home,
      port,
      output: run.output,
      // contexts are the browser contexts open on the preview.
      contexts: [],
      // stop closes the pages open on the preview, so that they don't see
      // it go, then interrupts it, which puts windows back and saves
      // state.json. It resolves with the preview's exit code.
      stop() {
        if (!stopped) {
          stopped = (async () => {
            await Promise.all(preview.contexts.splice(0).map((c) => c.close().catch(() => {})));
            if (!run.isGone()) run.proc.kill('SIGINT');
            const timer = setTimeout(() => run.proc.kill('SIGKILL'), 10000);
            const r = await run.exited;
            clearTimeout(timer);
            return r.code;
          })();
        }
        return stopped;
      },
    };
    if (t) t.after(() => preview.stop());
    return preview;
  }
  throw lastErr;
}

// Console messages that aren't errors in the page: Wails' notes about the
// browser, and refused calls (Go answers 422; the UI shows a toast).
const EXPECTED_CONSOLE = [
  /status of 422/,
];

// openPage opens the preview in a new browser context and waits for the
// first View to be drawn, Go to have the pane area, and the event socket to
// be open. Unless keepGuide is set, it closes the first-run guide if it's
// open. For a path other than "/", it only loads the page. It records every
// bench:state event in window.__states. It returns { page, context, close
// }; close fails the test if the page had errors.
async function openPage(preview, { viewport = { width: 1600, height: 1000 }, keepGuide = false, path: urlPath = '/' } = {}) {
  const b = await browser();
  const context = await b.newContext({ viewport });
  preview.contexts.push(context);
  const page = await context.newPage();
  const errors = [];
  page.on('pageerror', (err) => errors.push(String(err && err.stack ? err.stack : err)));
  page.on('console', (msg) => {
    if (msg.type() !== 'error') return;
    if (EXPECTED_CONSOLE.some((re) => re.test(msg.text()))) return;
    errors.push(msg.text());
  });
  // Record each bench:state as it arrives over Wails' event socket, and
  // whether the socket is open, by wrapping the socket before the page's
  // scripts run.
  await page.addInitScript(() => {
    window.__states = [];
    window.__eventsOpen = false;
    const Native = window.WebSocket;
    window.WebSocket = class extends Native {
      constructor(...args) {
        super(...args);
        if (!String(args[0]).includes('/wails/events')) return;
        this.addEventListener('open', () => { window.__eventsOpen = true; });
        this.addEventListener('close', () => { window.__eventsOpen = false; });
        this.addEventListener('message', (e) => {
          try {
            const ev = JSON.parse(e.data);
            if (ev && ev.name === 'bench:state') window.__states.push(ev.data);
          } catch { /* not an event */ }
        });
      }
    };
  });
  await page.goto(preview.url + urlPath);
  const close = async () => {
    await context.close();
    if (errors.length) throw new Error('The page had errors:\n' + errors.join('\n'));
  };
  if (urlPath !== '/') return { page, context, close, errors };
  await page.waitForFunction(() => {
    const v = window.bench && window.bench.store.view;
    return Boolean(v && v.display && v.display.area.h > 0 && document.querySelector('.pane'));
  }, null, { timeout: TIMEOUT });
  await eventsOpen(page);
  if (!keepGuide) await closeGuide(page);
  return { page, context, close, errors };
}

// closeGuide closes the first-run guide for good if it's open.
async function closeGuide(page) {
  if (await page.locator('[data-key="guide-continue"]').count()) {
    await page.click('[data-key="guide-continue"]');
    await waitView(page, (v) => !v.firstRun);
  }
}

// eventsOpen waits until the page's event socket is open, so that it gets
// every event from then on.
function eventsOpen(page) {
  return page.waitForFunction(() => window.__eventsOpen, null, { timeout: TIMEOUT });
}

// waitForToast waits until the page shows a toast with text, and returns
// it.
async function waitForToast(page, text) {
  try {
    await page.waitForFunction((t) => {
      const el = document.getElementById('toast');
      return el.classList.contains('show') && (t instanceof RegExp ? t.test(el.textContent) : el.textContent === t);
    }, text, { timeout: TIMEOUT });
  } catch (err) {
    const shown = await page.evaluate(() => {
      const el = document.getElementById('toast');
      return (el.classList.contains('show') ? 'showing ' : 'hidden ') + JSON.stringify(el.textContent);
    });
    throw new Error('no toast matching ' + text + ' (the toast is ' + shown + '): ' + err.message);
  }
  return page.textContent('#toast');
}

// view returns the latest View the page has drawn.
function view(page) {
  return page.evaluate(() => window.bench.store.view);
}

// waitView waits until pred(view, arg) is true for the page's latest View,
// and returns that View. pred runs in the page, so it can use only its
// arguments.
async function waitView(page, pred, arg, timeout = TIMEOUT) {
  const src = '(' + pred.toString() + ')';
  await page.waitForFunction(({ src: s, arg: a }) => {
    const v = window.bench && window.bench.store.view;
    // eslint-disable-next-line no-new-func
    return Boolean(v && Function('return ' + s)()(v, a));
  }, { src, arg }, { timeout });
  return view(page);
}

// stateCount returns how many bench:state events the page has had.
function stateCount(page) {
  return page.evaluate(() => window.__states.length);
}

// nextState waits for a bench:state event after the first n, for which
// pred(view, arg) is true if it's given, and returns it.
async function nextState(page, n, pred = null, arg = undefined, timeout = TIMEOUT) {
  const src = pred ? '(' + pred.toString() + ')' : '';
  const handle = await page.waitForFunction(({ n: from, src: s, arg: a }) => {
    const states = window.__states.slice(from);
    const f = s ? Function('return ' + s)() : null;
    return states.find((v) => !f || f(v, a)) || null;
  }, { n, src, arg }, { timeout });
  return handle.jsonValue();
}

// Helpers for reading a View in the tests.

// leaves returns a tree's panes in reading order.
function leaves(node) {
  if (!node) return [];
  if (node.type === 'pane') return [node];
  return (node.children || []).flatMap(leaves);
}

// tabIndex returns the index of the tab with a name.
function tabIndex(v, name) {
  const i = v.tabs.findIndex((t) => t.name === name);
  if (i < 0) throw new Error('there is no tab ' + name);
  return i;
}

// panesOf returns a tab's panes, in reading order.
function panesOf(v, name) {
  return leaves(v.tabs[tabIndex(v, name)].root);
}

// itemsOf returns the items of a tab's panes, as lists of { app, title }.
function itemsOf(v, name) {
  return panesOf(v, name).map((p) => (p.items || []).map((id) => {
    const it = v.items[id];
    return { app: it.app, title: it.title };
  }));
}

// itemID returns the ID of the item with a title, in a tab or not.
function itemID(v, title) {
  const it = Object.values(v.items).find((i) => i.title === title)
    || (v.floating || []).find((i) => i.title === title);
  if (!it) throw new Error('there is no item titled ' + title);
  return it.id;
}

// paneOf returns the ID of the pane that holds an item.
function paneOf(v, id) {
  for (const t of v.tabs) {
    const p = leaves(t.root).find((n) => (n.items || []).includes(id));
    if (p) return p.id;
  }
  return '';
}

// setScreen chooses a simulated screen in the demo bar and waits for Go to
// report its width.
async function setScreen(page, screen) {
  const width = { monitor: 2560, laptop: 1512 }[screen];
  await page.click('[data-action="screen"][data-screen="' + screen + '"]');
  return waitView(page, (v, a) => v.demo.screen === a.screen && (!a.width || v.screenWidth === a.width)
    && v.display.area.w > 0, { screen, width });
}

// switchTo clicks a tab and waits for it to be current.
async function switchTo(page, name) {
  const v = await view(page);
  const i = tabIndex(v, name);
  await page.click('.tab[data-index="' + i + '"]');
  return waitView(page, (w, a) => w.current === a, i);
}

// openTabProperties opens Tab properties for the tab with a name (the
// current tab by default) as a user does: it right-clicks the tab in the
// tab strip and chooses Tab properties… in the tab's menu.
async function openTabProperties(page, name = null) {
  const v = await view(page);
  const i = name === null ? v.current : tabIndex(v, name);
  await page.click('.tab[data-index="' + i + '"]', { button: 'right' });
  await page.click('#tab-menu .menu-item:has-text("Tab properties…")');
  await page.waitForSelector('.editor-sheet');
}

// chooseMode chooses a layout (auto, wide, or compact) as a user does: with
// its button in the title bar, or, in a slim title bar, where those buttons
// are one, from the layout button's menu.
async function chooseMode(page, mode) {
  const seg = page.locator('[data-action="mode"][data-mode="' + mode + '"]');
  if (await seg.isVisible()) {
    await seg.click();
    return;
  }
  const label = mode[0].toUpperCase() + mode.slice(1);
  await page.click('[data-action="layout-menu"]');
  await page.click('#layout-menu [role="menuitemradio"]:has-text("' + label + '")');
}

// openSettings opens the Settings dialog with the title bar's gear button.
async function openSettings(page) {
  await page.click('[data-action="open-settings"]');
  await page.waitForSelector('.settings-sheet');
}

module.exports = {
  REPO,
  TIMEOUT,
  browser,
  chooseMode,
  closeGuide,
  eventsOpen,
  freePort,
  itemID,
  itemsOf,
  leaves,
  newHome,
  nextState,
  openPage,
  openSettings,
  openTabProperties,
  paneOf,
  panesOf,
  setScreen,
  startPreview,
  stateCount,
  switchTo,
  tabIndex,
  view,
  waitForToast,
  waitUntil,
  waitView,
};
