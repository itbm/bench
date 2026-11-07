// Settings ▸ Keyboard: Bench's global shortcuts, one row for each field of
// the [shortcuts] table, as Go lists them (View.shortcuts). Each row says
// whether its shortcuts work, or why not, with a suggestion to use instead,
// and has a field that records new keys and a button that turns it off.
// Restore defaults puts back every default. Settings' Save sends the
// changes (settingsOf), and Go registers the shortcuts again at once.
//
// While a field records, Go releases Bench's global shortcuts
// (PauseShortcuts), which would otherwise take the keys before the page
// sees them, and the keys go to the field rather than to Bench's shortcuts
// in the browser preview (keyboard.js). Go checks each change as it's made
// (CheckShortcuts): whether it can register the shortcut, and whether
// macOS or VoiceOver uses it.

import { act, actLatest, store, update } from './state.js';
import { esc } from './util.js';
import { keyName, modsOf } from './keyboard.js';

// MODIFIERS are the modifiers' symbols, in the order shortcuts use, with
// their names in config.toml.
const MODIFIERS = [['⌃', 'ctrl'], ['⌥', 'alt'], ['⇧', 'shift'], ['⌘', 'cmd']];

// KEYS are the config names of the keys whose display names (keyName)
// aren't their config names in lower case.
const KEYS = {
  Space: 'space', Tab: 'tab', '↩': 'return', '⎋': 'escape', '⌫': 'delete', '⌦': 'forwarddelete',
  '↖': 'home', '↘': 'end', '⇞': 'pageup', '⇟': 'pagedown', '←': 'left', '→': 'right', '↑': 'up', '↓': 'down',
};

// MODIFIER_CODES are the KeyboardEvent codes of the modifier keys.
const MODIFIER_CODES = new Set([
  'ControlLeft', 'ControlRight', 'AltLeft', 'AltRight', 'ShiftLeft', 'ShiftRight', 'MetaLeft', 'MetaRight', 'CapsLock', 'Fn',
]);

// TYPING are the modifiers that keep a shortcut from taking over typing:
// every shortcut needs one, unless its key is a function key.
const TYPING = /[⌃⌥⌘]/;

// ADDED_KEYS are the keys that Bench adds to the fields that hold
// modifiers, in config form.
const ADDED_KEYS = {
  tab_modifiers: ['1', '2', '3', '4', '5', '6', '7', '8', '9'],
  focus_pane_modifiers: ['left', 'right', 'up', 'down'],
  move_window_modifiers: ['left', 'right', 'up', 'down'],
  send_to_tab_modifiers: ['1', '2', '3', '4', '5', '6', '7', '8', '9', '`'],
};

// keysDraft returns Settings ▸ Keyboard's part of the Settings dialog's
// draft: the fields' values, by name, as the dialog has them, and as it
// found them.
export function keysDraft(view) {
  const values = {};
  for (const r of (view && view.shortcuts) || []) values[r.field] = r.value || '';
  return {
    values,
    base: { ...values },
    // recording is the field that records keys, held the modifiers held
    // down meanwhile, and hint why the keys pressed can't be a shortcut.
    recording: null,
    held: '',
    hint: '',
    // checks are Go's answer for the draft's values (CheckShortcuts), by
    // field, and seq numbers the checks, so that a late answer is
    // dropped.
    checks: null,
    seq: 0,
  };
}

// keysChanges returns the fields whose values the draft has changed since
// the dialog opened, as SettingsView.shortcuts takes them, or null if it
// has changed none. So a shortcut that Reload config.toml changed
// meanwhile keeps its new value, unless the draft changed it too.
export function keysChanges(view, k) {
  if (!k) return null;
  const out = {};
  for (const r of (view && view.shortcuts) || []) {
    const value = k.values[r.field];
    if (value !== undefined && value !== (k.base[r.field] ?? '')) out[r.field] = value;
  }
  return Object.keys(out).length ? out : null;
}

// displayOf shows a shortcut, or modifiers alone, in config form, such as
// "ctrl+alt+space", as macOS does: "⌃⌥Space".
export function displayOf(value) {
  if (!value) return '';
  const parts = String(value).toLowerCase().split('+');
  const key = MODIFIERS.some(([, name]) => name === parts[parts.length - 1]) ? '' : parts.pop();
  const mods = MODIFIERS.filter(([, name]) => parts.includes(name)).map(([symbol]) => symbol).join('');
  const name = Object.keys(KEYS).find((k) => KEYS[k] === key);
  return mods + (name || (key.length === 1 ? key.toUpperCase() : key.replace(/^f(\d+)$/, 'F$1')));
}

