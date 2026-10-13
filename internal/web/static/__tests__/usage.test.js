import { strict as assert } from 'node:assert';
import { describe, it } from 'node:test';

import { approxTokens, formatBytes, formatCount, perCall, shortToolName } from '../usage.js';

describe('formatBytes', () => {
  it('keeps bytes under a kilobyte', () => {
    assert.equal(formatBytes(0), '0 B');
    assert.equal(formatBytes(1023), '1023 B');
  });
  it('uses one decimal under ten units, none above', () => {
    assert.equal(formatBytes(1536), '1.5 KB');
    assert.equal(formatBytes(20 * 1024), '20 KB');
    assert.equal(formatBytes(3.2 * 1024 * 1024), '3.2 MB');
  });
  it('treats junk as zero', () => {
    assert.equal(formatBytes(undefined), '0 B');
    assert.equal(formatBytes(-5), '0 B');
  });
});

describe('approxTokens', () => {
  it('counts about four bytes per token', () => {
    assert.equal(approxTokens(4000), 1000);
    assert.equal(approxTokens(null), 0);
  });
});

describe('formatCount', () => {
  it('separates thousands, then abbreviates', () => {
    assert.equal(formatCount(1234), '1,234');
    assert.equal(formatCount(45678), '46k');
    assert.equal(formatCount(2500000), '2.5M');
  });
});

describe('perCall and shortToolName', () => {
  it('averages without dividing by zero', () => {
    assert.equal(perCall(1000, 4), 250);
    assert.equal(perCall(1000, 0), 0);
  });
  it('drops the shared prefix', () => {
    assert.equal(shortToolName('nottario.tasks.get'), 'tasks.get');
    assert.equal(shortToolName('tools/list'), 'tools/list');
  });
});
