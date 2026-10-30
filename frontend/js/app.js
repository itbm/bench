// Bench's UI: the entry point. It asks Go for the View, draws it, and turns
// clicks, keys, and drags into calls to Go's bound methods.

import { call, errorText, on, onConnected } from './bridge.js';
import {
  act, closeMenu, closeOverlay, isDemo, noteNotices, overlaySent, resendOverlay, runToastAction, setCommander, setRenderer, store, toast,
  update,
} from './state.js';
import { contentPoint, focusedPaneID, sentence, shownInFocus } from './util.js';
import { focusTerminal, installRenderDeferral, measure, render, syncSlim } from './render.js';
import { installDrag } from './drag.js';
import { installDialogs } from './dialogs.js';
import { installStrips, syncStrips } from './strips.js';
import { installTiles } from './tiles.js';
import { hidePreview, installPreviews } from './preview.js';
import { installKeyboard } from './keyboard.js';
import {
  copyMessage, openAttention, openLayoutMenu, openMessages, openNotices, openPaneMenu, openTabMenu, openTerminalMenu, openWindowMenu,
  runMenuEntry, runNoticeAction,
} from './menu.js';
import {
  backToList, openAdd, openPalette, runListItem, showInBench, submitURL, togglePalette,
} from './palette.js';
import {
  deleteTab, openEditor, resetLayout, save, setName, setPlace, setPreset, setSmallScreens,
} from './editor.js';
import {
  openSettings, saveSettings, setChoice, setFontSize, setLaunchWait, setSetting, setThreshold, showKeyboardSettings,
} from './settings.js';
import {
  restoreDefaults, setShortcut, startRecording, stopRecording, stopRecordingFor, useSuggestion,
} from './keyboard-settings.js';
import {
  dismissGuide, removeStarter, requestAccessibility, setGuideLogin, showGuide, turnPage, useStarters,
} from './guide.js';
import { openShortcuts } from './shortcuts.js';
import { confirmQuit, openQuit, setDontAsk } from './quit.js';
import { closeTab, confirmClose, quickClose } from './close.js';
import { clickTab, onRenameFocusOut, setRenameValue, startTabRename } from './rename.js';
import {
  chooseFolder, doneLauncher, editLauncher, isLauncherField, isTerminalField, newLauncher, removeLauncher, setLauncherField,
  setLauncherTick, setTerminalField, setTerminalTick, submitTerminal,
} from './launchers.js';
import * as terminals from './terminals.js';
import { bootTerminalWindow, terminalID } from './terminal-window.js';
import { PAGE_ACTIONS, pageCommand, setFindText } from './pages.js';

// stateEvents counts bench:state events, so that ready can tell whether one
// arrived while Ready was on its way.
let stateEvents = 0;

// readyAsked is set once the UI has first asked Go for the View.
let readyAsked = false;

// demoWidthSent is set once the browser preview has told Go the browser's
// width.
let demoWidthSent = false;

// setView draws a View from Go.
function setView(v) {
  if (!v || !Array.isArray(v.tabs) || !v.tabs.length) return;
  store.view = v;
  noteNotices(v);
  update();
  sendDemoWidth(v);
}

// onState draws a bench:state event's View.
function onState(v) {
  stateEvents++;
  setView(v);
}

// sendDemoWidth tells Go the browser's width the first time the browser
// preview shows "Fit to browser", since Go can't measure it.
function sendDemoWidth(v) {
  if (demoWidthSent || !v.demo) return;
  demoWidthSent = true;
  if ((v.demo.screen || 'fit') === 'fit') act('SetDemoScreen', 'fit', window.innerWidth);
}

// addPane is the pane the add dialog adds to: the compact pane stands for
// the tab's focused pane.
function addPane(paneId) {
  return paneId === 'compact' ? focusedPaneID(store.view) || paneId : paneId;
}

// focusPane focuses a pane of the current tab, if it's shown and isn't
// focused already.
function focusPane(id) {
  const d = store.view && store.view.display;
  if (!d || id === 'compact' || id === d.focus || !(d.panes || []).some((p) => p.id === id)) return;
  act('FocusPane', id);
}

