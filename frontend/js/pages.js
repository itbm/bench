// A web page's controls: the Page group of its ⋯ and sub-tab menus, which
// the jump palette's This window lists too, and Pane ▸ Page in the menu bar:
// Reload Page, Back and Forward, the page's zoom, find in page, its link, and
// what it does with the camera, the microphone, and sound. Go does each
// (decision 50). Its find bar is drawn at the top of its pane's body, where
// Go leaves room above the page, whose web view is drawn over Bench's UI.
// The marks on a sub-tab and a tab whose page uses the camera or the
// microphone are here too.

import { setClipboardText } from './bridge.js';
import { act, isDemo, store, toast, update } from './state.js';
import { esc, ICONS, isPage, itemOf, joinNames } from './util.js';

// ZOOM_MIN and ZOOM_MAX are the page zooms, in percent, that Zoom Out and
// Zoom In stop at, as Go's do.
const ZOOM_MIN = 50;
const ZOOM_MAX = 300;

// pageEntries lists the Page group of a Web item's window menu, each under
// Page, which names them in the jump palette ("Page ▸ Find in Page…"), with
// the names that the menu bar's Pane ▸ Page gives them. Back, Forward, and
// the zooms are buttons in one row, labelled Page, as in a browser's menu,
// so that the group takes little room in a menu that has much else
// (renderEntries): Back and Forward are off where the page can't go back or
// forward, as Go last heard from its web view, and the zooms at their ends,
// with Actual Size showing the zoom. Reload Page is with the menu's common
// actions, at its top (reloadEntry). Copy Page Link is for a web page, not
// VS Code for the Web, whose address holds the key to its server. The
// camera and microphone entries are there while the page uses them. Stop
// Sound and Video is always there: Bench can't tell whether a page plays
// sound.
export function pageEntries(item) {
  if (!item || item.kind !== 'web') return [];
  const id = item.id;
  const under = 'Page';
  const entry = (label, on, run, more = {}) => (on ? { label, under, run, ...more } : { label, under, disabled: true, ...more });
  const button = (label, icon, on, run, more = {}) => entry(label, on, run, { row: 'page', icon, ...more });
  const zoom = item.zoom || 100;
  const entries = [];
  entries.push(button('Back', ICONS.back, item.canGoBack, () => act('GoBack', id)));
  entries.push(button('Forward', ICONS.forward, item.canGoForward, () => act('GoForward', id)));
  entries.push(button('Zoom Out', ICONS.minus, zoom > ZOOM_MIN, () => act('ZoomPage', id, -1), { gap: true }));
  entries.push(button('Actual Size', '', zoom !== 100, () => act('ZoomPage', id, 0), { text: zoom + '%', detail: 'Now ' + zoom + '%' }));
  entries.push(button('Zoom In', ICONS.plusSmall, zoom < ZOOM_MAX, () => act('ZoomPage', id, 1)));
  entries.push(entry('Find in Page…', true, () => openPageFind(id)));
  if (isPage(item)) entries.push(entry('Copy Page Link', true, () => copyPageLink(id)));
  if (item.microphone) {
    const muted = item.microphone === 'muted';
    entries.push(entry(muted ? 'Unmute Microphone' : 'Mute Microphone', true, () => act('MuteCapture', id, 'microphone', !muted)));
  }
  if (item.camera) {
    const off = item.camera === 'muted';
    entries.push(entry(off ? 'Turn On Camera' : 'Turn Off Camera', true, () => act('MuteCapture', id, 'camera', !off)));
  }
  entries.push(entry('Stop Sound and Video', true, () => act('StopMedia', id)));
  return entries;
}

// reloadEntry is a Web item's Reload Page, the first of its window menu's
// common actions, as Pane ▸ Page ▸ Reload Page in the menu bar.
export function reloadEntry(item) {
  return { label: 'Reload Page', run: () => act('ReloadPage', item.id) };
}

// copyPageLink puts the address of the page that a Web item shows on the
// clipboard, as Go gives it (PageLink), and says so.
export async function copyPageLink(itemId) {
  const r = await act('PageLink', itemId);
  if (!r.ok) return;
  try {
    await setClipboardText(r.value, isDemo());
  } catch {
    toast('Bench couldn\'t copy to the clipboard.');
    return;
  }
  toast('Copied the page\'s link.');
}

