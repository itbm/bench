// The Settings dialog: the settings that apply to every tab, the
// launchers (launchers.js), and the shortcuts (keyboard-settings.js), saved
// with SaveSettings, and Open at login. Each tab's own name, layout, and
// small-screen setting are in Tab properties (editor.js).
//
// The dialog shows one section at a time, which a list of the sections
// down its side chooses, or, in a narrow dialog, a pop-up menu beside its
// title (decision 66). It opens at the section it last showed, which the
// page keeps in its local storage between openings and restarts. While it's
// open, Go draws Bench as the chosen colours, density, and space between
// panes would make it, without saving them or moving any window
// (PreviewSettings), until Save or the dialog closes. Closing it with
// changes that aren't saved asks first (discard.js).

import { act, actLatest, closeOverlay, isDemo, openerKey, store, toast, update } from './state.js';
import { esc, keysOf, radioState, unsavedNote } from './util.js';
import { GRIP } from './dialogs.js';
import { launchersDraft, launchersOut, renderLauncherSettings, sameLaunchers } from './launchers.js';
import { keysChanges, keysClash, keysDraft, renderKeyboardSettings, stopRecording } from './keyboard-settings.js';

// SECTIONS are the dialog's sections, in the order its list shows them.
export const SECTIONS = [
  { id: 'general', label: 'General' },
  { id: 'appearance', label: 'Appearance' },
  { id: 'windows', label: 'Windows' },
  { id: 'keyboard', label: 'Keyboard' },
  { id: 'screens', label: 'Small screens' },
  { id: 'terminals', label: 'Terminals' },
  { id: 'launchers', label: 'Launchers' },
  { id: 'advanced', label: 'Advanced' },
];

// FIELD_SECTIONS are the sections of the settings that the dialog's tick
// boxes and choices change, by field. Any other is General's.
const FIELD_SECTIONS = {
  showStatusBar: 'appearance', paneHeaders: 'appearance', appearance: 'appearance', density: 'appearance', gap: 'appearance',
  tabShortcutHints: 'appearance', scrollBars: 'appearance',
  openMissingApps: 'windows', hideOtherTabs: 'windows', hideOtherApps: 'windows', livePreviews: 'windows', dragToAdd: 'windows',
  newWindows: 'windows', followFocus: 'windows',
  terminalOptionAsMeta: 'terminals', notifyTerminals: 'terminals', keepTerminalOutput: 'terminals',
};

// SECTION_KEY is where the page keeps the section that the dialog last
// showed, in its local storage.
const SECTION_KEY = 'bench.settings.section';

// lastSection is the section that the dialog last showed, for when the
// page can't keep it, as in a private window.
let lastSection = 'general';

// isSection reports whether id names one of the dialog's sections.
function isSection(id) {
  return SECTIONS.some((s) => s.id === id);
}

// rememberedSection returns the section that the dialog last showed.
function rememberedSection() {
  try {
    const kept = window.localStorage.getItem(SECTION_KEY);
    if (isSection(kept)) return kept;
  } catch {
    // The page can't read its storage: the section it noted will do.
  }
  return lastSection;
}

// rememberSection keeps the section that the dialog shows, for the next
// time it opens.
function rememberSection(id) {
  lastSection = id;
  try {
    window.localStorage.setItem(SECTION_KEY, id);
  } catch {
    // The page can't keep it: it lasts until the page loads again.
  }
}

// MENU_BAR is what the menu bar item can show: the menu_bar setting.
const MENU_BAR = [
  { id: 'name', label: 'Icon and tab name' },
  { id: 'icon', label: 'Icon only' },
  { id: 'hidden', label: 'Hidden' },
];

// GAPS are the spaces between panes that Settings offers, in points: the
// gap setting. A gap of 0 would leave no room to grab the dividers.
const GAPS = [8, 4, 2];

// TAB_HINTS are when tabs show their shortcuts: the tab_shortcut_hints
// setting.
const TAB_HINTS = [
  { id: 'auto', label: 'Automatic' },
  { id: 'always', label: 'Always' },
  { id: 'never', label: 'Never' },
];

// SCROLL_BARS are when the strips of tabs and sub-tabs show scroll bars:
// the scroll_bars setting.
const SCROLL_BARS = [
  { id: 'auto', label: 'Automatic' },
  { id: 'always', label: 'Always' },
  { id: 'never', label: 'Never' },
];

// APPEARANCE is whether Bench is light or dark: the appearance setting.
const APPEARANCE = [
  { id: 'system', label: 'Match macOS' },
  { id: 'light', label: 'Light' },
  { id: 'dark', label: 'Dark' },
];

// DENSITY is how roomy Bench's chrome is: the density setting.
const DENSITY = [
  { id: 'auto', label: 'Automatic' },
  { id: 'standard', label: 'Standard' },
  { id: 'compact', label: 'Compact' },
];

// NEW_WINDOWS are where an app's new windows go: the new_windows setting.
const NEW_WINDOWS = [
  { id: 'app-pane', label: 'The app\'s pane' },
  { id: 'focused-pane', label: 'The focused pane' },
  { id: 'float', label: 'Leave floating' },
];

// FOLLOW_FOCUS is what Bench does when another tab's window comes forward:
// the follow_focus setting.
const FOLLOW_FOCUS = [
  { id: 'switch-tab', label: 'Switch to its tab' },
  { id: 'bring-here', label: 'Bring it here' },
  { id: 'off', label: 'Do nothing' },
];

