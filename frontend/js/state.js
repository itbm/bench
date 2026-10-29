// The UI's state. Go owns the model and sends it as a View; the UI keeps only
// what's local to it: open menus and dialogs, the palette's query, a drag in
// progress.

import { call, errorText } from './bridge.js';
import { activeKey, contentOrigin, esc, focusKey, shownInFocus, termOfKey } from './util.js';

// store holds the latest View from Go and the UI's own state.
export const store = {
  // view is the latest workspace.View from Go, or null before the first.
  view: null,
  ui: {
    // menu is an open menu: { kind: 'window', itemId, paneId, shownIn, tab,
    // x, y, top, opener, webApp }, { kind: 'tab', index, tabName, x, y, top,
    // opener }, or { kind: 'notices', x, bottom, opener }, where opener is
    // the data-key of the control that opened it, tab is the tab it was
    // opened in, index and tabName are the tab a tab's menu is for, top is
    // where fitMenu placed it, which it keeps, and webApp is what Go offers
    // to put a web app in a web page's place (WebAppChoice), once it has
    // answered.
    menu: null,
    // menuEntries are the open menu's entries as last drawn.
    menuEntries: [],
    // overlay is the open dialog: null, 'palette', 'add', 'editor' (Tab
    // properties), 'settings', 'guide', 'quit' (the question before
    // quitting), or 'close' (the question before closing a tab or a
    // terminal).
    overlay: null,
    palette: { query: '', sel: 0 },
    // add is the add dialog: { paneId, tab, options, step: 'list' | 'url' |
    // 'make' | 'terminal', option, url, make, terminal, replace, busy },
    // where tab is the tab it was opened in, option the row that asked for
    // an address, terminal the New terminal… form's draft, { command, name,
    // folder, startAgain, save } (launchers.js), make { url,
    // browser, steps, poll } while the dialog waits for a browser to make a
    // web app, where poll is the call to AddWebAppWhenReady (or
    // UseWebAppWhenReady) under way, and busy is set while Go adds what the
    // address step asked for, or while the dialog stops waiting. With
    // replace { itemId, page }, the dialog opened at its make step, from a
    // web page's menu, to put the web app in the page's place (MakeWebApp):
    // page is its name, and paneId its pane.
    add: null,
    // editor is Tab properties' draft.
    editor: null,
    // settings is the Settings dialog's draft.
    settings: null,
    // quit is the question before quitting: { terminals, dontAsk, busy,
    // back }, where terminals are those running something, as Go named
    // them, and back is the dialog it covers, which Cancel goes back to
    // (quit.js).
    quit: null,
    // close is the question before closing: { kind: 'tab', tabName, q,
    // dontAsk, busy } or { kind: 'item', itemId, q, dontAsk, busy }, where
    // q is what Go would close (CloseQuestion, close.js).
    close: null,
    // rename is the field that renames a tab or sub-tab in place:
    // { kind: 'tab', index, tabName, value, busy, back } or { kind: 'item',
    // itemId, paneId, value, busy, back }, where value is what's typed, busy
    // is set while its save is on its way, and back is the data-key of the
    // control to focus once it closes (rename.js).
    rename: null,
    // guideClosed is set when the first-run guide is closed for now.
    guideClosed: false,
    // dialogPositions are where the user has moved or resized dialogs since
    // Bench started, by kind (overlay): their top-left, { x, y }, in window
    // content coordinates (dialogs.js).
    dialogPositions: {},
    // dialogSizes are the sizes that the user has given dialogs since Bench
    // started, by kind: { w, h }, as sent to Go (SetDialogSize). They win
    // over the View's, which may not have the latest yet.
    dialogSizes: {},
    // drag is a sub-tab being dragged, { itemId, paneId }, or a tab,
    // { tab } with its index (drag.js).
    drag: null,
    // divider is a divider being dragged.
    divider: null,
    // opener is the data-key of the control to focus when a menu or dialog
    // closes, or 'term:<item ID>' for a terminal.
    opener: null,
    // paneLater is what a click in a pane does once the menu or dialog it
    // opened closes, unless one of its entries runs: { paneId } focuses
    // the pane, and { paneId, itemId } shows the item there. Focusing an
    // App item's pane activates its app, which would take the keys meant
    // for Bench's menu or dialog, so it waits.
    paneLater: null,
    // pendingFocus is the data-key of the control to focus after the next
    // render.
    pendingFocus: null,
    // termFocus is a terminal that Go asked to give the keyboard focus, to
    // focus once it's the focused pane's shown item and no menu or dialog
    // is open: { id, until }, where until is 0 while one is open
    // (render.js).
    termFocus: null,
    // listItems are the palette or add dialog rows as last drawn.
    listItems: [],
    // choosing is set while macOS's folder picker, a sheet on Bench's
    // window, is open for a form's Choose… (launchers.js). Bench stays
    // raised meanwhile, even if the form closes.
    choosing: false,
  },
};

