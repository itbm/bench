// The jump palette and the add dialog. They share their markup and keys: a
// search field, grouped rows, arrows to move, Return to choose, and Escape
// or the backdrop to close. The add dialog's Web page… asks for an address,
// and may then wait while a browser makes a web app of the site. A web
// page's menu opens the add dialog at that step too, to put the web app in
// the page's place. Its New terminal… asks for a command, a name, and a
// folder (launchers.js), as does the row it offers for text that matches
// nothing. Its Browse… closes it for macOS's open panel. The add dialog
// lists launchers first, and the jump palette after the tabs and windows.
// The add dialog's windows can be ticked, to add several at once.

import { call, errorText } from './bridge.js';
import {
  act, closeOverlay, isDemo, openerKey, overlayOpen, runPaneLater, shownCurrent, store, switchTab, toast, update,
} from './state.js';
import {
  activeKey, contentRect, currentTab, esc, focusedPaneID, ICONS, itemOf, keysOf, labelOf, LAYOUT_MODES, leaves, realPaneOf, sentence,
  shownInFocus, undoHint,
} from './util.js';
import { openEditor } from './editor.js';
import { openOverview } from './overview.js';
import { GRIP } from './dialogs.js';
import { openMessages, openWindowMenu, panesCanClose, reopenItemLabel, windowMenuEntries, zoomEntry } from './menu.js';
import { openSettings } from './settings.js';
import { outsideTabs } from './fromwindows.js';
import { closeTab, closeTabLabel } from './close.js';
import { launcherItems, newTerminalIn, openTerminalForm, renderTerminalStep, runItem } from './launchers.js';
import { showGuide } from './guide.js';
import { askDiscard, dismissOverlay } from './discard.js';
import { tile } from './tiles.js';
import { openShortcuts } from './shortcuts.js';

// openPalette opens the jump palette.
export function openPalette() {
  const { ui } = store;
  if (!ui.overlay) ui.opener = openerKey();
  ui.menu = null;
  ui.paneLater = null;
  ui.overlay = 'palette';
  ui.add = null;
  ui.editor = null;
  ui.settings = null;
  // The row selected is chosen once the rows are drawn (renderRows).
  ui.palette = { query: '', sel: null };
  ui.pendingFocus = 'palette-query';
  update();
}

// togglePalette opens the jump palette, or closes it if it's open.
export function togglePalette() {
  if (store.ui.overlay === 'palette') closeOverlay();
  else openPalette();
}

// openAdd opens the add dialog for a pane, with its rows from Go. If Go
// refuses, the click that asked for it focuses its pane now
// (ui.paneLater). With split, the pane is the empty one that splitEmpty
// made for the dialog, which goes again if nothing is added to it.
export async function openAdd(paneId, split = false) {
  const opener = activeKey();
  const tab = store.view.current;
  const r = await act('AddOptions', paneId);
  if (!r.ok) {
    if (split) act('CancelSplit', paneId);
    runPaneLater();
    return;
  }
  const { ui } = store;
  if (!ui.overlay) ui.opener = opener;
  ui.menu = null;
  ui.editor = null;
  ui.settings = null;
  ui.overlay = 'add';
  ui.add = {
    paneId, tab, options: Array.isArray(r.value) ? r.value : [], step: 'list', option: null, url: '', make: null, busy: false,
    // ticked holds the IDs of the windows ticked, to add together.
    ticked: [],
  };
  if (split) {
    ui.add.split = 'new';
    splitAdd = ui.add;
  }
  ui.palette = { query: '', sel: 0 };
  ui.pendingFocus = 'palette-query';
  update();
}

// splitAdd is the add dialog that opened for the empty pane that a split
// made (splitEmpty), while it's open.
let splitAdd = null;

// splitEmpty puts an empty pane beside a pane, on the side zone gives
// (SplitPane), and opens the add dialog for it at once: a window menu's
// Split Right with an Empty Pane and Split Down with an Empty Pane.
export async function splitEmpty(paneId, zone) {
  const r = await act('SplitPane', paneId, zone);
  if (r.ok && r.value) await openAdd(r.value, true);
}

// syncSplit has Go take the empty pane that a split made away again
// (CancelSplit) once its add dialog has closed, however it closed. Go
// keeps the pane if something was added to it, or a new window is on its
// way there, so it waits for the dialog's calls that may add something
// (adding): a row that closes the dialog makes its call just after
// (runListItem).
export function syncSplit() {
  const add = splitAdd;
  if (!add || store.ui.add === add) return;
  splitAdd = null;
  queueMicrotask(async () => {
    await Promise.all(add.calls || []);
    act('CancelSplit', add.paneId);
  });
}

// adding returns a call that may add to the add dialog's pane, and keeps
// it on the dialog while it's under way, for syncSplit.
export function adding(add, p) {
  if (!add.split) return p;
  add.calls = (add.calls || []).concat(p);
  return p;
}

// makeWebApp has Go open a web page's site in the default browser, for the
// user to make a web app of it there (MakeWebApp), and opens the add
// dialog's make step for it, which waits for the web app and puts it in the
// page's place. If Go refuses, it says why, and nothing opens. The dialog
// doesn't open if the page has gone or Go has switched tabs meanwhile.
export async function makeWebApp(itemId, paneId) {
  const opener = activeKey();
  const tab = store.view.current;
  const item = itemOf(store.view, itemId);
  const page = item ? item.app : 'the page';
  const r = await act('MakeWebApp', itemId);
  if (!r.ok) return;
  const { ui, view } = store;
  const now = itemOf(view, itemId);
  if (view.current !== tab || !now || now.tab !== tab) return;
  const result = r.value || {};
  if (!ui.overlay) ui.opener = opener;
  ui.menu = null;
  ui.editor = null;
  ui.settings = null;
  ui.overlay = 'add';
  ui.add = {
    paneId, tab, options: [], step: 'make', option: null, url: '', busy: false, ticked: [],
    make: { url: '', browser: result.browser || 'your browser', steps: result.steps || '' },
    replace: { itemId, page },
  };
  ui.palette = { query: '', sel: 0 };
  ui.pendingFocus = 'make-cancel';
  update();
  waitForWebApp(ui.add, ui.add.make);
}

