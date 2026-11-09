// The welcome guide, and the banner shown while Bench doesn't have
// Accessibility permission. The guide has pages: the permissions, with
// Open at login; the first time Bench opens, a starting set-up, which
// offers first a tab of the windows that are open, in the New tab dialog
// (editor.js), and then a few tabs made of the apps that are installed
// (StarterTabs); then four tips. Help ▸ Show Welcome Guide opens it again.

import { act, closeOverlay, isDemo, openerKey, store, update } from './state.js';
import { anyColoured, esc, fmt, ICONS, joinNames, keysOf, LAYOUT_MODES, tabDot } from './util.js';
import { GRIP } from './dialogs.js';
import { tile } from './tiles.js';
import { openEditor } from './editor.js';
import { windowChoices } from './fromwindows.js';

// UPDATED_TEXT is what the guide and the banner say while macOS has stopped
// applying the Accessibility permission that an earlier version of Bench
// had, as Go's notice does.
const UPDATED_TEXT = 'Bench was updated, so macOS needs you to allow it again: select Bench, click −, then turn it on again.';

// WIDE_AGAIN_SLACK is how many points wider than compact_below Bench's
// window must be before Auto changes from the compact layout to the wide
// one, as layout.WideAgainSlack in Go is.
const WIDE_AGAIN_SLACK = 40;

// newGuide returns the guide's state, at its first page: page is the page
// shown; setup is set while it offers the starting set-up, setupDone once
// the user has added its tabs or none, oneTab while there was only the
// one tab of the first config as the set-up showed, and starters are the
// tabs it suggests, once Go has said, and removed the names of those the
// user removed; made are the tabs made of open windows from the set-up,
// { name, seen }, where seen is set once the tab is in the View (see
// followMade); busy is set while UseStarterTabs is on its way; login is
// the Open at login tick box as the user left it, while SetOpenAtLogin is
// on its way.
function newGuide(view) {
  const g = {
    page: 'permissions', setup: Boolean(view && view.firstRun), setupDone: false, oneTab: oneTab(view),
    starters: null, removed: [], made: [], busy: false, login: null,
  };
  if (g.setup) loadStarters(g);
  return g;
}

// NO_GUIDE stands in for the guide's state if a render finds none.
const NO_GUIDE = {
  page: 'permissions', setup: false, setupDone: false, oneTab: true, starters: [], removed: [], made: [], busy: false, login: null,
};

// openApps returns the apps of the windows that aren't in a tab, which the
// set-up offers to make a tab of, grouped as the New tab dialog groups
// them.
function openApps(view) {
  return view && view.floating ? windowChoices(view).outside : [];
}

