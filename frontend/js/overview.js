// The overview of every tab (decision 60): a card for each tab, with its
// panes drawn to scale (renderMini), the window each pane shows, with its
// app's tile and title, the pane's other windows' tiles, and the marks
// that the tab strip and sub-tabs show: a bell for a window that needs
// you, ⚠ for one with a problem, and the camera and microphone. It shows
// no pictures of the windows, so it needs no Screen Recording permission.
//
// It opens from View ▸ Show All Tabs, the jump palette's Show All Tabs,
// and the show_all_tabs shortcut, which closes it too. It's the open
// dialog, so it takes the place of any other, as Settings, asking first if
// that one has changes that aren't saved (afterAsking, discard.js). Its
// cards are in the tab bar's order, with the last used tab chosen, as the
// palette chooses it. Typing filters them by a tab's name or its windows'
// titles and apps. The arrow keys move the choice, from card to card and
// from row to row, and Return or a click switches to the tab chosen. With
// nothing typed, 1 to 9 switch to that tab. Escape closes it, and Bench
// gives the keyboard back to the window that had it.
//
// Holding last_tab's modifiers after its shortcut opens it too (Go's hold,
// hold.go, and View.hold): its cards are then in the order the tabs were
// last used, and Go chooses the tab, which the shortcut's key, the arrow
// keys (StepHold), and Go's focus_pane_modifiers shortcuts move, Up and
// Down by a row as drawn (holdRow). Letting go of the modifiers switches
// to the tab chosen, and Go ends the hold, which closes the overview.
// Escape ends it without switching (CancelHold), and a click or Return
// switches at once. The user is holding Control, so a click is a
// Control-click, which macOS makes a contextmenu event
// (overviewContextMenu).

import { act, closeOverlay, openerKey, store, switchTab, update } from './state.js';
import { renderMini } from './editor.js';
import { fold } from './palette.js';
import { captureMarks, captureNotes, NOT_LOADED, tabCaptureMark, tabCaptureText } from './pages.js';
import { tile } from './tiles.js';
import {
  anyColoured, bellMark, esc, ICONS, itemOf, keysOf, labelOf, leaves, markerOf, needsYouText, sentence, splitShortcut, statusMark,
  tabDot, unreadMark, unreadText, unreadTip,
} from './util.js';

// OTHERS is how many of a pane's other windows its card draws tiles for:
// more are counted.
const OTHERS = 4;

// openOverview opens the overview of every tab, with the last used tab
// chosen, or the current tab if there's none.
export function openOverview() {
  const { ui, view } = store;
  if (!view) return;
  if (!ui.overlay) ui.opener = openerKey();
  ui.menu = null;
  ui.paneLater = null;
  ui.overlay = 'overview';
  ui.add = null;
  ui.editor = null;
  ui.settings = null;
  const last = view.lastTab >= 0 && view.lastTab !== view.current && view.tabs[view.lastTab] ? view.lastTab : view.current;
  ui.overview = { query: '', sel: last, hold: 0 };
  ui.pendingFocus = 'overview-query';
  update();
}

// toggleOverview opens the overview, or closes it if it's open, for the
// show_all_tabs shortcut.
export function toggleOverview() {
  if (store.ui.overlay === 'overview') closeOverlay();
  else openOverview();
}

// followHold opens the overview for Go's hold once the View has it, unless
// a menu or dialog is open, and closes it once the hold has ended, as Go
// switches tabs then, or gives up. render's dropStale calls it before
// drawing, with restore, which puts the focus back on a control after the
// render if the overview had it; the hold-tabs command, which brings Bench
// forward for it, asks for that render.
export function followHold(view, ui, restore) {
  const hold = view.hold;
  const o = ui.overlay === 'overview' ? ui.overview : null;
  if (hold && hold.id !== ui.closedHold) {
    if (o && o.hold === hold.id) return;
    if (ui.overlay || ui.menu || ui.nativeMenu || ui.drag) return;
    ui.opener = openerKey();
    ui.paneLater = null;
    ui.overlay = 'overview';
    ui.overview = { query: '', sel: hold.selected, hold: hold.id, from: view.current };
    ui.pendingFocus = 'overview-list';
    return;
  }
  if (o && o.hold && (!hold || hold.id !== o.hold)) {
    // Go has switched tabs, which gives the keyboard to the tab's window.
    // Or Go has given up, as when the Mac woke, and Bench stays on the
    // tab: the control or terminal that had the keyboard gets it back, as
    // with Escape. (Go gives an App window back its own.)
    const opener = ui.opener;
    ui.overlay = null;
    ui.overview = null;
    ui.opener = null;
    if (view.current === o.from) restore(opener);
  }
}

