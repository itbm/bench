// Launchers, and the terminals that the add dialog's New terminal… form
// makes. The form asks for a command, which Bench types into the user's
// shell and runs, a name for the sub-tab, and a folder, with Choose… for
// macOS's folder picker. Its Save as launcher keeps the terminal in
// config.toml as a [[launcher]], which the add dialog and Jump to… list
// first, and Settings ▸ Launchers edits or removes. A launcher can also
// open an app on a folder or link: config.toml makes those, and Settings
// edits them.

import { act, closeOverlay, store, update } from './state.js';
import { esc, focusedPaneID, focusKey, ICONS } from './util.js';
import { GRIP } from './dialogs.js';
import { tile } from './tiles.js';
import { adding } from './palette.js';

// TERMINAL_MONO is the built-in terminal's monogram, as Go gives it.
const TERMINAL_MONO = '>_';

// The New terminal… form's fields, by id, and what each holds in the
// form's draft (ui.add.terminal).
const TERMINAL_FIELDS = { 'terminal-command': 'command', 'terminal-name': 'name', 'terminal-folder': 'folder' };

// The fields of the launcher that Settings ▸ Launchers is editing, by id,
// and what each holds in the launcher.
const LAUNCHER_FIELDS = {
  'launcher-name': 'name', 'launcher-command': 'command', 'launcher-folder': 'folder', 'launcher-url': 'url', 'launcher-title': 'title',
};

// field returns the HTML of a text field with its label and a line of
// help. Its text is left out, and put in after it's drawn (fillForms), so
// that a new View doesn't replace what's being typed.
function field(id, label, placeholder, help) {
  return '<div class="form-field"><label class="field-label" for="' + id + '">' + esc(label) + '</label>'
    + '<input class="text-input" id="' + id + '" data-key="' + id + '" type="text" autocomplete="off" autocapitalize="off" spellcheck="false"'
    + ' placeholder="' + esc(placeholder) + '" aria-describedby="' + id + '-help">'
    + '<p class="settings-help" id="' + id + '-help">' + esc(help) + '</p></div>';
}

// folderField returns the HTML of a Folder field, with a Choose… button
// that shows macOS's folder picker (chooseFolder).
function folderField(id, help) {
  return '<div class="form-field"><label class="field-label" for="' + id + '">Folder</label>'
    + '<div class="folder-field"><input class="text-input" id="' + id + '" data-key="' + id + '" type="text" autocomplete="off" autocapitalize="off" spellcheck="false"'
    + ' placeholder="~" aria-describedby="' + id + '-help">'
    + '<button type="button" class="btn small" data-action="choose-folder" data-field="' + id + '" data-key="' + id + '-choose">Choose…</button></div>'
    + '<p class="settings-help" id="' + id + '-help">' + esc(help) + '</p></div>';
}

// tick returns the HTML of a tick box with a line of help below it. Its
// data attribute says which draft it changes (onChange in app.js).
function tick(data, id, label, on, help) {
  return '<div><label class="toggle"><input type="checkbox" ' + data + ' id="' + id + '" data-key="' + id + '"' + (on ? ' checked' : '')
    + ' aria-describedby="' + id + '-help"><span>' + esc(label) + '</span></label>'
    + '<p class="settings-help toggle-help" id="' + id + '-help">' + esc(help) + '</p></div>';
}

// START_AGAIN_HELP says what Start again when Bench opens does.
const START_AGAIN_HELP = 'After Bench restarts, the terminal runs the command again. Untick it for a command to run once, such as a deploy.';

// openTerminalForm opens the add dialog's New terminal… form, with a
// command in it already for Run "x" in a new terminal…, or a command, a
// folder, and a name for a program that Browse… found. The focus goes to
// the command, or, if there's one, to the name.
export function openTerminalForm(command = '', folder = '', name = '') {
  const { ui } = store;
  if (!ui.add) return;
  ui.add.step = 'terminal';
  ui.add.terminal = { command, name, folder, startAgain: true, save: false };
  ui.pendingFocus = command ? 'terminal-name' : 'terminal-command';
  update();
}