// DEFAULT_FONT_SIZE is the terminals' text size by default, in points: the
// terminal_font_size setting.
const DEFAULT_FONT_SIZE = 12.5;

// openSettings opens the Settings dialog, with a draft of the settings, the
// launchers, and the shortcuts, and the settings as it found them, so that
// Save sends only what the dialog changed (settingsOf). It opens at the
// section it last showed, with the focus on the section's place in the list
// of sections, or with section, at that one. With section 'keyboard', it
// opens at Settings ▸ Keyboard, with the first shortcut that doesn't work
// focused, as a notice's Keyboard settings does.
export function openSettings(section = '') {
  const { ui } = store;
  const view = store.view;
  const s = view.settings || {};
  const shown = isSection(section) ? section : rememberedSection();
  if (isSection(section)) rememberSection(section);
  if (!ui.overlay) ui.opener = openerKey();
  ui.menu = null;
  ui.paneLater = null;
  ui.add = null;
  ui.editor = null;
  ui.overlay = 'settings';
  ui.settings = {
    compactBelow: String(s.compactBelow ?? ''),
    fontSize: String(s.terminalFontSize ?? DEFAULT_FONT_SIZE),
    launchWait: secondsText(s.launchTimeout),
    settings: { ...s },
    base: { ...s },
    openAtLogin: Boolean(view.openAtLogin),
    // busy is set while SaveSettings or SetOpenAtLogin is on its way.
    busy: false,
    // launchers, editing, and launchersChanged are Settings ▸ Launchers'.
    ...launchersDraft(s.launchers),
    // keys is Settings ▸ Keyboard's, and section the section shown.
    keys: keysDraft(view),
    section: shown,
  };
  ui.pendingFocus = section === 'keyboard' ? keyboardFocus(view) : sectionFocus(shown);
  update();
}

// sectionFocus returns where the focus goes in the list of sections: on
// the section's place in it, or, in a narrow dialog, where the list is
// hidden, on the pop-up menu of sections (ui.pendingFocus takes the first
// that can have it).
function sectionFocus(id) {
  return ['settings-section-' + id, 'settings-section-menu'];
}

// keyboardFocus returns where the focus goes in Settings ▸ Keyboard: on the
// first shortcut that doesn't work, or else on the first shortcut.
function keyboardFocus(view) {
  const rows = view.shortcuts || [];
  const first = rows.find((r) => r.problem) || rows[0];
  return first ? 'key-' + first.field : sectionFocus('keyboard');
}

// showSection shows a section of the open Settings dialog, as a click in
// the list of sections, or a choice in the pop-up menu, does. The focus
// stays where it is, on the section's place in the list, or goes to focus
// if given: the pop-up menu keeps it so, as WebKit may not focus a pop-up
// menu that is clicked.
export function showSection(id, focus = '') {
  const d = store.ui.settings;
  if (!d || !isSection(id)) return;
  rememberSection(id);
  if (d.section === id) return;
  stopRecording();
  d.section = id;
  if (focus) store.ui.pendingFocus = focus;
  update();
}

// showKeyboardSettings opens Settings at Settings ▸ Keyboard, or shows that
// section of an open Settings dialog, with the focus on the first shortcut
// that doesn't work, as openSettings does.
export function showKeyboardSettings() {
  const { ui } = store;
  if (ui.overlay !== 'settings' || !ui.settings) {
    openSettings('keyboard');
    return;
  }
  rememberSection('keyboard');
  ui.settings.section = 'keyboard';
  ui.pendingFocus = keyboardFocus(store.view);
  update();
}

// secondsText shows a length of time as Go gives it, such as "15s" or
// "1m30s", in seconds, for Wait for apps to open: "15", or "90".
export function secondsText(duration) {
  const units = { h: 3600, m: 60, s: 1, ms: 0.001, us: 0.000001, 'µs': 0.000001, ns: 0.000000001 };
  let total = 0;
  let matched = '';
  for (const m of String(duration || '').matchAll(/(\d+(?:\.\d+)?)(ms|us|µs|ns|h|m|s)/g)) {
    total += Number(m[1]) * units[m[2]];
    matched += m[0];
  }
  if (!matched || matched !== String(duration)) return '15';
  return String(Math.round(total * 1000) / 1000);
}

// setSetting changes one of the dialog's checkboxes.
export function setSetting(name, on) {
  const d = store.ui.settings;
  if (!d) return;
  if (name === 'openAtLogin') {
    d.openAtLogin = on;
  } else if (name === 'paneHeaders') {
    d.settings.paneHeaders = on ? 'auto' : 'always';
  } else {
    d.settings[name] = on;
    const view = store.view;
    if (name === 'livePreviews' && on && !isDemo() && !(view.permissions && view.permissions.screenRecording)) {
      act('RequestScreenRecording');
    }
  }
  update();
}

// setChoice changes one of the dialog's settings that has a choice of
// values, such as the menu bar item's. The gap is a number of points.
export function setChoice(name, value) {
  const d = store.ui.settings;
  if (!d) return;
  d.settings[name] = name === 'gap' ? Number(value) : value;
  update();
}