// showWindow switches to an item's tab if needed, then shows it in its pane:
// if the title bar doesn't show it as current (shownCurrent), as while a
// switch to another tab is on its way (decision 67).
async function showWindow(view, tabIndex, paneId, itemId) {
  if (tabIndex !== shownCurrent(view)) {
    const r = await switchTab(tabIndex);
    if (!r.ok) return;
  }
  await act('ShowItem', paneId, itemId);
}

// openWindowOptions opens the ⋯ menu of the item that a pane shows, in the
// jump palette's place, so that Bench stays raised for a menu drawn in
// HTML (a native menu doesn't need it): below the pane's ⋯ button, which
// the focus goes back to as the menu closes, or, for a pane with no
// header, inside its top right corner, and the focus goes back to where it
// was before the palette opened.
function openWindowOptions(paneId, itemId) {
  const { ui } = store;
  const opener = ui.opener;
  ui.overlay = null;
  ui.opener = null;
  ui.listItems = [];
  const button = document.querySelector('[data-action="menu"][data-pane="' + CSS.escape(paneId) + '"]');
  if (button) {
    openWindowMenu(button, itemId, paneId);
    return;
  }
  const el = document.querySelector('.pane[data-pane-id="' + CSS.escape(paneId) + '"]');
  if (!el) {
    update();
    return;
  }
  // As below a ⋯ button, the menu starts 260 points left of the right end,
  // here 4 points inside the pane.
  const r = contentRect(el);
  openWindowMenu(el, itemId, paneId, { x: r.x + r.w - 4 - 260, y: r.y + 4 }, opener);
}

// paneHint names the focused pane in the jump palette by the window it
// shows, or says that it's empty.
function paneHint(view) {
  const shown = shownInFocus(view);
  return shown ? labelOf(view, shown) : 'Empty pane';
}

// paletteItems lists the jump palette's rows, in group order. A window's
// row shows its title, and is found by its app's name too, in its whole
// title (labelOf), which filterItems counts as its label. Launchers
// come after the tabs and windows, unlike in the add dialog: Return with
// nothing typed, or a tab's name typed, jumps, and doesn't start a command.
// The tab used before this one comes first, so that Return goes back to it
// at once, as the last_tab shortcut does; the other tabs keep their order
// in the tab strip, which their shortcuts number. Windows come in the order
// they were last used (ItemView.used), the latest first.
export function paletteItems(view) {
  const items = [];
  const keys = keysOf(view);
  // Undo for the latest change, first, as Tab ▸ Undo names it, with the
  // undo shortcut. Return with nothing typed passes it by (renderRows).
  // It's found by "undo", and by the change's name only as a hint is
  // (findAs): "close page" then finds Close Page before Undo Close Page.
  const undo = view.undo;
  if (undo && undo.name) {
    items.push({
      group: 'Last change', label: 'Undo ' + undo.name, hint: undoHint(undo), keys: keys.undo || '', passOver: true,
      findAs: { label: 'Undo', hint: undo.name + ' · ' + undoHint(undo) },
      run: () => act('UndoLatest'),
    });
  }
  const last = view.lastTab >= 0 && view.lastTab !== view.current && view.tabs[view.lastTab] ? view.lastTab : -1;
  const tabRow = (t, i) => ({
    group: 'Tabs', mono: String(i + 1), label: t.name,
    hint: (t.apps || []).join(', ') + (i === view.current ? ' · current' : ''),
    keys: t.shortcut || '', run: () => switchTab(i),
  });
  if (last >= 0) {
    const row = tabRow(view.tabs[last], last);
    items.push({ ...row, hint: row.hint + ' · last used', keys: keys.lastTab || row.keys });
  }
  view.tabs.forEach((t, i) => { if (i !== last) items.push(tabRow(t, i)); });
  const windows = [];
  view.tabs.forEach((t, i) => leaves(t.root).forEach((p) => (p.items || []).forEach((id) => {
    const it = itemOf(view, id);
    // A sub-tab whose window has closed is out of sight until it opens again.
    if (!it || it.hidden) return;
    windows.push({
      group: 'Windows', mono: it.mono, icon: it.icon, label: it.title || it.app, title: labelOf(view, id), hint: t.name + ' · ' + it.app, keys: '', itemId: id,
      used: it.used || 0, run: () => showWindow(view, i, p.id, id),
    });
  })));
  // The sort keeps the tabs' order for windows used as long ago.
  items.push(...windows.sort((a, b) => b.used - a.used));
  // The drop-down has Show Drop-down, below, and Move into This Pane, in
  // This window.
  outsideTabs(view).forEach((it) => items.push({
    group: 'Not in a tab', mono: it.mono, icon: it.icon, label: it.title || it.app, title: labelOf(view, it.id), hint: it.app + ' · add to this tab', keys: '',
    itemId: it.id, run: () => act('AddToPane', focusedPaneID(view), { type: 'floating', id: it.id }),
  }));
  items.push(...launcherItems(view));
  items.push(...windowRows(view));
  items.push(...actionRows(view));
  return items;
}

// windowRows lists the jump palette's This window group: what the ⋯ menu
// offers for the window that the focused pane shows (windowMenuEntries), so
// that the two can't differ, each with its shortcut. An entry in a submenu
// there, or with under, is named with it, as "Move to Tab ▸ Comms" and
// "Page ▸ Find in Page…". Entries that are off are left out, and so is
// Zoom Pane, which is the pane's: Actions has it, as an empty pane has it
// too. Window Options… opens the menu itself, for what joins it once Go
// answers, such as a web page's web app.
function windowRows(view) {
  const shown = shownInFocus(view);
  if (!shown) return [];
  const d = view.display || {};
  const hint = labelOf(view, shown);
  const rows = windowMenuEntries(view, shown, realPaneOf(view, shown, d.focus))
    .flatMap((e) => (e.submenu ? e.submenu.map((s) => ({ ...s, under: e.label })) : [e]))
    .filter((e) => e.label && e.run && !e.disabled && !e.pane)
    .map((e) => ({ group: 'This window', label: e.under ? e.under + ' ▸ ' + e.label : e.label, hint, keys: e.keys || '', run: e.run }));
  rows.push({ group: 'This window', label: 'Window Options…', hint, keys: '', stay: true, run: () => openWindowOptions(d.focus, shown) });
  return rows;
}