// holdShown reports whether the overview is open for Go's hold.
export function holdShown() {
  const { ui } = store;
  return ui.overlay === 'overview' && Boolean(ui.overview && ui.overview.hold);
}

// shownTabs returns the indexes of the tabs whose cards the overview
// shows, in order: the hold's, most recently used first, or the tab bar's,
// leaving out those that don't have every word typed in their name or in a
// window's title or app.
export function shownTabs(view, o) {
  if (o.hold) return ((view.hold && view.hold.tabs) || []).filter((i) => view.tabs[i]);
  const all = view.tabs.map((_, i) => i);
  const words = fold(o.query).split(/\s+/).filter(Boolean);
  if (!words.length) return all;
  return all.filter((i) => {
    const t = view.tabs[i];
    const text = fold([t.name, ...windowsOf(view, t).map((id) => labelOf(view, id))].join('\n'));
    return words.every((w) => text.includes(w));
  });
}

// chosenTab returns the index of the tab chosen: Go's choice during a
// hold, or the overview's own, if its card is shown, or else the first
// card's; or -1 if no card is.
export function chosenTab(view, o) {
  if (o.hold) return view.hold && view.tabs[view.hold.selected] ? view.hold.selected : -1;
  const tabs = shownTabs(view, o);
  return tabs.includes(o.sel) ? o.sel : tabs.length ? tabs[0] : -1;
}

// windowsOf returns the IDs of a tab's windows, pane by pane, leaving out
// those whose window has closed, which are out of sight.
function windowsOf(view, t) {
  return leaves(t.root).flatMap((p) => (p.items || []).filter((id) => {
    const it = itemOf(view, id);
    return it && !it.hidden;
  }));
}

// shownIn returns the ID of the window that a pane shows: its active one,
// or its first that's in sight.
function shownIn(view, p) {
  const ids = (p.items || []).filter((id) => {
    const it = itemOf(view, id);
    return it && !it.hidden;
  });
  return ids.includes(p.active) ? p.active : ids[0] || '';
}

// setQuery filters the cards by what's typed.
export function setQuery(value) {
  const o = store.ui.overview;
  if (!o) return;
  o.query = value;
  update();
}

// moveChoice moves the choice by step cards in the order shown: 1 or -1
// for the next or previous card, or a row's worth for the card below or
// above. It stops at the first and last cards. During a hold, Go moves it,
// wrapping for 1 and -1.
export function moveChoice(step) {
  const { ui, view } = store;
  const o = ui.overview;
  if (!o || !view) return;
  if (o.hold) {
    act('StepHold', step);
    return;
  }
  const tabs = shownTabs(view, o);
  if (!tabs.length) return;
  const at = Math.max(tabs.indexOf(chosenTab(view, o)), 0);
  o.sel = tabs[Math.max(0, Math.min(at + step, tabs.length - 1))];
  update();
}

// rowLength returns how many cards each row of the overview has, as drawn.
export function rowLength() {
  const cards = Array.from(document.querySelectorAll('#overview-list .overview-card'));
  if (!cards.length) return 1;
  const top = cards[0].offsetTop;
  const n = cards.filter((c) => c.offsetTop === top).length;
  return Math.max(n, 1);
}

// holdRow moves the choice of Go's hold to the card below (1) or above
// (-1), for Go's focus_pane_modifiers shortcuts with Down and Up, which
// send hold-down and hold-up: only the frontend knows the rows.
export function holdRow(dir) {
  if (holdShown()) moveChoice(dir * rowLength());
}

