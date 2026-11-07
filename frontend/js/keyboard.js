// Keys. In the browser preview, the UI handles Bench's global shortcuts
// itself, as configured (View.keys), as the prototype does; in the app, Go
// handles them as global shortcuts. Menu, dialog, and divider keys work in
// both, as do the Command keys of Bench's window (commandKeys), the keys
// of the strips of tabs and sub-tabs and of the radio groups (roving.js),
// and recording a shortcut in Settings ▸ Keyboard (keyboard-settings.js),
// which comes before them all.

import { act, closeMenu, isDemo, store } from './state.js';
import { addTicked, backToList, moveSelection, runListItem, submitURL, tickRow, togglePalette } from './palette.js';
import { openEditor, save } from './editor.js';
import { isSettingsNumber, openSettings, saveSettings } from './settings.js';
import { quickClose } from './close.js';
import { evenDivider, nudgeDivider } from './drag.js';
import { closeSubmenu, openSubmenu, toggleZoom } from './menu.js';
import { commitRename, renameKeys } from './rename.js';
import { pageFindKeys } from './pages.js';
import { isLauncherField, isTerminalField, submitTerminal } from './launchers.js';
import { recordKey, recordKeyUp } from './keyboard-settings.js';
import { rovingKeys } from './roving.js';
import { afterAsking, dismissOverlay, keepEditing } from './discard.js';
import { itemOf, keysOf, realPaneOf, splitShortcut } from './util.js';
import { holdRow, holdShown, overviewKeys, toggleOverview } from './overview.js';
import * as terminals from './terminals.js';

const ARROWS = { ArrowLeft: 'left', ArrowRight: 'right', ArrowUp: 'up', ArrowDown: 'down' };

// KEY_NAMES are the display names of keys (as in shortcuts.Display) by
// KeyboardEvent.code, for those that aren't letters, digits, or F keys.
const KEY_NAMES = {
  Space: 'Space', Tab: 'Tab', Enter: '↩', NumpadEnter: '↩', Escape: '⎋', Backspace: '⌫', Delete: '⌦',
  Home: '↖', End: '↘', PageUp: '⇞', PageDown: '⇟',
  ArrowLeft: '←', ArrowRight: '→', ArrowUp: '↑', ArrowDown: '↓',
  Equal: '=', Minus: '-', BracketLeft: '[', BracketRight: ']', Quote: '\'', Semicolon: ';',
  Backslash: '\\', Comma: ',', Slash: '/', Period: '.', Backquote: '`',
};

// keyName returns the display name of a key event's key, or ''.
export function keyName(e) {
  if (KEY_NAMES[e.code]) return KEY_NAMES[e.code];
  const m = /^(?:Key([A-Z])|Digit([0-9])|(F[0-9]+))$/.exec(e.code || '');
  return m ? m[1] || m[2] || m[3] : '';
}

// modsOf returns a key event's modifiers in display form, in the order
// shortcuts use: ⌃⌥⇧⌘.
export function modsOf(e) {
  return (e.ctrlKey ? '⌃' : '') + (e.altKey ? '⌥' : '') + (e.shiftKey ? '⇧' : '') + (e.metaKey ? '⌘' : '');
}

// matches reports whether a key event is a shortcut, given in display form.
export function matches(e, shortcut) {
  if (!shortcut) return false;
  const { mods, key } = splitShortcut(shortcut);
  return mods === modsOf(e) && key === keyName(e);
}

// withModifiers reports whether a key event has exactly the modifiers
// given in display form, such as "⌃⌥", and returns its key's name if so.
function withModifiers(e, modifiers) {
  return modifiers && modifiers === modsOf(e) ? keyName(e) : '';
}

