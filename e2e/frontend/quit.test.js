'use strict';

// The questions Bench asks before ending terminals that are running
// something: before quitting, in the main window (confirm-quit), and before
// closing a popped-out terminal's window, in that window
// (confirm-close-terminal).

const assert = require('node:assert/strict');
const test = require('node:test');
const { openBench, openSettings, openTerminalWindow, settle } = require('./helpers/harness.js');

// BUSY are the terminals Go names as running something.
const BUSY = [
  { id: 'w5', title: 'claude', tab: 'Code', running: 'claude' },
  { id: 'w6', title: 'npm run dev', tab: 'Web', running: 'npm run dev' },
];

// askToQuit sends Go's confirm-quit command, and waits for the question.
async function askToQuit(page, fake, terminals = BUSY) {
  await fake.emit('bench:command', { name: 'confirm-quit', terminals });
  await page.waitForSelector('.question-sheet');
}

const focusedKey = (page) => page.evaluate(() => document.activeElement && document.activeElement.dataset.key);

test('the question before quitting names the terminals by sub-tab title and tab, and its Quit quits', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await askToQuit(page, fake);
  assert.equal(await page.getAttribute('.question-sheet', 'role'), 'alertdialog');
  assert.equal(await page.textContent('#question-title'), 'Quit Bench?');
  assert.equal(await page.textContent('#question-text'),
    '2 terminals are still running: “claude” (Code), “npm run dev” (Web). Quit Bench and end them?');
  assert.deepEqual(await page.$$eval('.question-sheet button', (els) => els.map((el) => el.textContent)), ['Cancel', 'Quit']);
  assert.equal(await page.textContent('.question-sheet .toggle span'), 'Don\'t ask again');
  assert.equal(await page.isChecked('[data-question="dont-ask"]'), false);
  // Bench is raised above docked windows while it asks, and Return quits.
  await fake.waitForCall('SetOverlay', (a) => a[0] === true);
  assert.equal(await focusedKey(page), 'question-run');
  assert.equal(fake.callsTo('Quit').length, 0);

  await page.keyboard.press('Enter');
  assert.deepEqual((await fake.waitForCall('Quit')).args, [false]);
  await page.waitForSelector('.question-sheet', { state: 'detached' });
  await fake.waitForLowered();
  assert.equal(fake.callsTo('Quit').length, 1);
});

test('the question\'s Don\'t ask again turns it off, and Settings shows it off', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await askToQuit(page, fake);
  await page.click('[data-question="dont-ask"]');
  await page.click('[data-action="quit-run"]');
  assert.deepEqual((await fake.waitForCall('Quit')).args, [true]);
  await page.waitForSelector('.question-sheet', { state: 'detached' });
  await page.waitForFunction(() => window.bench.store.view.settings.confirmQuitBusyTerminals === false);
  await openSettings(page);
  assert.equal(await page.isChecked('[data-setting="confirmQuitBusyTerminals"]'), false);
});

test('Cancel, Escape, and a click outside the question leave Bench running', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  for (const how of ['cancel', 'escape', 'backdrop']) {
    await askToQuit(page, fake);
    if (how === 'cancel') await page.click('[data-key="question-cancel"]');
    else if (how === 'escape') await page.keyboard.press('Escape');
    else await page.mouse.click(10, 880);
    await page.waitForSelector('.question-sheet', { state: 'detached' });
  }
  await fake.waitForLowered();
  assert.equal(fake.callsTo('Quit').length, 0);
});

test('the question keeps Tab inside it', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await askToQuit(page, fake);
  const keys = [];
  for (let i = 0; i < 4; i++) {
    await page.keyboard.press('Tab');
    keys.push(await focusedKey(page));
  }
  assert.deepEqual(keys, ['question-dont-ask', 'question-cancel', 'question-run', 'question-dont-ask']);
  // Space ticks the box, which Quit sends.
  await page.keyboard.press('Space');
  await page.click('[data-action="quit-run"]');
  assert.deepEqual((await fake.waitForCall('Quit')).args, [true]);
});

test('the question names one terminal, and one in a window of its own', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await askToQuit(page, fake, [{ id: 'w5', title: 'Shell', running: 'tail' }]);
  assert.equal(await page.textContent('#question-text'),
    'A terminal is still running: “Shell” (in its own window). Quit Bench and end it?');
  // Asked again, it names the terminals anew.
  await askToQuit(page, fake, BUSY.slice(0, 1));
  await page.waitForFunction(() => document.querySelector('#question-text').textContent.includes('(Code)'));
  assert.equal(await page.textContent('#question-text'),
    'A terminal is still running: “claude” (Code). Quit Bench and end it?');
  assert.equal(await page.$$eval('.question-sheet', (els) => els.length), 1);
});