// findOf returns what a page's find bar has, making it the first time.
function findOf(itemId) {
  const all = store.ui.pageFind;
  if (!all[itemId]) all[itemId] = { text: '', none: false, busy: false };
  return all[itemId];
}

// openPageFind opens a Web item's find bar, or, if it's open, gives its
// field the keyboard again, with what it has selected, so that typing
// replaces it. Go moves the page down to make room, and gives Bench's UI
// the keyboard (SetFindBar).
export async function openPageFind(itemId) {
  findOf(itemId);
  store.ui.findFocus = itemId;
  const r = await act('SetFindBar', itemId, true);
  if (!r.ok) {
    if (store.ui.findFocus === itemId) store.ui.findFocus = null;
    return;
  }
  update();
}

// closePageFind closes a Web item's find bar, which gives the page its
// room and, in a focused pane, the keyboard back.
export async function closePageFind(itemId) {
  const f = findOf(itemId);
  f.none = false;
  if (store.ui.findFocus === itemId) store.ui.findFocus = null;
  await act('SetFindBar', itemId, false);
}

// findInPage finds the next match of what a page's find bar has, or the
// one before, if backwards. WebKit selects it and scrolls to it; the bar
// says if there's none. It finds from the match selected, so it finds
// when asked, with Return or the bar's arrows, rather than as each letter
// is typed, which would skip the match that the letters before found.
export async function findInPage(itemId, backwards) {
  const f = findOf(itemId);
  if (!f.text) return;
  f.busy = true;
  const r = await act('FindInPage', itemId, f.text, backwards);
  f.busy = false;
  f.none = r.ok && !r.value;
  update();
}

// setFindText notes what's typed in a page's find bar.
export function setFindText(itemId, text) {
  const f = findOf(itemId);
  f.text = text;
  if (f.none) {
    f.none = false;
    update();
  }
}

// pageFindKeys handles keys in a find bar's field: Return finds the next
// match, Shift-Return the one before, and Escape closes the bar. Keys that
// an input method uses to compose text are its own.
export function pageFindKeys(e) {
  const id = e.target.dataset.pageFind;
  if (e.isComposing || e.keyCode === 229) return;
  if (e.key === 'Enter' && !e.metaKey && !e.ctrlKey && !e.altKey) {
    e.preventDefault();
    findInPage(id, e.shiftKey);
  } else if (e.key === 'Escape') {
    e.preventDefault();
    e.stopPropagation();
    closePageFind(id);
  }
}

// pageFindBar returns the find bar of a Web item whose bar is open
// (ItemView's finding): a field, a note when there's no match, and buttons
// for the match before and the next one, and to close the bar. It fills
// the room that Go leaves above the page (layout.FindBarHeight, which
// styles.css's --find-bar matches).
export function pageFindBar(item) {
  const id = esc(item.id);
  const f = findOf(item.id);
  const button = (action, label, icon) => '<button type="button" class="icon-btn page-find-btn" data-action="' + action + '" data-item="' + id + '"'
    + ' data-key="' + action + '-' + id + '" aria-label="' + esc(label) + '" title="' + esc(label) + '">' + icon + '</button>';
  return '<div class="page-find" role="search" aria-label="Find in page">'
    + '<input type="text" class="page-find-input' + (f.none ? ' none' : '') + '" data-page-find="' + id + '" data-key="find-' + id + '"'
    + ' value="' + esc(f.text) + '" placeholder="Find in page" aria-label="Find in page" autocomplete="off" autocapitalize="off"'
    + ' autocorrect="off" spellcheck="false">'
    + '<span class="page-find-count' + (f.none ? ' none' : '') + '" aria-live="polite">' + (f.none ? 'No matches' : '') + '</span>'
    + button('find-previous', 'Previous match (⇧↩)', ICONS.chevronUp)
    + button('find-next', 'Next match (↩)', ICONS.chevronDown)
    + button('close-find', 'Close (⎋)', ICONS.close)
    + '</div>';
}

// focusPageFind gives the keyboard to the find bar that openPageFind
// opened, once it's drawn, with its text selected.
export function focusPageFind(root) {
  const id = store.ui.findFocus;
  if (!id) return;
  const input = root.querySelector('[data-page-find="' + CSS.escape(id) + '"]');
  if (!input) return;
  store.ui.findFocus = null;
  input.focus();
  input.select();
}

