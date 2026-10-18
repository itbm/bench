// The layout editor: a sheet for the current tab or a new one, saved with
// SaveTab.

import { call, errorText } from './bridge.js';
import { act, closeOverlay, isDemo, store, toast, update } from './state.js';
import { activeKey, esc, fmt, itemOf, keysOf, pairText, shortcutPrefix } from './util.js';

// PRESETS are the layout presets, in the order the editor shows them.
export const PRESETS = [
  { id: 'single', label: 'Single' },
  { id: 'side', label: 'Side by side' },
  { id: 'main-side', label: 'Main and side' },
  { id: 'three', label: 'Three columns' },
  { id: 'top-bottom', label: 'Top and bottom' },
  { id: 'grid', label: 'Grid of four' },
];

const SMALL_SCREENS = [
  { id: 'stack', label: 'Stack panes' },
  { id: 'tabs', label: 'One pane with sub-tabs' },
  { id: 'same', label: 'Keep the layout' },
];

// openEditor opens the layout editor for tab index, or for a new tab if
// index is -1.
export function openEditor(index) {
  const { ui } = store;
  const view = store.view;
  const tab = index >= 0 ? view.tabs[index] : null;
  if (index >= 0 && !tab) return;
  const s = view.settings || {};
  if (!ui.overlay) ui.opener = activeKey();
  ui.menu = null;
  ui.add = null;
  ui.overlay = 'editor';
  ui.editor = {
    index,
    // tabName is the tab's name when the editor opened, to find it by.
    tabName: tab ? tab.name : '',
    name: tab ? tab.name : 'New tab',
    preset: tab ? tab.preset || 'single' : 'single',
    smallScreens: tab ? tab.smallScreens || 'stack' : 'stack',
    compactBelow: String(s.compactBelow ?? 1800),
    settings: { ...s },
    followFocusWas: s.followFocus,
    openAtLogin: Boolean(view.openAtLogin),
    // The preview starts as the tab's own tree, until PreviewTree answers.
    preview: tab ? tab.root : null,
    previewSeq: 0,
    // busy is set while SaveTab, DeleteTab, or ResetLayout is on its way.
    busy: false,
  };
  ui.pendingFocus = 'tab-name';
  loadPreview(ui.editor);
  update();
}

// loadPreview asks Go for the tree the draft's preset would make. A custom
// layout shows the tab's own tree.
function loadPreview(d) {
  const view = store.view;
  if (d.preset === 'custom') {
    const tab = view.tabs[d.index];
    d.preview = tab ? tab.root : null;
    return;
  }
  const seq = ++d.previewSeq;
  call('PreviewTree', d.index, d.preset).then((tree) => {
    if (store.ui.editor !== d || d.previewSeq !== seq) return;
    d.preview = tree || null;
    update();
  }, (err) => {
    if (store.ui.editor === d) toast(errorText(err));
  });
}

// setPreset chooses a preset in the draft.
export function setPreset(id) {
  const d = store.ui.editor;
  if (!d || d.preset === id) return;
  d.preset = id;
  loadPreview(d);
  update();
}

// setSmallScreens chooses the draft's small-screen setting.
export function setSmallScreens(id) {
  const d = store.ui.editor;
  if (!d) return;
  d.smallScreens = id;
  update();
}

// setToggle changes one of the editor's checkboxes.
export function setToggle(name, on) {
  const d = store.ui.editor;
  if (!d) return;
  if (name === 'openAtLogin') {
    d.openAtLogin = on;
  } else if (name === 'followFocus') {
    // The checkbox is "Switch tabs"; unticking it keeps "bring-here" if
    // that was the setting, and otherwise turns focus following off.
    d.settings.followFocus = on ? 'switch-tab' : (d.followFocusWas && d.followFocusWas !== 'switch-tab' ? d.followFocusWas : 'off');
  } else {
    d.settings[name] = on;
    const view = store.view;
    if (name === 'livePreviews' && on && !isDemo() && !(view.permissions && view.permissions.screenRecording)) {
      act('RequestScreenRecording');
    }
  }
  update();
}

// setField changes a text field of the draft.
export function setField(name, value) {
  const d = store.ui.editor;
  if (!d) return;
  d[name] = value;
  if (name === 'compactBelow') update();
}

// draftOf returns the TabDraft that SaveTab takes.
export function draftOf(d, view) {
  const threshold = parseInt(d.compactBelow, 10);
  const settings = { ...d.settings, compactBelow: threshold > 0 ? threshold : (view.settings || {}).compactBelow };
  return {
    name: d.name.trim() || 'Untitled',
    preset: d.preset,
    smallScreens: d.smallScreens,
    settings,
  };
}

// submit calls one of the editor's bound methods for draft d, and closes the
// editor if Go accepts it. While the call is on its way, the editor ignores
// further submissions, so that a double click can't add a tab twice, and a
// new View doesn't close it from under the call. after runs once Go has
// accepted the call, before the editor closes.
async function submit(d, method, args, after) {
  if (d.busy) return;
  d.busy = true;
  try {
    const r = await act(method, ...args);
    if (!r.ok) return;
    if (after) await after();
    if (store.ui.editor === d) closeOverlay();
  } finally {
    d.busy = false;
    // Check the draft against the latest View, which may have changed
    // while the call was on its way.
    if (store.ui.editor === d) update();
  }
}

