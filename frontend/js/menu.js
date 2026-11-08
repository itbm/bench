// The ⋯ menu for a window, a tab's menu, an empty pane's menu, a terminal's
// right-click menu, a slim title bar's layout menu, and the status bar's
// lists of the windows that need attention and of notices and recent
// messages, which are in the title bar while the status bar is hidden.

import { call, setClipboardText } from './bridge.js';
import {
  act, canRestore, closeMenu, isDemo, recentMessages, runCommand, runPaneLater, shownCurrent, store, switchTab, toast, update,
} from './state.js';
import {
  activeKey, attentionOf, contentOrigin, currentTab, esc, focusKey, ICONS, isPage, itemOf, keysOf, labelOf, LAYOUT_MODES, layoutText, leaves,
  markerOf, realPaneOf, sentence, statusMark, statusText, tabsOf, termKey, undoHint, windowElement,
} from './util.js';
import { makeWebApp, openAdd, splitEmpty } from './palette.js';
import { followTab, openEditor } from './editor.js';
import { closeItem, closeLabel, closeTab, closeTabLabel } from './close.js';
import { canRenameItem, startItemRename, startTabRename, useAutomaticTitle } from './rename.js';
import { pageEntries, reloadEntry } from './pages.js';
import * as terminals from './terminals.js';

// SIDE_BY_SIDE_WIDE is Show Side by Side in the "tabs" view, which changes
// to the wide layout to show the tab's panes (ShowSideBySide). Pane ▸ Split
// Right reads the same there.
export const SIDE_BY_SIDE_WIDE = 'Show Side by Side (Wide Layout)';

// windowMenuEntries lists what the ⋯ menu offers for an item shown in a
// pane, in groups. Its entries have title case, and the names that the
// menu bar's Pane menu gives the same commands. The common actions come
// first, where the menu opens: Reload Page, for a web page, Rename…, for a
// terminal or web page, and closing it, last in the group, which the
// focus doesn't start on (openWindowMenu). Then splitting and zooming,
// moving the sub-tab along its strip, the Move to Pane, Move to Tab, and
// Also Show In submenus, which list the panes and the tabs, the places of
// closed windows, a Web item's page controls (pageEntries), and the rest.
// Each entry has an id, unique in the menu, which a native menu reports
// when it's chosen (showNative). For a web page, webApp is what Go offers
// to put a web app of its site in its place (WebAppChoice), once it has
// answered. For an item that wasn't ready when the menu opened, status is
// why (statusLine): the menu's first line says so. The jump palette's This
// window lists the same entries (windowRows in palette.js): an entry in a
// submenu, or with under, is named with it, and Zoom Pane, which is the
// pane's, says so (pane).
export function windowMenuEntries(view, itemId, paneId, webApp = null, status = null) {
  const t = view.current;
  const d = view.display || {};
  const item = itemOf(view, itemId);
  const shown = (d.panes || []).find((p) => (p.items || []).includes(itemId));
  const common = [];
  // A web page's Reload Page, as in Pane ▸ Page (pages.js).
  if (item && item.kind === 'web') common.push(reloadEntry(item));
  // A terminal or web page can have a name of its own, which its sub-tab
  // shows in place of the title its shell or page sets, typed in place of
  // that title. An app's window takes its title from the app.
  if (shown && canRenameItem(view, itemId, shown.id)) {
    common.push({ id: 'rename', label: 'Rename…', run: () => startItemRename(itemId, shown.id) });
    if (item.label) common.push({ id: 'use-automatic-title', label: 'Use Automatic Title', detail: item.autoTitle || '', run: () => useAutomaticTitle(itemId) });
  }
  common.push(...closeEntries(view, item, t));
  const entries = status ? [{ status: status.text, marker: status.marker }, { sep: true }] : [];
  entries.push(...withSeparators([
    common, splitEntries(view, itemId, paneId), alongEntries(itemId, t, shown),
    [...paneEntries(view, itemId, paneId), ...tabEntries(view, item)], closedEntries(shown), dropdownEntries(view, paneId), pageEntries(item),
    moreEntries(item, paneId),
  ]));
  // The web app entry's label wraps, rather than hide the browser's name.
  if (webApp && webApp.offer === 'use') {
    entries.push({ sep: true }, {
      id: 'use-web-app', label: 'Use the ' + webApp.app + ' Web App from ' + webApp.browser, wrap: true, run: () => useWebApp(view, itemId, webApp),
    });
  } else if (webApp && webApp.offer === 'make') {
    entries.push({ sep: true }, { id: 'make-web-app', label: 'Make a Web App in ' + webApp.browser + '…', wrap: true, run: () => makeWebApp(itemId, paneId) });
  }
  return entries;
}

// splitEntries lists the window menu's splits and Zoom Pane. A split with
// an empty pane opens the add dialog for it at once. The "tabs" view hides
// the tab's splits, so it offers one thing in their place: showing the
// window side by side in the wide layout, which Go changes to, saying how
// to go back. The move keys' → does it there too, and the keyboard moves
// the shown window with the move modifiers and an arrow, splitting the
// pane where there's no neighbour.
function splitEntries(view, itemId, paneId) {
  const t = view.current;
  const d = view.display || {};
  const move = keysOf(view).moveWindow;
  const right = move ? move + '→' : '';
  const down = move ? move + '↓' : '';
  const entries = [];
  if (d.splitsDisabled) {
    entries.push({ id: 'side-by-side', label: SIDE_BY_SIDE_WIDE, keys: right, run: () => act('ShowSideBySide', itemId) });
  } else {
    entries.push({ id: 'split-right', label: 'Split Right', keys: right, run: () => act('MoveItem', itemId, t, paneId, 'right') });
    entries.push({ id: 'split-down', label: 'Split Down', keys: down, run: () => act('MoveItem', itemId, t, paneId, 'bottom') });
    entries.push({ id: 'split-right-empty', label: 'Split Right with an Empty Pane', run: () => splitEmpty(paneId, 'right') });
    entries.push({ id: 'split-down-empty', label: 'Split Down with an Empty Pane', run: () => splitEmpty(paneId, 'bottom') });
  }
  // Zoom Pane shows this pane alone for a while, and goes with the splits,
  // which change how the tab's panes are shown too.
  const zoom = zoomEntry(view, paneId);
  if (zoom) entries.push({ id: 'zoom-pane', label: zoom.zoomed ? 'Show All Panes' : 'Zoom Pane', keys: zoom.keys, pane: true, run: zoom.run });
  return entries;
}

// alongEntries lists Move Left and Move Right, which move the sub-tab along
// the strip it's drawn in, as a drag along it does: the "tabs" view's strip
// holds the whole tab.
function alongEntries(itemId, t, shown) {
  const strip = shown ? shown.items : [];
  const at = strip.indexOf(itemId);
  if (strip.length < 2) return [];
  return [
    at > 0 ? { id: 'move-left', label: 'Move Left', run: () => act('MoveItemAt', itemId, t, shown.id, at - 1) } : { id: 'move-left', label: 'Move Left', disabled: true },
    at < strip.length - 1
      ? { id: 'move-right', label: 'Move Right', run: () => act('MoveItemAt', itemId, t, shown.id, at + 2) }
      : { id: 'move-right', label: 'Move Right', disabled: true },
  ];
}

