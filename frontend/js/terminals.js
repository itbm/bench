// Terminal items, drawn with xterm.js. Each item gets one Terminal that lives
// as long as the item is in a tab (or, in a popped-out terminal's window, as
// long as it exists): renders move its element into the new pane body rather
// than making a new one. Only one window draws each terminal, so that only
// one answers its program's queries. Each has a find bar (⌘F), links that
// open on Command-click, its own text size (⌘+, ⌘−, and ⌘0), and the entries
// of its right-click menu (menuEntries).

import { Terminal } from '../vendor/xterm/xterm.mjs';
import { FitAddon } from '../vendor/xterm-addon-fit/addon-fit.mjs';
import { SearchAddon } from '../vendor/xterm-addon-search/addon-search.mjs';
import { WebLinksAddon } from '../vendor/xterm-addon-web-links/addon-web-links.mjs';
import { call, clipboardText, setClipboardText } from './bridge.js';
import { act, actLatest, toast } from './state.js';
import { ICONS, itemOf } from './util.js';

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
// finished. It isn't part of the output, so each view writes it itself. It's
// grey (bright black), as the output Bench shows again after it reopens is,
// which both sets of colours keep readable.
const ENDED = '\r\n\x1b[90m[The command has finished. Press Return to run it again.]\x1b[0m\r\n';

// moving is set while a render moves the terminals out of the pane bodies
// and back (park, then unpark).
let moving = false;

// The terminals' text size, in points: by default, as in config.toml's
// terminal_font_size, and the smallest and largest, as Settings allows. ⌘+
// and ⌘− change a terminal's size by SIZE_STEP.
export const DEFAULT_SIZE = 12.5;
const MIN_SIZE = 9;
const MAX_SIZE = 24;
const SIZE_STEP = 1;

// settings are the terminals' settings in Go's View (configure): their text
// size, and whether Option sends Meta. browser is set in the browser
// preview, whose clipboard is the browser's.
const settings = { size: DEFAULT_SIZE, optionAsMeta: false, browser: false };

// FIND_LIMIT is the most matches that find marks, and counts.
const FIND_LIMIT = 1000;

// matches returns the colours of find's matches, as the search add-on
// takes them (#RRGGBB): every match, and the one that's selected. The
// add-on needs colours for marks in a scroll bar too, which Bench's
// terminals don't show.
function matches() {
  const match = token('--term-match', '#4A3A1E');
  const border = token('--term-match-border', '#7A5A26');
  const active = token('--term-match-active-border', '#F2A33A');
  return {
    matchBackground: match,
    matchBorder: border,
    matchOverviewRuler: border,
    activeMatchBackground: token('--term-match-active', '#8A5A12'),
    activeMatchBorder: active,
    activeMatchColorOverviewRuler: active,
  };
}

function token(name, fallback) {
  const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  return v || fallback;
}

// theme returns the terminals' colours, from the tokens of the colours in
// use (theme.js).
function theme() {
  const bg = token('--bg-pane', '#0E1014');
  const fg = token('--text', '#E6E8EC');
  return {
    background: bg,
    foreground: fg,
    cursor: fg,
    cursorAccent: bg,
    selectionBackground: token('--term-selection', 'rgba(242, 163, 58, 0.32)'),
    black: token('--term-black', '#20252E'),
    red: token('--danger', '#F29C9C'),
    green: token('--ok', '#8FD694'),
    yellow: token('--syn-type', '#F2C97D'),
    blue: token('--syn-func', '#82C4FF'),
    magenta: token('--syn-keyword', '#D7A6FF'),
    cyan: token('--term-cyan', '#7FD4D0'),
    white: token('--text-muted', '#A6ADB9'),
    brightBlack: token('--text-faint', '#7D8594'),
    brightRed: token('--term-bright-red', '#F7B8B8'),
    brightGreen: token('--term-bright-green', '#B2E6B5'),
    brightYellow: token('--accent', '#F2A33A'),
    brightBlue: token('--term-bright-blue', '#A8D6FF'),
    brightMagenta: token('--term-bright-magenta', '#E4C2FF'),
    brightCyan: token('--term-bright-cyan', '#A6E4E1'),
    brightWhite: token('--text-strong', '#F1F3F6'),
  };
}

