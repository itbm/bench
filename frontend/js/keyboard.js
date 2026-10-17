// Keys. In the browser preview, the UI handles Bench's global shortcuts
// itself, as the prototype does; in the app, Go handles them as global
// shortcuts. Menu, dialog, and divider keys work in both.

import { act, closeMenu, closeOverlay, isDemo, store } from './state.js';
import { backToList, moveSelection, runListItem, submitURL, togglePalette } from './palette.js';
import { save } from './editor.js';
import { nudgeDivider } from './drag.js';
import * as terminals from './terminals.js';

const ARROWS = { ArrowLeft: 'left', ArrowRight: 'right', ArrowUp: 'up', ArrowDown: 'down' };

// shortcut runs one of Bench's global shortcuts in the browser preview.
function shortcut(e) {
  const { ui, view } = store;
  const chord = e.ctrlKey && e.altKey && !e.metaKey;
  if (chord && !ui.overlay) {
    let run = null;
    if (/^Digit[1-9]$/.test(e.code)) run = () => act('SwitchTab', Number(e.code.slice(5)) - 1);
    else if (e.code === 'Digit0') run = () => act('Rearrange');
    else if (e.code === 'BracketRight') run = () => act('StepTab', 1);
    else if (e.code === 'BracketLeft') run = () => act('StepTab', -1);
    else if (e.code === 'Tab') run = () => act('CycleSubtab', e.shiftKey ? -1 : 1);
    else if (ARROWS[e.code]) run = () => act(e.shiftKey ? 'MoveDirection' : 'FocusDirection', ARROWS[e.code]);
    if (run) {
      e.preventDefault();
      e.stopPropagation();
      if (ui.menu) closeMenu(false);
      run();
      return true;
    }
  }
  const ctrlK = e.ctrlKey && !e.altKey && !e.metaKey && e.code === 'KeyK' && !terminals.hasFocus();
  if ((chord && e.code === 'Space') || ctrlK) {
    e.preventDefault();
    e.stopPropagation();
    if (view) togglePalette();
    return true;
  }
  return false;
}

function onCapture(e) {
  if (isDemo()) shortcut(e);
}

function menuKeys(e) {
  const { ui } = store;
  if (e.key === 'Escape') {
    e.preventDefault();
    closeMenu(true);
    return;
  }
  if (e.key === 'Tab') {
    closeMenu(false);
    return;
  }
  if (ui.menu.kind !== 'window') return;
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

function overlayKeys(e) {
  const { ui } = store;
  if (e.key === 'Escape') {
    e.preventDefault();
    if (ui.overlay === 'add' && ui.add && ui.add.step === 'url') backToList();
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
    } else if (e.key === 'Enter' && !e.isComposing) {
      e.preventDefault();
      runListItem(ui.palette.sel);
    }
    return;
  }
  if (id === 'palette-url' && e.key === 'Enter' && !e.isComposing) {
    e.preventDefault();
    submitURL();
    return;
  }
  if (ui.overlay === 'editor' && (id === 'tab-name' || id === 'compact-below') && e.key === 'Enter' && !e.isComposing) {
    e.preventDefault();
    save();
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
