// Moving windows by dragging sub-tabs, and resizing panes by dragging the
// dividers between them.

import { act, actLatest, store, syncOverlay, update } from './state.js';
import { contentPoint } from './util.js';
import { hidePreview } from './preview.js';
import { renderIfDeferred } from './render.js';

// ---------- Sub-tabs ----------

function splitsDisabled() {
  const d = store.view && store.view.display;
  return Boolean(d && (d.compact || d.splitsDisabled));
}

// zoneFor says where a drop at (x, y) lands on a pane: an edge if it's in
// the outer quarter on that side, else the centre.
export function zoneFor(paneEl, x, y) {
  if (splitsDisabled()) return 'center';
  const r = paneEl.getBoundingClientRect();
  const px = (x - r.left) / r.width;
  const py = (y - r.top) / r.height;
  const edges = { left: px, right: 1 - px, top: py, bottom: 1 - py };
  const [edge, dist] = Object.entries(edges).sort((a, b) => a[1] - b[1])[0];
  return dist < 0.25 ? edge : 'center';
}

function showIndicator(paneEl, zone) {
  document.querySelectorAll('.drop-indicator').forEach((el) => { if (el.parentElement !== paneEl) el.hidden = true; });
  const ind = paneEl.querySelector('.drop-indicator');
  if (!ind) return;
  const box = {
    center: [6, 6, 'calc(100% - 12px)', 'calc(100% - 12px)'],
    left: [6, 6, 'calc(50% - 9px)', 'calc(100% - 12px)'],
    right: ['calc(50% + 3px)', 6, 'calc(50% - 9px)', 'calc(100% - 12px)'],
    top: [6, 6, 'calc(100% - 12px)', 'calc(50% - 9px)'],
    bottom: [6, 'calc(50% + 3px)', 'calc(100% - 12px)', 'calc(50% - 9px)'],
  }[zone];
  const len = (v) => (typeof v === 'number' ? v + 'px' : v);
  ind.style.left = len(box[0]);
  ind.style.top = len(box[1]);
  ind.style.width = len(box[2]);
  ind.style.height = len(box[3]);
  ind.dataset.zone = zone;
  ind.hidden = false;
}

function clearDropMarks() {
  document.querySelectorAll('.drop-indicator').forEach((el) => { el.hidden = true; });
  document.querySelectorAll('.tab.drop-target').forEach((el) => el.classList.remove('drop-target', 'drop-add'));
  if (ghost) ghost.classList.remove('adding');
}

// Sub-tab drags follow the pointer instead of using HTML drag and drop. In
// the app, WebKit runs an HTML drag as a native macOS drag session, which on
// a real Mac took seconds to deliver each drop and sometimes never ended, so
// the next drag couldn't start. Pointer events arrive like any other clicks.

// DRAG_START is how far the pointer must move, in CSS pixels, with the
// button held on a sub-tab before the press becomes a drag. A shorter press
// is a click, which shows the window.
const DRAG_START = 4;

// RELEASE_WAIT is how long, in milliseconds, a drag waits for the button to
// come up after a move that reports no button held. WebKit on macOS reads
// the buttons when it handles an event, not when it happened, so the last
// move of a quick release can arrive that way just before the pointerup.
// Only when no release follows did the button come up where Bench couldn't
// see it.
const RELEASE_WAIT = 250;

// press is a press on a sub-tab that may become a drag: the sub-tab, the
// pointer, and where it went down.
let press = null;
// ghost is the copy of the dragged sub-tab that follows the pointer.
let ghost = null;
// releaseTimer cancels the drag if no release follows a move with no button
// held (RELEASE_WAIT).
let releaseTimer = 0;

// targetAt finds what a drop at (x, y) would land on: a tab, a pane, or
// neither.
function targetAt(x, y) {
  const el = document.elementFromPoint(x, y);
  if (!el) return {};
  const tabEl = el.closest('[data-drop-tab]');
  if (tabEl) return { tabEl };
  const paneEl = el.closest('.pane[data-pane-id]');
  return paneEl ? { paneEl } : {};
}

