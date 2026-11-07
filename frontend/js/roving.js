// The keyboard in the strips of tabs and sub-tabs, and in the radio groups,
// such as the layout control and Settings' choices (decision 64). Each
// strip and each group is one stop for the Tab key: the tab, sub-tab, or
// choice that it shows, or the one that the arrow keys last moved to while
// the focus is in it. In a strip, Left Arrow and Right Arrow move the
// focus along it, wrapping, and Home and End to its ends, which scrolls
// the strip to keep the focus in view (revealFocus); Return and Space show
// the one with the focus, as a click does. Down Arrow on a tab goes to the
// focused pane's sub-tabs, and Up Arrow on a sub-tab back to the tab that
// Down Arrow came from, or else the tabs' stop. Delete and Backspace close
// the tab or sub-tab with the focus, as its × does, asking first where the
// × would. Shift-F10, Control-Return, and the menu key open its menu
// beside it, as a right-click does. In a radio group, the arrow keys,
// Home, and End choose as they move, as macOS radio buttons do, but in one
// whose choice acts at once on the whole window, such as the layout
// control, which arranges every window, they only move the focus, and
// Return or Space chooses, as in a segmented control on a Mac
// (data-activate="manual"). A list of tabs down the side, as Settings'
// sections (aria-orientation="vertical"), moves with Up Arrow and Down
// Arrow, and in one with data-activate="auto", the tab with the focus is
// shown as the focus moves. Only the strips of tabs and sub-tabs close
// and have menus.

import { store } from './state.js';
import { closeTab, quickClose } from './close.js';
import { openTabMenu, openWindowMenu } from './menu.js';
import { hidePreview } from './preview.js';
import { revealFocus } from './strips.js';
import { contentOrigin } from './util.js';

// GROUPS are the strips and radio groups, and MEMBERS what's in them. A
// tab or sub-tab being renamed is the field that renames it, which isn't
// a member.
const GROUPS = '[role="tablist"], [role="radiogroup"]';
const MEMBERS = '[role="tab"], [role="radio"]';

// lastPlace is where the keyboard last was in a strip: the data-key of the
// tab or sub-tab with the focus, with its strip's data-scroll and its place
// there, so that the focus can stay in the strip when that tab or sub-tab
// closes (keepPlace). It's null once the focus or the pointer moves
// elsewhere, apart from a dialog or menu, such as the question before
// closing.
let lastPlace = null;

// downFrom is the name of the tab that Down Arrow last left for the
// sub-tabs, for Up Arrow to go back to, until the focus goes anywhere but
// the sub-tabs.
let downFrom = null;

// members returns the members of a group that can have the focus.
function members(group) {
  return Array.from(group.querySelectorAll(MEMBERS)).filter((el) => !el.disabled && el.closest(GROUPS) === group);
}

// chosen reports whether a member is the one its group shows or has chosen.
function chosen(el) {
  return el.getAttribute('aria-selected') === 'true' || el.getAttribute('aria-checked') === 'true';
}

// stopOf returns the member of a group that the Tab key stops at.
function stopOf(group) {
  const ms = members(group);
  return ms.find((el) => el.getAttribute('tabindex') === '0') || ms.find(chosen) || ms[0] || null;
}

// syncRoving makes one member of each group a stop for the Tab key: the
// one with the focus, if the group has it, or else the one it shows or has
// chosen, or else its first. The HTML says which is shown (render.js), so
// it runs after each render, as the focus moves, and before that, puts
// back in its strip a focus that a redraw lost with the tab or sub-tab
// that had it (keepPlace).
export function syncRoving() {
  keepPlace();
  const active = document.activeElement;
  for (const group of document.querySelectorAll(GROUPS)) {
    const ms = members(group);
    const stop = ms.includes(active) ? active : ms.find(chosen) || ms[0];
    for (const el of ms) {
      const want = el === stop ? '0' : '-1';
      if (el.getAttribute('tabindex') !== want) el.setAttribute('tabindex', want);
    }
  }
}

// keepPlace moves the focus that a redraw has lost, as Delete closed the
// sub-tab that had it, or its window closed by itself, to the one now in
// its place in the same strip, or the strip's last. A pane left with one
// app's window has no strip (pane_headers): the focus goes to the current
// tab then. It waits while a dialog or menu is open, such as the question
// before closing. A tab's place is its data-key, so a tab that closes
// gives the focus to the current tab as the title bar is drawn
// (followTabKey in render.js).
function keepPlace() {
  const p = lastPlace;
  const active = document.activeElement;
  if (!p || store.ui.overlay || store.ui.menu || (active && active !== document.body)) return;
  if (document.querySelector('[data-key="' + CSS.escape(p.key) + '"]')) return;
  lastPlace = null;
  const strip = document.querySelector('[role="tablist"][data-scroll="' + CSS.escape(p.scroll) + '"]');
  const ms = strip ? members(strip) : [];
  const tabs = document.querySelector('#titlebar .tabs');
  const el = ms.length ? ms[Math.min(p.index, ms.length - 1)] : tabs && stopOf(tabs);
  if (el) move(el);
}

// move gives the focus to a member, scrolling its strip to keep it in view.
function move(el) {
  el.focus({ preventScroll: true });
  revealFocus(el);
}

// onFocusIn keeps the stops in step with the focus, and notes where the
// keyboard is in a strip. A click's focus isn't noted: only the keyboard's
// shows a focus ring (:focus-visible).
function onFocusIn(e) {
  const el = e.target;
  syncRoving();
  if (!(el instanceof Element)) return;
  if (!el.matches('.subtabs [role="tab"]')) downFrom = null;
  if (el.matches('[role="tab"]') && el.matches(':focus-visible')) {
    const strip = el.closest('[role="tablist"][data-scroll]');
    lastPlace = strip ? { key: el.dataset.key, scroll: strip.dataset.scroll, index: members(strip).indexOf(el) } : null;
  } else if (!el.closest('#overlay-root, #menu-root') && !el.matches('[role="tab"]')) {
    lastPlace = null;
  }
}

