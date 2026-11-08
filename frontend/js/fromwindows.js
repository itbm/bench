// The New tab dialog's "Windows that are open" (editor.js): a list of the
// windows that aren't in a tab, ticked as the dialog opens, grouped by
// app, and, in a group that starts closed, the windows of the tabs,
// unticked, which stay in their tabs and show in the new tab as well. The
// dialog suggests a layout by the number of apps ticked, as Go does, and
// shows Go's preview of the tab (PreviewTabFromWindows); Make tab makes it
// (NewTabFromWindows).

import { call, errorText } from './bridge.js';
import { store, toast, update } from './state.js';
import { esc, ICONS, leaves, uniqueTabName } from './util.js';
import { tile } from './tiles.js';

// appKey returns a key that's the same for the windows of one app, as Go's
// appKey does: its bundle ID, or for a page or terminal, its kind and name.
// Go groups the windows it's sent the same way.
export function appKey(it) {
  return it.bundleId ? it.bundleId.toLowerCase() : (it.kind || 'app') + ':' + String(it.app || '').toLowerCase();
}

// suggestFor returns the layout that Go suggests for the windows of a
// number of apps (suggestPreset): a pane for each, up to the grid of four.
export function suggestFor(apps) {
  if (apps <= 1) return 'single';
  if (apps === 2) return 'side';
  if (apps === 3) return 'three';
  return 'grid';
}

// windowChoices returns the windows to choose from, each list grouped by
// app, the apps by name: those not in a tab (outside), and those in the
// tabs (inTabs), each with the tabs it's in as its hint. A sub-tab whose
// window has closed is left out, as Go leaves it out, and so is the
// drop-down terminal, which is out of the tabs but no window to make a tab
// of (decision 72).
export function windowChoices(view) {
  const outside = [];
  outsideTabs(view).forEach((it) => addTo(outside, it, ''));
  const tabsOf = new Map();
  view.tabs.forEach((t) => leaves(t.root).forEach((p) => (p.items || []).forEach((id) => {
    const names = tabsOf.get(id) || [];
    if (!names.includes(t.name)) names.push(t.name);
    tabsOf.set(id, names);
  })));
  const inTabs = [];
  tabsOf.forEach((names, id) => {
    const it = view.items && view.items[id];
    if (it && !it.hidden) addTo(inTabs, it, 'In ' + names.join(', '));
  });
  const byName = (a, b) => a.name.localeCompare(b.name);
  return { outside: outside.sort(byName), inTabs: inTabs.sort(byName) };
}

// outsideTabs returns the windows "Not in a tab", other than the drop-down
// terminal.
export function outsideTabs(view) {
  const drop = view.dropdown ? view.dropdown.id : '';
  return (view.floating || []).filter((it) => it.id !== drop);
}

// addTo adds a window to its app's group.
function addTo(groups, it, hint) {
  const key = appKey(it);
  let g = groups.find((x) => x.key === key);
  if (!g) {
    g = { key, name: it.app || 'Untitled', mono: it.mono, icon: it.icon, windows: [] };
    groups.push(g);
  }
  g.windows.push({ id: it.id, title: it.title || it.app || '', hint });
}

// newWindowsDraft returns the windows part of a New tab draft: the windows
// not in a tab are ticked, and the group of the tabs' windows is closed.
export function newWindowsDraft(view) {
  const { outside } = windowChoices(view);
  return { ticked: outside.flatMap((g) => g.windows.map((w) => w.id)), others: false, apps: 0 };
}

// tickedWindows returns the ticked windows that are still there, in the
// list's order, which Go fills the panes in: those not in a tab, then those
// of the tabs. Each has its app's key and name.
export function tickedWindows(view, d) {
  const { outside, inTabs } = windowChoices(view);
  const ticked = new Set(d.win.ticked);
  const out = [];
  for (const g of outside.concat(inTabs)) {
    for (const w of g.windows) {
      if (ticked.has(w.id)) out.push({ id: w.id, key: g.key, app: g.name });
    }
  }
  return out;
}

// appCount returns how many apps the ticked windows are of.
function appCount(ticked) {
  return new Set(ticked.map((w) => w.key)).size;
}

// autoName returns the name the draft's tab gets until one is typed: the
// first ticked window's app's, numbered if a tab has it, as Go numbers it.
export function autoName(view, d) {
  const first = tickedWindows(view, d)[0];
  return uniqueTabName(view.tabs, first ? first.app : 'New tab', -1);
}

// followTicks updates the draft after its ticks change: its name and
// layout, unless the user has chosen their own, and Go's preview.
export function followTicks(d) {
  const view = store.view;
  const ticked = tickedWindows(view, d);
  d.win.apps = appCount(ticked);
  if (!d.nameTyped) d.name = autoName(view, d);
  if (!d.presetChosen) d.preset = suggestFor(d.win.apps);
  loadWindowsPreview(d);
}

// loadWindowsPreview asks Go for the tab that the ticked windows would
// make, with the layout chosen, or else the one suggested.
export function loadWindowsPreview(d) {
  const ids = tickedWindows(store.view, d).map((w) => w.id);
  const seq = ++d.previewSeq;
  call('PreviewTabFromWindows', d.presetChosen ? d.preset : '', ids).then((p) => {
    if (store.ui.editor !== d || d.previewSeq !== seq || d.start !== 'windows') return;
    const preview = p || {};
    d.win.apps = preview.apps || 0;
    if (!d.presetChosen && preview.suggested) d.preset = preview.suggested;
    d.preview = preview.root || null;
    update();
  }, (err) => {
    if (store.ui.editor === d) toast(errorText(err));
  });
}

