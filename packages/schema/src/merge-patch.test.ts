import { describe, expect, it } from 'vitest';
import { mergePatch } from './merge-patch.js';

// RFC 7386 Appendix A test cases (original, patch, result).
const rfc7386: [unknown, unknown, unknown][] = [
  [{ a: 'b' }, { a: 'c' }, { a: 'c' }],
  [{ a: 'b' }, { b: 'c' }, { a: 'b', b: 'c' }],
  [{ a: 'b' }, { a: null }, {}],
  [{ a: 'b', b: 'c' }, { a: null }, { b: 'c' }],
  [{ a: ['b'] }, { a: 'c' }, { a: 'c' }],
  [{ a: 'c' }, { a: ['b'] }, { a: ['b'] }],
  [{ a: { b: 'c' } }, { a: { b: 'd', c: null } }, { a: { b: 'd' } }],
  [{ a: [{ b: 'c' }] }, { a: [1] }, { a: [1] }],
  [
    ['a', 'b'],
    ['c', 'd'],
    ['c', 'd'],
  ],
  [{ a: 'b' }, ['c'], ['c']],
  [{ a: 'foo' }, null, null],
  [{ a: 'foo' }, 'bar', 'bar'],
  [{ e: null }, { a: 1 }, { e: null, a: 1 }],
  [[1, 2], { a: 'b', c: null }, { a: 'b' }],
  [{}, { a: { bb: { ccc: null } } }, { a: { bb: {} } }],
];

describe('mergePatch (RFC 7386)', () => {
  it.each(rfc7386)('Appendix A: %j + %j → %j', (original, patch, expected) => {
    expect(mergePatch(original, patch)).toEqual(expected);
  });

  it('ignores undefined patch members', () => {
    expect(mergePatch({ a: 1 }, { a: undefined, b: 2 })).toEqual({ a: 1, b: 2 });
  });

  it('does not mutate target or patch', () => {
    const target = { system: { hostname: 'a', banner: 'x' } };
    const patch = { system: { banner: null, timezone: 'UTC' } };
    const result = mergePatch(target, patch);
    expect(result).toEqual({ system: { hostname: 'a', timezone: 'UTC' } });
    expect(target).toEqual({ system: { hostname: 'a', banner: 'x' } });
    expect(patch).toEqual({ system: { banner: null, timezone: 'UTC' } });
  });
});