// setThreshold changes the draft's threshold for small screens. The field
// keeps what's typed in it without a redraw: redrawing the dialog would
// replace the field being typed in, and a number field's caret can't be
// put back. Only the marks of the sections changed follow it (markChanged).
export function setThreshold(value) {
  const d = store.ui.settings;
  if (!d) return;
  d.compactBelow = value;
  markChanged();
}

// setFontSize changes the draft's text size for terminals, without a
// redraw, as setThreshold does.
export function setFontSize(value) {
  const d = store.ui.settings;
  if (!d) return;
  d.fontSize = value;
  markChanged();
}

// setLaunchWait changes how long the draft waits for apps to open, in
// seconds, without a redraw, as setThreshold does.
export function setLaunchWait(value) {
  const d = store.ui.settings;
  if (!d) return;
  d.launchWait = value;
  markChanged();
}

// markChanged puts the marks of the sections with changes right, in the
// list of sections and the pop-up menu, as renderSettings draws them,
// without redrawing the dialog: for a number field, as it's typed in.
function markChanged() {
  const { ui, view } = store;
  const d = ui.settings;
  if (ui.overlay !== 'settings' || !d || !view) return;
  const changed = new Set(changedSections(view, d));
  for (const s of SECTIONS) {
    const tab = document.getElementById('settings-tab-' + s.id);
    const inner = tabInner(s, changed);
    if (tab && tab.innerHTML !== inner) tab.innerHTML = inner;
  }
  const menu = document.getElementById('settings-section-menu');
  if (!menu) return;
  for (const option of menu.options) {
    const s = SECTIONS.find((x) => x.id === option.value);
    if (s && option.textContent !== optionLabel(s, changed)) option.textContent = optionLabel(s, changed);
  }
}

// NUMBER_FIELDS are the Settings dialog's number fields, by id, and what
// each holds in the draft. Their values are left out of the dialog's HTML
// (fillSettings).
const NUMBER_FIELDS = { 'compact-below': 'compactBelow', 'terminal-font-size': 'fontSize', 'launch-wait': 'launchWait' };

// isSettingsNumber reports whether a field is one of the Settings dialog's
// number fields, where Return saves.
export function isSettingsNumber(id) {
  return Object.hasOwn(NUMBER_FIELDS, id || '');
}

// fillSettings puts the number fields' values, such as the threshold for
// small screens, into a newly drawn Settings dialog, which leaves them out
// of its HTML so that typing them doesn't redraw the dialog
// (setThreshold).
export function fillSettings(ui) {
  const d = ui.settings;
  if (ui.overlay !== 'settings' || !d) return;
  for (const [id, key] of Object.entries(NUMBER_FIELDS)) {
    const input = document.getElementById(id);
    if (input) input.value = d[key];
  }
}

// changedSections returns the sections, in the dialog's order, whose
// settings the draft has changed, and Save would save: those whose tick
// boxes, choices, fields, launchers, or shortcuts differ from the settings
// as the dialog found them, and General for Open at login if it differs
// from macOS's.
export function changedSections(view, d) {
  const changed = new Set();
  const base = d.base;
  for (const [name, value] of Object.entries(d.settings)) {
    if (value !== base[name]) changed.add(FIELD_SECTIONS[name] || 'general');
  }
  if (d.openAtLogin !== Boolean(view.openAtLogin)) changed.add('general');
  if (d.compactBelow !== String(base.compactBelow ?? '')) changed.add('screens');
  if (d.fontSize !== String(base.terminalFontSize ?? DEFAULT_FONT_SIZE)) changed.add('terminals');
  if (d.launchWait !== secondsText(base.launchTimeout)) changed.add('advanced');
  if (d.launchersChanged) changed.add('launchers');
  if (keysChanges(view, d.keys)) changed.add('keyboard');
  return SECTIONS.filter((s) => changed.has(s.id)).map((s) => s.id);
}

// sectionLabel returns a section's name, such as "Small screens".
export function sectionLabel(id) {
  const s = SECTIONS.find((x) => x.id === id);
  return s ? s.label : '';
}

// NO_PREVIEW is the preview of nothing, which PreviewSettings takes to stop
// previewing.
const NO_PREVIEW = { appearance: '', density: '', gap: -1 };

// previewOf returns what Go previews for the Settings dialog that's open:
// the colours, density, and space between panes that it has chosen and not
// saved yet, or NO_PREVIEW's empty value for each it hasn't changed. A
// setting that's as the dialog found it shows as saved, as Save would keep
// it (settingsOf). The question before discarding changes keeps the
// preview, since the changes are still there.
export function previewOf(ui) {
  const d = ui.settings;
  if (!d) return NO_PREVIEW;
  const s = d.settings;
  const base = d.base;
  return {
    appearance: s.appearance !== base.appearance ? s.appearance || '' : '',
    density: s.density !== base.density ? s.density || '' : '',
    gap: Number(s.gap) !== Number(base.gap) && Number.isFinite(Number(s.gap)) ? Number(s.gap) : -1,
  };
}

// previewSent is the preview that Go was last told of, as JSON.
let previewSent = JSON.stringify(NO_PREVIEW);

