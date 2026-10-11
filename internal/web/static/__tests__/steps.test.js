import { strict as assert } from 'node:assert';
import { describe, it } from 'node:test';

import { moveIndex, stepProgress, stepTextsFromPaste } from '../steps.js';

describe('stepTextsFromPaste', () => {
  it('makes one step per non-empty line', () => {
    assert.deepEqual(stepTextsFromPaste('one\n\n  two  \r\nthree\r'), ['one', 'two', 'three']);
  });
  it('strips list and checkbox markers', () => {
    assert.deepEqual(stepTextsFromPaste('- a\n* b\n+ c\n1. d\n2) e\n- [ ] f\n- [x] g\n[X] h'), [
      'a',
      'b',
      'c',
      'd',
      'e',
      'f',
      'g',
      'h',
    ]);
  });
  it('keeps text that only looks like a marker mid-line', () => {
    assert.deepEqual(stepTextsFromPaste('Bump v1.2 - then tag'), ['Bump v1.2 - then tag']);
  });
  it('drops lines that are only a marker', () => {
    assert.deepEqual(stepTextsFromPaste('- \n- [ ] \nreal'), ['real']);
  });
});

describe('stepProgress', () => {
  it('counts done over total', () => {
    assert.deepEqual(stepProgress([{ done: true }, { done: false }, { done: true }]), {
      done: 2,
      total: 3,
    });
    assert.deepEqual(stepProgress(null), { done: 0, total: 0 });
  });
});

describe('moveIndex', () => {
  const list = ['a', 'b', 'c', 'd'].map((text, position) => ({ text, position }));
  const texts = (l) => l.map((s) => s.text).join('');
  it('moves up and down and rewrites positions', () => {
    assert.equal(texts(moveIndex(list, 3, 0)), 'dabc');
    assert.equal(texts(moveIndex(list, 0, 2)), 'bcad');
    assert.deepEqual(
      moveIndex(list, 3, 0).map((s) => s.position),
      [0, 1, 2, 3],
    );
  });
  it('clamps and leaves the input alone', () => {
    assert.equal(texts(moveIndex(list, 1, 99)), 'acdb');
    assert.equal(texts(list), 'abcd');
  });
});
