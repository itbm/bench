// The ⋯ menu for a window, a tab's menu, an empty pane's menu, a terminal's
// right-click menu, a slim title bar's layout menu, and the status bar's
// lists of the windows that need attention and of notices and recent
// messages, which are in the title bar while the status bar is hidden.

import { call, setClipboardText } from './bridge.js';
import { act, closeMenu, isDemo, recentMessages, runCommand, store, toast, update } from './state.js';
import {
  activeKey, attentionOf, contentOrigin, currentTab, esc, ICONS, isPage, itemOf, keysOf, labelOf, LAYOUT_MODES, layoutText, leaves,
  markerOf, realPaneOf, sentence, statusMark, statusText, tabsOf, termKey, windowElement,
} from './util.js';
import { makeWebApp, openAdd, splitEmpty } from './palette.js';
import { openEditor } from './editor.js';
import { closeItem, closeLabel, closeTab, closeTabLabel } from './close.js';
import { canRenameItem, startItemRename, startTabRename, useAutomaticTitle } from './rename.js';
import { pageEntries } from './pages.js';
import * as terminals from './terminals.js';

// windowMenuEntries lists what the ⋯ menu offers for an item shown in a pane.
// A Web item's menu has its Page group (pageEntries). For a web page, webApp
// is what Go offers to put a web app of its site in its place
// (WebAppChoice), once it has answered. For an item that wasn't
// ready when the menu opened, status is why (statusLine): the menu's
// first line says so. The jump palette's This window lists the same
// entries (windowRows in palette.js): an entry under a heading names it
// (under), and Zoom pane, which is the pane's, says so (pane).
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
  // A split with an empty pane opens the add dialog for it at once.
  if (d.splitsDisabled) {
    entries.push({ label: 'Show side by side (split right)', keys: right, disabled: true });
    entries.push({ label: 'Split down', keys: down, disabled: true });
    entries.push({ label: 'Split right with an empty pane', disabled: true });
    entries.push({ label: 'Split down with an empty pane', disabled: true });
    entries.push({ note: d.splitsDisabled });
  } else {
    entries.push({ label: 'Show side by side (split right)', keys: right, run: () => act('MoveItem', itemId, t, paneId, 'right') });
    entries.push({ label: 'Split down', keys: down, run: () => act('MoveItem', itemId, t, paneId, 'bottom') });
    entries.push({ label: 'Split right with an empty pane', run: () => splitEmpty(paneId, 'right') });
    entries.push({ label: 'Split down with an empty pane', run: () => splitEmpty(paneId, 'bottom') });
  }
  // Zoom pane shows this pane alone for a while, and goes with the splits,
  // which change how the tab's panes are shown too.
  const zoom = zoomEntry(view, paneId);
  if (zoom) entries.push({ label: zoom.zoomed ? 'Show all panes' : 'Zoom pane', keys: zoom.keys, pane: true, run: zoom.run });
  // Move left and Move right move the sub-tab along the strip it's drawn
  // in, as a drag along it does: the "tabs" view's strip holds the whole
  // tab.
  const shown = (d.panes || []).find((p) => (p.items || []).includes(itemId));
  const strip = shown ? shown.items : [];
  const at = strip.indexOf(itemId);
  if (strip.length > 1) {
    entries.push({ sep: true });
    entries.push(at > 0
      ? { label: 'Move left', run: () => act('MoveItemAt', itemId, t, shown.id, at - 1) }
      : { label: 'Move left', disabled: true });
    entries.push(at < strip.length - 1
      ? { label: 'Move right', run: () => act('MoveItemAt', itemId, t, shown.id, at + 2) }
      : { label: 'Move right', disabled: true });
  }
  // While a pane is zoomed, the tab's other panes are out of sight, but
  // still there to move to.
  const tab = currentTab(view);
  const panes = d.zoomed && tab ? leaves(tab.root) : (d.panes || []);
  const others = panes.filter((p) => p.id !== paneId && p.id !== 'compact');
  if (others.length) {
    entries.push({ sep: true }, { heading: 'Move to pane' });
    others.forEach((p) => entries.push({
      label: p.active ? 'With ' + labelOf(view, p.active) : 'Empty pane',
      under: 'Move to pane',
      run: () => act('MoveItem', itemId, t, p.id, 'center'),
    }));
  }
  const item = itemOf(view, itemId);
  const tabs = tabsOf(item);
  entries.push({ sep: true });
  if ((view.tabs || []).length > 1) {
    entries.push({ heading: 'Move to tab' });
    view.tabs.forEach((other, i) => {
      if (i !== t) entries.push({ label: other.name, under: 'Move to tab', run: () => act('MoveItemToTab', itemId, i) });
    });
  }
  // A new tab of the window's own, named after its app, as a drop of its
  // sub-tab on the tabs' + makes.
  entries.push({ label: 'Move to new tab', run: () => act('MoveItemToNewTab', itemId, false) });
  // The same window can be in several tabs: it's shown in whichever is
  // current.
  entries.push({ sep: true });
  view.tabs.forEach((other, i) => {
    if (i !== t && !tabs.includes(i)) entries.push({ label: 'Also show in ' + other.name, run: () => act('AddItemToTab', itemId, i) });
  });
  entries.push({ label: 'Also show in new tab', run: () => act('MoveItemToNewTab', itemId, true) });
  // The pane that shows the item remembers the places of its windows that
  // have closed, out of sight: their apps can open again, or the pane can
  // forget them.
  const waiting = (shown && shown.remembered) || [];
  if (waiting.length) {
    entries.push({ sep: true }, { heading: 'Closed windows' });
    waiting.forEach((r) => {
      if (r.canOpen) entries.push({ label: 'Open ' + r.app, under: 'Closed windows', run: () => act('OpenItem', r.itemId) });
      entries.push({ label: 'Remove ' + r.app + ' from this pane', under: 'Closed windows', run: () => act('RemoveRemembered', shown.id, r.bundleId) });
    });
  }
  // A web page's controls, as a browser's: Reload, Back and Forward, its
  // zoom, find in page, its link, and its camera, microphone, and sound
  // (pages.js).
  entries.push(...pageEntries(item));
  entries.push({ sep: true });
  // A terminal or web page can have a name of its own, which its sub-tab
  // shows in place of the title its shell or page sets, typed in place of
  // that title. An app's window takes its title from the app.
  if (shown && canRenameItem(view, itemId, shown.id)) {
    entries.push({ label: 'Rename…', run: () => startItemRename(itemId, shown.id) });
    if (item.label) entries.push({ label: 'Use automatic title', detail: item.autoTitle || '', run: () => useAutomaticTitle(itemId) });
  }
  // A terminal opens another beside it, in the folder its shell is in now.
  if (item && item.kind === 'builtin') {
    const here = item.here || item.folder || '~';
    entries.push({ label: 'New terminal in ' + here, run: () => act('AddToPane', paneId, { type: 'terminal', folder: here }) });
  }
  if (item && item.status === 'undocked') entries.push({ label: 'Dock again', run: () => act('Redock', itemId) });
  // A minimised window comes back from the Dock, as clicking its sub-tab
  // brings it.
  if (item && item.status === 'minimised') entries.push({ label: 'Show again', run: () => act('ShowItem', paneId, itemId) });
  // A web page opens in the default browser, where its address can be
  // checked and the page used with the browser's own sign-ins.
  if (isPage(item)) entries.push({ label: 'Open in browser', run: () => act('OpenInBrowser', itemId) });
  // A sub-tab with no window has nothing to pop out, so Go forgets it.
  entries.push({ label: item && item.noWindow ? 'Remove from Bench' : 'Pop out of Bench', run: () => act('PopOut', itemId) });
  entries.push(...closeEntries(view, item, t));
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

