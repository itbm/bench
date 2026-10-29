// The ⋯ menu for a window, a tab's menu, an empty pane's menu, and the
// status bar's lists of the windows that need attention and of notices.

import { call } from './bridge.js';
import { act, closeMenu, store, toast, update } from './state.js';
import {
  attentionOf, contentOrigin, esc, isPage, itemOf, keysOf, labelOf, markerOf, realPaneOf, sentence, statusMark, statusText,
  tabsOf, windowElement,
} from './util.js';
import { makeWebApp, openAdd } from './palette.js';
import { openEditor } from './editor.js';

// windowMenuEntries lists what the ⋯ menu offers for an item shown in a pane.
// For a web page, webApp is what Go offers to put a web app of its site in
// its place (WebAppChoice), once it has answered. For an item that wasn't
// ready when the menu opened, status is why (statusLine): the menu's
// first line says so.
export function windowMenuEntries(view, itemId, paneId, webApp = null, status = null) {
  const t = view.current;
  const d = view.display || {};
  const entries = [];
  if (status) entries.push({ status: status.text, marker: status.marker }, { sep: true });
  // The keyboard moves the shown window with the move modifiers and an
  // arrow, splitting the pane where there's no neighbour.
  const move = keysOf(view).moveWindow;
  const right = move ? move + '→' : '';
  const down = move ? move + '↓' : '';
  if (d.splitsDisabled) {
    entries.push({ label: 'Show side by side (split right)', keys: right, disabled: true });
    entries.push({ label: 'Split down', keys: down, disabled: true });
    entries.push({ note: d.splitsDisabled });
  } else {
    entries.push({ label: 'Show side by side (split right)', keys: right, run: () => act('MoveItem', itemId, t, paneId, 'right') });
    entries.push({ label: 'Split down', keys: down, run: () => act('MoveItem', itemId, t, paneId, 'bottom') });
  }
  const others = (d.panes || []).filter((p) => p.id !== paneId && p.id !== 'compact');
  if (others.length) {
    entries.push({ sep: true }, { heading: 'Move to pane' });
    others.forEach((p) => entries.push({
      label: p.active ? 'With ' + labelOf(view, p.active) : 'Empty pane',
      run: () => act('MoveItem', itemId, t, p.id, 'center'),
    }));
  }
  const item = itemOf(view, itemId);
  const tabs = tabsOf(item);
  if ((view.tabs || []).length > 1) {
    entries.push({ sep: true }, { heading: 'Move to tab' });
    view.tabs.forEach((other, i) => {
      if (i !== t) entries.push({ label: other.name, run: () => act('MoveItemToTab', itemId, i) });
    });
    // The same window can be in several tabs: it's shown in whichever is
    // current.
    const more = view.tabs.map((_, i) => i).filter((i) => i !== t && !tabs.includes(i));
    if (more.length) {
      entries.push({ sep: true });
      more.forEach((i) => entries.push({ label: 'Also show in ' + view.tabs[i].name, run: () => act('AddItemToTab', itemId, i) }));
    }
  }
  // The pane that shows the item remembers the places of its windows that
  // have closed, out of sight: their apps can open again, or the pane can
  // forget them.
  const shown = (d.panes || []).find((p) => (p.items || []).includes(itemId));
  const waiting = (shown && shown.remembered) || [];
  if (waiting.length) {
    entries.push({ sep: true }, { heading: 'Closed windows' });
    waiting.forEach((r) => {
      if (r.canOpen) entries.push({ label: 'Open ' + r.app, run: () => act('OpenItem', r.itemId) });
      entries.push({ label: 'Remove ' + r.app + ' from this pane', run: () => act('RemoveRemembered', shown.id, r.bundleId) });
    });
  }
  entries.push({ sep: true });
  if (tabs.length > 1) entries.push({ label: 'Remove from this tab', run: () => act('RemoveItemFromTab', itemId, t) });
  if (item && item.status === 'undocked') entries.push({ label: 'Dock again', run: () => act('Redock', itemId) });
  // A web page opens in the default browser, where its address can be
  // checked and the page used with the browser's own sign-ins.
  if (isPage(item)) entries.push({ label: 'Open in browser', run: () => act('OpenInBrowser', itemId) });
  // A sub-tab with no window has nothing to pop out, so Go forgets it.
  entries.push({ label: item && item.noWindow ? 'Remove from Bench' : 'Pop out of Bench', run: () => act('PopOut', itemId) });
  // The web app entry's label wraps, rather than hide the browser's name.
  if (webApp && webApp.offer === 'use') {
    entries.push({ sep: true }, {
      label: 'Use the ' + webApp.app + ' web app from ' + webApp.browser, wrap: true, run: () => useWebApp(view, itemId, webApp),
    });
  } else if (webApp && webApp.offer === 'make') {
    entries.push({ sep: true }, { label: 'Make a web app in ' + webApp.browser + '…', wrap: true, run: () => makeWebApp(itemId, paneId) });
  }
  return entries;
}