// ACTIONS run the controls with a data-action.
const ACTIONS = {
  'screen': (t) => act('SetDemoScreen', t.dataset.screen, window.innerWidth),
  // The second click of a double-click doesn't switch again: the
  // double-click renames the tab (onDoubleClick), and switching again would
  // focus the tab's window, taking the keyboard from the field.
  'tab': (t, e) => {
    if (e && e.detail > 1) return;
    clickTab(Number(t.dataset.index), e ? e.detail : 0);
  },
  'mode': (t) => act('SetMode', t.dataset.mode),
  'activate': (t) => act('ShowItem', t.dataset.pane, t.dataset.item),
  'split-right': async (t) => {
    const reason = store.view.display && store.view.display.splitsDisabled;
    if (reason) {
      toast(sentence(reason));
      return;
    }
    // The click in the pane focuses it only if Go refuses the move, which
    // focuses the window's new pane otherwise (OWN_FOCUS).
    const { ok } = await act('MoveItem', t.dataset.item, store.view.current, t.dataset.pane, 'right');
    if (!ok) focusPane(t.dataset.pane);
  },
  'menu': (t) => {
    const m = store.ui.menu;
    if (m && m.kind === 'window' && m.shownIn === t.dataset.pane) closeMenu(true);
    else openWindowMenu(t, t.dataset.item, t.dataset.pane);
  },
  'menu-run': (t) => runMenuEntry(Number(t.dataset.index)),
  'notices': (t) => {
    if (store.ui.menu && store.ui.menu.kind === 'notices') closeMenu(true);
    else openNotices(t);
  },
  'notice-run': (t) => runNoticeAction(Number(t.dataset.notice), Number(t.dataset.index)),
  // A recent message's Copy.
  'message-copy': (t) => copyMessage(t, Number(t.dataset.index)),
  // A toast's button, such as Undo.
  'toast-run': () => runToastAction(),
  'layout-reason': (t) => toast(sentence(t.title)),
  'layout-menu': (t) => {
    if (store.ui.menu && store.ui.menu.kind === 'layout') closeMenu(true);
    else openLayoutMenu(t);
  },
  'attention': (t) => {
    if (store.ui.menu && store.ui.menu.kind === 'attention') closeMenu(true);
    else openAttention(t);
  },
  'add': (t) => openAdd(addPane(t.dataset.pane)),
  'open-palette': () => openPalette(),
  'new-tab': () => openEditor(-1),
  'open-settings': () => openSettings(),
  'close': () => closeOverlay(),
  'add-back': () => backToList(),
  'add-website': () => submitURL(),
  'add-in-bench': () => showInBench(),
  'add-terminal': () => submitTerminal(),
  // A form's Choose…, for its Folder field.
  'choose-folder': (t) => chooseFolder(t.dataset.field),
  'launcher-edit': (t) => editLauncher(Number(t.dataset.index)),
  'launcher-done': () => doneLauncher(),
  'launcher-remove': (t) => removeLauncher(Number(t.dataset.index)),
  'launcher-new': () => newLauncher(),
  'palette-run': (t) => runListItem(Number(t.dataset.index)),
  'draft-preset': (t) => setPreset(t.dataset.preset),
  'draft-small': (t) => setSmallScreens(t.dataset.small),
  'save-tab': () => save(),
  'save-settings': () => saveSettings(),
  'setting-choice': (t) => setChoice(t.dataset.choice, t.dataset.value),
  // Settings ▸ Keyboard: a shortcut's field, which records new keys, its
  // Turn off, its Use with Go's suggestion, and Restore defaults.
  'key-record': (t) => startRecording(t.dataset.field),
  'key-off': (t) => setShortcut(t.dataset.field, ''),
  'key-use': (t) => useSuggestion(t.dataset.field),
  'keys-defaults': () => restoreDefaults(),
  'delete-tab': () => deleteTab(),
  'reset-layout': () => resetLayout(),
  'redock': (t) => act('Redock', t.dataset.item),
  'remove-item': (t) => act('PopOut', t.dataset.item),
  'open-item': (t) => act('OpenItem', t.dataset.item),
  'remove-remembered': (t) => act('RemoveRemembered', t.dataset.pane, t.dataset.app),
  'close-pane': (t) => act('ClosePane', t.dataset.pane),
  // A zoomed pane's Zoomed chip, and the status bar's note of the zoom.
  'show-all-panes': () => act('ShowAllPanes'),
  'request-accessibility': () => requestAccessibility(),
  // Go also opens System Settings at Screen Recording's list, and Quit and
  // Reopen Bench is for macOS to apply it.
  'request-screen-recording': () => act('RequestScreenRecording'),
  'quit-and-reopen': () => act('QuitAndReopen', false, false),
  'open-login-items': () => act('OpenLoginItems'),
  // The welcome guide's pages, and its starting set-up.
  'guide-next': () => turnPage(1),
  'guide-back': () => turnPage(-1),
  'guide-remove': (t) => removeStarter(t.dataset.name),
  'guide-restore': () => removeStarter(''),
  'guide-one-tab': () => useStarters(true),
  'guide-add-tabs': () => useStarters(false),
  'dismiss-first-run': () => dismissGuide(),
  // The status bar's key hints open the Keyboard Shortcuts sheet, whose
  // Change Shortcuts… opens Settings ▸ Keyboard.
  'shortcuts': () => openShortcuts(),
  'keyboard-settings': () => showKeyboardSettings(),
  'quit-run': () => confirmQuit(),
  // The × on a tab and on a sub-tab, which shows on the current tab and
  // the shown sub-tab, and on the others under the pointer.
  'close-tab': (t) => closeTab(Number(t.dataset.index)),
  'close-item': (t) => quickClose(t.dataset.item),
  'close-run': () => confirmClose(),
  // A web page's find bar: its arrows, and its ×.
  ...PAGE_ACTIONS,
};

