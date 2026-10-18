// Bench's UI: the entry point. It asks Go for the View, draws it, and turns
// clicks, keys, and drags into calls to Go's bound methods.

import { call, errorText, on, onConnected } from './bridge.js';
import {
  act, closeMenu, closeOverlay, isDemo, resendOverlay, setRenderer, store, toast, update,
} from './state.js';
import { focusedPaneID, sentence } from './util.js';
import { measure, render } from './render.js';
import { installDrag } from './drag.js';
import { hidePreview, installPreviews } from './preview.js';
import { installKeyboard } from './keyboard.js';
import { openNotices, openWindowMenu, runMenuEntry } from './menu.js';
import { backToList, openAdd, openPalette, runListItem, togglePalette } from './palette.js';
import {
  deleteTab, explainShortcut, openEditor, resetLayout, save, setField, setPreset, setSmallScreens, setToggle,
} from './editor.js';
import { dismissGuide, openGuide, requestAccessibility } from './guide.js';
import * as terminals from './terminals.js';
import { bootTerminalWindow, terminalID } from './terminal-window.js';

// stateEvents counts bench:state events, so that ready can tell whether one
// arrived while Ready was on its way.
let stateEvents = 0;

// readyAsked is set once the UI has first asked Go for the View.
let readyAsked = false;

// demoWidthSent is set once the browser preview has told Go the browser's
// width.
let demoWidthSent = false;

// setView draws a View from Go.
function setView(v) {
  if (!v || !Array.isArray(v.tabs) || !v.tabs.length) return;
  store.view = v;
  update();
  sendDemoWidth(v);
}

// onState draws a bench:state event's View.
function onState(v) {
  stateEvents++;
  setView(v);
}

// sendDemoWidth tells Go the browser's width the first time the browser
// preview shows "Fit to browser", since Go can't measure it.
function sendDemoWidth(v) {
  if (demoWidthSent || !v.demo) return;
  demoWidthSent = true;
  if ((v.demo.screen || 'fit') === 'fit') act('SetDemoScreen', 'fit', window.innerWidth);
}

// addPane is the pane the add dialog adds to: the compact pane stands for
// the tab's focused pane.
function addPane(paneId) {
  return paneId === 'compact' ? focusedPaneID(store.view) || paneId : paneId;
}

// ACTIONS run the controls with a data-action.
const ACTIONS = {
  'screen': (t) => act('SetDemoScreen', t.dataset.screen, window.innerWidth),
  'tab': (t) => act('SwitchTab', Number(t.dataset.index)),
  'mode': (t) => act('SetMode', t.dataset.mode),
  'activate': (t) => act('ShowItem', t.dataset.pane, t.dataset.item),
  'split-right': (t) => {
    const reason = store.view.display && store.view.display.splitsDisabled;
    if (reason) toast(sentence(reason));
    else act('MoveItem', t.dataset.item, store.view.current, t.dataset.pane, 'right');
  },
  'menu': (t) => {
    const m = store.ui.menu;
    if (m && m.kind === 'window' && m.shownIn === t.dataset.pane) closeMenu(true);
    else openWindowMenu(t, t.dataset.item, t.dataset.pane);
  },
  'menu-run': (t) => runMenuEntry(Number(t.dataset.index)),
  'notices': (t) => {
    if (store.ui.menu && store.ui.menu.kind === 'notices') closeMenu(true);
    else openNotices(t);
  },
  'add': (t) => openAdd(addPane(t.dataset.pane)),
  'open-palette': () => openPalette(),
  'new-tab': () => openEditor(-1),
  'edit-tab': () => openEditor(store.view.current),
  'close': () => closeOverlay(),
  'add-back': () => backToList(),
  'palette-run': (t) => runListItem(Number(t.dataset.index)),
  'draft-preset': (t) => setPreset(t.dataset.preset),
  'draft-small': (t) => setSmallScreens(t.dataset.small),
  'shortcut': () => explainShortcut(),
  'save-tab': () => save(),
  'delete-tab': () => deleteTab(),
  'reset-layout': () => resetLayout(),
  'redock': (t) => act('Redock', t.dataset.item),
  'request-accessibility': () => requestAccessibility(),
  'dismiss-first-run': () => dismissGuide(),
};

function onClick(e) {
  const { ui, view } = store;
  if (!view) return;
  const target = e.target.closest('[data-action]');
  const action = target ? target.dataset.action : null;

  // Clicking anywhere in a pane focuses it. Showing an item focuses its
  // pane anyway.
  const paneEl = e.target.closest('.pane[data-pane-id]');
  if (paneEl && !ui.overlay && action !== 'activate') {
    const id = paneEl.dataset.paneId;
    if (id !== 'compact' && view.display && id !== view.display.focus) act('FocusPane', id);
  }
  // Clicking outside an open menu closes it.
  if (ui.menu && !e.target.closest('.menu') && action !== 'menu' && action !== 'notices') {
    closeMenu(false);
    if (!target) return;
  }
  if (!target) return;
  if (action === 'backdrop') {
    if (e.target === target) closeOverlay();
    return;
  }
  const run = ACTIONS[action];
  if (run) run(target, e);
}