function onSubtabDown(e) {
  if (e.button !== 0 || e.ctrlKey || store.ui.drag) return;
  const sub = e.target.closest && e.target.closest('.subtab');
  if (!sub) return;
  press = { sub, pointerId: e.pointerId, x: e.clientX, y: e.clientY };
}

function makeGhost(sub) {
  const g = document.createElement('div');
  g.className = 'drag-ghost';
  g.setAttribute('aria-hidden', 'true');
  for (const part of sub.querySelectorAll('.mono-tile, .subtab-title')) g.append(part.cloneNode(true));
  document.body.append(g);
  return g;
}

function startDrag(sub) {
  const { ui } = store;
  hidePreview();
  ui.drag = { itemId: sub.dataset.item, paneId: sub.dataset.pane };
  sub.classList.add('dragging');
  document.body.classList.add('dragging-subtab');
  ghost = makeGhost(sub);
  syncOverlay();
  // Close an open menu only now, so that Bench stays raised throughout. The
  // drag replaces what the click that opened it would have done
  // (ui.paneLater).
  if (ui.menu) {
    ui.menu = null;
    ui.paneLater = null;
    update();
  }
}

// markDrop moves the ghost to (x, y) and marks where a drop there would
// land. With add (Option held), a drop on another tab shows the window
// there as well as here, rather than moving it.
function markDrop(x, y, add) {
  const { ui, view } = store;
  ui.drag.at = { x, y };
  if (ghost) ghost.style.transform = 'translate(' + (x + 12) + 'px, ' + (y + 10) + 'px)';
  const { tabEl, paneEl } = targetAt(x, y);
  if (tabEl) {
    clearDropMarks();
    if (Number(tabEl.dataset.dropTab) !== view.current) {
      tabEl.classList.add('drop-target');
      tabEl.classList.toggle('drop-add', Boolean(add));
      if (ghost) ghost.classList.toggle('adding', Boolean(add));
    }
    return;
  }
  if (!paneEl) {
    clearDropMarks();
    return;
  }
  document.querySelectorAll('.tab.drop-target').forEach((el) => el.classList.remove('drop-target', 'drop-add'));
  if (ghost) ghost.classList.remove('adding');
  showIndicator(paneEl, zoneFor(paneEl, x, y));
}

// dropAt moves the dragged window to where a drop at (x, y) lands, or, with
// add, shows it in the tab dropped on as well.
function dropAt(x, y, add) {
  const { ui, view } = store;
  const { itemId, paneId: from } = ui.drag;
  const { tabEl, paneEl } = targetAt(x, y);
  if (tabEl) {
    const t = Number(tabEl.dataset.dropTab);
    if (t !== view.current) act(add ? 'AddItemToTab' : 'MoveItemToTab', itemId, t);
    return;
  }
  if (!paneEl || paneEl.dataset.paneId === 'compact') return;
  const to = paneEl.dataset.paneId;
  const zone = zoneFor(paneEl, x, y);
  if (to === from && zone === 'center') return;
  act('MoveItem', itemId, view.current, to, zone);
}

// endDrag ends a sub-tab drag, dropping at (x, y) if given (adding to a
// tab with add), or else cancelling it.
function endDrag(x, y, add) {
  const { ui } = store;
  press = null;
  clearTimeout(releaseTimer);
  releaseTimer = 0;
  if (!ui.drag) return;
  if (x !== undefined) dropAt(x, y, add);
  ui.drag = null;
  clearDropMarks();
  document.querySelectorAll('.subtab.dragging').forEach((el) => el.classList.remove('dragging'));
  document.body.classList.remove('dragging-subtab');
  if (ghost) ghost.remove();
  ghost = null;
  syncOverlay();
  renderIfDeferred();
}

// swallowClick stops the click that follows the button coming up at the
// end of a drag, which would otherwise show the window or focus a pane.
function swallowClick() {
  const stop = (e) => {
    e.preventDefault();
    e.stopPropagation();
  };
  window.addEventListener('click', stop, { capture: true, once: true });
  setTimeout(() => window.removeEventListener('click', stop, { capture: true }), 0);
}