// actionRows lists the jump palette's Actions: for the focused pane, the
// current tab, every tab, and Bench. Each has the name that the menu bar
// gives the same command, in title case, and a hint, in sentence case,
// that says what it's for.
function actionRows(view) {
  const items = [];
  const add = (label, hint, run, more = {}) => items.push({ group: 'Actions', label, hint, keys: '', run, ...more });
  const keys = keysOf(view);
  const tab = currentTab(view);
  const cur = tab ? tab.name : '';
  // The overview of every tab, as View ▸ Show All Tabs opens it. "overview"
  // and "cards" find it too.
  add('Show All Tabs', 'Each tab and its windows at once', () => openOverview(), { keys: keys.showAllTabs || '', stay: true, title: 'show all tabs overview cards' });
  add('Rearrange', cur ? 'Arrange ' + cur + ' again' : '', () => act('Rearrange'), { keys: keys.rearrange || '' });
  // The drop-down terminal, as View ▸ Show Drop-down shows or hides it, or
  // makes one in the home folder. "dropdown" finds it too.
  const drop = view.dropdown;
  add(drop && drop.shown ? 'Hide Drop-down' : 'Show Drop-down', drop ? labelOf(view, drop.id) : 'A new terminal that slides down over any app',
    () => act('ToggleDropdown'), { keys: keys.dropdown || '', title: 'show hide drop-down dropdown' });
  // The focused pane can fill the tab for now, as the zoom_pane shortcut
  // makes it, and the tab can show every pane again.
  const zoom = zoomEntry(view);
  if (zoom && zoom.zoomed) add('Show All Panes', cur, zoom.run, { keys: zoom.keys });
  else if (zoom) add('Zoom Pane', paneHint(view), zoom.run, { keys: zoom.keys });
  // The keys that arrange the panes, as the Pane and Tab menus have them:
  // only the rows that Go says would do something now (display.arrange).
  // The hint of Next Layout and Previous Layout is the layout it gives.
  // The rows that resize have more words to be found by, in a title that
  // isn't shown, which filterItems counts as the label: "resize" or
  // "wider" finds them too.
  const arrange = (view.display && view.display.arrange) || {};
  if (arrange.grow) add('Grow Pane', paneHint(view), () => act('GrowPane'), { keys: keys.growPane || '', title: 'resize larger bigger wider taller' });
  if (arrange.shrink) add('Shrink Pane', paneHint(view), () => act('ShrinkPane'), { keys: keys.shrinkPane || '', title: 'resize smaller narrower shorter' });
  if (arrange.equal) add('Make Panes Equal', cur, () => act('EqualPanes'), { keys: keys.equalPanes || '', title: 'resize same size even' });
  if (arrange.nextLayout) add('Next Layout', arrange.nextLayout, () => act('StepLayout', 1), { keys: keys.nextLayout || '' });
  if (arrange.previousLayout) add('Previous Layout', arrange.previousLayout, () => act('StepLayout', -1), { keys: keys.previousLayout || '' });
  // What a pane's +, its header's right-click menu, and Pane ▸ Close Pane
  // do, for a pane with no header too (pane_headers), whose window's own
  // title bar takes its place. A pane closes only once it's empty.
  const pane = focusedPaneID(view);
  if (pane) add('Add to Pane…', 'A window, app, web page, or terminal in this pane', () => openAdd(pane), { stay: true });
  // The window that was in front before Bench, if it's in no tab
  // (view.front): one popped out, one opened elsewhere, or one dragged out
  // of a pane. The add_front_window shortcut and Pane ▸ Add Front Window
  // do it too.
  if (pane && view.front) {
    add('Add Front Window', labelOf(view, view.front) || 'The window in front', () => act('AddFrontWindow'), { keys: keys.addFrontWindow || '' });
  }
  // Two of the add dialog's rows, straight from here: the dialog opens at
  // its New terminal… form, or at Web page…'s address.
  if (pane) {
    add('New Terminal…', 'In this pane: a command, a name, and a folder', async () => {
      await openAdd(pane);
      if (store.ui.add && store.ui.add.step === 'list') openTerminalForm();
    }, { stay: true });
    add('Add Web Page…', 'In this pane: a website, as a web app or in Bench', async () => {
      await openAdd(pane);
      const o = store.ui.add && store.ui.add.step === 'list' && store.ui.add.options.find((x) => x.asksForUrl);
      if (o) askForURL(o);
    }, { stay: true });
  }
  const d = view.display || {};
  if (pane && !shownInFocus(view) && panesCanClose(view)) add('Close Pane', paneHint(view), () => act('ClosePane', d.focus));
  // Closing the tab asks first, as it does from the tab's menu.
  if (tab) add(closeTabLabel(view), cur, () => closeTab(view.current));
  if (tab && view.tabs.length > 1) add('Delete Tab (Keep Windows)', cur, () => act('DeleteTab', view.current));
  // The browser preview has no window of its own to fill the screen with.
  if (!isDemo()) add('Fill the Screen', 'Zoom Bench\'s window', () => act('FillScreen'));
  // Every tab's properties, the current tab's first.
  const props = (t, i) => add('Tab Properties…', t.name + ': name, layout, small screens', () => openEditor(i), { stay: true });
  if (tab) props(tab, view.current);
  view.tabs.forEach((t, i) => { if (i !== view.current) props(t, i); });
  // Moving the tab along the tabs changes its shortcut.
  const at = view.current;
  if (tab && at > 0) add('Move Tab Left', cur, () => act('MoveTab', at, at - 1));
  if (tab && at < view.tabs.length - 1) add('Move Tab Right', cur, () => act('MoveTab', at, at + 1));
  add('New Tab…', '', () => openEditor(-1), { stay: true });
  add('New Tab from Open Windows…', 'Tick the windows for a new tab', () => openEditor(-1, { start: 'windows' }), { stay: true });
  // The tabs that Go keeps for Reopen Closed Tab, the latest first, and
  // the web pages and terminals for Reopen Closed Page or Terminal, each
  // with its app, as Windows names it, and the tab it was in.
  (view.closedTabs || []).forEach((name) => add('Reopen Closed Tab', name, () => act('ReopenTab', name)));
  (view.closedItems || []).forEach((c) => add(reopenItemLabel(c), c.label + (c.tab ? ' · ' + c.tab : ''), () => act('ReopenItem', c.id)));
  add('Settings…', 'For every tab', () => openSettings(), { stay: true });
  add('Reload config.toml', 'Use changes made to the file', () => act('ReloadConfig'));
  // The toasts and notices of late, to read again or copy.
  add('Recent Messages', 'Read or copy what Bench said', () => openMessages());
  // Help, as the Help menu has it.
  add('Keyboard Shortcuts', 'Help: what each shortcut does', () => openShortcuts(), { stay: true });
  add('Show Welcome Guide', 'Help: permissions and tips', () => showGuide(), { stay: true });
  // The layouts, as View ▸ Layout has them.
  LAYOUT_MODES.forEach(([id, name]) => {
    if (view.mode !== id) add('Layout ▸ ' + name, LAYOUT_HINTS[id], () => act('SetMode', id));
  });
  return items;
}