// closeEntries lists the window menu's entries that close an item: Close
// window, Close page, or Close terminal; or, for one that's in other tabs
// too, Remove from this tab, as its × does, and Close everywhere. A sub-tab
// with no window has Remove from Bench instead. An app's window closes
// through its close button, which needs Accessibility permission.
function closeEntries(view, item, t) {
  if (!item) return [];
  const shared = tabsOf(item).length > 1;
  const entries = [{ sep: true }];
  if (shared) entries.push({ label: 'Remove from this tab', run: () => act('RemoveItemFromTab', item.id, t) });
  if (item.noWindow) return entries.length > 1 ? entries : [];
  const label = shared ? 'Close everywhere' : closeLabel(item);
  const untrusted = item.kind === 'app' && Boolean(view.permissions) && !view.permissions.accessibility && !view.demo;
  if (untrusted) {
    entries.push({ label, disabled: true }, { note: 'Closing an app\'s window needs Accessibility permission.' });
  } else {
    entries.push({ label, run: () => closeItem(item.id) });
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
// own properties, which, unlike Settings, apply to it alone, and its name,
// typed in place (rename.js); moving it along the tabs, which changes its
// shortcut; arranging it; closing or deleting it; and adding a tab, or
// reopening the one closed last. The entries that open a dialog do so in
// the menu's place (opens). Close tab… closes the tab's windows, pages, and
// terminals, asking first where Go would; on the only tab, it's Close all
// windows…, which leaves the tab, and the last tab can't be deleted.
export function tabMenuEntries(view, index) {
  const current = index === view.current;
  const count = (view.tabs || []).length;
  const only = count === 1;
  return withSeparators([
    [
      { label: 'Tab properties…', opens: true, run: () => openEditor(index) },
      // The tab's name turns into a field, with the name selected, to type
      // a new one, as a double-click on the tab does.
      { label: 'Rename…', run: () => startTabRename(index) },
    ],
    only ? [] : [
      index > 0 ? { label: 'Move left', run: () => act('MoveTab', index, index - 1) } : { label: 'Move left', disabled: true },
      index < count - 1 ? { label: 'Move right', run: () => act('MoveTab', index, index + 1) } : { label: 'Move right', disabled: true },
    ],
    [
      { label: 'Rearrange', keys: current ? keysOf(view).rearrange || '' : '', run: () => rearrangeTab(index) },
      // As in Tab properties, Reset layout doesn't ask first.
      { label: 'Reset layout', run: () => act('ResetLayout', index) },
    ],
    [
      { label: closeTabLabel(view), run: () => closeTab(index) },
      ...(only ? [] : [{ label: 'Delete tab (keep windows)', run: () => act('DeleteTab', index) }]),
    ],
    [{ label: 'New tab…', opens: true, run: () => openEditor(-1) }, ...reopenEntries(view)],
  ]);
}

// reopenEntries is Reopen closed tab, for the tab closed or deleted last,
// which it names, if Go keeps one.
function reopenEntries(view) {
  const name = (view.closedTabs || [])[0];
  return name ? [{ label: 'Reopen closed tab', detail: name, run: () => act('ReopenTab', name) }] : [];
}

// paneMenuEntries lists what an empty pane's menu offers: what the buttons
// in its body do, and zooming it. A pane closes only in a tab that has more
// than one, shown or zoomed. Adding to it opens the add dialog for addTo,
// the pane that the add dialog adds to (the focused pane, for the "tabs"
// view's compact pane).
export function paneMenuEntries(view, paneId, addTo) {
  const entries = [{ label: 'Add a window or app…', run: () => openAdd(addTo) }];
  if (panesCanClose(view)) entries.push({ label: 'Close this pane', run: () => act('ClosePane', paneId) });
  const zoom = zoomEntry(view, paneId);
  if (zoom) entries.push({ sep: true }, { label: zoom.zoomed ? 'Show all panes' : 'Zoom pane', keys: zoom.keys, run: zoom.run });
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

// zoomEntry returns what Zoom pane does now, for the menus and the jump
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
  const entries = LAYOUT_MODES.map(([id, label]) => ({ label, checked: view.mode === id, run: () => act('SetMode', id) }));
  entries.push({ sep: true }, { note: sentence(layoutText(view, currentTab(view))) });
  return entries;
}

// openLayoutMenu opens the layout menu below a slim title bar's layout
// button, which stands for the layout control there, with the focus on the
// layout in use.
export function openLayoutMenu(button) {
  store.ui.paneLater = null;
  const o = contentOrigin();
  const r = button.getBoundingClientRect();
  store.ui.menu = {
    kind: 'layout',
    x: menuX(r.left - o.x),
    y: r.bottom - o.y + 4,
    opener: button.dataset.key || null,
  };
  const view = store.view;
  store.ui.pendingFocus = 'menu-item-' + Math.max(0, LAYOUT_MODES.findIndex(([id]) => view && id === view.mode));
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

// openTerminalMenu opens a terminal's menu (terminals.menuEntries), for a
// right-click in the terminal (host, its element), shown in pane paneId: at
// the pointer (at, in content coordinates), or at the terminal's top left
// without one. The terminal gets the keyboard back when it closes.
export function openTerminalMenu(host, itemId, paneId, at = null) {
  const view = store.view;
  store.ui.paneLater = null;
  const o = contentOrigin();
  const r = host.getBoundingClientRect();
  store.ui.menu = {
    kind: 'terminal',
    itemId,
    paneId,
    shownIn: paneId,
    tab: view.current,
    x: menuX(at ? at.x : r.left - o.x + 8),
    y: at ? at.y : r.top - o.y + 8,
    opener: termKey(itemId),
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
      + ' style="' + listStyle(m) + '">'
      + '<div class="menu-heading" role="presentation">Need attention</div>' + renderEntries(entries) + '</div>';
  }
  if (m.kind === 'layout') {
    const entries = layoutMenuEntries(view);
    ui.menuEntries = entries;
    // The note, why the layout is in use, describes the menu.
    return '<div class="menu" role="menu" aria-label="Layout" id="layout-menu" aria-describedby="menu-note-' + entries.findIndex((e) => e.note) + '"'
      + ' data-scroll="layout-menu" style="' + menuStyle(m) + '">' + renderEntries(entries) + '</div>';
  }
  if (m.kind === 'tab') {
    const tab = view.tabs[m.index];
    if (!tab) return '';
    const entries = tabMenuEntries(view, m.index);
    ui.menuEntries = entries;
    return '<div class="menu" role="menu" aria-label="' + esc('Options for the ' + tab.name + ' tab') + '" id="tab-menu"'
      + ' data-scroll="tab-menu" style="' + menuStyle(m) + '">' + renderEntries(entries) + '</div>';
  }
  if (m.kind === 'terminal') {
    const entries = terminals.menuEntries(m.itemId);
    if (!entries.length) return '';
    ui.menuEntries = entries;
    return renderTerminalMenu(entries, menuStyle(m));
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
// the one chosen. Entries in a row (row) are buttons side by side, each
// with its icon, or its text, and its label as its tooltip, after the row's
// name (under), with a gap before one that starts a group of its own (gap):
// a web page's Back, Forward, Reload, and zooms (pages.js).
function renderEntries(entries) {
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
    const key = e.disabled ? 'menu-disabled-' + i : 'menu-item-' + (n++);
    if (e.row) {
      const first = !entries[i - 1] || entries[i - 1].row !== e.row;
      const last = !entries[i + 1] || entries[i + 1].row !== e.row;
      const name = e.label + (e.detail ? ' (' + e.detail.toLowerCase() + ')' : '');
      return (first ? '<div class="menu-row" role="group" aria-label="' + esc(e.under || '') + '">'
          + '<span class="menu-row-label" aria-hidden="true">' + esc(e.under || '') + '</span>' : '')
        + (e.gap ? '<span class="menu-row-gap" aria-hidden="true"></span>' : '')
        + '<button type="button" class="menu-item menu-icon' + (e.text ? ' text' : '') + '" role="menuitem" data-action="menu-run" data-index="' + i + '"'
        + ' data-key="' + key + '" aria-label="' + esc(name) + '" title="' + esc(e.label) + '"' + (e.disabled ? ' disabled' : '') + '>'
        + (e.text ? esc(e.text) : e.icon) + '</button>'
        + (last ? '</div>' : '');
    }
    const label = e.detail
      ? '<span class="menu-label stacked"><span class="menu-main">' + esc(e.label) + '</span><span class="menu-detail">' + esc(e.detail) + '</span></span>'
      : '<span class="menu-label' + (e.wrap ? ' wrap' : '') + '">' + esc(e.label) + '</span>';
    const choice = typeof e.checked === 'boolean';
    const role = choice ? 'menuitemradio" aria-checked="' + e.checked : 'menuitem';
    const tick = choice ? '<span class="menu-tick">' + (e.checked ? ICONS.check : '') + '</span>' : '';
    return '<button type="button" class="menu-item" role="' + role + '" data-action="menu-run" data-index="' + i + '" data-key="' + key + '"'
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
// came, its whole text, and Copy. A notice's button calls the bound method
// that Go names for it (runNoticeAction).
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
      + '<button type="button" class="btn small" data-action="message-copy" data-index="' + i + '" data-key="message-' + i + '"'
      + ' aria-label="' + esc('Copy the message from ' + clockTime(msg.time)) + '">Copy</button></div>'
      + '<div class="message-text">' + esc(msg.text) + '</div></div>').join('')
    + '</div></div></div>';
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
export function fitMenu() {
  const el = document.querySelector('#menu-root .menu');
  const m = store.ui.menu;
  if (!el || !m || !['window', 'tab', 'pane', 'terminal', 'layout'].includes(m.kind) || m.top != null) return;
  const win = windowElement();
  const top = el.offsetTop;
  const room = m.kind === 'window' && !m.webApp && asksWebApp(itemOf(store.view, m.itemId)) ? entryRoom(el) : 0;
  const over = top + el.offsetHeight + room - (win.clientHeight - 8);
  m.top = over > 0 ? Math.max(8, top - over) : top;
  el.style.top = m.top + 'px';
  el.style.maxHeight = maxHeightBelow(m.top);
}

// maxHeightBelow returns the CSS max-height of a window or tab menu whose
// top is at top: the room down to the window's bottom, less a margin. A
// web page's menu, with its Page group, needs most of a laptop's window
// (decision 50).
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