// OWN_UI are the actions that open Bench's own menus and dialogs.
const OWN_UI = new Set(['add', 'menu', 'notices', 'attention']);

// TOGGLES are the actions of the buttons that open and close a menu.
const TOGGLES = new Set(['menu', 'notices', 'attention', 'layout-menu']);

// OWN_FOCUS are the actions that focus a pane themselves: Go focuses the
// pane that shows the window afterwards. Focusing the pane clicked in as
// well could undo that, since Go can handle the two calls in either order.
// A split that Go refuses focuses the pane clicked in after all. Closing a
// pane, or forgetting what it's waiting for, can close it, so focusing it
// could fail; so can closing one of its sub-tabs, after which Go focuses
// what the pane shows.
const OWN_FOCUS = new Set(['activate', 'split-right', 'open-item', 'remove-remembered', 'close-pane', 'close-item']);

// pressTarget is where the last press went down, so that a click on a
// dialog's backdrop closes it only if it began there too. A drag that
// selects text in a field and ends on the backdrop clicks the backdrop as
// well, because a click goes to what the press and release have in common.
let pressTarget = null;

function onPointerDown(e) {
  pressTarget = e.target;
}

function onClick(e) {
  const { ui, view } = store;
  if (!view) return;
  const target = e.target.closest('[data-action]');
  const action = target ? target.dataset.action : null;
  const paneEl = e.target.closest('.pane[data-pane-id]');
  // A click anywhere but the field that records keys stops it recording.
  stopRecordingFor(e.target);

  // Clicking outside an open menu closes it. A click on another control,
  // or in a pane, does what that does instead of what the click that
  // opened the menu would have done.
  if (ui.menu && !e.target.closest('.menu') && !TOGGLES.has(action)) {
    if (target || paneEl) ui.paneLater = null;
    closeMenu(false);
  }
  // Clicking anywhere in a pane focuses it. Showing an item focuses its
  // pane anyway. A control that opens one of Bench's own menus or dialogs
  // leaves the pane until that closes: focusing an App item's pane
  // activates its app, which would then get the keys meant for Bench. From
  // the keyboard (detail 0), the pane isn't focused at all, so that focus
  // stays in Bench.
  if (paneEl && !ui.overlay && !OWN_FOCUS.has(action)) {
    const id = paneEl.dataset.paneId;
    if (OWN_UI.has(action)) {
      ui.paneLater = e.detail > 0 && id !== 'compact' && view.display && id !== view.display.focus ? { paneId: id } : null;
    } else if (id !== 'compact' && view.display && id !== view.display.focus) {
      act('FocusPane', id);
    }
  }
  if (!target) return;
  if (action === 'backdrop') {
    if (e.target === target && pressTarget === target) closeOverlay();
    return;
  }
  const run = ACTIONS[action];
  if (run) run(target, e);
}