// syncPreview tells Go what the Settings dialog previews when that
// changes, after each render: Bench's chrome and panes follow the View, and
// Go gives Bench's windows the colours. Go stops previewing by itself when
// SaveSettings saves, when nothing is open any more (SetOverlay), and on
// Ready.
export function syncPreview() {
  const p = previewOf(store.ui);
  const sent = JSON.stringify(p);
  if (sent === previewSent) return;
  previewSent = sent;
  actLatest('settings-preview', 'PreviewSettings', p);
}

// resendPreview notes that Go has stopped previewing, as Ready does, and
// tells it again what the Settings dialog previews, if anything.
export function resendPreview() {
  previewSent = JSON.stringify(NO_PREVIEW);
  syncPreview();
}

// settingsOf returns the SettingsView that SaveSettings takes: the current
// settings, with those the dialog has changed since it opened. So a
// setting that Reload config.toml changed meanwhile keeps its new value,
// unless the dialog changed it too. A threshold, text size, or wait that
// isn't a number keeps the current one; Go refuses a text size it can't
// use, and says why. The launchers are left out, so that Go keeps them,
// unless they have changed: then they replace Go's. So are the shortcuts,
// unless Settings ▸ Keyboard has changed some: then they are those it
// changed.
export function settingsOf(d, view) {
  const threshold = parseInt(d.compactBelow, 10);
  const size = Number.parseFloat(d.fontSize);
  const wait = Number.parseFloat(d.launchWait);
  const base = d.base;
  const out = { ...view.settings };
  for (const [name, value] of Object.entries(d.settings)) if (value !== base[name]) out[name] = value;
  if (d.compactBelow !== String(base.compactBelow ?? '') && threshold > 0) out.compactBelow = threshold;
  if (d.fontSize !== String(base.terminalFontSize ?? DEFAULT_FONT_SIZE) && Number.isFinite(size)) out.terminalFontSize = size;
  if (d.launchWait !== secondsText(base.launchTimeout) && wait > 0) out.launchTimeout = wait + 's';
  delete out.launchers;
  delete out.shortcuts;
  if (d.launchersChanged) out.launchers = launchersOut(d);
  const keys = keysChanges(view, d.keys);
  if (keys) out.shortcuts = keys;
  return out;
}

// saveSettings saves the draft, then Open at login if it changed, and
// closes the dialog once Go has accepted the settings. If Go can't change
// Open at login, a toast says why, and the dialog closes anyway. While the
// calls are on their way, the dialog ignores further saves, so that a
// double click saves once.
export async function saveSettings() {
  const d = store.ui.settings;
  if (!d || d.busy) return;
  stopRecording();
  const view = store.view;
  // A shortcut changed to one that another field has is refused here, as
  // its row says, with the keyboard on it.
  const clash = keysClash(view, d.keys);
  if (clash) {
    toast('Bench can\'t save that shortcut: ' + clash.text);
    store.ui.pendingFocus = 'key-' + clash.field;
    update();
    return;
  }
  // Launchers changed here that Reload config.toml has changed too since
  // the dialog opened would replace what it brought in. Settings ▸
  // Launchers shows them as they are now instead, to change again.
  const launchers = (view.settings || {}).launchers;
  if (d.launchersChanged && !sameLaunchers(launchers, d.base.launchers)) {
    Object.assign(d, launchersDraft(launchers));
    d.base.launchers = launchers;
    toast('config.toml\'s launchers changed while Settings was open, so Settings now shows them as they are. Change them again, then save.');
    update();
    return;
  }
  d.busy = true;
  try {
    const r = await act('SaveSettings', settingsOf(d, view));
    if (!r.ok) return;
    if (d.openAtLogin !== Boolean(view.openAtLogin)) await act('SetOpenAtLogin', d.openAtLogin);
    if (store.ui.settings === d) closeOverlay();
  } finally {
    d.busy = false;
    // Check the draft against the latest View, which may have changed
    // while the calls were on their way.
    if (store.ui.settings === d) update();
  }
}

// needsRecording reports whether the draft turns live previews on in the
// app without Screen Recording permission, which previews need, or before
// macOS applies the permission that Bench got since it opened.
function needsRecording(view, d) {
  const p = view.permissions || {};
  return Boolean(d.settings.livePreviews) && !view.demo && (!p.screenRecording || Boolean(p.screenRecordingReopen));
}

// recordingNote says what previews need while they can't show yet, with
// buttons: one that opens System Settings at Screen Recording's list, and,
// once Bench has asked for the permission, Quit and Reopen Bench, as macOS
// applies it only as Bench opens.
function recordingNote(view) {
  const p = view.permissions || {};
  const text = p.screenRecording
    ? 'macOS applies Screen Recording permission when Bench opens again, so previews show once you quit and reopen Bench.'
    : 'Bench doesn\'t have Screen Recording permission yet, so previews won\'t show. Turn on Bench in System Settings ▸ Privacy & Security ▸ Screen Recording, then quit and reopen Bench.';
  const open = p.screenRecording ? ''
    : '<button type="button" class="btn small" data-action="request-screen-recording" data-key="settings-recording">Open Screen Recording</button>';
  const reopen = p.screenRecordingReopen
    ? '<button type="button" class="btn small" data-action="quit-and-reopen" data-key="settings-reopen">Quit and Reopen Bench</button>'
    : '';
  return '<p class="toggle-note" id="recording-note">' + esc(text) + '</p>'
    + '<div class="toggle-note-actions">' + open + reopen + '</div>';
}