// forgetPageFinds drops what the find bars of items that have gone had.
export function forgetPageFinds(view) {
  for (const id of Object.keys(store.ui.pageFind)) {
    if (!itemOf(view, id)) delete store.ui.pageFind[id];
  }
}

// CAPTURE describes the marks of a page that uses the camera or the
// microphone: their icons, live and turned off, and what they say.
const CAPTURE = {
  camera: { live: ICONS.camera, off: ICONS.cameraOff, using: 'Using the camera', muted: 'Camera turned off', name: 'the camera' },
  microphone: { live: ICONS.microphone, off: ICONS.microphoneOff, using: 'Using the microphone', muted: 'Microphone muted', name: 'the microphone' },
};

// captureNotes says how a Web item's page uses the camera and the
// microphone, a sentence each, for its sub-tab's tooltip and label.
export function captureNotes(item) {
  if (!item) return [];
  return ['camera', 'microphone'].filter((d) => item[d]).map((d) => (item[d] === 'muted' ? CAPTURE[d].muted : CAPTURE[d].using));
}

// captureMarks returns the marks of a Web item's page that uses the camera
// or the microphone, for its sub-tab: a camera and a microphone, green
// while live, and struck through and grey while turned off. Each says so
// in its tooltip; the sub-tab's own label says so to a screen reader.
export function captureMarks(item) {
  if (!item) return '';
  return ['camera', 'microphone'].filter((d) => item[d]).map((d) => {
    const muted = item[d] === 'muted';
    return '<span class="capture-mark" data-capture="' + (muted ? 'muted' : 'live') + '" aria-hidden="true" title="'
      + esc(muted ? CAPTURE[d].muted : CAPTURE[d].using) + '">' + (muted ? CAPTURE[d].off : CAPTURE[d].live) + '</span>';
  }).join('');
}

// tabCaptureText says how a tab's pages use the camera and the microphone
// (TabView's camera, microphone, cameraLive, and microphoneLive), or '' if
// none does: which are live, and which are turned off.
export function tabCaptureText(tab) {
  const devices = ['camera', 'microphone'].filter((d) => tab[d]);
  if (!devices.length) return '';
  const live = devices.filter((d) => tab[d + 'Live']).map((d) => CAPTURE[d].name);
  const off = devices.filter((d) => !tab[d + 'Live']).map((d) => CAPTURE[d].name);
  const many = Math.max(tab.camera || 0, tab.microphone || 0) > 1;
  if (!live.length) return (many ? 'Pages here have ' : 'A page here has ') + joinNames(off) + ' turned off';
  const text = (many ? 'Pages here are using ' : 'A page here is using ') + joinNames(live);
  return off.length ? text + ', with ' + joinNames(off) + ' turned off' : text;
}

// tabCaptureMark returns a tab's mark for its pages that use the camera or
// the microphone: one mark, green for a device that's live, the camera's
// if it is, or else struck through and grey, the camera's if any page uses
// it, with what it means in its tooltip, and for a screen reader.
export function tabCaptureMark(tab) {
  const text = tabCaptureText(tab);
  if (!text) return '';
  const live = tab.cameraLive || tab.microphoneLive;
  const d = live ? (tab.cameraLive ? 'camera' : 'microphone') : (tab.camera ? 'camera' : 'microphone');
  return '<span class="capture-mark tab-capture" data-capture="' + (live ? 'live' : 'muted') + '" data-device="' + d + '" aria-hidden="true" title="' + esc(text) + '">'
    + (live ? CAPTURE[d].live : CAPTURE[d].off) + '</span>'
    + '<span class="visually-hidden">, ' + esc(text) + '</span>';
}

// PAGE_ACTIONS run a find bar's buttons, by their data-action.
export const PAGE_ACTIONS = {
  'find-next': (t) => findInPage(t.dataset.item, false),
  'find-previous': (t) => findInPage(t.dataset.item, true),
  'close-find': (t) => closePageFind(t.dataset.item),
};

// pageCommand does what Pane ▸ Page's Find in Page… and Copy Page Link ask
// (find-in-page and copy-page-link), for the item that Go names.
export function pageCommand(name, itemId) {
  if (!itemId) return;
  if (name === 'find-in-page') openPageFind(itemId);
  else if (name === 'copy-page-link') copyPageLink(itemId);
}
