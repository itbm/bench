// The bridge to Go: bound methods of ui.Service and events from it.
//
// In the app (and the browser preview), calls go through the Wails runtime,
// which the asset server provides at /wails/runtime.js. A test can set
// window.__BENCH_MOCK__ before the page loads, with
// { call(method, args) => Promise, on(event, handler) }, to stand in for Go.

const SERVICE = 'github.com/itbm/bench/internal/ui.Service.';

let runtime = null;

// load returns the Wails runtime, importing it the first time.
function load() {
  if (!runtime) runtime = import('/wails/runtime.js');
  return runtime;
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

// errorText returns the message to show for a rejected call.
export function errorText(err) {
  if (!err) return 'Something went wrong.';
  if (typeof err === 'string') return err;
  if (typeof err.message === 'string' && err.message) return err.message;
  return String(err);
}
