// Live previews: hovering an inactive sub-tab for 450 ms shows its window,
// and so does selecting or hovering a window in the jump palette. In the
// app, Go shows a native panel fed by ScreenCaptureKit; the browser preview
// draws the prototype's placeholder instead.

import { actLatest, isDemo, store } from './state.js';
import { contentRect, esc, itemOf, windowElement } from './util.js';
import { placeholderFor } from './placeholders.js';

const DELAY = 450;

// The preview's size, in points, as the native panel draws it
// (internal/mac/preview.go), and the space kept between it and the palette
// or the window's edges.
const PANEL_WIDTH = 250;
const PANEL_HEIGHT = 186;
const PANEL_GAP = 8;
const PANEL_MARGIN = 12;

let timer = null;
let pendingItem = null;
let shownItem = null;
let nativeShown = false;

// source is what the preview is for: 'subtab' or 'palette'. paletteKey is
// the palette row's item ID and the query it was chosen with, and
// hoveredRow is the item ID of the palette row under the pointer ('' for a
// row that isn't a window), or null.
let source = null;
let paletteKey = null;
let hoveredRow = null;

// hidePreview hides any preview and cancels one that's about to show.
export function hidePreview() {
  clearTimeout(timer);
  timer = null;
  pendingItem = null;
  shownItem = null;
  source = null;
  paletteKey = null;
  const el = document.getElementById('preview');
  if (el) el.hidden = true;
  if (nativeShown) {
    nativeShown = false;
    actLatest('preview', 'HidePreview');
  }
}

// previewsOn reports whether hovering shows previews: live previews must be
// turned on and, in the app, Bench must have Screen Recording permission.
// Without it the setting has no effect, and Settings explains how to grant
// it.
export function previewsOn(view = store.view) {
  if (!view || !view.settings || !view.settings.livePreviews) return false;
  return Boolean(view.demo) || Boolean(view.permissions && view.permissions.screenRecording);
}

// display shows an item's preview below anchor, a rectangle in content
// coordinates, as the native panel places it.
function display(itemId, anchor) {
  const it = itemOf(store.view, itemId);
  if (!it) return;
  shownItem = itemId;
  if (!isDemo()) {
    nativeShown = true;
    actLatest('preview', 'ShowPreview', itemId, {
      x: Math.round(anchor.x), y: Math.round(anchor.y), w: Math.round(anchor.w), h: Math.round(anchor.h),
    });
    return;
  }
  const el = document.getElementById('preview');
  const win = windowElement();
  el.innerHTML = '<div class="preview-frame"><div class="preview-content">' + placeholderFor(it) + '</div></div>'
    + '<div class="preview-caption"><strong>' + esc(it.title || it.app) + '</strong><span>Live</span></div>';
  el.style.left = Math.max(8, Math.min(anchor.x, win.clientWidth - 260)) + 'px';
  el.style.top = (anchor.y + anchor.h + 6) + 'px';
  el.hidden = false;
}

function show(itemId) {
  timer = null;
  pendingItem = null;
  if (!previewsOn()) return;
  const sub = document.querySelector('.subtab[data-item="' + CSS.escape(itemId) + '"]');
  const it = itemOf(store.view, itemId);
  if (!sub || !it || sub.getAttribute('aria-pressed') === 'true') return;
  display(itemId, contentRect(sub));
}

// paletteTarget returns the item ID of the palette row to preview: the
// hovered row, or else the selected one, if it's a window that Go can show
// a preview of (an App or Web item).
function paletteTarget() {
  const { ui, view } = store;
  if (ui.overlay !== 'palette' || !previewsOn()) return '';
  const row = ui.listItems[ui.palette.sel];
  const id = hoveredRow !== null ? hoveredRow : (row && row.itemId) || '';
  const it = id ? itemOf(view, id) : null;
  return it && (it.kind === 'app' || it.kind === 'web') ? id : '';
}

