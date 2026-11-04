// Drawing the window from the View. Each region (title bar, panes, status
// bar, menu, dialog) is rebuilt from HTML only when its HTML changes, keeping
// focus, the caret, and scroll positions, as the prototype does.

import {
  actLatest, canRestore, isDemo, placeToast, recentMessages, store, syncOverlay,
} from './state.js';
import {
  activeOf, alsoInOf, attentionOf, badgeOf, bellMark, contentRect, currentTab, esc, focusKey, hostParts, ICONS, inCurrentTab, isPage,
  itemOf, isTabKey, joinNames, keysOf, labelOf, LAYOUT_MODES, layoutText, markerOf, needsYouText, notReady, px, RELEASE_WAIT, sameRect,
  scrollBarsShown, sentence, shownInFocus, STATUS_SHORT, statusBarShown, statusMark, statusText, tabKey, unreadMark, unreadNote, unreadText,
  unreadTip, withKeys,
} from './util.js';
import { placeholderFor } from './placeholders.js';
import { attentionKey, fitMenu, renderMenu } from './menu.js';
import { captureMarks, captureNotes, focusPageFind, forgetPageFinds, pageFindBar, tabCaptureMark, tabCaptureText } from './pages.js';
import {
  fillPalette, isList, renderPalette, renderRows, syncSelection, syncSplit,
} from './palette.js';
import { followTab, renderEditor } from './editor.js';
import {
  fillOverview, followHold, renderCards, renderOverview, syncOverview,
} from './overview.js';
import { fillWindows } from './fromwindows.js';
import { fillSettings, renderSettings } from './settings.js';
import { fillForms } from './launchers.js';
import { followMade, maybeOpenGuide, renderBanner, renderGuide } from './guide.js';
import { renderShortcuts } from './shortcuts.js';
import { renderQuit } from './quit.js';
import { closeTabLabel, quickCloseLabel, renderClose } from './close.js';
import { placeDialog } from './dialogs.js';
import {
  followRename, renameField, renamingItem, renamingTab,
} from './rename.js';
import { recheck as recheckPreview } from './preview.js';
import { syncStrips } from './strips.js';
import * as terminals from './terminals.js';
import { setAppearance } from './theme.js';
import { tile } from './tiles.js';

const SCREENS = [
  { id: 'fit', label: 'Fit to browser' },
  { id: 'monitor', label: 'Monitor · 2,560 pt' },
  { id: 'laptop', label: 'Laptop · 1,512 pt' },
];

// SLIM_BELOW is the width, in points, of a title bar too narrow for the
// whole layout control and Jump to… beside the tabs (see syncSlim).
const SLIM_BELOW = 1200;

// NOTE_WORDS_BELOW is the width, in points, of a title bar too narrow for
// the words of the hidden status bar's notes (see titleNotes): below it,
// each note shows only its number.
const NOTE_WORDS_BELOW = 1000;

