import { mergePatch } from '@ngfw/schema';
import { describe, expect, it } from 'vitest';
import { ApiError } from '../../api-problem';
import {
  collectionModel,
  collectionRows,
  createMergePatch,
  deepEqual,
  formValueOf,
  itemPointer,
  keyLabel,
  listKeyOf,
  localizeSchema,
  mapKeyIssue,
  nest,
  nextList,
  problemAt,
  valueAt,
} from './model';

const IFACES = collectionModel({ domain: 'interfaces', ns: 'interfaces', omit: ['subinterfaces'] });
const USERS = collectionModel({ domain: 'management', path: ['users'], ns: 'users' });
const STATIC = collectionModel({ domain: 'routing', path: ['static'], ns: 'routing' });

describe('collection model from the schema alone', () => {
  it('maps: interfaces and vrfs are keyed by name with the schema propertyNames; omit drops a member from the form', () => {
    expect(IFACES.shape.kind).toBe('map');
    expect(Object.keys(IFACES.itemSchema.properties ?? {})).toContain('subinterfaces');
    expect(Object.keys(IFACES.formSchema.properties ?? {})).not.toContain('subinterfaces');
    expect(Object.keys(IFACES.formSchema.properties ?? {})).toContain('mtu');
    const vrfs = collectionModel({ domain: 'vrfs', ns: 'vrfs' });
    expect(vrfs.shape.kind).toBe('map');
    expect(vrfs.itemSchema.required).toEqual(['id']);
    // a map inside a domain whose items are a union (objects.addresses: host | network | range | fqdn …)
    const addresses = collectionModel({ domain: 'objects', path: ['addresses'], ns: 'objects' });
    expect(addresses.shape.kind).toBe('map');
    expect(addresses.itemSchema.oneOf ?? addresses.itemSchema.anyOf).toBeDefined();
  });

  it('lists: the key members come from x-vrx-ui.itemKey (single and compound)', () => {
    expect(USERS.shape).toEqual({ kind: 'list', itemKey: ['username'] });
    expect(STATIC.shape).toEqual({ kind: 'list', itemKey: ['vrf', 'prefix'] });
    expect(listKeyOf({ username: 'alice', role: 'admin' }, ['username'])).toBe('alice');
    const id = listKeyOf({ vrf: 'red', prefix: '10.0.0.0/8', nextHops: [] }, ['vrf', 'prefix']);
    expect(id).toBe('["red","10.0.0.0/8"]');
    expect(keyLabel(STATIC, id)).toBe('red 10.0.0.0/8');
    // a list two levels down (RADIUS servers keyed by address + port)
    const radius = collectionModel({ domain: 'management', path: ['aaa', 'radius', 'servers'], ns: 'management' });
    expect(radius.shape).toEqual({ kind: 'list', itemKey: ['address', 'authPort'] });
    expect(Object.keys(radius.itemSchema.properties ?? {})).toContain('secretRef');
  });

  it('nested collections: a path may pass through a map key (P08 sub-interfaces of one interface)', () => {
    const subs = collectionModel({ domain: 'interfaces', path: ['GigabitEthernet0/8/0', 'subinterfaces'], ns: 'interfaces' });
    expect(subs.shape.kind).toBe('map');
    expect(Object.keys(subs.itemSchema.properties ?? {})).toContain('vlanId');
    expect(mapKeyIssue(subs, '100')).toBeUndefined();
    expect(mapKeyIssue(subs, '0100')).toBe('invalid'); // the schema's propertyNames, not a hand-written regex
    expect(itemPointer(subs, '100')).toBe('/interfaces/GigabitEthernet0~18~10/subinterfaces/100');
    expect(nest(subs.path, { '100': null })).toEqual({ 'GigabitEthernet0/8/0': { subinterfaces: { '100': null } } });
  });

  it('refuses a path that is not a collection or does not exist', () => {
    expect(() => collectionModel({ domain: 'system', ns: 'system' })).toThrow(/additionalProperties/);
    expect(() => collectionModel({ domain: 'management', path: ['nope'], ns: 'x' })).toThrow(/no member 'nope'/);
  });

  it('validates a new map key with the schema propertyNames', () => {
    expect(mapKeyIssue(IFACES, '')).toBe('empty');
    expect(mapKeyIssue(IFACES, 'host-w1l0')).toBeUndefined();
    expect(mapKeyIssue(IFACES, 'GigabitEthernet0/8/0')).toBeUndefined();
    expect(mapKeyIssue(IFACES, '0bad')).toBe('invalid');
    expect(mapKeyIssue(IFACES, 'has space')).toBe('invalid');
  });

  it('pointers are escaped RFC 6901 (interface names contain /)', () => {
    expect(itemPointer(IFACES, 'GigabitEthernet0/8/0')).toBe('/interfaces/GigabitEthernet0~18~10');
    expect(itemPointer(USERS, 2)).toBe('/management/users/2');
  });
});

