// Drops on Bench's window (decision 73).
//
// Files, folders, and apps dragged from the Finder never reach the page as
// HTML drag events: Wails' native view over the web view takes them, and
// calls window._wails.handlePlatformFileDrop with their paths and where
// they were dropped. Wails' runtime finds the nearest element there with a
// data-file-drop-target attribute, and sends its attributes to Go with the
// paths (workspace.DropTargetOf):
//
//   - "terminal", with data-drop-item: a terminal, which types the paths.
//   - "pane", with data-drop-pane: a pane's header, an empty pane, or a
//     pane whose item isn't ready, which adds folders and apps.
//   - "new-tab": the + after the tabs, which makes a new tab of them.
//   - "window": the body, anywhere else, where Go says where to drop.
//
// While a drag is over an element, Wails gives it the class
// file-drop-target-active, which styles.css draws.
//
// Anything else dragged onto the page, such as a link or text from another
// app, comes as HTML drag events, which WebKit would follow by loading the
// link in place of Bench's page. So the page takes every drag that nothing
// in it takes (guard): a field still takes text dropped in it, a drag that
// started in the page lands nowhere, and anything else says what Bench
// takes. Links are for later. In the app, a drag of files that comes as
// HTML drag events isn't from the Finder, whose drags Wails' native view
// takes first, but of files that have no paths yet, such as an image from
// a browser or a photo from Photos: the page takes it before Wails'
// runtime would light up what it's over, and says how to drop it.
//
// The browser preview's browser gives a page no paths, so its tests and
// screenshots call window._wails.handlePlatformFileDrop as Wails' native
// code does, and here it finds the element as Wails' runtime does and
// sends it to Go's DemoDropFiles instead (previewDrop): Wails' runtime
// would send it as a window event, which the preview's server-mode windows
// don't have. Only the app's windows that take drops have Wails' flag
// enableFileDrop set. Tests with a stand-in for Go, which load no Wails
// runtime, get the same, with hover handlers like Wails'.

import { runtimeReady } from './bridge.js';
import { act, toast } from './state.js';
import { esc } from './util.js';

// TARGET is the attribute of the elements that take drops, and ACTIVE the
// class of the one a drag is over: Wails' names.
export const TARGET = 'data-file-drop-target';
const ACTIVE = 'file-drop-target-active';

// NOT_FILES says what Bench takes, after a drop of something else, such as
// a link. NOT_FINDER says how to drop files that have no paths yet, in the
// app. Each fits the app's toast, which is one line, in Bench's smallest
// window, and a terminal's own window.
export const NOT_FILES = 'Bench takes files, folders, and apps from the Finder.';
export const NOT_FINDER = 'Save it as a file first, then drag the file from the Finder.';

// dropAttrs returns the attributes that make an element take drops of
// kind: a terminal's with the item's ID, a pane's with the pane's.
export function dropAttrs(kind, id = '') {
  if (kind === 'terminal') return ' ' + TARGET + '="terminal" data-drop-item="' + esc(id) + '"';
  if (kind === 'pane') return ' ' + TARGET + '="pane" data-drop-pane="' + esc(id) + '"';
  return ' ' + TARGET + '="' + esc(kind) + '"';
}

// setDropTarget makes element el take drops of kind, as dropAttrs says.
export function setDropTarget(el, kind, id = '') {
  el.setAttribute(TARGET, kind);
  if (kind === 'terminal') el.setAttribute('data-drop-item', id);
  if (kind === 'pane') el.setAttribute('data-drop-pane', id);
}

// targetAt returns the element that takes a drop at (x, y), as Wails'
// runtime finds it.
function targetAt(x, y) {
  const el = document.elementFromPoint(x, y);
  return el ? el.closest('[' + TARGET + ']') : null;
}

// clearActive takes the drag's highlight away.
function clearActive() {
  document.querySelectorAll('.' + ACTIVE).forEach((el) => el.classList.remove(ACTIVE));
}

// previewDrop sends a pretend drop of paths at (x, y) to Go, as Wails'
// runtime sends a real one: with the attributes of the element under it
// that takes drops.
function previewDrop(paths, x, y) {
  const target = targetAt(x, y);
  clearActive();
  if (!target) return;
  const attrs = {};
  for (const a of Array.from(target.attributes)) attrs[a.name] = a.value;
  act('DemoDropFiles', Array.from(paths || []), attrs);
}

