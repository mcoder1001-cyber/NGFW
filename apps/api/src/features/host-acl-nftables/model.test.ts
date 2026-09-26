import type { HostAclStateResponse } from '@ngfw/proto';
import { describe, expect, it } from 'vitest';
import { FakeAgent } from '../../testing/fake-agent.js';
import { fakeHostAclTable, setFakeHostAclCounters } from './fake.js';
import { aggregateRuleCounters, HostAclStateOut, hostAclStateJson } from './model.js';

const rule = (over: Partial<HostAclStateResponse['chains'][number]['rules'][number]>) => ({
  kind: 'rule',
  list: 'mgmt',
  sequence: 10,
  pointer: '/acl/host/mgmt/rules/0',
  text: 'tcp dport 22 accept',
  verdict: 'accept',
  comment: 'vrx:mgmt:10/0:0123abcd',
  packets: '0',
  bytes: '0',
  ...over,
});

const response = (): HostAclStateResponse => ({
  owner: 'w9',
  retrievedAt: new Date('2026-09-24T12:00:00Z'),
  table: 'vrx_w9',
  mode: 'netns',
  present: true,
  inSync: true,
  sets: [{ name: 'a4_admins', type: 'ipv4_addr', object: 'admins', elements: ['10.0.0.0/24'] }],
  chains: [
    {
      name: 'in_mgmt',
      hook: 'input',
      priority: 0,
      policy: 'accept',
      list: 'mgmt',
      rules: [
        rule({
          kind: 'established',
          list: '',
          sequence: 0,
          pointer: '',
          comment: 'vrx:pre:ct',
          packets: '99',
        }),
        // one host rule rendered to an IPv4 and an IPv6 kernel rule
        rule({ packets: '9007199254740993', bytes: '18446744073709551615' }),
        rule({ comment: 'vrx:mgmt:10/1:0123abcd', packets: '2', bytes: '0' }),
        rule({
          sequence: 5,
          pointer: '/acl/host/mgmt/rules/1',
          comment: 'vrx:mgmt:5/0:aa',
          packets: '1',
          bytes: '60',
        }),
        rule({ kind: 'bogus', list: '', sequence: 0, pointer: '', comment: '', packets: 'x' }),
      ],
    },
    {
      name: 'out_mgmt',
      hook: 'output',
      priority: 10,
      policy: 'accept',
      list: 'mgmt',
      rules: [rule({ comment: 'vrx:mgmt:10/2:0123abcd', packets: '1', bytes: '40' })],
    },
  ],
});

describe('host ACL state JSON (F-host-acl-nftables)', () => {
  it('sums the kernel rules of one configuration rule across families and chains, exactly (uint64 strings)', () => {
    const j = hostAclStateJson(response());
    expect(j.rules).toEqual([
      {
        list: 'mgmt',
        sequence: 5,
        pointer: '/acl/host/mgmt/rules/1',
        packets: '1',
        bytes: '60',
        nftRules: 1,
      },
      {
        list: 'mgmt',
        sequence: 10,
        pointer: '/acl/host/mgmt/rules/0',
        packets: '9007199254740996',
        bytes: '18446744073709551655',
        nftRules: 3,
      },
    ]);
  });

  it('maps unknown kinds to "unknown", bad counters to "0" and validates against the OpenAPI schema', () => {
    const j = hostAclStateJson(response());
    const last = j.chains[0]!.rules.at(-1)!;
    expect(last).toMatchObject({ kind: 'unknown', packets: '0' });
    expect(j.retrievedAt).toBe('2026-09-24T12:00:00.000Z');
    expect(HostAclStateOut.safeParse(j).success).toBe(true);
  });

  it('an empty table aggregates to nothing', () => {
    expect(aggregateRuleCounters([])).toEqual([]);
  });

  it('the fake derives chains and rules from the applied acl.host* and reports the counters a test sets', () => {
    const agent = new FakeAgent({ owner: 'w9' });
    agent.current = {
      acl: {
        host: {
          mgmt: {
            rules: [
              { sequence: 20, action: 'drop' },
              { sequence: 10, action: 'accept' },
              { sequence: 30, action: 'reject', enabled: false },
            ],
          },
        },
        hostAttachments: [
          { list: 'mgmt', chain: 'input', priority: -10, enabled: true },
          { list: 'mgmt', chain: 'output', priority: 0, enabled: false },
        ],
        hostSettings: { defaultInput: 'drop' },
      },
    };
    setFakeHostAclCounters(agent, { 'mgmt:20': { packets: 3, bytes: 180 } });
    const t = fakeHostAclTable(agent);
    expect(t).toMatchObject({ table: 'vrx_w9', mode: 'check', present: false, inSync: true });
    expect(t.chains).toHaveLength(1);
    expect(t.chains[0]).toMatchObject({
      name: 'in_mgmt',
      hook: 'input',
      priority: -10,
      policy: 'drop',
    });
    expect(t.chains[0]!.rules.map((r) => [r.sequence, r.verdict, r.pointer, r.packets])).toEqual([
      [10, 'accept', '/acl/host/mgmt/rules/1', '0'],
      [20, 'drop', '/acl/host/mgmt/rules/0', '3'],
    ]);
  });
});