// renderTerminalStep returns the New terminal… form's HTML.
export function renderTerminalStep(ui) {
  ui.listItems = [];
  const t = ui.add.terminal;
  return '<div class="dialog palette" role="dialog" aria-modal="true" aria-labelledby="terminal-title">'
    + '<div class="palette-search dialog-handle">' + ICONS.terminal
    + '<h2 class="palette-title" id="terminal-title">New terminal</h2>'
    + '<button type="button" class="esc" data-action="add-back" data-key="terminal-back" aria-label="Esc to go back to the list" title="Back to the list (esc)">esc</button></div>'
    + '<div class="palette-list terminal-form" id="palette-list" data-scroll="terminal-form">'
    + field('terminal-command', 'Command', 'Such as claude --continue',
      'Bench types it into your shell and runs it, as if you had typed it. Leave it empty for just the shell.')
    + field('terminal-name', 'Name', 'Such as Claude', 'For the sub-tab. Leave it empty for the title that the terminal sets.')
    + folderField('terminal-folder', 'Where the terminal starts, such as ~/code/bench. Leave it empty for your home folder.')
    + '<div class="toggles">'
    + tick('data-terminal="startAgain"', 'terminal-start-again', 'Start again when Bench opens', t.startAgain, START_AGAIN_HELP)
    + tick('data-terminal="save"', 'terminal-save', 'Save as launcher', t.save,
      'Keep it in config.toml, to add again from the add dialog, which lists launchers first, and Jump to….')
    + '</div></div>'
    + '<div class="palette-actions">'
    + '<button type="button" class="btn small" data-action="add-back" data-key="terminal-cancel">Back</button>'
    + '<button type="button" class="btn small primary" data-action="add-terminal" data-key="terminal-add">Add</button></div>'
    + '<div class="palette-foot dialog-handle"><span><kbd>↵</kbd> add</span><span><kbd>esc</kbd> back</span>'
    + '<span>Opens in this pane</span></div>' + GRIP + '</div>';
}

// setTerminalField keeps what's typed in one of the form's fields, without
// a redraw.
export function setTerminalField(id, value) {
  const add = store.ui.add;
  if (add && add.terminal && TERMINAL_FIELDS[id]) add.terminal[TERMINAL_FIELDS[id]] = value;
}

// setTerminalTick changes one of the form's tick boxes: startAgain or save.
export function setTerminalTick(name, on) {
  const add = store.ui.add;
  if (!add || !add.terminal || !['startAgain', 'save'].includes(name)) return;
  add.terminal[name] = on;
  update();
}

// submitTerminal adds the form's terminal to the dialog's pane, after
// saving it as a launcher if Save as launcher is ticked (SaveLauncher). A
// launcher saved without a name gets one from Go, which the terminal takes
// too. If Go refuses either, the form stays open, so that it can be fixed;
// a launcher that was saved isn't saved again.
export async function submitTerminal() {
  const { ui } = store;
  const add = ui.add;
  if (!add || add.step !== 'terminal' || !add.terminal || add.busy) return;
  const t = add.terminal;
  const source = { type: 'terminal', folder: t.folder.trim(), command: t.command.trim(), name: t.name.trim(), startAgain: t.startAgain };
  add.busy = true;
  try {
    if (t.save) {
      const saved = await act('SaveLauncher', {
        kind: 'terminal', name: source.name, command: source.command, folder: source.folder, startAgain: t.startAgain,
      });
      if (!saved.ok) return;
      t.save = false;
      if (saved.value) source.name = t.name = saved.value;
      if (ui.add === add) update();
    }
    const r = await adding(add, act('AddToPane', add.paneId, source));
    if (r.ok && ui.add === add) {
      ui.paneLater = null;
      closeOverlay();
    }
  } finally {
    add.busy = false;
  }
}

// runItem is the add dialog's row for text that matches nothing: it opens
// the New terminal… form with the text as its command.
export function runItem(command) {
  return {
    group: 'Terminal', mono: TERMINAL_MONO, label: 'Run “' + command + '” in a new terminal…', hint: 'Types it into your shell', keys: '',
    stay: true, offer: true, run: () => openTerminalForm(command),
  };
}

