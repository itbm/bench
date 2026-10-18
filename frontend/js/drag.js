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
  document.querySelectorAll('.tab.drop-target').forEach((el) => el.classList.remove('drop-target'));
}

function onDragStart(e) {
  const sub = e.target.closest && e.target.closest('.subtab');
  if (!sub) return;
  const { ui } = store;
  hidePreview();
  ui.drag = { itemId: sub.dataset.item, paneId: sub.dataset.pane };
  e.dataTransfer.effectAllowed = 'move';
  e.dataTransfer.setData('text/plain', sub.dataset.item);
  sub.classList.add('dragging');
  syncOverlay();
  // Close an open menu only now, so that Bench stays raised throughout.
  if (ui.menu) {
    ui.menu = null;
    update();
  }
}

// onDragOver marks where a drop would land. It also handles dragenter:
// accepting both is what makes an element a drop target.
function onDragOver(e) {
  const { ui, view } = store;
  if (!ui.drag) return;
  const tabEl = e.target.closest && e.target.closest('[data-drop-tab]');
  if (tabEl) {
    e.preventDefault();
    clearDropMarks();
    if (Number(tabEl.dataset.dropTab) !== view.current) tabEl.classList.add('drop-target');
    return;
  }
  const paneEl = e.target.closest && e.target.closest('.pane[data-pane-id]');
  if (!paneEl) {
    clearDropMarks();
    return;
  }
  e.preventDefault();
  document.querySelectorAll('.tab.drop-target').forEach((el) => el.classList.remove('drop-target'));
  showIndicator(paneEl, zoneFor(paneEl, e.clientX, e.clientY));
}

function onDrop(e) {
  const { ui, view } = store;
  if (!ui.drag) return;
  e.preventDefault();
  const { itemId, paneId: from } = ui.drag;
  clearDropMarks();
  const tabEl = e.target.closest && e.target.closest('[data-drop-tab]');
  if (tabEl) {
    const t = Number(tabEl.dataset.dropTab);
    if (t !== view.current) act('MoveItemToTab', itemId, t);
    return;
  }
  const paneEl = e.target.closest && e.target.closest('.pane[data-pane-id]');
  if (!paneEl || paneEl.dataset.paneId === 'compact') return;
  const to = paneEl.dataset.paneId;
  const zone = zoneFor(paneEl, e.clientX, e.clientY);
  if (to === from && zone === 'center') return;
  act('MoveItem', itemId, view.current, to, zone);
}

function onDragEnd() {
  const { ui } = store;
  if (!ui.drag) return;
  ui.drag = null;
  clearDropMarks();
  document.querySelectorAll('.subtab.dragging').forEach((el) => el.classList.remove('dragging'));
  syncOverlay();
  renderIfDeferred();
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
  document.addEventListener('dragstart', onDragStart);
  document.addEventListener('dragenter', onDragOver);
  document.addEventListener('dragover', onDragOver);
  document.addEventListener('drop', onDrop);
  document.addEventListener('dragend', onDragEnd);
  document.addEventListener('pointerdown', onPointerDown);
  window.addEventListener('pointermove', onPointerMove);
  window.addEventListener('pointerup', onPointerUp);
  window.addEventListener('pointercancel', onPointerUp);
}
