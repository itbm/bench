// Moving windows by dragging sub-tabs, putting tabs in order by dragging
// them along the title bar, and resizing panes by dragging the dividers
// between them. A drag near either end of a strip of tabs or sub-tabs
// scrolls it (edgeScroll in strips.js).

import { act, actLatest, store, syncOverlay, update } from './state.js';
import {
  activeKey, contentPoint, focusKey, RELEASE_WAIT, shownInFocus, termOfKey,
} from './util.js';
import { hidePreview } from './preview.js';
import { cancelTabClick } from './rename.js';
import { renderIfDeferred } from './render.js';
import { edgeScroll, stopEdgeScroll } from './strips.js';

// ---------- Sub-tabs and tabs ----------

// Where a dragged sub-tab lands follows one rule, which Go holds too, for
// when it finds drop targets itself (layout.DropAt, and decision 38 in
// architecture.md). Over a strip of sub-tabs, which is a pane's header, or
// in the "tabs" view its part of the title bar, it goes between two
// sub-tabs, where the caret shows, and never splits the pane. Over a
// pane's body, it splits the pane at an edge, in the outer quarter on that
// side, or joins the pane in the middle. Over a tab, it moves the window to
// that tab, and over the tabs' +, or the free space in the title bar
// just after it, to a new tab of its own. A dragged tab goes between two
// tabs.

function splitsDisabled() {
  const d = store.view && store.view.display;
  return Boolean(d && (d.compact || d.splitsDisabled));
}

// bodyOf returns a pane's body, which is under its header.
function bodyOf(paneEl) {
  return paneEl.querySelector(':scope > .pane-body') || paneEl;
}

// zoneFor says where a drop at (x, y) lands on a pane's body: an edge if
// it's in the outer quarter on that side, else the centre.
export function zoneFor(paneEl, x, y) {
  if (splitsDisabled()) return 'center';
  const r = bodyOf(paneEl).getBoundingClientRect();
  const px = (x - r.left) / r.width;
  const py = (y - r.top) / r.height;
  const edges = { left: px, right: 1 - px, top: py, bottom: 1 - py };
  const [edge, dist] = Object.entries(edges).sort((a, b) => a[1] - b[1])[0];
  return dist < 0.25 ? edge : 'center';
}

// showIndicator shows where a drop on a pane's body lands: on the half of
// the body that the window will take, for an edge, or on all of it.
function showIndicator(paneEl, zone) {
  hideCaret();
  document.querySelectorAll('.drop-indicator').forEach((el) => { if (el.parentElement !== paneEl) el.hidden = true; });
  const ind = paneEl.querySelector('.drop-indicator');
  if (!ind) return;
  // The indicator is placed in the pane, inside its border.
  const p = paneEl.getBoundingClientRect();
  const b = bodyOf(paneEl).getBoundingClientRect();
  const x = b.left - p.left - paneEl.clientLeft;
  const y = b.top - p.top - paneEl.clientTop;
  const w = b.width;
  const h = b.height;
  const box = {
    center: [6, 6, w - 12, h - 12],
    left: [6, 6, w / 2 - 9, h - 12],
    right: [w / 2 + 3, 6, w / 2 - 9, h - 12],
    top: [6, 6, w - 12, h / 2 - 9],
    bottom: [6, h / 2 + 3, w - 12, h / 2 - 9],
  }[zone];
  ind.style.left = (x + box[0]) + 'px';
  ind.style.top = (y + box[1]) + 'px';
  ind.style.width = Math.max(0, box[2]) + 'px';
  ind.style.height = Math.max(0, box[3]) + 'px';
  ind.dataset.zone = zone;
  ind.hidden = false;
}

// caret is the line between two tabs or sub-tabs that marks where a drop
// on their strip lands.
let caret = null;