// launcherItems are Jump to…'s rows for the launchers, which add what each
// opens to the focused pane. Typing finds them by name only (matchLabel).
export function launcherItems(view) {
  const launchers = (view.settings && view.settings.launchers) || [];
  return launchers.map((l) => ({
    group: 'Launchers', mono: l.mono || TERMINAL_MONO, icon: l.icon, label: l.name, hint: l.hint || '', keys: '', matchLabel: true,
    run: () => act('AddToPane', focusedPaneID(view), { type: 'launcher', id: l.name }),
  }));
}

// folderTarget returns the draft whose folder a Folder field holds: the
// New terminal… form's, or that of the launcher Settings ▸ Launchers is
// editing; or null if it has gone.
function folderTarget(id) {
  const { ui } = store;
  if (id === 'terminal-folder') return ui.overlay === 'add' && ui.add && ui.add.step === 'terminal' ? ui.add.terminal : null;
  if (id === 'launcher-folder') {
    const d = ui.overlay === 'settings' ? ui.settings : null;
    return d && d.editing >= 0 ? d.launchers[d.editing] || null : null;
  }
  return null;
}

// chooseFolder shows macOS's folder picker for a Folder field (ChooseFolder),
// starting in the folder typed there, and puts the folder chosen in the
// field. The picker is a sheet on Bench's window, so Bench stays raised
// while it's open (ui.choosing), even if the form closes meanwhile, as a
// tab switch closes the add dialog; then the folder chosen is dropped. The
// browser preview has no picker: Go says to type the folder.
export async function chooseFolder(id) {
  const { ui } = store;
  const target = folderTarget(id);
  if (!target || ui.choosing) return;
  ui.choosing = true;
  update();
  let r;
  try {
    r = await act('ChooseFolder', target.folder || '');
  } finally {
    ui.choosing = false;
    update();
  }
  if (folderTarget(id) !== target) return;
  if (r.ok && r.value) {
    target.folder = r.value;
    if (ui.settings && target === ui.settings.launchers[ui.settings.editing]) {
      target.changed = true;
      ui.settings.launchersChanged = true;
    }
    const input = document.getElementById(id);
    if (input) input.value = r.value;
  }
  focusKey(id);
}

// newTerminalIn is the add dialog's New terminal in…: macOS's folder picker
// asks for a folder (ChooseFolder), starting in the one that the row gives,
// the folder of the terminal in the pane, and a new terminal opens there,
// in the dialog's pane. Bench stays raised while the picker is open
// (ui.choosing). If the user cancels, the dialog stays at its list; if the
// dialog has closed or moved on meanwhile, the folder chosen is dropped.
// The browser preview has no picker: Go says to type the folder, and the
// New terminal… form opens at its Folder field.
export async function newTerminalIn(option) {
  const { ui } = store;
  const add = ui.add;
  if (!add || ui.choosing) return;
  const start = (option.source && option.source.folder) || '';
  ui.choosing = true;
  update();
  let r;
  try {
    r = await act('ChooseFolder', start);
  } finally {
    ui.choosing = false;
    update();
  }
  if (store.ui.add !== add || add.step !== 'list') return;
  if (!r.ok) {
    openTerminalForm('', start, '');
    ui.pendingFocus = 'terminal-folder';
    update();
    return;
  }
  if (!r.value) return;
  ui.paneLater = null;
  closeOverlay();
  adding(add, act('AddToPane', add.paneId, { type: 'terminal', folder: r.value }));
}

// launchersDraft returns Settings ▸ Launchers' draft of the launchers, for
// the Settings dialog's draft: a copy of each, which one is being edited
// (-1 for none), and whether any has changed, which SaveSettings then
// sends.
export function launchersDraft(launchers) {
  return { launchers: (launchers || []).map((l) => ({ ...l })), editing: -1, launchersChanged: false };
}

// launchersOut returns the launchers that SaveSettings saves, as Go's
// LauncherView takes them.
export function launchersOut(d) {
  return d.launchers.map((l) => {
    const app = l.kind === 'app';
    return {
      name: (l.name || '').trim(), kind: app ? 'app' : 'terminal', folder: (l.folder || '').trim(),
      command: app ? '' : (l.command || '').trim(), startAgain: !app && l.startAgain !== false,
      bundleId: app ? l.bundleId || '' : '', url: app ? (l.url || '').trim() : '', title: app ? (l.title || '').trim() : '',
    };
  });
}

