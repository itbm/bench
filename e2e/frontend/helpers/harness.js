'use strict';

// Shared set-up for the frontend tests: one browser and one static server
// per process, and openBench, which loads the UI against a FakeGo.

const { execSync } = require('node:child_process');
const path = require('node:path');
const { after } = require('node:test');
const { FakeGo } = require('./fakego.js');
const { defaultWorld } = require('./fixture.js');
const { serve } = require('./server.js');

const REPO = path.resolve(__dirname, '..', '..', '..');
const FRONTEND = path.join(REPO, 'frontend');
const DOCS = path.join(REPO, 'docs');
const SCREENSHOTS = path.join(__dirname, '..', 'screenshots');

// loadPlaywright finds the globally installed playwright package, with or
// without NODE_PATH set.
function loadPlaywright() {
  try {
    return require('playwright');
  } catch {
    const root = execSync('npm root -g', { encoding: 'utf8' }).trim();
    return require(path.join(root, 'playwright'));
  }
}

let browserPromise = null;
let serverPromise = null;

function browser() {
  if (!browserPromise) browserPromise = loadPlaywright().chromium.launch();
  return browserPromise;
}

function server() {
  if (!serverPromise) {
    serverPromise = serve([
      { prefix: '/__docs/', dir: DOCS },
      { prefix: '/', dir: FRONTEND },
    ]);
  }
  return serverPromise;
}

after(async () => {
  if (browserPromise) await (await browserPromise).close();
  if (serverPromise) await (await serverPromise).close();
  browserPromise = null;
  serverPromise = null;
});

// openBench loads the UI with a FakeGo holding world, and waits until the
// panes are drawn at the rectangles for the measured pane area. setup, if
// given, runs with the FakeGo before the page loads, to override methods.
// It returns { page, fake, close }; close fails if the page had errors.
async function openBench({ world = defaultWorld(), viewport = { width: 1440, height: 900 }, contextOptions = {}, setup = null } = {}) {
  const [b, s] = await Promise.all([browser(), server()]);
  const context = await b.newContext({ viewport, ...contextOptions });
  const page = await context.newPage();
  const fake = new FakeGo(world);
  page.on('pageerror', (err) => fake.errors.push(String(err && err.stack ? err.stack : err)));
  page.on('console', (msg) => {
    if (msg.type() === 'error') fake.errors.push(msg.text());
  });
  await fake.attach(page);
  if (setup) setup(fake);
  await page.goto(s.url + '/index.html');
  await fake.waitForCall('Ready');
  await fake.waitForCall('SetPaneArea');
  await page.waitForFunction(() => {
    const v = window.bench && window.bench.store.view;
    return Boolean(v && v.display.area.h > 0 && document.querySelector('.pane'));
  });
  // Closing fails the test if the page had errors.
  const close = async () => {
    await context.close();
    if (fake.errors.length) throw new Error('The page had errors:\n' + fake.errors.join('\n'));
  };
  return { page, fake, context, close };
}

// openPrototype loads docs/prototype.html, with its Google Fonts replaced
// by the vendored copies so that it works offline.
async function openPrototype({ viewport = { width: 1440, height: 900 } } = {}) {
  const [b, s] = await Promise.all([browser(), server()]);
  const context = await b.newContext({ viewport });
  const page = await context.newPage();
  await page.route(/fonts\.googleapis\.com/, (route) => route.fulfill({
    status: 200,
    contentType: 'text/css',
    body: '@import url("' + s.url + '/vendor/fonts/fonts.css");',
  }));
  await page.route(/fonts\.gstatic\.com/, (route) => route.abort());
  await page.goto(s.url + '/__docs/prototype.html');
  await page.evaluate(() => document.fonts.ready);
  return { page, context, close: () => context.close() };
}

// settle waits for pending renders, terminal fits, and calls to finish.
async function settle(page, ms = 120) {
  await page.evaluate(() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r))));
  await page.waitForTimeout(ms);
}

module.exports = { openBench, openPrototype, settle, SCREENSHOTS };
