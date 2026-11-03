// The strips of tabs and sub-tabs that don't fit: a vertical wheel scrolls
// them sideways, the current tab and each pane's shown sub-tab stay in
// view, an edge with more to show fades, and a drag near either end
// scrolls them (edgeScroll).

import { hidePreview } from './preview.js';

// STRIPS are the title bar's tabs and the panes' sub-tabs, including the
// "tabs" view's pane's, which are in the title bar.
const STRIPS = '#titlebar .tabs, #titlebar .subtabs, #panes .subtabs';

// LINE is how far, in points, a strip scrolls for each line of a wheel
// that counts in lines rather than points.
const LINE = 40;

// FADE is the width, in points, of the fade at an edge with more to show
// (--strip-fade in styles.css). A tab or sub-tab that's scrolled into view
// stays clear of it.
const FADE = 24;

// EDGE is how near, in points, a drag must come to either end of a strip
// to scroll it, and EDGE_SPEED the most it scrolls in a frame, at the very
// end.
const EDGE = 24;
const EDGE_SPEED = 14;

// browsed holds the strips the user has scrolled by hand, or moved the
// keyboard along, by their data-scroll, with what they showed then
// (shownOf). Redraws leave such a strip where it is until it shows another
// tab or sub-tab, or gets one more, or the window changes size.
const browsed = new Map();

// synced holds what each strip showed when last synced (shownOf), by its
// data-scroll.
const synced = new Map();

// edge is the strip that a drag is scrolling by its end, with the
// pointer's x, or null; edgeFrame is its animation frame, or 0.
let edge = null;
let edgeFrame = 0;

// shownOf names what a strip shows: its current tab or shown sub-tab, and
// how many it has.
function shownOf(strip) {
  const shown = strip.querySelector('[aria-pressed="true"]');
  return (shown ? shown.dataset.key : '') + ' ' + strip.children.length;
}

// keyboardAway reports whether the keyboard is in a strip, on another tab
// or sub-tab than the one it shows.
function keyboardAway(strip) {
  const el = document.activeElement;
  return Boolean(el) && strip.contains(el) && !el.closest('[aria-pressed="true"]');
}

// overflows reports whether a strip has more than fits.
function overflows(strip) {
  return strip.scrollWidth - strip.clientWidth >= 1;
}

// reveal scrolls a strip so that el is in view, clear of the fades, or as
// much of it as fits, from its start.
function reveal(strip, el) {
  const n = strip.getBoundingClientRect();
  const t = el.getBoundingClientRect();
  const pad = Math.min(FADE, Math.max(0, (n.width - t.width) / 2));
  if (t.left < n.left + pad) strip.scrollLeft -= n.left + pad - t.left;
  else if (t.right > n.right - pad) strip.scrollLeft += Math.min(t.right - n.right + pad, t.left - n.left - pad);
}

// settleFull measures a strip that's full again with data-more taken off,
// which gives the shown sub-tab back its slot for its × (styles.css), as a
// fresh redraw measures it. data-more comes back, with the strip's scroll
// position, only if the strip still doesn't fit. Without the slot, a strip
// beside a sibling that shrinks with it, as the "tabs" view's host does,
// is a few points narrower, so measured as it is, it could stay full at a
// width where a redraw fits it. Scrolling doesn't change whether a strip
// fits, so only syncStrips asks.
function settleFull(strip) {
  const more = strip.dataset.more;
  if (!more) return;
  const left = strip.scrollLeft;
  delete strip.dataset.more;
  if (!overflows(strip)) return;
  strip.dataset.more = more;
  strip.scrollLeft = left;
}

// syncFades marks the ends of a strip that have more to show
// (data-more="start end"), which styles.css fades.
function syncFades(strip) {
  const more = [];
  if (strip.scrollLeft >= 1) more.push('start');
  if (strip.scrollLeft + strip.clientWidth <= strip.scrollWidth - 1) more.push('end');
  const value = more.join(' ');
  if ((strip.dataset.more || '') === value) return;
  if (value) strip.dataset.more = value;
  else delete strip.dataset.more;
}

// syncStrips scrolls each strip to its current tab or shown sub-tab, and
// fades the ends with more to show. It runs after each render, and as the
// title bar or the panes change size (resized), which brings the shown
// ones into view even in strips the user has scrolled by hand. A strip
// that has the keyboard on another tab or sub-tab, which Tab has scrolled
// into view, stays where it is, as one scrolled by hand does, until it
// shows another.
export function syncStrips(resized = false) {
  if (resized) browsed.clear();
  const seen = new Set();
  for (const strip of document.querySelectorAll(STRIPS)) {
    const key = strip.dataset.scroll;
    const now = shownOf(strip);
    seen.add(key);
    if (browsed.has(key) && browsed.get(key) !== now) browsed.delete(key);
    if (!resized && synced.get(key) === now && keyboardAway(strip)) browsed.set(key, now);
    synced.set(key, now);
    settleFull(strip);
    const shown = strip.querySelector('[aria-pressed="true"]');
    if (shown && !browsed.has(key)) reveal(strip, shown);
    syncFades(strip);
  }
  for (const map of [browsed, synced]) {
    for (const key of map.keys()) {
      if (!seen.has(key)) map.delete(key);
    }
  }
}

