// The UI's state. Go owns the model and sends it as a View; the UI keeps only
// what's local to it: open menus and dialogs, the palette's query, a drag in
// progress.

import { call, errorText } from './bridge.js';
import { activeKey, contentOrigin, esc, focusKey, itemOf, shownInFocus, termOfKey } from './util.js';

// store holds the latest View from Go and the UI's own state.
export const store = {
  // view is the latest workspace.View from Go, or null before the first.
  view: null,
  ui: {
    // menu is an open menu: { kind: 'window', itemId, paneId, shownIn, tab,
    // x, y, top, opener, webApp }, { kind: 'tab', index, tabName, x, y, top,
    // opener }, or { kind: 'notices', x, bottom, opener }, where opener is
    // the data-key of the control that opened it, tab is the tab it was
    // opened in, index and tabName are the tab a tab's menu is for, top is
    // where fitMenu placed it, which it keeps, and webApp is what Go offers
    // to put a web app in a web page's place (WebAppChoice), once it has
    // answered. It's a menu drawn in HTML, which needs raised mode
    // (overlayOpen).
    menu: null,
    // menuEntries are the open menu's entries as last drawn.
    menuEntries: [],
    // nativeMenu is the menu that shows as a native macOS menu, in the
    // app, while it's open, as ui.menu describes one (showNative in
    // menu.js). It isn't drawn, and doesn't need raised mode, so that the
    // web pages and docked windows stay in view (decision 71).
    nativeMenu: null,
    // overlay is the open dialog: null, 'palette', 'add', 'editor' (Tab
    // properties), 'settings', 'guide' (the welcome guide), 'shortcuts'
    // (the Keyboard Shortcuts sheet), 'quit' (the question before
    // quitting), 'close' (the question before closing a tab or a
    // terminal), 'overview' (the overview of every tab), or 'discard' (the
    // question before discarding a dialog's changes, over that dialog).
    overlay: null,
    palette: { query: '', sel: 0 },
    // overview is the overview of every tab (overview.js): { query, sel,
    // hold }, where sel is the index of the tab chosen, and hold the ID of
    // Go's hold that opened it (View.hold), or 0. closedHold is the ID of
    // the last hold whose overview closed, which a View sent before Go
    // heard doesn't open again.
    overview: null,
    closedHold: 0,
    // add is the add dialog: { paneId, tab, options, step: 'list' | 'url' |
    // 'make' | 'terminal', option, url, make, terminal, replace, busy },
    // where tab is the tab it was opened in, option the row that asked for
    // an address, terminal the New terminal… form's draft, { command, name,
    // folder, startAgain, save } (launchers.js), make { url,
    // browser, steps, poll } while the dialog waits for a browser to make a
    // web app, where poll is the call to AddWebAppWhenReady (or
    // UseWebAppWhenReady) under way, and busy is set while Go adds what the
    // address step asked for, or while the dialog stops waiting. With
    // replace { itemId, page }, the dialog opened at its make step, from a
    // web page's menu, to put the web app in the page's place (MakeWebApp):
    // page is its name, and paneId its pane.
    add: null,
    // editor is Tab properties' draft.
    editor: null,
    // settings is the Settings dialog's draft.
    settings: null,
    // quit is the question before quitting: { terminals, dontAsk, busy,
    // back }, where terminals are those running something, as Go named
    // them, and back is the dialog it covers, which Cancel goes back to
    // (quit.js).
    quit: null,
    // discard is the question before discarding a dialog's changes:
    // { back, what, next, stays, focus }, where back is the dialog it
    // covers, which Keep editing goes back to, what is what it would
    // discard, next is what runs once the changes are saved or discarded,
    // stays says that Discard leaves the dialog open, as for the add
    // dialog's Escape and Back, and focus is the data-key of the control
    // that had the focus (discard.js).
    discard: null,
    // close is the question before closing: { kind: 'tab', tabName, q,
    // dontAsk, busy } or { kind: 'item', itemId, q, dontAsk, busy }, where
    // q is what Go would close (CloseQuestion, close.js).
    close: null,
    // rename is the field that renames a tab or sub-tab in place:
    // { kind: 'tab', index, tabName, value, busy, back } or { kind: 'item',
    // itemId, paneId, value, busy, back }, where value is what's typed, busy
    // is set while its save is on its way, and back is the data-key of the
    // control to focus once it closes (rename.js).
    rename: null,
    // guideClosed is set when the welcome guide is closed for now.
    guideClosed: false,
    // guide is the welcome guide's state: { page, setup, setupDone,
    // oneTab, starters, removed, made, busy, login }, where page is
    // 'permissions', 'setup', or 'tips', setup is set while it offers the
    // starting set-up, setupDone once the user has added its tabs or none,
    // oneTab while there was one tab as the set-up showed, starters
    // are the tabs Go suggests (StarterTabs), once it has said, removed the
    // names of those the user removed, made the tabs it made of open
    // windows, busy is set while UseStarterTabs is on its way, and login
    // is the Open at login tick box while SetOpenAtLogin is (guide.js).
    guide: null,
    // dialogPositions are where the user has moved or resized dialogs since
    // Bench started, by kind (overlay): their top-left, { x, y }, in window
    // content coordinates (dialogs.js).
    dialogPositions: {},
    // dialogSizes are the sizes that the user has given dialogs since Bench
    // started, by kind: { w, h }, as sent to Go (SetDialogSize). They win
    // over the View's, which may not have the latest yet.
    dialogSizes: {},
    // drag is a sub-tab being dragged, { itemId, paneId }, or a tab,
    // { tab } with its index (drag.js).
    drag: null,
    // divider is a divider being dragged.
    divider: null,
    // opener is the data-key of the control to focus when a menu or dialog
    // closes, or 'term:<item ID>' for a terminal.
    opener: null,
    // paneLater is what a click in a pane does once the menu or dialog it
    // opened closes, unless one of its entries runs: { paneId } focuses
    // the pane, and { paneId, itemId } shows the item there. Focusing an
    // App item's pane activates its app, which would take the keys meant
    // for Bench's menu or dialog, so it waits.
    paneLater: null,
    // pendingFocus is the data-key of the control to focus after the next
    // render.
    pendingFocus: null,
    // termFocus is a terminal that Go asked to give the keyboard focus, to
    // focus once it's the focused pane's shown item and no menu or dialog
    // is open: { id, until }, where until is 0 while one is open
    // (render.js).
    termFocus: null,
    // listItems are the palette or add dialog rows as last drawn.
    listItems: [],
    // choosing is set while macOS's folder picker, a sheet on Bench's
    // window, is open for a form's Choose… (launchers.js). Bench stays
    // raised meanwhile, even if the form closes.
    choosing: false,
    // pageFind is what each web page's find bar has, by item ID: { text,
    // none, busy }, where text is what's typed, none is set when the page
    // had no match for it, and busy while Go looks. findFocus is the item
    // whose find bar gets the keyboard once it's drawn (pages.js).
    pageFind: {},
    findFocus: null,
    // switching is the tab that a switch is on its way to, { index }, which
    // the title bar shows as current until the View has it current, or the
    // switch is refused (showSwitch).
    switching: null,
  },
};