// tickWindow ticks a window, or with on false unticks it.
export function tickWindow(id, on) {
  const d = store.ui.editor;
  if (!d || !d.win) return;
  d.win.ticked = d.win.ticked.filter((x) => x !== id);
  if (on) d.win.ticked.push(id);
  followTicks(d);
  update();
}

// tickApp ticks every window of an app in one of the lists (where is
// outside or inTabs), or with on false unticks them.
export function tickApp(key, where, on) {
  const d = store.ui.editor;
  if (!d || !d.win) return;
  const g = (windowChoices(store.view)[where] || []).find((x) => x.key === key);
  if (!g) return;
  const ids = g.windows.map((w) => w.id);
  d.win.ticked = d.win.ticked.filter((x) => !ids.includes(x));
  if (on) d.win.ticked.push(...ids);
  followTicks(d);
  update();
}

// toggleOthers opens or closes the group of the tabs' windows.
export function toggleOthers() {
  const d = store.ui.editor;
  if (!d || !d.win) return;
  d.win.others = !d.win.others;
  update();
}

// windowsText returns "1 window", or n windows.
function windowsText(n) {
  return n === 1 ? '1 window' : n + ' windows';
}

// groupHTML returns an app's group in the list: a row that ticks all its
// windows, then a row for each. An app with one window has one row.
function groupHTML(g, where, ticked) {
  const n = g.windows.filter((w) => ticked.has(w.id)).length;
  const label = esc(g.name);
  if (g.windows.length === 1) {
    const w = g.windows[0];
    return '<div class="win-app" data-app-name="' + label + '">'
      + '<label class="win-row win-head"><input type="checkbox" data-win="' + esc(w.id) + '" data-key="win-' + esc(w.id) + '"' + (n ? ' checked' : '') + '>'
      + tile(g.mono, g.icon) + '<span class="win-name">' + label + '</span>'
      + '<span class="win-title">' + esc(w.title) + '</span>'
      + (w.hint ? '<span class="win-hint">' + esc(w.hint) + '</span>' : '') + '</label></div>';
  }
  const all = n === g.windows.length;
  return '<div class="win-app" role="group" aria-label="' + label + '" data-app-name="' + label + '">'
    + '<label class="win-row win-head"><input type="checkbox" data-win-app="' + esc(g.key) + '" data-win-where="' + where + '" data-key="win-app-' + where + '-' + esc(g.key) + '"'
    + (all ? ' checked' : '') + (n && !all ? ' data-mixed="true"' : '') + '>'
    + tile(g.mono, g.icon) + '<span class="win-name">' + label + '</span>'
    + '<span class="win-count">' + esc(windowsText(g.windows.length)) + '</span></label>'
    + g.windows.map((w) => '<label class="win-row win-window"><input type="checkbox" data-win="' + esc(w.id) + '" data-key="win-' + esc(w.id) + '"'
      + (ticked.has(w.id) ? ' checked' : '') + '><span class="win-title">' + esc(w.title) + '</span>'
      + (w.hint ? '<span class="win-hint">' + esc(w.hint) + '</span>' : '') + '</label>').join('')
    + '</div>';
}

// renderWindowsChoice returns the draft's list of windows, for the New tab
// dialog with "Windows that are open" chosen.
export function renderWindowsChoice(view, d) {
  const { outside, inTabs } = windowChoices(view);
  const ticked = new Set(d.win.ticked);
  const count = tickedWindows(view, d).length;
  const note = count
    ? windowsText(count) + ' ticked. Each app\'s windows share a pane, as sub-tabs.'
    : 'Tick the windows for the tab.';
  const list = outside.length
    ? '<div class="win-groups">' + outside.map((g) => groupHTML(g, 'outside', ticked)).join('') + '</div>'
    : '<p class="win-empty">There are no windows outside your tabs. Open the windows you want, or tick windows of your tabs below.</p>';
  let others = '';
  if (inTabs.length) {
    const inTabsTicked = inTabs.reduce((n, g) => n + g.windows.filter((w) => ticked.has(w.id)).length, 0);
    const more = 'Windows in other tabs (' + inTabs.reduce((n, g) => n + g.windows.length, 0) + ')'
      + (inTabsTicked ? ' · ' + inTabsTicked + ' ticked' : '');
    others = '<button type="button" class="win-more" data-action="win-others" data-key="win-others" aria-expanded="' + d.win.others + '" aria-controls="win-others">'
      + (d.win.others ? ICONS.chevronDown : ICONS.chevron) + '<span>' + esc(more) + '</span></button>'
      + (d.win.others
        ? '<div class="win-others" id="win-others"><p class="win-note">These stay in their tabs, and show in the new tab as well.</p>'
          + '<div class="win-groups">' + inTabs.map((g) => groupHTML(g, 'inTabs', ticked)).join('') + '</div></div>'
        : '');
  }
  return '<div class="editor-row"><h3 id="win-label">Windows</h3><p id="win-note" aria-live="polite">' + esc(note) + '</p></div>'
    + '<div class="win-list" role="group" aria-labelledby="win-label" aria-describedby="win-note">' + list + others + '</div>';
}

// fillWindows shows the tick box of an app with some of its windows ticked
// as partly ticked, which HTML can't say.
export function fillWindows() {
  document.querySelectorAll('input[data-mixed="true"]').forEach((el) => { el.indeterminate = true; });
}
