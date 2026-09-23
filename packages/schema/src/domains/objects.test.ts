import { describe, expect, it } from 'vitest';
import { z } from 'zod';
import {
  AddressGroupSchema,
  AddressObjectSchema,
  hexColor,
  ipPrefix,
  l4PortNumber,
  l4PortRange,
  ObjectsSchema,
  parsePortRange,
  ScheduleSchema,
  ServiceObjectSchema,
  ServiceSpecSchema,
  TagSchema,
  timeOfDay,
  WEEKDAYS,
  ZoneSchema,
} from './objects.js';

const ok = (schema: z.ZodType, value: unknown): void =>
  expect(schema.safeParse(value).success, JSON.stringify(value)).toBe(true);
const bad = (schema: z.ZodType, value: unknown): void =>
  expect(schema.safeParse(value).success, JSON.stringify(value)).toBe(false);

describe('ObjectsSchema', () => {
  it('accepts {} and yields seven empty records', () => {
    expect(ObjectsSchema.parse({})).toEqual({
      addresses: {},
      addressGroups: {},
      services: {},
      serviceGroups: {},
      schedules: {},
      zones: {},
      tags: {},
    });
  });

  it('validates record keys as object names (unique per kind, never global)', () => {
    const host = { type: 'host', address: '10.0.0.1' };
    ok(ObjectsSchema, { addresses: { 'web-1': host, 'web.v2_x': host, constructor: host } });
    bad(ObjectsSchema, { addresses: { 'bad name': host } });
    bad(ObjectsSchema, { addresses: { '': host } });
    bad(ObjectsSchema, { addresses: { '-x': host } });
    bad(ObjectsSchema, { addresses: { 'a/b': host } });
    bad(ObjectsSchema, { addresses: { ['a'.repeat(64)]: host } });
    ok(ObjectsSchema, { addresses: { ['a'.repeat(63)]: host } });
    bad(ObjectsSchema, { addresses: [host] });
    bad(ObjectsSchema, { addressGroups: { g: { members: ['x'] } }, groups: {} });
    // the same name may exist in different kinds at schema level (objects.names-disjoint is semantic)
    ok(ObjectsSchema, { addresses: { web: host }, services: { web: { protocol: 'any' } } });
  });

  it('exposes UI hints and key patterns in JSON Schema', () => {
    const js = z.toJSONSchema(ObjectsSchema, { target: 'draft-2020-12', io: 'input' });
    expect(js.title).toBe('Objects');
    expect(js['x-vrx-ui']).toMatchObject({ order: 70 });
    const props = js.properties as Record<string, Record<string, unknown>>;
    expect(props.addresses?.['x-vrx-ui']).toMatchObject({ group: 'Addresses', order: 1 });
    expect(props.addresses?.propertyNames).toMatchObject({ pattern: expect.any(String) });
  });
});

describe('local primitives', () => {
  it('l4PortNumber / l4PortRange / parsePortRange', () => {
    ok(l4PortNumber, 1);
    ok(l4PortNumber, 65535);
    bad(l4PortNumber, 0);
    bad(l4PortNumber, 65536);
    bad(l4PortNumber, 80.5);
    bad(l4PortNumber, '80');
    for (const v of ['1', '443', '65535', '8000-8080', '1-65535', '80-80']) ok(l4PortRange, v);
    for (const v of [
      '',
      '0',
      '65536',
      '70000',
      '80-70',
      '80-',
      '-80',
      ' 80',
      '80 ',
      '80-90-100',
      'http',
      '0x50',
      '8080-70000',
    ])
      bad(l4PortRange, v);
    expect(parsePortRange('443')).toEqual({ from: 443, to: 443 });
    expect(parsePortRange('8000-8080')).toEqual({ from: 8000, to: 8080 });
    expect(parsePortRange('x')).toBeUndefined();
    expect(parsePortRange('1-2-3')).toBeUndefined();
  });

  it('ipPrefix / timeOfDay / hexColor', () => {
    ok(ipPrefix, '10.0.0.0/24');
    ok(ipPrefix, '2001:db8::/32');
    ok(ipPrefix, '0.0.0.0/0');
    bad(ipPrefix, '10.0.0.1');
    bad(ipPrefix, '10.0.0.0/33');
    bad(ipPrefix, '2001:db8::/129');
    bad(ipPrefix, '');
    for (const v of ['00:00', '09:30', '23:59']) ok(timeOfDay, v);
    for (const v of ['24:00', '9:00', '09:60', '09:00:00', '0900', '', ' 09:00']) bad(timeOfDay, v);
    ok(hexColor, '#1e88e5');
    ok(hexColor, '#ABCDEF');
    for (const v of ['#abc', '1e88e5', 'red', '#1e88e5ff', '#ggggggg', '']) bad(hexColor, v);
    expect(WEEKDAYS).toEqual(['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun']);
  });
});