// loginNote says, while macOS needs the user to allow Bench's login item,
// where to do that, with a button that opens it.
function loginNote(view) {
  if (!view.loginApproval) return '';
  return '<p class="toggle-note" id="login-approval-note">' + esc('macOS needs you to allow Bench in System Settings ▸ General ▸ Login Items, or it won\'t open Bench at login.') + '</p>'
    + '<div class="toggle-note-actions"><button type="button" class="btn small" data-action="open-login-items" data-key="settings-login-items">Open Login Items</button></div>';
}

// updatesHelp says what Check for a new version once a day does: with an
// updater (Sparkle, in a Bench.app that has it), Bench offers to install a
// new version; without one, it links to the download.
function updatesHelp(view) {
  if (view.installsUpdates) {
    return 'Bench looks in the latest release on GitHub, and says when there\'s a newer version, with Install… to download and install it. As a browser does, it sends GitHub Bench\'s version and the cookies GitHub gave it. Bench ▸ Check for Updates… looks at once.';
  }
  return 'Bench asks GitHub for its latest release, and says when there\'s a newer one, with a link to download it. It sends only Bench\'s version. Bench ▸ Check for Updates… opens the latest release\'s page.';
}

// menuBarHelp says what the menu bar item shows with a choice: with the
// tab's name, the unread total too, while unread is set (the
// unread_counts setting as the dialog has it). For Hidden, it says what
// brings back a hidden Bench instead: the jump shortcut, unless that's
// off, and Bench's icon in the Dock.
function menuBarHelp(view, choice, unread) {
  switch (choice) {
    case 'icon': return 'Bench\'s icon alone, to save room in the menu bar. Click it for your tabs, Jump to…, and Settings.';
    case 'hidden': {
      const jump = keysOf(view).jump;
      return 'No menu bar item. While Bench is hidden, ' + (jump ? 'press ' + jump + ' or ' : '') + 'click Bench\'s icon in the Dock to bring it back.';
    }
    default: return 'Bench\'s icon, with up to 16 characters of the current tab\'s name' + (unread ? ' and the number of unread messages in all your tabs' : '')
      + '. Click it for your tabs, Jump to…, and Settings.';
  }
}

// choice returns the HTML of one of the dialog's choices: its label, a
// button for each value, which a screen reader finds as a radio group, and
// a line of help, such as what the menu bar item shows.
function choice(name, label, values, chosen, help) {
  const id = name.replace(/[A-Z]/g, (c) => '-' + c.toLowerCase());
  const seg = values.map((c) =>
    '<button type="button" data-action="setting-choice" data-choice="' + name + '" data-value="' + esc(c.id) + '" data-key="' + id + '-' + esc(c.id) + '"'
    + radioState(chosen === c.id) + '>' + esc(c.label) + '</button>').join('');
  return '<div class="settings-choice"><span class="settings-label" id="' + id + '-label">' + esc(label) + '</span>'
    + '<div class="seg" role="radiogroup" aria-labelledby="' + id + '-label" aria-describedby="' + id + '-help">' + seg + '</div>'
    + '<p class="settings-help" id="' + id + '-help" aria-live="polite">' + esc(help) + '</p></div>';
}

// newWindowsHelp says where an app's new windows go with a choice.
function newWindowsHelp(choice) {
  switch (choice) {
    case 'focused-pane': return 'A new window joins this tab\'s focused pane.';
    case 'float': return 'A new window stays out of your tabs, for you to add where you like.';
    default: return 'A new window joins the pane with the app\'s other windows, or the pane the config gives the app, and otherwise the focused pane.';
  }
}

// followFocusHelp says what happens with a choice when a window in another
// tab comes forward, as when you click it in the Dock.
function followFocusHelp(choice) {
  switch (choice) {
    case 'bring-here': return 'When a window in another tab comes forward, as when you click it in the Dock, Bench moves it into this tab\'s focused pane.';
    case 'off': return 'When a window in another tab comes forward, as when you click it in the Dock, Bench stays on this tab and leaves the window where it is.';
    default: return 'When a window in another tab comes forward, as when you click it in the Dock, Bench switches to that tab.';
  }
}

// gapChoices are the spaces between panes that Settings offers, and the
// current one if config.toml sets another.
function gapChoices(gap) {
  const gaps = GAPS.includes(gap) ? GAPS : [...GAPS, gap];
  return gaps.map((g) => ({ id: String(g), label: g + ' pt' }));
}

// tabHintsHelp says when tabs show their shortcuts with a choice.
function tabHintsHelp(view, hints) {
  const tabs = keysOf(view).tabs;
  const keys = tabs ? ', such as ' + tabs + '1,' : '';
  switch (hints) {
    case 'always': return 'Tabs always show their shortcuts' + keys + ' after their names.';
    case 'never': return 'Tabs show only their names. Hold the pointer over a tab to see its shortcut.';
    default: return 'Tabs show their shortcuts' + keys + ' except on small screens, where they need the room.';
  }
}