// besidePalette returns the anchor that puts a preview beside the palette,
// level with a row: on the right if there's room, else on the left, else
// at the window's right edge.
function besidePalette(dialog, row) {
  const win = windowElement();
  let x = dialog.x + dialog.w + PANEL_GAP;
  if (x + PANEL_WIDTH > win.clientWidth - PANEL_MARGIN) x = dialog.x - PANEL_GAP - PANEL_WIDTH;
  if (x < PANEL_MARGIN) x = win.clientWidth - PANEL_MARGIN - PANEL_WIDTH;
  const top = Math.max(PANEL_MARGIN, Math.min(row.y, win.clientHeight - PANEL_MARGIN - PANEL_HEIGHT));
  return { x, y: top - 6, w: 0, h: 0 };
}

function showPaletteRow(itemId, key) {
  timer = null;
  pendingItem = null;
  if (paletteKey !== key || paletteTarget() !== itemId) return;
  const row = document.querySelector('#palette-list .palette-item[data-item="' + CSS.escape(itemId) + '"]');
  const dialog = document.querySelector('#overlay-root .palette');
  if (!row || !dialog) return;
  display(itemId, besidePalette(contentRect(dialog), contentRect(row)));
}

// syncPalette shows the preview of the palette row that's selected or
// hovered, after the delay, and hides it when that row, or the query,
// changes, or the palette closes.
function syncPalette() {
  if (store.ui.overlay !== 'palette') hoveredRow = null;
  const id = paletteTarget();
  if (!id) {
    if (source === 'palette') hidePreview();
    return;
  }
  const key = id + '\n' + store.ui.palette.query;
  if (source === 'palette' && paletteKey === key) return;
  hidePreview();
  source = 'palette';
  paletteKey = key;
  pendingItem = id;
  timer = setTimeout(() => showPaletteRow(id, key), DELAY);
}

// recheck, after each render, hides a sub-tab's preview, or cancels one
// about to show, whose sub-tab the render has removed or shown, or when
// previews have been turned off or a dialog has opened. A removed element
// gets no mouseout, so nothing else would hide it. It also keeps the
// palette's preview in step with its rows.
export function recheck() {
  if (source === 'subtab') {
    const id = shownItem || pendingItem;
    const sub = id ? document.querySelector('.subtab[data-item="' + CSS.escape(id) + '"]') : null;
    if (!sub || sub.getAttribute('aria-pressed') === 'true' || !previewsOn() || store.ui.overlay) hidePreview();
  }
  syncPalette();
}

function onOver(e) {
  const row = e.target.closest && e.target.closest('#palette-list .palette-item');
  if (row) {
    const id = row.dataset.item || '';
    if (id !== hoveredRow) {
      hoveredRow = id;
      syncPalette();
    }
    return;
  }
  const sub = e.target.closest && e.target.closest('.subtab');
  const { ui } = store;
  // The pointer on a sub-tab's × is about to close it: no preview.
  if (sub && e.target.closest('.close-cross')) {
    if (source === 'subtab') hidePreview();
    return;
  }
  if (!sub || ui.drag || ui.divider || ui.overlay || !previewsOn()) return;
  // Nor for the shown sub-tab, or one being renamed.
  if (sub.getAttribute('aria-pressed') === 'true' || sub.classList.contains('renaming')) return;
  const id = sub.dataset.item;
  if (source === 'subtab' && (id === pendingItem || id === shownItem)) return;
  hidePreview();
  source = 'subtab';
  pendingItem = id;
  timer = setTimeout(() => show(id), DELAY);
}

function onOut(e) {
  const to = e.relatedTarget;
  const row = e.target.closest && e.target.closest('#palette-list .palette-item');
  if (row) {
    if (to && to.closest && to.closest('#palette-list .palette-item')) return;
    if (hoveredRow !== null) {
      hoveredRow = null;
      syncPalette();
    }
    return;
  }
  const sub = e.target.closest && e.target.closest('.subtab');
  if (!sub) return;
  if (to && sub.contains(to)) return;
  if (source === 'subtab') hidePreview();
}

// installPreviews listens for hovering over sub-tabs and palette rows.
export function installPreviews() {
  document.addEventListener('mouseover', onOver);
  document.addEventListener('mouseout', onOut);
  document.addEventListener('mousedown', hidePreview, true);
  window.addEventListener('blur', hidePreview);
}