describe('AddressObjectSchema / AddressGroupSchema', () => {
  it('discriminates on type and rejects mixed or malformed shapes', () => {
    expect(AddressObjectSchema.parse({ type: 'host', address: '10.0.0.1' })).toEqual({
      type: 'host',
      address: '10.0.0.1',
      tags: [],
    });
    ok(AddressObjectSchema, {
      type: 'host',
      address: '2001:db8::1',
      description: 'v6',
      tags: ['a'],
    });
    ok(AddressObjectSchema, { type: 'network', prefix: '10.0.0.0/8' });
    ok(AddressObjectSchema, { type: 'range', start: '10.0.0.1', end: '10.0.0.9' });
    ok(AddressObjectSchema, { type: 'fqdn', fqdn: 'updates.example.net' });
    bad(AddressObjectSchema, { type: 'host', address: '10.0.0.1/32' });
    bad(AddressObjectSchema, { type: 'host', address: 'localhost' });
    bad(AddressObjectSchema, { type: 'network', prefix: '10.0.0.1' });
    bad(AddressObjectSchema, { type: 'network', address: '10.0.0.0/8' });
    bad(AddressObjectSchema, { type: 'range', start: '10.0.0.1' });
    bad(AddressObjectSchema, { type: 'fqdn', fqdn: '-bad.example' });
    bad(AddressObjectSchema, { type: 'fqdn', fqdn: 'a..b' });
    bad(AddressObjectSchema, { type: 'mac', address: 'aa:bb:cc:dd:ee:ff' });
    bad(AddressObjectSchema, { type: 'host', address: '10.0.0.1', prefix: '10.0.0.0/8' });
    bad(AddressObjectSchema, { type: 'host', address: '10.0.0.1', tags: ['bad tag'] });
    bad(AddressObjectSchema, {
      type: 'host',
      address: '10.0.0.1',
      tags: Array<string>(33).fill('t'),
    });
    bad(AddressObjectSchema, { address: '10.0.0.1' });
    bad(AddressObjectSchema, 'host');
    ok(AddressGroupSchema, { members: ['a'] });
    bad(AddressGroupSchema, { members: [] });
    bad(AddressGroupSchema, { members: ['a b'] });
    bad(AddressGroupSchema, {});
    bad(AddressGroupSchema, { members: ['a'], interfaces: [] });
  });
});

describe('ServiceObjectSchema / ServiceSpecSchema', () => {
  it('discriminates on protocol and keeps ports/flags/icmp fields per variant', () => {
    expect(ServiceObjectSchema.parse({ protocol: 'tcp' })).toEqual({
      protocol: 'tcp',
      destinationPorts: [],
      sourcePorts: [],
      tags: [],
    });
    ok(ServiceObjectSchema, {
      protocol: 'tcp',
      destinationPorts: ['80', '8000-8080'],
      tcpFlags: { mask: 18, value: 2 },
    });
    ok(ServiceObjectSchema, { protocol: 'tcp-udp', destinationPorts: ['53'] });
    ok(ServiceObjectSchema, { protocol: 'udp', sourcePorts: ['1024-65535'] });
    ok(ServiceObjectSchema, { protocol: 'sctp', destinationPorts: ['3868'] });
    ok(ServiceObjectSchema, { protocol: 'icmp', type: 8, code: 0 });
    ok(ServiceObjectSchema, { protocol: 'icmp6' });
    ok(ServiceObjectSchema, { protocol: 'any', description: 'everything' });
    ok(ServiceObjectSchema, { protocol: 'other', number: 47 });
    bad(ServiceObjectSchema, { protocol: 'tcp', destinationPorts: ['0'] });
    bad(ServiceObjectSchema, { protocol: 'tcp', destinationPorts: ['65536'] });
    bad(ServiceObjectSchema, { protocol: 'tcp', destinationPorts: ['80-70'] });
    bad(ServiceObjectSchema, { protocol: 'tcp', destinationPorts: [80] });
    bad(ServiceObjectSchema, { protocol: 'tcp', destinationPorts: Array<string>(65).fill('80') });
    bad(ServiceObjectSchema, { protocol: 'tcp', tcpFlags: { mask: 256, value: 0 } });
    bad(ServiceObjectSchema, { protocol: 'tcp', tcpFlags: { mask: 2 } });
    bad(ServiceObjectSchema, { protocol: 'udp', tcpFlags: { mask: 2, value: 2 } });
    bad(ServiceObjectSchema, { protocol: 'icmp', type: 256 });
    bad(ServiceObjectSchema, { protocol: 'icmp', code: -1 });
    bad(ServiceObjectSchema, { protocol: 'icmp', destinationPorts: ['80'] });
    bad(ServiceObjectSchema, { protocol: 'other' });
    bad(ServiceObjectSchema, { protocol: 'other', number: 256 });
    bad(ServiceObjectSchema, { protocol: 'any', destinationPorts: ['80'] });
    bad(ServiceObjectSchema, { protocol: 'ip' });
    bad(ServiceObjectSchema, { protocol: 'TCP' });
    bad(ServiceObjectSchema, {});
  });

  it('ServiceSpecSchema is the inline variant without description/tags', () => {
    ok(ServiceSpecSchema, { protocol: 'tcp', destinationPorts: ['443'] });
    bad(ServiceSpecSchema, { protocol: 'tcp', description: 'x' });
    bad(ServiceSpecSchema, { protocol: 'any', tags: [] });
    ok(ServiceObjectSchema, { protocol: 'any', tags: [] });
  });
});