// MORE_CONTRAST matches while macOS's Increase contrast is on (System
// Settings ▸ Accessibility ▸ Display), which WebKit tells pages.
const MORE_CONTRAST = '(prefers-contrast: more)';

// contrast returns the contrast that terminals keep text at, at least,
// against its background: 4.5:1, as WCAG asks of text, or 7:1 with Increase
// contrast on. xterm.js draws text in a colour that's fainter, such as a
// program's black on a dark background, darker or lighter until it's
// readable. It asks half as much of faint text (SGR 2), so styles.css draws
// that in the terminal's own colour with a token (--term-dim) instead.
function contrast() {
  return window.matchMedia && window.matchMedia(MORE_CONTRAST).matches ? 7 : 4.5;
}

// restyle gives every terminal the colours in use, and the contrast to keep
// text at. It runs when the colours change, and when Increase contrast is
// turned on or off, which changes some of the tokens that theme reads too
// (--text-muted and --text-faint). A new theme also makes xterm.js forget
// the colours it worked out for faint text, which a new contrast alone
// doesn't.
function restyle() {
  const colours = theme();
  const ratio = contrast();
  for (const s of sessions.values()) {
    s.term.options.theme = colours;
    s.term.options.minimumContrastRatio = ratio;
  }
}
document.addEventListener('bench:theme', restyle);
if (window.matchMedia) {
  const query = window.matchMedia(MORE_CONTRAST);
  if (query.addEventListener) query.addEventListener('change', restyle);
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
  const handler = links(host);
  const term = new Terminal({
    fontFamily: "'IBM Plex Mono', ui-monospace, 'SF Mono', Menlo, monospace",
    fontSize: settings.size,
    lineHeight: 1.35,
    cursorBlink: !reducedMotion(),
    scrollback: 5000,
    macOptionIsMeta: settings.optionAsMeta,
    theme: theme(),
    minimumContrastRatio: contrast(),
    linkHandler: handler,
    // The search add-on marks matches with decorations, which xterm.js
    // counts as proposed API.
    allowProposedApi: true,
  });
  const fit = new FitAddon();
  term.loadAddon(fit);
  const search = new SearchAddon({ highlightLimit: FIND_LIMIT });
  term.loadAddon(search);
  // Addresses that a program prints as plain text open as the links it
  // prints with OSC 8 do.
  term.loadAddon(new WebLinksAddon(handler.activate, { hover: handler.hover, leave: handler.leave }));
  // early holds output that arrives while the snapshot is on its way, as
  // { at, bytes }; seen is the offset the output written so far ends at,
  // drawn how far xterm.js has drawn it, and acked how far Go has been told
  // about. replaying is set while the snapshot is drawn. typed holds input
  // waiting to be sent, and sending is set while some is on its way. ended
  // follows the item's ended, and noted is set once ENDED is written.
  // While the terminal moves, hadFocus says whether it had the focus
  // before, findHad whether its find bar's field had it, and focusHeld is
  // set if a focus report was held back. size is the text size that ⌘+ and
  // ⌘− have given it, or 0 for the settings' size. find is its find bar,
  // once it has one (findBar).
  const s = {
    id, host, term, fit, search, opened: false, ready: false, early: [], cols: 0, rows: 0, frame: 0, ro: null,
    seen: 0, drawn: 0, acked: 0, replaying: false, typed: '', sending: false, ended: false, noted: false,
    hadFocus: false, findHad: false, focusHeld: false, size: 0, find: null,
  };
  term.attachCustomKeyEventHandler((e) => ownKeys(s, e));
  search.onDidChangeResults((r) => showCount(s, r));
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

// links returns the handler for the links that programs print in the
// terminal drawn in host: with OSC 8, and as plain addresses, which the
// web links add-on finds. The output chooses both an OSC 8 link's address
// and the text it covers, which can fill the screen, and printed text can
// look like one address and be another, so a link opens in the default
// browser only on Command-click, never on a click meant to focus the
// terminal or select text, and holding the pointer over it shows the
// address it opens. xterm.js and the add-on pass on only http and https
// links, and Go opens only those.
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

// terminalKey names what a key does in a terminal that has the keyboard, of
// the keys that Bench's terminals give their own meaning, or returns '':
// ⌘F opens the find bar, ⌘G and ⇧⌘G find the next and previous match, ⌘+
// (or ⌘=), ⌘−, and ⌘0 make the text bigger, smaller, or the settings' size,
// ⌘K clears the terminal, and ⌘A selects all its text. Letters are matched
// by the character they type, as macOS matches menu keys, or by their place
// on keyboards whose letters aren't Latin.
export function terminalKey(e) {
  if (!e || !e.metaKey || e.ctrlKey || e.altKey) return '';
  const key = e.key || '';
  const letter = /^[a-z]$/i.test(key) ? key.toLowerCase() : ((/^Key([A-Z])$/.exec(e.code || '') || [])[1] || '').toLowerCase();
  if (letter === 'g') return e.shiftKey ? 'previous' : 'next';
  if (key === '+' || key === '=' || e.code === 'NumpadAdd') return 'bigger';
  if (e.shiftKey) return '';
  if (letter === 'f') return 'find';
  if (letter === 'k') return 'clear';
  if (letter === 'a') return 'selectAll';
  if (key === '-' || key === '−' || e.code === 'Minus' || e.code === 'NumpadSubtract') return 'smaller';
  if (key === '0' || e.code === 'Digit0' || e.code === 'Numpad0') return 'reset';
  return '';
}

// ownKeys handles the keys that terminalKey names, for xterm.js, which asks
// about each key event before it handles it. It keeps them from the
// program, and from the menu bar, which would otherwise take ⌘A, and does
// what they do once, as each key goes down.
function ownKeys(s, e) {
  const what = terminalKey(e);
  if (!what) return true;
  e.preventDefault();
  if (e.type === 'keydown') run(s, what);
  return false;
}

// run does what terminalKey named, in terminal s.
function run(s, what) {
  if (what === 'find') openFind(s);
  else if (what === 'next' || what === 'previous') findFromKeys(s, what === 'previous');
  else if (what === 'bigger') resize(s, SIZE_STEP);
  else if (what === 'smaller') resize(s, -SIZE_STEP);
  else if (what === 'reset') resize(s, 0);
  else if (what === 'clear') clear(s);
  else if (what === 'selectAll') s.term.selectAll();
}

// clear clears terminal s's screen and what has scrolled off it, and has Go
// forget that output too (TermClear), so that it doesn't come back when the
// terminal is drawn again, such as in a window of its own, or after a
// restart.
function clear(s) {
  s.term.clear();
  call('TermClear', s.id).catch(() => {});
}

// sizeIn returns a text size within what Settings allows.
function sizeIn(size) {
  return Math.min(MAX_SIZE, Math.max(MIN_SIZE, size));
}

// resize makes terminal s's text step points bigger, or smaller, within
// what Settings allows, or, for a step of 0, the settings' size again.
function resize(s, step) {
  s.size = step ? sizeIn((s.size || settings.size) + step) : 0;
  applySettings(s);
}

// applySettings gives terminal s its text size and Option key. A new size
// changes its rows and columns, so it's fitted again, which tells Go.
function applySettings(s) {
  const o = s.term.options;
  if (o.macOptionIsMeta !== settings.optionAsMeta) o.macOptionIsMeta = settings.optionAsMeta;
  const size = s.size || settings.size;
  if (o.fontSize === size) return;
  o.fontSize = size;
  scheduleFit(s);
}

// configure follows the terminals' settings in Go's View, as Settings
// changes them, for every terminal this window draws. A new text size
// replaces the sizes that ⌘+ and ⌘− gave terminals.
export function configure(view) {
  if (!view) return;
  const v = view.settings || {};
  settings.browser = Boolean(view.demo);
  const size = Number(v.terminalFontSize) > 0 ? sizeIn(Number(v.terminalFontSize)) : DEFAULT_SIZE;
  const meta = Boolean(v.terminalOptionAsMeta);
  if (size === settings.size && meta === settings.optionAsMeta) return;
  const resized = size !== settings.size;
  settings.size = size;
  settings.optionAsMeta = meta;
  for (const s of sessions.values()) {
    if (resized) s.size = 0;
    applySettings(s);
  }
}

// optionsOf returns the text size of item id's terminal, and whether
// Option sends Meta in it, for tests.
export function optionsOf(id) {
  const s = sessions.get(id);
  return s ? { fontSize: s.term.options.fontSize, optionAsMeta: s.term.options.macOptionIsMeta } : null;
}

// coloursOf returns the background and text colours of item id's terminal,
// for tests.
export function coloursOf(id) {
  const s = sessions.get(id);
  return s ? { background: s.term.options.theme.background, foreground: s.term.options.theme.foreground } : null;
}

// contrastOf returns the contrast that item id's terminal keeps text at,
// for tests.
export function contrastOf(id) {
  const s = sessions.get(id);
  return s ? s.term.options.minimumContrastRatio : null;
}

// findBar returns terminal s's find bar, making it the first time: a field,
// which finds as it's typed in, the count of matches, and buttons for the
// previous and next match and to close the bar. It sits at the top right
// of the terminal, in its element, so that it moves with it.
function findBar(s) {
  if (s.find) return s.find;
  const root = document.createElement('div');
  root.className = 'term-find';
  root.setAttribute('role', 'search');
  root.hidden = true;
  const input = document.createElement('input');
  input.type = 'text';
  input.className = 'term-find-input';
  input.placeholder = 'Find';
  input.setAttribute('aria-label', 'Find in terminal');
  input.setAttribute('autocomplete', 'off');
  input.setAttribute('autocapitalize', 'off');
  input.setAttribute('autocorrect', 'off');
  input.spellcheck = false;
  const count = document.createElement('span');
  count.className = 'term-find-count';
  count.setAttribute('aria-live', 'polite');
  const button = (name, label, icon, onClick) => {
    const b = document.createElement('button');
    b.type = 'button';
    b.className = 'icon-btn term-find-btn';
    b.dataset.find = name;
    b.title = label;
    b.setAttribute('aria-label', label);
    b.innerHTML = icon;
    b.addEventListener('click', onClick);
    return b;
  };
  root.append(
    input,
    count,
    button('previous', 'Previous match (⇧↩)', ICONS.chevronUp, () => find(s, true)),
    button('next', 'Next match (↩)', ICONS.chevronDown, () => find(s, false)),
    button('close', 'Close (⎋)', ICONS.close, () => closeFind(s)),
  );
  input.addEventListener('input', () => find(s, false, true));
  input.addEventListener('keydown', (e) => findKeys(s, e));
  s.host.appendChild(root);
  s.find = { root, input, count };
  return s.find;
}

// findKeys handles keys in the find bar's field: Return finds the next
// match, and Shift-Return the previous one, Escape closes the bar, and ⌘F,
// ⌘G, ⇧⌘G, ⌘+, ⌘−, and ⌘0 do what they do in the terminal. Keys that an
// input method uses to compose text are its own.
function findKeys(s, e) {
  if (e.isComposing || e.keyCode === 229) return;
  if (e.key === 'Enter' && !e.metaKey && !e.ctrlKey && !e.altKey) {
    e.preventDefault();
    find(s, e.shiftKey);
    return;
  }
  if (e.key === 'Escape') {
    e.preventDefault();
    e.stopPropagation();
    closeFind(s);
    return;
  }
  const what = terminalKey(e);
  if (!['find', 'next', 'previous', 'bigger', 'smaller', 'reset'].includes(what)) return;
  e.preventDefault();
  if (what === 'find') e.target.select();
  else run(s, what);
}

// openFind shows terminal s's find bar, with what it last found selected,
// so that typing replaces it, and marks its matches again.
function openFind(s) {
  const f = findBar(s);
  const shown = !f.root.hidden;
  f.root.hidden = false;
  f.input.focus();
  f.input.select();
  if (!shown && f.input.value) find(s, false, true);
}

// findFromKeys finds the next or previous match from the terminal, with ⌘G
// or ⇧⌘G: what the find bar last found, or, with nothing to find, it opens
// the bar.
function findFromKeys(s, previous) {
  if (!s.find || !s.find.input.value) {
    openFind(s);
    return;
  }
  s.find.root.hidden = false;
  find(s, previous);
}

// find selects the next or previous match of the find bar's text in
// terminal s, from the match that's selected, and marks every match. As
// the text is typed (incremental), the selected match stays while it
// still matches. The add-on then counts the matches (showCount).
function find(s, previous, incremental = false) {
  const f = s.find;
  if (!f) return;
  const text = f.input.value;
  if (!text) {
    s.search.clearDecorations();
    s.term.clearSelection();
    setCount(f, '', false);
    return;
  }
  const options = { incremental, decorations: matches() };
  if (previous) s.search.findPrevious(text, options);
  else s.search.findNext(text, options);
}

// showCount shows how many matches the find bar has found in terminal s,
// and which is selected: from the add-on, whose index is -1 when the
// selected match is past the most it counts.
function showCount(s, r) {
  const f = s.find;
  if (!f || f.root.hidden || !f.input.value || !r) return;
  const total = r.resultCount >= FIND_LIMIT ? FIND_LIMIT + '+' : String(r.resultCount);
  if (!r.resultCount) setCount(f, 'No matches', true);
  else if (r.resultIndex >= 0) setCount(f, (r.resultIndex + 1) + ' of ' + total, false);
  else setCount(f, total + (r.resultCount === 1 ? ' match' : ' matches'), false);
}

function setCount(f, text, none) {
  f.count.textContent = text;
  f.count.classList.toggle('none', none);
}

// closeFind hides terminal s's find bar and takes away its marks, leaving
// the match it found selected, and gives the terminal the keyboard back.
function closeFind(s) {
  const f = s.find;
  if (!f || f.root.hidden) return;
  f.root.hidden = true;
  s.search.clearDecorations();
  s.term.focus();
}

// menuEntries lists what item id's terminal's right-click menu offers, as
// menu entries (menu.js): Copy, with text selected, and Paste, as ⌘C and ⌘V
// do; Find…, which opens the find bar; Clear, which clears the screen and
// what has scrolled off it; and Select all. A right-click on a word that
// isn't selected selects it first, as in Terminal.
export function menuEntries(id) {
  const s = sessions.get(id);
  if (!s || !s.opened) return [];
  return [
    { label: 'Copy', keys: '⌘C', disabled: !s.term.hasSelection(), run: () => copy(s) },
    { label: 'Paste', keys: '⌘V', run: () => paste(s) },
    { sep: true },
    { label: 'Find…', keys: '⌘F', run: () => openFind(s) },
    { sep: true },
    { label: 'Clear', keys: '⌘K', run: () => clear(s) },
    { label: 'Select all', keys: '⌘A', run: () => s.term.selectAll() },
  ];
}

// copy puts terminal s's selected text on the clipboard.
async function copy(s) {
  const text = s.term.getSelection();
  if (!text) return;
  try {
    await setClipboardText(text, settings.browser);
  } catch {
    toast('Bench couldn\'t copy to the clipboard.');
  }
}

// paste types the clipboard's text into terminal s, as ⌘V does: xterm.js
// marks it as pasted for a program that asks (bracketed paste).
async function paste(s) {
  let text = '';
  try {
    text = await clipboardText(settings.browser);
  } catch {
    toast('Bench couldn\'t read the clipboard.');
    return;
  }
  if (text && sessions.get(s.id) === s) s.term.paste(text);
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
// replaces them. It returns the ID of the terminal that had focus, if any,
// in itself or its find bar. The terminal loses the focus as it moves, so
// its focus reports are held back until unpark.
export function park() {
  const lot = document.getElementById('term-park');
  let focused = '';
  moving = true;
  for (const s of sessions.values()) {
    s.hadFocus = s.host.contains(document.activeElement);
    s.findHad = Boolean(s.find) && s.find.input === document.activeElement;
    s.focusHeld = false;
    if (s.hadFocus) focused = s.id;
    if (s.host.parentElement !== lot) lot.appendChild(s.host);
  }
  return focused;
}

// unpark ends a render's move, after attach: it gives the terminal id the
// focus again, if it's given, or its find bar's field, if that had it, and
// of the focus reports held back since park, sends only what has changed:
// a terminal that had the focus and doesn't now reports that it lost it. A
// program such as vim would otherwise hear that it lost the focus and got
// it back at every redraw. WebKit sends no blur when a focused element
// moves, so a change is sent to a program that asked for focus reports
// even if none was held back.
export function unpark(id) {
  const back = id && sessions.get(id);
  if (back && back.findHad && back.host.closest('[data-term]')) back.find.input.focus();
  else if (id) focus(id);
  moving = false;
  for (const s of sessions.values()) {
    const has = s.host.contains(document.activeElement);
    const reports = s.focusHeld || Boolean(s.term.modes && s.term.modes.sendFocusMode);
    if (reports && has !== s.hadFocus) send(s, has ? FOCUS_IN : FOCUS_OUT);
    s.focusHeld = false;
    s.hadFocus = false;
    s.findHad = false;
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
