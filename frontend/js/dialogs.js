// Moving and resizing dialogs. Tab properties, Settings, the add dialog,
// Jump to…, and the welcome guide move when the user drags their header
// (an element with the dialog-handle class), but not from a field or a
// button in it, and resize from the grip in their bottom-right corner. The
// questions before quitting and before closing (quit.js, close.js) and the
// Keyboard Shortcuts sheet (shortcuts.js) move too, but have no grip. A double-click on the header puts the dialog back
// where it opens, in the middle. Menus don't move.
//
// Where the user moves a dialog is kept by kind (the open overlay, such as
// 'settings') in store.ui.dialogPositions, and the size they give it in
// store.ui.dialogSizes, both in window content coordinates, as they left
// them. placeDialog writes them into the dialog's style after every render
// and whenever the window changes size, so redraws keep them, fitting the
// dialog to the window as it is then: it stays inside the window and, in
// the app, clear of the traffic lights, which take the clicks over them.
// Moving a dialog doesn't size it: one with no size of its own is as tall
// as its contents, down to the window's bottom edge, where its middle
// scrolls. Go remembers the size the user gives a dialog (SetDialogSize),
// so that it opens at that size, after a restart too; where it was is kept
// only while Bench runs.

import { actLatest, store } from './state.js';
import { contentPoint, contentRect, windowElement } from './util.js';

// EDGE is the least room, in points, that a dialog leaves between itself
// and the window's edges.
const EDGE = 8;

// TRAFFIC is the window's top-left corner, in points, where the traffic
// lights take clicks in the app. A dialog keeps out of it, so that its
// header can always be grabbed.
const TRAFFIC = { w: 78, h: 52 };

// MIN_SIZES are the smallest that dialogs can be made, by kind, in points.
// Jump to… and the add dialog can be shorter than the rest, since their
// lists scroll.
const MIN_SIZES = {
  palette: { w: 420, h: 200 },
  add: { w: 420, h: 200 },
};
const MIN_SIZE = { w: 420, h: 300 };

// MOVE_START is how far the pointer must move, in points, with the button
// held on a dialog's header or grip before the dialog moves or resizes, as
// for sub-tabs (drag.js), so that a click that wobbles changes nothing.
const MOVE_START = 4;

// NOT_HANDLE are the controls in a dialog's header that a press works as
// usual on, rather than moving the dialog.
const NOT_HANDLE = 'input, button, select, textarea, a, label';

// GRIP is the HTML of the grip in a dialog's bottom-right corner, which
// each dialog's HTML ends with.
export const GRIP = '<span class="dialog-grip" aria-hidden="true" title="Drag to resize"></span>';

// moving is a press on a dialog's header or grip: { kind, start, from,
// resize, started, before }, where start is where the press went down, in
// content coordinates, from is the dialog's frame on screen then, started
// says that the pointer has moved far enough to move or resize it, and
// before is where the dialog was and the size it had, as kept.
let moving = null;

// dialogElement returns the open dialog's element, or null.
function dialogElement() {
  return document.querySelector('#overlay-root .dialog');
}

// minSize returns the smallest a dialog of a kind can be made.
function minSize(kind) {
  return MIN_SIZES[kind] || MIN_SIZE;
}

// sizeOf returns the size the user gave a dialog of a kind, { w, h }, if
// they've resized one: this time Bench runs, or before (the View's
// dialogs).
function sizeOf(kind) {
  const size = store.ui.dialogSizes[kind] || ((store.view && store.view.dialogs) || {})[kind];
  return size && size.w > 0 && size.h > 0 ? { w: size.w, h: size.h } : null;
}

// openFrame returns where a dialog of a size opens: across the middle of
// the window, as high as the overlay puts dialogs (styles.css), which is
// lower on a tall window.
function openFrame(overlay, size) {
  const s = getComputedStyle(overlay);
  const left = parseFloat(s.paddingLeft) || 0;
  const right = parseFloat(s.paddingRight) || 0;
  const top = parseFloat(s.paddingTop) || 0;
  return { x: left + (overlay.clientWidth - left - right - size.w) / 2, y: top, w: size.w, h: size.h };
}

