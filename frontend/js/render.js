// Drawing the window from the View. Each region (title bar, panes, status
// bar, menu, dialog) is rebuilt from HTML only when its HTML changes, keeping
// focus, the caret, and scroll positions, as the prototype does.

import { act, isDemo, store, syncOverlay } from './state.js';
import {
  badgeOf, contentRect, currentTab, esc, focusKey, ICONS, itemOf, labelOf, px, sameRect,
} from './util.js';
import { placeholderFor } from './placeholders.js';
import { fitMenu, renderMenu } from './menu.js';
import { renderPalette } from './palette.js';
import { followTab, renderEditor } from './editor.js';
import { maybeOpenGuide, renderBanner, renderGuide } from './guide.js';
import { recheck as recheckPreview } from './preview.js';
import * as terminals from './terminals.js';

const SCREENS = [
  { id: 'fit', label: 'Fit to browser' },
  { id: 'monitor', label: 'Monitor · 2,560 pt' },
  { id: 'laptop', label: 'Laptop · 1,512 pt' },
];

const SMALL_SCREEN_TEXT = {
  stack: 'Stack panes',
  tabs: 'One pane, every window as a sub-tab',
  same: 'Keep the layout',
};

const lastHTML = new WeakMap();
let deferred = false;
let lastCurrent = -1;
let lastArea = null;

const $ = (id) => document.getElementById(id);

// setRegion replaces an element's contents if its HTML has changed, and
// keeps focus, the caret, and scroll positions (by data-key and
// data-scroll). It reports whether it replaced anything.
function setRegion(el, html, hooks = {}) {
  if (lastHTML.get(el) === html) return false;
  const active = document.activeElement;
  let key = null;
  let caret = null;
  if (active && el.contains(active)) {
    key = active.dataset ? active.dataset.key || null : null;
    try {
      caret = typeof active.selectionStart === 'number' ? active.selectionStart : null;
    } catch {
      caret = null;
    }
  }
  const scrolls = Array.from(el.querySelectorAll('[data-scroll]'), (n) => [n.dataset.scroll, n.scrollLeft, n.scrollTop]);
  if (hooks.before) hooks.before();
  el.innerHTML = html;
  lastHTML.set(el, html);
  for (const [k, left, top] of scrolls) {
    const n = el.querySelector('[data-scroll="' + CSS.escape(k) + '"]');
    if (n) {
      n.scrollLeft = left;
      n.scrollTop = top;
    }
  }
  if (key) {
    const n = el.querySelector('[data-key="' + CSS.escape(key) + '"]');
    if (n) {
      n.focus({ preventScroll: true });
      if (caret !== null && typeof n.setSelectionRange === 'function') {
        try { n.setSelectionRange(caret, caret); } catch { /* not a text field */ }
      }
    }
  }
  if (hooks.after) hooks.after();
  return true;
}

// render draws everything from the store. While a sub-tab is being dragged
// it draws only the menu, which can close then, and waits until the drag
// ends for the rest, so the dragged element isn't replaced.
export function render() {
  const { view, ui } = store;
  if (!view) return;
  if (ui.drag) {
    deferred = true;
    setRegion($('menu-root'), renderMenu(view, ui), { after: fitMenu });
    syncExpanded(ui);
    return;
  }
  deferred = false;
  dropStale(view, ui);
  applyFrame(view);
  maybeOpenGuide(view);

  setRegion($('titlebar'), renderTitlebar(view));
  const banner = renderBanner(view);
  $('banner').hidden = !banner;
  setRegion($('banner'), banner);

  const panes = $('panes');
  const tab = currentTab(view);
  panes.setAttribute('aria-label', tab ? tab.name + ' tab' : 'Panes');
  let focusedTerm = '';
  setRegion(panes, renderPanes(view), {
    before: () => { focusedTerm = terminals.park(); },
    after: () => {
      terminals.attach(panes);
      if (focusedTerm) terminals.focus(focusedTerm);
    },
  });

  setRegion($('statusbar'), renderStatus(view));
  setRegion($('menu-root'), renderMenu(view, ui), { after: fitMenu });
  setRegion($('overlay-root'), renderOverlay(view, ui));

  if (ui.pendingFocus) {
    const key = ui.pendingFocus;
    ui.pendingFocus = null;
    if (focusKey(key)) {
      const el = document.activeElement;
      if (el && el.tagName === 'INPUT' && el.type === 'text') el.select();
    }
  }
  if (view.current !== lastCurrent) {
    lastCurrent = view.current;
    revealCurrentTab();
  }
  syncExpanded(ui);
  terminals.prune(view);
  recheckPreview();
  syncOverlay();
  measure();
}