// scrollBarsHelp says when the strips of tabs and sub-tabs show scroll bars
// with a choice, and for Automatic, whether macOS shows them now.
function scrollBarsHelp(view, bars) {
  switch (bars) {
    case 'always': return 'Tabs and sub-tabs that don\'t all fit show a thin scroll bar below them.';
    case 'never': return 'Tabs and sub-tabs that don\'t all fit fade out at the edge. Scroll them with a trackpad or mouse wheel.';
    default: return view.systemScrollBars
      ? 'As macOS does, which shows scroll bars all the time now (System Settings, Appearance), so tabs and sub-tabs that don\'t all fit show a thin one.'
      : 'As macOS does, which shows scroll bars only while scrolling now (System Settings, Appearance), so tabs and sub-tabs that don\'t all fit fade out at the edge.';
  }
}

// appearanceHelp says what Bench looks like with a choice. Web pages and
// the apps in panes follow macOS whatever it is.
function appearanceHelp(appearance) {
  const others = ' Web pages and the apps in your panes follow macOS either way.';
  switch (appearance) {
    case 'light': return 'Bench is light, even when macOS is dark.' + others;
    case 'dark': return 'Bench is dark, even when macOS is light.' + others;
    default: return 'Bench is light or dark as macOS is (System Settings, Appearance), and changes when it does.' + others;
  }
}

// densityHelp says what a density does. In the app, the title bar keeps the
// height it opened with, since macOS sets the traffic lights' place as the
// window is made, so when the density would change it, that waits until
// Bench next opens. Automatic is compact in the compact size class.
function densityHelp(view, density) {
  let help;
  let uses = density;
  switch (density) {
    case 'standard': help = 'Roomy tabs, sub-tabs, and buttons, with pane headers 40 points tall.'; break;
    case 'compact': help = 'Smaller tabs, sub-tabs, and buttons, with pane headers 32 points tall, to leave your apps more room on a laptop.'; break;
    default:
      help = 'Compact while Bench uses the compact layout, as on a small screen, and standard otherwise.';
      uses = view.sizeClass === 'compact' ? 'compact' : 'standard';
  }
  const opened = view.titleBar === 'compact' ? 'compact' : 'standard';
  return view.demo || uses === opened ? help : help + ' The title bar changes height when Bench next opens.';
}

// TOGGLE_HELP is the line of help under some of the Windows group's tick
// boxes.
const TOGGLE_HELP = {
  hideOtherApps: 'When you switch tabs, apps with no windows in a tab hide too, as Hide Others does.',
  dragToAdd: 'Drag a window by its title bar over a pane, and hold it there a moment: a guide shows where it will go. Drop it in the middle to add it to the pane, or at an edge to give it a new pane there. Windows that a rule floats are left alone.',
};

