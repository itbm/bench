// Drawing the window from the View. Each region (title bar, panes, status
// bar, menu, dialog) is rebuilt from HTML only when its HTML changes, keeping
// focus, the caret, and scroll positions, as the prototype does.

import { actLatest, canRestore, isDemo, store, syncOverlay } from './state.js';
import {
  activeOf, alsoInOf, attentionOf, badgeOf, contentRect, currentTab, esc, focusKey, hostParts, ICONS, inCurrentTab, isPage,
  itemOf, isTabKey, joinNames, keysOf, labelOf, markerOf, notReady, px, RELEASE_WAIT, sameRect, sentence, shownInFocus,
  STATUS_SHORT, statusMark, statusText, tabKey, withKeys,
} from './util.js';
import { placeholderFor } from './placeholders.js';
import { attentionKey, fitMenu, renderMenu } from './menu.js';
import {
  fillPalette, isList, renderPalette, renderRows, syncSelection,
} from './palette.js';
import { followTab, renderEditor } from './editor.js';
import { fillSettings, renderSettings } from './settings.js';
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

// holding is set while the primary button is down in the UI, and
// composing while an input method is composing text: replacing the
// element under the pointer would lose the click or stop a drag from
// starting, and replacing the field being composed in would break the
// composition. Renders wait until they end.
let holding = false;
let composing = false;

// focusSeen is the tab, focused pane, and that pane's shown item as last
// drawn, to tell when the focus moves.
let focusSeen = null;

// drawnTabs are the tabs' names, in order, as last drawn in the title bar.
let drawnTabs = [];

// TERM_FOCUS_WAIT is how long, in milliseconds, a terminal waits to be
// drawn in the focused pane before giving up on the focus.
const TERM_FOCUS_WAIT = 2000;

const $ = (id) => document.getElementById(id);

// setRegion replaces an element's contents if its HTML has changed, and
// keeps focus, the selection, and scroll positions (by data-key and
// data-scroll). It reports whether it replaced anything. Hooks: before
// runs before the replacement; fill, straight after it, before focus is
// put back; lost, if focus was inside and its element has gone; after, at
// the end.
function setRegion(el, html, hooks = {}) {
  if (lastHTML.get(el) === html) return false;
  const active = document.activeElement;
  const inside = Boolean(active) && active !== el && el.contains(active);
  let key = null;
  let selection = null;
  if (inside) {
    key = active.dataset ? active.dataset.key || null : null;
    try {
      if (typeof active.selectionStart === 'number') {
        selection = [active.selectionStart, active.selectionEnd, active.selectionDirection || 'none'];
      }
    } catch {
      selection = null;
    }
  }
  const own = [el.scrollLeft, el.scrollTop];
  const scrolls = Array.from(el.querySelectorAll('[data-scroll]'), (n) => [n.dataset.scroll, n.scrollLeft, n.scrollTop]);
  if (hooks.before) hooks.before();
  el.innerHTML = html;
  lastHTML.set(el, html);
  [el.scrollLeft, el.scrollTop] = own;
  for (const [k, left, top] of scrolls) {
    const n = el.querySelector('[data-scroll="' + CSS.escape(k) + '"]');
    if (n) {
      n.scrollLeft = left;
      n.scrollTop = top;
    }
  }
  if (hooks.fill) hooks.fill();
  let kept = false;
  if (key) {
    const n = el.querySelector('[data-key="' + CSS.escape(key) + '"]');
    if (n) {
      n.focus({ preventScroll: true });
      kept = document.activeElement === n;
      if (selection && typeof n.setSelectionRange === 'function') {
        try { n.setSelectionRange(...selection); } catch { /* not a text field */ }
      }
    }
  }
  if (inside && !kept && hooks.lost) hooks.lost();
  if (hooks.after) hooks.after();
  return true;
}

// emptyFocus returns where the focus is among the buttons of an empty pane,
// such as Remove from this pane, which can go when the panes are drawn
// again: the pane's ID, the place in its list of the app whose button it
// is (or -1), and the button's place among the pane's buttons; or null.
function emptyFocus() {
  const el = document.activeElement;
  const box = el && el.closest ? el.closest('#panes .empty') : null;
  const pane = box && box.closest('.pane');
  if (!pane) return null;
  const app = el.closest('.remembered');
  return {
    pane: pane.dataset.paneId,
    app: app ? Array.from(box.querySelectorAll('.remembered')).indexOf(app) : -1,
    index: Array.from(box.querySelectorAll('button')).indexOf(el),
  };
}