describe('values', () => {
  it('valueAt / nest / formValueOf', () => {
    expect(valueAt({ a: { b: 1 } }, ['a', 'b'])).toBe(1);
    expect(valueAt({ a: 1 }, ['a', 'b'])).toBeUndefined();
    expect(valueAt({ a: 1 }, [])).toEqual({ a: 1 });
    expect(nest(['users'], [1])).toEqual({ users: [1] });
    expect(nest(['a', 'b'], null)).toEqual({ a: { b: null } });
    expect(nest([], { x: 1 })).toEqual({ x: 1 });
    expect(formValueOf(IFACES, { mtu: 1500, subinterfaces: { '1': {} } })).toEqual({ mtu: 1500 });
    expect(formValueOf(IFACES, undefined)).toBeUndefined();
  });

  it('deepEqual ignores member order and undefined members, not array order', () => {
    expect(deepEqual({ a: 1, b: [1, { c: 2 }] }, { b: [1, { c: 2 }], a: 1, d: undefined })).toBe(true);
    expect(deepEqual([1, 2], [2, 1])).toBe(false);
    expect(deepEqual({ a: 1 }, { a: 1, b: 2 })).toBe(false);
  });

  it('createMergePatch: removed → null, nested objects recurse, arrays and scalars replace', () => {
    expect(createMergePatch({ a: 1, b: { c: 1, d: 2 }, e: [1] }, { a: 1, b: { c: 3 }, e: [1, 2], f: 'x' })).toEqual({ b: { c: 3, d: null }, e: [1, 2], f: 'x' });
    expect(createMergePatch({ a: 1 }, { a: 1 })).toEqual({});
    expect(createMergePatch(undefined, { a: 1 })).toEqual({ a: 1 });
    expect(createMergePatch({ a: 1 }, undefined)).toBeNull();
  });

  it('createMergePatch round-trips through RFC 7386 mergePatch (packages/schema) on generated documents', () => {
    // deterministic pseudo-random JSON documents without nulls (a merge patch cannot express a null member)
    let seed = 7;
    const rnd = (n: number) => {
      seed = (seed * 1103515245 + 12345) % 2 ** 31;
      return seed % n;
    };
    const gen = (depth: number): unknown => {
      const k = rnd(depth > 2 ? 4 : 6);
      if (k === 0) return rnd(100);
      if (k === 1) return `s${rnd(5)}`;
      if (k === 2) return rnd(2) === 0;
      if (k === 3) return [rnd(3), `x${rnd(3)}`];
      const o: Record<string, unknown> = {};
      for (let i = 0; i < 1 + rnd(4); i++) o[`k${rnd(6)}`] = gen(depth + 1);
      return o;
    };
    for (let i = 0; i < 300; i++) {
      const from = gen(3);
      const to = gen(3);
      if (typeof from !== 'object' || typeof to !== 'object' || Array.isArray(from) || Array.isArray(to)) continue;
      expect(mergePatch(from, createMergePatch(from, to))).toEqual(to);
    }
  });
});

describe('nextList — P08 review N4 for arrays', () => {
  const key = ['username'];
  const alice = { username: 'alice', role: 'operator', fullName: 'A' };
  const bob = { username: 'bob', role: 'readonly' };

  it('applies only the edited members to the item as it is NOW (another session changed fullName meanwhile)', () => {
    const now = [{ ...alice, fullName: 'Alice Elsewhere' }, bob];
    const { list, index } = nextList(now, key, 'alice', alice, { ...alice, role: 'admin' });
    expect(index).toBe(0);
    expect(list).toEqual([{ username: 'alice', role: 'admin', fullName: 'Alice Elsewhere' }, bob]);
  });

  it('a rename finds the item by its original key; a new item and a vanished item are appended', () => {
    expect(nextList([alice, bob], key, 'alice', alice, { ...alice, username: 'ally' }).list[0]).toMatchObject({ username: 'ally' });
    expect(nextList([alice], key, null, undefined, bob)).toEqual({ list: [alice, bob], index: 1 });
    expect(nextList([bob], key, 'alice', alice, alice)).toEqual({ list: [bob, alice], index: 1 });
  });
});

