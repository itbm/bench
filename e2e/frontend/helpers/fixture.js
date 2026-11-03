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
  checkForUpdates: false,
  confirmQuitBusyTerminals: true,
  confirmClose: true,
  compactBelow: 1800,
  gap: 4,
  showStatusBar: true,
  tabShortcutHints: 'auto',
  scrollBars: 'auto',
  appearance: 'dark',
  density: 'auto',
  paneHeaders: 'always',
  openMissingApps: true,
  hideOtherTabs: true,
  hideOtherApps: false,
  newWindows: 'app-pane',
  followFocus: 'switch-tab',
  livePreviews: false,
  dragToAdd: true,
  terminalFontSize: 12.5,
  terminalOptionAsMeta: false,
  notifyTerminals: true,
  keepTerminalOutput: true,
  launchTimeout: '15s',
  launchers: [],
};

// STARTER_TABS are the tabs that Go's StarterTabs suggests on a Mac with
// Slack, Mail, VS Code, and Obsidian installed.
const STARTER_TABS = [
  { name: 'Comms', apps: [{ bundleId: 'com.tinyspeck.slackmacgap', name: 'Slack', mono: 'Sl' }, { bundleId: 'com.apple.mail', name: 'Mail', mono: 'Ma' }] },
  { name: 'Code', apps: [{ bundleId: 'com.microsoft.VSCode', name: 'Visual Studio Code', mono: 'VS' }, { name: 'Terminal', mono: '>_' }] },
  { name: 'Plan', apps: [{ bundleId: 'md.obsidian', name: 'Obsidian', mono: 'Ob' }] },
];

// monoOf makes a monogram of a name, roughly as Go's apps.Mono does: the
// initials of its first two words, or the first two letters of one word.
function monoOf(name) {
  const words = String(name).split(/[^\p{L}\p{N}]+/u).filter(Boolean);
  if (words.length > 1) return words[0][0].toUpperCase() + words[1][0].toUpperCase();
  const w = words[0] || String(name);
  return w.slice(0, 1).toUpperCase() + w.slice(1, 2);
}

// launcher describes a launcher as Go's LauncherView does, with its
// monogram and what it opens: a terminal that runs a command, or, with a
// bundleId, an app's window on a folder or link. app names the app.
function launcher(l) {
  if (l.kind === 'app' || l.bundleId) {
    const app = l.app || l.bundleId;
    return {
      name: l.name, kind: 'app', folder: l.folder || '', command: '', startAgain: false,
      bundleId: l.bundleId, url: l.url || '', title: l.title || '', app, mono: monoOf(app), hint: app + ' · ' + (l.url || l.folder),
    };
  }
  return {
    name: l.name, kind: 'terminal', folder: l.folder || '', command: l.command || '', startAgain: l.startAgain !== false,
    bundleId: '', url: '', title: '', app: 'Terminal', mono: monoOf(l.name), hint: (l.command || 'zsh') + ' · ' + (l.folder || '~'),
  };
}

// DEFAULT_KEYS are the default [shortcuts] in display form, as Go sends
// them in View.keys.
const DEFAULT_KEYS = {
  tabs: '⌃⌥',
  nextTab: '⌃⌥]',
  previousTab: '⌃⌥[',
  lastTab: '⌃⌥`',
  jump: '⌃⌥Space',
  rearrange: '⌃⌥0',
  focusPane: '⌃⌥',
  moveWindow: '⌃⌥⇧',
  nextWindow: '⌃⌥Tab',
  zoomPane: '⌃⌥↩',
  addFrontWindow: '',
};

