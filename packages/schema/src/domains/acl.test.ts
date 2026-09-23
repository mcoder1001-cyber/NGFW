import { describe, expect, it } from 'vitest';
import { z } from 'zod';
import {
  AclAttachmentSchema,
  AclRuleSchema,
  AclSchema,
  AddressMatchSchema,
  HostAttachmentSchema,
  HostRuleSchema,
  linuxInterfaceName,
  MacipAttachmentSchema,
  MacipRuleSchema,
  ruleSequence,
  ServiceMatchSchema,
} from './acl.js';

const ok = (schema: z.ZodType, value: unknown): void =>
  expect(schema.safeParse(value).success, JSON.stringify(value)).toBe(true);
const bad = (schema: z.ZodType, value: unknown): void =>
  expect(schema.safeParse(value).success, JSON.stringify(value)).toBe(false);

describe('AclSchema', () => {
  it('accepts {} and yields empty records and lists', () => {
    expect(AclSchema.parse({})).toEqual({
      lists: {},
      macip: {},
      host: {},
      attachments: [],
      macipAttachments: [],
      hostAttachments: [],
    });
  });

  it('is strict and validates list names', () => {
    ok(AclSchema, { lists: { 'lan-in': {} } });
    expect(AclSchema.parse({ lists: { l: {} } }).lists.l).toEqual({ tags: [], rules: [] });
    bad(AclSchema, { lists: { 'lan in': {} } });
    bad(AclSchema, { lists: [] });
    bad(AclSchema, { rules: [] });
    bad(AclSchema, { lists: { l: { rules: {} } } });
  });

  it('exposes UI hints', () => {
    const js = z.toJSONSchema(AclSchema, { target: 'draft-2020-12', io: 'input' });
    expect(js.title).toBe('ACL');
    expect(js['x-vrx-ui']).toMatchObject({ order: 80 });
    const props = js.properties as Record<string, Record<string, unknown>>;
    expect(props.attachments?.['x-vrx-ui']).toMatchObject({ group: 'Attachments' });
  });
});

describe('match primitives', () => {
  it('ruleSequence / linuxInterfaceName', () => {
    ok(ruleSequence, 1);
    ok(ruleSequence, 2 ** 31 - 1);
    bad(ruleSequence, 0);
    bad(ruleSequence, 2 ** 31);
    bad(ruleSequence, 1.5);
    bad(ruleSequence, '10');
    ok(linuxInterfaceName, 'eth0');
    ok(linuxInterfaceName, 'a'.repeat(15));
    bad(linuxInterfaceName, 'a'.repeat(16));
    bad(linuxInterfaceName, '');
    bad(linuxInterfaceName, 'eth 0');
    bad(linuxInterfaceName, 'eth0/1');
  });

  it('AddressMatchSchema / ServiceMatchSchema discriminate on kind', () => {
    ok(AddressMatchSchema, { kind: 'any' });
    ok(AddressMatchSchema, { kind: 'prefix', prefix: '10.0.0.0/8' });
    ok(AddressMatchSchema, { kind: 'prefix', prefix: '2001:db8::/32' });
    ok(AddressMatchSchema, { kind: 'object', name: 'web-servers' });
    bad(AddressMatchSchema, { kind: 'any', prefix: '10.0.0.0/8' });
    bad(AddressMatchSchema, { kind: 'prefix', prefix: '10.0.0.1' });
    bad(AddressMatchSchema, { kind: 'object' });
    bad(AddressMatchSchema, { kind: 'object', name: 'a b' });
    bad(AddressMatchSchema, { kind: 'zone', zone: 'lan' });
    bad(AddressMatchSchema, 'any');
    ok(ServiceMatchSchema, { kind: 'any' });
    ok(ServiceMatchSchema, { kind: 'object', name: 'web' });
    ok(ServiceMatchSchema, { kind: 'inline', spec: { protocol: 'tcp', destinationPorts: ['80'] } });
    bad(ServiceMatchSchema, {
      kind: 'inline',
      spec: { protocol: 'tcp', description: 'no bookkeeping inline' },
    });
    bad(ServiceMatchSchema, { kind: 'inline' });
    bad(ServiceMatchSchema, { kind: 'object', spec: { protocol: 'any' } });
  });
});

