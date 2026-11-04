// Bench's colours, which the appearance setting chooses: dark, light, or as
// macOS is (system). styles.css has the dark tokens, and light.css the
// light ones, which index.html's <link id="light-tokens"> applies while its
// media query matches. The query starts as (prefers-color-scheme: light),
// which in the app follows the appearance of the page's window, and Go
// gives each of Bench's windows the setting's appearance, so a page starts
// in the right colours before the View comes. setAppearance follows the
// View's appearance from then on: the setting, or the one that Settings
// previews while it's open (decision 66). The browser preview has no
// window for Go to set, and in the app a new appearance reaches the window
// a moment after the View. The page's <html> says which colours are in
// use, as data-theme ("light" or "dark"), and a bench:theme event on the
// document says when they change, for what draws its own colours, such as
// terminals.

const LIGHT = '(prefers-color-scheme: light)';

// MEDIA is the light tokens' media query for each appearance.
const MEDIA = { light: 'all', dark: 'not all', system: LIGHT };

// appearance is the appearance in use; until a View comes, it's as the
// page starts.
let appearance = 'system';

// systemLight reports whether the page's window is light now.
function systemLight() {
  return Boolean(window.matchMedia && window.matchMedia(LIGHT).matches);
}

// themeNow returns the colours in use: light or dark.
export function themeNow() {
  if (appearance === 'light' || appearance === 'dark') return appearance;
  return systemLight() ? 'light' : 'dark';
}

// applyMedia gives the light tokens the media query for the appearance:
// through the style sheet's own media list, which takes effect at once,
// as the link's media attribute takes effect only once the browser has
// read the link again. A sheet that hasn't loaded yet takes the attribute.
function applyMedia() {
  const link = document.getElementById('light-tokens');
  if (!link) return;
  const media = MEDIA[appearance];
  if (link.sheet) {
    if (link.sheet.media.mediaText !== media) link.sheet.media.mediaText = media;
  } else if (link.media !== media) {
    link.media = media;
  }
}

// announce sets data-theme, and fires bench:theme, when the colours in use
// change, or always when told to.
function announce(always = false) {
  const theme = themeNow();
  const root = document.documentElement;
  if (root.dataset.theme === theme && !always) return;
  root.dataset.theme = theme;
  document.dispatchEvent(new CustomEvent('bench:theme', { detail: { theme } }));
}

// sync puts the light tokens' media query and data-theme right, and says
// when the colours change.
function sync() {
  applyMedia();
  announce();
}

// setAppearance follows the View's appearance, system, light, or dark:
// the setting, or the one that Settings previews (decision 66). Anything
// else is system, Go's default.
export function setAppearance(value) {
  const next = Object.hasOwn(MEDIA, value) ? value : 'system';
  if (next === appearance && document.documentElement.dataset.theme) return;
  appearance = next;
  sync();
}

// The window turning light or dark matters while the setting is system.
if (window.matchMedia) {
  const query = window.matchMedia(LIGHT);
  if (query.addEventListener) query.addEventListener('change', sync);
}
// A browser may load a style sheet whose media doesn't match after the
// page's scripts run: the light tokens then arrive late, so what drew
// its colours from the tokens draws them again.
const link = document.getElementById('light-tokens');
if (link && !link.sheet) {
  link.addEventListener('load', () => {
    applyMedia();
    announce(true);
  }, { once: true });
}
sync();
