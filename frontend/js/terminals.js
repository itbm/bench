// Terminal items, drawn with xterm.js. Each item gets one Terminal that lives
// as long as the item is in a tab (or, in a popped-out terminal's window, as
// long as it exists): renders move its element into the new pane body rather
// than making a new one. Only one window draws each terminal, so that only
// one answers its program's queries.

import { Terminal } from '../vendor/xterm/xterm.mjs';
import { FitAddon } from '../vendor/xterm-addon-fit/addon-fit.mjs';
import { call } from './bridge.js';
import { act, actLatest } from './state.js';
import { itemOf } from './util.js';

// sessions holds each terminal by item ID.
const sessions = new Map();

// ACK_EVERY is how much output, in bytes, a terminal draws between telling
// Go how far it has got (TermAck). Go stops reading a program's output once
// the views are 256 KB behind, so this must be well under that.
const ACK_EVERY = 64 * 1024;

// The focus reports xterm.js sends a program that asks for them (DECSET
// 1004) as the terminal gets and loses the keyboard focus.
const FOCUS_IN = '\x1b[I';
const FOCUS_OUT = '\x1b[O';

// ENDED is written after the output of a terminal whose command has
// finished. It isn't part of the output, so each view writes it itself.
const ENDED = '\r\n\x1b[2m[The command has finished. Press Return to run it again.]\x1b[0m\r\n';

// moving is set while a render moves the terminals out of the pane bodies
// and back (park, then unpark).
let moving = false;

function token(name, fallback) {
  const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  return v || fallback;
}

function theme() {
  const bg = token('--bg-pane', '#0E1014');
  const fg = token('--text', '#E6E8EC');
  return {
    background: bg,
    foreground: fg,
    cursor: fg,
    cursorAccent: bg,
    selectionBackground: 'rgba(242, 163, 58, 0.32)',
    black: '#20252E',
    red: token('--danger', '#F29C9C'),
    green: token('--ok', '#8FD694'),
    yellow: token('--syn-type', '#F2C97D'),
    blue: token('--syn-func', '#82C4FF'),
    magenta: token('--syn-keyword', '#D7A6FF'),
    cyan: '#7FD4D0',
    white: token('--text-muted', '#A6ADB9'),
    brightBlack: token('--text-faint', '#7D8594'),
    brightRed: '#F7B8B8',
    brightGreen: '#B2E6B5',
    brightYellow: token('--accent', '#F2A33A'),
    brightBlue: '#A8D6FF',
    brightMagenta: '#E4C2FF',
    brightCyan: '#A6E4E1',
    brightWhite: token('--text-strong', '#F1F3F6'),
  };
}

function reducedMotion() {
  return window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
}

