// The Keyboard Shortcuts sheet: Bench's shortcuts as configured, to read.
// Its first list is Settings ▸ Keyboard's (View.shortcuts), in the same
// words, so that the two always agree; then the keys that work in a
// terminal, which aren't configured. Help ▸ Keyboard Shortcuts, Jump to…,
// and the status bar's key hints open it, and its Change Shortcuts… opens
// Settings ▸ Keyboard.

import { openerKey, store, update } from './state.js';
import { esc, ICONS } from './util.js';
import { addedText, displayOf } from './keyboard-settings.js';

// TERMINAL_KEYS are the keys that work while a terminal has the keyboard
// (terminalKey in terminals.js), and opening a link.
const TERMINAL_KEYS = [
  ['Find', '⌘F', 'then ⌘G and ⇧⌘G for the next and previous match'],
  ['Make the text bigger or smaller', '⌘+ and ⌘−', 'for now; ⌘0 puts it back'],
  ['Clear', '⌘K', ''],
  ['Select everything', '⌘A', ''],
  ['Open a link', '⌘-click', ''],
];

// openShortcuts opens the sheet.
export function openShortcuts() {
  const { ui } = store;
  if (ui.overlay === 'shortcuts') return;
  if (!ui.overlay) ui.opener = openerKey();
  ui.menu = null;
  ui.paneLater = null;
  ui.add = null;
  ui.editor = null;
  ui.settings = null;
  ui.overlay = 'shortcuts';
  ui.pendingFocus = 'shortcuts-done';
  update();
}

// shortcutRow returns the HTML of one row: what the shortcut does, its
// keys, and, if they don't work, why.
function shortcutRow(name, keys, more, problem, off) {
  return '<div class="sheet-row' + (off ? ' off' : '') + '"><dt>' + esc(name) + '</dt>'
    + '<dd><kbd>' + esc(keys) + '</kbd>' + (more ? ' <span class="key-added">' + esc(more) + '</span>' : '')
    + (problem ? '<span class="sheet-problem">' + ICONS.warning + esc(problem) + '</span>' : '') + '</dd></div>';
}

// renderShortcuts returns the sheet's HTML.
export function renderShortcuts(view) {
  const rows = view.shortcuts || [];
  const global = rows.map((r) => {
    if (!r.value) return shortcutRow(r.label || r.field, 'Off', '', '', true);
    const keys = r.display || displayOf(r.value);
    return shortcutRow(r.label || r.field, keys, addedText(r, keys), r.problem, false);
  }).join('');
  const intro = view.demo
    ? 'In the browser preview, these work while the page has the keyboard.'
    : 'These work in every app, even while Bench is hidden.';
  return '<div class="dialog shortcuts-sheet" role="dialog" aria-modal="true" aria-labelledby="shortcuts-title" aria-describedby="shortcuts-intro">'
    + '<div class="guide-head dialog-handle"><h2 id="shortcuts-title">Keyboard Shortcuts</h2>'
    + '<p id="shortcuts-intro">' + esc(intro) + '</p></div>'
    + '<div class="dialog-body" data-scroll="shortcuts-body">'
    + (global ? '<dl class="sheet-rows" aria-label="Bench\'s shortcuts">' + global + '</dl>' : '')
    + '<h3 class="sheet-heading" id="shortcuts-terminal">In a terminal</h3>'
    + '<dl class="sheet-rows" aria-labelledby="shortcuts-terminal">'
    + TERMINAL_KEYS.map(([name, keys, more]) => shortcutRow(name, keys, more, '', false)).join('') + '</dl>'
    + '</div>'
    + '<div class="editor-foot">'
    + '<button type="button" class="btn" data-action="keyboard-settings" data-key="shortcuts-change">Change Shortcuts…</button>'
    + '<div class="spacer"><button type="button" class="btn primary" data-action="close" data-key="shortcuts-done">Done</button></div>'
    + '</div></div>';
}
