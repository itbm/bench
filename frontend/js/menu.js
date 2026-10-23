// The ⋯ menu for a window, and the status bar's list of notices.

import { call } from './bridge.js';
import { act, closeMenu, store, toast, update } from './state.js';
import { contentOrigin, esc, isPage, itemOf, keysOf, labelOf, realPaneOf, tabsOf, windowElement } from './util.js';
import { makeWebApp } from './palette.js';

// windowMenuEntries lists what the ⋯ menu offers for an item shown in a pane.
// For a web page, webApp is what Go offers to put a web app of its site in
// its place (WebAppChoice), once it has answered.
export function windowMenuEntries(view, itemId, paneId, webApp = null) {
  const t = view.current;
  const d = view.display || {};
  const entries = [];
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

// openWindowMenu opens the ⋯ menu below a pane's menu button.
export function openWindowMenu(button, itemId, paneId) {
  const view = store.view;
  const win = windowElement();
  const o = contentOrigin();
  const r = button.getBoundingClientRect();
  store.ui.menu = {
    kind: 'window',
    itemId,
    paneId: realPaneOf(view, itemId, paneId),
    shownIn: paneId,
    tab: view.current,
    x: Math.max(8, Math.min(r.right - o.x - 260, win.clientWidth - 330)),
    y: r.bottom - o.y + 4,
    opener: button.dataset.key || null,
    webApp: null,
  };
  store.ui.pendingFocus = 'menu-item-0';
  update();
  if (asksWebApp(itemOf(view, itemId))) askWebAppChoice(store.ui.menu);
}

// openNotices opens the list of notices above the status bar. If it
// replaces a window menu, the click that opened that menu no longer
// focuses its pane (ui.paneLater).
export function openNotices(button) {
  store.ui.paneLater = null;
  const win = windowElement();
  const o = contentOrigin();
  const r = button.getBoundingClientRect();
  store.ui.menu = {
    kind: 'notices',
    x: Math.max(8, Math.min(r.left - o.x, win.clientWidth - 490)),
    bottom: win.clientHeight - (r.top - o.y) + 6,
    opener: button.dataset.key || null,
  };
  store.ui.pendingFocus = 'notices-list';
  update();
}

// runMenuEntry runs the open menu's entry at index and closes the menu.
// The entry does its own focusing, so the click that opened the menu
// doesn't (ui.paneLater).
export function runMenuEntry(index) {
  const entry = store.ui.menuEntries[index];
  if (entry && entry.run && !entry.disabled) store.ui.paneLater = null;
  closeMenu(true);
  if (entry && entry.run && !entry.disabled) entry.run();
}

// renderMenu returns the open menu's HTML.
export function renderMenu(view, ui) {
  const m = ui.menu;
  ui.menuEntries = [];
  if (!m) return '';
  if (m.kind === 'notices') return renderNotices(view, m);
  const item = itemOf(view, m.itemId);
  if (!item) return '';
  const entries = windowMenuEntries(view, m.itemId, m.paneId, m.webApp);
  ui.menuEntries = entries;
  let n = 0;
  // The note says why the disabled entries are disabled.
  const note = entries.findIndex((e) => e.note);
  const noteId = note >= 0 ? 'menu-note-' + note : '';
  const html = entries.map((e, i) => {
    if (e.sep) return '<div class="menu-sep" role="separator"></div>';
    if (e.heading) return '<div class="menu-heading" role="presentation">' + esc(e.heading) + '</div>';
    if (e.note) return '<div class="menu-note" role="none" id="menu-note-' + i + '">' + esc(e.note) + '</div>';
    const key = e.disabled ? 'menu-disabled-' + i : 'menu-item-' + (n++);
    return '<button type="button" class="menu-item" role="menuitem" data-action="menu-run" data-index="' + i + '" data-key="' + key + '"'
      + (e.disabled ? ' disabled' + (noteId ? ' aria-describedby="' + noteId + '"' : '') : '') + '>'
      + '<span class="menu-label' + (e.wrap ? ' wrap' : '') + '">' + esc(e.label) + '</span>' + (e.keys ? '<kbd>' + esc(e.keys) + '</kbd>' : '') + '</button>';
  }).join('');
  // Once fitMenu has placed the menu (m.top), it stays there.
  let style = 'left: ' + m.x + 'px; top: ' + (m.top ?? m.y) + 'px';
  if (m.top != null) style += '; max-height: ' + maxHeightBelow(m.top);
  return '<div class="menu' + (asksWebApp(item) ? ' fixed-width' : '') + '" role="menu" aria-label="Window options" id="window-menu"'
    + ' data-scroll="window-menu" style="' + style + '">' + html + '</div>';
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

// fitMenu places a window menu when it opens: it moves the menu up if it
// would go past the bottom of the window, leaving room below for the entry
// that joins a web page's menu when Go answers (askWebAppChoice). The menu
// then stays where it is (ui.menu.top), so that nothing in it moves when an
// entry joins it, or when the view changes: if it grows past the bottom of
// the window, it scrolls instead.
export function fitMenu() {
  const el = document.querySelector('#menu-root .menu');
  const m = store.ui.menu;
  if (!el || !m || m.kind !== 'window' || m.top != null) return;
  const win = windowElement();
  const top = el.offsetTop;
  const room = !m.webApp && asksWebApp(itemOf(store.view, m.itemId)) ? entryRoom(el) : 0;
  const over = top + el.offsetHeight + room - (win.clientHeight - 8);
  m.top = over > 0 ? Math.max(8, top - over) : top;
  el.style.top = m.top + 'px';
  el.style.maxHeight = maxHeightBelow(m.top);
}

// maxHeightBelow returns the CSS max-height of a window menu whose top is
// at top: the menus' 70% of the window, or the room down to its bottom.
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