// decode turns base64 into bytes, or no bytes if it isn't base64.
export function decode(b64) {
  let bin;
  try {
    bin = atob(b64 || '');
  } catch {
    return new Uint8Array(0);
  }
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

function create(id) {
  const host = document.createElement('div');
  host.className = 'term-host';
  host.dataset.termHost = id;
  const term = new Terminal({
    fontFamily: "'IBM Plex Mono', ui-monospace, 'SF Mono', Menlo, monospace",
    fontSize: 12.5,
    lineHeight: 1.35,
    cursorBlink: !reducedMotion(),
    scrollback: 5000,
    macOptionIsMeta: false,
    theme: theme(),
    linkHandler: links(host),
  });
  const fit = new FitAddon();
  term.loadAddon(fit);
  // early holds output that arrives while the snapshot is on its way, as
  // { at, bytes }; seen is the offset the output written so far ends at,
  // drawn how far xterm.js has drawn it, and acked how far Go has been told
  // about. replaying is set while the snapshot is drawn. typed holds input
  // waiting to be sent, and sending is set while some is on its way. ended
  // follows the item's ended, and noted is set once ENDED is written.
  // While the terminal moves, hadFocus says whether it had the focus
  // before, and focusHeld is set if a focus report was held back.
  const s = {
    id, host, term, fit, opened: false, ready: false, early: [], cols: 0, rows: 0, frame: 0, ro: null,
    seen: 0, drawn: 0, acked: 0, replaying: false, typed: '', sending: false, ended: false, noted: false,
    hadFocus: false, focusHeld: false,
  };
  term.onData((data) => input(s, data));
  // Mouse reports in the default (X10) encoding come as binary strings.
  // TermInput takes text, so only reports that are plain ASCII (positions
  // up to column and row 95) can be passed on unchanged.
  term.onBinary((data) => {
    if (/^[\x00-\x7f]*$/.test(data)) input(s, data);
  });
  s.ro = new ResizeObserver(() => scheduleFit(s));
  s.ro.observe(host);
  sessions.set(id, s);
  return s;
}

// links returns the handler for the links that programs print with OSC 8 in
// the terminal drawn in host. The output chooses both a link's address and
// the text it covers, which can fill the screen, so a link opens in the
// default browser only on Command-click, never on a click meant to focus
// the terminal or select text, and holding the pointer over it shows the
// address it opens. xterm.js passes on only http and https links, and Go
// opens only those.
function links(host) {
  return {
    activate(event, uri) {
      if (!event || !event.metaKey) return;
      event.preventDefault();
      act('OpenLink', uri);
    },
    hover(event, uri) {
      showLink(host, event, uri);
    },
    leave() {
      const tip = linkTip(host, false);
      if (tip) tip.hidden = true;
    },
  };
}

// The most of each part of a link's address that its tooltip shows, in
// characters: what comes before the host (the scheme, and any user name and
// password), the host, and what comes after it. The output chooses the
// address, so this keeps the tooltip small, and the host in view, however
// long the address is.
const LINK_BEFORE = 40;
const LINK_HOST = 120;
const LINK_AFTER = 160;

// How far a link's tooltip sits below the pointer, clear of the pointer
// itself, or above it when there's no room below, and how close it comes to
// the terminal's edges, in pixels.
const LINK_BELOW = 18;
const LINK_ABOVE = 8;
const LINK_EDGE = 4;

// linkParts splits a link's address, as the browser opens it, into the
// parts its tooltip shows: what comes before the host, the host, and what
// comes after it, each cut to its most. The URL parser writes the address
// in ASCII, with the host in punycode, so letters from other scripts that
// look like Latin ones show as what they are, and any part can be cut
// anywhere. A long host keeps its end, which names the site, and a user
// name keeps the @ after it, which shows that the host comes next. It
// returns null for an address that doesn't parse, which xterm.js doesn't
// pass on.
function linkParts(uri) {
  let u;
  try {
    u = new URL(uri);
  } catch {
    return null;
  }
  const href = u.href;
  let before = u.protocol + '//';
  if (u.username || u.password) before += u.username + (u.password ? ':' + u.password : '') + '@';
  let host = u.host;
  if (!href.startsWith(before + host)) {
    before = '';
    host = '';
  }
  let after = href.slice(before.length + host.length);
  if (before.length > LINK_BEFORE) before = before.slice(0, LINK_BEFORE - 2) + '…@';
  if (host.length > LINK_HOST) host = '…' + host.slice(host.length - LINK_HOST + 1);
  if (after.length > LINK_AFTER) after = after.slice(0, LINK_AFTER - 1) + '…';
  return { before, host, after };
}

// linkTip returns host's link tooltip, making it first if make is set.
function linkTip(host, make) {
  let tip = host.querySelector(':scope > .term-link');
  if (tip || !make) return tip;
  tip = document.createElement('div');
  tip.className = 'term-link';
  tip.setAttribute('role', 'tooltip');
  tip.hidden = true;
  const address = document.createElement('div');
  address.className = 'term-link-address';
  address.append(document.createElement('span'), document.createElement('strong'), document.createElement('span'));
  const hint = document.createElement('div');
  hint.className = 'term-link-hint';
  hint.textContent = 'Command-click to open';
  tip.append(address, hint);
  host.appendChild(tip);
  return tip;
}

// showLink shows a link's address, with its host in bold, and how to open
// it, in a tooltip near the pointer and within the terminal's element host.
function showLink(host, event, uri) {
  const parts = linkParts(uri);
  if (!parts || !event) return;
  const tip = linkTip(host, true);
  const [before, site, after] = tip.firstChild.children;
  before.textContent = parts.before;
  site.textContent = parts.host;
  after.textContent = parts.after;
  // It's measured at the top left, where it has the most room.
  tip.style.left = '0px';
  tip.style.top = '0px';
  tip.hidden = false;
  const box = host.getBoundingClientRect();
  const x = event.clientX - box.left;
  const y = event.clientY - box.top;
  const room = (size, whole) => Math.max(LINK_EDGE, whole - size - LINK_EDGE);
  let top = y + LINK_BELOW;
  if (top > room(tip.offsetHeight, host.clientHeight)) top = y - LINK_ABOVE - tip.offsetHeight;
  tip.style.left = Math.max(LINK_EDGE, Math.min(x, room(tip.offsetWidth, host.clientWidth))) + 'px';
  tip.style.top = Math.max(LINK_EDGE, Math.min(top, room(tip.offsetHeight, host.clientHeight))) + 'px';
}

// input handles what xterm.js sends the program: what's typed, and its
// answers to the program's queries. While it draws a snapshot, its answers
// are to queries from long ago, which nothing is waiting for, so they're
// dropped. While a render moves the terminal, the focus reports that the
// move makes are held back (see unpark).
function input(s, data) {
  if (s.replaying) return;
  if (moving && (data === FOCUS_IN || data === FOCUS_OUT)) {
    s.focusHeld = true;
    return;
  }
  send(s, data);
}

// send sends input to the terminal's session, in order. Go can handle calls
// out of order, so one TermInput is on its way at a time, and what's typed
// meanwhile goes together in the next.
async function send(s, data) {
  s.typed += data;
  if (s.sending) return;
  s.sending = true;
  while (s.typed) {
    const chunk = s.typed;
    s.typed = '';
    await act('TermInput', s.id, chunk);
  }
  s.sending = false;
}

// open shows the terminal for the first time, once its element is in a pane
// body, and replays the session's recent output, followed by any output
// that arrived meanwhile and isn't in the replay. Output and the snapshot
// can arrive in either order, so offsets tell which output the snapshot
// already holds.
function open(s) {
  s.term.open(s.host);
  s.opened = true;
  let snap = new Uint8Array(0);
  let end = 0;
  call('TermSnapshot', s.id)
    .then((r) => {
      snap = decode(r && r.data);
      end = Number(r && r.offset) || 0;
    })
    .catch(() => {})
    .finally(() => {
      if (sessions.get(s.id) !== s) return;
      s.seen = end;
      s.replaying = true;
      s.term.write(snap, () => {
        s.replaying = false;
        drawn(s, end, true);
      });
      for (const c of s.early) show(s, c.at, c.bytes);
      s.early = [];
      s.ready = true;
      noteEnded(s);
    });
  scheduleFit(s);
}

// show writes output that starts at offset at, leaving out what the
// terminal has already written.
function show(s, at, bytes) {
  const end = at + bytes.length;
  if (end <= s.seen) return;
  s.term.write(at < s.seen ? bytes.subarray(s.seen - at) : bytes, () => drawn(s, end));
  s.seen = end;
}

// drawn records that xterm.js has drawn the output up to offset, and tells
// Go every ACK_EVERY bytes, or now if asked.
function drawn(s, offset, now = false) {
  if (sessions.get(s.id) !== s) return;
  s.drawn = Math.max(s.drawn, offset);
  if (s.drawn - s.acked >= ACK_EVERY || (now && s.drawn > s.acked)) {
    s.acked = s.drawn;
    call('TermAck', s.id, s.drawn).catch(() => {});
  }
}

// noteEnded writes ENDED once a drawn terminal's command has finished.
function noteEnded(s) {
  if (!s.ended) {
    s.noted = false;
    return;
  }
  if (!s.ready || s.noted) return;
  s.noted = true;
  s.term.write(ENDED);
}

function scheduleFit(s) {
  if (s.frame) return;
  s.frame = requestAnimationFrame(() => {
    s.frame = 0;
    fitNow(s);
  });
}

// fitNow fits the terminal to its pane body and tells Go its new size.
function fitNow(s) {
  if (!s.opened || !s.host.isConnected || s.host.clientWidth === 0 || s.host.clientHeight === 0) return;
  const dims = s.fit.proposeDimensions();
  if (!dims || !Number.isFinite(dims.cols) || !Number.isFinite(dims.rows) || dims.cols < 1 || dims.rows < 1) return;
  try {
    s.fit.fit();
  } catch {
    return;
  }
  if (s.term.cols === s.cols && s.term.rows === s.rows) return;
  s.cols = s.term.cols;
  s.rows = s.term.rows;
  actLatest('resize:' + s.id, 'TermResize', s.id, s.cols, s.rows);
}

// park moves every terminal out of the pane bodies, ahead of a render that
// replaces them. It returns the ID of the terminal that had focus, if any.
// The terminal loses the focus as it moves, so its focus reports are held
// back until unpark.
export function park() {
  const lot = document.getElementById('term-park');
  let focused = '';
  moving = true;
  for (const s of sessions.values()) {
    s.hadFocus = s.host.contains(document.activeElement);
    s.focusHeld = false;
    if (s.hadFocus) focused = s.id;
    if (s.host.parentElement !== lot) lot.appendChild(s.host);
  }
  return focused;
}

// unpark ends a render's move, after attach: it gives the terminal id the
// focus again, if it's given, and of the focus reports held back since
// park, sends only what has changed: a terminal that had the focus and
// doesn't now reports that it lost it. A program such as vim would
// otherwise hear that it lost the focus and got it back at every redraw.
// WebKit sends no blur when a focused element moves, so a change is sent to
// a program that asked for focus reports even if none was held back.
export function unpark(id) {
  if (id) focus(id);
  moving = false;
  for (const s of sessions.values()) {
    const has = s.host.contains(document.activeElement);
    const reports = s.focusHeld || Boolean(s.term.modes && s.term.modes.sendFocusMode);
    if (reports && has !== s.hadFocus) send(s, has ? FOCUS_IN : FOCUS_OUT);
    s.focusHeld = false;
    s.hadFocus = false;
  }
}

// attach puts each terminal into its pane body (an element with data-term
// set to its item ID), making the terminal the first time. A terminal left
// out forgets the size it last sent: while it's hidden, its session can be
// resized elsewhere, such as by the terminal's own window when it's popped
// out, so it sends its size again when it's next shown.
export function attach(root) {
  const shown = new Set();
  for (const body of root.querySelectorAll('[data-term]')) {
    const id = body.dataset.term;
    const s = sessions.get(id) || create(id);
    shown.add(s);
    if (s.host.parentElement !== body) body.appendChild(s.host);
    if (!s.opened) open(s);
    scheduleFit(s);
  }
  for (const s of sessions.values()) {
    if (shown.has(s)) continue;
    s.cols = 0;
    s.rows = 0;
  }
}

// focus gives a terminal that's in a pane body the keyboard focus, and
// reports whether it has it.
export function focus(id) {
  const s = sessions.get(id);
  if (!s || !s.opened || !s.host.closest('[data-term]')) return false;
  s.term.focus();
  return s.host.contains(document.activeElement);
}

// hasFocus reports whether a terminal has keyboard focus.
export function hasFocus() {
  const el = document.activeElement;
  return Boolean(el && el.closest && el.closest('.term-host'));
}

// inTab reports whether an item is in one of view's tabs, as the main
// window's terminals must be: one that's popped out, or not in a tab, is
// drawn by its own window or not at all.
function inTab(view, id) {
  const it = view && view.items && view.items[id];
  return Boolean(it) && it.tab >= 0;
}

// prune ends the terminals that this window no longer draws: by default,
// those of items that aren't in a tab. keep(id) says which to keep instead.
// It also notes which terminals' commands have finished.
export function prune(view, keep = (id) => inTab(view, id)) {
  for (const [id, s] of sessions) {
    if (keep(id)) {
      const it = itemOf(view, id);
      const ended = Boolean(it && it.ended);
      if (s.ended && !ended) {
        // The command runs again in a new session, which starts at the
        // default size, so the terminal sends its own again.
        s.cols = 0;
        s.rows = 0;
        scheduleFit(s);
      }
      s.ended = ended;
      noteEnded(s);
      continue;
    }
    if (s.frame) cancelAnimationFrame(s.frame);
    s.ro.disconnect();
    s.term.dispose();
    s.host.remove();
    sessions.delete(id);
  }
}

// write shows a term:data event's output. A terminal that isn't drawn yet
// gets it from its snapshot when it is.
export function write(data) {
  const s = data && sessions.get(data.id);
  if (!s) return;
  const bytes = decode(data.data);
  const at = Number(data.offset) || 0;
  if (s.ready) show(s, at, bytes);
  else s.early.push({ at, bytes });
}

// count returns how many terminals exist, for tests.
export function count() {
  return sessions.size;
}