// sectionBody returns the HTML of a section of the Settings dialog: its
// tick boxes, choices, and fields.
function sectionBody(view, d, id) {
  const recording = needsRecording(view, d);
  // A tick box can be described by a line of help or a note (describedBy).
  const toggle = ([name, label, on, describedBy]) => '<label class="toggle"><input type="checkbox" data-setting="' + name + '" data-key="setting-' + name + '"'
    + (on ? ' checked' : '') + (describedBy ? ' aria-describedby="' + describedBy + '"' : '') + '>'
    + '<span>' + esc(label) + '</span></label>'
    + (name === 'livePreviews' && recording ? recordingNote(view) : '');
  switch (id) {
    case 'general': {
      const menuBar = MENU_BAR.some((c) => c.id === d.settings.menuBar) ? d.settings.menuBar : 'name';
      return '<div class="toggles"><div>'
        + toggle(['openAtLogin', 'Open Bench at login', d.openAtLogin, view.loginApproval ? 'login-approval-note' : '']) + loginNote(view) + '</div>'
        + toggle(['fillScreen', 'Open Bench filling the screen', Boolean(d.settings.fillScreen)])
        + toggle(['showOnTabShortcut', 'Bring back a hidden Bench with tab shortcuts', Boolean(d.settings.showOnTabShortcut)]) + '<div>'
        + toggle(['unreadCounts', 'Show unread counts on tabs and sub-tabs', d.settings.unreadCounts !== false, 'unread-help'])
        + '<p class="settings-help toggle-help" id="unread-help">' + esc('Bench reads the badges on apps\' Dock icons, with Accessibility permission, and the counts at the start of web pages\' titles, such as (3). A badge without a number shows as a dot.') + '</p></div><div>'
        + toggle(['checkForUpdates', 'Check for a new version once a day', Boolean(d.settings.checkForUpdates), 'updates-help'])
        + '<p class="settings-help toggle-help" id="updates-help">' + esc(updatesHelp(view)) + '</p></div><div>'
        + toggle(['confirmQuitBusyTerminals', 'Ask before quitting while a terminal is running something', Boolean(d.settings.confirmQuitBusyTerminals), 'confirm-quit-help'])
        + '<p class="settings-help toggle-help" id="confirm-quit-help">' + esc('Also before closing a terminal that\'s running something, from its sub-tab, its own window, or Close tab. Quitting or closing ends what it runs.') + '</p></div><div>'
        + toggle(['confirmClose', 'Ask before closing a tab\'s windows', Boolean(d.settings.confirmClose), 'confirm-close-help'])
        + '<p class="settings-help toggle-help" id="confirm-close-help">' + esc('Close tab asks first when it would close two or more windows, pages, or terminals, or a terminal that\'s running something.') + '</p></div></div>'
        + choice('menuBar', 'Menu bar item', MENU_BAR, menuBar, menuBarHelp(view, menuBar, d.settings.unreadCounts !== false));
    }
    case 'appearance': {
      const hints = TAB_HINTS.some((c) => c.id === d.settings.tabShortcutHints) ? d.settings.tabShortcutHints : 'auto';
      const bars = SCROLL_BARS.some((c) => c.id === d.settings.scrollBars) ? d.settings.scrollBars : 'auto';
      const density = DENSITY.some((c) => c.id === d.settings.density) ? d.settings.density : 'auto';
      const appearance = APPEARANCE.some((c) => c.id === d.settings.appearance) ? d.settings.appearance : 'system';
      const gap = Number(d.settings.gap);
      return '<p class="settings-help settings-lead" id="appearance-preview-help">'
        + esc('Bench shows the colours, density, and space between panes that you choose here immediately. Save keeps them. If you close Settings without saving, they go back.') + '</p>'
        + choice('appearance', 'Colours', APPEARANCE, appearance, appearanceHelp(appearance))
        + choice('density', 'Density', DENSITY, density, densityHelp(view, density))
        + choice('gap', 'Space between panes', gapChoices(gap), String(gap),
          'Less space leaves more room for your apps, and narrower dividers to drag. Your windows move to fit when you save.')
        + '<div class="toggles settings-toggles-after"><div>'
        + toggle(['showStatusBar', 'Show status bar', d.settings.showStatusBar !== false, 'status-bar-help'])
        + '<p class="settings-help toggle-help" id="status-bar-help">' + esc('Without it, the panes are taller, and notices, windows that need attention, and messages show in the title bar.') + '</p></div><div>'
        + toggle(['paneHeaders', 'Hide the sub-tab row when a pane holds one app window', d.settings.paneHeaders === 'auto', 'pane-headers-help'])
        + '<p class="settings-help toggle-help" id="pane-headers-help">' + esc('The app\'s own title bar takes the row\'s place. The row comes back when the pane gets another window, or the window needs attention. Jump to… can add to the pane, or show its window side by side.') + '</p></div></div>'
        + choice('tabShortcutHints', 'Show shortcuts on tabs', TAB_HINTS, hints, tabHintsHelp(view, hints))
        + choice('scrollBars', 'Show scroll bars', SCROLL_BARS, bars, scrollBarsHelp(view, bars));
    }
    case 'windows': {
      const windows = [
        ['openMissingApps', 'Open apps that aren\'t running', Boolean(d.settings.openMissingApps)],
        ['hideOtherTabs', 'Hide windows that belong to other tabs', Boolean(d.settings.hideOtherTabs)],
        ['hideOtherApps', 'Also hide apps that aren\'t in any tab', Boolean(d.settings.hideOtherApps), 'hide-other-apps-help'],
        ['livePreviews', 'Show live previews of windows (needs Screen Recording)', Boolean(d.settings.livePreviews), recording ? 'recording-note' : ''],
        ['dragToAdd', 'Drag windows onto Bench to add them', d.settings.dragToAdd !== false, 'drag-to-add-help'],
      ].map((t) => (TOGGLE_HELP[t[0]]
        ? '<div>' + toggle(t) + '<p class="settings-help toggle-help" id="' + t[3] + '">' + esc(TOGGLE_HELP[t[0]]) + '</p></div>'
        : toggle(t))).join('');
      const newWindows = NEW_WINDOWS.some((c) => c.id === d.settings.newWindows) ? d.settings.newWindows : 'app-pane';
      const follow = FOLLOW_FOCUS.some((c) => c.id === d.settings.followFocus) ? d.settings.followFocus : 'switch-tab';
      return '<div class="toggles">' + windows + '</div>'
        + choice('newWindows', 'New windows go to', NEW_WINDOWS, newWindows, newWindowsHelp(newWindows))
        + choice('followFocus', 'When another tab\'s window comes forward', FOLLOW_FOCUS, follow, followFocusHelp(follow));
    }
    case 'keyboard': return renderKeyboardSettings(view, d);
    case 'screens':
      return '<label class="settings-label" for="compact-below">Compact layout when Bench is narrower than</label>'
        + '<div class="settings-field"><input class="number-input" id="compact-below" data-key="compact-below" type="number" min="800" max="4000" step="10" aria-describedby="compact-below-help">'
        + '<span>points</span></div>'
        + '<p class="settings-help" id="compact-below-help">' + esc('Bench compares this with the width of its own window, so a window in half the screen counts as small. In a narrower window, Auto chooses the compact layout, where each tab follows its own small-screen setting.') + '</p>';
    case 'terminals':
      return '<label class="settings-label" for="terminal-font-size">Text size</label>'
        + '<div class="settings-field"><input class="number-input" id="terminal-font-size" data-key="terminal-font-size" type="number" min="9" max="24" step="0.5" aria-describedby="terminal-font-size-help">'
        + '<span>points</span></div>'
        + '<p class="settings-help" id="terminal-font-size-help">' + esc('From 9 to 24, for every terminal. While a terminal has the keyboard, ⌘+ and ⌘− make its text bigger or smaller for now, and ⌘0 puts it back to this size.') + '</p>'
        + '<div class="toggles settings-toggles-after"><div>'
        + toggle(['terminalOptionAsMeta', 'Use Option as Meta key', Boolean(d.settings.terminalOptionAsMeta), 'option-meta-help'])
        + '<p class="settings-help toggle-help" id="option-meta-help">' + esc('Option works as Meta, as with Terminal\'s setting of the same name: Option-B and Option-F move by words in the shell. Option then no longer types its own characters, such as # or € on some keyboards.') + '</p></div><div>'
        + toggle(['notifyTerminals', 'Notify me when a terminal in another tab needs attention', d.settings.notifyTerminals !== false, 'notify-terminals-help'])
        + '<p class="settings-help toggle-help" id="notify-terminals-help">' + esc('When a terminal you can\'t see rings its bell, asks for a notification, as Claude Code and Codex can, or finishes its command. Its sub-tab and tab get a bell either way, until you look at it.') + '</p></div><div>'
        + toggle(['keepTerminalOutput', 'Keep terminal output between launches', d.settings.keepTerminalOutput !== false, 'keep-output-help'])
        + '<p class="settings-help toggle-help" id="keep-output-help">' + esc('Each terminal shows what it last showed when Bench next opens, with its programs\' colours and the rest in grey. Bench keeps it in its support folder, where only you can read it, until you close the terminal. Output can hold secrets, such as passwords and keys: turning this off removes what Bench kept.') + '</p></div></div>';
    case 'launchers': return renderLauncherSettings(d);
    case 'advanced':
      return '<label class="settings-label" for="launch-wait">Wait for apps to open</label>'
        + '<div class="settings-field"><input class="number-input" id="launch-wait" data-key="launch-wait" type="number" min="1" max="600" step="1" aria-describedby="launch-wait-help">'
        + '<span>seconds</span></div>'
        + '<p class="settings-help" id="launch-wait-help">' + esc('How long Bench waits for apps to open their windows, when you switch tabs and when you add an app. A slow app may need longer.') + '</p>';
    default: return '';
  }
}