// showCaret puts the caret at place gap among els, the tabs or sub-tabs of
// strip; kind, 'tab' or 'subtab', styles it.
function showCaret(strip, els, gap, kind) {
  document.querySelectorAll('.drop-indicator').forEach((el) => { el.hidden = true; });
  if (!caret) {
    caret = document.createElement('div');
    caret.className = 'drop-caret';
    caret.setAttribute('aria-hidden', 'true');
    document.body.append(caret);
  }
  const s = strip.getBoundingClientRect();
  const rect = (i) => els[i].getBoundingClientRect();
  let x = s.left;
  if (els.length && gap <= 0) x = rect(0).left;
  else if (els.length && gap >= els.length) x = rect(els.length - 1).right;
  else if (els.length) x = (rect(gap - 1).right + rect(gap).left) / 2;
  // It stays on the part of the strip in view.
  x = Math.min(Math.max(x, s.left + 1), Math.max(s.left + 1, s.right - 1));
  // An empty strip's caret is as tall as most of its header.
  let top;
  let height;
  if (els.length) {
    const r = rect(0);
    top = r.top;
    height = r.height;
  } else {
    const r = (strip.parentElement || strip).getBoundingClientRect();
    top = r.top + r.height / 5;
    height = (r.height * 3) / 5;
  }
  caret.dataset.kind = kind;
  caret.style.left = x + 'px';
  caret.style.top = top + 'px';
  caret.style.height = height + 'px';
  caret.hidden = false;
}

function hideCaret() {
  if (caret) caret.hidden = true;
}

// clearTabMarks takes the marks of a drop on a tab, or on the tabs' +, off.
function clearTabMarks() {
  document.querySelectorAll('.tab.drop-target, [data-drop-new].drop-target').forEach((el) => el.classList.remove('drop-target', 'drop-add'));
  if (ghost) ghost.classList.remove('adding');
}

function clearDropMarks() {
  document.querySelectorAll('.drop-indicator').forEach((el) => { el.hidden = true; });
  clearTabMarks();
  hideCaret();
}

// stripPlace returns where a drop at x on a strip lands among els, its tabs
// or sub-tabs, as gap: before the first whose middle is right of x, or
// after the last, for a drop past the end of the strip in view, such as on
// the controls after it. moving is the place among els of the tab or
// sub-tab being dragged, or -1 if it's in another strip. A drop right
// before or after it would leave the strip as it is, so it's still.
function stripPlace(strip, els, x, moving) {
  const s = strip.getBoundingClientRect();
  let gap = els.length;
  if (x < s.right) {
    const at = Math.max(x, s.left);
    const i = els.findIndex((el) => {
      const r = el.getBoundingClientRect();
      return at < r.left + r.width / 2;
    });
    if (i >= 0) gap = i;
  }
  return { gap, still: moving >= 0 && (gap === moving || gap === moving + 1) };
}

// subtabPlace returns where a sub-tab dropped at x on a strip of sub-tabs
// lands (stripPlace), counting the sub-tabs as drawn, as Go's MoveItemAt
// does.
function subtabPlace(strip, x) {
  const els = [...strip.querySelectorAll('.subtab[data-item]')];
  const moving = els.findIndex((el) => el.dataset.item === store.ui.drag.itemId);
  return { els, ...stripPlace(strip, els, x, moving) };
}

// tabPlace returns where a tab dropped at (x, y) lands among the tabs
// (stripPlace), or null for a drop outside the title bar, which leaves the
// tab where it is.
function tabPlace(x, y) {
  const bar = document.getElementById('titlebar');
  const strip = bar && bar.querySelector('.tabs');
  if (!strip) return null;
  const b = bar.getBoundingClientRect();
  if (x < b.left || x > b.right || y < b.top || y > b.bottom) return null;
  const els = [...strip.querySelectorAll('.tab[data-index]')];
  return { strip, els, ...stripPlace(strip, els, x, store.ui.drag.tab) };
}

// Drags follow the pointer instead of using HTML drag and drop. In the app,
// WebKit runs an HTML drag as a native macOS drag session, which on a real
// Mac took seconds to deliver each drop and sometimes never ended, so the
// next drag couldn't start. Pointer events arrive like any other clicks.

// DRAG_START is how far the pointer must move, in CSS pixels, with the
// button held on a tab or sub-tab before the press becomes a drag. A
// shorter press is a click, which shows the tab or the window.
const DRAG_START = 4;

// press is a press on a tab or sub-tab that may become a drag: the
// element, the pointer, and where it went down.
let press = null;
// ghost is the copy of the dragged tab or sub-tab that follows the pointer.
let ghost = null;
// releaseTimer cancels the drag if no release follows a move with no button
// held (RELEASE_WAIT).
let releaseTimer = 0;
// cancelled is set when a drag is cancelled, by Escape say, while the
// button may still be held, so that the click its release makes doesn't
// show the window or focus a pane. A press clears it: by then the button
// came up where Bench couldn't see it.
let cancelled = false;

