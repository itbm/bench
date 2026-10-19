// Placeholder window contents for the browser preview, from the prototype:
// grey shapes stand in for other apps, and the code and terminal text is
// illustrative. The app never shows these.

import { esc } from './util.js';

// Kinds of placeholder, by app name. Apps not listed get one by item kind.
const KINDS = {
  'Slack': 'chat',
  'Outlook': 'mail',
  'Mail': 'mail',
  'Web page': 'browser',
  'Terminal': 'terminal',
  'Zed': 'code',
  'VS Code': 'code',
  'Visual Studio Code': 'code',
  'Xcode': 'code',
  'Brave': 'browser',
  'Brave Browser': 'browser',
  'Safari': 'browser',
  'Obsidian': 'notes',
  'Notes': 'notes',
};

// kindFor chooses an item's placeholder by its app's name, as the prototype
// does, falling back on its kind. Only KINDS' own keys count: a web item's
// app name can be any word, such as "constructor".
export function kindFor(item) {
  if (Object.hasOwn(KINDS, item.app)) return KINDS[item.app];
  if (item.kind === 'web') return 'browser';
  if (item.kind === 'builtin') return 'terminal';
  return 'generic';
}

// placeholderFor returns the placeholder HTML for an item.
export function placeholderFor(item) {
  return CONTENT[kindFor(item)](item);
}

const CONTENT = {
  code: (it) => '<div class="code">'
    + '<div class="files" aria-label="Files"><div class="files-title">' + esc(String(it.title || '').split(' — ').pop()) + '</div>'
    + '<div>cmd/bench/main.go</div><div>internal/mac/ax_darwin.go</div><div>internal/layout/tree.go</div>'
    + '<div class="current">internal/workspace/switch.go</div><div>internal/config/config.go</div><div>config.toml</div></div>'
    + '<div class="editor"><div class="gutter" aria-hidden="true">'
    + Array.from({ length: 13 }, (_, k) => '<div>' + (k + 1) + '</div>').join('')
    + '</div><div class="lines">'
    + '<div><span class="k">package </span>workspace</div>'
    + '<div>&#8203;</div>'
    + '<div class="c">// MoveWindow puts a window in a pane, or beside it.</div>'
    + '<div><span class="k">func </span>(w *<span class="t">Workspace</span>) <span class="f">MoveWindow</span>(id <span class="t">WindowID</span>, to <span class="t">Target</span>) <span class="t">error</span> {</div>'
    + '<div class="i1">pane := w.<span class="f">paneOf</span>(id)</div>'
    + '<div class="i1">pane.<span class="f">Remove</span>(id)</div>'
    + '<div class="i1"><span class="k">if </span>to.Zone == <span class="t">Center</span> {</div>'
    + '<div class="i2"><span class="k">return </span>to.Pane.<span class="f">Add</span>(id)</div>'
    + '<div class="i1">}</div>'
    + '<div class="i1">fresh := w.Tab.<span class="f">Split</span>(to.Pane, to.Zone)</div>'
    + '<div class="i1"><span class="k">return </span>fresh.<span class="f">Add</span>(id)</div>'
    + '<div>}</div>'
    + '<div>&#8203;</div>'
    + '</div></div></div>',

  terminal: (it) => '<div class="terminal">'
    + '<div class="dim">~/code/bench on main</div>'
    + (String(it.title || '').startsWith('go test')
      ? '<div><span class="prompt">$ </span>go test ./...</div>'
        + '<div><span class="ok">ok   </span>bench/internal/layout    <span class="dim">(cached)</span></div>'
        + '<div><span class="ok">ok   </span>bench/internal/workspace <span class="dim">(cached)</span></div>'
      : '<div><span class="prompt">$ </span>go run ./cmd/bench</div>'
        + '<div class="dim">bench: 3 tabs loaded from config.toml</div>'
        + '<div class="dim">bench: large screen, using wide layouts</div>')
    + '<div><span class="prompt">$ </span><span class="cursor" aria-hidden="true"></span></div>'
    + '</div>',

  chat: () => '<div class="sk-row" aria-hidden="true">'
    + '<div class="sk-col sk-side">' + bars(['72%', '58%', '66%', '50%', '62%', '44%'], 'nav', true) + '</div>'
    + '<div class="sk-col sk-main">'
    + msg('28%', ['82%', '64%']) + msg('22%', ['74%']) + msg('34%', ['90%', '70%', '40%']) + msg('26%', ['58%'])
    + '<div class="composer"></div></div></div>',

  mail: () => '<div class="sk-row" aria-hidden="true">'
    + '<div class="sk-col sk-side" style="flex-basis: 120px">' + bars(['70%', '55%', '62%', '48%'], 'nav', true) + '</div>'
    + '<div class="sk-col mail-list">'
    + mailItem('46%', '84%', true) + mailItem('38%', '76%') + mailItem('52%', '68%') + mailItem('30%', '80%') + mailItem('44%', '60%')
    + '</div>'
    + '<div class="sk-col reading">'
    + '<div class="bar title" style="width: 58%"></div><div class="bar head" style="width: 30%"></div><div style="height: 18px"></div>'
    + bars(['92%', '86%', '90%', '54%'])
    + '</div></div>',

  notes: () => '<div class="sk-row" aria-hidden="true">'
    + '<div class="sk-col sk-side" style="flex-basis: 150px">' + bars(['60%', '72%', '50%', '66%', '42%'], 'nav', true) + '</div>'
    + '<div class="sk-col" style="flex: 1 1 auto; gap: 10px; padding: 24px 28px">'
    + '<div class="bar title" style="width: 40%; height: 16px"></div><div style="height: 10px"></div>'
    + bars(['88%', '80%', '64%']) + '<div style="height: 10px"></div>'
    + ['46%', '38%', '52%'].map((w) => '<div class="check"><span></span><div class="bar" style="width: ' + w + '"></div></div>').join('')
    + '</div></div>',

  browser: (it) => '<div class="sk-col" style="flex: 1 1 auto">'
    + '<div class="urlbar"><span class="dot" aria-hidden="true"></span><span class="dot" aria-hidden="true"></span><div class="field">' + esc(it.url || it.title) + '</div></div>'
    + '<div class="sk-col" style="flex: 1 1 auto; gap: 14px; padding: 22px" aria-hidden="true">'
    + '<div class="bar title" style="width: 46%; height: 14px"></div><div class="bar" style="width: 78%"></div>'
    + '<div class="cards"><div></div><div></div><div></div><div></div></div>'
    + '</div></div>',

  generic: (it) => '<div class="sk-col" style="flex: 1 1 auto; gap: 14px; padding: 22px">'
    + '<span class="generic-label">' + esc(it.app) + ' window</span>'
    + '<div class="bar title" style="width: 36%" aria-hidden="true"></div>'
    + '<div class="cards" aria-hidden="true"><div></div><div></div></div>'
    + '</div>',
};

function bars(widths, kind, firstHead) {
  return widths.map((w, i) => '<div class="bar ' + (firstHead && i === 0 ? 'head' : (kind || '')) + '" style="width: ' + w + '"></div>').join('');
}

function msg(nameWidth, lineWidths) {
  return '<div class="msg"><div class="avatar"></div><div class="sk-col"><div class="bar head" style="width: ' + nameWidth + '"></div>'
    + lineWidths.map((w) => '<div class="bar" style="width: ' + w + '"></div>').join('') + '</div></div>';
}

function mailItem(a, b, current) {
  return '<div class="mail-item' + (current ? ' current' : '') + '"><div class="bar head" style="width: ' + a + '"></div><div class="bar nav" style="width: ' + b + '"></div></div>';
}