// keepEmptyFocus puts the focus that was on a button of an empty pane, if
// the button has gone, rather than leave it to the page. If the pane still
// lists apps, it goes to the first button of the app that took the place
// of the one it was on, or of the last; if it was on another button, to the
// button in its place, or the last. Otherwise it goes to the focused
// pane's first button, or its Add button.
function keepEmptyFocus(panes, was) {
  const el = document.activeElement;
  if (el && el !== document.body) return;
  let next = null;
  const same = Array.from(panes.querySelectorAll('.pane')).find((n) => n.dataset.paneId === was.pane);
  if (same) {
    const apps = same.querySelectorAll('.empty .remembered');
    const buttons = same.querySelectorAll('.empty button');
    if (was.app >= 0 && apps.length) next = apps[Math.min(was.app, apps.length - 1)].querySelector('button');
    else if (buttons.length) next = buttons[Math.min(Math.max(was.index, 0), buttons.length - 1)];
  }
  if (!next) {
    const p = panes.querySelector('.pane[data-focused="true"]') || panes.querySelector('.pane');
    next = p && (p.querySelector('.empty button') || p.querySelector('[data-action="add"]'));
  }
  if (!next) return;
  next.focus({ preventScroll: true });
  // Only the pane's list scrolls to show it, not the page.
  if (next.closest('.empty') && typeof next.scrollIntoView === 'function') next.scrollIntoView({ block: 'nearest', inline: 'nearest' });
}

// focusTerminal gives a terminal the keyboard focus, as Go's
// focus-terminal command asks when its pane is focused (ui.termFocus). If
// the terminal isn't drawn as the focused pane's shown item yet, because
// the View that shows it hasn't arrived, it gets the focus when it is. A
// menu or dialog keeps the focus while it's open: the terminal gets it
// when that closes, instead of the control that opened it.
export function focusTerminal(id) {
  if (!id) return;
  store.ui.termFocus = { id, until: Date.now() + TERM_FOCUS_WAIT };
  applyTermFocus();
}

function applyTermFocus() {
  const { view, ui } = store;
  const want = ui.termFocus;
  if (!want || !view) return;
  // The wait for the terminal to be drawn starts again once the menu or
  // dialog has closed.
  if (ui.menu || ui.overlay) {
    want.until = 0;
    return;
  }
  if (!want.until) want.until = Date.now() + TERM_FOCUS_WAIT;
  if (Date.now() > want.until) {
    ui.termFocus = null;
    return;
  }
  if (holding || composing || ui.drag || shownInFocus(view) !== want.id) return;
  if (terminals.focus(want.id)) ui.termFocus = null;
}