// LAYOUT_HINTS say what each layout does, for the jump palette.
const LAYOUT_HINTS = {
  auto: 'Compact or wide, by the width of Bench\'s window',
  wide: 'Use the wide layout',
  compact: 'Use the compact layout',
};

// addItems lists the add dialog's rows, from AddOptions. Web page… asks
// for an address, New terminal… for a command, a name, and a folder, New
// terminal in… for a folder with the folder picker (newTerminalIn), and
// Browse… closes the dialog itself (browse). A window's row, not in a tab
// or in another tab, can be ticked (tick, its ID), to add several.
export function addItems(ui) {
  const add = ui.add;
  if (!add) return [];
  return add.options.map((o) => ({
    group: o.group, mono: o.mono, icon: o.icon, label: o.label, hint: o.hint || '', keys: '',
    tick: o.source && (o.source.type === 'floating' || o.source.type === 'item') ? o.source.id : '',
    matchLabel: Boolean(o.source && o.source.type === 'launcher'),
    stay: Boolean(o.asksForUrl || o.asksForCommand || o.browses || o.choosesFolder),
    run: () => {
      if (o.asksForUrl) askForURL(o);
      else if (o.asksForCommand) openTerminalForm();
      else if (o.choosesFolder) newTerminalIn(o);
      else if (o.browses) browse(add);
      else adding(add, act('AddToPane', add.paneId, o.source));
    },
  }));
}

// browse is the add dialog's Browse…. The dialog closes first, which
// lowers Bench, and Go shows macOS's open panel, a window of its own, for
// the user to choose an app, or a program or script (BrowseToAdd). Go adds
// an app to the pane. For a program or script, the dialog opens again at
// its New terminal… form, filled in for the user to check, and if the user
// cancels, at its list; but not if the tab has changed meanwhile, or
// another dialog or menu has opened. An empty pane that a split made for
// the dialog stays while the panel is open: the dialog that opens again
// takes it over, or else it goes, as it would have, if nothing was added.
async function browse(add) {
  const { ui } = store;
  const split = splitAdd === add;
  if (split) splitAdd = null;
  ui.paneLater = null;
  closeOverlay();
  const r = await act('BrowseToAdd', add.paneId);
  const result = (r.ok && r.value) || {};
  const again = r.ok && !result.added && store.view.current === add.tab && !overlayOpen();
  if (!again) {
    if (split) act('CancelSplit', add.paneId);
    if (result.command) toast('Bench didn\'t open the New terminal form, as you switched tabs or opened something else meanwhile. Use Browse… again.');
    return;
  }
  await openAdd(add.paneId, split);
  const now = store.ui.add;
  if (result.command && now && now.paneId === add.paneId && now.step === 'list') openTerminalForm(result.command, result.folder || '', result.name || '');
}

function askForURL(option) {
  const { ui } = store;
  if (!ui.add) return;
  ui.add.step = 'url';
  ui.add.option = option;
  ui.add.url = '';
  ui.add.urlError = '';
  ui.add.make = null;
  ui.pendingFocus = 'palette-url';
  update();
}

// backToList leaves the add dialog's address step or New terminal… form, or
// stops waiting for a web app, and goes back to the list: Escape and Back.
// If the web app is added just then, the dialog closes instead (see
// settlePoll). An address or a form with something typed asks first, and
// Discard goes back to the list without it (discard.js). A dialog that a
// web page's menu opened has no list: it closes, after asking in the same
// way, and the page stays.
export async function backToList() {
  const { ui } = store;
  const add = ui.add;
  if (!add) return;
  if (add.step === 'make') {
    if (add.busy) return;
    add.busy = true;
    const added = await settlePoll(add);
    add.busy = false;
    if (added || ui.add !== add) return;
  }
  if (add.replace) {
    dismissOverlay();
    return;
  }
  if (!askDiscard(() => toList(add), true)) toList(add);
}

// toList shows the add dialog's list again, if the dialog is still open.
// What the New terminal… form had goes, and the address step starts afresh
// when it's chosen again (askForURL).
function toList(add) {
  const { ui } = store;
  if (ui.add !== add) return;
  add.step = 'list';
  add.make = null;
  add.terminal = null;
  ui.pendingFocus = 'palette-query';
  update();
}

// settlePoll waits for the make step's call to AddWebAppWhenReady (or
// UseWebAppWhenReady), if one is under way, and reports whether it added
// the web app, which waitForWebApp then says, closing the dialog. Whoever
// calls it sets add.busy first, so that no other call starts meanwhile, and
// a choice to stop waiting can't add a second item, or leave the step as
// the web app is added.
async function settlePoll(add) {
  const make = add.step === 'make' ? add.make : null;
  return Boolean(make && make.poll && await make.poll);
}

// POLL_MS is how often the make step asks Go whether the web app is ready.
const POLL_MS = 1500;

// typedURL returns the address the add dialog adds, or '' after asking for
// one, under the field: the one the make step waits for, or else the one
// typed.
function typedURL(add) {
  const url = (add.step === 'make' && add.make ? add.make.url : add.url).trim();
  if (!url) urlRefused(add, 'Type a web address, such as example.com.');
  return url;
}

// addAddress makes Go's call p, which adds the typed address, and answers
// as act does. Go's refusal, such as of an address that isn't a web
// address, shows under the address field, for the user to fix, while the
// dialog still asks for it; or else as a toast.
async function addAddress(add, p) {
  const r = await adding(add, p.then((value) => ({ ok: true, value }), (error) => ({ ok: false, error })));
  if (!r.ok) urlRefused(add, errorText(r.error));
  return r;
}

