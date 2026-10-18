'use strict';

// The entry point for `node --test e2e/app/`: Node runs a folder's
// index.js, so this loads every *.test.js file here. Each file also runs on
// its own, as in `node --test e2e/app/moving.test.js`. These tests drive
// the browser preview (the server build of cmd/bench) end to end, so they
// need Go as well as Playwright; see helpers/preview.js.

const fs = require('node:fs');
const path = require('node:path');

for (const name of fs.readdirSync(__dirname).filter((n) => n.endsWith('.test.js')).sort()) {
  require(path.join(__dirname, name));
}
