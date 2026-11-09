'use strict';

// The status that agents in terminals report (OSC 7501, decision 78), as
// Go sends it in the View (ItemView's agent, agentKind, and agentText, and
// TabView's working, waiting, done, and errored): a mark on the tile of a
// terminal's sub-tab while its agent works, has finished, or stopped with
// an error, and the bell with the agent's own words while it needs you;
// the same marks, rolled up, on its tab and the overview's card; what
// VoiceOver says for each event that Go counts (ItemView's agentEvent and
// agentEventLine); and the views that each window tells Go it draws
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
// for toastTexts, and when it showed, for toastTimes.
const recordToast = (page) => page.evaluate(() => {
  const el = document.getElementById('toast');
  window.toastTexts = [];
  window.toastTimes = [];
  new MutationObserver(() => {
    window.toastTexts.push(el.textContent);
    window.toastTimes.push(performance.now());
  }).observe(el, { childList: true, subtree: true, characterData: true });
});
const toastTexts = (page) => page.evaluate(() => window.toastTexts);
const toastTimes = (page) => page.evaluate(() => window.toastTimes);

// toastGone hides the toast, as if it had shown long enough to read, so
// that what's announced next shows at once.
const toastGone = (page) => page.evaluate(() => document.getElementById('toast').classList.remove('show'));

// AGENT_GAP is how long, in milliseconds, VoiceOver waits at least between
// two announcements of a terminal's agent, as terminals.js has it.
const AGENT_GAP = 1000;

// hold has fake Go answer method only once the function that it returns
// is called, as Go answers TermAttach once it has the engine's lock.
function hold(fake, method) {
  let release;
  const gate = new Promise((r) => {
    release = r;
  });
  fake.on(method, async (args, f) => {
    await gate;
    return f['m' + method](...args);
  });
  return release;
}

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

test('an agent\'s words with markup, controls, characters that turn text round, or line breaks show as plain text, on one line', async (t) => {
  const world = agentWorld();
  // A program chooses the words. Go takes out what it mustn't have first;
  // the UI does again. A line or paragraph separator becomes a space, so
  // that the words can't seem to have a line of Bench's own.
  const words = '<img src=x onerror="window.pwned = 1"> \u202Etxt.exe\u202C \u2066a\u2069 \u200Eb\uFEFF \u0007c\u009B \u{1F469}\u200D\u{1F4BB} & "d"'
    + ' compiling\u2028Also in Personal\u2029e';
  const shown = '<img src=x onerror="window.pwned = 1"> txt.exe a b c \u{1F469}\u200D\u{1F4BB} & "d" compiling Also in Personal e';
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
  fake.agentEvent('w5', PERMISSION, { agent: 'waiting', agentKind: 'permission', agentText: 'approve Bash: npm test' });
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
  fake.agentEvent('w5', 'Has a question for you: Which branch?', { agent: 'waiting', agentKind: 'question', agentText: 'Which branch?' });
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
  fake.agentEvent('w6', 'Finished: All 12 tests pass', { agent: 'done', agentText: 'All 12 tests pass', attention: 'Finished: All 12 tests pass' });
  await fake.emitState();
  await toastSays(page, 'go test — ~/code/bench in Code: finished: All 12 tests pass.');
  fake.agentEvent('w6', 'Stopped with an error', { agent: 'error', agentText: '', attention: 'Stopped with an error' });
  await fake.emitState();
  await toastSays(page, 'go test — ~/code/bench in Code: stopped with an error.');
  assert.equal((await toastTexts(page)).length, 5);
});

test('VoiceOver hears an agent that finishes and carries on at once, once, and not the bell that comes with it', async (t) => {
  const world = defaultWorld();
  world.voiceOver = true;
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await page.waitForFunction(() => window.bench.terminalReader('w5') === true);
  await page.click(tab(0));
  await fake.waitForCall('SwitchTab');
  await page.waitForSelector(tab(0) + '[aria-selected="true"]');
  await recordToast(page);
  // Claude Code finishes, rings the bell, and starts a prompt that waited,
  // all in one batch: Go's View has the agent at work, and its event, with
  // the line that Go marked and posted.
  await fake.termOut('w5', 'one\x07');
  fake.agentEvent('w5', 'Finished: Tests passed', { agent: 'working', attention: 'Finished: Tests passed' });
  await fake.emitState();
  const finished = 'zsh — ~/code/bench in Code: finished: Tests passed.';
  await toastSays(page, finished);
  await toastGone(page);
  await settle(page, 600);
  assert.deepEqual(await toastTexts(page), [finished]);
  // A bell 2 seconds later is announced.
  await later(page, 2100);
  await fake.termOut('w5', 'two\x07');
  await toastSays(page, 'zsh — ~/code/bench in Code: rang the bell.');
});

