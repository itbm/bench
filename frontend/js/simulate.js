// simulate.js: the browser preview's Simulate, on the demo bar, which
// shows below it the things that the preview can pretend happen, as on a
// Mac: Minimise window, Bring Bench forward, Unplug monitor (or Plug in
// monitor), VoiceOver, Rectangle, and Keep moving window. They're a list
// of buttons that Simulate shows and hides, which Tab reaches in turn.
// Choosing one hides them, and so do Escape, which gives the focus back to
// Simulate, a click elsewhere, and the focus leaving them. So the demo bar
// keeps room for its hint.

const group = () => document.getElementById('demo-simulate-group');
const button = () => document.getElementById('demo-simulate');
const list = () => document.getElementById('demo-simulate-menu');

// simulateShown reports whether Simulate's list shows.
export function simulateShown() {
  const l = list();
  return Boolean(l && !l.hidden);
}

// showSimulate shows Simulate's list, with the focus on its first button,
// or hides it.
export function showSimulate(shown = !simulateShown()) {
  const b = button();
  const l = list();
  if (!b || !l) return;
  l.hidden = !shown;
  b.setAttribute('aria-expanded', String(shown));
  if (shown) {
    const first = l.querySelector('button');
    if (first) first.focus();
  }
}

// hideSimulate hides Simulate's list, giving the focus back to Simulate
// with refocus.
export function hideSimulate(refocus = false) {
  if (!simulateShown()) return;
  showSimulate(false);
  if (refocus && button()) button().focus();
}

// simulateAction wraps one of the list's actions: the list hides, then
// the action runs. From the keyboard (detail 0), the focus goes back to
// Simulate.
export function simulateAction(run) {
  return (target, e) => {
    hideSimulate(Boolean(e) && e.detail === 0);
    run(target, e);
  };
}

// installSimulate hides the list as the pointer goes down elsewhere, as
// the focus leaves it, and at Escape in it.
export function installSimulate() {
  const g = group();
  if (!g) return;
  document.addEventListener('pointerdown', (e) => {
    if (simulateShown() && !(e.target instanceof Element && g.contains(e.target))) hideSimulate(false);
  }, true);
  g.addEventListener('focusout', (e) => {
    if (simulateShown() && e.relatedTarget instanceof Element && !g.contains(e.relatedTarget)) hideSimulate(false);
  });
  g.addEventListener('keydown', (e) => {
    if (e.key !== 'Escape' || !simulateShown()) return;
    e.preventDefault();
    e.stopPropagation();
    hideSimulate(true);
  });
}