test('the question covers an open dialog, which Cancel goes back to with its draft', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await openSettings(page);
  await page.click('[data-setting="hideOtherTabs"]');
  await page.fill('#compact-below', '1600');
  await askToQuit(page, fake);
  assert.equal(await page.$('.settings-sheet'), null);
  await page.click('[data-key="question-cancel"]');
  await page.waitForSelector('.settings-sheet');
  assert.equal(await page.isChecked('[data-setting="hideOtherTabs"]'), false);
  assert.equal(await page.inputValue('#compact-below'), '1600');
  // Bench stayed raised for the dialog throughout.
  await settle(page);
  assert.ok(!fake.callsTo('SetOverlay').some((c, i, all) => i > 0 && c.args[0] === false && all[i - 1].args[0] === true),
    'Bench was lowered between the question and the dialog: ' + JSON.stringify(fake.callsTo('SetOverlay').map((c) => c.args[0])));
  assert.equal(fake.callsTo('SaveSettings').length, 0);
});

test('the main window leaves the question before closing a terminal\'s window to that window', async (t) => {
  const { page, fake, close } = await openBench();
  t.after(close);
  await fake.emit('bench:command', { name: 'confirm-close-terminal', itemId: 'w5', terminals: BUSY.slice(0, 1) });
  await settle(page);
  assert.equal(await page.$('.question-sheet'), null);
  assert.equal(await page.evaluate(() => window.bench.store.ui.overlay), null);
});

test('a popped-out terminal\'s window asks before closing while the terminal is running something', async (t) => {
  const { page, fake, close } = await openTerminalWindow({ id: 'w5' });
  t.after(close);
  await page.waitForFunction(() => document.querySelector('.terminal-window .xterm-rows').textContent.includes('go run ./cmd/bench'));
  // A question for another terminal's window isn't this window's.
  await fake.emit('bench:command', { name: 'confirm-close-terminal', itemId: 'w6', terminals: [{ id: 'w6', title: 'go test', running: 'go' }] });
  await fake.emit('bench:command', { name: 'confirm-quit', terminals: BUSY });
  await settle(page);
  assert.equal(await page.$('.question-sheet'), null);

  const ask = async () => {
    await fake.emit('bench:command', {
      name: 'confirm-close-terminal', itemId: 'w5', terminals: [{ id: 'w5', title: 'zsh — ~/code/bench', running: 'claude' }],
    });
    await page.waitForSelector('.question-sheet');
  };
  await ask();
  assert.equal(await page.getAttribute('.question-sheet', 'role'), 'alertdialog');
  assert.equal(await page.textContent('#question-title'), 'Close this terminal?');
  assert.equal(await page.textContent('#question-text'), '“claude” is still running in this terminal. Close the window and end it?');
  assert.deepEqual(await page.$$eval('.question-sheet button', (els) => els.map((el) => el.textContent)), ['Cancel', 'Close']);
  assert.equal(await focusedKey(page), 'question-run');

  // Escape keeps the terminal, which gets the keyboard back, and nothing
  // reaches it.
  await page.keyboard.press('Escape');
  await page.waitForSelector('.question-sheet', { state: 'detached' });
  assert.equal(await page.evaluate(() => Boolean(document.activeElement.closest('.term-host[data-term-host="w5"]'))), true);
  assert.deepEqual(fake.callsTo('TermInput').filter((c) => c.args[1].includes('\x1b')), []);
  // So does Cancel; Tab stays in the question.
  await ask();
  await page.keyboard.press('Tab');
  assert.equal(await focusedKey(page), 'question-dont-ask');
  await page.keyboard.press('Shift+Tab');
  assert.equal(await focusedKey(page), 'question-run');
  await page.click('[data-key="question-cancel"]');
  await page.waitForSelector('.question-sheet', { state: 'detached' });
  assert.equal(fake.callsTo('CloseTerminal').length, 0);

  // Close, with Don't ask again, ends the terminal and turns the question
  // off. Go's toast that says so is for the main window, as this one
  // closes.
  await ask();
  await page.click('[data-question="dont-ask"]');
  await page.click('[data-action="close-run"]');
  assert.deepEqual((await fake.waitForCall('CloseTerminal')).args, ['w5', true]);
  await page.waitForSelector('.terminal-window .empty');
  assert.equal(await page.textContent('.terminal-window .empty'), 'This terminal has closed.');
  assert.equal(await page.$('.question-sheet'), null);
  assert.equal(fake.world.settings.confirmQuitBusyTerminals, false);
  for (const m of ['Ready', 'SetOverlay', 'Quit']) assert.equal(fake.callsTo(m).length, 0, m);
});

test('a refused Close keeps the question, and says why', async (t) => {
  const { page, fake, close } = await openTerminalWindow({
    id: 'w5',
    setup: (f) => f.on('CloseTerminal', () => { throw new Error('That terminal has closed.'); }),
  });
  t.after(close);
  await fake.emit('bench:command', { name: 'confirm-close-terminal', itemId: 'w5', terminals: [{ id: 'w5', title: 'zsh', running: 'vim' }] });
  await page.waitForSelector('.question-sheet');
  await page.click('[data-action="close-run"]');
  await page.waitForFunction(() => document.querySelector('#toast').textContent === 'That terminal has closed.');
  assert.equal(await page.isEnabled('[data-action="close-run"]'), true);
  assert.equal(await focusedKey(page), 'question-run');
});
