// The jump palette and the add dialog. They share their markup and keys: a
// search field, grouped rows, arrows to move, Return to choose, and Escape
// or the backdrop to close. The add dialog's Web page… asks for an address,
// and may then wait while a browser makes a web app of the site. A web
// page's menu opens the add dialog at that step too, to put the web app in
// the page's place.

import { call, errorText } from './bridge.js';
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
  ui.add = {
    paneId, tab, options: Array.isArray(r.value) ? r.value : [], step: 'list', option: null, url: '', make: null, busy: false,
  };
  ui.palette = { query: '', sel: 0 };
  ui.pendingFocus = 'palette-query';
  update();
}

// makeWebApp has Go open a web page's site in the default browser, for the
// user to make a web app of it there (MakeWebApp), and opens the add
// dialog's make step for it, which waits for the web app and puts it in the
// page's place. If Go refuses, it says why, and nothing opens. The dialog
// doesn't open if the page has gone or Go has switched tabs meanwhile.
export async function makeWebApp(itemId, paneId) {
  const opener = activeKey();
  const tab = store.view.current;
  const item = itemOf(store.view, itemId);
  const page = item ? item.app : 'the page';
  const r = await act('MakeWebApp', itemId);
  if (!r.ok) return;
  const { ui, view } = store;
  const now = itemOf(view, itemId);
  if (view.current !== tab || !now || now.tab !== tab) return;
  const result = r.value || {};
  if (!ui.overlay) ui.opener = opener;
  ui.menu = null;
  ui.editor = null;
  ui.overlay = 'add';
  ui.add = {
    paneId, tab, options: [], step: 'make', option: null, url: '', busy: false,
    make: { url: '', browser: result.browser || 'your browser', steps: result.steps || '' },
    replace: { itemId, page },
  };
  ui.palette = { query: '', sel: 0 };
  ui.pendingFocus = 'make-cancel';
  update();
  waitForWebApp(ui.add, ui.add.make);
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
  ui.add.make = null;
  ui.pendingFocus = 'palette-url';
  update();
}

// backToList leaves the add dialog's address step, or stops waiting for a
// web app, and goes back to the list. If the web app is added just then,
// the dialog closes instead (see settlePoll). A dialog that a web page's
// menu opened has no list: it closes, and the page stays.
export async function backToList() {
  const { ui } = store;
  const add = ui.add;
  if (!add) return;
  if (add.step === 'make') {
    if (add.busy) return;
    add.busy = true;
    const added = await settlePoll(add);
    add.busy = false;
    if (added || ui.add !== add) return;
  }
  if (add.replace) {
    closeOverlay();
    return;
  }
  add.step = 'list';
  add.make = null;
  ui.pendingFocus = 'palette-query';
  update();
}

// settlePoll waits for the make step's call to AddWebAppWhenReady (or
// UseWebAppWhenReady), if one is under way, and reports whether it added
// the web app, which waitForWebApp then says, closing the dialog. Whoever
// calls it sets add.busy first, so that no other call starts meanwhile, and
// a choice to stop waiting can't add a second item, or leave the step as
// the web app is added.
async function settlePoll(add) {
  const make = add.step === 'make' ? add.make : null;
  return Boolean(make && make.poll && await make.poll);
}

// POLL_MS is how often the make step asks Go whether the web app is ready.
const POLL_MS = 1500;

// typedURL returns the address the add dialog adds, or '' after asking for
// one: the one the make step waits for, or else the one typed.
function typedURL(add) {
  const url = (add.step === 'make' && add.make ? add.make.url : add.url).trim();
  if (!url) toast('Type the address of a web page.');
  return url;
}

// submitURL adds the website at the typed address. With inBench (Option-
// Return), it's a web page that Bench shows itself, as showInBench adds.
// Otherwise Go adds the site's web app if there's one, or else asks the
// default browser to make one, and the dialog waits for it (the make step),
// or, if Bench can't dock web apps or the browser can't make them, adds a
// web page that Bench shows (AddWebsite). The dialog stays open if Go
// refuses the address, so that it can be fixed.
export async function submitURL(inBench = false) {
  const { ui } = store;
  const add = ui.add;
  if (!add || !add.option || add.step !== 'url' || add.busy) return;
  if (inBench) {
    await showInBench();
    return;
  }
  const url = typedURL(add);
  if (!url) return;
  add.busy = true;
  const r = await act('AddWebsite', add.paneId, url);
  add.busy = false;
  if (!r.ok) return;
  const result = r.value || {};
  if (result.added) {
    // Adding focuses the pane. The note says what was added, even if the
    // dialog has closed meanwhile.
    if (ui.add === add) {
      ui.paneLater = null;
      closeOverlay();
    }
    toast(result.note);
    return;
  }
  if (ui.add !== add || add.step !== 'url') return;
  add.step = 'make';
  add.make = { url, browser: result.browser || 'your browser', steps: result.steps || '' };
  ui.pendingFocus = 'make-cancel';
  update();
  waitForWebApp(add, add.make);
}

// showInBench adds the address as a web page that Bench shows itself, for
// Option-Return and the Show in Bench buttons.
export async function showInBench() {
  const { ui } = store;
  const add = ui.add;
  if (!add || !add.option || add.step === 'list' || add.busy) return;
  const url = typedURL(add);
  if (!url) return;
  add.busy = true;
  if (await settlePoll(add)) return;
  const r = await act('AddToPane', add.paneId, { ...add.option.source, url });
  add.busy = false;
  if (r.ok && ui.add === add) {
    ui.paneLater = null;
    closeOverlay();
  }
}

