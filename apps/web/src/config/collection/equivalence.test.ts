/**
 * The kit's generic helpers against the per-screen ones they were extracted from (P08 interfaces, System › Users), on
 * the same inputs: the proof that moving those screens onto the kit keeps their behaviour. When a screen is migrated
 * and its copy deleted, delete its block here. Deliberate differences are asserted explicitly below.
 */
import { describe, expect, it } from 'vitest';
import { ApiError } from '../../api-problem';
import { problemFor } from '../../domains/interfaces/InterfaceDrawer';
import { pageOf, toRow, type Row } from '../../domains/interfaces/InterfacesPage';
import * as p08 from '../../domains/interfaces/model';
import * as users from '../../pages/UsersPage';
import type { ServerPageRequest } from '@ngfw/ui-kit/data-grid';
import { pageRows } from '../widgets/paging';
import { collectionModel, collectionRows, createMergePatch, itemPointer, localizeSchema, problemAt } from './model';

const IFACES = collectionModel({ domain: 'interfaces', ns: 'interfaces', omit: ['subinterfaces'] });
const SUBS = collectionModel({ domain: 'interfaces', ns: 'interfaces' });
const USERS = collectionModel({ domain: 'management', path: ['users'], ns: 'users' });

const DOCS: [unknown, unknown][] = [
  [{ enabled: true, mtu: 1400, ipv4: ['10.1.1.1/24'], dhcpClient: { hostname: 'a', setBroadcastFlag: false } }, { enabled: true, ipv4: ['10.1.1.1/24', '10.1.3.1/24'], dhcpClient: { setBroadcastFlag: true } }],
  [{ a: { b: { c: 1 } } }, { a: { b: {} } }],
  [undefined, { enabled: false }],
  [{ x: 1 }, undefined],
  [{ x: [1, { y: 2 }] }, { x: [1, { y: 2 }] }],
  [{ x: 'a' }, { x: 'a', y: null }],
];