// render draws everything from the store. While a sub-tab is being dragged
// it draws only the menu, which can close then, and waits until the drag
// ends for the rest, so the dragged element isn't replaced. While the
// pointer is held down or text is being composed, it waits too.
export function render() {
  const { view, ui } = store;
  if (!view) return;
  if (ui.drag) {
    deferred = true;
    setRegion($('menu-root'), renderMenu(view, ui), { after: fitMenu });
    syncExpanded(ui);
    return;
  }
  if (holding || composing) {
    deferred = true;
    return;
  }
  deferred = false;
  dropStale(view, ui);
  applyFrame(view);
  maybeOpenGuide(view);

  // Focus on a tab's button follows its tab (followTabKey).
  const focusedTab = focusedTabKey();
  if (setRegion($('titlebar'), renderTitlebar(view)) && focusedTab) focusKey(followTabKey(view, focusedTab));
  drawnTabs = view.tabs.map((t) => t.name);
  const banner = renderBanner(view);
  $('banner').hidden = !banner;
  setRegion($('banner'), banner);

  const panes = $('panes');
  const tab = currentTab(view);
  panes.setAttribute('aria-label', tab ? tab.name + ' tab' : 'Panes');
  // A terminal that has the focus keeps it across the redraw, unless the
  // focus has moved to another pane or item: then it lets go, so typing
  // doesn't reach a terminal that isn't in the focused pane. Go asks for
  // the focus of a terminal that's newly in the focused pane
  // (focusTerminal).
  const shown = shownInFocus(view);
  const sig = view.current + ' ' + ((view.display && view.display.focus) || '') + ' ' + shown;
  const moved = focusSeen !== null && sig !== focusSeen;
  focusSeen = sig;
  let focusedTerm = '';
  const inEmpty = emptyFocus();
  setRegion(panes, renderPanes(view), {
    before: () => { focusedTerm = terminals.park(); },
    after: () => {
      terminals.attach(panes);
      terminals.unpark(focusedTerm && (!moved || focusedTerm === shown) ? focusedTerm : '');
      if (inEmpty) keepEmptyFocus(panes, inEmpty);
    },
  });
  if (moved) {
    const el = document.activeElement;
    const host = el && el.closest ? el.closest('.term-host[data-term-host]') : null;
    if (host && host.dataset.termHost !== shown) el.blur();
  }
  applyTermFocus();

  setRegion($('statusbar'), renderStatus(view));
  setRegion($('menu-root'), renderMenu(view, ui), { after: fitMenu });
  const overlay = $('overlay-root');
  setRegion(overlay, renderOverlay(view, ui), {
    fill: () => {
      fillPalette(ui);
      fillSettings(ui);
    },
    // Focus that was in a dialog stays in it, as when the first-run guide
    // replaces its Turn on Accessibility button once permission is given.
    lost: () => focusFirst(overlay.querySelector('.dialog')),
  });
  if (isList(ui)) {
    const rows = $('palette-list');
    if (rows) setRegion(rows, renderRows(view, ui));
    syncSelection(ui);
  } else {
    ui.listItems = [];
  }

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

// focusedTabKey returns the data-key of the tab button in the title bar
// that has the focus, or null.
function focusedTabKey() {
  const el = document.activeElement;
  return el && el.matches && el.matches('#titlebar .tab[data-index]') ? el.dataset.key : null;
}

// followTabKey returns the data-key of the button of the tab whose button
// had key as the title bar was last drawn (drawnTabs): tab buttons'
// data-keys are their places, which change as tabs are added, deleted, or
// moved. If the tab has gone, it returns the current tab's button's, as
// when Delete tab's View comes after its dialog has closed. A renamed tab
// keeps its place. Any other key is returned as it is.
function followTabKey(view, key) {
  if (!isTabKey(key)) return key;
  const name = drawnTabs[Number(key.slice('tab-'.length))];
  if (name === undefined) return key;
  const i = view.tabs.findIndex((t) => t.name === name);
  if (i >= 0) return tabKey(i);
  return view.tabs.length < drawnTabs.length ? tabKey(view.current) : key;
}

// syncExpanded marks the control whose menu is open. It's set on the
// elements rather than in the HTML, so that opening a menu doesn't redraw
// the panes.
function syncExpanded(ui) {
  const m = ui.menu;
  document.querySelectorAll('[aria-expanded="true"][data-action="menu"], [aria-expanded="true"][data-action="notices"], [aria-expanded="true"][data-action="attention"]')
    .forEach((el) => el.setAttribute('aria-expanded', 'false'));
  if (!m || !m.opener) return;
  const el = document.querySelector('[data-key="' + CSS.escape(m.opener) + '"]');
  if (el && el.hasAttribute('aria-expanded')) el.setAttribute('aria-expanded', 'true');
}

// staleMenu reports whether an open menu's subject has gone: a window
// menu's window has closed or left the pane it was opened in, or Go has
// switched tabs; a tab's menu's tab has been deleted (it follows its tab
// when others are added or deleted, as Tab properties does); an empty
// pane's menu's pane has closed or isn't empty any more, or Go has
// switched tabs; there are no notices any more; or the windows that need
// attention have changed, or Go has switched tabs, so that the list's
// entries don't move under the pointer.
function staleMenu(view, m) {
  if (m.kind === 'notices') return !(view.notices || []).length;
  if (m.kind === 'attention') return m.tab !== view.current || attentionKey(view) !== m.key;
  if (m.kind === 'tab') return !followTab(m, view);
  const panes = (view.display && view.display.panes) || [];
  if (m.kind === 'pane') {
    const pane = panes.find((p) => p.id === m.paneId);
    return m.tab !== view.current || !pane || (pane.items || []).length > 0;
  }
  if (!itemOf(view, m.itemId) || m.tab !== view.current) return true;
  const pane = panes.find((p) => p.id === m.shownIn);
  return !pane || !(pane.items || []).includes(m.itemId);
}

// inTab reports whether an item is in tab ti.
function inTab(view, itemId, ti) {
  const it = itemOf(view, itemId);
  return Boolean(it) && (it.tab === ti || (it.tabs || []).includes(ti));
}

// dropStale closes a menu, the add dialog, or Tab properties whose subject
// has gone, such as a window that closed, a tab deleted elsewhere,
// a tab that Go has switched away from (with a global shortcut, the menu
// bar item, or focus following), or the web page that the add dialog waits
// to put a web app in the place of, which the web app may have taken.
function dropStale(view, ui) {
  // Focus goes back to the control that opened what closes, after this
  // render, if it's still there and focus was inside what closed (and
  // canRestore agrees).
  const restore = (opener, root) => {
    if (opener && root && root.contains(document.activeElement) && canRestore(opener)) ui.pendingFocus = opener;
  };
  // A tab's button that opened what's open follows its tab.
  ui.opener = followTabKey(view, ui.opener);
  const m = ui.menu;
  if (m) m.opener = followTabKey(view, m.opener);
  if (m && staleMenu(view, m)) {
    ui.menu = null;
    ui.paneLater = null;
    restore(m.opener, $('menu-root'));
  }
  if (ui.overlay === 'add' && (!ui.add || ui.add.tab !== view.current || !inCurrentTab(view, ui.add.paneId)
    || (ui.add.replace && !inTab(view, ui.add.replace.itemId, ui.add.tab)))) {
    ui.overlay = null;
    ui.add = null;
    ui.listItems = [];
    ui.paneLater = null;
    const opener = ui.opener;
    ui.opener = null;
    restore(opener, $('overlay-root'));
  }
  if (ui.overlay === 'editor' && (!ui.editor || !followTab(ui.editor, view))) {
    ui.overlay = null;
    ui.editor = null;
    const opener = ui.opener;
    ui.opener = null;
    restore(opener, $('overlay-root'));
  }
}

// renderIfDeferred draws what changed during a drag, while the pointer
// was held down, or while text was being composed.
export function renderIfDeferred() {
  if (deferred) render();
}

// release ends holding or composing, and draws what changed meanwhile once
// the event that ended it, and any click it makes, has been handled. A
// terminal that was asked to take the focus meanwhile takes it then.
function release() {
  setTimeout(() => {
    renderIfDeferred();
    applyTermFocus();
  }, 0);
}

// movesWindow reports whether pressing on an element moves the window, as
// Wails decides it: the title bar's empty space, in the app. Wails then
// moves the window natively, and the page doesn't see the button come up.
function movesWindow(el) {
  return getComputedStyle(el).getPropertyValue('--wails-draggable').trim() === 'drag';
}

// installRenderDeferral watches for the pointer being held down and for
// text being composed, which put renders off. Dragging a divider doesn't:
// the panes follow it as Go resizes them. Nor does moving the window.
export function installRenderDeferral() {
  // upTimer ends holding if no release follows a move that reports no
  // button held (RELEASE_WAIT): WebKit sends the last move of a quick
  // release that way, and drawing then would replace the element pressed
  // before the click.
  let upTimer = 0;
  document.addEventListener('pointerdown', (e) => {
    const el = e.target;
    if (e.button !== 0 || !(el instanceof Element) || el.closest('.divider') || movesWindow(el)) return;
    clearTimeout(upTimer);
    upTimer = 0;
    holding = true;
  }, true);
  const up = () => {
    clearTimeout(upTimer);
    upTimer = 0;
    if (!holding) return;
    holding = false;
    release();
  };
  window.addEventListener('pointerup', up, true);
  window.addEventListener('mouseup', up, true);
  window.addEventListener('pointercancel', up, true);
  window.addEventListener('pointermove', (e) => {
    if (!holding) return;
    if ((e.buttons & 1) !== 0) {
      clearTimeout(upTimer);
      upTimer = 0;
    } else if (!upTimer) {
      upTimer = setTimeout(up, RELEASE_WAIT);
    }
  }, true);
  window.addEventListener('blur', up);
  // A sub-tab drag puts renders off by itself (ui.drag). An HTML drag (of
  // selected text, say) keeps the button coming up from the page. A menu
  // opened by Control-clicking opens straight away.
  document.addEventListener('dragstart', up, true);
  document.addEventListener('contextmenu', up, true);
  document.addEventListener('compositionstart', () => { composing = true; }, true);
  const done = () => {
    if (!composing) return;
    composing = false;
    release();
  };
  document.addEventListener('compositionend', done, true);
  document.addEventListener('focusout', done, true);
}

// focusFirst focuses the first control in a dialog.
function focusFirst(dialog) {
  if (!dialog) return;
  const el = dialog.querySelector('button:not([tabindex="-1"]):not([disabled]), input:not([disabled]), select, [href]');
  if (el) el.focus({ preventScroll: true });
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
    setRegion($('demo-hint'), esc(demoHint(view)));
  }
}

