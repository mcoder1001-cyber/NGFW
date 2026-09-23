import { describe, expect, it } from 'vitest';
import { mergePatch, mergePatchAt } from './merge-patch.js';

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

describe('mergePatchAt (PATCH /api/v1/config/{path})', () => {
  const doc = {
    system: { hostname: 'a', banner: { motd: 'hi' } },
    routing: { static: [{ prefix: '0.0.0.0/0', nextHops: [{ address: '10.0.0.1' }] }] },
  };

  it('applies the patch at the pointer and leaves the rest untouched', () => {
    expect(mergePatchAt(doc, '/system', { hostname: 'b', banner: null })).toEqual({
      system: { hostname: 'b' },
      routing: doc.routing,
    });
    expect(mergePatchAt(doc, '', { system: null })).toEqual({ routing: doc.routing });
  });

  it('creates missing objects along the path and escapes interface names', () => {
    expect(mergePatchAt({}, '/interfaces/TenGigabitEthernet0~10~10', { mtu: 9000 })).toEqual({
      interfaces: { 'TenGigabitEthernet0/0/0': { mtu: 9000 } },
    });
    expect(mergePatchAt({ a: 1 }, '/a/b', { c: 2 })).toEqual({ a: { b: { c: 2 } } });
  });

  it('patches into arrays by index and appends at the length', () => {
    expect(mergePatchAt(doc, '/routing/static/0/nextHops/0', { weight: 5 })).toEqual({
      ...doc,
      routing: { static: [{ prefix: '0.0.0.0/0', nextHops: [{ address: '10.0.0.1', weight: 5 }] }] },
    });
    expect(mergePatchAt({ l: [1] }, '/l/1', 2)).toEqual({ l: [1, 2] });
    expect(() => mergePatchAt({ l: [1] }, '/l/x', 2)).toThrow(/invalid array index 'x'/);
    expect(() => mergePatchAt({ l: [1] }, '/l/2', 2)).toThrow(/invalid array index '2'/);
    expect(() => mergePatchAt({}, 'nope', 1)).toThrow(/invalid JSON pointer/);
  });

  it('does not mutate the document', () => {
    const before = JSON.stringify(doc);
    mergePatchAt(doc, '/system/banner', { motd: null });
    mergePatchAt(doc, '/routing/static/0', { distance: 5 });
    expect(JSON.stringify(doc)).toBe(before);
  });
});
