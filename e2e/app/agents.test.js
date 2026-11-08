'use strict';

// Agent status (decision 78): what a program in a terminal, such as Claude
// Code, reports about itself with OSC 7501. The preview's pretend terminal
// (term.Echo) has an agent command that prints such a report, as a real
// agent would, after a wait: agent [seconds] state [kind] [words].

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const {
  itemID, openPage, openSettings, setScreen, startPreview, switchTo, tabIndex, view, waitForToast, waitUntil, waitView,
} = require('./helpers/preview.js');

const APPROVE = 'approve Bash: npm test';

// open starts the preview on the monitor, shows the Code tab, and puts the
// keyboard in its terminal. It returns the page, the terminal's item ID,
// and the selector of its sub-tab.
async function open(t) {
  const preview = await startPreview({ t });
  const { page, close } = await openPage(preview);
  t.after(close);
  await setScreen(page, 'monitor');
  const v = await switchTo(page, 'Code');
  const term = itemID(v, 'zsh — ~/code');
  await page.click('[data-term="' + term + '"] .xterm');
  return { preview, page, term, subtab: '.subtab[data-item="' + term + '"]' };
}

// typeLine types a line into the terminal that has the keyboard, and
// presses Return.
async function typeLine(page, text) {
  await page.keyboard.type(text);
  await page.keyboard.press('Enter');
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

test('an agent that needs you in another tab gets one bell, with its own words, which a later bell keeps', async (t) => {
  const { page, term } = await open(t);
  // The agent rings the bell as it reports, as Claude Code does, and again
  // later, after the event.
  const typed = Date.now();
  await typeLine(page, 'agent 1 blocked permission ' + APPROVE);
  await typeLine(page, 'bell 1');
  await typeLine(page, 'bell 4');
  await switchTo(page, 'Comms');

  let v = await waitView(page, (x, id) => x.items[id].agent === 'waiting', term);
  assert.equal(v.items[term].agentKind, 'permission');
  assert.equal(v.items[term].agentText, APPROVE);
  const code = tabIndex(v, 'Code');
  const tab = '.tab[data-index="' + code + '"]';
  v = await waitView(page, (x, id) => Boolean(x.items[id].attention), term);
  assert.equal(v.items[term].attention, 'Needs your permission: ' + APPROVE);
  assert.equal(v.tabs[code].attention, 1);
  assert.equal(v.tabs[code].waiting, 1);
  await page.waitForSelector(tab + ' .tab-bell');

  // The later bell changes nothing.
  await sleep(Math.max(0, typed + 5000 - Date.now()));
  v = await view(page);
  assert.equal(v.items[term].attention, 'Needs your permission: ' + APPROVE);
  assert.equal(v.items[term].agent, 'waiting');

  // Showing it takes the mark off, but the agent still waits, and its
  // sub-tab's bell says why, in its words.
  await switchTo(page, 'Code');
  v = await waitView(page, (x, id) => !x.items[id].attention, term);
  assert.equal(v.items[term].agent, 'waiting');
  assert.equal(v.tabs[code].attention, 1);
  const bell = '.subtab[data-item="' + term + '"] .subtab-bell';
  await page.waitForSelector(bell);
  assert.equal(await page.getAttribute(bell, 'title'), 'Needs your permission: ' + APPROVE);

  // Once it works again, it no longer waits.
  await page.click('[data-term="' + term + '"] .xterm');
  await typeLine(page, 'agent 0 working');
  v = await waitView(page, (x, id) => x.items[id].agent === 'working', term);
  assert.equal(v.tabs[code].attention, undefined);
  assert.equal(v.tabs[code].working, 1);
  await page.waitForSelector('.subtab[data-item="' + term + '"] .agent-mark[data-agent="working"]');
  await page.waitForFunction((sel) => !document.querySelector(sel), bell);
});

test('an agent that finishes in another tab shows it until it\'s seen, and one in view shows nothing', async (t) => {
  const { page, term, subtab } = await open(t);
  await typeLine(page, 'agent 1 done Tests passed');
  await switchTo(page, 'Comms');

  let v = await waitView(page, (x, id) => x.items[id].agent === 'done', term);
  assert.equal(v.items[term].agentText, 'Tests passed');
  const code = tabIndex(v, 'Code');
  v = await waitView(page, (x, id) => Boolean(x.items[id].attention), term);
  assert.equal(v.items[term].attention, 'Finished: Tests passed');
  assert.equal(v.tabs[code].done, 1);
  const tab = '.tab[data-index="' + code + '"]';
  await page.waitForSelector(tab + ' .tab-agent[data-agent="done"]');

  // Showing it takes the mark off, and the agent's report with it.
  await switchTo(page, 'Code');
  v = await waitView(page, (x, id) => !x.items[id].agent && !x.items[id].attention, term);
  assert.equal(v.tabs[code].done, undefined);
  await page.waitForFunction((sel) => !document.querySelector(sel + ' .agent-mark'), subtab);

  // One that stops with an error in view shows nothing either.
  await page.click('[data-term="' + term + '"] .xterm');
  await typeLine(page, 'agent 0 error Tests failed');
  await sleep(500);
  v = await view(page);
  assert.equal(v.items[term].agent, undefined);
  assert.equal(v.items[term].attention, undefined);
});

test('with agent status off in Settings, an agent\'s report changes nothing, and its bell marks the terminal', async (t) => {
  const { preview, page, term } = await open(t);
  await openSettings(page, 'terminals');
  assert.equal(await page.isChecked('[data-setting="agentStatus"]'), true);
  await page.uncheck('[data-setting="agentStatus"]');
  await page.click('[data-action="save-settings"]');
  await waitForToast(page, 'Saved settings');
  await waitView(page, (x) => x.settings.agentStatus === false && x.settings.agentWords === true);
  await waitUntil(() => /^agent_status = false\s/m.test(fs.readFileSync(path.join(preview.home, 'config.toml'), 'utf8')),
    'config.toml to have agent status off');

  await page.click('[data-term="' + term + '"] .xterm');
  await typeLine(page, 'agent 1 blocked permission ' + APPROVE);
  await typeLine(page, 'bell 1');
  await switchTo(page, 'Comms');
  const v = await waitView(page, (x, id) => Boolean(x.items[id].attention), term);
  assert.equal(v.items[term].attention, 'Rang the bell');
  assert.equal(v.items[term].agent, undefined);
  assert.equal(v.tabs[tabIndex(v, 'Code')].waiting, undefined);
});