// newTabAt reports whether a drop of a sub-tab on el, at (x, y), makes a
// new tab for its window: on the tabs' +, or on the title bar just after
// it, beside it, for as far again as the + is wide, short of the next
// control. The rest of the title bar is the window's, so a drop there
// still cancels the drag. That space still moves the window when pressed:
// a drag of a sub-tab is Bench's own, and doesn't press there.
function newTabAt(el, x, y) {
  if (el.closest('[data-drop-new]')) return true;
  const bar = document.getElementById('titlebar');
  const plus = bar && bar.querySelector('[data-drop-new]');
  if (el !== bar || !plus) return false;
  const r = plus.getBoundingClientRect();
  const next = plus.nextElementSibling;
  const n = next ? next.getBoundingClientRect() : null;
  const nextLeft = n && n.top < r.bottom && n.bottom > r.top ? n.left : bar.getBoundingClientRect().right;
  const end = Math.min(nextLeft, r.right + r.width);
  return x >= r.right && x < end && y >= r.top - 8 && y <= r.bottom + 8;
}

// targetAt finds what a drop of a sub-tab at (x, y) would land on: a new
// tab (newTabAt), a tab, a strip of sub-tabs with the pane it's for, a
// pane's body, or none of them.
function targetAt(x, y) {
  const el = document.elementFromPoint(x, y);
  if (!el) return {};
  if (newTabAt(el, x, y)) return { newTab: true };
  const tabEl = el.closest('[data-drop-tab]');
  if (tabEl) return { tabEl };
  const head = el.closest('.pane-header, .title-pane');
  if (head) {
    const strip = head.querySelector('.subtabs');
    const holder = head.closest('[data-pane-id]');
    return strip && holder ? { strip, paneId: holder.dataset.paneId } : {};
  }
  const paneEl = el.closest('.pane[data-pane-id]');
  return paneEl ? { paneEl } : {};
}

// onPressDown notes a press on a sub-tab, or on a tab when there are
// others to put it among. The title bar around the tabs moves the window
// in the app, and isn't a tab.
function onPressDown(e) {
  if (e.button !== 0 || e.ctrlKey || store.ui.drag) return;
  const el = e.target.closest && e.target.closest('.subtab, .tab[data-index]');
  // A press on the × of a tab or sub-tab closes it, rather than drag it,
  // and one on a tab or sub-tab being renamed selects its text.
  if (!el || e.target.closest('.close-cross') || el.classList.contains('renaming')) return;
  if (el.classList.contains('tab') && !(store.view && store.view.tabs.length > 1)) return;
  press = { el, pointerId: e.pointerId, x: e.clientX, y: e.clientY };
}

function makeGhost(el) {
  const g = document.createElement('div');
  g.className = 'drag-ghost';
  g.setAttribute('aria-hidden', 'true');
  for (const part of el.querySelectorAll('.mono-tile, .subtab-title, .tab-name')) g.append(part.cloneNode(true));
  document.body.append(g);
  return g;
}