// hover draws the highlight on the element under (x, y) that takes drops,
// as Wails' runtime does, for pages without it.
function hover(x, y) {
  const target = targetAt(x, y);
  document.querySelectorAll('.' + ACTIVE).forEach((el) => { if (el !== target) el.classList.remove(ACTIVE); });
  if (target) target.classList.add(ACTIVE);
}

// hasFiles reports whether an HTML drag carries files.
function hasFiles(e) {
  const types = e.dataTransfer ? Array.from(e.dataTransfer.types || []) : [];
  return types.includes('Files');
}

// editable reports whether a drag is over a field that takes text dropped
// in it.
function editable(e) {
  const el = e.target && e.target.nodeType === 1 ? e.target : e.target && e.target.parentElement;
  const field = el && el.closest ? el.closest('input, textarea, [contenteditable]:not([contenteditable="false"])') : null;
  return Boolean(field) && !field.disabled && !field.readOnly;
}

// inside is set while an HTML drag that started in the page goes on, such
// as of selected text. A drag that the page stops as it starts, as drag.js
// stops one of a tab's text, never starts, and has no dragend. A drag ends
// with a dragend at its source, which may have left the page by then, as
// when a new view drew it again, so the source hears it too; and a press of
// the pointer, which can't come during a drag, ends one that went unheard.
let inside = false;

// guard takes the drags that nothing in the page takes: they're cancelled,
// so that WebKit doesn't load a link dropped on the page, and a drop of
// something other than files, from another app, says what Bench takes.
// Wails' runtime cancels drags of files first, and its native view takes
// them from the Finder before the page sees them. So in the app, a drag of
// files that the page sees has no paths (notFinder): the page takes it as
// it reaches the window, before Wails' runtime, which would light up what
// it's over, and a drop of it says how to drop such files.
function guard() {
  const end = () => { inside = false; };
  window.addEventListener('pointerdown', end, true);
  window.addEventListener('dragstart', (e) => {
    inside = !e.defaultPrevented;
    if (inside && e.target && e.target.addEventListener) e.target.addEventListener('dragend', end, { once: true });
  });
  document.addEventListener('dragend', end, true);
  const notFinder = (e) => nativeDrops() && hasFiles(e);
  const take = (e) => {
    if (!notFinder(e)) return;
    e.stopPropagation();
    e.preventDefault();
    clearActive();
    if (e.dataTransfer) e.dataTransfer.dropEffect = inside ? 'none' : 'copy';
  };
  window.addEventListener('dragenter', take, true);
  window.addEventListener('dragover', take, true);
  window.addEventListener('drop', (e) => {
    if (!notFinder(e)) return;
    const from = inside;
    inside = false;
    e.stopPropagation();
    e.preventDefault();
    clearActive();
    if (!from) toast(NOT_FINDER);
  }, true);
  const over = (e) => {
    if (e.defaultPrevented || (editable(e) && !hasFiles(e))) return;
    e.preventDefault();
    if (e.dataTransfer) e.dataTransfer.dropEffect = inside || hasFiles(e) ? 'none' : 'copy';
  };
  document.addEventListener('dragenter', over);
  document.addEventListener('dragover', over);
  document.addEventListener('drop', (e) => {
    const from = inside;
    inside = false;
    if (e.defaultPrevented || (editable(e) && !hasFiles(e))) return;
    e.preventDefault();
    clearActive();
    if (!from && !hasFiles(e)) toast(NOT_FILES);
  });
}

// nativeDrops reports whether this page is in one of the app's windows
// that take drops from the Finder, which Wails flags as it loads the page.
function nativeDrops() {
  const flags = window._wails && window._wails.flags;
  return Boolean(flags && flags.enableFileDrop === true);
}

// installDrops guards the page against stray drops, and once Wails'
// runtime has loaded, sends the drops that its native code didn't make to
// Go's DemoDropFiles. Tests wait for its handler by its name, benchDrop.
export async function installDrops() {
  guard();
  await runtimeReady().catch(() => {});
  const w = window._wails || (window._wails = {});
  const native = w.handlePlatformFileDrop;
  w.handlePlatformFileDrop = function benchDrop(paths, x, y) {
    if (native && nativeDrops()) native(paths, x, y);
    else previewDrop(paths, x, y);
  };
  if (!w.handleDragOver) {
    w.handleDragEnter = () => {};
    w.handleDragOver = hover;
    w.handleDragLeave = clearActive;
  }
}