// chooseTab switches to a tab, and closes the overview. During a hold, the
// switch ends it.
export function chooseTab(index) {
  const { ui, view } = store;
  if (!view || !view.tabs[index] || ui.overlay !== 'overview') return;
  const o = ui.overview;
  if (o && o.hold) {
    ui.closedHold = o.hold;
    o.hold = 0;
  }
  ui.paneLater = null;
  // The switch gives the keyboard to the tab's window: the control that
  // opened the overview doesn't take it back.
  ui.opener = null;
  closeOverlay();
  switchTab(index);
}

// overviewContextMenu handles a Control-click in the open overview, as a
// click: macOS sends a contextmenu event for it, not a click, and during a
// hold the user is holding Control. On a card, it switches to that tab,
// and on the backdrop, it closes the overview. It reports whether it did.
export function overviewContextMenu(e) {
  if (store.ui.overlay !== 'overview' || !e.ctrlKey || !e.target.closest) return false;
  const card = e.target.closest('.overview-card[data-index]');
  const backdrop = e.target.matches('.overlay[data-action="backdrop"]');
  if (!card && !backdrop) return false;
  e.preventDefault();
  if (card) chooseTab(Number(card.dataset.index));
  else closeOverlay();
  return true;
}

// overviewKeys handles the keys of the open overview, after Escape and
// Tab: the arrow keys move the choice (moveChoice), Return switches to the
// tab chosen, unless the esc button has the focus, and with nothing typed,
// 1 to 9 switch to that tab. In the search field, the arrow keys with
// Command or Option move the caret, as usual.
export function overviewKeys(e) {
  const { ui, view } = store;
  const o = ui.overview;
  if (!o || !view) return;
  const inField = e.target && e.target.id === 'overview-query';
  if (ARROWS[e.key]) {
    if (inField && (e.metaKey || (e.altKey && !e.ctrlKey))) return;
    e.preventDefault();
    const row = e.key === 'ArrowUp' || e.key === 'ArrowDown';
    const back = e.key === 'ArrowLeft' || e.key === 'ArrowUp';
    moveChoice((back ? -1 : 1) * (row ? rowLength() : 1));
    return;
  }
  if (e.key === 'Enter') {
    // Return on the esc button presses it, as on any button: it closes the
    // overview, or during a hold stays on the current tab.
    if (e.target && e.target.closest && e.target.closest('[data-key="overview-close"]')) return;
    e.preventDefault();
    const i = chosenTab(view, o);
    if (i >= 0) chooseTab(i);
    return;
  }
  if (/^[1-9]$/.test(e.key) && !e.metaKey && !e.ctrlKey && (o.hold || !o.query)) {
    e.preventDefault();
    chooseTab(Number(e.key) - 1);
  }
}

const ARROWS = { ArrowLeft: true, ArrowRight: true, ArrowUp: true, ArrowDown: true };

// renderOverview returns the overview's HTML, without its cards
// (renderCards) or the search field's text (fillOverview), so that typing
// doesn't replace the field while it's in use. During a hold, there's no
// field: the list of cards has the keyboard.
export function renderOverview(view, ui) {
  const o = ui.overview;
  if (!o) return '';
  const keys = keysOf(view);
  const title = o.hold ? 'Switch to a tab' : 'All tabs';
  const head = o.hold
    ? '<h2 class="overview-title" id="overview-title">' + esc(title) + '</h2>'
      + '<span class="overview-hint">' + esc(holdHint(keys)) + '</span>'
    : '<h2 class="visually-hidden" id="overview-title">' + esc(title) + '</h2>' + ICONS.searchLarge
      + '<label for="overview-query" class="visually-hidden">Filter the tabs</label>'
      + '<input id="overview-query" data-key="overview-query" type="text" autocomplete="off" spellcheck="false" role="combobox" aria-expanded="true"'
      + ' aria-controls="overview-list" aria-autocomplete="list" placeholder="Filter tabs by name or window">';
  const list = '<div class="overview-list" id="overview-list" role="listbox" aria-label="Tabs" data-scroll="overview"'
    + (o.hold ? ' data-key="overview-list" tabindex="0"' : '') + '></div>';
  const foot = o.hold
    ? '<span><kbd>←→↑↓</kbd> move</span><span><kbd>↵</kbd> switch now</span><span><kbd>esc</kbd> stay here</span>'
    : '<span><kbd>←→↑↓</kbd> move</span><span><kbd>↵</kbd> switch</span><span><kbd>1–9</kbd> tab</span><span><kbd>esc</kbd> close</span>';
  return '<div class="dialog overview" role="dialog" aria-modal="true" aria-labelledby="overview-title"' + (o.hold ? ' data-hold="true"' : '') + '>'
    + '<div class="overview-head palette-search">' + head
    + '<button type="button" class="esc" data-action="close" data-key="overview-close" aria-label="Esc to close" title="Close (esc)">esc</button></div>'
    + list
    + '<div class="palette-foot overview-foot">' + foot + '</div></div>';
}