// onContextMenu shows a right-clicked sub-tab's window and opens its menu.
function onContextMenu(e) {
  const sub = e.target.closest && e.target.closest('.subtab');
  if (!sub || !store.view) return;
  e.preventDefault();
  hidePreview();
  const { pane, item } = sub.dataset;
  if (sub.getAttribute('aria-pressed') !== 'true') act('ShowItem', pane, item);
  const button = document.querySelector('.pane[data-pane-id="' + CSS.escape(pane) + '"] [data-action="menu"]');
  if (button) openWindowMenu(button, item, pane);
}

function onInput(e) {
  const t = e.target;
  const { ui } = store;
  if (t.id === 'palette-query') {
    ui.palette.query = t.value;
    ui.palette.sel = 0;
    update();
  } else if (t.id === 'palette-url' && ui.add) {
    ui.add.url = t.value;
  } else if (t.id === 'tab-name') {
    setField('name', t.value);
  } else if (t.id === 'compact-below') {
    setField('compactBelow', t.value);
  }
}

function onChange(e) {
  const t = e.target;
  if (store.ui.editor && t.dataset && t.dataset.setting) setToggle(t.dataset.setting, t.checked);
}

// runCommand does what a bench:command asks, such as opening the palette
// for the jump shortcut.
function runCommand(cmd) {
  if (!store.view) return;
  const name = cmd && typeof cmd === 'object' ? cmd.name : cmd;
  switch (name) {
    case 'toggle-palette': togglePalette(); break;
    case 'open-palette': if (store.ui.overlay !== 'palette') openPalette(); break;
    case 'new-tab': openEditor(-1); break;
    case 'edit-tab': openEditor(store.view.current); break;
    case 'first-run':
      store.ui.guideClosed = false;
      openGuide();
      break;
    default: break;
  }
}

// loadFonts waits briefly for the UI fonts, so that terminals measure the
// right font.
function loadFonts() {
  if (!document.fonts || !document.fonts.load) return Promise.resolve();
  const fonts = Promise.all([
    document.fonts.load('400 13px "IBM Plex Sans"'),
    document.fonts.load('500 13px "IBM Plex Sans"'),
    document.fonts.load('600 13px "IBM Plex Sans"'),
    document.fonts.load('400 13px "IBM Plex Mono"'),
    document.fonts.load('500 13px "IBM Plex Mono"'),
  ]).catch(() => {});
  return Promise.race([fonts, new Promise((r) => setTimeout(r, 1500))]);
}

// ready asks Go for the View. Go sends bench:state after every change, so a
// state that arrives while Ready is on its way is at least as new as Ready's
// answer, or is followed by one that is: Ready's answer is then dropped
// rather than drawn over it.
async function ready() {
  readyAsked = true;
  for (let attempt = 1; ; attempt++) {
    const seen = stateEvents;
    try {
      const v = await call('Ready');
      if (stateEvents === seen) setView(v);
      return;
    } catch (err) {
      // A state from Go makes Ready unnecessary.
      if (store.view) return;
      if (attempt >= 3) {
        toast(errorText(err));
        return;
      }
      await new Promise((r) => setTimeout(r, 500 * attempt));
    }
  }
}

// catchUp asks Go for the View again when the browser preview's event
// socket opens, because events sent before it opened are lost. Ready lowers
// Bench's window, so it then tells Go again whether a menu or dialog is
// open.
async function catchUp() {
  if (!readyAsked) return;
  const seen = stateEvents;
  try {
    const v = await call('Ready');
    if (stateEvents === seen) setView(v);
  } catch {
    // The next bench:state brings the View.
  }
  resendOverlay();
}

function watchSize() {
  new ResizeObserver(() => measure()).observe(document.getElementById('panes'));
  let timer = null;
  window.addEventListener('resize', () => {
    measure();
    if (!isDemo() || (store.view.demo.screen || 'fit') !== 'fit') return;
    clearTimeout(timer);
    timer = setTimeout(() => act('SetDemoScreen', 'fit', window.innerWidth), 100);
  });
}

async function boot() {
  setRenderer(render);
  document.addEventListener('click', onClick);
  document.addEventListener('contextmenu', onContextMenu);
  document.addEventListener('input', onInput);
  document.addEventListener('change', onChange);
  installDrag();
  installPreviews();
  installKeyboard();
  onConnected(catchUp);
  try {
    await Promise.all([
      on('bench:state', onState),
      on('bench:toast', (msg) => toast(typeof msg === 'string' ? msg : msg && msg.message)),
      on('bench:command', runCommand),
      on('term:data', (d) => terminals.write(d)),
    ]);
  } catch (err) {
    toast(errorText(err));
    return;
  }
  await loadFonts();
  await ready();
  watchSize();
}

// For debugging in the browser console, and for tests.
window.bench = { store, render: update, terminalCount: terminals.count };

// A popped-out terminal's window loads this page at /?terminal=<item ID>,
// to show just that terminal.
const popped = terminalID();
if (popped) bootTerminalWindow(popped);
else boot();
