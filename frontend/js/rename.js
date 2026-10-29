// Renaming in place. Double-clicking a tab, or choosing Rename… in its
// menu, turns its name into a text field; so does Rename… in the menu of a
// terminal's or web page's sub-tab, for a name of its own, which its sub-tab
// shows instead of the title its shell or page sets. Return saves, and
// Escape, or the field losing the focus, cancels.

import { act, overlayOpen, store, update } from './state.js';
import { esc, itemOf, labelOf, tabKey, uniqueTabName } from './util.js';
import { hidePreview } from './preview.js';

// MAX_NAME is the longest name the field takes, as Go keeps for a
// terminal's or web page's name (maxLabel).
const MAX_NAME = 200;

// SWITCH_WAIT is how long, in milliseconds, a click on the current tab
// waits before it gives the keyboard back to the tab's window: about a
// double-click's time, so that the double-click that renames the tab can
// stop it.
const SWITCH_WAIT = 300;

// switching is the SwitchTab that a click on a tab sent, while it's on its
// way to Go, and switchTimer is the one that a click on the current tab is
// waiting to send (clickTab).
let switching = null;
let switchTimer = 0;

// clickTab switches to tab index, for a click on it, with clicks, the
// click's count (0 from the keyboard). On the current tab, a click only
// gives the keyboard to the tab's window again, so a single click waits
// for SWITCH_WAIT, in case it's the first of a double-click, which renames
// the tab instead (startTabRename): the window would take the keyboard
// from the field.
export function clickTab(index, clicks) {
  cancelTabClick();
  const send = () => {
    const call = act('SwitchTab', index);
    switching = call;
    call.then(() => { if (switching === call) switching = null; });
  };
  if (clicks === 1 && store.view && store.view.current === index) {
    // By then, another tab, or a menu or dialog, can have the keyboard,
    // and the click is out of date.
    switchTimer = setTimeout(() => {
      switchTimer = 0;
      if (store.view && store.view.current === index && !overlayOpen()) send();
    }, SWITCH_WAIT);
    return;
  }
  send();
}

// cancelTabClick stops the switch that a click on the current tab is
// waiting to send.
function cancelTabClick() {
  clearTimeout(switchTimer);
  switchTimer = 0;
}

// takeKeyboard asks Go to give Bench's own UI the keyboard for field r
// (FocusUI), once the switch that a click sent, if any, has answered: the
// switch gives the keyboard to the tab's window or page, and so does Bench
// being lowered as a menu closes, which would leave the field without it.
function takeKeyboard(r) {
  Promise.resolve(switching).then(() => {
    if (store.ui.rename === r) act('FocusUI');
  });
}

// startTabRename turns tab index's name into a text field, with the name
// selected, to type a new one.
export function startTabRename(index) {
  const { ui, view } = store;
  const tab = view && view.tabs[index];
  if (!tab || ui.overlay || ui.drag) return;
  cancelTabClick();
  hidePreview();
  ui.rename = { kind: 'tab', index, tabName: tab.name, value: tab.name, busy: false, back: tabKey(index) };
  ui.pendingFocus = 'rename-input';
  update();
  takeKeyboard(ui.rename);
}

// stripOf returns pane paneId as drawn, if its sub-tabs are: in its header,
// or in the "tabs" view, in the title bar. A pane whose header is 0 has
// none.
function stripOf(view, paneId) {
  const d = (view && view.display) || {};
  const p = (d.panes || []).find((x) => x.id === paneId);
  if (!p) return null;
  return (p.id === 'compact' ? Boolean(d.compact) : Number(p.header) > 0) ? p : null;
}

// canRenameItem reports whether the sub-tab of itemId in pane paneId can be
// renamed in place: a terminal's or web page's, drawn there. An app's
// window takes its title from the app.
export function canRenameItem(view, itemId, paneId) {
  const item = itemOf(view, itemId);
  const p = stripOf(view, paneId);
  return Boolean(item) && item.kind !== 'app' && Boolean(p) && (p.items || []).includes(itemId);
}

// startItemRename turns the title of a terminal's or web page's sub-tab in
// a pane into a text field, with the title selected.
export function startItemRename(itemId, paneId) {
  const { ui, view } = store;
  const item = itemOf(view, itemId);
  if (!canRenameItem(view, itemId, paneId) || ui.overlay || ui.drag) return;
  hidePreview();
  ui.rename = { kind: 'item', itemId, paneId, value: item.title || item.app, busy: false, back: 'sub-' + itemId };
  ui.pendingFocus = 'rename-input';
  update();
  takeKeyboard(ui.rename);
}

// renamingTab reports whether the field is open for tab index.
export function renamingTab(index) {
  const r = store.ui.rename;
  return Boolean(r) && r.kind === 'tab' && r.index === index;
}

// renamingItem reports whether the field is open for the sub-tab of itemId
// in pane paneId.
export function renamingItem(itemId, paneId) {
  const r = store.ui.rename;
  return Boolean(r) && r.kind === 'item' && r.itemId === itemId && r.paneId === paneId;
}

// followRename keeps the field with its tab when tabs are added, deleted,
// or moved meanwhile, by the tab's name, as Tab properties does, and
// reports whether its tab or sub-tab is still there. A field whose save is
// on its way is left alone: its tab may be being renamed by it.
export function followRename(r, view) {
  if (r.busy) return true;
  if (r.kind === 'item') return canRenameItem(view, r.itemId, r.paneId);
  if (view.tabs[r.index] && view.tabs[r.index].name === r.tabName) return true;
  const i = view.tabs.findIndex((t) => t.name === r.tabName);
  if (i < 0) return false;
  r.index = i;
  r.back = tabKey(i);
  return true;
}