let renderer = () => {};

// setRenderer sets the function that update calls.
export function setRenderer(fn) {
  renderer = fn;
}

// update draws the UI from the store.
export function update() {
  renderer();
}

let commander = () => {};

// setCommander sets the function that does what a bench:command asks
// (app.js), which a notice's button can ask for too (runCommand).
export function setCommander(fn) {
  commander = fn;
}

// runCommand does what a bench:command asks, such as "keyboard-settings",
// which opens Settings at its Keyboard section.
export function runCommand(cmd) {
  commander(cmd);
}

// isDemo reports whether this is the browser preview.
export function isDemo() {
  return Boolean(store.view && store.view.demo);
}

// act calls a bound method and shows Go's message as a toast if it's
// refused. It resolves with { ok, value }.
export async function act(method, ...args) {
  try {
    return { ok: true, value: await call(method, ...args) };
  } catch (err) {
    toast(errorText(err));
    return { ok: false, error: err };
  }
}

// SWITCH_WAIT_MS is the longest, in milliseconds, that the title bar shows
// a tab as current before the View has it current (showSwitch): a switch
// that waits for apps to open can take that long, but a View that never
// comes mustn't leave the wrong tab shown. SWITCH_SETTLE_MS is how long it
// waits for that View once Go has answered the switch, which sends it a
// frame later.
const SWITCH_WAIT_MS = 5000;
const SWITCH_SETTLE_MS = 1000;