// syncExpanded marks the control whose menu is open. It's set on the
// elements rather than in the HTML, so that opening a menu doesn't redraw
// the panes.
function syncExpanded(ui) {
  const m = ui.menu;
  document.querySelectorAll('[aria-expanded="true"][data-action="menu"], [aria-expanded="true"][data-action="notices"]')
    .forEach((el) => el.setAttribute('aria-expanded', 'false'));
  if (!m || !m.opener) return;
  const el = document.querySelector('[data-key="' + CSS.escape(m.opener) + '"]');
  if (el && el.hasAttribute('aria-expanded')) el.setAttribute('aria-expanded', 'true');
}

// dropStale closes a menu or the layout editor whose subject has gone, such
// as a window that closed or a tab deleted elsewhere.
function dropStale(view, ui) {
  // Focus goes back to the control that opened what closes, after this
  // render, if it's still there and focus was inside what closed.
  const restore = (opener, root) => {
    if (opener && root && root.contains(document.activeElement)) ui.pendingFocus = opener;
  };
  const m = ui.menu;
  if (m && ((m.kind === 'window' && !itemOf(view, m.itemId)) || (m.kind === 'notices' && !(view.notices || []).length))) {
    ui.menu = null;
    restore(m.opener, $('menu-root'));
  }
  if (ui.overlay === 'editor' && (!ui.editor || !followTab(ui.editor, view))) {
    ui.overlay = null;
    ui.editor = null;
    const opener = ui.opener;
    ui.opener = null;
    restore(opener, $('overlay-root'));
  }
}

// renderIfDeferred draws what changed during a drag.
export function renderIfDeferred() {
  if (deferred) render();
}

// applyFrame sets up the page for the app or the browser preview.
function applyFrame(view) {
  const demo = isDemo();
  document.body.classList.toggle('is-demo', demo);
  document.body.classList.toggle('is-app', !demo);
  $('demo-bar').hidden = !demo;
  const win = $('app-window');
  const screen = demo ? (view.demo.screen || 'fit') : 'app';
  if (win.dataset.screen !== screen) win.dataset.screen = screen;
  if (demo) {
    setRegion($('screen-picker'), SCREENS.map((s) =>
      '<button type="button" data-action="screen" data-screen="' + s.id + '" data-key="screen-' + s.id + '" aria-pressed="' + (screen === s.id) + '">'
      + esc(s.label) + '</button>').join(''));
  }
}

// measure tells Go where the pane area is, in window content coordinates,
// when it changes.
export function measure() {
  if (!store.view) return;
  const r = contentRect($('panes'));
  const area = { x: Math.round(r.x), y: Math.round(r.y), w: Math.round(r.w), h: Math.round(r.h) };
  if (sameRect(area, lastArea)) return;
  lastArea = area;
  act('SetPaneArea', area);
}

// revealCurrentTab scrolls the tab strip so the current tab is in view.
function revealCurrentTab() {
  const nav = document.querySelector('#titlebar .tabs');
  const tab = nav && nav.querySelector('.tab[aria-pressed="true"]');
  if (!tab) return;
  const n = nav.getBoundingClientRect();
  const t = tab.getBoundingClientRect();
  if (t.left < n.left) nav.scrollLeft -= n.left - t.left + 8;
  else if (t.right > n.right) nav.scrollLeft += t.right - n.right + 8;
}

function renderTitlebar(view) {
  const traffic = isDemo() ? '<div class="traffic" aria-hidden="true"><span></span><span></span><span></span></div>' : '';
  const tabs = view.tabs.map((t, i) =>
    '<button type="button" class="tab" data-action="tab" data-index="' + i + '" data-drop-tab="' + i + '" data-key="tab-' + i + '" aria-pressed="' + (i === view.current) + '">'
    + '<span>' + esc(t.name) + '</span>' + (t.shortcut ? '<kbd>' + esc(t.shortcut) + '</kbd>' : '') + '</button>').join('');
  const modes = [['auto', 'Auto'], ['wide', 'Wide'], ['compact', 'Compact']].map(([id, label]) =>
    '<button type="button" data-action="mode" data-mode="' + id + '" data-key="mode-' + id + '" aria-pressed="' + (view.mode === id) + '">' + label + '</button>').join('');
  return traffic
    + '<nav class="tabs" aria-label="Workspace tabs" data-scroll="tabs">' + tabs + '</nav>'
    + '<button type="button" class="icon-btn" data-action="new-tab" data-key="new-tab" aria-label="New tab">' + ICONS.plus + '</button>'
    + '<div class="titlebar-end">'
    + '<div class="seg" role="group" aria-label="Layout">' + modes + '</div>'
    + '<button type="button" class="jump-btn" data-action="open-palette" data-key="open-palette">' + ICONS.search + '<span>Jump to…</span><kbd>⌃⌥Space</kbd></button>'
    + '<button type="button" class="icon-btn bordered" data-action="edit-tab" data-key="edit-tab" aria-label="Edit this tab\'s layout">' + ICONS.layout + '</button>'
    + '</div>';
}

