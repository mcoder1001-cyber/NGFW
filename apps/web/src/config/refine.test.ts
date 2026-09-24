import { describe, expect, it } from 'vitest';
import { refineChanges } from './refine';

const u = (username: string, extra: Record<string, unknown> = {}) => ({ username, role: 'readonly', scope: '*', sshKeys: [], disabled: false, ...extra });

describe('refineChanges (per-item view of whole-list replaces, on x-vrx-ui.itemKey)', () => {
  it('pairs management.users on username: field change, add and remove', () => {
    const from = [u('admin', { role: 'admin' }), u('bob'), u('carol')];
    const to = [u('admin', { role: 'admin' }), u('bob', { fullName: 'Bob' }), u('dave')];
    expect(refineChanges([{ op: 'replace', pointer: '/management/users', from, to }])).toEqual([
      { op: 'add', pointer: '/management/users/1/fullName', to: 'Bob' },
      { op: 'add', pointer: '/management/users/2', to: u('dave') },
      { op: 'remove', pointer: '/management/users/2', from: u('carol') },
    ]);
  });

  it('keeps the whole-list replace for a reorder, duplicate keys or lists without an item key', () => {
    const reorder = { op: 'replace' as const, pointer: '/management/users', from: [u('a'), u('b')], to: [u('b'), u('a')] };
    expect(refineChanges([reorder])).toEqual([reorder]);
    const dup = { op: 'replace' as const, pointer: '/management/users', from: [u('a')], to: [u('a'), u('a')] };
    expect(refineChanges([dup])).toEqual([dup]);
    const scalars = { op: 'replace' as const, pointer: '/system/dns/servers', from: ['192.0.2.1'], to: ['192.0.2.2'] };
    expect(refineChanges([scalars])).toEqual([scalars]);
    const unknown = { op: 'replace' as const, pointer: '/nope/list', from: [{ a: 1 }], to: [{ a: 2 }] };
    expect(refineChanges([unknown])).toEqual([unknown]);
  });

  it('the first user added to an empty list is one add per user', () => {
    expect(refineChanges([{ op: 'replace', pointer: '/management/users', from: [], to: [u('admin', { role: 'admin' })] }])).toEqual([
      { op: 'add', pointer: '/management/users/0', to: u('admin', { role: 'admin' }) },
    ]);
  });
});
