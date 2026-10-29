// Tab properties: a sheet for a tab's own name, layout, and small-screen
// setting, or a new tab's, saved with SaveTab. It opens from a tab's menu
// (right-click a tab), the jump palette, View ▸ Tab Properties…, and + for a
// new tab. The settings that apply to every tab are in the Settings dialog
// (settings.js).

import { call, errorText } from './bridge.js';
import { act, closeOverlay, openerKey, store, toast, update } from './state.js';
import { GRIP } from './dialogs.js';
import {
  esc, fmt, itemOf, keysOf, pairText, shortcutPrefix, tabKey, uniqueTabName, unsavedNote,
} from './util.js';

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

// openEditor opens Tab properties for tab index, or for a new tab if index
// is -1. A new tab's name starts as "New tab", numbered if a tab has that
// name already.
export function openEditor(index) {
  const { ui } = store;
  const view = store.view;
  const tab = index >= 0 ? view.tabs[index] : null;
  if (index >= 0 && !tab) return;
  if (!ui.overlay) ui.opener = openerKey();
  ui.menu = null;
  ui.paneLater = null;
  ui.add = null;
  ui.settings = null;
  ui.overlay = 'editor';
  ui.editor = {
    index,
    // tabName is the tab's name when the editor opened, to find it by.
    tabName: tab ? tab.name : '',
    name: tab ? tab.name : uniqueTabName(view.tabs, 'New tab', -1),
    preset: tab ? tab.preset || 'single' : 'single',
    smallScreens: tab ? tab.smallScreens || 'stack' : 'stack',
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

// setName changes the draft's name. The field keeps what's typed in it
// without a redraw, so the note under it, which says what the tab will be
// called, changes in place.
export function setName(value) {
  const d = store.ui.editor;
  if (!d) return;
  d.name = value;
  const note = document.getElementById('tab-name-note');
  if (note) note.textContent = nameNote(store.view, d);
}

// nameNote says what the draft's tab will be called when that isn't the
// name typed: with no name, it's called Untitled, and Go numbers a name that
// another tab has, ignoring case (uniqueTabName). It's '' otherwise. It
// follows the tabs as they are, so it changes if a tab is added, renamed, or
// deleted while Tab properties is open.
export function nameNote(view, d) {
  const typed = d.name.trim();
  const name = uniqueTabName(view.tabs, typed || 'Untitled', d.index);
  if (!typed) return 'With no name, it will be called ' + name + '.';
  if (name === typed) return '';
  const other = view.tabs.find((t, i) => i !== d.index && t.name.toLowerCase() === typed.toLowerCase());
  return 'There\'s already a tab called ' + (other ? other.name : typed) + ', so this one will be called ' + name + '.';
}

// draftOf returns the TabDraft that SaveTab takes. Go numbers the name if
// it's taken (nameNote).
export function draftOf(d) {
  return {
    name: d.name.trim() || 'Untitled',
    preset: d.preset,
    smallScreens: d.smallScreens,
  };
}

// submit calls one of the editor's bound methods for draft d, and closes the
// editor if Go accepts it, after running accepted if it's given. While the
// call is on its way, the editor ignores further submissions, so that a
// double click can't add a tab twice, and a new View doesn't close it from
// under the call.
async function submit(d, method, args, accepted = null) {
  if (d.busy) return;
  d.busy = true;
  try {
    const r = await act(method, ...args);
    if (!r.ok) return;
    if (store.ui.editor !== d) return;
    if (accepted) accepted();
    closeOverlay();
  } finally {
    d.busy = false;
    // Check the draft against the latest View, which may have changed
    // while the call was on its way.
    if (store.ui.editor === d) update();
  }
}

// save saves the draft.
export async function save() {
  const d = store.ui.editor;
  if (!d) return;
  await submit(d, 'SaveTab', [d.index, draftOf(d)]);
}

// deleteTab deletes the draft's tab. Its windows move to "Not in a tab".
// The tab's button goes with it, so if it opened Tab properties, focus goes
// to the current tab's button instead. Once the View without the tab has
// come, render has done that already (followTabKey); until then, it's done
// here, and the focus follows the current tab when the View comes. (If the
// tab was current, Go chooses another, which the focus moves to then.)
export async function deleteTab() {
  const d = store.ui.editor;
  if (!d || d.index < 0) return;
  await submit(d, 'DeleteTab', [d.index], () => {
    const { ui, view } = store;
    const shown = view.tabs[d.index] && view.tabs[d.index].name === d.tabName;
    if (shown && ui.opener === tabKey(d.index)) ui.opener = tabKey(view.current);
  });
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
      return it && !it.hidden ? '<span>' + esc(it.app + ': ' + it.title) + '</span>' : '';
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

// smallScreensText says which screens the small-screen setting is for: those
// narrower than the threshold in Settings, which applies to every tab.
function smallScreensText(view) {
  const below = (view.settings || {}).compactBelow;
  return 'For screens narrower than ' + fmt(below > 0 ? below : 1800) + ' points, as set in Settings.';
}

// renderEditor returns Tab properties' HTML.
export function renderEditor(view, ui) {
  const d = ui.editor;
  if (!d) return '';
  const custom = d.preset === 'custom';
  const presets = PRESETS.map((p) =>
    '<button type="button" class="preset" data-action="draft-preset" data-preset="' + p.id + '" data-key="preset-' + p.id + '" aria-pressed="' + (d.preset === p.id) + '">'
    + thumbFor(p.id) + '<span>' + esc(p.label) + '</span></button>').join('');
  const compactSeg = SMALL_SCREENS.map((c) =>
    '<button type="button" data-action="draft-small" data-small="' + c.id + '" data-key="small-' + c.id + '" aria-pressed="' + (d.smallScreens === c.id) + '">'
    + esc(c.label) + '</button>').join('');

  const canDelete = d.index >= 0 && view.tabs.length > 1;
  const note = unsavedNote(view);

  // The note under the name is always there, empty unless the name will
  // change, so that screen readers hear it change. The name and the buttons
  // stay in view, and the rest scrolls if the window is short.
  return '<div class="dialog editor-sheet" role="dialog" aria-modal="true" aria-labelledby="editor-title">'
    + '<div class="editor-head dialog-handle">'
    + '<h2 id="editor-title">' + (d.index < 0 ? 'New tab' : 'Tab properties') + '</h2>'
    + '<div class="editor-fields"><div class="name-field">'
    + '<label class="field-label" for="tab-name">Name</label>'
    + '<input class="text-input" id="tab-name" data-key="tab-name" type="text" autocomplete="off" aria-describedby="tab-name-note" value="' + esc(d.name) + '">'
    + '<p class="field-note" id="tab-name-note" aria-live="polite">' + esc(nameNote(view, d)) + '</p></div>'
    + '<div class="shortcut-field"><span class="field-label" id="shortcut-label">Shortcut</span>'
    + '<button type="button" class="shortcut-btn" data-action="shortcut" data-key="shortcut" aria-labelledby="shortcut-label shortcut-value shortcut-change">'
    + '<span id="shortcut-value" style="font-family: var(--font-mono)">' + esc(shortcutText(view, d)) + '</span><span id="shortcut-change" style="color: var(--text-muted)">Change</span></button></div></div></div>'
    + '<div class="dialog-body" data-scroll="editor-body">'
    + '<div class="editor-row"><h3>Layout</h3><p>' + (custom ? 'Custom, from dragging windows. Choose a layout to start again; windows stay in order.' : 'Windows stay in their panes, in order. Drag sub-tabs later to fine-tune.') + '</p></div>'
    + '<div class="presets" role="group" aria-label="Layout presets">' + presets + '</div>'
    + '<div class="preview-wrap"><div class="mini" role="group" aria-label="Preview of the layout">' + renderMini(view, d.preview) + '</div></div>'
    + '<div class="editor-row"><h3 id="small-screens-label">On small screens</h3>'
    + '<div class="seg" role="group" aria-labelledby="small-screens-label" aria-describedby="small-screens-text">' + compactSeg + '</div>'
    + '<p id="small-screens-text">' + esc(smallScreensText(view)) + '</p></div></div>'
    + '<div class="editor-foot">'
    + (canDelete ? '<button type="button" class="btn danger" data-action="delete-tab" data-key="delete-tab">Delete tab</button>' : '')
    + (d.index >= 0 ? '<button type="button" class="btn" data-action="reset-layout" data-key="reset-layout">Reset layout</button>' : '')
    + '<div class="spacer"><button type="button" class="btn" data-action="close" data-key="editor-cancel">Cancel</button>'
    + '<button type="button" class="btn primary" data-action="save-tab" data-key="editor-save">Save</button></div></div>'
    + note
    + GRIP
    + '</div>';
}
