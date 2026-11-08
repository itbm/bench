// The bridge to Go: bound methods of ui.Service and events from it.
//
// In the app (and the browser preview), calls go through the Wails runtime,
// which the asset server provides at /wails/runtime.js. A test can set
// window.__BENCH_MOCK__ before the page loads, with
// { call(method, args) => Promise, on(event, handler) }, to stand in for Go.

const SERVICE = 'github.com/itbm/bench/internal/ui.Service.';

let runtime = null;
let connected = null;

// load returns the Wails runtime, importing it the first time.
function load() {
  if (!runtime) {
    watchEventSocket();
    runtime = import('/wails/runtime.js');
  }
  return runtime;
}

// watchEventSocket notices each time the browser preview's event socket
// opens. In Wails' server mode, events come over a WebSocket to
// /wails/events that the runtime opens a moment after it loads, and opens
// again if it drops; events sent while it isn't open are lost. The app's
// web view gets events directly and never opens one.
function watchEventSocket() {
  const Native = window.WebSocket;
  if (!Native) return;
  window.WebSocket = class extends Native {
    constructor(...args) {
      super(...args);
      if (String(args[0]).includes('/wails/events')) {
        this.addEventListener('open', () => { if (connected) connected(); });
      }
    }
  };
}

// runtimeReady resolves once the Wails runtime has loaded, which sets up
// window._wails, or at once with a test's stand-in for Go, which loads no
// runtime.
export function runtimeReady() {
  return mock() ? Promise.resolve() : load();
}

// onConnected calls handler each time the browser preview's event socket
// opens, so that the UI can catch up on events it missed.
export function onConnected(handler) {
  connected = handler;
}

function mock() {
  return typeof window !== 'undefined' ? window.__BENCH_MOCK__ : undefined;
}

// call calls a bound method of ui.Service by name. It resolves with the
// method's result, or rejects with an Error whose message is Go's.
export async function call(method, ...args) {
  const m = mock();
  if (m) return m.call(method, args);
  const { Call } = await load();
  return Call.ByName(SERVICE + method, ...args);
}

// on calls handler with each event's data.
export async function on(event, handler) {
  const m = mock();
  if (m) {
    m.on(event, handler);
    return;
  }
  const { Events } = await load();
  Events.On(event, (e) => handler(e.data));
}

// clipboardText resolves with the text on the clipboard, and
// setClipboardText puts text there, for a terminal's Copy and Paste menu
// entries, which, unlike ⌘C and ⌘V, come with no copy or paste event. In the
// app they use the Wails runtime, as a page may not read the clipboard
// itself; in the browser preview (browser set), whose Go has no clipboard,
// they use the browser's. A test's stand-in for Go can give its own
// ({ text(), setText(text) }).
export async function clipboardText(browser) {
  const m = mock();
  if (m && m.clipboard) return m.clipboard.text();
  if (browser || m) return navigator.clipboard ? navigator.clipboard.readText() : '';
  const { Clipboard } = await load();
  return Clipboard.Text();
}

export async function setClipboardText(text, browser) {
  const m = mock();
  if (m && m.clipboard) return m.clipboard.setText(text);
  if (browser || m) return navigator.clipboard ? navigator.clipboard.writeText(text) : undefined;
  const { Clipboard } = await load();
  return Clipboard.SetText(text);
}

// errorText returns the message to show for a rejected call.
export function errorText(err) {
  if (!err) return 'Something went wrong.';
  if (typeof err === 'string') return err;
  if (typeof err.message === 'string' && err.message) return err.message;
  return String(err);
}