// actionFor returns what a key event does as one of Bench's shortcuts, as
// configured, or null.
export function actionFor(e, keys) {
  const tab = withModifiers(e, keys.tabs);
  if (/^[1-9]$/.test(tab)) return () => act('SwitchTab', Number(tab) - 1);
  if (matches(e, keys.nextTab)) return () => act('StepTab', 1);
  if (matches(e, keys.previousTab)) return () => act('StepTab', -1);
  if (matches(e, keys.lastTab)) return () => act('LastTab');
  if (matches(e, keys.rearrange)) return () => act('Rearrange');
  if (matches(e, keys.zoomPane)) return () => toggleZoom();
  if (matches(e, keys.growPane)) return () => act('GrowPane');
  if (matches(e, keys.shrinkPane)) return () => act('ShrinkPane');
  if (matches(e, keys.equalPanes)) return () => act('EqualPanes');
  if (matches(e, keys.nextLayout)) return () => act('StepLayout', 1);
  if (matches(e, keys.previousLayout)) return () => act('StepLayout', -1);
  if (matches(e, keys.addFrontWindow)) return () => act('AddFrontWindow');
  if (matches(e, keys.undo)) return () => act('UndoLatest');
  if (matches(e, keys.nextWindow)) return () => act('CycleSubtab', 1);
  // The previous window is the next window's shortcut with Shift added.
  if (e.shiftKey && matches({ ...pick(e), shiftKey: false }, keys.nextWindow)) return () => act('CycleSubtab', -1);
  const arrow = ARROWS[e.code];
  if (arrow && withModifiers(e, keys.focusPane)) return () => act('FocusDirection', arrow);
  if (arrow && withModifiers(e, keys.moveWindow)) return () => act('MoveDirection', arrow);
  // The send modifiers with a tab's number, or with ` for the last used
  // tab, send the shown window there.
  const send = withModifiers(e, keys.sendToTab);
  if (/^[1-9]$/.test(send)) return () => act('SendToTab', Number(send) - 1);
  if (send === '`') return () => act('SendToLastTab');
  return null;
}

// holdAction returns what a key event does as one of Bench's shortcuts
// while the overview is open for Go's hold after last_tab's shortcut, as
// Go's global shortcuts do then in the app: last_tab again chooses the
// next tab, next_tab, previous_tab, and focus_pane_modifiers with an arrow
// key move the choice, Up and Down by a row, send_to_last_tab's ⇧` moves
// it back, and a tab's shortcut switches to it. Others do nothing, except
// zoom_pane's Return, which the overview's keys take.
function holdAction(e, keys) {
  if (matches(e, keys.lastTab)) return () => act('LastTab');
  if (matches(e, keys.nextTab)) return () => act('StepHold', 1);
  if (matches(e, keys.previousTab)) return () => act('StepHold', -1);
  if (withModifiers(e, keys.sendToTab) === '`') return () => act('StepHold', -1);
  const arrow = ARROWS[e.code];
  if (arrow && withModifiers(e, keys.focusPane)) {
    if (arrow === 'up' || arrow === 'down') return () => holdRow(arrow === 'up' ? -1 : 1);
    return () => act('StepHold', arrow === 'left' ? -1 : 1);
  }
  const tab = withModifiers(e, keys.tabs);
  if (/^[1-9]$/.test(tab)) return () => act('SwitchTab', Number(tab) - 1);
  return null;
}

// pick copies what matches reads from a key event.
function pick(e) {
  return { code: e.code, ctrlKey: e.ctrlKey, altKey: e.altKey, shiftKey: e.shiftKey, metaKey: e.metaKey };
}