test('VoiceOver hears an agent that finishes in view once, as Go forgets it at once', async (t) => {
  const world = defaultWorld();
  world.voiceOver = true;
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await page.waitForFunction(() => window.bench.terminalReader('w5') === true);
  await recordToast(page);
  // The user sees the terminal, so Go shows nothing of the agent, but its
  // event counts.
  fake.agentEvent('w5', 'Finished: Tests passed');
  await fake.emitState();
  const finished = 'zsh — ~/code/bench: finished: Tests passed.';
  await toastSays(page, finished);
  await toastGone(page);
  // Views with the same event say nothing more.
  await fake.emitState();
  await fake.emitState();
  await settle(page, AGENT_GAP + 200);
  assert.deepEqual(await toastTexts(page), [finished]);
});

test('VoiceOver hears a command that ends just after its agent finished once, in the agent\'s line', async (t) => {
  const world = defaultWorld();
  world.voiceOver = true;
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await page.waitForFunction(() => window.bench.terminalReader('w5') === true);
  await recordToast(page);
  // The agent reports as it ends, so the end of its command is part of
  // the same event, as Go posts one notification for both.
  fake.agentEvent('w6', 'Finished: Tests passed', { agent: 'done', agentText: 'Tests passed', attention: 'Finished: Tests passed' });
  await fake.emitState();
  const finished = 'go test — ~/code/bench: finished: Tests passed.';
  await toastSays(page, finished);
  await toastGone(page);
  Object.assign(fake.world.items.w6, { ended: true, attention: 'Its command has finished' });
  await fake.emitState();
  await settle(page, AGENT_GAP + 200);
  assert.deepEqual(await toastTexts(page), [finished]);

  // A command that ends 2 seconds after its agent's event is announced.
  Object.assign(fake.world.items.w6, { ended: false, attention: '' });
  await fake.emitState();
  await later(page, 2100);
  fake.world.items.w6.ended = true;
  await fake.emitState();
  await toastSays(page, 'go test — ~/code/bench: its command has finished.');
});

test('VoiceOver hears an agent whose shell ends with its event once, in one View with the end or in the one before', async (t) => {
  const world = defaultWorld();
  world.voiceOver = true;
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await page.waitForFunction(() => window.bench.terminalReader('w5') === true);
  await page.click(tab(0));
  await fake.waitForCall('SwitchTab');
  await page.waitForSelector(tab(0) + '[aria-selected="true"]');
  await recordToast(page);
  // The agent reports, and its shell ends, in one batch: no View has the
  // terminal with its event, but the View lists it as gone.
  fake.agentEvent('w5', 'Finished: Tests passed', { agent: 'done', agentText: 'Tests passed', attention: 'Finished: Tests passed' });
  fake.shellEnds('w5');
  await fake.emitState();
  const finished = 'zsh — ~/code/bench in Code: finished: Tests passed.';
  await toastSays(page, finished);
  await toastGone(page);
  await fake.emitState();
  // In two batches, the View with the end has nothing new.
  fake.agentEvent('w6', 'Stopped with an error: rsync failed', { agent: 'error', agentText: 'rsync failed', attention: 'Stopped with an error: rsync failed' });
  await fake.emitState();
  const stopped = 'go test — ~/code/bench in Code: stopped with an error: rsync failed.';
  await toastSays(page, stopped);
  await toastGone(page);
  fake.shellEnds('w6');
  await fake.emitState();
  await settle(page, AGENT_GAP + 200);
  assert.deepEqual(await toastTexts(page), [finished, stopped]);
});

test('VoiceOver hears the line that waits for a second as the shell ends, at once', async (t) => {
  const world = defaultWorld();
  world.voiceOver = true;
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await page.waitForFunction(() => window.bench.terminalReader('w5') === true);
  await recordToast(page);
  const permission = 'zsh — ~/code/bench: needs your permission.';
  fake.agentEvent('w5', 'Needs your permission', { agent: 'waiting', agentKind: 'permission' });
  await fake.emitState();
  await toastSays(page, permission);
  await toastGone(page);
  // The next event waits for a second, but the shell ends first.
  fake.agentEvent('w5', 'Finished: Tests passed', { agent: '', agentKind: '' });
  await fake.emitState();
  fake.shellEnds('w5');
  await fake.emitState();
  const finished = 'zsh — ~/code/bench: finished: Tests passed.';
  await toastSays(page, finished);
  assert.deepEqual(await toastTexts(page), [permission, finished]);
  const [first, second] = await toastTimes(page);
  assert.ok(second - first < AGENT_GAP, 'the second came ' + Math.round(second - first) + ' ms after the first');
});

