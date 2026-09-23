import { describe, expect, it } from 'vitest';
import { deepEqual, isPlainObject } from './json.js';

describe('isPlainObject', () => {
  it.each([
    [{}, true],
    [{ a: 1 }, true],
    [[], false],
    [null, false],
    ['s', false],
    [1, false],
    [undefined, false],
  ])('%j → %s', (value, expected) => {
    expect(isPlainObject(value)).toBe(expected);
  });
});

describe('deepEqual', () => {
  it('compares nested JSON structurally', () => {
    expect(deepEqual({ a: [1, { b: null }] }, { a: [1, { b: null }] })).toBe(true);
    expect(deepEqual({ a: 1, b: 2 }, { b: 2, a: 1 })).toBe(true);
  });
  it('detects differences in values, lengths and key sets', () => {
    expect(deepEqual({ a: 1 }, { a: 2 })).toBe(false);
    expect(deepEqual([1, 2], [1, 2, 3])).toBe(false);
    expect(deepEqual({ a: 1 }, { a: 1, b: 1 })).toBe(false);
    expect(deepEqual({ a: 1 }, { b: 1 })).toBe(false);
    expect(deepEqual([], {})).toBe(false);
    expect(deepEqual(1, '1')).toBe(false);
  });
});
