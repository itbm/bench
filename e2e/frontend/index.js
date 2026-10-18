'use strict';

// The entry point for `node --test e2e/frontend/`: Node runs a folder's
// index.js, so this loads every *.test.js file here. Each file also runs on
// its own, as in `node --test e2e/frontend/drag.test.js`.

const fs = require('node:fs');
const path = require('node:path');

for (const name of fs.readdirSync(__dirname).filter((n) => n.endsWith('.test.js')).sort()) {
  require(path.join(__dirname, name));
}