// fitFrame returns a dialog's frame made to fit the window: no smaller
// than the kind's smallest, unless the window is, no larger than the
// window, and inside it, EDGE from its edges. In the app, a dialog that
// would cover the traffic lights moves right or down, whichever is less of
// a move and fits, or else down, and shorter. A dialog with no size of its
// own (grows) is as tall as its contents, f.h, cut short to fit below its
// top, but not shorter than the kind's smallest, or its contents if they
// are less.
function fitFrame(f, kind, grows = false) {
  const win = windowElement();
  const width = win.clientWidth;
  const height = win.clientHeight;
  const min = minSize(kind);
  const maxW = Math.max(0, width - 2 * EDGE);
  const maxH = Math.max(0, height - 2 * EDGE);
  const w = Math.min(Math.max(f.w, Math.min(min.w, maxW)), maxW);
  let h = grows ? f.h : Math.min(Math.max(f.h, Math.min(min.h, maxH)), maxH);
  // least is how much of the dialog must fit below its top.
  const least = grows ? Math.min(f.h, min.h, maxH) : h;
  let x = Math.max(Math.min(f.x, width - EDGE - w), EDGE);
  let y = Math.max(Math.min(f.y, height - EDGE - least), EDGE);
  if (document.body.classList.contains('is-app') && x < TRAFFIC.w && y < TRAFFIC.h) {
    const right = TRAFFIC.w + w <= width - EDGE;
    const down = TRAFFIC.h + least <= height - EDGE;
    if (right && (!down || TRAFFIC.w - x <= TRAFFIC.h - y)) {
      x = TRAFFIC.w;
    } else {
      y = TRAFFIC.h;
    }
  }
  h = Math.min(h, Math.max(0, height - EDGE - y));
  return { x: Math.round(x), y: Math.round(y), w: Math.round(w), h: Math.round(h) };
}

// contentsSize returns the size of a dialog's element, placed with no size
// of its own: as wide as its kind lets it be (styles.css), but no wider
// than the window, and as tall as its contents.
function contentsSize(el) {
  el.classList.add('placed');
  el.style.maxWidth = '';
  el.style.width = Math.max(0, windowElement().clientWidth - 2 * EDGE) + 'px';
  el.style.height = '';
  el.style.maxHeight = 'none';
  return { w: el.offsetWidth, h: el.offsetHeight };
}

// setFrame places a dialog's element at a frame. One with no size of its
// own (grows) takes the frame's width, and the height of its contents, up
// to the window's bottom edge.
function setFrame(el, f, grows) {
  el.classList.add('placed');
  el.style.left = f.x + 'px';
  el.style.top = f.y + 'px';
  el.style.width = f.w + 'px';
  el.style.maxWidth = grows ? '' : 'none';
  el.style.height = grows ? '' : f.h + 'px';
  el.style.maxHeight = grows ? Math.max(0, windowElement().clientHeight - EDGE - f.y) + 'px' : '';
}

// unplace leaves a dialog's element where the overlay puts it.
function unplace(el) {
  if (!el.classList.contains('placed')) return;
  el.classList.remove('placed');
  for (const p of ['left', 'top', 'width', 'maxWidth', 'height', 'maxHeight']) el.style[p] = '';
}

// placeDialog writes the open dialog's frame into its style, fitted to the
// window, and returns it, or null for a dialog that the overlay places. A
// dialog that the user hasn't moved opens in the middle, and one that they
// haven't resized is as large as its contents. Only the style takes the
// fitted frame: where the user left the dialog, and the size they gave
// it, stay as they were, so that a dialog that a smaller window cut short
// gets them back when the window grows.
export function placeDialog() {
  const { ui } = store;
  const el = dialogElement();
  if (!el) return null;
  const kind = ui.overlay;
  const at = ui.dialogPositions[kind];
  const size = sizeOf(kind);
  if (!size) {
    if (!at) {
      unplace(el);
      return null;
    }
    const frame = fitFrame({ ...at, ...contentsSize(el) }, kind, true);
    setFrame(el, frame, true);
    return frame;
  }
  const frame = fitFrame(at ? { ...at, ...size } : openFrame(el.parentElement, size), kind);
  setFrame(el, frame, false);
  return frame;
}

// grabs says what a press on an element does to the open dialog: 'move'
// on its header, but not on a control in it, 'resize' on its grip, or ''.
function grabs(el) {
  if (!(el instanceof Element) || !el.closest('#overlay-root .dialog')) return '';
  if (el.closest('.dialog-grip')) return 'resize';
  return el.closest('.dialog-handle') && !el.closest(NOT_HANDLE) ? 'move' : '';
}