// shortcut runs one of Bench's global shortcuts in the browser preview.
// last_tab's goes to Go after the modifiers held (demoModifiers), so that
// Go's hold after it works as in the app.
function shortcut(e) {
  const { ui, view } = store;
  const keys = keysOf(view);
  const run = ui.overlay ? (holdShown() ? holdAction(e, keys) : null) : actionFor(e, keys);
  if (run) {
    e.preventDefault();
    e.stopPropagation();
    // The shortcut does its own focusing, so the click that opened the
    // menu doesn't (ui.paneLater): Go could handle the two in either order.
    if (ui.menu) {
      ui.paneLater = null;
      closeMenu(false);
    }
    if (matches(e, keys.lastTab)) {
      demoModifiers(e, true);
      inOrder(run);
    } else {
      run();
    }
    return true;
  }
  const ctrlK = e.ctrlKey && !e.altKey && !e.metaKey && e.code === 'KeyK' && !terminals.hasFocus();
  if (matches(e, keys.jump) || ctrlK) {
    e.preventDefault();
    e.stopPropagation();
    if (view) afterAsking(togglePalette);
    return true;
  }
  if (matches(e, keys.showAllTabs)) {
    e.preventDefault();
    e.stopPropagation();
    if (view) afterAsking(toggleOverview);
    return true;
  }
  return false;
}

// demoKeys holds, in the browser preview, the calls that must reach Go in
// order: the modifier keys held (DemoModifiers), and last_tab's shortcut
// after them. held is the modifiers that Go was last told are held, by
// name.
let demoKeys = Promise.resolve();
let held = [];

// inOrder makes a call after the calls before it have been answered.
function inOrder(run) {
  demoKeys = demoKeys.then(run, run);
}

// heldNames returns the modifier keys that a key event has held, by the
// names DemoModifiers takes.
function heldNames(e) {
  return [e.ctrlKey && 'ctrl', e.altKey && 'alt', e.shiftKey && 'shift', e.metaKey && 'cmd'].filter(Boolean);
}

// demoModifiers tells Go which modifier keys are held in the browser
// preview, for its hold after last_tab's shortcut: with start, as the
// shortcut is pressed, and after that each time they change, until none
// are held.
function demoModifiers(e, start) {
  if (!isDemo() || (!start && !held.length)) return;
  const now = e ? heldNames(e) : [];
  if (now.join('+') === held.join('+')) return;
  held = now;
  inOrder(() => act('DemoModifiers', now));
}

// focusedTerminal returns the terminal in a pane that has the keyboard
// focus, as { itemId, paneId }, or null.
function focusedTerminal() {
  const el = document.activeElement;
  const host = el && el.closest ? el.closest('#panes .term-host[data-term-host]') : null;
  const pane = host ? host.closest('[data-term]') && host.closest('[data-pane-id]') : null;
  return pane ? { itemId: host.dataset.termHost, paneId: pane.dataset.paneId } : null;
}

// commandKeys handles the Command keys of Bench's window, as other Mac
// apps have them, and reports whether it did (decision 64). The menu bar
// doesn't have them, so the web view passes them on. ⌘, opens Settings,
// and ⌘T New tab…, unless a dialog is open. In a terminal, ⌘T opens a new
// terminal in the same pane, in the folder that its shell is in, as its
// menu's New Terminal in does, and ⌘W closes it, as its × does, asking
// first if it's running something. Every other Command key is left alone:
// ⌘W outside a terminal doesn't close Bench's window. A terminal's own
// window closes with ⌘W (shell.go). In the field that renames a tab or
// sub-tab, ⌘, and ⌘T save the name typed first, as a Mac's text field
// ends editing for a menu command, and the dialog gives the focus back to
// the tab or sub-tab as it closes.
function commandKeys(e) {
  const { ui, view } = store;
  if (!view || !e.metaKey || e.ctrlKey || e.altKey || e.shiftKey) return false;
  const renaming = e.target && e.target.id === 'rename-input' ? ui.rename : null;
  const key = (e.key || '').toLowerCase();
  const term = key === 't' || key === 'w' ? focusedTerminal() : null;
  let run = null;
  if (term && key === 't') {
    const it = itemOf(view, term.itemId);
    const folder = (it && (it.here || it.folder)) || '~';
    run = () => act('AddToPane', realPaneOf(view, term.itemId, term.paneId), { type: 'terminal', folder });
  } else if (term && key === 'w') {
    run = () => quickClose(term.itemId);
  } else if (key === ',' && !ui.overlay) {
    run = () => openSettings();
  } else if (key === 't' && !ui.overlay) {
    run = () => openEditor(-1);
  }
  if (!run) return false;
  e.preventDefault();
  e.stopPropagation();
  if (ui.menu) {
    ui.paneLater = null;
    closeMenu(false);
  }
  if (renaming) commitRename();
  run();
  if (renaming && ui.overlay) ui.opener = renaming.back;
  return true;
}

