'use strict';

// The status that agents in terminals report (OSC 7501, decision 78), as
// Go sends it in the View (ItemView's agent, agentKind, and agentText, and
// TabView's working, waiting, done, and errored): a mark on the tile of a
// terminal's sub-tab while its agent works, has finished, or stopped with
// an error, and the bell with the agent's own words while it needs you;
// the same marks, rolled up, on its tab and the overview's card; what
// VoiceOver says; and the views that each window tells Go it draws
// (TermAttach and TermDetach), as Go answers a program's question for the
// terminal's attributes only while no view draws the terminal.

const assert = require('node:assert/strict');
const test = require('node:test');
const {
  openBench, openSettings, openTerminalWindow, settle,
} = require('./helpers/harness.js');
const { defaultWorld } = require('./helpers/fixture.js');

const b64 = (s) => Buffer.from(s, 'utf8').toString('base64');
const sub = (id) => '.subtab[data-item="' + id + '"]';
const tab = (i) => '.tab[data-index="' + i + '"]';

// PERMISSION is what Go says of w6's agent in agentWorld, and marks it
// with while the user can't see it.
const PERMISSION = 'Needs your permission: approve Bash: npm test';

// agentWorld returns the default world with w5's agent at work and w6's
// waiting for the user's permission, as Go sends them: w6 isn't shown, so
// Go has marked it too.
function agentWorld(opts) {
  const world = defaultWorld(opts);
  world.items.w5.agent = 'working';
  Object.assign(world.items.w6, { agent: 'waiting', agentKind: 'permission', agentText: 'approve Bash: npm test', attention: PERMISSION });
  return world;
}

// tipLines returns the lines of an element's tooltip.
const tipLines = async (page, sel) => (await page.getAttribute(sel, 'title')).split('\n');

// hiddenTexts returns the texts for a screen reader only in an element.
const hiddenTexts = (page, sel) => page.$$eval(sel + ' .visually-hidden', (els) => els.map((el) => el.textContent));

// toastSays waits until the toast shows text.
const toastSays = (page, text) => page.waitForFunction(
  (want) => document.querySelector('#toast.show') && document.querySelector('#toast').textContent === want, text,
);

// recordToast notes each text that the toast shows from now on, in order,
// for toastTexts.
const recordToast = (page) => page.evaluate(() => {
  const el = document.getElementById('toast');
  window.toastTexts = [];
  new MutationObserver(() => window.toastTexts.push(el.textContent)).observe(el, { childList: true, subtree: true, characterData: true });
});
const toastTexts = (page) => page.evaluate(() => window.toastTexts);

// later moves the page's clock, as Date.now() tells it, ms milliseconds on.
const later = (page, ms) => page.evaluate((by) => {
  const now = Date.now;
  Date.now = () => now() + by;
}, ms);

// ring has terminal w5 write mark and ring the bell, and waits until it has
// drawn the mark, by when xterm.js has rung it.
async function ring(page, fake, mark) {
  await fake.termOut('w5', mark + '\x07');
  await page.waitForFunction((m) => document.querySelector('.pane[data-pane-id="p4"] .xterm-rows').textContent.includes(m), mark);
}

// openFromPalette opens the overview with Jump to…'s Show All Tabs.
async function openFromPalette(page) {
  await page.click('[data-action="open-palette"]');
  await page.fill('#palette-query', 'show all tabs');
  await page.keyboard.press('Enter');
  await page.waitForSelector('.overview');
}