// configOf returns a shortcut in display form, such as "⌃⌥J", in config
// form: "ctrl+alt+j".
function configOf(mods, key) {
  const parts = MODIFIERS.filter(([symbol]) => mods.includes(symbol)).map(([, name]) => name);
  if (key) parts.push(KEYS[key] || key.toLowerCase());
  return parts.join('+');
}

// draft returns the open Settings dialog's Keyboard draft, or null.
function draft() {
  const d = store.ui.settings;
  return d && d.keys ? d.keys : null;
}

// rowOf returns the view's row for a field.
function rowOf(view, field) {
  return ((view && view.shortcuts) || []).find((r) => r.field === field) || null;
}

// stateOf returns what's known of a row's shortcuts with the draft's
// value: Go's check of the draft, or, for a field the draft leaves as it
// is, what Go found as it registered them; or null until Go has checked.
function stateOf(r, k, value) {
  const checked = k.checks && k.checks[r.field];
  if (checked && checked.value === value) return checked;
  if ((r.value || '') === value) return r;
  return null;
}

// shortcutsOf returns the shortcuts that a field's value makes, in config
// form, as Go's shortcuts.Bindings does: modifiers with the keys Bench adds
// to them, and next_window with ⇧ added for the previous window.
function shortcutsOf(field, value) {
  if (!value) return [];
  if (ADDED_KEYS[field]) return ADDED_KEYS[field].map((key) => value + '+' + key);
  if (field !== 'next_window') return [value];
  const { mods, key } = splitDisplay(displayOf(value));
  return [value, configOf(mods + '⇧', key)];
}

// splitDisplay splits a shortcut in display form into its modifiers and
// its key.
function splitDisplay(display) {
  const mods = display.match(/^[⌃⌥⇧⌘]*/)[0];
  return { mods, key: display.slice(mods.length) };
}

// clashOf says which other field already makes one of the shortcuts that
// a field the draft has changed makes, or ''. Go says the same when it
// checks the draft, but on whichever of the two comes later in config.toml.
function clashOf(view, k, r, value) {
  if (!value || value === (r.value || '')) return '';
  const mine = shortcutsOf(r.field, value);
  for (const o of (view && view.shortcuts) || []) {
    if (o.field === r.field) continue;
    const shared = shortcutsOf(o.field, k.values[o.field]).find((x) => mine.includes(x));
    if (shared) return displayOf(shared) + ' is already the shortcut for ' + o.label + '.';
  }
  return '';
}

// keysClash returns the first field that the draft has changed to a
// shortcut that another field makes, with what the field's row says about
// it, or null. Settings refuses to save the draft then, naming the field
// changed: Go would name whichever of the two comes later in config.toml.
export function keysClash(view, k) {
  if (!k) return null;
  for (const r of (view && view.shortcuts) || []) {
    const text = clashOf(view, k, r, k.values[r.field] || '');
    if (text) return { field: r.field, text };
  }
  return null;
}

// startRecording has a field record the keys pressed next, releasing
// Bench's global shortcuts meanwhile, or stops it if it's recording
// already.
export function startRecording(field) {
  const k = draft();
  if (!k) return;
  if (k.recording === field) {
    stopRecording();
    return;
  }
  const was = k.recording;
  k.recording = field;
  k.held = '';
  k.hint = '';
  if (!was) actLatest('pause-shortcuts', 'PauseShortcuts', true);
  store.ui.pendingFocus = 'key-' + field;
  update();
}

// stopRecording stops a field recording, and registers Bench's global
// shortcuts again.
export function stopRecording() {
  const k = draft();
  if (!k || !k.recording) return;
  k.recording = null;
  k.held = '';
  k.hint = '';
  actLatest('pause-shortcuts', 'PauseShortcuts', false);
  update();
}

// stopRecordingFor stops recording when a click lands anywhere but the
// field that records.
export function stopRecordingFor(target) {
  const k = draft();
  if (!k || !k.recording) return;
  const field = target && target.closest ? target.closest('[data-action="key-record"]') : null;
  if (!field || field.dataset.field !== k.recording) stopRecording();
}

