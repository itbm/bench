'use strict';

// The fixture's layout maths against the cases in the spec's "Layout maths"
// section, so that the rectangles the UI tests use are the ones Go sends.

const assert = require('node:assert/strict');
const test = require('node:test');
const { body, dividers, frames, pane, split } = require('./helpers/fixture.js');

const p = (id) => pane(id, []);
const two = (dir, sizes) => split('s', dir, sizes, [p('a'), p('b')]);

const CASES = [
  {
    name: 'main-side 65, 35',
    area: { x: 0, y: 25, w: 2560, h: 1415 }, gap: 8, tree: two('row', [65, 35]),
    want: [{ x: 8, y: 33, w: 1651, h: 1399 }, { x: 1667, y: 33, w: 885, h: 1399 }],
  },
  {
    name: 'side 50, 50',
    area: { x: 0, y: 25, w: 2560, h: 1415 }, gap: 8, tree: two('row', [50, 50]),
    want: [{ x: 8, y: 33, w: 1268, h: 1399 }, { x: 1284, y: 33, w: 1268, h: 1399 }],
  },
  {
    name: 'side 45, 55',
    area: { x: 0, y: 25, w: 2560, h: 1415 }, gap: 8, tree: two('row', [45, 55]),
    want: [{ x: 8, y: 33, w: 1140, h: 1399 }, { x: 1156, y: 33, w: 1396, h: 1399 }],
  },
  {
    name: 'top-bottom 65, 35',
    area: { x: 0, y: 33, w: 1512, h: 949 }, gap: 8, tree: two('column', [65, 35]),
    want: [{ x: 8, y: 41, w: 1496, h: 604 }, { x: 8, y: 653, w: 1496, h: 321 }],
  },
  {
    name: 'side 50, 50 in a small area',
    area: { x: 0, y: 0, w: 1000, h: 800 }, gap: 8, tree: two('row', [50, 50]),
    want: [{ x: 8, y: 8, w: 488, h: 784 }, { x: 504, y: 8, w: 488, h: 784 }],
  },
  {
    name: 'grid',
    area: { x: 0, y: 0, w: 1000, h: 800 }, gap: 8,
    tree: split('s', 'column', [50, 50], [split('t', 'row', [50, 50], [p('a'), p('b')]), split('u', 'row', [50, 50], [p('c'), p('d')])]),
    want: [{ x: 8, y: 8, w: 488, h: 388 }, { x: 504, y: 8, w: 488, h: 388 }, { x: 8, y: 404, w: 488, h: 388 }, { x: 504, y: 404, w: 488, h: 388 }],
  },
  {
    name: 'row 65, 35 with a column on the right',
    area: { x: 0, y: 25, w: 2560, h: 1415 }, gap: 8,
    tree: split('s', 'row', [65, 35], [p('a'), split('t', 'column', [50, 50], [p('b'), p('c')])]),
    want: [{ x: 8, y: 33, w: 1651, h: 1399 }, { x: 1667, y: 33, w: 885, h: 696 }, { x: 1667, y: 737, w: 885, h: 696 }],
  },
  {
    name: 'single with no gap',
    area: { x: 100, y: 25, w: 500, h: 400 }, gap: 0, tree: p('a'),
    want: [{ x: 100, y: 25, w: 500, h: 400 }],
  },
];

for (const c of CASES) {
  test('fixture frames: ' + c.name, () => {
    assert.deepEqual(frames(c.tree, c.area, c.gap).map((f) => f.rect), c.want);
  });
}

// Divider cases from internal/layout's TestDividers.
const DIVIDERS = [
  {
    name: 'side 50, 50',
    area: { x: 0, y: 0, w: 1000, h: 800 }, gap: 8, tree: split('s1', 'row', [50, 50], [p('p1'), p('p2')]),
    want: [{ splitId: 's1', index: 0, dir: 'row', rect: { x: 496, y: 8, w: 8, h: 784 }, splitRect: { x: 8, y: 8, w: 984, h: 784 }, sizes: [50, 50] }],
  },
  {
    name: 'three 40, 30, 30, where rounding makes the second gap 7 pt',
    area: { x: 0, y: 0, w: 1000, h: 800 }, gap: 8, tree: split('s1', 'row', [40, 30, 30], [p('p1'), p('p2'), p('p3')]),
    want: [
      { splitId: 's1', index: 0, dir: 'row', rect: { x: 397, y: 8, w: 8, h: 784 }, splitRect: { x: 8, y: 8, w: 984, h: 784 }, sizes: [40, 30, 30] },
      { splitId: 's1', index: 1, dir: 'row', rect: { x: 695, y: 8, w: 7, h: 784 }, splitRect: { x: 8, y: 8, w: 984, h: 784 }, sizes: [40, 30, 30] },
    ],
  },
  {
    name: 'nested spec case',
    area: { x: 0, y: 25, w: 2560, h: 1415 }, gap: 8,
    tree: split('s1', 'row', [65, 35], [p('p1'), split('s2', 'column', [50, 50], [p('p2'), p('p3')])]),
    want: [
      { splitId: 's1', index: 0, dir: 'row', rect: { x: 1659, y: 33, w: 8, h: 1399 }, splitRect: { x: 8, y: 33, w: 2544, h: 1399 }, sizes: [65, 35] },
      { splitId: 's2', index: 0, dir: 'column', rect: { x: 1667, y: 729, w: 885, h: 8 }, splitRect: { x: 1667, y: 33, w: 885, h: 1399 }, sizes: [50, 50] },
    ],
  },
  {
    name: 'no gap: 1 pt across, centred where the panes meet',
    area: { x: 0, y: 0, w: 1000, h: 800 }, gap: 0, tree: split('s1', 'row', [30, 70], [p('p1'), p('p2')]),
    want: [{ splitId: 's1', index: 0, dir: 'row', rect: { x: 299.5, y: 0, w: 1, h: 800 }, splitRect: { x: 0, y: 0, w: 1000, h: 800 }, sizes: [30, 70] }],
  },
];

for (const c of DIVIDERS) {
  test('fixture dividers: ' + c.name, () => {
    const got = dividers(c.tree, c.area, c.gap).map((d) => ({ ...d, sizes: d.sizes.map((v) => Math.round(v * 1e6) / 1e6) }));
    assert.deepEqual(got, c.want);
  });
}

test('fixture bodies match layout.Body', () => {
  assert.deepEqual(body({ x: 8, y: 33, w: 1651, h: 1399 }), { x: 9, y: 75, w: 1649, h: 1356 });
  assert.deepEqual(body({ x: 0, y: 0, w: 100, h: 100 }), { x: 1, y: 42, w: 98, h: 57 });
  assert.deepEqual(body({ x: 10, y: 10, w: 1, h: 20 }), { x: 11, y: 52, w: 0, h: 0 });
});