describe('ScheduleSchema / ZoneSchema / TagSchema', () => {
  it('schedules: recurring HH:MM windows and RFC 3339 one-time windows with offset', () => {
    ok(ScheduleSchema, { type: 'recurring', days: ['mon', 'fri'], start: '08:00', end: '18:00' });
    ok(ScheduleSchema, {
      type: 'once',
      start: '2026-10-01T00:00:00Z',
      end: '2026-10-02T00:00:00+03:30',
    });
    bad(ScheduleSchema, { type: 'recurring', days: [], start: '08:00', end: '18:00' });
    bad(ScheduleSchema, { type: 'recurring', days: ['monday'], start: '08:00', end: '18:00' });
    bad(ScheduleSchema, {
      type: 'recurring',
      days: Array<string>(8).fill('mon'),
      start: '08:00',
      end: '18:00',
    });
    bad(ScheduleSchema, { type: 'recurring', days: ['mon'], start: '8:00', end: '18:00' });
    bad(ScheduleSchema, { type: 'recurring', days: ['mon'], start: '08:00' });
    bad(ScheduleSchema, {
      type: 'once',
      start: '2026-10-01T00:00:00',
      end: '2026-10-02T00:00:00Z',
    });
    bad(ScheduleSchema, { type: 'once', start: '2026-10-01', end: '2026-10-02' });
    bad(ScheduleSchema, { type: 'once', start: 'tomorrow', end: 'later' });
    bad(ScheduleSchema, { type: 'weekly', days: ['mon'], start: '08:00', end: '18:00' });
    bad(ScheduleSchema, {
      type: 'recurring',
      days: ['mon'],
      start: '08:00',
      end: '18:00',
      timezone: 'UTC',
    });
  });

  it('zones and tags', () => {
    expect(ZoneSchema.parse({})).toEqual({ interfaces: [], tags: [] });
    ok(ZoneSchema, {
      interfaces: ['TenGigabitEthernet0/0/0', 'loop0'],
      description: 'x',
      tags: ['t'],
    });
    bad(ZoneSchema, { interfaces: ['Gig 0'] });
    bad(ZoneSchema, { interfaces: 'Gig0/0/0' });
    bad(ZoneSchema, { members: [] });
    expect(TagSchema.parse({})).toEqual({});
    ok(TagSchema, { description: 'prod', color: '#1e88e5' });
    bad(TagSchema, { color: 'red' });
    bad(TagSchema, { tags: [] });
  });
});

type JsonNode = Record<string, unknown>;

/** JSON pointers of every node that carries `x-vrx-ui` and an address/prefix `format` but no `widget` (review H1). */
function leavesWithoutWidget(root: unknown): string[] {
  const out: string[] = [];
  const walk = (n: unknown, path: string): void => {
    if (n === null || typeof n !== 'object') return;
    if (Array.isArray(n)) {
      n.forEach((x, i) => walk(x, `${path}/${String(i)}`));
      return;
    }
    const node = n as JsonNode;
    const ui = node['x-vrx-ui'] as { widget?: string } | undefined;
    const anyOf = node.anyOf as JsonNode[] | undefined;
    const formatted =
      typeof node.format === 'string' ||
      (Array.isArray(anyOf) && anyOf.every((x) => typeof x.format === 'string'));
    if (ui !== undefined && formatted && ui.widget === undefined) out.push(path);
    for (const [k, v] of Object.entries(node)) walk(v, `${path}/${k}`);
  };
  walk(root, '');
  return out;
}

const at = (root: unknown, pointer: string): JsonNode =>
  pointer
    .split('/')
    .slice(1)
    .reduce<unknown>((n, k) => (n as JsonNode)[k], root) as JsonNode;

describe('objects leaf UI hints survive re-wrapping (review H1)', () => {
  const js = z.toJSONSchema(ObjectsSchema, { target: 'draft-2020-12', io: 'input' });

  it('keeps widget/help of re-wrapped primitives on leaf fields', () => {
    expect(
      at(js, '/properties/addresses/additionalProperties/oneOf/0/properties/address')['x-vrx-ui'],
      '/properties/addresses/additionalProperties/oneOf/0/properties/address',
    ).toMatchObject({ widget: 'ip' });
    expect(
      at(js, '/properties/addresses/additionalProperties/oneOf/1/properties/prefix')['x-vrx-ui'],
      '/properties/addresses/additionalProperties/oneOf/1/properties/prefix',
    ).toMatchObject({ widget: 'cidr', help: expect.any(String) });
    expect(
      at(js, '/properties/addresses/additionalProperties/oneOf/2/properties/start')['x-vrx-ui'],
      '/properties/addresses/additionalProperties/oneOf/2/properties/start',
    ).toMatchObject({ widget: 'ip' });
  });

  it('every address/prefix leaf with UI hints has a widget', () => {
    expect(leavesWithoutWidget(js)).toEqual([]);
  });
});