function onSubtabMove(e) {
  if (store.ui.drag) {
    if ((e.buttons & 1) !== 0) {
      clearTimeout(releaseTimer);
      releaseTimer = 0;
      markDrop(e.clientX, e.clientY, e.altKey);
    } else if (!releaseTimer) {
      releaseTimer = setTimeout(() => endDrag(), RELEASE_WAIT);
    }
    return;
  }
  if (!press || e.pointerId !== press.pointerId) return;
  if ((e.buttons & 1) === 0 || !press.sub.isConnected) {
    press = null;
    return;
  }
  if (Math.hypot(e.clientX - press.x, e.clientY - press.y) < DRAG_START) return;
  const { sub } = press;
  press = null;
  startDrag(sub);
  markDrop(e.clientX, e.clientY, e.altKey);
}

// onSubtabUp drops on the button coming up. It handles mouseup as well as
// pointerup: when WebKit still reads the button as held as it handles the
// release, it sends the release on as a pointermove, and only the mouseup
// says the button came up.
function onSubtabUp(e) {
  if (!store.ui.drag) {
    press = null;
    return;
  }
  swallowClick();
  endDrag(e.clientX, e.clientY, e.altKey);
}

// onDragKey cancels a drag with Escape, and marks the drop again when
// Option goes down or up.
function onDragKey(e) {
  const { drag } = store.ui;
  if (!drag) return;
  if (e.key === 'Alt') {
    if (drag.at) markDrop(drag.at.x, drag.at.y, e.type === 'keydown');
    return;
  }
  if (e.key !== 'Escape' || e.type !== 'keydown') return;
  e.preventDefault();
  e.stopPropagation();
  endDrag();
}

// ---------- Dividers ----------

// MIN_SIZE is the smallest a child of a split can be dragged to, as a
// percentage of the split.
const MIN_SIZE = 10;

function dividerFor(el) {
  const d = store.view && store.view.display;
  if (!d) return null;
  return (d.dividers || []).find((dv) => dv.splitId === el.dataset.split && dv.index === Number(el.dataset.index)) || null;
}

// sizesAt returns the split's sizes with the divider at content position
// pos (x for a row, y for a column), keeping each of the two neighbouring
// children at MIN_SIZE or more. Positions map to sizes through the spec's
// frame formula, so the gap between the two children stays under the
// pointer.
export function sizesAt(drag, pos, gap) {
  const row = drag.dir === 'row';
  const start = row ? drag.split.x : drag.split.y;
  const len = row ? drag.split.w : drag.split.h;
  if (len + gap <= 0) return null;
  const at = ((pos - drag.offset - start + gap / 2) / (len + gap)) * 100;
  const i = drag.index;
  const before = drag.sizes.slice(0, i).reduce((a, b) => a + b, 0);
  const pair = drag.sizes[i] + drag.sizes[i + 1];
  if (!(pair >= 2 * MIN_SIZE)) return null;
  const a = Math.round(Math.min(Math.max(at - before, MIN_SIZE), pair - MIN_SIZE) * 100) / 100;
  const sizes = drag.sizes.slice();
  sizes[i] = a;
  sizes[i + 1] = Math.round((pair - a) * 100) / 100;
  return sizes;
}

// gapSetting is the gap between panes, in points.
function gapSetting() {
  const s = store.view && store.view.settings;
  return s && Number.isFinite(s.gap) ? s.gap : 8;
}

function centreOf(dv) {
  return dv.dir === 'row' ? dv.rect.x + dv.rect.w / 2 : dv.rect.y + dv.rect.h / 2;
}

