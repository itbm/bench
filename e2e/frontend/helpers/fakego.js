'use strict';

// FakeGo stands in for Bench's Go side in the frontend tests. The page gets
// window.__BENCH_MOCK__ (see bridge.js), whose calls come here through a
// Playwright binding. FakeGo records every call, answers the ones the UI
// needs answers to, keeps a small world model so that renders look like the
// app's, and pushes events to the page.

const { buildView, find, leaves, presetTree } = require('./fixture.js');

// MOCK_SCRIPT runs in the page before its scripts.
const MOCK_SCRIPT = `
(() => {
  const listeners = {};
  // An error in a handler is thrown again outside the emit, as an uncaught
  // page error, as it would be from the Wails runtime, so that tests see it.
  window.__benchEmit = (event, data) => {
    for (const h of listeners[event] || []) {
      try {
        h(data);
      } catch (err) {
        setTimeout(() => { throw err; });
      }
    }
  };
  window.__BENCH_MOCK__ = {
    call(method, args) {
      return window.__benchCall(method, JSON.parse(JSON.stringify(args || []))).then((r) => {
        if (r && typeof r.error === 'string') throw new Error(r.error);
        return r ? r.value : undefined;
      });
    },
    on(event, handler) {
      (listeners[event] = listeners[event] || []).push(handler);
    },
  };
})();
`;

const SNAPSHOTS = {
  'zsh — ~/code/bench': '\x1b[38;5;246m~/code/bench on main\x1b[0m\r\n\x1b[38;5;214m$ \x1b[0mgo run ./cmd/bench\r\n\x1b[38;5;246mbench: 3 tabs loaded from config.toml\x1b[0m\r\n\x1b[38;5;246mbench: large screen, using wide layouts\x1b[0m\r\n\x1b[38;5;214m$ \x1b[0m',
  'go test — ~/code/bench': '\x1b[38;5;246m~/code/bench on main\x1b[0m\r\n\x1b[38;5;214m$ \x1b[0mgo test ./...\r\n\x1b[32mok   \x1b[0mbench/internal/layout    \x1b[38;5;246m(cached)\x1b[0m\r\n\x1b[32mok   \x1b[0mbench/internal/workspace \x1b[38;5;246m(cached)\x1b[0m\r\n\x1b[38;5;214m$ \x1b[0m',
};

class FakeGo {
  constructor(world) {
    this.world = world;
    this.calls = [];
    this.errors = [];
    this.overrides = new Map();
    this.page = null;
    this.autoState = true;
    // termText holds each terminal's output so far, as bytes, from its
    // snapshot text and termOut.
    this.termText = new Map();
  }

  // attach installs the mock in a page before it loads.
  async attach(page) {
    this.page = page;
    await page.exposeBinding('__benchCall', async (_source, method, args) => {
      try {
        return { value: await this.handle(method, args) };
      } catch (err) {
        return { error: err && err.message ? err.message : String(err) };
      }
    });
    await page.addInitScript(MOCK_SCRIPT);
  }

  // on overrides a method: fn(args, fake) returns its result or throws.
  on(method, fn) {
    this.overrides.set(method, fn);
  }

  view() {
    return buildView(this.world);
  }

  // emit pushes an event to the page.
  async emit(event, data) {
    if (!this.page || this.page.isClosed()) return;
    await this.page.evaluate(([e, d]) => window.__benchEmit(e, d), [event, data]).catch(() => {});
  }

  // emitState pushes the current view.
  emitState() {
    return this.emit('bench:state', this.view());
  }

  // changed sends the view soon, after the call that changed it returns,
  // as Go does.
  changed() {
    if (this.autoState) setTimeout(() => this.emitState(), 0);
  }

  callsTo(method) {
    return this.calls.filter((c) => c.method === method);
  }

  // waitForCall waits for a call to method whose arguments pass pred.
  async waitForCall(method, pred = () => true, timeout = 4000) {
    const end = Date.now() + timeout;
    for (;;) {
      const hit = this.calls.find((c) => c.method === method && pred(c.args));
      if (hit) return hit;
      if (Date.now() > end) {
        const seen = this.calls.map((c) => c.method + JSON.stringify(c.args)).join('\n  ');
        throw new Error('No call to ' + method + ' matched. Calls:\n  ' + seen);
      }
      await new Promise((r) => setTimeout(r, 20));
    }
  }

