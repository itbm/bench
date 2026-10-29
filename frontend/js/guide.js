// The welcome guide, and the banner shown while Bench doesn't have
// Accessibility permission. The guide has pages: the permissions, with
// Open at login; the first time Bench opens, a starting set-up, which
// offers a few tabs made of the apps that are installed (StarterTabs);
// then four tips. Help ▸ Show Welcome Guide opens it again.

import { act, closeOverlay, isDemo, openerKey, store, update } from './state.js';
import { esc, ICONS, keysOf } from './util.js';
import { GRIP } from './dialogs.js';

// UPDATED_TEXT is what the guide and the banner say while macOS has stopped
// applying the Accessibility permission that an earlier version of Bench
// had, as Go's notice does.
const UPDATED_TEXT = 'Bench was updated, so macOS needs you to allow it again: select Bench, click −, then turn it on again.';

// newGuide returns the guide's state, at its first page: page is the page
// shown; setup is set while it offers the starting set-up, setupDone once
// the user has added its tabs or none, oneTab while there was only the
// one tab of the first config as the set-up showed, and starters are the
// tabs it suggests, once Go has said, and removed the names of those the
// user removed; busy is set while UseStarterTabs is on its way; login is
// the Open at login tick box as the user left it, while SetOpenAtLogin is
// on its way.
function newGuide(view) {
  const g = {
    page: 'permissions', setup: Boolean(view && view.firstRun), setupDone: false, oneTab: oneTab(view),
    starters: null, removed: [], busy: false, login: null,
  };
  if (g.setup) loadStarters(g);
  return g;
}

// NO_GUIDE stands in for the guide's state if a render finds none.
const NO_GUIDE = { page: 'permissions', setup: false, setupDone: false, oneTab: true, starters: [], removed: [], busy: false, login: null };

// loadStarters asks Go for the tabs to suggest. With none, or if Go can't
// say, the guide leaves the set-up out.
async function loadStarters(g) {
  const r = await act('StarterTabs');
  const tabs = r.ok && Array.isArray(r.value) ? r.value : [];
  g.starters = tabs;
  if (!tabs.length) g.setup = false;
  if (store.ui.guide !== g) return;
  if (g.page === 'setup') {
    if (!g.setup) g.page = 'tips';
    store.ui.pendingFocus = firstFocus(store.view, g);
  }
  update();
}

// pagesOf returns the guide's pages, in order. A set-up that's done still
// counts, so that the steps don't count back, though Back and Continue
// skip it (turnPage).
function pagesOf(g) {
  return g.setup || g.setupDone ? ['permissions', 'setup', 'tips'] : ['permissions', 'tips'];
}

// oneTab reports whether there's only the one tab of the first config, for
// the set-up's Start with one tab. With more, as in a config.toml copied
// from another Mac, it's Don't add tabs, as Bench removes none. The guide
// notes it as it shows the set-up (oneTab), so that the tabs it adds don't
// change the page while they're on their way.
function oneTab(view) {
  return !view || !view.tabs || view.tabs.length <= 1;
}

// kept returns the suggested tabs that the user hasn't removed.
function kept(g) {
  return (g.starters || []).filter((t) => !g.removed.includes(t.name));
}

// firstFocus returns the data-key of the control to focus on the guide's
// page: what to do next there.
function firstFocus(view, g) {
  switch (g.page) {
    case 'setup': return g.starters && kept(g).length ? 'guide-add-tabs' : 'guide-one-tab';
    case 'tips': return 'guide-done';
    default: return view && view.permissions && view.permissions.accessibility ? 'guide-continue' : 'guide-allow';
  }
}

// openGuide opens the welcome guide at its first page.
function openGuide() {
  const { ui } = store;
  if (ui.overlay === 'guide') return;
  if (!ui.overlay) ui.opener = openerKey();
  ui.menu = null;
  ui.paneLater = null;
  ui.add = null;
  ui.editor = null;
  ui.settings = null;
  ui.overlay = 'guide';
  ui.guide = newGuide(store.view);
  ui.pendingFocus = firstFocus(store.view, ui.guide);
  update();
}

// showGuide opens the guide again, for Help ▸ Show Welcome Guide and Jump
// to…, even if it was closed.
export function showGuide() {
  store.ui.guideClosed = false;
  openGuide();
}