// loadStarters asks Go for the tabs to suggest. With none, or if Go can't
// say, and no windows open to make a tab of, the guide leaves the set-up
// out.
async function loadStarters(g) {
  const r = await act('StarterTabs');
  const tabs = r.ok && Array.isArray(r.value) ? r.value : [];
  g.starters = tabs;
  if (!tabs.length && !openApps(store.view).length && !g.made.length) g.setup = false;
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
    case 'setup':
      if (g.starters && kept(g).length) return 'guide-add-tabs';
      return openApps(view).length ? 'guide-windows' : 'guide-one-tab';
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

// makeFromWindows is the set-up's Choose windows…: the New tab dialog, with
// the windows that aren't in a tab ticked. Its Cancel comes back here, and
// so does Make tab, which the set-up then names, as it asks Go for its
// suggestions again: those of the apps now in a tab go.
export function makeFromWindows() {
  const g = store.ui.guide;
  if (!g || g.busy) return;
  openEditor(-1, {
    start: 'windows',
    back: {
      overlay: 'guide',
      focus: 'guide-windows',
      made: (name) => {
        g.made = g.made.concat({ name, seen: false });
        g.oneTab = false;
        loadStarters(g);
      },
    },
  });
}

// followMade keeps the set-up's tabs made of open windows in step with the
// View. A tab that has gone since, as the message's Undo deletes it, goes
// from the set-up too, which then asks Go for its suggestions again, as
// the tab's apps may be free again. With none left, Start with one tab
// comes back if there's only the one tab. A tab counts as gone only once
// the View has had it, as Make tab can come back before the View does.
export function followMade(view) {
  const g = store.ui.guide;
  if (!g || !g.made.length || !view.tabs) return;
  const has = (name) => view.tabs.some((t) => t.name.toLowerCase() === name.toLowerCase());
  g.made.forEach((m) => {
    if (has(m.name)) m.seen = true;
  });
  const left = g.made.filter((m) => !m.seen || has(m.name));
  if (left.length === g.made.length) return;
  g.made = left;
  if (!left.length) g.oneTab = oneTab(view);
  if (g.setup) loadStarters(g);
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

// madeText says which tabs the set-up has made of open windows.
function madeText(g) {
  const names = g.made.map((m) => m.name);
  return (names.length === 1 ? 'Made the tab ' : 'Made the tabs ') + joinNames(names) + '.';
}

// windowsBlock returns the set-up's first choice: a tab of the windows that
// are open, with their apps, if there are any, or else what the set-up made
// of them.
function windowsBlock(view, g) {
  const apps = openApps(view);
  const made = g.made.length ? okHTML(madeText(g)) : '';
  if (!apps.length) return made ? '<div class="starters"><div class="windows-starter">' + made + '</div></div>' : '';
  const shown = apps.slice(0, 5);
  const more = apps.length > shown.length ? '<span class="starter-more">' + esc('and ' + (apps.length - shown.length) + ' more') + '</span>' : '';
  const off = g.busy ? ' disabled' : '';
  return '<div class="starters"><div class="starter windows-starter" role="group" aria-labelledby="windows-starter-name">'
    + '<div class="windows-starter-text"><span class="starter-name" id="windows-starter-name">' + esc(g.made.length ? 'Another tab of your open windows' : 'A tab of your open windows') + '</span>'
    + '<span class="starter-apps">' + shown.map((a) => '<span class="starter-app">' + tile(a.mono, a.icon) + esc(a.name) + '</span>').join('') + more + '</span>'
    + made + '</div>'
    + '<button type="button" class="btn small" data-action="guide-from-windows" data-key="guide-windows"' + off + '>Choose windows…</button></div></div>';
}

// setupPage returns the starting set-up: a tab of the windows that are
// open, then the suggested tabs, each with its apps and Remove.
function setupPage(view, g) {
  const windows = windowsBlock(view, g);
  const suggested = windows && g.starters && g.starters.length ? '<h3 class="guide-sub">Suggested tabs</h3>' : '';
  return windows + suggested + startersHTML(g);
}

// startersHTML returns the suggested tabs, each with its apps and Remove,
// and the dot of the colour it will have.
function startersHTML(g) {
  if (!g.starters) return '<p class="guide-loading">Looking for your apps…</p>';
  if (!g.starters.length) return '';
  const tabs = kept(g);
  if (!tabs.length) {
    const skip = g.oneTab ? 'Start with one tab' : 'Go on with the tabs you have';
    return '<p class="guide-loading">' + esc('No tabs to add. ' + skip + ', or put the suggestions back.') + '</p>'
      + '<div class="guide-action guide-loading"><button type="button" class="btn" data-action="guide-restore" data-key="guide-restore">Put them back</button></div>';
  }
  const off = g.busy ? ' disabled' : '';
  const dots = anyColoured({ tabs });
  return '<div class="starters">' + tabs.map((t) => '<div class="starter" role="group" aria-label="' + esc(t.name + ' tab') + '">'
    + '<span class="starter-name">' + (dots ? tabDot(t) : '') + esc(t.name) + '</span>'
    + '<span class="starter-apps">' + (t.apps || []).map((a) => '<span class="starter-app">' + tile(a.mono, a.icon)
      + esc(a.name) + '</span>').join('') + '</span>'
    + '<button type="button" class="btn small" data-action="guide-remove" data-name="' + esc(t.name) + '" data-key="starter-remove-' + esc(t.name) + '"'
    + ' aria-label="' + esc('Remove the ' + t.name + ' tab') + '"' + off + '>Remove</button></div>').join('') + '</div>';
}

// layoutTip says, in the compact layout, why Bench's window has it, what it
// does, and how to show windows side by side instead: with the layout
// control at the right of the title bar, which the compact layout makes
// one button that names the mode in use. It's '' in the wide layout, where
// a tab's windows are side by side. Auto keeps the compact layout until the
// window is WIDE_AGAIN_SLACK points wider than compact_below, so a window
// that is as wide as compact_below, or a little wider, can still have it:
// the tip then says until when.
function layoutTip(view) {
  if (view.sizeClass !== 'compact') return '';
  const s = view.settings || {};
  const mode = (LAYOUT_MODES.find(([id]) => id === view.mode) || LAYOUT_MODES[0])[1];
  const below = s.compactBelow > 0 ? s.compactBelow : 0;
  let why = 'Bench\'s window is ' + (below ? 'narrower than ' + fmt(below) + ' points' : 'narrow') + ', so it uses the compact layout';
  if (view.mode === 'compact') why = 'You chose the compact layout';
  else if (below && view.windowWidth >= below) {
    why = 'Bench\'s window was narrower than ' + fmt(below) + ' points, so it keeps the compact layout until it\'s '
      + fmt(below + WIDE_AGAIN_SLACK) + ' points wide';
  }
  return why + ': a tab shows its windows one above the other, or one at a time. '
    + 'To show them side by side, click ' + mode + ' at the right of the title bar, then choose Wide.';
}

// tipsPage returns the four tips, with the shortcuts as configured. The
// first says which layout Bench's window has, as a tab's windows are side
// by side only in the wide one.
function tipsPage(view) {
  const keys = keysOf(view);
  const compact = layoutTip(view);
  const set = compact ? 'Each tab is a set of windows.' : 'Each tab is a set of windows side by side.';
  const tabs = keys.tabs
    ? set + ' Click a tab, or press ' + keys.tabs + '1 to ' + keys.tabs + '9 in any app, to switch to it.'
    : set + ' Click a tab to switch to it.';
  const jump = (keys.jump
    ? 'Press ' + keys.jump + ' in any app to jump to a tab or a window, or to find something to do, such as Settings.'
    : 'Click Jump to… beside the tabs to jump to a tab or a window, or to find something to do, such as Settings.')
    + ' Type words from its name in any order. With nothing typed, Return goes back to the tab you were on.';
  return stepHTML(1, 'Tabs', '<p>' + esc(tabs) + '</p>' + (compact ? '<p>' + esc(compact) + '</p>' : ''))
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

// setupIntro returns the set-up's introduction, which names its choices:
// a tab of the windows that are open, if there are any, and the
// suggested tabs, if Go suggests any.
function setupIntro(view, g) {
  const windows = openApps(view).length > 0;
  const suggests = !g.starters || g.starters.length > 0;
  if (!windows) return HEADS[g.oneTab ? 'setup' : 'setupMore'][1];
  if (!suggests) {
    return g.oneTab
      ? 'Make a tab of the windows you have open, or start with one tab and make your own.'
      : 'Make a tab of the windows you have open, or go on with the tabs you have.';
  }
  return g.oneTab
    ? 'Make a tab of the windows you have open, or add a few tabs made of apps you have. Remove any you don\'t want, or start with one tab and make your own.'
    : 'Make a tab of the windows you have open, or add a few tabs made of apps you have, after your tabs. Remove any you don\'t want, or don\'t add any.';
}

// footHTML returns a page's buttons: Back, and what goes on.
function footHTML(view, g) {
  const pages = pagesOf(g);
  const at = pages.indexOf(g.page);
  const off = g.busy ? ' disabled' : '';
  const back = at > 0 ? '<button type="button" class="btn" data-action="guide-back" data-key="guide-back"' + off + '>Back</button>' : '';
  let next;
  if (g.page === 'setup') {
    const n = kept(g).length;
    let skip = g.oneTab ? 'Start with one tab' : 'Don\'t add tabs';
    if (g.made.length) skip = n ? 'Don\'t add these' : 'Continue';
    // With no suggestions at all, there's nothing to add.
    const suggests = Boolean(g.starters && g.starters.length);
    next = '<button type="button" class="btn' + (suggests ? '' : ' primary') + '" data-action="guide-one-tab" data-key="guide-one-tab"' + off + '>' + skip + '</button>'
      + (suggests ? '<button type="button" class="btn primary" data-action="guide-add-tabs" data-key="guide-add-tabs"' + (n && !g.busy ? '' : ' disabled') + '>'
        + esc(n === 1 ? 'Add 1 tab' : 'Add ' + n + ' tabs') + '</button>' : '');
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
  const [title, head] = HEADS[g.page === 'setup' && !g.oneTab ? 'setupMore' : g.page];
  const intro = g.page === 'setup' ? setupIntro(view, g) : head;
  let body;
  if (g.page === 'setup') body = setupPage(view, g);
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
