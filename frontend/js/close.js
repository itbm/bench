// Closing tabs and sub-tabs: Close Tab… (Go's CloseTab), which closes a
// tab's windows, pages, and terminals and deletes it, or on the only tab,
// Close All Windows…, which leaves it empty; and Close Window, Close Page,
// or Close Terminal (CloseItem), from the tab and window menus, the × on
// tabs and sub-tabs, a middle-click on either, the jump palette, and the
// Tab and Pane menus. On a sub-tab that's in other tabs too, the × and a
// middle-click take it out of this tab only, as Remove from This Tab does,
// and the window menu offers Close Everywhere. Menu entries have title
// case; the tooltips of the ×s are sentences, in sentence case.
//
// Go asks first where it would (confirm_close for a tab, and
// confirm_quit_busy_terminals for a terminal that's running something): it
// closes nothing, and says what it would close, which the question here
// shows. Its Close calls again, sure, with Don't ask again if ticked. A
// close that Go doesn't ask about leaves an open dialog as it is; the
// question takes the dialog's place, after asking if it has changes that
// aren't saved (afterAsking).

import { act, closeOverlay, openerKey, store, update } from './state.js';
import { itemOf, tabsOf } from './util.js';
import { renderQuestion } from './quit.js';
import { afterAsking } from './discard.js';

// closeLabel names what Close does to an item, in a menu: Close Window,
// Close Page, or Close Terminal, as the menu bar's Pane menu does.
export function closeLabel(item) {
  if (item && item.kind === 'builtin') return 'Close Terminal';
  if (item && item.kind === 'web') return 'Close Page';
  return 'Close Window';
}

// closeTabLabel names Close Tab…, which reads Close All Windows… on the
// only tab, as closing it leaves the tab, empty.
export function closeTabLabel(view) {
  return (view.tabs || []).length === 1 ? 'Close All Windows…' : 'Close Tab…';
}

// closeTabTip is the tooltip of a tab's ×: what closeTabLabel does, in
// sentence case.
export function closeTabTip(view) {
  return (view.tabs || []).length === 1 ? 'Close all windows' : 'Close tab';
}

// closeTab closes tab index, or asks first, as Go says.
export async function closeTab(index) {
  const tab = (store.view.tabs || [])[index];
  if (!tab) return;
  const r = await act('CloseTab', index, false, false);
  if (r.ok && r.value && r.value.ask) afterAsking(() => openClose({ kind: 'tab', tabName: tab.name, q: r.value }));
}

// closeItem closes an item, from every tab it's in, or asks first, as Go
// says.
export async function closeItem(itemId) {
  const r = await act('CloseItem', itemId, false, false);
  if (r.ok && r.value && r.value.ask) afterAsking(() => openClose({ kind: 'item', itemId, q: r.value }));
}

// quickClose is the × on a sub-tab, and a middle-click on it: an item
// that's in other tabs too leaves this tab only, as Remove from This Tab
// does; any other closes.
export function quickClose(itemId) {
  const view = store.view;
  const item = itemOf(view, itemId);
  if (!item) return;
  if (tabsOf(item).length > 1) act('RemoveItemFromTab', itemId, view.current);
  else closeItem(itemId);
}

// quickCloseLabel is the tooltip of a sub-tab's ×. On a sub-tab with no
// window, which only remembers a place, Close forgets it, as Remove from
// Bench in its menu does.
export function quickCloseLabel(item) {
  if (tabsOf(item).length > 1) return 'Remove from this tab';
  if (item && item.noWindow) return 'Remove from Bench';
  if (item && item.kind === 'builtin') return 'Close terminal';
  if (item && item.kind === 'web') return 'Close page';
  return 'Close window';
}

// openClose shows Go's question, c: { kind: 'tab', tabName, q } or
// { kind: 'item', itemId, q }, where q is the CloseQuestion. A question that
// closes other apps' windows has no default button (renderQuestion): the
// dialog has the focus, rather than Close.
function openClose(c) {
  const { ui } = store;
  if (!ui.overlay) ui.opener = openerKey();
  ui.menu = null;
  ui.paneLater = null;
  // busy is set while the question's Close is on its way, and details
  // while the question shows its details.
  ui.close = { ...c, dontAsk: false, busy: false, details: false };
  ui.overlay = 'close';
  ui.pendingFocus = noDefault(c) ? 'question' : 'question-run';
  update();
}

// noDefault reports whether question c closes other apps' windows, which
// may have unsaved work, so that Return mustn't press its Close.
const noDefault = (c) => c.kind === 'tab' && Boolean(c.q && c.q.windows);

// toggleCloseDetails shows or hides the question's details.
export function toggleCloseDetails() {
  const c = store.ui.close;
  if (!c || store.ui.overlay !== 'close') return;
  c.details = !c.details;
  update();
}