// paneEntries lists the Move to Pane submenu: the panes that the window
// can move to, each named by what it shows. It's a submenu, as Move to Tab
// is, so that a native menu, which has no headings, names the move too.
// While a pane is zoomed, the tab's other panes are out of sight, but
// still there to move to.
function paneEntries(view, itemId, paneId) {
  const d = view.display || {};
  const tab = currentTab(view);
  const panes = d.zoomed && tab ? leaves(tab.root) : (d.panes || []);
  const others = panes.filter((p) => p.id !== paneId && p.id !== 'compact');
  if (!others.length) return [];
  return [{
    id: 'move-to-pane',
    label: 'Move to Pane',
    submenu: others.map((p) => ({
      id: 'move-to-pane:' + p.id,
      label: p.active ? 'Pane with ' + labelOf(view, p.active) : 'Empty Pane',
      run: () => act('MoveItem', itemId, view.current, p.id, 'center'),
    })),
  }];
}

// tabEntries lists the Move to Tab and Also Show In submenus. Each lists
// every tab, as Pane ▸ Move to Tab does, with the tabs that the window is in
// already off, and then New Tab. The same window can be in several tabs:
// it's shown in whichever is current. The first nine tabs have the send
// modifiers with their number, which move the focused pane's shown window
// there too: Go says which, as a key that another shortcut has goes
// without. A new tab of the window's own is named after its app, as a drop
// of its sub-tab on the tabs' + makes.
function tabEntries(view, item) {
  if (!item) return [];
  const t = view.current;
  const tabs = tabsOf(item);
  const moveTo = (view.tabs || []).map((other, i) => (i === t
    ? { id: 'move-to-tab:' + i, label: other.name, keys: other.sendShortcut || '', disabled: true }
    : { id: 'move-to-tab:' + i, label: other.name, keys: other.sendShortcut || '', run: () => act('MoveItemToTab', item.id, i) }));
  moveTo.push({ sep: true }, { id: 'move-to-tab:new', label: 'New Tab', run: () => act('MoveItemToNewTab', item.id, false) });
  const also = (view.tabs || []).map((other, i) => (i === t || tabs.includes(i)
    ? { id: 'also-show-in:' + i, label: other.name, disabled: true }
    : { id: 'also-show-in:' + i, label: other.name, run: () => act('AddItemToTab', item.id, i) }));
  also.push({ sep: true }, { id: 'also-show-in:new', label: 'New Tab', run: () => act('MoveItemToNewTab', item.id, true) });
  return [{ id: 'move-to-tab', label: 'Move to Tab', submenu: moveTo }, { id: 'also-show-in', label: 'Also Show In', submenu: also }];
}

// closedEntries lists the places that the pane that shows the item keeps
// for its windows that have closed, out of sight: their apps can open
// again, or the pane can forget them.
function closedEntries(shown) {
  const waiting = (shown && shown.remembered) || [];
  if (!waiting.length) return [];
  const entries = [{ heading: 'Closed Windows' }];
  waiting.forEach((r, i) => {
    if (r.canOpen) entries.push({ id: 'open-closed:' + i, label: 'Open ' + r.app, under: 'Closed Windows', run: () => act('OpenItem', r.itemId) });
    entries.push({
      id: 'remove-closed:' + i, label: 'Remove ' + r.app + ' from This Pane', under: 'Closed Windows', run: () => act('RemoveRemembered', shown.id, r.bundleId),
    });
  });
  return entries;
}

// dropdownEntries lists Move into This Pane, under the heading Drop-down,
// while a terminal is the drop-down (view.dropdown): it comes back out of
// the drop-down into the pane, as a sub-tab, and the drop-down is off
// until the dropdown shortcut makes a new one, or another terminal's Use
// as Drop-down. It's the window menu's and the empty pane menu's.
function dropdownEntries(view, paneId) {
  const drop = view && view.dropdown;
  if (!drop || !paneId) return [];
  return [
    { heading: 'Drop-down' },
    { id: 'move-dropdown-here', label: 'Move into This Pane', under: 'Drop-down', run: () => act('AddToPane', paneId, { type: 'floating', id: drop.id }) },
  ];
}

// moreEntries lists the window menu's last group: a terminal's New
// Terminal in its folder and Use as Drop-down, Dock Again, Show Again,
// Open in Browser, and Pop Out of Bench.
function moreEntries(item, paneId) {
  if (!item) return [];
  const entries = [];
  // A terminal opens another beside it, in the folder its shell is in now.
  if (item.kind === 'builtin') {
    const here = item.here || item.folder || '~';
    entries.push({ id: 'new-terminal-here', label: 'New Terminal in ' + here, run: () => act('AddToPane', paneId, { type: 'terminal', folder: here }) });
    // It leaves its pane and slides down from the top of the screen, over
    // any app, with the dropdown shortcut (dropdown.go).
    entries.push({ id: 'use-as-dropdown', label: 'Use as Drop-down', run: () => act('UseAsDropdown', item.id) });
  }
  if (item.status === 'undocked' || item.status === 'moving') {
    entries.push({ id: 'dock-again', label: 'Dock Again', run: () => act('Redock', item.id) });
  }
  // A minimised window comes back from the Dock, as clicking its sub-tab
  // brings it.
  if (item.status === 'minimised') entries.push({ id: 'show-again', label: 'Show Again', run: () => act('ShowItem', paneId, item.id) });
  // A web page opens in the default browser, where its address can be
  // checked and the page used with the browser's own sign-ins.
  if (isPage(item)) entries.push({ id: 'open-in-browser', label: 'Open in Browser', run: () => act('OpenInBrowser', item.id) });
  // A sub-tab with no window has nothing to pop out, so Go forgets it.
  entries.push({ id: 'pop-out', label: item.noWindow ? 'Remove from Bench' : 'Pop Out of Bench', run: () => act('PopOut', item.id) });
  return entries;
}