// holdHint says how to finish a hold: let go of last_tab's modifiers.
function holdHint(keys) {
  const mods = splitShortcut(keys.lastTab || '').mods;
  return mods ? 'Let go of ' + mods + ' to switch' : 'Let go to switch';
}

// fillOverview puts what's typed in the search field, after the overview
// is drawn again.
export function fillOverview(ui) {
  const field = document.getElementById('overview-query');
  if (field && ui.overview && field.value !== ui.overview.query) field.value = ui.overview.query;
}

// syncOverview points the search field, or during a hold the list, at the
// card chosen, and scrolls the card into view once it's chosen.
export function syncOverview(view, ui) {
  const o = ui.overview;
  if (!o) return;
  const el = document.getElementById(o.hold ? 'overview-list' : 'overview-query');
  if (!el) return;
  const i = chosenTab(view, o);
  const id = i >= 0 ? 'overview-card-' + i : '';
  if ((el.getAttribute('aria-activedescendant') || '') === id) return;
  if (!id) {
    el.removeAttribute('aria-activedescendant');
    return;
  }
  el.setAttribute('aria-activedescendant', id);
  const card = document.getElementById(id);
  if (card && typeof card.scrollIntoView === 'function') card.scrollIntoView({ block: 'nearest' });
}

// renderCards returns the overview's cards, or a note that nothing
// matches.
export function renderCards(view, ui) {
  const o = ui.overview;
  if (!o) return '';
  const tabs = shownTabs(view, o);
  if (!tabs.length) {
    const query = o.query.trim();
    return '<p class="palette-empty overview-empty">' + esc(query ? 'No tab matches “' + query + '”.' : 'There are no tabs.') + '</p>';
  }
  const chosen = chosenTab(view, o);
  const area = (view.display && view.display.area) || {};
  const ratio = area.w > 0 && area.h > 0 ? area.w.toFixed(0) + ' / ' + area.h.toFixed(0) : '16 / 10';
  return tabs.map((i) => card(view, i, i === chosen, ratio)).join('');
}

