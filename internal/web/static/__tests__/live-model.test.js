import { strict as assert } from 'node:assert';
import { describe, it } from 'node:test';

import {
  activityState,
  isIdle,
  RESUME_COMMANDS,
  resumeCommand,
  sessionCopyText,
  sessionName,
  sortWaiting,
  stepLine,
} from '../live-model.js';

const KEY = '3f2a9c10-7b44-4e1d-9a8e-0c5d6e7f8a9b';

describe('RESUME_COMMANDS', () => {
  it('maps lowercase client names to a command builder', () => {
    for (const [name, build] of Object.entries(RESUME_COMMANDS)) {
      assert.equal(name, name.toLowerCase());
      assert.equal(typeof build, 'function');
      assert.ok(build(KEY).includes(KEY));
    }
  });
});

describe('resumeCommand', () => {
  it('builds the command of a known client, whatever the case', () => {
    assert.equal(resumeCommand({ key: KEY, client: 'claude-code' }), `claude --resume ${KEY}`);
    assert.equal(resumeCommand({ key: KEY, client: 'claude-cli' }), `claude --resume ${KEY}`);
    assert.equal(resumeCommand({ key: KEY, client: 'Claude-Code' }), `claude --resume ${KEY}`);
  });

  it('gives no command for a client without an entry', () => {
    for (const client of ['cursor', 'claude-ai', 'claude', 'codex-mcp-client', '', 'toString']) {
      assert.equal(resumeCommand({ key: KEY, client }), null, client);
    }
  });

  it('gives no command without a session, a key or a client', () => {
    assert.equal(resumeCommand(null), null);
    assert.equal(resumeCommand(undefined), null);
    assert.equal(resumeCommand({ client: 'claude-code' }), null);
    assert.equal(resumeCommand({ key: KEY }), null);
    assert.equal(resumeCommand({ key: KEY, client: null }), null);
  });
});

describe('sessionCopyText', () => {
  it('copies the resume command when the client has one and the bare key otherwise', () => {
    assert.equal(sessionCopyText({ key: KEY, client: 'claude-cli' }), `claude --resume ${KEY}`);
    assert.equal(sessionCopyText({ key: KEY, client: 'cursor' }), KEY);
    assert.equal(sessionCopyText({ key: KEY }), KEY);
    assert.equal(sessionCopyText(null), '');
  });
});

describe('sessionName', () => {
  it('prefers the label', () => {
    assert.equal(sessionName({ key: KEY, label: 'migration work' }), 'migration work');
  });

  it('falls back to the first 8 characters of the key without a label', () => {
    assert.equal(sessionName({ key: KEY }), '3f2a9c10');
    assert.equal(sessionName({ key: KEY, label: '' }), '3f2a9c10');
  });

  it('is empty without a session', () => {
    assert.equal(sessionName(null), '');
  });
});

describe('stepLine', () => {
  it('shows the count and the in-progress step', () => {
    const item = {
      progress: { done: 3, total: 8 },
      steps: [{ id: 's1', position: 4, text: 'Write migration' }],
    };
    assert.equal(stepLine(item), '3/8 · Write migration');
  });

  it('shows only the count when no step is in progress', () => {
    assert.equal(stepLine({ progress: { done: 3, total: 8 }, steps: [] }), '3/8');
    assert.equal(stepLine({ progress: { done: 0, total: 2 } }), '0/2');
  });

  it('is empty for a task without steps', () => {
    assert.equal(stepLine({ progress: null, steps: [] }), '');
    assert.equal(stepLine({}), '');
  });
});

describe('sortWaiting', () => {
  it('orders oldest first and leaves the input alone', () => {
    const qs = [
      { id: 'b', created_at: '2026-10-30T10:05:00Z' },
      { id: 'a', created_at: '2026-10-30T09:00:00Z' },
      { id: 'c', created_at: '2026-10-30T11:00:00Z' },
    ];
    assert.deepEqual(
      sortWaiting(qs).map((q) => q.id),
      ['a', 'b', 'c'],
    );
    assert.deepEqual(
      qs.map((q) => q.id),
      ['b', 'a', 'c'],
    );
  });
});

describe('isIdle', () => {
  const now = Date.parse('2026-10-30T12:00:00Z');

  it('is idle past the threshold', () => {
    assert.ok(isIdle({ last_activity_at: '2026-10-30T11:29:00Z' }, 30, now));
  });

  it('is not idle within the threshold', () => {
    assert.ok(!isIdle({ last_activity_at: '2026-10-30T11:31:00Z' }, 30, now));
    assert.ok(!isIdle({ last_activity_at: '2026-10-30T11:30:00Z' }, 30, now));
  });

  it('is not idle without recorded activity', () => {
    assert.ok(!isIdle({}, 30, now));
    assert.ok(!isIdle({ last_activity_at: null }, 30, now));
  });
});

describe('activityState', () => {
  const now = Date.parse('2026-10-30T12:00:00Z');
  const at = (min) => ({ last_activity_at: new Date(now - min * 60_000).toISOString() });

  it('is recent under two minutes', () => {
    assert.equal(activityState(at(1), 30, now), 'recent');
  });

  it('is quiet between recent and idle', () => {
    assert.equal(activityState(at(5), 30, now), 'quiet');
  });

  it('is idle past the threshold or when the server says so', () => {
    assert.equal(activityState(at(31), 30, now), 'idle');
    assert.equal(activityState({ ...at(1), idle: true }, 30, now), 'idle');
  });
});
