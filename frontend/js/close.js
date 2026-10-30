// Closing tabs and sub-tabs: Close tab… (Go's CloseTab), which closes a
// tab's windows, pages, and terminals and deletes it, or on the only tab,
// Close all windows…, which leaves it empty; and Close window, Close page,
// or Close terminal (CloseItem), from the tab and window menus, the × on
// tabs and sub-tabs, a middle-click on either, the jump palette, and the
// Tab and Pane menus. On a sub-tab that's in other tabs too, the × and a
// middle-click take it out of this tab only, as Remove from this tab does,
// and the window menu offers Close everywhere.
//
// Go asks first where it would (confirm_close for a tab, and
// confirm_quit_busy_terminals for a terminal that's running something): it
// closes nothing, and says what it would close, which the question here
// shows. Its Close calls again, sure, with Don't ask again if ticked.

import { act, closeOverlay, openerKey, store, update } from './state.js';
import { itemOf, tabsOf } from './util.js';
import { renderQuestion } from './quit.js';

// closeLabel names what Close does to an item: Close window, Close page,
// or Close terminal.
export function closeLabel(item) {
  if (item && item.kind === 'builtin') return 'Close terminal';
  if (item && item.kind === 'web') return 'Close page';
  return 'Close window';
}

// closeTabLabel names Close tab…, which reads Close all windows… on the
// only tab, as closing it leaves the tab, empty.
export function closeTabLabel(view) {
  return (view.tabs || []).length === 1 ? 'Close all windows…' : 'Close tab…';
}

// closeTab closes tab index, or asks first, as Go says.
export async function closeTab(index) {
  const tab = (store.view.tabs || [])[index];
  if (!tab) return;
  const r = await act('CloseTab', index, false, false);
  if (r.ok && r.value && r.value.ask) openClose({ kind: 'tab', tabName: tab.name, q: r.value });
}

// closeItem closes an item, from every tab it's in, or asks first, as Go
// says.
export async function closeItem(itemId) {
  const r = await act('CloseItem', itemId, false, false);
  if (r.ok && r.value && r.value.ask) openClose({ kind: 'item', itemId, q: r.value });
}

// quickClose is the × on a sub-tab, and a middle-click on it: an item
// that's in other tabs too leaves this tab only, as Remove from this tab
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
  return item && item.noWindow ? 'Remove from Bench' : closeLabel(item);
}

// openClose shows Go's question, c: { kind: 'tab', tabName, q } or
// { kind: 'item', itemId, q }, where q is the CloseQuestion.
function openClose(c) {
  const { ui } = store;
  if (!ui.overlay) ui.opener = openerKey();
  ui.menu = null;
  ui.paneLater = null;
  // busy is set while the question's Close is on its way.
  ui.close = { ...c, dontAsk: false, busy: false };
  ui.overlay = 'close';
  ui.pendingFocus = 'question-run';
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

// closeTabText is the question before closing a tab: what closes, what
// stays, and what won't come back, even with Undo.
export function closeTabText(q) {
  const parts = [];
  const what = [count(q.windows, 'window'), count(q.pages, 'page'), count(q.terminals, 'terminal')].filter(Boolean);
  if (what.length) parts.push('This closes ' + list(what) + '.');
  if ((q.busy || []).length) parts.push(busyText(q.busy));
  if (q.stay === 1) parts.push('The one that\'s also in another tab stays there.');
  else if (q.stay > 1) parts.push('The ' + q.stay + ' that are also in other tabs stay there.');
  if (q.windows) parts.push('Apps can still ask about unsaved changes.');
  if (q.only) {
    if (q.listed) parts.push('What config.toml lists for this tab won\'t open again when Bench next opens; Reset layout brings it back.');
  } else {
    const rules = q.rules ? ', with the ' + (q.rules === 1 ? 'rule that names it' : q.rules + ' rules that name it') : '';
    parts.push('The tab goes from config.toml' + rules + ', so it won\'t come back when Bench next opens.');
  }
  if (what.length) parts.push('Undo brings back the ' + (q.only ? 'layout' : 'tab') + ', but not what closes.');
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
  let title = 'Close terminal?';
  let text = closeItemText(q);
  let run = 'Close';
  if (c.kind === 'tab') {
    title = q.only ? 'Close all windows in ' + c.tabName + '?' : 'Close ' + c.tabName + '?';
    text = closeTabText(q);
    run = q.only ? 'Close all' : 'Close tab';
  }
  return renderQuestion({ title, text, run, action: 'close-run', dontAsk: c.dontAsk, busy: c.busy, movable: true });
}
