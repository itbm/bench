// A popped-out terminal's own window. The shell opens it at
// /?terminal=<item ID>, and it shows just that terminal, filling the window.
// It doesn't call Ready or report a pane area: those belong to Bench's main
// window.

import { call, errorText, on } from './bridge.js';
import { esc, itemOf, labelOf } from './util.js';
import * as terminals from './terminals.js';

// terminalID returns the item ID the page's address asks for, or ''.
export function terminalID(search = window.location.search) {
  return new URLSearchParams(search).get('terminal') || '';
}

let body = null;
let ended = false;
// latest is the latest View, for the terminal's state once it's drawn.
let latest = null;

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
}

// keepOnly keeps this window's terminal while its item exists, wherever it
// is, and notes whether its command has finished.
function keepOnly(id, v) {
  terminals.prune(v, (tid) => tid === id && Boolean(itemOf(v, tid)));
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
  // Keep the toast, for refused input, and the terminals' parking place.
  for (const el of Array.from(win.children)) {
    if (el.id !== 'toast' && el.id !== 'term-park') el.remove();
  }
  body = document.createElement('main');
  body.className = 'pane-body terminal-window';
  body.setAttribute('aria-label', 'Terminal');
  win.prepend(body);

  try {
    await Promise.all([
      on('term:data', (d) => { if (!ended) terminals.write(d); }),
      on('bench:state', (v) => onState(id, v)),
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
