import { strict as assert } from 'node:assert';
import { describe, it } from 'node:test';

import { PROJECT_VIEWS, defaultPathFor, viewByKey } from '../views.js';

// The view registry decides where a project link lands. A stale key,
// which a project row can carry after a rename, must not strand
// somebody on a blank page.
describe('viewByKey', () => {
  it('finds every registered view', () => {
    for (const v of PROJECT_VIEWS) {
      assert.equal(viewByKey(v.key).key, v.key);
    }
  });

  it('falls back to the first view for an unknown key', () => {
    assert.equal(viewByKey('board/nonexistent').key, PROJECT_VIEWS[0].key);
    assert.equal(viewByKey(undefined).key, PROJECT_VIEWS[0].key);
  });
});

describe('defaultPathFor', () => {
  it('builds the path of the project default view', () => {
    assert.equal(
      defaultPathFor({ id: 'abc', default_view: 'board/gantt' }),
      '/projects/abc/board/gantt',
    );
  });

  it('uses the kanban board when the project has no preference', () => {
    assert.equal(defaultPathFor({ id: 'abc' }), '/projects/abc/board/kanban');
  });

  it('never returns undefined for a missing project', () => {
    assert.equal(defaultPathFor(null), '/');
    assert.equal(defaultPathFor(undefined), '/');
  });
});