// SHORTCUTS are the fields of the [shortcuts] table, in config order, as
// Go lists them in View.shortcuts: each with its name in View.keys, its
// label, the keys Bench adds to a field of modifiers, and its default.
const SHORTCUTS = [
  { field: 'tab_modifiers', key: 'tabs', label: 'Tabs 1 to 9', keys: '1 to 9', default: 'ctrl+alt' },
  { field: 'next_tab', key: 'nextTab', label: 'Next tab', default: 'ctrl+alt+]' },
  { field: 'previous_tab', key: 'previousTab', label: 'Previous tab', default: 'ctrl+alt+[' },
  { field: 'last_tab', key: 'lastTab', label: 'Last used tab', default: 'ctrl+alt+`' },
  { field: 'jump', key: 'jump', label: 'Jump to…', default: 'ctrl+alt+space' },
  { field: 'rearrange', key: 'rearrange', label: 'Rearrange', default: 'ctrl+alt+0' },
  { field: 'focus_pane_modifiers', key: 'focusPane', label: 'Focus a pane', keys: 'the arrow keys', default: 'ctrl+alt' },
  { field: 'move_window_modifiers', key: 'moveWindow', label: 'Move a window', keys: 'the arrow keys', default: 'ctrl+alt+shift' },
  { field: 'next_window', key: 'nextWindow', label: 'Next window', default: 'ctrl+alt+tab' },
  { field: 'zoom_pane', key: 'zoomPane', label: 'Zoom pane', default: 'ctrl+alt+return' },
  { field: 'add_front_window', key: 'addFrontWindow', label: 'Add front window to this pane', default: '' },
];

// SHORTCUT_MODS are the modifiers' symbols and their names in config.toml,
// in the order shortcuts use.
const SHORTCUT_MODS = [['⌃', 'ctrl'], ['⌥', 'alt'], ['⇧', 'shift'], ['⌘', 'cmd']];

// SHORTCUT_KEYS are the display names of the keys whose config names
// aren't their display names in lower case.
const SHORTCUT_KEYS = {
  Space: 'space', Tab: 'tab', '↩': 'return', '⎋': 'escape', '⌫': 'delete', '⌦': 'forwarddelete',
  '↖': 'home', '↘': 'end', '⇞': 'pageup', '⇟': 'pagedown', '←': 'left', '→': 'right', '↑': 'up', '↓': 'down',
};

// configShortcut turns a shortcut in display form, such as "⌃⌥Space", into
// config form: "ctrl+alt+space".
function configShortcut(display) {
  if (!display) return '';
  const mods = display.match(/^[⌃⌥⇧⌘]*/)[0];
  const key = display.slice(mods.length);
  const parts = SHORTCUT_MODS.filter(([sym]) => mods.includes(sym)).map(([, name]) => name);
  if (key) parts.push(SHORTCUT_KEYS[key] || key.toLowerCase());
  return parts.join('+');
}

// displayShortcut turns a shortcut in config form into display form, as
// Go's shortcuts.Display does.
function displayShortcut(value) {
  if (!value) return '';
  const parts = value.split('+');
  const last = parts[parts.length - 1];
  const key = SHORTCUT_MODS.some(([, name]) => name === last) ? '' : parts.pop();
  const mods = SHORTCUT_MODS.filter(([, name]) => parts.includes(name)).map(([sym]) => sym).join('');
  const named = Object.keys(SHORTCUT_KEYS).find((k) => SHORTCUT_KEYS[k] === key);
  return mods + (named || (key.length === 1 ? key.toUpperCase() : key.replace(/^f(\d+)$/, 'F$1')));
}

// shortcutsOf returns the shortcuts that a field's value makes, in config
// form, as Go's shortcuts.Bindings does: modifiers with the keys Bench adds
// to them, and next_window with shift added for the previous window.
function shortcutsOf(field, value) {
  if (!value) return [];
  const added = {
    tab_modifiers: ['1', '2', '3', '4', '5', '6', '7', '8', '9'],
    focus_pane_modifiers: ['left', 'right', 'up', 'down'],
    move_window_modifiers: ['left', 'right', 'up', 'down'],
  }[field];
  if (added) return added.map((k) => value + '+' + k);
  return field === 'next_window' ? [value, configShortcut('⇧' + displayShortcut(value))] : [value];
}