let renderer = () => {};

// setRenderer sets the function that update calls.
export function setRenderer(fn) {
  renderer = fn;
}

// update draws the UI from the store.
export function update() {
  renderer();
}

// isDemo reports whether this is the browser preview.
export function isDemo() {
  return Boolean(store.view && store.view.demo);
}

// act calls a bound method and shows Go's message as a toast if it's
// refused. It resolves with { ok, value }.
export async function act(method, ...args) {
  try {
    return { ok: true, value: await call(method, ...args) };
  } catch (err) {
    toast(errorText(err));
    return { ok: false, error: err };
  }
}

// lanes hold calls that must reach Go in order, by key. Go can handle calls
// out of order (each is its own request), so a lane sends one call at a
// time, and while one is on its way it keeps only the latest call to send
// next: for state that the last call sets, such as raised mode or a split's
// sizes.
const lanes = new Map();

// actLatest calls a bound method in the lane for key, after the call on its
// way in that lane, if any, replacing any call still waiting there. Go's
// refusals show as toasts, as with act.
export function actLatest(key, method, ...args) {
  let lane = lanes.get(key);
  if (!lane) {
    lane = { busy: false, next: null };
    lanes.set(key, lane);
  }
  lane.next = { method, args };
  if (!lane.busy) runLane(key, lane);
}

async function runLane(key, lane) {
  lane.busy = true;
  while (lane.next) {
    const { method, args } = lane.next;
    lane.next = null;
    await act(method, ...args);
  }
  lane.busy = false;
  lanes.delete(key);
}

let toastTimer = null;

// toastAction is the button of the toast that's showing, if it has one:
// { label, method, arg }.
let toastAction = null;

// TITLE_TOAST_MS is the longest, in milliseconds, that a toast shows in the
// title bar, where it covers the space that moves the window, and maybe
// tabs.
const TITLE_TOAST_MS = 4000;

// ACTION_TOAST_MS is how long, in milliseconds, a toast with a button, such
// as Undo, shows, wherever it is: long enough to read it and change one's
// mind.
const ACTION_TOAST_MS = 10000;

// TITLE_TOAST_ROOM is the narrowest, in points, that a space in the title
// bar can be for a toast to go there (placeToast).
const TITLE_TOAST_ROOM = 240;