// maybeOpenGuide opens the guide when Go says it's the first run, unless
// it's been closed already.
export function maybeOpenGuide(view) {
  const { ui } = store;
  if (!view.firstRun || ui.guideClosed || ui.overlay || ui.drag) return;
  ui.opener = openerKey();
  ui.menu = null;
  ui.paneLater = null;
  ui.overlay = 'guide';
  ui.guide = newGuide(view);
  ui.pendingFocus = firstFocus(view, ui.guide);
}

// dismissGuide closes the guide for good.
export function dismissGuide() {
  store.ui.guideClosed = true;
  store.ui.guide = null;
  closeOverlay();
  act('DismissFirstRun');
}

// turnPage shows the next page of the guide, or with step -1 the one
// before.
export function turnPage(step) {
  const g = store.ui.guide;
  if (!g || g.busy) return;
  const pages = pagesOf(g);
  let at = pages.indexOf(g.page) + step;
  if (pages[at] === 'setup' && g.setupDone) at += step;
  const next = pages[at];
  if (!next) return;
  if (next === 'setup') g.oneTab = oneTab(store.view);
  g.page = next;
  store.ui.pendingFocus = firstFocus(store.view, g);
  update();
}

// removeStarter takes a suggested tab out of the set-up, or with name ''
// puts them all back.
export function removeStarter(name) {
  const g = store.ui.guide;
  if (!g || g.busy) return;
  g.removed = name ? [...g.removed, name] : [];
  store.ui.pendingFocus = firstFocus(store.view, g);
  update();
}

// useStarters is the set-up's Add: Go adds the tabs that the user kept,
// after the tabs there are, and the guide goes on to its tips. Start with
// one tab, or Don't add tabs (none), goes on without adding any. Either way
// the set-up is done, and Back skips it.
export async function useStarters(none) {
  const g = store.ui.guide;
  if (!g || g.busy) return;
  const names = none ? [] : kept(g).map((t) => t.name);
  if (names.length) {
    g.busy = true;
    update();
    const r = await act('UseStarterTabs', names);
    g.busy = false;
    if (!r.ok) {
      if (store.ui.guide === g) update();
      return;
    }
  }
  if (store.ui.guide !== g) return;
  g.setup = false;
  g.setupDone = true;
  g.page = 'tips';
  store.ui.pendingFocus = firstFocus(store.view, g);
  update();
}

// setGuideLogin is the guide's Open Bench when you log in. Go turns Open
// at login on or off, and if macOS needs the user to allow it, says so.
export async function setGuideLogin(on) {
  const g = store.ui.guide;
  if (!g) return;
  g.login = on;
  await act('SetOpenAtLogin', on);
  if (store.ui.guide !== g) return;
  g.login = null;
  update();
}

// stepHTML returns one numbered step of a page.
function stepHTML(n, title, body) {
  return '<div class="guide-step"><span class="mono-tile" aria-hidden="true">' + n + '</span><div>'
    + '<h3>' + esc(title) + '</h3>' + body + '</div></div>';
}

// okHTML says that Bench has something, with a tick.
function okHTML(text) {
  return '<span class="guide-ok">' + ICONS.check + esc(text) + '</span>';
}