// shortcutRows returns Settings ▸ Keyboard's rows, as Go's View.shortcuts
// has them: the [shortcuts] table, from world.keys, with what
// world.keyStates says about a field whose value it describes, such as
// { jump: { value: 'ctrl+alt+space', problem, suggestion } }.
function shortcutRows(world) {
  return SHORTCUTS.map((f) => {
    const value = configShortcut(world.keys[f.key]);
    const state = (world.keyStates || {})[f.field];
    const known = state && state.value === value ? state : {};
    return {
      field: f.field, label: f.label, ...(f.keys ? { keys: f.keys } : {}), value, ...(value ? { display: displayShortcut(value) } : {}), default: f.default,
      ...(known.problem ? { problem: known.problem } : {}),
      ...(known.suggestion ? { suggestion: known.suggestion, suggestionDisplay: displayShortcut(known.suggestion) } : {}),
    };
  });
}

// defaultWorld is the example config's three tabs, with the prototype's
// windows. Options: demo (the browser preview), current, mode, screen,
// screenWidth, livePreviews, keys (shortcuts to change from DEFAULT_KEYS),
// systemScrollBars (macOS shows scroll bars all the time), titleBar (the
// title bar's density as the app opened, which otherwise follows the
// density in use, as in the browser preview), closedTabs (the names of the
// tabs that Reopen closed tab can bring back, latest first), lastTab (the
// index of the tab used before the current one).
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
  const world = {
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
    settings: { ...DEFAULT_SETTINGS, livePreviews: Boolean(opts.livePreviews), launchers: (opts.launchers || []).map(launcher) },
    keys: { ...DEFAULT_KEYS, ...opts.keys },
    permissions: { accessibility: opts.accessibility ?? true, screenRecording: Boolean(opts.screenRecording) },
    systemScrollBars: Boolean(opts.systemScrollBars),
    notices: opts.notices || [],
    configWritable: opts.configWritable ?? true,
    configPath: '/Users/me/Library/Application Support/Bench/config.toml',
    firstRun: Boolean(opts.firstRun),
    openAtLogin: false,
    // loginApproval is set while macOS needs the user to allow Bench's
    // login item, and needsApproval has SetOpenAtLogin find that it does.
    loginApproval: Boolean(opts.loginApproval),
    needsApproval: Boolean(opts.needsApproval),
    // installsUpdates is set when Bench has Sparkle, which installs new
    // versions.
    installsUpdates: Boolean(opts.installsUpdates),
    // starterTabs are the tabs Go would suggest for the welcome guide's
    // starting set-up, as StarterTabs returns them, before it leaves out
    // those whose name a tab has.
    starterTabs: opts.starterTabs || STARTER_TABS.map(clone),
    // dialogs are the sizes the user gave dialogs, by kind, as state.json
    // keeps them.
    dialogs: opts.dialogs || {},
    area: opts.area || { x: 0, y: 0, w: 0, h: 0 },
    compactShown: {},
    titleBar: opts.titleBar || '',
    closedTabs: opts.closedTabs || [],
    // used counts uses of items, as Go's Model does, for ItemView.used.
    used: 0,
  };
  // lastTab is the tab used before the current one, given by its index.
  world.lastTab = opts.lastTab >= 0 ? world.tabs[opts.lastTab] : null;
  return world;
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

// sizeClassOf is the size class in use.
function sizeClassOf(world) {
  if (world.mode !== 'auto') return world.mode;
  return screenWidthOf(world) < world.settings.compactBelow ? 'compact' : 'wide';
}

// zoomedPane returns the zoomed pane's ID, or '': world.zoom names the tab
// and pane that ZoomPane zoomed, which stays zoomed while that tab is
// current, with that pane focused, and more than one pane, outside the
// "tabs" view, as Go's zoomedLocked says.
function zoomedPane(world) {
  const z = world.zoom;
  const tab = world.tabs[world.current];
  if (!z || z.tab !== world.current || !tab || tab.focus !== z.pane || leaves(tab.root).length < 2) return '';
  if (sizeClassOf(world) === 'compact' && tab.smallScreens === 'tabs') return '';
  return find(tab.root, z.pane) ? z.pane : '';
}

