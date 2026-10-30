// An app's or a page's tile: the square before a sub-tab's title, and
// before each row of Jump to…, the add dialog, Settings ▸ Launchers, and
// the welcome guide's starter tabs. It shows the icon whose address Go
// gives (ItemView's icon: an app's, which macOS draws, or a page's, which
// Go fetched), and the two-letter monogram until the icon has loaded, or
// for good if there's no icon or it doesn't load: a terminal, a page with
// none, an app that isn't installed, and everything in the browser
// preview.

import { esc } from './util.js';

// loaded and failed hold the addresses of the icons that have loaded, and
// of those that couldn't. A tile whose icon has loaded draws it at once
// when the window is drawn again, from the browser's cache, with no
// monogram first. One that couldn't is drawn with the monogram alone. A
// page's icon gets a new address each time it changes.
const loaded = new Set();
const failed = new Set();

// tile returns a tile's HTML: the icon, if there is one, then the monogram,
// then extra, such as a sub-tab's status marker.
export function tile(mono, icon, extra = '') {
  const src = icon && !failed.has(icon) ? icon : '';
  const has = src && loaded.has(src);
  return '<span class="mono-tile' + (has ? ' has-icon' : '') + '" aria-hidden="true">'
    + (src ? '<img class="tile-icon" src="' + esc(src) + '" alt="" draggable="false">' : '')
    + '<span class="tile-mono">' + esc(mono || '') + '</span>' + extra + '</span>';
}

// installTiles follows icons as they load, or don't. Images' load and error
// events don't bubble, so they're caught on their way down.
export function installTiles() {
  document.addEventListener('load', onLoad, true);
  document.addEventListener('error', onError, true);
}

function tileIcon(e) {
  const img = e.target;
  return img instanceof HTMLImageElement && img.classList.contains('tile-icon') ? img : null;
}

// onLoad shows an icon that has loaded in place of its tile's monogram.
function onLoad(e) {
  const img = tileIcon(e);
  if (!img) return;
  loaded.add(img.getAttribute('src'));
  if (img.parentElement) img.parentElement.classList.add('has-icon');
}

// onError leaves the monogram of an icon that couldn't load.
function onError(e) {
  const img = tileIcon(e);
  if (!img) return;
  failed.add(img.getAttribute('src'));
  if (img.parentElement) img.parentElement.classList.remove('has-icon');
  img.remove();
}
