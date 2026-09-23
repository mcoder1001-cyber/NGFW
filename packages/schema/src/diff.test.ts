import { describe, expect, it } from 'vitest';
import { diff } from './diff.js';

describe('diff', () => {
  it('returns no changes for structurally equal documents', () => {
    expect(diff({ a: { b: [1, 2] } }, { a: { b: [1, 2] } })).toEqual([]);
    expect(diff({}, {})).toEqual([]);
  });

  it('reports add / remove / replace with RFC 6901 pointers', () => {
    const running = { system: { hostname: 'old' }, vrfs: { default: { id: 0 } } };
    const candidate = { system: { hostname: 'new', banner: 'hi' }, interfaces: {} };
    expect(diff(running, candidate)).toEqual([
      { op: 'add', pointer: '/interfaces', to: {} },
      { op: 'add', pointer: '/system/banner', to: 'hi' },
      { op: 'replace', pointer: '/system/hostname', from: 'old', to: 'new' },
      { op: 'remove', pointer: '/vrfs', from: { default: { id: 0 } } },
    ]);
  });

  it('escapes VPP interface names in pointers', () => {
    const changes = diff(
      { interfaces: { 'TenGigabitEthernet0/0/0': { mtu: 1500 } } },
      { interfaces: { 'TenGigabitEthernet0/0/0': { mtu: 9000 } } },
    );
    expect(changes).toEqual([
      { op: 'replace', pointer: '/interfaces/TenGigabitEthernet0~10~10/mtu', from: 1500, to: 9000 },
    ]);
  });

  it('treats arrays as leaves (one replace for the whole array)', () => {
    expect(diff({ s: [1, 2] }, { s: [1, 3] })).toEqual([
      { op: 'replace', pointer: '/s', from: [1, 2], to: [1, 3] },
    ]);
  });

  it('treats undefined-valued keys as absent and handles root scalars', () => {
    expect(diff({ a: undefined }, {})).toEqual([]);
    expect(diff(1, 2)).toEqual([{ op: 'replace', pointer: '', from: 1, to: 2 }]);
    expect(diff({ a: 1 }, null)).toEqual([
      { op: 'replace', pointer: '', from: { a: 1 }, to: null },
    ]);
  });

  it('does not mutate its inputs', () => {
    const a = { x: { y: 1 } };
    const b = { x: { y: 2 } };
    diff(a, b);
    expect(a).toEqual({ x: { y: 1 } });
    expect(b).toEqual({ x: { y: 2 } });
  });
});
