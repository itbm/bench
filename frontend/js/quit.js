// The questions Bench asks before it ends terminals that are running
// something: before quitting, in the main window (Go's confirm-quit
// command), and before closing a popped-out terminal's window, in that
// window (confirm-close-terminal, terminal-window.js). Each has Don't ask
// again, which turns confirm_quit_busy_terminals off, as Settings can, and
// Settings can turn it back on. Go asks the question before quitting for
// Quit and Reopen Bench too (QuitAndReopen), with reopen set.

import { act, closeOverlay, openerKey, store, update } from './state.js';
import { esc } from './util.js';

// BACK_TO are the dialogs that the question before quitting can cover: its
// Cancel goes back to the one that was open, with its draft (closeOverlay).
const BACK_TO = new Set(['palette', 'add', 'editor', 'settings', 'guide']);

// terminalName names a terminal by its sub-tab title, in quotes, and its
// tab, or for a terminal in a window of its own, says so.
export function terminalName(t) {
  return '“' + ((t && t.title) || 'Terminal') + '” (' + ((t && t.tab) || 'in its own window') + ')';
}

// quitText is the question before quitting, naming the terminals that are
// running something, such as "2 terminals are still running: “claude”
// (Code), “npm run dev” (Web). Quit Bench and end them?" With reopen, it's
// Quit and Reopen Bench's.
export function quitText(terminals, reopen = false) {
  const list = terminals || [];
  const names = list.map(terminalName).join(', ');
  const ask = reopen ? 'Quit and reopen Bench, and end ' : 'Quit Bench and end ';
  if (list.length === 1) return 'A terminal is still running: ' + names + '. ' + ask + 'it?';
  return list.length + ' terminals are still running: ' + names + '. ' + ask + 'them?';
}

// closeText is the question before closing a popped-out terminal's window,
// naming what the terminal runs, or failing that, the terminal.
export function closeText(t) {
  const what = (t && (t.running || t.title)) || 'A program';
  return '“' + what + '” is still running in this terminal. Close the window and end it?';
}

// renderQuestion returns a question's dialog: its title and text, a Don't
// ask again tick box, Cancel, and the button that goes ahead (run, which
// runs the data-action action). A movable one moves by its header
// (dialogs.js). While busy, its buttons are disabled.
export function renderQuestion({ title, text, run, action, dontAsk, busy, movable }) {
  const off = busy ? ' disabled' : '';
  return '<div class="dialog question-sheet" role="alertdialog" aria-modal="true" aria-labelledby="question-title" aria-describedby="question-text">'
    + '<div class="question-head' + (movable ? ' dialog-handle' : '') + '"><h2 id="question-title">' + esc(title) + '</h2>'
    + '<p id="question-text">' + esc(text) + '</p></div>'
    + '<div class="editor-foot">'
    + '<label class="toggle"><input type="checkbox" data-question="dont-ask" data-key="question-dont-ask"'
    + (dontAsk ? ' checked' : '') + off + '><span>Don\'t ask again</span></label>'
    + '<div class="spacer">'
    + '<button type="button" class="btn" data-action="close" data-key="question-cancel"' + off + '>Cancel</button>'
    + '<button type="button" class="btn primary" data-action="' + action + '" data-key="question-run"' + off + '>' + esc(run) + '</button>'
    + '</div></div></div>';
}

// openQuit asks before quitting, for Go's confirm-quit command, which has
// the terminals that are running something. It covers a dialog that's
// open, which Cancel goes back to; asked again, it names the terminals
// anew, keeping the tick box.
export function openQuit(cmd) {
  const { ui } = store;
  const terminals = (cmd && cmd.terminals) || [];
  const q = ui.overlay === 'quit' ? ui.quit : null;
  if (!ui.overlay) ui.opener = openerKey();
  ui.menu = null;
  ui.paneLater = null;
  ui.quit = {
    terminals,
    // reopen is set for Quit and Reopen Bench, whose button calls
    // QuitAndReopen rather than Quit.
    reopen: Boolean(cmd && cmd.reopen),
    dontAsk: q ? q.dontAsk : false,
    // busy is set while Quit is on its way.
    busy: false,
    back: q ? q.back : (BACK_TO.has(ui.overlay) ? ui.overlay : null),
  };
  ui.overlay = 'quit';
  ui.pendingFocus = 'question-run';
  update();
}

// setDontAsk ticks or unticks the open question's Don't ask again: the
// question before quitting, or before closing (close.js).
export function setDontAsk(on) {
  const { ui } = store;
  const q = ui.overlay === 'close' ? ui.close : ui.quit;
  if (q) q.dontAsk = on;
}

// confirmQuit is the question's Quit, or Quit and Reopen: Go quits Bench,
// without asking again, and opens it again for the latter, and with Don't
// ask again ticked, turns the question off first.
// The question stays until Go answers, so that a double click quits once,
// and closes, with any dialog it covered, if Go answers at all, as in the
// browser preview, which never quits.
export async function confirmQuit() {
  const { ui } = store;
  const q = ui.quit;
  if (!q || q.busy) return;
  q.busy = true;
  update();
  const r = q.reopen ? await act('QuitAndReopen', true, q.dontAsk) : await act('Quit', q.dontAsk);
  q.busy = false;
  if (ui.quit !== q || ui.overlay !== 'quit') return;
  if (r.ok) {
    q.back = null;
    closeOverlay();
  } else {
    ui.pendingFocus = 'question-run';
    update();
  }
}

// renderQuit returns the question before quitting.
export function renderQuit(view, ui) {
  const q = ui.quit;
  if (!q) return '';
  return renderQuestion({
    title: q.reopen ? 'Quit and Reopen Bench?' : 'Quit Bench?', text: quitText(q.terminals, q.reopen),
    run: q.reopen ? 'Quit and Reopen' : 'Quit', action: 'quit-run', dontAsk: q.dontAsk, busy: q.busy, movable: true,
  });
}