// startDrag starts dragging el, a tab or a sub-tab.
function startDrag(el) {
  const { ui } = store;
  hidePreview();
  // A click on the current tab just before the drag would otherwise send
  // its switch after the drop, and take Bench back from the tab that the
  // drop made.
  cancelTabClick();
  if (el.classList.contains('tab')) {
    ui.drag = { tab: Number(el.dataset.index) };
    document.body.classList.add('dragging-tab');
  } else {
    ui.drag = { itemId: el.dataset.item, paneId: el.dataset.pane };
    document.body.classList.add('dragging-subtab');
  }
  el.classList.add('dragging');
  ghost = makeGhost(el);
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
// land. With add (Option held), a drop of a sub-tab on another tab, or on
// the tabs' +, shows the window there as well as here, rather than moving
// it.
function markDrop(x, y, add) {
  const { ui, view } = store;
  const { drag } = ui;
  drag.at = { x, y };
  drag.add = Boolean(add);
  if (ghost) ghost.style.transform = 'translate(' + (x + 12) + 'px, ' + (y + 10) + 'px)';
  if (drag.tab !== undefined) {
    const place = tabPlace(x, y);
    if (place && !place.still) showCaret(place.strip, place.els, place.gap, 'tab');
    else hideCaret();
    return;
  }
  const { newTab, tabEl, strip, paneEl } = targetAt(x, y);
  if (newTab) {
    clearDropMarks();
    const plus = document.querySelector('#titlebar [data-drop-new]');
    if (plus) {
      plus.classList.add('drop-target');
      plus.classList.toggle('drop-add', Boolean(add));
    }
    if (ghost) ghost.classList.toggle('adding', Boolean(add));
    return;
  }
  if (tabEl) {
    clearDropMarks();
    if (Number(tabEl.dataset.dropTab) !== view.current) {
      tabEl.classList.add('drop-target');
      tabEl.classList.toggle('drop-add', Boolean(add));
      if (ghost) ghost.classList.toggle('adding', Boolean(add));
    }
    return;
  }
  if (strip) {
    const place = subtabPlace(strip, x);
    clearDropMarks();
    if (!place.still) showCaret(strip, place.els, place.gap, 'subtab');
    return;
  }
  if (!paneEl) {
    clearDropMarks();
    return;
  }
  clearTabMarks();
  showIndicator(paneEl, zoneFor(paneEl, x, y));
}

// dropAt does what a drop at (x, y) does: moves the dragged tab, or moves
// the dragged window, or, with add, shows it in the tab dropped on as well.
// A window dropped on the tabs' + gets a new tab of its own (Go names it
// after its app), as Move to New Tab does, or with add, Also Show in New
// Tab.
function dropAt(x, y, add) {
  const { ui, view } = store;
  const { drag } = ui;
  if (drag.tab !== undefined) {
    const place = tabPlace(x, y);
    if (place && !place.still) act('MoveTab', drag.tab, place.gap > drag.tab ? place.gap - 1 : place.gap);
    return;
  }
  const { itemId, paneId: from } = drag;
  const { newTab, tabEl, strip, paneId, paneEl } = targetAt(x, y);
  if (newTab) {
    act('MoveItemToNewTab', itemId, Boolean(add));
    return;
  }
  if (tabEl) {
    const t = Number(tabEl.dataset.dropTab);
    if (t !== view.current) act(add ? 'AddItemToTab' : 'MoveItemToTab', itemId, t);
    return;
  }
  if (strip) {
    const place = subtabPlace(strip, x);
    if (!place.still) act('MoveItemAt', itemId, view.current, paneId, place.gap);
    return;
  }
  if (!paneEl || paneEl.dataset.paneId === 'compact') return;
  const to = paneEl.dataset.paneId;
  const zone = zoneFor(paneEl, x, y);
  if (to === from && zone === 'center') return;
  act('MoveItem', itemId, view.current, to, zone);
}

// endDrag ends a drag, dropping at (x, y) if given (adding to a tab with
// add), or else cancelling it.
function endDrag(x, y, add) {
  const { ui } = store;
  press = null;
  clearTimeout(releaseTimer);
  releaseTimer = 0;
  if (!ui.drag) return;
  stopEdgeScroll();
  if (x !== undefined) dropAt(x, y, add);
  else cancelled = true;
  ui.drag = null;
  clearDropMarks();
  if (caret) caret.remove();
  caret = null;
  document.querySelectorAll('.subtab.dragging, .tab.dragging').forEach((el) => el.classList.remove('dragging'));
  document.body.classList.remove('dragging-subtab', 'dragging-tab');
  if (ghost) ghost.remove();
  ghost = null;
  syncOverlay();
  renderIfDeferred();
}

// swallowClick stops the click that follows the button coming up at the
// end of a drag, which would otherwise show the tab or window, or focus a
// pane.
function swallowClick() {
  const stop = (e) => {
    e.preventDefault();
    e.stopPropagation();
  };
  window.addEventListener('click', stop, { capture: true, once: true });
  setTimeout(() => window.removeEventListener('click', stop, { capture: true }), 0);
}

function onDragMove(e) {
  if (store.ui.drag) {
    if ((e.buttons & 1) !== 0) {
      clearTimeout(releaseTimer);
      releaseTimer = 0;
      markDrop(e.clientX, e.clientY, e.altKey);
      edgeScroll(e.clientX, e.clientY);
    } else if (!releaseTimer) {
      releaseTimer = setTimeout(() => endDrag(), RELEASE_WAIT);
    }
    return;
  }
  if (!press || e.pointerId !== press.pointerId) return;
  if ((e.buttons & 1) === 0 || !press.el.isConnected) {
    press = null;
    return;
  }
  if (Math.hypot(e.clientX - press.x, e.clientY - press.y) < DRAG_START) return;
  const { el } = press;
  press = null;
  startDrag(el);
  markDrop(e.clientX, e.clientY, e.altKey);
  edgeScroll(e.clientX, e.clientY);
}

// onDragScroll marks the drop again when something scrolls during a drag,
// such as a strip under the pointer, by the wheel or as the drag nears one
// of its ends, since what's under the pointer has moved.
function onDragScroll() {
  const { drag } = store.ui;
  if (drag && drag.at) markDrop(drag.at.x, drag.at.y, drag.add);
}

// onDragUp drops on the button coming up. It handles mouseup as well as
// pointerup: when WebKit still reads the button as held as it handles the
// release, it sends the release on as a pointermove, and only the mouseup
// says the button came up. A mouseup also comes for the other buttons,
// which don't end the drag. After a drag that was cancelled, the release
// isn't a click either.
function onDragUp(e) {
  if (e.button !== 0) return;
  if (!store.ui.drag) {
    press = null;
    if (cancelled) {
      cancelled = false;
      swallowClick();
    }
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
// percentage of the split: layout.MinSize, which Grow Pane and Shrink Pane
// keep to in Go.
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
    // focus is what had the keyboard focus, to give it back on release:
    // the press focuses the divider, but a resize is no reason to take the
    // keys from a terminal, say.
    focus: activeKey(),
  };
  try { el.setPointerCapture(e.pointerId); } catch { /* the element may be gone */ }
  el.classList.add('active');
  el.focus({ preventScroll: true });
  document.body.classList.add(row ? 'resizing-row' : 'resizing-column');
}

// onDividerMouseDown stops a press on a divider from selecting text, or
// from focusing the divider as a mouse press would (onPointerDown does
// that itself). It cancels the mousedown rather than the pointerdown,
// because that would also stop the mouseup, which may be the only sign
// that the button came up.
function onDividerMouseDown(e) {
  if (e.button === 0 && e.target.closest && e.target.closest('.divider')) e.preventDefault();
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
  // The focus goes back unless something else has taken it meanwhile, such
  // as a terminal that Go asked to focus. A terminal gets it back only if
  // it's still the focused pane's shown item, so that typing doesn't reach
  // a terminal outside the focused pane.
  const el = document.activeElement;
  if (el && el.classList && el.classList.contains('divider')) {
    const term = termOfKey(drag.focus);
    const back = !term || term === shownInFocus(store.view);
    if (!back || !focusKey(drag.focus)) el.blur();
  }
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

// evenDivider gives the panes of a divider's split equal sizes, as a
// double-click on the divider, or Return on it, asks (EvenSizes): Go says
// so, with Undo. It goes in the split's lane, after the SetSizes that a
// click's release or an arrow key sent, so that those can't land after it.
// It reports whether el is a divider it knows.
export function evenDivider(el) {
  const dv = dividerFor(el);
  if (!dv) return false;
  actLatest('sizes:' + dv.splitId, 'EvenSizes', dv.splitId);
  return true;
}

function onDividerDoubleClick(e) {
  const el = e.target.closest && e.target.closest('.divider');
  if (el && e.button === 0 && evenDivider(el)) e.preventDefault();
}

// installDrag listens for drags of tabs, sub-tabs, and dividers, and
// double-clicks on dividers.
export function installDrag() {
  window.addEventListener('pointerdown', (e) => { if (e.button === 0) cancelled = false; }, true);
  document.addEventListener('pointerdown', onPressDown);
  window.addEventListener('pointermove', onDragMove);
  window.addEventListener('pointerup', onDragUp);
  window.addEventListener('mouseup', onDragUp);
  window.addEventListener('pointercancel', () => endDrag());
  window.addEventListener('blur', () => endDrag());
  window.addEventListener('keydown', onDragKey, true);
  window.addEventListener('keyup', onDragKey, true);
  document.addEventListener('scroll', onDragScroll, { capture: true, passive: true });
  // WebKit could still start an HTML drag from a tab's or sub-tab's text,
  // but not from the text selected in the field that renames one.
  document.addEventListener('dragstart', (e) => {
    const t = e.target.nodeType === 1 ? e.target : e.target.parentElement;
    if (t && t.closest('.subtab, .tab') && !t.closest('input')) e.preventDefault();
  });
  document.addEventListener('pointerdown', onPointerDown);
  document.addEventListener('mousedown', onDividerMouseDown);
  document.addEventListener('dblclick', onDividerDoubleClick);
  window.addEventListener('pointermove', onPointerMove);
  window.addEventListener('pointerup', onPointerUp);
  // As for tabs and sub-tabs, only the mouseup may say that the button came
  // up.
  window.addEventListener('mouseup', (e) => { if (e.button === 0) onPointerUp(); });
  window.addEventListener('pointercancel', onPointerUp);
}
