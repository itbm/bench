// The jump palette and the add dialog. They share their markup and keys: a
// search field, grouped rows, arrows to move, Return to choose, and Escape
// or the backdrop to close.

import { act, closeOverlay, runPaneLater, store, toast, update } from './state.js';
import { activeKey, currentTab, esc, focusedPaneID, ICONS, itemOf, keysOf, leaves, sentence } from './util.js';
import { openEditor } from './editor.js';

// openPalette opens the jump palette.
export function openPalette() {
  const { ui } = store;
  if (!ui.overlay) ui.opener = activeKey();
  ui.menu = null;
  ui.paneLater = null;
  ui.overlay = 'palette';
  ui.add = null;
  ui.editor = null;
  ui.palette = { query: '', sel: 0 };
  ui.pendingFocus = 'palette-query';
  update();
}

// togglePalette opens the jump palette, or closes it if it's open.
export function togglePalette() {
  if (store.ui.overlay === 'palette') closeOverlay();
  else openPalette();
}

// openAdd opens the add dialog for a pane, with its rows from Go. If Go
// refuses, the click that asked for it focuses its pane now
// (ui.paneLater).
export async function openAdd(paneId) {
  const opener = activeKey();
  const tab = store.view.current;
  const r = await act('AddOptions', paneId);
  if (!r.ok) {
    runPaneLater();
    return;
  }
  const { ui } = store;
  if (!ui.overlay) ui.opener = opener;
  ui.menu = null;
  ui.editor = null;
  ui.overlay = 'add';
  ui.add = { paneId, tab, options: Array.isArray(r.value) ? r.value : [], step: 'list', option: null, url: '' };
  ui.palette = { query: '', sel: 0 };
  ui.pendingFocus = 'palette-query';
  update();
}

// showWindow switches to an item's tab if needed, then shows it in its pane.
async function showWindow(view, tabIndex, paneId, itemId) {
  if (tabIndex !== view.current) {
    const r = await act('SwitchTab', tabIndex);
    if (!r.ok) return;
  }
  await act('ShowItem', paneId, itemId);
}

// paletteItems lists the jump palette's rows, in group order.
export function paletteItems(view) {
  const items = [];
  view.tabs.forEach((t, i) => items.push({
    group: 'Tabs', mono: String(i + 1), label: t.name,
    hint: (t.apps || []).join(', ') + (i === view.current ? ' · current' : ''),
    keys: t.shortcut || '', run: () => act('SwitchTab', i),
  }));
  view.tabs.forEach((t, i) => leaves(t.root).forEach((p) => (p.items || []).forEach((id) => {
    const it = itemOf(view, id);
    if (!it) return;
    items.push({
      group: 'Windows', mono: it.mono, label: it.title || it.app, hint: t.name + ' · ' + it.app, keys: '', itemId: id,
      run: () => showWindow(view, i, p.id, id),
    });
  })));
  (view.floating || []).forEach((it) => items.push({
    group: 'Not in a tab', mono: it.mono, label: it.title || it.app, hint: it.app + ' · add to this tab', keys: '', itemId: it.id,
    run: () => act('AddToPane', focusedPaneID(view), { type: 'floating', id: it.id }),
  }));
  const tab = currentTab(view);
  const cur = tab ? tab.name : '';
  items.push({ group: 'Actions', label: 'Rearrange this tab', hint: '', keys: keysOf(view).rearrange || '', run: () => act('Rearrange') });
  items.push({ group: 'Actions', label: 'Edit the layout of ' + cur + '…', hint: '', keys: '', stay: true, run: () => openEditor(view.current) });
  items.push({ group: 'Actions', label: 'New tab…', hint: '', keys: '', stay: true, run: () => openEditor(-1) });
  if (view.mode !== 'compact') items.push({ group: 'Actions', label: 'Use the compact layout', hint: '', keys: '', run: () => act('SetMode', 'compact') });
  if (view.mode !== 'wide') items.push({ group: 'Actions', label: 'Use the wide layout', hint: '', keys: '', run: () => act('SetMode', 'wide') });
  if (view.mode !== 'auto') items.push({ group: 'Actions', label: 'Let Auto choose the layout', hint: '', keys: '', run: () => act('SetMode', 'auto') });
  return items;
}

