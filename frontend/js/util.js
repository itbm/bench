// Small helpers shared by the UI's modules.

// esc escapes text for HTML.
export const esc = (s) => String(s ?? '')
  .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
  .replace(/"/g, '&quot;').replace(/'/g, '&#39;');

// sentence ends text with a full stop if it doesn't have one.
export function sentence(text) {
  const t = String(text).trim();
  return /[.!?…]$/.test(t) ? t : t + '.';
}

// undoHint says what the latest change did (View.undo), from a capital
// letter, as "Closed Docs", for the hints of the Undo that names it.
export function undoHint(undo) {
  const did = String((undo && undo.did) || '');
  return did.charAt(0).toUpperCase() + did.slice(1);
}

// fmt formats a number the British way, such as 2,560.
export const fmt = (n) => Number(n).toLocaleString('en-GB');

// px formats a length in CSS pixels.
export const px = (n) => (Math.round(Number(n) * 100) / 100) + 'px';

// ICONS are the UI's inline SVG icons, from the prototype.
export const ICONS = {
  plus: '<svg aria-hidden="true" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><path d="M8 3v10M3 8h10"/></svg>',
  search: '<svg aria-hidden="true" width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><circle cx="7" cy="7" r="4.5"/><path d="M10.5 10.5L14 14"/></svg>',
  searchLarge: '<svg aria-hidden="true" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><circle cx="7" cy="7" r="4.5"/><path d="M10.5 10.5L14 14"/></svg>',
  // A terminal's window with a prompt: the add dialog's New terminal… form.
  terminal: '<svg aria-hidden="true" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><rect x="1.75" y="2.75" width="12.5" height="10.5" rx="1.5"/><path d="M4.5 6.25L6.5 8l-2 1.75M8.25 10h3"/></svg>',
  globe: '<svg aria-hidden="true" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><circle cx="8" cy="8" r="5.5"/><path d="M2.5 8h11M8 2.5c1.6 1.6 2.4 3.4 2.4 5.5S9.6 11.9 8 13.5M8 2.5C6.4 4.1 5.6 5.9 5.6 8s.8 3.9 2.4 5.5"/></svg>',
  // A cog: Settings, which apply to every tab.
  gear: '<svg aria-hidden="true" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"><path d="M6.63 3.51L6.89 1.7A6.4 6.4 0 0 1 9.11 1.7L9.37 3.51A4.7 4.7 0 0 1 10.21 3.85L11.67 2.76A6.4 6.4 0 0 1 13.24 4.33L12.15 5.79A4.7 4.7 0 0 1 12.49 6.63L14.3 6.89A6.4 6.4 0 0 1 14.3 9.11L12.49 9.37A4.7 4.7 0 0 1 12.15 10.21L13.24 11.67A6.4 6.4 0 0 1 11.67 13.24L10.21 12.15A4.7 4.7 0 0 1 9.37 12.49L9.11 14.3A6.4 6.4 0 0 1 6.89 14.3L6.63 12.49A4.7 4.7 0 0 1 5.79 12.15L4.33 13.24A6.4 6.4 0 0 1 2.76 11.67L3.85 10.21A4.7 4.7 0 0 1 3.51 9.37L1.7 9.11A6.4 6.4 0 0 1 1.7 6.89L3.51 6.63A4.7 4.7 0 0 1 3.85 5.79L2.76 4.33A6.4 6.4 0 0 1 4.33 2.76L5.79 3.85A4.7 4.7 0 0 1 6.63 3.51Z"/><circle cx="8" cy="8" r="2"/></svg>',
  splitRight: '<svg aria-hidden="true" width="15" height="15" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="3" width="12" height="10" rx="1.5"/><path d="M8 3v10M10.5 8h2M11.5 7v2"/></svg>',
  more: '<svg aria-hidden="true" width="15" height="15" viewBox="0 0 16 16" fill="currentColor"><circle cx="3.5" cy="8" r="1.25"/><circle cx="8" cy="8" r="1.25"/><circle cx="12.5" cy="8" r="1.25"/></svg>',
  chevron: '<svg aria-hidden="true" width="13" height="13" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M5.5 3.5L10 8l-4.5 4.5"/></svg>',
  // A button that opens a menu below it, such as a slim title bar's layout.
  chevronDown: '<svg aria-hidden="true" width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M4 6l4 4 4-4"/></svg>',
  chevronUp: '<svg aria-hidden="true" width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M4 10l4-4 4 4"/></svg>',
  // A speech bubble: the status bar's button for the recent messages.
  messages: '<svg aria-hidden="true" width="13" height="13" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M2.5 3h11v8H7.5L4.5 13.5V11h-2z"/><path d="M5.25 6h5.5M5.25 8.25h3.5"/></svg>',
  warning: '<svg aria-hidden="true" width="13" height="13" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M8 2.5l6 10.5H2z"/><path d="M8 6.5v3M8 11.5v.01"/></svg>',
  // The × that closes a tab or sub-tab.
  close: '<svg aria-hidden="true" width="10" height="10" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M3.5 3.5l9 9M12.5 3.5l-9 9"/></svg>',
  check: '<svg aria-hidden="true" width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round"><path d="M3.5 8.5l3 3 6-7"/></svg>',
  // Two overlapping windows: a window that's in other tabs too.
  shared: '<svg aria-hidden="true" width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="5.5" width="8.5" height="8.5" rx="1.5"/><path d="M5.5 5.5V3.5A1.5 1.5 0 0 1 7 2h5.5A1.5 1.5 0 0 1 14 3.5V9a1.5 1.5 0 0 1-1.5 1.5h-2"/></svg>',
  // A bell: a window that needs you, as a terminal that rang its bell or
  // a page that posted a notification while you couldn't see it does
  // (bellMark). Filled, and blue, so that it isn't taken for a
  // marker of a window that isn't ready.
  bell: '<svg aria-hidden="true" width="11" height="11" viewBox="0 0 16 16" fill="currentColor"><path d="M8 1.25c.55 0 1 .45 1 1v.55a4.5 4.5 0 0 1 3.5 4.39v2.56l1.3 1.73c.3.4.02.97-.48.97H2.68c-.5 0-.78-.57-.48-.97l1.3-1.73V7.19A4.5 4.5 0 0 1 7 2.8v-.55c0-.55.45-1 1-1z"/><path d="M6.25 13.5h3.5a1.75 1.75 0 0 1-3.5 0z"/></svg>',
  // Arrows back and forward, and a minus and a plus: a web page's Back,
  // Forward, Zoom Out, and Zoom In, in the Page row of its menu (pages.js).
  back: '<svg aria-hidden="true" width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M13 8H3.5M7.5 4L3.5 8l4 4"/></svg>',
  forward: '<svg aria-hidden="true" width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M3 8h9.5M8.5 4l4 4-4 4"/></svg>',
  minus: '<svg aria-hidden="true" width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><path d="M3.5 8h9"/></svg>',
  plusSmall: '<svg aria-hidden="true" width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><path d="M8 3.5v9M3.5 8h9"/></svg>',
  // A video camera and a microphone, and each struck through: a page that
  // uses the camera or the microphone, live or turned off (pages.js).
  camera: '<svg aria-hidden="true" width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><rect x="1.5" y="4" width="9" height="8" rx="1.5"/><path d="M10.5 7l4-2.25v6.5L10.5 9"/></svg>',
  cameraOff: '<svg aria-hidden="true" width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><rect x="1.5" y="4" width="9" height="8" rx="1.5"/><path d="M10.5 7l4-2.25v6.5L10.5 9M1.5 1.5l13 13"/></svg>',
  microphone: '<svg aria-hidden="true" width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><rect x="5.75" y="1.5" width="4.5" height="8" rx="2.25"/><path d="M3.25 7.5a4.75 4.75 0 0 0 9.5 0M8 12.25v2.25"/></svg>',
  microphoneOff: '<svg aria-hidden="true" width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><rect x="5.75" y="1.5" width="4.5" height="8" rx="2.25"/><path d="M3.25 7.5a4.75 4.75 0 0 0 9.5 0M8 12.25v2.25M1.5 1.5l13 13"/></svg>',
  // Four corners pointing out: a pane zoomed to fill its tab.
  zoomed: '<svg aria-hidden="true" width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M2.5 6V2.5H6M10 2.5h3.5V6M13.5 10v3.5H10M6 13.5H2.5V10"/></svg>',
  // The markers of a window that isn't ready (see MARKERS): a filled
  // warning sign, a hollow ring, and a filled circle with an i. Their
  // shapes tell them apart, not only their colours. The cut-out lines are
  // drawn in the dark colour (styles.css).
  markProblem: '<svg aria-hidden="true" width="11" height="11" viewBox="0 0 16 16"><path d="M8 1.5l7 12.5H1z" fill="currentColor" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"/><path class="mark-cut" d="M8 6v3.5M8 12v.01" fill="none" stroke-width="2" stroke-linecap="round"/></svg>',
  markOpening: '<svg aria-hidden="true" width="11" height="11" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2.5"><circle cx="8" cy="8" r="5.5"/></svg>',
  markInfo: '<svg aria-hidden="true" width="11" height="11" viewBox="0 0 16 16"><circle cx="8" cy="8" r="7" fill="currentColor"/><path class="mark-cut" d="M8 7.5v4M8 4.75v.01" fill="none" stroke-width="2" stroke-linecap="round"/></svg>',
};

// STATUS_SHORT names the statuses of items that aren't ready, for their
// sub-tabs and pane headers.
export const STATUS_SHORT = {
  'launching': 'Opening',
  'missing': 'Not open',
  'undocked': 'Moved out of the pane',
  'needs-room': 'Needs more room',
  'full-screen': 'In full screen',
  'unavailable': 'On another desktop',
  'minimised': 'Minimised',
};

// MARKERS are the markers on the sub-tabs of items that aren't ready, by
// status: a problem, which needs the user to do something; opening, while
// Bench waits for the window; or information, about where the window is.
const MARKERS = {
  'launching': 'opening',
  'missing': 'problem',
  'undocked': 'problem',
  'needs-room': 'problem',
  'full-screen': 'info',
  'unavailable': 'info',
  'minimised': 'info',
};

const MARK_ICONS = { problem: ICONS.markProblem, opening: ICONS.markOpening, info: ICONS.markInfo };

// needsYouText says how many of a tab's windows need you (TabView's
// attention), for its tooltip and a screen reader.
export function needsYouText(n) {
  return n === 1 ? '1 window needs you' : n + ' windows need you';
}

// bellMark returns the bell that marks a sub-tab whose item needs you
// (ItemView's attention), which says why in its tooltip, or a tab with
// some, which also says how many if there's more than one; or '' for none.
// A screen reader reads why from the sub-tab's or tab's own label.
export function bellMark(cls, why, count = 1) {
  if (!why) return '';
  return '<span class="bell-mark ' + cls + '" aria-hidden="true" title="' + esc(why) + '">' + ICONS.bell
    + (count > 1 ? '<span class="bell-count" aria-hidden="true">' + count + '</span>' : '') + '</span>';
}

// UNREAD_MAX is the largest count a pill shows: past it, it shows 999+, so
// that it stays narrow. Tooltips and a screen reader have the whole count.
const UNREAD_MAX = 999;

// hasUnread reports whether an unread count (ItemView's or TabView's
// unread) has something unread: a count, or a dot.
export function hasUnread(u) {
  return Boolean(u) && (u.count > 0 || Boolean(u.dot));
}

// unreadText says an unread count, for a tooltip or a screen reader: "3
// unread", "99+ unread", or "Unread" for a badge that isn't a number; or
// '' for none.
export function unreadText(u) {
  if (!hasUnread(u)) return '';
  return u.count > 0 ? fmt(u.count) + (u.more ? '+' : '') + ' unread' : 'Unread';
}

// unreadTip says an item's unread count for its tooltip, with "(whole app)"
// when it's its app's Dock badge and the app has other windows in Bench.
export function unreadTip(u) {
  const text = unreadText(u);
  return text && u.wholeApp ? text + ' (whole app)' : text;
}

// unreadNote says an item's unread count in lower case, for the list of
// notes in its sub-tab's label that a screen reader reads.
export function unreadNote(u) {
  const text = unreadText(u).toLowerCase();
  return text && u.wholeApp ? text + ' in the whole app' : text;
}

// unreadMark returns the grey pill with an unread count, or a grey dot for
// a badge that isn't a number, or '' for none. Its tooltip says the count;
// a screen reader reads it from the sub-tab's or tab's own label. A pill
// says how many characters it shows (data-chars), for the room that a
// sub-tab keeps for it at its narrowest (styles.css).
export function unreadMark(cls, u) {
  if (!hasUnread(u)) return '';
  const tip = esc(unreadTip(u));
  if (!(u.count > 0)) return '<span class="unread-mark unread-dot ' + cls + '" aria-hidden="true" title="' + tip + '"></span>';
  const shown = String(u.count > UNREAD_MAX ? UNREAD_MAX + '+' : u.count + (u.more ? '+' : ''));
  return '<span class="unread-mark ' + cls + '" aria-hidden="true" data-chars="' + shown.length + '" title="' + tip + '">' + esc(shown) + '</span>';
}

// notReady reports whether an item has a status other than ready.
export function notReady(item) {
  return Boolean(item) && Boolean(item.status) && item.status !== 'ready';
}

// markerOf returns the marker for an item's status, or '' for an item
// that's ready.
export function markerOf(item) {
  return notReady(item) ? MARKERS[item.status] || '' : '';
}

// statusMark returns a marker's HTML, or '' for none.
export function statusMark(marker) {
  return MARK_ICONS[marker] ? '<span class="status-mark" data-marker="' + marker + '">' + MARK_ICONS[marker] + '</span>' : '';
}

// statusText explains an item that isn't ready, if Go didn't.
export function statusText(item) {
  if (item.statusText) return item.statusText;
  switch (item.status) {
    case 'launching': return 'Opening ' + item.app + '…';
    case 'missing': return item.app + ' isn\'t open.';
    case 'undocked': return 'This window has been moved out of the pane.';
    case 'needs-room': return item.app + '\'s window can\'t shrink to fit this pane.';
    case 'full-screen': return 'In full screen';
    case 'unavailable': return 'This window is on another desktop.';
    case 'minimised': return 'Minimised';
    default: return '';
  }
}

// attentionOf returns the windows of the current tab, as drawn, that need
// attention: those with a problem, not those opening, in full screen, on
// another desktop, or minimised, which are for information. Each comes with
// the pane that shows it.
export function attentionOf(view) {
  const d = (view && view.display) || {};
  const out = [];
  for (const p of d.panes || []) {
    for (const id of p.items || []) {
      if (markerOf(itemOf(view, id)) === 'problem') out.push({ itemId: id, paneId: p.id });
    }
  }
  return out;
}

// radioState returns the ARIA state of a button of a segmented control,
// which is a radio group: whether it's chosen. Only the chosen one is a
// stop for the Tab key; the arrow keys move along the group, choosing as
// they go (roving.js). A group with none chosen, such as Tab properties'
// layouts for a custom layout, has its first as its stop (roving.js).
export function radioState(chosen) {
  return ' role="radio" aria-checked="' + chosen + '" tabindex="' + (chosen ? '0' : '-1') + '"';
}

// withKeys adds a shortcut to a control's tooltip, if it has one.
export function withKeys(text, keys) {
  return keys ? text + ' (' + keys + ')' : text;
}

// leaves returns a tree's panes in reading order.
export function leaves(node) {
  if (!node) return [];
  if (node.type === 'pane') return [node];
  return (node.children || []).flatMap(leaves);
}

// itemOf returns the item with an ID, from the tabs or "Not in a tab".
export function itemOf(view, id) {
  if (!view || !id) return null;
  if (view.items && view.items[id]) return view.items[id];
  return (view.floating || []).find((f) => f.id === id) || null;
}

// labelOf names an item as "App — title", or just "App" if its title is
// the app's name, as a web page's is before it sets one.
export function labelOf(view, id) {
  const it = itemOf(view, id);
  if (!it) return '';
  return it.title && it.title !== it.app ? it.app + ' — ' + it.title : it.app;
}

// joinNames joins names into a list: "Code", "Code and Notes", or "Code,
// Notes, and Research".
export function joinNames(names) {
  if (names.length < 3) return names.join(' and ');
  return names.slice(0, -1).join(', ') + ', and ' + names[names.length - 1];
}

// tabsOf returns the indexes of the tabs holding an item: Go lists them for
// an item in several tabs, and gives the one tab of any other.
export function tabsOf(item) {
  if (!item) return [];
  if (item.tabs && item.tabs.length) return item.tabs;
  return item.tab >= 0 ? [item.tab] : [];
}

// alsoInOf returns the names of the other tabs, besides the current one,
// that hold an item.
export function alsoInOf(view, item) {
  return tabsOf(item).filter((i) => i !== view.current && view.tabs[i]).map((i) => view.tabs[i].name);
}

// badgeOf returns the badge for an item's kind.
export function badgeOf(item) {
  return { web: 'Web', app: 'App', builtin: 'Built-in' }[item && item.kind] || '';
}

// isPage reports whether an item is a web page: a Web item other than VS
// Code for the Web, for which Go sends a folder instead of an address.
export function isPage(item) {
  return Boolean(item) && item.kind === 'web' && !item.folder;
}

// hostParts splits a page's host into the part before its site and the
// site, which the host chip emphasises: ['app.', 'slack.com'] for
// app.slack.com. A host that doesn't end with its site, as an IP address
// doesn't, is all site.
export function hostParts(host, site) {
  const h = String(host || '');
  const s = String(site || '');
  if (s && h.endsWith(s) && (h.length === s.length || h[h.length - s.length - 1] === '.')) {
    return [h.slice(0, h.length - s.length), s];
  }
  return ['', h];
}

// LAYOUT_MODES are the layout modes, with their names in the title bar.
export const LAYOUT_MODES = [['auto', 'Auto'], ['wide', 'Wide'], ['compact', 'Compact']];

// SMALL_SCREEN_TEXT describes the small-screen settings, for layoutText.
const SMALL_SCREEN_TEXT = {
  stack: 'Stack panes',
  tabs: 'One pane, every window as a sub-tab',
  same: 'Keep the layout',
};

// layoutText is the size class and why, for the tooltips of the layout
// control and of the status bar's layout, and the layout menu's note.
export function layoutText(view, tab) {
  const reason = view.sizeReason || '';
  if (/^(wide|compact) layout/i.test(reason)) return reason;
  let s = view.sizeClass === 'compact' ? 'Compact layout' : 'Wide layout';
  if (view.sizeClass === 'compact' && tab && SMALL_SCREEN_TEXT[tab.smallScreens]) s += ' (' + SMALL_SCREEN_TEXT[tab.smallScreens].toLowerCase() + ')';
  return reason ? s + ', ' + reason : s;
}

// statusBarShown reports whether the status bar is shown (the
// show_status_bar setting).
export function statusBarShown(view) {
  return ((view && view.settings) || {}).showStatusBar !== false;
}

// unsavedNote returns the note that Settings and Tab properties show while
// config.toml has problems, or ''. It says where the problems are listed:
// in the status bar, or, while that's hidden, in the title bar.
export function unsavedNote(view) {
  if (!view || view.configWritable !== false) return '';
  const where = statusBarShown(view) ? 'The status bar lists the problems.' : 'The notices in the title bar list the problems.';
  return '<p class="editor-note" role="note">' + esc('config.toml has problems, so changes apply but aren\'t saved to it. ' + where) + '</p>';
}

// scrollBarsShown reports whether the strips of tabs and sub-tabs show
// scroll bars: the scroll_bars setting, where auto follows macOS
// (systemScrollBars).
export function scrollBarsShown(view) {
  const choice = ((view && view.settings) || {}).scrollBars;
  if (choice === 'always' || choice === 'never') return choice === 'always';
  return Boolean(view && view.systemScrollBars);
}

// currentTab returns the current TabView.
export function currentTab(view) {
  return view && view.tabs ? view.tabs[view.current] : null;
}

// uniqueTabName returns name for tab except (-1 for a new tab), numbered as
// Go numbers it when it saves (uniqueTabName in internal/workspace) if
// another tab has it, ignoring case: "Code" becomes "Code (1)", or "Code
// (2)" if that's taken too. A number in brackets that it ends with is
// replaced, not added to.
export function uniqueTabName(tabs, name, except) {
  const same = (a, b) => a.toLowerCase() === b.toLowerCase();
  const taken = (n) => tabs.some((t, i) => i !== except && same(t.name, n));
  if (!name || !taken(name)) return name;
  const m = /^(.*) \((\d+)\)$/.exec(name);
  const base = m && m[1].trim() ? m[1].trim() : name;
  for (let n = 1; ; n++) {
    const next = base + ' (' + n + ')';
    if (!taken(next)) return next;
  }
}

// focusedPaneID returns the current tab's focused pane, as a real pane ID
// (never "compact").
export function focusedPaneID(view) {
  const tab = currentTab(view);
  if (tab && tab.focus) return tab.focus;
  const first = tab ? leaves(tab.root)[0] : null;
  return first ? first.id : '';
}

// activeOf returns the item a pane of the View's display shows.
export function activeOf(p) {
  const items = p.items || [];
  return p.active && items.includes(p.active) ? p.active : (items[0] || '');
}

// shownInFocus returns the item shown in the focused pane, as drawn.
export function shownInFocus(view) {
  const d = (view && view.display) || {};
  const p = (d.panes || []).find((x) => x.id === d.focus);
  return p ? activeOf(p) : '';
}

// inCurrentTab reports whether a pane is in the current tab.
export function inCurrentTab(view, paneId) {
  const tab = currentTab(view);
  return Boolean(tab) && leaves(tab.root).some((p) => p.id === paneId);
}

// realPaneOf returns the pane that holds an item in its tab, for when it's
// shown in the compact pane.
export function realPaneOf(view, itemID, paneID) {
  if (paneID !== 'compact') return paneID;
  const it = itemOf(view, itemID);
  return (it && it.pane) || paneID;
}

// windowElement returns the element that stands for Bench's window.
export function windowElement() {
  return document.getElementById('app-window');
}

// contentOrigin returns the client position of the window's content area:
// the top-left inside the window element's border.
export function contentOrigin() {
  const win = windowElement();
  const r = win.getBoundingClientRect();
  return { x: r.left + win.clientLeft, y: r.top + win.clientTop };
}

// contentRect returns an element's rectangle in window content coordinates.
export function contentRect(el) {
  const o = contentOrigin();
  const r = el.getBoundingClientRect();
  return { x: r.left - o.x, y: r.top - o.y, w: r.width, h: r.height };
}

// contentPoint returns a pointer event's position in content coordinates.
export function contentPoint(e) {
  const o = contentOrigin();
  return { x: e.clientX - o.x, y: e.clientY - o.y };
}

// sameRect reports whether two rectangles are equal.
export function sameRect(a, b) {
  return Boolean(a && b) && a.x === b.x && a.y === b.y && a.w === b.w && a.h === b.h;
}

// keysOf returns the configured global shortcuts in display form
// (workspace.KeysView), such as { jump: '⌃⌥Space' }. A shortcut that's
// turned off is ''.
export function keysOf(view) {
  return (view && view.keys) || {};
}

// shortcutPrefix returns the modifiers of the tabs' shortcuts, such as
// "⌃⌥", or '' if tab shortcuts are turned off.
export function shortcutPrefix(view) {
  return keysOf(view).tabs || '';
}

// MODIFIERS matches the modifiers at the start of a shortcut's display
// form, which are always in the order ⌃⌥⇧⌘.
const MODIFIERS = /^[⌃⌥⇧⌘]*/;

// splitShortcut returns a shortcut's modifiers and key, such as
// { mods: '⌃⌥', key: 'Space' } for "⌃⌥Space".
export function splitShortcut(display) {
  const s = String(display || '');
  const mods = s.match(MODIFIERS)[0];
  return { mods, key: s.slice(mods.length) };
}

// pairText shows two shortcuts together, sharing their modifiers if they
// have the same ones: "⌃⌥[ ]" for "⌃⌥[" and "⌃⌥]".
export function pairText(a, b) {
  if (!a || !b) return a || b || '';
  const x = splitShortcut(a);
  const y = splitShortcut(b);
  return x.mods === y.mods ? x.mods + x.key + ' ' + y.key : a + ' ' + b;
}

// RELEASE_WAIT is how long, in milliseconds, the UI waits for the button
// to come up after a move that reports no button held. WebKit on macOS reads
// the buttons when it handles an event, not when it happened, so the last
// move of a quick release can arrive that way just before the pointerup.
// Only when no release follows did the button come up where Bench couldn't
// see it.
export const RELEASE_WAIT = 250;

// TERM_KEY starts the key that stands for a terminal, which has no
// data-key: 'term:' and the item ID.
const TERM_KEY = 'term:';

// termKey returns the key that stands for item id's terminal.
export const termKey = (id) => TERM_KEY + id;

// termOfKey returns the item ID of the terminal that a 'term:<item ID>' key
// stands for, or '' for any other key.
export function termOfKey(key) {
  return key && key.startsWith(TERM_KEY) ? key.slice(TERM_KEY.length) : '';
}

// tabKey returns the data-key of tab index's button in the tab strip.
export const tabKey = (index) => 'tab-' + index;

// isTabKey reports whether a data-key is a tab button's (tabKey).
export const isTabKey = (key) => /^tab-\d+$/.test(key || '');

// focusKey focuses the control with a data-key, or the terminal that a
// 'term:<item ID>' key stands for if it's in a pane, and reports whether
// there was one.
export function focusKey(key) {
  if (!key) return false;
  if (key.startsWith(TERM_KEY)) {
    const host = document.querySelector('.term-host[data-term-host="' + CSS.escape(key.slice(TERM_KEY.length)) + '"]');
    const input = host && host.closest('[data-term]') ? host.querySelector('textarea') : null;
    if (!input) return false;
    input.focus();
    return document.activeElement === input;
  }
  const el = document.querySelector('[data-key="' + CSS.escape(key) + '"]');
  if (!el) return false;
  el.focus();
  return true;
}

// activeKey returns the data-key of the focused control, if it has one, or
// 'term:<item ID>' if a terminal has focus.
export function activeKey() {
  const el = document.activeElement;
  if (!el) return null;
  const host = el.closest ? el.closest('.term-host[data-term-host]') : null;
  if (host) return TERM_KEY + host.dataset.termHost;
  return el.dataset && el.dataset.key ? el.dataset.key : null;
}
