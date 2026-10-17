// The ⋯ menu for a window, and the status bar's list of notices.

import { act, closeMenu, store, update } from './state.js';
import { contentOrigin, esc, itemOf, labelOf, realPaneOf, windowElement } from './util.js';

// windowMenuEntries lists what the ⋯ menu offers for an item shown in a pane.
export function windowMenuEntries(view, itemId, paneId) {
  const t = view.current;
  const d = view.display || {};
  const entries = [];
  if (d.splitsDisabled) {
    entries.push({ label: 'Show side by side (split right)', keys: '⌃⌥⇧→', disabled: true });
    entries.push({ label: 'Split down', keys: '⌃⌥⇧↓', disabled: true });
    entries.push({ note: d.splitsDisabled });
  } else {
    entries.push({ label: 'Show side by side (split right)', keys: '⌃⌥⇧→', run: () => act('MoveItem', itemId, t, paneId, 'right') });
    entries.push({ label: 'Split down', keys: '⌃⌥⇧↓', run: () => act('MoveItem', itemId, t, paneId, 'bottom') });
  }
  const others = (d.panes || []).filter((p) => p.id !== paneId && p.id !== 'compact');
  if (others.length) {
    entries.push({ sep: true }, { heading: 'Move to pane' });
    others.forEach((p) => entries.push({
      label: p.active ? 'With ' + labelOf(view, p.active) : 'Empty pane',
      run: () => act('MoveItem', itemId, t, p.id, 'center'),
    }));
  }
  if ((view.tabs || []).length > 1) {
    entries.push({ sep: true }, { heading: 'Move to tab' });
    view.tabs.forEach((other, i) => {
      if (i !== t) entries.push({ label: other.name, run: () => act('MoveItemToTab', itemId, i) });
    });
  }
  entries.push({ sep: true });
  const item = itemOf(view, itemId);
  if (item && item.status === 'undocked') entries.push({ label: 'Dock again', run: () => act('Redock', itemId) });
  entries.push({ label: 'Pop out of Bench', run: () => act('PopOut', itemId) });
  return entries;
}

// openWindowMenu opens the ⋯ menu below a pane's menu button.
export function openWindowMenu(button, itemId, paneId) {
  const view = store.view;
  const win = windowElement();
  const o = contentOrigin();
  const r = button.getBoundingClientRect();
  store.ui.menu = {
    kind: 'window',
    itemId,
    paneId: realPaneOf(view, itemId, paneId),
    shownIn: paneId,
    x: Math.max(8, Math.min(r.right - o.x - 260, win.clientWidth - 330)),
    y: r.bottom - o.y + 4,
    opener: button.dataset.key || null,
  };
  store.ui.pendingFocus = 'menu-item-0';
  update();
}

// openNotices opens the list of notices above the status bar.
export function openNotices(button) {
  const win = windowElement();
  const o = contentOrigin();
  const r = button.getBoundingClientRect();
  store.ui.menu = {
    kind: 'notices',
    x: Math.max(8, Math.min(r.left - o.x, win.clientWidth - 490)),
    bottom: win.clientHeight - (r.top - o.y) + 6,
    opener: button.dataset.key || null,
  };
  store.ui.pendingFocus = 'notices-list';
  update();
}

// runMenuEntry runs the open menu's entry at index and closes the menu.
export function runMenuEntry(index) {
  const entry = store.ui.menuEntries[index];
  closeMenu(true);
  if (entry && entry.run && !entry.disabled) entry.run();
}

// renderMenu returns the open menu's HTML.
export function renderMenu(view, ui) {
  const m = ui.menu;
  ui.menuEntries = [];
  if (!m) return '';
  if (m.kind === 'notices') return renderNotices(view, m);
  if (!itemOf(view, m.itemId)) return '';
  const entries = windowMenuEntries(view, m.itemId, m.paneId);
  ui.menuEntries = entries;
  let n = 0;
  let noteId = '';
  const disabledIds = [];
  const html = entries.map((e, i) => {
    if (e.sep) return '<div class="menu-sep" role="separator"></div>';
    if (e.heading) return '<div class="menu-heading" role="presentation">' + esc(e.heading) + '</div>';
    if (e.note) {
      noteId = 'menu-note-' + i;
      return '<div class="menu-note" role="none" id="' + noteId + '">' + esc(e.note) + '</div>';
    }
    if (e.disabled) disabledIds.push(i);
    const key = e.disabled ? 'menu-disabled-' + i : 'menu-item-' + (n++);
    return '<button type="button" class="menu-item" role="menuitem" data-action="menu-run" data-index="' + i + '" data-key="' + key + '"'
      + (e.disabled ? ' disabled aria-describedby="@NOTE@"' : '') + '>'
      + '<span class="menu-label">' + esc(e.label) + '</span>' + (e.keys ? '<kbd>' + esc(e.keys) + '</kbd>' : '') + '</button>';
  }).join('').replaceAll('@NOTE@', noteId);
  return '<div class="menu" role="menu" aria-label="Window options" id="window-menu" style="left: ' + m.x + 'px; top: ' + m.y + 'px">' + html + '</div>';
}

function renderNotices(view, m) {
  const notices = view.notices || [];
  if (!notices.length) return '';
  return '<div class="menu wide" role="dialog" aria-label="Notices" id="notices-menu" style="left: ' + m.x + 'px; bottom: ' + m.bottom + 'px">'
    + '<div class="menu-heading" role="presentation">Notices</div>'
    + '<div tabindex="-1" data-key="notices-list" role="list" aria-label="Notices">'
    + notices.map((n) => '<div class="menu-text" role="listitem">' + esc(n) + '</div>').join('')
    + '</div></div>';
}

// fitMenu moves an open menu up if it would go past the bottom of the
// window.
export function fitMenu() {
  const el = document.querySelector('#menu-root .menu');
  if (!el || !store.ui.menu || store.ui.menu.kind !== 'window') return;
  const win = windowElement();
  const top = el.offsetTop;
  const over = top + el.offsetHeight - (win.clientHeight - 8);
  if (over > 0) el.style.top = Math.max(8, top - over) + 'px';
}