const lastHTML = new WeakMap();
let deferred = false;
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
// menu or dialog, or the field that renames a tab or sub-tab, keeps the
// focus while it's open: the terminal gets it when that closes, instead of
// the control that opened it.
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
  // dialog, or the field that renames a tab or sub-tab, has closed.
  if (ui.menu || ui.overlay || ui.rename) {
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

// render draws everything from the store. While a tab or sub-tab is being
// dragged it draws only the menu, which can close then, and waits until the
// drag ends for the rest, so the dragged element isn't replaced. While the
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
  followMade(view);
  // The strips of tabs and sub-tabs draw their scroll bars inside their own
  // padding (see styles.css), so showing them moves nothing else.
  document.body.classList.toggle('scroll-bars', scrollBarsShown(view));
  applyDensity(view);
  setAppearance((view.settings || {}).appearance);

  // Focus on a tab's button follows its tab (followTabKey).
  const focusedTab = focusedTabKey();
  if (setRegion($('titlebar'), renderTitlebar(view)) && focusedTab) focusKey(followTabKey(view, focusedTab));
  drawnTabs = view.tabs.map((t) => t.name);
  syncSlim();
  const banner = renderBanner(view);
  $('banner').hidden = !banner;
  setRegion($('banner'), banner);

  // The terminals follow Settings' text size and Option key, before any new
  // one is made.
  terminals.configure(view);
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
  // A find bar that was asked for gets the keyboard once it's drawn.
  focusPageFind(panes);
  if (moved) {
    const el = document.activeElement;
    const host = el && el.closest ? el.closest('.term-host[data-term-host]') : null;
    if (host && host.dataset.termHost !== shown) el.blur();
  }
  applyTermFocus();

  // Hiding the status bar gives its height to the panes, which measure
  // reports to Go. A toast that's showing moves to where it can be seen.
  const status = $('statusbar');
  const bar = statusBarShown(view);
  if (status.hidden === bar) {
    status.hidden = !bar;
    placeToast();
  }
  setRegion(status, bar ? renderStatus(view) : '');
  setRegion($('menu-root'), renderMenu(view, ui), { after: fitMenu });
  const overlay = $('overlay-root');
  setRegion(overlay, renderOverlay(view, ui), {
    fill: () => {
      fillPalette(ui);
      fillOverview(ui);
      fillSettings(ui);
      fillForms(ui);
      fillWindows();
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
  if (ui.overlay === 'overview') {
    const cards = $('overview-list');
    if (cards) setRegion(cards, renderCards(view, ui));
    syncOverview(view, ui);
  }
  // A dialog that has been moved or resized keeps its place and size, and
  // one that the user has resized before opens at that size. It goes after
  // the rows, since a moved dialog is as tall as its contents.
  placeDialog();

  if (ui.pendingFocus) {
    const key = ui.pendingFocus;
    ui.pendingFocus = null;
    if (focusKey(key)) {
      const el = document.activeElement;
      if (el && el.tagName === 'INPUT' && el.type === 'text') el.select();
    }
  }
  syncStrips();
  syncExpanded(ui);
  terminals.prune(view);
  recheckPreview();
  syncOverlay();
  syncSplit();
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
  document.querySelectorAll('[aria-expanded="true"]:is([data-action="menu"], [data-action="notices"], [data-action="attention"], [data-action="layout-menu"])')
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
// switched tabs; or the windows that need attention have changed, or Go
// has switched tabs, so that the list's entries don't move under the
// pointer. The layout menu's subject never goes, nor do the notices', which
// keep the recent messages once the notices have gone.
function staleMenu(view, m) {
  if (m.kind === 'layout') return false;
  if (m.kind === 'notices') return false;
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

// dropStale closes a menu, the add dialog, Tab properties, or the field
// that renames a tab or sub-tab, whose subject has gone, such as a window
// that closed, a tab deleted elsewhere,
// a tab that Go has switched away from (with a global shortcut, the menu
// bar item, or focus following), or the web page that the add dialog waits
// to put a web app in the place of, which the web app may have taken. It
// opens and closes the overview of every tab for Go's hold, too.
function dropStale(view, ui) {
  // Focus goes back to the control that opened what closes, after this
  // render, if it's still there and focus was inside what closed (and
  // canRestore agrees).
  const restore = (opener, root) => {
    if (opener && root && root.contains(document.activeElement) && canRestore(opener)) ui.pendingFocus = opener;
  };
  // The overview of every tab opens for Go's hold after last_tab's
  // shortcut, and closes once it ends.
  followHold(view, ui, (opener) => restore(opener, $('overlay-root')));
  // A tab's button that opened what's open follows its tab.
  ui.opener = followTabKey(view, ui.opener);
  const m = ui.menu;
  if (m) m.opener = followTabKey(view, m.opener);
  if (m && staleMenu(view, m)) {
    ui.menu = null;
    ui.paneLater = null;
    restore(m.opener, $('menu-root'));
  }
  // The empty pane that a split made for the add dialog (splitEmpty) may
  // not be in the View yet as the dialog opens: it counts once it is.
  const add = ui.add;
  if (add && add.split === 'new' && inCurrentTab(view, add.paneId)) add.split = 'shown';
  if (ui.overlay === 'add' && (!add || add.tab !== view.current || (!inCurrentTab(view, add.paneId) && add.split !== 'new')
    || (add.replace && !inTab(view, add.replace.itemId, add.tab)))) {
    ui.overlay = null;
    ui.add = null;
    ui.listItems = [];
    ui.paneLater = null;
    const opener = ui.opener;
    ui.opener = null;
    restore(opener, $('overlay-root'));
  }
  // The field that renames a tab or sub-tab goes with it, and so does
  // what a closed page's find bar had.
  if (ui.rename && !followRename(ui.rename, view)) ui.rename = null;
  forgetPageFinds(view);
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
  // A drag of a tab or sub-tab puts renders off by itself (ui.drag). An
  // HTML drag (of selected text, say) keeps the button coming up from the
  // page. A menu opened by Control-clicking opens straight away.
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

// focusFirst focuses the control in a dialog that's marked to have the
// focus when the one that had it goes (data-focus-first), such as the
// welcome guide's Continue, or else its first control.
function focusFirst(dialog) {
  if (!dialog) return;
  const el = dialog.querySelector('[data-focus-first]:not([disabled])')
    || dialog.querySelector('button:not([tabindex="-1"]):not([disabled]), input:not([disabled]), select, [href]');
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

// applyDensity sizes Bench's chrome for the density in use (styles.css):
// the panes' sub-tabs and buttons for the View's density, and the title bar
// for its own, which in the app is the density the window's native title
// bar was made with at launch. Each pane's header is as tall as Go says
// (renderPane), since Go docks windows below it.
function applyDensity(view) {
  const density = view.density === 'compact' ? 'compact' : 'standard';
  const title = view.titleBar === 'compact' ? 'compact' : 'standard';
  if (document.body.dataset.density !== density) document.body.dataset.density = density;
  if (document.body.dataset.titleBar !== title) document.body.dataset.titleBar = title;
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

// syncSlim slims the title bar on a narrow window, or a small screen (the
// compact size class), to leave the tabs more room: the layout control
// becomes one button that opens a menu of the layouts, and Jump to… its
// icon (see styles.css). It runs as the title bar is drawn, and as it
// changes size. A layout control with the focus hands it to the one that
// takes its place, as when choosing Wide from the menu widens the layout.
// Below NOTE_WORDS_BELOW, it also shortens the notes to their numbers.
export function syncSlim() {
  const bar = $('titlebar');
  if (!bar || !store.view) return;
  const short = String(bar.clientWidth < NOTE_WORDS_BELOW);
  if (bar.dataset.shortNotes !== short) bar.dataset.shortNotes = short;
  const slim = String(bar.clientWidth < SLIM_BELOW || store.view.sizeClass === 'compact');
  if (bar.dataset.slim === slim) return;
  const active = document.activeElement;
  const key = active && bar.contains(active) ? active.dataset.key || '' : '';
  bar.dataset.slim = slim;
  if (slim === 'false' && key === 'layout-menu') focusKey('mode-' + store.view.mode);
  else if (slim === 'true' && key.startsWith('mode-')) focusKey('layout-menu');
}

// tabKeysShown reports whether tabs show their shortcuts: the
// tab_shortcut_hints setting, where auto shows them except on small screens
// (the compact size class), whose tabs need the room. A tab's tooltip has
// its shortcut either way.
function tabKeysShown(view) {
  const hints = (view.settings || {}).tabShortcutHints;
  if (hints === 'always' || hints === 'never') return hints === 'always';
  return view.sizeClass !== 'compact';
}

// renderTitlebar draws the tabs, with + right after them, which a sub-tab
// can be dropped on to make a new tab for its window (drag.js), and the
// layout control, Jump to…, and Settings at the right. The space between is
// the title bar's own, which moves the window in the app. Each tab's tooltip
// has its whole name and its shortcut, and the layout control's says why
// the layout is in use. A slim title bar (syncSlim) shows the layout
// button and the icon of Jump to… instead, so both are drawn. A tab's
// shortcut is at its right end. In the "tabs" view, the one pane's sub-tabs
// and controls come after the tabs' + (titlePane), in place of the pane's
// header. A tab's unread total is in a grey pill right after its name, on
// the current tab too, as the counts are the apps' own and looking at a tab
// doesn't change them; but not on the current tab in the "tabs" view, whose
// sub-tabs show the counts right beside it, in a title bar where its strip
// has no room for more: the pill would push its × out of view. Its tooltip
// and the name a screen reader reads still have the total. A tab's × closes
// it (closeCross), in its shortcut's place, or in an empty slot of its own
// on a tab that doesn't show one, so that it never covers the tab's name.
function renderTitlebar(view) {
  const jump = keysOf(view).jump;
  const keys = tabKeysShown(view);
  const paneInTitle = Boolean(titlePaneOf(view));
  const traffic = isDemo() ? '<div class="traffic" aria-hidden="true"><span></span><span></span><span></span></div>' : '';
  const close = closeTabLabel(view).replace('…', '');
  const tabs = view.tabs.map((t, i) => (renamingTab(i)
    ? '<div class="tab renaming" data-index="' + i + '" data-drop-tab="' + i + '" data-current="' + (i === view.current) + '">'
      + renameField(view, store.ui.rename) + '</div>'
    : '<button type="button" class="tab" data-action="tab" data-index="' + i + '" data-drop-tab="' + i + '" data-key="' + tabKey(i) + '" aria-pressed="' + (i === view.current) + '"'
    + ' title="' + esc(withKeys(t.name, t.shortcut) + (unreadText(t.unread) ? '\n' + unreadText(t.unread) : '')
      + (tabCaptureText(t) ? '\n' + tabCaptureText(t) : '') + (t.attention ? '\n' + needsYouText(t.attention) : '')) + '">'
    + '<span class="tab-name">' + esc(t.name) + '</span>'
    + (unreadText(t.unread) ? (paneInTitle && i === view.current ? '' : unreadMark('tab-unread', t.unread))
      + '<span class="visually-hidden">, ' + esc(unreadText(t.unread).toLowerCase()) + '</span>' : '')
    + tabCaptureMark(t)
    + (t.attention ? bellMark('tab-bell', needsYouText(t.attention), t.attention)
      + '<span class="visually-hidden">, ' + esc(needsYouText(t.attention)) + '</span>' : '')
    + (keys && t.shortcut ? '<kbd>' + esc(t.shortcut) + '</kbd>' : '<span class="cross-slot" aria-hidden="true"></span>')
    + closeCross('close-tab', ' data-index="' + i + '"', close) + '</button>')).join('');
  const reason = esc(layoutText(view, currentTab(view)));
  const modes = LAYOUT_MODES.map(([id, label]) =>
    '<button type="button" data-action="mode" data-mode="' + id + '" data-key="mode-' + id + '" aria-pressed="' + (view.mode === id) + '">' + label + '</button>').join('');
  const mode = (LAYOUT_MODES.find(([id]) => id === view.mode) || LAYOUT_MODES[0])[1];
  const jumpLabel = esc(withKeys('Jump to…', jump));
  return traffic
    + '<nav class="tabs" aria-label="Workspace tabs" data-scroll="tabs">' + tabs + '</nav>'
    + '<button type="button" class="icon-btn new-tab" data-action="new-tab" data-key="new-tab" data-drop-new aria-label="New tab" title="New tab">' + ICONS.plus + '</button>'
    + titlePane(view)
    + '<div class="titlebar-end">'
    + '<div class="seg layout-seg" role="group" aria-label="Layout" title="' + reason + '">' + modes + '</div>'
    + '<button type="button" class="layout-btn" data-action="layout-menu" data-key="layout-menu" aria-haspopup="menu" aria-expanded="false"'
    + ' aria-label="' + esc('Layout: ' + mode) + '" title="' + reason + '">' + esc(mode) + ICONS.chevronDown + '</button>'
    + titleNotes(view)
    + '<button type="button" class="jump-btn" data-action="open-palette" data-key="open-palette" aria-label="' + jumpLabel + '" title="' + jumpLabel + '">'
    + ICONS.search + '<span>Jump to…</span>' + (jump ? '<kbd>' + esc(jump) + '</kbd>' : '') + '</button>'
    + '<button type="button" class="icon-btn bordered" data-action="open-settings" data-key="open-settings" aria-label="Settings" title="Settings">' + ICONS.gear + '</button>'
    + '</div>';
}

// titlePane draws the "tabs" view's pane's sub-tabs, its +, and the rest of
// its header's controls, for the title bar: the pane has no header of its
// own (its header is 0), which leaves its windows the room. The sub-tabs
// scroll on their own, as the tabs do, and are dragged, right-clicked, and
// marked as in a header.
function titlePane(view) {
  const p = titlePaneOf(view);
  if (!p) return '';
  const active = activeOf(p);
  return '<div class="title-pane" data-pane-id="' + esc(p.id) + '">'
    + subtabStrip(view, p, active, 'Windows in this tab')
    + paneControls(view, p, active)
    + '</div>';
}

// titlePaneOf returns the "tabs" view's one pane, whose sub-tabs are in the
// title bar, or null in any other view.
function titlePaneOf(view) {
  const d = view.display || {};
  return (d.compact && (d.panes || []).find((x) => x.id === 'compact')) || null;
}

// titleNotes draws the status bar's notes in the title bar while the status
// bar is hidden, beside Jump to…: how many windows need attention, and the
// notices, each a button that opens the same list as in the status bar.
// Docked windows cover the panes, so the title bar is the one place where
// they're sure to be seen. A title bar under NOTE_WORDS_BELOW shows each
// note's count without its words (title-note-words), which its accessible
// name and tooltip keep.
function titleNotes(view) {
  if (statusBarShown(view)) return '';
  const waiting = attentionOf(view).length;
  const notices = view.notices || [];
  let html = '';
  if (waiting) {
    const text = waiting + (waiting === 1 ? ' window needs' : ' windows need') + ' attention';
    html += '<button type="button" class="title-note" data-action="attention" data-key="attention" aria-haspopup="menu" aria-expanded="false"'
      + ' aria-label="' + esc(text) + '" title="' + esc(text) + '">' + ICONS.warning
      + '<span>' + waiting + '<span class="title-note-words">' + (waiting === 1 ? ' needs' : ' need') + ' attention</span></span></button>';
  }
  if (notices.length) {
    const text = notices.length + (notices.length === 1 ? ' notice' : ' notices');
    html += '<button type="button" class="title-note" data-action="notices" data-key="notices" aria-haspopup="dialog" aria-expanded="false"'
      + ' aria-label="' + esc(text) + '" title="' + esc(notices.map((n) => n.text).join('\n')) + '">' + ICONS.warning
      + '<span>' + notices.length + '<span class="title-note-words">' + (notices.length === 1 ? ' notice' : ' notices') + '</span></span></button>';
  }
  return html;
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

// renderPane draws a pane: its header, as tall as Go says (--pane-header),
// with its sub-tabs and controls, then its body. A pane whose header is 0
// has none: the "tabs" view's pane, whose header is in the title bar
// (titlePane), and a pane whose one App window's own title bar stands in
// for it (pane_headers). Its body starts just inside its border, as Go's
// does.
function renderPane(view, p, area) {
  const d = view.display;
  const active = activeOf(p);
  const item = itemOf(view, active);
  const header = Number(p.header) || 0;
  let headerHTML = '';
  let flag = '';
  if (header > 0) {
    headerHTML = '<div class="pane-header">' + subtabStrip(view, p, active, 'Windows in this pane') + paneControls(view, p, active) + '</div>';
  } else {
    flag = ' data-header="' + (p.id === 'compact' ? 'title-bar' : 'none') + '"';
  }
  const focused = p.id === d.focus;
  return '<section class="pane" data-pane-id="' + esc(p.id) + '" data-focused="' + focused + '"' + flag
    + ' aria-label="' + esc(item ? labelOf(view, active) : 'Empty pane') + '"'
    + ' style="' + boxStyle(p.rect, area) + '; --pane-header: ' + px(header) + '">'
    + headerHTML
    + renderBody(view, p, item)
    + '<div class="drop-indicator" hidden></div>'
    + '</section>';
}

// closeCross draws the × at the right end of a tab or sub-tab, which
// closes it: the click runs action, with attrs, not the tab's or
// sub-tab's own. It shows on the current tab and the shown sub-tab, and on
// the others under the pointer or with the keyboard focus. On a tab, it's
// in the place of its shortcut, or of the empty slot that a tab without
// one has, so tabs are as wide with it as without. The shown sub-tab has
// a slot of its own (subtabStrip), after its title rather than over it,
// unless its strip is full (see styles.css); on the other sub-tabs, it's
// over the end of the title, which fades into it. Hidden, it takes no
// clicks (visibility: hidden). It isn't a button of its own: the tab and
// window menus close them from the keyboard, so a screen reader skips it.
function closeCross(action, attrs, title) {
  return '<span class="close-cross" data-action="' + action + '"' + attrs + ' aria-hidden="true" title="' + esc(title) + '">' + ICONS.close + '</span>';
}

// subtabStrip draws a pane's sub-tabs, with active shown, in a strip that
// scrolls (strips.js), labelled for a screen reader. Each has a × that
// closes it, or for one in other tabs too, takes it out of this one
// (quickClose). The shown one has a slot for its × after its title, as a
// tab has (closeCross).
function subtabStrip(view, p, active, label) {
  const subtabs = (p.items || []).map((id) => {
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
    // A bell marks one that needs you, and says why: a terminal that rang
    // its bell, or a page that posted a notification, while you couldn't
    // see it. It goes before the title, where the × never covers it.
    const why = it.attention ? sentence(it.attention) : '';
    // A page that uses the camera or the microphone has a mark of each,
    // before its title, where the × never covers it (captureMarks). So
    // does an unread count, in a grey pill after the bell (unreadMark).
    const capture = captureNotes(it);
    const notes = [short.toLowerCase(), unreadNote(it.unread), ...capture.map((c) => c.toLowerCase()), why ? 'needs you' : '',
      alsoIn.length ? 'also in ' + joinNames(alsoIn) : ''].filter(Boolean);
    // The tooltip has the whole of a title cut short, with its app, then
    // the title its shell or page gives it, under a name of the user's
    // own, why the item isn't ready, how many unread, why it needs you, and
    // which other tabs it's in, a line each.
    const auto = it.label && it.autoTitle ? 'Automatic title: ' + it.autoTitle : '';
    const tip = [labelOf(view, id), auto, short ? sentence(statusText(it)) : '', unreadTip(it.unread), ...capture, why,
      alsoIn.length ? 'Also in ' + joinNames(alsoIn) : ''].filter(Boolean);
    // A sub-tab being renamed has the field in place of its title
    // (rename.js), and keeps its place to drop on.
    if (renamingItem(id, p.id)) {
      return '<div class="subtab renaming" data-item="' + esc(id) + '" data-pane="' + esc(p.id) + '" title="' + esc(tip.join('\n')) + '">'
        + tile(it.mono, it.icon)
        + renameField(view, store.ui.rename) + '</div>';
    }
    return '<button type="button" class="subtab" draggable="false" data-action="activate" data-item="' + esc(id) + '" data-pane="' + esc(p.id) + '"'
      + ' data-key="sub-' + esc(id) + '" aria-pressed="' + (id === active) + '" aria-label="' + esc(labelOf(view, id) + (notes.length ? ' (' + notes.join(', ') + ')' : '')) + '"'
      + ' title="' + esc(tip.join('\n')) + '"' + status + '>'
      + tile(it.mono, it.icon, statusMark(marker)) + captureMarks(it) + bellMark('subtab-bell', why) + unreadMark('subtab-unread', it.unread)
      + '<span class="subtab-title">' + esc(it.title || it.app) + '</span>' + shared
      + (id === active ? '<span class="cross-slot" aria-hidden="true"></span>' : '')
      + closeCross('close-item', ' data-item="' + esc(id) + '" data-pane="' + esc(p.id) + '"', quickCloseLabel(it)) + '</button>';
  }).join('');
  return '<div class="subtabs" role="group" aria-label="' + esc(label) + '" data-scroll="subtabs-' + esc(p.id) + '">' + subtabs + '</div>';
}

// paneControls draws a pane's header controls after its sub-tabs: + to add
// to the pane, then the Zoomed chip of a zoomed pane, which shows every
// pane again, and, for the window it shows, a note if the window needs
// more room, its badge or web page's host, the split button, and ⋯.
function paneControls(view, p, active) {
  const d = view.display;
  const item = itemOf(view, active);
  const zoomed = d.zoomed && d.zoomed === p.id
    ? '<button type="button" class="zoom-chip" data-action="show-all-panes" data-key="unzoom-' + esc(p.id) + '"'
      + ' aria-label="Zoomed: show all panes" title="' + esc(withKeys('Show all panes', keysOf(view).zoomPane)) + '">'
      + ICONS.zoomed + '<span>Zoomed</span></button>'
    : '';
  // A window that needs more room covers its pane body, and the
  // explanation there, so the header says so too.
  const room = item && item.status === 'needs-room'
    ? '<span class="pane-note" title="' + esc(sentence(statusText(item))) + '">' + esc(STATUS_SHORT['needs-room']) + '</span>'
    : '';
  const badge = item ? room + (hostChip(item) || '<span class="badge">' + esc(badgeOf(item)) + '</span>') : '';
  let actions = '';
  if (item) {
    // The split button's tooltip has its shortcut. In the "tabs" view,
    // which hides the tab's splits, it changes to the wide layout to show
    // the window side by side, and its name says so, for VoiceOver too.
    const move = keysOf(view).moveWindow;
    const name = d.splitsDisabled ? 'Show this window side by side, in the wide layout' : 'Show this window side by side';
    const tip = withKeys(name, move ? move + '→' : '');
    actions = '<button type="button" class="icon-btn" data-action="split-right" data-pane="' + esc(p.id) + '" data-item="' + esc(active) + '" data-key="split-' + esc(p.id) + '"'
      + ' aria-label="' + esc(name) + '" title="' + esc(tip) + '">' + ICONS.splitRight + '</button>'
      + '<button type="button" class="icon-btn" data-action="menu" data-item="' + esc(active) + '" data-pane="' + esc(p.id) + '" data-key="menu-' + esc(p.id) + '" aria-label="Window options" title="Window options" aria-haspopup="menu" aria-expanded="false">' + ICONS.more + '</button>';
  }
  return '<button type="button" class="icon-btn add-btn" data-action="add" data-pane="' + esc(p.id) + '" data-key="add-' + esc(p.id) + '" aria-label="Add a window or app to this pane" title="Add a window or app to this pane">' + ICONS.plus + '</button>'
    + '<div class="pane-actions">' + zoomed + badge + actions + '</div>';
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

// renderBody draws a pane's body: what it shows, or why it shows nothing.
// A web page whose find bar is open (ItemView's finding) has the bar at the
// top, where Go leaves room above its web view (pages.js).
function renderBody(view, p, item) {
  if (!item) return '<div class="pane-body">' + contentEmpty(view, p) + '</div>';
  if (notReady(item)) return '<div class="pane-body">' + statusBody(p, item) + '</div>';
  if (item.kind === 'builtin') return '<div class="pane-body" data-term="' + esc(item.id) + '"></div>';
  const finding = item.kind === 'web' && item.finding;
  const bar = finding ? pageFindBar(item) : '';
  const flag = finding ? ' data-finding="true"' : '';
  if (isDemo()) return '<div class="pane-body"' + flag + '>' + bar + placeholderFor(item) + '</div>';
  // In the app, a web view or the app's own window covers the body.
  return '<div class="pane-body" data-native="' + esc(item.id) + '"' + flag + '>' + bar + '</div>';
}

// contentEmpty is the body of a pane with nothing to show. A pane whose
// windows have closed lists the apps it's waiting for, each with a button to
// open it again, unless it isn't installed, and one to forget its place
// there. A pane in a split can close, so that the rest of the tab closes up,
// zoomed or not.
function contentEmpty(view, p) {
  const d = view.display || {};
  const tab = currentTab(view);
  const id = esc(p.id);
  const waiting = (p.remembered || []).map((r) => {
    const key = id + '-' + esc(r.bundleId);
    return '<div class="remembered" role="group" aria-label="' + esc(r.app) + '">'
      + '<span class="remembered-app">' + tile(r.mono, r.icon)
      + '<span class="status-text">' + esc(r.text) + '</span></span>'
      + '<span class="empty-actions">'
      + (r.canOpen ? '<button type="button" class="btn primary" data-action="open-item" data-item="' + esc(r.itemId) + '" data-key="open-' + key + '">Open ' + esc(r.app) + '</button>' : '')
      + '<button type="button" class="btn" data-action="remove-remembered" data-pane="' + id + '" data-app="' + esc(r.bundleId) + '" data-key="forget-' + key + '">Remove from this pane</button>'
      + '</span></div>';
  }).join('');
  const close = (d.zoomed ? (tab && tab.panes) || 0 : (d.panes || []).length) > 1
    ? '<button type="button" class="btn" data-action="close-pane" data-pane="' + id + '" data-key="close-pane-' + id + '">Close this pane</button>'
    : '';
  return '<div class="empty" data-scroll="empty-' + esc(view.current) + '-' + id + '">' + (waiting || '<span>No windows in this pane yet.</span>' + emptyHint(view))
    + '<span class="empty-actions">'
    + '<button type="button" class="btn" data-action="add" data-pane="' + id + '" data-key="empty-add-' + id + '">Add a window or app</button>'
    + close + '</span></div>';
}

// emptyHint says how to bring a window into an empty pane, with the move
// window shortcut as configured.
function emptyHint(view) {
  const move = keysOf(view).moveWindow;
  const text = move
    ? 'Drag a sub-tab here, or press ' + move + ' and an arrow to move the shown window here.'
    : 'Drag a sub-tab here.';
  return '<span class="empty-hint">' + esc(text) + '</span>';
}

// statusBody explains an item that isn't ready, in pane p, with buttons for
// what can be done about it: dock a moved window again, show a minimised one
// again or take it out of the tab, or open the app of a sub-tab that has no
// window, or forget it.
function statusBody(p, item) {
  let button = '';
  if (item.status === 'undocked') {
    button = '<button type="button" class="btn" data-action="redock" data-item="' + esc(item.id) + '" data-key="redock-' + esc(item.id) + '">Dock again</button>';
  } else if (item.status === 'minimised') {
    button = '<span class="empty-actions">'
      + '<button type="button" class="btn primary" data-action="show-again" data-pane="' + esc(p.id) + '" data-item="' + esc(item.id) + '" data-key="show-again-' + esc(item.id) + '">Show again</button>'
      + '<button type="button" class="btn" data-action="remove-from-tab" data-item="' + esc(item.id) + '" data-key="remove-from-tab-' + esc(item.id) + '">Remove from this tab</button>'
      + '</span>';
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

// layoutShort names the size class in the status bar: Wide or Compact, or
// "Auto: wide" or "Auto: compact" when Auto chose it.
function layoutShort(view) {
  const compact = view.sizeClass === 'compact';
  if (view.mode === 'wide' || view.mode === 'compact') return compact ? 'Compact' : 'Wide';
  return compact ? 'Auto: compact' : 'Auto: wide';
}

// countText says how many windows the current tab has, in how many panes
// as drawn. In the tabs view, a tab of several panes shows them as one. A
// zoomed pane hides the others for now, so they're counted.
function countText(view, tab) {
  const d = view.display || {};
  const count = tab.windows || 0;
  const windows = count + (count === 1 ? ' window' : ' windows');
  if (d.compact && (tab.panes || 0) > 1) return windows + ', shown as one pane';
  const panes = d.zoomed ? tab.panes || 0 : (d.panes || []).length;
  return windows + ' in ' + panes + (panes === 1 ? ' pane' : ' panes');
}

// renderStatus draws the status bar, on one line: the current tab, its
// counts, a note that a pane is zoomed, which shows every pane again when
// clicked, a note of the windows that need attention, the layout in use,
// the notices, or, with none, a button for the recent messages, and the key
// hints. When there's no room, the notice and
// then the layout are cut short, and the key hints go (see styles.css).
// Why the layout is in use is in the layout's tooltip, and clicking it
// says so too, so that it isn't only for those who hover.
function renderStatus(view) {
  const tab = currentTab(view);
  if (!tab) return '';
  const zoomed = (view.display || {}).zoomed
    ? '<button type="button" class="status-zoom" data-action="show-all-panes" data-key="status-unzoom"'
      + ' title="' + esc(withKeys('Show all panes', keysOf(view).zoomPane)) + '">' + ICONS.zoomed + '<span>One pane zoomed</span></button>'
    : '';
  const waiting = attentionOf(view).length;
  const attention = waiting
    ? '<button type="button" class="status-attention" data-action="attention" data-key="attention" aria-haspopup="menu" aria-expanded="false">' + ICONS.warning
      + '<span>' + waiting + (waiting === 1 ? ' window needs' : ' windows need') + ' attention</span></button>'
    : '';
  const notices = view.notices || [];
  let notice = '';
  if (notices.length) {
    notice = '<button type="button" class="status-notice" data-action="notices" data-key="notices" aria-haspopup="dialog" aria-expanded="false">' + ICONS.warning
      + '<span class="notice-text">' + esc(notices[0].text) + '</span>'
      + (notices.length > 1 ? '<span class="notice-more">' + (notices.length - 1) + ' more</span>' : '') + '</button>';
  } else if (recentMessages(view).length) {
    // With no notices, the recent messages are a quiet button away.
    notice = '<button type="button" class="status-messages" data-action="notices" data-key="notices" aria-haspopup="dialog" aria-expanded="false"'
      + ' aria-label="Recent messages" title="Recent messages">' + ICONS.messages + '</button>';
  }
  return '<div class="status-main">'
    + '<span class="status-count">' + esc(countText(view, tab)) + '</span>'
    + zoomed
    + attention
    + '<button type="button" class="status-layout" data-action="layout-reason" data-key="layout-reason" title="' + esc(layoutText(view, tab)) + '">'
    + esc(layoutShort(view)) + '</button>'
    + notice
    + '</div>'
    + keyHints(view);
}

// keyHints reminds the user of the main shortcuts, as configured, as a
// button that opens the Keyboard Shortcuts sheet, which has the rest.
function keyHints(view) {
  const keys = keysOf(view);
  const hints = [];
  if (keys.tabs) hints.push(keys.tabs + '1–9 tabs');
  if (keys.jump) hints.push(keys.jump + ' jump');
  if (keys.moveWindow) hints.push(keys.moveWindow + ' arrows move window');
  if (!hints.length) return '';
  return '<button type="button" class="status-keys" data-action="shortcuts" data-key="status-keys" title="Keyboard Shortcuts"'
    + ' aria-haspopup="dialog">' + esc(hints.join(' · ')) + '</button>';
}

function renderOverlay(view, ui) {
  let inner = '';
  if (ui.overlay === 'palette' || ui.overlay === 'add') inner = renderPalette(view, ui);
  else if (ui.overlay === 'editor') inner = renderEditor(view, ui);
  else if (ui.overlay === 'settings') inner = renderSettings(view, ui);
  else if (ui.overlay === 'guide') inner = renderGuide(view);
  else if (ui.overlay === 'shortcuts') inner = renderShortcuts(view);
  else if (ui.overlay === 'quit') inner = renderQuit(view, ui);
  else if (ui.overlay === 'close') inner = renderClose(view, ui);
  else if (ui.overlay === 'overview') inner = renderOverview(view, ui);
  if (!inner) return '';
  return '<div class="overlay" data-action="backdrop">' + inner + '</div>';
}