// save saves the draft, then Open at login if it changed.
export async function save() {
  const d = store.ui.editor;
  if (!d) return;
  const view = store.view;
  await submit(d, 'SaveTab', [d.index, draftOf(d, view)], async () => {
    if (d.openAtLogin !== Boolean(view.openAtLogin)) await act('SetOpenAtLogin', d.openAtLogin);
  });
}

// deleteTab deletes the draft's tab. Its windows move to "Not in a tab".
export async function deleteTab() {
  const d = store.ui.editor;
  if (!d || d.index < 0) return;
  await submit(d, 'DeleteTab', [d.index]);
}

// resetLayout returns the draft's tab to its layout from config.toml.
export async function resetLayout() {
  const d = store.ui.editor;
  if (!d || d.index < 0) return;
  await submit(d, 'ResetLayout', [d.index]);
}

// followTab keeps the draft pointing at its tab when tabs are added or
// deleted elsewhere, by the tab's name, which is unique. It reports false if
// the tab has gone. A draft with a call on its way is left alone: its tab
// may be being renamed or deleted by that call.
export function followTab(d, view) {
  if (d.busy || d.index < 0) return true;
  if (view.tabs[d.index] && view.tabs[d.index].name === d.tabName) return true;
  const i = view.tabs.findIndex((t) => t.name === d.tabName);
  if (i < 0) return false;
  d.index = i;
  return true;
}

// explainShortcut says why the shortcut can't be changed.
export function explainShortcut() {
  const p = shortcutPrefix(store.view);
  if (p) toast('Shortcuts follow tab order: the first nine tabs get ' + p + '1 to ' + p + '9.');
  else toast('Tabs have no shortcuts, because tab_modifiers is empty in config.toml.');
}

function renderMini(view, node) {
  if (!node) return '<div class="mini-loading">Getting the preview…</div>';
  if (node.type === 'pane') {
    const chips = (node.items || []).map((id) => {
      const it = itemOf(view, id);
      return it ? '<span>' + esc(it.app + ': ' + it.title) + '</span>' : '';
    }).join('');
    return '<div class="mini-pane">' + (chips ? '<div class="chips">' + chips + '</div>' : '<span class="empty-note">Empty pane</span>') + '</div>';
  }
  const sizes = node.sizes || [];
  return '<div class="mini-split" data-dir="' + esc(node.dir) + '">' + (node.children || []).map((c, n) =>
    '<div class="mini-cell" style="flex: ' + (Number(sizes[n]) || 1) + ' 1 0px">' + renderMini(view, c) + '</div>').join('') + '</div>';
}

function thumbFor(id) {
  const s = (flex) => '<span style="flex: ' + flex + ' 1 0px"></span>';
  switch (id) {
    case 'single': return '<span class="thumb" aria-hidden="true">' + s(1) + '</span>';
    case 'side': return '<span class="thumb" aria-hidden="true">' + s(1) + s(1) + '</span>';
    case 'main-side': return '<span class="thumb" aria-hidden="true">' + s(65) + s(35) + '</span>';
    case 'three': return '<span class="thumb" aria-hidden="true">' + s(40) + s(30) + s(30) + '</span>';
    case 'top-bottom': return '<span class="thumb" aria-hidden="true" style="flex-direction: column">' + s(65) + s(35) + '</span>';
    case 'grid': return '<span class="thumb" aria-hidden="true" style="flex-direction: column"><span class="group" style="flex: 1 1 0px; flex-direction: row">' + s(1) + s(1) + '</span><span class="group" style="flex: 1 1 0px; flex-direction: row">' + s(1) + s(1) + '</span></span>';
    default: return '';
  }
}

// needsRecording reports whether the draft turns live previews on in the
// app without Screen Recording permission, which previews need.
function needsRecording(view, d) {
  return Boolean(d.settings.livePreviews) && !view.demo && !(view.permissions && view.permissions.screenRecording);
}

function shortcutText(view, d) {
  const p = shortcutPrefix(view);
  let keys = '';
  // The tab can be missing for a moment while Delete tab is on its way.
  if (d.index >= 0) keys = (view.tabs[d.index] && view.tabs[d.index].shortcut) || '';
  else if (p && view.tabs.length < 9) keys = p + (view.tabs.length + 1);
  if (keys) return keys;
  const k = keysOf(view);
  const steps = pairText(k.previousTab, k.nextTab);
  return steps ? 'None (use ' + steps + ' or Jump to…)' : 'None (use Jump to…)';
}