// toast shows a short message at the bottom of the window, or, while the
// status bar is hidden, in the title bar, for a shorter time (placeToast).
// It can have one button, action: { label, method, arg }, such as Undo,
// which calls the bound method method with arg (runToastAction); the toast
// then shows for ACTION_TOAST_MS, wherever it is.
export function toast(text, action = null) {
  const el = document.getElementById('toast');
  if (!el || !text) return;
  toastAction = action && action.label && /^[A-Z][A-Za-z]*$/.test(action.method || '') ? action : null;
  if (toastAction) {
    el.innerHTML = '<span class="toast-text">' + esc(text) + '</span>'
      + '<button type="button" class="toast-action" data-action="toast-run">' + esc(toastAction.label) + '</button>';
  } else {
    el.textContent = text;
  }
  el.classList.toggle('with-action', Boolean(toastAction));
  const titled = placeToast();
  el.classList.add('show');
  clearTimeout(toastTimer);
  const ms = toastAction ? ACTION_TOAST_MS : Math.min(titled ? TITLE_TOAST_MS : 6000, Math.max(2200, String(text).length * 60));
  toastTimer = setTimeout(hideToast, ms);
}

// hideToast hides the toast. Its button, if it has one, no longer runs.
function hideToast() {
  const el = document.getElementById('toast');
  if (el) {
    el.classList.remove('show');
    if (el.contains(document.activeElement)) document.activeElement.blur();
  }
  toastAction = null;
}

// runToastAction runs the button of the toast that's showing, once: the
// toast goes, and Go's answer, such as what Undo put back, comes as a toast
// and a new View.
export function runToastAction() {
  const action = toastAction;
  clearTimeout(toastTimer);
  hideToast();
  if (action) act(action.method, ...(action.arg === undefined ? [] : [action.arg]));
}

// placeToast puts the toast where it can be seen, and reports whether
// that's in the title bar. In the app, docked windows and web views cover
// the pane bodies, so a toast sits over the status bar (see styles.css);
// while the status bar is hidden, it sits in the title bar, in the middle
// of its empty space between the + after the tabs, or the "tabs" view's
// sub-tabs and their controls, and the controls at its right. If that's
// too narrow, it goes over the tabs, and failing that,
// across the title bar, but never over the traffic lights, whose clicks
// macOS takes. The browser preview does the same.
export function placeToast() {
  const el = document.getElementById('toast');
  const bar = document.getElementById('statusbar');
  const title = document.getElementById('titlebar');
  if (!el) return false;
  const titled = Boolean(bar && bar.hidden && title);
  el.classList.toggle('in-titlebar', titled);
  if (!titled) {
    el.style.left = '';
    el.style.maxWidth = '';
    return false;
  }
  const o = contentOrigin();
  const r = title.getBoundingClientRect();
  const edge = (sel, side, fallback) => {
    const n = title.querySelector(sel);
    return n ? n.getBoundingClientRect()[side] : fallback;
  };
  // The traffic lights are in the title bar's left padding in the app, and
  // drawn at its start in the browser preview.
  const start = Math.max(r.left + parseFloat(getComputedStyle(title).paddingLeft), edge('.traffic', 'right', r.left) + 8);
  const end = edge('.titlebar-end', 'left', r.right) - 8;
  const after = title.querySelector('.title-pane') ? '.title-pane' : '.new-tab';
  const spaces = [[edge(after, 'right', r.left) + 8, end], [start, end], [start, r.right - 12]];
  const [left, right] = spaces.find(([a, b]) => b - a >= TITLE_TOAST_ROOM) || spaces[2];
  el.style.left = (left + right) / 2 - o.x + 'px';
  el.style.maxWidth = Math.max(0, right - left) + 'px';
  return true;
}

// overlayReported is what Go was last told, or null before the first
// render. The first render always tells Go, so that a reloaded page lowers
// a Bench left raised by the page before it.
let overlayReported = null;

// overlayOpen reports whether anything that needs raised mode is open.
export function overlayOpen() {
  const { ui } = store;
  return Boolean(ui.menu || ui.overlay || ui.drag || ui.choosing);
}

// syncOverlay tells Go when the first menu, dialog, or drag opens, and when
// the last one closes, so that Bench is raised above docked windows while
// any of them is open.
export function syncOverlay() {
  const open = overlayOpen();
  if (open === overlayReported) return;
  overlayReported = open;
  actLatest('overlay', 'SetOverlay', open);
}