// buildView turns a world into the View that Go sends.
// lastTabOf returns the index of the tab used before the current one
// (world.lastTab), or -1.
function lastTabOf(world) {
  const i = world.lastTab ? world.tabs.indexOf(world.lastTab) : -1;
  return i === world.current ? -1 : i;
}

function buildView(world) {
  const tab = world.tabs[world.current];
  const sw = screenWidthOf(world);
  const cls = sizeClassOf(world);
  // A zoomed pane is shown alone, with its header.
  const zoomed = zoomedPane(world);
  const tree = shownTree(tab.root, world.items);
  const shown = zoomed ? find(tree, zoomed) : adapt(tree, tab.smallScreens, cls, tab.focus, world.compactShown[world.current]);
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
      // attention counts the tab's windows marked as needing the user, and
      // camera and microphone its pages that use them, live or not.
      const attention = ids.filter((id) => world.items[id].attention).length;
      const camera = ids.filter((id) => world.items[id].camera).length;
      const microphone = ids.filter((id) => world.items[id].microphone).length;
      const cameraLive = ids.some((id) => world.items[id].camera === 'active');
      const microphoneLive = ids.some((id) => world.items[id].microphone === 'active');
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
        ...(attention ? { attention } : {}),
        ...(camera ? { camera } : {}),
        ...(microphone ? { microphone } : {}),
        ...(cameraLive ? { cameraLive } : {}),
        ...(microphoneLive ? { microphoneLive } : {}),
      };
    }),
    current: world.current,
    // lastTab is the tab used before the current one, which Go follows as
    // tabs move, and forgets once it's deleted.
    lastTab: lastTabOf(world),
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
        const h = compact || (!zoomed && soleApp(world, n)) ? 0 : header;
        return { id, rect: r, body: body(r, h), header: h, items: n.items.slice(), active: n.active, remembered };
      }),
      dividers: dividers(shown, area, gap),
      focus: compact ? 'compact' : tab.focus,
      compact,
      splitsDisabled: compact ? 'Splits are hidden on small screens.' : '',
      ...(zoomed ? { zoomed } : {}),
    },
    items,
    floating: world.floating.map((f) => ({ ...f, tab: -1 })),
    // front is the window in front before Bench, if it's in no tab, for
    // Add front window to this pane.
    ...(world.front ? { front: world.front } : {}),
    mode: world.mode,
    sizeClass: cls,
    sizeReason: world.mode === 'auto' ? 'chosen for a ' + sw.toLocaleString('en-GB') + '-pt screen' : 'set by hand',
    screenWidth: sw,
    settings: { ...world.settings },
    density,
    titleBar: world.titleBar || density,
    systemScrollBars: Boolean(world.systemScrollBars),
    keys: { ...world.keys },
    shortcuts: shortcutRows(world),
    permissions: { ...world.permissions },
    // A notice can be given as its text alone.
    notices: world.notices.map((n) => (typeof n === 'string' ? { text: n } : clone(n))),
    configWritable: world.configWritable,
    configPath: world.configPath,
    firstRun: world.firstRun,
    openAtLogin: world.openAtLogin,
    ...(world.loginApproval ? { loginApproval: true } : {}),
    ...(world.installsUpdates ? { installsUpdates: true } : {}),
    ...(Object.keys(world.dialogs || {}).length ? { dialogs: clone(world.dialogs) } : {}),
    ...(world.demo ? { demo: { ...world.demo } } : {}),
    ...((world.closedTabs || []).length ? { closedTabs: world.closedTabs.slice() } : {}),
  };
}

module.exports = {
  adapt, body, buildView, clone, configShortcut, defaultWorld, displayShortcut, dividers, find, frame, frames, launcher, leaves, manyTabs,
  normalise, pane, presetTree, reachTop, shortcutRows, shortcutsOf, split, zoomedPane, DEFAULT_KEYS, DEFAULT_SETTINGS, SHORTCUTS,
};