// onWheel scrolls a strip that doesn't fit sideways with a mainly vertical
// wheel, as tab bars elsewhere do, converting lines to points; a sideways
// swipe scrolls it as it would anyway. Scrolling moves the sub-tab under a
// resting pointer, so it hides any live preview.
function onWheel(e) {
  const strip = e.target instanceof Element ? e.target.closest('.tabs, .subtabs') : null;
  if (!strip) return;
  hidePreview();
  if (e.ctrlKey || !overflows(strip)) return;
  browsed.set(strip.dataset.scroll, shownOf(strip));
  if (Math.abs(e.deltaY) <= Math.abs(e.deltaX)) return;
  let unit = 1;
  if (e.deltaMode === WheelEvent.DOM_DELTA_LINE) unit = LINE;
  else if (e.deltaMode === WheelEvent.DOM_DELTA_PAGE) unit = strip.clientWidth;
  e.preventDefault();
  strip.scrollLeft += e.deltaY * unit;
}

// onPress notes a press on a strip's own scroll bar, which scrolls it by
// hand.
function onPress(e) {
  const strip = e.target;
  if (strip instanceof Element && strip.matches(STRIPS) && e.offsetY >= strip.clientHeight) {
    browsed.set(strip.dataset.scroll, shownOf(strip));
  }
}

function onScroll(e) {
  if (e.target instanceof Element && e.target.matches(STRIPS)) syncFades(e.target);
}

// edgeSpeed returns how far to scroll a strip in a frame with the pointer
// at x: back near its start, on near its end, faster nearer, or 0 if it's
// near neither, or can't scroll that way.
function edgeSpeed(strip, x) {
  const r = strip.getBoundingClientRect();
  const end = strip.scrollWidth - strip.clientWidth;
  if (end < 1) return 0;
  const fromStart = x - r.left;
  const fromEnd = r.right - x;
  if (fromStart < EDGE && strip.scrollLeft > 0) return -Math.ceil(EDGE_SPEED * (1 - Math.max(fromStart, 0) / EDGE));
  if (fromEnd < EDGE && strip.scrollLeft < end - 0.5) return Math.ceil(EDGE_SPEED * (1 - Math.max(fromEnd, 0) / EDGE));
  return 0;
}

// stripAt returns the strip at (x, y), in client coordinates, or null.
function stripAt(x, y) {
  for (const strip of document.querySelectorAll(STRIPS)) {
    const r = strip.getBoundingClientRect();
    if (x >= r.left && x <= r.right && y >= r.top && y <= r.bottom) return strip;
  }
  return null;
}

function stepEdge() {
  edgeFrame = 0;
  const speed = edge && edge.strip.isConnected ? edgeSpeed(edge.strip, edge.x) : 0;
  if (!speed) {
    edge = null;
    return;
  }
  edge.strip.scrollLeft += speed;
  edgeFrame = requestAnimationFrame(stepEdge);
}

// edgeScroll scrolls the strip at (x, y), in client coordinates, a little
// each frame while a drag holds the pointer near either of its ends, so
// that tabs and sub-tabs out of view can take a drop. A drag calls it as
// the pointer moves, and stopEdgeScroll as it ends. The strip's scroll
// events say when what's under the pointer has moved.
export function edgeScroll(x, y) {
  const strip = stripAt(x, y);
  edge = strip && edgeSpeed(strip, x) ? { strip, x } : null;
  if (edge && !edgeFrame) edgeFrame = requestAnimationFrame(stepEdge);
}

// stopEdgeScroll stops edgeScroll.
export function stopEdgeScroll() {
  edge = null;
  cancelAnimationFrame(edgeFrame);
  edgeFrame = 0;
}

// installStrips listens for wheels over the strips, which it may turn
// sideways, so not passively, and for the strips scrolling. The title bar
// and the panes stay as their contents are drawn again.
export function installStrips() {
  for (const id of ['titlebar', 'panes']) {
    const el = document.getElementById(id);
    el.addEventListener('wheel', onWheel, { passive: false });
    el.addEventListener('pointerdown', onPress, true);
  }
  document.addEventListener('scroll', onScroll, { capture: true, passive: true });
}
