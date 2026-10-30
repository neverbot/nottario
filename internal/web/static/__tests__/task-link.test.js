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
  it('resolves the project from the route', () => {
    assert.deepEqual(parseTaskHash('/projects/p1/board/gantt', '#task=t1'), {
      taskId: 't1',
      projectId: 'p1',
    });
  });
  it('prefers the explicit project', () => {
    assert.deepEqual(parseTaskHash('/projects/p1/docs', '#task=t1&project=p2'), {
      taskId: 't1',
      projectId: 'p2',
    });
    assert.deepEqual(parseTaskHash('/live', '#task=t1&project=p2'), {
      taskId: 't1',
      projectId: 'p2',
    });
  });
  it('is null without a task or a project', () => {
    assert.equal(parseTaskHash('/projects/p1/board', '#cycle=c1'), null);
    assert.equal(parseTaskHash('/live', '#task=t1'), null);
    assert.equal(parseTaskHash('/live', ''), null);
  });
});

describe('parseTaskURL', () => {
  const origin = 'http://localhost:8080';
  it('reads the legacy kanban form from markdown chips', () => {
    assert.deepEqual(parseTaskURL('/projects/p1/board/kanban#task=t1', origin), {
      taskId: 't1',
      projectId: 'p1',
    });
  });
  it('reads absolute same-origin links', () => {
    assert.deepEqual(parseTaskURL('http://localhost:8080/live#task=t1&project=p1', origin), {
      taskId: 't1',
      projectId: 'p1',
    });
  });
  it('ignores other origins and non-task links', () => {
    assert.equal(parseTaskURL('https://github.com/x/y#task=t1&project=p1', origin), null);
    assert.equal(parseTaskURL('/projects/p1/docs#path=x', origin), null);
    assert.equal(parseTaskURL('', origin), null);
  });
});