// useWebApp puts the web app that the menu offered in a web page's place,
// and says so.
async function useWebApp(view, itemId, webApp) {
  const item = itemOf(view, itemId);
  const page = item ? item.app : 'the page';
  const r = await act('UseWebApp', itemId);
  if (r.ok) toast('Replaced ' + page + ' with the ' + webApp.app + ' web app.');
}

// asksWebApp reports whether an item's menu asks Go what it can offer to put
// a web app in the item's place: the menu of a Web item.
const asksWebApp = (item) => Boolean(item) && item.kind === 'web';

// askWebAppChoice asks Go what a web page's menu can offer to put a web app
// of its site in its place. Go scans the installed apps to answer, which
// can take a moment, so the menu opens at once, and the entry joins it, at
// the end, when the answer comes: the entries above it don't move, so a
// click or an arrow key on its way still lands where it was meant. For that,
// the menu opens as wide as menus get, and with room below it for the entry
// (see fitMenu). An answer for a menu that has closed meanwhile is dropped,
// and so is a refusal, which means the page has gone.
function askWebAppChoice(menu) {
  call('WebAppChoice', menu.itemId).then((choice) => {
    if (store.ui.menu !== menu || !choice || !choice.offer) return;
    menu.webApp = choice;
    update();
  }, () => {});
}

// statusLine returns why an item isn't ready, with its marker, for the
// first line of its menu, or null for an item that's ready.
function statusLine(item) {
  const marker = markerOf(item);
  return marker ? { text: sentence(statusText(item)), marker } : null;
}

// openWindowMenu opens the ⋯ menu below a pane's menu button, or, for a
// right-click on a sub-tab or the pane's header, at the pointer (at, in
// content coordinates). Its first line says why the window isn't ready, as
// it was when the menu opened, so that the entries below don't move if that
// changes.
export function openWindowMenu(button, itemId, paneId, at = null) {
  const view = store.view;
  const o = contentOrigin();
  const r = button.getBoundingClientRect();
  store.ui.menu = {
    kind: 'window',
    itemId,
    paneId: realPaneOf(view, itemId, paneId),
    shownIn: paneId,
    tab: view.current,
    x: menuX(at ? at.x : r.right - o.x - 260),
    y: at ? at.y : r.bottom - o.y + 4,
    opener: button.dataset.key || null,
    webApp: null,
    status: statusLine(itemOf(view, itemId)),
  };
  store.ui.pendingFocus = 'menu-item-0';
  update();
  if (asksWebApp(itemOf(view, itemId))) askWebAppChoice(store.ui.menu);
}

// menuX keeps a window or tab menu that starts at x inside the window
// across: menus are at most 320 points wide. fitMenu keeps it inside the
// window down.
function menuX(x) {
  return Math.max(8, Math.min(x, windowElement().clientWidth - 330));
}

// tabMenuEntries lists what tab index's menu offers, in groups: the tab's
// own properties, which, unlike Settings, apply to it alone; arranging it;
// and adding a tab. The entries that open a dialog do so in the menu's
// place (opens).
export function tabMenuEntries(view, index) {
  const current = index === view.current;
  return withSeparators([
    [
      { label: 'Tab properties…', opens: true, run: () => openEditor(index) },
      // Tab properties opens with the name selected, to type a new one.
      { label: 'Rename…', opens: true, run: () => openEditor(index) },
    ],
    [
      { label: 'Rearrange', keys: current ? keysOf(view).rearrange || '' : '', run: () => rearrangeTab(index) },
      // As in Tab properties, Reset layout doesn't ask first.
      { label: 'Reset layout', run: () => act('ResetLayout', index) },
    ],
    [{ label: 'New tab…', opens: true, run: () => openEditor(-1) }],
  ]);
}

