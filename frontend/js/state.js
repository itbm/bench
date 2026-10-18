// The UI's state. Go owns the model and sends it as a View; the UI keeps only
// what's local to it: open menus and dialogs, the palette's query, a drag in
// progress.

import { call, errorText } from './bridge.js';
import { focusKey } from './util.js';

// store holds the latest View from Go and the UI's own state.
export const store = {
  // view is the latest workspace.View from Go, or null before the first.
  view: null,
  ui: {
    // menu is an open menu: { kind: 'window', itemId, paneId, x, y, opener }
    // or { kind: 'notices', x, bottom, opener }, where opener is the data-key
    // of the control that opened it.
    menu: null,
    // menuEntries are the open menu's entries as last drawn.
    menuEntries: [],
    // overlay is the open dialog: null, 'palette', 'add', 'editor', or
    // 'guide'.
    overlay: null,
    palette: { query: '', sel: 0 },
    // add is the add dialog: { paneId, options, step: 'list' | 'url',
    // option, url }.
    add: null,
    // editor is the layout editor's draft.
    editor: null,
    // guideClosed is set when the first-run guide is closed for now.
    guideClosed: false,
    // drag is a sub-tab being dragged: { itemId, paneId }.
    drag: null,
    // divider is a divider being dragged.
    divider: null,
    // opener is the data-key of the control to focus when a menu or dialog
    // closes.
    opener: null,
    // pendingFocus is the data-key of the control to focus after the next
    // render.
    pendingFocus: null,
    // listItems are the palette or add dialog rows as last drawn.
    listItems: [],
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

// toast shows a short message at the bottom of the window.
export function toast(text) {
  const el = document.getElementById('toast');
  if (!el || !text) return;
  el.textContent = text;
  el.classList.add('show');
  clearTimeout(toastTimer);
  const ms = Math.min(6000, Math.max(2200, String(text).length * 60));
  toastTimer = setTimeout(() => el.classList.remove('show'), ms);
}

// overlayReported is what Go was last told, or null before the first
// render. The first render always tells Go, so that a reloaded page lowers
// a Bench left raised by the page before it.
let overlayReported = null;

// overlayOpen reports whether anything that needs raised mode is open.
export function overlayOpen() {
  const { ui } = store;
  return Boolean(ui.menu || ui.overlay || ui.drag);
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

// closeOverlay closes the open dialog and any menu, and puts focus back
// where it was before the dialog opened.
export function closeOverlay() {
  const { ui } = store;
  if (ui.overlay === 'guide') ui.guideClosed = true;
  ui.overlay = null;
  ui.add = null;
  ui.editor = null;
  ui.menu = null;
  ui.listItems = [];
  const opener = ui.opener;
  ui.opener = null;
  update();
  if (opener) focusKey(opener);
}

// closeMenu closes an open menu, putting focus back on the control that
// opened it if restore is set.
export function closeMenu(restore) {
  const { ui } = store;
  if (!ui.menu) return;
  const opener = ui.menu.opener;
  ui.menu = null;
  update();
  if (restore && opener) focusKey(opener);
}