// onCapture gives a key to Settings ▸ Keyboard while one of its fields
// records keys, and otherwise runs the Command keys of Bench's window, and
// in the browser preview, Bench's shortcuts, after telling Go which
// modifier keys are held there (demoModifiers).
function onCapture(e) {
  if (composing(e) || recordKey(e)) return;
  demoModifiers(e, false);
  if (commandKeys(e)) return;
  if (isDemo()) shortcut(e);
}

// onKeyUpCapture gives a key that's let go to Settings ▸ Keyboard while one
// of its fields records modifiers.
function onKeyUpCapture(e) {
  recordKeyUp(e);
  demoModifiers(e, false);
}

// onBlur lets go of every modifier key, as far as Go's hold in the browser
// preview knows, when the page loses the keyboard: it won't see them let
// go of.
function onBlur(e) {
  if (e.target === window) demoModifiers(null, false);
}

// menuKeys moves the focus in an open menu: the arrow keys, Home, and End
// move it among the entries of the menu or submenu that has it, wrapping,
// and Tab, or Escape, closes the menu, giving the focus back to what opened
// it. A window's menu has the focus itself when it opens (openWindowMenu):
// Down Arrow and Home go to its first entry, Up Arrow and End to its last.
// In an entry with a submenu, such as Move to Tab, Right Arrow opens the
// submenu with the focus on its first entry, as Return and Space do (the
// menu-sub action); in the submenu, Left Arrow and Escape close it, giving
// the focus back to that entry. Escape in the menu closes the menu, with
// any submenu that the pointer opened. Moving on from an entry closes its
// submenu.
function menuKeys(e) {
  const { ui } = store;
  const active = document.activeElement;
  const inSub = Boolean(active && active.closest && active.closest('#menu-sub'));
  if (e.key === 'Escape') {
    e.preventDefault();
    if (inSub) closeSubmenu(true);
    else closeMenu(true);
    return;
  }
  if (ui.menu.kind === 'notices') {
    noticeKeys(e);
    return;
  }
  if (e.key === 'Tab') {
    // Focus goes back to the menu's button first, so that Tab moves on
    // from there. Moving on in Bench isn't a reason to focus the pane.
    ui.paneLater = null;
    closeMenu(true);
    return;
  }
  if (inSub && e.key === 'ArrowLeft') {
    e.preventDefault();
    closeSubmenu(true);
    return;
  }
  if (e.key === 'ArrowRight' && active && active.dataset && active.dataset.action === 'menu-sub') {
    e.preventDefault();
    openSubmenu(Number(active.dataset.index), true);
    return;
  }
  const scope = inSub ? '#menu-sub' : '#menu-root > .menu:not(.submenu)';
  const items = Array.from(document.querySelectorAll(scope + ' .menu-item:not([disabled])'));
  if (!items.length) return;
  const i = items.indexOf(active);
  // In a row of buttons, such as a web page's Back, Forward, and zooms,
  // the left and right arrows move along it, as down and up do.
  const row = active && active.closest ? active.closest('.menu-row') : null;
  if (row && (e.key === 'ArrowRight' || e.key === 'ArrowLeft')) {
    const next = items[i + (e.key === 'ArrowRight' ? 1 : -1)];
    e.preventDefault();
    if (next && row.contains(next)) next.focus();
    return;
  }
  // The focus moves to entry el. In the menu, that closes an open submenu.
  const go = (el) => {
    if (inSub || !ui.menu.sub) {
      el.focus();
      return;
    }
    ui.pendingFocus = el.dataset.key;
    closeSubmenu(false);
  };
  if (e.key === 'ArrowDown') {
    e.preventDefault();
    go(items[i + 1] || items[0]);
  } else if (e.key === 'ArrowUp') {
    e.preventDefault();
    go(items[i - 1] || items[items.length - 1]);
  } else if (e.key === 'Home') {
    e.preventDefault();
    go(items[0]);
  } else if (e.key === 'End') {
    e.preventDefault();
    go(items[items.length - 1]);
  }
}