test('a terminal\'s sub-tab has a mark while its agent works, and the bell with the agent\'s words while it needs you', async (t) => {
  const { page, fake, close } = await openBench({ world: agentWorld() });
  t.after(close);
  const mark = sub('w5') + ' .agent-mark';
  await page.waitForSelector(mark);

  // At work: grey dots on the corner of the tile, as a marker of a window
  // that isn't ready would be, and a word in the label and the tooltip.
  assert.equal(await page.getAttribute(mark, 'data-agent'), 'working');
  assert.equal(await page.getAttribute(mark, 'title'), 'Working');
  assert.ok(await page.$(sub('w5') + ' .mono-tile > .agent-mark'));
  const corner = await page.$eval(sub('w5'), (el) => {
    const tile = el.querySelector('.mono-tile').getBoundingClientRect();
    const m = el.querySelector('.agent-mark').getBoundingClientRect();
    return { right: Math.round(m.right - tile.right), top: Math.round(m.top - tile.top) };
  });
  assert.deepEqual(corner, { right: 6, top: -5 });
  assert.equal(await page.$(sub('w5') + ' .subtab-bell'), null);
  assert.equal(await page.getAttribute(sub('w5'), 'aria-label'), 'zsh — ~/code/bench, Terminal (working)');
  assert.deepEqual(await tipLines(page, sub('w5')), ['Terminal — zsh — ~/code/bench', 'Working']);

  // Needing you: the bell, with the agent's words as they are, and no mark
  // on the tile. Its name keeps the words out, for Voice Control.
  const bell = sub('w6') + ' .subtab-bell';
  assert.equal(await page.getAttribute(bell, 'title'), PERMISSION);
  assert.equal(await page.$(sub('w6') + ' .agent-mark'), null);
  assert.equal(await page.getAttribute(sub('w6'), 'aria-label'), 'go test — ~/code/bench, Terminal (needs you)');
  assert.deepEqual(await tipLines(page, sub('w6')), ['Terminal — go test — ~/code/bench', PERMISSION]);

  // Each kind of need says what it is, and words at work show too. The
  // bell stays while the agent waits, after Go has taken its mark off,
  // as the user looks at the terminal.
  fake.world.items.w5.agentText = 'Running the tests';
  for (const [kind, words, want] of [
    ['question', 'Which branch?', 'Has a question for you: Which branch?'],
    ['auth', '', 'Needs you to sign in'],
    ['', '', 'Needs you'],
  ]) {
    Object.assign(fake.world.items.w6, { agentKind: kind, agentText: words, attention: '' });
    await fake.emitState();
    await page.waitForFunction(([sel, text]) => document.querySelector(sel) && document.querySelector(sel).title === text, [bell, want]);
  }
  assert.equal(await page.getAttribute(mark, 'title'), 'Working: Running the tests');
  assert.deepEqual(await tipLines(page, sub('w5')), ['Terminal — zsh — ~/code/bench', 'Working: Running the tests']);

  // Finished, and stopped with an error, until the user looks: a green
  // tick and a red octagon, and Go's bell, with the agent's words.
  Object.assign(fake.world.items.w5, { agent: 'done', agentText: 'All 12 tests pass', attention: 'Finished: All 12 tests pass' });
  Object.assign(fake.world.items.w6, { agent: 'error', agentKind: '', agentText: '', attention: 'Stopped with an error' });
  await fake.emitState();
  await page.waitForSelector(sub('w5') + ' .agent-mark[data-agent="done"]');
  assert.equal(await page.getAttribute(mark, 'title'), 'Finished: All 12 tests pass');
  assert.equal(await page.getAttribute(sub('w5') + ' .subtab-bell', 'title'), 'Finished: All 12 tests pass');
  assert.equal(await page.getAttribute(sub('w5'), 'aria-label'), 'zsh — ~/code/bench, Terminal (done, needs you)');
  assert.deepEqual(await tipLines(page, sub('w5')), ['Terminal — zsh — ~/code/bench', 'Finished: All 12 tests pass']);
  assert.equal(await page.getAttribute(sub('w6') + ' .agent-mark', 'data-agent'), 'error');
  assert.equal(await page.getAttribute(sub('w6') + ' .agent-mark', 'title'), 'Stopped with an error');
  assert.equal(await page.getAttribute(sub('w6'), 'aria-label'), 'go test — ~/code/bench, Terminal (stopped with an error, needs you)');

  // Seen, Go forgets them, and the marks go.
  for (const id of ['w5', 'w6']) Object.assign(fake.world.items[id], { agent: '', agentText: '', attention: '' });
  await fake.emitState();
  await page.waitForFunction(() => !document.querySelector('.agent-mark, .bell-mark'));
  assert.equal(await page.getAttribute(sub('w5'), 'aria-label'), 'zsh — ~/code/bench, Terminal');
});

test('a mark on a sub-tab\'s tile takes no room, so the sub-tab is as wide as without it', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.waitForSelector(sub('w5'));
  await settle(page);
  const width = () => page.$eval(sub('w5'), (el) => el.getBoundingClientRect().width);
  const before = await width();
  for (const agent of ['working', 'done', 'error']) {
    fake.world.items.w5.agent = agent;
    await fake.emitState();
    await page.waitForSelector(sub('w5') + ' .agent-mark[data-agent="' + agent + '"]');
    await settle(page);
    assert.equal(await width(), before, agent);
  }
});

