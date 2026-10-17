// Live previews: hovering an inactive sub-tab for 450 ms shows its window.
// In the app, Go shows a native panel fed by ScreenCaptureKit; the browser
// preview draws the prototype's placeholder instead.

import { act, isDemo, store } from './state.js';
import { contentOrigin, contentRect, esc, itemOf, windowElement } from './util.js';
import { placeholderFor } from './placeholders.js';

const DELAY = 450;

let timer = null;
let pendingItem = null;
let shownItem = null;
let nativeShown = false;

// hidePreview hides any preview and cancels one that's about to show.
export function hidePreview() {
  clearTimeout(timer);
  timer = null;
  pendingItem = null;
  shownItem = null;
  const el = document.getElementById('preview');
  if (el) el.hidden = true;
  if (nativeShown) {
    nativeShown = false;
    act('HidePreview');
  }
}

// previewsOn reports whether hovering shows previews: live previews must be
// turned on and, in the app, Bench must have Screen Recording permission.
// Without it the setting has no effect, and the layout editor explains how
// to grant it.
export function previewsOn(view = store.view) {
  if (!view || !view.settings || !view.settings.livePreviews) return false;
  return Boolean(view.demo) || Boolean(view.permissions && view.permissions.screenRecording);
}

function show(itemId) {
  timer = null;
  pendingItem = null;
  if (!previewsOn()) return;
  const sub = document.querySelector('.subtab[data-item="' + CSS.escape(itemId) + '"]');
  const it = itemOf(store.view, itemId);
  if (!sub || !it || sub.getAttribute('aria-pressed') === 'true') return;
  shownItem = itemId;
  if (!isDemo()) {
    const r = contentRect(sub);
    nativeShown = true;
    act('ShowPreview', itemId, { x: Math.round(r.x), y: Math.round(r.y), w: Math.round(r.w), h: Math.round(r.h) });
    return;
  }
  const el = document.getElementById('preview');
  const win = windowElement();
  const o = contentOrigin();
  const r = sub.getBoundingClientRect();
  el.innerHTML = '<div class="preview-frame"><div class="preview-content">' + placeholderFor(it) + '</div></div>'
    + '<div class="preview-caption"><strong>' + esc(it.title || it.app) + '</strong><span>Live</span></div>';
  el.style.left = Math.max(8, Math.min(r.left - o.x, win.clientWidth - 260)) + 'px';
  el.style.top = (r.bottom - o.y + 6) + 'px';
  el.hidden = false;
}

// recheck hides a preview, or cancels one about to show, whose sub-tab a
// redraw has removed or shown, or when previews have been turned off. A
// removed element gets no mouseout, so nothing else would hide it.
export function recheck() {
  const id = shownItem || pendingItem;
  if (!id) return;
  const sub = document.querySelector('.subtab[data-item="' + CSS.escape(id) + '"]');
  if (!sub || sub.getAttribute('aria-pressed') === 'true' || !previewsOn()) hidePreview();
}

function onOver(e) {
  const sub = e.target.closest && e.target.closest('.subtab');
  const { ui } = store;
  if (!sub || ui.drag || ui.divider || ui.overlay || !previewsOn()) return;
  if (sub.getAttribute('aria-pressed') === 'true') return;
  const id = sub.dataset.item;
  if (id === pendingItem || id === shownItem) return;
  hidePreview();
  pendingItem = id;
  timer = setTimeout(() => show(id), DELAY);
}

function onOut(e) {
  const sub = e.target.closest && e.target.closest('.subtab');
  if (!sub) return;
  const to = e.relatedTarget;
  if (to && sub.contains(to)) return;
  hidePreview();
}

// installPreviews listens for hovering over sub-tabs.
export function installPreviews() {
  document.addEventListener('mouseover', onOver);
  document.addEventListener('mouseout', onOut);
  document.addEventListener('mousedown', hidePreview, true);
  window.addEventListener('blur', hidePreview);
}
