// The Settings dialog: the settings that apply to every tab, saved with
// SaveSettings, and Open at login. Each tab's own name, layout, and
// small-screen setting are in Tab properties (editor.js).

import { act, closeOverlay, isDemo, openerKey, store, update } from './state.js';
import { esc } from './util.js';

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
  } else {
    d.settings[name] = on;
    const view = store.view;
    if (name === 'livePreviews' && on && !isDemo() && !(view.permissions && view.permissions.screenRecording)) {
      act('RequestScreenRecording');
    }
  }
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

// renderSettings returns the Settings dialog's HTML.
export function renderSettings(view, ui) {
  const d = ui.settings;
  if (!d) return '';
  const recording = needsRecording(view, d);
  const toggle = ([id, label, on]) => '<label class="toggle"><input type="checkbox" data-setting="' + id + '" data-key="setting-' + id + '"'
    + (on ? ' checked' : '') + (id === 'livePreviews' && recording ? ' aria-describedby="recording-note"' : '') + '>'
    + '<span>' + esc(label) + '</span></label>'
    + (id === 'livePreviews' && recording
      ? '<p class="toggle-note" id="recording-note">' + esc('Bench doesn\'t have Screen Recording permission yet, so previews won\'t show. In System Settings, open Privacy & Security, then Screen Recording, and turn on Bench.') + '</p>'
      : '');
  const windows = [
    ['openMissingApps', 'Open apps that aren\'t running', Boolean(d.settings.openMissingApps)],
    ['hideOtherTabs', 'Hide windows that belong to other tabs', Boolean(d.settings.hideOtherTabs)],
    ['followFocus', 'Switch tabs when another tab\'s window comes forward', d.settings.followFocus === 'switch-tab'],
    ['livePreviews', 'Show live previews of windows (needs Screen Recording)', Boolean(d.settings.livePreviews)],
  ].map(toggle).join('');
  const note = view.configWritable === false
    ? '<p class="editor-note" role="note">' + esc('config.toml has problems, so changes apply but aren\'t saved to it. The status bar lists the problems.') + '</p>'
    : '';
  // The heading and the buttons stay in view, and the groups between them
  // scroll if the window is short.
  const group = (id, title, body) => '<section class="settings-group" aria-labelledby="' + id + '"><h3 id="' + id + '">' + esc(title) + '</h3>' + body + '</section>';

  return '<div class="dialog settings-sheet" role="dialog" aria-modal="true" aria-labelledby="settings-title" aria-describedby="settings-intro">'
    + '<div class="settings-head"><h2 id="settings-title">Settings</h2>'
    + '<p id="settings-intro">' + esc('These settings apply to every tab. To change a tab\'s name, layout, or small-screen setting, right-click the tab and choose Tab properties.') + '</p></div>'
    + '<div class="dialog-body" data-scroll="settings-body">'
    + group('settings-general', 'General', '<div class="toggles">' + toggle(['openAtLogin', 'Open Bench at login', d.openAtLogin]) + '</div>')
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