// setShortcut sets a field's value in the draft, in config form, or turns
// it off with '', and has Go check the draft.
export function setShortcut(field, value) {
  const k = draft();
  if (!k || !(field in k.values)) return;
  const recording = k.recording;
  k.values[field] = value;
  if (recording) {
    k.recording = null;
    k.held = '';
    k.hint = '';
    actLatest('pause-shortcuts', 'PauseShortcuts', false);
  }
  checkKeys();
  update();
}

// useSuggestion sets a field to what Go suggests in its place.
export function useSuggestion(field) {
  const k = draft();
  const r = rowOf(store.view, field);
  if (!k || !r) return;
  const s = stateOf(r, k, k.values[field]);
  if (s && s.suggestion) setShortcut(field, s.suggestion);
}

// restoreDefaults sets every field to its default.
export function restoreDefaults() {
  const k = draft();
  if (!k) return;
  stopRecording();
  for (const r of store.view.shortcuts || []) k.values[r.field] = r.default || '';
  checkKeys();
  update();
}

// checkKeys asks Go whether the draft's shortcuts would work, and shows
// its answer, unless the draft has changed again meanwhile.
async function checkKeys() {
  const k = draft();
  if (!k) return;
  const seq = ++k.seq;
  const changes = keysChanges(store.view, k);
  if (!changes) {
    k.checks = null;
    return;
  }
  const r = await act('CheckShortcuts', changes);
  if (draft() !== k || k.seq !== seq || !r.ok) return;
  k.checks = {};
  for (const v of r.value || []) k.checks[v.field] = v;
  update();
}

// recordKey takes a key pressed while a field records: a shortcut, or for
// a field of modifiers, the modifiers held with any key. Escape stops
// recording, Delete turns the shortcut off, and Tab moves on. It reports
// whether it took the key, which then goes no further.
export function recordKey(e) {
  const k = draft();
  if (!k || !k.recording || store.ui.overlay !== 'settings') return false;
  const mods = modsOf(e);
  if (!mods && e.key === 'Tab') {
    stopRecording();
    return false;
  }
  e.preventDefault();
  e.stopPropagation();
  if (e.repeat) return true;
  const field = k.recording;
  const r = rowOf(store.view, field);
  if (MODIFIER_CODES.has(e.code)) {
    k.held = MODIFIERS.map(([symbol]) => symbol).filter((s) => (k.held + mods).includes(s)).join('');
    k.hint = '';
    update();
    return true;
  }
  if (!mods && e.key === 'Escape') {
    stopRecording();
    return true;
  }
  if (!mods && (e.key === 'Backspace' || e.key === 'Delete')) {
    setShortcut(field, '');
    return true;
  }
  const key = keyName(e);
  if (r && r.keys) {
    if (TYPING.test(mods)) setShortcut(field, configOf(mods, ''));
    else hint(k, 'Hold ⌃, ⌥, or ⌘, with ⇧ if you like, and let go.');
    return true;
  }
  if (!key) hint(k, 'Bench can\'t use that key. Use a letter, a digit, punctuation, Space, Tab, Return, an arrow, or F1 to F20.');
  else if (!TYPING.test(mods) && !/^F\d+$/.test(key)) hint(k, 'Add ⌃, ⌥, or ⌘, so that the shortcut doesn\'t take over typing.');
  else setShortcut(field, configOf(mods, key));
  return true;
}

// recordKeyUp takes the modifiers that a field of modifiers records as
// they're let go, so that no key need be pressed with them. It reports
// whether it took the key.
export function recordKeyUp(e) {
  const k = draft();
  if (!k || !k.recording || store.ui.overlay !== 'settings') return false;
  e.preventDefault();
  e.stopPropagation();
  if (!MODIFIER_CODES.has(e.code) || modsOf(e)) return true;
  const r = rowOf(store.view, k.recording);
  const held = k.held;
  k.held = '';
  if (r && r.keys && TYPING.test(held)) setShortcut(k.recording, configOf(held, ''));
  else update();
  return true;
}

// hint says why the keys pressed can't be the shortcut, and keeps
// recording.
function hint(k, text) {
  k.held = '';
  k.hint = text;
  update();
}

