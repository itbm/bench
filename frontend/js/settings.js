// The Settings dialog: the settings that apply to every tab, saved with
// SaveSettings, and Open at login. Each tab's own name, layout, and
// small-screen setting are in Tab properties (editor.js).

import { act, closeOverlay, isDemo, openerKey, store, update } from './state.js';
import { esc, keysOf, unsavedNote } from './util.js';

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

// DENSITY is how roomy Bench's chrome is: the density setting.
const DENSITY = [
  { id: 'auto', label: 'Automatic' },
  { id: 'standard', label: 'Standard' },
  { id: 'compact', label: 'Compact' },
];

// openSettings opens the Settings dialog, with a draft of the settings.
export function openSettings() {
  const { ui } = store;
  const view = store.view;
  const s = view.settings || {};
  if (!ui.overlay) ui.opener = openerKey();
  ui.menu = null;
  ui.paneLater = null;
  ui.add = null;
  ui.editor = null;
  ui.overlay = 'settings';
  ui.settings = {
    compactBelow: String(s.compactBelow ?? 1800),
    settings: { ...s },
    followFocusWas: s.followFocus,
    openAtLogin: Boolean(view.openAtLogin),
    // busy is set while SaveSettings or SetOpenAtLogin is on its way.
    busy: false,
  };
  ui.pendingFocus = 'setting-openAtLogin';
  update();
}

