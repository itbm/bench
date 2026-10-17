'use strict';

// A small static file server for the frontend tests, with no dependencies.

const fs = require('node:fs');
const http = require('node:http');
const path = require('node:path');

const TYPES = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.mjs': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.json': 'application/json',
  '.woff2': 'font/woff2',
  '.png': 'image/png',
  '.svg': 'image/svg+xml',
  '.txt': 'text/plain; charset=utf-8',
  '.md': 'text/plain; charset=utf-8',
};

// serve serves each root's folder under its URL prefix, on a random port on
// 127.0.0.1. It resolves with { url, close }.
function serve(roots) {
  const server = http.createServer((req, res) => {
    const url = new URL(req.url, 'http://localhost');
    let pathname;
    try {
      pathname = decodeURIComponent(url.pathname);
    } catch {
      res.writeHead(400).end('Bad request');
      return;
    }
    const root = roots.find((r) => pathname.startsWith(r.prefix));
    if (!root) {
      res.writeHead(404).end('Not found');
      return;
    }
    let rel = pathname.slice(root.prefix.length);
    if (rel === '' || rel.endsWith('/')) rel += 'index.html';
    const file = path.resolve(root.dir, rel);
    if (file !== root.dir && !file.startsWith(root.dir + path.sep)) {
      res.writeHead(403).end('Forbidden');
      return;
    }
    fs.readFile(file, (err, data) => {
      if (err) {
        res.writeHead(404).end('Not found');
        return;
      }
      res.writeHead(200, {
        'Content-Type': TYPES[path.extname(file)] || 'application/octet-stream',
        'Cache-Control': 'no-store',
      });
      res.end(data);
    });
  });
  return new Promise((resolve, reject) => {
    server.once('error', reject);
    server.listen(0, '127.0.0.1', () => {
      const { port } = server.address();
      resolve({
        url: 'http://127.0.0.1:' + port,
        close: () => new Promise((r) => server.close(() => r())),
      });
    });
  });
}

module.exports = { serve };