// urlRefused shows why the add dialog's address can't be added (text):
// under the address field, while the dialog still asks for it, or else as
// a toast. The field gets the keyboard, from the Add button too, with its
// text as it was where it had it already. Typing in the field clears it.
function urlRefused(add, text) {
  const { ui } = store;
  if (ui.add !== add || add.step !== 'url') {
    toast(text);
    return;
  }
  add.urlError = text;
  if (!document.activeElement || document.activeElement.id !== 'palette-url') ui.pendingFocus = 'palette-url';
  update();
}

// typeURL keeps the address typed so far, and clears what was wrong with
// the last one.
export function typeURL(text) {
  const add = store.ui.add;
  if (!add) return;
  add.url = text;
  if (!add.urlError) return;
  add.urlError = '';
  update();
}

// submitURL adds the website at the typed address. With inBench (Option-
// Return), it's a web page that Bench shows itself, as showInBench adds.
// Otherwise Go adds the site's web app if there's one, or else asks the
// default browser to make one, and the dialog waits for it (the make step),
// or, if Bench can't dock web apps or the browser can't make them, adds a
// web page that Bench shows (AddWebsite). The dialog stays open if Go
// refuses the address, so that it can be fixed.
export async function submitURL(inBench = false) {
  const { ui } = store;
  const add = ui.add;
  if (!add || !add.option || add.step !== 'url' || add.busy) return;
  if (inBench) {
    await showInBench();
    return;
  }
  const url = typedURL(add);
  if (!url) return;
  add.busy = true;
  const r = await addAddress(add, call('AddWebsite', add.paneId, url));
  add.busy = false;
  if (!r.ok) return;
  const result = r.value || {};
  if (result.added) {
    // Adding focuses the pane. The note says what was added, even if the
    // dialog has closed meanwhile.
    if (ui.add === add) {
      ui.paneLater = null;
      closeOverlay();
    }
    toast(result.note);
    return;
  }
  if (ui.add !== add || add.step !== 'url') return;
  add.step = 'make';
  add.make = { url, browser: result.browser || 'your browser', steps: result.steps || '' };
  ui.pendingFocus = 'make-cancel';
  update();
  waitForWebApp(add, add.make);
}

// showInBench adds the address as a web page that Bench shows itself, for
// Option-Return and the Show in Bench buttons.
export async function showInBench() {
  const { ui } = store;
  const add = ui.add;
  if (!add || !add.option || add.step === 'list' || add.busy) return;
  const url = typedURL(add);
  if (!url) return;
  add.busy = true;
  if (await settlePoll(add)) return;
  const r = await addAddress(add, call('AddToPane', add.paneId, { ...add.option.source, url }));
  add.busy = false;
  if (r.ok && ui.add === add) {
    ui.paneLater = null;
    closeOverlay();
  }
}

// waitForWebApp asks Go every POLL_MS, one call at a time, whether the web
// app that the make step waits for is installed, which adds it to the pane
// (AddWebAppWhenReady), or puts it in the web page's place
// (UseWebAppWhenReady), until it is, or the dialog leaves the step. The
// user is in the browser meanwhile, so the dialog stays open when Bench
// isn't the active app. It doesn't ask while the user's choice to stop
// waiting is under way (add.busy), and that choice waits for the call
// under way, make.poll (see settlePoll). A refusal shows once, not after
// every try. The dialog closes if the web page it replaces goes (see
// dropStale in render.js).
async function waitForWebApp(add, make) {
  const waiting = () => store.ui.add === add && add.step === 'make' && add.make === make;
  const ask = () => (add.replace
    ? call('UseWebAppWhenReady', add.replace.itemId)
    : call('AddWebAppWhenReady', add.paneId, make.url));
  let refused = '';
  while (waiting()) {
    await new Promise((r) => setTimeout(r, POLL_MS));
    if (!waiting()) return;
    if (add.busy) continue;
    make.poll = adding(add, ask().then(Boolean, (err) => {
      const text = errorText(err);
      if (text !== refused) toast(text);
      refused = text;
      return false;
    }));
    const ready = await make.poll;
    make.poll = null;
    if (!ready) continue;
    // A web app that's ready as the dialog closes is still added. Go may
    // have added one that another browser made, so the toast names none.
    if (waiting()) {
      store.ui.paneLater = null;
      closeOverlay();
    }
    toast(add.replace ? 'Replaced ' + add.replace.page + ' with its web app.' : 'Added the web app.');
    return;
  }
}

// filterItems keeps Jump to…'s rows that have every word of the query, in
// any order, in their label and hint together, and ranks them: a word at
// the start of the label counts most, then at the start of one of its
// words, anywhere in it, at the start of a word in the hint, and anywhere
// in the hint. A row's title, if it has one, counts as its label does.
// Rows keep their groups, and the group with the best row comes first;
// rows and groups that rank the same keep their order. If no row has every
// word, a word may also have letters left out, as "termnal" is "terminal"
// with an i missing: it's then found in a word that starts with its first
// letter. A launcher's row (matchLabel) is kept by its name only: its hint
// has its command and folder, which would bring it up for a word such as
// "code" in a tab's or window's name, and Return would then start the
// launcher. A row with findAs is found by that label and hint instead of
// those it shows. The add dialog has addFilter instead.
export function filterItems(items, query) {
  const words = wordsOf(query);
  if (!words.length) return items;
  let found = scoreItems(items, words, false);
  if (!found.length) found = scoreItems(items, words, true);
  const best = new Map();
  found.forEach((f) => best.set(f.item.group, Math.max(best.get(f.item.group) ?? 0, f.score)));
  const groups = [...best.keys()];
  const rank = (f) => [-best.get(f.item.group), groups.indexOf(f.item.group), -f.score, f.index];
  return found.map((f) => ({ item: f.item, key: rank(f) })).sort((a, b) => compareKeys(a.key, b.key)).map((f) => f.item);
}

// wordsOf returns a query's words, folded (see fold). A word with no
// letter or digit, such as "." or "-", is left out, as it's found almost
// anywhere, unless the query has nothing else.
function wordsOf(query) {
  const words = fold(query).split(/\s+/).filter(Boolean);
  const real = words.filter((w) => WORD_CHAR.test(w));
  return real.length ? real : words;
}