test('a tab has one mark for its agents, the one that matters most, and its bell counts those that need you', async (t) => {
  const { page, fake, close } = await openBench({ world: agentWorld({ current: 0 }) });
  t.after(close);
  const code = tab(1);
  await page.waitForSelector(code + ' .tab-agent');
  assert.equal(await page.getAttribute(code + ' .tab-agent', 'data-agent'), 'working');
  assert.equal(await page.getAttribute(code + ' .tab-agent', 'title'), '1 agent working');
  assert.equal(await page.getAttribute(code + ' .tab-agent', 'aria-hidden'), 'true');
  assert.equal(await page.getAttribute(code + ' .tab-bell', 'title'), '1 window needs you');
  assert.deepEqual((await tipLines(page, code)).slice(1), ['1 agent working', '1 window needs you']);
  assert.deepEqual(await hiddenTexts(page, code), [', 1 agent working', ', 1 window needs you']);
  assert.equal(await page.$(tab(0) + ' .tab-agent'), null);
  assert.equal(await page.$(tab(2) + ' .tab-agent'), null);

  // An agent that needs you counts in the bell, whether Go has marked its
  // window or not, and once if it has.
  Object.assign(fake.world.items.w5, { agent: 'waiting', agentKind: 'question', agentText: 'Which branch?' });
  await fake.emitState();
  await page.waitForFunction((sel) => (document.querySelector(sel + ' .bell-count') || {}).textContent === '2', code);
  assert.equal(await page.$(code + ' .tab-agent'), null);
  assert.deepEqual(await hiddenTexts(page, code), [', 2 windows need you']);

  // An error matters most, then a finished agent, then one at work.
  Object.assign(fake.world.items.w5, { agent: 'done', agentKind: '', agentText: '', attention: 'Finished' });
  Object.assign(fake.world.items.w6, { agent: 'error', agentKind: '', agentText: '', attention: 'Stopped with an error' });
  await fake.emitState();
  await page.waitForSelector(code + ' .tab-agent[data-agent="error"]');
  assert.equal(await page.getAttribute(code + ' .tab-agent', 'title'), '1 agent done, 1 agent stopped with an error');
  Object.assign(fake.world.items.w6, { agent: 'working', attention: '' });
  await fake.emitState();
  await page.waitForSelector(code + ' .tab-agent[data-agent="done"]');
  assert.equal(await page.getAttribute(code + ' .tab-agent', 'title'), '1 agent working, 1 agent done');
  assert.deepEqual(await hiddenTexts(page, code), [', 1 agent working, 1 agent done', ', 1 window needs you']);
  Object.assign(fake.world.items.w5, { agent: 'working', attention: '' });
  await fake.emitState();
  await page.waitForFunction((sel) => (document.querySelector(sel + ' .tab-agent') || {}).title === '2 agents working', code);
  assert.equal(await page.getAttribute(code + ' .tab-agent', 'data-agent'), 'working');
});

test('the overview\'s card has the tab\'s mark for its agents, and its panes the marks of their windows', async (t) => {
  const { page, close } = await openBench({ world: agentWorld({ current: 0 }) });
  t.after(close);
  await openFromPalette(page);
  const card = '#overview-card-1';
  assert.equal(await page.getAttribute(card + ' .overview-card-head .tab-agent', 'data-agent'), 'working');
  assert.equal(await page.getAttribute(card + ' .overview-card-head .tab-agent', 'title'), '1 agent working');
  // The card's label has it, so the mark has no text of its own.
  assert.equal(await page.$(card + ' .overview-card-head .visually-hidden'), null);
  assert.match(await page.getAttribute(card, 'aria-label'), /, 1 window needs you, 1 agent working$/);
  // w5 is the window its second pane shows, and w6 another one there.
  const p4 = page.locator(card + ' .overview-pane').nth(1);
  assert.equal(await p4.locator('.overview-shown .agent-mark').getAttribute('data-agent'), 'working');
  assert.deepEqual((await p4.locator('.overview-shown').getAttribute('title')).split('\n'), ['Terminal — zsh — ~/code/bench', 'Working']);
  assert.equal(await p4.locator('.overview-other .overview-bell').getAttribute('title'), PERMISSION);
  assert.equal(await p4.locator('.overview-other .agent-mark').count(), 0);
});