// addItems lists the add dialog's rows, from AddOptions.
export function addItems(ui) {
  const add = ui.add;
  if (!add) return [];
  return add.options.map((o) => ({
    group: o.group, mono: o.mono, label: o.label, hint: o.hint || '', keys: '',
    stay: Boolean(o.asksForUrl),
    run: () => (o.asksForUrl ? askForURL(o) : act('AddToPane', add.paneId, o.source)),
  }));
}

function askForURL(option) {
  const { ui } = store;
  if (!ui.add) return;
  ui.add.step = 'url';
  ui.add.option = option;
  ui.add.url = '';
  ui.pendingFocus = 'palette-url';
  update();
}

// backToList leaves the add dialog's address step.
export function backToList() {
  const { ui } = store;
  if (!ui.add) return;
  ui.add.step = 'list';
  ui.pendingFocus = 'palette-query';
  update();
}

// submitURL adds a web page with the typed address. The dialog stays open
// if Go refuses it, so the address can be fixed.
export async function submitURL() {
  const { ui } = store;
  const add = ui.add;
  if (!add || !add.option) return;
  const url = add.url.trim();
  if (!url) {
    toast('Type the address of a web page.');
    return;
  }
  const r = await act('AddToPane', add.paneId, { ...add.option.source, url });
  if (r.ok && ui.add === add) {
    // Adding focuses the pane.
    ui.paneLater = null;
    closeOverlay();
  }
}

// filterItems keeps the rows whose label or hint contains the query.
export function filterItems(items, query) {
  const q = String(query || '').trim().toLowerCase();
  return items.filter((it) => !q || it.label.toLowerCase().includes(q) || it.hint.toLowerCase().includes(q));
}

// listItems returns the open dialog's rows, filtered by its query.
export function listItems(view, ui) {
  const all = ui.overlay === 'add' ? addItems(ui) : paletteItems(view);
  return filterItems(all, ui.palette.query);
}

// runListItem runs a row of the open palette or add dialog. A row that
// closes the dialog does its own focusing, so the click that opened the
// dialog doesn't (ui.paneLater).
export function runListItem(index) {
  const item = store.ui.listItems[index];
  if (!item) return;
  if (!item.stay) {
    store.ui.paneLater = null;
    closeOverlay();
  }
  item.run();
}

// moveSelection moves the selected row up or down.
export function moveSelection(step) {
  const { ui } = store;
  const n = ui.listItems.length;
  ui.palette.sel = Math.max(0, Math.min(ui.palette.sel + step, Math.max(n - 1, 0)));
  update();
  const el = document.querySelector('.palette-item[aria-selected="true"]');
  if (el && typeof el.scrollIntoView === 'function') el.scrollIntoView({ block: 'nearest' });
}

function renderList(items, sel, emptyText) {
  let html = '';
  let group = null;
  items.forEach((it, i) => {
    if (it.group !== group) {
      group = it.group;
      html += '<div class="palette-group" role="presentation">' + esc(group) + '</div>';
    }
    const selected = i === sel;
    const tile = it.mono
      ? '<span class="mono-tile" aria-hidden="true">' + esc(it.mono) + '</span>'
      : '<span class="action-tile" aria-hidden="true">' + ICONS.chevron + '</span>';
    const keys = selected ? (it.keys ? it.keys + '  ↵' : '↵') : it.keys;
    html += '<button type="button" class="palette-item" role="option" id="palette-item-' + i + '" aria-selected="' + selected + '" data-action="palette-run" data-index="' + i + '"'
      + (it.itemId ? ' data-item="' + esc(it.itemId) + '"' : '') + ' tabindex="-1">'
      + tile + '<span class="palette-label">' + esc(it.label) + '</span>'
      + '<span class="palette-hint">' + esc(it.hint) + '</span>'
      + '<kbd class="palette-keys">' + esc(keys) + '</kbd></button>';
  });
  if (!items.length) html = '<div class="palette-empty">' + esc(emptyText) + '</div>';
  return html;
}