// closeEntries lists the window menu's entries that close an item: Close
// Window, Close Page, or Close Terminal; or, for one that's in other tabs
// too, Remove from This Tab, as its × does, and Close Everywhere. A sub-tab
// with no window has Remove from Bench instead, in moreEntries. An app's
// window closes through its close button, which needs Accessibility
// permission.
function closeEntries(view, item, t) {
  if (!item) return [];
  const shared = tabsOf(item).length > 1;
  const entries = [];
  if (shared) entries.push({ id: 'remove-from-tab', label: 'Remove from This Tab', run: () => act('RemoveItemFromTab', item.id, t) });
  if (item.noWindow) return entries;
  const label = shared ? 'Close Everywhere' : closeLabel(item);
  const untrusted = item.kind === 'app' && Boolean(view.permissions) && !view.permissions.accessibility && !view.demo;
  if (untrusted) {
    entries.push({ id: 'close', label, disabled: true }, { note: 'Closing an app\'s window needs Accessibility permission.' });
  } else {
    entries.push({ id: 'close', label, run: () => closeItem(item.id) });
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

// WEB_APP_WAIT_MS is how long, in milliseconds, a web page's native menu
// waits for Go to say what it can offer to put a web app in the page's
// place, before it opens without that entry (webAppChoiceFor).
const WEB_APP_WAIT_MS = 300;

// webAppAnswers keeps Go's last answer to WebAppChoice for each web page,
// or null for no offer, for a native menu that opens before Go answers
// again.
const webAppAnswers = new Map();

// webAppChoiceFor asks Go what a web page's native menu can offer to put a
// web app of its site in its place, as askWebAppChoice does for the HTML
// menu. A native menu can't take an entry once it's open. So once Go has
// answered for the page, the menu opens at once with that answer, and Go's
// new answer, when it comes, is kept for the next time. Until then, the
// menu waits for the answer, but for WEB_APP_WAIT_MS at most, and then
// opens without the entry. A refusal, which means the page has gone,
// offers nothing.
function webAppChoiceFor(itemId) {
  const asked = call('WebAppChoice', itemId).then((choice) => {
    const offer = choice && choice.offer ? choice : null;
    webAppAnswers.set(itemId, offer);
    return offer;
  }, () => {
    webAppAnswers.delete(itemId);
    return null;
  });
  if (webAppAnswers.has(itemId)) return Promise.resolve(webAppAnswers.get(itemId));
  const waited = new Promise((resolve) => {
    setTimeout(() => resolve(null), WEB_APP_WAIT_MS);
  });
  return Promise.race([asked, waited]);
}

// nativeMenus reports whether the window, tab, empty pane, terminal, and
// layout menus show as native macOS menus (View.nativeMenus): in the app,
// where they open without raised mode, so that the web pages and the
// docked windows stay in view (decision 71). The browser preview draws
// them in HTML, with the same entries. The status bar's lists, which are
// lists more than menus, are always drawn in HTML.
export function nativeMenus(view) {
  return Boolean(view && view.nativeMenus);
}

// openMenu opens menu m: as a native menu with its top left corner at
// point, in the points of the UI's viewport (showNative), where Bench has
// native menus; or else in HTML, with the focus on the control with
// data-key focus. later is what the click that opened it does in its pane
// if it closes with nothing chosen, or null (ui.paneLater). A native menu
// keeps it with itself (m.later), as it's shown after a wait, and then
// waits for the user. While one is on its way or open, no other menu
// opens: the click that would have opened it does nothing, in its pane
// too.
function openMenu(m, focus, point, later = null) {
  const { ui } = store;
  if (!nativeMenus(store.view)) {
    ui.paneLater = later;
    openHTML(m, focus);
    return;
  }
  ui.paneLater = null;
  if (ui.nativeMenu) return;
  m.later = later;
  showNative(m, focus, point);
}

// openHTML draws menu m in HTML, with the focus on the control with
// data-key focus. A web page's menu asks Go what it can offer to put a web
// app in the page's place, and takes the entry when Go answers
// (askWebAppChoice).
function openHTML(m, focus) {
  store.ui.menu = m;
  store.ui.pendingFocus = focus;
  update();
  if (m.kind === 'window' && !m.webApp && asksWebApp(itemOf(store.view, m.itemId))) askWebAppChoice(m);
}

// showNative shows menu m as a native menu (ShowMenu), with its top left
// corner at point, and runs the entry chosen, as runMenuEntry does. One
// shows at a time (ui.nativeMenu). Bench isn't raised for it: overlayOpen
// leaves it out. A web page's menu waits a moment for Go's web app offer
// first (webAppChoiceFor). Once the menu has closed, the entry chosen runs
// if the menu's subject is still there (staleMenu) and the entry still
// reads as it did: it runs as the view has it now, so that a tab's menu,
// for one, follows its tab if it moved meanwhile. The focus goes back to
// the control that opened the menu (canRestore), and with nothing chosen,
// the click that opened it does what it would have done in its pane
// (m.later), as closeMenu does. If Go can't show the menu, it opens in
// HTML instead, with the focus on focus.
async function showNative(m, focus, point) {
  const { ui } = store;
  ui.nativeMenu = m;
  ui.menu = null;
  update();
  if (m.kind === 'window' && asksWebApp(itemOf(store.view, m.itemId))) m.webApp = await webAppChoiceFor(m.itemId);
  const shown = store.view && !staleMenu(store.view, m) ? menuEntriesOf(store.view, m) : [];
  let id = '';
  if (shown.length) {
    try {
      id = await call('ShowMenu', { entries: plainEntries(shown), x: Math.round(point.x), y: Math.round(point.y) });
    } catch {
      ui.nativeMenu = null;
      ui.paneLater = m.later;
      openHTML(m, focus);
      return;
    }
  }
  ui.nativeMenu = null;
  const view = store.view;
  const entry = id && view && !staleMenu(view, m) ? chosenEntry(menuEntriesOf(view, m), shown, id) : null;
  ui.paneLater = entry ? null : m.later;
  const back = canRestore(m.opener);
  update();
  if (back) focusKey(m.opener);
  if (entry) entry.run();
  else runPaneLater();
}

// plainEntries returns a menu's entries as ShowMenu takes them
// (workspace.MenuEntry): data, without what they run, with an entry that
// runs nothing off. Only Bench's global shortcuts go with them, which the
// native item's label shows, as the app menu does: keys that work only
// where the menu is, such as a terminal's ⌘C (localKeys), are left out,
// as Mac context menus show no keys.
export function plainEntries(entries) {
  return entries.map((e) => {
    if (e.sep) return { sep: true };
    if (e.heading) return { heading: e.heading };
    if (e.note) return { note: e.note };
    if (e.status) return { status: e.status };
    const p = { id: e.id || '', label: e.label || '' };
    if (e.keys && !e.localKeys) p.keys = e.keys;
    if (e.detail) p.detail = e.detail;
    if (e.disabled || (!e.run && !e.submenu)) p.disabled = true;
    if (typeof e.checked === 'boolean') p.checked = e.checked;
    if (e.submenu) p.submenu = plainEntries(e.submenu);
    return p;
  });
}

// findEntry returns the entry with id among a menu's entries, or those of
// a submenu that's on, if it runs, or null.
export function findEntry(entries, id) {
  for (const e of entries) {
    if (e.submenu) {
      const found = e.disabled ? null : findEntry(e.submenu, id);
      if (found) return found;
    } else if (e.id === id && e.run && !e.disabled) {
      return e;
    }
  }
  return null;
}

// chosenEntry returns the entry with id in a menu's entries now, if it
// runs and reads as it did in the entries shown, or null.
function chosenEntry(now, shown, id) {
  const was = findEntry(shown, id);
  const entry = findEntry(now, id);
  return was && entry && entry.label === was.label ? entry : null;
}

// menuEntriesOf returns the entries of a window, tab, empty pane,
// terminal, or layout menu m, as the view has them now: the same whether
// they're drawn in HTML (renderMenu) or shown natively (showNative).
function menuEntriesOf(view, m) {
  switch (m.kind) {
    case 'window': return itemOf(view, m.itemId) ? windowMenuEntries(view, m.itemId, m.paneId, m.webApp, m.status) : [];
    case 'tab': return view.tabs[m.index] ? tabMenuEntries(view, m.index) : [];
    case 'pane': return paneMenuEntries(view, m.paneId, m.addTo);
    case 'terminal': return terminals.menuEntries(m.itemId);
    case 'layout': return layoutMenuEntries(view);
    default: return [];
  }
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
export function staleMenu(view, m) {
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

// under returns the point below element el, where its menu opens without
// a pointer, as a pop-up button's does, in the points of the UI's
// viewport: at its left, or dx points in, and dy points below it, or, with
// top, dy points below its top.
function under(el, dx = 0, dy = 4, top = false) {
  const r = el.getBoundingClientRect();
  return { x: r.left + dx, y: (top ? r.top : r.bottom) + dy };
}

// inViewport returns point at, in content coordinates, in the points of
// the UI's viewport, where a native menu opens.
function inViewport(at) {
  const o = contentOrigin();
  return { x: at.x + o.x, y: at.y + o.y };
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
// changes. The menu itself has the focus, with no entry chosen, as a macOS
// menu opens: its first entry can close the window, which a second Return
// from habit mustn't do. Down Arrow and Home go to the first entry, and Up
// Arrow and End to the last (menuKeys in keyboard.js). Closing it gives the
// focus back to opener, the data-key of the control that opened it: the
// menu button, or a sub-tab that opened it from the keyboard (roving.js).
// A native menu opens at the pointer too, or else below the button, at its
// left, as a pop-up button's does (openMenu). later is what the click that
// opened it does in its pane if it closes with nothing chosen.
export function openWindowMenu(button, itemId, paneId, at = null, opener = button.dataset.key || null, later = null) {
  const view = store.view;
  const o = contentOrigin();
  const r = button.getBoundingClientRect();
  const m = {
    kind: 'window',
    itemId,
    paneId: realPaneOf(view, itemId, paneId),
    shownIn: paneId,
    tab: view.current,
    x: menuX(at ? at.x : r.right - o.x - 260),
    y: at ? at.y : r.bottom - o.y + 4,
    opener,
    webApp: null,
    status: statusLine(itemOf(view, itemId)),
  };
  openMenu(m, 'window-menu', at ? inViewport(at) : under(button), later);
}

// menuX keeps a window or tab menu that starts at x inside the window
// across: menus are at most 320 points wide. fitMenu keeps it inside the
// window down.
function menuX(x) {
  return Math.max(8, Math.min(x, windowElement().clientWidth - 330));
}

// tabMenuEntries lists what tab index's menu offers, in groups, with the
// names that the menu bar's Tab menu gives them: the tab's own properties,
// which, unlike Settings, apply to it alone, and its name, typed in place
// (rename.js); moving it along the tabs, which changes its shortcut;
// arranging it; closing or deleting it; and adding a tab, or reopening the
// one closed last. The entries that open a dialog do so in the menu's place
// (opens). Close Tab… closes the tab's windows, pages, and terminals,
// asking first where Go would; on the only tab, it's Close All Windows…,
// which leaves the tab, and the last tab can't be deleted.
export function tabMenuEntries(view, index) {
  const current = index === view.current;
  const count = (view.tabs || []).length;
  const only = count === 1;
  return withSeparators([
    [
      { id: 'tab-properties', label: 'Tab Properties…', opens: true, run: () => openEditor(index) },
      // The tab's name turns into a field, with the name selected, to type
      // a new one, as a double-click on the tab does.
      { id: 'rename', label: 'Rename…', run: () => startTabRename(index) },
    ],
    only ? [] : [
      index > 0
        ? { id: 'move-tab-left', label: 'Move Tab Left', run: () => act('MoveTab', index, index - 1) }
        : { id: 'move-tab-left', label: 'Move Tab Left', disabled: true },
      index < count - 1
        ? { id: 'move-tab-right', label: 'Move Tab Right', run: () => act('MoveTab', index, index + 1) }
        : { id: 'move-tab-right', label: 'Move Tab Right', disabled: true },
    ],
    [
      { id: 'rearrange', label: 'Rearrange', keys: current ? keysOf(view).rearrange || '' : '', run: () => rearrangeTab(index) },
      // As in Tab properties, Reset Layout doesn't ask first.
      { id: 'reset-layout', label: 'Reset Layout', run: () => act('ResetLayout', index) },
    ],
    [
      { id: 'close-tab', label: closeTabLabel(view), run: () => closeTab(index) },
      ...(only ? [] : [{ id: 'delete-tab', label: 'Delete Tab (Keep Windows)', run: () => act('DeleteTab', index) }]),
    ],
    [{ id: 'new-tab', label: 'New Tab…', opens: true, run: () => openEditor(-1) }, ...reopenEntries(view)],
  ]);
}

// reopenEntries is Reopen Closed Tab, for the tab closed or deleted last,
// which it names, if Go keeps one, and Reopen Closed Page or Terminal, for
// the web page or terminal closed last, likewise, with its app, as the
// toast that said it closed named it.
function reopenEntries(view) {
  const name = (view.closedTabs || [])[0];
  const item = (view.closedItems || [])[0];
  return [
    ...(name ? [{ id: 'reopen-tab', label: 'Reopen Closed Tab', detail: name, run: () => act('ReopenTab', name) }] : []),
    ...(item ? [{ id: 'reopen-item', label: reopenItemLabel(item), detail: item.label, run: () => act('ReopenItem', item.id) }] : []),
  ];
}

// reopenItemLabel names what Reopen Closed Page or Terminal opens again, as
// the menu bar's Tab menu does: Reopen Closed Page, or Reopen Closed
// Terminal.
export function reopenItemLabel(item) {
  return item.kind === 'builtin' ? 'Reopen Closed Terminal' : 'Reopen Closed Page';
}

// paneMenuEntries lists what an empty pane's menu offers, with the names
// that the menu bar's Pane menu gives them: what the buttons in its body
// do, moving the drop-down into it (dropdownEntries), and zooming it. A
// pane closes only in a tab that has more than one, shown or zoomed.
// Adding to it, or moving the drop-down into it, is for addTo, the pane
// that the add dialog adds to (the focused pane, for the "tabs" view's
// compact pane).
export function paneMenuEntries(view, paneId, addTo) {
  const entries = [{ id: 'add-to-pane', label: 'Add to Pane…', run: () => openAdd(addTo) }];
  if (panesCanClose(view)) entries.push({ id: 'close-pane', label: 'Close Pane', run: () => act('ClosePane', paneId) });
  const drop = dropdownEntries(view, addTo);
  if (drop.length) entries.push({ sep: true }, ...drop);
  const zoom = zoomEntry(view, paneId);
  if (zoom) entries.push({ sep: true }, { id: 'zoom-pane', label: zoom.zoomed ? 'Show All Panes' : 'Zoom Pane', keys: zoom.keys, run: zoom.run });
  return entries;
}

// panesCanClose reports whether the current tab's empty panes can close: it
// has more than one, shown or zoomed, and doesn't show them as one, as the
// "tabs" view does.
export function panesCanClose(view) {
  const d = (view && view.display) || {};
  const tab = currentTab(view);
  return (d.zoomed ? (tab && tab.panes) || 0 : (d.panes || []).length) > 1;
}

// zoomEntry returns what Zoom Pane does now, for the menus and the jump
// palette: while a pane is zoomed (zoomed), show every pane again; or else
// zoom pane paneId, or the focused pane for ''. It returns null where no
// pane can be zoomed: in the "tabs" view, which shows one pane anyway, and
// in a tab with only one pane. keys is the zoom_pane shortcut, which does
// the same.
export function zoomEntry(view, paneId = '') {
  const d = (view && view.display) || {};
  const keys = keysOf(view).zoomPane || '';
  if (d.zoomed) return { zoomed: true, keys, run: () => act('ShowAllPanes') };
  const tab = currentTab(view);
  if (d.compact || !tab || (tab.panes || 0) < 2) return null;
  return { zoomed: false, keys, run: () => act('ZoomPane', paneId) };
}

// toggleZoom zooms the focused pane, or shows every pane again while one is
// zoomed, as the zoom_pane shortcut does in the browser preview. Go says
// why where it can't.
export function toggleZoom() {
  const d = (store.view && store.view.display) || {};
  return d.zoomed ? act('ShowAllPanes') : act('ZoomPane', '');
}

// withSeparators joins groups of menu entries, with a separator between
// each group and the next, leaving out empty groups.
function withSeparators(groups) {
  return groups.filter((g) => g.length).flatMap((g, i) => (i ? [{ sep: true }, ...g] : g));
}

// rearrangeTab arranges tab index again. Go arranges only the current tab,
// so another tab is switched to first, as is the tab Go has current while
// a switch to another is on its way (shownCurrent, decision 67).
async function rearrangeTab(index) {
  if (index !== shownCurrent(store.view)) {
    const r = await switchTab(index);
    if (!r.ok) return;
  }
  act('Rearrange');
}

// openTabMenu opens a tab's menu, for a right-click (or Control-click) on
// the tab in the tab strip: at the pointer (at, in content coordinates), or
// below the tab without one. The tab isn't switched to: the menu is about
// that tab, whichever is current. If it replaces a window menu, the click
// that opened that menu no longer focuses its pane (openMenu).
export function openTabMenu(tabEl, index, at = null) {
  const view = store.view;
  const tab = view.tabs[index];
  if (!tab) return;
  const o = contentOrigin();
  const r = tabEl.getBoundingClientRect();
  const m = {
    kind: 'tab',
    index,
    // tabName is the tab's name when the menu opened, to find it by
    // (followTab).
    tabName: tab.name,
    x: menuX(at ? at.x : r.left - o.x),
    y: at ? at.y : r.bottom - o.y + 4,
    opener: tabEl.dataset.key || null,
  };
  openMenu(m, 'menu-item-0', at ? inViewport(at) : under(tabEl));
}

// openNotices opens the list of notices by the button that opens it.
export function openNotices(button) {
  openList(button, { kind: 'notices' }, 'notices-list');
}

// attentionKey names the windows of the current tab that need attention,
// to tell when they change.
export function attentionKey(view) {
  return attentionOf(view).map((w) => w.paneId + '/' + w.itemId).join(' ');
}

// openAttention opens the list of the windows that need attention by the
// button that opens it: each says why, and shows its window.
export function openAttention(button) {
  const view = store.view;
  openList(button, { kind: 'attention', tab: view.current, key: attentionKey(view) }, 'menu-item-0');
}

// openList opens one of the status bar's lists by the button that opens
// it, and focuses the control with data-key focus: above the button in the
// status bar, or below it in the title bar, where the button is while the
// status bar is hidden. If it replaces a window menu, the click that opened
// that menu no longer focuses its pane (ui.paneLater).
function openList(button, menu, focus) {
  store.ui.paneLater = null;
  const win = windowElement();
  const o = contentOrigin();
  const r = button.getBoundingClientRect();
  const place = button.closest('#titlebar') ? { y: r.bottom - o.y + 6 } : { bottom: win.clientHeight - (r.top - o.y) + 6 };
  store.ui.menu = {
    ...menu,
    x: Math.max(8, Math.min(r.left - o.x, win.clientWidth - 490)),
    ...place,
    opener: button.dataset.key || null,
  };
  store.ui.pendingFocus = focus;
  update();
}

// listStyle places one of the status bar's lists: above its button, or
// below it in the title bar.
function listStyle(m) {
  return 'left: ' + m.x + 'px; ' + (m.bottom != null ? 'bottom: ' + m.bottom : 'top: ' + m.y) + 'px';
}

// layoutMenuEntries lists the layouts, with the one in use ticked, and why
// the layout is as it is.
export function layoutMenuEntries(view) {
  const entries = LAYOUT_MODES.map(([id, label]) => ({ id: 'layout:' + id, label, checked: view.mode === id, run: () => act('SetMode', id) }));
  entries.push({ sep: true }, { note: sentence(layoutText(view, currentTab(view))) });
  return entries;
}

// openLayoutMenu opens the layout menu below a slim title bar's layout
// button, which stands for the layout control there, with the focus on the
// layout in use.
export function openLayoutMenu(button) {
  const o = contentOrigin();
  const r = button.getBoundingClientRect();
  const m = {
    kind: 'layout',
    x: menuX(r.left - o.x),
    y: r.bottom - o.y + 4,
    opener: button.dataset.key || null,
  };
  const view = store.view;
  openMenu(m, 'menu-item-' + Math.max(0, LAYOUT_MODES.findIndex(([id]) => view && id === view.mode)), under(button));
}

// attentionEntries lists the windows that need attention, each with why:
// choosing one shows it.
function attentionEntries(view) {
  return attentionOf(view).map(({ itemId, paneId }) => {
    const item = itemOf(view, itemId);
    return {
      id: 'show:' + paneId + '/' + itemId,
      label: labelOf(view, itemId),
      detail: sentence(statusText(item)),
      marker: markerOf(item),
      run: () => act('ShowItem', paneId, itemId),
    };
  });
}

// runMenuEntry runs the open menu's entry at index, or entry sub of the
// submenu at index, and closes the menu. The entry does its own focusing,
// so the click that opened the menu doesn't (ui.paneLater). An entry that
// opens a dialog (opens) replaces the menu with it, and the dialog gives
// the focus back to the menu's opener when it closes (openerKey): closing
// the menu first would draw Bench with neither open, which would lower it
// (syncOverlay).
export function runMenuEntry(index, sub = null) {
  const { ui } = store;
  const parent = ui.menuEntries[index];
  const entry = sub == null ? parent : ((parent && parent.submenu) || [])[sub];
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

// SUBMENU_KEY is the data-key of a submenu's first entry that's on, which
// gets the focus when the keyboard opens it.
const SUBMENU_KEY = 'menu-sub-0';

// openSubmenu opens the submenu of the open menu's entry at index, such as
// Move to Tab, beside it (placeSubmenu), and with into, as the keyboard
// does, gives the focus to its first entry. The pointer opens it without
// moving the focus.
export function openSubmenu(index, into) {
  const m = store.ui.menu;
  const entry = store.ui.menuEntries[index];
  if (!m || !entry || !entry.submenu) return;
  stopHover();
  if (m.sub !== entry.id) {
    m.sub = entry.id;
    m.subPos = null;
  }
  if (into) store.ui.pendingFocus = SUBMENU_KEY;
  update();
}

// closeSubmenu closes the open menu's submenu, and with back, gives the
// focus to the entry that opened it, as Left Arrow and Escape do.
export function closeSubmenu(back) {
  const m = store.ui.menu;
  if (!m || !m.sub) return;
  const parent = document.getElementById('menu-parent');
  stopHover();
  m.sub = null;
  m.subPos = null;
  if (back && parent) store.ui.pendingFocus = parent.dataset.key;
  update();
}

// HOVER_MS is how long, in milliseconds, the pointer rests on an entry
// with a submenu before it opens, or on another entry before an open
// submenu closes. AIM_MS is how long the pointer may pause on its way to
// an open submenu, across the entries between, before the submenu follows
// the entry it's on.
const HOVER_MS = 200;
const AIM_MS = 400;

// hover is where the pointer last was over a menu, and the change it waits
// to make: { point, want, timer }, where want is the id of the submenu to
// open, or null to close the open one.
const hover = { point: null, want: undefined, timer: null };

// stopHover forgets the submenu change that the pointer waits to make.
function stopHover() {
  clearTimeout(hover.timer);
  hover.timer = null;
  hover.want = undefined;
}

// onMenuPointerMove opens a submenu when the pointer rests on its entry,
// and closes it when the pointer rests on another entry of the menu, after
// HOVER_MS. While the pointer moves towards the open submenu, across the
// entries between, the submenu stays open (aimsAt), so that a diagonal
// move to it doesn't close it; it follows the entry under the pointer once
// the pointer stops, after AIM_MS. Over the submenu, nothing changes, and
// outside the menus, an open submenu stays as it is, but one that the
// pointer only passed over on its way out doesn't open.
function onMenuPointerMove(e) {
  const m = store.ui.menu;
  if (!m || m.kind !== 'window' || e.pointerType === 'touch') return;
  const prev = hover.point;
  const point = { x: e.clientX, y: e.clientY };
  hover.point = point;
  const el = e.target && e.target.closest ? e.target : null;
  if (!el || el.closest('#menu-sub')) {
    stopHover();
    return;
  }
  const menu = el.closest('#menu-root > .menu:not(.submenu)');
  if (!menu) {
    if (hover.want) stopHover();
    return;
  }
  const entry = el.closest('.menu-item');
  const index = entry ? Number(entry.dataset.index) : -1;
  const target = entry && entry.dataset.action === 'menu-sub' ? (store.ui.menuEntries[index] || {}).id || null : null;
  if (target === (m.sub || null)) {
    stopHover();
    return;
  }
  const sub = document.getElementById('menu-sub');
  if (m.sub && sub && prev && aimsAt(prev, point, sub.getBoundingClientRect(), menu.getBoundingClientRect())) {
    clearTimeout(hover.timer);
    hover.want = undefined;
    hover.timer = setTimeout(() => hoverSettle(m), AIM_MS);
    return;
  }
  if (hover.want === target && hover.timer) return;
  clearTimeout(hover.timer);
  hover.want = target;
  hover.timer = setTimeout(() => hoverApply(m, target, entry ? entry.dataset.key : null), HOVER_MS);
}

// hoverSettle makes the change that the entry under the pointer asks for,
// once the pointer has stopped on its way to the submenu.
function hoverSettle(m) {
  hover.timer = null;
  if (store.ui.menu !== m || !hover.point) return;
  const el = document.elementFromPoint(hover.point.x, hover.point.y);
  if (!el || el.closest('#menu-sub') || !el.closest('#menu-root > .menu:not(.submenu)')) return;
  const entry = el.closest('.menu-item');
  const index = entry ? Number(entry.dataset.index) : -1;
  const target = entry && entry.dataset.action === 'menu-sub' ? (store.ui.menuEntries[index] || {}).id || null : null;
  if (target !== (m.sub || null)) hoverApply(m, target, entry ? entry.dataset.key : null);
}

// hoverApply opens submenu target, or closes the open one for null, in
// menu m if it's still open. Focus that was in a submenu that closes goes
// to the entry under the pointer (key).
function hoverApply(m, target, key) {
  hover.timer = null;
  hover.want = undefined;
  if (store.ui.menu !== m || (m.sub || null) === target) return;
  const inSub = Boolean(document.activeElement && document.activeElement.closest && document.activeElement.closest('#menu-sub'));
  m.sub = target;
  m.subPos = null;
  if (inSub && key) store.ui.pendingFocus = key;
  update();
}

// aimsAt reports whether the pointer, moving from prev to point, heads for
// a submenu at rect beside a menu at menuRect: whether point is in the
// triangle between prev and the submenu's near edge.
function aimsAt(prev, point, rect, menuRect) {
  if (prev.x === point.x && prev.y === point.y) return false;
  const x = rect.left >= menuRect.left ? rect.left : rect.right;
  const a = prev;
  const b = { x, y: rect.top - 4 };
  const c = { x, y: rect.bottom + 4 };
  const side = (p, q, r) => (p.x - r.x) * (q.y - r.y) - (q.x - r.x) * (p.y - r.y);
  const d1 = side(point, a, b);
  const d2 = side(point, b, c);
  const d3 = side(point, c, a);
  const neg = d1 < 0 || d2 < 0 || d3 < 0;
  const pos = d1 > 0 || d2 > 0 || d3 > 0;
  return !(neg && pos);
}

// installMenuHover follows the pointer over the menus, for their submenus.
export function installMenuHover() {
  document.addEventListener('pointermove', onMenuPointerMove);
}

// openPaneMenu opens an empty pane's menu, for a right-click on its header
// or body: at the pointer (at, in content coordinates), or below the
// header (el) without one. later is what the click does in the pane if
// the menu closes with nothing chosen.
export function openPaneMenu(el, paneId, addTo, at = null, later = null) {
  const view = store.view;
  const o = contentOrigin();
  const r = el.getBoundingClientRect();
  const m = {
    kind: 'pane',
    paneId,
    addTo,
    tab: view.current,
    x: menuX(at ? at.x : r.left - o.x + 8),
    y: at ? at.y : r.bottom - o.y + 4,
    opener: null,
  };
  openMenu(m, 'menu-item-0', at ? inViewport(at) : under(el, 8), later);
}

// openTerminalMenu opens a terminal's menu (terminals.menuEntries), for a
// right-click in the terminal (host, its element), shown in pane paneId: at
// the pointer (at, in content coordinates), or at the terminal's top left
// without one. The terminal gets the keyboard back when it closes. later
// is what the click does in the pane if the menu closes with nothing
// chosen.
export function openTerminalMenu(host, itemId, paneId, at = null, later = null) {
  const view = store.view;
  const o = contentOrigin();
  const r = host.getBoundingClientRect();
  const m = {
    kind: 'terminal',
    itemId,
    paneId,
    shownIn: paneId,
    tab: view.current,
    x: menuX(at ? at.x : r.left - o.x + 8),
    y: at ? at.y : r.top - o.y + 8,
    opener: termKey(itemId),
  };
  openMenu(m, 'menu-item-0', at ? inViewport(at) : under(host, 8, 8, true), later);
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
      + ' style="' + listStyle(m) + '">'
      + '<div class="menu-heading" role="presentation">Need attention</div>' + renderEntries(entries) + '</div>';
  }
  const entries = menuEntriesOf(view, m);
  if (m.kind === 'layout') {
    ui.menuEntries = entries;
    // The note, why the layout is in use, describes the menu.
    return '<div class="menu" role="menu" aria-label="Layout" id="layout-menu" aria-describedby="menu-note-' + entries.findIndex((e) => e.note) + '"'
      + ' data-scroll="layout-menu" style="' + menuStyle(m) + '">' + renderEntries(entries) + '</div>';
  }
  if (m.kind === 'tab') {
    const tab = view.tabs[m.index];
    if (!tab) return '';
    ui.menuEntries = entries;
    return '<div class="menu" role="menu" aria-label="' + esc('Options for the ' + tab.name + ' tab') + '" id="tab-menu"'
      + ' data-scroll="tab-menu" style="' + menuStyle(m) + '">' + renderEntries(entries) + '</div>';
  }
  if (m.kind === 'terminal') {
    if (!entries.length) return '';
    ui.menuEntries = entries;
    return renderTerminalMenu(entries, menuStyle(m));
  }
  if (m.kind === 'pane') {
    ui.menuEntries = entries;
    return '<div class="menu" role="menu" aria-label="Pane options" id="pane-menu"'
      + ' data-scroll="pane-menu" style="' + menuStyle(m) + '">' + renderEntries(entries) + '</div>';
  }
  const item = itemOf(view, m.itemId);
  if (!item) return '';
  ui.menuEntries = entries;
  // An open submenu whose entry has gone closes.
  const open = m.sub ? entries.findIndex((e) => e.submenu && e.id === m.sub) : -1;
  if (open < 0) {
    m.sub = null;
    m.subPos = null;
  }
  // The status line, which isn't an entry, describes the menu. The menu
  // takes the focus when it opens (openWindowMenu).
  const described = m.status ? ' aria-describedby="menu-status"' : '';
  const html = '<div class="menu' + (asksWebApp(item) ? ' fixed-width' : '') + '" role="menu" aria-label="Window options" id="window-menu"' + described
    + ' tabindex="-1" data-key="window-menu" data-scroll="window-menu" style="' + menuStyle(m) + '">' + renderEntries(entries, { open: m.sub }) + '</div>';
  if (open < 0) return html;
  // The open submenu is beside the menu, not in it, so that the menu's
  // scrolling doesn't cut it off. Its entry names it.
  return html + '<div class="menu submenu" role="menu" id="menu-sub" aria-labelledby="menu-parent" data-scroll="menu-sub" style="' + subStyle(m) + '">'
    + renderEntries(entries[open].submenu, { parent: open }) + '</div>';
}

// subStyle places the open submenu where placeSubmenu put it. Until then,
// it's drawn out of sight, to be measured.
function subStyle(m) {
  const p = m.subPos;
  if (!p) return 'left: 0; top: 0; visibility: hidden';
  return 'left: ' + p.x + 'px; top: ' + p.y + 'px; max-height: ' + maxHeightBelow(p.y);
}

// renderTerminalMenu returns the HTML of a terminal's menu, with its
// entries, placed by style: in the main window, and in a popped-out
// terminal's own window.
export function renderTerminalMenu(entries, style) {
  return '<div class="menu" role="menu" aria-label="Terminal" id="terminal-menu" data-scroll="terminal-menu" style="' + style + '">'
    + renderEntries(entries) + '</div>';
}

// renderEntries returns the HTML of a menu's entries. An entry can have a
// marker before its label, and a detail on a line below it. A status line
// says why a window isn't ready, as a disabled line with its marker. An
// entry that's one of a choice (checked, true or false) is ticked if it's
// the one chosen. An entry with a submenu, such as Move to Tab or a web
// page's Page, opens it (data-action "menu-sub"), and is
// marked as expanded while submenu open is. With parent, the entries are
// those of the submenu of the menu's entry at parent: each runs as entry
// data-sub of it.
function renderEntries(entries, { open = null, parent = null } = {}) {
  const sub = parent != null;
  const prefix = sub ? 'menu-sub-' : 'menu-item-';
  const at = (i) => (sub ? ' data-index="' + parent + '" data-sub="' + i + '"' : ' data-index="' + i + '"');
  let n = 0;
  // A note says why the disabled entries before it, up to the separator
  // before them, are disabled.
  const noteFor = (i) => {
    for (let j = i + 1; j < entries.length && !entries[j].sep; j++) {
      if (entries[j].note) return 'menu-note-' + j;
    }
    return '';
  };
  return entries.map((e, i) => {
    const noteId = e.disabled ? noteFor(i) : '';
    if (e.sep) return '<div class="menu-sep" role="separator"></div>';
    if (e.heading) return '<div class="menu-heading" role="presentation">' + esc(e.heading) + '</div>';
    if (e.note) return '<div class="menu-note" role="none" id="menu-note-' + i + '">' + esc(e.note) + '</div>';
    if (e.status) return '<div class="menu-status" role="none" id="menu-status">' + statusMark(e.marker) + '<span>' + esc(e.status) + '</span></div>';
    const key = e.disabled ? (sub ? 'menu-sub-disabled-' : 'menu-disabled-') + i : prefix + (n++);
    if (e.submenu) {
      const expanded = open === e.id;
      return '<button type="button" class="menu-item has-sub" role="menuitem" aria-haspopup="menu" aria-expanded="' + expanded + '"'
        + (expanded ? ' id="menu-parent" aria-controls="menu-sub"' : '')
        + ' data-action="menu-sub" data-index="' + i + '" data-key="' + key + '">'
        + '<span class="menu-label">' + esc(e.label) + '</span><span class="menu-sub-mark" aria-hidden="true">' + ICONS.chevron + '</span></button>';
    }
    const label = e.detail
      ? '<span class="menu-label stacked"><span class="menu-main">' + esc(e.label) + '</span><span class="menu-detail">' + esc(e.detail) + '</span></span>'
      : '<span class="menu-label' + (e.wrap ? ' wrap' : '') + '">' + esc(e.label) + '</span>';
    const choice = typeof e.checked === 'boolean';
    const role = choice ? 'menuitemradio" aria-checked="' + e.checked : 'menuitem';
    const tick = choice ? '<span class="menu-tick">' + (e.checked ? ICONS.check : '') + '</span>' : '';
    return '<button type="button" class="menu-item" role="' + role + '" data-action="menu-run"' + at(i) + ' data-key="' + key + '"'
      + (e.detail ? ' aria-label="' + esc(e.label + ': ' + e.detail) + '"' : '')
      + (e.disabled ? ' disabled' + (noteId ? ' aria-describedby="' + noteId + '"' : '') : '') + '>'
      + tick + statusMark(e.marker) + label + (e.keys ? '<kbd>' + esc(e.keys) + '</kbd>' : '') + '</button>';
  }).join('');
}

// menuStyle places a window, tab, pane, terminal, or layout menu. Once
// fitMenu has placed it (m.top), it stays there.
function menuStyle(m) {
  let style = 'left: ' + m.x + 'px; top: ' + (m.top ?? m.y) + 'px';
  if (m.top != null) style += '; max-height: ' + maxHeightBelow(m.top);
  return style;
}

// renderNotices returns the list of notices, each with its buttons, such
// as Reload for a config.toml changed outside Bench, then the recent
// messages: the toasts and the notices that have gone, each with when it
// came, its whole text, and Copy, after the toast's Undo or Keep over Bench
// while it still applies. A notice's button calls the bound method that Go
// names for it (runNoticeAction).
function renderNotices(view, m) {
  const notices = view.notices || [];
  const messages = recentMessages(view);
  let html = '<div class="menu wide" role="dialog" aria-label="' + (notices.length ? 'Notices' : 'Recent messages') + '" id="notices-menu"'
    + ' data-scroll="notices-menu" style="' + listStyle(m) + '">'
    + '<div tabindex="-1" data-key="notices-list">';
  if (notices.length) {
    html += '<div class="menu-heading" role="presentation">Notices</div>'
      + '<div role="list" aria-label="Notices">'
      + notices.map((n, i) => '<div class="menu-text" role="listitem">' + esc(n.text) + noticeButtons(n, i) + '</div>').join('')
      + '</div><div class="menu-sep" role="separator"></div>';
  }
  html += '<div class="menu-heading" role="presentation">Recent messages</div>';
  if (!messages.length) return html + '<div class="menu-note">No messages yet.</div></div></div>';
  return html + '<div role="list" aria-label="Recent messages">'
    + messages.map((msg, i) => '<div class="message" role="listitem">'
      + '<div class="message-head"><time datetime="' + esc(new Date(msg.time).toISOString()) + '">' + esc(clockTime(msg.time)) + '</time>'
      + (msg.kind === 'notice' ? '<span>Notice</span>' : '')
      + (msg.count > 1 ? '<span>' + msg.count + ' times</span>' : '')
      + messageUndo(view, msg) + messageKeep(view, msg, i)
      + '<button type="button" class="btn small" data-action="message-copy" data-index="' + i + '" data-key="message-' + i + '"'
      + ' aria-label="' + esc('Copy the message from ' + clockTime(msg.time)) + '">Copy</button></div>'
      + '<div class="message-text">' + esc(msg.text) + '</div></div>').join('')
    + '</div></div></div>';
}

// messageUndo returns the HTML of a recent message's Undo, while the
// change that its toast's Undo takes back is still the latest that Go can
// take back (View.undo), or is Bench's move back to the display that the
// user chose, while Go can still take that back (View.wentBack), which
// isn't one of the changes that Tab ▸ Undo takes back: its toast may have
// gone, or be under the list (keepToastOffList). It says what it takes
// back, as the menu bar's Tab ▸ Undo does.
function messageUndo(view, msg) {
  if (!msg.undo) return '';
  const undo = [view.undo, view.wentBack].find((u) => u && u.step === msg.undo);
  if (!undo) return '';
  const key = undo === view.undo ? 'message-undo' : 'message-undo-back';
  return '<button type="button" class="btn small" data-action="message-undo" data-step="' + msg.undo + '" data-key="' + key + '"'
    + ' aria-label="' + esc('Undo ' + (undo.name || '')) + '" title="' + esc(sentence(undoHint(undo) || 'Undo the change')) + '">Undo</button>';
}

// messageKeep returns the HTML of recent message i's Keep over Bench, as
// its toast had, while Go can still keep that app's windows over Bench
// (View.keepOver): the toast shows for a few seconds, and Bench says once
// for each app that it put the app's windows out of sight.
function messageKeep(view, msg, i) {
  if (!msg.keep || !(view.keepOver || []).includes(msg.keep)) return '';
  return '<button type="button" class="btn small" data-action="message-keep" data-item="' + esc(msg.keep) + '" data-key="message-keep-' + i + '"'
    + ' aria-label="' + esc('Keep over Bench, for the message from ' + clockTime(msg.time)) + '"'
    + ' title="Let this app\'s windows stay over Bench. Bench adds a rule to config.toml.">Keep over Bench</button>';
}

// keepMessage keeps an app's windows over Bench, for a recent message's
// Keep over Bench, as its toast's button does. The list stays open, and
// shows what Go says at its top; the keyboard goes back to the list, as
// the button goes.
export function keepMessage(bundleID) {
  store.ui.pendingFocus = 'notices-list';
  act('KeepOverBench', bundleID);
}

// undoMessage takes back the change of a recent message's Undo, step. The
// list stays open, and shows what Undo says at its top; the keyboard goes
// back to the list, as the button goes with the change.
export function undoMessage(step) {
  store.ui.pendingFocus = 'notices-list';
  act('Undo', step);
}

// clockTime returns the time of day of a time in milliseconds, as "14:05".
function clockTime(ms) {
  const d = new Date(ms);
  return String(d.getHours()).padStart(2, '0') + ':' + String(d.getMinutes()).padStart(2, '0');
}

// copyMessage puts recent message i's text on the clipboard, and says so
// on its Copy button (button).
export async function copyMessage(button, i) {
  const msg = recentMessages(store.view)[i];
  if (!msg) return;
  try {
    await setClipboardText(msg.text, isDemo());
  } catch {
    toast('Bench couldn\'t copy to the clipboard.');
    return;
  }
  button.textContent = 'Copied';
}

// openMessages opens the list of notices and recent messages for Help ▸
// Recent Messages: by the button that opens it, in the status bar or the
// title bar, or, while there's none, below the title bar at its right.
export function openMessages() {
  const button = document.querySelector('[data-action="notices"]');
  if (button) {
    openNotices(button);
    return;
  }
  store.ui.paneLater = null;
  const win = windowElement();
  const o = contentOrigin();
  const title = document.getElementById('titlebar');
  const bottom = title ? title.getBoundingClientRect().bottom - o.y : 0;
  store.ui.menu = { kind: 'notices', x: Math.max(8, win.clientWidth - 490), y: bottom + 6, opener: activeKey() || null };
  store.ui.pendingFocus = 'notices-list';
  update();
}

// noticeButtons returns the HTML of notice i's buttons, if it has any.
function noticeButtons(notice, i) {
  const actions = notice.actions || [];
  if (!actions.length) return '';
  return '<div class="notice-actions">'
    + actions.map((a, j) => '<button type="button" class="btn small" data-action="notice-run" data-notice="' + i + '" data-index="' + j
      + '" data-key="notice-' + i + '-' + j + '">' + esc(a.label) + '</button>').join('')
    + '</div>';
}

// runNoticeAction runs button j of notice i: it closes the notices and
// calls the button's bound method, which takes nothing, or the string arg
// that Go gives with it, such as the shortcuts to change for Use this
// instead. Go's answer, such as a reloaded config, comes as a toast and a
// new view. A button with a command instead does what that bench:command
// does, such as opening Settings at its Keyboard section.
export function runNoticeAction(i, j) {
  const notice = ((store.view && store.view.notices) || [])[i];
  const action = notice && (notice.actions || [])[j];
  if (!action) return;
  const method = /^[A-Z][A-Za-z]*$/.test(action.method || '') ? action.method : '';
  if (!method && !action.command) return;
  store.ui.paneLater = null;
  closeMenu(true);
  if (!method) runCommand({ name: action.command });
  else if (action.arg) act(method, action.arg);
  else act(method);
}

// fitMenu places a window, tab, pane, terminal, or layout menu when it
// opens: it moves the menu up if it would go past the bottom of the window,
// leaving room below for the entry that joins a web page's menu when Go
// answers (askWebAppChoice). The menu then stays where it is (ui.menu.top),
// so that nothing in it moves when an entry joins it, or when the view
// changes: if it grows past the bottom of the window, it scrolls instead.
// It places a submenu that opens too (placeSubmenu).
export function fitMenu() {
  const el = document.querySelector('#menu-root .menu');
  const m = store.ui.menu;
  if (!el || !m || !['window', 'tab', 'pane', 'terminal', 'layout'].includes(m.kind)) return;
  if (m.top == null) {
    const win = windowElement();
    const top = el.offsetTop;
    const room = m.kind === 'window' && !m.webApp && asksWebApp(itemOf(store.view, m.itemId)) ? entryRoom(el) : 0;
    const over = top + el.offsetHeight + room - (win.clientHeight - 8);
    m.top = over > 0 ? Math.max(8, top - over) : top;
    el.style.top = m.top + 'px';
    el.style.maxHeight = maxHeightBelow(m.top);
  }
  placeSubmenu(el, m);
}

// placeSubmenu places menu m's submenu once, when it opens: to the right of
// the menu (el), overlapping it a little, with its first entry level with
// the entry that opened it; to the left of the menu where there's no room
// on the right; and moved up, or across, as far as it needs to stay inside
// the window. It then stays there (m.subPos).
function placeSubmenu(el, m) {
  const sub = document.getElementById('menu-sub');
  const parent = document.getElementById('menu-parent');
  if (!sub || !parent || m.subPos) return;
  const win = windowElement();
  const width = sub.offsetWidth;
  let x = el.offsetLeft + el.offsetWidth - 4;
  if (x + width > win.clientWidth - 8) x = el.offsetLeft - width + 4;
  x = Math.max(8, Math.min(x, win.clientWidth - 8 - width));
  // The menu's border and padding, and the submenu's, cancel out but for
  // the padding above the submenu's first entry.
  let y = el.offsetTop + parent.offsetTop - el.scrollTop - 6;
  y = Math.max(8, Math.min(y, win.clientHeight - 8 - sub.offsetHeight));
  m.subPos = { x, y };
  sub.style.left = x + 'px';
  sub.style.top = y + 'px';
  sub.style.maxHeight = maxHeightBelow(y);
  sub.style.visibility = '';
}

// maxHeightBelow returns the CSS max-height of a window or tab menu whose
// top is at top: the room down to the window's bottom, less a margin: a
// menu taller than that scrolls.
function maxHeightBelow(top) {
  return Math.max(0, windowElement().clientHeight - 8 - top) + 'px';
}

// entryRoom returns the height that a separator and an entry add to a menu,
// with a second line for an entry whose names are long enough to wrap.
function entryRoom(el) {
  const sep = el.querySelector('.menu-sep');
  const item = el.querySelector('.menu-item');
  const label = item && item.querySelector('.menu-label');
  if (!sep || !item) return 0;
  const s = getComputedStyle(sep);
  const height = (n) => (n ? n.getBoundingClientRect().height : 0);
  return Math.ceil(height(sep) + parseFloat(s.marginTop) + parseFloat(s.marginBottom) + height(item) + height(label));
}
