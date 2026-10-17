// Terminal items, drawn with xterm.js. Each item gets one Terminal that lives
// as long as the item does: renders move its element into the new pane body
// rather than making a new one.

import { Terminal } from '../vendor/xterm/xterm.mjs';
import { FitAddon } from '../vendor/xterm-addon-fit/addon-fit.mjs';
import { call } from './bridge.js';
import { act } from './state.js';
import { itemOf } from './util.js';

// sessions holds each terminal by item ID.
const sessions = new Map();

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
  });
  const fit = new FitAddon();
  term.loadAddon(fit);
  // early holds output that arrives while the snapshot is on its way.
  const s = { id, host, term, fit, opened: false, ready: false, early: [], cols: 0, rows: 0, frame: 0, ro: null };
  term.onData((data) => { act('TermInput', id, data); });
  // Mouse reports in the default (X10) encoding come as binary strings.
  // TermInput takes text, so only reports that are plain ASCII (positions
  // up to column and row 95) can be passed on unchanged.
  term.onBinary((data) => {
    if (/^[\x00-\x7f]*$/.test(data)) act('TermInput', id, data);
  });
  s.ro = new ResizeObserver(() => scheduleFit(s));
  s.ro.observe(host);
  sessions.set(id, s);
  return s;
}

// open shows the terminal for the first time, once its element is in a pane
// body, and replays the session's recent output, followed by any output
// that arrived meanwhile and isn't in the replay.
function open(s) {
  s.term.open(s.host);
  s.opened = true;
  let snap = new Uint8Array(0);
  call('TermSnapshot', s.id)
    .then((b64) => { snap = decode(b64); })
    .catch(() => {})
    .finally(() => {
      if (sessions.get(s.id) !== s) return;
      if (snap.length) s.term.write(snap);
      for (const chunk of unreplayed(snap, s.early)) s.term.write(chunk);
      s.early = [];
      s.ready = true;
    });
  scheduleFit(s);
}

// endsWith reports whether bytes ends with tail.
function endsWith(bytes, tail) {
  if (tail.length > bytes.length) return false;
  const start = bytes.length - tail.length;
  for (let i = 0; i < tail.length; i++) {
    if (bytes[start + i] !== tail[i]) return false;
  }
  return true;
}

// concat joins chunks of bytes.
function concat(chunks) {
  const out = new Uint8Array(chunks.reduce((n, c) => n + c.length, 0));
  let at = 0;
  for (const c of chunks) {
    out.set(c, at);
    at += c.length;
  }
  return out;
}

// unreplayed returns the chunks of output, received while the snapshot was
// on its way, that the snapshot doesn't already hold. Events arrive in
// order, but not necessarily in order with the snapshot: chunks sent before
// it was taken end it, so the longest run of leading chunks that ends the
// snapshot is dropped, and the rest are kept.
function unreplayed(snap, chunks) {
  for (let k = chunks.length; k > 0; k--) {
    if (endsWith(snap, concat(chunks.slice(0, k)))) return chunks.slice(k);
  }
  return chunks;
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
  act('TermResize', s.id, s.cols, s.rows);
}

// park moves every terminal out of the pane bodies, ahead of a render that
// replaces them. It returns the ID of the terminal that had focus, if any.
export function park() {
  const lot = document.getElementById('term-park');
  let focused = '';
  for (const s of sessions.values()) {
    if (s.host.contains(document.activeElement)) focused = s.id;
    if (s.host.parentElement !== lot) lot.appendChild(s.host);
  }
  return focused;
}

// attach puts each terminal into its pane body (an element with data-term
// set to its item ID), making the terminal the first time.
export function attach(root) {
  for (const body of root.querySelectorAll('[data-term]')) {
    const id = body.dataset.term;
    const s = sessions.get(id) || create(id);
    body.appendChild(s.host);
    if (!s.opened) open(s);
    scheduleFit(s);
  }
}

// focus gives a terminal keyboard focus.
export function focus(id) {
  const s = sessions.get(id);
  if (s && s.opened && s.host.isConnected) s.term.focus();
}

// hasFocus reports whether a terminal has keyboard focus.
export function hasFocus() {
  const el = document.activeElement;
  return Boolean(el && el.closest && el.closest('.term-host'));
}

// prune ends the terminals of items that no longer exist.
export function prune(view) {
  for (const [id, s] of sessions) {
    if (itemOf(view, id)) continue;
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
  if (s.ready) s.term.write(bytes);
  else s.early.push(bytes);
}

// count returns how many terminals exist, for tests.
export function count() {
  return sessions.size;
}