// onDoubleClick renames a double-clicked tab in place (rename.js), but not
// from its ×, nor while a dialog is open or something is being dragged. It
// keeps the double-click from the rest of the title bar, which zooms the
// window: the tab doesn't move the window anyway (styles.css), so Wails
// leaves it alone.
function onDoubleClick(e) {
  const { ui } = store;
  if (!store.view || !e.target.closest || e.button !== 0) return;
  const tab = e.target.closest('#titlebar .tab[data-index]');
  if (!tab) return;
  e.preventDefault();
  e.stopPropagation();
  if (tab.classList.contains('renaming') || e.target.closest('.close-cross') || ui.overlay || ui.drag) return;
  if (ui.menu) closeMenu(false);
  startTabRename(Number(tab.dataset.index));
}

// onBlur closes an open menu when another app takes the keyboard, so that
// Bench isn't left raised above that app's windows. Dialogs stay open: Go
// lowers Bench's window itself while Bench is inactive, and the add dialog
// waits while the user makes a web app in their browser.
function onBlur(e) {
  if (e.target !== window) return;
  // Bench's global shortcuts come back while another app has the keyboard.
  stopRecording();
  if (!store.ui.menu) return;
  store.ui.paneLater = null;
  closeMenu(false);
}

// onContextMenu opens a right-clicked tab's menu, or sub-tab's menu, at the
// pointer. A right-click on the empty part of a pane's header, or of the
// "tabs" view's pane's part of the title bar, opens the menu of the window
// the pane shows. The window is shown, and its pane focused, when the menu
// closes without running an entry: showing it first would activate an App
// item's app, which would then get the menu's keys. A right-click on an
// empty pane, header or body, opens the pane's menu, and one in a terminal
// opens the terminal's, as the pane's header does: its pane is focused when
// the menu closes.
function onContextMenu(e) {
  if (!store.view || !e.target.closest) return;
  // The field that renames a tab or sub-tab, and a terminal's find bar,
  // have the usual menu for text.
  if (e.target.closest('.renaming, .term-find')) return;
  const tab = e.target.closest('.tab[data-index]');
  if (tab) {
    e.preventDefault();
    hidePreview();
    openTabMenu(tab, Number(tab.dataset.index), pointerIn(e, tab));
    return;
  }
  const host = e.target.closest('.term-host[data-term-host]');
  const termPane = host && host.closest('[data-term]') ? host.closest('[data-pane-id]') : null;
  if (termPane) {
    e.preventDefault();
    hidePreview();
    const pane = termPane.dataset.paneId;
    const item = host.dataset.termHost;
    openTerminalMenu(host, item, pane, pointerIn(e, host));
    store.ui.paneLater = { paneId: pane, itemId: item };
    return;
  }
  const sub = e.target.closest('.subtab');
  const control = e.target.closest('button, input, a');
  const header = sub || control ? null : e.target.closest('.pane-header, .title-pane');
  const more = header && header.querySelector('[data-action="menu"]');
  if (more) {
    e.preventDefault();
    hidePreview();
    const { pane, item } = more.dataset;
    openWindowMenu(more, item, pane, pointerIn(e, header));
    store.ui.paneLater = { paneId: pane, itemId: item };
    return;
  }
  // An empty pane has no ⋯ button: its header and body open its own menu.
  const paneEl = sub || control ? null : e.target.closest('.pane[data-pane-id], .title-pane[data-pane-id]');
  if (paneEl && isEmptyPane(paneEl.dataset.paneId)) {
    e.preventDefault();
    hidePreview();
    const pane = paneEl.dataset.paneId;
    openPaneMenu(paneEl.querySelector('.pane-header') || paneEl, pane, addPane(pane), pointerIn(e, paneEl));
    store.ui.paneLater = { paneId: pane };
    return;
  }
  if (!sub) return;
  e.preventDefault();
  hidePreview();
  const { pane, item } = sub.dataset;
  const button = document.querySelector('[data-action="menu"][data-pane="' + CSS.escape(pane) + '"]');
  if (!button) {
    act('ShowItem', pane, item);
    return;
  }
  openWindowMenu(button, item, pane, pointerIn(e, sub));
  store.ui.paneLater = { paneId: pane, itemId: item };
}