describe('P08 interfaces ≡ kit', () => {
  it('item and form schema are the same generated schema', () => {
    expect(IFACES.formSchema).toEqual(p08.interfaceFormSchema());
    expect(SUBS.itemSchema).toEqual(p08.interfaceItemSchema());
    expect(collectionModel({ domain: 'interfaces', path: ['x', 'subinterfaces'], ns: 'interfaces' }).itemSchema).toEqual(p08.subinterfaceSchema());
  });

  it('createMergePatch gives the same patches', () => {
    for (const [from, to] of DOCS) expect(createMergePatch(from, to)).toEqual(p08.createMergePatch(from, to));
  });

  it('localizeSchema: same titles, help and groups; enum labels default to the value (what SchemaForm shows anyway)', () => {
    const dict: Record<string, string> = { 'field.mtu.title': 'MTU (bytes)', 'field.mtu.help': 'help', 'group.addressing': 'Adressen' };
    const t = (k: string, o?: Record<string, unknown>) => dict[k] ?? String(o?.['defaultValue'] ?? k);
    const kit = localizeSchema(IFACES.formSchema, t);
    const old = p08.localizeSchema(IFACES.formSchema, t);
    for (const [name, prop] of Object.entries(old.properties ?? {})) {
      const k = kit.properties?.[name];
      expect(k?.title, name).toBe(prop.title);
      const { enumLabels, ...hints } = (k?.['x-vrx-ui'] ?? {}) as Record<string, unknown>;
      expect(hints, name).toEqual(prop['x-vrx-ui']);
      if (enumLabels) for (const [v, l] of Object.entries(enumLabels as Record<string, string>)) expect(l).toBe(v);
    }
  });

  it('problem pointers map the same way — except a longer name with the same prefix, which P08 mis-mapped', () => {
    const err = new ApiError(400, { status: 400, errors: [
      { pointer: '/interfaces/host-w1l0/mtu', message: 'm' },
      { pointer: '/vrfs/red', message: 'v' },
      { pointer: '/interfaces/host-w1l0x/mtu', message: 'other interface' },
    ] }, false);
    const kit = problemAt(err, itemPointer(IFACES, 'host-w1l0'))?.errors ?? [];
    const old = problemFor(err, '/interfaces/host-w1l0')?.errors ?? [];
    expect(kit.slice(0, 2)).toEqual(old.slice(0, 2));
    expect(old[2]?.pointer).toBe('x/mtu'); // P08: a field "x/mtu" of the edited interface (wrong)
    expect(kit[2]?.pointer).toBe('/interfaces/host-w1l0x/mtu'); // kit: another item, listed on top of the form
  });

  it('client-side paging gives the same pages as pageOf', () => {
    const live = (name: string, extra: Record<string, unknown> = {}) => ({ name, vppName: name, swIfIndex: 1, type: 'af-packet', adminUp: true, linkUp: true, mtu: 1500, linkMtu: 1500, mac: '', ipv4: ['10.0.0.1/24'], ipv6: [], vrf: 'default', tableId: 0, rxMode: '', ...extra });
    const items = ['host-w1w0', 'host-w1l0', 'loop10', 'host-w1l1'].map((n, i) => ({
      name: n, kind: 'interface', parent: null, state: live(n, { mtu: 1500 + i * 100, vrf: i % 2 ? 'red' : 'default' }), config: null, running: null, counters: { errors: String(i), drops: '1' }, hasPendingChange: false,
    })) as unknown as p08.InterfaceItem[];
    const rows = items.map(toRow);
    const text = (r: Row) => `${r.name} ${r.type} ${r.addresses} ${r.vrf}`;
    const base: ServerPageRequest = { page: 0, pageSize: 2, sort: [], filter: [], filterLogic: 'and', quickFilter: [] };
    const reqs: ServerPageRequest[] = [
      base,
      { ...base, page: 1 },
      { ...base, pageSize: 25, sort: [{ field: 'name', dir: 'asc' }] },
      { ...base, pageSize: 25, sort: [{ field: 'mtu', dir: 'desc' }] },
      { ...base, pageSize: 25, sort: [{ field: 'vrf', dir: 'asc' }, { field: 'name', dir: 'desc' }] },
      { ...base, pageSize: 25, quickFilter: ['red'] },
      { ...base, pageSize: 25, quickFilter: ['host', 'w1l'] },
      { ...base, pageSize: 25, filter: [{ field: 'name', operator: 'contains', value: 'W1' }] },
    ];
    for (const req of reqs) expect(pageRows(rows, req, text)).toEqual(pageOf(rows, req));
  });
});

describe('System › Users ≡ kit', () => {
  it('item schema is the same generated schema', () => {
    expect(USERS.itemSchema).toEqual(users.userItemSchema());
  });

  it('localizeSchema: same titles, help and enum labels as localizeUserSchema', () => {
    const dict: Record<string, string> = { 'field.role.title': 'Rolle', 'field.role.enum.admin': 'Verwalter', 'field.passwordHash.help': 'nie' };
    const t = (k: string, o?: Record<string, unknown>) => dict[k] ?? String(o?.['defaultValue'] ?? k);
    expect(localizeSchema(USERS.itemSchema, t)).toEqual(users.localizeUserSchema(USERS.itemSchema, t));
  });

  it('row states match rowStates (new / changed / removed by username)', () => {
    const cand = [
      { username: 'a', role: 'admin' },
      { username: 'b', role: 'operator', fullName: 'B' },
      { username: 'c', role: 'readonly' },
    ] as users.ConfigUser[];
    const run = [
      { username: 'b', role: 'operator' },
      { username: 'c', role: 'readonly' },
      { username: 'd', role: 'admin' },
    ] as users.ConfigUser[];
    const old = users.rowStates(cand, run).map((r) => [r.user.username, r.index, r.state]);
    const kit = collectionRows(USERS, cand, run).map((r) => [r.id, r.index, r.state]);
    expect(kit).toEqual(old);
  });

  it('problem pointers of the edited index map like problemForItem', () => {
    const err = new ApiError(400, { status: 400, errors: [
      { pointer: '/management/users/1/role', message: 'r' },
      { pointer: '/management/users/1', message: 'item' },
      { pointer: '/management/users/0/role', message: 'other user' },
    ] }, false);
    expect(problemAt(err, itemPointer(USERS, 1))).toEqual(users.problemForItem(err, 1));
  });
});