// setSetting changes one of the dialog's checkboxes.
export function setSetting(name, on) {
  const d = store.ui.settings;
  if (!d) return;
  if (name === 'openAtLogin') {
    d.openAtLogin = on;
  } else if (name === 'followFocus') {
    // The checkbox is "Switch tabs"; unticking it keeps "bring-here" if
    // that was the setting, and otherwise turns focus following off.
    d.settings.followFocus = on ? 'switch-tab' : (d.followFocusWas && d.followFocusWas !== 'switch-tab' ? d.followFocusWas : 'off');
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
// put back.
export function setThreshold(value) {
  const d = store.ui.settings;
  if (d) d.compactBelow = value;
}

// fillSettings puts the threshold for small screens into a newly drawn
// Settings dialog, which leaves it out of its HTML so that typing it
// doesn't redraw the dialog (setThreshold).
export function fillSettings(ui) {
  const d = ui.settings;
  if (ui.overlay !== 'settings' || !d) return;
  const input = document.getElementById('compact-below');
  if (input) input.value = d.compactBelow;
}

// settingsOf returns the SettingsView that SaveSettings takes. A threshold
// that isn't a number keeps the current one.
export function settingsOf(d, view) {
  const threshold = parseInt(d.compactBelow, 10);
  return { ...d.settings, compactBelow: threshold > 0 ? threshold : (view.settings || {}).compactBelow };
}

// saveSettings saves the draft, then Open at login if it changed, and
// closes the dialog once Go has accepted the settings. If Go can't change
// Open at login, a toast says why, and the dialog closes anyway. While the
// calls are on their way, the dialog ignores further saves, so that a
// double click saves once.
export async function saveSettings() {
  const d = store.ui.settings;
  if (!d || d.busy) return;
  const view = store.view;
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
// app without Screen Recording permission, which previews need.
function needsRecording(view, d) {
  return Boolean(d.settings.livePreviews) && !view.demo && !(view.permissions && view.permissions.screenRecording);
}

// menuBarHelp says what the menu bar item shows with a choice. For Hidden,
// it says what brings back a hidden Bench instead: the jump shortcut,
// unless that's off, and Bench's icon in the Dock.
function menuBarHelp(view, choice) {
  switch (choice) {
    case 'icon': return 'Bench\'s icon alone, to save room in the menu bar. Click it for your tabs, Jump to…, and Settings.';
    case 'hidden': {
      const jump = keysOf(view).jump;
      return 'No menu bar item. While Bench is hidden, ' + (jump ? 'press ' + jump + ' or ' : '') + 'click Bench\'s icon in the Dock to bring it back.';
    }
    default: return 'Bench\'s icon, with up to 16 characters of the current tab\'s name. Click it for your tabs, Jump to…, and Settings.';
  }
}

// choice returns the HTML of one of the dialog's choices: its label, a
// button for each value, and a line of help, such as what the menu bar item
// shows.
function choice(name, label, values, chosen, help) {
  const id = name.replace(/[A-Z]/g, (c) => '-' + c.toLowerCase());
  const seg = values.map((c) =>
    '<button type="button" data-action="setting-choice" data-choice="' + name + '" data-value="' + esc(c.id) + '" data-key="' + id + '-' + esc(c.id) + '"'
    + ' aria-pressed="' + (chosen === c.id) + '">' + esc(c.label) + '</button>').join('');
  return '<div class="settings-choice"><span class="settings-label" id="' + id + '-label">' + esc(label) + '</span>'
    + '<div class="seg" role="group" aria-labelledby="' + id + '-label" aria-describedby="' + id + '-help">' + seg + '</div>'
    + '<p class="settings-help" id="' + id + '-help" aria-live="polite">' + esc(help) + '</p></div>';
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

// renderSettings returns the Settings dialog's HTML.
export function renderSettings(view, ui) {
  const d = ui.settings;
  if (!d) return '';
  const recording = needsRecording(view, d);
  // A tick box can be described by a line of help or a note (describedBy).
  const toggle = ([id, label, on, describedBy]) => '<label class="toggle"><input type="checkbox" data-setting="' + id + '" data-key="setting-' + id + '"'
    + (on ? ' checked' : '') + (describedBy ? ' aria-describedby="' + describedBy + '"' : '') + '>'
    + '<span>' + esc(label) + '</span></label>'
    + (id === 'livePreviews' && recording
      ? '<p class="toggle-note" id="recording-note">' + esc('Bench doesn\'t have Screen Recording permission yet, so previews won\'t show. In System Settings, open Privacy & Security, then Screen Recording, and turn on Bench.') + '</p>'
      : '');
  const windows = [
    ['openMissingApps', 'Open apps that aren\'t running', Boolean(d.settings.openMissingApps)],
    ['hideOtherTabs', 'Hide windows that belong to other tabs', Boolean(d.settings.hideOtherTabs)],
    ['followFocus', 'Switch tabs when another tab\'s window comes forward', d.settings.followFocus === 'switch-tab'],
    ['livePreviews', 'Show live previews of windows (needs Screen Recording)', Boolean(d.settings.livePreviews), recording ? 'recording-note' : ''],
  ].map(toggle).join('');
  const menuBar = MENU_BAR.some((c) => c.id === d.settings.menuBar) ? d.settings.menuBar : 'name';
  const hints = TAB_HINTS.some((c) => c.id === d.settings.tabShortcutHints) ? d.settings.tabShortcutHints : 'auto';
  const bars = SCROLL_BARS.some((c) => c.id === d.settings.scrollBars) ? d.settings.scrollBars : 'auto';
  const density = DENSITY.some((c) => c.id === d.settings.density) ? d.settings.density : 'auto';
  const gap = Number(d.settings.gap);
  const note = unsavedNote(view);
  // The heading and the buttons stay in view, and the groups between them
  // scroll if the window is short.
  const group = (id, title, body) => '<section class="settings-group" aria-labelledby="' + id + '"><h3 id="' + id + '">' + esc(title) + '</h3>' + body + '</section>';

  return '<div class="dialog settings-sheet" role="dialog" aria-modal="true" aria-labelledby="settings-title" aria-describedby="settings-intro">'
    + '<div class="settings-head"><h2 id="settings-title">Settings</h2>'
    + '<p id="settings-intro">' + esc('These settings apply to every tab. To change a tab\'s name, layout, or small-screen setting, right-click the tab and choose Tab properties.') + '</p></div>'
    + '<div class="dialog-body" data-scroll="settings-body">'
    + group('settings-general', 'General', '<div class="toggles">' + toggle(['openAtLogin', 'Open Bench at login', d.openAtLogin])
      + toggle(['fillScreen', 'Open Bench filling the screen', Boolean(d.settings.fillScreen)])
      + toggle(['showOnTabShortcut', 'Bring back a hidden Bench with tab shortcuts', Boolean(d.settings.showOnTabShortcut)]) + '</div>'
      + choice('menuBar', 'Menu bar item', MENU_BAR, menuBar, menuBarHelp(view, menuBar)))
    + group('settings-appearance', 'Appearance', '<div class="toggles"><div>'
      + toggle(['showStatusBar', 'Show status bar', d.settings.showStatusBar !== false, 'status-bar-help'])
      + '<p class="settings-help toggle-help" id="status-bar-help">' + esc('Without it, the panes are taller, and notices, windows that need attention, and messages show in the title bar.') + '</p></div><div>'
      + toggle(['paneHeaders', 'Hide the sub-tab row when a pane holds one app window', d.settings.paneHeaders === 'auto', 'pane-headers-help'])
      + '<p class="settings-help toggle-help" id="pane-headers-help">' + esc('The app\'s own title bar takes the row\'s place. The row comes back when the pane gets another window, or the window needs attention. Jump to… can add to the pane, or show its window side by side.') + '</p></div></div>'
      + choice('density', 'Density', DENSITY, density, densityHelp(view, density))
      + choice('gap', 'Space between panes', gapChoices(gap), String(gap),
        'Less space leaves more room for your apps, and narrower dividers to drag.')
      + choice('tabShortcutHints', 'Show shortcuts on tabs', TAB_HINTS, hints, tabHintsHelp(view, hints))
      + choice('scrollBars', 'Show scroll bars', SCROLL_BARS, bars, scrollBarsHelp(view, bars)))
    + group('settings-windows', 'Windows', '<div class="toggles">' + windows + '</div>')
    + group('settings-screens', 'Small screens',
      '<label class="settings-label" for="compact-below">Small screens are narrower than</label>'
      + '<div class="settings-field"><input class="number-input" id="compact-below" data-key="compact-below" type="number" min="800" max="4000" step="10" aria-describedby="compact-below-help">'
      + '<span>points</span></div>'
      + '<p class="settings-help" id="compact-below-help">' + esc('Bench compares this with the width of the screen the window is on. On a smaller screen, Auto chooses the compact layout, where each tab follows its own small-screen setting.') + '</p>')
    + '</div>'
    + '<div class="editor-foot"><div class="spacer">'
    + '<button type="button" class="btn" data-action="close" data-key="settings-cancel">Cancel</button>'
    + '<button type="button" class="btn primary" data-action="save-settings" data-key="settings-save">Save</button></div></div>'
    + note
    + '</div>';
}