describe('collectionRows', () => {
  it('maps: candidate order with new/changed state, then what the candidate removes', () => {
    const rows = collectionRows(IFACES, { b: { mtu: 1 }, a: { mtu: 2 }, n: {} }, { a: { mtu: 2 }, b: { mtu: 9 }, gone: { mtu: 3 } });
    // a key named like an Object.prototype member is an ordinary key
    expect(collectionRows(IFACES, { constructor: { mtu: 1 } }, {}).map((r) => [r.id, r.state])).toEqual([['constructor', 'new']]);
    expect(rows.map((r) => [r.id, r.state])).toEqual([
      ['b', 'changed'],
      ['a', undefined],
      ['n', 'new'],
      ['gone', 'removed'],
    ]);
    expect(rows[3]).toMatchObject({ value: undefined, running: { mtu: 3 }, index: -1 });
  });

  it('lists: keyed by itemKey, index into the candidate array; empty and missing nodes are no rows', () => {
    const rows = collectionRows(USERS, [{ username: 'x', role: 'admin' }, { username: 'y', role: 'admin' }], [{ username: 'y', role: 'operator' }]);
    expect(rows.map((r) => [r.id, r.index, r.state])).toEqual([
      ['x', 0, 'new'],
      ['y', 1, 'changed'],
    ]);
    expect(collectionRows(USERS, undefined, undefined)).toEqual([]);
    expect(collectionRows(IFACES, [], null)).toEqual([]);
  });
});

describe('problemAt', () => {
  const problem = new ApiError(400, {
    type: 'https://vrx.dev/problems/validation',
    title: 'Validation failed',
    status: 400,
    errors: [
      { pointer: '/interfaces/GigabitEthernet0~18~10/mtu', message: 'too big' },
      { pointer: '/interfaces/GigabitEthernet0~18~10', message: 'item' },
      { pointer: '/interfaces/GigabitEthernet0~18~10x/mtu', message: 'another item with a longer name' },
      { pointer: '/vrfs/red', message: 'elsewhere' },
    ],
  }, false);

  it('makes pointers of the edited item relative (field errors), keeps others absolute (listed on top)', () => {
    const p = problemAt(problem, itemPointer(IFACES, 'GigabitEthernet0/8/0'));
    expect(p?.errors?.map((e) => e.pointer)).toEqual(['/mtu', '', '/interfaces/GigabitEthernet0~18~10x/mtu', '/vrfs/red']);
    expect(p?.errors?.[0]?.detail).toBe('too big');
  });

  it('is null for anything but an API problem', () => {
    expect(problemAt(new Error('x'), '/a')).toBeNull();
    expect(problemAt(null, '/a')).toBeNull();
  });
});

describe('localizeSchema', () => {
  const dict: Record<string, string> = {
    'field.role.title': 'Rolle',
    'field.role.help': 'Hilfe',
    'field.role.enum.admin': 'Verwalter',
  };
  const t = (k: string, o?: Record<string, unknown>) => dict[k] ?? String(o?.['defaultValue'] ?? k);

  it('translates title, help and enum labels from the domain namespace; the schema text stays the fallback', () => {
    const s = localizeSchema(USERS.itemSchema, t);
    const role = s.properties?.['role'];
    expect(role?.title).toBe('Rolle');
    expect(role?.['x-vrx-ui']?.help).toBe('Hilfe');
    expect(role?.['x-vrx-ui']?.['enumLabels']).toEqual({ admin: 'Verwalter', operator: 'operator', readonly: 'readonly' });
    expect(s.properties?.['username']?.title).toBe('Username');
    expect(s.properties?.['username']?.['x-vrx-ui']?.help).toBeUndefined();
  });
});