// middle is the tab or sub-tab that the middle button went down on, for
// the middle-click that closes it.
let middle = null;

// closeTarget returns the tab or sub-tab that a mouse event is on, if a
// middle-click there may close it: not while a dialog is open, or a
// sub-tab is dragged, nor one that's being renamed.
function closeTarget(e) {
  const { ui } = store;
  if (!store.view || ui.overlay || ui.drag || !e.target.closest) return null;
  const el = e.target.closest('.tab[data-index], .subtab[data-item]');
  return el && !el.classList.contains('renaming') ? el : null;
}

// onMiddleDown notes a press of the middle button on a tab or sub-tab.
function onMiddleDown(e) {
  if (e.button !== 1) return;
  middle = closeTarget(e);
  if (middle) e.preventDefault();
}

// onMiddleUp closes the tab or sub-tab that the middle button went down and
// came up on, as its × does: on auxclick, or on the mouseup before it,
// since WebKit may not send auxclick, whichever comes first.
function onMiddleUp(e) {
  if (e.button !== 1) return;
  const down = middle;
  middle = null;
  const el = closeTarget(e);
  if (!el || el !== down) return;
  e.preventDefault();
  hidePreview();
  if (el.classList.contains('tab')) closeTab(Number(el.dataset.index));
  else quickClose(el.dataset.item);
}

// isEmptyPane reports whether a pane of the current tab, as drawn, has no
// windows.
function isEmptyPane(id) {
  const p = ((store.view.display && store.view.display.panes) || []).find((x) => x.id === id);
  return Boolean(p) && !(p.items || []).length;
}

// pointerIn returns where a context menu event was, in content
// coordinates, if that's on el, as for a right-click. A menu asked for from
// the keyboard may say it was elsewhere: then it returns null, and the menu
// opens where the button that opens it would: below the tab, or below the
// pane's ⋯ button.
function pointerIn(e, el) {
  const r = el.getBoundingClientRect();
  const on = e.clientX >= r.left && e.clientX <= r.right && e.clientY >= r.top && e.clientY <= r.bottom;
  return on ? contentPoint(e) : null;
}

function onInput(e) {
  const t = e.target;
  const { ui } = store;
  if (t.id === 'palette-query') {
    ui.palette.query = t.value;
    ui.palette.sel = 0;
    update();
  } else if (t.id === 'palette-url' && ui.add) {
    ui.add.url = t.value;
  } else if (t.id === 'tab-name') {
    setName(t.value);
  } else if (t.id === 'rename-input') {
    setRenameValue(t.value);
  } else if (t.id === 'compact-below') {
    setThreshold(t.value);
  } else if (t.id === 'terminal-font-size') {
    setFontSize(t.value);
  } else if (t.id === 'launch-wait') {
    setLaunchWait(t.value);
  } else if (isTerminalField(t.id)) {
    setTerminalField(t.id, t.value);
  } else if (isLauncherField(t.id)) {
    setLauncherField(t.id, t.value);
  } else if (t.dataset && t.dataset.pageFind) {
    setFindText(t.dataset.pageFind, t.value);
  }
}