function renderPanes(view) {
  const d = view.display || {};
  const area = d.area || { x: 0, y: 0, w: 0, h: 0 };
  return (d.panes || []).map((p) => renderPane(view, p, area)).join('')
    + (d.dividers || []).map((dv) => renderDivider(dv, area)).join('');
}

function boxStyle(r, area) {
  return 'left: ' + px(r.x - area.x) + '; top: ' + px(r.y - area.y) + '; width: ' + px(r.w) + '; height: ' + px(r.h);
}

function renderPane(view, p, area) {
  const d = view.display;
  const items = p.items || [];
  const active = p.active && items.includes(p.active) ? p.active : (items[0] || '');
  const item = itemOf(view, active);
  const subtabs = items.map((id) => {
    const it = itemOf(view, id);
    if (!it) return '';
    return '<button type="button" class="subtab" draggable="true" data-action="activate" data-item="' + esc(id) + '" data-pane="' + esc(p.id) + '"'
      + ' data-key="sub-' + esc(id) + '" aria-pressed="' + (id === active) + '" aria-label="' + esc(labelOf(view, id)) + '">'
      + '<span class="mono-tile" aria-hidden="true">' + esc(it.mono) + '</span>'
      + '<span class="subtab-title">' + esc(it.title || it.app) + '</span></button>';
  }).join('');
  const badge = item ? '<span class="badge">' + esc(badgeOf(item)) + '</span>' : '';
  let actions = '';
  if (item) {
    const reason = d.splitsDisabled;
    actions = '<button type="button" class="icon-btn" data-action="split-right" data-pane="' + esc(p.id) + '" data-item="' + esc(active) + '" data-key="split-' + esc(p.id) + '"'
      + ' aria-label="Show this window side by side"' + (reason ? ' aria-disabled="true" title="' + esc(reason) + '"' : '') + '>' + ICONS.splitRight + '</button>'
      + '<button type="button" class="icon-btn" data-action="menu" data-item="' + esc(active) + '" data-pane="' + esc(p.id) + '" data-key="menu-' + esc(p.id) + '" aria-label="Window options" aria-haspopup="menu" aria-expanded="false">' + ICONS.more + '</button>';
  }
  const focused = p.id === d.focus;
  return '<section class="pane" data-pane-id="' + esc(p.id) + '" data-focused="' + focused + '" aria-label="' + esc(item ? labelOf(view, active) : 'Empty pane') + '"'
    + ' style="' + boxStyle(p.rect, area) + '">'
    + '<div class="pane-header">'
    + '<div class="subtabs" role="group" aria-label="Windows in this pane" data-scroll="subtabs-' + esc(p.id) + '">' + subtabs + '</div>'
    + '<button type="button" class="icon-btn add-btn" data-action="add" data-pane="' + esc(p.id) + '" data-key="add-' + esc(p.id) + '" aria-label="Add a window or app to this pane">' + ICONS.plus + '</button>'
    + '<div class="pane-actions">' + badge + actions + '</div>'
    + '</div>'
    + renderBody(view, p, item)
    + '<div class="drop-indicator" hidden></div>'
    + '</section>';
}

function renderBody(view, p, item) {
  if (!item) return '<div class="pane-body">' + contentEmpty(p.id) + '</div>';
  if (item.status && item.status !== 'ready') return '<div class="pane-body">' + statusBody(item) + '</div>';
  if (item.kind === 'builtin') return '<div class="pane-body" data-term="' + esc(item.id) + '"></div>';
  if (isDemo()) return '<div class="pane-body">' + placeholderFor(item) + '</div>';
  // In the app, a web view or the app's own window covers the body.
  return '<div class="pane-body" data-native="' + esc(item.id) + '"></div>';
}

function contentEmpty(paneId) {
  return '<div class="empty"><span>No windows in this pane yet.</span>'
    + '<button type="button" class="btn" data-action="add" data-pane="' + esc(paneId) + '" data-key="empty-add-' + esc(paneId) + '">Add a window or app</button></div>';
}

