'use strict';

// FakeGo stands in for Bench's Go side in the frontend tests. The page gets
// window.__BENCH_MOCK__ (see bridge.js), whose calls come here through a
// Playwright binding. FakeGo records every call, answers the ones the UI
// needs answers to, keeps a small world model so that renders look like the
// app's, and pushes events to the page.

const {
  buildView, clone, configShortcut, displayShortcut, find, launcher, leaves, normalise, pane, presetTree, shortcutRows, shortcutsOf,
  split, zoomedPane, SHORTCUTS,
} = require('./fixture.js');

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
    // The clipboard, which the app reads and writes through the Wails
    // runtime rather than a bound method, as FakeGo's ClipboardText and
    // ClipboardSetText.
    clipboard: {
      text() {
        return window.__BENCH_MOCK__.call('ClipboardText', []);
      },
      setText(text) {
        return window.__BENCH_MOCK__.call('ClipboardSetText', [text]);
      },
    },
  };
})();
`;

const SNAPSHOTS = {
  'zsh — ~/code/bench': '\x1b[38;5;246m~/code/bench on main\x1b[0m\r\n\x1b[38;5;214m$ \x1b[0mgo run ./cmd/bench\r\n\x1b[38;5;246mbench: 3 tabs loaded from config.toml\x1b[0m\r\n\x1b[38;5;246mbench: large screen, using wide layouts\x1b[0m\r\n\x1b[38;5;214m$ \x1b[0m',
  'go test — ~/code/bench': '\x1b[38;5;246m~/code/bench on main\x1b[0m\r\n\x1b[38;5;214m$ \x1b[0mgo test ./...\r\n\x1b[32mok   \x1b[0mbench/internal/layout    \x1b[38;5;246m(cached)\x1b[0m\r\n\x1b[32mok   \x1b[0mbench/internal/workspace \x1b[38;5;246m(cached)\x1b[0m\r\n\x1b[38;5;214m$ \x1b[0m',
};

// launcherName names a launcher saved without a name, as Go does: a
// terminal launcher after its command's program, or else its folder, or
// else Terminal, and an app launcher after its app.
// suggestPreset returns the layout that Go suggests for a tab of the
// windows of a number of apps.
function suggestPreset(apps) {
  if (apps <= 1) return 'single';
  if (apps === 2) return 'side';
  return apps === 3 ? 'three' : 'grid';
}

function launcherName(l) {
  if (l.kind === 'app' || l.bundleId) return l.app || l.bundleId;
  const word = (l.command || '').split(/\s+/).find((w) => w && !w.includes('='));
  const folder = (l.folder || '').replace(/\/+$/, '');
  return word ? word.split('/').pop() : (folder && folder !== '~' ? folder.split('/').pop() : 'Terminal');
}

// numbered returns name, numbered while taken says a launcher has it.
function numbered(name, taken) {
  for (let n = 1, base = name; taken(name); n++) name = base + ' (' + n + ')';
  return name;
}

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
    // termCleared holds where each terminal was cleared (TermClear): its
    // snapshot starts there, and offsets carry on.
    this.termCleared = new Map();
    // undo holds what Undo puts back, oldest first, as Go's undo history
    // does: each step has its id, the world as it was, and the toast
    // Undo shows.
    this.undo = [];
    this.undoSeq = 0;
    // closed holds the tabs that Reopen Closed Tab brings back, by name.
    this.closed = new Map();
    // split is the empty pane that mSplitPane made, with its tab as it was
    // before, for mCancelSplit.
    this.split = null;
    // clipboard is the clipboard's text (mClipboardText).
    this.clipboard = '';
    // browsed is what the user chooses in macOS's open panel (mBrowseToAdd).
    this.browsed = null;
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

  // view builds the view. A zoom that no longer applies, as the tab or
  // the focused pane has changed, is forgotten, as Go forgets it.
  view() {
    if (this.world.zoom && !zoomedPane(this.world)) this.world.zoom = null;
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
    this.switchTo(i);
    this.changed();
  }

  mStepTab(step) {
    const n = this.world.tabs.length;
    this.switchTo((this.world.current + step + n) % n);
    this.changed();
  }

  // mLastTab goes back to the tab used before the current one, as Go's
  // LastTab does, or says there's none yet.
  mLastTab() {
    const i = this.world.lastTab ? this.world.tabs.indexOf(this.world.lastTab) : -1;
    if (i < 0 || i === this.world.current) throw new Error('There\'s no tab to go back to yet. Switch tabs first.');
    this.switchTo(i);
    this.changed();
  }

  // mStepHold chooses the next tab of a hold that shows the tabs
  // (world.hold), or the one before, wrapping, or for a longer step, a
  // row's worth, the tab that many on, stopping at the ends, as Go's
  // StepHold does.
  mStepHold(step) {
    const h = this.world.hold;
    if (!h || !step) return;
    const n = h.tabs.length;
    const k = h.tabs.indexOf(h.selected);
    const to = Math.abs(step) === 1 ? ((k + step) % n + n) % n : Math.max(0, Math.min(k + step, n - 1));
    h.selected = h.tabs[to];
    this.changed();
  }

  // mCancelHold ends the hold without switching tabs, as Go's CancelHold
  // does.
  mCancelHold() {
    if (!this.world.hold) return;
    this.world.hold = null;
    this.changed();
  }

  // switchTo switches to tab i, as Go's switchLocked does: the tab it
  // leaves is the one to go back to (world.lastTab), what the new tab's
  // focused pane shows is in use now (use), and a hold ends.
  switchTo(i) {
    const w = this.world;
    w.hold = null;
    if (i !== w.current) w.lastTab = w.tabs[w.current];
    w.current = i;
    const p = find(w.tabs[i].root, w.tabs[i].focus);
    if (p) this.use(p.active || p.items[0]);
  }

  // use marks an item as the one used last, as Go's Model.Use does, for
  // the jump palette's order of windows (ItemView.used).
  use(id) {
    const it = this.world.items[id];
    if (it) it.used = ++this.world.used;
  }

  mSetMode(mode) {
    this.world.mode = mode;
    this.changed();
  }

  // mShowSideBySide is Show side by side in the "tabs" view, as Go's
  // ShowSideBySide does: it changes to the wide layout and shows the item
  // in its pane, focused, with Undo, which puts back the mode too. FakeGo
  // doesn't split the item's pane, nor do anything outside the "tabs"
  // view.
  mShowSideBySide(itemId) {
    const tab = this.currentTab();
    const p = leaves(tab.root).find((x) => x.items.includes(itemId));
    if (!p) throw new Error('That window isn\'t in a pane.');
    if (!this.tabsView()) return null;
    const was = this.world.mode;
    const name = { auto: 'Auto', compact: 'Compact' }[was] || 'Auto';
    const step = this.keepUndo(was === 'compact' ? 'Back to the compact layout.' : 'Back to Auto, which chooses the layout by the window\'s width.');
    this.undo[this.undo.length - 1].mode = was;
    this.world.mode = 'wide';
    p.active = itemId;
    tab.focus = p.id;
    this.use(itemId);
    const it = this.world.items[itemId];
    this.undoToast('Changed to the wide layout, to show ' + it.app + ' — ' + it.title + ' side by side. To go back, choose ' + name + ' at the right of the title bar.', step);
    this.changed();
    return null;
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

  // mFocusUI gives Bench's own UI the keyboard, as Go's FocusUI does: the
  // view doesn't change.
  mFocusUI() {
    return null;
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
    this.use(itemId);
    this.changed();
  }

  mSetSizes(splitId, sizes) {
    const s = find(this.currentTab().root, splitId);
    if (!s || s.type !== 'split' || sizes.length !== s.children.length) throw new Error('There\'s no such split.');
    const sum = sizes.reduce((a, b) => a + b, 0);
    s.sizes = sizes.map((v) => (v / sum) * 100);
    this.changed();
  }

  // mEvenSizes gives a split's panes equal sizes, as Go's EvenSizes does
  // for a double-click on a divider, with Undo. A split already even is
  // left as it is, without a toast.
  mEvenSizes(splitId) {
    const s = this.world.tabs.map((t) => find(t.root, splitId)).find(Boolean);
    if (!s || s.type !== 'split') throw new Error('There\'s no such split.');
    const even = normalise(null, s.children.length);
    if (normalise(s.sizes, s.children.length).every((v, i) => Math.abs(v - even[i]) < 0.05)) return null;
    const step = this.keepUndo('Put back the sizes of the panes.');
    s.sizes = even;
    this.undoToast('Made the panes the same size.', step);
    this.changed();
    return null;
  }

  // tabsView reports whether the current tab is shown in the "tabs" view.
  tabsView() {
    return this.view().display.compact;
  }

  // mZoomPane zooms a pane of the current tab, or the focused pane for '',
  // as Go's ZoomPane does: buildView shows it alone until the zoom ends.
  mZoomPane(paneId) {
    const tab = this.currentTab();
    if (this.tabsView()) throw new Error('On this screen, the tab already shows its windows as one pane.');
    const p = find(tab.root, paneId || tab.focus);
    if (!p || p.type !== 'pane') throw new Error('There\'s no such pane.');
    if (leaves(tab.root).length < 2) throw new Error('This tab has only one pane, so it fills the tab already.');
    tab.focus = p.id;
    this.world.zoom = { tab: this.world.current, pane: p.id };
    this.changed();
  }

  // mShowAllPanes ends the zoom.
  mShowAllPanes() {
    this.world.zoom = null;
    this.changed();
  }

  // mSplitPane puts an empty pane beside a pane, as Go's SplitPane does, as
  // layout.Split does it, focuses it, and returns its ID. It keeps the tab
  // as it was, for mCancelSplit.
  mSplitPane(paneId, zone) {
    const tab = this.currentTab();
    if (!find(tab.root, paneId)) throw new Error('There\'s no such pane.');
    if (this.tabsView()) throw new Error('Splits are hidden in the compact layout. Choose Wide to show them.');
    const edges = { left: ['row', true], right: ['row', false], top: ['column', true], bottom: ['column', false] };
    if (!edges[zone]) throw new Error('That isn\'t a place to put a window.');
    const [dir, before] = edges[zone];
    this.split = { tab, pane: '', root: clone(tab.root), focus: tab.focus, preset: tab.preset };
    const id = this.nextId('p');
    const fresh = pane(id, []);
    const parentOf = (n) => {
      if (n.type !== 'split') return null;
      return n.children.some((c) => c.id === paneId) ? n : n.children.map(parentOf).find(Boolean);
    };
    const parent = parentOf(tab.root);
    if (parent && parent.dir === dir) {
      const i = parent.children.findIndex((c) => c.id === paneId);
      const sizes = normalise(parent.sizes, parent.children.length);
      const at = before ? i : i + 1;
      sizes[i] /= 2;
      parent.children.splice(at, 0, fresh);
      sizes.splice(at, 0, sizes[i]);
      parent.sizes = sizes;
    } else {
      const p = find(tab.root, paneId);
      const s = split(this.nextId('s'), dir, [50, 50], before ? [fresh, clone(p)] : [clone(p), fresh]);
      if (parent) parent.children[parent.children.indexOf(p)] = s;
      else tab.root = s;
    }
    tab.focus = id;
    tab.preset = 'custom';
    this.split.pane = id;
    this.changed();
    return id;
  }

  // mCancelSplit puts the tab back as it was before mSplitPane, unless the
  // new pane has something in it now.
  mCancelSplit(paneId) {
    const s = this.split;
    if (!s || s.pane !== paneId) return null;
    this.split = null;
    const p = find(s.tab.root, paneId);
    if (!p || p.items.length) return null;
    Object.assign(s.tab, { root: s.root, focus: s.focus, preset: s.preset });
    this.changed();
    return null;
  }

  // mAddOptions lists the add dialog's rows, as Go does: the first Built in
  // row is a new terminal in the folder of the terminal that the pane, or
  // else the focused pane, shows, unless that's the home folder.
  mAddOptions(paneId) {
    const tab = this.currentTab();
    const here = this.hereOf(tab, paneId);
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
      ...(this.world.settings.launchers || []).map((l) => ({ group: 'Launchers', mono: l.mono, icon: l.icon, label: l.name, hint: l.hint, source: { type: 'launcher', id: l.name } })),
      ...this.world.floating.map((f) => ({ group: 'Windows not in a tab', mono: f.mono, icon: f.icon, label: f.title, hint: f.app, source: { type: 'floating', id: f.id } })),
      ...others.map((it) => ({ group: 'Windows in other tabs', mono: it.mono, icon: it.icon, label: it.title, hint: (it.tabs || [it.tab]).map((i) => this.world.tabs[i].name).join(' and ') + ' · ' + it.app, source: { type: 'item', id: it.id } })),
      ...apps.map((a) => ({ group: 'New window', mono: a.mono, icon: a.icon, label: 'New ' + a.app + ' window', hint: '', source: { type: 'new-window', bundleId: a.bundleId } })),
      ...(here && here !== '~' ? [{ group: 'Built in', mono: '>_', label: 'New terminal in ' + here, hint: 'Where the terminal here is', source: { type: 'terminal', folder: here } }] : []),
      { group: 'Built in', mono: '>_', label: 'New terminal', hint: '~', source: { type: 'terminal', folder: '' } },
      { group: 'Built in', mono: '>_', label: 'New terminal in…', hint: 'Choose a folder', source: { type: 'terminal', folder: here }, choosesFolder: true },
      { group: 'Built in', mono: '>_', label: 'New terminal…', hint: 'With a command, a name, or a folder', source: { type: 'terminal' }, asksForCommand: true },
      { group: 'Built in', mono: 'Wb', label: 'Web page…', hint: 'As a web app, or in Bench', source: { type: 'web' }, asksForUrl: true, webAppBrowser: 'Safari' },
      { group: 'Built in', mono: 'Br', label: 'Browse…', hint: 'An app, or a program to run in a terminal', source: { type: 'browse' }, browses: true },
      { group: 'Apps', mono: 'Ca', label: 'Calendar', hint: 'Open, or use its open windows', source: { type: 'app', bundleId: 'com.apple.iCal' } },
      { group: 'Apps', mono: 'Fi', label: 'Figma', hint: 'Open, or use its open windows', source: { type: 'app', bundleId: 'com.figma.Desktop' } },
      { group: 'Apps', mono: 'Li', label: 'Linear', hint: 'Open, or use its open windows', source: { type: 'app', bundleId: 'com.linear' } },
    ];
  }

  // hereOf returns the folder that the shell is in of the terminal that a
  // pane of tab shows, or else its focused pane does, as Go's hereLocked
  // does, or ''.
  hereOf(tab, paneId) {
    for (const id of [paneId, tab.focus]) {
      const p = leaves(tab.root).find((l) => l.id === id);
      const it = p && this.world.items[p.active];
      if (it && it.kind === 'builtin') return it.here || it.folder || '~';
    }
    return '';
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

  // webPage returns a Web item for a page control, or throws Go's refusal.
  webPage(itemId) {
    const it = this.world.items[itemId];
    if (!it) throw new Error('That window has closed.');
    if (it.kind !== 'web') throw new Error('Only web pages have page controls.');
    return it;
  }

  // mReloadPage counts reloads, as reloads.
  mReloadPage(itemId) {
    const it = this.webPage(itemId);
    it.reloads = (it.reloads || 0) + 1;
    return null;
  }

  // mGoBack and mGoForward go nowhere, as the browser preview's pages
  // have no history, but refuse as Go does while the page says it can't.
  mGoBack(itemId) {
    if (!this.webPage(itemId).canGoBack) throw new Error('There\'s no page to go back to.');
    return null;
  }

  mGoForward(itemId) {
    if (!this.webPage(itemId).canGoForward) throw new Error('There\'s no page to go forward to.');
    return null;
  }

  // mZoomPage steps a page's zoom through Go's zooms, in percent, which
  // the view leaves out at 100, and says so.
  mZoomPage(itemId, step) {
    const it = this.webPage(itemId);
    const levels = [50, 67, 75, 80, 90, 100, 110, 125, 150, 175, 200, 250, 300];
    const now = it.zoom || 100;
    let next = 100;
    if (step > 0) next = levels.find((l) => l > now);
    else if (step < 0) next = [...levels].reverse().find((l) => l < now);
    if (next === undefined) throw new Error(step > 0 ? 'The page is as large as Bench makes it.' : 'The page is as small as Bench makes it.');
    if (next === 100) delete it.zoom;
    else it.zoom = next;
    this.emit('bench:toast', next === 100 ? it.app + ' is at actual size.' : 'Zoomed ' + it.app + ' to ' + next + '%.');
    this.changed();
  }

  // mPageLink returns a page's address, as Copy Page Link copies it,
  // refusing VS Code for the Web's.
  mPageLink(itemId) {
    const it = this.webPage(itemId);
    if (it.folder) throw new Error('VS Code for the Web\'s address only works in Bench, so there\'s no link to copy.');
    if (!it.url) throw new Error('This page has no web address to copy.');
    return it.url;
  }

  // mSetFindBar opens or closes a page's find bar, as finding in its view.
  mSetFindBar(itemId, open) {
    const it = this.webPage(itemId);
    if (open) it.finding = true;
    else delete it.finding;
    this.changed();
  }

  // mFindInPage finds text in a page's text, ignoring case, which a test
  // sets as its pageText, or else in its title.
  mFindInPage(itemId, text) {
    const it = this.webPage(itemId);
    const page = String(it.pageText || it.title || '').toLowerCase();
    return Boolean(text) && page.includes(String(text).toLowerCase());
  }

  // mMuteCapture mutes or turns off a page's camera or microphone, or turns
  // it back on, as Go does through its web view, which reports the change.
  mMuteCapture(itemId, device, mute) {
    const it = this.webPage(itemId);
    if (device !== 'camera' && device !== 'microphone') throw new Error('That isn\'t the camera or the microphone.');
    if (!it[device]) throw new Error('The page isn\'t using the ' + device + '.');
    it[device] = mute ? 'muted' : 'active';
    const toasts = {
      'camera true': 'Turned off ' + it.app + '\'s camera.', 'camera false': 'Turned ' + it.app + '\'s camera back on.',
      'microphone true': 'Muted ' + it.app + '\'s microphone.', 'microphone false': 'Turned ' + it.app + '\'s microphone back on.',
    };
    this.emit('bench:toast', toasts[device + ' ' + Boolean(mute)]);
    this.changed();
  }

  // mStopMedia pauses a page's sound and video, and says so.
  mStopMedia(itemId) {
    const it = this.webPage(itemId);
    this.emit('bench:toast', 'Paused ' + it.app + '\'s sound and video.');
    return null;
  }

  // The config notices' buttons, and Reload config.toml. FakeGo has no
  // config.toml: reloading it, or using its backup, takes away the notices
  // that have buttons, as Go does once the file loads, and Overwrite the
  // ones that offer it.
  mReloadConfig() {
    this.world.notices = this.world.notices.filter((n) => typeof n === 'string' || !(n.actions || []).length);
    this.world.configWritable = true;
    this.emit('bench:toast', 'Reloaded config.toml');
    this.changed();
  }

  mUseConfigBackup() {
    this.mReloadConfig();
  }

  mOverwriteConfig() {
    this.world.notices = this.world.notices.filter((n) => typeof n === 'string' || !(n.actions || []).some((a) => a.method === 'OverwriteConfig'));
    this.changed();
  }

  // mOpenConfig and mShowConfigInFinder open nothing.
  mOpenConfig() {
    return null;
  }

  mShowConfigInFinder() {
    return null;
  }

  // The arrangement notice's buttons. FakeGo has no state.json: restoring
  // the arrangement, or dismissing the notice, takes away the notices that
  // offer them, as Go does once nothing is left to restore.
  mRestoreArrangement() {
    this.dropNotices('RestoreArrangement');
    this.emit('bench:toast', 'Restored the arrangement');
  }

  mDismissArrangementNotice() {
    this.dropNotices('DismissArrangementNotice');
  }

  // dropNotices takes away the notices with a button that calls method.
  dropNotices(method) {
    this.world.notices = this.world.notices.filter((n) => typeof n === 'string' || !(n.actions || []).some((a) => a.method === method));
    this.changed();
  }

  // mFillScreen does nothing, as there's no window to zoom.
  mFillScreen() {
    return null;
  }

  // mSetDialogSize remembers a dialog's size, as Go does, for the View's
  // dialogs.
  mSetDialogSize(kind, w, h) {
    if (!['palette', 'add', 'editor', 'settings', 'guide'].includes(kind)) throw new Error('There\'s no such dialog.');
    if (!(w > 0 && h > 0)) throw new Error('That isn\'t a size for a dialog.');
    this.world.dialogs = { ...this.world.dialogs, [kind]: { w: Math.round(w), h: Math.round(h) } };
    this.changed();
  }

  // mQuit is the question before quitting's Quit. FakeGo can't quit: it
  // records that it would have (world.quit), and with dontAsk turns the
  // question off, as Go does.
  mQuit(dontAsk) {
    if (dontAsk) this.world.settings.confirmQuitBusyTerminals = false;
    this.world.quit = true;
    this.changed();
  }

  // mQuitAndReopen is Quit and Reopen Bench. Unless sure, it asks first
  // while world.busyTerminals lists terminals that are running something,
  // as Go does, with the question's reopen set. FakeGo can't quit: it
  // records that it would have, and opened again (world.reopened).
  mQuitAndReopen(sure, dontAsk) {
    const busy = this.world.busyTerminals || [];
    if (!sure && busy.length && this.world.settings.confirmQuitBusyTerminals) {
      this.emit('bench:command', { name: 'confirm-quit', terminals: clone(busy), reopen: true });
      return null;
    }
    if (dontAsk) this.world.settings.confirmQuitBusyTerminals = false;
    this.world.reopened = true;
    this.changed();
    return null;
  }

  // mDismissFirstRun closes the welcome guide for good, as Go does.
  mDismissFirstRun() {
    this.world.firstRun = false;
    this.changed();
  }

  // mRequestScreenRecording asks for Screen Recording permission, which
  // FakeGo never gives, and offers Quit and Reopen Bench, as Go does once
  // it has asked.
  mRequestScreenRecording() {
    this.world.permissions.screenRecordingReopen = true;
    this.changed();
  }

  // mSetOpenAtLogin turns Open at login on or off. With
  // world.needsApproval, macOS needs the user to allow it, as Go then
  // says.
  mSetOpenAtLogin(on) {
    if (on && this.world.needsApproval) {
      this.world.openAtLogin = false;
      this.world.loginApproval = true;
      this.changed();
      throw new Error('macOS needs you to allow Bench in System Settings ▸ General ▸ Login Items. Bench has opened it: turn on Bench there.');
    }
    this.world.openAtLogin = on;
    this.world.loginApproval = false;
    this.changed();
  }

  // mOpenLoginItems would open System Settings at Login Items.
  mOpenLoginItems() {
    return null;
  }

  // mStarterTabs returns the tabs to suggest for the welcome guide's
  // starting set-up, leaving out those whose name a tab has, as Go does.
  mStarterTabs() {
    const taken = (n) => this.world.tabs.some((t) => t.name.toLowerCase() === n.toLowerCase());
    return clone((this.world.starterTabs || []).filter((t) => !taken(t.name)));
  }

  // mUseStarterTabs adds the suggested tabs it names after the others, one
  // pane for each app, with a toast, as Go does. A name it doesn't suggest
  // is refused, and nothing is added.
  mUseStarterTabs(names) {
    const suggested = this.mStarterTabs();
    const tabs = (names || []).map((name) => {
      const t = suggested.find((s) => s.name.toLowerCase() === String(name).toLowerCase());
      if (!t) throw new Error('Bench doesn\'t suggest a tab called ' + name + ' any more.');
      return t;
    });
    for (const t of tabs) {
      const panes = t.apps.map((a) => {
        const id = this.nextId('w');
        this.world.items[id] = a.bundleId
          ? { id, kind: 'app', app: a.name, mono: a.mono, title: a.name, status: 'ready', bundleId: a.bundleId }
          : { id, kind: 'builtin', app: 'Terminal', mono: '>_', title: 'zsh — ~', status: 'ready', folder: '~' };
        return pane(this.nextId('p'), [id]);
      });
      const root = panes.length === 1 ? panes[0] : split(this.nextId('s'), 'row', [50, 50], panes);
      this.world.tabs.push({ name: t.name, preset: panes.length === 1 ? 'single' : 'side', smallScreens: 'tabs', focus: panes[0].id, root });
    }
    if (tabs.length) this.emit('bench:toast', 'Added ' + tabs.map((t) => t.name).join(' and ') + '.');
    this.changed();
  }

  // mDismissUpdate takes away the notice of a newer version.
  mDismissUpdate() {
    this.world.notices = this.world.notices.filter((n) => !(typeof n === 'object' && (n.actions || []).some((a) => a.method === 'DismissUpdate')));
    this.changed();
  }

  // mInstallUpdate is the notice's Install…, which shows Sparkle's window
  // for the newer version. There's no Sparkle here, so it does nothing.
  mInstallUpdate() {}

  // mCloseTerminal is the question before closing a popped-out terminal's
  // window's Close: the terminal goes from wherever it is, as Go removes
  // it, and with dontAsk the question is turned off, which a toast says.
  mCloseTerminal(itemId, dontAsk) {
    const it = this.world.items[itemId] || this.world.floating.find((f) => f.id === itemId);
    if (!it || it.kind !== 'builtin') throw new Error('That terminal has closed.');
    delete this.world.items[itemId];
    this.world.floating = this.world.floating.filter((f) => f.id !== itemId);
    for (const t of this.world.tabs) {
      for (const p of leaves(t.root)) {
        p.items = p.items.filter((id) => id !== itemId);
        if (p.active === itemId) p.active = p.items[0] || '';
      }
    }
    if (dontAsk) {
      this.world.settings.confirmQuitBusyTerminals = false;
      this.emit('bench:toast', 'Bench won\'t ask again before ending a terminal that\'s running something. Settings can turn this back on.');
    }
    this.changed();
  }

  // tabsHolding returns the indexes of the tabs that hold an item.
  tabsHolding(itemId) {
    return this.world.tabs.map((t, i) => (leaves(t.root).some((p) => p.items.includes(itemId)) ? i : -1)).filter((i) => i >= 0);
  }

  // busyTerminal describes a terminal that's running something, as Go's
  // BusyTerminal does, or returns null: world.busy holds what each busy
  // terminal runs, by item ID.
  busyTerminal(itemId) {
    const running = (this.world.busy || {})[itemId];
    const it = this.world.items[itemId];
    if (!running || !it || it.kind !== 'builtin') return null;
    const ti = this.tabsHolding(itemId)[0];
    return { id: itemId, title: it.title, tab: ti >= 0 ? this.world.tabs[ti].name : '', running };
  }

  // dropItem forgets an item, from every pane and Not in a tab, as Go's
  // removeItemLocked does.
  dropItem(itemId) {
    delete this.world.items[itemId];
    this.world.floating = this.world.floating.filter((f) => f.id !== itemId);
    for (const t of this.world.tabs) {
      for (const p of leaves(t.root)) {
        p.items = p.items.filter((id) => id !== itemId);
        if (p.active === itemId) p.active = p.items[0] || '';
      }
    }
  }

  // closeQuestion works out what closing tab index closes, as Go's
  // CloseQuestion says.
  closeQuestion(index) {
    const tab = this.world.tabs[index];
    const q = { ask: false, tab: tab.name, only: this.world.tabs.length === 1, windows: 0, pages: 0, terminals: 0, stay: 0, listed: 0, rules: 0, busy: [] };
    for (const id of leaves(tab.root).flatMap((p) => p.items)) {
      const it = this.world.items[id];
      if (!it) continue;
      if (this.tabsHolding(id).length > 1) {
        q.stay++;
        continue;
      }
      if (it.kind === 'app' && !it.noWindow) q.windows++;
      else if (it.kind === 'web') q.pages++;
      else if (it.kind === 'builtin') q.terminals++;
      const busy = this.busyTerminal(id);
      if (busy) q.busy.push(busy);
      if (!it.noWindow) q.listed++;
    }
    if (!q.only) q.rules = (this.world.rules || []).filter((r) => r.tab === tab.name).length;
    return q;
  }

  // mCloseTab closes a tab's windows, pages, and terminals, other than
  // those in other tabs too, and deletes it; the only tab stays, empty.
  // Unless sure, with confirm_close on, it asks first for two or more
  // things, or a busy terminal, and with it off, still for a busy terminal,
  // with confirm_quit_busy_terminals on, as Go does; Don't ask again turns
  // off the setting that asked. FakeGo's windows close at once.
  mCloseTab(index, sure, dontAsk) {
    const tab = this.world.tabs[index];
    if (!tab) throw new Error('There\'s no tab ' + (index + 1) + '.');
    const q = this.closeQuestion(index);
    const { settings } = this.world;
    const confirm = settings.confirmClose !== false;
    if (q.busy.length && !confirm && settings.confirmQuitBusyTerminals !== false) q.busyOnly = true;
    if (!sure && ((confirm && (q.windows + q.pages + q.terminals >= 2 || q.busy.length)) || q.busyOnly)) {
      return { ...q, ask: true };
    }
    for (const id of leaves(tab.root).flatMap((p) => p.items)) {
      if (this.tabsHolding(id).length === 1) this.dropItem(id);
    }
    if (q.only) {
      for (const p of leaves(tab.root)) p.active = '';
    } else {
      this.world.tabs.splice(index, 1);
      if (index < this.world.current || this.world.current >= this.world.tabs.length) this.world.current = Math.max(0, this.world.current - 1);
    }
    let msg = q.only ? 'Closed everything in ' + tab.name + '.' : 'Closed ' + tab.name + '.';
    if (dontAsk && q.busyOnly) {
      settings.confirmQuitBusyTerminals = false;
      msg += ' Bench won\'t ask again before ending a terminal that\'s running something. Settings can turn this back on.';
    } else if (dontAsk) {
      settings.confirmClose = false;
      msg += ' Bench won\'t ask again before closing a tab\'s windows. Settings can turn this back on.';
    }
    this.emit('bench:toast', msg);
    this.changed();
    return q;
  }

  // mCloseItem closes an item, from every tab. Unless sure, a busy
  // terminal asks first, with confirm_quit_busy_terminals on, as Go does.
  mCloseItem(itemId, sure, dontAsk) {
    const it = this.world.items[itemId] || this.world.floating.find((f) => f.id === itemId);
    if (!it) throw new Error('That window has closed.');
    const busy = this.busyTerminal(itemId);
    const q = { ask: false, windows: it.kind === 'app' ? 1 : 0, pages: it.kind === 'web' ? 1 : 0, terminals: it.kind === 'builtin' ? 1 : 0, busy: busy ? [busy] : [] };
    if (!sure && busy && this.world.settings.confirmQuitBusyTerminals !== false) return { ...q, ask: true };
    this.dropItem(itemId);
    if (dontAsk) {
      this.world.settings.confirmQuitBusyTerminals = false;
      this.emit('bench:toast', 'Bench won\'t ask again before ending a terminal that\'s running something. Settings can turn this back on.');
    }
    this.changed();
    return q;
  }

  // keepUndo remembers the world before a change, for mUndo, and returns
  // the step's number for the toast's Undo.
  keepUndo(undone) {
    const w = this.world;
    this.undoSeq += 1;
    this.undo.push({
      id: this.undoSeq, undone,
      tabs: clone(w.tabs), current: w.current, items: clone(w.items), floating: clone(w.floating), closedTabs: w.closedTabs.slice(),
    });
    return this.undoSeq;
  }

  // undoToast sends a toast with Undo, for step.
  undoToast(text, step) {
    return this.emit('bench:toast', { text, action: { label: 'Undo', method: 'Undo', arg: step } });
  }

  // mDeleteTab deletes a tab and keeps its windows, which go to Not in a
  // tab, as Go's DeleteTab does. Its toast has Undo, and Reopen Closed Tab
  // can bring it back.
  mDeleteTab(index) {
    const tab = this.world.tabs[index];
    if (!tab) throw new Error('There\'s no tab ' + (index + 1) + '.');
    if (this.world.tabs.length === 1) throw new Error('Bench needs at least one tab.');
    const step = this.keepUndo('Brought back ' + tab.name + '.');
    for (const id of leaves(tab.root).flatMap((p) => p.items)) {
      if (this.tabsHolding(id).length > 1 || !this.world.items[id]) continue;
      this.world.floating.push({ ...this.world.items[id], tab: -1 });
      delete this.world.items[id];
    }
    this.world.tabs.splice(index, 1);
    if (index < this.world.current || this.world.current >= this.world.tabs.length) this.world.current = Math.max(0, this.world.current - 1);
    this.closed.set(tab.name, { tab: clone(tab), index });
    this.world.closedTabs = [tab.name, ...this.world.closedTabs.filter((n) => n !== tab.name)].slice(0, 5);
    this.undoToast('Deleted ' + tab.name + '. Its windows are in Jump to… under “Not in a tab”.', step);
    this.changed();
  }

  // mResetLayout puts a tab's windows back in its preset's layout, as Go
  // does; FakeGo leaves the tree as it is. Its toast has Undo.
  mResetLayout(index) {
    const tab = this.world.tabs[index];
    if (!tab) throw new Error('There\'s no tab ' + (index + 1) + '.');
    const step = this.keepUndo('Put back the layout of ' + tab.name + '.');
    this.undoToast('Reset the layout of ' + tab.name, step);
    this.changed();
  }

  // mMoveItemAt puts an item at index among a pane's sub-tabs as drawn,
  // as Go's MoveItemAt does: a place next to where it is already does
  // nothing. In the "tabs" view's pane, it goes into the pane of the
  // sub-tab it goes before, or of the last; Go keeps it in its own pane
  // when it can. Its toast has Undo.
  mMoveItemAt(itemId, index, paneId, at) {
    const tab = this.world.tabs[index];
    if (!tab) throw new Error('There\'s no tab ' + (index + 1) + '.');
    const it = this.world.items[itemId];
    if (!it) throw new Error('That window has gone.');
    const panes = leaves(tab.root);
    const target = paneId === 'compact' ? null : panes.find((p) => p.id === paneId);
    if (paneId !== 'compact' && !target) throw new Error('That pane has gone.');
    const strip = (target ? target.items : panes.flatMap((p) => p.items)).filter((id) => !(this.world.items[id] && this.world.items[id].hidden));
    const place = Math.min(Math.max(at, 0), strip.length);
    const k = strip.indexOf(itemId);
    if (k >= 0 && (place === k || place === k + 1)) return null;
    const label = it.app + ' — ' + it.title;
    const step = this.keepUndo('Moved ' + label + ' back.');
    const before = place < strip.length ? strip[place] : '';
    for (const p of panes) {
      p.items = p.items.filter((id) => id !== itemId);
      if (p.active === itemId) p.active = p.items[0] || '';
    }
    const into = target || panes.find((p) => p.items.includes(before || strip[strip.length - 1])) || panes[0];
    const i = before && into.items.includes(before) ? into.items.indexOf(before) : into.items.length;
    into.items.splice(i, 0, itemId);
    into.active = itemId;
    this.undoToast('Moved ' + label + '.', step);
    this.changed();
    return null;
  }

  // mMoveTab moves a tab to another place among the tabs, as Go's MoveTab
  // does: the current tab stays current, and the shortcuts follow the
  // order. Its toast has Undo.
  mMoveTab(from, to) {
    const w = this.world;
    const tab = w.tabs[from];
    if (!tab) throw new Error('There\'s no tab ' + (from + 1) + '.');
    if (to < 0) throw new Error(tab.name + ' is the first tab already.');
    if (to >= w.tabs.length) throw new Error(tab.name + ' is the last tab already.');
    if (to === from) return null;
    const step = this.keepUndo('Moved ' + tab.name + ' back.');
    const order = w.tabs.map((_, i) => i);
    order.splice(to, 0, ...order.splice(from, 1));
    w.tabs = order.map((i) => w.tabs[i]);
    w.current = order.indexOf(w.current);
    const shown = {};
    order.forEach((old, i) => { if (w.compactShown[old] !== undefined) shown[i] = w.compactShown[old]; });
    w.compactShown = shown;
    const n = to + 1;
    const suffix = n % 100 >= 11 && n % 100 <= 13 ? 'th' : { 1: 'st', 2: 'nd', 3: 'rd' }[n % 10] || 'th';
    const keys = w.keys.tabs && n <= 9 ? ' (' + w.keys.tabs + n + ')' : '';
    this.undoToast('Moved ' + tab.name + ' to ' + n + suffix + ' place' + keys + '.', step);
    this.changed();
    return null;
  }

  // mMoveItemToNewTab adds a tab with the single layout after the others,
  // named after an item's app, numbered if a tab has that name, and moves
  // the item there from the tab it's shown in, or with also shows it there
  // as well, then switches to it, as Go's MoveItemToNewTab does. A
  // popped-out item comes back into Bench, but can't also be shown. Its
  // toast has Undo, which takes the tab away again.
  mMoveItemToNewTab(itemId, also) {
    const w = this.world;
    const floating = w.floating.find((f) => f.id === itemId);
    const it = w.items[itemId] || floating;
    if (!it) throw new Error('That window has closed.');
    if (also && floating) {
      throw new Error(w.settings.dragToAdd ? 'That window is out of Bench. To add it back, drag it onto a pane and wait a moment, or use +.'
        : 'That window is out of Bench. Add it to a pane with +.');
    }
    const taken = (n) => w.tabs.some((t) => t.name.toLowerCase() === n.toLowerCase());
    let name = it.app;
    for (let n = 1; taken(name); n++) name = it.app + ' (' + n + ')';
    const label = it.title && it.title !== it.app ? it.app + ' — ' + it.title : it.app;
    const step = this.keepUndo(also ? 'Deleted the new tab, ' + name + '.' : 'Moved ' + label + ' back.');
    if (floating) {
      w.floating = w.floating.filter((f) => f !== floating);
      const { tab: _, ...rest } = floating;
      w.items[itemId] = rest;
    } else if (!also) {
      const holding = this.tabsHolding(itemId);
      const from = w.tabs[holding.includes(w.current) ? w.current : holding[0]];
      for (const p of leaves(from.root)) {
        p.items = p.items.filter((id) => id !== itemId);
        if (p.active === itemId) p.active = p.items[0] || '';
      }
      from.preset = 'custom';
    }
    const id = this.nextId('p');
    w.tabs.push({ name, preset: 'single', smallScreens: 'stack', focus: id, root: pane(id, [itemId]) });
    w.current = w.tabs.length - 1;
    this.undoToast(also ? 'Showing ' + label + ' in a new tab, ' + name + ', too.' : 'Moved ' + label + ' to a new tab, ' + name + '.', step);
    this.changed();
  }

  // windowGroups groups windows, by their IDs in floating and items, by
  // app, in the order the apps first come, as Go's windowGroupsLocked
  // does: by bundle ID, or a page's or terminal's kind and name. Windows
  // that have gone, hidden ones, and repeats are left out.
  windowGroups(ids) {
    const w = this.world;
    const groups = [];
    for (const id of new Set(ids || [])) {
      const it = w.items[id] || w.floating.find((f) => f.id === id);
      if (!it || it.hidden) continue;
      const key = it.bundleId ? it.bundleId.toLowerCase() : it.kind + ':' + String(it.app).toLowerCase();
      let g = groups.find((x) => x.key === key);
      if (!g) {
        g = { key, name: it.app, ids: [] };
        groups.push(g);
      }
      g.ids.push(id);
    }
    return groups;
  }

  // windowsTree returns a tab of a preset's panes with the windows of
  // groups in them, each app's in a pane of its own and the apps after the
  // last pane in it, as Go's fillPanes does.
  windowsTree(preset, groups) {
    const root = presetTree(null, preset, (p) => this.nextId(p));
    const panes = leaves(root);
    groups.forEach((g, i) => panes[Math.min(i, panes.length - 1)].items.push(...g.ids));
    panes.forEach((p) => { p.active = p.items[0] || ''; });
    return root;
  }

  // mPreviewTabFromWindows previews a tab made of windows that are open,
  // with the layout suggested by the number of apps, as Go does.
  mPreviewTabFromWindows(preset, ids) {
    const groups = this.windowGroups(ids);
    const suggested = suggestPreset(groups.length);
    return { suggested, apps: groups.length, root: this.windowsTree(preset || suggested, groups) };
  }

  // mNewTabFromWindows makes a tab of windows that are open, as Go's
  // NewTabFromWindows does: those not in a tab move there, and those of
  // tabs show there as well. Bench switches to it. Its toast has Undo,
  // which takes the tab away and puts the windows back.
  mNewTabFromWindows(draft, ids) {
    const w = this.world;
    const groups = this.windowGroups(ids);
    if (!groups.length) throw new Error(ids && ids.length ? 'Those windows have closed.' : 'Tick at least one window.');
    const taken = (n) => w.tabs.some((t) => t.name.toLowerCase() === n.toLowerCase());
    const typed = String(draft.name || '').trim() || groups[0].name;
    const name = numbered(typed, taken);
    const preset = draft.preset || suggestPreset(groups.length);
    const step = this.keepUndo('Deleted the new tab, ' + name + ', and put its windows back.');
    for (const g of groups) {
      for (const id of g.ids) {
        const floating = w.floating.find((f) => f.id === id);
        if (!floating) continue;
        w.floating = w.floating.filter((f) => f !== floating);
        const { tab: _, ...rest } = floating;
        w.items[id] = rest;
      }
    }
    const root = this.windowsTree(preset, groups);
    const tab = { name, preset, smallScreens: draft.smallScreens || 'stack', focus: leaves(root)[0].id, root };
    const at = draft.place > 0 ? Math.min(draft.place, w.tabs.length + 1) - 1 : w.tabs.length;
    w.tabs.splice(at, 0, tab);
    w.current = at;
    const n = groups.reduce((k, g) => k + g.ids.length, 0);
    this.undoToast('Made a new tab, ' + name + ', with ' + (n === 1 ? '1 window' : n + ' windows') + '.', step);
    this.changed();
  }

  // mAddWindowsToPane adds the windows ticked in the add dialog to a pane
  // of the current tab, in order, and shows the first, as Go's
  // AddWindowsToPane does.
  mAddWindowsToPane(paneId, ids) {
    const w = this.world;
    const tab = this.currentTab();
    const p = paneId === 'compact' ? find(tab.root, tab.focus) || leaves(tab.root)[0] : leaves(tab.root).find((l) => l.id === paneId);
    if (!p) throw new Error('There\'s no such pane.');
    if (!ids || !ids.length) throw new Error('Tick at least one window.');
    let first = '';
    for (const id of ids) {
      const floating = w.floating.find((f) => f.id === id);
      if (floating) {
        w.floating = w.floating.filter((f) => f !== floating);
        const { tab: _, ...rest } = floating;
        w.items[id] = rest;
      } else if (!w.items[id] || leaves(tab.root).some((l) => l.items.includes(id))) {
        continue;
      }
      p.items.push(id);
      first = first || id;
    }
    if (!first) throw new Error('That window has closed.');
    p.active = first;
    tab.focus = p.id;
    this.changed();
  }

  // mAddFrontWindow adds the window in front before Bench (world.front),
  // which is in no tab, to the current tab's focused pane, as Go's
  // AddFrontWindow does.
  mAddFrontWindow() {
    const w = this.world;
    const floating = w.front ? w.floating.find((f) => f.id === w.front) : null;
    if (!floating) throw new Error('There\'s no window to add: the window you used last before Bench is in a tab already, or a rule keeps it floating. Click the window you want to add, then try again.');
    this.dock(floating);
  }

  // mBackToBench puts a popped-out item back in the current tab's focused
  // pane, as Go's BackToBench does. An item in a tab stays where it is.
  mBackToBench(itemId) {
    const floating = this.world.floating.find((f) => f.id === itemId);
    if (floating) this.dock(floating);
  }

  // dock adds a window that's in no tab to the current tab's focused pane
  // and shows it there, with a toast whose Pop out takes it out again, as
  // Go's dockAndSayLocked does.
  dock(floating) {
    const w = this.world;
    const tab = this.currentTab();
    const p = find(tab.root, tab.focus) || leaves(tab.root)[0];
    w.floating = w.floating.filter((f) => f !== floating);
    const { tab: _, ...rest } = floating;
    w.items[floating.id] = rest;
    p.items.push(floating.id);
    p.active = floating.id;
    if (w.front === floating.id) w.front = '';
    const label = rest.title && rest.title !== rest.app ? rest.app + ' — ' + rest.title : rest.app;
    this.emit('bench:toast', { text: 'Added ' + label + ' to this pane.', action: { label: 'Pop out', method: 'PopOut', arg: 0, item: floating.id } });
    this.changed();
  }

  // mRenameItem gives a terminal or web page a name of its own, which its
  // sub-tab shows as its title, or with '' takes it away, as Go's
  // RenameItem does: the view has the name as label, and the title it
  // hides as autoTitle. An app's window keeps its app's title.
  mRenameItem(itemId, label) {
    const it = this.world.items[itemId];
    if (!it) throw new Error('That window has closed.');
    if (it.kind === 'app') throw new Error('An app\'s window takes its title from the app. Only terminals and web pages can be renamed.');
    const name = String(label).split(/\s+/).filter(Boolean).join(' ');
    const auto = it.label ? it.autoTitle : it.title;
    if (name) {
      Object.assign(it, { title: name, label: name, autoTitle: auto });
    } else {
      it.title = auto;
      delete it.label;
      delete it.autoTitle;
    }
    this.changed();
  }

  // mUndo puts the world back as it was before step, undoing it and every
  // later step, as Go's Undo does.
  mUndo(step) {
    const at = this.undo.findIndex((s) => s.id === step);
    if (at < 0) throw new Error('Bench can\'t undo that any more.');
    const steps = this.undo.slice(at);
    const s = steps[0];
    this.undo.length = at;
    Object.assign(this.world, { tabs: s.tabs, current: s.current, items: s.items, floating: s.floating, closedTabs: s.closedTabs });
    // A step that changed the mode, the first of those undone, has the
    // mode from before it.
    const moded = steps.find((x) => x.mode);
    if (moded) this.world.mode = moded.mode;
    for (const name of this.closed.keys()) {
      if (!s.closedTabs.includes(name)) this.closed.delete(name);
    }
    this.emit('bench:toast', s.undone);
    this.changed();
  }

  // mReopenTab brings back a tab that was deleted or closed, where it was,
  // and switches to it, as Go's ReopenTab does: its windows that are Not
  // in a tab come back to it, and those now in another tab stay there. A
  // tab given only by name, in world.closedTabs, comes back empty.
  mReopenTab(name) {
    if (!this.world.closedTabs.includes(name)) throw new Error('Bench no longer has that tab to reopen.');
    let c = this.closed.get(name);
    if (!c) {
      const id = this.nextId('p');
      c = { tab: { name, preset: 'single', smallScreens: 'stack', focus: id, root: pane(id, []) }, index: this.world.tabs.length };
    }
    const index = Math.min(c.index, this.world.tabs.length);
    const tab = clone(c.tab);
    for (const p of leaves(tab.root)) {
      p.items = p.items.filter((id) => {
        const f = this.world.floating.find((x) => x.id === id);
        if (!f) return false;
        this.world.floating = this.world.floating.filter((x) => x !== f);
        const { tab: _, ...it } = f;
        this.world.items[id] = it;
        return true;
      });
      if (!p.items.includes(p.active)) p.active = p.items[0] || '';
    }
    this.world.tabs.splice(index, 0, tab);
    this.world.current = index;
    this.closed.delete(name);
    this.world.closedTabs = this.world.closedTabs.filter((n) => n !== name);
    this.undo.length = 0;
    this.emit('bench:toast', 'Reopened ' + name + '.');
    this.changed();
  }

  // mSaveSettings applies the Settings dialog's settings, as Go does. Its
  // launchers, if it sends them, replace the ones there were, and those
  // without a name are named as mSaveLauncher names them. Its shortcuts, if
  // it sends any, change those fields of the [shortcuts] table, unless two
  // fields would have the same shortcut.
  mSaveSettings(settings) {
    if (settings.shortcuts) {
      const clash = this.mCheckShortcuts(settings.shortcuts).find((r) => / is already the shortcut for /.test(r.problem || ''));
      if (clash) throw new Error('Bench can\'t save that shortcut: ' + clash.problem);
      for (const [field, value] of Object.entries(settings.shortcuts)) {
        this.world.keys[SHORTCUTS.find((f) => f.field === field).key] = displayShortcut(value);
      }
    }
    const next = { ...this.world.settings, ...settings };
    delete next.shortcuts;
    if (settings.launchers) {
      const ls = settings.launchers.map((l) => ({ ...l, name: (l.name || '').trim(), app: this.appName(l.bundleId) }));
      const taken = (n) => ls.some((l) => l.name.toLowerCase() === n.toLowerCase());
      for (const l of ls) if (!l.name) l.name = numbered(launcherName(l), taken);
      next.launchers = ls.map((l) => launcher(l));
    } else next.launchers = this.world.settings.launchers;
    this.world.settings = next;
    this.changed();
  }

  // mCheckShortcuts checks Settings ▸ Keyboard's draft, roughly as Go
  // does: a field with the same shortcut as a field before it, or the same
  // modifiers for the same keys, has a problem, as does a shortcut in
  // world.takenShortcuts (in config form), which another app has, with ⌘
  // added as the suggestion. A field the draft leaves as it is has what
  // world.keyStates says about it.
  mCheckShortcuts(changes) {
    const rows = shortcutRows(this.world);
    const values = Object.fromEntries(rows.map((r) => [r.field, r.value]));
    for (const [field, value] of Object.entries(changes || {})) {
      if (!(field in values)) throw new Error('Bench has no shortcut called ' + field + '.');
      values[field] = value;
    }
    return rows.map((r, i) => {
      const value = values[r.field];
      const out = { field: r.field, label: r.label, ...(r.keys ? { keys: r.keys } : {}), value, ...(value ? { display: displayShortcut(value) } : {}), default: r.default };
      if (!value) return out;
      const mine = shortcutsOf(r.field, value);
      for (const o of rows.slice(0, i)) {
        const shared = shortcutsOf(o.field, values[o.field]).find((x) => mine.includes(x));
        if (shared) return { ...out, problem: displayShortcut(shared) + ' is already the shortcut for ' + o.label + '.' };
      }
      if ((this.world.takenShortcuts || []).includes(value)) {
        const suggestion = configShortcut('⌘' + displayShortcut(value));
        return { ...out, problem: 'Bench couldn\'t register ' + displayShortcut(value) + ': another app might be using it.', suggestion, suggestionDisplay: displayShortcut(suggestion) };
      }
      if (value === r.value && r.problem) {
        return { ...out, problem: r.problem, ...(r.suggestion ? { suggestion: r.suggestion, suggestionDisplay: r.suggestionDisplay } : {}) };
      }
      return out;
    });
  }

  // mPauseShortcuts notes whether Settings ▸ Keyboard has Bench's global
  // shortcuts released, to record one (world.keysPaused). Closing the last
  // menu or dialog registers them again, as Go does.
  mPauseShortcuts(paused) {
    this.world.keysPaused = paused;
  }

  mSetOverlay(open) {
    if (!open) this.world.keysPaused = false;
  }

  // mUseSuggestedShortcuts takes world.keyStates' suggestions for the
  // fields, as a notice's Use this instead has Go do, and takes away the
  // notices about them.
  mUseSuggestedShortcuts(fields) {
    const changed = [];
    for (const field of String(fields || '').split(/\s+/).filter(Boolean)) {
      const row = shortcutRows(this.world).find((r) => r.field === field);
      if (!row || !row.suggestion) continue;
      this.world.keys[SHORTCUTS.find((f) => f.field === field).key] = displayShortcut(row.suggestion);
      changed.push(row);
    }
    if (!changed.length) throw new Error('Bench has no other shortcut to suggest now. Choose one in Settings ▸ Keyboard.');
    this.world.notices = this.world.notices.filter((n) => typeof n === 'string' || !(n.actions || []).some((a) => a.method === 'UseSuggestedShortcuts'));
    const one = changed.length === 1 ? changed[0] : null;
    this.emit('bench:toast', one ? 'Changed ' + one.label + ' to ' + one.suggestionDisplay + (one.keys ? ' with ' + one.keys : '') : 'Changed the shortcuts');
    this.changed();
  }

  // appName returns the name of the app with a bundle ID, from its windows,
  // or the bundle ID if it has none.
  appName(bundleId) {
    const it = Object.values(this.world.items).concat(this.world.floating).find((x) => x.bundleId === bundleId);
    return it ? it.app : bundleId;
  }

  // mSaveLauncher adds a launcher after the others, as the add dialog's
  // Save as launcher has Go do, and returns its name: one without a name is
  // named after its command's program, or its folder, numbered if that's
  // taken. A name that's taken, ignoring case, is refused, naming the
  // launcher that has it.
  mSaveLauncher(v) {
    const launchers = this.world.settings.launchers || [];
    const holder = (n) => launchers.find((l) => l.name.toLowerCase() === n.toLowerCase());
    const taken = (n) => Boolean(holder(n));
    let name = (v.name || '').trim();
    if (name && taken(name)) throw new Error('There\'s a launcher called ' + holder(name).name + ' already. Give this one another name.');
    if (!name) name = numbered(launcherName(v), taken);
    this.world.settings = { ...this.world.settings, launchers: [...launchers, launcher({ ...v, name })] };
    this.emit('bench:toast', 'Saved launcher ' + name);
    this.changed();
    return name;
  }

  // mClipboardText and mClipboardSetText stand in for the Wails runtime's
  // clipboard, which a terminal's Copy and Paste use.
  mClipboardText() {
    return this.clipboard;
  }

  mClipboardSetText(text) {
    this.clipboard = String(text);
  }

  // mBrowseToAdd answers as the user cancelling macOS's open panel. Tests
  // set browsed to what the user chooses instead: { added } for an app,
  // which Go adds to the pane, or { command, name, folder } for a program.
  // The browser preview has no panel, as Go says.
  mBrowseToAdd() {
    if (this.world.demo) throw new Error('The browser preview can\'t look for apps or programs on this computer. In the Mac app, Browse… finds one on your Mac.');
    const chosen = this.browsed;
    if (!chosen) return {};
    return chosen.added ? { added: chosen.added } : { command: chosen.command, name: chosen.name || '', folder: chosen.folder || '' };
  }

  // mChooseFolder answers as the user choosing ~/code/bench in macOS's
  // folder picker. The browser preview has no picker, as Go says.
  mChooseFolder() {
    if (this.world.demo) throw new Error('This build of Bench can\'t show a folder picker. Type the folder instead, such as ~/code/bench.');
    return '~/code/bench';
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
    const data = bytes.subarray(this.termCleared.get(itemId) || 0);
    return { data: data.toString('base64'), offset: bytes.length };
  }

  mTermClear(itemId) {
    this.termCleared.set(itemId, this.termBytes(itemId).length);
  }
}

module.exports = { FakeGo, MOCK_SCRIPT };
