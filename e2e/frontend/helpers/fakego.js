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
    const apps = [];
    for (const p of leaves(tab.root)) {
      for (const id of p.items) {
        const it = this.world.items[id];
        if (it.kind === 'app' && !apps.some((a) => a.bundleId === it.bundleId)) apps.push(it);
      }
    }
    return [
      ...this.world.floating.map((f) => ({ group: 'Windows not in a tab', mono: f.mono, label: f.title, hint: f.app, source: { type: 'floating', id: f.id } })),
      ...apps.map((a) => ({ group: 'New window', mono: a.mono, label: 'New ' + a.app + ' window', hint: '', source: { type: 'new-window', bundleId: a.bundleId } })),
      { group: 'Built in', mono: '>_', label: 'New terminal', hint: '~', source: { type: 'terminal', folder: '' } },
      { group: 'Built in', mono: 'Wb', label: 'Web page…', hint: 'Opens in a Bench web view', source: { type: 'web' }, asksForUrl: true },
      { group: 'Apps', mono: 'Ca', label: 'Calendar', hint: 'Open, or use its open windows', source: { type: 'app', bundleId: 'com.apple.iCal' } },
      { group: 'Apps', mono: 'Fi', label: 'Figma', hint: 'Open, or use its open windows', source: { type: 'app', bundleId: 'com.figma.Desktop' } },
      { group: 'Apps', mono: 'Li', label: 'Linear', hint: 'Open, or use its open windows', source: { type: 'app', bundleId: 'com.linear' } },
    ];
  }

  mPreviewTree(index, preset) {
    const tab = index >= 0 ? this.world.tabs[index] : null;
    return presetTree(tab ? tab.root : null, preset, (p) => this.nextId(p));
  }

  mTermSnapshot(itemId) {
    const it = this.world.items[itemId];
    const text = (it && SNAPSHOTS[it.title]) || '$ ';
    return Buffer.from(text, 'utf8').toString('base64');
  }
}

module.exports = { FakeGo, MOCK_SCRIPT };