// demoHint is the demo bar's reminder of what to try. In the browser
// preview, Control+K also opens the palette.
function demoHint(view) {
  const keys = keysOf(view);
  const hints = ['Drag a sub-tab onto another pane, a pane\'s edge, or a tab'];
  hints.push(keys.jump ? keys.jump + ' or ⌃K jump' : '⌃K jump');
  if (keys.moveWindow) hints.push(keys.moveWindow + ' arrows move the window');
  return hints.join(' · ');
}

// measure tells Go where the pane area is, in window content coordinates,
// when it changes.
export function measure() {
  if (!store.view) return;
  const r = contentRect($('panes'));
  const area = { x: Math.round(r.x), y: Math.round(r.y), w: Math.round(r.w), h: Math.round(r.h) };
  if (sameRect(area, lastArea)) return;
  lastArea = area;
  actLatest('area', 'SetPaneArea', area);
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

// renderTitlebar draws the tabs, with + right after them, and the layout
// control, Jump to…, and Settings at the right. The space between is the
// title bar's own, which moves the window in the app. Each tab's tooltip
// has its whole name, and the layout control's says why the layout is in
// use.
function renderTitlebar(view) {
  const jump = keysOf(view).jump;
  const traffic = isDemo() ? '<div class="traffic" aria-hidden="true"><span></span><span></span><span></span></div>' : '';
  const tabs = view.tabs.map((t, i) =>
    '<button type="button" class="tab" data-action="tab" data-index="' + i + '" data-drop-tab="' + i + '" data-key="' + tabKey(i) + '" aria-pressed="' + (i === view.current) + '"'
    + ' title="' + esc(withKeys(t.name, t.shortcut)) + '">'
    + '<span>' + esc(t.name) + '</span>' + (t.shortcut ? '<kbd>' + esc(t.shortcut) + '</kbd>' : '') + '</button>').join('');
  const modes = [['auto', 'Auto'], ['wide', 'Wide'], ['compact', 'Compact']].map(([id, label]) =>
    '<button type="button" data-action="mode" data-mode="' + id + '" data-key="mode-' + id + '" aria-pressed="' + (view.mode === id) + '">' + label + '</button>').join('');
  return traffic
    + '<nav class="tabs" aria-label="Workspace tabs" data-scroll="tabs">' + tabs + '</nav>'
    + '<button type="button" class="icon-btn new-tab" data-action="new-tab" data-key="new-tab" aria-label="New tab" title="New tab">' + ICONS.plus + '</button>'
    + '<div class="titlebar-end">'
    + '<div class="seg" role="group" aria-label="Layout" title="' + esc(layoutText(view, currentTab(view))) + '">' + modes + '</div>'
    + '<button type="button" class="jump-btn" data-action="open-palette" data-key="open-palette">' + ICONS.search + '<span>Jump to…</span>' + (jump ? '<kbd>' + esc(jump) + '</kbd>' : '') + '</button>'
    + '<button type="button" class="icon-btn bordered" data-action="open-settings" data-key="open-settings" aria-label="Settings" title="Settings">' + ICONS.gear + '</button>'
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
  const active = activeOf(p);
  const item = itemOf(view, active);
  const subtabs = items.map((id) => {
    const it = itemOf(view, id);
    if (!it) return '';
    // An item that isn't ready says so on its sub-tab, because its body
    // may be covered: by its own window when it needs more room, for one.
    // A marker on its monogram shows what kind of status it is, by its
    // shape as well as its colour (markerOf).
    const short = notReady(it) ? STATUS_SHORT[it.status] || '' : '';
    const marker = short ? markerOf(it) : '';
    const status = short ? ' data-status="' + esc(it.status) + '"' + (marker ? ' data-marker="' + marker + '"' : '') : '';
    // A window that's in other tabs too has a mark that names them.
    const alsoIn = alsoInOf(view, it);
    const shared = alsoIn.length
      ? '<span class="subtab-shared" title="' + esc('Also in ' + joinNames(alsoIn)) + '">' + ICONS.shared + '</span>'
      : '';
    const notes = [short.toLowerCase(), alsoIn.length ? 'also in ' + joinNames(alsoIn) : ''].filter(Boolean);
    // The tooltip has the whole of a title cut short, then why the item
    // isn't ready, and which other tabs it's in, a line each.
    const tip = [labelOf(view, id), short ? sentence(statusText(it)) : '', alsoIn.length ? 'Also in ' + joinNames(alsoIn) : ''].filter(Boolean);
    return '<button type="button" class="subtab" draggable="false" data-action="activate" data-item="' + esc(id) + '" data-pane="' + esc(p.id) + '"'
      + ' data-key="sub-' + esc(id) + '" aria-pressed="' + (id === active) + '" aria-label="' + esc(labelOf(view, id) + (notes.length ? ' (' + notes.join(', ') + ')' : '')) + '"'
      + ' title="' + esc(tip.join('\n')) + '"' + status + '>'
      + '<span class="mono-tile" aria-hidden="true">' + esc(it.mono) + statusMark(marker) + '</span>'
      + '<span class="subtab-title">' + esc(it.title || it.app) + '</span>' + shared + '</button>';
  }).join('');
  // A window that needs more room covers its pane body, and the
  // explanation there, so the header says so too.
  const room = item && item.status === 'needs-room'
    ? '<span class="pane-note" title="' + esc(sentence(statusText(item))) + '">' + esc(STATUS_SHORT['needs-room']) + '</span>'
    : '';
  const badge = item ? room + (hostChip(item) || '<span class="badge">' + esc(badgeOf(item)) + '</span>') : '';
  let actions = '';
  if (item) {
    // The split button's tooltip has its shortcut, or says why it's off.
    const reason = d.splitsDisabled;
    const move = keysOf(view).moveWindow;
    const tip = reason || withKeys('Show this window side by side', move ? move + '→' : '');
    actions = '<button type="button" class="icon-btn" data-action="split-right" data-pane="' + esc(p.id) + '" data-item="' + esc(active) + '" data-key="split-' + esc(p.id) + '"'
      + ' aria-label="Show this window side by side"' + (reason ? ' aria-disabled="true"' : '') + ' title="' + esc(tip) + '">' + ICONS.splitRight + '</button>'
      + '<button type="button" class="icon-btn" data-action="menu" data-item="' + esc(active) + '" data-pane="' + esc(p.id) + '" data-key="menu-' + esc(p.id) + '" aria-label="Window options" title="Window options" aria-haspopup="menu" aria-expanded="false">' + ICONS.more + '</button>';
  }
  const focused = p.id === d.focus;
  return '<section class="pane" data-pane-id="' + esc(p.id) + '" data-focused="' + focused + '" aria-label="' + esc(item ? labelOf(view, active) : 'Empty pane') + '"'
    + ' style="' + boxStyle(p.rect, area) + '">'
    + '<div class="pane-header">'
    + '<div class="subtabs" role="group" aria-label="Windows in this pane" data-scroll="subtabs-' + esc(p.id) + '">' + subtabs + '</div>'
    + '<button type="button" class="icon-btn add-btn" data-action="add" data-pane="' + esc(p.id) + '" data-key="add-' + esc(p.id) + '" aria-label="Add a window or app to this pane" title="Add a window or app to this pane">' + ICONS.plus + '</button>'
    + '<div class="pane-actions">' + badge + actions + '</div>'
    + '</div>'
    + renderBody(view, p, item)
    + '<div class="drop-indicator" hidden></div>'
    + '</section>';
}

// HOST_WARNINGS are the host chip's warnings, by what they warn of, with
// their label and advice for a page on a host, or, given '', for a page with
// no web address. A page that isn't secure warns of that, even if it's on
// another site too.
const HOST_WARNINGS = {
  'not-secure': {
    text: 'Not secure:',
    label: (host) => 'Not secure: ' + (host ? 'page on ' + host : 'a page with no address'),
    advice: () => 'Don\'t enter passwords or other private details on this page.',
  },
  'off-site': {
    text: 'Other site:',
    label: (host) => 'Other site: ' + (host || 'a page with no address'),
    advice: (host) => (host
      ? 'The page has moved to another site.'
      : 'The page has moved to one with no web address, so Bench can\'t tell which site it\'s from.')
      + ' Check that it\'s the one you expect before you type a password.',
  },
};

// NO_ADDRESS stands in the host chip for the host of a page with no web
// address.
const NO_ADDRESS = 'page with no address';

// hostChip returns the pane header's chip for a web page, following
// Chromium's URL display guidelines: its host, with its site emphasised and
// the start of a long host left out to make room, and a warning if the page
// isn't secure or has moved to another site. A page with no web address,
// such as a data: page, which Go reports as on another site, says so in
// place of a host, so that it can't pass for a page still to load. It
// returns '' for an item with no page to show, which keeps its badge: a page
// that's still loading, VS Code for the Web, or a page after a restart
// before its web view reports it.
function hostChip(item) {
  if (!isPage(item)) return '';
  const host = item.host || '';
  let kind = '';
  if (item.notSecure) kind = 'not-secure';
  else if (item.offSite) kind = 'off-site';
  if (!host && !kind) return '';
  const [sub, site] = hostParts(host, item.site);
  const warning = HOST_WARNINGS[kind];
  const label = warning ? warning.label(host) : 'Page on ' + host;
  const title = warning ? warning.text + ' ' + (host || NO_ADDRESS) + '. ' + warning.advice(host) : host;
  // The site is never cut short. The part of the host before it is in an
  // isolated left-to-right run inside a right-to-left box, so that it's
  // cut short at its start, where the ellipsis goes. In a narrow pane, a
  // warning's words give way to its icon, and then the host goes too (see
  // styles.css), leaving the whole of it to the label and tooltip.
  return '<span class="host-chip" role="note"' + (kind ? ' data-warning="' + kind + '"' : '')
    + ' aria-label="' + esc(label) + '" title="' + esc(title) + '">'
    + (warning ? '<span class="host-warning">' + ICONS.warning + '<span class="host-warning-text">' + esc(warning.text) + '</span></span> ' : '')
    + (sub ? '<span class="host-sub" dir="rtl"><bdi dir="ltr">' + esc(sub) + '</bdi></span>' : '')
    + (host ? '<span class="host-site">' + esc(site) + '</span>' : '<span class="host-none">' + esc(NO_ADDRESS) + '</span>')
    + '</span>';
}

function renderBody(view, p, item) {
  if (!item) return '<div class="pane-body">' + contentEmpty(view, p) + '</div>';
  if (notReady(item)) return '<div class="pane-body">' + statusBody(item) + '</div>';
  if (item.kind === 'builtin') return '<div class="pane-body" data-term="' + esc(item.id) + '"></div>';
  if (isDemo()) return '<div class="pane-body">' + placeholderFor(item) + '</div>';
  // In the app, a web view or the app's own window covers the body.
  return '<div class="pane-body" data-native="' + esc(item.id) + '"></div>';
}

// contentEmpty is the body of a pane with nothing to show. A pane whose
// windows have closed lists the apps it's waiting for, each with a button to
// open it again, unless it isn't installed, and one to forget its place
// there. A pane in a split can close, so that the rest of the tab closes up.
function contentEmpty(view, p) {
  const d = view.display || {};
  const id = esc(p.id);
  const waiting = (p.remembered || []).map((r) => {
    const key = id + '-' + esc(r.bundleId);
    return '<div class="remembered" role="group" aria-label="' + esc(r.app) + '">'
      + '<span class="remembered-app"><span class="mono-tile" aria-hidden="true">' + esc(r.mono) + '</span>'
      + '<span class="status-text">' + esc(r.text) + '</span></span>'
      + '<span class="empty-actions">'
      + (r.canOpen ? '<button type="button" class="btn primary" data-action="open-item" data-item="' + esc(r.itemId) + '" data-key="open-' + key + '">Open ' + esc(r.app) + '</button>' : '')
      + '<button type="button" class="btn" data-action="remove-remembered" data-pane="' + id + '" data-app="' + esc(r.bundleId) + '" data-key="forget-' + key + '">Remove from this pane</button>'
      + '</span></div>';
  }).join('');
  const close = (d.panes || []).length > 1
    ? '<button type="button" class="btn" data-action="close-pane" data-pane="' + id + '" data-key="close-pane-' + id + '">Close this pane</button>'
    : '';
  return '<div class="empty" data-scroll="empty-' + esc(view.current) + '-' + id + '">' + (waiting || '<span>No windows in this pane yet.</span>')
    + '<span class="empty-actions">'
    + '<button type="button" class="btn" data-action="add" data-pane="' + id + '" data-key="empty-add-' + id + '">Add a window or app</button>'
    + close + '</span></div>';
}

// statusBody explains an item that isn't ready, with buttons for what can
// be done about it: dock a moved window again, or open the app of a sub-tab
// that has no window, or forget it.
function statusBody(item) {
  let button = '';
  if (item.status === 'undocked') {
    button = '<button type="button" class="btn" data-action="redock" data-item="' + esc(item.id) + '" data-key="redock-' + esc(item.id) + '">Dock again</button>';
  } else if (item.status === 'missing' && item.noWindow) {
    button = '<span class="empty-actions">'
      + (item.canOpen ? '<button type="button" class="btn primary" data-action="open-item" data-item="' + esc(item.id) + '" data-key="open-' + esc(item.id) + '">Open ' + esc(item.app) + '</button>' : '')
      + '<button type="button" class="btn" data-action="remove-item" data-item="' + esc(item.id) + '" data-key="remove-' + esc(item.id) + '">Remove from Bench</button>'
      + '</span>';
  }
  return '<div class="empty pane-status" data-status="' + esc(item.status) + '"><span class="status-text">' + esc(statusText(item)) + '</span>' + button + '</div>';
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

// layoutText is the size class and why, for the tooltips of the layout
// control and of the status bar's layout.
function layoutText(view, tab) {
  const reason = view.sizeReason || '';
  if (/^(wide|compact) layout/i.test(reason)) return reason;
  let s = view.sizeClass === 'compact' ? 'Compact layout' : 'Wide layout';
  if (view.sizeClass === 'compact' && tab && SMALL_SCREEN_TEXT[tab.smallScreens]) s += ' (' + SMALL_SCREEN_TEXT[tab.smallScreens].toLowerCase() + ')';
  return reason ? s + ', ' + reason : s;
}

// layoutShort names the size class in the status bar: Wide or Compact, or
// "Auto: wide" or "Auto: compact" when Auto chose it.
function layoutShort(view) {
  const compact = view.sizeClass === 'compact';
  if (view.mode === 'wide' || view.mode === 'compact') return compact ? 'Compact' : 'Wide';
  return compact ? 'Auto: compact' : 'Auto: wide';
}

// countText says how many windows the current tab has, in how many panes
// as drawn. In the tabs view, a tab of several panes shows them as one.
function countText(view, tab) {
  const d = view.display || {};
  const count = tab.windows || 0;
  const windows = count + (count === 1 ? ' window' : ' windows');
  if (d.compact && (tab.panes || 0) > 1) return windows + ', shown as one pane';
  const panes = (d.panes || []).length;
  return windows + ' in ' + panes + (panes === 1 ? ' pane' : ' panes');
}

// renderStatus draws the status bar, on one line: the current tab, its
// counts, a note of the windows that need attention, the layout in use,
// the notices, and the key hints. When there's no room, the notice and
// then the layout are cut short, and the key hints go (see styles.css).
// Why the layout is in use is in the layout's tooltip, and clicking it
// says so too, so that it isn't only for those who hover.
function renderStatus(view) {
  const tab = currentTab(view);
  if (!tab) return '';
  const waiting = attentionOf(view).length;
  const attention = waiting
    ? '<button type="button" class="status-attention" data-action="attention" data-key="attention" aria-haspopup="menu" aria-expanded="false">' + ICONS.warning
      + '<span>' + waiting + (waiting === 1 ? ' window needs' : ' windows need') + ' attention</span></button>'
    : '';
  const notices = view.notices || [];
  const notice = notices.length
    ? '<button type="button" class="status-notice" data-action="notices" data-key="notices" aria-haspopup="dialog" aria-expanded="false">' + ICONS.warning
      + '<span class="notice-text">' + esc(notices[0]) + '</span>'
      + (notices.length > 1 ? '<span class="notice-more">' + (notices.length - 1) + ' more</span>' : '') + '</button>'
    : '';
  return '<div class="status-main">'
    + '<span class="status-count">' + esc(countText(view, tab)) + '</span>'
    + attention
    + '<button type="button" class="status-layout" data-action="layout-reason" data-key="layout-reason" title="' + esc(layoutText(view, tab)) + '">'
    + esc(layoutShort(view)) + '</button>'
    + notice
    + '</div>'
    + keyHints(view);
}

// keyHints reminds the user of the main shortcuts, as configured.
function keyHints(view) {
  const keys = keysOf(view);
  const hints = [];
  if (keys.tabs) hints.push(keys.tabs + '1–9 tabs');
  if (keys.jump) hints.push(keys.jump + ' jump');
  if (keys.moveWindow) hints.push(keys.moveWindow + ' arrows move window');
  return hints.length ? '<span class="status-keys">' + esc(hints.join(' · ')) + '</span>' : '';
}

function renderOverlay(view, ui) {
  let inner = '';
  if (ui.overlay === 'palette' || ui.overlay === 'add') inner = renderPalette(view, ui);
  else if (ui.overlay === 'editor') inner = renderEditor(view, ui);
  else if (ui.overlay === 'settings') inner = renderSettings(view, ui);
  else if (ui.overlay === 'guide') inner = renderGuide(view);
  if (!inner) return '';
  return '<div class="overlay" data-action="backdrop">' + inner + '</div>';
}
