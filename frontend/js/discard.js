// The question before Bench throws away what a dialog has that isn't saved
// (decision 66): Settings, Tab properties, New tab, and the add dialog's
// web address and New terminal… form. Escape, Cancel, or a click outside
// such a dialog with changes, or a command that opens another dialog in its
// place, asks first, as "Discard changes to Settings?", over the dialog:
// Save, the default, saves the changes as the dialog's own button does and
// then goes on; Discard throws them away and goes on; Keep editing, and
// Escape, go back to the dialog as it was. A dialog with no changes closes
// at once, as before. In the add dialog, Escape and Back go back to its
// list, and ask first in the same way, where Discard goes back to the list
// (backToList). Jump to…'s search isn't kept: it's no more than a search.

import { closeOverlay, store, update } from './state.js';
import { activeKey, esc, joinNames } from './util.js';
import { changedSections, saveSettings, sectionLabel } from './settings.js';
import { canSave, editorChanged, save as saveTab, saveLabel } from './editor.js';
import { submitURL } from './palette.js';
import { submitTerminal, terminalChanged } from './launchers.js';
import { stopRecording } from './keyboard-settings.js';

// unsaved returns what the open dialog would lose if it closed now:
// 'settings', 'tab' (Tab properties), 'new-tab', 'url' (the add dialog's
// web address), or 'terminal' (its New terminal… form), or '' if nothing.
export function unsaved(view, ui) {
  switch (ui.overlay) {
    case 'settings': return ui.settings && changedSections(view, ui.settings).length ? 'settings' : '';
    case 'editor':
      if (!editorChanged(ui.editor)) return '';
      return ui.editor.index < 0 ? 'new-tab' : 'tab';
    case 'add': {
      const add = ui.add;
      if (!add) return '';
      if (add.step === 'url' && (add.url || '').trim()) return 'url';
      if (add.step === 'terminal' && terminalChanged(add.terminal)) return 'terminal';
      return '';
    }
    default: return '';
  }
}

// draftOf returns the draft of a dialog of a kind (ui.overlay), to tell
// whether it has closed.
function draftOf(ui, kind) {
  if (kind === 'settings') return ui.settings;
  if (kind === 'editor') return ui.editor;
  if (kind === 'add') return ui.add;
  return null;
}

// askDiscard asks before the open dialog loses its changes, if it has any,
// and reports whether it asked, or is asking already. next, if given, runs
// once the changes are saved or discarded. With stays, for the add
// dialog's Escape and Back, Discard leaves the dialog open, and next takes
// it back a step, to its list; Save, which closes it, doesn't run next.
export function askDiscard(next = null, stays = false) {
  const { ui, view } = store;
  if (ui.overlay === 'discard') return true;
  const what = unsaved(view, ui);
  if (!what) return false;
  // Settings ▸ Keyboard stops recording keys, so that Bench's global
  // shortcuts come back.
  if (what === 'settings') stopRecording();
  ui.menu = null;
  ui.discard = { back: ui.overlay, what, next, stays, focus: activeKey() };
  ui.overlay = 'discard';
  ui.pendingFocus = saveOf(view, ui) ? 'discard-save' : 'discard-keep';
  update();
  return true;
}

// dismissOverlay closes the open dialog, as Escape, Cancel, and a click
// outside do, after asking if it has changes that aren't saved (askDiscard),
// and then runs next, if given.
export function dismissOverlay(next = null) {
  if (askDiscard(next)) return;
  closeOverlay();
  if (next) next();
}

// afterAsking runs next, which opens a dialog in the place of the open one
// or closes it, at once, or once the open dialog's changes are saved or
// discarded. While this question, or the question before quitting, is
// open, it does nothing: the question comes first, and the question before
// quitting may cover a dialog with changes (quit.js). Settings ▸ Keyboard
// stops recording keys first, so that Bench's global shortcuts come back:
// the dialog that takes its place drops its draft, and with it the field
// that records, and a click in the menu bar is no click in the page
// (stopRecordingFor).
export function afterAsking(next) {
  const open = store.ui.overlay;
  if (open === 'discard' || open === 'quit') return;
  if (open === 'settings') stopRecording();
  if (!askDiscard(next)) next();
}

// keepEditing goes back to the dialog that the question covers, as it was,
// with the focus where it was: Keep editing, and Escape.
export function keepEditing() {
  const { ui } = store;
  const q = ui.discard;
  if (!q) return;
  ui.discard = null;
  ui.overlay = q.back;
  ui.pendingFocus = q.focus;
  update();
}