// sameLaunchers reports whether two lists of launchers, as Go's View has
// them, are the same in config.toml, whatever their icons.
export function sameLaunchers(a, b) {
  return JSON.stringify(launchersOut({ launchers: a || [] })) === JSON.stringify(launchersOut({ launchers: b || [] }));
}

// hintOf says what a launcher opens, as Go's hint does, for one that has
// been changed since Go described it.
function hintOf(l) {
  if (l.kind === 'app') return (l.app || 'App') + ' · ' + ((l.url || '').trim() || (l.folder || '').trim());
  return ((l.command || '').trim() || 'zsh') + ' · ' + ((l.folder || '').trim() || '~');
}

// renderLauncherSettings returns Settings ▸ Launchers: a row for each
// launcher, with Edit and Remove, the fields of the one being edited, and
// New terminal launcher.
export function renderLauncherSettings(d) {
  const rows = d.launchers.map((l, i) => launcherRow(l, i, d.editing === i)).join('');
  return '<p class="settings-help launchers-help" id="launchers-help">'
    + esc('A launcher adds a terminal that runs a command, such as claude, or an app\'s window on a folder or link, each time you choose it. '
      + 'The add dialog lists launchers first, and Jump to… finds them by name. To make an app launcher, add a [[launcher]] to config.toml.') + '</p>'
    + (rows ? '<ul class="launchers" aria-label="Launchers">' + rows + '</ul>' : '')
    + '<div class="launchers-add"><button type="button" class="btn small" data-action="launcher-new" data-key="launcher-new">New terminal launcher</button></div>';
}

// launcherRow returns one launcher's row: its app's icon or its monogram,
// its name, and what it opens, or, while it's edited, its fields.
function launcherRow(l, i, editing) {
  const name = (l.name || '').trim() || 'New launcher';
  const app = l.kind === 'app';
  const text = editing
    ? '<span class="launcher-name">' + esc(app ? (l.app || 'App') + ' launcher' : 'Terminal launcher') + '</span>'
    : '<span class="launcher-name">' + esc(name) + '</span><span class="launcher-hint">' + esc(l.changed ? hintOf(l) : l.hint || hintOf(l)) + '</span>';
  const button = editing
    ? '<button type="button" class="btn small" data-action="launcher-done" data-key="launcher-done">Done</button>'
    : '<button type="button" class="btn small" data-action="launcher-edit" data-index="' + i + '" data-key="launcher-edit-' + i + '" aria-label="Edit ' + esc(name) + '">Edit</button>';
  return '<li class="launcher' + (editing ? ' editing' : '') + '"><div class="launcher-head">'
    + tile(l.mono || TERMINAL_MONO, l.icon)
    + '<span class="launcher-text">' + text + '</span>' + button
    + '<button type="button" class="btn small" data-action="launcher-remove" data-index="' + i + '" data-key="launcher-remove-' + i + '" aria-label="Remove ' + esc(name) + '">Remove</button>'
    + '</div>' + (editing ? launcherFields(l) : '') + '</li>';
}

// launcherFields returns the fields of the launcher being edited: a
// terminal's command, name, folder, and Start again when Bench opens, or
// an app launcher's name, and the folder or link it opens, with the title
// of the window to dock.
function launcherFields(l) {
  const name = field('launcher-name', 'Name', 'Such as Claude', 'The add dialog and Jump to… list it by this name.');
  if (l.kind === 'app') {
    return '<div class="launcher-fields">' + name
      + folderField('launcher-folder', 'A folder to open in ' + (l.app || 'the app') + ', such as ~/code/bench. Leave it empty to open a link instead.')
      + field('launcher-url', 'Link', 'Such as https://docs.example.com', 'A link to open in the app instead of a folder.')
      + field('launcher-title', 'Window title', 'Part of the window\'s title',
        'Bench docks the app\'s window whose title has this. For a folder, leave it empty for the folder\'s name.')
      + '</div>';
  }
  return '<div class="launcher-fields">'
    + field('launcher-command', 'Command', 'Such as claude --continue', 'Bench types it into your shell and runs it. Leave it empty for just the shell.')
    + name
    + folderField('launcher-folder', 'Where the terminal starts. Leave it empty for your home folder.')
    + '<div class="toggles">' + tick('data-launcher="startAgain"', 'launcher-start-again', 'Start again when Bench opens', l.startAgain !== false, START_AGAIN_HELP) + '</div>'
    + '</div>';
}

