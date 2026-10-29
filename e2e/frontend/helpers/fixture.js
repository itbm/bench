'use strict';

// View fixtures for the frontend tests: a small model of Bench's world, and
// buildView, which turns it into a workspace.View as Go would, computing
// pane frames with the spec's formula (docs/spec.md, "Layout maths").

// round is floor(v + 0.5), with the same allowance for float error as Go.
const round = (v) => Math.floor(v + 0.5 + 1e-9);

// normalise returns n sizes adding up to 100, or equal sizes if sizes is
// unusable.
function normalise(sizes, n) {
  if (!Array.isArray(sizes) || sizes.length !== n || sizes.some((s) => !(s > 0) || !Number.isFinite(s))) {
    return Array.from({ length: n }, () => 100 / n);
  }
  const sum = sizes.reduce((a, b) => a + b, 0);
  return sizes.map((s) => (s / sum) * 100);
}

// childUnits divides a split's unit rectangle among its children.
function childUnits(node, u) {
  const sizes = normalise(node.sizes, node.children.length);
  let sum = 0;
  return sizes.map((s, i) => {
    const start = sum / 100;
    sum += s;
    const end = i === sizes.length - 1 ? 1 : sum / 100;
    return node.dir === 'column'
      ? { x: u.x, y: u.y + start * u.h, w: u.w, h: (end - start) * u.h }
      : { x: u.x + start * u.w, y: u.y, w: (end - start) * u.w, h: u.h };
  });
}

function visit(node, u, fn) {
  fn(node, u);
  if (node.type !== 'split') return;
  childUnits(node, u).forEach((cu, i) => visit(node.children[i], cu, fn));
}

// frame turns a unit rectangle into a frame in the pane area.
function frame(u, area, gap) {
  const half = gap / 2;
  const x0 = area.x + half;
  const w0 = area.w - gap;
  const y0 = area.y + half;
  const h0 = area.h - gap;
  return {
    x: round(x0 + u.x * w0 + half),
    y: round(y0 + u.y * h0 + half),
    w: Math.max(0, round(u.w * w0 - gap)),
    h: Math.max(0, round(u.h * h0 - gap)),
  };
}

// frames returns each pane's frame in reading order.
function frames(root, area, gap) {
  const out = [];
  visit(root, { x: 0, y: 0, w: 1, h: 1 }, (n, u) => {
    if (n.type === 'pane') out.push({ id: n.id, rect: frame(u, area, gap) });
  });
  return out;
}

// span returns the start and size of the gap from aEnd to bStart: at least
// 1 pt across, centred on the gap's middle when it has to grow, as
// layout.Dividers does.
function span(aEnd, bStart) {
  if (bStart - aEnd >= 1) return [aEnd, bStart - aEnd];
  return [(aEnd + bStart) / 2 - 0.5, 1];
}

// dividers returns the gaps between each split's children, as
// layout.Dividers does.
function dividers(root, area, gap) {
  const out = [];
  visit(root, { x: 0, y: 0, w: 1, h: 1 }, (n, u) => {
    if (n.type !== 'split' || n.children.length < 2) return;
    const dir = n.dir === 'column' ? 'column' : 'row';
    const f = frame(u, area, gap);
    const units = childUnits(n, u);
    const sizes = normalise(n.sizes, n.children.length);
    for (let i = 0; i < units.length - 1; i++) {
      const a = frame(units[i], area, gap);
      const b = frame(units[i + 1], area, gap);
      let rect;
      if (dir === 'column') {
        const [start, size] = span(a.y + a.h, b.y);
        rect = { x: f.x, y: start, w: f.w, h: size };
      } else {
        const [start, size] = span(a.x + a.w, b.x);
        rect = { x: start, y: f.y, w: size, h: f.h };
      }
      out.push({ splitId: n.id, index: i, dir, rect, splitRect: f, sizes: sizes.slice() });
    }
  });
  return out;
}

// body is a pane frame without its border and its header, of header
// points and a 1 pt divider, or none if header is 0, as layout.Body does.
function body(f, header = 40) {
  const top = 1 + (header > 0 ? header + 1 : 0);
  return { x: f.x + 1, y: f.y + top, w: Math.max(0, f.w - 2), h: Math.max(0, f.h - top - 1) };
}

// reachTop stretches a pane frame up to the top of the pane area, as Go
// does for the "tabs" view's pane, whose sub-tabs are in the title bar.
function reachTop(f, area) {
  return { ...f, y: area.y, h: Math.max(0, f.y + f.h - area.y) };
}

// densityOf is the density in use for the density setting: auto is compact
// in the compact size class.
function densityOf(setting, cls) {
  if (setting === 'standard' || setting === 'compact') return setting;
  return cls === 'compact' ? 'compact' : 'standard';
}