  // waitForLowered waits for a SetOverlay(false) call that follows a
  // SetOverlay(true) call, so that the first render's SetOverlay(false)
  // doesn't count.
  async waitForLowered(timeout = 4000) {
    const end = Date.now() + timeout;
    for (;;) {
      const seq = this.callsTo('SetOverlay').map((c) => c.args[0]);
      const raised = seq.indexOf(true);
      if (raised >= 0 && seq.indexOf(false, raised + 1) >= 0) return;
      if (Date.now() > end) throw new Error('No SetOverlay(false) after SetOverlay(true). SetOverlay calls: ' + JSON.stringify(seq));
      await new Promise((r) => setTimeout(r, 20));
    }
  }

  clearCalls() {
    this.calls.length = 0;
  }

  nextId(prefix) {
    this.world.seq += 1;
    return prefix + this.world.seq;
  }

  currentTab() {
    return this.world.tabs[this.world.current];
  }

  async handle(method, args) {
    this.calls.push({ method, args, at: Date.now() });
    if (this.overrides.has(method)) return this.overrides.get(method)(args, this);
    const fn = this['m' + method];
    if (fn) return fn.apply(this, args);
    return null;
  }

  // The bound methods that change what's shown.

  mReady() {
    return this.view();
  }

  mSetPaneArea(area) {
    this.world.area = area;
    this.changed();
  }

  mSwitchTab(i) {
    if (i < 0 || i >= this.world.tabs.length) throw new Error('There\'s no tab ' + (i + 1) + '.');
    this.world.current = i;
    this.changed();
  }

  mStepTab(step) {
    const n = this.world.tabs.length;
    this.world.current = (this.world.current + step + n) % n;
    this.changed();
  }

  mSetMode(mode) {
    this.world.mode = mode;
    this.changed();
  }

  mSetDemoScreen(screen, width) {
    if (!this.world.demo) throw new Error('Simulated screens are only for the browser preview.');
    this.world.demo.screen = screen;
    if (screen === 'fit' && width > 0) this.world.fitWidth = width;
    this.changed();
  }

  mFocusPane(id) {
    const tab = this.currentTab();
    if (!find(tab.root, id)) throw new Error('There\'s no such pane.');
    tab.focus = id;
    this.changed();
  }

  mShowItem(paneId, itemId) {
    const tab = this.currentTab();
    if (paneId === 'compact') {
      this.world.compactShown[this.world.current] = itemId;
    } else {
      const p = find(tab.root, paneId);
      if (!p || !p.items.includes(itemId)) throw new Error('That window isn\'t in that pane.');
      p.active = itemId;
      tab.focus = paneId;
    }
    this.changed();
  }

  mSetSizes(splitId, sizes) {
    const s = find(this.currentTab().root, splitId);
    if (!s || s.type !== 'split' || sizes.length !== s.children.length) throw new Error('There\'s no such split.');
    const sum = sizes.reduce((a, b) => a + b, 0);
    s.sizes = sizes.map((v) => (v / sum) * 100);
    this.changed();
  }

  mAddOptions() {
    const tab = this.currentTab();
    const view = this.view();
    const others = Object.values(view.items).filter((it) => it.tab >= 0 && !(it.tabs || [it.tab]).includes(this.world.current));
    const apps = [];
    for (const p of leaves(tab.root)) {
      for (const id of p.items) {
        const it = this.world.items[id];
        if (it.kind === 'app' && !apps.some((a) => a.bundleId === it.bundleId)) apps.push(it);
      }
    }
    return [
      ...this.world.floating.map((f) => ({ group: 'Windows not in a tab', mono: f.mono, label: f.title, hint: f.app, source: { type: 'floating', id: f.id } })),
      ...others.map((it) => ({ group: 'Windows in other tabs', mono: it.mono, label: it.title, hint: (it.tabs || [it.tab]).map((i) => this.world.tabs[i].name).join(' and ') + ' · ' + it.app, source: { type: 'item', id: it.id } })),
      ...apps.map((a) => ({ group: 'New window', mono: a.mono, label: 'New ' + a.app + ' window', hint: '', source: { type: 'new-window', bundleId: a.bundleId } })),
      { group: 'Built in', mono: '>_', label: 'New terminal', hint: '~', source: { type: 'terminal', folder: '' } },
      { group: 'Built in', mono: 'Wb', label: 'Web page…', hint: 'As a web app, or in Bench', source: { type: 'web' }, asksForUrl: true, webAppBrowser: 'Safari' },
      { group: 'Apps', mono: 'Ca', label: 'Calendar', hint: 'Open, or use its open windows', source: { type: 'app', bundleId: 'com.apple.iCal' } },
      { group: 'Apps', mono: 'Fi', label: 'Figma', hint: 'Open, or use its open windows', source: { type: 'app', bundleId: 'com.figma.Desktop' } },
      { group: 'Apps', mono: 'Li', label: 'Linear', hint: 'Open, or use its open windows', source: { type: 'app', bundleId: 'com.linear' } },
    ];
  }