test('a bell or a notification after the agent\'s report never takes the place of its words', async (t) => {
  const world = agentWorld();
  // As if Go had marked w6 for a notification since.
  world.items.w6.attention = 'Notification: Claude Code: Claude needs your permission';
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  const bell = sub('w6') + ' .subtab-bell';
  await page.waitForSelector(bell);
  assert.equal(await page.getAttribute(bell, 'title'), PERMISSION);
  assert.equal(await page.locator(sub('w6') + ' .bell-mark').count(), 1);
  assert.deepEqual(await tipLines(page, sub('w6')), ['Terminal — go test — ~/code/bench', PERMISSION]);
  assert.equal(await page.getAttribute(sub('w6'), 'aria-label'), 'go test — ~/code/bench, Terminal (needs you)');
  fake.world.items.w6.attention = 'Rang the bell';
  await fake.emitState();
  await settle(page);
  assert.equal(await page.getAttribute(bell, 'title'), PERMISSION);
});

test('an agent\'s words with markup, controls, or characters that turn text round show as plain text', async (t) => {
  const world = agentWorld();
  // A program chooses the words. Go takes out what it mustn't have first;
  // the UI does again.
  const words = '<img src=x onerror="window.pwned = 1"> \u202Etxt.exe\u202C \u2066a\u2069 \u200Eb\uFEFF \u0007c\u009B \u{1F469}\u200D\u{1F4BB} & "d"';
  const shown = '<img src=x onerror="window.pwned = 1"> txt.exe a b c \u{1F469}\u200D\u{1F4BB} & "d"';
  Object.assign(world.items.w6, { agentText: words, attention: '' });
  world.items.w5.agentText = words;
  const { page, close } = await openBench({ world });
  t.after(close);
  await page.waitForSelector(sub('w6') + ' .subtab-bell');
  assert.equal(await page.getAttribute(sub('w6') + ' .subtab-bell', 'title'), 'Needs your permission: ' + shown);
  assert.equal(await page.getAttribute(sub('w5') + ' .agent-mark', 'title'), 'Working: ' + shown);
  assert.ok((await tipLines(page, sub('w6'))).includes('Needs your permission: ' + shown));
  assert.equal(await page.$('#titlebar img, #panes img:not(.tile-icon)'), null);
  await settle(page);
  assert.equal(await page.evaluate(() => window.pwned), undefined);
});

test('VoiceOver hears an agent that needs you once, in its words, and not the bell that comes with it', async (t) => {
  const world = defaultWorld();
  world.voiceOver = true;
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await page.waitForFunction(() => window.bench.terminalReader('w5') === true);
  await recordToast(page);
  const permission = 'zsh — ~/code/bench: needs your permission: approve Bash: npm test.';

  // The report first, and then the bell.
  Object.assign(fake.world.items.w5, { agent: 'waiting', agentKind: 'permission', agentText: 'approve Bash: npm test' });
  await fake.emitState();
  await toastSays(page, permission);
  await ring(page, fake, 'one');
  await settle(page, 600);
  assert.deepEqual(await toastTexts(page), [permission]);

  // An agent at work isn't announced. Then the bell first, and the report
  // just after it. Words that end with a question mark get no full stop.
  Object.assign(fake.world.items.w5, { agent: 'working', agentKind: '', agentText: '' });
  await fake.emitState();
  await settle(page);
  await fake.termOut('w5', 'two\x07');
  Object.assign(fake.world.items.w5, { agent: 'waiting', agentKind: 'question', agentText: 'Which branch?' });
  await fake.emitState();
  const question = 'zsh — ~/code/bench: has a question for you: Which branch?';
  await toastSays(page, question);
  await settle(page, 600);
  assert.deepEqual(await toastTexts(page), [permission, question]);

  // A bell with no report is still announced, once the report it may come
  // with is 2 seconds old.
  Object.assign(fake.world.items.w5, { agent: 'working', agentKind: '', agentText: '' });
  await fake.emitState();
  await later(page, 2100);
  await ring(page, fake, 'three');
  await toastSays(page, 'zsh — ~/code/bench: rang the bell.');

  // A finished agent and one that stopped with an error, in a terminal in
  // another tab, say which.
  await page.click(tab(0));
  await fake.waitForCall('SwitchTab');
  await page.waitForSelector(tab(0) + '[aria-selected="true"]');
  Object.assign(fake.world.items.w6, { agent: 'done', agentText: 'All 12 tests pass', attention: 'Finished: All 12 tests pass' });
  await fake.emitState();
  await toastSays(page, 'go test — ~/code/bench in Code: finished: All 12 tests pass.');
  Object.assign(fake.world.items.w6, { agent: 'error', agentText: '', attention: 'Stopped with an error' });
  await fake.emitState();
  await toastSays(page, 'go test — ~/code/bench in Code: stopped with an error.');
  assert.equal((await toastTexts(page)).length, 5);
});

