// Frontend unit tests run on Node's built-in runner (`node --test`),
// so the repo keeps its no-build-step, no-dependency frontend: these
// modules are plain ES modules and import straight from source.
import { strict as assert } from 'node:assert';
import { describe, it } from 'node:test';

import { priorityBand, priorityLabel } from '../priorities.js';

const SEEDED = [
  { key: 'low', value: 30 },
  { key: 'medium', value: 60 },
  { key: 'high', value: 90 },
  { key: 'critical', value: 100 },
];

describe('priorityLabel', () => {
  it('names a value that lands on a bucket', () => {
    assert.equal(priorityLabel(60, SEEDED), 'medium');
    assert.equal(priorityLabel(100, SEEDED), 'critical');
  });

  it('falls back to p<value> when nothing matches', () => {
    // A raw priority set through the API is legal and has no name.
    assert.equal(priorityLabel(70, SEEDED), 'p70');
  });

  it('survives a project whose buckets have not loaded yet', () => {
    assert.equal(priorityLabel(60, undefined), 'p60');
    assert.equal(priorityLabel(60, []), 'p60');
  });
});

describe('priorityBand', () => {
  it('ranks against the project catalogue, not a fixed scale', () => {
    assert.equal(priorityBand(30, SEEDED), 'low');
    assert.equal(priorityBand(60, SEEDED), 'medium');
    assert.equal(priorityBand(100, SEEDED), 'high');
  });

  it('keeps the colours meaningful for an unusual catalogue', () => {
    // The point of ranking relatively: 500 is the middle of this
    // project even though it is off the 0-100 scale entirely.
    const wide = [{ value: 200 }, { value: 500 }, { value: 800 }];
    assert.equal(priorityBand(200, wide), 'low');
    assert.equal(priorityBand(500, wide), 'medium');
    assert.equal(priorityBand(800, wide), 'high');
  });

  it('places values that miss every bucket by where they fall', () => {
    assert.equal(priorityBand(70, SEEDED), 'medium');
    assert.equal(priorityBand(95, SEEDED), 'high');
    assert.equal(priorityBand(31, SEEDED), 'low');
  });

  it('does not paint everything grey before the catalogue arrives', () => {
    assert.equal(priorityBand(10, undefined), 'low');
    assert.equal(priorityBand(50, []), 'medium');
    assert.equal(priorityBand(90, null), 'high');
  });

  it('gives up gracefully when there is no spread to rank against', () => {
    assert.equal(priorityBand(5, [{ value: 5 }]), 'medium');
    assert.equal(priorityBand(5, [{ value: 5 }, { value: 5 }]), 'medium');
  });
});