// discardChanges closes the dialog that the question covers, throwing its
// changes away, and goes on with what asked: Discard. A New tab dialog that
// the welcome guide opened goes back to the guide (closeOverlay). Settings'
// preview ends as the dialog closes. The add dialog's Escape and Back
// leave the dialog open, for next to take it back to its list (stays).
export function discardChanges() {
  const { ui } = store;
  const q = ui.discard;
  if (!q) return;
  ui.discard = null;
  ui.overlay = q.back;
  if (!q.stays) closeOverlay();
  if (q.next) q.next();
}

// SAVES are what each kind of dialog's button that saves it does.
const SAVES = {
  settings: saveSettings,
  tab: saveTab,
  'new-tab': saveTab,
  url: () => submitURL(),
  terminal: submitTerminal,
};

// saveChanges goes back to the dialog that the question covers and saves
// it, as the dialog's own button does: Save. If that closes the dialog,
// what asked goes on; if Go refuses, as for a name or an address it can't
// use, the dialog stays open with the reason, to fix.
export async function saveChanges() {
  const { ui, view } = store;
  const q = ui.discard;
  if (!q || !saveOf(view, ui)) return;
  ui.discard = null;
  ui.overlay = q.back;
  ui.pendingFocus = q.focus;
  const draft = draftOf(ui, q.back);
  update();
  await SAVES[q.what]();
  if (q.next && !q.stays && draft && draftOf(store.ui, q.back) !== draft && store.ui.overlay !== 'discard') q.next();
}

// saveOf returns the label of the question's button that saves, as the
// covered dialog's own button reads, or '' if the dialog can't be saved
// now, as a new tab of windows with none ticked: the question then has no
// such button.
function saveOf(view, ui) {
  const q = ui.discard;
  switch (q && q.what) {
    case 'settings': return 'Save';
    case 'tab':
    case 'new-tab': return ui.editor && canSave(view, ui.editor) ? saveLabel(ui.editor) : '';
    case 'url':
    case 'terminal': return 'Add';
    default: return '';
  }
}

// questionOf returns the question's title and text for what it would
// discard.
function questionOf(view, ui) {
  const q = ui.discard;
  switch (q.what) {
    case 'settings': {
      const names = ui.settings ? changedSections(view, ui.settings).map(sectionLabel) : [];
      return {
        title: 'Discard changes to Settings?',
        text: names.length ? 'Your changes to ' + joinNames(names) + ' aren\'t saved.' : 'Your changes aren\'t saved.',
      };
    }
    case 'tab':
      return { title: 'Discard changes to Tab properties?', text: 'Your changes to the ' + ((ui.editor && ui.editor.tabName) || 'tab') + ' tab aren\'t saved.' };
    case 'new-tab':
      return { title: 'Discard the new tab?', text: 'Bench hasn\'t made the tab yet.' };
    case 'url':
      return { title: 'Discard the web address?', text: '“' + ((ui.add && ui.add.url) || '').trim() + '” isn\'t added yet.' };
    case 'terminal':
      return { title: 'Discard the new terminal?', text: 'The terminal isn\'t added yet.' };
    default:
      return { title: 'Discard changes?', text: '' };
  }
}

// renderDiscard returns the question's HTML: an alert dialog over the one
// it covers, with Discard at the left, and Keep editing and Save at the
// right, Save the default, as a Mac's question before closing a document
// that isn't saved has them.
export function renderDiscard(view, ui) {
  if (!ui.discard) return '';
  const { title, text } = questionOf(view, ui);
  const save = saveOf(view, ui);
  return '<div class="dialog question-sheet discard-sheet" role="alertdialog" aria-modal="true" aria-labelledby="discard-title"'
    + (text ? ' aria-describedby="discard-text"' : '') + '>'
    + '<div class="question-head"><h2 id="discard-title">' + esc(title) + '</h2>'
    + (text ? '<p id="discard-text">' + esc(text) + '</p>' : '') + '</div>'
    + '<div class="editor-foot">'
    + '<button type="button" class="btn" data-action="discard-discard" data-key="discard-discard">Discard</button>'
    + '<div class="spacer">'
    + '<button type="button" class="btn" data-action="discard-keep" data-key="discard-keep">Keep editing</button>'
    + (save ? '<button type="button" class="btn primary" data-action="discard-save" data-key="discard-save">' + esc(save) + '</button>' : '')
    + '</div></div></div>';
}

// coveredUI returns the UI state as the dialog that the question covers
// sees it, to draw and fill that dialog under the question, or ui itself
// when no question is open.
export function coveredUI(ui) {
  return ui.overlay === 'discard' && ui.discard ? { ...ui, overlay: ui.discard.back } : ui;
}