// editLauncher opens a launcher's fields in Settings ▸ Launchers, closing
// any other's.
export function editLauncher(index) {
  const d = store.ui.settings;
  if (!d || !d.launchers[index]) return;
  d.editing = index;
  store.ui.pendingFocus = d.launchers[index].kind === 'app' ? 'launcher-name' : 'launcher-command';
  update();
}

// doneLauncher closes the launcher's fields, keeping what was typed for
// Save.
export function doneLauncher() {
  const d = store.ui.settings;
  if (!d || d.editing < 0) return;
  const index = d.editing;
  d.editing = -1;
  store.ui.pendingFocus = 'launcher-edit-' + index;
  update();
}

// removeLauncher takes a launcher out of the draft. Save removes it from
// config.toml, and Cancel keeps it.
export function removeLauncher(index) {
  const d = store.ui.settings;
  if (!d || !d.launchers[index]) return;
  d.launchers.splice(index, 1);
  d.launchersChanged = true;
  if (d.editing === index) d.editing = -1;
  else if (d.editing > index) d.editing -= 1;
  const next = Math.min(index, d.launchers.length - 1);
  store.ui.pendingFocus = next < 0 ? 'launcher-new' : (d.editing === next ? 'launcher-done' : 'launcher-edit-' + next);
  update();
}

// newLauncher adds a terminal launcher to the draft and opens its fields.
export function newLauncher() {
  const d = store.ui.settings;
  if (!d) return;
  d.launchers.push({ kind: 'terminal', name: '', command: '', folder: '', startAgain: true, mono: TERMINAL_MONO, changed: true });
  d.editing = d.launchers.length - 1;
  d.launchersChanged = true;
  store.ui.pendingFocus = 'launcher-command';
  update();
}

// setLauncherField keeps what's typed in one of the edited launcher's
// fields, without a redraw.
export function setLauncherField(id, value) {
  const d = store.ui.settings;
  const l = d && d.editing >= 0 ? d.launchers[d.editing] : null;
  if (!l || !LAUNCHER_FIELDS[id]) return;
  l[LAUNCHER_FIELDS[id]] = value;
  l.changed = true;
  d.launchersChanged = true;
}

// setLauncherTick changes the edited launcher's Start again when Bench
// opens.
export function setLauncherTick(on) {
  const d = store.ui.settings;
  const l = d && d.editing >= 0 ? d.launchers[d.editing] : null;
  if (!l) return;
  l.startAgain = on;
  l.changed = true;
  d.launchersChanged = true;
  update();
}

// fillForms puts the drafts' text into newly drawn fields: the New
// terminal… form's, and those of the launcher being edited.
export function fillForms(ui) {
  const fill = (fields, draft) => {
    for (const [id, key] of Object.entries(fields)) {
      const input = document.getElementById(id);
      if (input) input.value = draft[key] || '';
    }
  };
  if (ui.overlay === 'add' && ui.add && ui.add.step === 'terminal' && ui.add.terminal) fill(TERMINAL_FIELDS, ui.add.terminal);
  const d = ui.overlay === 'settings' ? ui.settings : null;
  if (d && d.editing >= 0 && d.launchers[d.editing]) fill(LAUNCHER_FIELDS, d.launchers[d.editing]);
}

// isTerminalField and isLauncherField report whether a field is one of the
// form's or of the edited launcher's, for Return (keyboard.js) and typing
// (app.js).
export const isTerminalField = (id) => Boolean(TERMINAL_FIELDS[id]);
export const isLauncherField = (id) => Boolean(LAUNCHER_FIELDS[id]);