// addFilter keeps the add dialog's rows whose label or hint has the query
// as typed, whole, in any case and without accents. Words in any order, as
// in Jump to…, would find a window for a command such as "code .", in
// another tab's VS Code window titled "palette.js — bench", and Return
// would then add that window instead of running the command, which the
// dialog offers only when nothing matches. A launcher's row (matchLabel)
// is kept by its name only, as in filterItems.
function addFilter(items, query) {
  const q = fold(query.trim());
  if (!q) return items;
  return items.filter((it) => fold(it.label).includes(q) || (!it.matchLabel && fold(it.hint).includes(q)));
}

// compareKeys compares two lists of numbers, the first that differ deciding.
function compareKeys(a, b) {
  for (let i = 0; i < a.length; i++) {
    if (a[i] !== b[i]) return a[i] - b[i];
  }
  return 0;
}

// scoreItems returns the rows that have every word, each with its score
// and index, the higher the better (see filterItems). With fuzzy, a word
// may also have letters left out.
function scoreItems(items, words, fuzzy) {
  const found = [];
  const phrase = words.join(' ');
  items.forEach((item, index) => {
    const find = item.findAs || item;
    const names = [fold(find.label)].concat(item.title ? [fold(item.title)] : []);
    const hint = item.matchLabel ? '' : fold(find.hint);
    let score = 0;
    for (const word of words) {
      const best = (f) => Math.max(...names.map((name) => f(word, name, hint)));
      const s = best(wordScore) || (fuzzy ? best(lettersScore) : 0);
      if (!s) return;
      score += s;
    }
    // Words typed as the label has them, side by side, count for more:
    // "move tab" finds Move Tab Left before Move to Tab ▸ Comms.
    if (words.length > 1 && names.some((name) => name.includes(phrase))) score += 3;
    found.push({ item, index, score });
  });
  return found;
}

// wordScore scores a typed word in a row's label and hint: 6 at the start
// of the label, 5 at the start of a word in it, 3 anywhere in it, 2 at the
// start of a word in the hint, 1 anywhere in it, and 0 if neither has it.
function wordScore(word, label, hint) {
  const inLabel = placeOf(word, label);
  if (inLabel) return [0, 3, 5, 6][inLabel];
  return [0, 1, 2, 2][placeOf(word, hint)];
}

// placeOf returns where a word is in a text at best: 3 at its start, 2 at
// the start of one of its words, 1 inside one, and 0 if it isn't there.
function placeOf(word, text) {
  let best = 0;
  for (let i = text.indexOf(word); i >= 0 && best < 3; i = text.indexOf(word, i + 1)) {
    best = Math.max(best, i === 0 ? 3 : WORD_CHAR.test(text[i - 1]) ? 1 : 2);
  }
  return best;
}

// lettersScore scores a typed word that's a word of a row's label or hint
// with letters left out, and that starts as that word does: 0.5 in the
// label, 0.25 in the hint, and 0 if neither has one.
function lettersScore(word, label, hint) {
  const has = (text) => text.split(NOT_WORD_CHARS).some((w) => w[0] === word[0] && isSubsequence(word, w));
  if (has(label)) return 0.5;
  return has(hint) ? 0.25 : 0;
}

// isSubsequence reports whether a word's letters are all in another's, in
// the same order, as "termnal" is in "terminal".
function isSubsequence(word, other) {
  let at = 0;
  for (const c of other) {
    if (c === word[at]) at++;
    if (at === word.length) return true;
  }
  return false;
}

// WORD_CHAR matches a letter or digit, and NOT_WORD_CHARS what's between
// words.
const WORD_CHAR = /[\p{L}\p{N}]/u;
const NOT_WORD_CHARS = /[^\p{L}\p{N}]+/u;

// fold returns text for matching: in lower case, without accents. The
// overview of every tab matches with it too.
export function fold(text) {
  return String(text || '').normalize('NFD').replace(/[\u0300-\u036f]/g, '').toLowerCase();
}

// listItems returns the open dialog's rows, filtered by its query. In the
// add dialog, text that matches nothing may be a command: the one row then
// offers to run it in a new terminal.
export function listItems(view, ui) {
  if (ui.overlay !== 'add') return filterItems(paletteItems(view), ui.palette.query);
  const items = addFilter(addItems(ui), ui.palette.query);
  const query = ui.palette.query.trim();
  if (!items.length && query) items.push(runItem(query));
  return items;
}

// tickRow ticks or unticks a window's row in the add dialog. The keyboard
// stays in the search field.
export function tickRow(index) {
  const { ui } = store;
  const item = ui.listItems[index];
  if (ui.overlay !== 'add' || !ui.add || !item || !item.tick) return;
  const add = ui.add;
  add.ticked = add.ticked.includes(item.tick) ? add.ticked.filter((id) => id !== item.tick) : add.ticked.concat(item.tick);
  ui.palette.sel = index;
  ui.pendingFocus = 'palette-query';
  update();
}

// addTicked adds the windows ticked in the add dialog to its pane, in the
// dialog's order, and closes it (AddWindowsToPane).
export function addTicked() {
  const { ui } = store;
  const add = ui.add;
  if (ui.overlay !== 'add' || !add || !add.ticked.length) return;
  const ids = add.options.filter((o) => o.source && add.ticked.includes(o.source.id)).map((o) => o.source.id);
  ui.paneLater = null;
  closeOverlay();
  adding(add, act('AddWindowsToPane', add.paneId, ids));
}

// tickedCount returns how many windows the add dialog has ticked.
function tickedCount(ui) {
  return ui.overlay === 'add' && ui.add && ui.add.ticked ? ui.add.ticked.length : 0;
}

// clickRow is a click on a row of the open palette or add dialog. While
// the add dialog has windows ticked, a click on a window's row ticks or
// unticks it, as in a checklist, so that the windows ticked aren't lost.
// Otherwise it runs the row.
export function clickRow(index) {
  const item = store.ui.listItems[index];
  if (item && item.tick && tickedCount(store.ui)) tickRow(index);
  else runListItem(index);
}

// runListItem runs a row of the open palette or add dialog. A row that
// closes the dialog does its own focusing, so the click that opened the
// dialog doesn't (ui.paneLater).
export function runListItem(index) {
  const item = store.ui.listItems[index];
  if (!item) return;
  if (!item.stay) {
    store.ui.paneLater = null;
    closeOverlay();
  }
  item.run();
}