test('a terminal\'s own window announces what its agent reports too', async (t) => {
  const world = defaultWorld();
  world.voiceOver = true;
  const { page, fake, close } = await openTerminalWindow({ id: 'w5', world });
  t.after(close);
  await fake.emitState();
  await page.waitForFunction(() => window.bench.terminalReader('w5') === true);
  Object.assign(fake.world.items.w5, { agent: 'waiting', agentKind: 'auth', agentText: 'GitHub token expired' });
  await fake.emitState();
  await toastSays(page, 'zsh — ~/code/bench: needs you to sign in: GitHub token expired.');
});

test('the dots of an agent at work pulse quietly, but not with Reduce Motion on', async (t) => {
  for (const [motion, animation] of [['no-preference', 'agent-working'], ['reduce', 'none']]) {
    const { page, close } = await openBench({ world: agentWorld(), contextOptions: { reducedMotion: motion } });
    t.after(close);
    const dots = sub('w5') + ' .agent-mark .dot';
    await page.waitForSelector(dots);
    const styles = await page.$$eval(dots, (els) => els.map((el) => [getComputedStyle(el).animationName, getComputedStyle(el).opacity]));
    assert.equal(styles.length, 3, motion);
    assert.deepEqual(styles.map((s) => s[0]), [animation, animation, animation], motion);
    // Still, each dot shows in full.
    if (motion === 'reduce') assert.deepEqual(styles.map((s) => s[1]), ['1', '1', '1']);
  }
});

test('with agent status off, Go sends no agents, and none shows', async (t) => {
  const world = agentWorld({ current: 0 });
  world.settings.agentStatus = false;
  // Go doesn't mark a window for a report it ignores.
  world.items.w6.attention = '';
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await settle(page);
  assert.equal(await page.$('.agent-mark, .bell-mark'), null);
  assert.equal(await page.$(tab(1) + ' .visually-hidden'), null);
  assert.equal(await page.evaluate(() => window.bench.store.view.items.w6.agent), undefined);
  // On again, they show.
  fake.world.settings.agentStatus = true;
  await fake.emitState();
  await page.waitForSelector(tab(1) + ' .tab-agent');
  assert.equal(await page.getAttribute(tab(1) + ' .tab-bell', 'title'), '1 window needs you');
});

test('Settings ▸ Terminals has a switch for agents\' status, and one for their words in notifications', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await openSettings(page, 'terminals');
  for (const [name, label, help, text] of [
    ['agentStatus', 'Show the status that agents report', 'agent-status-help',
      'Agents such as Claude Code and Codex get a mark on their sub-tab while they work, when they need you, and when they finish. Off, Bench doesn\'t answer their status check.'],
    ['agentWords', 'Show what the agent says in notifications', 'agent-words-help',
      'Such as “approve Bash: npm test”. Off, a notification says only what the agent needs, such as “Needs your permission”. Bench\'s own marks always show the words.'],
  ]) {
    const box = '[data-setting="' + name + '"]';
    assert.equal(await page.isChecked(box), true, name);
    assert.equal(await page.$eval(box, (el) => el.closest('label').textContent), label);
    assert.equal(await page.getAttribute(box, 'aria-describedby'), help);
    assert.equal(await page.textContent('#' + help), text);
  }
  // They come after Notify me, which they go with, and change Terminals.
  const order = await page.$$eval('#settings-panel [data-setting]', (els) => els.map((el) => el.dataset.setting));
  assert.deepEqual(order, ['terminalOptionAsMeta', 'notifyTerminals', 'agentStatus', 'agentWords', 'keepTerminalOutput']);
  await page.click('[data-setting="agentWords"]');
  assert.equal(await page.textContent('[data-key="settings-section-terminals"]'), 'Terminals, changed');
  await page.click('[data-action="save-settings"]');
  const save = await fake.waitForCall('SaveSettings');
  assert.equal(save.args[0].agentStatus, true);
  assert.equal(save.args[0].agentWords, false);
});