describe('rules', () => {
  it('AclRuleSchema defaults and rejections', () => {
    expect(AclRuleSchema.parse({ sequence: 10, action: 'permit' })).toEqual({
      sequence: 10,
      action: 'permit',
      enabled: true,
      ipVersion: 'any',
      source: { kind: 'any' },
      destination: { kind: 'any' },
      service: { kind: 'any' },
      log: false,
    });
    ok(AclRuleSchema, {
      sequence: 1,
      action: 'reflect',
      schedule: 'business-hours',
      ipVersion: 'ipv6',
    });
    bad(AclRuleSchema, { sequence: 0, action: 'permit' });
    bad(AclRuleSchema, { action: 'permit' });
    bad(AclRuleSchema, { sequence: 1 });
    bad(AclRuleSchema, { sequence: 1, action: 'allow' });
    bad(AclRuleSchema, { sequence: 1, action: 'accept' });
    bad(AclRuleSchema, { sequence: 1, action: 'permit', ipVersion: 'v4' });
    bad(AclRuleSchema, { sequence: 1, action: 'permit', source: '10.0.0.0/8' });
    bad(AclRuleSchema, { sequence: 1, action: 'permit', schedule: 'bad name' });
    bad(AclRuleSchema, { sequence: 1, action: 'permit', log: 'yes' });
    bad(AclRuleSchema, { sequence: 1, action: 'permit', priority: 1 });
    bad(AclRuleSchema, { sequence: 1, action: 'permit', description: 'x'.repeat(256) });
  });

  it('MacipRuleSchema', () => {
    expect(
      MacipRuleSchema.parse({ sequence: 1, action: 'permit', sourceMac: 'aa:bb:cc:dd:ee:ff' }),
    ).toEqual({
      sequence: 1,
      action: 'permit',
      sourceMac: 'aa:bb:cc:dd:ee:ff',
      sourceMacMask: 'ff:ff:ff:ff:ff:ff',
    });
    ok(MacipRuleSchema, {
      sequence: 1,
      action: 'deny',
      sourceMac: 'AA-BB-CC-DD-EE-FF',
      sourceMacMask: 'ff:ff:ff:00:00:00',
      sourcePrefix: '10.0.0.0/8',
    });
    bad(MacipRuleSchema, { sequence: 1, action: 'permit', sourceMac: 'aa:bb:cc' });
    bad(MacipRuleSchema, { sequence: 1, action: 'permit' });
    bad(MacipRuleSchema, { sequence: 1, action: 'reflect', sourceMac: 'aa:bb:cc:dd:ee:ff' });
    bad(MacipRuleSchema, {
      sequence: 1,
      action: 'permit',
      sourceMac: 'aa:bb:cc:dd:ee:ff',
      sourcePrefix: '10.0.0.1',
    });
    bad(MacipRuleSchema, {
      sequence: 1,
      action: 'permit',
      sourceMac: 'aa:bb:cc:dd:ee:ff',
      service: { kind: 'any' },
    });
  });

  it('HostRuleSchema uses nftables verdicts', () => {
    expect(HostRuleSchema.parse({ sequence: 1, action: 'accept' })).toMatchObject({
      enabled: true,
      ipVersion: 'any',
      log: false,
    });
    ok(HostRuleSchema, { sequence: 1, action: 'reject', interface: 'eth0' });
    ok(HostRuleSchema, {
      sequence: 1,
      action: 'drop',
      service: { kind: 'inline', spec: { protocol: 'icmp' } },
    });
    bad(HostRuleSchema, { sequence: 1, action: 'permit' });
    bad(HostRuleSchema, { sequence: 1, action: 'accept', interface: 'a'.repeat(16) });
    bad(HostRuleSchema, { sequence: 1, action: 'accept', schedule: 'x' });
  });
});

describe('attachments', () => {
  const target = { kind: 'interface', interface: 'TenGigabitEthernet0/0/0' };

  it('AclAttachmentSchema', () => {
    expect(AclAttachmentSchema.parse({ list: 'l', target, sequence: 1 })).toEqual({
      list: 'l',
      target,
      direction: 'in',
      sequence: 1,
      enabled: true,
    });
    ok(AclAttachmentSchema, {
      list: 'l',
      target: { kind: 'zone', zone: 'lan' },
      direction: 'out',
      sequence: 5,
      vrf: 'default',
    });
    bad(AclAttachmentSchema, { list: 'l', target, direction: 'inbound', sequence: 1 });
    bad(AclAttachmentSchema, { list: 'l', target });
    bad(AclAttachmentSchema, {
      list: 'l',
      target: { kind: 'interface', interface: 'bad name' },
      sequence: 1,
    });
    bad(AclAttachmentSchema, { list: 'l', target: { kind: 'zone' }, sequence: 1 });
    bad(AclAttachmentSchema, { list: 'l', target: 'TenGigabitEthernet0/0/0', sequence: 1 });
    bad(AclAttachmentSchema, { list: 'l', target, sequence: 1, vrf: 'a b' });
    bad(AclAttachmentSchema, { list: 'l', target, sequence: 1, interface: 'x' });
  });

  it('MacipAttachmentSchema / HostAttachmentSchema', () => {
    expect(MacipAttachmentSchema.parse({ list: 'm', interface: 'loop0' })).toEqual({
      list: 'm',
      interface: 'loop0',
      enabled: true,
    });
    bad(MacipAttachmentSchema, { list: 'm' });
    bad(MacipAttachmentSchema, { list: 'm', interface: 'loop0', direction: 'in' });
    expect(HostAttachmentSchema.parse({ list: 'h', chain: 'input' })).toEqual({
      list: 'h',
      chain: 'input',
      priority: 0,
      enabled: true,
    });
    ok(HostAttachmentSchema, { list: 'h', chain: 'forward', priority: -500 });
    ok(HostAttachmentSchema, { list: 'h', chain: 'output', priority: 500 });
    bad(HostAttachmentSchema, { list: 'h', chain: 'prerouting' });
    bad(HostAttachmentSchema, { list: 'h', chain: 'input', priority: -501 });
    bad(HostAttachmentSchema, { list: 'h', chain: 'input', priority: 501 });
    bad(HostAttachmentSchema, { list: 'h', chain: 'input', priority: 0.5 });
    bad(HostAttachmentSchema, { list: 'h' });
  });
});