// isList reports whether the open dialog is the palette or the add
// dialog's list of rows.
export function isList(ui) {
  return ui.overlay === 'palette' || (ui.overlay === 'add' && Boolean(ui.add) && ui.add.step !== 'url');
}

// renderPalette returns the jump palette's or add dialog's HTML, without
// its rows (renderRows) or the search field's text (fillPalette), so that
// neither typing nor a new View replaces the search field while it's in
// use.
export function renderPalette(view, ui) {
  const kind = ui.overlay;
  if (kind === 'add' && ui.add && ui.add.step === 'url') return renderURLStep(ui);
  const title = kind === 'add' ? 'Add a window or app to this pane' : 'Jump to a tab, window, or action';
  return '<div class="dialog palette" role="dialog" aria-modal="true" aria-label="' + esc(title) + '">'
    + '<div class="palette-search">' + ICONS.searchLarge
    + '<label for="palette-query" class="visually-hidden">' + esc(title) + '</label>'
    + '<input id="palette-query" data-key="palette-query" type="text" autocomplete="off" spellcheck="false" role="combobox" aria-expanded="true" aria-controls="palette-list" aria-autocomplete="list"'
    + ' placeholder="' + esc(title) + '">'
    + '<button type="button" class="esc" data-action="close" data-key="palette-close" aria-label="Close">esc</button></div>'
    + '<div class="palette-list" id="palette-list" role="listbox" aria-label="Results"></div>'
    + '<div class="palette-foot"><span><kbd>↑ ↓</kbd> move</span><span><kbd>↵</kbd> choose</span><span><kbd>esc</kbd> close</span>'
    + '<span>' + (kind === 'add' ? 'Any app works, not just these' : 'Type a window title or app name') + '</span></div></div>';
}

// renderRows returns the open palette's or add dialog's rows, filtered by
// its query, and keeps them as ui.listItems.
export function renderRows(view, ui) {
  const items = listItems(view, ui);
  ui.listItems = items;
  const sel = Math.min(ui.palette.sel, Math.max(items.length - 1, 0));
  ui.palette.sel = sel;
  const query = ui.palette.query.trim();
  const empty = query ? 'Nothing matches “' + query + '”.' : 'There\'s nothing to choose from yet.';
  return renderList(items, sel, empty);
}

// fillPalette puts the text typed so far into a newly drawn search or
// address field.
export function fillPalette(ui) {
  const query = document.getElementById('palette-query');
  if (query) query.value = ui.palette.query;
  const url = document.getElementById('palette-url');
  if (url && ui.add) url.value = ui.add.url;
}

// syncSelection points the search field at the selected row.
export function syncSelection(ui) {
  const query = document.getElementById('palette-query');
  if (!query) return;
  const id = ui.listItems.length ? 'palette-item-' + ui.palette.sel : '';
  if (query.getAttribute('aria-activedescendant') !== id) query.setAttribute('aria-activedescendant', id);
}

function renderURLStep(ui) {
  ui.listItems = [];
  const o = ui.add.option;
  const title = 'Add a web page to this pane';
  return '<div class="dialog palette" role="dialog" aria-modal="true" aria-label="' + esc(title) + '">'
    + '<div class="palette-search">' + ICONS.globe
    + '<label for="palette-url" class="visually-hidden">Web page address</label>'
    + '<input id="palette-url" data-key="palette-url" type="text" inputmode="url" autocomplete="off" spellcheck="false"'
    + ' aria-describedby="palette-url-hint" placeholder="Web page address, such as example.com">'
    + '<button type="button" class="esc" data-action="add-back" data-key="palette-back" aria-label="Back to the list">esc</button></div>'
    + '<div class="palette-list" id="palette-list">'
    + '<div class="palette-group" role="presentation">' + esc(o.label || 'Web page…') + '</div>'
    + '<p class="palette-note" id="palette-url-hint">' + esc(sentence(o.hint || 'Opens in a Bench web view')) + ' Bench adds https:// if you leave it out.</p></div>'
    + '<div class="palette-foot"><span><kbd>↵</kbd> add</span><span><kbd>esc</kbd> back</span>'
    + '<span>Opens in this pane</span></div></div>';
}