// addedText says what Bench adds to the fields that hold modifiers, and to
// next_window, after their keys in a row, here and in the Keyboard
// Shortcuts sheet (shortcuts.js).
export function addedText(r) {
  if (r.keys) return 'with ' + r.keys;
  if (r.field === 'next_window') return 'and ⇧ for the previous';
  return '';
}

// renderKeyboardSettings returns the HTML of Settings ▸ Keyboard.
export function renderKeyboardSettings(view, d) {
  const k = d.keys;
  const rows = view.shortcuts || [];
  if (!k || !rows.length) return '';
  const intro = view.demo
    ? 'In the browser preview, these work while the page has the keyboard. Click a shortcut, then press the keys you want.'
    : 'These work in every app. Click a shortcut, then press the keys you want.';
  const defaults = rows.every((r) => (k.values[r.field] || '') === (r.default || ''));
  return '<p class="settings-help keys-help">' + esc(intro) + '</p>'
    + '<div class="key-rows">' + rows.map((r) => keyRow(view, k, r)).join('') + '</div>'
    + '<div class="keys-foot"><button type="button" class="btn small" data-action="keys-defaults" data-key="keys-defaults"'
    + (defaults ? ' disabled' : '') + '>Restore defaults</button></div>';
}

// keyRow returns the HTML of one field's row: what it does, its keys,
// which record new ones when clicked, Turn off, and whether it works.
function keyRow(view, k, r) {
  const value = k.values[r.field] || '';
  const recording = k.recording === r.field;
  const id = 'key-' + r.field.replace(/_/g, '-');
  let shown = value ? displayOf(value) : 'Off';
  if (recording) shown = k.held ? k.held + '…' : (r.keys ? 'Hold modifiers…' : 'Press keys…');
  const more = addedText(r);
  return '<div class="key-row' + (recording ? ' recording' : '') + '">'
    + '<span class="key-name" id="' + id + '-label">' + esc(r.label || r.field) + '</span>'
    + '<span class="key-keys"><button type="button" class="key-field' + (value || recording ? '' : ' off') + '" data-action="key-record" data-field="' + esc(r.field) + '"'
    + ' data-key="key-' + esc(r.field) + '" aria-pressed="' + recording + '" aria-labelledby="' + id + '-label ' + id + '-value"'
    + ' aria-describedby="' + id + '-state"><span id="' + id + '-value">' + esc(shown) + '</span></button>'
    + (more ? '<span class="key-added">' + esc(more) + '</span>' : '') + '</span>'
    + '<button type="button" class="btn small key-off" data-action="key-off" data-field="' + esc(r.field) + '" data-key="key-' + esc(r.field) + '-off"'
    + ' aria-describedby="' + id + '-label"' + (value ? '' : ' disabled') + '>Turn off</button>'
    + keyState(view, k, r, value, id)
    + '</div>';
}

// keyState returns the HTML of the line under a row: what to press while
// it records; otherwise whether its shortcuts work, or why not, with a
// button to use Go's suggestion.
function keyState(view, k, r, value, id) {
  const line = (cls, text, extra = '') => '<p class="key-state' + cls + '" id="' + id + '-state" aria-live="polite">' + esc(text) + extra + '</p>';
  if (k.recording === r.field) {
    if (k.hint) return line(' warn', k.hint);
    return line('', r.keys
      ? 'Hold the modifiers, such as ⌃⌥, and let go. Escape stops; Delete turns it off.'
      : 'Press the keys, such as ⌃⌥J. Escape stops; Delete turns it off.');
  }
  if (!value) return line('', 'Off. Bench has no shortcut for this.');
  const clash = clashOf(view, k, r, value);
  if (clash) return line(' warn', clash);
  const s = stateOf(r, k, value);
  if (!s) return line('', '');
  if (!s.problem) return line(' ok', 'Works.');
  const use = s.suggestion
    ? ' <button type="button" class="btn small key-use" data-action="key-use" data-field="' + esc(r.field) + '" data-key="key-' + esc(r.field) + '-use">'
      + esc('Use ' + (s.suggestionDisplay || displayOf(s.suggestion))) + '</button>'
    : '';
  return line(' warn', s.problem, use);
}

// showKeyboard scrolls Settings to its Keyboard section, once it's drawn,
// for Settings ▸ Keyboard and the notices' Keyboard settings.
export function showKeyboard() {
  const group = document.getElementById('settings-keyboard');
  if (group) group.scrollIntoView({ block: 'start' });
}