let switchTimer = 0;

// showSwitch shows tab index as current in the title bar at once, for a
// switch to it on its way to Go, as Go arranges the tab before its View
// has it current, which can take a while (decision 67). The View that has
// it current settles it (settleSwitch), and a refusal puts the title bar
// back as the View has it (dropSwitch); so does the end of ms. It returns
// what it shows, or null for the current tab.
export function showSwitch(index, ms = SWITCH_WAIT_MS) {
  const { ui, view } = store;
  if (!view || !view.tabs[index] || (index === view.current && !ui.switching)) return null;
  const s = { index };
  ui.switching = s;
  waitSwitch(s, ms);
  update();
  return s;
}

// waitSwitch drops what showSwitch shows, s, if it still shows after ms.
function waitSwitch(s, ms) {
  clearTimeout(switchTimer);
  switchTimer = setTimeout(() => {
    if (store.ui.switching === s) dropSwitch();
  }, ms);
}

// dropSwitch puts the title bar back as the View has it.
export function dropSwitch() {
  if (!store.ui.switching) return;
  clearTimeout(switchTimer);
  store.ui.switching = null;
  update();
}

// settleSwitch ends what showSwitch shows once a View has the tab current,
// or no longer has the tab, before the View is drawn.
export function settleSwitch(view) {
  const s = store.ui.switching;
  if (s && (view.current === s.index || !view.tabs[s.index])) {
    clearTimeout(switchTimer);
    store.ui.switching = null;
  }
}

// shownCurrent returns the tab that the title bar shows as current: the
// one a switch is on its way to, or the View's current tab.
export function shownCurrent(view) {
  const s = store.ui.switching;
  return s && view.tabs[s.index] ? s.index : view.current;
}

// switchTab switches to tab index, as SwitchTab does, or with StepTab and
// step, for the next or previous tab, which Go counts from its own current
// tab, and shows the tab as current at once (showSwitch). It resolves as
// act does.
export async function switchTab(index, step = 0) {
  const s = showSwitch(index);
  const r = step ? await act('StepTab', step) : await act('SwitchTab', index);
  if (s && store.ui.switching === s) {
    if (r.ok) waitSwitch(s, SWITCH_SETTLE_MS);
    else dropSwitch();
  }
  return r;
}

// stepTarget returns the tab that a step to the next (1) or previous (-1)
// tab goes to, from the one shown as current, wrapping, or -1 without a
// View.
export function stepTarget(view, step) {
  const n = view && view.tabs ? view.tabs.length : 0;
  return n ? ((shownCurrent(view) + step) % n + n) % n : -1;
}

// lanes hold calls that must reach Go in order, by key. Go can handle calls
// out of order (each is its own request), so a lane sends one call at a
// time, and while one is on its way it keeps only the latest call to send
// next: for state that the last call sets, such as raised mode or a split's
// sizes.
const lanes = new Map();

// actLatest calls a bound method in the lane for key, after the call on its
// way in that lane, if any, replacing any call still waiting there. Go's
// refusals show as toasts, as with act.
export function actLatest(key, method, ...args) {
  let lane = lanes.get(key);
  if (!lane) {
    lane = { busy: false, next: null };
    lanes.set(key, lane);
  }
  lane.next = { method, args };
  if (!lane.busy) runLane(key, lane);
}

async function runLane(key, lane) {
  lane.busy = true;
  while (lane.next) {
    const { method, args } = lane.next;
    lane.next = null;
    await act(method, ...args);
  }
  lane.busy = false;
  lanes.delete(key);
}

let toastTimer = null;

// toastActions are the buttons of the toast that's showing, if it has
// any: each { label, method, arg, item }.
let toastActions = [];

// toastRead is when, as Date.now() counts, the toast that's showing will
// have shown long enough to read (readTime), and toastEnds when it goes.
let toastRead = 0;
let toastEnds = 0;

// waiting is what announce has to say once the toast that's showing has
// been read, each text once, and readTimer waits for then. It keeps the
// newest WAITING_MOST texts, so that many terminals that report over and
// over can't make one toast that takes minutes to read.
let waiting = [];
let readTimer = null;
const WAITING_MOST = 5;