function onPointerDown(e) {
  const el = e.target.closest && e.target.closest('.divider');
  if (!el || e.button !== 0) return;
  const dv = dividerFor(el);
  if (!dv) return;
  e.preventDefault();
  const p = contentPoint(e);
  const row = dv.dir === 'row';
  store.ui.divider = {
    splitId: dv.splitId,
    index: dv.index,
    dir: row ? 'row' : 'column',
    split: { ...dv.splitRect },
    sizes: dv.sizes.slice(),
    offset: (row ? p.x : p.y) - centreOf(dv),
    latest: null,
    sent: null,
    frame: 0,
  };
  try { el.setPointerCapture(e.pointerId); } catch { /* the element may be gone */ }
  el.classList.add('active');
  el.focus({ preventScroll: true });
  document.body.classList.add(row ? 'resizing-row' : 'resizing-column');
}

function send(drag) {
  if (!drag.latest) return;
  drag.sent = drag.latest;
  actLatest('sizes:' + drag.splitId, 'SetSizes', drag.splitId, drag.latest);
}

function onPointerMove(e) {
  const drag = store.ui.divider;
  if (!drag) return;
  // The button was released where Bench couldn't see it, such as outside
  // the window after a redraw replaced the divider that had captured the
  // pointer: end the drag rather than resizing on hover.
  if ((e.buttons & 1) === 0) {
    onPointerUp();
    return;
  }
  const p = contentPoint(e);
  const sizes = sizesAt(drag, drag.dir === 'row' ? p.x : p.y, gapSetting());
  if (!sizes) return;
  drag.latest = sizes;
  // At most one SetSizes per frame.
  if (!drag.frame) {
    drag.frame = requestAnimationFrame(() => {
      drag.frame = 0;
      if (store.ui.divider === drag && drag.latest !== drag.sent) send(drag);
    });
  }
}

function onPointerUp() {
  const drag = store.ui.divider;
  if (!drag) return;
  if (drag.frame) cancelAnimationFrame(drag.frame);
  store.ui.divider = null;
  document.body.classList.remove('resizing-row', 'resizing-column');
  document.querySelectorAll('.divider.active').forEach((el) => el.classList.remove('active'));
  // Once more on release, with the final sizes.
  send(drag);
}

// nudgeDivider moves a focused divider with the arrow keys, by 2% (or 10%
// with Shift).
export function nudgeDivider(el, key, shift) {
  const dv = dividerFor(el);
  if (!dv) return false;
  const row = dv.dir === 'row';
  const step = { ArrowLeft: row ? -1 : 0, ArrowRight: row ? 1 : 0, ArrowUp: row ? 0 : -1, ArrowDown: row ? 0 : 1 }[key];
  if (!step) return false;
  const i = dv.index;
  const pair = dv.sizes[i] + dv.sizes[i + 1];
  if (!(pair >= 2 * MIN_SIZE)) return true;
  const a = Math.min(Math.max(dv.sizes[i] + step * (shift ? 10 : 2), MIN_SIZE), pair - MIN_SIZE);
  const sizes = dv.sizes.slice();
  sizes[i] = a;
  sizes[i + 1] = pair - a;
  actLatest('sizes:' + dv.splitId, 'SetSizes', dv.splitId, sizes);
  return true;
}

// installDrag listens for drags of sub-tabs and dividers.
export function installDrag() {
  document.addEventListener('pointerdown', onSubtabDown);
  window.addEventListener('pointermove', onSubtabMove);
  window.addEventListener('pointerup', onSubtabUp);
  window.addEventListener('mouseup', onSubtabUp);
  window.addEventListener('pointercancel', () => endDrag());
  window.addEventListener('blur', () => endDrag());
  window.addEventListener('keydown', onDragKey, true);
  window.addEventListener('keyup', onDragKey, true);
  // WebKit could still start an HTML drag from a sub-tab's text.
  document.addEventListener('dragstart', (e) => {
    if (e.target.closest && e.target.closest('.subtab')) e.preventDefault();
  });
  document.addEventListener('pointerdown', onPointerDown);
  window.addEventListener('pointermove', onPointerMove);
  window.addEventListener('pointerup', onPointerUp);
  window.addEventListener('pointercancel', onPointerUp);
}