// tabInner returns the HTML inside a section's place in the list of
// sections: its name, and for a section with changes (changed), a dot,
// which screen readers hear as "changed".
function tabInner(s, changed) {
  return '<span>' + esc(s.label) + '</span>'
    + (changed.has(s.id) ? '<span class="settings-dot" aria-hidden="true"></span><span class="visually-hidden">, changed</span>' : '');
}

// optionLabel returns a section's name in the pop-up menu of sections,
// with "(changed)" after it for a section with changes (changed).
function optionLabel(s, changed) {
  return s.label + (changed.has(s.id) ? ' (changed)' : '');
}

// renderSettings returns the Settings dialog's HTML: its title, the list of
// sections down its side, which screen readers find as tabs, and the
// section shown, which scrolls if the dialog is short, between the title
// and the buttons. A narrow dialog hides the list, and shows a pop-up menu
// of the sections beside the title instead (styles.css). A section with
// changes that aren't saved has a dot in the list, and says so to screen
// readers.
export function renderSettings(view, ui) {
  const d = ui.settings;
  if (!d) return '';
  const shown = isSection(d.section) ? d.section : 'general';
  const changed = new Set(changedSections(view, d));
  const label = sectionLabel(shown);
  const tabs = SECTIONS.map((s) => {
    const on = s.id === shown;
    return '<button type="button" role="tab" class="settings-tab" id="settings-tab-' + s.id + '" data-action="settings-section" data-section="' + s.id + '"'
      + ' data-key="settings-section-' + s.id + '" aria-selected="' + on + '" aria-controls="settings-panel"' + (on ? '' : ' tabindex="-1"') + '>'
      + tabInner(s, changed) + '</button>';
  }).join('');
  const options = SECTIONS.map((s) => '<option value="' + s.id + '"' + (s.id === shown ? ' selected' : '') + '>'
    + esc(optionLabel(s, changed)) + '</option>').join('');
  const note = unsavedNote(view);

  return '<div class="dialog settings-sheet" role="dialog" aria-modal="true" aria-labelledby="settings-title" aria-describedby="settings-intro">'
    + '<div class="settings-head dialog-handle"><div class="settings-title-row"><h2 id="settings-title">Settings</h2>'
    + '<select class="select-input settings-menu" id="settings-section-menu" data-key="settings-section-menu" aria-label="Section">' + options + '</select></div>'
    + '<p id="settings-intro">' + esc('These settings apply to every tab. To change a tab\'s name, layout, or small-screen setting, right-click the tab, then choose Tab Properties… in its menu.') + '</p></div>'
    + '<div class="settings-main">'
    + '<div class="settings-nav" role="tablist" aria-orientation="vertical" aria-label="Sections" data-activate="auto">' + tabs + '</div>'
    + '<div class="dialog-body settings-panel" id="settings-panel" role="tabpanel" aria-labelledby="settings-tab-' + shown + '" data-scroll="settings-' + shown + '">'
    + '<section class="settings-group" aria-labelledby="settings-' + shown + '"><h3 id="settings-' + shown + '">' + esc(label) + '</h3>'
    + sectionBody(view, d, shown) + '</section></div></div>'
    + '<div class="editor-foot"><div class="spacer">'
    + '<button type="button" class="btn" data-action="close" data-key="settings-cancel">Cancel</button>'
    + '<button type="button" class="btn primary" data-action="save-settings" data-key="settings-save">Save</button></div></div>'
    + note
    + GRIP
    + '</div>';
}