// soleApp reports whether a pane goes without its header under
// pane_headers = "auto": it shows one App window, which is ready. Go waits
// a moment before a header comes or goes; the fixture doesn't.
function soleApp(world, n) {
  if (world.settings.paneHeaders !== 'auto' || !world.permissions.accessibility || n.items.length !== 1) return false;
  const it = world.items[n.items[0]];
  return Boolean(it) && it.kind === 'app' && !it.noWindow && it.status === 'ready';
}

function leaves(node) {
  return node.type === 'pane' ? [node] : node.children.flatMap(leaves);
}

function clone(n) {
  return JSON.parse(JSON.stringify(n));
}

// shownTree returns a copy of a tab's tree without its hidden items, the
// sub-tabs of windows that have closed, as Go displays it. A pane whose
// active item is hidden shows its first other item.
function shownTree(root, items) {
  const t = clone(root);
  for (const p of leaves(t)) {
    p.items = p.items.filter((id) => !(items[id] && items[id].hidden));
    if (!p.items.includes(p.active)) p.active = p.items[0] || '';
  }
  return t;
}

function find(root, id) {
  if (root.id === id) return root;
  if (root.type !== 'split') return null;
  for (const c of root.children) {
    const hit = find(c, id);
    if (hit) return hit;
  }
  return null;
}

// adapt returns the displayed tree for a size class and small-screen
// setting, as layout.Adapt does.
function adapt(root, setting, cls, focus, compactShown) {
  if (cls !== 'compact' || setting === 'same') return clone(root);
  if (setting === 'stack') {
    const t = clone(root);
    if (t.type === 'split' && t.dir === 'row') t.dir = 'column';
    return t;
  }
  const items = leaves(root).flatMap((p) => p.items);
  const focused = find(root, focus);
  let active = items.includes(compactShown) ? compactShown : '';
  if (!active && focused && focused.active && items.includes(focused.active)) active = focused.active;
  if (!active) active = items[0] || '';
  return { type: 'pane', id: 'compact', items, active };
}

function pane(id, items, active) {
  return { type: 'pane', id, items: items.slice(), active: active || items[0] || '' };
}

function split(id, dir, sizes, children) {
  return { type: 'split', id, dir, sizes: normalise(sizes, children.length), children };
}

const PRESETS = {
  'single': { slots: 1 },
  'side': { slots: 2, dir: 'row', sizes: [50, 50] },
  'main-side': { slots: 2, dir: 'row', sizes: [65, 35] },
  'three': { slots: 3, dir: 'row', sizes: [40, 30, 30] },
  'top-bottom': { slots: 2, dir: 'column', sizes: [65, 35] },
  'grid': { slots: 4 },
};

// presetTree builds a preset's tree from a tab's panes, keeping items in
// order, as layout.ApplyPreset does.
function presetTree(old, presetId, nextId) {
  const p = PRESETS[presetId];
  if (!p) throw new Error('There\'s no layout called ' + presetId + '.');
  const olds = old ? leaves(old) : [];
  const fresh = Array.from({ length: p.slots }, (_, i) => pane(olds[i] ? olds[i].id : nextId('p'), []));
  olds.forEach((o, i) => {
    const t = fresh[Math.min(i, p.slots - 1)];
    t.items.push(...o.items);
    if (!t.active && o.active) t.active = o.active;
  });
  fresh.forEach((f) => { if (!f.active) f.active = f.items[0] || ''; });
  if (presetId === 'single') return fresh[0];
  if (presetId === 'grid') {
    return split(nextId('s'), 'column', [50, 50], [
      split(nextId('s'), 'row', [50, 50], [fresh[0], fresh[1]]),
      split(nextId('s'), 'row', [50, 50], [fresh[2], fresh[3]]),
    ]);
  }
  return split(nextId('s'), p.dir, p.sizes, fresh);
}

function item(id, kind, app, mono, title, extra) {
  return { id, kind, app, mono, title, status: 'ready', ...extra };
}

const DEFAULT_SETTINGS = {
  fillScreen: true,
  menuBar: 'name',
  showOnTabShortcut: true,
  compactBelow: 1800,
  gap: 4,
  showStatusBar: true,
  tabShortcutHints: 'auto',
  scrollBars: 'auto',
  density: 'auto',
  paneHeaders: 'always',
  openMissingApps: true,
  hideOtherTabs: true,
  hideOtherApps: false,
  newWindows: 'app-pane',
  followFocus: 'switch-tab',
  livePreviews: false,
  launchTimeout: '15s',
};