test('VoiceOver doesn\'t hear an agent\'s events from before the window\'s first View, or again as agent status comes back on', async (t) => {
  const world = defaultWorld();
  world.voiceOver = true;
  Object.assign(world.items.w6, {
    agent: 'done', agentText: 'Tests passed', attention: 'Finished: Tests passed', agentEvent: 3, agentEventLine: 'Finished: Tests passed',
  });
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await page.waitForFunction(() => window.bench.terminalReader('w5') === true);
  await recordToast(page);
  await fake.emitState();
  await settle(page, AGENT_GAP + 200);
  assert.equal(await page.$('#toast.show'), null);
  // With agent status off, Go leaves the events out; back on, they're the
  // same events.
  fake.world.settings.agentStatus = false;
  await fake.emitState();
  await settle(page);
  fake.world.settings.agentStatus = true;
  await fake.emitState();
  await settle(page, AGENT_GAP + 200);
  assert.deepEqual(await toastTexts(page), []);
  // The next one is new.
  fake.agentEvent('w6', 'Stopped with an error', { agent: 'error', agentText: '', attention: 'Stopped with an error' });
  await fake.emitState();
  await toastSays(page, 'go test — ~/code/bench: stopped with an error.');
});

test('VoiceOver hears each new report of an agent, with the line it had a moment ago too, but once a second at most', async (t) => {
  const world = defaultWorld();
  world.voiceOver = true;
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await page.waitForFunction(() => window.bench.terminalReader('w5') === true);
  await recordToast(page);
  // Codex sends no words. It needs the user's permission, works, and
  // needs it again for its next command, at once.
  const permission = 'zsh — ~/code/bench: needs your permission.';
  fake.agentEvent('w5', 'Needs your permission', { agent: 'waiting', agentKind: 'permission' });
  await fake.emitState();
  await toastSays(page, permission);
  await toastGone(page);
  Object.assign(fake.world.items.w5, { agent: 'working', agentKind: '' });
  await fake.emitState();
  await settle(page);
  fake.agentEvent('w5', 'Needs your permission', { agent: 'waiting', agentKind: 'permission' });
  await fake.emitState();
  await page.waitForFunction(() => window.toastTexts.length === 2);
  assert.deepEqual(await toastTexts(page), [permission, permission]);
  const [first, second] = await toastTimes(page);
  assert.ok(second - first >= AGENT_GAP - 50 && second - first < 5000, 'the second came ' + Math.round(second - first) + ' ms after the first');
});

test('VoiceOver hears a report once when its words come just after it, from a notification', async (t) => {
  const world = defaultWorld();
  world.voiceOver = true;
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await page.waitForFunction(() => window.bench.terminalReader('w5') === true);
  await recordToast(page);
  // Codex reports without words, and posts a notification just after it.
  // Go gives the report the notification's words, in a View of its own,
  // with the same event.
  const permission = 'zsh — ~/code/bench: needs your permission.';
  fake.agentEvent('w5', 'Needs your permission', { agent: 'waiting', agentKind: 'permission', agentText: '' });
  await fake.emitState();
  await toastSays(page, permission);
  await toastGone(page);
  await settle(page, 100);
  Object.assign(fake.world.items.w5, { agentText: 'Approval requested: npm test', agentEventLine: 'Needs your permission: Approval requested: npm test' });
  await fake.emitState();
  // The bell has the words, but they aren't announced again.
  await page.waitForFunction((sel) => (document.querySelector(sel) || {}).title === 'Needs your permission: Approval requested: npm test',
    sub('w5') + ' .subtab-bell');
  await settle(page, AGENT_GAP + 200);
  assert.deepEqual(await toastTexts(page), [permission]);
});

test('VoiceOver hears a program that reports over and over once a second at most, with its latest words', async (t) => {
  const world = defaultWorld();
  world.voiceOver = true;
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await page.waitForFunction(() => window.bench.terminalReader('w5') === true);
  await recordToast(page);
  const said = (n) => 'zsh — ~/code/bench: needs your permission: request ' + n + '.';
  let n = 0;
  const report = () => {
    fake.agentEvent('w5', 'Needs your permission: request ' + n, { agent: 'waiting', agentKind: 'permission', agentText: 'request ' + n });
    return fake.emitState();
  };
  await report();
  await toastSays(page, said(0));
  // A new event in each View, many within a second, twice over.
  for (const last of [20, 40]) {
    await toastGone(page);
    while (n < last) {
      n += 1;
      await report();
    }
    await toastSays(page, said(last));
  }
  await settle(page, AGENT_GAP + 200);
  assert.deepEqual(await toastTexts(page), [said(0), said(20), said(40)]);
  const times = await toastTimes(page);
  for (let i = 1; i < times.length; i++) {
    assert.ok(times[i] - times[i - 1] >= AGENT_GAP - 50, 'announcement ' + i + ' came ' + Math.round(times[i] - times[i - 1]) + ' ms after the one before');
  }
});