// TITLE_TOAST_MS is the longest, in milliseconds, that a toast shows in the
// title bar, where it covers the space that moves the window, and maybe
// tabs.
const TITLE_TOAST_MS = 4000;

// ACTION_TOAST_MS is how long, in milliseconds, a toast with a button, such
// as Undo, shows: long enough to read it and change one's mind. One that
// covers tabs in the title bar shows for TITLE_TOAST_MS, as others there
// do. Its time stops while the pointer is over it, or the keyboard is in
// it (holdToast), and it shows for at least HELD_TOAST_MS after.
const ACTION_TOAST_MS = 10000;
const HELD_TOAST_MS = 3000;

// held says what keeps the toast with buttons that's showing from going:
// the pointer over it, the keyboard in it, or both. toastLeft is how long
// it had left when its time stopped.
const held = { pointer: false, focus: false };
let toastLeft = 0;

// TITLE_TOAST_ROOM is the narrowest, in points, that a space in the title
// bar can be for a toast to go there (placeToast).
const TITLE_TOAST_ROOM = 240;

// MESSAGES is how many recent messages the UI keeps (recentMessages).
const MESSAGES = 20;

// messages are the recent toasts and notices, the latest first, for the
// list of notices to show again, so that a toast that went too soon can be
// read: { time, text, kind, count, undo, keep, key }, where time is when
// it last came, kind is 'toast' or 'notice', count is how many times it
// came in a row, undo is the change that its Undo took back, if it had one,
// by Go's step number: the list offers Undo again while that change is the
// latest (View.undo); keep is the app, by bundle ID, whose windows its
// Keep over Bench kept over Bench, if it had one: the list offers it again
// while Go can still do it (View.keepOver); and key is a notice's key, if
// Go gave it one (noticeKey).
const messages = [];

// noticesSeen are the notices of the last View: their text, by noticeKey.
let noticesSeen = new Map();

// noticeKey names a notice: by the key Go gives a notice whose text can
// change while it shows, such as a window tool's, which follows Bench's
// shortcuts; or else by its text.
function noticeKey(n) {
  return n.key ? 'key:' + n.key : 'text:' + n.text;
}

// noteMessage keeps a toast or a notice in the recent messages, with the
// change that its Undo takes back, undo, and the app that its Keep over
// Bench keeps over Bench, keep, if it has them. The same message again,
// straight after, counts again rather than taking another place.
export function noteMessage(text, kind, undo = 0, keep = '', key = '') {
  if (!text) return;
  const last = messages[0];
  if (last && last.text === text && last.kind === kind) {
    last.time = Date.now();
    last.count++;
    last.undo = undo;
    last.keep = keep;
    last.key = key;
    return;
  }
  messages.unshift({ time: Date.now(), text: String(text), kind, count: 1, undo, keep, key });
  messages.length = Math.min(messages.length, MESSAGES);
}

// noteNotices keeps the notices of a View that the last didn't have in the
// recent messages, as they come. A notice with a key whose text changed,
// as a window tool's does as the user changes Bench's shortcuts, keeps its
// one place, with its new text, so that no copy of its old text, which
// may name clashes that have gone, stays among them.
export function noteNotices(view) {
  const now = new Map(((view && view.notices) || []).map((n) => [noticeKey(n), n]));
  now.forEach((n, id) => {
    const was = noticesSeen.get(id);
    if (was === undefined) {
      noteMessage(n.text, 'notice', 0, '', n.key || '');
      return;
    }
    if (was === n.text || !n.key) return;
    const kept = messages.find((m) => m.kind === 'notice' && m.key === n.key);
    if (kept) kept.text = String(n.text);
    else noteMessage(n.text, 'notice', 0, '', n.key);
  });
  noticesSeen = new Map([...now].map(([id, n]) => [id, n.text]));
}

// recentMessages returns the recent toasts and notices, the latest first,
// leaving out the notices that the View still has, which the list of
// notices shows above them.
export function recentMessages(view) {
  const current = new Set(((view && view.notices) || []).map(noticeKey));
  return messages.filter((m) => m.kind !== 'notice' || !current.has(m.key ? 'key:' + m.key : 'text:' + m.text));
}