// moveSelection moves the selected row up or down.
export function moveSelection(step) {
  const { ui } = store;
  const n = ui.listItems.length;
  ui.palette.sel = Math.max(0, Math.min((ui.palette.sel ?? 0) + step, Math.max(n - 1, 0)));
  update();
  const el = document.querySelector('.palette-item[aria-selected="true"]');
  if (el && typeof el.scrollIntoView === 'function') el.scrollIntoView({ block: 'nearest' });
}

// renderList returns the rows of the open palette or add dialog. When some
// rows are windows that can be ticked, every row keeps a slot of the same
// width at its start, as a menu keeps a column for its ticks, so that the
// rows' tiles line up. The rows of a group, such as Windows, are a group
// for a screen reader, named by its heading, which it reads as it moves
// into the group; the heading itself isn't an option.
function renderList(items, sel, emptyText, ticked = []) {
  let html = '';
  let group = null;
  let groups = 0;
  const ticks = items.some((it) => it.tick);
  items.forEach((it, i) => {
    if (i === 0 || it.group !== group) {
      if (i > 0) html += '</div>';
      group = it.group;
      const id = 'palette-group-' + groups++;
      html += '<div class="palette-set" role="group" aria-labelledby="' + id + '">'
        + '<div class="palette-group" id="' + id + '" aria-hidden="true">' + esc(group) + '</div>';
    }
    const selected = i === sel;
    const square = it.mono
      ? tile(it.mono, it.icon)
      : '<span class="action-tile" aria-hidden="true">' + ICONS.chevron + '</span>';
    const keys = selected ? (it.keys ? it.keys + '  ↵' : '↵') : it.keys;
    // A window's tick box, which a click on it ticks, rather than adding
    // the window at once. The part that takes the click is larger than
    // the box.
    const on = Boolean(it.tick) && ticked.includes(it.tick);
    let tick = '';
    if (it.tick) {
      tick = '<span class="row-tick" data-action="palette-tick" data-index="' + i + '" title="Tick to add with others (⇧↵)" aria-hidden="true">'
        + '<span class="row-tick-box">' + (on ? ICONS.check : '') + '</span></span>';
    } else if (ticks) {
      tick = '<span class="row-tick-slot" aria-hidden="true"></span>';
    }
    html += '<button type="button" class="palette-item" role="option" id="palette-item-' + i + '" aria-selected="' + selected + '" data-action="palette-run" data-index="' + i + '"'
      + (it.tick ? ' aria-checked="' + on + '"' : '')
      + (it.itemId ? ' data-item="' + esc(it.itemId) + '"' : '') + ' tabindex="-1">'
      + tick + square + '<span class="palette-label">' + esc(it.label) + '</span>'
      + '<span class="palette-hint">' + esc(it.hint) + '</span>'
      + '<kbd class="palette-keys">' + esc(keys) + '</kbd></button>';
  });
  if (items.length) html += '</div>';
  else html = '<div class="palette-empty">' + esc(emptyText) + '</div>';
  return html;
}

// isList reports whether the open dialog is the palette or the add
// dialog's list of rows.
export function isList(ui) {
  return ui.overlay === 'palette' || (ui.overlay === 'add' && Boolean(ui.add) && ui.add.step === 'list');
}

// renderPalette returns the jump palette's or add dialog's HTML, without
// its rows (renderRows) or the search field's text (fillPalette), so that
// neither typing nor a new View replaces the search field while it's in
// use.
export function renderPalette(view, ui) {
  const kind = ui.overlay;
  if (kind === 'add' && ui.add && ui.add.step === 'url') return renderURLStep(ui);
  if (kind === 'add' && ui.add && ui.add.step === 'terminal' && ui.add.terminal) return renderTerminalStep(ui);
  if (kind === 'add' && ui.add && ui.add.step === 'make' && ui.add.make) return renderMakeStep(ui);
  const title = kind === 'add' ? 'Add a window or app to this pane' : 'Jump to a tab, window, or action';
  const n = tickedCount(ui);
  const ticked = n
    ? '<div class="palette-actions palette-ticked"><span class="palette-ticked-note" aria-live="polite">' + esc(n === 1 ? '1 window ticked' : n + ' windows ticked') + '</span>'
      + '<button type="button" class="btn small primary" data-action="add-ticked" data-key="add-ticked">' + esc(n === 1 ? 'Add 1 window' : 'Add ' + n + ' windows') + '</button></div>'
    : '';
  const keys = kind !== 'add' ? '<span><kbd>↵</kbd> choose</span>'
    : n ? '<span><kbd>↵</kbd> add ticked</span><span><kbd>⇧↵</kbd> tick</span>'
      : '<span><kbd>↵</kbd> choose</span><span><kbd>⇧↵</kbd> tick a window</span>';
  return '<div class="dialog palette" role="dialog" aria-modal="true" aria-label="' + esc(title) + '">'
    + '<div class="palette-search dialog-handle">' + ICONS.searchLarge
    + '<label for="palette-query" class="visually-hidden">' + esc(title) + '</label>'
    + '<input id="palette-query" data-key="palette-query" type="text" autocomplete="off" spellcheck="false" role="combobox" aria-expanded="true" aria-controls="palette-list" aria-autocomplete="list"'
    + ' placeholder="' + esc(title) + '">'
    + '<button type="button" class="esc" data-action="close" data-key="palette-close" aria-label="Esc to close" title="Close (esc)">esc</button></div>'
    + '<div class="palette-list" id="palette-list" role="listbox" aria-label="Results"></div>'
    + ticked
    + '<div class="palette-foot dialog-handle"><span><kbd>↑ ↓</kbd> move</span>' + keys + '<span><kbd>esc</kbd> close</span>'
    + '<span>' + (kind === 'add' ? ADD_FOOT : 'Type a window title or app name') + '</span></div>' + GRIP + '</div>';
}

// ADD_FOOT says, at the foot of the add dialog, how to add an app that
// isn't listed.
const ADD_FOOT = 'Open any app, and its windows show under Windows not in a tab';

