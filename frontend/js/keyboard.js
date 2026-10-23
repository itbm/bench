// Keys. In the browser preview, the UI handles Bench's global shortcuts
// itself, as configured (View.keys), as the prototype does; in the app, Go
// handles them as global shortcuts. Menu, dialog, and divider keys work in
// both.

import { act, closeMenu, closeOverlay, isDemo, store } from './state.js';
import { backToList, moveSelection, runListItem, submitURL, togglePalette } from './palette.js';
import { save } from './editor.js';
import { saveSettings } from './settings.js';
import { nudgeDivider } from './drag.js';
import { keysOf, splitShortcut } from './util.js';
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
function modsOf(e) {
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
  if (matches(e, keys.rearrange)) return () => act('Rearrange');
  if (matches(e, keys.nextWindow)) return () => act('CycleSubtab', 1);
  // The previous window is the next window's shortcut with Shift added.
  if (e.shiftKey && matches({ ...pick(e), shiftKey: false }, keys.nextWindow)) return () => act('CycleSubtab', -1);
  const arrow = ARROWS[e.code];
  if (arrow && withModifiers(e, keys.focusPane)) return () => act('FocusDirection', arrow);
  if (arrow && withModifiers(e, keys.moveWindow)) return () => act('MoveDirection', arrow);
  return null;
}

// pick copies what matches reads from a key event.
function pick(e) {
  return { code: e.code, ctrlKey: e.ctrlKey, altKey: e.altKey, shiftKey: e.shiftKey, metaKey: e.metaKey };
}

// shortcut runs one of Bench's global shortcuts in the browser preview.
function shortcut(e) {
  const { ui, view } = store;
  const keys = keysOf(view);
  const run = ui.overlay ? null : actionFor(e, keys);
  if (run) {
    e.preventDefault();
    e.stopPropagation();
    // The shortcut does its own focusing, so the click that opened the
    // menu doesn't (ui.paneLater): Go could handle the two in either order.
    if (ui.menu) {
      ui.paneLater = null;
      closeMenu(false);
    }
    run();
    return true;
  }
  const ctrlK = e.ctrlKey && !e.altKey && !e.metaKey && e.code === 'KeyK' && !terminals.hasFocus();
  if (matches(e, keys.jump) || ctrlK) {
    e.preventDefault();
    e.stopPropagation();
    if (view) togglePalette();
    return true;
  }
  return false;
}

function onCapture(e) {
  if (isDemo() && !composing(e)) shortcut(e);
}

function menuKeys(e) {
  const { ui } = store;
  if (e.key === 'Escape') {
    e.preventDefault();
    closeMenu(true);
    return;
  }
  if (e.key === 'Tab') {
    // Focus goes back to the menu's button first, so that Tab moves on
    // from there. Moving on in Bench isn't a reason to focus the pane.
    ui.paneLater = null;
    closeMenu(true);
    return;
  }
  if (ui.menu.kind === 'notices') return;
  const items = Array.from(document.querySelectorAll('#menu-root .menu-item:not([disabled])'));
  if (!items.length) return;
  const i = items.indexOf(document.activeElement);
  if (e.key === 'ArrowDown') {
    e.preventDefault();
    (items[i + 1] || items[0]).focus();
  } else if (e.key === 'ArrowUp') {
    e.preventDefault();
    (items[i - 1] || items[items.length - 1]).focus();
  } else if (e.key === 'Home') {
    e.preventDefault();
    items[0].focus();
  } else if (e.key === 'End') {
    e.preventDefault();
    items[items.length - 1].focus();
  }
}

// trapTab keeps Tab inside the open dialog.
function trapTab(e) {
  const dialog = document.querySelector('#overlay-root .dialog');
  if (!dialog) return;
  const focusables = Array.from(dialog.querySelectorAll('button:not([tabindex="-1"]), input, select, [href]'))
    .filter((el) => !el.disabled);
  if (!focusables.length) return;
  const first = focusables[0];
  const last = focusables[focusables.length - 1];
  if (!dialog.contains(document.activeElement)) {
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
    // In the add dialog's address step, or while it waits for a web app,
    // Escape goes back to the list.
    if (ui.overlay === 'add' && ui.add && ui.add.step !== 'list') backToList();
    else closeOverlay();
    return;
  }
  if (e.key === 'Tab') {
    trapTab(e);
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
      runListItem(ui.palette.sel);
    }
    return;
  }
  if (id === 'palette-url' && e.key === 'Enter') {
    e.preventDefault();
    // Option-Return shows the page in Bench, rather than as a web app.
    submitURL(e.altKey);
    return;
  }
  if (ui.overlay === 'editor' && id === 'tab-name' && e.key === 'Enter') {
    e.preventDefault();
    save();
  } else if (ui.overlay === 'settings' && id === 'compact-below' && e.key === 'Enter') {
    e.preventDefault();
    saveSettings();
  }
}

function onKey(e) {
  const { ui } = store;
  if (ui.menu) {
    menuKeys(e);
    return;
  }
  if (e.target && e.target.classList && e.target.classList.contains('divider') && ARROWS[e.key] && !e.ctrlKey && !e.altKey && !e.metaKey) {
    if (nudgeDivider(e.target, e.key, e.shiftKey)) e.preventDefault();
    return;
  }
  if (ui.overlay) overlayKeys(e);
}

// installKeyboard listens for keys.
export function installKeyboard() {
  // Capture, so that shortcuts reach Bench before a focused terminal.
  document.addEventListener('keydown', onCapture, true);
  document.addEventListener('keydown', onKey);
}