// toast shows a short message at the bottom of the window, or, while the
// status bar is hidden, in the title bar, for a shorter time (placeToast).
// The recent messages keep it (noteMessage).
// It can have a button, action: { label, method, arg, item }, such as
// Undo, which calls the bound method method with arg, or with item, an
// item's ID, if it has one, as Pop out does (runToastAction), and a second
// button after it, second, of the same form, such as Go there; the toast
// then shows for ACTION_TOAST_MS, unless it covers tabs in the title bar,
// and longer while the pointer is over it or the keyboard is in it
// (holdToast).
export function toast(text, action = null, second = null) {
  if (!text) return;
  const undo = action && action.method === 'Undo' && Number(action.arg) > 0 ? Number(action.arg) : 0;
  const keep = action && action.method === 'KeepOverBench' && action.item ? String(action.item) : '';
  remember(text, undo, keep);
  const el = document.getElementById('toast');
  if (!el) return;
  toastActions = [action, second].filter((a) => a && a.label && /^[A-Z][A-Za-z]*$/.test(a.method || ''));
  if (toastActions.length) {
    el.innerHTML = '<span class="toast-text">' + esc(text) + '</span>'
      + toastActions.map((a, i) => '<button type="button" class="toast-action" data-action="toast-run" data-index="' + i + '">'
        + esc(a.label) + '</button>').join('');
  } else {
    el.textContent = text;
  }
  el.classList.toggle('with-action', toastActions.length > 0);
  const where = placeToast();
  el.classList.add('show');
  clearTimeout(toastTimer);
  held.pointer = false;
  held.focus = false;
  const read = readTime(text, where);
  const ms = toastActions.length ? (where === 'over' ? TITLE_TOAST_MS : ACTION_TOAST_MS) : read;
  toastTimer = setTimeout(hideToast, ms);
  const now = Date.now();
  toastRead = now + read;
  toastEnds = now + ms;
  // What announce is waiting to say waits for this one to be read too.
  if (waiting.length) waitToRead();
}

// readTime returns how long, in milliseconds, a toast's text takes to
// read, which is how long a toast without buttons shows: at most
// TITLE_TOAST_MS in the title bar (where), and 6 seconds elsewhere.
function readTime(text, where) {
  return Math.min(where ? TITLE_TOAST_MS : 6000, Math.max(2200, String(text).length * 60));
}

// remember keeps a toast's text in the recent messages, with the change
// its Undo takes back, and the app its Keep over Bench keeps over Bench,
// if it has them. The first message brings the status bar its button for
// them, and an open list shows the new one.
function remember(text, undo = 0, keep = '') {
  const redraw = !messages.length || noticesOpen();
  noteMessage(text, 'toast', undo, keep);
  if (redraw && store.view) queueMicrotask(update);
}

// noticesOpen reports whether the list of notices and recent messages is
// open.
function noticesOpen() {
  return Boolean(store.ui.menu && store.ui.menu.kind === 'notices');
}

// hideToast hides the toast. Its buttons, if it has any, no longer run.
// What announce waited to say shows then.
function hideToast() {
  const el = document.getElementById('toast');
  if (el) {
    el.classList.remove('show');
    if (el.contains(document.activeElement)) document.activeElement.blur();
  }
  toastActions = [];
  held.pointer = false;
  held.focus = false;
  if (waiting.length) sayWaiting();
}

// holdToast stops the time of a toast with buttons, such as Undo, while
// the pointer is over it (why 'pointer') or the keyboard is in it
// ('focus'), so that it doesn't go while the user reads it or reaches for
// its button, and starts it again once neither is, with what it had left,
// or HELD_TOAST_MS if that's longer. A toast without buttons goes in its
// own time, as there's nothing in it to reach.
function holdToast(why, on) {
  const el = document.getElementById('toast');
  const live = Boolean(el && toastActions.length && el.classList.contains('show'));
  const was = held.pointer || held.focus;
  held[why] = live && on;
  const now = held.pointer || held.focus;
  if (!live || now === was) return;
  if (now) {
    clearTimeout(toastTimer);
    toastTimer = null;
    toastLeft = Math.max(0, toastEnds - Date.now());
    return;
  }
  const ms = Math.max(toastLeft, HELD_TOAST_MS);
  toastEnds = Date.now() + ms;
  toastTimer = setTimeout(hideToast, ms);
}

// toastHeld reports whether the toast's time has stopped (holdToast).
function toastHeld() {
  return held.pointer || held.focus;
}