// card returns a tab's card. A screen reader reads its name first, as it
// shows, then what's in it and its marks, with how many of its pages
// aren't loaded yet, which have no counts (decision 76). While a tab has a
// colour, each name has the dot of its tab's colour before it (tabDot).
function card(view, i, chosen, ratio) {
  const t = view.tabs[i];
  const ids = windowsOf(view, t);
  const problems = ids.filter((id) => markerOf(itemOf(view, id)) === 'problem').length;
  const notLoaded = ids.filter((id) => (itemOf(view, id) || {}).notLoaded).length;
  const shown = leaves(t.root).map((p) => shownIn(view, p)).filter(Boolean);
  const notes = [
    i === view.current ? 'current tab' : '',
    i === view.lastTab && i !== view.current ? 'last used' : '',
    ids.length === 1 ? '1 window' : ids.length + ' windows',
    shown.length ? 'showing ' + shown.map((id) => labelOf(view, id)).join(', ') : '',
    unreadText(t.unread).toLowerCase(),
    notLoaded ? (notLoaded === 1 ? '1 page not loaded yet' : notLoaded + ' pages not loaded yet') : '',
    t.attention ? needsYouText(t.attention) : '',
    problems ? (problems === 1 ? '1 window has a problem' : problems + ' windows have a problem') : '',
    tabCaptureText(t),
  ].filter(Boolean);
  const label = t.name + (t.shortcut ? ' ' + t.shortcut : '') + ', ' + notes.join(', ');
  const problemText = problems === 1 ? '1 window has a problem' : problems + ' windows have a problem';
  const marks = unreadMark('overview-unread', t.unread) + tabCaptureMarkOnly(t)
    + (t.attention ? bellMark('overview-bell', needsYouText(t.attention), t.attention) : '')
    + (problems ? '<span class="overview-problem" aria-hidden="true" title="' + esc(problemText) + '">' + ICONS.warning + '</span>' : '');
  return '<div class="overview-card" role="option" id="overview-card-' + i + '" data-action="overview-tab" data-index="' + i + '"'
    + ' aria-selected="' + chosen + '"' + (i === view.current ? ' data-current="true"' : '') + ' aria-label="' + esc(label) + '">'
    + '<div class="overview-card-head">' + (anyColoured(view) ? tabDot(t) : '') + '<span class="overview-name">' + esc(t.name) + '</span>' + marks
    + (i === view.current ? '<span class="overview-current">Current</span>' : '')
    + (t.shortcut ? '<kbd>' + esc(t.shortcut) + '</kbd>' : '') + '</div>'
    + '<div class="mini overview-mini" style="aspect-ratio: ' + ratio + '" aria-hidden="true">' + renderMini(view, t.root, pane) + '</div>'
    + '</div>';
}

// tabCaptureMarkOnly returns a tab's camera or microphone mark, without the
// text for a screen reader, which the card's own label has.
function tabCaptureMarkOnly(t) {
  return tabCaptureMark(t).replace(/<span class="visually-hidden">[\s\S]*?<\/span>$/, '');
}

// pane draws a pane of a card: the window it shows, with its tile, marks,
// and title, then its other windows' tiles, or a note that it's empty.
function pane(view, node) {
  const id = shownIn(view, node);
  if (!id) return '<div class="mini-pane overview-pane"><span class="empty-note">Empty pane</span></div>';
  const it = itemOf(view, id);
  const others = (node.items || []).filter((o) => o !== id && itemOf(view, o) && !itemOf(view, o).hidden);
  const more = others.length > OTHERS ? '<span class="overview-more">+' + (others.length - OTHERS) + '</span>' : '';
  return '<div class="mini-pane overview-pane">'
    + '<div class="overview-shown" title="' + esc(windowTip(view, id)) + '">' + windowTile(it) + captureMarks(it)
    + bellMark('overview-bell', it.attention ? sentence(it.attention) : '') + unreadMark('overview-unread', it.unread)
    + '<span class="overview-window">' + esc(it.title || it.app) + '</span></div>'
    + (others.length ? '<div class="overview-others">' + others.slice(0, OTHERS).map((o) => {
      const other = itemOf(view, o);
      return '<span class="overview-other" title="' + esc(windowTip(view, o)) + '">' + windowTile(other)
        + (other.attention ? bellMark('overview-bell', sentence(other.attention)) : '') + '</span>';
    }).join('') + more + '</div>' : '')
    + '</div>';
}

// windowTile returns a window's tile, with its status's marker, as its
// sub-tab has it.
function windowTile(it) {
  return tile(it.mono, it.icon, statusMark(markerOf(it)));
}

// windowTip names a window, with that its page isn't loaded yet, how many
// unread, how its page uses the camera and the microphone, and why it
// needs you, as its sub-tab's tooltip does.
function windowTip(view, id) {
  const it = itemOf(view, id);
  return [labelOf(view, id), it.notLoaded ? NOT_LOADED : '', unreadTip(it.unread), ...captureNotes(it), it.attention ? sentence(it.attention) : '']
    .filter(Boolean).join('\n');
}
