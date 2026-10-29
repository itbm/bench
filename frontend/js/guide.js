// The first-run guide to permissions, and the banner shown while Bench
// doesn't have Accessibility permission.

import { act, closeOverlay, isDemo, openerKey, store, update } from './state.js';
import { esc, ICONS } from './util.js';

// openGuide opens the first-run guide.
export function openGuide() {
  const { ui } = store;
  if (ui.overlay === 'guide') return;
  if (!ui.overlay) ui.opener = openerKey();
  ui.menu = null;
  ui.paneLater = null;
  ui.add = null;
  ui.editor = null;
  ui.settings = null;
  ui.overlay = 'guide';
  ui.pendingFocus = store.view && store.view.permissions && store.view.permissions.accessibility ? 'guide-continue' : 'guide-allow';
  update();
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
  ui.pendingFocus = view.permissions && view.permissions.accessibility ? 'guide-continue' : 'guide-allow';
}

// dismissGuide closes the guide for good.
export function dismissGuide() {
  store.ui.guideClosed = true;
  closeOverlay();
  act('DismissFirstRun');
}

// renderGuide returns the first-run guide's HTML.
export function renderGuide(view) {
  const p = view.permissions || {};
  const allow = p.accessibility
    ? '<span class="guide-ok">' + ICONS.check + 'Bench has Accessibility permission.</span>'
    : '<button type="button" class="btn primary" data-action="request-accessibility" data-key="guide-allow">Turn on Accessibility</button>';
  const recording = p.screenRecording
    ? '<span class="guide-ok">' + ICONS.check + 'Bench has Screen Recording permission.</span>'
    : '';
  // The heading and Continue stay in view, and the steps between them
  // scroll if the window is short.
  return '<div class="dialog guide-sheet" role="dialog" aria-modal="true" aria-labelledby="guide-title" aria-describedby="guide-intro">'
    + '<div class="guide-head"><h2 id="guide-title">Welcome to Bench</h2>'
    + '<p id="guide-intro">Bench gathers your apps into one window with tabs. Before you start, it needs one permission from macOS.</p></div>'
    + '<div class="dialog-body" data-scroll="guide-body">'
    + '<div class="guide-step"><span class="mono-tile" aria-hidden="true">1</span><div>'
    + '<h3>Accessibility</h3>'
    + '<p>Bench needs Accessibility permission to arrange other apps\' windows: to move and size them into its panes, and to hide the ones that belong to other tabs. macOS asks you to allow Bench in System Settings.</p>'
    + '<div class="guide-action">' + allow + '</div></div></div>'
    + '<div class="guide-step"><span class="mono-tile" aria-hidden="true">2</span><div>'
    + '<h3>Screen Recording (optional)</h3>'
    + '<p>Screen Recording is only for live previews of windows when you hover a sub-tab. Previews are off until you turn them on in Settings, and macOS asks for permission then.</p>'
    + (recording ? '<div class="guide-action">' + recording + '</div>' : '')
    + '</div></div></div>'
    + '<div class="editor-foot"><div class="spacer">'
    + '<button type="button" class="btn' + (p.accessibility ? ' primary' : '') + '" data-action="dismiss-first-run" data-key="guide-continue">Continue</button>'
    + '</div></div></div>';
}

// renderBanner returns the Accessibility banner's HTML, or '' if it isn't
// needed.
export function renderBanner(view) {
  if (isDemo() || !view.permissions || view.permissions.accessibility) return '';
  return '<span class="banner-text">' + esc('Bench can\'t arrange other apps\' windows until it has Accessibility permission.') + '</span>'
    + '<button type="button" class="btn small" data-action="request-accessibility" data-key="banner-allow">Turn on Accessibility</button>';
}

// requestAccessibility asks macOS for Accessibility permission.
export function requestAccessibility() {
  act('RequestAccessibility');
}