// watchToast follows the pointer and the keyboard for holdToast. The toast
// lets clicks through to what's below it, so the pointer is found over it
// by where it is, as it moves over Bench's window, rather than by the
// toast's own events; it leaves when it leaves the window. A toast under
// the open list of notices can't be reached (keepToastOffList).
export function watchToast() {
  const el = document.getElementById('toast');
  if (!el) return;
  document.addEventListener('pointermove', (e) => {
    if (!toastActions.length || !el.classList.contains('show')) return;
    const r = el.getBoundingClientRect();
    const over = !el.classList.contains('under-list')
      && e.clientX >= r.left && e.clientX <= r.right && e.clientY >= r.top && e.clientY <= r.bottom;
    holdToast('pointer', over);
  }, { passive: true });
  document.addEventListener('mouseout', (e) => { if (!e.relatedTarget) holdToast('pointer', false); });
  el.addEventListener('focusin', () => holdToast('focus', true));
  el.addEventListener('focusout', (e) => { if (!el.contains(e.relatedTarget)) holdToast('focus', false); });
}

// announce says text, such as that a terminal rang the bell, with a
// screen reader, as a toast: the toast is a live region, which VoiceOver
// reads out (decision 64). While another toast shows, the text waits until
// that one has shown long enough to read (readTime), so that neither is
// lost; the same text waits once (sayWaiting), and only the newest
// WAITING_MOST texts wait.
export function announce(text) {
  if (!text) return;
  if (!waiting.includes(text)) waiting.push(text);
  if (waiting.length > WAITING_MOST) waiting = waiting.slice(-WAITING_MOST);
  const el = document.getElementById('toast');
  if (el && el.classList.contains('show') && Date.now() < toastRead) waitToRead();
  else sayWaiting();
}

// waitToRead says what announce is waiting to say once the toast that's
// showing has been read.
function waitToRead() {
  clearTimeout(readTimer);
  readTimer = setTimeout(sayWaiting, Math.max(0, toastRead - Date.now()));
}

// sayWaiting says what announce has waited to say. A toast without buttons
// has been read by then, and the text replaces it. A toast with buttons,
// such as Undo, keeps them, and its text, until its time is up: the text
// is added after its own, as a part of the live region that's read out
// alone, and the toast stays until that has been read too.
function sayWaiting() {
  clearTimeout(readTimer);
  readTimer = null;
  const texts = waiting;
  waiting = [];
  const el = document.getElementById('toast');
  const own = el && toastActions.length && el.classList.contains('show') ? el.querySelector('.toast-text') : null;
  if (!own) {
    if (texts.length) toast(texts.join(' '));
    return;
  }
  const have = Array.from(own.querySelectorAll('.toast-said'), (n) => n.textContent.trim());
  const added = texts.filter((t) => !have.includes(t));
  for (const text of added) {
    const part = document.createElement('span');
    part.className = 'toast-said';
    part.textContent = ' ' + text;
    own.append(part);
    remember(text);
  }
  if (!added.length) return;
  const ends = Date.now() + readTime(added.join(' '), placeToast());
  if (toastHeld()) {
    toastLeft = Math.max(toastLeft, ends - Date.now());
    return;
  }
  if (ends > toastEnds) {
    clearTimeout(toastTimer);
    toastTimer = setTimeout(hideToast, ends - Date.now());
    toastEnds = ends;
  }
}

// runToastAction runs a button of the toast that's showing, the first
// unless index says which, once: the toast goes, with its other button,
// and Go's answer, such as what Undo put back, comes as a toast and a new
// View.
export function runToastAction(index = 0) {
  const action = toastActions[index] || null;
  clearTimeout(toastTimer);
  hideToast();
  if (!action) return;
  if (action.item) act(action.method, action.item);
  else act(action.method, ...(action.arg === undefined ? [] : [action.arg]));
}