// statusText explains an item that isn't ready, if Go didn't.
function statusText(item) {
  if (item.statusText) return item.statusText;
  switch (item.status) {
    case 'launching': return 'Opening ' + item.app + '…';
    case 'missing': return item.app + ' isn\'t open.';
    case 'undocked': return 'This window has been moved out of the pane.';
    case 'needs-room': return item.app + '\'s window can\'t shrink to fit this pane.';
    case 'full-screen': return 'In full screen';
    case 'unavailable': return 'This window is on another desktop.';
    default: return '';
  }
}

function statusBody(item) {
  const dock = item.status === 'undocked'
    ? '<button type="button" class="btn" data-action="redock" data-item="' + esc(item.id) + '" data-key="redock-' + esc(item.id) + '">Dock again</button>'
    : '';
  return '<div class="empty pane-status" data-status="' + esc(item.status) + '"><span class="status-text">' + esc(statusText(item)) + '</span>' + dock + '</div>';
}

// renderDivider draws the gap between two children of a split, at least
// 8 points across so that it's easy to grab.
function renderDivider(dv, area) {
  const r = { ...dv.rect };
  const min = 8;
  if (dv.dir === 'row' && r.w < min) { r.x -= (min - r.w) / 2; r.w = min; }
  if (dv.dir !== 'row' && r.h < min) { r.y -= (min - r.h) / 2; r.h = min; }
  const sizes = dv.sizes || [];
  const pos = sizes.slice(0, dv.index + 1).reduce((a, b) => a + b, 0);
  const drag = store.ui.divider;
  const active = drag && drag.splitId === dv.splitId && drag.index === dv.index;
  return '<div class="divider' + (active ? ' active' : '') + '" role="separator" tabindex="0"'
    + ' data-key="divider-' + esc(dv.splitId) + '-' + dv.index + '" data-split="' + esc(dv.splitId) + '" data-index="' + dv.index + '" data-dir="' + (dv.dir === 'row' ? 'row' : 'column') + '"'
    + ' aria-orientation="' + (dv.dir === 'row' ? 'vertical' : 'horizontal') + '" aria-valuemin="0" aria-valuemax="100" aria-valuenow="' + Math.round(pos) + '"'
    + ' aria-label="Resize panes" style="' + boxStyle(r, area) + '"></div>';
}

// layoutText is the status bar's size class and why.
function layoutText(view, tab) {
  const reason = view.sizeReason || '';
  if (/^(wide|compact) layout/i.test(reason)) return reason;
  let s = view.sizeClass === 'compact' ? 'Compact layout' : 'Wide layout';
  if (view.sizeClass === 'compact' && tab && SMALL_SCREEN_TEXT[tab.smallScreens]) s += ' (' + SMALL_SCREEN_TEXT[tab.smallScreens].toLowerCase() + ')';
  return reason ? s + ', ' + reason : s;
}

function renderStatus(view) {
  const tab = currentTab(view);
  if (!tab) return '';
  const count = tab.windows || 0;
  const panes = tab.panes || 0;
  const notices = view.notices || [];
  const notice = notices.length
    ? '<button type="button" class="status-notice" data-action="notices" data-key="notices" aria-haspopup="dialog" aria-expanded="false">' + ICONS.warning
      + '<span class="notice-text">' + esc(notices[0]) + '</span>'
      + (notices.length > 1 ? '<span class="notice-more">' + (notices.length - 1) + ' more</span>' : '') + '</button>'
    : '';
  return '<span class="status-tab"><span class="status-dot" aria-hidden="true"></span>' + esc(tab.name) + '</span>'
    + '<span>' + count + (count === 1 ? ' window' : ' windows') + ' in ' + panes + (panes === 1 ? ' pane' : ' panes') + '</span>'
    + '<span>' + esc(layoutText(view, tab)) + '</span>'
    + notice
    + '<span class="status-keys">⌃⌥1–9 tabs · ⌃⌥Space jump · ⌃⌥⇧ arrows move window</span>';
}

function renderOverlay(view, ui) {
  let inner = '';
  if (ui.overlay === 'palette' || ui.overlay === 'add') inner = renderPalette(view, ui);
  else if (ui.overlay === 'editor') inner = renderEditor(view, ui);
  else if (ui.overlay === 'guide') inner = renderGuide(view);
  if (!inner) return '';
  return '<div class="overlay" data-action="backdrop">' + inner + '</div>';
}