function onPointerDown(e) {
  if (e.button !== 0 || moving || !store.ui.overlay) return;
  const how = grabs(e.target);
  const el = dialogElement();
  if (!how || !el) return;
  const { ui } = store;
  const kind = ui.overlay;
  // A move or resize starts from where the dialog is drawn, which a
  // smaller window may have moved or cut short.
  const r = contentRect(el);
  const from = { x: Math.round(r.x), y: Math.round(r.y), w: Math.round(r.w), h: Math.round(r.h) };
  const before = { at: ui.dialogPositions[kind], size: ui.dialogSizes[kind] };
  moving = { kind, start: contentPoint(e), from, resize: how === 'resize', started: false, before };
  try { e.target.setPointerCapture(e.pointerId); } catch { /* the element may be gone */ }
  document.body.classList.add(how === 'resize' ? 'resizing-dialog' : 'moving-dialog');
}

// onMouseDown stops a press that moves or resizes a dialog from selecting
// text, or from taking the focus from the dialog's field. It cancels the
// mousedown rather than the pointerdown, which would also stop the
// mouseup.
function onMouseDown(e) {
  if (e.button === 0 && grabs(e.target)) e.preventDefault();
}

function onPointerMove(e) {
  const m = moving;
  if (!m) return;
  // The button came up where Bench couldn't see it, or the dialog has
  // closed, as with Escape.
  if ((e.buttons & 1) === 0 || store.ui.overlay !== m.kind) {
    endMove();
    return;
  }
  const p = contentPoint(e);
  const dx = p.x - m.start.x;
  const dy = p.y - m.start.y;
  if (!m.started && Math.hypot(dx, dy) < MOVE_START) return;
  m.started = true;
  const { ui } = store;
  const f = m.from;
  if (m.resize) {
    // The corner follows the pointer, up to the window's edge. The top-left
    // stays where it's drawn.
    const win = windowElement();
    const next = fitFrame({
      x: f.x, y: f.y,
      w: Math.min(f.w + dx, win.clientWidth - EDGE - f.x),
      h: Math.min(f.h + dy, win.clientHeight - EDGE - f.y),
    }, m.kind);
    ui.dialogPositions[m.kind] = { x: next.x, y: next.y };
    ui.dialogSizes[m.kind] = { w: next.w, h: next.h };
    placeDialog();
    return;
  }
  // A move keeps the dialog's size, and keeps where it fits, so that it
  // doesn't jump back from past the window's edge.
  ui.dialogPositions[m.kind] = { x: f.x + dx, y: f.y + dy };
  const shown = placeDialog();
  if (shown) ui.dialogPositions[m.kind] = { x: shown.x, y: shown.y };
}

// endMove ends a move or resize. A dialog that the user resized tells Go
// its new size, to open at next time; one that ends at the size it was
// drawn at keeps the size and place it had before.
function endMove() {
  const m = moving;
  if (!m) return;
  moving = null;
  document.body.classList.remove('moving-dialog', 'resizing-dialog');
  if (!m.resize || !m.started) return;
  const { ui } = store;
  const f = ui.dialogSizes[m.kind];
  if (f && (f.w !== m.from.w || f.h !== m.from.h)) {
    actLatest('dialog-size:' + m.kind, 'SetDialogSize', m.kind, f.w, f.h);
    return;
  }
  keep(ui.dialogPositions, m.kind, m.before.at);
  keep(ui.dialogSizes, m.kind, m.before.size);
  if (ui.overlay === m.kind) placeDialog();
}

// keep sets an entry of a map by kind, or deletes it for no value.
function keep(map, kind, value) {
  if (value) map[kind] = value;
  else delete map[kind];
}

// onDoubleClick puts a dialog whose header was double-clicked back where it
// opens, in the middle, at the size the user gave it.
function onDoubleClick(e) {
  if (grabs(e.target) !== 'move') return;
  delete store.ui.dialogPositions[store.ui.overlay];
  placeDialog();
}

// installDialogs listens for presses on dialogs' headers and grips, and
// keeps the open dialog inside the window as the window changes size.
export function installDialogs() {
  document.addEventListener('pointerdown', onPointerDown);
  document.addEventListener('mousedown', onMouseDown);
  document.addEventListener('dblclick', onDoubleClick);
  window.addEventListener('pointermove', onPointerMove);
  window.addEventListener('pointerup', endMove);
  // As for sub-tabs, only the mouseup may say that the button came up.
  window.addEventListener('mouseup', (e) => { if (e.button === 0) endMove(); });
  window.addEventListener('pointercancel', endMove);
  window.addEventListener('blur', endMove);
  new ResizeObserver(() => placeDialog()).observe(windowElement());
}