test('VoiceOver hears at most 200 characters of an agent\'s line, on one line, and the bell\'s tooltip has all its words', async (t) => {
  const world = defaultWorld();
  world.voiceOver = true;
  const { page, fake, close } = await openBench({ world });
  t.after(close);
  await page.waitForFunction(() => window.bench.terminalReader('w5') === true);
  // "Has a question for you: " has 24 characters, so the line's 200th is
  // the emoji. A character outside the Basic Multilingual Plane counts as
  // one, and isn't cut in two.
  const words = 'x'.repeat(175) + '\u{1F600}' + 'y'.repeat(300);
  fake.agentEvent('w5', 'Has a question for you: ' + words, { agent: 'waiting', agentKind: 'question', agentText: words });
  await fake.emitState();
  await toastSays(page, 'zsh — ~/code/bench: has a question for you: ' + 'x'.repeat(175) + '\u{1F600}…');
  assert.equal(await page.getAttribute(sub('w5') + ' .subtab-bell', 'title'), 'Has a question for you: ' + words);

  // A line separator in the line becomes a space, and characters that
  // turn text round go, as in the words.
  await toastGone(page);
  await later(page, AGENT_GAP);
  fake.agentEvent('w5', 'Has a question for you: Which branch‮?', { agentText: 'Which branch‮?' });
  await fake.emitState();
  await toastSays(page, 'zsh — ~/code/bench: has a question for you: Which branch?');
});

test('a terminal\'s own window announces what its agent reports too', async (t) => {
  const world = defaultWorld();
  world.voiceOver = true;
  const { page, fake, close } = await openTerminalWindow({ id: 'w5', world });
  t.after(close);
  await fake.emitState();
  await page.waitForFunction(() => window.bench.terminalReader('w5') === true);
  fake.agentEvent('w5', 'Needs you to sign in: GitHub token expired', { agent: 'waiting', agentKind: 'auth', agentText: 'GitHub token expired' });
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

test('a window tells Go that it draws a terminal, and that it doesn\'t, in turn, so that Go gets them in order', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await page.waitForSelector('.pane[data-pane-id="p4"] .xterm');
  const shows = (id) => page.waitForFunction((i) => document.querySelector('.pane[data-pane-id="p4"] .term-host').dataset.termHost === i, id);
  const calls = () => fake.calls.filter((c) => /^Term(Attach|Detach)$/.test(c.method) && c.args[0] === 'w6').map((c) => [c.method, ...c.args]);
  // Go answers TermAttach once it has the engine's lock, which can be
  // late: here, once it's let go.
  const attached = hold(fake, 'TermAttach');
  await page.click(sub('w6'));
  await fake.waitForCall('TermAttach', (a) => a[0] === 'w6');
  // Popped out meanwhile, w6 is let go only once Go has answered. A
  // TermDetach that came first would leave Go with a view that doesn't
  // draw the terminal, and Go wouldn't answer its program's DA1.
  const p4 = fake.world.tabs[1].root.children[1];
  Object.assign(p4, { items: ['w5'], active: 'w5' });
  fake.world.floating.push({ ...fake.world.items.w6, tab: -1 });
  await fake.emitState();
  await shows('w5');
  await settle(page);
  // Back in its tab, w6 is drawn again, and Go is told so only once it
  // has answered the TermDetach. A TermAttach that came first would be
  // undone, and DA1 would get two answers.
  Object.assign(p4, { items: ['w5', 'w6'], active: 'w6' });
  fake.world.floating = fake.world.floating.filter((f) => f.id !== 'w6');
  await fake.emitState();
  await shows('w6');
  await settle(page);
  assert.deepEqual(calls(), [['TermAttach', 'w6', 'main']]);
  const detached = hold(fake, 'TermDetach');
  attached();
  await fake.waitForCall('TermDetach', (a) => a[0] === 'w6');
  await settle(page);
  assert.deepEqual(calls(), [['TermAttach', 'w6', 'main'], ['TermDetach', 'w6', 'main']]);
  detached();
  await fake.waitForCall('TermAttach', () => calls().length === 3);
  assert.deepEqual(calls(), [['TermAttach', 'w6', 'main'], ['TermDetach', 'w6', 'main'], ['TermAttach', 'w6', 'main']]);
  assert.deepEqual([...fake.termViews.get('w6')], ['main']);
  // The new terminal draws its snapshot.
  await page.waitForFunction(() => document.querySelector('.term-host[data-term-host="w6"] .xterm-rows').textContent.includes('go test ./...'));
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