test('each window tells Go which terminals it draws, and stops when it no longer does', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.waitForSelector('.pane[data-pane-id="p4"] .xterm');
  // The main window draws w5, and gets its snapshot as it says so.
  assert.deepEqual(fake.callsTo('TermAttach').map((c) => c.args), [['w5', 'main']]);
  assert.equal(fake.callsTo('TermSnapshot').length, 0);
  assert.equal(fake.callsTo('TermDetach').length, 0);
  // A sub-tab switch keeps the terminal, hidden, and Go is told nothing.
  await page.click(sub('w6'));
  await page.waitForFunction(() => document.querySelector('.pane[data-pane-id="p4"] .term-host').dataset.termHost === 'w6');
  await settle(page);
  assert.deepEqual(fake.callsTo('TermAttach').map((c) => c.args), [['w5', 'main'], ['w6', 'main']]);
  assert.equal(fake.callsTo('TermDetach').length, 0);
  // Popped out, the main window lets it go.
  const p4 = fake.world.tabs[1].root.children[1];
  p4.items = ['w6'];
  fake.world.floating.push({ ...fake.world.items.w5, tab: -1 });
  await fake.emitState();
  assert.deepEqual((await fake.waitForCall('TermDetach')).args, ['w5', 'main']);
  assert.deepEqual([...fake.termViews.get('w5')], []);

  // Its own window draws it as its own view, after checking that it's
  // there, which tells Go nothing.
  const own = await openTerminalWindow({ id: 'w5' });
  t.after(own.close);
  await own.page.waitForSelector('.terminal-window .xterm');
  assert.deepEqual(own.fake.callsTo('TermSnapshot').map((c) => c.args), [['w5']]);
  assert.deepEqual(own.fake.callsTo('TermAttach').map((c) => c.args), [['w5', 'own']]);
});

test('a View that comes before Ready\'s answer draws a terminal, which tells Go again after Ready forgets it', async (t) => {
  // Go's Ready forgets the views of the main window, as when it loads
  // again; here a View comes first, as Go sends one after each change.
  const { page, fake, close } = await openBench({
    world: defaultWorld({ area: { x: 0, y: 52, w: 1440, h: 820 } }),
    setup: (fake) => fake.on('Ready', async (_args, f) => {
      await f.emitState();
      await f.waitForCall('TermAttach', (a) => a[0] === 'w5');
      return f.mReady();
    }),
  });
  t.after(close);
  await page.waitForSelector('.pane[data-pane-id="p4"] .xterm');
  await fake.waitForCall('TermAttach', () => fake.callsTo('TermAttach').length === 2);
  assert.deepEqual(fake.callsTo('TermAttach').map((c) => c.args), [['w5', 'main'], ['w5', 'main']]);
  assert.deepEqual([...fake.termViews.get('w5')], ['main']);
  // The snapshot is drawn once.
  await page.waitForFunction(() => document.querySelector('.pane[data-pane-id="p4"] .xterm-rows').textContent.includes('go run ./cmd/bench'));
  await settle(page);
  const text = await page.$eval('.pane[data-pane-id="p4"] .xterm-rows', (el) => el.textContent);
  assert.equal(text.split('go run ./cmd/bench').length - 1, 1);
});

test('a terminal never answers an agent\'s status check, which Go answers, but answers a question for its attributes', async (t) => {
  // An old status check and question, in the snapshot, get no answer.
  const { page, fake, close } = await openBench({
    setup: (fake) => fake.on('TermAttach', () => ({ data: b64('$ claude\r\n\x1b]7501;?\x07\x1b[c$ '), offset: 30 })),
  });
  t.after(close);
  await page.waitForSelector('.pane[data-pane-id="p4"] .xterm');
  await fake.waitForCall('TermAck', (a) => a[0] === 'w5' && a[1] === 30);
  await settle(page);
  assert.deepEqual(fake.callsTo('TermInput'), []);
  // Live, with either ending: the status check gets no answer from the
  // terminal, and the question its usual one.
  let at = 30;
  for (const end of ['\x1b\\', '\x07']) {
    fake.clearCalls();
    const out = '\x1b]7501;?' + end + '\x1b[c';
    await fake.emit('term:data', { id: 'w5', data: b64(out), offset: at });
    at += Buffer.byteLength(out);
    await fake.waitForCall('TermInput', (a) => a[0] === 'w5');
    await settle(page, 200);
    assert.deepEqual(fake.callsTo('TermInput').map((c) => c.args), [['w5', '\x1b[?1;2c']], JSON.stringify(end));
  }
});
