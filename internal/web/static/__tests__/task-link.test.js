import { strict as assert } from 'node:assert';
import { describe, it } from 'node:test';

import {
  buildTaskHref,
  hashWithoutTask,
  parseTaskHash,
  parseTaskURL,
  projectFromPath,
  taskHash,
} from '../utils/task-link.js';

describe('projectFromPath', () => {
  it('reads the project of a project route', () => {
    assert.equal(projectFromPath('/projects/p1/board/kanban'), 'p1');
    assert.equal(projectFromPath('/projects/p1'), 'p1');
    assert.equal(projectFromPath('/projects/p1/'), 'p1');
  });
  it('is null on global pages', () => {
    assert.equal(projectFromPath('/live'), null);
    assert.equal(projectFromPath('/projects'), null);
    assert.equal(projectFromPath('/'), null);
    assert.equal(projectFromPath(''), null);
  });
});

describe('buildTaskHref', () => {
  it('uses the route project without naming it', () => {
    assert.equal(
      buildTaskHref('t1', 'p1', { pathname: '/projects/p1/live', hash: '' }),
      '/projects/p1/live#task=t1',
    );
  });
  it('names the project on global pages', () => {
    assert.equal(
      buildTaskHref('t1', 'p1', { pathname: '/questions', hash: '' }),
      '/questions#task=t1&project=p1',
    );
  });
  it('names the project when it differs from the route', () => {
    assert.equal(
      buildTaskHref('t1', 'p2', { pathname: '/projects/p1/docs', hash: '' }),
      '/projects/p1/docs#task=t1&project=p2',
    );
  });
  it('keeps the page hash state and replaces an open task', () => {
    assert.equal(
      buildTaskHref('t2', 'p1', {
        pathname: '/projects/p1/board/kanban',
        hash: '#cycle=c1&mine=1&task=t1',
      }),
      '/projects/p1/board/kanban#cycle=c1&mine=1&task=t2',
    );
  });
  it('drops hashes that are not key=value state', () => {
    assert.equal(
      buildTaskHref('t1', 'p1', { pathname: '/projects/p1/questions', hash: '#q-abc' }),
      '/projects/p1/questions#task=t1',
    );
  });
  it('keeps the query string', () => {
    assert.equal(
      buildTaskHref('t1', 'p1', { pathname: '/live', search: '?x=1', hash: '' }),
      '/live?x=1#task=t1&project=p1',
    );
  });
});

describe('taskHash / hashWithoutTask', () => {
  it('round-trips the page state', () => {
    const h = taskHash('t1', 'p9', { pathname: '/projects/p1/docs', hash: '#path=a%2Fb.md' });
    assert.equal(h, '#path=a%2Fb.md&task=t1&project=p9');
    assert.equal(hashWithoutTask(h), '#path=a%2Fb.md');
  });
  it('is empty when only the task was there', () => {
    assert.equal(hashWithoutTask('#task=t1&project=p1'), '');
    assert.equal(hashWithoutTask(''), '');
  });
});

describe('parseTaskHash', () => {
  const T = '5fbe3c57-6c98-4d1c-8368-fcc75d1ccff9';
  const P = 'e55a6659-991b-4d36-8894-16b8dac4486c';
  const P2 = 'fcedb5de-663d-4064-96a3-d8b1388f6340';
  it('resolves the project from the route', () => {
    assert.deepEqual(parseTaskHash(`/projects/${P}/board/gantt`, `#task=${T}`), {
      taskId: T,
      projectId: P,
    });
  });
  it('prefers the explicit project', () => {
    assert.deepEqual(parseTaskHash(`/projects/${P}/docs`, `#task=${T}&project=${P2}`), {
      taskId: T,
      projectId: P2,
    });
    assert.deepEqual(parseTaskHash('/live', `#task=${T}&project=${P}`), {
      taskId: T,
      projectId: P,
    });
  });
  it('is null without a task or a project', () => {
    assert.equal(parseTaskHash(`/projects/${P}/board`, '#cycle=c1'), null);
    assert.equal(parseTaskHash('/live', `#task=${T}`), null);
    assert.equal(parseTaskHash('/live', ''), null);
  });
  it('ignores ids that are not UUID-shaped', () => {
    assert.equal(parseTaskHash(`/projects/${P}/board`, '#task=../../me'), null);
    assert.equal(parseTaskHash(`/projects/${P}/board`, '#task=t1'), null);
    assert.equal(parseTaskHash('/live', `#task=${T}&project=../x`), null);
    assert.equal(parseTaskHash('/projects/p1/board', `#task=${T}`), null);
  });
});

describe('parseTaskURL', () => {
  const origin = 'http://localhost:8080';
  const T = '5fbe3c57-6c98-4d1c-8368-fcc75d1ccff9';
  const P = 'e55a6659-991b-4d36-8894-16b8dac4486c';
  it('reads the legacy kanban form from markdown chips', () => {
    assert.deepEqual(parseTaskURL(`/projects/${P}/board/kanban#task=${T}`, origin), {
      taskId: T,
      projectId: P,
    });
  });
  it('reads absolute same-origin links', () => {
    assert.deepEqual(parseTaskURL(`http://localhost:8080/live#task=${T}&project=${P}`, origin), {
      taskId: T,
      projectId: P,
    });
  });
  it('ignores other origins, bad ids and non-task links', () => {
    assert.equal(parseTaskURL(`https://github.com/x/y#task=${T}&project=${P}`, origin), null);
    assert.equal(parseTaskURL(`/projects/${P}/board#task=../../me`, origin), null);
    assert.equal(parseTaskURL(`/projects/${P}/docs#path=x`, origin), null);
    assert.equal(parseTaskURL('', origin), null);
  });
});
