// Small helpers shared by the UI's modules.

// esc escapes text for HTML.
export const esc = (s) => String(s ?? '')
  .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
  .replace(/"/g, '&quot;').replace(/'/g, '&#39;');

// fmt formats a number the British way, such as 2,560.
export const fmt = (n) => Number(n).toLocaleString('en-GB');

// px formats a length in CSS pixels.
export const px = (n) => (Math.round(Number(n) * 100) / 100) + 'px';

// ICONS are the UI's inline SVG icons, from the prototype.
export const ICONS = {
  plus: '<svg aria-hidden="true" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><path d="M8 3v10M3 8h10"/></svg>',
  search: '<svg aria-hidden="true" width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><circle cx="7" cy="7" r="4.5"/><path d="M10.5 10.5L14 14"/></svg>',
  searchLarge: '<svg aria-hidden="true" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><circle cx="7" cy="7" r="4.5"/><path d="M10.5 10.5L14 14"/></svg>',
  globe: '<svg aria-hidden="true" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><circle cx="8" cy="8" r="5.5"/><path d="M2.5 8h11M8 2.5c1.6 1.6 2.4 3.4 2.4 5.5S9.6 11.9 8 13.5M8 2.5C6.4 4.1 5.6 5.9 5.6 8s.8 3.9 2.4 5.5"/></svg>',
  layout: '<svg aria-hidden="true" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><rect x="2" y="3" width="12" height="10" rx="1.5"/><path d="M9.5 3v10"/></svg>',
  splitRight: '<svg aria-hidden="true" width="15" height="15" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="3" width="12" height="10" rx="1.5"/><path d="M8 3v10M10.5 8h2M11.5 7v2"/></svg>',
  more: '<svg aria-hidden="true" width="15" height="15" viewBox="0 0 16 16" fill="currentColor"><circle cx="3.5" cy="8" r="1.25"/><circle cx="8" cy="8" r="1.25"/><circle cx="12.5" cy="8" r="1.25"/></svg>',
  chevron: '<svg aria-hidden="true" width="13" height="13" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M5.5 3.5L10 8l-4.5 4.5"/></svg>',
  warning: '<svg aria-hidden="true" width="13" height="13" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M8 2.5l6 10.5H2z"/><path d="M8 6.5v3M8 11.5v.01"/></svg>',
  check: '<svg aria-hidden="true" width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round"><path d="M3.5 8.5l3 3 6-7"/></svg>',
};

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

// labelOf names an item as "App — title".
export function labelOf(view, id) {
  const it = itemOf(view, id);
  if (!it) return '';
  return it.title ? it.app + ' — ' + it.title : it.app;
}

// badgeOf returns the badge for an item's kind.
export function badgeOf(item) {
  return { web: 'Web', app: 'App', builtin: 'Built-in' }[item && item.kind] || '';
}

// currentTab returns the current TabView.
export function currentTab(view) {
  return view && view.tabs ? view.tabs[view.current] : null;
}

// focusedPaneID returns the current tab's focused pane, as a real pane ID
// (never "compact").
export function focusedPaneID(view) {
  const tab = currentTab(view);
  if (tab && tab.focus) return tab.focus;
  const first = tab ? leaves(tab.root)[0] : null;
  return first ? first.id : '';
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

// shortcutPrefix returns the modifiers of the tabs' shortcuts, such as
// "⌃⌥", from the first tab that has one.
export function shortcutPrefix(view) {
  const withKeys = (view.tabs || []).find((t) => t.shortcut);
  return withKeys ? withKeys.shortcut.replace(/\d+$/, '') : '⌃⌥';
}

// focusKey focuses the control with a data-key, and reports whether there
// was one.
export function focusKey(key) {
  if (!key) return false;
  const el = document.querySelector('[data-key="' + CSS.escape(key) + '"]');
  if (!el) return false;
  el.focus();
  return true;
}

// activeKey returns the data-key of the focused control, if it has one.
export function activeKey() {
  const el = document.activeElement;
  return el && el.dataset && el.dataset.key ? el.dataset.key : null;
}