// paneMenuEntries lists what an empty pane's menu offers: what the buttons
// in its body do. A pane closes only in a tab that shows more than one.
// Adding to it opens the add dialog for addTo, the pane that the add dialog
// adds to (the focused pane, for the "tabs" view's compact pane).
export function paneMenuEntries(view, paneId, addTo) {
  const entries = [{ label: 'Add a window or app…', run: () => openAdd(addTo) }];
  if (((view.display && view.display.panes) || []).length > 1) {
    entries.push({ label: 'Close this pane', run: () => act('ClosePane', paneId) });
  }
  return entries;
}

// withSeparators joins groups of menu entries, with a separator between
// each group and the next, leaving out empty groups.
function withSeparators(groups) {
  return groups.filter((g) => g.length).flatMap((g, i) => (i ? [{ sep: true }, ...g] : g));
}

// rearrangeTab arranges tab index again. Go arranges only the current tab,
// so another tab is switched to first.
async function rearrangeTab(index) {
  if (index !== store.view.current) {
    const r = await act('SwitchTab', index);
    if (!r.ok) return;
  }
  act('Rearrange');
}

// openTabMenu opens a tab's menu, for a right-click (or Control-click) on
// the tab in the tab strip: at the pointer (at, in content coordinates), or
// below the tab without one. The tab isn't switched to: the menu is about
// that tab, whichever is current. If it replaces a window menu, the click
// that opened that menu no longer focuses its pane (ui.paneLater).
export function openTabMenu(tabEl, index, at = null) {
  const view = store.view;
  const tab = view.tabs[index];
  if (!tab) return;
  store.ui.paneLater = null;
  const o = contentOrigin();
  const r = tabEl.getBoundingClientRect();
  store.ui.menu = {
    kind: 'tab',
    index,
    // tabName is the tab's name when the menu opened, to find it by
    // (followTab).
    tabName: tab.name,
    x: menuX(at ? at.x : r.left - o.x),
    y: at ? at.y : r.bottom - o.y + 4,
    opener: tabEl.dataset.key || null,
  };
  store.ui.pendingFocus = 'menu-item-0';
  update();
}

// openNotices opens the list of notices above the status bar.
export function openNotices(button) {
  openAbove(button, { kind: 'notices' }, 'notices-list');
}

// attentionKey names the windows of the current tab that need attention,
// to tell when they change.
export function attentionKey(view) {
  return attentionOf(view).map((w) => w.paneId + '/' + w.itemId).join(' ');
}

// openAttention opens the list of the windows that need attention above the
// status bar: each says why, and shows its window.
export function openAttention(button) {
  const view = store.view;
  openAbove(button, { kind: 'attention', tab: view.current, key: attentionKey(view) }, 'menu-item-0');
}

// openAbove opens one of the status bar's lists above the button that
// opens it, and focuses the control with data-key focus. If it replaces a
// window menu, the click that opened that menu no longer focuses its pane
// (ui.paneLater).
function openAbove(button, menu, focus) {
  store.ui.paneLater = null;
  const win = windowElement();
  const o = contentOrigin();
  const r = button.getBoundingClientRect();
  store.ui.menu = {
    ...menu,
    x: Math.max(8, Math.min(r.left - o.x, win.clientWidth - 490)),
    bottom: win.clientHeight - (r.top - o.y) + 6,
    opener: button.dataset.key || null,
  };
  store.ui.pendingFocus = focus;
  update();
}

// attentionEntries lists the windows that need attention, each with why:
// choosing one shows it.
function attentionEntries(view) {
  return attentionOf(view).map(({ itemId, paneId }) => {
    const item = itemOf(view, itemId);
    return {
      label: labelOf(view, itemId),
      detail: sentence(statusText(item)),
      marker: markerOf(item),
      run: () => act('ShowItem', paneId, itemId),
    };
  });
}