// DEFAULT_KEYS are the default [shortcuts] in display form, as Go sends
// them in View.keys.
const DEFAULT_KEYS = {
  tabs: '⌃⌥',
  nextTab: '⌃⌥]',
  previousTab: '⌃⌥[',
  jump: '⌃⌥Space',
  rearrange: '⌃⌥0',
  focusPane: '⌃⌥',
  moveWindow: '⌃⌥⇧',
  nextWindow: '⌃⌥Tab',
};

// defaultWorld is the example config's three tabs, with the prototype's
// windows. Options: demo (the browser preview), current, mode, screen,
// screenWidth, livePreviews, keys (shortcuts to change from DEFAULT_KEYS),
// systemScrollBars (macOS shows scroll bars all the time), titleBar (the
// title bar's density as the app opened, which otherwise follows the
// density in use, as in the browser preview).
function defaultWorld(opts = {}) {
  const items = {
    w1: item('w1', 'web', 'Slack', 'Sl', '# general', { url: 'https://app.slack.com/client', host: 'app.slack.com', site: 'slack.com' }),
    w2: item('w2', 'web', 'Outlook', 'Ou', 'Inbox', { url: 'https://outlook.office.com/mail/', host: 'outlook.office.com', site: 'office.com' }),
    w3: item('w3', 'app', 'Zed', 'Zd', 'bench — switch.go', { bundleId: 'dev.zed.Zed' }),
    w4: item('w4', 'app', 'VS Code', 'VS', 'index.html — bench-ui', { bundleId: 'com.microsoft.VSCode' }),
    w5: item('w5', 'builtin', 'Terminal', '>_', 'zsh — ~/code/bench', { folder: '~/code/bench' }),
    w6: item('w6', 'builtin', 'Terminal', '>_', 'go test — ~/code/bench', { folder: '~/code/bench' }),
    w7: item('w7', 'app', 'Brave', 'Br', 'Wails v3 docs', { bundleId: 'com.brave.Browser' }),
    w8: item('w8', 'app', 'Brave', 'Br', 'Pull request #42 · bench', { bundleId: 'com.brave.Browser' }),
    w9: item('w9', 'app', 'Brave', 'Br', 'Go packages', { bundleId: 'com.brave.Browser' }),
    w10: item('w10', 'app', 'Obsidian', 'Ob', 'Daily note', { bundleId: 'md.obsidian' }),
  };
  return {
    seq: 100,
    items,
    tabs: [
      { name: 'Comms', preset: 'side', smallScreens: 'tabs', focus: 'p1', root: split('s1', 'row', [45, 55], [pane('p1', ['w1']), pane('p2', ['w2'])]) },
      { name: 'Code', preset: 'main-side', smallScreens: 'stack', focus: 'p3', root: split('s2', 'row', [65, 35], [pane('p3', ['w3', 'w4']), pane('p4', ['w5', 'w6'])]) },
      { name: 'Research', preset: 'side', smallScreens: 'tabs', focus: 'p5', root: split('s3', 'row', [60, 40], [pane('p5', ['w7', 'w8', 'w9']), pane('p6', ['w10'])]) },
    ],
    floating: [
      item('f101', 'app', 'Brave', 'Br', 'Release notes', { bundleId: 'com.brave.Browser', tab: -1 }),
      item('f102', 'app', 'Finder', 'Fn', 'Downloads', { bundleId: 'com.apple.finder', tab: -1 }),
    ],
    current: opts.current ?? 1,
    mode: opts.mode || 'auto',
    demo: opts.demo ? { screen: opts.screen || 'fit' } : null,
    fitWidth: opts.fitWidth || 1440,
    screenWidth: opts.screenWidth || 2560,
    settings: { ...DEFAULT_SETTINGS, livePreviews: Boolean(opts.livePreviews) },
    keys: { ...DEFAULT_KEYS, ...opts.keys },
    permissions: { accessibility: opts.accessibility ?? true, screenRecording: Boolean(opts.screenRecording) },
    systemScrollBars: Boolean(opts.systemScrollBars),
    notices: opts.notices || [],
    configWritable: opts.configWritable ?? true,
    configPath: '/Users/me/Library/Application Support/Bench/config.toml',
    firstRun: Boolean(opts.firstRun),
    openAtLogin: false,
    // dialogs are the sizes the user gave dialogs, by kind, as state.json
    // keeps them.
    dialogs: opts.dialogs || {},
    area: opts.area || { x: 0, y: 0, w: 0, h: 0 },
    compactShown: {},
    titleBar: opts.titleBar || '',
  };
}