// placeToast puts the toast where it can be seen, and reports where: ''
// at the bottom, 'free' in the title bar's empty space, or 'over' over the
// tabs or across the title bar. In the app, docked windows and web views
// cover the pane bodies, so a toast sits over the status bar (see
// styles.css); while the status bar is hidden, it sits in the title bar,
// in the middle of its empty space between the + after the tabs, or the
// "tabs" view's sub-tabs and their controls, and the controls at its
// right. If that's too narrow, it goes over the tabs, and failing that,
// across the title bar, but never over the traffic lights, whose clicks
// macOS takes. The browser preview does the same.
export function placeToast() {
  const el = document.getElementById('toast');
  const bar = document.getElementById('statusbar');
  const title = document.getElementById('titlebar');
  if (!el) return '';
  keepToastOffList();
  const titled = Boolean(bar && bar.hidden && title);
  el.classList.toggle('in-titlebar', titled);
  if (!titled) {
    el.style.left = '';
    el.style.maxWidth = '';
    return '';
  }
  const o = contentOrigin();
  const r = title.getBoundingClientRect();
  const edge = (sel, side, fallback) => {
    const n = title.querySelector(sel);
    return n ? n.getBoundingClientRect()[side] : fallback;
  };
  // The traffic lights are in the title bar's left padding in the app, and
  // drawn at its start in the browser preview.
  const start = Math.max(r.left + parseFloat(getComputedStyle(title).paddingLeft), edge('.traffic', 'right', r.left) + 8);
  const end = edge('.titlebar-end', 'left', r.right) - 8;
  const after = title.querySelector('.title-pane') ? '.title-pane' : '.new-tab';
  const spaces = [[edge(after, 'right', r.left) + 8, end], [start, end], [start, r.right - 12]];
  const i = spaces.findIndex(([a, b]) => b - a >= TITLE_TOAST_ROOM);
  const [left, right] = spaces[i < 0 ? 2 : i];
  el.style.left = (left + right) / 2 - o.x + 'px';
  el.style.maxWidth = Math.max(0, right - left) + 'px';
  return i === 0 ? 'free' : 'over';
}

// keepToastOffList keeps a toast from covering the list of notices and
// recent messages while it's open: the toast is out of sight, but still
// read out, and the list shows its message at the top of Recent messages,
// with Undo if the toast has it (renderNotices). Its buttons can't be
// reached meanwhile. The toast shows again if the list closes while it has
// time left. Each render calls it, as the list opens and closes.
export function keepToastOffList() {
  const el = document.getElementById('toast');
  if (!el) return;
  const under = noticesOpen();
  const was = el.classList.contains('under-list');
  el.classList.toggle('under-list', under);
  el.querySelectorAll('button').forEach((b) => { b.tabIndex = under ? -1 : 0; });
  if (under && !was) holdToast('pointer', false);
}

// overlayReported is what Go was last told, or null before the first
// render. The first render always tells Go, so that a reloaded page lowers
// a Bench left raised by the page before it.
let overlayReported = null;

// overlayOpen reports whether anything that needs raised mode is open: a
// menu drawn in HTML, a dialog, a drag, or the folder picker. A native
// menu doesn't (ui.nativeMenu).
export function overlayOpen() {
  const { ui } = store;
  return Boolean(ui.menu || ui.overlay || ui.drag || ui.choosing);
}

// syncOverlay tells Go when the first menu, dialog, or drag opens, and when
// the last one closes, so that Bench is raised above docked windows while
// any of them is open.
export function syncOverlay() {
  const open = overlayOpen();
  if (open === overlayReported) return;
  overlayReported = open;
  actLatest('overlay', 'SetOverlay', open);
}

// resendOverlay tells Go again whether anything that needs raised mode is
// open, after Ready has lowered Bench.
export function resendOverlay() {
  overlayReported = null;
  syncOverlay();
}

// overlaySent reports whether Go has been told about raised mode since the
// page loaded.
export function overlaySent() {
  return overlayReported !== null;
}

// canRestore reports whether focus may go back to the control or terminal
// with key when the menu or dialog it opened closes. A terminal that Go
// asked to focus while it was open takes its place if it's the focused
// pane's shown item (ui.termFocus); one that isn't drawn there yet takes
// the focus from the opener when it is. A terminal gets the focus back
// only if it's still the focused pane's shown item, so that typing doesn't
// reach a terminal outside the focused pane.
export function canRestore(key) {
  if (!key) return false;
  const shown = shownInFocus(store.view);
  const want = store.ui.termFocus;
  if (want && want.id === shown) return false;
  const term = termOfKey(key);
  return !term || term === shown;
}

