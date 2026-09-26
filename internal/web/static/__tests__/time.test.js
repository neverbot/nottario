import { strict as assert } from 'node:assert';
import { describe, it } from 'node:test';

import { formatDate, formatDateTime, formatRelativeTime } from '../time.js';

// These helpers exist because six copy-pasted versions disagreed with
// each other. The steps below are the contract that replaced them.
describe('formatRelativeTime', () => {
  const ago = (ms) => new Date(Date.now() - ms).toISOString();
  const minute = 60_000;
  const hour = 60 * minute;
  const day = 24 * hour;

  it('walks the steps from seconds to weeks', () => {
    assert.equal(formatRelativeTime(ago(5_000)), 'just now');
    assert.equal(formatRelativeTime(ago(5 * minute)), '5m ago');
    assert.equal(formatRelativeTime(ago(3 * hour)), '3h ago');
    assert.equal(formatRelativeTime(ago(2 * day)), '2d ago');
    assert.equal(formatRelativeTime(ago(14 * day)), '2w ago');
  });

  it('crosses each boundary exactly once', () => {
    assert.equal(formatRelativeTime(ago(59_000)), 'just now');
    assert.equal(formatRelativeTime(ago(minute)), '1m ago');
    assert.equal(formatRelativeTime(ago(59 * minute)), '59m ago');
    assert.equal(formatRelativeTime(ago(hour)), '1h ago');
    assert.equal(formatRelativeTime(ago(23 * hour)), '23h ago');
    assert.equal(formatRelativeTime(ago(day)), '1d ago');
    assert.equal(formatRelativeTime(ago(6 * day)), '6d ago');
    assert.equal(formatRelativeTime(ago(7 * day)), '1w ago');
  });

  it('falls back to an absolute date past twelve weeks', () => {
    const old = ago(200 * day);
    assert.equal(formatRelativeTime(old), formatDate(old));
    assert.ok(
      !/ago$/.test(formatRelativeTime(old)),
      'an old timestamp should not read as relative',
    );
  });

  it('returns an empty string rather than "Invalid Date"', () => {
    for (const bad of [undefined, null, '', 'not a date']) {
      assert.equal(formatRelativeTime(bad), '');
    }
  });
});

describe('formatDate and formatDateTime', () => {
  it('avoid the all-numeric form that means two different days', () => {
    const iso = '2026-05-27T16:34:00Z';
    assert.ok(
      !/^\d+[/-]\d+[/-]\d+$/.test(formatDate(iso)),
      `all-numeric dates are ambiguous across locales: ${formatDate(iso)}`,
    );
    assert.match(formatDate(iso), /2026/);
  });

  it('put a time next to the date, for hover titles', () => {
    const shown = formatDateTime('2026-05-19T16:34:00Z');
    assert.match(shown, /2026/);
    assert.match(shown, /\d{1,2}:\d{2}/);
  });

  it('return an empty string on bad input', () => {
    for (const bad of [undefined, null, '', 'nonsense']) {
      assert.equal(formatDate(bad), '');
      assert.equal(formatDateTime(bad), '');
    }
  });
});