function onChange(e) {
  const t = e.target;
  if (!t.dataset) return;
  if (store.ui.settings && t.dataset.setting) setSetting(t.dataset.setting, t.checked);
  else if (t.dataset.terminal) setTerminalTick(t.dataset.terminal, t.checked);
  else if (t.dataset.launcher) setLauncherTick(t.checked);
  else if (t.dataset.question === 'dont-ask') setDontAsk(t.checked);
  else if (t.dataset.guide === 'open-at-login') setGuideLogin(t.checked);
  else if (t.id === 'tab-place') setPlace(t.value);
}

// editing reports whether Tab properties is open for tab index.
function editing(index) {
  const { ui } = store;
  return ui.overlay === 'editor' && Boolean(ui.editor) && ui.editor.index === index;
}

// runCommand does what a bench:command asks, such as opening the palette
// for the jump shortcut.
function runCommand(cmd) {
  if (!store.view) return;
  const name = cmd && typeof cmd === 'object' ? cmd.name : cmd;
  switch (name) {
    case 'toggle-palette': togglePalette(); break;
    case 'open-palette': if (store.ui.overlay !== 'palette') openPalette(); break;
    case 'new-tab': openEditor(-1); break;
    // A dialog that's open already keeps its draft.
    case 'edit-tab': if (!editing(store.view.current)) openEditor(store.view.current); break;
    // The Tab menu's Rename…, for the current tab, as a double-click on it
    // renames it.
    case 'rename-tab': renameCurrentTab(); break;
    // The Pane menu's Add to Pane…, for the focused pane.
    case 'add-to-pane': if (store.ui.overlay !== 'add') openAdd(focusedPaneID(store.view)); break;
    case 'settings': if (store.ui.overlay !== 'settings') openSettings(); break;
    // A notice's Keyboard settings.
    case 'keyboard-settings': showKeyboardSettings(); break;
    // Help ▸ Keyboard Shortcuts, which shows them as configured.
    case 'keyboard-shortcuts': openShortcuts(); break;
    // Help ▸ Recent Messages.
    case 'recent-messages': if (!store.ui.menu || store.ui.menu.kind !== 'notices') openMessages(); break;
    case 'first-run': showGuide(); break;
    case 'focus-terminal': focusTerminal(cmd.itemId); break;
    // Go asks before quitting while terminals are running something.
    case 'confirm-quit': openQuit(cmd); break;
    // The Tab and Pane menus' Close Tab… and Close Window, for the current
    // tab, and the focused pane's window, as a tab's menu and a sub-tab's ×
    // close them.
    case 'close-tab': closeTab(store.view.current); break;
    case 'close-window': closeShown(); break;
    // Pane ▸ Page's Find in Page… and Copy Page Link, for the web page
    // that the focused pane shows, as its menu's do.
    case 'find-in-page':
    case 'copy-page-link': pageCommand(name, cmd.itemId); break;
    default: break;
  }
}

// renameCurrentTab turns the current tab's name into a field, to type a
// new one, closing any menu or dialog first, as the tab strip is under
// them.
function renameCurrentTab() {
  if (store.ui.overlay) closeOverlay();
  else if (store.ui.menu) closeMenu(false);
  startTabRename(store.view.current);
}

// showToast shows Go's toast: its text, or { text, action } for one with a
// button, such as Undo.
function showToast(msg) {
  if (typeof msg === 'string') toast(msg);
  else if (msg && msg.text) toast(msg.text, msg.action);
  else if (msg) toast(msg.message);
}

// closeShown closes the window, page, or terminal that the focused pane
// shows, as its sub-tab's × does, or says that there's none.
function closeShown() {
  const shown = shownInFocus(store.view);
  if (shown) quickClose(shown);
  else toast('There\'s nothing to close in this pane.');
}