// confirmClose is the question's Close: Go closes, without asking again,
// and with Don't ask again ticked, turns the question off. A tab is found
// by its name, which is unique, in case tabs moved meanwhile. The question
// stays until Go answers, so that a double click closes once.
export async function confirmClose() {
  const { ui } = store;
  const c = ui.close;
  if (!c || c.busy) return;
  let call;
  if (c.kind === 'tab') {
    const index = (store.view.tabs || []).findIndex((t) => t.name === c.tabName);
    if (index < 0) {
      closeOverlay();
      return;
    }
    call = ['CloseTab', index, true, c.dontAsk];
  } else {
    call = ['CloseItem', c.itemId, true, c.dontAsk];
  }
  c.busy = true;
  update();
  await act(...call);
  c.busy = false;
  if (ui.close !== c || ui.overlay !== 'close') return;
  closeOverlay();
}

// count says how many of a thing there are, as "2 windows", or "" for none.
function count(n, what) {
  if (!n) return '';
  return n + ' ' + what + (n === 1 ? '' : 's');
}

// list joins phrases: "a", "a and b", or "a, b, and c".
function list(parts) {
  if (parts.length < 3) return parts.join(' and ');
  return parts.slice(0, -1).join(', ') + ', and ' + parts[parts.length - 1];
}

// busyText names the terminals that are running something, which closing
// ends.
function busyText(busy) {
  const names = busy.map((b) => '“' + (b.running || b.title || 'a program') + '”');
  if (busy.length === 1) return names[0] + ' is still running in a terminal, and closing ends it.';
  return busy.length + ' terminals are still running something (' + list(names) + '), and closing ends them.';
}

// closeTabTitle is the question before closing a tab, in one sentence,
// with what closes: "Close Plan and its 2 windows?", or on the only tab,
// which stays, "Close the 2 windows in Plan?".
export function closeTabTitle(name, q) {
  const what = list([count(q.windows, 'window'), count(q.pages, 'page'), count(q.terminals, 'terminal')].filter(Boolean));
  if (q.only) return what ? 'Close the ' + what + ' in ' + name + '?' : 'Close everything in ' + name + '?';
  return what ? 'Close ' + name + ' and its ' + what + '?' : 'Close ' + name + '?';
}

// closeTabText is what the question before closing a tab always says: the
// terminals that are running something, which closing ends, if any.
export function closeTabText(q) {
  return (q.busy || []).length ? busyText(q.busy) : '';
}

// closeTabDetails is what the question's Details show: what stays, what
// won't come back, even with Undo, and that apps can still ask about
// unsaved changes.
export function closeTabDetails(q) {
  const parts = [];
  if (q.stay === 1) parts.push('The one that\'s also in another tab stays there.');
  else if (q.stay > 1) parts.push('The ' + q.stay + ' that are also in other tabs stay there.');
  if (q.windows) parts.push('Apps can still ask about unsaved changes.');
  if (q.only) {
    if (q.listed) parts.push('What config.toml lists for this tab won\'t open again when Bench next opens; Reset Layout brings it back.');
  } else {
    const rules = q.rules ? ', with the ' + (q.rules === 1 ? 'rule that names it' : q.rules + ' rules that name it') : '';
    parts.push('The tab goes from config.toml' + rules + ', so it won\'t come back when Bench next opens.');
  }
  const kinds = [[q.windows, 'window'], [q.pages, 'page'], [q.terminals, 'terminal']].filter(([n]) => n);
  if (kinds.length) {
    const total = kinds.reduce((n, [k]) => n + k, 0);
    const names = list(kinds.map(([n, what]) => 'the ' + what + (n === 1 ? '' : 's')));
    parts.push('Undo brings back the ' + (q.only ? 'layout' : 'tab') + ', but not ' + names + ' that ' + (total === 1 ? 'closes' : 'close') + '.');
  }
  return parts.join(' ');
}

// closeItemText is the question before closing a terminal that's running
// something.
export function closeItemText(q) {
  const t = (q.busy || [])[0] || {};
  const what = t.running || t.title || 'A program';
  return '“' + what + '” is still running in ' + (t.title && t.title !== what ? '“' + t.title + '”' : 'this terminal') + '. Close the terminal and end it?';
}

// renderClose returns the question before closing.
export function renderClose(view, ui) {
  const c = ui.close;
  if (!c) return '';
  const q = c.q || {};
  const common = { action: 'close-run', dontAsk: c.dontAsk, busy: c.busy, movable: true };
  if (c.kind === 'tab') {
    return renderQuestion({
      ...common, title: closeTabTitle(c.tabName, q), text: closeTabText(q), run: q.only ? 'Close all' : 'Close tab',
      details: closeTabDetails(q), detailsOpen: c.details, noDefault: noDefault(c),
    });
  }
  return renderQuestion({ ...common, title: 'Close terminal?', text: closeItemText(q), run: 'Close' });
}
