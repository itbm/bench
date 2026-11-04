// A popped-out terminal's own window. The shell opens it at
// /?terminal=<item ID>, and it shows just that terminal, filling the window.
// It doesn't call Ready or report a pane area: those belong to Bench's main
// window. When the user closes the window while the terminal is running
// something, Go keeps it open and asks here first (confirm-close-terminal).
// A right-click in the terminal opens its menu, as in the main window. A bar
// along the window's top, and the menu, have Back to Bench, which puts the
// terminal back in the focused pane, as the Window menu's Back to Bench and
// dragging the window onto a pane do.

import { call, errorText, on } from './bridge.js';
import { act } from './state.js';
import { esc, itemOf, labelOf } from './util.js';
import { closeText, renderQuestion } from './quit.js';
import { renderTerminalMenu } from './menu.js';
import * as terminals from './terminals.js';
import { setAppearance } from './theme.js';

// terminalID returns the item ID the page's address asks for, or ''.
export function terminalID(search = window.location.search) {
  return new URLSearchParams(search).get('terminal') || '';
}

let body = null;
let bar = null;
let ended = false;
// latest is the latest View, for the terminal's state once it's drawn.
let latest = null;
// question is the question before closing the window, while it's open:
// { terminal, dontAsk, busy }, where terminal is as Go named it.
let question = null;
// menu is the terminal's right-click menu, while it's open: { entries }.
let menu = null;

// showMessage replaces the terminal with a sentence, such as why it isn't
// there.
function showMessage(text) {
  body.removeAttribute('data-term');
  body.innerHTML = '<div class="empty"><span class="status-text">' + esc(text) + '</span></div>';
}

// onState follows the item in Go's View: the window's title follows the
// terminal's, its text size, Option key, and colours follow Settings, the
// colours as Settings previews them too, and the terminal ends when its
// item does.
function onState(id, v) {
  if (ended || !v || !v.items) return;
  latest = v;
  setAppearance(v.appearance || (v.settings || {}).appearance);
  terminals.configure(v);
  if (itemOf(v, id)) {
    document.title = labelOf(v, id);
    keepOnly(id, v);
    return;
  }
  ended = true;
  closeMenu(id, false);
  keepOnly(id, v);
  showMessage('This terminal has closed.');
  // There's nothing left to put back.
  if (bar) bar.hidden = true;
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

// backToBench asks Go to put the terminal back in the focused pane, which
// closes this window. If Go refuses, a toast says why.
function backToBench(id) {
  if (!ended) act('BackToBench', id);
}

// renderBar returns the HTML of the bar along the window's top.
function renderBar() {
  return '<span class="term-window-note">' + esc('Popped out of Bench') + '</span>'
    + '<button type="button" class="btn small" data-action="back-to-bench" data-key="back-to-bench"'
    + ' title="' + esc('Put this terminal back in Bench, in the focused pane') + '">' + esc('Back to Bench') + '</button>';
}

// openMenu opens the terminal's menu (terminals.menuEntries, and Back to
// Bench) at the pointer, for a right-click event e in the terminal, within
// the window, and gives its first entry the keyboard.
function openMenu(id, e) {
  const root = document.getElementById('menu-root');
  const own = terminals.menuEntries(id);
  if (!root || !own.length) return;
  const entries = [...own, { sep: true }, { label: 'Back to Bench', run: () => backToBench(id) }];
  menu = { entries };
  root.innerHTML = renderTerminalMenu(entries, 'left: 0px; top: 0px');
  const el = root.querySelector('.menu');
  const win = document.getElementById('app-window');
  const box = win.getBoundingClientRect();
  const within = (at, size, room) => Math.max(8, Math.min(at, room - size - 8));
  el.style.left = within(e.clientX - box.left, el.offsetWidth, win.clientWidth) + 'px';
  el.style.top = within(e.clientY - box.top, el.offsetHeight, win.clientHeight) + 'px';
  const first = el.querySelector('.menu-item:not([disabled])');
  if (first) first.focus();
}

// closeMenu closes the terminal's menu, giving the terminal the keyboard
// back if refocus is set.
function closeMenu(id, refocus) {
  if (!menu) return;
  menu = null;
  const root = document.getElementById('menu-root');
  if (root) root.innerHTML = '';
  if (refocus) terminals.focus(id);
}

// runMenuEntry closes the menu and runs its entry at index.
function runMenuEntry(id, index) {
  const entry = menu && menu.entries[index];
  closeMenu(id, true);
  if (entry && entry.run && !entry.disabled) entry.run();
}

// menuKeys moves through the menu's entries with the arrow keys, Home, and
// End, as in the main window. Escape and Tab close it.
function menuKeys(id, e) {
  if (e.key === 'Escape' || e.key === 'Tab') {
    e.preventDefault();
    e.stopPropagation();
    closeMenu(id, true);
    return;
  }
  const items = Array.from(document.querySelectorAll('#menu-root .menu-item:not([disabled])'));
  const i = items.indexOf(document.activeElement);
  let next = null;
  if (e.key === 'ArrowDown') next = items[i + 1] || items[0];
  else if (e.key === 'ArrowUp') next = items[i - 1] || items[items.length - 1];
  else if (e.key === 'Home') next = items[0];
  else if (e.key === 'End') next = items[items.length - 1];
  if (!next) return;
  e.preventDefault();
  e.stopPropagation();
  next.focus();
}

// installMenu opens the terminal's menu on a right-click in it, but not in
// its find bar, whose field has the usual menu for text, and closes it on a
// click outside it, or when another app takes the keyboard.
function installMenu(id) {
  document.addEventListener('contextmenu', (e) => {
    const host = e.target.closest ? e.target.closest('.term-host') : null;
    if (!host || e.target.closest('.term-find') || question || ended) return;
    e.preventDefault();
    openMenu(id, e);
  });
  document.addEventListener('click', (e) => {
    if (!menu || !e.target.closest) return;
    const entry = e.target.closest('#menu-root [data-action="menu-run"]');
    if (entry) runMenuEntry(id, Number(entry.dataset.index));
    else if (!e.target.closest('#menu-root .menu')) closeMenu(id, false);
  });
  // Capture, so that the menu's keys reach it before the terminal.
  document.addEventListener('keydown', (e) => {
    if (menu && !e.isComposing) menuKeys(id, e);
  }, true);
  window.addEventListener('blur', (e) => {
    if (e.target === window) closeMenu(id, false);
  });
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
  // Keep the toast, for refused input, the terminal's menu, the question
  // before closing, and the terminals' parking place.
  const keep = new Set(['toast', 'term-park', 'menu-root', 'overlay-root']);
  for (const el of Array.from(win.children)) {
    if (!keep.has(el.id)) el.remove();
  }
  body = document.createElement('main');
  body.className = 'pane-body terminal-window';
  body.setAttribute('aria-label', 'Terminal');
  win.prepend(body);
  bar = document.createElement('div');
  bar.className = 'term-window-bar';
  bar.innerHTML = renderBar();
  win.prepend(bar);
  bar.querySelector('[data-action="back-to-bench"]').addEventListener('click', () => backToBench(id));
  installQuestion(id);
  installMenu(id);

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
    bar.hidden = true;
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