// tabNameNote says what a tab will be called when that isn't the name
// typed: Go numbers a name that another tab has, ignoring case, as Tab
// properties says (uniqueTabName). It's null otherwise, and for no name,
// which keeps the tab's name.
function tabNameNote(view, r) {
  const typed = r.value.trim();
  const name = uniqueTabName(view.tabs, typed, r.index);
  if (!typed || name === typed) return null;
  const other = view.tabs.find((t, i) => i !== r.index && t.name.toLowerCase() === typed.toLowerCase());
  return { name, text: 'There\'s already a tab called ' + (other ? other.name : typed) + ', so this one will be called ' + name + '.' };
}

// noteHTML returns the HTML inside the note after the field: the name a
// tab will have, with the whole sentence for a screen reader.
function noteHTML(note) {
  if (!note) return '';
  return '<span aria-hidden="true">→ ' + esc(note.name) + '</span><span class="visually-hidden">' + esc(note.text) + '</span>';
}

// renameField returns the field's HTML, in place of a tab's name or a
// sub-tab's title. The note after it, always there and empty unless the
// name will change, is read out when it does.
export function renameField(view, r) {
  const item = r.kind === 'item' ? itemOf(view, r.itemId) : null;
  const what = item ? (item.kind === 'builtin' ? 'terminal' : 'page') : 'tab';
  const label = item ? 'Name of this ' + what + ', ' + labelOf(view, r.itemId) : 'Name of the ' + r.tabName + ' tab';
  const help = item
    ? 'Return saves, Escape cancels. With no name, the ' + what + ' shows its automatic title.'
    : 'Return saves, Escape cancels.';
  const note = item ? null : tabNameNote(view, r);
  return '<input type="text" class="rename-input" id="rename-input" data-key="rename-input" value="' + esc(r.value) + '"'
    + ' maxlength="' + MAX_NAME + '" autocomplete="off" spellcheck="false" aria-label="' + esc(label) + '"'
    + ' aria-describedby="rename-help rename-note"' + (r.busy ? ' readonly' : '') + '>'
    + '<span class="visually-hidden" id="rename-help">' + esc(help) + '</span>'
    + '<span class="rename-note" id="rename-note" aria-live="polite"' + (note ? ' title="' + esc(note.text) + '"' : '') + '>' + noteHTML(note) + '</span>';
}

// setRenameValue keeps what's typed in the field without a redraw, so that
// the note after it changes in place.
export function setRenameValue(value) {
  const r = store.ui.rename;
  if (!r) return;
  r.value = value;
  if (r.kind !== 'tab') return;
  const el = document.getElementById('rename-note');
  if (!el) return;
  const note = tabNameNote(store.view, r);
  el.innerHTML = noteHTML(note);
  if (note) el.title = note.text;
  else el.removeAttribute('title');
}

// closeRename closes the field, and with restore gives the focus to its
// tab or sub-tab. A name that wasn't saved is dropped.
export function closeRename(restore) {
  const { ui } = store;
  const r = ui.rename;
  if (!r) return;
  ui.rename = null;
  if (restore) ui.pendingFocus = r.back;
  update();
}

// commitRename saves the name typed. A tab's name is saved with SaveTab,
// with its layout and small-screen setting as they are ("custom" keeps the
// tab's panes as they are), and numbered by Go if another tab has it; an
// unchanged or empty name keeps the tab's. A terminal's or web page's
// name is saved with RenameItem, and an empty one gives it its automatic
// title back. While the call is on its way, the field waits; if Go refuses,
// its message shows, and the field stays for another try, unless the focus
// has gone elsewhere meanwhile.
export async function commitRename() {
  const { ui, view } = store;
  const r = ui.rename;
  if (!r || r.busy || !view) return;
  const typed = r.value.trim();
  let call;
  if (r.kind === 'tab') {
    const tab = view.tabs[r.index];
    if (!tab || !typed || typed === tab.name) {
      closeRename(true);
      return;
    }
    call = ['SaveTab', r.index, { name: typed, preset: 'custom', smallScreens: tab.smallScreens || 'stack' }];
  } else {
    const item = itemOf(view, r.itemId);
    const shown = item ? item.label || '' : '';
    if (!item || typed === shown || (!item.label && typed === item.title)) {
      closeRename(true);
      return;
    }
    call = ['RenameItem', r.itemId, typed];
  }
  r.busy = true;
  update();
  const result = await act(...call);
  r.busy = false;
  if (store.ui.rename !== r) return;
  if (result.ok || document.activeElement !== document.getElementById('rename-input')) {
    closeRename(result.ok);
    return;
  }
  update();
}

// useAutomaticTitle takes a terminal's or web page's own name away, so its
// sub-tab shows the title its shell or page sets.
export function useAutomaticTitle(itemId) {
  act('RenameItem', itemId, '');
}

// renameKeys handles Return and Escape in the field. Keys that an input
// method uses to compose text are its own.
export function renameKeys(e) {
  if (e.isComposing || e.keyCode === 229) return;
  if (e.key === 'Enter') {
    e.preventDefault();
    commitRename();
  } else if (e.key === 'Escape') {
    e.preventDefault();
    e.stopPropagation();
    closeRename(true);
  }
}

// onRenameFocusOut cancels when the field loses the focus: to a click
// elsewhere, or to another app. A redraw that replaces the field gives the
// new one the focus straight away, which doesn't count, nor does a save on
// its way.
export function onRenameFocusOut(e) {
  if (!e.target || e.target.id !== 'rename-input') return;
  const r = store.ui.rename;
  if (!r) return;
  setTimeout(() => {
    if (store.ui.rename !== r || r.busy) return;
    const el = document.getElementById('rename-input');
    if (el && document.activeElement === el && document.hasFocus()) return;
    closeRename(false);
  }, 0);
}