// permissionsPage returns the first page: Accessibility, Screen Recording,
// and Open at login.
function permissionsPage(view, g) {
  const p = view.permissions || {};
  const allow = p.accessibility
    ? okHTML('Bench has Accessibility permission.')
    : '<button type="button" class="btn primary" data-action="request-accessibility" data-key="guide-allow">Turn on Accessibility</button>';
  let how = '';
  if (!p.accessibility) {
    how = p.accessibilityUpdated
      ? '<p class="guide-warn">' + esc(UPDATED_TEXT) + '</p>'
      : '<p>' + esc('The button opens System Settings at Privacy & Security ▸ Accessibility. Turn on Bench there.') + '</p>';
  }
  let recording;
  if (p.screenRecordingReopen) {
    recording = '<p>' + esc('macOS applies Screen Recording when Bench opens again.') + '</p>'
      + '<div class="guide-action"><button type="button" class="btn" data-action="quit-and-reopen" data-key="guide-reopen">Quit and Reopen Bench</button></div>';
  } else if (p.screenRecording) {
    recording = '<div class="guide-action">' + okHTML('Bench has Screen Recording permission.') + '</div>';
  } else {
    recording = '<div class="guide-action"><button type="button" class="btn" data-action="request-screen-recording" data-key="guide-recording">Turn on Screen Recording</button></div>';
  }
  const login = g.login === null ? Boolean(view.openAtLogin) : g.login;
  const approval = view.loginApproval
    ? '<p class="guide-warn" id="guide-login-note">' + esc('macOS needs you to allow Bench in System Settings ▸ General ▸ Login Items.') + '</p>'
      + '<div class="guide-action"><button type="button" class="btn" data-action="open-login-items" data-key="guide-login-items">Open Login Items</button></div>'
    : '';
  return stepHTML(1, 'Accessibility',
    '<p>' + esc('Bench needs Accessibility permission to arrange other apps\' windows: to move and size them into its panes, and to hide the ones that belong to other tabs.') + '</p>'
    + how + '<div class="guide-action">' + allow + '</div>')
    + stepHTML(2, 'Screen Recording (optional)',
      '<p>' + esc('Screen Recording is only for live previews of windows when you hover a sub-tab. Previews are off until you turn them on in Settings.') + '</p>'
      + recording)
    + stepHTML(3, 'Open at login',
      '<p>' + esc('Bench can open when you log in, so that your tabs are ready. Settings can change this later.') + '</p>'
      + '<div class="guide-action"><label class="toggle"><input type="checkbox" data-guide="open-at-login" data-key="guide-login"'
      + (login ? ' checked' : '') + (approval ? ' aria-describedby="guide-login-note"' : '') + '><span>Open Bench when you log in</span></label></div>'
      + approval);
}

// setupPage returns the starting set-up: the suggested tabs, each with its
// apps and Remove.
function setupPage(g) {
  if (!g.starters) return '<p class="guide-loading">Looking for your apps…</p>';
  const tabs = kept(g);
  if (!tabs.length) {
    const skip = g.oneTab ? 'Start with one tab' : 'Go on with the tabs you have';
    return '<p class="guide-loading">' + esc('No tabs to add. ' + skip + ', or put the suggestions back.') + '</p>'
      + '<div class="guide-action guide-loading"><button type="button" class="btn" data-action="guide-restore" data-key="guide-restore">Put them back</button></div>';
  }
  const off = g.busy ? ' disabled' : '';
  return '<div class="starters">' + tabs.map((t) => '<div class="starter" role="group" aria-label="' + esc(t.name + ' tab') + '">'
    + '<span class="starter-name">' + esc(t.name) + '</span>'
    + '<span class="starter-apps">' + (t.apps || []).map((a) => '<span class="starter-app"><span class="mono-tile" aria-hidden="true">'
      + esc(a.mono) + '</span>' + esc(a.name) + '</span>').join('') + '</span>'
    + '<button type="button" class="btn small" data-action="guide-remove" data-name="' + esc(t.name) + '" data-key="starter-remove-' + esc(t.name) + '"'
    + ' aria-label="' + esc('Remove the ' + t.name + ' tab') + '"' + off + '>Remove</button></div>').join('') + '</div>';
}

// tipsPage returns the four tips, with the shortcuts as configured.
function tipsPage(view) {
  const keys = keysOf(view);
  const tabs = keys.tabs
    ? 'Each tab is a set of windows side by side. Click a tab, or press ' + keys.tabs + '1 to ' + keys.tabs + '9 in any app, to switch to it.'
    : 'Each tab is a set of windows side by side. Click a tab to switch to it.';
  const jump = (keys.jump
    ? 'Press ' + keys.jump + ' in any app to jump to a tab or a window, or to find something to do, such as Settings.'
    : 'Click Jump to… beside the tabs to jump to a tab or a window, or to find something to do, such as Settings.')
    + ' Type words from its name in any order. With nothing typed, Return goes back to the tab you were on.';
  return stepHTML(1, 'Tabs', '<p>' + esc(tabs) + '</p>')
    + stepHTML(2, 'Sub-tabs and dragging', '<p>' + esc('A pane shows one window at a time, with a sub-tab for each, and its + adds another. '
      + 'Drag a sub-tab to another pane, to a pane\'s edge to split it, or onto a tab to move the window there. '
      + 'Hold Option as you drop it on a tab to show the window in both tabs.') + '</p>')
    + stepHTML(3, 'Jump to…', '<p>' + esc(jump) + '</p>')
    + stepHTML(4, 'Right-click menus', '<p>' + esc('Right-click a tab, a sub-tab, or a pane\'s header to see what you can do with it. '
      + 'Help ▸ Keyboard Shortcuts lists every shortcut.') + '</p>');
}