  // mAddWebsite opens the site in the default browser, Safari, to make a web
  // app of it, as Go does for a site that has none. Tests override it for
  // the other outcomes.
  mAddWebsite() {
    return { added: '', browser: 'Safari', steps: 'Choose File ▸ Add to Dock, then click Add.' };
  }

  // mAddWebAppWhenReady reports that the web app isn't made yet.
  mAddWebAppWhenReady() {
    return false;
  }

  // mWebAppChoice offers Brave's web app of Slack for Slack's page, and
  // Safari to make one for other web pages, as Go would with those
  // installed. It offers nothing for other items. Tests override it for
  // the other answers.
  mWebAppChoice(itemId) {
    const it = this.world.items[itemId];
    if (!it || it.kind !== 'web' || it.app === 'VS Code') return { offer: '' };
    if (it.app === 'Slack') return { offer: 'use', app: 'Slack', browser: 'Brave' };
    return { offer: 'make', browser: 'Safari' };
  }

  // mUseWebApp puts Brave's web app of Slack in the web page's place.
  mUseWebApp(itemId) {
    this.replaceWithApp(itemId, { app: 'Slack', mono: 'Sl', bundleId: 'com.brave.Browser.app.slack' });
  }

  // mMakeWebApp opens the page's site in the default browser, Safari, to
  // make a web app of it, as Go does.
  mMakeWebApp() {
    return { added: '', browser: 'Safari', steps: 'Choose File ▸ Add to Dock, then click Add.' };
  }

  // mUseWebAppWhenReady reports that the web app isn't made yet.
  mUseWebAppWhenReady() {
    return false;
  }

  // replaceWithApp puts a new App item of an app ({ app, mono, bundleId })
  // in an item's place in every pane that holds it, as Go's UseWebApp does,
  // and returns its ID.
  replaceWithApp(itemId, app) {
    if (!this.world.items[itemId]) throw new Error('That window has closed.');
    const id = this.nextId('w');
    this.world.items[id] = { id, kind: 'app', app: app.app, mono: app.mono, title: app.app, status: 'ready', bundleId: app.bundleId };
    delete this.world.items[itemId];
    for (const t of this.world.tabs) {
      for (const p of leaves(t.root)) {
        const i = p.items.indexOf(itemId);
        if (i < 0) continue;
        p.items[i] = id;
        if (p.active === itemId) p.active = id;
      }
    }
    for (const [ti, shown] of Object.entries(this.world.compactShown)) {
      if (shown === itemId) this.world.compactShown[ti] = id;
    }
    this.changed();
    return id;
  }

  // mOpenInBrowser opens nothing: Go's browser preview refuses, which a
  // test can stand for with on('OpenInBrowser', …).
  mOpenInBrowser(itemId) {
    const it = this.world.items[itemId];
    if (!it) throw new Error('That window has closed.');
    if (it.kind !== 'web' || it.folder) throw new Error('Only web pages open in a browser.');
    return null;
  }

  // mSaveSettings applies the Settings dialog's settings, as Go does.
  mSaveSettings(settings) {
    this.world.settings = { ...this.world.settings, ...settings };
    this.changed();
  }

  mPreviewTree(index, preset) {
    const tab = index >= 0 ? this.world.tabs[index] : null;
    return presetTree(tab ? tab.root : null, preset, (p) => this.nextId(p));
  }

  // termBytes returns a terminal's output so far, starting with the
  // snapshot text for its title.
  termBytes(itemId) {
    if (!this.termText.has(itemId)) {
      const it = this.world.items[itemId];
      this.termText.set(itemId, Buffer.from((it && SNAPSHOTS[it.title]) || '$ ', 'utf8'));
    }
    return this.termText.get(itemId);
  }

  // termOut sends a terminal's output as Go does, in a term:data event
  // with the offset where it starts, and adds it to the snapshot.
  async termOut(itemId, text) {
    const before = this.termBytes(itemId);
    const bytes = Buffer.from(text, 'utf8');
    this.termText.set(itemId, Buffer.concat([before, bytes]));
    await this.emit('term:data', { id: itemId, data: bytes.toString('base64'), offset: before.length });
  }

  mTermSnapshot(itemId) {
    const bytes = this.termBytes(itemId);
    return { data: bytes.toString('base64'), offset: bytes.length };
  }
}

module.exports = { FakeGo, MOCK_SCRIPT };