// openerKey returns the data-key that focus goes back to when a dialog
// that opens now closes: that of the control that opened the open menu,
// which the dialog replaces, or else that of the focused control.
export function openerKey() {
  const m = store.ui.menu || store.ui.nativeMenu;
  return m ? m.opener : activeKey();
}

// menuOpen reports whether a menu is open: one drawn in HTML, or a native
// one.
export function menuOpen() {
  return Boolean(store.ui.menu || store.ui.nativeMenu);
}

// closeOverlay closes the open dialog and any menu, and puts focus back
// where it was before the dialog opened (canRestore). Then it does what the
// click that opened the dialog would have done in its pane (ui.paneLater).
// Closing the question before quitting goes back to the dialog it covered
// instead, if that's still there.
export function closeOverlay() {
  const { ui } = store;
  const back = ui.overlay === 'quit' ? coveredDialog(ui) : null;
  ui.quit = null;
  ui.close = null;
  ui.discard = null;
  if (back) {
    ui.overlay = back;
    update();
    return;
  }
  // A New tab dialog that the welcome guide opened goes back to the guide
  // (openEditor's opts.back), with the focus where the guide says.
  const editorBack = ui.overlay === 'editor' && ui.editor && ui.editor.back;
  if (editorBack && editorBack.overlay === 'guide' && ui.guide) {
    ui.editor = null;
    ui.menu = null;
    ui.overlay = 'guide';
    ui.pendingFocus = editorBack.focus || null;
    update();
    return;
  }
  if (ui.overlay === 'guide') ui.guideClosed = true;
  // Closing the overview that Go's hold after last_tab's shortcut opened
  // ends the hold without switching tabs, unless Go has ended it already.
  const o = ui.overlay === 'overview' ? ui.overview : null;
  if (o && o.hold) {
    ui.closedHold = o.hold;
    const hold = store.view && store.view.hold;
    if (hold && hold.id === o.hold) act('CancelHold');
  }
  ui.overview = null;
  // Settings ▸ Keyboard was recording keys: Bench's global shortcuts come
  // back.
  if (ui.settings && ui.settings.keys && ui.settings.keys.recording) actLatest('pause-shortcuts', 'PauseShortcuts', false);
  ui.overlay = null;
  ui.add = null;
  ui.editor = null;
  ui.settings = null;
  ui.menu = null;
  ui.listItems = [];
  const opener = ui.opener;
  ui.opener = null;
  // Asked before the render, which gives a waiting terminal the focus.
  const restore = canRestore(opener);
  update();
  if (restore) focusKey(opener);
  runPaneLater();
}

// coveredDialog returns the dialog that the question before quitting
// covers, if it's still there.
function coveredDialog(ui) {
  const back = ui.quit && ui.quit.back;
  if ((back === 'add' && !ui.add) || (back === 'editor' && !ui.editor) || (back === 'settings' && !ui.settings)) return null;
  return back || null;
}

// closeMenu closes an open menu, putting focus back on the control that
// opened it if restore is set (and canRestore agrees). Then it does what
// the click that opened the menu would have done in its pane
// (ui.paneLater).
export function closeMenu(restore) {
  const { ui } = store;
  if (!ui.menu) return;
  const opener = ui.menu.opener;
  ui.menu = null;
  const back = restore && canRestore(opener);
  update();
  if (back) focusKey(opener);
  runPaneLater();
}

// runPaneLater focuses the pane, or shows the item, that ui.paneLater
// holds, if the pane is still shown and that would change anything. A window
// that the user minimised isn't shown, which would bring it back from the
// Dock: its menu has Show again for that. Its pane is focused instead.
export function runPaneLater() {
  const { ui, view } = store;
  const later = ui.paneLater;
  ui.paneLater = null;
  if (!later || !view || !view.display || ui.menu || ui.overlay) return;
  const pane = (view.display.panes || []).find((p) => p.id === later.paneId);
  if (!pane) return;
  if (later.itemId) {
    const minimised = (itemOf(view, later.itemId) || {}).status === 'minimised';
    if (!minimised && (pane.items || []).includes(later.itemId) && pane.active !== later.itemId) act('ShowItem', later.paneId, later.itemId);
    else if (later.paneId !== 'compact' && view.display.focus !== later.paneId) act('FocusPane', later.paneId);
  } else if (later.paneId !== 'compact' && view.display.focus !== later.paneId) {
    act('FocusPane', later.paneId);
  }
}