// waitForWebApp asks Go every POLL_MS, one call at a time, whether the web
// app that the make step waits for is installed, which adds it to the pane
// (AddWebAppWhenReady), or puts it in the web page's place
// (UseWebAppWhenReady), until it is, or the dialog leaves the step. The
// user is in the browser meanwhile, so the dialog stays open when Bench
// isn't the active app. It doesn't ask while the user's choice to stop
// waiting is under way (add.busy), and that choice waits for the call
// under way, make.poll (see settlePoll). A refusal shows once, not after
// every try. The dialog closes if the web page it replaces goes (see
// dropStale in render.js).
async function waitForWebApp(add, make) {
  const waiting = () => store.ui.add === add && add.step === 'make' && add.make === make;
  const ask = () => (add.replace
    ? call('UseWebAppWhenReady', add.replace.itemId)
    : call('AddWebAppWhenReady', add.paneId, make.url));
  let refused = '';
  while (waiting()) {
    await new Promise((r) => setTimeout(r, POLL_MS));
    if (!waiting()) return;
    if (add.busy) continue;
    make.poll = ask().then(Boolean, (err) => {
      const text = errorText(err);
      if (text !== refused) toast(text);
      refused = text;
      return false;
    });
    const ready = await make.poll;
    make.poll = null;
    if (!ready) continue;
    // A web app that's ready as the dialog closes is still added. Go may
    // have added one that another browser made, so the toast names none.
    if (waiting()) {
      store.ui.paneLater = null;
      closeOverlay();
    }
    toast(add.replace ? 'Replaced ' + add.replace.page + ' with its web app.' : 'Added the web app.');
    return;
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
  return ui.overlay === 'palette' || (ui.overlay === 'add' && Boolean(ui.add) && ui.add.step === 'list');
}

// renderPalette returns the jump palette's or add dialog's HTML, without
// its rows (renderRows) or the search field's text (fillPalette), so that
// neither typing nor a new View replaces the search field while it's in
// use.
export function renderPalette(view, ui) {
  const kind = ui.overlay;
  if (kind === 'add' && ui.add && ui.add.step === 'url') return renderURLStep(ui);
  if (kind === 'add' && ui.add && ui.add.step === 'make' && ui.add.make) return renderMakeStep(ui);
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

// urlNote says what Return and Option-Return do with the typed address: for
// a site with no web app, a browser makes one if the row names one
// (AddOption.webAppBrowser), and otherwise Bench shows the page itself.
function urlNote(option) {
  const browser = option.webAppBrowser || '';
  const note = browser
    ? 'Return adds your web app for this site, or opens the site in ' + browser + ' so that you can make one. Option-Return shows the page in Bench instead.'
    : 'Return adds your web app for this site if you have one, or else shows the page in Bench, as Option-Return always does.';
  return note + ' Bench adds https:// if you leave it out.';
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
    + '<p class="palette-note" id="palette-url-hint">' + esc(urlNote(o)) + '</p>'
    + '<div class="palette-actions">'
    + '<button type="button" class="btn small" data-action="add-in-bench" data-key="palette-in-bench">Show in Bench</button>'
    + '<button type="button" class="btn small primary" data-action="add-website" data-key="palette-add">Add</button></div></div>'
    + '<div class="palette-foot"><span><kbd>↵</kbd> add</span><span><kbd>⌥↵</kbd> show in Bench</span><span><kbd>esc</kbd> back</span>'
    + '<span>Opens in this pane</span></div></div>';
}

// renderMakeStep shows the steps that make a web app in the browser that
// AddWebsite or MakeWebApp opened the site in, while the dialog waits for
// it (waitForWebApp). For a web page's web app, which takes the page's
// place, there's nothing to show in Bench instead: the page is there.
function renderMakeStep(ui) {
  ui.listItems = [];
  const { make: m, replace } = ui.add;
  const title = 'Make a web app in ' + m.browser;
  const site = replace ? replace.page : m.url;
  const steps = 'Bench has opened ' + site + ' in ' + m.browser + '.' + (m.steps ? ' ' + sentence(m.steps) : '');
  const note = replace
    ? 'Bench puts the web app in place of this page as soon as it\'s ready.'
    : 'Bench adds the web app to this pane as soon as it\'s ready.';
  return '<div class="dialog palette" role="dialog" aria-modal="true" aria-labelledby="palette-make-title" aria-describedby="palette-make-steps">'
    + '<div class="palette-search">' + ICONS.globe
    + '<h2 class="palette-title" id="palette-make-title">' + esc(title) + '</h2>'
    + '<button type="button" class="esc" data-action="add-back" data-key="make-back" aria-label="Cancel">esc</button></div>'
    + '<div class="palette-list" id="palette-list">'
    + '<p class="palette-note" id="palette-make-steps">' + esc(steps) + '</p>'
    + '<p class="palette-note">' + esc(note) + '</p>'
    + '<div class="palette-actions">'
    + (replace ? '' : '<button type="button" class="btn small" data-action="add-in-bench" data-key="make-in-bench">Show in Bench instead</button>')
    + '<button type="button" class="btn small" data-action="add-back" data-key="make-cancel">Cancel</button></div></div>'
    + '<div class="palette-foot"><span><kbd>esc</kbd> cancel</span><span>Waiting for the web app…</span></div></div>';
}