// HEADS are each page's title and introduction. The set-up has two: with
// more than one tab, it doesn't offer to start with one (g.oneTab).
const HEADS = {
  permissions: ['Welcome to Bench', 'Bench gathers your apps into one window with tabs. Before you start, it needs one permission from macOS.'],
  setup: ['Choose a starting set-up', 'Bench can add a few tabs made of apps you have. Remove any you don\'t want, or start with one tab and make your own.'],
  setupMore: ['Choose a starting set-up', 'Bench can add a few tabs made of apps you have, after your tabs. Remove any you don\'t want, or don\'t add any.'],
  tips: ['A few tips', 'Four things worth knowing. Help ▸ Show Welcome Guide shows them again.'],
};

// footHTML returns a page's buttons: Back, and what goes on.
function footHTML(view, g) {
  const pages = pagesOf(g);
  const at = pages.indexOf(g.page);
  const off = g.busy ? ' disabled' : '';
  const back = at > 0 ? '<button type="button" class="btn" data-action="guide-back" data-key="guide-back"' + off + '>Back</button>' : '';
  let next;
  if (g.page === 'setup') {
    const n = kept(g).length;
    const skip = g.oneTab ? 'Start with one tab' : 'Don\'t add tabs';
    next = '<button type="button" class="btn" data-action="guide-one-tab" data-key="guide-one-tab"' + off + '>' + skip + '</button>'
      + '<button type="button" class="btn primary" data-action="guide-add-tabs" data-key="guide-add-tabs"' + (n && !g.busy ? '' : ' disabled') + '>'
      + esc(n === 1 ? 'Add 1 tab' : 'Add ' + n + ' tabs') + '</button>';
  } else if (g.page === 'tips') {
    next = '<button type="button" class="btn primary" data-action="dismiss-first-run" data-key="guide-done">Done</button>';
  } else {
    const ready = view.permissions && view.permissions.accessibility;
    next = '<button type="button" class="btn' + (ready ? ' primary' : '') + '" data-action="guide-next" data-key="guide-continue" data-focus-first>Continue</button>';
  }
  return '<div class="editor-foot"><span class="guide-count">' + esc('Step ' + (at + 1) + ' of ' + pages.length) + '</span>'
    + '<div class="spacer">' + back + next + '</div></div>';
}

// renderGuide returns the welcome guide's HTML.
export function renderGuide(view) {
  const g = store.ui.guide || NO_GUIDE;
  const [title, intro] = HEADS[g.page === 'setup' && !g.oneTab ? 'setupMore' : g.page];
  let body;
  if (g.page === 'setup') body = setupPage(g);
  else if (g.page === 'tips') body = tipsPage(view);
  else body = permissionsPage(view, g);
  // The heading and the buttons stay in view, and the steps between them
  // scroll if the window is short.
  return '<div class="dialog guide-sheet" role="dialog" aria-modal="true" aria-labelledby="guide-title" aria-describedby="guide-intro" data-page="' + g.page + '">'
    + '<div class="guide-head dialog-handle"><h2 id="guide-title">' + esc(title) + '</h2>'
    + '<p id="guide-intro">' + esc(intro) + '</p></div>'
    + '<div class="dialog-body" data-scroll="guide-' + g.page + '">' + body + '</div>'
    + footHTML(view, g) + GRIP + '</div>';
}

// renderBanner returns the Accessibility banner's HTML, or '' if it isn't
// needed. After an update, it says what macOS needs the user to do.
export function renderBanner(view) {
  if (isDemo() || !view.permissions || view.permissions.accessibility) return '';
  const text = view.permissions.accessibilityUpdated
    ? UPDATED_TEXT
    : 'Bench can\'t arrange other apps\' windows until it has Accessibility permission.';
  return '<span class="banner-text">' + esc(text) + '</span>'
    + '<button type="button" class="btn small" data-action="request-accessibility" data-key="banner-allow">Turn on Accessibility</button>';
}

// requestAccessibility asks macOS for Accessibility permission. Go also
// opens System Settings at Accessibility's list while Bench doesn't have
// it.
export function requestAccessibility() {
  act('RequestAccessibility');
}