// noticeKeys moves the focus between the notices' buttons, such as Reload
// for config.toml, and the recent messages' Undo and Copy buttons, in the
// order they show: Tab and Shift-Tab, and the arrow keys, which wrap. Tab past the last button, or
// Shift-Tab before the first, closes the notices, giving the focus back to
// their button in the status bar, as Tab does in a menu.
function noticeKeys(e) {
  const { ui } = store;
  const buttons = Array.from(document.querySelectorAll(
    '#notices-menu :is([data-action="notice-run"], [data-action="message-undo"], [data-action="message-copy"])',
  ));
  const i = buttons.indexOf(document.activeElement);
  if (e.key === 'Tab') {
    const next = e.shiftKey ? (i < 0 ? -1 : i - 1) : i + 1;
    if (next < 0 || next >= buttons.length) {
      ui.paneLater = null;
      closeMenu(true);
      return;
    }
    e.preventDefault();
    buttons[next].focus();
    return;
  }
  if (!buttons.length) return;
  if (e.key === 'ArrowDown' || e.key === 'ArrowRight') {
    e.preventDefault();
    (buttons[i + 1] || buttons[0]).focus();
  } else if (e.key === 'ArrowUp' || e.key === 'ArrowLeft') {
    e.preventDefault();
    (buttons[i - 1] || buttons[buttons.length - 1]).focus();
  } else if (e.key === 'Home') {
    e.preventDefault();
    buttons[0].focus();
  } else if (e.key === 'End') {
    e.preventDefault();
    buttons[buttons.length - 1].focus();
  }
}

// trapTab keeps Tab inside the open dialog. A control that's hidden, as
// Settings' list of sections or its pop-up menu of them, whichever the
// dialog's width hides, doesn't count. An element made focusable with
// tabindex 0, as the hold's list of cards (overview.js), counts too, so
// that Tab goes from it to the dialog's buttons and back.
function trapTab(e) {
  const dialog = document.querySelector('#overlay-root .dialog');
  if (!dialog) return;
  const focusables = Array.from(dialog.querySelectorAll('button:not([tabindex="-1"]), input, select, [href], [tabindex="0"]'))
    .filter((el) => !el.disabled && el.getClientRects().length > 0);
  if (!focusables.length) return;
  const first = focusables[0];
  const last = focusables[focusables.length - 1];
  // A question with no default button has the focus itself when it opens
  // (renderQuestion): Tab goes to its first control, Shift-Tab to its last.
  if (document.activeElement === dialog) {
    e.preventDefault();
    (e.shiftKey ? last : first).focus();
  } else if (!dialog.contains(document.activeElement)) {
    e.preventDefault();
    first.focus();
  } else if (e.shiftKey && document.activeElement === first) {
    e.preventDefault();
    last.focus();
  } else if (!e.shiftKey && document.activeElement === last) {
    e.preventDefault();
    first.focus();
  }
}

// composing reports whether a key event belongs to an input method's
// composition: while text is being composed, and, in WebKit, for the
// Return or Escape that ends it, which comes after compositionend with
// keyCode 229.
function composing(e) {
  return e.isComposing || e.keyCode === 229;
}