// resendOverlay tells Go again whether anything that needs raised mode is
// open, after Ready has lowered Bench.
export function resendOverlay() {
  overlayReported = null;
  syncOverlay();
}

// overlaySent reports whether Go has been told about raised mode since the
// page loaded.
export function overlaySent() {
  return overlayReported !== null;
}

// canRestore reports whether focus may go back to the control or terminal
// with key when the menu or dialog it opened closes. A terminal that Go
// asked to focus while it was open takes its place if it's the focused
// pane's shown item (ui.termFocus); one that isn't drawn there yet takes
// the focus from the opener when it is. A terminal gets the focus back
// only if it's still the focused pane's shown item, so that typing doesn't
// reach a terminal outside the focused pane.
export function canRestore(key) {
  if (!key) return false;
  const shown = shownInFocus(store.view);
  const want = store.ui.termFocus;
  if (want && want.id === shown) return false;
  const term = termOfKey(key);
  return !term || term === shown;
}

// openerKey returns the data-key that focus goes back to when a dialog
// that opens now closes: that of the control that opened the open menu,
// which the dialog replaces, or else that of the focused control.
export function openerKey() {
  const m = store.ui.menu;
  return m ? m.opener : activeKey();
}

// closeOverlay closes the open dialog and any menu, and puts focus back
// where it was before the dialog opened (canRestore). Then it does what the
// click that opened the dialog would have done in its pane (ui.paneLater).
// Closing the question before quitting goes back to the dialog it covered
// instead, if that's still there.
export function closeOverlay() {
  const { ui } = store;
  const back = ui.overlay === 'quit' ? coveredDialog(ui) : null;
  ui.quit = null;
  ui.close = null;
  if (back) {
    ui.overlay = back;
    update();
    return;
  }
  if (ui.overlay === 'guide') ui.guideClosed = true;
  ui.overlay = null;
  ui.add = null;
  ui.editor = null;
  ui.settings = null;
  ui.menu = null;
  ui.listItems = [];
  const opener = ui.opener;
  ui.opener = null;
  // Asked before the render, which gives a waiting terminal the focus.
  const restore = canRestore(opener);
  update();
  if (restore) focusKey(opener);
  runPaneLater();
}

// coveredDialog returns the dialog that the question before quitting
// covers, if it's still there.
function coveredDialog(ui) {
  const back = ui.quit && ui.quit.back;
  if ((back === 'add' && !ui.add) || (back === 'editor' && !ui.editor) || (back === 'settings' && !ui.settings)) return null;
  return back || null;
}

// closeMenu closes an open menu, putting focus back on the control that
// opened it if restore is set (and canRestore agrees). Then it does what
// the click that opened the menu would have done in its pane
// (ui.paneLater).
export function closeMenu(restore) {
  const { ui } = store;
  if (!ui.menu) return;
  const opener = ui.menu.opener;
  ui.menu = null;
  const back = restore && canRestore(opener);
  update();
  if (back) focusKey(opener);
  runPaneLater();
}

// runPaneLater focuses the pane, or shows the item, that ui.paneLater
// holds, if the pane is still shown and that would change anything.
export function runPaneLater() {
  const { ui, view } = store;
  const later = ui.paneLater;
  ui.paneLater = null;
  if (!later || !view || !view.display || ui.menu || ui.overlay) return;
  const pane = (view.display.panes || []).find((p) => p.id === later.paneId);
  if (!pane) return;
  if (later.itemId) {
    if ((pane.items || []).includes(later.itemId) && pane.active !== later.itemId) act('ShowItem', later.paneId, later.itemId);
    else if (later.paneId !== 'compact' && view.display.focus !== later.paneId) act('FocusPane', later.paneId);
  } else if (later.paneId !== 'compact' && view.display.focus !== later.paneId) {
    act('FocusPane', later.paneId);
  }
}