// runMenuEntry runs the open menu's entry at index and closes the menu.
// The entry does its own focusing, so the click that opened the menu
// doesn't (ui.paneLater). An entry that opens a dialog (opens) replaces the
// menu with it, and the dialog gives the focus back to the menu's opener
// when it closes (openerKey): closing the menu first would draw Bench with
// neither open, which would lower it (syncOverlay).
export function runMenuEntry(index) {
  const { ui } = store;
  const entry = ui.menuEntries[index];
  const runs = Boolean(entry && entry.run && !entry.disabled);
  if (runs) ui.paneLater = null;
  if (runs && entry.opens) {
    entry.run();
    closeMenu(true);
    return;
  }
  closeMenu(true);
  if (runs) entry.run();
}

// openPaneMenu opens an empty pane's menu, for a right-click on its header
// or body: at the pointer (at, in content coordinates), or below the
// header (el) without one.
export function openPaneMenu(el, paneId, addTo, at = null) {
  const view = store.view;
  store.ui.paneLater = null;
  const o = contentOrigin();
  const r = el.getBoundingClientRect();
  store.ui.menu = {
    kind: 'pane',
    paneId,
    addTo,
    tab: view.current,
    x: menuX(at ? at.x : r.left - o.x + 8),
    y: at ? at.y : r.bottom - o.y + 4,
    opener: null,
  };
  store.ui.pendingFocus = 'menu-item-0';
  update();
}

// renderMenu returns the open menu's HTML.
export function renderMenu(view, ui) {
  const m = ui.menu;
  ui.menuEntries = [];
  if (!m) return '';
  if (m.kind === 'notices') return renderNotices(view, m);
  if (m.kind === 'attention') {
    const entries = attentionEntries(view);
    if (!entries.length) return '';
    ui.menuEntries = entries;
    return '<div class="menu wide" role="menu" aria-label="Windows that need attention" id="attention-menu" data-scroll="attention-menu"'
      + ' style="left: ' + m.x + 'px; bottom: ' + m.bottom + 'px">'
      + '<div class="menu-heading" role="presentation">Need attention</div>' + renderEntries(entries) + '</div>';
  }
  if (m.kind === 'tab') {
    const tab = view.tabs[m.index];
    if (!tab) return '';
    const entries = tabMenuEntries(view, m.index);
    ui.menuEntries = entries;
    return '<div class="menu" role="menu" aria-label="' + esc('Options for the ' + tab.name + ' tab') + '" id="tab-menu"'
      + ' data-scroll="tab-menu" style="' + menuStyle(m) + '">' + renderEntries(entries) + '</div>';
  }
  if (m.kind === 'pane') {
    const entries = paneMenuEntries(view, m.paneId, m.addTo);
    ui.menuEntries = entries;
    return '<div class="menu" role="menu" aria-label="Pane options" id="pane-menu"'
      + ' data-scroll="pane-menu" style="' + menuStyle(m) + '">' + renderEntries(entries) + '</div>';
  }
  const item = itemOf(view, m.itemId);
  if (!item) return '';
  const entries = windowMenuEntries(view, m.itemId, m.paneId, m.webApp, m.status);
  ui.menuEntries = entries;
  // The status line, which isn't an entry, describes the menu.
  const described = m.status ? ' aria-describedby="menu-status"' : '';
  return '<div class="menu' + (asksWebApp(item) ? ' fixed-width' : '') + '" role="menu" aria-label="Window options" id="window-menu"' + described
    + ' data-scroll="window-menu" style="' + menuStyle(m) + '">' + renderEntries(entries) + '</div>';
}