function overlayKeys(e) {
  const { ui } = store;
  // Keys that the input method uses to compose, choose, or cancel text are
  // its own.
  if (composing(e)) return;
  if (e.key === 'Escape') {
    e.preventDefault();
    // In the add dialog's address step or New terminal… form, or while it
    // waits for a web app, Escape goes back to the list, asking first if
    // something is typed (backToList). In the question before discarding a
    // dialog's changes, it goes back to the dialog. Elsewhere it closes the
    // dialog, asking first if it has changes that aren't saved.
    if (ui.overlay === 'discard') keepEditing();
    else if (ui.overlay === 'add' && ui.add && ui.add.step !== 'list') backToList();
    else dismissOverlay();
    return;
  }
  if (e.key === 'Tab') {
    trapTab(e);
    return;
  }
  if (ui.overlay === 'overview') {
    overviewKeys(e);
    return;
  }
  const id = e.target && e.target.id;
  if ((ui.overlay === 'palette' || ui.overlay === 'add') && id === 'palette-query') {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      moveSelection(1);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      moveSelection(-1);
    } else if (e.key === 'Enter') {
      e.preventDefault();
      // In the add dialog, Shift-Return on a window's row ticks it, and
      // Return adds the windows ticked, if there are any. On another row,
      // Shift-Return is Return.
      const item = ui.listItems[ui.palette.sel];
      if (ui.overlay === 'add' && e.shiftKey && item && item.tick) tickRow(ui.palette.sel);
      else if (ui.overlay === 'add' && ui.add && ui.add.ticked && ui.add.ticked.length) addTicked();
      else runListItem(ui.palette.sel);
    }
    return;
  }
  if (id === 'palette-url' && e.key === 'Enter') {
    e.preventDefault();
    // Option-Return shows the page in Bench, rather than as a web app.
    submitURL(e.altKey);
    return;
  }
  // Return in a field of the New terminal… form adds the terminal.
  if (ui.overlay === 'add' && isTerminalField(id) && e.key === 'Enter') {
    e.preventDefault();
    submitTerminal();
    return;
  }
  if (ui.overlay === 'editor' && id === 'tab-name' && e.key === 'Enter') {
    e.preventDefault();
    save();
  } else if (ui.overlay === 'settings' && (isSettingsNumber(id) || isLauncherField(id)) && e.key === 'Enter') {
    e.preventDefault();
    saveSettings();
  }
}

function onKey(e) {
  const { ui } = store;
  // Return and Escape in the field that renames a tab or sub-tab are its
  // own.
  if (e.target && e.target.id === 'rename-input') {
    renameKeys(e);
    return;
  }
  // So are those in a web page's find bar (pages.js).
  if (e.target && e.target.dataset && e.target.dataset.pageFind && !ui.menu && !ui.overlay) {
    pageFindKeys(e);
    return;
  }
  if (ui.menu) {
    menuKeys(e);
    return;
  }
  // The arrow keys move along a strip of tabs or sub-tabs, or a radio
  // group, such as Settings' choices (roving.js).
  if (rovingKeys(e)) return;
  if (e.target && e.target.classList && e.target.classList.contains('divider') && ARROWS[e.key] && !e.ctrlKey && !e.altKey && !e.metaKey) {
    if (nudgeDivider(e.target, e.key, e.shiftKey)) e.preventDefault();
    return;
  }
  // Return on a divider gives its split's panes equal sizes, as a
  // double-click does.
  if (e.target && e.target.classList && e.target.classList.contains('divider') && e.key === 'Enter' && !e.ctrlKey && !e.altKey && !e.metaKey && !e.shiftKey) {
    if (evenDivider(e.target)) e.preventDefault();
    return;
  }
  if (ui.overlay) overlayKeys(e);
}

// installKeyboard listens for keys.
export function installKeyboard() {
  // Capture, so that shortcuts reach Bench before a focused terminal, and
  // Settings ▸ Keyboard's keys reach it before anything else.
  document.addEventListener('keydown', onCapture, true);
  document.addEventListener('keyup', onKeyUpCapture, true);
  document.addEventListener('keydown', onKey);
  window.addEventListener('blur', onBlur);
}