// onPointerDown forgets where the keyboard was, as the pointer moves on,
// unless it's in a dialog or menu, such as the question before closing.
function onPointerDown(e) {
  if (!(e.target instanceof Element) || !e.target.closest('#overlay-root, #menu-root')) lastPlace = null;
}

// menuKey reports whether a key event opens the menu of what has the
// focus: Shift-F10, Control-Return, or the menu key, as elsewhere.
function menuKey(e) {
  const mods = (e.ctrlKey ? 'c' : '') + (e.altKey ? 'a' : '') + (e.shiftKey ? 's' : '') + (e.metaKey ? 'm' : '');
  return (e.key === 'F10' && mods === 's') || (e.key === 'Enter' && mods === 'c') || (e.key === 'ContextMenu' && !mods);
}

// openMenuBeside opens the menu of a tab or sub-tab beside it, as a
// right-click on it does, with the focus in the menu. Closing the menu
// gives the focus back to the tab or sub-tab.
function openMenuBeside(el) {
  hidePreview();
  if (el.matches('.tab[data-index]')) {
    openTabMenu(el, Number(el.dataset.index));
    return;
  }
  const { pane, item } = el.dataset;
  const button = document.querySelector('[data-action="menu"][data-pane="' + CSS.escape(pane) + '"]');
  if (!button) return;
  const o = contentOrigin();
  const r = el.getBoundingClientRect();
  openWindowMenu(button, item, pane, { x: r.left - o.x, y: r.bottom - o.y + 4 }, el.dataset.key || null);
}

// tabBack returns the tab that Up Arrow on a sub-tab goes to: the one that
// Down Arrow came from, if it's still there, or else the tabs' stop.
function tabBack() {
  const tabs = document.querySelector('#titlebar .tabs');
  if (!tabs) return null;
  const i = downFrom === null ? -1 : ((store.view && store.view.tabs) || []).findIndex((t) => t.name === downFrom);
  const back = i < 0 ? null : tabs.querySelector('[role="tab"][data-index="' + i + '"]');
  return back || stopOf(tabs);
}

// manual reports whether a radio group chooses with Return or Space only.
function manual(group) {
  return group.getAttribute('data-activate') === 'manual';
}

// paneStrip returns the strip of the focused pane's sub-tabs, or of the
// first pane with some, or null.
function paneStrip() {
  const d = (store.view && store.view.display) || {};
  const id = d.compact ? 'compact' : d.focus;
  const own = id ? document.querySelector('.subtabs[data-scroll="subtabs-' + CSS.escape(id) + '"]') : null;
  return own && members(own).length ? own : document.querySelector('#titlebar .subtabs, #panes .subtabs');
}

// rovingKeys handles a key on a tab, sub-tab, or radio button, and reports
// whether it did.
export function rovingKeys(e) {
  const el = e.target;
  if (!(el instanceof Element) || !el.matches(MEMBERS) || store.ui.drag) return false;
  const group = el.closest(GROUPS);
  if (!group) return false;
  const tab = el.matches('[role="tab"]');
  // strip is set for the title bar's tabs and a pane's sub-tabs, and
  // vertical for a list of tabs down the side.
  const strip = tab && group.matches('.tabs, .subtabs');
  const vertical = group.getAttribute('aria-orientation') === 'vertical';
  if (menuKey(e)) {
    if (!strip) return false;
    e.preventDefault();
    openMenuBeside(el);
    return true;
  }
  if (e.ctrlKey || e.altKey || e.metaKey || e.shiftKey) return false;
  const ms = members(group);
  const i = ms.indexOf(el);
  const n = ms.length;
  let next = null;
  const back = vertical ? e.key === 'ArrowUp' : e.key === 'ArrowLeft' || (!tab && e.key === 'ArrowUp');
  const on = vertical ? e.key === 'ArrowDown' : e.key === 'ArrowRight' || (!tab && e.key === 'ArrowDown');
  if (back) next = ms[(i - 1 + n) % n];
  else if (on) next = ms[(i + 1) % n];
  else if (e.key === 'Home') next = ms[0];
  else if (e.key === 'End') next = ms[n - 1];
  else if (tab && e.key === 'ArrowDown' && group.matches('#titlebar .tabs')) {
    const strip = paneStrip();
    next = strip ? stopOf(strip) : null;
    if (!next) return false;
    const t = ((store.view && store.view.tabs) || [])[Number(el.dataset.index)];
    e.preventDefault();
    move(next);
    downFrom = t ? t.name : null;
    return true;
  } else if (tab && e.key === 'ArrowUp' && group.matches('.subtabs')) {
    next = tabBack();
    if (!next) return false;
  } else if (strip && (e.key === 'Delete' || e.key === 'Backspace')) {
    e.preventDefault();
    if (el.matches('.tab[data-index]')) closeTab(Number(el.dataset.index));
    else quickClose(el.dataset.item);
    return true;
  } else {
    return false;
  }
  e.preventDefault();
  if (next && next !== el) {
    move(next);
    if (tab ? group.getAttribute('data-activate') === 'auto' : !manual(group)) next.click();
  }
  return true;
}

// installRoving keeps the stops in step with the focus.
export function installRoving() {
  document.addEventListener('focusin', onFocusIn);
  document.addEventListener('pointerdown', onPointerDown, true);
}