// manyTabs adds tabs until there are n, each with one Brave window.
function manyTabs(world, n) {
  for (let i = world.tabs.length; i < n; i++) {
    const id = 'w' + (200 + i);
    world.items[id] = item(id, 'app', 'Brave', 'Br', 'Page ' + (i + 1), { bundleId: 'com.brave.Browser' });
    world.tabs.push({ name: 'Tab ' + (i + 1), preset: 'single', smallScreens: 'stack', focus: 'p' + (200 + i), root: pane('p' + (200 + i), [id]) });
  }
  return world;
}

// screenWidthOf is the width of the screen the window is on.
function screenWidthOf(world) {
  if (!world.demo) return world.screenWidth;
  return { fit: world.fitWidth, monitor: 2560, laptop: 1512 }[world.demo.screen] || world.fitWidth;
}

// buildView turns a world into the View that Go sends.
function buildView(world) {
  const tab = world.tabs[world.current];
  const sw = screenWidthOf(world);
  const cls = world.mode !== 'auto' ? world.mode : (sw < world.settings.compactBelow ? 'compact' : 'wide');
  const shown = adapt(shownTree(tab.root, world.items), tab.smallScreens, cls, tab.focus, world.compactShown[world.current]);
  const compact = shown.id === 'compact';
  const gap = world.settings.gap;
  const area = world.area;
  const density = densityOf(world.settings.density, cls);
  const header = density === 'compact' ? 32 : 40;
  // An item in several tabs is placed in the current tab if it's there, or
  // else in the first holding it, and lists every tab holding it.
  const places = {};
  world.tabs.forEach((t, ti) => leaves(t.root).forEach((p) => p.items.forEach((id) => {
    (places[id] = places[id] || []).push({ tab: ti, pane: p.id });
  })));
  const items = {};
  for (const [id, ps] of Object.entries(places)) {
    const at = ps.find((x) => x.tab === world.current) || ps[0];
    items[id] = { ...world.items[id], tab: at.tab, pane: at.pane, ...(ps.length > 1 ? { tabs: ps.map((x) => x.tab) } : {}) };
  }
  for (const f of world.floating) items[f.id] = { ...f, tab: -1 };
  return {
    tabs: world.tabs.map((t, i) => {
      const ps = leaves(t.root);
      const ids = ps.flatMap((p) => p.items).filter((id) => !world.items[id].hidden);
      return {
        name: t.name,
        shortcut: world.keys.tabs && i < 9 ? world.keys.tabs + (i + 1) : '',
        preset: t.preset,
        smallScreens: t.smallScreens,
        root: clone(t.root),
        focus: t.focus,
        apps: ids.map((id) => world.items[id].app).filter((a, k, all) => all.indexOf(a) === k),
        windows: ids.length,
        panes: ps.length,
      };
    }),
    current: world.current,
    display: {
      area: { ...area },
      root: shown,
      panes: frames(shown, area, gap).map(({ id, rect }) => {
        const n = find(shown, id);
        // world.remembered lists, by pane, the apps a pane is waiting for.
        const remembered = ((world.remembered || {})[id] || []).map((r) => ({ ...r }));
        // The "tabs" view's pane reaches up to the title bar, which has its
        // sub-tabs.
        const r = compact ? reachTop(rect, area) : rect;
        const h = compact || soleApp(world, n) ? 0 : header;
        return { id, rect: r, body: body(r, h), header: h, items: n.items.slice(), active: n.active, remembered };
      }),
      dividers: dividers(shown, area, gap),
      focus: compact ? 'compact' : tab.focus,
      compact,
      splitsDisabled: compact ? 'Splits are hidden on small screens.' : '',
    },
    items,
    floating: world.floating.map((f) => ({ ...f, tab: -1 })),
    mode: world.mode,
    sizeClass: cls,
    sizeReason: world.mode === 'auto' ? 'chosen for a ' + sw.toLocaleString('en-GB') + '-pt screen' : 'set by hand',
    screenWidth: sw,
    settings: { ...world.settings },
    density,
    titleBar: world.titleBar || density,
    systemScrollBars: Boolean(world.systemScrollBars),
    keys: { ...world.keys },
    permissions: { ...world.permissions },
    // A notice can be given as its text alone.
    notices: world.notices.map((n) => (typeof n === 'string' ? { text: n } : clone(n))),
    configWritable: world.configWritable,
    configPath: world.configPath,
    firstRun: world.firstRun,
    openAtLogin: world.openAtLogin,
    ...(Object.keys(world.dialogs || {}).length ? { dialogs: clone(world.dialogs) } : {}),
    ...(world.demo ? { demo: { ...world.demo } } : {}),
  };
}

module.exports = {
  adapt, body, buildView, clone, defaultWorld, dividers, find, frame, frames, leaves, manyTabs,
  normalise, pane, presetTree, reachTop, split, DEFAULT_KEYS, DEFAULT_SETTINGS,
};