// renderEntries returns the HTML of a menu's entries. An entry can have a
// marker before its label, and a detail on a line below it. A status line
// says why a window isn't ready, as a disabled line with its marker.
function renderEntries(entries) {
  let n = 0;
  // The note says why the disabled entries are disabled.
  const note = entries.findIndex((e) => e.note);
  const noteId = note >= 0 ? 'menu-note-' + note : '';
  return entries.map((e, i) => {
    if (e.sep) return '<div class="menu-sep" role="separator"></div>';
    if (e.heading) return '<div class="menu-heading" role="presentation">' + esc(e.heading) + '</div>';
    if (e.note) return '<div class="menu-note" role="none" id="menu-note-' + i + '">' + esc(e.note) + '</div>';
    if (e.status) return '<div class="menu-status" role="none" id="menu-status">' + statusMark(e.marker) + '<span>' + esc(e.status) + '</span></div>';
    const key = e.disabled ? 'menu-disabled-' + i : 'menu-item-' + (n++);
    const label = e.detail
      ? '<span class="menu-label stacked"><span class="menu-main">' + esc(e.label) + '</span><span class="menu-detail">' + esc(e.detail) + '</span></span>'
      : '<span class="menu-label' + (e.wrap ? ' wrap' : '') + '">' + esc(e.label) + '</span>';
    return '<button type="button" class="menu-item" role="menuitem" data-action="menu-run" data-index="' + i + '" data-key="' + key + '"'
      + (e.detail ? ' aria-label="' + esc(e.label + ': ' + e.detail) + '"' : '')
      + (e.disabled ? ' disabled' + (noteId ? ' aria-describedby="' + noteId + '"' : '') : '') + '>'
      + statusMark(e.marker) + label + (e.keys ? '<kbd>' + esc(e.keys) + '</kbd>' : '') + '</button>';
  }).join('');
}

// menuStyle places a window, tab, or pane menu. Once fitMenu has placed it
// (m.top), it stays there.
function menuStyle(m) {
  let style = 'left: ' + m.x + 'px; top: ' + (m.top ?? m.y) + 'px';
  if (m.top != null) style += '; max-height: ' + maxHeightBelow(m.top);
  return style;
}

function renderNotices(view, m) {
  const notices = view.notices || [];
  if (!notices.length) return '';
  return '<div class="menu wide" role="dialog" aria-label="Notices" id="notices-menu" style="left: ' + m.x + 'px; bottom: ' + m.bottom + 'px">'
    + '<div class="menu-heading" role="presentation">Notices</div>'
    + '<div tabindex="-1" data-key="notices-list" role="list" aria-label="Notices">'
    + notices.map((n) => '<div class="menu-text" role="listitem">' + esc(n) + '</div>').join('')
    + '</div></div>';
}

// fitMenu places a window, tab, or pane menu when it opens: it moves the
// menu up if it would go past the bottom of the window, leaving room below
// for the entry that joins a web page's menu when Go answers
// (askWebAppChoice). The menu then stays where it is (ui.menu.top), so that
// nothing in it moves when an entry joins it, or when the view changes: if
// it grows past the bottom of the window, it scrolls instead.
export function fitMenu() {
  const el = document.querySelector('#menu-root .menu');
  const m = store.ui.menu;
  if (!el || !m || !['window', 'tab', 'pane'].includes(m.kind) || m.top != null) return;
  const win = windowElement();
  const top = el.offsetTop;
  const room = m.kind === 'window' && !m.webApp && asksWebApp(itemOf(store.view, m.itemId)) ? entryRoom(el) : 0;
  const over = top + el.offsetHeight + room - (win.clientHeight - 8);
  m.top = over > 0 ? Math.max(8, top - over) : top;
  el.style.top = m.top + 'px';
  el.style.maxHeight = maxHeightBelow(m.top);
}

// maxHeightBelow returns the CSS max-height of a window or tab menu whose
// top is at top: the menus' 70% of the window, or the room down to its bottom.
function maxHeightBelow(top) {
  return 'min(70%, ' + Math.max(0, windowElement().clientHeight - 8 - top) + 'px)';
}

// entryRoom returns the height that a separator and an entry add to a menu.
function entryRoom(el) {
  const sep = el.querySelector('.menu-sep');
  const item = el.querySelector('.menu-item');
  if (!sep || !item) return 0;
  const s = getComputedStyle(sep);
  const height = (n) => n.getBoundingClientRect().height;
  return Math.ceil(height(sep) + parseFloat(s.marginTop) + parseFloat(s.marginBottom) + height(item));
}