// renderRows returns the open palette's or add dialog's rows, filtered by
// its query, and keeps them as ui.listItems. When nothing in the add dialog
// matches, a note says how to add another app's window, above the row that
// offers to run the text in a terminal.
//
// Until a row is chosen, the first row is selected, but with nothing typed
// a row that's passed by, such as Undo, isn't: Return then goes to the tab
// used before this one, as it always has (decision 49), and ↑ reaches
// Undo.
export function renderRows(view, ui) {
  const items = listItems(view, ui);
  ui.listItems = items;
  if (ui.palette.sel == null) {
    const first = ui.palette.query.trim() ? 0 : items.findIndex((it) => !it.passOver);
    ui.palette.sel = Math.max(first, 0);
  }
  const sel = Math.min(ui.palette.sel, Math.max(items.length - 1, 0));
  ui.palette.sel = sel;
  const query = ui.palette.query.trim();
  const empty = query ? 'Nothing matches “' + query + '”.' : 'There\'s nothing to choose from yet.';
  if (items.length === 1 && items[0].offer) {
    return '<p class="palette-note" id="palette-nothing">' + esc('Nothing matches “' + query + '”. To add the window of an app that isn\'t listed, '
      + 'open the app: its windows show here, under Windows not in a tab.') + '</p>' + renderList(items, sel, empty);
  }
  return renderList(items, sel, empty, ui.add ? ui.add.ticked || [] : []);
}

// fillPalette puts the text typed so far into a newly drawn search or
// address field.
export function fillPalette(ui) {
  const query = document.getElementById('palette-query');
  if (query) query.value = ui.palette.query;
  const url = document.getElementById('palette-url');
  if (url && ui.add) url.value = ui.add.url;
}

// syncSelection points the search field at the selected row.
export function syncSelection(ui) {
  const query = document.getElementById('palette-query');
  if (!query) return;
  const id = ui.listItems.length ? 'palette-item-' + ui.palette.sel : '';
  if (query.getAttribute('aria-activedescendant') !== id) query.setAttribute('aria-activedescendant', id);
}

// urlNote says what Return and Option-Return do with the typed address: for
// a site with no web app, a browser makes one if the row names one
// (AddOption.webAppBrowser), and otherwise Bench shows the page itself.
function urlNote(option) {
  const browser = option.webAppBrowser || '';
  const note = browser
    ? 'Return adds your web app for this site, or opens the site in ' + browser + ' so that you can make one. Option-Return shows the page in Bench instead.'
    : 'Return adds your web app for this site if you have one, or else shows the page in Bench, as Option-Return always does.';
  return note + ' Bench adds https:// if you leave it out, or http:// for a page on this Mac, such as localhost:3000.';
}

function renderURLStep(ui) {
  ui.listItems = [];
  const o = ui.add.option;
  const title = 'Add a web page to this pane';
  // What was wrong with the address last tried is under the field, which it
  // describes, and marks as not valid, until the address changes.
  const error = ui.add.urlError || '';
  return '<div class="dialog palette" role="dialog" aria-modal="true" aria-label="' + esc(title) + '">'
    + '<div class="palette-search dialog-handle">' + ICONS.globe
    + '<label for="palette-url" class="visually-hidden">Web page address</label>'
    + '<input id="palette-url" data-key="palette-url" type="text" inputmode="url" autocomplete="off" spellcheck="false"'
    + ' aria-describedby="' + (error ? 'palette-url-error ' : '') + 'palette-url-hint"' + (error ? ' aria-invalid="true"' : '')
    + ' placeholder="Web page address, such as example.com">'
    + '<button type="button" class="esc" data-action="add-back" data-key="palette-back" aria-label="Esc to go back to the list" title="Back to the list (esc)">esc</button></div>'
    + '<div class="palette-list" id="palette-list">'
    + (error ? '<p class="palette-error" id="palette-url-error" role="alert">' + ICONS.warning + '<span>' + esc(error) + '</span></p>' : '')
    + '<div class="palette-group" role="presentation">' + esc(o.label || 'Web page…') + '</div>'
    + '<p class="palette-note" id="palette-url-hint">' + esc(urlNote(o)) + '</p></div>'
    + '<div class="palette-actions">'
    + '<button type="button" class="btn small" data-action="add-in-bench" data-key="palette-in-bench">Show in Bench</button>'
    + '<button type="button" class="btn small primary" data-action="add-website" data-key="palette-add">Add</button></div>'
    + '<div class="palette-foot dialog-handle"><span><kbd>↵</kbd> add</span><span><kbd>⌥↵</kbd> show in Bench</span><span><kbd>esc</kbd> back</span>'
    + '<span>Opens in this pane</span></div>' + GRIP + '</div>';
}

// renderMakeStep shows the steps that make a web app in the browser that
// AddWebsite or MakeWebApp opened the site in, while the dialog waits for
// it (waitForWebApp). For a web page's web app, which takes the page's
// place, there's nothing to show in Bench instead: the page is there.
function renderMakeStep(ui) {
  ui.listItems = [];
  const { make: m, replace } = ui.add;
  const title = 'Make a web app in ' + m.browser;
  const site = replace ? replace.page : m.url;
  const steps = 'Bench has opened ' + site + ' in ' + m.browser + '.' + (m.steps ? ' ' + sentence(m.steps) : '');
  const note = replace
    ? 'Bench puts the web app in place of this page as soon as it\'s ready.'
    : 'Bench adds the web app to this pane as soon as it\'s ready.';
  return '<div class="dialog palette" role="dialog" aria-modal="true" aria-labelledby="palette-make-title" aria-describedby="palette-make-steps">'
    + '<div class="palette-search dialog-handle">' + ICONS.globe
    + '<h2 class="palette-title" id="palette-make-title">' + esc(title) + '</h2>'
    + '<button type="button" class="esc" data-action="add-back" data-key="make-back" aria-label="Esc to cancel" title="Cancel (esc)">esc</button></div>'
    + '<div class="palette-list" id="palette-list">'
    + '<p class="palette-note" id="palette-make-steps">' + esc(steps) + '</p>'
    + '<p class="palette-note">' + esc(note) + '</p></div>'
    + '<div class="palette-actions">'
    + (replace ? '' : '<button type="button" class="btn small" data-action="add-in-bench" data-key="make-in-bench">Show in Bench instead</button>')
    + '<button type="button" class="btn small" data-action="add-back" data-key="make-cancel">Cancel</button></div>'
    + '<div class="palette-foot dialog-handle"><span><kbd>esc</kbd> cancel</span><span>Waiting for the web app…</span></div>' + GRIP + '</div>';
}
