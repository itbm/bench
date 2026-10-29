// A popped-out terminal's own window. The shell opens it at
// /?terminal=<item ID>, and it shows just that terminal, filling the window.
// It doesn't call Ready or report a pane area: those belong to Bench's main
// window. When the user closes the window while the terminal is running
// something, Go keeps it open and asks here first (confirm-close-terminal).

import { call, errorText, on } from './bridge.js';
import { act } from './state.js';
import { esc, itemOf, labelOf } from './util.js';
import { closeText, renderQuestion } from './quit.js';
import * as terminals from './terminals.js';

// terminalID returns the item ID the page's address asks for, or ''.
export function terminalID(search = window.location.search) {
  return new URLSearchParams(search).get('terminal') || '';
}

let body = null;
let ended = false;
// latest is the latest View, for the terminal's state once it's drawn.
let latest = null;
// question is the question before closing the window, while it's open:
// { terminal, dontAsk, busy }, where terminal is as Go named it.
let question = null;

// showMessage replaces the terminal with a sentence, such as why it isn't
// there.
function showMessage(text) {
  body.removeAttribute('data-term');
  body.innerHTML = '<div class="empty"><span class="status-text">' + esc(text) + '</span></div>';
}

// onState follows the item in Go's View: the window's title follows the
// terminal's, and the terminal ends when its item does.
function onState(id, v) {
  if (ended || !v || !v.items) return;
  latest = v;
  if (itemOf(v, id)) {
    document.title = labelOf(v, id);
    keepOnly(id, v);
    return;
  }
  ended = true;
  keepOnly(id, v);
  showMessage('This terminal has closed.');
  // There's nothing left to ask about.
  question = null;
  drawQuestion();
}

// keepOnly keeps this window's terminal while its item exists, wherever it
// is, and notes whether its command has finished.
function keepOnly(id, v) {
  terminals.prune(v, (tid) => tid === id && Boolean(itemOf(v, tid)));
}

// askToClose asks before closing the window, for Go's confirm-close-terminal
// command for this terminal: the user closed the window, and Go kept it
// open, as the terminal is running something.
function askToClose(id, cmd) {
  if (!cmd || cmd.name !== 'confirm-close-terminal' || cmd.itemId !== id || ended) return;
  const t = (cmd.terminals || []).find((x) => x.id === id) || { title: document.title };
  question = { terminal: t, dontAsk: question ? question.dontAsk : false, busy: false };
  drawQuestion();
  focusIn('[data-key="question-run"]');
}

// drawQuestion draws the question, or takes it away.
function drawQuestion() {
  const root = document.getElementById('overlay-root');
  if (!root) return;
  root.innerHTML = question
    ? '<div class="overlay" data-action="backdrop">' + renderQuestion({
      title: 'Close this terminal?', text: closeText(question.terminal), run: 'Close', action: 'close-run',
      dontAsk: question.dontAsk, busy: question.busy, movable: false,
    }) + '</div>'
    : '';
}

// focusIn focuses the question's element that matches selector.
function focusIn(selector) {
  const el = document.querySelector('#overlay-root ' + selector);
  if (el) el.focus();
}

// cancelClose takes the question away, and gives the terminal the keyboard
// back.
function cancelClose(id) {
  if (!question || question.busy) return;
  question = null;
  drawQuestion();
  terminals.focus(id);
}

// closeTerminal is the question's Close: Go ends the terminal, which closes
// the window, and with Don't ask again ticked, turns the question off. If
// Go refuses, a toast says why, and the question stays.
async function closeTerminal(id) {
  const q = question;
  if (!q || q.busy) return;
  q.busy = true;
  drawQuestion();
  const r = await act('CloseTerminal', id, q.dontAsk);
  if (question !== q) return;
  q.busy = false;
  if (r.ok) {
    question = null;
    drawQuestion();
    return;
  }
  drawQuestion();
  focusIn('[data-key="question-run"]');
}

// installQuestion handles the question's buttons and keys: Escape and a
// click outside it cancel, and Tab stays inside it.
function installQuestion(id) {
  document.addEventListener('click', (e) => {
    const target = e.target.closest ? e.target.closest('#overlay-root [data-action]') : null;
    if (!target) return;
    const action = target.dataset.action;
    if (action === 'close') cancelClose(id);
    else if (action === 'close-run') closeTerminal(id);
    else if (action === 'backdrop' && e.target === target) cancelClose(id);
  });
  document.addEventListener('change', (e) => {
    if (question && e.target.dataset && e.target.dataset.question === 'dont-ask') question.dontAsk = e.target.checked;
  });
  // Capture, so that Escape and Tab reach the question before the terminal.
  document.addEventListener('keydown', (e) => {
    if (!question || e.isComposing) return;
    if (e.key === 'Escape') {
      e.preventDefault();
      e.stopPropagation();
      cancelClose(id);
    } else if (e.key === 'Tab') {
      keepTabIn(e);
    }
  }, true);
}

// keepTabIn keeps Tab inside the question.
function keepTabIn(e) {
  const els = Array.from(document.querySelectorAll('#overlay-root input, #overlay-root button')).filter((el) => !el.disabled);
  if (!els.length) return;
  const i = els.indexOf(document.activeElement);
  const next = i < 0 ? 0 : (i + (e.shiftKey ? -1 : 1) + els.length) % els.length;
  e.preventDefault();
  e.stopPropagation();
  els[next].focus();
}

// loadFont waits briefly for the terminal's font, so that it measures the
// right one.
function loadFont() {
  if (!document.fonts || !document.fonts.load) return Promise.resolve();
  const font = document.fonts.load('400 13px "IBM Plex Mono"').catch(() => {});
  return Promise.race([font, new Promise((r) => setTimeout(r, 1500))]);
}

// bootTerminalWindow draws the terminal with item ID id in the whole window.
export async function bootTerminalWindow(id) {
  document.title = 'Terminal';
  document.body.classList.add('is-terminal-window');
  document.getElementById('demo-bar').hidden = true;
  const win = document.getElementById('app-window');
  win.dataset.screen = 'terminal';
  // Keep the toast, for refused input, the question before closing, and
  // the terminals' parking place.
  for (const el of Array.from(win.children)) {
    if (el.id !== 'toast' && el.id !== 'term-park' && el.id !== 'overlay-root') el.remove();
  }
  body = document.createElement('main');
  body.className = 'pane-body terminal-window';
  body.setAttribute('aria-label', 'Terminal');
  win.prepend(body);
  installQuestion(id);

  try {
    await Promise.all([
      on('term:data', (d) => { if (!ended) terminals.write(d); }),
      on('bench:state', (v) => onState(id, v)),
      on('bench:command', (cmd) => askToClose(id, cmd)),
    ]);
    // Go refuses the snapshot of an item that isn't a terminal any more.
    await call('TermSnapshot', id);
  } catch (err) {
    ended = true;
    showMessage(errorText(err));
    return;
  }
  await loadFont();
  if (ended) return;
  body.dataset.term = id;
  terminals.attach(win);
  if (latest) keepOnly(id, latest);
  terminals.focus(id);
}