// renderEditor returns the layout editor's HTML.
export function renderEditor(view, ui) {
  const d = ui.editor;
  if (!d) return '';
  const threshold = parseInt(d.compactBelow, 10) || 0;
  const custom = d.preset === 'custom';
  const presets = PRESETS.map((p) =>
    '<button type="button" class="preset" data-action="draft-preset" data-preset="' + p.id + '" data-key="preset-' + p.id + '" aria-pressed="' + (d.preset === p.id) + '">'
    + thumbFor(p.id) + '<span>' + esc(p.label) + '</span></button>').join('');
  const compactSeg = SMALL_SCREENS.map((c) =>
    '<button type="button" data-action="draft-small" data-small="' + c.id + '" data-key="small-' + c.id + '" aria-pressed="' + (d.smallScreens === c.id) + '">'
    + esc(c.label) + '</button>').join('');

  const toggles = [
    ['openMissingApps', 'Open apps that aren\'t running', Boolean(d.settings.openMissingApps)],
    ['hideOtherTabs', 'Hide windows that belong to other tabs', Boolean(d.settings.hideOtherTabs)],
    ['followFocus', 'Switch tabs when another tab\'s window comes forward', d.settings.followFocus === 'switch-tab'],
    ['livePreviews', 'Show live previews of windows (needs Screen Recording)', Boolean(d.settings.livePreviews)],
    ['openAtLogin', 'Open Bench at login', d.openAtLogin],
  ].map(([id, label, on]) => '<label class="toggle"><input type="checkbox" data-setting="' + id + '" data-key="setting-' + id + '"'
    + (on ? ' checked' : '') + (id === 'livePreviews' && needsRecording(view, d) ? ' aria-describedby="recording-note"' : '') + '>'
    + '<span>' + esc(label) + '</span></label>'
    + (id === 'livePreviews' && needsRecording(view, d)
      ? '<p class="toggle-note" id="recording-note">' + esc('Bench doesn\'t have Screen Recording permission yet, so previews won\'t show. In System Settings, open Privacy & Security, then Screen Recording, and turn on Bench.') + '</p>'
      : '')).join('');

  const canDelete = d.index >= 0 && view.tabs.length > 1;
  const note = view.configWritable === false
    ? '<p class="editor-note" role="note">' + esc('config.toml has problems, so changes apply but aren\'t saved to it. The status bar lists the problems.') + '</p>'
    : '';

  return '<div class="dialog editor-sheet" role="dialog" aria-modal="true" aria-labelledby="editor-title">'
    + '<div class="editor-head"><div style="flex: 1 1 240px; min-width: 0">'
    + '<h2 id="editor-title">' + (d.index < 0 ? 'New tab' : 'Edit tab') + '</h2>'
    + '<label class="field-label" for="tab-name">Name</label>'
    + '<input class="text-input" id="tab-name" data-key="tab-name" type="text" autocomplete="off" value="' + esc(d.name) + '"></div>'
    + '<div style="display: flex; flex-direction: column; gap: 6px"><span class="field-label" id="shortcut-label" style="margin: 0">Shortcut</span>'
    + '<button type="button" class="shortcut-btn" data-action="shortcut" data-key="shortcut" aria-labelledby="shortcut-label shortcut-value shortcut-change">'
    + '<span id="shortcut-value" style="font-family: var(--font-mono)">' + esc(shortcutText(view, d)) + '</span><span id="shortcut-change" style="color: var(--text-muted)">Change</span></button></div></div>'
    + '<div class="editor-row"><h3>Layout</h3><p>' + (custom ? 'Custom, from dragging windows. Choose a layout to start again; windows stay in order.' : 'Windows stay in their panes, in order. Drag sub-tabs later to fine-tune.') + '</p></div>'
    + '<div class="presets" role="group" aria-label="Layout presets">' + presets + '</div>'
    + '<div class="preview-wrap"><div class="mini" role="group" aria-label="Preview of the layout">' + renderMini(view, d.preview) + '</div></div>'
    + '<div class="editor-row"><h3>On small screens</h3><div class="seg" role="group" aria-label="On small screens">' + compactSeg + '</div>'
    + '<p>Under ' + fmt(threshold) + ' points wide</p></div>'
    + '<div class="editor-settings">'
    + '<div><label for="compact-below" style="display: block; margin-bottom: 8px; font-weight: 500">Small screens are narrower than</label>'
    + '<div style="display: flex; align-items: center; gap: 8px"><input class="number-input" id="compact-below" data-key="compact-below" type="number" min="800" max="4000" step="10" value="' + esc(d.compactBelow) + '">'
    + '<span style="color: var(--text-muted)">points</span></div>'
    + '<p style="margin: 8px 0 0; font-size: 12px; color: var(--text-muted)">Bench compares this with the width of the screen the window is on.</p></div>'
    + '<div class="toggles">' + toggles + '</div></div>'
    + '<div class="editor-foot">'
    + (canDelete ? '<button type="button" class="btn danger" data-action="delete-tab" data-key="delete-tab">Delete tab</button>' : '')
    + (d.index >= 0 ? '<button type="button" class="btn" data-action="reset-layout" data-key="reset-layout">Reset layout</button>' : '')
    + '<div class="spacer"><button type="button" class="btn" data-action="close" data-key="editor-cancel">Cancel</button>'
    + '<button type="button" class="btn primary" data-action="save-tab" data-key="editor-save">Save</button></div></div>'
    + note
    + '</div>';
}