// loadFonts waits briefly for the UI fonts, so that terminals measure the
// right font.
function loadFonts() {
  if (!document.fonts || !document.fonts.load) return Promise.resolve();
  const fonts = Promise.all([
    document.fonts.load('400 13px "IBM Plex Sans"'),
    document.fonts.load('500 13px "IBM Plex Sans"'),
    document.fonts.load('600 13px "IBM Plex Sans"'),
    document.fonts.load('400 13px "IBM Plex Mono"'),
    document.fonts.load('500 13px "IBM Plex Mono"'),
  ]).catch(() => {});
  return Promise.race([fonts, new Promise((r) => setTimeout(r, 1500))]);
}

// ready asks Go for the View. Go sends bench:state after every change, so a
// state that arrives while Ready is on its way is at least as new as Ready's
// answer, or is followed by one that is: Ready's answer is then dropped
// rather than drawn over it.
//
// Go's Ready lowers Bench's window. If a state was drawn before Ready
// answered, the UI has already told Go whether a menu or dialog (such as
// the first-run guide) is open, and Go may have handled that before Ready,
// so the UI tells Go again.
async function ready() {
  readyAsked = true;
  for (let attempt = 1; ; attempt++) {
    const seen = stateEvents;
    try {
      const v = await call('Ready');
      const told = overlaySent();
      if (stateEvents === seen) setView(v);
      if (told) resendOverlay();
      return;
    } catch (err) {
      // A state from Go makes Ready unnecessary.
      if (store.view) return;
      if (attempt >= 3) {
        toast(errorText(err));
        return;
      }
      await new Promise((r) => setTimeout(r, 500 * attempt));
    }
  }
}

// catchUp asks Go for the View again when the browser preview's event
// socket opens, because events sent before it opened are lost. Ready lowers
// Bench's window, so it then tells Go again whether a menu or dialog is
// open.
async function catchUp() {
  if (!readyAsked) return;
  const seen = stateEvents;
  try {
    const v = await call('Ready');
    if (stateEvents === seen) setView(v);
  } catch {
    // The next bench:state brings the View.
  }
  resendOverlay();
}

// watchSize tells Go where the panes are as the window changes size, and
// keeps the title bar and the strips of tabs and sub-tabs in step.
function watchSize() {
  new ResizeObserver(() => {
    measure();
    syncStrips(true);
  }).observe(document.getElementById('panes'));
  new ResizeObserver(() => {
    syncSlim();
    syncStrips(true);
  }).observe(document.getElementById('titlebar'));
  let timer = null;
  window.addEventListener('resize', () => {
    measure();
    if (!isDemo() || (store.view.demo.screen || 'fit') !== 'fit') return;
    clearTimeout(timer);
    timer = setTimeout(() => act('SetDemoScreen', 'fit', window.innerWidth), 100);
  });
}

async function boot() {
  setRenderer(render);
  setCommander(runCommand);
  document.addEventListener('pointerdown', onPointerDown, true);
  document.addEventListener('click', onClick);
  document.addEventListener('dblclick', onDoubleClick);
  document.addEventListener('contextmenu', onContextMenu);
  document.addEventListener('mousedown', onMiddleDown);
  document.addEventListener('mouseup', onMiddleUp);
  document.addEventListener('auxclick', onMiddleUp);
  document.addEventListener('input', onInput);
  document.addEventListener('focusout', onRenameFocusOut);
  document.addEventListener('change', onChange);
  window.addEventListener('blur', onBlur);
  installRenderDeferral();
  installDrag();
  installDialogs();
  installStrips();
  installTiles();
  installPreviews();
  installKeyboard();
  onConnected(catchUp);
  try {
    await Promise.all([
      on('bench:state', onState),
      on('bench:toast', showToast),
      on('bench:command', runCommand),
      on('term:data', (d) => terminals.write(d)),
    ]);
  } catch (err) {
    toast(errorText(err));
    return;
  }
  await loadFonts();
  await ready();
  watchSize();
}

// For debugging in the browser console, and for tests.
window.bench = {
  store, render: update, terminalCount: terminals.count, terminalOptions: terminals.optionsOf, terminalColours: terminals.coloursOf,
};

// A popped-out terminal's window loads this page at /?terminal=<item ID>,
// to show just that terminal.
const popped = terminalID();
if (popped) bootTerminalWindow(popped);
else boot();
